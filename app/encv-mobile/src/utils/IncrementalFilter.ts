/**
 * IncrementalFilter<T> - 增量过滤器
 * =====================================================
 * 解决 DevLogs `filteredBackend = arr.filter(...)` 在 1M 量级 O(N) 重新扫描 问题。
 *
 * 核心思路：
 *   - 维护 source（RingBuffer）+ cachedResult（LogEntry[]）
 *   - 新 item 到达时，O(1) 检查是否过 filter，过则 append 到 cachedResult
 *   - filter 改变时，一次性 O(N) rebuild（不可避免）
 *   - 读 filtered.length 是 O(1)
 *
 * 时间复杂度：
 *   - 持续推送（filter 不变）：每帧 O(newItems)
 *   - filter 切换：一次性 O(N)
 *   - 读 result：O(1) length / O(k) 取前 k 条
 *
 * 内存：
 *   - source: capacity × sizeof(T)
 *   - result: 最坏 case source 大小（filter 全过）
 *   - 平均: 50% × source（filter 过滤一半）
 *
 * 2026-06-15 创建
 */

import type { LogEntry } from '@/composables/useFrontendLogs'
import { RingBuffer } from './RingBuffer'

/** 重导出方便消费者（如 RingBuffer.bench.test.ts）单点引入 */
export type { LogEntry }

export type Level = 'debug' | 'info' | 'warn' | 'error' | 'all'
export type FilterPredicate = (entry: LogEntry) => boolean

export interface FilterState {
  /** 等级集合（'all' 表示全过） */
  levels: ReadonlySet<Level>
  /** 搜索文本（小写，空字符串表示不过滤） */
  searchLower: string
}

export function buildPredicate(state: FilterState): FilterPredicate {
  const { levels, searchLower } = state
  // 🆕 修复 A2: 移除 `|| levels.size === 0` 短路
  // 空 set 必须 0 通过（用户已取消全选，预期看不到任何日志）
  // 'all' 必须显式在 set 中（由 toggleLevel 在 select-all 时 push 进 set）
  const allLevels = levels.has('all')
  return (entry: LogEntry): boolean => {
    if (!allLevels && !levels.has(entry.level as Level)) return false
    if (searchLower && !entry.message.toLowerCase().includes(searchLower)) return false
    return true
  }
}

export class IncrementalFilter {
  /** 源数据（ring buffer） */
  readonly source: RingBuffer<LogEntry>
  /** 当前 filter 状态（用于判断是否需要 rebuild） */
  private state: FilterState = { levels: new Set<Level>(['all']), searchLower: '' }
  /** 当前 predicate */
  private predicate: FilterPredicate = buildPredicate(this.state)
  /** 缓存的过滤结果（数组，仅追加） */
  private result: LogEntry[] = []
  /** 已处理到的 push 序号（下次 processPending 从这里开始） */
  private processed: number = 0
  /** 自上次 rebuild 以来跳过的 item 数（debug 用） */
  public droppedSinceRebuild: number = 0
  /** 上一帧过滤 push 调用耗时（ms，debug 用） */
  public lastProcessMs: number = 0
  /**
   * 变更通知订阅列表。
   * - 推/批推/rebuild/clear 时同步 notify()
   * - DevLogs.vue 用它驱动 backendUpdateTick ref → 触发 computed 重算
   * - 替代 `watch(() => filter.version, ...)` 模式（markRaw 对象上 watch 永远不 invoke）
   */
  private listeners: Set<() => void> = new Set()
  /**
   * 单调递增的版本号（push/pushMany/rebuild/clear 自增）。保留作为 debug 探针。
   * ⚠️ 不可单独用作 Vue watch source — markRaw 对象的属性读写不会被 vue track。
   * 必须配合 subscribe() 通知机制使用。
   */
  public version: number = 0

  /**
   * 订阅变更通知。返回取消订阅函数。
   * 用于 vue 层把 IncrementalFilter 状态同步到 reactive ref。
   */
  subscribe(cb: () => void): () => void {
    this.listeners.add(cb)
    return () => { this.listeners.delete(cb) }
  }

