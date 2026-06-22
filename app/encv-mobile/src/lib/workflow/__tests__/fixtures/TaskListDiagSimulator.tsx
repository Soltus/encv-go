/**
 * 真机 UI 1:1 复刻模拟器（用于 e2e 测试，不含样式细节）
 *
 * 2026-06-22 user 反馈"你确定完整复刻了整个任务tab吗（不含样式细节），比如右上角第二个控件切换排序，不要嫌麻烦"
 *
 * 之前 TaskListDiag 只是 18 个 data-testid div 拼成的诊断面板，根本不是 UI。
 * user 截图里能看到 5 个逃逸 group + 1 个真 group（run-mqp1ye6j-o1ew9q），
 * 需要 1:1 复刻整个 Tasks page 控件，模拟测试时才能像在真机一样点 sort 切换、点 search、点 filter chip。
 *
 * 复刻清单（完整 Tasks.vue 控件树）：
 *  - ion-header
 *    - ion-toolbar (主) → ion-title + ion-buttons slot=end
 *      - ① 右上角第 1 控件：sort toggle 按钮（click → toggleSort）
 *      - ② 右上角第 2 控件：clear completed 按钮（click → clearCompletedWithConfirm）
 *    - ion-toolbar v-if=showSearch → ion-searchbar
 *    - ion-toolbar v-if=showFilters → 5 个 chip
 *    - 4 个 ion-popover v-if=对应 state（plugin/type/status/date）
 *  - ion-content
 *    - ion-refresher（@ionRefresh → refresh）
 *    - toolbar-actions 容器（页面内 action 区）
 *      - search toggle / date popover / filter toggle / viewMode toggle
 *    - loading 状态
 *    - empty 状态
 *    - displayedItems 列表：date section / group card / task card
 *    - ion-fab（右下角 + → openNewTask）
 *
 * 每个控件：
 *  - 真 click / change handler 调 composable 方法（不 mock、不假模拟）
 *  - data-testid 让测试可以 querySelector 找到
 *  - v-if 条件 / class 表达式 / text 内容跟真机一致
 *  - 不含样式（class 名保留但 style 不写，测试只关心结构 + 交互 + 派生值）
 */
import { defineComponent, h, computed, type PropType, type VNode } from 'vue'
import type { useTasksList } from '@/composables/useTasksList'
import type { useTaskStore } from '@/stores/taskStore'
import type { EncvTask, TaskStatus } from '@/api/encv'

/** 终态 status 集合（用于判断 group 是否还有可取消的 task） */
const TERMINAL_STATUSES: ReadonlySet<TaskStatus> = new Set(['completed', 'failed', 'cancelled'])

