/**
 * taskViewCompute.worker — 视图计算 Web Worker
 *
 * 2026-06-23 spec backend-sql-authority-view-pagination Task 10：
 *   - 把 O(N) 视图计算委托给 Web Worker，避免阻塞 UI 主线程
 *   - 1000+ task 时 filteredTasks / sortedTasks / groupedTasksByRunId / displayedItems
 *     computed 遍历会卡顿（即使虚拟滚动只渲染可见行）
 *   - Worker 接收 tasks 快照 + filter/sort/view 状态，返回 displayedItems 数组
 *
 * 职责：
 *   - filteredTasks：按 plugin/type/status/triggeredBy/date/search 过滤
 *   - sortedTasks：按 activity/created 排序
 *   - groupedTasksByRunId：按 runId 聚合 + pinned 排序
 *   - displayedItems：构建 date/group/task 异构列表（含 counters/displayData）
 *
 * 不在 Worker 中做的事：
 *   - i18n 标签（date section 返回 dateKey，主线程映射为 label）
 *   - 响应式（Worker 不持响应式状态，只做纯计算）
 *
 * 通信协议：
 *   主线程 → Worker：{ type: 'compute', tasks, viewMode, sortBy, ...filters, pinnedRunIds, requestId }
 *   Worker → 主线程：{ type: 'result', items, requestId }
 *
 * 降级策略（在主线程 useTaskViewCompute 中处理）：
 *   - Worker 不可用 → 主线程同步计算（向后兼容）
 *   - Worker 初始化失败 → 主线程同步计算 + warn 日志
 */
import type { EncvTask } from "@/api/encv";

// ============ 类型定义（与主线程共享） ============
export interface ComputeInput {
  type: "compute";
  tasks: EncvTask[];
  viewMode: "group" | "flat";
  sortBy: "activity" | "created";
  searchQuery: string;
  filterPlugins: string[];
  filterTypes: string[];
  filterStatuses: string[];
  filterTriggeredBy: string[];
  filterDateRange: { from?: string; to?: string };
  pinnedRunIds: string[];
  requestId: number;
}

export interface ComputeOutput {
  type: "result";
  items: any[];
  requestId: number;
}

// ============ 纯计算函数（从 useTasksList.ts 提取，保持逻辑一致） ============

function getTaskDisplayName(task: EncvTask): string {
  if (task.targetPath) return task.targetPath.split("/").pop() || task.targetPath;
  if (task.sourcePath) return task.sourcePath.split("/").pop() || task.sourcePath;
  return task.id.slice(0, 8);
}

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

/**
 * 过滤 tasks（与 taskStore.filteredTasks 逻辑一致）
 */
function filterTasks(
  tasks: EncvTask[],
  filters: {
    searchQuery: string;
    filterPlugins: string[];
    filterTypes: string[];
    filterStatuses: string[];
    filterTriggeredBy: string[];
    filterDateRange: { from?: string; to?: string };
  }
): EncvTask[] {
  const q = filters.searchQuery.trim().toLowerCase();
  const fromTs = filters.filterDateRange.from;
  const toTs = filters.filterDateRange.to;
  const hasDate = !!fromTs || !!toTs;
  const hasSearch = q.length > 0;
  const plugins = filters.filterPlugins;
  const types = filters.filterTypes;
  const statuses = filters.filterStatuses;
  const triggeredBy = filters.filterTriggeredBy;
  const out: EncvTask[] = [];
  for (const t of tasks) {
    if (plugins.length > 0 && !plugins.includes(t.pluginName || "__unknown__")) continue;
    if (types.length > 0 && !types.includes(t.type)) continue;
    if (statuses.length > 0 && !statuses.includes(t.status)) continue;
    if (triggeredBy.length > 0) {
      const by = t.triggeredBy ?? "user";
      if (!triggeredBy.includes(by as any)) continue;
    }
    if (hasDate) {
      if (fromTs && t.createdAt < fromTs) continue;
      if (toTs && t.createdAt >= toTs) continue;
    }
    if (hasSearch) {
      const name = (t.targetPath?.split("/").pop() ?? t.sourcePath?.split("/").pop() ?? t.id.slice(0, 8)).toLowerCase();
      const plugin = (t.pluginName || "").toLowerCase();
      const error = (t.error || "").toLowerCase();
      const id = t.id.toLowerCase();
      if (!name.includes(q) && !plugin.includes(q) && !error.includes(q) && !id.includes(q)) continue;
    }
    out.push(t);
  }
  return out;
}

/**
 * 排序 tasks（与 taskStore.sortedTasks 逻辑一致）
 */
function sortTasks(tasks: EncvTask[], sortBy: "activity" | "created"): EncvTask[] {
  const arr = [...tasks];
  if (sortBy === "activity") {
    arr.sort((a, b) => {
      const aKey = Math.max(new Date(a.createdAt).getTime(), a.completedAt ? new Date(a.completedAt).getTime() : 0);
      const bKey = Math.max(new Date(b.createdAt).getTime(), b.completedAt ? new Date(b.completedAt).getTime() : 0);
      return bKey - aKey;
    });
  } else {
    arr.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
  }
  return arr;
}

/**
 * 按 runId 聚合（与 taskStore.groupedTasksByRunId 逻辑一致）
 */
