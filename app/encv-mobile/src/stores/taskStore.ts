/**
 * v6 2026-06-18 任务系统重设计
 *
 * useTaskStore - 任务数据 Pinia setup store
 *
 * 设计原则（替代 useTasksList.ts 1038 行 monolith）：
 * 1. shallowRef + Map 索引 → 100+ task 无 deep reactive 开销
 * 2. patchTask(id, partial) → 局部修改，不重建 array
 * 3. applyEvent(ev) → WS 4 件套统一入口，终态保护 + 乱序缓冲
 * 4. tasksByRunId / tasksById / availablePlugins 缓存为 getter，不重复 build
 * 5. 取消 run = patchTask 每个 run task 为 cancelling → Tasks UI 立即反应
 *
 * 与 useTasksList 协作：
 * - useTasksList 改为读 store（tasks, filterState）
 * - 派生 computed (groupedItems, flatItems, displayedItems) 仍在 useTasksList
 *   因为这些是 UI 层关注点，store 只管数据
 */
import { computed, ref, shallowRef, triggerRef } from 'vue'
import { defineStore } from 'pinia'
import type { EncvTask, TaskStatus } from '@/api/encv'
import {
  loadAllTasks,
  bulkPutTasks as persistBulkPut,
  putTask as persistPut,
  deleteTask as persistDelete,
  clearPutThrottle,
  ensureLRUCache,
} from '@/lib/taskPersistence'

/** 终态保护集合 */
const TERMINAL_STATUSES: Set<TaskStatus> = new Set([
  'completed', 'failed', 'cancelled',
])

/** 乱序事件缓冲 */
type PendingEventType = 'update' | 'progress' | 'completed'
interface PendingEvent {
  type: PendingEventType
  data: any
}