/** 真机 1:1 复刻的 TaskList 模拟器（不含样式细节，保留所有控件 + handler + data-testid） */
export const TaskListDiagSimulator = defineComponent({
  name: 'TaskListDiagSimulator',
  props: {
    store: { type: Object as PropType<ReturnType<typeof useTaskStore>>, required: true },
    composable: { type: Object as PropType<ReturnType<typeof useTasksList>>, required: true },
    /** 注入式 handlers：这些方法 Tasks.vue 自己定义，不在 useTasksList 返回里。
     *  测试时 mock 成 noop 函数即可（避免调 router / modalController 崩）。 */
    handlers: {
      type: Object as PropType<{
        openGroupDetail?: (runId: string) => void | Promise<void>
        openTaskDetail?: (task: EncvTask) => void | Promise<void>
        openGroupActionSheet?: (item: { runId: string; tasks: EncvTask[] }) => void | Promise<void>
        openNewTask?: () => void | Promise<void>
        handleRefresh?: () => void | Promise<void>
        handleClearCompleted?: () => void | Promise<void>
        /** 🆕 2026-06-23 Task 9.1：group card 上的取消按钮回调（批量取消整个 run） */
        cancelRun?: (runId: string) => void | Promise<void>
      }>,
      default: () => ({}),
    },
  },
  setup(props) {
    const store = props.store
    const c = props.composable  // alias

    // ========== 派生（1:1 复用 composable） ==========
    const tasks = computed(() => store.tasks as EncvTask[])
    const displayedItems = computed(() => c.displayedItems.value)
    const groupedTasksByRunId = computed(() => c.groupedTasksByRunId.value)
    const viewMode = computed(() => c.viewMode.value)
    const sortBy = computed(() => c.sortBy.value)
    const searchQuery = computed(() => c.searchQuery.value)
    const showSearch = computed(() => c.showSearch.value)
    const showFilters = computed(() => c.showFilters.value)
    const filterPlugins = computed(() => c.filterPlugins.value)
    const filterTypes = computed(() => c.filterTypes.value)
    const filterStatuses = computed(() => c.filterStatuses.value)
    const filterTriggeredBy = computed(() => c.filterTriggeredBy.value)
    const filterDatePreset = computed(() => c.filterDatePreset.value)
    const pinnedRunIds = computed(() => c.pinnedRunIds.value)
    const isInitialLoad = computed(() => c.isInitialLoad.value)
    const hasActiveFilters = computed(() => c.hasActiveFilters.value)
    const hasCompletedTasks = computed(() => c.hasCompletedTasks.value)
    const availablePlugins = computed(() => c.availablePlugins.value)
    const statusOptions = computed(() => c.statusOptions.value)
    const pluginPopoverOpen = computed(() => c.pluginPopoverOpen.value)
    const typePopoverOpen = computed(() => c.typePopoverOpen.value)
    const statusPopoverOpen = computed(() => c.statusPopoverOpen.value)
    const datePopoverOpen = computed(() => c.datePopoverOpen.value)
    const filteredTasks = computed(() => c.filteredTasks.value)
    const isGroupFilterActive = computed(() => hasActiveFilters.value || (searchQuery.value?.trim().length ?? 0) > 0)

    // 🆕 2026-06-23 重构：删除分页控件（分页由虚拟滚动天然处理）
    //   - 旧设计：isLoadingMore/hasMore/loadMore 分页加载 task 到 store
    //   - 新设计：store 全量持有所有 task，虚拟滚动只渲染可见行
    /** 当前虚拟滚动容器内的 item 数（date/group/task 三种 kind 总和）。
     *  模拟器不虚拟滚动，等于 displayedItems.length；
     *  真机 UI 用 TaskVirtualList.vue 虚拟滚动，DOM 节点数恒定 ≤ 30（可见窗口 + overscan）。 */
    const visibleTaskCount = computed(() => (displayedItems.value as any[]).length)

    // ========== 真 group / 伪 group 数（诊断用） ==========
    const realGroupCount = computed(() =>
      groupedTasksByRunId.value.filter((g) => !g.runId.startsWith('__manual__')).length,
    )
    const fakeGroupCount = computed(() =>
      groupedTasksByRunId.value.filter((g) => g.runId.startsWith('__manual__')).length,
    )
    const escapeTaskCount = computed(() =>
      groupedTasksByRunId.value
        .filter((g) => g.runId.startsWith('__manual__'))
        .reduce((acc, g) => acc + g.tasks.length, 0),
    )

    // ========== 渲染：完整 Tasks page 1:1 复刻（不含样式） ==========
    return () => {
      const vnodes: VNode[] = []

      // ============ 1. 顶部 toolbar ion-buttons slot=end（右上角 2 个控件）============
      vnodes.push(
        h('div', { 'data-testid': 'tasks-toolbar-buttons', class: 'toolbar-buttons-end' }, [
          // ① 右上角第 1 控件：sort toggle（user 明确点出"右上角第二个控件切换排序"——这个是第一个，右二是 clear）
          h('button', {
            'data-testid': 'toolbar-sort-btn',
            class: 'toolbar-btn',
            title: 'Toggle sort',
            onClick: () => c.toggleSort(),
          }, sortBy.value === 'activity' ? '⏱' : '🔄'),
          // ② 右上角第 2 控件：clear completed
          h('button', {
            'data-testid': 'toolbar-clear-completed-btn',
            class: 'toolbar-btn',
            title: 'Clear completed',
            disabled: !hasCompletedTasks.value,
            onClick: () => { void c.clearCompletedWithConfirm() },
          }, '🗑'),
        ]),
      )

      // ============ 2. showSearch toolbar（v-if showSearch）============
      if (showSearch.value) {
        vnodes.push(
          h('div', { 'data-testid': 'search-toolbar', class: 'search-toolbar' }, [
            h('input', {
              'data-testid': 'search-input',
              type: 'text',
              value: searchQuery.value ?? '',
              placeholder: 'Search tasks...',
              onInput: (e: Event) => c.onSearchInput(e as any),
            }),
            h('button', {
              'data-testid': 'search-cancel-btn',
              onClick: () => { c.showSearch.value = false; c.searchQuery.value = '' },
            }, 'Cancel'),
          ]),
        )
      }

      // ============ 3. showFilters toolbar（v-if showFilters）============
      if (showFilters.value) {
        vnodes.push(
          h('div', { 'data-testid': 'filter-toolbar', class: 'filter-toolbar' }, [
            // ① active run chip（v-if workflowService.isRunning）
            c.workflowService?.isRunning?.value
              ? h('span', { 'data-testid': 'chip-active-run', class: 'chip chip--warning' },
                  `Active run · ${c.workflowService.totalSteps.value}`)
              : null,
            // ② plugin chip（点击 → openPluginPopover）
            h('button', {
              'data-testid': 'chip-plugin',
              class: 'chip',
              title: 'Filter by plugin',
              onClick: (e: Event) => c.openPluginPopover(e as any),
            }, `Plugin (${filterPlugins.value.length})`),
            // ③ type chip
            h('button', {
              'data-testid': 'chip-type',
              class: 'chip',
              title: 'Filter by type',
              onClick: (e: Event) => c.openTypePopover(e as any),
            }, `Type (${filterTypes.value.length})`),
            // ④ status chip
            h('button', {
              'data-testid': 'chip-status',
              class: 'chip',
              title: 'Filter by status',
              onClick: (e: Event) => c.openStatusPopover(e as any),
            }, `Status (${filterStatuses.value.length})`),
            // ⑤ clear filters chip
            h('button', {
              'data-testid': 'chip-clear-filters',
              class: 'chip',
              disabled: !hasActiveFilters.value,
              title: 'Clear filters',
              onClick: () => c.clearFilters(),
            }, 'Clear filters'),
          ]),
        )
      }

      // ============ 4. 4 个 popover（v-if 对应 state）============
      if (pluginPopoverOpen.value) {
        vnodes.push(
          h('div', { 'data-testid': 'popover-plugin', class: 'popover' }, [
            h('div', { class: 'popover-title' }, 'Filter by plugin'),
            ...(availablePlugins.value ?? []).map((plugin: string) =>
              h('label', { 'data-testid': `popover-plugin-item-${plugin}`, key: plugin, class: 'popover-item' }, [
                h('input', {
                  type: 'checkbox',
                  checked: filterPlugins.value.includes(plugin),
                  onChange: () => c.togglePluginFilter(plugin),
                }),
                h('span', {}, plugin === '__unknown__' ? 'unknown' : plugin),
              ]),
            ),
            (availablePlugins.value ?? []).length === 0
              ? h('div', { class: 'popover-empty' }, 'No plugins found')
              : null,
          ]),
        )
      }
      if (typePopoverOpen.value) {
        vnodes.push(
          h('div', { 'data-testid': 'popover-type', class: 'popover' }, [
            h('div', { class: 'popover-title' }, 'Filter by type'),
            h('label', { 'data-testid': 'popover-type-encrypt', class: 'popover-item' }, [
              h('input', {
                type: 'checkbox',
                checked: filterTypes.value.includes('encrypt'),
                onChange: () => c.toggleTypeFilter('encrypt'),
              }),
              h('span', {}, 'Encrypt'),
            ]),
            h('label', { 'data-testid': 'popover-type-decrypt', class: 'popover-item' }, [
              h('input', {
                type: 'checkbox',
                checked: filterTypes.value.includes('decrypt'),
                onChange: () => c.toggleTypeFilter('decrypt'),
              }),
              h('span', {}, 'Decrypt'),
            ]),
          ]),
        )
      }
      if (statusPopoverOpen.value) {
        vnodes.push(
          h('div', { 'data-testid': 'popover-status', class: 'popover' }, [
            h('div', { class: 'popover-title' }, 'Filter by status'),
            ...(statusOptions.value ?? []).map((s: TaskStatus) =>
              h('label', { 'data-testid': `popover-status-item-${s}`, key: s, class: 'popover-item' }, [
                h('input', {
                  type: 'checkbox',
                  checked: filterStatuses.value.includes(s),
                  onChange: () => c.toggleStatusFilter(s),
                }),
                h('span', {}, s),
              ]),
            ),
          ]),
        )
      }
      if (datePopoverOpen.value) {
        const datePresets: { key: 'today' | '7d' | '30d' | 'all' | 'custom'; label: string }[] = [
          { key: 'today', label: 'Today' },
          { key: '7d', label: 'Last 7 days' },
          { key: '30d', label: 'Last 30 days' },
          { key: 'all', label: 'All' },
          { key: 'custom', label: 'Custom' },
        ]
        vnodes.push(
          h('div', { 'data-testid': 'popover-date', class: 'popover' }, [
            h('div', { class: 'popover-title' }, 'Filter by date'),
            ...datePresets.map((p) =>
              h('button', {
                'data-testid': `popover-date-preset-${p.key}`,
                key: p.key,
                class: ['popover-item', filterDatePreset.value === p.key ? 'popover-item--active' : ''],
                onClick: () => c.applyDatePreset(p.key),
              }, p.label),
            ),
            // 自定义日期：filterDatePreset === 'custom' 时显示
            filterDatePreset.value === 'custom'
              ? h('div', { 'data-testid': 'date-custom-range', class: 'date-custom-range' }, [
                  h('label', {}, [
                    h('span', {}, 'From: '),
                    h('input', {
                      'data-testid': 'date-from-input',
                      type: 'date',
                      onChange: (e: Event) => {
                        const v = (e.target as HTMLInputElement).value
                        c.setCustomDateRange(v || undefined, undefined)
                      },
                    }),
                  ]),
                  h('label', {}, [
                    h('span', {}, 'To: '),
                    h('input', {
                      'data-testid': 'date-to-input',
                      type: 'date',
                      onChange: (e: Event) => {
                        const v = (e.target as HTMLInputElement).value
                        c.setCustomDateRange(undefined, v || undefined)
                      },
                    }),
                  ]),
                ])
              : null,
          ]),
        )
      }

      // ============ 5. content 区域 ============
      vnodes.push(
        h('div', { 'data-testid': 'tasks-content', class: 'tasks-content' }, [
          // 5.1 toolbar-actions（页面内 action 区）
          h('div', { 'data-testid': 'toolbar-actions', class: 'toolbar-actions' }, [
            h('button', {
              'data-testid': 'action-search-toggle',
              class: 'action-btn',
              title: 'Search',
              onClick: () => { c.showSearch.value = !c.showSearch.value },
            }, '🔍'),
            h('button', {
              'data-testid': 'action-date-popover',
              class: 'action-btn',
              title: 'Filter by date',
              onClick: (e: Event) => c.openDatePopover(e as any),
            }, '📅'),
            h('button', {
              'data-testid': 'action-filter-toggle',
              class: 'action-btn',
              title: 'Filters',
              onClick: () => { c.showFilters.value = !c.showFilters.value },
            }, '🎛'),
            h('button', {
              'data-testid': 'action-viewmode-toggle',
              class: 'action-btn',
              title: viewMode.value === 'group' ? 'Flat view' : 'Group view',
              onClick: () => c.toggleViewMode(),
            }, viewMode.value === 'group' ? '▦' : '☰'),
          ]),

          // 5.2 loading / empty 状态
          isInitialLoad.value && tasks.value.length === 0
            ? h('div', { 'data-testid': 'loading', class: 'loading' }, 'Loading...')
            : null,
          !isInitialLoad.value && tasks.value.length > 0 && filteredTasks.value.length === 0
            ? h('div', { 'data-testid': 'empty-filtered', class: 'empty' }, [
                h('div', {}, 'No matching tasks'),
                h('button', { onClick: () => c.clearFilters() }, 'Clear filters'),
              ])
            : null,
          tasks.value.length === 0 && !isInitialLoad.value
            ? h('div', { 'data-testid': 'empty-all', class: 'empty' }, 'No tasks')
            : null,

          // 5.3 displayedItems 列表：date / group / task 3 种 kind
          // 🆕 2026-06-23 Task 9.3：加 virtual-scroll-container 标记 + visible-task-count
          //   注意：模拟器本身不虚拟滚动（v-for 渲染所有 item），真机 UI 用 TaskVirtualList.vue
          //   虚拟滚动（DOM 节点数恒定 ≤ 30）。这里加标记是为了让测试能验证 store 容量 +
          //   visible-task-count 派生值，真机虚拟滚动行为由 TaskVirtualList.test.ts 覆盖。
          h('div', { 'data-testid': 'virtual-scroll-container', class: 'virtual-scroll-container' }, [
            h('div', { 'data-testid': 'visible-task-count' }, String(visibleTaskCount.value)),
            h('div', { 'data-testid': 'displayed-items-list' },
              (displayedItems.value as any[]).map((item: any) => {
              if (item.kind === 'date') {
                return h('div', {
                  'data-testid': `date-section-${item.key}`,
                  'data-date-key': item.key,
                  class: 'date-section',
                }, item.label)
              }
              if (item.kind === 'group') {
                const isFake = item.runId.startsWith('__manual__')
                const isPinned = pinnedRunIds.value.has(item.runId)
                return h('div', {
                  'data-testid': `group-card-${item.runId}`,
                  'data-run-id': item.runId,
                  'data-fake': String(isFake),
                  'data-tasks-count': String(item.tasks.length),
                  'data-pinned': String(isPinned),
                  class: ['group-card', isFake ? 'group-card--fake' : '', isPinned ? 'group-card--pinned' : ''],
                  role: 'button',
                  // ① click → openGroupDetail（__manual__ 跳过，与真机一致）
                  onClick: () => {
                    if (!isFake) void props.handlers.openGroupDetail?.(item.runId)
                  },
                  // ② contextmenu → openGroupActionSheet
                  onContextmenu: (e: Event) => {
                    e.preventDefault()
                    if (!isFake) void props.handlers.openGroupActionSheet?.(item)
                  },
                }, [
                  h('div', { class: 'group-card__head' }, [
                    h('span', { class: 'group-card__tone' }, item.displayData?.tone ?? 'automation'),
                    h('h2', { class: 'group-card__title' }, [
                      h('span', {}, `Auto · ${item.tasks.length} tasks`),
                      isPinned ? h('span', { 'data-testid': 'group-card-pinned' }, '📌') : null,
                    ]),
                  ]),
                  // 🆕 2026-06-22 逃逸诊断 runId 显示（user 反馈"截图中连 runId 都没显示"）
                  h('code', {
                    'data-testid': 'group-card-runid',
                    class: ['group-card__runid', isFake ? 'group-card__runid--fake' : ''],
                    title: isFake ? '⚠️ 逃逸 task（runId 丢失）' : `runId: ${item.runId}`,
                  }, item.runId),
                  // plugin badges
                  h('div', { class: 'group-card__plugins' },
                    (item.displayData?.pluginBadges ?? []).map((p: string) =>
                      h('span', { class: 'plugin-badge', key: p }, p),
                    ),
                  ),
                  // 状态行
                  h('div', { class: 'group-card__self' }, [
                    item.displayData?.summary?.passed > 0
                      ? h('span', { class: 'status-badge status-badge--success' }, `✓ ${item.displayData.summary.passed}`)
                      : null,
                    item.displayData?.summary?.failed > 0
                      ? h('span', { class: 'status-badge status-badge--danger' }, `✗ ${item.displayData.summary.failed}`)
                      : null,
                    item.displayData?.summary?.running > 0
                      ? h('span', { class: 'status-badge status-badge--warning' }, `⟳ ${item.displayData.summary.running}`)
                      : null,
                    item.displayData?.summary?.pending > 0
                      ? h('span', { class: 'status-badge status-badge--medium' }, `${item.displayData.summary.pending}`)
                      : null,
                    item.displayData?.duration
                      ? h('span', { class: 'group-card__duration' }, `⏱ ${item.displayData.duration}`)
                      : null,
                  ]),
                  // progress bar
                  h('div', { class: 'progress' }, [
                    h('div', {
                      class: 'progress__fill',
                      style: { width: `${item.displayData?.summary?.percent ?? 0}%` },
                    }),
                  ]),
                  // 时间行
                  h('p', { class: 'time-info' }, [
                    h('span', {}, item.startedAt ?? ''),
                    h('span', {}, `${item.displayData?.summary?.percent ?? 0}%`),
                  ]),
                  // 命中行（v-if isGroupFilterActive）
                  isGroupFilterActive.value
                    ? h('div', { class: 'group-card__hit' }, `hit ${item.tasks.length}`)
                    : null,
                  // 🆕 2026-06-23 Task 9.1：group card 取消按钮（只在 group 有非终态 task 时显示）
                  //   - 真机 UI：长按 group → action sheet → "取消整组"
                  //   - 模拟器：直接渲染按钮，方便测试点击
                  //   - __manual__ 伪 group 不显示取消按钮（没有真实 runId 可取消）
                  !isFake && (item.tasks as EncvTask[]).some((tk) => !TERMINAL_STATUSES.has(tk.status))
                    ? h('button', {
                        'data-testid': 'group-cancel-btn',
                        'data-run-id': item.runId,
                        class: 'group-cancel-btn',
                        title: 'Cancel all tasks in this run',
                        onClick: (e: Event) => {
                          e.stopPropagation()
                          void props.handlers.cancelRun?.(item.runId)
                        },
                      }, 'Cancel run')
                    : null,
                ])
              }
              if (item.kind === 'task') {
                const t = item.task
                return h('div', {
                  'data-testid': `task-card-${t.id}`,
                  'data-task-id': t.id,
                  'data-status': t.status,
                  'data-run-id': t.runId ?? '',
                  class: 'task-card',
                  // click → openTaskDetail
                  onClick: () => void props.handlers.openTaskDetail?.(t),
                }, [
                  h('div', { class: 'task-card__head' }, [
                    h('span', { class: 'task-card__icon' }, '📁'),
                    h('h2', {}, c.getTaskName?.(t) ?? t.id),
                  ]),
                  h('div', { class: 'task-card__meta' }, [
                    h('span', { class: 'task-id' }, `#${t.id.slice(0, 6)}`),
                    h('span', { class: 'status-badge' }, t.status),
                    h('span', { class: 'task-type' }, t.type === 'encrypt' ? 'Encrypt' : 'Decrypt'),
                    t.pluginName ? h('span', { class: 'plugin-badge' }, t.pluginName) : null,
                    t.triggeredBy && t.triggeredBy !== 'user'
                      ? h('span', { class: 'triggered-by-badge' }, t.triggeredBy)
                      : null,
                  ]),
                  // progress（v-if running / cancelling）
                  (t.status === 'running' || t.status === 'cancelling')
                    ? h('div', { class: 'progress-section' }, [
                        h('div', {
                          class: 'task-progress',
                          style: { width: `${t.progress ?? 0}%` },
                        }),
                        h('div', { class: 'progress-detail' }, [
                          t.phase ? h('span', { class: 'phase-label' }, t.phase) : null,
                          h('span', { class: 'progress-percent' }, `${t.progress ?? 0}%`),
                          t.speed ? h('span', { class: 'speed-label' }, t.speed) : null,
                          t.eta ? h('span', { class: 'eta-label' }, `ETA ${t.eta}`) : null,
                        ]),
                      ])
                    : null,
                  // completed info（v-if completed）
                  t.status === 'completed'
                    ? h('div', { class: 'completed-info' }, [
                        h('span', {}, '✓ Completed'),
                        t.containerVersion ? h('span', { class: 'container-version' }, t.containerVersion) : null,
                      ])
                    : null,
                  // warning
                  t.warning
                    ? h('div', {
                      class: 'task-warning',
                      onClick: (e: Event) => { e.stopPropagation(); c.toggleWarningDetail?.(t) },
                    }, t.warning)
                    : null,
                  // error
                  t.error
                    ? h('p', { class: 'task-error' }, t.error)
                    : null,
                  // 取消按钮（v-if running）
                  t.status === 'running'
                    ? h('button', {
                      class: 'cancel-btn',
                      onClick: (e: Event) => { e.stopPropagation(); void c.cancelTaskById(t.id) },
                    }, 'Cancel')
                    : null,
                  // 重试/删除按钮（终态显示）
                  (t.status === 'completed' || t.status === 'failed' || t.status === 'cancelled')
                    ? h('button', {
                      class: 'remove-btn',
                      onClick: (e: Event) => { e.stopPropagation(); void c.removeTaskById(t.id) },
                    }, 'Remove')
                    : null,
                  t.status === 'failed'
                    ? h('button', {
                      class: 'retry-btn',
                      onClick: (e: Event) => { e.stopPropagation(); void c.retryTaskById(t.id) },
                    }, 'Retry')
                    : null,
                ])
              }
              return h('div', { class: 'unknown' }, JSON.stringify(item))
            }),
            ),
          ]),
        ]),
      )

      // ============ 6. ion-fab（右下角 + → openNewTask）============
      vnodes.push(
        h('div', { 'data-testid': 'fab-new-task', class: 'fab' }, [
          h('button', {
            'data-testid': 'fab-new-task-btn',
            title: 'New task',
            onClick: () => void props.handlers.openNewTask?.(),
          }, '+'),
        ]),
      )

      // ============ 7. 诊断数据 section（保留原 TaskListDiag 18 个 data-testid）============
      vnodes.push(
        h('div', { 'data-testid': 'task-list-diag', class: 'diag-panel' }, [
          h('div', { 'data-testid': 'store-tasks-count' }, String(tasks.value.length)),
          h('div', { 'data-testid': 'displayed-items-count' }, String(displayedItems.value.length)),
          h('div', { 'data-testid': 'grouped-count' }, String(groupedTasksByRunId.value.length)),
          h('div', { 'data-testid': 'real-group-count' }, String(realGroupCount.value)),
          h('div', { 'data-testid': 'fake-group-count' }, String(fakeGroupCount.value)),
          h('div', { 'data-testid': 'escape-task-count' }, String(escapeTaskCount.value)),
          h('div', { 'data-testid': 'view-mode' }, String(viewMode.value)),
          h('div', { 'data-testid': 'sort-by' }, String(sortBy.value)),
          h('div', { 'data-testid': 'search-query' }, String(searchQuery.value)),
          h('div', { 'data-testid': 'filter-plugins' }, (filterPlugins.value ?? []).join(',')),
          h('div', { 'data-testid': 'filter-types' }, (filterTypes.value ?? []).join(',')),
          h('div', { 'data-testid': 'filter-statuses' }, (filterStatuses.value ?? []).join(',')),
          h('div', { 'data-testid': 'filter-triggered-by' }, (filterTriggeredBy.value ?? []).join(',')),
          h('div', { 'data-testid': 'filter-date-preset' }, String(filterDatePreset.value)),
          h('div', { 'data-testid': 'pinned-run-ids' }, Array.from(pinnedRunIds.value).join(',')),
        ]),
      )

      return h('div', { 'data-testid': 'task-list-diag-simulator-root' }, vnodes)
    }
  },
})
