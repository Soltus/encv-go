/**
 * 状态机 — 验证状态转换合法性 + 计算 Job 结论
 *
 * 设计目标：
 * - 纯函数，无副作用
 * - 可在 composable 和单元测试中直接使用
 * - 对齐 GitHub Actions 作业生命周期
 */

import type { JobConclusion, JobRun, StepRun, StepStatus, WorkflowStatus } from "@encv/shared-components/lib/workflow/types";
import { isTerminalStep } from "@encv/shared-components/lib/workflow/types";

// ==================== 合法转换表 ====================

/** 每个 status 允许转入的下一状态集合 */
const VALID_TRANSITIONS: Record<StepStatus, Set<StepStatus>> = {
  pending: new Set(["submitted", "cancelled"]),
  submitted: new Set(["queued", "cancelled"]),
  queued: new Set(["running", "cancelled"]),
  running: new Set(["cancelling", "success", "failure", "cancelled", "timed_out"]),
  cancelling: new Set(["cancelled", "failure", "success"]), // 取消中 → 已取消/成功/失败
  success: new Set(), // 终态
  failure: new Set(), // 终态
  cancelled: new Set(), // 终态
  skipped: new Set(), // 终态
  timed_out: new Set(), // 终态
};

/**
 * 验证状态转换是否合法。
 * 返回 true 表示允许转换，false 表示非法。
 */
export function canTransition(from: StepStatus, to: StepStatus): boolean {
  // 终态不能转出
  if (isTerminalStep(from) && from !== to) return false;
  const allowed = VALID_TRANSITIONS[from];
  return allowed ? allowed.has(to) : false;
}

/**
 * 执行状态转换（带验证）。
 * 如果转换合法返回新状态，否则抛出错误。
 */
export function transition(current: StepStatus, target: StepStatus): StepStatus {
  if (!canTransition(current, target)) {
    throw new Error(`Invalid state transition: ${current} → ${target}`);
  }
  return target;
}

// ==================== Job 结论计算 ====================

/**
 * 根据 Job 下所有 Steps 的最终状态计算 Job 的结论（conclusion）。
 *
 * @param steps - 步骤运行实例
 * @param continueOnErrorMap - stepDefId → 是否允许继续（来自 StepDefinition）
 */
export function computeJobConclusion(steps: StepRun[], continueOnErrorMap?: Map<string, boolean>): JobConclusion {
  if (steps.length === 0) return "skipped";

  const hasCancelled = steps.some(s => s.status === "cancelled");
  if (hasCancelled) return "cancelled";

  const failures = steps.filter(s => s.status === "failure" || s.status === "timed_out");
  const nonContinuedFailures = failures.filter(s => {
    return !(continueOnErrorMap?.get(s.stepDefId) ?? false);
  });

  // 有不可忽略的失败 → failure
  if (nonContinuedFailures.length > 0) return "failure";
  // 有可忽略的失败 + 无成功 → 仍为 failure
  if (failures.length > 0 && !steps.some(s => s.status === "success")) return "failure";
  // 全部 skipped
  if (steps.every(s => s.status === "skipped")) return "skipped";
  // 其余情况（有 success 或全 skipped）→ success
  return "success";
}

// ==================== Workflow 状态推断 ====================

/**
 * 根据所有 Jobs 的结论推断 WorkflowRun 的最终状态。
 */
export function inferWorkflowStatus(jobs: JobRun[]): WorkflowStatus {
  if (jobs.length === 0) return "pending";
  if (jobs.some(j => j.status === "running" || j.status === "queued" || j.status === "pending")) {
    return "running";
  }
  if (jobs.some(j => j.conclusion === "failure") || jobs.some(j => j.conclusion === "cancelled")) {
    return "failure";
  }
  return "success";
}
