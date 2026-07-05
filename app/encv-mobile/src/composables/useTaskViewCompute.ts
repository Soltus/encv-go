/**
 * useTaskViewCompute — Web Worker 委托视图计算（主线程封装）
 *
 * 2026-06-23 spec backend-sql-authority-view-pagination Task 10：
 *   - 把 O(N) 视图计算委托给 Web Worker，避免阻塞 UI 主线程
 *   - 1000+ task 时 displayedItems computed 遍历会卡顿
 *   - Worker 接收 tasks 快照 + filter 状态，返回 displayedItems 数组
 *
 * 设计：
 *   - watch tasks + filter/sort/view 状态变化
 *   - debounce 16ms（1 帧）→ postMessage 给 worker
 *   - 接收 worker 结果 → 更新 displayedItems ref（触发虚拟滚动重渲染）
 *   - date section label：worker 返回 dateKey，主线程映射为 i18n label
 *
 * 降级策略：
 *   - Web Worker 不可用（SSR / 旧浏览器 / Worker 构造失败）→ 主线程同步计算
 *   - Worker 计算出错 → 保留旧值 + warn 日志
 *
 * Capacitor 兼容：
 *   - Android WebView 支持 Web Worker（API 21+）
 *   - iOS WKWebView 支持 Web Worker（iOS 10+）
 *   - 无需 polyfill
 */
import { type ComputedRef, computed, type Ref, ref, toRaw, watch } from "vue";
import type { EncvTask } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";
import type { ComputeInput, ComputeOutput } from "@/workers/taskViewCompute.worker";
// 🆕 Vite 原生支持 ?worker import（dev 用 esbuild，build 用 rollup 单独打包）
import TaskViewComputeWorker from "@/workers/taskViewCompute.worker?worker";

export interface UseTaskViewComputeOptions {
  /** 任务列表（响应式） */
  tasks: Ref<EncvTask[]> | ComputedRef<EncvTask[]>;
  /** 视图模式 */
  viewMode: Ref<"group" | "flat"> | ComputedRef<"group" | "flat">;
  /** 排序模式 */
  sortBy: Ref<"activity" | "created"> | ComputedRef<"activity" | "created">;
  /** 搜索关键词 */
  searchQuery: Ref<string> | ComputedRef<string>;
  /** 插件筛选 */
  filterPlugins: Ref<string[]> | ComputedRef<string[]>;
  /** 类型筛选 */
  filterTypes: Ref<string[]> | ComputedRef<string[]>;
  /** 状态筛选 */
  filterStatuses: Ref<string[]> | ComputedRef<string[]>;
  /** 触发者筛选 */
  filterTriggeredBy: Ref<string[]> | ComputedRef<string[]>;
  /** 日期范围筛选 */
  filterDateRange: Ref<{ from?: string; to?: string }> | ComputedRef<{ from?: string; to?: string }>;
  /** 置顶 runId 集合 */
  pinnedRunIds: Ref<Set<string>> | ComputedRef<Set<string>>;
}

export interface UseTaskViewCompute {
  /** 计算后的 displayedItems（date section label 已映射为 i18n） */
  displayedItems: Ref<any[]>;
  /** 是否正在计算（worker 异步） */
  isComputing: Ref<boolean>;
  /** 是否降级为主线程同步计算 */
  isFallback: Ref<boolean>;
}

/**
 * 主线程同步计算（降级用）
 *   - 与 worker 内 buildDisplayedItems 逻辑一致
 *   - date section label 直接用 i18n
 */
