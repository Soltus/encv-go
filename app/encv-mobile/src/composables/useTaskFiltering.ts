/**
 * 🆕 2026-06-22 Q4：任务筛选 + 搜索 composable
 *
 * 职责：
 * - 合并 search + status + taskType + plugin 4 种筛选条件
 * - 返回 filteredTasks（响应式）
 * - 返回 filterChipCount + activeFilterChips（用于 chip 行展示）
 *
 * 性能：
 * - O(n) 一次扫描，无重复过滤
 * - 用 computed 缓存
 */
import { computed, type ComputedRef, type Ref } from 'vue'
import type { EncvTask } from '@/api/encv'

export interface UseTaskFilteringOptions {
  tasks: ComputedRef<EncvTask[]> | Ref<EncvTask[]>
  searchQuery: Ref<string>
  statusFilter: Ref<Set<string>>
  taskTypeFilter: Ref<Set<string>>
  pluginFilter: Ref<Set<string>>
}

export function useTaskFiltering(options: UseTaskFilteringOptions) {
  const { tasks, searchQuery, statusFilter, taskTypeFilter, pluginFilter } = options

  // 文本搜索（小写 + 子串匹配）
  const searchLower = computed(() => (searchQuery.value ?? '').toLowerCase().trim())

  const filteredTasks = computed<EncvTask[]>(() => {
    const list = tasks.value
    const q = searchLower.value
    const st = statusFilter.value
    const tt = taskTypeFilter.value
    const pl = pluginFilter.value

    // 快速路径：所有筛选都为空时直接返回原列表
    if (!q && st.size === 0 && tt.size === 0 && pl.size === 0) {
      return list
    }

    return list.filter((t) => {
      // status
      if (st.size > 0 && !st.has(t.status)) return false
      // taskType
      if (tt.size > 0 && !tt.has(t.type)) return false
      // plugin
      if (pl.size > 0 && !pl.has(t.pluginName ?? '')) return false
      // text search
      if (q) {
        const haystack = [
          t.id,
          t.sourcePath ?? '',
          t.targetPath ?? '',
          t.pluginName ?? '',
          t.type ?? '',
        ].join(' ').toLowerCase()
        if (!haystack.includes(q)) return false
      }
      return true
    })
  })

  // 已应用的筛选 chip 列表
  const activeFilterChips = computed<Array<{ key: string; label: string }>>(() => {
    const chips: Array<{ key: string; label: string }> = []
    for (const s of statusFilter.value) {
      chips.push({ key: `status:${s}`, label: `状态:${s}` })
    }
    for (const t of taskTypeFilter.value) {
      chips.push({ key: `type:${t}`, label: `类型:${t}` })
    }
    for (const p of pluginFilter.value) {
      chips.push({ key: `plugin:${p}`, label: `插件:${p}` })
    }
    return chips
  })

  const filterChipCount = computed(() => activeFilterChips.value.length)

  return {
    filteredTasks,
    activeFilterChips,
    filterChipCount,
  }
}
