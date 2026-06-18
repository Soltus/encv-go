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
import { computed, ref, shallowRef, watch, triggerRef } from 'vue'
import { defineStore } from 'pinia'
import type { EncvTask, TaskStatus } from '@/api/encv'
import {
  loadAllTasks,
  bulkPutTasks as persistBulkPut,
  deleteTask as persistDelete,
} from '@/lib/taskPersistence'
import { getTaskMetadata } from '@/composables/useTaskTrigger'

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
   */
  async function hydrate(): Promise<void> {
    if (hydrated.value) return
    try {
      const list = await loadAllTasks()
      tasks.value = list
      hasAnyTask.value = list.length > 0
      hydrated.value = true
      console.info('[useTaskStore] hydrated from IndexedDB:', list.length)
    } catch (err) {
      console.warn('[useTaskStore.hydrate] failed:', err)
      hydrated.value = true  // 标记已尝试（避免反复重试）
    }
  }

  /**
   * 自动持久化：debounced bulk put
   * - watch(tasks) 浅引用变化触发
   * - 500ms debounce 合并连续 patch
   * - 失败静默（用户无感）
   */
  let persistTimer: ReturnType<typeof setTimeout> | null = null
  watch(tasks, () => {
    if (!hydrated.value) return
    if (persistTimer) clearTimeout(persistTimer)
    persistTimer = setTimeout(() => {
      try {
        persistBulkPut(tasks.value)
      } catch (err) {
        console.warn('[useTaskStore.persist] failed:', err)
      }
    }, 500)
  })

  /**
   * 删除单个 task 时同步从 IndexedDB 移除（不等 debounce）
   */
  function persistRemove(id: string): void {
    try {
      void persistDelete(id)
    } catch (err) {
      console.warn('[useTaskStore.persistRemove] failed:', err)
    }
  }

  /**
   * 批量替换 tasks（用于 fetchTasks / hydrate）
   * 注意：用 tasks.value = newArr 触发响应；shallowRef 不深代理每个 task
   */
  function bulkSetTasks(newTasks: EncvTask[]): void {
    // 后端 task 通常不带 runId / triggeredBy，但 useTaskTrigger 里有当前 session 的关联
    const enriched = newTasks.map((t) => {
      if (t.runId && t.triggeredBy) return t
      const meta = getTaskMetadata(t.id)
      return {
        ...t,
        runId: t.runId || meta?.runId,
        triggeredBy: t.triggeredBy || meta?.triggeredBy,
      }
    })
    tasks.value = enriched
    hasAnyTask.value = enriched.length > 0
    // 立即 persist（不走 debounce，避免后端拉的新数据被旧 debounce 覆盖）
    if (hydrated.value) {
      try {
        persistBulkPut(enriched)
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
   */
  function patchTask(id: string, partial: Partial<EncvTask>): boolean {
    const arr = tasks.value
    const idx = arr.findIndex((t) => t.id === id)
    if (idx === -1) return false
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
   * 追加新 task（applyTaskCreated 用）
   * 已有同 id → 跳过
   */
  function appendTask(task: EncvTask): void {
    if (tasks.value.some((t) => t.id === task.id)) return
    tasks.value = [task, ...tasks.value]
    hasAnyTask.value = true
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
    return targets
  }

  function removeTask(id: string): void {
    tasks.value = tasks.value.filter((t) => t.id !== id)
    hasAnyTask.value = tasks.value.length > 0
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
    if (data.type) {
      patchTask(data.id, {
        type: data.type as any,
        status: data.status as TaskStatus,
        progress: data.progress ?? 0,
      })
    } else {
      // 兼容老 payload：只 patch status/progress
      patchTask(data.id, {
        status: data.status as TaskStatus,
        progress: data.progress ?? 0,
      })
    }
  }

  function applyTaskProgress(data: { id: string; progress: number; phase?: string; speed?: string; eta?: string }): void {
    patchTask(data.id, {
      progress: data.progress,
      phase: data.phase,
      speed: data.speed,
      eta: data.eta,
    })
  }

  function applyTaskCreated(data: Partial<EncvTask> & { id: string }): void {
    if (tasks.value.some((t) => t.id === data.id)) {
      // 已有则 patch；保留已有的 runId / triggeredBy（避免 WS 无 runId payload 覆盖）
      const existing = tasks.value.find((t) => t.id === data.id)!
      patchTask(data.id, {
        ...data,
        runId: data.runId || existing.runId,
        triggeredBy: data.triggeredBy || existing.triggeredBy,
      })
      replayPendingEvents(data.id)
      return
    }
    // 后端 WS payload 通常不带 runId / triggeredBy，从 useTaskTrigger 补一次
    const meta = getTaskMetadata(data.id)
    const runId = data.runId || meta?.runId
    const triggeredBy = data.triggeredBy || meta?.triggeredBy
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
      runId,
      triggeredBy,
      outputPath: data.outputPath,
      cipherMode: data.cipherMode,
      compressionMode: data.compressionMode,
    }
    appendTask(newTask)
    replayPendingEvents(data.id)
  }

  function applyTaskCompleted(data: { id: string; error?: string; outputPath?: string }): void {
    const arr = tasks.value
    const idx = arr.findIndex((t) => t.id === data.id)
    if (idx === -1) {
      bufferPendingEvent(data.id, 'completed', data)
      return
    }
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
    $reset,
  }
})
