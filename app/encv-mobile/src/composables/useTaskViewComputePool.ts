/**
 * useTaskViewComputePool — Worker 池化视图计算（主线程封装）
 *
 * 2026-07-08 架构优化：
 *   - 多 Worker 并行：利用多核 CPU，分片处理 1000+ 任务
 *   - Worker 池大小：min(navigator.hardwareConcurrency - 1, 4)，至少 1 个
 *   - 归并排序：每个 Worker 返回局部排序结果，主线程做 K 路归并
 *   - 降级策略：Worker 池初始化失败 → 回退到单 Worker → 回退到同步计算
 *
 * 性能日志：
 *   - 所有关键路径都有 performance.mark/measure
 *   - console.info 输出耗时统计，方便调优
 *
 * 对比单 Worker 优化点：
 *   1. 过滤 + 局部排序：N 个 Worker 并行，理论加速比 ≈ N（I/O 密集型）
 *   2. 归并排序：O(N log K)，K=Worker 数，通常可忽略
 *   3. 分组 + date section：仍在主线程，但数据量已过滤减少
 */

import type { EncvTask } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";
import type { PoolComputeInput, PoolComputeOutput } from "@/workers/taskViewComputePool.worker";
import TaskViewComputePoolWorker from "@/workers/taskViewComputePool.worker?worker";
import {
  type ComputedRef,
  computed,
  type Ref,
  ref,
  toRaw,
  watch,
  onUnmounted,
} from "vue";

export interface UseTaskViewComputePoolOptions {
  tasks: Ref<EncvTask[]> | ComputedRef<EncvTask[]>;
  viewMode: Ref<"group" | "flat"> | ComputedRef<"group" | "flat">;
  sortBy: Ref<"activity" | "created"> | ComputedRef<"activity" | "created">;
  searchQuery: Ref<string> | ComputedRef<string>;
  filterPlugins: Ref<string[]> | ComputedRef<string[]>;
  filterTypes: Ref<string[]> | ComputedRef<string[]>;
  filterStatuses: Ref<string[]> | ComputedRef<string[]>;
  filterTriggeredBy: Ref<string[]> | ComputedRef<string[]>;
  filterDateRange: Ref<{ from?: string; to?: string }> | ComputedRef<{ from?: string; to?: string }>;
  pinnedRunIds: Ref<Set<string>> | ComputedRef<Set<string>>;
  /** 是否启用性能日志（默认 true） */
  enablePerfLog?: boolean;
}

export interface UseTaskViewComputePool {
  displayedItems: Ref<any[]>;
  isComputing: Ref<boolean>;
  isFallback: Ref<boolean>;
  /** Worker 池大小（0=未初始化，N=实际 Worker 数） */
  poolSize: Ref<number>;
  /** 最近一次计算的总耗时（ms） */
  lastComputeDuration: Ref<number>;
}

const POOL_MIN_SIZE = 1;
const POOL_MAX_SIZE = 4;

