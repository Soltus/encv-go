/**
 * useTasksList — 任务视图薄壳（v7 2026-06-22 重构）
 *
 * v6 之前：1038 行 monolith，自己管 tasks ref + filter + view state
 * v6：拆成 useTaskStore + useTaskFilter + useTasksList 三层
 * v7（当前）：taskStore 是唯一 owner；本文件只保留 helper 函数 + UI 局部 state
 *
 * 视图组件 import 路径不变（继续用 useTasksList），内部委托给 useTaskStore
 */
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useTaskStore } from '@/stores/taskStore'
import {
  cancelTask,
  removeTask as removeTaskApi,
  getTasks as apiGetTasks,
  retryTask as retryTaskApi,
  isWrongPasswordError,
  type EncvTask,
  type TaskStatus,
} from '@/api/encv'
import { useTaskEventBridge } from '@/composables/useTaskEventBridge'
import { useI18n } from '@/composables/useI18n'
import { useWorkflowTaskService } from '@/composables/useWorkflowTaskService'
import { formatDateTime as dateFormat } from '@/composables/useDateFormat'
import { getTaskTypeLabel } from '@/lib/taskTypeLabel'

// ============ helper：单例 ref state（UI 局部） ============
// 这些状态属于"视图临时态"（popover / expanded warning / search 框显示）
// 不进 store，但跨组件共享 → 模块级 singleton
let _viewInstance: ReturnType<typeof createView> | null = null
let _tasksListInstance: ReturnType<typeof createUseTasksList> | null = null
function createView() {
  return {
    showSearch: ref(false),
    showFilters: ref(false),
    expandedWarningDetail: ref<Set<string>>(new Set()),
    pluginPopoverOpen: ref(false),
    typePopoverOpen: ref(false),
    statusPopoverOpen: ref(false),
    datePopoverOpen: ref(false),
    pluginPopoverEvent: ref<any>(null),
    typePopoverEvent: ref<any>(null),
    statusPopoverEvent: ref<any>(null),
    datePopoverEvent: ref<any>(null),
  }
}

// ============ helper：任务显示 ============
function getTaskDisplayName(task: EncvTask): string {
  if (task.targetPath) return task.targetPath.split('/').pop() || task.targetPath
  if (task.sourcePath) return task.sourcePath.split('/').pop() || task.sourcePath
  return task.id.slice(0, 8)
}

function formatTaskDuration(task: EncvTask): string {
  if (!task.completedAt) return ''
  const ms = new Date(task.completedAt).getTime() - new Date(task.createdAt).getTime()
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

function getStatusColorInner(status: TaskStatus): string {
  switch (status) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'running': case 'cancelling': return 'warning'
    case 'cancelled': return 'medium'
    case 'queued': return 'primary'
    default: return 'medium'
  }
}

function isPasswordError(task: EncvTask): boolean {
  if (!task.error) return false
  return isWrongPasswordError(task.error)
}

// ============ helper：group 派生（兜底，store 已有但保持兼容） ============
function computeGroupCounters(tasks: EncvTask[], filter: any): any {
  const plugins: Record<string, { hit: number; total: number }> = {}
  const types: Record<string, { hit: number; total: number }> = {}
  const statuses: Record<string, { hit: number; total: number }> = {}
  const date = { hit: 0, total: tasks.length }
  const search = { hit: 0, total: tasks.length }

  const q = (filter.searchQuery.value ?? '').trim().toLowerCase()
  const fromTs = filter.filterDateRange.value?.from
  const toTs = filter.filterDateRange.value?.to
  const hasSearch = q.length > 0
  const hasDate = !!fromTs || !!toTs

  for (const tk of tasks) {
    const pName = tk.pluginName || '__unknown__'
    if (!plugins[pName]) plugins[pName] = { hit: 0, total: 0 }
    plugins[pName].total++
    if (filter.filterPlugins.value.length === 0 || filter.filterPlugins.value.includes(pName)) {
      plugins[pName].hit++
    }
    if (!types[tk.type]) types[tk.type] = { hit: 0, total: 0 }
    types[tk.type].total++
    if (filter.filterTypes.value.length === 0 || filter.filterTypes.value.includes(tk.type)) {
      types[tk.type].hit++
    }
    if (!statuses[tk.status]) statuses[tk.status] = { hit: 0, total: 0 }
    statuses[tk.status].total++
    if (filter.filterStatuses.value.length === 0 || filter.filterStatuses.value.includes(tk.status)) {
      statuses[tk.status].hit++
    }
    if (!hasDate || ((!fromTs || tk.createdAt >= fromTs) && (!toTs || tk.createdAt < toTs))) {
      date.hit++
    }
    if (!hasSearch) {
      search.hit++
    } else {
      const name = getTaskDisplayName(tk).toLowerCase()
      const plugin = (tk.pluginName || '').toLowerCase()
      const error = (tk.error || '').toLowerCase()
      const id = tk.id.toLowerCase()
      if (name.includes(q) || plugin.includes(q) || error.includes(q) || id.includes(q)) {
        search.hit++
      }
    }
  }
  const hitAny =
    Object.values(plugins).some((p) => p.hit > 0) &&
    Object.values(types).some((ty) => ty.hit > 0) &&
    Object.values(statuses).some((s) => s.hit > 0) &&
    date.hit > 0
  return { plugins, types, statuses, date, search, hitAny }
}

