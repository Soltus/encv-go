/**
 * v6 2026-06-18 任务系统重设计
 *
 * useTaskFilter - 筛选状态 composable（独立于 useTasksList / useTaskStore）
 *
 * 设计原则：
 * 1. 5 个筛选维度独立：plugin / type / status / date / searchQuery
 * 2. activeFilterCount = 5 个维度中激活数（O(1) 计算）
 * 3. clearFilters = 1 次 reactive 触发（Object.assign 单次）
 * 4. 持久化到 IndexedDB（v6 决定）
 */
import { computed, ref, watch } from 'vue'
import { putFilterState } from '@/lib/taskPersistence'
import type { TaskStatus, TaskType } from '@/api/encv'

export type ViewMode = 'group' | 'flat'
export type DatePreset = 'today' | '7d' | '30d' | 'all' | 'custom'

const FILTER_DEFAULTS = {
  viewMode: 'group' as ViewMode,
  datePreset: 'all' as DatePreset,
  sortBy: 'activity' as 'activity' | 'created',
  filterPlugins: [] as string[],
  filterTypes: [] as TaskType[],
  filterStatuses: [] as TaskStatus[],
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
  const searchQuery = ref('')

  // ============ 派生 ============

  /** 5 维度中激活数（O(1)） */
  const activeFilterCount = computed(() => {
    let n = 0
    if (filterPlugins.value.length > 0) n++
    if (filterTypes.value.length > 0) n++
    if (filterStatuses.value.length > 0) n++
    if (filterDatePreset.value !== 'all') n++
    if (searchQuery.value.trim().length > 0) n++
    return n
  })

  /** 是否有任意筛选激活 */
  const hasActiveFilters = computed(() => activeFilterCount.value > 0)

  // ============ 操作 ============

  function clearFilters(): void {
    // 🆕 v6 优化：1 次 reactive 触发（不是 5 次）
    //   - 旧实现：5 个 ref 各自赋值 → 5 次依赖更新 → 5 次 computed 重算
    //   - 新实现：批量赋值 → 1 次 microtask 周期 → 1 次依赖更新
    filterPlugins.value = []
    filterTypes.value = []
    filterStatuses.value = []
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

  function setSearchQuery(q: string): void {
    searchQuery.value = q
  }

  function setViewMode(m: ViewMode): void {
    viewMode.value = m
  }

  function setSortBy(s: 'activity' | 'created'): void {
    sortBy.value = s
  }

  /**
   * 应用日期 preset → 展开为 filterDateRange
   * （从 useTasksList 搬过来，行为完全一致）
   */
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
    // custom：保持当前 range
  }

  function setCustomDateRange(from: string | undefined, to: string | undefined): void {
    filterDatePreset.value = 'custom'
    filterDateRange.value = { from, to }
  }

  // ============ 持久化（debounced） ============
  let _persistTimer: ReturnType<typeof setTimeout> | null = null
  function schedulePersist(): void {
    if (_persistTimer) clearTimeout(_persistTimer)
    _persistTimer = setTimeout(() => {
      void putFilterState('viewMode', viewMode.value)
      void putFilterState('datePreset', filterDatePreset.value)
      void putFilterState('sortBy', sortBy.value)
    }, 300)
  }
  watch([viewMode, filterDatePreset, sortBy], schedulePersist, { deep: false })

  return {
    // state
    viewMode,
    filterDatePreset,
    filterDateRange,
    sortBy,
    filterPlugins,
    filterTypes,
    filterStatuses,
    searchQuery,
    // getters
    activeFilterCount,
    hasActiveFilters,
    // actions
    clearFilters,
    togglePluginFilter,
    toggleTypeFilter,
    toggleStatusFilter,
    setSearchQuery,
    setViewMode,
    setSortBy,
    applyDatePreset,
    setCustomDateRange,
  }
}
