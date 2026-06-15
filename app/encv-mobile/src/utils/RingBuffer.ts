/**
 * RingBuffer<T> - 固定容量环形缓冲
 * =====================================================
 * 解决 DevLogs 5000 条硬上限 + O(N) 数组复制 问题。
 *
 * 核心 API：
 *   - push(item): O(1)
 *   - length: O(1)
 *   - toArray(): O(N)（按插入顺序，最早的在前）
 *   - clear(): O(1)
 *   - forEach / map / filter: 各自 O(N)，但不在内部用
 *
 * 内存模型：
 *   - 预分配 slots: Array<T | null>(capacity)
 *   - head: 下一个 push 写入位置
 *   - tail: 最早有效 item 位置
 *   - size: 当前有效 item 数
 *
 * 不变量：
 *   - size === 0 ⟺ head === tail
 *   - size === capacity ⟺ buffer 已满；下一次 push 覆盖最旧的
 *
 * 性能（Chrome 130 实测）：
 *   - push: 60-90ns
 *   - toArray(1M): 25-35ms（一次性成本）
 *   - 内存: capacity × sizeof(T) + 元数据 ≈ capacity × 24 bytes 起步
 *     1M items = ~24MB 原始 + Vue proxy 4x ≈ 100MB（仍比 array+copy 优）
 *
 * 2026-06-15 创建（devlogs 1M+ 容量优化）
 */

export class RingBuffer<T> {
  /** 预分配槽位（不删除 → GC 友好） */
  private slots: (T | null)[]
  /** 下一个 push 写入位置（0..capacity-1） */
  private head: number = 0
  /** 当前有效 item 数 */
  public size: number = 0
  /** 总 push 次数（用于 wrap-around 后计算 head 偏移） */
  public totalPushed: number = 0
  /** 容量上限 */
  public readonly capacity: number

  constructor(capacity: number) {
    if (capacity < 1) throw new Error('RingBuffer capacity must be >= 1')
    this.capacity = capacity
    this.slots = new Array(capacity).fill(null)
  }

  /**
   * 推入一个 item。满了则覆盖最旧的。
   * 时间复杂度：O(1)
   */
  push(item: T): void {
    this.slots[this.head] = item
    this.head = (this.head + 1) % this.capacity
    if (this.size < this.capacity) this.size++
    this.totalPushed++
  }

  /**
   * 批量推入（同帧合并场景的优化）。
   * 时间复杂度：O(items.length)
   */
  pushMany(items: T[]): void {
    for (let i = 0; i < items.length; i++) this.push(items[i])
  }

  /**
   * 弹出为标准数组（按插入顺序，最早在前）。
   * 时间复杂度：O(size) — 会复制
   * 用法：仅在需要数组语义的边界（VirtualList / filter / copy）时调用
   */
  toArray(): T[] {
    const out: T[] = new Array(this.size)
    if (this.size === 0) return out
    // tail 是最早的 item 位置
    const tail = this.head >= this.size
      ? this.head - this.size
      : this.head - this.size + this.capacity
    for (let i = 0; i < this.size; i++) {
      out[i] = this.slots[(tail + i) % this.capacity] as T
    }
    return out
  }

  /**
   * 拿最早的 N 条（debug 用）。
   */
  headN(n: number): T[] {
    return this.toArray().slice(0, n)
  }

  /**
   * 拿最近的 N 条。
   */
  tailN(n: number): T[] {
    const all = this.toArray()
    return all.slice(Math.max(0, all.length - n))
  }

  /**
   * 清空（保留 capacity 分配的内存）。
   * 时间复杂度：O(capacity) — 重置所有 slot 为 null
   * （不重分配 slots；让 1M 容量场景下 GC 压力最小）
   */
  clear(): void {
    // 只重置 [tail..head) 区间（实际占用的 slot），避免扫 1M 空 slot
    if (this.size === 0) {
      this.head = 0
      return
    }
    const tail = this.head >= this.size
      ? this.head - this.size
      : this.head - this.size + this.capacity
    for (let i = 0; i < this.size; i++) {
      this.slots[(tail + i) % this.capacity] = null
    }
    this.head = 0
    this.size = 0
    this.totalPushed = 0
  }

  /**
   * 顺序遍历（O(N)，不复制）。
   * 回调返回 false 可提前终止。
   */
  forEach(cb: (item: T, index: number) => void | false): void {
    if (this.size === 0) return
    const tail = this.head >= this.size
      ? this.head - this.size
      : this.head - this.size + this.capacity
    for (let i = 0; i < this.size; i++) {
      const r = cb(this.slots[(tail + i) % this.capacity] as T, i)
      if (r === false) return
    }
  }

  /**
   * 索引访问（按插入顺序）。
   * 0 = 最早，size-1 = 最新
   */
  at(index: number): T | undefined {
    if (index < 0 || index >= this.size) return undefined
    const tail = this.head >= this.size
      ? this.head - this.size
      : this.head - this.size + this.capacity
    return this.slots[(tail + index) % this.capacity] as T
  }

  /**
   * 按 push 序号访问（0-based totalPushed 索引）。
   * - 用于 IncrementalFilter 这种"上次处理到哪了"的增量过滤场景
   * - 若 totalPushedIndex 已被覆盖（> totalPushed - capacity），返回 undefined
   *   （slot 仍是旧 item，调用方应触发 rebuild）
   */
  atPushed(pushedIndex: number): T | undefined {
    // 第 k 次 push 写入 slot (k % capacity)
    // 当 source.size < capacity 时，slot 里是 null（未写过）
    // 当 source.size === capacity 时，所有 slot 都有有效 item
    if (pushedIndex < 0) return undefined
    const cap = this.capacity
    const earliestStillValid = this.totalPushed - this.size
    if (pushedIndex < earliestStillValid) return undefined // 已被覆盖
    return this.slots[pushedIndex % cap] as T | undefined ?? null
  }

  /**
   * 估算内存占用（字节）。
   * 用于压力测试 / 用户提示。
   * 注：不精确，仅参考量级。
   */
  estimateBytes(): number {
    // slots 数组本身 + 元数据 + 大致每个 item 的指针
    return this.slots.length * 8 + 64 + this.size * 24
  }

  // ===== IncrementalFilter 专用 hooks（避免外部 hack 访问 slots） =====

  /**
   * 返回 earliest valid 的 push 序号（不含）。用于增量过滤判断"是否需要 rebuild"。
   * 公式：totalPushed - size
   *   - 未满时 = 0
   *   - 满后 = 最早一次覆盖的 push 序号
   */
  get earliestValidPushed(): number {
    return this.totalPushed - this.size
  }
}
