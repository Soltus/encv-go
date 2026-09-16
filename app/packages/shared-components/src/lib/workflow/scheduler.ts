/**
 * DAG 调度器 — 拓扑排序 + 层级分组
 *
 * 基于 Kahn's algorithm 实现：
 * 1. 根据 JobDefinition.needs 构建依赖图
 * 2. 拓扑排序检测循环依赖
 * 3. 按层级分组（同层 Job 可并行执行）
 */

import type { JobDefinition } from "./types";

/**
 * 执行顺序结果：每一层是一组可并行执行的 job ID。
 */
export type ExecutionLayers = string[][];

/**
 * 解析 Job 之间的依赖关系，返回分层执行计划。
 *
 * @throws 如果检测到循环依赖则抛出错误
 */
export function resolveExecutionOrder(jobs: JobDefinition[]): ExecutionLayers {
  if (jobs.length === 0) return [];

  // 构建 id → index 映射
  const idToIdx = new Map<string, number>();
  for (let i = 0; i < jobs.length; i++) {
    idToIdx.set(jobs[i].id, i);
  }

  // 计算入度
  const inDegree = new Array(jobs.length).fill(0);
  const adj: number[][] = Array.from({ length: jobs.length }, () => []);

  for (const job of jobs) {
    const u = idToIdx.get(job.id)!;
    for (const needId of job.needs ?? []) {
      const v = idToIdx.get(needId);
      if (v === undefined) {
        console.warn(`[Scheduler] Job "${job.id}" depends on unknown job "${needId}", ignoring`);
        continue;
      }
      adj[v].push(u); // v → u（v 完成后 u 才能开始）
      inDegree[u]++;
    }
  }

  // Kahn's algorithm — BFS 分层
  const layers: ExecutionLayers = [];
  const queue: number[] = [];

  // 初始：入度为 0 的节点
  for (let i = 0; i < jobs.length; i++) {
    if (inDegree[i] === 0) queue.push(i);
  }

  let visited = 0;

  while (queue.length > 0) {
    // 当前层所有入度为 0 的节点
    const layerIds: string[] = [];
    const nextQueue: number[] = [];

    for (const u of queue) {
      layerIds.push(jobs[u].id);
      visited++;

      for (const v of adj[u]) {
        inDegree[v]--;
        if (inDegree[v] === 0) nextQueue.push(v);
      }
    }

    layers.push(layerIds);
    queue.splice(0, queue.length, ...nextQueue);
  }

  // 检测循环依赖
  if (visited !== jobs.length) {
    const cycleNodes = jobs.filter((_, i) => inDegree[i] > 0).map(j => j.id);
    throw new Error(`Circular dependency detected among jobs: ${cycleNodes.join(", ")}`);
  }

  return layers;
}

/**
 * 给定当前已完成的 job ID 集合，计算下一批可以启动的 job ID。
 */
export function getNextReadyJobs(jobs: JobDefinition[], completedJobIds: Set<string>): string[] {
  return jobs
    .filter(job => {
      // 已经完成的不重复启动
      if (completedJobIds.has(job.id)) return false;
      // 所有 needs 都已完成
      const needs = job.needs ?? [];
      return needs.every(nId => completedJobIds.has(nId));
    })
    .map(j => j.id);
}