  /** 同步通知所有订阅者 */
  private notify(): void {
    for (const cb of this.listeners) cb()
  }

  constructor(capacity: number) {
    this.source = new RingBuffer<LogEntry>(capacity)
  }

  /**
   * 推入新 item。O(1)（满则 O(1) ring buffer push + O(1) predicate check）。
   */
  push(item: LogEntry): void {
    this.source.push(item)
    this.processPending()
    this.version++
    this.notify()
  }

  /**
   * 批量推入（同帧合并优化）。
   * 时间复杂度：O(items.length)
   *
   * 🆕 2026-06-15 修：补 this.notify()。rAF 合并路径下这是 hot path，漏掉会导致
   * DevLogs.vue 的 backendUpdateTick 永远不增 → 4 个症状（全不响应）：
   *   1. 自动滚动失效（watch(tick) 不触发）
   *   2. "共 1 条（已筛选 10186 条）" 计数错（totalBackendCount 缓存 stale）
   *   3. 看着像级别筛选/搜索也失效（其实 setFilter 路径正常，但 pushMany 不通知 → UI 卡死）
   */
  pushMany(items: LogEntry[]): void {
    for (let i = 0; i < items.length; i++) this.source.push(items[i])
    this.processPending()
    this.version++
    this.notify()
  }

  /**
   * 处理自上次 processPending 以来的所有新 item。
   * 时间复杂度：O(newItems) — 远比 O(N) 优
   */
  private processPending(): void {
    if (this.source.totalPushed === this.processed) return
    const t0 = performance.now()
    const earliestValid = this.source.earliestValidPushed
    const start = Math.max(this.processed, earliestValid)
    const end = this.source.totalPushed
    if (start >= end) {
      // 落后超过 capacity → 必须 rebuild
      this.rebuild()
      this.lastProcessMs = performance.now() - t0
      return
    }
    for (let t = start; t < end; t++) {
      const item = this.source.atPushed(t)
      if (item && this.predicate(item)) {
        this.result.push(item)
      } else {
        this.droppedSinceRebuild++
      }
    }
    this.processed = end
    this.lastProcessMs = performance.now() - t0
  }

  /**
   * 切换 filter（触发 rebuild）。
   * 时间复杂度：O(N)
   */
  setFilter(state: FilterState): void {
    if (this.isSameState(state)) return
    this.state = state
    this.predicate = buildPredicate(state)
    this.rebuild()
    this.version++
    this.notify()
  }

  private isSameState(s: FilterState): boolean {
    if (s.searchLower !== this.state.searchLower) return false
    if (s.levels.size !== this.state.levels.size) return false
    for (const lvl of s.levels) {
      if (!this.state.levels.has(lvl)) return false
    }
    return true
  }

  /**
   * 强制 rebuild（filter 改变 / source 太老）。
   * 时间复杂度：O(N)
   */
  private rebuild(): void {
    const t0 = performance.now()
    this.result = []
    this.droppedSinceRebuild = 0
    this.source.forEach((item) => {
      if (this.predicate(item)) this.result.push(item)
      else this.droppedSinceRebuild++
    })
    this.processed = this.source.totalPushed
    this.lastProcessMs = performance.now() - t0
  }

  /**
   * 强制 rebuild（公开 API，用于"切 filter 时想同步等结果"场景）。
   */
  forceRebuild(): void {
    this.rebuild()
  }

  /**
   * 拿当前过滤结果（标准数组，O(1) 长度）。
   * 注意：返回的是内部引用，不要直接 mutate。
   */
  getResult(): readonly LogEntry[] {
    return this.result
  }

  /**
   * 拿过滤后的总数。
   */
  get length(): number {
    return this.result.length
  }

  /**
   * 拿源 buffer 的总数（不过滤）。
   */
  get totalLength(): number {
    return this.source.size
  }

  /**
   * 清空（保留 filter state）。
   */
  clear(): void {
    this.source.clear()
    this.result = []
    this.processed = 0
    this.droppedSinceRebuild = 0
    this.version++
    this.notify()
  }

  /**
   * 当前 filter 状态（只读）。
   */
  getState(): Readonly<FilterState> {
    return this.state
  }
}
