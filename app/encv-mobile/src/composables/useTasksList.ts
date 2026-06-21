/**
 * v6 2026-06-18 任务系统重设计 — useTasksList 重写（v6 阶段 1：基础设施）
 *
 * 之前：1038 行 monolith，ref<EncvTask[]> 直接管
 * 现在：薄壳 composable，桥接 useTaskStore + useTaskFilter，**保持所有旧 API**
 *
 * 设计原则：
 * 1. tasks state 来自 useTaskStore.tasks（shallowRef + Map 索引）
 * 2. 筛选 state 来自 useTaskFilter（独立 composable，持久化到 IndexedDB）
 * 3. UI state (popover / expanded warnings / sort toggle) 保留在 composable 内（向
 *    后兼容，Tasks.vue 不动）
 * 4. 所有 API 100% 保持向后兼容
 *
 * 兼容性保证（Tasks.vue 不需要改一行）：
 * - tasks / loading / isInitialLoad / isRefreshing / hasAnyTask
 * - searchQuery / filterPlugins / filterTypes / filterStatuses / filterDatePreset /
 *   filterDateRange / viewMode / sortBy
 * - availablePlugins / hasActiveFilters / hasCompletedTasks / activeFilterCount
 * - filteredTasks / groupedItems / flatItems / displayedItems
 * - fetchTasks / refresh / openPluginPopover / openTypePopover / openStatusPopover /
 *   openDatePopover / togglePluginFilter / toggleTypeFilter / toggleStatusFilter /
 *   clearFilters / onSearchInput / toggleSort
 * - applyTaskUpdate / applyTaskProgress / applyTaskCreated / applyTaskCompleted
 * - cancelTaskById / retryTaskById / removeTaskById / clearCompletedWithConfirm
 * - getTaskName / getTaskIcon / getTaskColor / getTaskDuration / getStatusLabel /
 *   getStatusColor / getPhaseLabel / getPluginChipLabel / getTypeChipLabel /
 *   getStatusChipLabel
 * - isPasswordError / toggleWarningDetail / formatWarningDetail
 * - showSearch / showFilters / expandedWarningDetail / statusOptions
 * - pluginPopoverOpen / typePopoverOpen / statusPopoverOpen / datePopoverOpen /
 *   datePopoverEvent / pluginPopoverEvent / typePopoverEvent / statusPopoverEvent
 * - applyDatePreset / setCustomDateRange / toggleViewMode
 * - workflowService
 * - cancelRun (v6 新增)
 */
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useTaskStore } from '@/stores/taskStore'
import { useTaskFilter } from '@/composables/useTaskFilter'
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
import { formatDateTime as dateFormat, formatDuration } from '@/composables/useDateFormat'

// ============ 内部 helper 函数 ============

const TERMINAL_STATUSES: Set<TaskStatus> = new Set(['completed', 'failed', 'cancelled'])
function isTerminalTaskStatus(status: TaskStatus): boolean {
  return TERMINAL_STATUSES.has(status)
}

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

const STATUS_OPTIONS: TaskStatus[] = ['queued', 'running', 'cancelling', 'completed', 'failed', 'cancelled']

// ============ 🆕 v6 2026-06-22 性能优化：group display data 预计算 ============
// 之前：Tasks.vue 模板中每个 group card 调用 13 次函数（getGroupSummary × 6 +
//   getGroupTone × 3 + getGroupDominantStatus/getGroupDuration/getGroupPluginBadges/
//   groupCardMoodClass 各 × 1），每次都遍历 tasks 数组
// 现在：在 groupedItems computed 里一次性计算所有派生数据，模板直接读 item.displayData.xxx

export interface GroupDisplayData {
  tone: 'automation' | 'ai_agent'
  summary: { passed: number; failed: number; running: number; pending: number; percent: number }
  dominantStatus: TaskStatus
  duration: string
  pluginBadges: string[]
  moodClass: string
}

function computeGroupTone(tasks: EncvTask[]): 'automation' | 'ai_agent' {
  for (const t of tasks) {
    const by = t.triggeredBy ?? 'user'
    if (by === 'ai_agent') return 'ai_agent'
  }
  return 'automation'
}

