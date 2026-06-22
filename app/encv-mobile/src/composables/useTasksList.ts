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
import { useRunSummariesSingleton } from '@/composables/useRunSummaries'
import { useTaskViewCompute } from '@/composables/useTaskViewCompute'
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

  // ============ WS 4 件套桥接（按视图上下文过滤） ============
  // 🆕 2026-06-23 Task 9：WS 事件按视图上下文过滤（spec §2.3）
  //   - Tasks 列表页是"视图分页"：store 只持有当前页的 task（~100 个）
  //   - WS task:created 不进 store（task 可能不属于当前页；手动创建走 task:refresh）
  //     → 只触发 runSummaries 刷新（group card 计数从后端 SQL 拿）
  //   - WS task:update / task:progress / task:completed：只 patch 已加载的 task
  //     → patchTaskById 不在 store 则 return false（天然过滤）
  //   - WS task:completed：额外刷新对应 runId 的 summary（计数变化）
  //   - 离开 Tasks 页时 useTaskEventBridge 的 onUnmounted 自动 stopListening
  const runSummaries = useRunSummariesSingleton()
  useTaskEventBridge({
    onUpdate: (payload) => store.applyEvent('update', payload),
    onProgress: (payload) => store.applyEvent('progress', payload),
    // 🆕 Task 9：task:created 不进 store，只刷新 summary
    //   - 手动创建任务走 eventBus.emit('task:refresh') → fetchTasks 从后端加载
    //   - 自动化/workflow 创建的 task 属于 run，用户通过 group card → GroupDetail 查看
    //   - 这里只刷新 summary，让 group card 计数实时更新
    onCreate: (payload) => {
      const runId = (payload as any)?.runId
      if (runId) {
        void runSummaries.refreshOnTaskCompleted(runId)
      } else {
        // 没有 runId（旧任务）→ fetchAll 刷新所有 run summary
        void runSummaries.fetchAll()
      }
    },
    onComplete: (payload) => {
      store.applyEvent('completed', payload)
      // 🆕 Task 9：task 完成后刷新对应 runId 的 summary（计数变化）
      //   - patchTaskById 只 patch 已加载的 task，不在 store 则 return false
      //   - 但 summary 必须刷新（group card 显示的 passed/failed 计数）
      //   - 从 store 查找 task 拿 runId（payload 可能不带 runId）
      const taskId = payload?.id
      const task = taskId ? store.getTaskById(taskId) : undefined
      const runId = task?.runId
      if (runId) {
        void runSummaries.refreshOnTaskCompleted(runId)
      }
    },
  })

  // ============ displayedItems（视图层包装：注入 kind/counters/displayData） ============
  // ⚠️ 模板 TaskVirtualList 期望 item.kind === 'date' | 'group' | 'task' 三种
  //   - date: { kind:'date', key, label }
  //   - group: { kind:'group', key, runId, startedAt, tasks, counters, displayData }
  //   - task: { kind:'task', key, task }
  // 🆕 2026-06-23 Task 10：dateSectionKey / buildGroupDisplayData 已迁移到
  //   useTaskViewCompute（worker + computeSync 内联），此处不再保留

  // ============ displayedItems（Web Worker 委托计算） ============
  // 🆕 2026-06-23 Task 10：把 O(N) 视图计算委托给 Web Worker，避免阻塞 UI 主线程
  //   - 1000+ task 时 displayedItems computed 遍历会卡顿
  //   - Worker 接收 tasks 快照 + filter/sort/view 状态，返回 displayedItems 数组
  //   - debounce 16ms（1 帧）→ postMessage → 接收结果 → 更新 ref
  //   - 降级：Worker 不可用 → 主线程同步计算（computeSync）
  //   - date section label：worker 返回 dateKey，主线程映射为 i18n label
  //   - 旧的 dateSectionKey / buildGroupDisplayData / computeGroupCounters 函数保留
  //     作为 fallback（computeSync 内联了相同逻辑，但保留这些函数供测试 _computeGroupCounters 用）
  const { displayedItems: workerDisplayedItems } = useTaskViewCompute({
    tasks: storeRefs.tasks,
    viewMode: storeRefs.viewMode,
    sortBy: storeRefs.sortBy,
    searchQuery: storeRefs.searchQuery,
    filterPlugins: storeRefs.filterPlugins,
    filterTypes: storeRefs.filterTypes,
    filterStatuses: storeRefs.filterStatuses,
    filterTriggeredBy: storeRefs.filterTriggeredBy,
    filterDateRange: storeRefs.filterDateRange,
    pinnedRunIds: storeRefs.pinnedRunIds,
  })
  // 暴露为 ref（与原 computed 接口兼容，模板用 displayedItems.value 访问）
  const displayedItems = workerDisplayedItems as any

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

  // ============ fetch / refresh / loadMore（视图分页加载） ============
  // 🆕 2026-06-23 重新设计（后端 SQL 权威 + 前端视图分页）：
  //   - 后端是唯一权威：GET /api/tasks?runId=&offset=&limit= + X-Total-Count
  //   - 前端 store 只持有"当前视图需要的"task（不是所有 task）
  //   - 聚合计数独立：后端 SQL COUNT，前端不靠 store.tasks.length 算总数
  //   - WS task:created 守卫：store 满后不 push（避免视图分页被破坏）
  //     跳过的 task 在 loadMore / refresh 时从后端获取最新状态
  const PAGE_SIZE = 100
  const _paginationOffset = ref(0)
  const hasMore = ref(false)
  const isLoadingMore = ref(false)

  async function fetchTasks(_opts?: { silent?: boolean }): Promise<void> {
    store.isRefreshing = true
    try {
      const list = await apiGetTasks({ offset: 0, limit: PAGE_SIZE })
      store.rebuildFromBackend(list)
      _paginationOffset.value = 0
      hasMore.value = list.length >= PAGE_SIZE
    } catch (err) {
      console.warn('[useTasksList.fetchTasks] failed:', err)
    } finally {
      store.isRefreshing = false
    }
  }
  function refresh(): Promise<void> { return fetchTasks({ silent: true }) }

  async function loadMore(): Promise<void> {
    if (isLoadingMore.value || !hasMore.value) return
    isLoadingMore.value = true
    try {
      const nextOffset = _paginationOffset.value + PAGE_SIZE
      const list = await apiGetTasks({ offset: nextOffset, limit: PAGE_SIZE })
      store.appendTasksPage(list)
      _paginationOffset.value = nextOffset
      hasMore.value = list.length >= PAGE_SIZE
    } catch (err) {
      console.warn('[useTasksList.loadMore] failed:', err)
    } finally {
      isLoadingMore.value = false
    }
  }

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
    fetchTasks, refresh, loadMore,
    hasMore, isLoadingMore,
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

/** 🆕 测试用：重置 module-level singleton（让下一次 useTasksList() 重新创建 instance） */
export function _resetTasksListSingletonForTests(): void {
  _tasksListInstance = null
  _viewInstance = null
}
