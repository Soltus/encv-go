/**
 * useTaskFilter - 筛选状态 composable（独立 ref state，不依赖任何 store）
 *
 * 设计原则：
 * 1. 5 个筛选维度独立：plugin / type / status / date / searchQuery
 * 2. activeFilterCount = 5 个维度中激活数（O(1) 计算）
 * 3. clearFilters 批量赋值 → 1 次 reactive 触发
 * 4. 不持久化到 IndexedDB（避免与任务数据耦合，刷新即重置）
 */
import { computed, ref } from 'vue'
import type { TaskStatus, TaskType } from '@/api/encv'

export type ViewMode = 'group' | 'flat'
export type DatePreset = 'today' | '7d' | '30d' | 'all' | 'custom'
export type TriggeredBy = 'user' | 'automation' | 'ai_agent'

const FILTER_DEFAULTS = {
  viewMode: 'group' as ViewMode,
  datePreset: 'all' as DatePreset,
  sortBy: 'activity' as 'activity' | 'created',
  filterPlugins: [] as string[],
  filterTypes: [] as TaskType[],
  filterStatuses: [] as TaskStatus[],
  filterTriggeredBy: [] as TriggeredBy[],
  searchQuery: '',
}

export function useTaskFilter() {
  const viewMode = ref<ViewMode>(FILTER_DEFAULTS.viewMode)
  const filterDatePreset = ref<DatePreset>(FILTER_DEFAULTS.datePreset)
  const filterDateRange = ref<{ from?: string; to?: string }>({})
  const sortBy = ref<'activity' | 'created'>(FILTER_DEFAULTS.sortBy)
  const filterPlugins = ref<string[]>([])
  const filterTypes = ref<TaskType[]>([])
  const filterStatuses = ref<TaskStatus[]>([])
  const filterTriggeredBy = ref<TriggeredBy[]>([])
  const searchQuery = ref('')

  // ============ 派生 ============

  /** 6 维度中激活数（O(1)） */
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

  /** 是否有任意筛选激活 */
  const hasActiveFilters = computed(() => activeFilterCount.value > 0)

  // ============ 操作 ============

  function clearFilters(): void {
    filterPlugins.value = []
    filterTypes.value = []
    filterStatuses.value = []
    filterTriggeredBy.value = []
    filterDatePreset.value = 'all'
    filterDateRange.value = {}
    searchQuery.value = ''
  }

  function togglePluginFilter(plugin: string): void {
    const idx = filterPlugins.value.indexOf(plugin)
    if (idx === -1) filterPlugins.value.push(plugin)
    else filterPlugins.value.splice(idx, 1)
  }

  function toggleTypeFilter(type: TaskType): void {
    const idx = filterTypes.value.indexOf(type)
    if (idx === -1) filterTypes.value.push(type)
    else filterTypes.value.splice(idx, 1)
  }

  function toggleStatusFilter(status: TaskStatus): void {
    const idx = filterStatuses.value.indexOf(status)
    if (idx === -1) filterStatuses.value.push(status)
    else filterStatuses.value.splice(idx, 1)
  }

  function toggleTriggeredByFilter(by: TriggeredBy): void {
    const idx = filterTriggeredBy.value.indexOf(by)
    if (idx === -1) filterTriggeredBy.value.push(by)
    else filterTriggeredBy.value.splice(idx, 1)
  }

  function setSearchQuery(q: string): void {
    searchQuery.value = q
  }

  function setViewMode(m: ViewMode): void {
    viewMode.value = m
  }

  function setSortBy(s: 'activity' | 'created'): void {
    sortBy.value = s
  }

  function applyDatePreset(preset: DatePreset): void {
    filterDatePreset.value = preset
    if (preset === 'all') {
      filterDateRange.value = {}
      return
    }
    const now = new Date()
    if (preset === 'today') {
      const localStart = new Date(now.getFullYear(), now.getMonth(), now.getDate())
      const localEnd = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
      filterDateRange.value = { from: localStart.toISOString(), to: localEnd.toISOString() }
      return
    }
    if (preset === '7d' || preset === '30d') {
      const days = preset === '7d' ? 7 : 30
      const localEnd = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1)
      const localStart = new Date(localEnd.getTime() - days * 86400000)
      filterDateRange.value = { from: localStart.toISOString(), to: localEnd.toISOString() }
      return
    }
  }

  function setCustomDateRange(from: string | undefined, to: string | undefined): void {
    filterDatePreset.value = 'custom'
    filterDateRange.value = { from, to }
  }

  function toggleViewMode(): void {
    viewMode.value = viewMode.value === 'group' ? 'flat' : 'group'
  }

  function toggleSort(): void {
    sortBy.value = sortBy.value === 'activity' ? 'created' : 'activity'
  }

  return {
    // state
    viewMode,
    filterDatePreset,
    filterDateRange,
    sortBy,
    filterPlugins,
    filterTypes,
    filterStatuses,
    filterTriggeredBy,
    searchQuery,
    // derived
    activeFilterCount,
    hasActiveFilters,
    // actions
    clearFilters,
    togglePluginFilter,
    toggleTypeFilter,
    toggleStatusFilter,
    toggleTriggeredByFilter,
    setSearchQuery,
    setViewMode,
    setSortBy,
    applyDatePreset,
    setCustomDateRange,
    toggleViewMode,
    toggleSort,
  }
}