function computeSync(input: Omit<ComputeInput, "type" | "requestId">, t: (key: string, opts?: any) => string): any[] {
  // 复用 worker 的纯计算函数（通过动态 import 避免主线程打包冗余）
  // 这里直接内联逻辑，因为 worker 模块不能在主线程 import（?worker 后缀）
  const filters = {
    searchQuery: input.searchQuery,
    filterPlugins: input.filterPlugins,
    filterTypes: input.filterTypes,
    filterStatuses: input.filterStatuses,
    filterTriggeredBy: input.filterTriggeredBy,
    filterDateRange: input.filterDateRange,
  };

  // filter
  const q = filters.searchQuery.trim().toLowerCase();
  const fromTs = filters.filterDateRange.from;
  const toTs = filters.filterDateRange.to;
  const hasDate = !!fromTs || !!toTs;
  const hasSearch = q.length > 0;
  const filtered: EncvTask[] = [];
  for (const tk of input.tasks) {
    if (filters.filterPlugins.length > 0 && !filters.filterPlugins.includes(tk.pluginName || "__unknown__")) continue;
    if (filters.filterTypes.length > 0 && !filters.filterTypes.includes(tk.type)) continue;
    if (filters.filterStatuses.length > 0 && !filters.filterStatuses.includes(tk.status)) continue;
    if (filters.filterTriggeredBy.length > 0) {
      const by = tk.triggeredBy ?? "user";
      if (!filters.filterTriggeredBy.includes(by as any)) continue;
    }
    if (hasDate) {
      if (fromTs && tk.createdAt < fromTs) continue;
      if (toTs && tk.createdAt >= toTs) continue;
    }
    if (hasSearch) {
      const name = (tk.targetPath?.split("/").pop() ?? tk.sourcePath?.split("/").pop() ?? tk.id.slice(0, 8)).toLowerCase();
      const plugin = (tk.pluginName || "").toLowerCase();
      const error = (tk.error || "").toLowerCase();
      const id = tk.id.toLowerCase();
      if (!name.includes(q) && !plugin.includes(q) && !error.includes(q) && !id.includes(q)) continue;
    }
    filtered.push(tk);
  }

  // sort
  const sorted = [...filtered];
  if (input.sortBy === "activity") {
    sorted.sort((a, b) => {
      const aKey = Math.max(new Date(a.createdAt).getTime(), a.completedAt ? new Date(a.completedAt).getTime() : 0);
      const bKey = Math.max(new Date(b.createdAt).getTime(), b.completedAt ? new Date(b.completedAt).getTime() : 0);
      return bKey - aKey;
    });
  } else {
    sorted.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
  }

  // date section key
  function dateSectionKey(date: string): string {
    const d = new Date(date);
    const now = new Date();
    const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
    const yesterdayStart = todayStart - 86400000;
    const weekStart = todayStart - 7 * 86400000;
    const monthStart = new Date(now.getFullYear(), now.getMonth(), 1).getTime();
    const ts = d.getTime();
    if (ts >= todayStart) return "today";
    if (ts >= yesterdayStart) return "yesterday";
    if (ts >= weekStart) return "thisWeek";
    if (ts >= monthStart) return "thisMonth";
    return "earlier";
  }

  const items: any[] = [];
  let lastDateKey = "";
  function pushDateHeader(key: string) {
    if (key === lastDateKey) return;
    lastDateKey = key;
    items.push({ kind: "date", key: `date-${key}`, label: t(`tasks.date.${key}`, { defaultValue: key }) });
  }

  if (input.viewMode === "group") {
    // group by runId
    const groups = new Map<string, { runId: string; tasks: EncvTask[]; startedAt: string }>();
    for (const tk of sorted) {
      const key = tk.runId || `__manual__${tk.id}`;
      const g = groups.get(key);
      if (g) g.tasks.push(tk);
      else groups.set(key, { runId: tk.runId || key, tasks: [tk], startedAt: tk.createdAt });
    }
    const groupList: { key: string; runId: string; startedAt: string; tasks: EncvTask[] }[] = [];
    for (const [key, g] of groups) {
      groupList.push({ key, runId: g.runId, startedAt: g.startedAt, tasks: g.tasks });
    }
    const pinnedSet = new Set(input.pinnedRunIds);
    groupList.sort((a, b) => {
      const aPinned = pinnedSet.has(a.runId);
      const bPinned = pinnedSet.has(b.runId);
      if (aPinned !== bPinned) return aPinned ? -1 : 1;
      return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime();
    });

    for (const g of groupList) {
      pushDateHeader(dateSectionKey(g.startedAt));
      // computeGroupCounters（内联）
      const counters = computeGroupCountersSync(g.tasks, filters);
      if (!counters.hitAny) continue;
      // buildGroupDisplayData（内联）
      const displayData = buildGroupDisplayDataSync(g.tasks, g.startedAt);
      items.push({
        kind: "group",
        key: g.key,
        runId: g.runId,
        startedAt: g.startedAt,
        tasks: g.tasks,
        counters,
        displayData,
      });
    }
  } else {
    for (const tk of sorted) {
      pushDateHeader(dateSectionKey(tk.createdAt));
      items.push({ kind: "task", key: `t-${tk.id}`, task: tk });
    }
  }
  return items;
}

