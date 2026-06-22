/**
 * taskStore — 唯一任务数据 + 视图状态 owner
 *
 * 设计原则（v7 2026-06-22 重构）：
 * 1. 后端是权威，WS 推什么用什么 — 单一 applyEvent 入口，不做"防护"
 * 2. 视图状态（viewMode/sortBy/filter/search）合并进 store，computed 自动追踪
 * 3. 派生（filteredTasks/sortedTasks/groupedItems/flatItems/displayedItems）在 store
 * 4. 不持久化 filter/sort/view（避免与任务数据耦合）
 *
 * 与视图组件的边界：
 * - 视图只 import useTaskStore（通过 useTasksList 薄壳 re-export）
 * - 不需要 useTaskFilter / useTaskFiltering 等独立 composable
 */
import { computed, ref, shallowRef, triggerRef } from 'vue'
import { defineStore } from 'pinia'
import type { EncvTask, TaskStatus, TaskType } from '@/api/encv'
import {
  loadAllTasks,
  bulkPutTasks as persistBulkPut,
  putTask as persistPut,
  deleteTask as persistDelete,
  clearPutThrottle,
  ensureLRUCache,
} from '@/lib/taskPersistence'

export type ViewMode = 'group' | 'flat'
export type DatePreset = 'today' | '7d' | '30d' | 'all' | 'custom'
export type TriggeredBy = 'user' | 'automation' | 'ai_agent'
export type SortBy = 'activity' | 'created'

const TERMINAL_STATUSES: ReadonlySet<TaskStatus> = new Set(['completed', 'failed', 'cancelled'])
const STATUS_OPTIONS: TaskStatus[] = ['queued', 'running', 'cancelling', 'completed', 'failed', 'cancelled']

