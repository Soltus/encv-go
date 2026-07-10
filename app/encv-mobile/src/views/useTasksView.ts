// useTasksView.ts - Tasks.vue 的 script 逻辑拆出（composable）
// 拆分自 Tasks.vue。所有 reactive state / handler / lifecycle 集中在此。
// Tasks.vue 只剩 template + 调 useTasksView() 拿到返回值后解构使用。

import { actionSheetController, alertController, modalController, onIonViewWillEnter } from "@ionic/vue";
import { cogOutline, hardwareChipOutline, person } from "ionicons/icons";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import type { EncvTask } from "@/api/encv";
import { clearCompletedTasks } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";
import { useNewTaskModal } from "@/composables/useNewTaskModal";
import { useTasksList } from "@/composables/useTasksList";
import { showToast } from "@/composables/useToast";

/**
 * useTasksView - Tasks.vue 的核心 composable
 *
 * 拆分自 Tasks.vue。把所有 reactive state / handler / lifecycle 集中在此。
 * Files.vue 在 <script setup> 中调用 useTasksView()，解构返回值，template 用法保持不变。
 */
export function useTasksView() {
  const { t } = useI18n();
  const route = useRoute();
  const router = useRouter();
  const { openNewTask } = useNewTaskModal();

  // 🆕 2026-06-22 任务诊断面板：dev 环境默认显示 + 默认展开
  //   - dev 模式（vite/沙箱）：直接显示（user 一打开 Tasks 页面就能看到逃逸诊断）
  //   - production：需要 ?debug=tasks query 才显示（避免遮挡正常 task 列表）
  // user 反馈"加到哪去了"——dev 模式直接默认显示
  const debugEnabled = computed(() => {
    if (import.meta.env.DEV) return true;
    return route.query.debug === "tasks";
  });

  // 🆕 Task 15：虚拟滚动所需的 ion-content 滚动容器引用
  // ion-content 内部用 shadow DOM 渲染 .inner-scroll，需要通过 shadowRoot.querySelector 获取
  // 参考 DevLogs.vue 的 ensureScrollEl() 模式
  const contentRef = ref<any>(null);
  const scrollEl = ref<HTMLElement | null>(null);
  const virtualListRef = ref<{ forceMeasure: () => void } | null>(null);

  /**
   * 🆕 Task 15：从 ion-content shadow DOM 获取 .inner-scroll 滚动容器
   *
   * ion-content 是 Ionic 的 shadow DOM 组件，实际滚动发生在内部 .inner-scroll 元素上，
   * 不是 ion-content host 本身。@tanstack/vue-virtual 需要拿到这个真实滚动元素才能
   * 监听 scroll 事件 + 测量视口高度。
   *
   * 时序问题：onMounted 时 ion-content 可能还没完成 shadow DOM 渲染 → scrollEl=null
   * 修法：多次重试（rAF + setTimeout 指数退避）+ ResizeObserver 兜底监听 host 尺寸变化
   */
  function ensureScrollEl(): HTMLElement | null {
    if (!contentRef.value) return null;
    const hostEl = (contentRef.value.$el || contentRef.value) as HTMLElement | undefined;
    if (!hostEl?.shadowRoot) return null;
    const el = hostEl.shadowRoot.querySelector(".inner-scroll") as HTMLElement | null;
    if (el && el !== scrollEl.value) scrollEl.value = el;
    return scrollEl.value;
  }

  let scrollElRetryTimer: ReturnType<typeof setTimeout> | null = null;
  let scrollElRO: ResizeObserver | null = null;

  function initScrollElWithRetry(): void {
    let retryCount = 0;
    const maxRetries = 8;
    const tryInit = (): void => {
      const el = ensureScrollEl();
      if (el) {
        // 拿到 .inner-scroll 后，强制 virtualizer 重算（首次 watch 已触发 measure()，
        // 这里再 measure 一次确保虚拟列表渲染首屏 items）
        virtualListRef.value?.forceMeasure?.();
        return;
      }
      retryCount++;
      if (retryCount < maxRetries) {
        // 指数退避：50ms → 100ms → 150ms → 200ms → 250ms → 300ms
        const delay = Math.min(50 * retryCount, 300);
        scrollElRetryTimer = setTimeout(tryInit, delay);
      }
    };
    tryInit();

    // 兜底：ResizeObserver 监听 contentRef 尺寸变化（ion-content 完成渲染时会触发）
    if (typeof ResizeObserver !== "undefined" && contentRef.value) {
      const hostEl = (contentRef.value.$el || contentRef.value) as HTMLElement | undefined;
      if (hostEl) {
        scrollElRO = new ResizeObserver(() => {
          if (!scrollEl.value) tryInit();
        });
        scrollElRO.observe(hostEl);
      }
    }
  }

  const {
    tasks,
    isInitialLoad,
    expandedWarningDetail,
    sortBy,
    showSearch,
    searchQuery,
    showFilters,
    filterPlugins,
    filterTypes,
    filterStatuses,
    filterTriggeredBy,
    statusOptions,
    pluginPopoverOpen,
    typePopoverOpen,
    statusPopoverOpen,
    datePopoverOpen,
    datePopoverEvent,
    pluginPopoverEvent,
    typePopoverEvent,
    statusPopoverEvent,
    availablePlugins,
    hasActiveFilters,
    hasCompletedTasks,
    filteredTasks,
    fetchTasks,
    refresh,
    loadMore,
    hasMore,
    isLoadingMore,
    openPluginPopover,
    openTypePopover,
    openStatusPopover,
    openDatePopover,
    togglePluginFilter,
    toggleTypeFilter,
    toggleStatusFilter,
    clearFilters,
    onSearchInput,
    toggleSort,
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
    // 🆕 2026-07-02：向量搜索相关度（前端 RelevanceBadge 用）
    getTaskSearchScore,
    // 🆕 2026-07-02：搜索模式（strict/combined/greedy/none，前端横幅用）
    searchMode,
    // 🆕 v4 M3
    viewMode,
    filterDatePreset,
    filterDateRange,
    displayedItems,
    // 🆕 2026-06-22 任务诊断面板需要的派生 + 状态
    groupedTasksByRunId,
    pinnedRunIds,
    applyDatePreset,
    setCustomDateRange,
    toggleViewMode,
    // 🆕 v4 M5：单例 workflowService 数据源（groupedItems 已通过 serviceRuns 派生，这里只消费）
    workflowService,
    // 🆕 v6-bug3fix 2026-06-18：hydrate + cancelRun（group card 取消用）
    hydrate,
    cancelRun,
    // 🆕 v6 2026-06-18：左滑删除 + 置顶（group card 操作）
    removeRunTasks,
    togglePinRun,
    isRunPinned,
  } = useTasksList();

  // 任务触发者标签 helpers — 🆕 v6 2026-06-18：从 task 对象读（单一数据源）
  function getTriggeredByColor(task: EncvTask): string {
    const v = task.triggeredBy ?? "user";
    return v === "automation" ? "primary" : v === "ai_agent" ? "secondary" : "medium";
  }
  function getTriggeredByIcon(task: EncvTask): any {
    const v = task.triggeredBy ?? "user";
    return v === "automation" ? cogOutline : v === "ai_agent" ? hardwareChipOutline : person;
  }

  // 🆕 2026-06-18 Task 18：任务卡片副标题 crypto params 摘要
  // 返回 "AES-256 · zstd" / "AES-128" / "zstd" / ""（旧任务无 crypto 字段时返回空串）
  function getCryptoSummary(task: EncvTask): string {
    const parts: string[] = [];
    if (task.cipherMode !== undefined && task.cipherMode !== null) {
      parts.push(task.cipherMode === 1 ? t("tasks.cipherMode256") : t("tasks.cipherMode128"));
    }
    if (task.compressionMode === "zstd") {
      parts.push("Zstd");
    } else if (task.compressionMode === "none") {
      parts.push(t("tasks.compressionNone"));
    }
    return parts.join(" · ");
  }

  async function openTaskDetail(task: EncvTask) {
    const { default: TaskDetailModal } = await import("@/components/TaskDetailModal.vue");
    const modal = await modalController.create({
      component: TaskDetailModal,
      componentProps: { task },
      cssClass: "task-detail-modal",
    });
    await modal.present();
    const { data, role } = await modal.onDidDismiss();
    if (role === "dismiss" && data) {
      if (data.action === "cancel") await cancelTaskById(data.id);
      else if (data.action === "retry") await retryTaskById(data.id);
      else if (data.action === "remove") await removeTaskById(data.id);
    }
  }

  async function handleRefresh(event: CustomEvent) {
    await refresh();
    (event.target as any)?.complete?.();
  }

  // 🆕 2026-06-23 Task 7.2：ion-infinite-scroll 触发 loadMore
  //   - 滚动到底部时触发，加载下一页 task
  //   - complete() 让 infinite-scroll 重置（可以再次触发）
  async function onInfinite(event: CustomEvent) {
    await loadMore();
    (event.target as any)?.complete?.();
  }

  async function handleClearCompleted() {
    const completedCount = await clearCompletedWithConfirm();
    if (!completedCount) return;
    const alert = await alertController.create({
      header: t("tasks.clearConfirmTitle"),
      message: t("tasks.clearConfirmMessage", { count: String(completedCount) }),
      buttons: [
        { text: t("tasks.cancel"), role: "cancel" },
        {
          text: t("tasks.clearConfirm"),
          role: "destructive",
          handler: async () => {
            try {
              const result = await clearCompletedTasks();
              showToast({ message: t("tasks.cleared", { count: String(result.removed) }), duration: 2000, color: "success" });
              await fetchTasks();
            } catch {
              showToast({ message: t("tasks.clearFailed"), duration: 2000, color: "danger" });
            }
          },
        },
      ],
    });
    await alert.present();
  }

  // 🆕 v4 M3：日期筛选 popover 的 preset 列表 + 自定义日期输入框 v-model
  // 预设：今天 / 7天 / 30天 / 全部 / 自定义（4+1 选项）
  const datePresets: { key: "today" | "7d" | "30d" | "all" | "custom"; label: string }[] = [
    { key: "today", label: t("tasks.datePresetToday") },
    { key: "7d", label: t("tasks.datePreset7d") },
    { key: "30d", label: t("tasks.datePreset30d") },
    { key: "all", label: t("tasks.datePresetAll") },
    { key: "custom", label: t("tasks.datePresetCustom") },
  ];

  /** 自定义日期输入框的值（YYYY-MM-DD） — 与 filterDateRange.from/to 双向同步 */
  const customFromInput = ref<string>("");
  const customToInput = ref<string>("");
  watch(
    [() => filterDateRange.value.from, () => filterDateRange.value.to],
    ([from, to]) => {
      customFromInput.value = from ? from.slice(0, 10) : "";
      customToInput.value = to ? to.slice(0, 10) : "";
    },
    { immediate: true }
  );

  function onDatePresetClick(key: "today" | "7d" | "30d" | "all" | "custom") {
    applyDatePreset(key);
  }
  function onCustomFromChange(event: Event) {
    const v = (event.target as HTMLInputElement).value;
    setCustomDateRange(v || undefined, customToInput.value || undefined);
  }
  function onCustomToChange(event: Event) {
    const v = (event.target as HTMLInputElement).value;
    setCustomDateRange(customFromInput.value || undefined, v || undefined);
  }

  // 🆕 2026-06-18 v5-bug3fix：L1 group card 智能状态行 + 命中行
  //   - 整张 card clickable → push 到 L2 GroupDetail（移除展开态）
  //   - 自身状态（passed/failed/running/pending）始终显示
  //   - 命中行（hit N/M + 当前激活的筛选提示）仅在 isGroupFilterActive=true 时显示
  //   - 二者智能共存，不拥挤不冲突
  const isGroupFilterActive = computed(() => {
    return hasActiveFilters.value || searchQuery.value.trim().length > 0;
  });

  /** L1 group card 智能命中行文本
   *  - 4 维度 (plugin/type/status/date) 折叠为单行
   *  - 命中 0 → 显示 "无匹配"（red）
   *  - 命中 = total → 显示 "全部 N"（green）
   *  - 否则 "命中 N/M" + 搜索词（truncated 12 chars）+ 日期 preset
   */
  function getGroupHitSummary(item: { tasks: EncvTask[] }): string {
    const total = item.tasks.length;
    const hit = computeGroupHit(item.tasks);
    const q = searchQuery.value.trim();
    const dateLabel = dateRangeChipLabel();
    const hasDate = filterDatePreset.value !== "all";
    const hasSearch = q.length > 0;

    if (hit === 0) return t("tasks.groupCard.hitZero");
    if (hit === total) {
      if (hasSearch || hasDate) {
        // 全部命中但有搜索/日期过滤 → 显示"全部 + 过滤条件"
        return formatHitSummaryWithExtras(total, q, dateLabel, hasSearch, hasDate);
      }
      return t("tasks.groupCard.hitFull", { total: String(total) });
    }
    return formatHitSummaryWithExtras(hit, q, dateLabel, hasSearch, hasDate, total);
  }

  /** 计算 group 在所有激活筛选下的命中数（plugin/type/status/date/search 交集） */
  function computeGroupHit(tasks: EncvTask[]): number {
    const q = searchQuery.value.trim().toLowerCase();
    const hasSearch = q.length > 0;
    const fromTs = filterDateRange.value.from;
    const toTs = filterDateRange.value.to;
    const hasDate = !!fromTs || !!toTs;
    let hit = 0;
    for (const t of tasks) {
      if (filterPlugins.value.length > 0 && !filterPlugins.value.includes(t.pluginName || "__unknown__")) continue;
      if (filterTypes.value.length > 0 && !filterTypes.value.includes(t.type)) continue;
      if (filterStatuses.value.length > 0 && !filterStatuses.value.includes(t.status)) continue;
      if (hasDate) {
        if (fromTs && t.createdAt < fromTs) continue;
        if (toTs && t.createdAt >= toTs) continue;
      }
      if (hasSearch) {
        const name = getTaskName(t).toLowerCase();
        const plugin = (t.pluginName || "").toLowerCase();
        const error = (t.error || "").toLowerCase();
        const id = t.id.toLowerCase();
        if (!name.includes(q) && !plugin.includes(q) && !error.includes(q) && !id.includes(q)) continue;
      }
      hit++;
    }
    return hit;
  }

  function formatHitSummaryWithExtras(
    hit: number,
    q: string,
    dateLabel: string,
    hasSearch: boolean,
    hasDate: boolean,
    total?: number
  ): string {
    const truncatedQuery = q.length > 12 ? q.slice(0, 12) + "…" : q;
    if (hasSearch && hasDate) {
      return t("tasks.groupCard.hitSummaryFull", {
        hit: String(hit),
        total: total !== undefined ? String(total) : String(hit),
        query: truncatedQuery,
        datePreset: dateLabel,
      });
    }
    if (hasSearch) {
      return t("tasks.groupCard.hitSummaryWithSearch", {
        hit: String(hit),
        total: total !== undefined ? String(total) : String(hit),
        query: truncatedQuery,
      });
    }
    if (hasDate) {
      return t("tasks.groupCard.hitSummaryWithDate", {
        hit: String(hit),
        total: total !== undefined ? String(total) : String(hit),
        datePreset: dateLabel,
      });
    }
    return t("tasks.groupCard.hitSummary", { hit: String(hit), total: total !== undefined ? String(total) : String(hit) });
  }

  /** 🆕 2026-06-18 v5-bug3fix：整张 group card clickable → push 到 L2 GroupDetail
   *  - 不依赖展开态机制（已删除）
   *  - 不跳转 PluginTestsDetail（解耦：插件测试在设置 tab 开发者选项独立页面）
   */
  async function openGroupDetail(runId: string) {
    // __manual__ 前缀的合成 group（user 创建的单个 task）→ 不跳转 L2
    if (!runId || runId.startsWith("__manual__")) return;
    await router.push(`/tabs/tasks/group/${encodeURIComponent(runId)}`);
  }

  /** 🆕 v6-bug3fix 2026-06-18：group card 取消操作
   *  - 仅 running group 显示取消按钮
   *  - 弹 alert 确认 → cancelRun → 乐观更新 + 失败回滚
   */
  function hasRunningTasks(tasks: EncvTask[]): boolean {
    return tasks.some(tk => tk.status === "running" || tk.status === "queued" || tk.status === "cancelling");
  }

  async function confirmCancelGroup(runId: string, tasks: EncvTask[]): Promise<void> {
    if (!runId || runId.startsWith("__manual__")) return;
    const runningCount = tasks.filter(tk => tk.status === "running" || tk.status === "queued").length;
    const alert = await alertController.create({
      header: t("tasks.cancelRunHeader"),
      message: t("tasks.cancelRunMessage", { count: String(runningCount) }),
      buttons: [
        { text: t("common.cancel"), role: "cancel" },
        {
          text: t("tasks.cancelRunConfirm"),
          role: "destructive",
          handler: async () => {
            try {
              await cancelRun(runId);
            } catch (err) {
              const a = await alertController.create({
                header: t("tasks.cancelRunFailedHeader"),
                message: String((err as any)?.message ?? err),
                buttons: [t("common.ok")],
              });
              await a.present();
            }
          },
        },
      ],
    });
    await alert.present();
  }

  // ============ 长按/右键 action-sheet（v6）============

  /** 长按计时器：key=item.key → timer id */
  const _longPressTimers = new Map<string, ReturnType<typeof setTimeout>>();
  const LONG_PRESS_MS = 500;

  function onGroupTouchStart(_e: TouchEvent, item: { key: string }): void {
    if (item.key.startsWith("__manual__") || item.key.startsWith("date-")) return;
    const timer = setTimeout(() => {
      // 触发 action-sheet（仅当 touch 持续 500ms 时）
      const groupItem = item as any;
      if (groupItem.runId) void openGroupActionSheet(groupItem);
    }, LONG_PRESS_MS);
    _longPressTimers.set(item.key, timer);
  }

  function onGroupTouchEnd(_e: TouchEvent, item: { key: string }): void {
    const timer = _longPressTimers.get(item.key);
    if (timer) {
      clearTimeout(timer);
      _longPressTimers.delete(item.key);
    }
  }

  /**
   * 弹出 group action-sheet（长按或右键触发）
   * - 取消：仅 running group
   * - 置顶/取消置顶
   * - 删除：仅终态 group
   * - 查看详情：始终显示
   */
  async function openGroupActionSheet(item: { runId: string; tasks: EncvTask[] }): Promise<void> {
    if (!item.runId || item.runId.startsWith("__manual__")) return;
    const hasRunning = hasRunningTasks(item.tasks);
    const isPinned = isRunPinned(item.runId);
    const buttons: any[] = [
      {
        text: t("tasks.groupCard.openDetail"),
        role: undefined,
        handler: () => {
          void openGroupDetail(item.runId);
        },
      },
      {
        text: isPinned ? t("tasks.unpin") : t("tasks.pin"),
        role: undefined,
        handler: () => {
          const pinned = togglePinRun(item.runId);
          showToast({
            message: pinned ? t("tasks.pinned") : t("tasks.unpinned"),
            duration: 1500,
            color: "medium",
          });
        },
      },
    ];
    if (hasRunning) {
      buttons.push({
        text: t("tasks.cancelRun"),
        role: "destructive",
        handler: () => {
          void confirmCancelGroup(item.runId, item.tasks);
        },
      });
    }
    if (!hasRunning) {
      buttons.push({
        text: t("tasks.remove"),
        role: "destructive",
        handler: () => {
          void confirmRemoveGroup(item.runId, item.tasks);
        },
      });
    }
    buttons.push({ text: t("common.cancel"), role: "cancel" });
    const sheet = await actionSheetController.create({ buttons });
    await sheet.present();
  }

  /** 删除确认 alert（从 openGroupActionSheet 复用） */
  async function confirmRemoveGroup(runId: string, tasks: EncvTask[]): Promise<void> {
    const taskCount = tasks.length;
    const alert = await alertController.create({
      header: t("tasks.removeRunHeader"),
      message: t("tasks.removeRunMessage", { count: String(taskCount) }),
      buttons: [
        { text: t("common.cancel"), role: "cancel" },
        {
          text: t("tasks.remove"),
          role: "destructive",
          handler: async () => {
            try {
              const { removed, failed } = await removeRunTasks(runId);
              if (removed > 0) {
                showToast({
                  message: t("tasks.removeRunSuccess", { removed: String(removed) }),
                  duration: 2000,
                  color: "success",
                });
              }
              if (failed > 0) {
                showToast({
                  message: t("tasks.removeRunPartial", { failed: String(failed) }),
                  duration: 2500,
                  color: "warning",
                });
              }
            } catch (err) {
              const a = await alertController.create({
                header: t("common.error"),
                message: String((err as any)?.message ?? err),
                buttons: [t("common.ok")],
              });
              await a.present();
            }
          },
        },
      ],
    });
    await alert.present();
  }

  // 🆕 v4 M3：把 filterDateRange 转成 YYYY-MM-DD 形式（用于 chip 显示）
  function dateRangeChipLabel(): string {
    if (filterDatePreset.value === "all") return t("tasks.datePresetAll");
    if (filterDatePreset.value === "today") return t("tasks.datePresetToday");
    if (filterDatePreset.value === "7d") return t("tasks.datePreset7d");
    if (filterDatePreset.value === "30d") return t("tasks.datePreset30d");
    if (filterDatePreset.value === "custom") {
      const f = customFromInput.value || "?";
      const t2 = customToInput.value || "?";
      return `${f} → ${t2}`;
    }
    return t("tasks.datePresetAll");
  }

  // 🆕 onMounted：只处理路由 query（长按菜单跳转过来时打开 new task modal）
  // 首次 fetchTasks 由 onIonViewWillEnter 接管（每次切回 tab 智能刷新）。
  onMounted(() => {
    // 🆕 v6 2026-06-18：冷启动从 IndexedDB 同步加载 tasks（v6 核心改造点）
    //   - 之前用 localStorage 存，每次启动同步读取几百个 task 字符串 → 阻塞 main thread
    //   - 现在用 IndexedDB 异步加载，主线程 0 阻塞
    //   - store 暴露 hydrate()；失败回退空数组 + 后续 fetchTasks
    void hydrate();

    // 🆕 Task 15：初始化虚拟滚动所需的 ion-content .inner-scroll 引用
    // ion-content shadow DOM 异步渲染，需要重试 + ResizeObserver 兜底
    initScrollElWithRetry();

    if (route.query.action === "new") {
      const sourcePath = route.query.source as string;
      const taskType = (route.query.type || "encrypt") as "encrypt" | "decrypt";
      router.replace({ path: "/tabs/tasks", query: {} });
      if (sourcePath) {
        openNewTask(sourcePath, taskType);
      } else {
        openNewTask();
      }
    }
  });

  // 🆕 Task 15：组件卸载时清理 scrollEl 重试定时器 + ResizeObserver
  onBeforeUnmount(() => {
    if (scrollElRetryTimer) {
      clearTimeout(scrollElRetryTimer);
      scrollElRetryTimer = null;
    }
    if (scrollElRO) {
      scrollElRO.disconnect();
      scrollElRO = null;
    }
  });

  // 🆕 onIonViewWillEnter：参考 Files.vue 实现切回 tab 自动刷新
  //   智能条件：如果存在 running/queued task 立即拉一次最新列表；否则只靠 WS 增量更新
  //   避免无谓的 GET /api/tasks 调用
  //   2026-06-18：套 try/catch + console.error 防御 — 历史教训：useTasksList 内部
  //   store.tasks 自动解包曾导致 tasks.value 抛 TypeError 把 tab 冻住
  onIonViewWillEnter(() => {
    try {
      const arr = tasks.value;
      if (!Array.isArray(arr)) {
        console.error("[Tasks.onIonViewWillEnter] tasks.value is not array:", typeof arr, arr);
        return;
      }
      if (arr.length === 0) {
        void fetchTasks();
        return;
      }
      // 存在 running/queued → 立即拉一次最新
      const hasActive = arr.some(t => t.status === "running" || t.status === "queued" || t.status === "cancelling");
      if (hasActive) {
        void fetchTasks();
      }
    } catch (err) {
      console.error("[Tasks.onIonViewWillEnter] crashed (caught, do not block tab):", err);
    }
  });

  return {
    // i18n
    t,
    // virtual scroll
    contentRef,
    scrollEl,
    virtualListRef,
    ensureScrollEl,
    initScrollElWithRetry,
    // from useTasksList
    tasks,
    isInitialLoad,
    expandedWarningDetail,
    sortBy,
    showSearch,
    searchQuery,
    showFilters,
    filterPlugins,
    filterTypes,
    filterStatuses,
    filterTriggeredBy,
    statusOptions,
    pluginPopoverOpen,
    typePopoverOpen,
    statusPopoverOpen,
    datePopoverOpen,
    datePopoverEvent,
    pluginPopoverEvent,
    typePopoverEvent,
    statusPopoverEvent,
    availablePlugins,
    hasActiveFilters,
    hasCompletedTasks,
    filteredTasks,
    openPluginPopover,
    openTypePopover,
    openStatusPopover,
    openDatePopover,
    togglePluginFilter,
    toggleTypeFilter,
    toggleStatusFilter,
    clearFilters,
    onSearchInput,
    toggleSort,
    cancelTaskById,
    retryTaskById,
    removeTaskById,
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
    getTaskSearchScore,
    searchMode,
    viewMode,
    filterDatePreset,
    filterDateRange,
    displayedItems,
    groupedTasksByRunId,
    pinnedRunIds,
    toggleViewMode,
    workflowService,
    // infinite scroll (template 用)
    hasMore,
    isLoadingMore,
    // group card actions (template 用)
    isRunPinned,
    // modal / new task
    openNewTask,
    // triggeredBy icons (template 用)
    hardwareChipOutline,
    cogOutline,
    // inline helpers
    debugEnabled,
    getTriggeredByColor,
    getTriggeredByIcon,
    getCryptoSummary,
    openTaskDetail,
    handleRefresh,
    onInfinite,
    handleClearCompleted,
    datePresets,
    customFromInput,
    customToInput,
    onDatePresetClick,
    onCustomFromChange,
    onCustomToChange,
    isGroupFilterActive,
    getGroupHitSummary,
    openGroupDetail,
    onGroupTouchStart,
    onGroupTouchEnd,
    openGroupActionSheet,
  };
}