function computeGroupCountersSync(
  tasks: EncvTask[],
  filters: {
    searchQuery: string;
    filterPlugins: string[];
    filterTypes: string[];
    filterStatuses: string[];
    filterDateRange: { from?: string; to?: string };
  }
): any {
  const plugins: Record<string, { hit: number; total: number }> = {};
  const types: Record<string, { hit: number; total: number }> = {};
  const statuses: Record<string, { hit: number; total: number }> = {};
  const date = { hit: 0, total: tasks.length };
  const search = { hit: 0, total: tasks.length };
  const q = (filters.searchQuery ?? "").trim().toLowerCase();
  const fromTs = filters.filterDateRange?.from;
  const toTs = filters.filterDateRange?.to;
  const hasSearch = q.length > 0;
  const hasDate = !!fromTs || !!toTs;
  for (const tk of tasks) {
    const pName = tk.pluginName || "__unknown__";
    if (!plugins[pName]) plugins[pName] = { hit: 0, total: 0 };
    plugins[pName].total++;
    if (filters.filterPlugins.length === 0 || filters.filterPlugins.includes(pName)) plugins[pName].hit++;
    if (!types[tk.type]) types[tk.type] = { hit: 0, total: 0 };
    types[tk.type].total++;
    if (filters.filterTypes.length === 0 || filters.filterTypes.includes(tk.type)) types[tk.type].hit++;
    if (!statuses[tk.status]) statuses[tk.status] = { hit: 0, total: 0 };
    statuses[tk.status].total++;
    if (filters.filterStatuses.length === 0 || filters.filterStatuses.includes(tk.status)) statuses[tk.status].hit++;
    if (!hasDate || ((!fromTs || tk.createdAt >= fromTs) && (!toTs || tk.createdAt < toTs))) date.hit++;
    if (!hasSearch) search.hit++;
    else {
      const name = (tk.targetPath?.split("/").pop() ?? tk.sourcePath?.split("/").pop() ?? tk.id.slice(0, 8)).toLowerCase();
      const plugin = (tk.pluginName || "").toLowerCase();
      const error = (tk.error || "").toLowerCase();
      const id = tk.id.toLowerCase();
      if (name.includes(q) || plugin.includes(q) || error.includes(q) || id.includes(q)) search.hit++;
    }
  }
  const hitAny =
    Object.values(plugins).some(p => p.hit > 0) &&
    Object.values(types).some(ty => ty.hit > 0) &&
    Object.values(statuses).some(s => s.hit > 0) &&
    date.hit > 0;
  return { plugins, types, statuses, date, search, hitAny };
}

