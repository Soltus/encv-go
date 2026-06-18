import { ref, shallowRef, computed, watch } from 'vue'
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
  // 🆕 v4 2026-06-18 M2：拆分为 isInitialLoad（首屏占位）+ isRefreshing（下拉刷新专用）
  //   - 旧 loading 在 fetchTasks in-flight 期间会替换整个列表为 spinner → 闪动
  //   - isInitialLoad 只在 tasks.length===0 时显示 → 已有内容时不闪
  //   - isRefreshing 走 ion-refresher 自身的 spinner
  const isInitialLoad = ref(true)
  const isRefreshing = ref(false)
  // 兼容 Tasks.vue 老模板仍引用 loading
  const loading = computed(() => isInitialLoad.value || isRefreshing.value)
  const expandedWarningDetail = ref<string | null>(null)

  const showSearch = ref(false)
  const searchQuery = ref('')
  const showFilters = ref(false)
  const filterPlugins = ref<string[]>([])
  const filterTypes = ref<TaskType[]>([])
  const filterStatuses = ref<TaskStatus[]>([])

  // 🆕 v4 2026-06-18 M3：视图模式 + 日期筛选 + 排序
  type ViewMode = 'group' | 'flat'
  type DatePreset = 'today' | '7d' | '30d' | 'all' | 'custom'
  // 视图模式：聚合（按 runId 折叠为 group card）/ 平铺（每 task 一行）
  const VIEW_MODE_KEY = 'encv-tasks-viewmode'
  const initialViewMode: ViewMode =
    (typeof localStorage !== 'undefined' &&
      (localStorage.getItem(VIEW_MODE_KEY) as ViewMode)) || 'group'
  const viewMode = ref<ViewMode>(initialViewMode)
  // 日期区间筛选
  const filterDatePreset = ref<DatePreset>('all')
  const filterDateRange = ref<{ from?: string; to?: string }>({})
  // 排序键
  const sortBy = ref<SortBy>('activity')
  // 排序键 persistence
  watch(viewMode, (v) => {
    if (typeof localStorage !== 'undefined') localStorage.setItem(VIEW_MODE_KEY, v)
  })

  const statusOptions: TaskStatus[] = ['queued', 'running', 'completed', 'failed', 'cancelled']

  const pluginPopoverOpen = ref(false)
  const typePopoverOpen = ref(false)
  const statusPopoverOpen = ref(false)
  // 🆕 v4 M3：日期 popover
  const datePopoverOpen = ref(false)
  const datePopoverEvent = ref<Event | null>(null)
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

    // 🆕 v4 M3：日期区间筛选（与 plugin/type/status 并列的平级筛选）
    if (filterDateRange.value.from || filterDateRange.value.to) {
      const fromTs = filterDateRange.value.from ? new Date(filterDateRange.value.from).getTime() : -Infinity
      const toTs = filterDateRange.value.to ? new Date(filterDateRange.value.to).getTime() + 86400000 : Infinity
      result = result.filter((task) => {
        const ts = new Date(task.createdAt).getTime()
        return ts >= fromTs && ts < toTs
      })
    }

    return result
  })

  // 🆕 v4 M3：日期分隔 header 段（聚合 + 平铺模式都用）
  //   段名：今 / 昨 / 本周 / 本月 / 更早
  type DateSection = 'today' | 'yesterday' | 'thisWeek' | 'thisMonth' | 'earlier'
  const DATE_SECTION_LABELS: Record<DateSection, string> = {
    today: '今天',
    yesterday: '昨天',
    thisWeek: '本周',
    thisMonth: '本月',
    earlier: '更早',
  }
  const _today = new Date()
  const _startOfToday = new Date(_today.getFullYear(), _today.getMonth(), _today.getDate()).getTime()
  const _startOfYesterday = _startOfToday - 86400000
  const _startOfThisWeek = _startOfToday - _today.getDay() * 86400000
  const _startOfThisMonth = new Date(_today.getFullYear(), _today.getMonth(), 1).getTime()
  function dateSectionFor(ts: number): DateSection {
    if (ts >= _startOfToday) return 'today'
    if (ts >= _startOfYesterday) return 'yesterday'
    if (ts >= _startOfThisWeek) return 'thisWeek'
    if (ts >= _startOfThisMonth) return 'thisMonth'
    return 'earlier'
  }

  // 🆕 v4 M3：聚合模式 group items
  //   - 按 runId 分组 → 1 个 group = 1 个 run
  //   - group 排序：与平铺模式一致的"最新/最近完成"切换（最新默认）
  //   - 每个 group 计算：plugin × N / type × M / status × 6 / 日期命中
  //   - group 内部不排序：保持任务添加顺序
  type GroupHitCounters = {
    plugins: Record<string, { hit: number; total: number }>
    types: Record<TaskType, { hit: number; total: number }>
    statuses: Record<TaskStatus, { hit: number; total: number }>
    date: { hit: number; total: number }
    // 命中筛选条件
    hitAny: boolean
  }
  function computeGroupCounters(groupTasks: EncvTask[]): GroupHitCounters {
    const counters: GroupHitCounters = {
      plugins: {},
      types: {} as any,
      statuses: {} as any,
      date: { hit: 0, total: groupTasks.length },
      hitAny: false,
    }
    for (const t of groupTasks) {
      if (t.pluginName) {
        const p = counters.plugins[t.pluginName] ?? { hit: 0, total: 0 }
        p.total++
        if (filterPlugins.value.length === 0 || filterPlugins.value.includes(t.pluginName)) p.hit++
        counters.plugins[t.pluginName] = p
      }
      const ty = counters.types[t.type] ?? { hit: 0, total: 0 }
      ty.total++
      if (filterTypes.value.length === 0 || filterTypes.value.includes(t.type)) ty.hit++
      counters.types[t.type] = ty
      const st = counters.statuses[t.status] ?? { hit: 0, total: 0 }
      st.total++
      if (filterStatuses.value.length === 0 || filterStatuses.value.includes(t.status)) st.hit++
      counters.statuses[t.status] = st
      if (
        (filterDateRange.value.from || filterDateRange.value.to) &&
        t.createdAt >= (filterDateRange.value.from ?? '') &&
        t.createdAt < (filterDateRange.value.to ?? '~')
      ) {
        counters.date.hit++
      }
    }
    // hitAny: 在所有筛选条件下至少一项命中
    const pluginHit = filterPlugins.value.length === 0 || Object.values(counters.plugins).some((p) => p.hit > 0)
    const typeHit = filterTypes.value.length === 0 || Object.values(counters.types).some((p) => p.hit > 0)
    const statusHit = filterStatuses.value.length === 0 || Object.values(counters.statuses).some((p) => p.hit > 0)
    const dateHit =
      !filterDateRange.value.from && !filterDateRange.value.to
        ? true
        : counters.date.hit > 0
    counters.hitAny = pluginHit && typeHit && statusHit && dateHit
    return counters
  }

  // 🆕 v4 M3：聚合/平铺模式共用的 display item（TaskVirtualList 泛型 T extends { key: string } 要求单一类型）
  //   - 'date'         → 日期分隔 header（今/昨/本周/本月/更早）
  //   - 'group'        → 1 个 run 的 group card（带 hit counter chips + 展开后的子 task）
  //   - 'task'         → 平铺模式下的单 task 卡片
  type DisplayItem =
    | { kind: 'date'; section: DateSection; label: string; key: string }
    | {
        kind: 'group'
        runId: string
        /** 真实 runId；'__manual__'+taskId 表示 user 触发的单 task group */
        tasks: EncvTask[]
        counters: GroupHitCounters
        startedAt: number
        completedAt: number
        key: string
      }
    | { kind: 'task'; task: EncvTask; key: string }

  const groupedItems = computed<DisplayItem[]>(() => {
    // 聚合：先按 runId 分组；没有 runId 的 user 触发 task 视为单独的"无 runId"组（按 createdAt 排）
    const groupsByRun = new Map<string, EncvTask[]>()
    const ungrouped: EncvTask[] = []
    for (const t of filteredTasks.value) {
      if (t.runId) {
        const arr = groupsByRun.get(t.runId) ?? []
        arr.push(t)
        groupsByRun.set(t.runId, arr)
      } else {
        ungrouped.push(t)
      }
    }
    // 排序 group：sortBy 决定
    //  - activity: 取 group 内最新 completedAt || max createdAt
    //  - createdAt: 取 group 内 min createdAt
    const items: { runId: string; tasks: EncvTask[]; sortKey: number }[] = []
    for (const [runId, tasks] of groupsByRun) {
      let sortKey: number
      if (sortBy.value === 'activity') {
        // 平铺模式状态更新就重排 → 这里聚合模式 follow 平铺模式（同语义）
        const maxCompleted = Math.max(
          ...tasks.filter((t) => t.completedAt).map((t) => new Date(t.completedAt!).getTime()),
        )
        const maxCreated = Math.max(...tasks.map((t) => new Date(t.createdAt).getTime()))
        sortKey = isFinite(maxCompleted) && maxCompleted > 0 ? maxCompleted : maxCreated
      } else {
        sortKey = Math.max(...tasks.map((t) => new Date(t.createdAt).getTime()))
      }
      items.push({ runId, tasks, sortKey })
    }
    items.sort((a, b) => b.sortKey - a.sortKey)

    const result: DisplayItem[] = []
    let lastSection: DateSection | null = null
    for (const { runId, tasks } of items) {
      const counters = computeGroupCounters(tasks)
      // 决定 group 的 startedAt
      const createdTimes = tasks.map((t) => new Date(t.createdAt).getTime())
      const startedAt = Math.min(...createdTimes)
      const completedTimes = tasks
        .filter((t) => t.completedAt)
        .map((t) => new Date(t.completedAt!).getTime())
      const completedAt = completedTimes.length > 0 ? Math.max(...completedTimes) : 0
      const section = dateSectionFor(startedAt)
      if (section !== lastSection) {
        result.push({ kind: 'date', section, label: DATE_SECTION_LABELS[section], key: `date-${section}` })
        lastSection = section
      }
      result.push({ kind: 'group', runId, tasks, counters, startedAt, completedAt, key: `group-${runId}` })
    }
    // 无 runId 的 task 拼到最后（user 触发任务），按 createdAt 倒序
    const ungroupedSorted = [...ungrouped].sort(
      (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
    )
    for (const t of ungroupedSorted) {
      const ts = new Date(t.createdAt).getTime()
      const section = dateSectionFor(ts)
      if (section !== lastSection) {
        result.push({ kind: 'date', section, label: DATE_SECTION_LABELS[section], key: `date-${section}-manual` })
        lastSection = section
      }
      // 单独 task 也作为 kind:'group'（runId = '__manual__'，方便统一渲染）
      result.push({
        kind: 'group',
        runId: '__manual__' + t.id,
        tasks: [t],
        counters: computeGroupCounters([t]),
        startedAt: ts,
        completedAt: t.completedAt ? new Date(t.completedAt).getTime() : 0,
        key: `group-manual-${t.id}`,
      })
    }
    return result
  })

  // 🆕 v4 M3：平铺模式 items（含日期 header）
  const flatItems = computed<DisplayItem[]>(() => {
    const result: DisplayItem[] = []
    let lastSection: DateSection | null = null
    for (const t of filteredTasks.value) {
      const ts = new Date(t.createdAt).getTime()
      const section = dateSectionFor(ts)
      if (section !== lastSection) {
        result.push({ kind: 'date', section, label: DATE_SECTION_LABELS[section], key: `date-${section}` })
        lastSection = section
      }
      result.push({ kind: 'task', task: t, key: `task-${t.id}` })
    }
    return result
  })

  // 当前模式 items
  const displayedItems = computed(() =>
    viewMode.value === 'group' ? groupedItems.value : flatItems.value,
  )

  // 🆕 v4 M3：应用日期 preset → 展开为 filterDateRange
  function applyDatePreset(preset: DatePreset) {
    filterDatePreset.value = preset
    if (preset === 'all') {
      filterDateRange.value = {}
      return
    }
    const now = new Date()
    if (preset === 'today') {
      const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
      filterDateRange.value = { from: today.toISOString(), to: today.toISOString() }
      return
    }
    if (preset === '7d') {
      const d = new Date(now.getTime() - 7 * 86400000)
      filterDateRange.value = { from: d.toISOString(), to: now.toISOString() }
      return
    }
    if (preset === '30d') {
      const d = new Date(now.getTime() - 30 * 86400000)
      filterDateRange.value = { from: d.toISOString(), to: now.toISOString() }
      return
    }
    // custom：保持当前 range 不变（由 date popover 内部 datepicker 决定）
  }

  /**
   * 🆕 v4 M3：直接设置自定义日期范围（绑定到 date popover 内的 2 个 <input type="date">）
   * - from: YYYY-MM-DD 或 undefined
   * - to:   YYYY-MM-DD 或 undefined
   * - 自动联动 filterDatePreset = 'custom'（让 popover 高亮「自定义」）
   */
  function setCustomDateRange(from: string | undefined, to: string | undefined) {
    filterDatePreset.value = 'custom'
    filterDateRange.value = { from, to }
  }

  // 🆕 v4 M3：计数器点击 → toggle 筛选条件
  function toggleFilterFromCounter(dimension: 'plugin' | 'type' | 'status', value: string) {
    if (dimension === 'plugin') {
      const idx = filterPlugins.value.indexOf(value)
      if (idx >= 0) filterPlugins.value.splice(idx, 1)
      else filterPlugins.value.push(value)
    } else if (dimension === 'type') {
      const idx = filterTypes.value.indexOf(value as TaskType)
      if (idx >= 0) filterTypes.value.splice(idx, 1)
      else filterTypes.value.push(value as TaskType)
    } else if (dimension === 'status') {
      const idx = filterStatuses.value.indexOf(value as TaskStatus)
      if (idx >= 0) filterStatuses.value.splice(idx, 1)
      else filterStatuses.value.push(value as TaskStatus)
    }
  }

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

  async function fetchTasks(opts?: { refresh?: boolean }) {
    const isRefresh = !!opts?.refresh
    if (isRefresh) {
      isRefreshing.value = true
    }
    // 首屏占位只在首次且 tasks 为空时
    const showInitialPlaceholder = tasks.value.length === 0
    if (showInitialPlaceholder) {
      isInitialLoad.value = true
    }
    try {
      const data = await getTasks()
      const enriched = mergeWithLocalState(data)
      // 🆕 v4 M2：dirty-write 成功后立刻覆盖；失败时保留旧数据
      tasks.value = enriched
    } catch (err) {
      // 失败：保留旧 tasks.value（不抹除）；只在空状态下才回退到 []
      console.warn('[useTasksList] fetchTasks failed, keeping previous tasks:', err)
      if (tasks.value.length === 0) {
        tasks.value = []
      }
    } finally {
      isInitialLoad.value = false
      isRefreshing.value = false
    }
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
      await fetchTasks({ refresh: true })
    } catch {
      showToast({ message: t('tasks.taskCancelFailed'), duration: 2000, color: 'danger' })
    }
  }

  async function retryTaskById(id: string) {
    try {
      await retryTask(id)
      await fetchTasks({ refresh: true })
    } catch {
      showToast({ message: t('tasks.taskRetryFailed'), duration: 2000, color: 'danger' })
    }
  }

  async function removeTaskById(id: string) {
    try {
      await removeTask(id)
      await fetchTasks({ refresh: true })
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
    // 🆕 v4 M2：拆分 isInitialLoad（首屏占位）+ isRefreshing（下拉刷新专用）
    isInitialLoad,
    isRefreshing,
    expandedWarningDetail,
    sortBy,
    showSearch,
    searchQuery,
    showFilters,
    filterPlugins,
    filterTypes,
    filterStatuses,
    // 🆕 v4 M3：视图模式 + 日期筛选 + 分组 items
    viewMode,
    filterDatePreset,
    filterDateRange,
    datePopoverOpen,
    datePopoverEvent,
    groupedItems,
    flatItems,
    displayedItems,
    applyDatePreset,
    setCustomDateRange,
    toggleFilterFromCounter,
    toggleViewMode() {
      viewMode.value = viewMode.value === 'group' ? 'flat' : 'group'
    },
    openDatePopover(event: Event) {
      datePopoverEvent.value = event
      datePopoverOpen.value = true
    },
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