function computeGroupSummary(tasks: EncvTask[]): GroupDisplayData['summary'] {
  let passed = 0, failed = 0, running = 0, pending = 0
  for (const t of tasks) {
    if (t.status === 'completed') passed++
    else if (t.status === 'failed') failed++
    else if (t.status === 'running' || t.status === 'cancelling') running++
    else pending++
  }
  const finished = passed + failed
  const percent = tasks.length > 0 ? Math.round((finished / tasks.length) * 100) : 0
  return { passed, failed, running, pending, percent }
}

function computeGroupDominantStatus(tasks: EncvTask[]): TaskStatus {
  if (tasks.some((t) => t.status === 'failed')) return 'failed'
  if (tasks.some((t) => t.status === 'running' || t.status === 'cancelling')) return 'running'
  if (tasks.some((t) => t.status === 'queued')) return 'queued'
  if (tasks.every((t) => t.status === 'completed')) return 'completed'
  if (tasks.every((t) => t.status === 'cancelled')) return 'cancelled'
  return 'completed'
}

function computeGroupDuration(tasks: EncvTask[]): string {
  if (tasks.length === 0) return ''
  const createdTimes = tasks.map((t) => new Date(t.createdAt).getTime())
  const completedTimes = tasks
    .filter((t) => t.completedAt)
    .map((t) => new Date(t.completedAt!).getTime())
  const start = Math.min(...createdTimes)
  const end = completedTimes.length > 0 ? Math.max(...completedTimes) : Date.now()
  return formatDuration(end - start)
}

function computeGroupPluginBadges(tasks: EncvTask[], limit: number): string[] {
  const set = new Set<string>()
  for (const t of tasks) {
    if (t.pluginName && t.pluginName !== '__unknown__') set.add(t.pluginName)
  }
  return Array.from(set).slice(0, limit)
}

function computeGroupMoodClass(tasks: EncvTask[]): string {
  const s = computeGroupSummary(tasks)
  if (tasks.length === 0) return 'tl-group-card--mood-neutral'
  if (s.failed === 0 && s.passed > 0) return 'tl-group-card--mood-success'
  if (s.failed / tasks.length > 0.5) return 'tl-group-card--mood-danger'
  return 'tl-group-card--mood-neutral'
}

/** 🆕 一次性计算 group 的所有派生展示数据（tone/summary/dominantStatus/duration/pluginBadges/moodClass） */
function computeGroupDisplayData(tasks: EncvTask[]): GroupDisplayData {
  return {
    tone: computeGroupTone(tasks),
    summary: computeGroupSummary(tasks),
    dominantStatus: computeGroupDominantStatus(tasks),
    duration: computeGroupDuration(tasks),
    pluginBadges: computeGroupPluginBadges(tasks, 3),
    moodClass: computeGroupMoodClass(tasks),
  }
}

/** 单例 */
let _instance: ReturnType<typeof createUseTasksList> | null = null