function buildGroupDisplayDataSync(groupTasks: EncvTask[], startedAt: string): any {
  const summary = { total: groupTasks.length, passed: 0, failed: 0, running: 0, pending: 0, percent: 0 };
  let dominantStatus = "running";
  const plugins = new Set<string>();
  for (const tk of groupTasks) {
    if (tk.status === "completed") summary.passed++;
    else if (tk.status === "failed") summary.failed++;
    else if (tk.status === "running" || tk.status === "cancelling") summary.running++;
    else if (tk.status === "queued") summary.pending++;
    if (tk.pluginName) plugins.add(tk.pluginName);
  }
  const finished = summary.passed + summary.failed;
  if (groupTasks.length > 0) summary.percent = Math.round((finished / groupTasks.length) * 100);
  if (summary.failed > 0) dominantStatus = "failed";
  else if (summary.running > 0) dominantStatus = "running";
  else if (summary.pending > 0) dominantStatus = "queued";
  else if (summary.passed > 0) dominantStatus = "completed";
  const tone = groupTasks[0]?.triggeredBy === "ai_agent" ? "ai_agent" : groupTasks[0]?.triggeredBy === "automation" ? "automation" : "user";
  const moodClass =
    summary.failed > 0
      ? "tl-mood--fail"
      : summary.running > 0
        ? "tl-mood--running"
        : summary.passed === groupTasks.length && groupTasks.length > 0
          ? "tl-mood--success"
          : "";
  const pluginBadges = Array.from(plugins).slice(0, 3);
  let duration = "";
  const completed = groupTasks.filter(tk => tk.completedAt);
  if (completed.length > 0) {
    const maxEnd = Math.max(...completed.map(tk => new Date(tk.completedAt!).getTime()));
    const ms = maxEnd - new Date(startedAt).getTime();
    if (ms > 0) {
      const minutes = Math.floor(ms / 60000);
      const seconds = Math.floor((ms % 60000) / 1000);
      duration = minutes > 0 ? `${minutes}m ${seconds}s` : `${(ms / 1000).toFixed(1)}s`;
    }
  }
  return { summary, dominantStatus, tone, moodClass, pluginBadges, duration };
}

