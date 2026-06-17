import { ref, shallowRef, computed } from 'vue'
import {
  timer,
  sync,
  checkmarkCircle,
  closeCircle,
} from 'ionicons/icons'
import {
  getTasks,
  cancelTask,
  retryTask,
  removeTask,
  isWrongPasswordError,
} from '@/api/encv'
import type { EncvTask, TaskType, TaskStatus } from '@/api/encv'
import { useI18n } from '@/composables/useI18n'
import { formatDuration } from '@/composables/useDateFormat'
import { showToast } from '@/composables/useToast'
import { getTaskMetadata } from './useTaskTrigger'

export type SortBy = 'activity' | 'created'

/**
 * 终态任务状态集合（Task 9：终态保护）
 *
 * 已 completed / failed / cancelled 的任务不再被 task:update / task:progress / task:completed 覆盖。
 * cancelling 不算终态（仍可 → cancelled / failed / success）。
 */
const TERMINAL_TASK_STATUS: Set<TaskStatus> = new Set([
  'completed', 'failed', 'cancelled',
])

/** 判断任务是否已终态（Task 9） */
function isTerminalTaskStatus(status: TaskStatus): boolean {
  return TERMINAL_TASK_STATUS.has(status)
}

/**
 * 乱序事件缓冲（Task 10：乱序缓冲）
 *
 * WS 事件可能乱序到达（task:update 先于 task:created），此时 idx=-1。
 * 缓存到 pendingEvents，等 applyTaskCreated 创建后回放。
 */
type PendingEventType = 'update' | 'progress' | 'completed'
interface PendingEvent {
  type: PendingEventType
  data: any
}

