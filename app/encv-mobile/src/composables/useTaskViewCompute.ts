/**
 * useTaskViewCompute — 应用层薄壳（Phase 3 2026-07-12 提升）
 *
 * 纯逻辑已提升到 @encv/shared-components/composables/useTaskViewCompute
 * （含 DI workerFactory，shared 不实例化 Worker）。
 * 本壳负责 Worker 实例化（?worker 是 Vite 特性，必须在应用层），
 * 注入给 shared 版。下游（useTasksList）经 @/composables/useTaskViewCompute
 * 别名无感。
 */
import type { UseTaskViewCompute, UseTaskViewComputeOptions } from "@encv/shared-components/composables/useTaskViewCompute";
import { useTaskViewCompute as sharedUseTaskViewCompute } from "@encv/shared-components/composables/useTaskViewCompute";
import TaskViewComputeWorker from "@/workers/taskViewCompute.worker?worker";

export type { UseTaskViewCompute, UseTaskViewComputeOptions } from "@encv/shared-components/composables/useTaskViewCompute";

export function useTaskViewCompute(options: UseTaskViewComputeOptions): UseTaskViewCompute {
  return sharedUseTaskViewCompute({
    ...options,
    workerFactory: () => new TaskViewComputeWorker(),
  });
}