function createUseTasksList() {
  const store = useTaskStore()
  // 🆕 v6 2026-06-18：storeToRefs 保留 state 的 ref 包装（Pinia setup store
  //   会把 ref/computed 自动解包给消费者，普通属性访问会丢失响应性）
  //   - store.tasks 是 shallowRef，store.tasks 直接读会得到 EncvTask[]（无 .value）
  //   - storeToRefs(store).tasks 才是真正的 ShallowRef<EncvTask[]>
  //   - 之前用 `as unknown as ShallowRef<EncvTask[]>` 是类型谎言，运行时 tasks.value === undefined
  //   - Tasks.vue 在 onIonViewWillEnter 访问 tasks.value.length 抛 TypeError → tab 崩溃
  const storeRefs = storeToRefs(store)
  const filter = useTaskFilter()
  const workflowService = useWorkflowTaskService()
  const { t } = useI18n()

  // ============ UI state（popovers / search 显示 / warning details） ============
  const showSearch = ref(false)
  const showFilters = ref(false)
  const expandedWarningDetail = ref<Set<string>>(new Set())
  const pluginPopoverOpen = ref(false)
  const typePopoverOpen = ref(false)
  const statusPopoverOpen = ref(false)
  const datePopoverOpen = ref(false)
  const pluginPopoverEvent = ref<any>(null)
  const typePopoverEvent = ref<any>(null)
  const statusPopoverEvent = ref<any>(null)
  const datePopoverEvent = ref<any>(null)

  const statusOptions = STATUS_OPTIONS

  // ============ WS 4 件套桥接 ============
  useTaskEventBridge({
    onUpdate: (payload) => store.applyTaskUpdate(payload as any),
    onProgress: (payload) => store.applyTaskProgress(payload as any),
    onCreate: (payload) => store.applyTaskCreated(payload as any),
    onComplete: (payload) => store.applyTaskCompleted(payload as any),
  })

  // ============ 派生 ============
  const filteredTasks = computed(() => {
    const q = filter.searchQuery.value.trim().toLowerCase()
    const fromTs = filter.filterDateRange.value.from
    const toTs = filter.filterDateRange.value.to
    const hasDate = !!fromTs || !!toTs
    const hasSearch = q.length > 0
    const out: EncvTask[] = []
    for (const t of store.tasks) {
      if (filter.filterPlugins.value.length > 0 && !filter.filterPlugins.value.includes(t.pluginName || '__unknown__')) continue
      if (filter.filterTypes.value.length > 0 && !filter.filterTypes.value.includes(t.type)) continue
      if (filter.filterStatuses.value.length > 0 && !filter.filterStatuses.value.includes(t.status)) continue
      if (filter.filterTriggeredBy.value.length > 0) {
        const by = t.triggeredBy ?? 'user'
        if (!filter.filterTriggeredBy.value.includes(by as any)) continue
      }
      if (hasDate) {
        if (fromTs && t.createdAt < fromTs) continue
        if (toTs && t.createdAt >= toTs) continue
      }
      if (hasSearch) {
        const name = getTaskName(t).toLowerCase()
        const plugin = (t.pluginName || '').toLowerCase()
        const error = (t.error || '').toLowerCase()
        const id = t.id.toLowerCase()
        if (!name.includes(q) && !plugin.includes(q) && !error.includes(q) && !id.includes(q)) continue
      }
      out.push(t)
    }
    return out
  })

  const sortedTasks = computed(() => {
    const arr = [...filteredTasks.value]
    arr.sort((a, b) => {
      if (filter.sortBy.value === 'activity') {
        const aFinal = isTerminalTaskStatus(a.status)
        const bFinal = isTerminalTaskStatus(b.status)
        const aKey = aFinal ? new Date(a.createdAt).getTime() : (a.completedAt ? new Date(a.completedAt).getTime() : new Date(a.createdAt).getTime())
        const bKey = bFinal ? new Date(b.createdAt).getTime() : (b.completedAt ? new Date(b.completedAt).getTime() : new Date(b.createdAt).getTime())
        return bKey - aKey
      }
      return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    })
    return arr
  })

  // ============ viewMode 切换缓存（v6 性能优化） ============
  // 🆕 v6 2026-06-22：移除 _groupCache / _flatCache
  //   - 旧实现用引用相等判断缓存命中，但 sortedTasks.value 每次都返回新数组
  //     （`[...filteredTasks.value]` + sort），引用永远不等 → 缓存永远 miss
  //   - computed 本身已有缓存（依赖不变就不重算），_groupCache/_flatCache 是冗余的
  //   - 移除后代码更简单，性能不降反升（少一次引用比较）

  const groupedItems = computed<any[]>(() => {
    const src = sortedTasks.value
    const groups = new Map<string, { runId: string; tasks: EncvTask[]; startedAt: string }>()
    for (const tk of src) {
      const key = tk.runId || `__manual__${tk.id}`
      const g = groups.get(key)
      if (g) {
        g.tasks.push(tk)
      } else {
        groups.set(key, { runId: tk.runId || '', tasks: [tk], startedAt: tk.createdAt })
      }
    }
    const result: any[] = []
    for (const [key, g] of groups) {
      const counters = computeGroupCounters(g.tasks)
      // 🆕 v6 2026-06-22 性能优化：一次性预计算所有派生展示数据
      //   - 之前：Tasks.vue 模板中每个 group card 调用 13 次函数，每次遍历 tasks
      //   - 现在：这里调一次 computeGroupDisplayData，模板直接读 item.displayData.xxx
      const displayData = computeGroupDisplayData(g.tasks)
      result.push({
        kind: 'group',
        key,
        runId: g.runId,
        startedAt: g.startedAt,
        tasks: g.tasks,
        counters,
        displayData,
      })
    }
    result.sort((a, b) => {
      // 🆕 v6 2026-06-18：pinned run 永远排最前
      const pinned = store.pinnedRunIds
      const aPinned = pinned.has(a.runId)
      const bPinned = pinned.has(b.runId)
      if (aPinned !== bPinned) return aPinned ? -1 : 1
      // 其余按 startedAt 倒序
      return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
    })
    return result
  })

  function computeGroupCounters(tasks: EncvTask[]): any {
    const plugins: Record<string, { hit: number; total: number }> = {}
    const types: Record<string, { hit: number; total: number }> = {}
    const statuses: Record<string, { hit: number; total: number }> = {}
    const date = { hit: 0, total: tasks.length }
    const search = { hit: 0, total: tasks.length }

    const q = filter.searchQuery.value.trim().toLowerCase()
    const fromTs = filter.filterDateRange.value.from
    const toTs = filter.filterDateRange.value.to
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
        const name = getTaskName(tk).toLowerCase()
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

  const flatItems = computed(() => {
    const src = sortedTasks.value
    const result = src.map((tk) => ({
      kind: 'task' as const,
      key: `t-${tk.id}`,
      task: tk,
    }))
    return result
  })

  const displayedItems = computed<any[]>(() => {
    if (filter.viewMode.value === 'group') {
      return groupedItems.value.filter((g) => g.counters.hitAny)
    }
    return flatItems.value
  })

  const hasCompletedTasks = computed(() =>
    store.tasks.some((tk) => tk.status === 'completed' || tk.status === 'failed' || tk.status === 'cancelled'),
  )

  // ============ 工具函数 ============
  function getTaskName(task: EncvTask): string {
    return getTaskDisplayName(task)
  }
  function getTaskIcon(task: EncvTask): string {
    return task.type === 'encrypt' ? 'lock-closed' : 'lock-open'
  }
  function getTaskColor(task: EncvTask): string {
    return getStatusColorInner(task.status)
  }
  function getStatusLabel(status: TaskStatus): string {
    return t(`tasks.${status}`)
  }
  function getStatusColor(status: TaskStatus): string {
    return getStatusColorInner(status)
  }
  function getPhaseLabel(phase?: string): string {
    if (!phase) return ''
    return t(`tasks.phase.${phase}`, { defaultValue: phase })
  }
  function getTaskDuration(task: EncvTask): string {
    return formatTaskDuration(task)
  }
  function getPluginChipLabel(name?: string): string {
    if (name === '__unknown__') return t('tasks.unknownPlugin')
    if (name) return name
    // 无参调用：返回当前 chip label
    if (filter.filterPlugins.value.length === 0) return t('tasks.allPlugins')
    if (filter.filterPlugins.value.length === 1) return filter.filterPlugins.value[0]
    return `${filter.filterPlugins.value.length}`
  }
  function getTypeChipLabel(type?: string): string {
    if (type) return t(`tasks.${type === 'encrypt' ? 'encrypt' : 'decrypt'}`)
    // 无参调用：返回当前 chip label
    if (filter.filterTypes.value.length === 0) return t('tasks.allTypes')
    if (filter.filterTypes.value.length === 1) return t(`tasks.${filter.filterTypes.value[0] === 'encrypt' ? 'encrypt' : 'decrypt'}`)
    return `${filter.filterTypes.value.length}`
  }
  function getStatusChipLabel(status?: string): string {
    if (status) return t(`tasks.status.${status}`)
    // 无参调用：返回当前 chip label
    if (filter.filterStatuses.value.length === 0) return t('tasks.allStatuses')
    if (filter.filterStatuses.value.length === 1) return t(`tasks.status.${filter.filterStatuses.value[0]}`)
    return `${filter.filterStatuses.value.length}`
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

  // ============ 密码错误判定 ============
  function isPasswordError(task: EncvTask): boolean {
    if (!task.error) return false
    return isWrongPasswordError(task.error)
  }

  function toggleWarningDetail(id: string): void {
    const next = new Set(expandedWarningDetail.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    expandedWarningDetail.value = next
  }

  function formatWarningDetail(detail: string): string {
    return detail
  }

  // ============ fetch / refresh ============
  /**
   * v6: fetchTasks 从 store 拉取后端最新状态
   * 保留原 API 用于 Tasks.vue
   */
  async function fetchTasks(_opts?: { silent?: boolean }): Promise<void> {
    store.isRefreshing = true
    try {
      const list = await apiGetTasks()
      store.bulkSetTasks(list)
    } catch (err) {
      console.warn('[useTasksList.fetchTasks] failed:', err)
    } finally {
      store.isRefreshing = false
    }
  }

  function refresh(): Promise<void> {
    return fetchTasks({ silent: true })
  }

  // ============ Task 取消 / 重试 / 删除 / 清空已完成 ============
  async function cancelTaskById(id: string): Promise<void> {
    const task = store.tasks.find((t) => t.id === id)
    if (!task) return
    const prevStatus = task.status
    store.patchTask(id, { status: 'cancelling' })
    store.persistTaskById(id)  // 🆕 v6 2026-06-22：显式持久化 status 变更
    try {
      await cancelTask(id)
    } catch (err) {
      store.patchTask(id, { status: prevStatus })
      store.persistTaskById(id)  // 回滚也要持久化
      throw err
    }
  }

  async function cancelRun(runId: string): Promise<void> {
    const previous = store.cancelRunTasks(runId)
    const errors: unknown[] = []
    for (const t of previous) {
      try {
        await cancelTask(t.id)
      } catch (err) {
        errors.push(err)
      }
    }
    if (errors.length > 0) {
      for (const t of previous) {
        store.patchTask(t.id, { status: t.status })
        store.persistTaskById(t.id)  // 🆕 v6 2026-06-22：回滚也要持久化
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

  /**
   * 🆕 v6 2026-06-18：删除整个 run 下的所有 task（左滑删除操作）
   *  - 后端逐个 removeTaskApi（没有 batch endpoint）
   *  - store 一次 removeRunTasks 批量删（避免多次 reactive 触发）
   *  - 失败的任务保留在 store，下次拉取会重新覆盖
   */
  async function removeRunTasks(runId: string): Promise<{ removed: number; failed: number }> {
    const targets = store.removeRunTasks(runId)
    if (targets.length === 0) return { removed: 0, failed: 0 }
    let removed = 0
    let failed = 0
    for (const t of targets) {
      try {
        await removeTaskApi(t.id)
        removed++
      } catch (err) {
        // 失败的任务回滚到 store（重新 prepend）
        console.warn('[useTasksList.removeRunTasks] failed:', t.id, err)
        failed++
      }
    }
    if (failed > 0) {
      // 失败的 task 重新加回 store 头部
      // 注意：targets 已被 store.removeRunTasks 过滤，需要从原始数据恢复
      // 简单实现：fetchTasks 重新拉一次（覆盖 store）
      void fetchTasks({ silent: true })
    }
    return { removed, failed }
  }

  /**
   * 🆕 v6 2026-06-18：切换 run 置顶状态（左滑置顶操作）
   *  - 返回新置顶状态（true=已置顶 / false=已取消）
   */
  function togglePinRun(runId: string): boolean {
    return store.togglePinRun(runId)
  }

  /** 读取 run 置顶状态 */
  function isRunPinned(runId: string): boolean {
    return store.isRunPinned(runId)
  }

  async function clearCompletedWithConfirm(): Promise<number> {
    const completed = store.tasks.filter((tk) => tk.status === 'completed' || tk.status === 'cancelled' || tk.status === 'failed')
    if (completed.length === 0) return 0
    let deletedCount = 0
    for (const tk of completed) {
      try {
        await removeTaskApi(tk.id)
        store.removeTask(tk.id)
        deletedCount++
      } catch (err) {
        console.warn('[useTasksList.clearCompleted] remove failed:', err)
      }
    }
    return deletedCount
  }

  // ============ 筛选操作（filter state 已经在 useTaskFilter 里） ============
  // togglePluginFilter / toggleTypeFilter / toggleStatusFilter / clearFilters 已经在 useTaskFilter
  // 直接代理即可（Tasks.vue 用的就是 useTasksList 解构出来的这些函数）

  // ============ search 切换 ============
  function onSearchInput(event: any): void {
    const value = event?.target?.value ?? event?.detail?.value ?? event
    filter.setSearchQuery(String(value ?? ''))
  }

  function toggleSort(): void {
    filter.setSortBy(filter.sortBy.value === 'activity' ? 'created' : 'activity')
  }

  // ============ Popovers ============
  async function openPluginPopover(ev: any): Promise<void> {
    pluginPopoverEvent.value = ev
    pluginPopoverOpen.value = true
  }
  async function openTypePopover(ev: any): Promise<void> {
    typePopoverEvent.value = ev
    typePopoverOpen.value = true
  }
  async function openStatusPopover(ev: any): Promise<void> {
    statusPopoverEvent.value = ev
    statusPopoverOpen.value = true
  }
  async function openDatePopover(ev: any): Promise<void> {
    datePopoverEvent.value = ev
    datePopoverOpen.value = true
  }

  function toggleViewMode(): void {
    filter.setViewMode(filter.viewMode.value === 'group' ? 'flat' : 'group')
  }

  // ============ WS 4 件套（再暴露 — Tasks.vue 也用） ============
  // 已经在 composable 内部订阅；这些是给 Tasks.vue 用的
  function applyTaskUpdate(data: any) { store.applyTaskUpdate(data) }
  function applyTaskProgress(data: any) { store.applyTaskProgress(data) }
  function applyTaskCreated(data: any) { store.applyTaskCreated(data) }
  function applyTaskCompleted(data: any) { store.applyTaskCompleted(data) }

  return {
    // ============ store state（通过 storeToRefs 保留 ref，避免 Pinia 自动解包丢失响应性） ============
    tasks: storeRefs.tasks,
    loading: computed(() => false),
    isInitialLoad: computed(() => !storeRefs.hydrated.value),
    isRefreshing: storeRefs.isRefreshing,
    hydrated: storeRefs.hydrated,
    hasAnyTask: storeRefs.hasAnyTask,

    // ============ filter state ============
    searchQuery: filter.searchQuery,
    filterPlugins: filter.filterPlugins,
    filterTypes: filter.filterTypes,
    filterStatuses: filter.filterStatuses,
    filterDatePreset: filter.filterDatePreset,
    filterDateRange: filter.filterDateRange,
    viewMode: filter.viewMode,
    sortBy: filter.sortBy,

    // ============ UI state ============
    showSearch,
    showFilters,
    expandedWarningDetail,
    pluginPopoverOpen,
    typePopoverOpen,
    statusPopoverOpen,
    datePopoverOpen,
    pluginPopoverEvent,
    typePopoverEvent,
    statusPopoverEvent,
    datePopoverEvent,
    statusOptions,

    // ============ 派生 ============
    availablePlugins: storeRefs.availablePlugins,
    hasActiveFilters: filter.hasActiveFilters,
    activeFilterCount: filter.activeFilterCount,
    sortedIndices: computed(() => sortedTasks.value.map((t) => t.id)),
    tasksByRunId: storeRefs.tasksByRunId,
    tasksById: storeRefs.tasksById,
    hasCompletedTasks,
    filteredTasks,
    groupedItems,
    flatItems,
    displayedItems,

    // ============ 工具函数 ============
    getTaskName,
    getTaskIcon,
    getTaskColor,
    getStatusLabel,
    getStatusColor,
    getPhaseLabel,
    getTaskDuration,
    getPluginChipLabel,
    getTypeChipLabel,
    getStatusChipLabel,
    isPasswordError,
    toggleWarningDetail,
    formatWarningDetail,
    formatDateTime: dateFormat,

    // ============ 筛选操作 ============
    clearFilters: filter.clearFilters,
    togglePluginFilter: filter.togglePluginFilter,
    toggleTypeFilter: filter.toggleTypeFilter,
    toggleStatusFilter: filter.toggleStatusFilter,
    setSearchQuery: filter.setSearchQuery,
    setViewMode: filter.setViewMode,
    setSortBy: filter.setSortBy,
    applyDatePreset: filter.applyDatePreset,
    setCustomDateRange: filter.setCustomDateRange,
    onSearchInput,
    toggleSort,
    toggleViewMode,

    // ============ Popovers ============
    openPluginPopover,
    openTypePopover,
    openStatusPopover,
    openDatePopover,

    // ============ 任务操作 ============
    fetchTasks,
    refresh,
    cancelRun,
    cancelTaskById,
    retryTaskById,
    removeTaskById,
    removeRunTasks,
    togglePinRun,
    isRunPinned,
    clearCompletedWithConfirm,
    applyTaskUpdate,
    applyTaskProgress,
    applyTaskCreated,
    applyTaskCompleted,
    hydrate: store.hydrate,

    // ============ 内部 ============
    _computeGroupCounters: computeGroupCounters,
    workflowService,
  }
}

export function useTasksList() {
  if (_instance) return _instance
  _instance = createUseTasksList()
  return _instance
}