export function useTasksList() {
  const { t } = useI18n()

  // 🆕 Task 12：shallowRef 替代 ref（避免深层响应式，提升性能）
  const tasks = shallowRef<EncvTask[]>([])
  const loading = ref(false)
  const expandedWarningDetail = ref<string | null>(null)
  const sortBy = ref<SortBy>('activity')

  const showSearch = ref(false)
  const searchQuery = ref('')
  const showFilters = ref(false)
  const filterPlugins = ref<string[]>([])
  const filterTypes = ref<TaskType[]>([])
  const filterStatuses = ref<TaskStatus[]>([])

  const statusOptions: TaskStatus[] = ['queued', 'running', 'completed', 'failed', 'cancelled']

  const pluginPopoverOpen = ref(false)
  const typePopoverOpen = ref(false)
  const statusPopoverOpen = ref(false)
  const pluginPopoverEvent = ref<Event | null>(null)
  const typePopoverEvent = ref<Event | null>(null)
  const statusPopoverEvent = ref<Event | null>(null)

  // 🆕 Task 10：乱序事件缓冲 Map<taskId, PendingEvent[]>
  const pendingEvents = new Map<string, PendingEvent[]>()

  const availablePlugins = computed(() => {
    const plugins = new Set<string>()
    for (const task of tasks.value) {
      if (task.pluginName) plugins.add(task.pluginName)
    }
    return Array.from(plugins).sort()
  })

  const hasActiveFilters = computed(() =>
    filterPlugins.value.length > 0 || filterTypes.value.length > 0 || filterStatuses.value.length > 0
  )

  const hasCompletedTasks = computed(() =>
    tasks.value.some(
      (task) => task.status === 'completed' || task.status === 'failed' || task.status === 'cancelled'
    )
  )

  // 🆕 Task 12：预构建排序索引（避免每次比较重新创建 Date）
  const sortedIndices = computed(() => {
    const arr = tasks.value.map((_, i) => i)
    if (sortBy.value === 'activity') {
      // 预计算时间戳，避免比较函数内重复 new Date
      const timestamps = tasks.value.map((t) => {
        const completed = t.completedAt ? new Date(t.completedAt).getTime() : 0
        const created = new Date(t.createdAt).getTime()
        return { completed, created, activity: completed || created }
      })
      arr.sort((a, b) => {
        const ta = timestamps[a]
        const tb = timestamps[b]
        if (tb.activity !== ta.activity) return tb.activity - ta.activity
        return tb.created - ta.created
      })
    } else {
      const timestamps = tasks.value.map((t) => new Date(t.createdAt).getTime())
      arr.sort((a, b) => timestamps[b] - timestamps[a])
    }
    return arr
  })

  const sortedTasks = computed(() => sortedIndices.value.map((i) => tasks.value[i]))

  // 🆕 Task 12：预构建 tasksByRunId 索引（O(n) 分组，消除内层 find 退化 O(n²)）
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

  const filteredTasks = computed(() => {
    let result = sortedTasks.value

    if (searchQuery.value.trim()) {
      const q = searchQuery.value.trim().toLowerCase()
      result = result.filter((task) => {
        const name = getTaskName(task).toLowerCase()
        const plugin = (task.pluginName || '').toLowerCase()
        const error = (task.error || '').toLowerCase()
        const id = task.id.toLowerCase()
        return name.includes(q) || plugin.includes(q) || error.includes(q) || id.includes(q)
      })
    }

    if (filterPlugins.value.length > 0) {
      result = result.filter((task) => task.pluginName && filterPlugins.value.includes(task.pluginName))
    }

    if (filterTypes.value.length > 0) {
      result = result.filter((task) => filterTypes.value.includes(task.type))
    }

    if (filterStatuses.value.length > 0) {
      result = result.filter((task) => filterStatuses.value.includes(task.status))
    }

    return result
  })

  function openPluginPopover(event: Event) {
    pluginPopoverEvent.value = event
    pluginPopoverOpen.value = true
  }

  function openTypePopover(event: Event) {
    typePopoverEvent.value = event
    typePopoverOpen.value = true
  }

  function openStatusPopover(event: Event) {
    statusPopoverEvent.value = event
    statusPopoverOpen.value = true
  }

  function togglePluginFilter(plugin: string) {
    const idx = filterPlugins.value.indexOf(plugin)
    if (idx === -1) filterPlugins.value.push(plugin)
    else filterPlugins.value.splice(idx, 1)
  }

  function toggleTypeFilter(type: TaskType) {
    const idx = filterTypes.value.indexOf(type)
    if (idx === -1) filterTypes.value.push(type)
    else filterTypes.value.splice(idx, 1)
  }

  function toggleStatusFilter(status: TaskStatus) {
    const idx = filterStatuses.value.indexOf(status)
    if (idx === -1) filterStatuses.value.push(status)
    else filterStatuses.value.splice(idx, 1)
  }

  function clearFilters() {
    filterPlugins.value = []
    filterTypes.value = []
    filterStatuses.value = []
    searchQuery.value = ''
  }

  function onSearchInput(event: CustomEvent) {
    searchQuery.value = event.detail.value ?? ''
  }

  function toggleSort() {
    sortBy.value = sortBy.value === 'activity' ? 'created' : 'activity'
  }

  function applyFilter(opts: { plugins?: string[]; types?: TaskType[]; statuses?: TaskStatus[]; query?: string }) {
    if (opts.plugins !== undefined) filterPlugins.value = opts.plugins
    if (opts.types !== undefined) filterTypes.value = opts.types
    if (opts.statuses !== undefined) filterStatuses.value = opts.statuses
    if (opts.query !== undefined) searchQuery.value = opts.query
  }

  function applySort(sort: SortBy) {
    sortBy.value = sort
  }

  // 🆕 Task 11：fetchTasks 合并本地实时状态（progress/phase/speed/eta）
  // 历史 bug：fetchTasks 用后端数据整体替换 tasks.value，丢失 WS 推送的实时状态
  // 修复：本地运行中任务的 progress/phase/speed/eta 优先于远端（远端可能滞后）
  function mergeWithLocalState(remote: EncvTask[]): EncvTask[] {
    const localMap = new Map(tasks.value.map((t) => [t.id, t]))
    return remote.map((t) => {
      const local = localMap.get(t.id)
      if (!local) return t
      // 本地运行中任务的实时字段优先
      if (!isTerminalTaskStatus(local.status) && local.status !== 'queued') {
        return {
          ...t,
          progress: local.progress,
          phase: local.phase,
          speed: local.speed,
          eta: local.eta,
          // 保留本地元数据（后端不存）
          triggeredBy: local.triggeredBy ?? t.triggeredBy,
          runId: local.runId ?? t.runId,
        }
      }
      // 终态任务：保留本地元数据
      return {
        ...t,
        triggeredBy: local.triggeredBy ?? t.triggeredBy,
        runId: local.runId ?? t.runId,
      }
    })
  }

  async function fetchTasks() {
    loading.value = true
    try {
      const data = await getTasks()
      const enriched = mergeWithLocalState(data)
      tasks.value = enriched
    } catch {
      tasks.value = []
    }
    loading.value = false
  }

  async function refresh() {
    try {
      const data = await getTasks()
      const enriched = mergeWithLocalState(data)
      tasks.value = enriched
    } catch {
      // silent
    }
  }

  // 🆕 Task 9 + 10：所有 apply* 函数加终态保护 + 乱序缓冲

  function applyTaskUpdate(data: { id: string; type: string; status: string; progress: number }) {
    const idx = tasks.value.findIndex((t) => t.id === data.id)
    if (idx === -1) {
      // Task 10：乱序缓冲 — task:update 先于 task:created
      bufferPendingEvent(data.id, 'update', data)
      return
    }
    // Task 9：终态保护 — 已终态任务不被 update 覆盖
    if (isTerminalTaskStatus(tasks.value[idx].status)) return

    const next = [...tasks.value]
    next[idx] = {
      ...next[idx],
      ...data,
      type: data.type as TaskType,
      status: data.status as TaskStatus,
      progress: data.progress,
    }
    tasks.value = next
  }

  function applyTaskProgress(data: {
    id: string
    progress: number
    phase: string
    speed: string
    eta: string
  }) {
    const idx = tasks.value.findIndex((t) => t.id === data.id)
    if (idx === -1) {
      // Task 10：乱序缓冲
      bufferPendingEvent(data.id, 'progress', data)
      return
    }
    // Task 9：终态保护
    if (isTerminalTaskStatus(tasks.value[idx].status)) return

    // Task 12：局部 patch（shallowRef 需要新数组引用触发更新）
    const next = [...tasks.value]
    next[idx] = {
      ...next[idx],
      progress: data.progress,
      phase: data.phase,
      speed: data.speed,
      eta: data.eta,
    }
    tasks.value = next
  }

  function applyTaskCreated(data: {
    id: string
    type: string
    sourcePath: string
    pluginName?: string
    version?: number
    targetPath?: string
    createdAt?: string
    [k: string]: any
  }) {
    const exists = tasks.value.some((t) => t.id === data.id)
    if (!exists) {
      const meta = getTaskMetadata(data.id)
      const next = [...tasks.value]
      next.unshift({
        ...data,
        id: data.id,
        type: data.type as TaskType,
        sourcePath: data.sourcePath,
        pluginName: data.pluginName,
        containerVersion: data.version,
        targetPath: data.targetPath,
        status: data.status ?? 'queued',
        progress: data.progress ?? 0,
        phase: data.phase,
        createdAt: data.createdAt ?? new Date().toISOString(),
        triggeredBy: meta?.triggeredBy,
        runId: meta?.runId,
      })
      tasks.value = next

      // Task 10：回放该 task 的乱序缓冲事件
      replayPendingEvents(data.id)
    }
  }

  function applyTaskCompleted(data: { id: string; error?: string; outputPath?: string }) {
    const idx = tasks.value.findIndex((t) => t.id === data.id)
    if (idx === -1) {
      // Task 10：乱序缓冲
      bufferPendingEvent(data.id, 'completed', data)
      return
    }
    // Task 9：终态保护 — 已终态任务不被 completed 覆盖
    if (isTerminalTaskStatus(tasks.value[idx].status)) return

    const prev = tasks.value[idx]
    // 🆕 v3 2026-06-18 Task 7：用 WS 推送的 outputPath 补写 task.outputPath + 最后一个 step.detail
    // 后端 task:completed 事件 payload 已包含 outputPath（无需下拉刷新即可显示产物）
    const wsOutputPath = data.outputPath ?? ''
    const prevSteps = prev.steps ?? []
    const nextSteps = wsOutputPath && prevSteps.length > 0
      ? prevSteps.map((step, i) =>
          // 最后一个未完成的 step：补写 outputPath（与后端 task_manager.go 行为一致）
          i === prevSteps.length - 1 ? { ...step, detail: wsOutputPath } : step,
        )
      : prev.steps

    const next = [...tasks.value]
    next[idx] = {
      ...prev,
      status: data.error ? 'failed' : 'completed',
      progress: data.error ? prev.progress : 100,
      phase: data.error ? prev.phase : 'completed',
      speed: '',
      eta: '',
      error: data.error,
      completedAt: new Date().toISOString(),
      // 🆕 Task 7：WS 推送的 outputPath 写入 task 对象
      outputPath: wsOutputPath || prev.outputPath,
      steps: nextSteps,
    }
    tasks.value = next
  }

  // ==================== Task 10：乱序缓冲辅助函数 ====================

  /** 缓存乱序到达的事件 */
  function bufferPendingEvent(taskId: string, type: PendingEventType, data: any): void {
    const arr = pendingEvents.get(taskId)
    if (arr) arr.push({ type, data })
    else pendingEvents.set(taskId, [{ type, data }])
  }

  /** 回放并清除指定 task 的乱序缓冲事件 */
  function replayPendingEvents(taskId: string): void {
    const arr = pendingEvents.get(taskId)
    if (!arr || arr.length === 0) return
    // 按缓存顺序回放
    for (const event of arr) {
      if (event.type === 'update') applyTaskUpdate(event.data)
      else if (event.type === 'progress') applyTaskProgress(event.data)
      else if (event.type === 'completed') applyTaskCompleted(event.data)
    }
    pendingEvents.delete(taskId)
  }

  async function cancelTaskById(id: string) {
    try {
      await cancelTask(id)
      await fetchTasks()
    } catch {
      showToast({ message: t('tasks.taskCancelFailed'), duration: 2000, color: 'danger' })
    }
  }

  async function retryTaskById(id: string) {
    try {
      await retryTask(id)
      await fetchTasks()
    } catch {
      showToast({ message: t('tasks.taskRetryFailed'), duration: 2000, color: 'danger' })
    }
  }

  async function removeTaskById(id: string) {
    try {
      await removeTask(id)
      await fetchTasks()
    } catch {
      showToast({ message: t('tasks.taskRemoveFailed'), duration: 2000, color: 'danger' })
    }
  }

  async function clearCompletedWithConfirm() {
    const completedCount = tasks.value.filter(
      (task) =>
        task.status === 'completed' || task.status === 'failed' || task.status === 'cancelled'
    ).length
    if (completedCount === 0) return
    return completedCount
  }

  function getTaskName(task: EncvTask) {
    const parts = task.sourcePath.replace(/\\/g, '/').split('/')
    const basename = parts[parts.length - 1] || task.sourcePath
    return task.pluginName ? `${basename} [${task.pluginName}]` : basename
  }

  function getTaskDuration(task: EncvTask): string {
    if (!task.createdAt) return ''
    const created = new Date(task.createdAt).getTime()
    if (isNaN(created)) return ''
    if (task.completedAt) {
      const completed = new Date(task.completedAt).getTime()
      if (isNaN(completed)) return ''
      return formatDuration(completed - created)
    }
    if (task.status === 'running' || task.status === 'cancelling') {
      return formatDuration(Date.now() - created)
    }
    return ''
  }

  function getPluginChipLabel(): string {
    if (filterPlugins.value.length === 0) return t('tasks.allPlugins')
    if (filterPlugins.value.length === 1) return filterPlugins.value[0]
    return `${t('tasks.allPlugins')} (${filterPlugins.value.length})`
  }

  function getTypeChipLabel(): string {
    if (filterTypes.value.length === 0) return t('tasks.allTypes')
    return filterTypes.value
      .map((ty) => (ty === 'encrypt' ? t('tasks.encrypt') : t('tasks.decrypt')))
      .join(', ')
  }

  function getStatusChipLabel(): string {
    if (filterStatuses.value.length === 0) return t('tasks.allStatuses')
    return filterStatuses.value.map((s) => getStatusLabel(s)).join(', ')
  }

  function getStatusLabel(status: TaskStatus) {
    switch (status) {
      case 'queued': return t('tasks.queued')
      case 'running': return t('tasks.running')
      case 'completed': return t('tasks.completed')
      case 'failed': return t('tasks.failed')
      case 'cancelled': return t('tasks.cancelled')
      case 'cancelling': return t('tasks.cancelling')
      default: return status
    }
  }

  function isPasswordError(task: EncvTask): boolean {
    if (!task.error) return false
    return isWrongPasswordError(task.error)
  }

  function toggleWarningDetail(task: EncvTask) {
    expandedWarningDetail.value = expandedWarningDetail.value === task.id ? null : task.id
  }

  function formatWarningDetail(detail: string): string {
    try {
      return JSON.stringify(JSON.parse(detail), null, 2)
    } catch {
      return detail
    }
  }

  function getTaskIcon(task: EncvTask) {
    switch (task.status) {
      case 'queued': return timer
      case 'running': return sync
      case 'completed': return checkmarkCircle
      case 'failed': return closeCircle
      default: return timer
    }
  }

  function getTaskColor(task: EncvTask) {
    switch (task.status) {
      case 'queued': return 'medium'
      case 'running': return 'primary'
      case 'completed': return 'success'
      case 'failed': return 'danger'
      default: return 'medium'
    }
  }

  function getStatusColor(status: TaskStatus) {
    switch (status) {
      case 'queued': return 'medium'
      case 'running': return 'primary'
      case 'completed': return 'success'
      case 'failed': return 'danger'
      default: return 'medium'
    }
  }

  function getPhaseLabel(phase: string) {
    switch (phase) {
      case 'analyzing': return t('tasks.phaseAnalyzing')
      case 'initializing': return t('tasks.phaseInitializing')
      case 'preprocessing': return t('tasks.phasePreprocessing')
      case 'encrypting': return t('tasks.phaseEncrypting')
      case 'decrypting': return t('tasks.phaseDecrypting')
      case 'packing': return t('tasks.phasePacking')
      case 'verifying': return t('tasks.phaseVerifying')
      case 'completed': return t('tasks.phaseCompleted')
      default: return phase
    }
  }

  return {
    tasks,
    loading,
    expandedWarningDetail,
    sortBy,
    showSearch,
    searchQuery,
    showFilters,
    filterPlugins,
    filterTypes,
    filterStatuses,
    statusOptions,
    pluginPopoverOpen,
    typePopoverOpen,
    statusPopoverOpen,
    pluginPopoverEvent,
    typePopoverEvent,
    statusPopoverEvent,
    availablePlugins,
    hasActiveFilters,
    hasCompletedTasks,
    sortedTasks,
    sortedIndices,
    tasksByRunId,
    filteredTasks,
    fetchTasks,
    refresh,
    applyFilter,
    applySort,
    openPluginPopover,
    openTypePopover,
    openStatusPopover,
    togglePluginFilter,
    toggleTypeFilter,
    toggleStatusFilter,
    clearFilters,
    onSearchInput,
    toggleSort,
    applyTaskUpdate,
    applyTaskProgress,
    applyTaskCreated,
    applyTaskCompleted,
    cancelTaskById,
    retryTaskById,
    removeTaskById,
    clearCompletedWithConfirm,
    getTaskName,
    getTaskDuration,
    getPluginChipLabel,
    getTypeChipLabel,
    getStatusChipLabel,
    getStatusLabel,
    isPasswordError,
    toggleWarningDetail,
    formatWarningDetail,
    getTaskIcon,
    getTaskColor,
    getStatusColor,
    getPhaseLabel,
  }
}