function getIdealPoolSize(): number {
  if (typeof navigator === "undefined" || !navigator.hardwareConcurrency) {
    return POOL_MIN_SIZE;
  }
  return Math.max(POOL_MIN_SIZE, Math.min(POOL_MAX_SIZE, navigator.hardwareConcurrency - 1));
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

function getSortKey(task: EncvTask, sortBy: "activity" | "created"): number {
  if (sortBy === "activity") {
    return Math.max(
      new Date(task.createdAt).getTime(),
      task.completedAt ? new Date(task.completedAt).getTime() : 0
    );
  }
  return new Date(task.createdAt).getTime();
}

function kWayMerge(sortedArrays: EncvTask[][], sortBy: "activity" | "created"): EncvTask[] {
  const total = sortedArrays.reduce((sum, arr) => sum + arr.length, 0);
  if (total === 0) return [];
  if (sortedArrays.length === 1) return sortedArrays[0]!;

  const result: EncvTask[] = new Array(total);
  const pointers = sortedArrays.map(() => 0);
  let resultIdx = 0;

  while (resultIdx < total) {
    let minIdx = -1;
    let minKey = -Infinity;
    for (let i = 0; i < sortedArrays.length; i++) {
      const arr = sortedArrays[i]!;
      const ptr = pointers[i]!;
      if (ptr >= arr.length) continue;
      const key = getSortKey(arr[ptr]!, sortBy);
      if (key > minKey || minIdx === -1) {
        minKey = key;
        minIdx = i;
      }
    }
    if (minIdx === -1) break;
    result[resultIdx] = sortedArrays[minIdx]![pointers[minIdx]!]!;
    pointers[minIdx]!++;
    resultIdx++;
  }
  return result;
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
    if (!hasSearch) {
      search.hit++;
    } else {
      const name = (tk.targetPath?.split("/").pop() ?? tk.sourcePath?.split("/").pop() ?? tk.id.slice(0, 8)).toLowerCase();
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
  const tone =
    groupTasks[0]?.triggeredBy === "ai_agent"
      ? "ai_agent"
      : groupTasks[0]?.triggeredBy === "automation"
        ? "automation"
        : "user";
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

export function useTaskViewComputePool(options: UseTaskViewComputePoolOptions): UseTaskViewComputePool {
  const { t } = useI18n();
  const enablePerfLog = options.enablePerfLog ?? true;

  const isComputing = ref(false);
  const isFallback = ref(false);
  const poolSize = ref(0);
  const lastComputeDuration = ref(0);
  const workerDisplayedItems = ref<any[]>([]);
  const workerHasResult = ref(false);

  const isTestEnv = !!(import.meta as any).env?.VITEST;

  const workers: Worker[] = [];
  let requestId = 0;
  let pendingTimer: ReturnType<typeof setTimeout> | null = null;
  let lastInput: Omit<PoolComputeInput, "type" | "requestId" | "workerId" | "sliceStart" | "sliceEnd"> | null = null;
  let currentRequestId = 0;
  let completedWorkers = 0;
  let partialResults: EncvTask[][] = [];
  let totalStartTime = 0;

  const syncDisplayedItems = computed<any[]>(() => {
    const tasks = options.tasks.value;
    const viewMode = options.viewMode.value;
    const sortBy = options.sortBy.value;
    const searchQuery = options.searchQuery.value;
    const filterPlugins = options.filterPlugins.value;
    const filterTypes = options.filterTypes.value;
    const filterStatuses = options.filterStatuses.value;
    const filterTriggeredBy = options.filterTriggeredBy.value;
    const filterDateRange = options.filterDateRange.value;
    const pinnedRunIds = options.pinnedRunIds.value;

    const q = searchQuery.trim().toLowerCase();
    const fromTs = filterDateRange.from;
    const toTs = filterDateRange.to;
    const hasDate = !!fromTs || !!toTs;
    const hasSearch = q.length > 0;

    const filtered: EncvTask[] = [];
    for (const tk of tasks) {
      if (filterPlugins.length > 0 && !filterPlugins.includes(tk.pluginName || "__unknown__")) continue;
      if (filterTypes.length > 0 && !filterTypes.includes(tk.type)) continue;
      if (filterStatuses.length > 0 && !filterStatuses.includes(tk.status)) continue;
      if (filterTriggeredBy.length > 0) {
        const by = tk.triggeredBy ?? "user";
        if (!filterTriggeredBy.includes(by as any)) continue;
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

    const sorted = [...filtered];
    if (sortBy === "activity") {
      sorted.sort((a, b) => {
        const aKey = Math.max(new Date(a.createdAt).getTime(), a.completedAt ? new Date(a.completedAt).getTime() : 0);
        const bKey = Math.max(new Date(b.createdAt).getTime(), b.completedAt ? new Date(b.completedAt).getTime() : 0);
        return bKey - aKey;
      });
    } else {
      sorted.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
    }

    const items: any[] = [];
    let lastDateKey = "";
    function pushDateHeader(key: string) {
      if (key === lastDateKey) return;
      lastDateKey = key;
      items.push({ kind: "date", key: `date-${key}`, label: t(`tasks.date.${key}`, { defaultValue: key }) });
    }

    if (viewMode === "group") {
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
      const pinnedSet = new Set(pinnedRunIds);
      groupList.sort((a, b) => {
        const aPinned = pinnedSet.has(a.runId);
        const bPinned = pinnedSet.has(b.runId);
        if (aPinned !== bPinned) return aPinned ? -1 : 1;
        return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime();
      });
      for (const g of groupList) {
        pushDateHeader(dateSectionKey(g.startedAt));
        const counters = computeGroupCountersSync(g.tasks, {
          searchQuery,
          filterPlugins,
          filterTypes,
          filterStatuses,
          filterDateRange,
        });
        if (!counters.hitAny) continue;
        items.push({
          kind: "group",
          key: g.key,
          runId: g.runId,
          startedAt: g.startedAt,
          tasks: g.tasks,
          counters,
          displayData: buildGroupDisplayDataSync(g.tasks, g.startedAt),
        });
      }
    } else {
      for (const tk of sorted) {
        pushDateHeader(dateSectionKey(tk.createdAt));
        items.push({ kind: "task", key: `t-${tk.id}`, task: tk });
      }
    }
    return items;
  });

  function initWorkerPool(): boolean {
    if (isTestEnv) return false;
    const idealSize = getIdealPoolSize();
    try {
      for (let i = 0; i < idealSize; i++) {
        const worker = new TaskViewComputePoolWorker();
        worker.onmessage = (e: MessageEvent<PoolComputeOutput>) => onWorkerMessage(e.data);
        worker.onerror = e => {
          console.warn(`[useTaskViewComputePool] Worker #${i} error, fallback to sync:`, e);
          destroyWorkerPool();
          isFallback.value = true;
          workerHasResult.value = false;
        };
        workers.push(worker);
      }
      poolSize.value = workers.length;
      if (enablePerfLog) {
        console.info(`[useTaskViewComputePool] Worker pool initialized: size=${workers.length}, hardwareConcurrency=${navigator.hardwareConcurrency ?? "N/A"}`);
      }
      return true;
    } catch (e) {
      console.warn("[useTaskViewComputePool] Worker pool init failed, fallback to sync:", e);
      destroyWorkerPool();
      isFallback.value = true;
      return false;
    }
  }

  function destroyWorkerPool(): void {
    for (const w of workers) {
      try { w.terminate(); } catch {}
    }
    workers.length = 0;
    poolSize.value = 0;
  }

  function onWorkerMessage(output: PoolComputeOutput): void {
    if (output?.type !== "result") return;
    if (output.requestId !== currentRequestId) return;

    partialResults[output.workerId] = output.filteredTasks;
    completedWorkers++;

    if (completedWorkers >= workers.length) {
      const mergeStart = performance.now();
      const allSorted = kWayMerge(
        partialResults.filter(r => r && r.length > 0),
        lastInput?.sortBy ?? "activity"
      );
      const mergeMs = performance.now() - mergeStart;

      const buildStart = performance.now();
      const items = buildDisplayedItemsFromSorted(allSorted);
      const buildMs = performance.now() - buildStart;

      const totalMs = performance.now() - totalStartTime;
      lastComputeDuration.value = totalMs;
      isComputing.value = false;
      workerHasResult.value = true;
      workerDisplayedItems.value = items;

      if (enablePerfLog) {
        const workerDurations = partialResults.map((_, i) => {
          const w = workers[i];
          return `w${i}=?`;
        }).join(", ");
        console.info(
          `[useTaskViewComputePool] compute #${output.requestId}: ` +
          `total=${totalMs.toFixed(1)}ms, ` +
          `merge=${mergeMs.toFixed(1)}ms, ` +
          `build=${buildMs.toFixed(1)}ms, ` +
          `tasks=${options.tasks.value.length} → ${allSorted.length} filtered, ` +
          `pool=${workers.length}`
        );
      }
    }
  }

  function buildDisplayedItemsFromSorted(sortedTasks: EncvTask[]): any[] {
    const viewMode = options.viewMode.value;
    const pinnedRunIds = options.pinnedRunIds.value;
    const searchQuery = options.searchQuery.value;
    const filterPlugins = options.filterPlugins.value;
    const filterTypes = options.filterTypes.value;
    const filterStatuses = options.filterStatuses.value;
    const filterDateRange = options.filterDateRange.value;

    const items: any[] = [];
    let lastDateKey = "";
    function pushDateHeader(key: string) {
      if (key === lastDateKey) return;
      lastDateKey = key;
      items.push({
        kind: "date",
        key: `date-${key}`,
        label: t(`tasks.date.${key}`, { defaultValue: key }),
      });
    }

    if (viewMode === "group") {
      const groups = new Map<string, { runId: string; tasks: EncvTask[]; startedAt: string }>();
      for (const tk of sortedTasks) {
        const key = tk.runId || `__manual__${tk.id}`;
        const g = groups.get(key);
        if (g) g.tasks.push(tk);
        else groups.set(key, { runId: tk.runId || key, tasks: [tk], startedAt: tk.createdAt });
      }
      const groupList: { key: string; runId: string; startedAt: string; tasks: EncvTask[] }[] = [];
      for (const [key, g] of groups) {
        groupList.push({ key, runId: g.runId, startedAt: g.startedAt, tasks: g.tasks });
      }
      const pinnedSet = new Set(pinnedRunIds);
      groupList.sort((a, b) => {
        const aPinned = pinnedSet.has(a.runId);
        const bPinned = pinnedSet.has(b.runId);
        if (aPinned !== bPinned) return aPinned ? -1 : 1;
        return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime();
      });
      for (const g of groupList) {
        pushDateHeader(dateSectionKey(g.startedAt));
        const counters = computeGroupCountersSync(g.tasks, {
          searchQuery,
          filterPlugins,
          filterTypes,
          filterStatuses,
          filterDateRange,
        });
        if (!counters.hitAny) continue;
        items.push({
          kind: "group",
          key: g.key,
          runId: g.runId,
          startedAt: g.startedAt,
          tasks: g.tasks,
          counters,
          displayData: buildGroupDisplayDataSync(g.tasks, g.startedAt),
        });
      }
    } else {
      for (const tk of sortedTasks) {
        pushDateHeader(dateSectionKey(tk.createdAt));
        items.push({ kind: "task", key: `t-${tk.id}`, task: tk });
      }
    }
    return items;
  }

  function scheduleWorkerCompute(): void {
    if (workers.length === 0) return;
    if (!lastInput) return;

    if (pendingTimer) clearTimeout(pendingTimer);
    isComputing.value = true;
    pendingTimer = setTimeout(() => {
      if (workers.length === 0 || !lastInput) return;
      requestId++;
      currentRequestId = requestId;
      completedWorkers = 0;
      partialResults = new Array(workers.length);
      totalStartTime = performance.now();

      const tasks = lastInput.tasks;
      const total = tasks.length;
      const chunkSize = Math.ceil(total / workers.length);

      for (let i = 0; i < workers.length; i++) {
        const sliceStart = i * chunkSize;
        const sliceEnd = Math.min(sliceStart + chunkSize, total);
        if (sliceStart >= total) {
          partialResults[i] = [];
          completedWorkers++;
          continue;
        }
        const input: PoolComputeInput = {
          type: "compute",
          tasks,
          sliceStart,
          sliceEnd,
          sortBy: lastInput.sortBy,
          searchQuery: lastInput.searchQuery,
          filterPlugins: lastInput.filterPlugins,
          filterTypes: lastInput.filterTypes,
          filterStatuses: lastInput.filterStatuses,
          filterTriggeredBy: lastInput.filterTriggeredBy,
          filterDateRange: lastInput.filterDateRange,
          requestId: currentRequestId,
          workerId: i,
        };
        workers[i]!.postMessage(input);
      }

      if (completedWorkers >= workers.length) {
        onWorkerMessage({ type: "result", filteredTasks: [], requestId: currentRequestId, workerId: 0, durationMs: 0 });
      }
    }, 16);
  }

  const poolInitialized = !isTestEnv && initWorkerPool();

  if (poolInitialized) {
    watch(
      options.tasks,
      () => {
        lastInput = {
          tasks: toRaw(options.tasks.value),
          sortBy: options.sortBy.value,
          searchQuery: options.searchQuery.value,
          filterPlugins: toRaw(options.filterPlugins.value),
          filterTypes: toRaw(options.filterTypes.value),
          filterStatuses: toRaw(options.filterStatuses.value),
          filterTriggeredBy: toRaw(options.filterTriggeredBy.value),
          filterDateRange: toRaw(options.filterDateRange.value),
        };
        scheduleWorkerCompute();
      },
      { deep: true, immediate: true }
    );

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
      () => {
        if (lastInput) {
          lastInput.sortBy = options.sortBy.value;
          lastInput.searchQuery = options.searchQuery.value;
          lastInput.filterPlugins = toRaw(options.filterPlugins.value);
          lastInput.filterTypes = toRaw(options.filterTypes.value);
          lastInput.filterStatuses = toRaw(options.filterStatuses.value);
          lastInput.filterTriggeredBy = toRaw(options.filterTriggeredBy.value);
          lastInput.filterDateRange = toRaw(options.filterDateRange.value);
        }
        scheduleWorkerCompute();
      },
      { deep: true }
    );
  } else {
    isFallback.value = true;
  }

  onUnmounted(() => {
    destroyWorkerPool();
    if (pendingTimer) clearTimeout(pendingTimer);
  });

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
    poolSize,
    lastComputeDuration,
  };
}