function groupByRunId(
  sortedTasks: EncvTask[],
  pinnedRunIds: string[]
): { key: string; runId: string; startedAt: string; tasks: EncvTask[] }[] {
  const groups = new Map<string, { runId: string; tasks: EncvTask[]; startedAt: string }>();
  for (const tk of sortedTasks) {
    const key = tk.runId || `__manual__${tk.id}`;
    const g = groups.get(key);
    if (g) g.tasks.push(tk);
    else groups.set(key, { runId: tk.runId || key, tasks: [tk], startedAt: tk.createdAt });
  }
  const result: { key: string; runId: string; startedAt: string; tasks: EncvTask[] }[] = [];
  for (const [key, g] of groups) {
    result.push({ key, runId: g.runId, startedAt: g.startedAt, tasks: g.tasks });
  }
  const pinnedSet = new Set(pinnedRunIds);
  result.sort((a, b) => {
    const aPinned = pinnedSet.has(a.runId);
    const bPinned = pinnedSet.has(b.runId);
    if (aPinned !== bPinned) return aPinned ? -1 : 1;
    return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime();
  });
  return result;
}

/**
 * 计算 group 的 counters（与 useTasksList.computeGroupCounters 逻辑一致）
 */
function computeGroupCounters(
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
    if (filters.filterPlugins.length === 0 || filters.filterPlugins.includes(pName)) {
      plugins[pName].hit++;
    }
    if (!types[tk.type]) types[tk.type] = { hit: 0, total: 0 };
    types[tk.type].total++;
    if (filters.filterTypes.length === 0 || filters.filterTypes.includes(tk.type)) {
      types[tk.type].hit++;
    }
    if (!statuses[tk.status]) statuses[tk.status] = { hit: 0, total: 0 };
    statuses[tk.status].total++;
    if (filters.filterStatuses.length === 0 || filters.filterStatuses.includes(tk.status)) {
      statuses[tk.status].hit++;
    }
    if (!hasDate || ((!fromTs || tk.createdAt >= fromTs) && (!toTs || tk.createdAt < toTs))) {
      date.hit++;
    }
    if (!hasSearch) {
      search.hit++;
    } else {
      const name = getTaskDisplayName(tk).toLowerCase();
      const plugin = (tk.pluginName || "").toLowerCase();
      const error = (tk.error || "").toLowerCase();
      const id = tk.id.toLowerCase();
      if (name.includes(q) || plugin.includes(q) || error.includes(q) || id.includes(q)) {
        search.hit++;
      }
    }
  }
  const hitAny =
    Object.values(plugins).some(p => p.hit > 0) &&
    Object.values(types).some(ty => ty.hit > 0) &&
    Object.values(statuses).some(s => s.hit > 0) &&
    date.hit > 0;
  return { plugins, types, statuses, date, search, hitAny };
}

/**
 * 构建 group displayData（与 useTasksList.buildGroupDisplayData 逻辑一致）
 */
function buildGroupDisplayData(groupTasks: EncvTask[], startedAt: string): any {
  const summary = { total: groupTasks.length, passed: 0, failed: 0, running: 0, pending: 0, percent: 0 };
  let dominantStatus: string = "running";
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

/**
 * 构建 displayedItems（与 useTasksList.displayedItems 逻辑一致）
 *   - date section 返回 dateKey（不返回 label，主线程做 i18n 映射）
 *   - group item 包含 counters + displayData
 *   - task item 包含 task 引用
 */
function buildDisplayedItems(input: ComputeInput): any[] {
  const filters = {
    searchQuery: input.searchQuery,
    filterPlugins: input.filterPlugins,
    filterTypes: input.filterTypes,
    filterStatuses: input.filterStatuses,
    filterTriggeredBy: input.filterTriggeredBy,
    filterDateRange: input.filterDateRange,
  };

  // 1. filter
  const filtered = filterTasks(input.tasks, filters);
  // 2. sort
  const sorted = sortTasks(filtered, input.sortBy);

  const items: any[] = [];
  let lastDateKey = "";
  function pushDateHeader(key: string) {
    if (key === lastDateKey) return;
    lastDateKey = key;
    // 返回 dateKey，主线程映射为 i18n label
    items.push({ kind: "date", key: `date-${key}`, dateKey: key });
  }

  if (input.viewMode === "group") {
    // 3. group by runId
    const groups = groupByRunId(sorted, input.pinnedRunIds);
    for (const g of groups) {
      pushDateHeader(dateSectionKey(g.startedAt));
      const counters = computeGroupCounters(g.tasks, filters);
      if (!counters.hitAny) continue;
      items.push({
        kind: "group",
        key: g.key,
        runId: g.runId,
        startedAt: g.startedAt,
        tasks: g.tasks,
        counters,
        displayData: buildGroupDisplayData(g.tasks, g.startedAt),
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

// ============ Worker 消息处理 ============
self.onmessage = (e: MessageEvent<ComputeInput>) => {
  const input = e.data;
  if (!input || input.type !== "compute") return;
  try {
    const items = buildDisplayedItems(input);
    const output: ComputeOutput = { type: "result", items, requestId: input.requestId };
    (self as any).postMessage(output);
  } catch (err) {
    // Worker 内计算出错：返回空数组（主线程会保留旧值）
    console.error("[taskViewCompute.worker] compute failed:", err);
    const output: ComputeOutput = { type: "result", items: [], requestId: input.requestId };
    (self as any).postMessage(output);
  }
};