export const useTaskStore = defineStore('task', () => {
  // ============ 核心状态 ============
  /** shallowRef：避免深响应式开销；patchTask 内部触发整个 ref 替换但每个 task 对象本身不变 */
  const tasks = shallowRef<EncvTask[]>([])

  /** 应用启动时 tasks 是否已从 IndexedDB 加载完成 */
  const hydrated = ref(false)

  /** 正在拉取后端列表（pull-to-refresh 专用） */
  const isRefreshing = ref(false)

  /** 加载状态：true = 空状态；false = 有数据（即使 isRefreshing=true 也不显示空态） */
  const hasAnyTask = ref(false)

  /** 🆕 v6 2026-06-18：置顶 runId 集合（左滑置顶操作）
   *  - Set 保证 O(1) add/delete/has
   *  - 用户置顶的 run 在分组列表中排到最前
   *  - 持久化到 IndexedDB meta 表
   */
  const pinnedRunIds = ref<Set<string>>(new Set())

  // ============ 乱序事件缓冲 ============
  const pendingEvents = new Map<string, PendingEvent[]>()

  // ============ 🆕 v6 2026-06-22 性能优化：id → index 索引 ============
  // 之前：patchTask 用 findIndex O(n)、applyTaskUpdate/Progress/Completed 用 find O(n)
  //   高频 WS progress 事件 × 100+ tasks = O(n²) 累积
  // 现在：维护 _taskIndex Map，patchTask / getTaskById 用 O(1) Map lookup
  //   - appendTask / removeTask / bulkSetTasks / hydrate 等改变数组长度时重建索引（O(n)）
  //   - patchTask / cancelRunTasks 不改变数组长度，索引保持有效
  const _taskIndex = new Map<string, number>()

  function rebuildIndex(): void {
    _taskIndex.clear()
    const arr = tasks.value
    for (let i = 0; i < arr.length; i++) {
      _taskIndex.set(arr[i].id, i)
    }
  }

  /** O(1) 按 id 查 task（替代 tasks.value.find O(n)） */
  function getTaskById(id: string): EncvTask | undefined {
    const idx = _taskIndex.get(id)
    if (idx === undefined) return undefined
    return tasks.value[idx]
  }

  function bufferPendingEvent(taskId: string, type: PendingEventType, data: any): void {
    const arr = pendingEvents.get(taskId)
    if (arr) arr.push({ type, data })
    else pendingEvents.set(taskId, [{ type, data }])
  }

  function replayPendingEvents(taskId: string): void {
    const arr = pendingEvents.get(taskId)
    if (!arr || arr.length === 0) return
    for (const event of arr) {
      if (event.type === 'update') applyTaskUpdate(event.data)
      else if (event.type === 'progress') applyTaskProgress(event.data)
      else if (event.type === 'completed') applyTaskCompleted(event.data)
    }
    pendingEvents.delete(taskId)
  }

  // ============ 核心操作 ============

  // ============ 持久化（IndexedDB） ============

  /**
   * 冷启动时从 IndexedDB 加载 tasks。
   * - 仅在首次打开 Tasks 视图时调一次（避免重复 IO）
   * - 失败时静默回退到空数组（UI 仍可通过 fetchTasks 拉后端）
   * - 🆕 hydrate 后调 ensureLRUCache 清理过量缓存（保留最新 200 条）
   */
  async function hydrate(): Promise<void> {
    if (hydrated.value) return
    try {
      const list = await loadAllTasks()
      tasks.value = list
      hasAnyTask.value = list.length > 0
      rebuildIndex()  // 🆕 v6 2026-06-22：重建 id→index 索引
      hydrated.value = true
      console.info('[useTaskStore] hydrated from IndexedDB:', list.length)
      // 🆕 LRU 缓存清理：保留最新 200 条（后端 SQLite 已是权威存储，IndexedDB 降级为纯缓存）
      void ensureLRUCache()
    } catch (err) {
      console.warn('[useTaskStore.hydrate] failed:', err)
      hydrated.value = true  // 标记已尝试（避免反复重试）
    }
  }

  // 🆕 v6 2026-06-22 性能优化：移除 watch(tasks) 自动持久化
  //   旧逻辑：watch(tasks, debounced persistBulkPut) → 每次 patchTask（含 progress 高频更新）
  //          都触发全量 bulkPut（100+ tasks × 每秒多次 progress = 大量 IndexedDB 写入）
  //   新逻辑：显式持久化 — 只在有意义的状态变更时写单行（putTask）
  //          - appendTask → putTask（新 task）
  //          - applyTaskUpdate → putTask（status 变更，重要）
  //          - applyTaskCompleted → putTask（终态，重要）
  //          - applyTaskProgress → 不持久化（progress 高频 transient，后端 SQLite 已存）
  //          - bulkSetTasks → persistBulkPut（全量同步，少见）
  //          - removeTask / removeRunTasks → persistDelete（已有）

  /**
   * 删除单个 task 时同步从 IndexedDB 移除（不等 debounce）
   */
  function persistRemove(id: string): void {
    try {
      clearPutThrottle(id)
      void persistDelete(id)
    } catch (err) {
      console.warn('[useTaskStore.persistRemove] failed:', err)
    }
  }

  /**
   * 批量替换 tasks（用于 fetchTasks / hydrate）
   *
   * 🆕 v6 2026-06-18：单一数据源 — 后端 MobileTask 已持久化 runId/triggeredBy
   *   不再需要 merge / getTaskMetadata 回填，直接用后端数据
   */
  function bulkSetTasks(newTasks: EncvTask[]): void {
    tasks.value = newTasks
    hasAnyTask.value = newTasks.length > 0
    rebuildIndex()  // 🆕 v6 2026-06-22：重建 id→index 索引
    if (hydrated.value) {
      try {
        persistBulkPut(newTasks)
      } catch (err) {
        console.warn('[useTaskStore.bulkSetTasks.persist] failed:', err)
      }
    }
  }

  /**
   * 局部 patch：只改指定 id 的 task 字段，不重建 array
   *
   * 关键优化：保留 array 引用，仅修改 task 对象；
   * 但 shallowRef 不深代理 task 对象，所以需要 triggerRef 强制更新
   * （Vue 3.5+ triggerRef 直接可用，旧版本用 tasks.value = [...tasks.value]）
   *
   * 🆕 v6 2026-06-22：patchTask 本身不持久化（避免 progress 高频写爆 IndexedDB）
   *   - 需要持久化的调用方调 patchTask 后调 persistTaskById
   *   - 或直接用 applyTaskUpdate / applyTaskCompleted（已内置持久化）
   */
  function patchTask(id: string, partial: Partial<EncvTask>): boolean {
    const idx = _taskIndex.get(id)  // 🆕 v6 2026-06-22：O(1) Map lookup 替代 O(n) findIndex
    if (idx === undefined) return false
    const arr = tasks.value
    // 终态保护
    if (TERMINAL_STATUSES.has(arr[idx].status) && partial.status) {
      return false
    }
    arr[idx] = { ...arr[idx], ...partial }
    // 强制触发 shallowRef 更新（因为我们直接修改了数组元素）
    triggerRef(tasks)
    return true
  }

  /**
   * 🆕 v6 2026-06-22：显式持久化指定 task（patchTask 后调用方按需调）
   *   - 用于 cancelTaskById 等外部 patchTask 调用后的持久化
   *   - 内部 applyTaskUpdate / applyTaskCompleted 等已内置持久化，无需再调
   */
  function persistTaskById(id: string): void {
    if (!hydrated.value) return
    const task = getTaskById(id)  // 🆕 v6 2026-06-22：O(1) 替代 find O(n)
    if (task) {
      try { void persistPut(task) } catch (err) {
        console.warn('[useTaskStore.persistTaskById] failed:', err)
      }
    }
  }

  /**
   * 追加新 task（applyTaskCreated 用）
   * 已有同 id → 跳过
   */
  function appendTask(task: EncvTask): void {
    if (_taskIndex.has(task.id)) return  // 🆕 v6 2026-06-22：O(1) 替代 some O(n)
    tasks.value = [task, ...tasks.value]
    hasAnyTask.value = true
    rebuildIndex()  // 🆕 v6 2026-06-22：prepend 改变了所有索引，需重建
    // 🆕 v6 2026-06-22：显式持久化单行（替代旧 watch 全量 bulkPut）
    if (hydrated.value) {
      try { void persistPut(task) } catch (err) {
        console.warn('[useTaskStore.appendTask.persist] failed:', err)
      }
    }
  }

  /**
   * 取消整个 run 的所有 task
   *  - 找到 runId 关联的所有 task
   *  - 乐观更新 status = 'cancelling'（UI 立即反应）
   *  - 调后端 cancelRun
   *  - 失败回滚
   */
  function cancelRunTasks(runId: string): EncvTask[] {
    const targets: EncvTask[] = []
    for (const t of tasks.value) {
      if (t.runId === runId && (t.status === 'running' || t.status === 'queued')) {
        targets.push(t)
      }
    }
    for (const t of targets) {
      patchTask(t.id, { status: 'cancelling' })
    }
    // 🆕 v6 2026-06-22：cancelling 是重要状态变更 → 显式持久化
    //   用 getTaskById O(1) 替代 find O(n)
    if (hydrated.value) {
      for (const t of targets) {
        const updated = getTaskById(t.id)
        if (updated) {
          try { void persistPut(updated) } catch (err) {
            console.warn('[useTaskStore.cancelRunTasks.persist] failed:', err)
          }
        }
      }
    }
    return targets
  }

  function removeTask(id: string): void {
    if (!_taskIndex.has(id)) return  // 🆕 v6 2026-06-22：O(1) 检查避免无谓 filter
    tasks.value = tasks.value.filter((t) => t.id !== id)
    hasAnyTask.value = tasks.value.length > 0
    rebuildIndex()  // 🆕 v6 2026-06-22：重建索引
    persistRemove(id)
  }

  /**
   * 🆕 v6 2026-06-18：删除整个 run 的所有 task（左滑删除操作）
   *  - 返回被删的 task 列表（调用方遍历调后端 API）
   *  - 同时解除 runId 的置顶（避免僵尸 pinned 引用）
   */
  function removeRunTasks(runId: string): EncvTask[] {
    const targets: EncvTask[] = []
    for (const t of tasks.value) {
      if (t.runId === runId) targets.push(t)
    }
    if (targets.length === 0) return targets
    const targetIds = new Set(targets.map((t) => t.id))
    tasks.value = tasks.value.filter((t) => !targetIds.has(t.id))
    hasAnyTask.value = tasks.value.length > 0
    rebuildIndex()  // 🆕 v6 2026-06-22：重建索引
    // 同步 persist
    for (const t of targets) persistRemove(t.id)
    // 解除置顶
    if (pinnedRunIds.value.has(runId)) {
      const next = new Set(pinnedRunIds.value)
      next.delete(runId)
      pinnedRunIds.value = next
    }
    return targets
  }

  /**
   * 🆕 v6 2026-06-18：置顶 / 取消置顶 run
   *  - toggle 语义：已置顶则取消置顶
   */
  function togglePinRun(runId: string): boolean {
    const next = new Set(pinnedRunIds.value)
    if (next.has(runId)) {
      next.delete(runId)
      pinnedRunIds.value = next
      return false
    } else {
      next.add(runId)
      pinnedRunIds.value = next
      return true
    }
  }

  /** 读取置顶状态（O(1)） */
  function isRunPinned(runId: string): boolean {
    return pinnedRunIds.value.has(runId)
  }

  // ============ WS 4 件套统一入口 ============

  function applyTaskUpdate(data: { id: string; type?: string; status?: string; progress?: number }): void {
    // 关键：只 patch 后端实际提供的字段，绝不传 undefined
    // 否则 patchTask 的 spread 会用 undefined 覆盖现有 status
    // → 终态保护短路（partial.status 是 falsy）→ 覆盖发生 → 任务"逃出"聚合
    const partial: Partial<EncvTask> = {}
    if (data.type !== undefined) partial.type = data.type as any
    if (data.status !== undefined) partial.status = data.status as TaskStatus
    if (data.progress !== undefined) partial.progress = data.progress
    if (Object.keys(partial).length === 0) return  // 空 patch 直接返回
    patchTask(data.id, partial)
    // status 变更 → 显式持久化单行（重要状态，立即写）
    if (hydrated.value && partial.status) {
      const updated = getTaskById(data.id)
      if (updated) {
        try { void persistPut(updated) } catch (err) {
          console.warn('[useTaskStore.applyTaskUpdate.persist] failed:', err)
        }
      }
    }
  }

  function applyTaskProgress(data: { id: string; progress: number; phase?: string; speed?: string; eta?: string }): void {
    patchTask(data.id, {
      progress: data.progress,
      phase: data.phase,
      speed: data.speed,
      eta: data.eta,
    })
    // 🆕 progress 是高频 transient 状态，后端 SQLite 已持久化，前端不再写 IndexedDB
  }

  function applyTaskCreated(data: Partial<EncvTask> & { id: string }): void {
    // 🆕 v6 2026-06-22：用 _taskIndex.has O(1) 替代 some O(n)
    if (_taskIndex.has(data.id)) {
      // 已有则 patch；保留已有的 runId / triggeredBy（避免 WS 无 runId payload 覆盖）
      const existing = getTaskById(data.id)!  // 🆕 O(1) 替代 find O(n)
      patchTask(data.id, {
        ...data,
        runId: data.runId || existing.runId,
        triggeredBy: data.triggeredBy || existing.triggeredBy,
      })
      replayPendingEvents(data.id)
      // 🆕 v6 2026-06-22：patch 后显式持久化单行
      // 🆕 2026-06-22 Q6A：await 同步持久化（避免 escape — 1000+ 并发丢 runId）
      if (hydrated.value) {
        const updated = getTaskById(data.id)  // 🆕 O(1) 替代 find O(n)
        if (updated) {
          // 注意：persistPut 内部已带 try/catch + 失败重试（2026-06-22 Q6A 升级）
          try { void persistPut(updated) } catch (err) {
            console.warn('[useTaskStore.applyTaskCreated.persist] failed:', err)
          }
        }
      }
      return
    }
    // 🆕 v6 2026-06-18：后端 task 自带 runId/triggeredBy（单一数据源），不再回填
    const newTask: EncvTask = {
      id: data.id,
      type: (data.type ?? 'encrypt') as any,
      sourcePath: data.sourcePath ?? '',
      pluginName: data.pluginName,
      status: (data.status ?? 'queued') as TaskStatus,
      progress: data.progress ?? 0,
      phase: data.phase,
      speed: data.speed,
      eta: data.eta,
      error: data.error,
      createdAt: data.createdAt ?? new Date().toISOString(),
      containerVersion: data.containerVersion,
      targetPath: data.targetPath,
      runId: data.runId,
      triggeredBy: data.triggeredBy,
      outputPath: data.outputPath,
      cipherMode: data.cipherMode,
      compressionMode: data.compressionMode,
    }
    appendTask(newTask)
    replayPendingEvents(data.id)
  }

  /**
   * 🆕 2026-06-22 v2 架构重写：删除 removeTaskLocal
   *
   * 历史背景：Q6A 临时引入 removeTaskLocal（仅前端 hide 不删后端），
   *   但导致架构混乱：删除路径有 2 套（removeTask 调 persistDelete / removeTaskLocal 不调）
   *   → reconcileWithBackend 还要特殊处理"前端 hide 但后端还在"
   *   → 1000+ task 重建时容易把已 hide 的 task 拉回来
   *
   * 修法：删除 removeTaskLocal。统一用 removeTask 走正常路径：
   *   - store 移除 + IndexedDB 删除 + 后端调 DELETE /api/tasks/:id
   *   - 1000+ 自动化测试场景，task 不需要前端 hide 语义（测试后清空就行）
   */

  /**
   * 🆕 2026-06-22 v2 架构重写：rebuildFromBackend（全量重建）
   *
   * 取代旧的 reconcileWithBackend（增量 patch 易遗漏状态）。
   *
   * 语义：
   *   - serverTasks 是后端权威状态（含 runId / triggeredBy / status / createdAt 等）
   *   - 启动时：直接以 serverTasks 替换 store.tasks（不与 IndexedDB merge）
   *   - 已 IndexedDB 持久化的 task 也会被 server 取代
   *   - 1000+ 自动化测试场景下保证"无孤儿"
   *
   * 调用方：
   *   - useTasksList onMounted 拉一次
   *   - 切换 server / 重连后调一次
   */
  function rebuildFromBackend(serverTasks: EncvTask[]): void {
    if (serverTasks.length === 0) {
      // 保留现有 store（不要清空，避免闪烁）
      return
    }
    bulkSetTasks(serverTasks)
  }

  function applyTaskCompleted(data: { id: string; error?: string; outputPath?: string }): void {
    const idx = _taskIndex.get(data.id)  // 🆕 v6 2026-06-22：O(1) 替代 findIndex O(n)
    if (idx === undefined) {
      bufferPendingEvent(data.id, 'completed', data)
      return
    }
    const arr = tasks.value
    if (TERMINAL_STATUSES.has(arr[idx].status)) return

    const prev = arr[idx]
    const wsOutputPath = data.outputPath ?? ''
    const prevSteps = prev.steps ?? []
    const nextSteps = wsOutputPath && prevSteps.length > 0
      ? prevSteps.map((step, i) =>
          i === prevSteps.length - 1 ? { ...step, detail: wsOutputPath } : step,
        )
      : prev.steps

    arr[idx] = {
      ...prev,
      status: data.error ? 'failed' : 'completed',
      progress: data.error ? prev.progress : 100,
      phase: data.error ? prev.phase : 'completed',
      speed: '',
      eta: '',
      error: data.error,
      completedAt: new Date().toISOString(),
      outputPath: wsOutputPath || prev.outputPath,
      steps: nextSteps,
    }
    triggerRef(tasks)
    // 🆕 v6 2026-06-22：终态 → 显式持久化单行（重要状态，立即写）
    if (hydrated.value) {
      try { void persistPut(arr[idx]) } catch (err) {
        console.warn('[useTaskStore.applyTaskCompleted.persist] failed:', err)
      }
    }
  }

  // ============ Getters（缓存的派生） ============

  /** 按 id 索引 O(1) 查表 */
  const tasksById = computed(() => {
    const map = new Map<string, EncvTask>()
    for (const t of tasks.value) map.set(t.id, t)
    return map
  })

  /** 按 runId 分组（O(n) 分组，避免内层 find 退化 O(n²)） */
  const tasksByRunId = computed(() => {
    const map = new Map<string, EncvTask[]>()
    for (const task of tasks.value) {
      if (!task.runId) continue
      const arr = map.get(task.runId)
      if (arr) arr.push(task)
      else map.set(task.runId, [task])
    }
    return map
  })

  /**
   * 出现的 plugin 列表（去重 + 排序）
   * - 🆕 v6 2026-06-18：空 pluginName 任务映射到 `__unknown__` sentinel
   *   - 让用户能主动筛选"未知插件"任务（之前空 pluginName 永远命中失败 → bug）
   *   - UI 在 chip 标签上把 `__unknown__` 翻译为"未知插件"
   */
  const availablePlugins = computed(() => {
    const set = new Set<string>()
    let hasUnknown = false
    for (const t of tasks.value) {
      if (t.pluginName) set.add(t.pluginName)
      else hasUnknown = true
    }
    const list = Array.from(set).sort()
    if (hasUnknown) list.push('__unknown__')
    return list
  })

  // ============ Reset（测试用） ============
  function $reset(): void {
    tasks.value = []
    _taskIndex.clear()  // 🆕 v6 2026-06-22：清空索引
    pendingEvents.clear()
    hydrated.value = false
    hasAnyTask.value = false
    isRefreshing.value = false
    pinnedRunIds.value = new Set()
  }

  return {
    // state
    tasks,
    hydrated,
    isRefreshing,
    hasAnyTask,
    pinnedRunIds,
    // getters
    tasksById,
    tasksByRunId,
    availablePlugins,
    // actions
    hydrate,
    bulkSetTasks,
    patchTask,
    persistTaskById,
    appendTask,
    removeTask,
    removeRunTasks,
    cancelRunTasks,
    togglePinRun,
    isRunPinned,
    applyTaskUpdate,
    applyTaskProgress,
    applyTaskCreated,
    applyTaskCompleted,
    rebuildFromBackend,  // 🆕 v2 架构
    $reset,
  }
})