export const useTaskStore = defineStore('task', () => {
  // ============ 原始数据 ============
  const tasks = shallowRef<EncvTask[]>([])
  const hydrated = ref(false)
  const isRefreshing = ref(false)
  const hasAnyTask = ref(false)
  const pinnedRunIds = ref<Set<string>>(new Set())

  // O(1) id→index 索引（patch / getTaskById 性能优化）
  const _taskIndex = new Map<string, number>()

  function rebuildIndex(): void {
    _taskIndex.clear()
    const arr = tasks.value
    for (let i = 0; i < arr.length; i++) {
      _taskIndex.set(arr[i].id, i)
    }
  }

  function getTaskById(id: string): EncvTask | undefined {
    const idx = _taskIndex.get(id)
    if (idx === undefined) return undefined
    return tasks.value[idx]
  }

  // ============ 视图状态（统一 owner） ============
  const viewMode = ref<ViewMode>('group')
  const sortBy = ref<SortBy>('activity')
  const filterPlugins = ref<string[]>([])
  const filterTypes = ref<TaskType[]>([])
  const filterStatuses = ref<TaskStatus[]>([])
  const filterTriggeredBy = ref<TriggeredBy[]>([])
  const searchQuery = ref('')
  const filterDatePreset = ref<DatePreset>('all')
  const filterDateRange = ref<{ from?: string; to?: string }>({})

  // ============ Hydrate ============
  async function hydrate(): Promise<void> {
    if (hydrated.value) return
    try {
      const list = await loadAllTasks()
      tasks.value = list
      hasAnyTask.value = list.length > 0
      rebuildIndex()
      hydrated.value = true
      void ensureLRUCache()
    } catch (err) {
      hydrated.value = true
    }
  }

  // ============ 持久化 ============
  function persistRemove(id: string): void {
    try { clearPutThrottle(id); void persistDelete(id) } catch {}
  }
  function persistPutTask(id: string): void {
    if (!hydrated.value) return
    const t = getTaskById(id)
    if (t) try { void persistPut(t) } catch {}
  }

  // ============ 原始数据操作 ============
  function bulkSetTasks(newTasks: EncvTask[]): void {
    // 🆕 2026-06-22 A 方向修复根治"任务逃逸"：
    //   merge 模式 — 对每个 newTask，如果 store 已有同 id 的 prev，
    //   保留 prev 的 IDENTITY_FIELDS（runId/triggeredBy/pluginName/type/createdAt）当 newTask 缺这些字段时。
    //   解决：fetchTasks 返回的 task.RunId="" → Go omitempty 省略 → 前端 task.runId=undefined
    //   之前直接覆盖 → store 里 runId 丢失 → groupedTasksByRunId 兜底成 __manual__${id} 伪 group
    //   现在保留 prev.runId → 不会变孤儿
    const _IDENTITY_FIELDS: (keyof EncvTask)[] = [
      'runId', 'triggeredBy', 'pluginName', 'type', 'createdAt',
    ]
    const merged: EncvTask[] = newTasks.map((newTk) => {
      const idx = _taskIndex.get(newTk.id)
      if (idx === undefined) return newTk
      const prev = tasks.value[idx]
      const result: EncvTask = { ...newTk }
      for (const k of _IDENTITY_FIELDS) {
        const newVal = (newTk as any)[k]
        // newTask 缺这个字段（null/undefined/空字符串）→ 保留 prev 的
        if (newVal === undefined || newVal === null || newVal === '') {
          ;(result as any)[k] = (prev as any)[k]
        }
      }
      return result
    })
    tasks.value = merged
    hasAnyTask.value = merged.length > 0
    rebuildIndex()
    if (hydrated.value) {
      try { void persistBulkPut(merged) } catch {}
    }
  }

  /**
   * 局部 patch：只 merge 后端实际提供的字段（跳过 undefined 和 null）
   *
   * 为什么必须跳过 null：
   * - Go 后端结构体未设置字段 JSON 序列化为 null（不是 undefined）
   * - 后端 WS 事件（task:update / task:progress）通常只发"变化的字段"
   *   payload = {id, type, status, progress}，其他字段是 nil → null
   * - 如果 spread 时把 null 字段也覆盖 prev → task 失去 runId → groupedTasksByRunId
   *   把整组自动化 task 拆成 9 + 1，1 个逃成独立 group（逃逸根因）
   *
   * 业务字段 vs 标识字段：
   * - 标识字段（runId / triggeredBy / pluginName / type）：null 一律跳过（"未提供"≠"清空"）
   * - 业务字段（status / progress / error / outputPath / completedAt）：正常 merge
   *
   * 这是**正确的默认行为**，不是防护：
   *   后端少发字段 ≠ 字段被清空；只有显式空字符串才视为"清空"
   */
  const IDENTITY_FIELDS = new Set<keyof EncvTask>(['runId', 'triggeredBy', 'pluginName', 'type'])
  function patchTaskById(id: string, partial: Partial<EncvTask>): boolean {
    const idx = _taskIndex.get(id)
    if (idx === undefined) return false
    const prev = tasks.value[idx]
    const merged: EncvTask = { ...prev }
    for (const k of Object.keys(partial) as (keyof EncvTask)[]) {
      const v = partial[k]
      if (v === undefined) continue
      // 🆕 2026-06-22 真因修复（B 方向）：IDENTITY_FIELDS 跳过 null + 空字符串
      //   历史 bug：WS update 事件 payload 里 task.RunId='' 字符串
      //   （Go 端 omitempty 没省略 → JSON 序列化为 "" 而非 null）
      //   patchTaskById 之前只跳过 null → 空字符串覆盖 prev.runId
      //   → 1000+ task 散成多个 group（"任务逃逸"动态变化）
      //   修法：IDENTITY_FIELDS 字段值是 null 或空字符串 → 跳过（保留 prev 的）
      if (IDENTITY_FIELDS.has(k) && (v === null || v === '')) continue
      ;(merged as any)[k] = v
    }
    tasks.value[idx] = merged
    triggerRef(tasks)
    return true
  }

  function appendTask(task: EncvTask): void {
    if (_taskIndex.has(task.id)) {
      patchTaskById(task.id, task)
      return
    }
    // 🆕 2026-06-22 真因修复 4：appendTask 推入新 task 时，runId 是空 → warn 抓数据
    //   场景：HttpPollBackend list 第一次拉到 task，t.runId 已经是空（后端 MobileTask.RunId="" + omitempty），
    //   此时 lastFullTask cache 还没建过 → 无法回填 → push 进 store 就成孤儿
    //   warn 让 user 在真机复现时能抓到具体是哪个 task 触发
    if (!task.runId) {
      // eslint-disable-next-line no-console
      console.warn(
        '[taskStore.appendTask] 新 task runId 为空 → 推入 store 后会成孤儿 group（__manual__${id}）:',
        { id: task.id, type: task.type, sourcePath: task.sourcePath, pluginName: task.pluginName, triggeredBy: task.triggeredBy, createdAt: task.createdAt },
      )
    }
    tasks.value = [task, ...tasks.value]
    hasAnyTask.value = true
    rebuildIndex()
    if (hydrated.value) {
      try { void persistPut(task) } catch {}
    }
  }

  function removeTask(id: string): void {
    if (!_taskIndex.has(id)) return
    tasks.value = tasks.value.filter((t) => t.id !== id)
    hasAnyTask.value = tasks.value.length > 0
    rebuildIndex()
    persistRemove(id)
  }

  function removeRunTasks(runId: string): EncvTask[] {
    const targets: EncvTask[] = []
    for (const t of tasks.value) {
      if (t.runId === runId) targets.push(t)
    }
    if (targets.length === 0) return targets
    const targetIds = new Set(targets.map((t) => t.id))
    tasks.value = tasks.value.filter((t) => !targetIds.has(t.id))
    hasAnyTask.value = tasks.value.length > 0
    rebuildIndex()
    for (const t of targets) persistRemove(t.id)
    if (pinnedRunIds.value.has(runId)) {
      const next = new Set(pinnedRunIds.value)
      next.delete(runId)
      pinnedRunIds.value = next
    }
    return targets
  }

  function cancelRunTasks(runId: string): EncvTask[] {
    const targets: EncvTask[] = []
    for (const t of tasks.value) {
      if (t.runId === runId && (t.status === 'running' || t.status === 'queued')) {
        targets.push(t)
      }
    }
    for (const t of targets) {
      patchTaskById(t.id, { status: 'cancelling' })
      persistPutTask(t.id)
    }
    return targets
  }

  function togglePinRun(runId: string): boolean {
    const next = new Set(pinnedRunIds.value)
    if (next.has(runId)) { next.delete(runId); pinnedRunIds.value = next; return false }
    next.add(runId); pinnedRunIds.value = next; return true
  }
  function isRunPinned(runId: string): boolean {
    return pinnedRunIds.value.has(runId)
  }

  // ============ WS 4 件套 → 单一 applyEvent ============
  /**
   * 单一事件入口：后端推什么用什么，不加防护
   * - created:  新建或 patch
   * - update / progress: patch 后端提供的字段
   * - completed: patch 终态 + completedAt + progress=100
   */
  function applyEvent(type: 'created' | 'update' | 'progress' | 'completed', data: any): void {
    if (!data || !data.id) return
    const id = data.id
    if (type === 'created') {
      appendTask(data as EncvTask)
      persistPutTask(id)
    } else if (type === 'update' || type === 'progress') {
      patchTaskById(id, data)
      if (type === 'update' && data.status) persistPutTask(id)
    } else if (type === 'completed') {
      // 失败 → status='failed'，progress 保留 prev（patchTaskById 只 merge 提供的字段）
      // 成功 → status='completed', progress=100, completedAt=now
      patchTaskById(id, {
        ...data,
        status: data.error ? 'failed' : 'completed',
        completedAt: new Date().toISOString(),
        ...(data.error ? {} : { progress: 100 }),
      })
      persistPutTask(id)
    }
  }

  // 兼容旧 API：4 个独立函数（如果外部还在用）
  function applyTaskUpdate(data: any) { applyEvent('update', data) }
  function applyTaskProgress(data: any) { applyEvent('progress', data) }
  function applyTaskCreated(data: any) { applyEvent('created', data) }
  function applyTaskCompleted(data: any) { applyEvent('completed', data) }

  /** 后端权威：直接覆盖 */
  function rebuildFromBackend(serverTasks: EncvTask[]): void {
    bulkSetTasks(serverTasks)
  }

  // ============ 派生 computed（统一 owner） ============
  const tasksById = computed(() => {
    const map = new Map<string, EncvTask>()
    for (const t of tasks.value) map.set(t.id, t)
    return map
  })

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

  const hasCompletedTasks = computed(() =>
    tasks.value.some((tk) => TERMINAL_STATUSES.has(tk.status)),
  )

  const filteredTasks = computed<EncvTask[]>(() => {
    const q = searchQuery.value.trim().toLowerCase()
    const fromTs = filterDateRange.value.from
    const toTs = filterDateRange.value.to
    const hasDate = !!fromTs || !!toTs
    const hasSearch = q.length > 0
    const plugins = filterPlugins.value
    const types = filterTypes.value
    const statuses = filterStatuses.value
    const triggeredBy = filterTriggeredBy.value
    const out: EncvTask[] = []
    for (const t of tasks.value) {
      if (plugins.length > 0 && !plugins.includes(t.pluginName || '__unknown__')) continue
      if (types.length > 0 && !types.includes(t.type)) continue
      if (statuses.length > 0 && !statuses.includes(t.status)) continue
      if (triggeredBy.length > 0) {
        const by = t.triggeredBy ?? 'user'
        if (!triggeredBy.includes(by as any)) continue
      }
      if (hasDate) {
        if (fromTs && t.createdAt < fromTs) continue
        if (toTs && t.createdAt >= toTs) continue
      }
      if (hasSearch) {
        const name = (t.targetPath?.split('/').pop() ?? t.sourcePath?.split('/').pop() ?? t.id.slice(0, 8)).toLowerCase()
        const plugin = (t.pluginName || '').toLowerCase()
        const error = (t.error || '').toLowerCase()
        const id = t.id.toLowerCase()
        if (!name.includes(q) && !plugin.includes(q) && !error.includes(q) && !id.includes(q)) continue
      }
      out.push(t)
    }
    return out
  })

  const sortedTasks = computed<EncvTask[]>(() => {
    const arr = [...filteredTasks.value]
    if (sortBy.value === 'activity') {
      // activity 模式：按"最近活动时刻"——max(createdAt, completedAt) 即可
      // 不分支判断（终态/非终态用同一个 key），task 状态变化时位置自然更新
      arr.sort((a, b) => {
        const aKey = Math.max(
          new Date(a.createdAt).getTime(),
          a.completedAt ? new Date(a.completedAt).getTime() : 0,
        )
        const bKey = Math.max(
          new Date(b.createdAt).getTime(),
          b.completedAt ? new Date(b.completedAt).getTime() : 0,
        )
        return bKey - aKey
      })
    } else {
      arr.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
    }
    return arr
  })

  // ============ Raw 派生（仅数据，不含视图 kind/counters/displayData） ============
  // ⚠️ 视图组件需要 kind/counters/displayData 等字段，由 useTasksList 包装
  const groupedTasksByRunId = computed<any[]>(() => {
    const groups = new Map<string, { runId: string; tasks: EncvTask[]; startedAt: string }>()
    for (const tk of sortedTasks.value) {
      const key = tk.runId || `__manual__${tk.id}`
      const g = groups.get(key)
      if (g) g.tasks.push(tk)
      else groups.set(key, { runId: tk.runId || key, tasks: [tk], startedAt: tk.createdAt })
    }
    const result: any[] = []
    for (const [key, g] of groups) {
      result.push({ key, runId: g.runId, startedAt: g.startedAt, tasks: g.tasks })
    }
    result.sort((a, b) => {
      const aPinned = pinnedRunIds.value.has(a.runId)
      const bPinned = pinnedRunIds.value.has(b.runId)
      if (aPinned !== bPinned) return aPinned ? -1 : 1
      return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
    })
    return result
  })

  const flatTaskList = computed<any[]>(() =>
    sortedTasks.value.map((tk) => ({ key: `t-${tk.id}`, task: tk })),
  )

  // ============ Filter 操作 ============
  const activeFilterCount = computed(() => {
    let n = 0
    if (filterPlugins.value.length > 0) n++
    if (filterTypes.value.length > 0) n++
    if (filterStatuses.value.length > 0) n++
    if (filterTriggeredBy.value.length > 0) n++
    if (filterDatePreset.value !== 'all') n++
    if (searchQuery.value.trim().length > 0) n++
    return n
  })
  const hasActiveFilters = computed(() => activeFilterCount.value > 0)

  function clearFilters(): void {
    filterPlugins.value = []
    filterTypes.value = []
    filterStatuses.value = []
    filterTriggeredBy.value = []
    filterDatePreset.value = 'all'
    filterDateRange.value = {}
    searchQuery.value = ''
  }

  function togglePluginFilter(p: string): void {
    const i = filterPlugins.value.indexOf(p)
    if (i === -1) filterPlugins.value.push(p)
    else filterPlugins.value.splice(i, 1)
  }
  function toggleTypeFilter(t: TaskType): void {
    const i = filterTypes.value.indexOf(t)
    if (i === -1) filterTypes.value.push(t)
    else filterTypes.value.splice(i, 1)
  }
  function toggleStatusFilter(s: TaskStatus): void {
    const i = filterStatuses.value.indexOf(s)
    if (i === -1) filterStatuses.value.push(s)
    else filterStatuses.value.splice(i, 1)
  }
  function toggleTriggeredByFilter(b: TriggeredBy): void {
    const i = filterTriggeredBy.value.indexOf(b)
    if (i === -1) filterTriggeredBy.value.push(b)
    else filterTriggeredBy.value.splice(i, 1)
  }
  function setSearchQuery(q: string): void { searchQuery.value = q }
  function setViewMode(m: ViewMode): void { viewMode.value = m }
  function setSortBy(s: SortBy): void { sortBy.value = s }
  function applyDatePreset(preset: DatePreset): void {
    filterDatePreset.value = preset
    if (preset === 'all') { filterDateRange.value = {}; return }
    const now = new Date()
    if (preset === 'today') {
      const s = new Date(now.getFullYear(), now.getMonth(), now.getDate())
      const e = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
      filterDateRange.value = { from: s.toISOString(), to: e.toISOString() }
    } else if (preset === '7d' || preset === '30d') {
      const days = preset === '7d' ? 7 : 30
      const e = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
      const s = new Date(e.getTime() - days * 86400000)
      filterDateRange.value = { from: s.toISOString(), to: e.toISOString() }
    }
  }
  function setCustomDateRange(from: string | undefined, to: string | undefined): void {
    filterDatePreset.value = 'custom'
    filterDateRange.value = { from, to }
  }
  function toggleViewMode(): void { viewMode.value = viewMode.value === 'group' ? 'flat' : 'group' }
  function toggleSort(): void { sortBy.value = sortBy.value === 'activity' ? 'created' : 'activity' }

  // ============ Reset ============
  function $reset(): void {
    tasks.value = []
    _taskIndex.clear()
    hydrated.value = false
    hasAnyTask.value = false
    isRefreshing.value = false
    pinnedRunIds.value = new Set()
    viewMode.value = 'group'
    sortBy.value = 'activity'
    clearFilters()
  }

  return {
    // 原始 state
    tasks, hydrated, isRefreshing, hasAnyTask, pinnedRunIds,
    // 视图 state
    viewMode, sortBy, filterPlugins, filterTypes, filterStatuses, filterTriggeredBy,
    searchQuery, filterDatePreset, filterDateRange,
    // 派生（raw，不含视图 kind/counters/displayData）
    tasksById, tasksByRunId, availablePlugins, hasCompletedTasks,
    filteredTasks, sortedTasks, groupedTasksByRunId, flatTaskList,
    activeFilterCount, hasActiveFilters,
    // 原始操作
    hydrate, bulkSetTasks, patchTaskById, appendTask, removeTask, removeRunTasks, cancelRunTasks,
    getTaskById,
    togglePinRun, isRunPinned,
    // WS 入口
    applyEvent, applyTaskUpdate, applyTaskProgress, applyTaskCreated, applyTaskCompleted,
    rebuildFromBackend,
    // filter 操作
    clearFilters, togglePluginFilter, toggleTypeFilter, toggleStatusFilter, toggleTriggeredByFilter,
    setSearchQuery, setViewMode, setSortBy, applyDatePreset, setCustomDateRange, toggleViewMode, toggleSort,
    // 常量
    statusOptions: STATUS_OPTIONS,
    $reset,
  }
})
