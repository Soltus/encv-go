/**
 * taskViewComputeCore — 视图计算的纯函数核心（无 Worker / 无 DOM / 无响应式）
 *
 * 2026-07-12 Phase 3：从 `app/encv-mobile/src/workers/taskViewCompute.worker.ts`
 * 抽离的纯计算部分。原 worker 文件与 `useTaskViewCompute` 的 `computeSync`
 * 各内联了一份相同逻辑（两份重复），现统一到此模块：
 *   - worker 文件改为 `import { buildDisplayedItems } from "@encv/shared-components/lib/taskViewComputeCore"`（壳不变）
 *   - shared 的 `useTaskViewCompute` 用本模块作同步降级，不再内联
 *
 * 本模块只做纯计算（filter / sort / group / 构建异构列表），不持有响应式
 * 状态、不碰 i18n（date section 返回 dateKey，由主线程映射 label）、
 * 不实例化 Worker（Worker 实例化是应用层责任，经 DI 注入）。
 */
import type { EncvTask } from "@encv/shared-components/types/task";

// ============ 类型定义（与主线程 / worker 共享） ============
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

// ============ 纯计算函数 ============

export function getTaskDisplayName(task: EncvTask): string {
  if (task.targetPath) return task.targetPath.split("/").pop() || task.targetPath;
  if (task.sourcePath) return task.sourcePath.split("/").pop() || task.sourcePath;
  return task.id.slice(0, 8);
}

/**
 * 任务搜索命中判定（name/plugin/error/id 四字段 includes）的单一真源。
 * 消除 filterTasks / computeGroupCounters / useTasksView.computeGroupHit 三处逐字重复。
 * 空查询返回 true（视为全部命中）。
 */
export function matchTaskSearch(task: EncvTask, q: string): boolean {
  const query = (q ?? "").trim().toLowerCase();
  if (query.length === 0) return true;
  const name = getTaskDisplayName(task).toLowerCase();
  const plugin = (task.pluginName || "").toLowerCase();
  const error = (task.error || "").toLowerCase();
  const id = task.id.toLowerCase();
  return name.includes(query) || plugin.includes(query) || error.includes(query) || id.includes(query);
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
 * 过滤 tasks（store / worker 共用的单一真源）。
 * `backendSearchResultIds` 非空时（后端向量搜索激活），按 id 集合过滤而非本地名字匹配。
 */
export function filterTasks(
  tasks: EncvTask[],
  filters: {
    searchQuery: string;
    filterPlugins: string[];
    filterTypes: string[];
    filterStatuses: string[];
    filterTriggeredBy: string[];
    filterDateRange: { from?: string; to?: string };
    backendSearchResultIds?: Set<string> | null;
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
  const backendIds = filters.backendSearchResultIds;
  const useBackendSearch = hasSearch && backendIds !== null && backendIds !== undefined;
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
      if (useBackendSearch) {
        if (!backendIds!.has(t.id)) continue;
      } else {
        if (!matchTaskSearch(t, filters.searchQuery)) continue;
      }
    }
    out.push(t);
  }
  return out;
}

/**
 * 排序 tasks（store / worker 共用）。
 */
export function sortTasks(tasks: EncvTask[], sortBy: "activity" | "created"): EncvTask[] {
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
 * 按 runId 聚合（store / worker 共用）。
 */
export function groupByRunId(
  sortedTasks: EncvTask[],
  pinnedRunIds: ReadonlySet<string> | string[]
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
  const pinnedSet = pinnedRunIds instanceof Set ? pinnedRunIds : new Set(pinnedRunIds);
  result.sort((a, b) => {
    const aPinned = pinnedSet.has(a.runId);
    const bPinned = pinnedSet.has(b.runId);
    if (aPinned !== bPinned) return aPinned ? -1 : 1;
    return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime();
  });
  return result;
}

/**
 * 计算 group 的 counters（store / useTasksList 共用）。
 */
export function computeGroupCounters(
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
      if (matchTaskSearch(tk, filters.searchQuery)) {
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
 * 计算 group 在「所有激活筛选」下的命中数（plugin/type/status/date/search 交集）。
 * useTasksView.computeGroupHit 的单一真源（原本在 composable 内联一份等价实现）。
 * search 命中判定复用 matchTaskSearch。
 */
export function countGroupHit(
  tasks: EncvTask[],
  filters: {
    searchQuery: string;
    filterPlugins: string[];
    filterTypes: string[];
    filterStatuses: string[];
    filterDateRange: { from?: string; to?: string };
  }
): number {
  const q = (filters.searchQuery ?? "").trim().toLowerCase();
  const fromTs = filters.filterDateRange?.from;
  const toTs = filters.filterDateRange?.to;
  const hasSearch = q.length > 0;
  const hasDate = !!fromTs || !!toTs;
  let hit = 0;
  for (const t of tasks) {
    if (filters.filterPlugins.length > 0 && !filters.filterPlugins.includes(t.pluginName || "__unknown__")) continue;
    if (filters.filterTypes.length > 0 && !filters.filterTypes.includes(t.type)) continue;
    if (filters.filterStatuses.length > 0 && !filters.filterStatuses.includes(t.status)) continue;
    if (hasDate) {
      if (fromTs && t.createdAt < fromTs) continue;
      if (toTs && t.createdAt >= toTs) continue;
    }
    if (hasSearch && !matchTaskSearch(t, filters.searchQuery)) continue;
    hit++;
  }
  return hit;
}

/**
 * 构建 group displayData。
 */
export function buildGroupDisplayData(groupTasks: EncvTask[], startedAt: string): any {
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
 * 构建 displayedItems：
 *   - date section 返回 dateKey（不返回 label，主线程做 i18n 映射）
 *   - group item 包含 counters + displayData
 *   - task item 包含 task 引用
 */
export function buildDisplayedItems(input: ComputeInput): any[] {
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
      // computeGroupCounters
      const counters = computeGroupCounters(g.tasks, filters);
      if (!counters.hitAny) continue;
      // buildGroupDisplayData
      const displayData = buildGroupDisplayData(g.tasks, g.startedAt);
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
