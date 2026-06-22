/**
 * useTaskFilter - 任务筛选 state composable
 *
 * 🆕 2026-06-22 v2 架构重写：统一为 useTaskViewState 薄包装
 *
 * 历史问题：
 * - useTaskFilter 内部有独立 ref（filterPlugins/filterTypes/filterStatuses/searchQuery）
 * - useTaskViewState（GroupDetail 用）也有一份独立 ref
 * - 同一个筛选维度有 2 份 state → Tasks.vue 与 GroupDetail 切换时筛选条件丢失
 *
 * 修法：useTaskFilter 不再持有 local state，全部 storeToRefs(useTaskViewState())
 *   - 全部状态变成全局单例
 *   - 任何视图改 → 所有视图实时同步
 *   - API 保持不变（ref 名相同），Tasks.vue / useTasksList 不用改
 */
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTaskViewState } from '@/stores/taskViewState'

export type ViewMode = 'group' | 'flat'
export type DatePreset = 'today' | '7d' | '30d' | 'all' | 'custom'

export function useTaskFilter() {
  const vs = useTaskViewState()
  const refs = storeToRefs(vs) as any

  // 派生：activeFilterCount（5 维度）
  const activeFilterCount = computed(() => {
    let n = 0
    if (refs.pluginFilter.value.size > 0) n++
    if (refs.typeFilter.value.size > 0) n++
    if (refs.statusFilter.value.size > 0) n++
    if (refs.searchQuery.value.trim().length > 0) n++
    return n
  })
  const hasActiveFilters = computed(() => activeFilterCount.value > 0)

  // 兼容 API：ref 名与 useTaskFilter 旧版一致
  // 旧 API 期望 string[]，useTaskViewState 用 Set → 暴露 array getter 形式
  const filterPlugins = computed<string[]>(() => Array.from(refs.pluginFilter.value))
  const filterTypes = computed<string[]>(() => Array.from(refs.typeFilter.value))
  const filterStatuses = computed<string[]>(() => Array.from(refs.statusFilter.value))
  const searchQuery = computed<string>({
    get: () => refs.searchQuery.value,
    set: (v: string) => (refs.searchQuery.value = v),
  })

  function clearFilters(): void {
    vs.resetFilters()
  }
  function togglePluginFilter(p: string): void {
    vs.togglePlugin(p)
  }
  function toggleTypeFilter(t: string): void {
    vs.toggleType(t)
  }
  function toggleStatusFilter(s: string): void {
    vs.toggleStatus(s)
  }
  function setSearchQuery(q: string): void {
    refs.searchQuery.value = q
  }

  // 保留但弃用的字段（v2 架构 viewMode/datePreset/sortBy 移到 useTaskViewState）
  // 这些字段已无实际筛选作用，但 useTasksList 还在用
  // 用 readonly computed 避免 setter 兼容性问题
  const viewMode = computed<ViewMode>(() => 'group')
  const filterDatePreset = computed<DatePreset>(() => 'all')
  const filterDateRange = computed<{ from?: string; to?: string }>(() => ({}))
  const sortBy = computed<'activity' | 'created'>(() => 'activity')
  const filterTriggeredBy = computed<string[]>(() => [])

  function toggleTriggeredByFilter(_value: 'user' | 'automation' | 'ai_agent'): void {
    // v2 架构：triggeredBy 筛选已并入 typeFilter（按 type 分组）
  }
  function setViewMode(_m: ViewMode): void { /* noop */ }
  function setSortBy(_s: 'activity' | 'created'): void { /* noop */ }
  function applyDatePreset(_preset: DatePreset): void { /* noop */ }
  function setCustomDateRange(_from: string | undefined, _to: string | undefined): void { /* noop */ }

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
    filterTriggeredBy,
    // getters
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
  }
}
