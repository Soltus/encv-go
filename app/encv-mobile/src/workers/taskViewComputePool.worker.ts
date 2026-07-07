/**
 * taskViewComputePool.worker — 分片计算 Worker（Worker 池中的每个 worker）
 *
 * 2026-07-08 架构优化：
 *   - Worker 池化：多个 Worker 并行处理任务分片，利用多核 CPU
 *   - 每个 Worker 只处理 tasks 数组的一个分片（slice），返回该分片的过滤/排序结果
 *   - 主线程负责聚合所有分片的结果，做最终的排序/分组/分页
 *
 * 设计：
 *   - Worker 池大小 = min(navigator.hardwareConcurrency - 1, 4)（至少 1 个）
 *   - 任务分片：tasks 数组均匀分割给 N 个 Worker
 *   - 每个 Worker 做：过滤 + 局部排序
 *   - 主线程做：归并排序 + 分组 + date section
 *
 * 通信协议：
 *   主线程 → Worker：{ type: 'compute', tasks, sliceStart, sliceEnd, ...filters, requestId }
 *   Worker → 主线程：{ type: 'result', filteredTasks, requestId, workerId }
 */
import type { EncvTask } from "@/api/encv";

export interface PoolComputeInput {
  type: "compute";
  tasks: EncvTask[];
  sliceStart: number;
  sliceEnd: number;
  sortBy: "activity" | "created";
  searchQuery: string;
  filterPlugins: string[];
  filterTypes: string[];
  filterStatuses: string[];
  filterTriggeredBy: string[];
  filterDateRange: { from?: string; to?: string };
  requestId: number;
  workerId: number;
}

export interface PoolComputeOutput {
  type: "result";
  filteredTasks: EncvTask[];
  requestId: number;
  workerId: number;
  durationMs: number;
}

function getTaskDisplayName(task: EncvTask): string {
  if (task.targetPath) return task.targetPath.split("/").pop() || task.targetPath;
  if (task.sourcePath) return task.sourcePath.split("/").pop() || task.sourcePath;
  return task.id.slice(0, 8);
}

function filterTasksSlice(
  tasks: EncvTask[],
  sliceStart: number,
  sliceEnd: number,
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
  const end = Math.min(sliceEnd, tasks.length);
  for (let i = sliceStart; i < end; i++) {
    const t = tasks[i]!;
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
      const name = getTaskDisplayName(t).toLowerCase();
      const plugin = (t.pluginName || "").toLowerCase();
      const error = (t.error || "").toLowerCase();
      const id = t.id.toLowerCase();
      if (!name.includes(q) && !plugin.includes(q) && !error.includes(q) && !id.includes(q)) continue;
    }
    out.push(t);
  }
  return out;
}

function sortTasksSlice(tasks: EncvTask[], sortBy: "activity" | "created"): EncvTask[] {
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

self.onmessage = (e: MessageEvent<PoolComputeInput>) => {
  const input = e.data;
  if (input?.type !== "compute") return;
  const startTime = performance.now();
  try {
    const filters = {
      searchQuery: input.searchQuery,
      filterPlugins: input.filterPlugins,
      filterTypes: input.filterTypes,
      filterStatuses: input.filterStatuses,
      filterTriggeredBy: input.filterTriggeredBy,
      filterDateRange: input.filterDateRange,
    };
    const filtered = filterTasksSlice(input.tasks, input.sliceStart, input.sliceEnd, filters);
    const sorted = sortTasksSlice(filtered, input.sortBy);
    const durationMs = performance.now() - startTime;
    const output: PoolComputeOutput = {
      type: "result",
      filteredTasks: sorted,
      requestId: input.requestId,
      workerId: input.workerId,
      durationMs,
    };
    (self as any).postMessage(output);
  } catch (err) {
    console.error(`[taskViewComputePool.worker #${input.workerId}] compute failed:`, err);
    const output: PoolComputeOutput = {
      type: "result",
      filteredTasks: [],
      requestId: input.requestId,
      workerId: input.workerId,
      durationMs: performance.now() - startTime,
    };
    (self as any).postMessage(output);
  }
};