export function useTaskViewCompute(options: UseTaskViewComputeOptions): UseTaskViewCompute {
  const { t } = useI18n();
  const isComputing = ref(false);
  const isFallback = ref(false);

  // 🆕 检测测试环境（vitest 设置 import.meta.env.VITEST = true）
  //   - jsdom 不支持真正的 Web Worker（new Worker() 可能不抛错但永不响应）
  //   - 测试环境直接用同步计算，避免异步等待 worker 响应
  const isTestEnv = !!(import.meta as any).env?.VITEST;

  // 尝试初始化 Worker（非测试环境）
  let worker: Worker | null = null;
  let requestId = 0;
  let pendingTimer: ReturnType<typeof setTimeout> | null = null;
  let lastInput: Omit<ComputeInput, "type" | "requestId"> | null = null;

  if (!isTestEnv) {
    try {
      worker = new TaskViewComputeWorker();
    } catch (e) {
      console.warn("[useTaskViewCompute] Worker init failed, fallback to sync:", e);
      worker = null;
      isFallback.value = true;
    }
  } else {
    isFallback.value = true;
  }

  // ============ 降级路径：同步 computed（测试环境 + Worker 不可用 + Worker 未就绪兜底） ============
  //   - 用 computed 保持同步语义（与原 displayedItems computed 行为一致）
  //   - 🆕 2026-06-23 修复真机空白：worker 异步计算期间用 sync 兜底
  //     - worker 首次计算前 workerDisplayedItems=[] → 页面空白
  //     - worker onerror 后 isFallback=true 但旧逻辑不切回 sync
  //     - 修法：displayedItems 用 computed，worker 未就绪/fallback 时用 sync
  const syncDisplayedItems = computed<any[]>(() => {
    const input = {
      tasks: options.tasks.value,
      viewMode: options.viewMode.value,
      sortBy: options.sortBy.value,
      searchQuery: options.searchQuery.value,
      filterPlugins: options.filterPlugins.value,
      filterTypes: options.filterTypes.value,
      filterStatuses: options.filterStatuses.value,
      filterTriggeredBy: options.filterTriggeredBy.value,
      filterDateRange: options.filterDateRange.value,
      pinnedRunIds: Array.from(options.pinnedRunIds.value),
    };
    return computeSync(input, t);
  });

  // ============ Worker 路径：异步 ref（debounce 16ms） ============
  const workerDisplayedItems = ref<any[]>([]);
  // 🆕 worker 是否已返回过有效结果（首次计算前为 false → 用 sync 兜底）
  //   - 必须是 ref：displayedItems computed 依赖此值，worker 返回结果后切到 workerDisplayedItems
  const workerHasResult = ref(false);

  if (worker) {
    worker.onmessage = (e: MessageEvent<ComputeOutput>) => {
      const output = e.data;
      if (output?.type !== "result") return;
      isComputing.value = false;
      workerHasResult.value = true;
      // date section: dateKey → i18n label
      const items = output.items.map((item: any) => {
        if (item.kind === "date" && item.dateKey) {
          return { ...item, label: t(`tasks.date.${item.dateKey}`, { defaultValue: item.dateKey }) };
        }
        return item;
      });
      workerDisplayedItems.value = items;
    };
    worker.onerror = e => {
      console.warn("[useTaskViewCompute] Worker error, fallback to sync:", e);
      isFallback.value = true;
      workerHasResult.value = false;
    };
  }

  /**
   * 触发 Worker 计算（debounce 16ms = 1 帧）
   *   - 连续 filter 变化时只发最后一次（避免 worker 队列积压）
   *   - 16ms 对应 60fps，用户感知不到延迟
   */
  function scheduleWorkerCompute(): void {
    if (!worker) return;
    // 收集当前输入
    // ⚠️ 关键：tasks 用 toRaw 剥掉 Pinia reactive Proxy
    //   - worker.postMessage 走 structured clone 算法，不能 clone Proxy
    //   - 否则抛 DataCloneError: Failed to execute 'postMessage' on 'Worker'
    //   - 生产环境会导致页面空白（worker 永远不返回结果）
    lastInput = {
      tasks: toRaw(options.tasks.value),
      viewMode: options.viewMode.value,
      sortBy: options.sortBy.value,
      searchQuery: options.searchQuery.value,
      filterPlugins: toRaw(options.filterPlugins.value),
      filterTypes: toRaw(options.filterTypes.value),
      filterStatuses: toRaw(options.filterStatuses.value),
      filterTriggeredBy: toRaw(options.filterTriggeredBy.value),
      filterDateRange: toRaw(options.filterDateRange.value),
      pinnedRunIds: Array.from(options.pinnedRunIds.value),
    };

    // debounce
    if (pendingTimer) clearTimeout(pendingTimer);
    isComputing.value = true;
    pendingTimer = setTimeout(() => {
      if (!worker || !lastInput) return;
      requestId++;
      const input: ComputeInput = {
        type: "compute",
        ...lastInput,
        requestId,
      };
      worker.postMessage(input);
    }, 16);
  }

  // Worker 模式：watch 所有依赖 → debounce → postMessage
  //
  // 🆕 2026-07-02 性能修复：拆分 watch，避免 searchQuery 变化时深度遍历 tasks 数组
  //   - 原问题：watch([tasks, searchQuery, ...], cb, { deep: true }) 会在 searchQuery
  //     变化时对 tasks（可能几千个任务）做深度遍历，阻塞 UI 主线程
  //   - 修复：tasks 单独 deep watch（任务对象会被局部 patch，需 deep 检测）
  //           其他 filter/search 状态单独 watch（数组/字符串，deep 遍历成本低）
  //   - 效果：搜索输入时只触发 filter watch（浅遍历），不遍历 tasks 数组
  if (worker) {
    // tasks 变化：deep watch（检测任务对象的局部 patch，如 status/progress 变化）
    watch(options.tasks, () => scheduleWorkerCompute(), { deep: true, immediate: true });
    // filter/search/view 状态变化：这些是短数组或字符串，deep 遍历成本低
    watch(
      [
        options.viewMode,
        options.sortBy,
        options.searchQuery,
        options.filterPlugins,
        options.filterTypes,
        options.filterStatuses,
        options.filterTriggeredBy,
        options.filterDateRange,
        options.pinnedRunIds,
      ],
      () => scheduleWorkerCompute(),
      { deep: true }
    );
  }

  // 🆕 2026-06-23 修复真机空白：统一出口改为 computed
  //   - isFallback=true（测试/worker 不可用/worker onerror）→ syncDisplayedItems
  //   - isFallback=false 但 worker 尚未返回结果（workerHasResult=false）→ syncDisplayedItems 兜底
  //   - isFallback=false 且 worker 已返回结果 → workerDisplayedItems
  //   - 这样 worker 异步计算期间不会空白（sync 立即给出结果），worker 返回后切换
  const displayedItems = computed<any[]>(() => {
    if (isFallback.value || !workerHasResult.value) {
      return syncDisplayedItems.value;
    }
    return workerDisplayedItems.value;
  });

  return {
    displayedItems,
    isComputing,
    isFallback,
  };
}