function createUseTasksList() {
  const store = useTaskStore()
  const storeRefs = storeToRefs(store)
  const { t } = useI18n()
  const workflowService = useWorkflowTaskService()
  const view = _viewInstance ?? (_viewInstance = createView())

  // ============ WS 4 件套桥接（统一走 store.applyEvent） ============
  useTaskEventBridge({
    onUpdate: (payload) => store.applyEvent('update', payload),
    onProgress: (payload) => store.applyEvent('progress', payload),
    onCreate: (payload) => store.applyEvent('created', payload),
    onComplete: (payload) => store.applyEvent('completed', payload),
  })

  // ============ displayedItems（视图层包装：注入 kind/counters/displayData） ============
  // ⚠️ 模板 TaskVirtualList 期望 item.kind === 'date' | 'group' | 'task' 三种
  //   - date: { kind:'date', key, label }
  //   - group: { kind:'group', key, runId, startedAt, tasks, counters, displayData }
  //   - task: { kind:'task', key, task }
  function dateSectionKey(date: string): string {
    const d = new Date(date)
    const now = new Date()
    const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
    const yesterdayStart = todayStart - 86400000
    const weekStart = todayStart - 7 * 86400000
    const monthStart = new Date(now.getFullYear(), now.getMonth(), 1).getTime()
    const ts = d.getTime()
    if (ts >= todayStart) return 'today'
    if (ts >= yesterdayStart) return 'yesterday'
    if (ts >= weekStart) return 'thisWeek'
    if (ts >= monthStart) return 'thisMonth'
    return 'earlier'
  }

  function buildGroupDisplayData(groupTasks: EncvTask[], startedAt: string): any {
    const summary = { total: groupTasks.length, passed: 0, failed: 0, running: 0, pending: 0, percent: 0 }
    let dominantStatus: TaskStatus = 'running'
    const plugins = new Set<string>()
    for (const tk of groupTasks) {
      if (tk.status === 'completed') summary.passed++
      else if (tk.status === 'failed') summary.failed++
      else if (tk.status === 'running' || tk.status === 'cancelling') summary.running++
      else if (tk.status === 'queued') summary.pending++
      if (tk.pluginName) plugins.add(tk.pluginName)
    }
    const finished = summary.passed + summary.failed
    if (groupTasks.length > 0) summary.percent = Math.round((finished / groupTasks.length) * 100)
    if (summary.failed > 0) dominantStatus = 'failed'
    else if (summary.running > 0) dominantStatus = 'running'
    else if (summary.pending > 0) dominantStatus = 'queued'
    else if (summary.passed > 0) dominantStatus = 'completed'

    const tone = groupTasks[0]?.triggeredBy === 'ai_agent' ? 'ai_agent'
              : groupTasks[0]?.triggeredBy === 'automation' ? 'automation'
              : 'user'
    const moodClass = summary.failed > 0 ? 'tl-mood--fail'
                    : summary.running > 0 ? 'tl-mood--running'
                    : (summary.passed === groupTasks.length && groupTasks.length > 0) ? 'tl-mood--success'
                    : ''
    const pluginBadges = Array.from(plugins).slice(0, 3)
    let duration = ''
    const completed = groupTasks.filter((tk) => tk.completedAt)
    if (completed.length > 0) {
      const maxEnd = Math.max(...completed.map((tk) => new Date(tk.completedAt!).getTime()))
      const ms = maxEnd - new Date(startedAt).getTime()
      if (ms > 0) {
        const minutes = Math.floor(ms / 60000)
        const seconds = Math.floor((ms % 60000) / 1000)
        duration = minutes > 0 ? `${minutes}m ${seconds}s` : `${(ms / 1000).toFixed(1)}s`
      }
    }
    return { summary, dominantStatus, tone, moodClass, pluginBadges, duration }
  }

  const displayedItems = computed<any[]>(() => {
    const isGroupMode = storeRefs.viewMode.value === 'group'
    const items: any[] = []
    let lastDateKey = ''
    function pushDateHeader(key: string) {
      if (key === lastDateKey) return
      lastDateKey = key
      items.push({ kind: 'date', key: `date-${key}`, label: t(`tasks.date.${key}`, { defaultValue: key }) })
    }
    if (isGroupMode) {
      for (const g of storeRefs.groupedTasksByRunId.value) {
        pushDateHeader(dateSectionKey(g.startedAt))
        // 🆕 v7 2026-06-22：伪 group 拆解
        //   手动 task（runId 为空）被 groupedTasksByRunId 分到 __manual__${id} key，
        //   每个手动 task 单独成 group → 显示成 group card 看着像聚合，实际只能点 1 个 task
        //   修复：runId 空 → 显示成 task row（kind='task'），不走 group card
        //   自动化 task（runId 非空 + 多 task）→ 走 group card
        if (!g.runId) {
          for (const tk of g.tasks) {
            items.push({ kind: 'task', key: `t-${tk.id}`, task: tk })
          }
          continue
        }
        const counters = computeGroupCounters(g.tasks, storeRefs)
        if (!counters.hitAny) continue
        items.push({
          kind: 'group',
          key: g.key,
          runId: g.runId,
          startedAt: g.startedAt,
          tasks: g.tasks,
          counters,
          displayData: buildGroupDisplayData(g.tasks, g.startedAt),
        })
      }
    } else {
      for (const ft of storeRefs.flatTaskList.value) {
        pushDateHeader(dateSectionKey(ft.task.createdAt))
        items.push({ kind: 'task', key: ft.key, task: ft.task })
      }
    }
    return items
  })

  // ============ Helper：状态显示 ============
  function getTaskName(task: EncvTask): string { return getTaskDisplayName(task) }
  function getTaskIcon(task: EncvTask): string { return task.type === 'encrypt' ? 'lock-closed' : 'lock-open' }
  function getTaskColor(task: EncvTask): string { return getStatusColorInner(task.status) }
  function getStatusLabel(status: TaskStatus): string { return t(`tasks.${status}`) }
  function getStatusColor(status: TaskStatus): string { return getStatusColorInner(status) }
  function getPhaseLabel(phase?: string): string {
    if (!phase) return ''
    return t(`tasks.phase.${phase}`, { defaultValue: phase })
  }
  function getTaskDuration(task: EncvTask): string { return formatTaskDuration(task) }
  function getPluginChipLabel(name?: string): string {
    if (name === '__unknown__') return t('tasks.unknownPlugin')
    if (name) return name
    if (store.filterPlugins.length === 0) return t('tasks.allPlugins')
    if (store.filterPlugins.length === 1) return store.filterPlugins[0]
    return `${store.filterPlugins.length}`
  }
  function getTypeChipLabel(type?: string): string {
    if (type) return getTaskTypeLabel(type, t)
    if (store.filterTypes.length === 0) return t('tasks.allTypes')
    if (store.filterTypes.length === 1) return getTaskTypeLabel(store.filterTypes[0], t)
    return `${store.filterTypes.length}`
  }
  function getStatusChipLabel(status?: string): string {
    if (status) return t(`tasks.status.${status}`)
    if (store.filterStatuses.length === 0) return t('tasks.allStatuses')
    if (store.filterStatuses.length === 1) return t(`tasks.status.${store.filterStatuses[0]}`)
    return `${store.filterStatuses.length}`
  }

  function toggleWarningDetail(id: string): void {
    const next = new Set(view.expandedWarningDetail.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    view.expandedWarningDetail.value = next
  }
  function formatWarningDetail(detail: string): string { return detail }

  // ============ fetch / refresh（直接调 store） ============
  async function fetchTasks(_opts?: { silent?: boolean }): Promise<void> {
    store.isRefreshing = true
    try {
      const list = await apiGetTasks()
      store.rebuildFromBackend(list)
    } catch (err) {
      console.warn('[useTasksList.fetchTasks] failed:', err)
    } finally {
      store.isRefreshing = false
    }
  }
  function refresh(): Promise<void> { return fetchTasks({ silent: true }) }

  // ============ 任务操作 ============
  async function cancelTaskById(id: string): Promise<void> {
    const task = store.getTaskById(id) ?? store.tasks.find((t) => t.id === id)
    if (!task) return
    const prevStatus = task.status
    store.patchTaskById(id, { status: 'cancelling' })
    try {
      await cancelTask(id)
    } catch (err) {
      store.patchTaskById(id, { status: prevStatus })
      throw err
    }
  }
  async function cancelRun(runId: string): Promise<void> {
    const previous = store.cancelRunTasks(runId)
    const errors: unknown[] = []
    for (const t of previous) {
      try { await cancelTask(t.id) } catch (err) { errors.push(err) }
    }
    if (errors.length > 0) {
      for (const t of previous) {
        store.patchTaskById(t.id, { status: t.status })
      }
      throw errors[0]
    }
  }
  async function retryTaskById(id: string): Promise<void> {
    try {
      await retryTaskApi(id)
      void fetchTasks({ silent: true })
    } catch (err) {
      console.warn('[useTasksList.retryTaskById] failed:', err)
    }
  }
  async function removeTaskById(id: string): Promise<void> {
    try {
      await removeTaskApi(id)
      store.removeTask(id)
    } catch (err) {
      console.warn('[useTasksList.removeTaskById] failed:', err)
    }
  }
  async function removeRunTasks(runId: string): Promise<{ removed: number; failed: number }> {
    const targets = store.removeRunTasks(runId)
    if (targets.length === 0) return { removed: 0, failed: 0 }
    let removed = 0, failed = 0
    for (const t of targets) {
      try { await removeTaskApi(t.id); removed++ }
      catch (err) { failed++ }
    }
    if (failed > 0) void fetchTasks({ silent: true })
    return { removed, failed }
  }
  async function clearCompletedWithConfirm(): Promise<number> {
    const completed = store.tasks.filter((tk) => tk.status === 'completed' || tk.status === 'cancelled' || tk.status === 'failed')
    if (completed.length === 0) return 0
    let deletedCount = 0
    for (const tk of completed) {
      try { await removeTaskApi(tk.id); store.removeTask(tk.id); deletedCount++ }
      catch (err) { console.warn('[useTasksList.clearCompleted] remove failed:', err) }
    }
    return deletedCount
  }

  // ============ search / sort / view 操作（透传到 store） ============
  function onSearchInput(event: any): void {
    const value = event?.target?.value ?? event?.detail?.value ?? event
    store.setSearchQuery(String(value ?? ''))
  }
  function toggleSort(): void { store.toggleSort() }
  function toggleViewMode(): void { store.toggleViewMode() }
  async function openPluginPopover(ev: any): Promise<void> { view.pluginPopoverEvent.value = ev; view.pluginPopoverOpen.value = true }
  async function openTypePopover(ev: any): Promise<void> { view.typePopoverEvent.value = ev; view.typePopoverOpen.value = true }
  async function openStatusPopover(ev: any): Promise<void> { view.statusPopoverEvent.value = ev; view.statusPopoverOpen.value = true }
  async function openDatePopover(ev: any): Promise<void> { view.datePopoverEvent.value = ev; view.datePopoverOpen.value = true }

  // ============ 兼容旧 API：applyTaskUpdate 等 ============
  function applyTaskUpdate(data: any) { store.applyEvent('update', data) }
  function applyTaskProgress(data: any) { store.applyEvent('progress', data) }
  function applyTaskCreated(data: any) { store.applyEvent('created', data) }
  function applyTaskCompleted(data: any) { store.applyEvent('completed', data) }

  // ============ loading / isInitialLoad 兼容（Tasks.vue 还在用） ============
  const loading = computed(() => false)
  const isInitialLoad = computed(() => !store.hydrated)

  return {
    // store state（通过 storeToRefs 保留 ref 包装，避免 Pinia 自动 unwrap 丢失响应性）
    ...storeRefs,
    // 兼容旧 computed
    loading,
    isInitialLoad,
    // 🆕 v7：视图层包装的 displayedItems（带 kind/counters/displayData）
    displayedItems,
    // UI state
    ...view,
    // helper
    getTaskName, getTaskIcon, getTaskColor, getStatusLabel, getStatusColor,
    getPhaseLabel, getTaskDuration, getPluginChipLabel, getTypeChipLabel,
    getStatusChipLabel, isPasswordError, toggleWarningDetail, formatWarningDetail,
    formatDateTime: dateFormat,
    // 操作
    fetchTasks, refresh,
    cancelRun, cancelTaskById, retryTaskById, removeTaskById, removeRunTasks, clearCompletedWithConfirm,
    onSearchInput, toggleSort, toggleViewMode,
    openPluginPopover, openTypePopover, openStatusPopover, openDatePopover,
    applyTaskUpdate, applyTaskProgress, applyTaskCreated, applyTaskCompleted,
    // 🆕 v7：透传 filter / pin / run 操作
    clearFilters: store.clearFilters,
    togglePluginFilter: store.togglePluginFilter,
    toggleTypeFilter: store.toggleTypeFilter,
    toggleStatusFilter: store.toggleStatusFilter,
    toggleTriggeredByFilter: store.toggleTriggeredByFilter,
    applyDatePreset: store.applyDatePreset,
    setCustomDateRange: store.setCustomDateRange,
    togglePinRun: store.togglePinRun,
    isRunPinned: store.isRunPinned,
    hydrate: store.hydrate,
    // 内部：给 group-detail 使用
    _computeGroupCounters: (tasks: EncvTask[]) => computeGroupCounters(tasks, storeRefs),
    workflowService,
  }
}

export function useTasksList() {
  if (_tasksListInstance) return _tasksListInstance
  _tasksListInstance = createUseTasksList()
  return _tasksListInstance
}
