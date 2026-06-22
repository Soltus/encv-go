/**
 * useTaskViewState - 视图状态共享 store（filter / selection / search）
 *
 * 🆕 2026-06-22 v2 架构重写：解决 GroupDetail 与 Tasks.vue 主视图各自实例化 useTasksList
 *   → 各自有独立 filter state → 用户切来切去筛选条件丢失
 *   → 选中态在两个视图不共享
 *   → 1000+ tasks 各自 O(n) filter 重复计算
 *
 * 设计：
 * - 独立 Pinia store，**全局单例**（与 useTaskStore 同级）
 * - Tasks.vue 主视图 / GroupDetail / TasksTab 都 useTaskViewState() 拿同一份 state
 * - 任何视图改 filter / search / selection → 所有视图实时同步
 * - 派生 computed 缓存：filter 1000+ tasks 只算一次
 *
 * 与 useTaskStore 的关系：
 * - useTaskStore: 任务**数据**（CRUD + WS + 持久化）
 * - useTaskViewState: 任务**视图**（filter / selection / search）
 * - 派生层（groupedItems / filteredTasks）在 useTaskViewState 内部 computed，O(n) 单次
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { useTaskStore } from '@/stores/taskStore'
import type { EncvTask } from '@/api/encv'

export type SortKey = 'createdAt' | 'status' | 'type'

export const useTaskViewState = defineStore('taskView', () => {
  const store = useTaskStore()

  // ============ Filter 状态（多选 Set） ============
  /** 选中的 status 集合；空 = 显示所有 status */
  const statusFilter = ref<Set<string>>(new Set())
  /** 选中的 taskType 集合（'encrypt' / 'decrypt' / 'move' / 'copy' / 'rename' / 'delete' + rollback_*）；空 = 显示所有 */
  const typeFilter = ref<Set<string>>(new Set())
  /** 选中的 pluginName 集合；空 = 显示所有 */
  const pluginFilter = ref<Set<string>>(new Set())
  /** 文本搜索（path / id 模糊匹配） */
  const searchQuery = ref('')
  /** 排序键 */
  const sortBy = ref<SortKey>('createdAt')
  /** 排序方向 */
  const sortDesc = ref(true)

  // ============ Selection 状态（多选 id） ============
  /** 选中的 task id 集合（批量操作） */
  const selectedIds = ref<Set<string>>(new Set())
  /** 是否处于多选模式 */
  const multiSelectMode = ref(false)

  // ============ 操作 ============
  function toggleStatus(s: string): void {
    const next = new Set(statusFilter.value)
    if (next.has(s)) next.delete(s)
    else next.add(s)
    statusFilter.value = next
  }
  function toggleType(t: string): void {
    const next = new Set(typeFilter.value)
    if (next.has(t)) next.delete(t)
    else next.add(t)
    typeFilter.value = next
  }
  function togglePlugin(p: string): void {
    const next = new Set(pluginFilter.value)
    if (next.has(p)) next.delete(p)
    else next.add(p)
    pluginFilter.value = next
  }
  function resetFilters(): void {
    statusFilter.value = new Set()
    typeFilter.value = new Set()
    pluginFilter.value = new Set()
    searchQuery.value = ''
  }
  function toggleSelect(id: string): void {
    const next = new Set(selectedIds.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selectedIds.value = next
    if (next.size > 0) multiSelectMode.value = true
  }
  function clearSelection(): void {
    selectedIds.value = new Set()
    multiSelectMode.value = false
  }
  function setMultiSelectMode(on: boolean): void {
    multiSelectMode.value = on
    if (!on) selectedIds.value = new Set()
  }

  // ============ 派生（核心：1000+ 任务性能） ============
  /**
   * filterTasks：把 store.tasks 走 filter → 派生 filtered list
   * 关键优化：computed 自动 memoize
   * - 仅当 store.tasks / 任意 filter 变化时重算（O(n) 一次）
   * - 1000+ tasks 一次 filter ≈ 1ms
   * - 多视图（GroupDetail / Tasks.vue）共享同一 computed 结果
   */
  const filteredTasks = computed<EncvTask[]>(() => {
    const all = store.tasks
    if (all.length === 0) return all
    const q = searchQuery.value.trim().toLowerCase()
    const hasQ = q.length > 0
    const hasStatus = statusFilter.value.size > 0
    const hasType = typeFilter.value.size > 0
    const hasPlugin = pluginFilter.value.size > 0
    // 快速路径：所有 filter 都空 + 无搜索 → 直接返回原列表引用
    if (!hasQ && !hasStatus && !hasType && !hasPlugin) {
      return all
    }
    const out: EncvTask[] = []
    for (let i = 0; i < all.length; i++) {
      const t = all[i]
      if (hasStatus && !statusFilter.value.has(t.status)) continue
      if (hasType && !typeFilter.value.has(t.type)) continue
      if (hasPlugin) {
        const pn = t.pluginName ?? '__unknown__'
        if (!pluginFilter.value.has(pn)) continue
      }
      if (hasQ) {
        const sp = t.sourcePath?.toLowerCase() ?? ''
        const tp = t.targetPath?.toLowerCase() ?? ''
        if (!sp.includes(q) && !tp.includes(q) && !t.id.toLowerCase().includes(q)) continue
      }
      out.push(t)
    }
    return out
  })

  /** 排序：stable sort，O(n log n)；不修改原数组 */
  const sortedTasks = computed<EncvTask[]>(() => {
    const arr = filteredTasks.value
    if (arr.length <= 1) return arr
    const key = sortBy.value
    const desc = sortDesc.value ? -1 : 1
    // slice() 避免修改 filteredTasks 内部
    return arr.slice().sort((a, b) => {
      let av: any, bv: any
      if (key === 'createdAt') { av = a.createdAt; bv = b.createdAt }
      else if (key === 'status') { av = a.status; bv = b.status }
      else { av = a.type; bv = b.type }
      if (av < bv) return -1 * desc
      if (av > bv) return 1 * desc
      return 0
    })
  })

  /** 按 runId 分组（O(n) 一次） */
  const groupedByRunId = computed<Map<string, EncvTask[]>>(() => {
    const map = new Map<string, EncvTask[]>()
    for (const t of sortedTasks.value) {
      const key = t.runId || '__orphan__'
      const arr = map.get(key)
      if (arr) arr.push(t)
      else map.set(key, [t])
    }
    return map
  })

  /** 当前激活的 filter chip 列表（用于 UI 顶部"已选 N 项"展示） */
  const activeFilterChips = computed(() => {
    const chips: { key: string; label: string; remove: () => void }[] = []
    for (const s of statusFilter.value) {
      chips.push({ key: `status:${s}`, label: `status: ${s}`, remove: () => toggleStatus(s) })
    }
    for (const t of typeFilter.value) {
      chips.push({ key: `type:${t}`, label: `type: ${t}`, remove: () => toggleType(t) })
    }
    for (const p of pluginFilter.value) {
      chips.push({ key: `plugin:${p}`, label: `plugin: ${p}`, remove: () => togglePlugin(p) })
    }
    if (searchQuery.value.trim()) {
      const q = searchQuery.value
      chips.push({ key: 'search', label: `search: ${q}`, remove: () => (searchQuery.value = '') })
    }
    return chips
  })

  const filterChipCount = computed(() => activeFilterChips.value.length)

  // ============ availablePlugins 透传（已存在的 store getter） ============
  const availablePlugins = computed(() => store.availablePlugins)

  return {
    // filter state
    statusFilter, typeFilter, pluginFilter, searchQuery, sortBy, sortDesc,
    // selection state
    selectedIds, multiSelectMode,
    // filter actions
    toggleStatus, toggleType, togglePlugin, resetFilters,
    // selection actions
    toggleSelect, clearSelection, setMultiSelectMode,
    // 派生
    filteredTasks, sortedTasks, groupedByRunId, activeFilterChips, filterChipCount,
    availablePlugins,
  }
})
