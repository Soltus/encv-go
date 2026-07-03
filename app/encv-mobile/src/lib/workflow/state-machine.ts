/**
 * 状态机工具函数 — 终态保护 + 状态转换校验
 *
 * 设计目标：
 * - 纯函数，无副作用，可被任意 composable / 测试直接复用
 * - 配合 useTaskEventBridge 的 4 件套 WS 事件订阅，提供状态机校验底座
 * - 对齐 automation-workflow 规则 §四 的状态转换矩阵
 *
 * 与 stateMachine.ts（camelCase）的关系：
 *   stateMachine.ts 提供 canTransition / transition / computeJobConclusion / inferWorkflowStatus，
 *   主要服务于 useWorkflowEngine（Task 7/8 退役后弃用）。
 *   本文件（state-machine.ts，连字符）是 Task 3 引入的"官方"工具函数，
 *   服务于 useTaskEventBridge + useWorkflowTaskService（Task 4）。
 *
 * VALID_TRANSITIONS 与 stateMachine.ts 中的版本差异：
 *   stateMachine.ts 版本更严格（pending 只能 → submitted/cancelled）。
 *   本文件版本对齐 automation-workflow 规则 §四，允许 pending → queued/running/skipped
 *   等跳级转换，以兼容后端可能跳发事件（task:created 后直接 task:completed，
 *   中间不发 task:update）。
 */

import type { StepRun, StepStatus } from "./types";
import { isTerminalStep } from "./types";

// ==================== 合法转换表 ====================

/**
 * 每个 status 允许转入的下一状态集合。
 *
 * 对齐 automation-workflow 规则 §四：
 *   pending    -> submitted, queued, running, cancelled, skipped
 *   submitted  -> queued, running, cancelled, skipped
 *   queued     -> running, cancelling, cancelled, skipped
 *   running    -> cancelling, success, failure, cancelled, timed_out
 *   cancelling -> cancelled, failure, success
 *   终态之间不允许转换
 *
 * 相比 stateMachine.ts 中的同名常量，本表允许跳级转换
 * （pending → queued / running / skipped 等），以兼容后端事件丢失场景。
 */
export const VALID_TRANSITIONS: Record<StepStatus, Set<StepStatus>> = {
  pending: new Set<StepStatus>(["submitted", "queued", "running", "cancelled", "skipped"]),
  submitted: new Set<StepStatus>(["queued", "running", "cancelled", "skipped"]),
  queued: new Set<StepStatus>(["running", "cancelling", "cancelled", "skipped"]),
  running: new Set<StepStatus>(["cancelling", "success", "failure", "cancelled", "timed_out"]),
  cancelling: new Set<StepStatus>(["cancelled", "failure", "success"]),
  // 终态：不允许转出
  success: new Set<StepStatus>(),
  failure: new Set<StepStatus>(),
  cancelled: new Set<StepStatus>(),
  skipped: new Set<StepStatus>(),
  timed_out: new Set<StepStatus>(),
};

// ==================== 终态保护 ====================

/**
 * 终态保护：已终态的 StepRun 不被覆盖。
 *
 * 用于 useTaskEventBridge 的 task:update / task:progress 回调中，
 * 防止后端延迟到达的事件把已 success / failure / cancelled / skipped / timed_out
 * 的 step 状态降级回 running / queued 等中间态。
 *
 * 行为：
 * - current 为 null/undefined 或非终态 → 原样返回 update（保留 status 字段）
 * - current 已终态 → 剥离 update.status 字段，仅保留其他可更新字段
 *   （如 progress / phase / speed / eta 等元数据，这些字段在终态后仍可刷新）
 *
 * @param current 当前 StepRun（或类似带 status 字段的对象），可为 null
 * @param update  待合并的更新对象
 * @returns 过滤后的 update（调用方应使用 Object.assign(step, filtered) 合并）
 *
 * @example
 * ```ts
 * const step: StepRun = { id: 's1', stepDefId: 'd1', status: 'success' }
 * const update = applyTerminalGuard(step, { status: 'running', progress: 50 })
 * // update => { progress: 50 }  （status 被剥离）
 * Object.assign(step, update)
 * // step => { id: 's1', stepDefId: 'd1', status: 'success', progress: 50 }
 * ```
 */
export function applyTerminalGuard(
  current: Pick<StepRun, "status"> | null | undefined,
  update: Partial<StepRun> & { status?: StepStatus }
): Partial<StepRun> & { status?: StepStatus } {
  // current 缺失或非终态 → 不需要保护，原样返回 update
  if (!current || !isTerminalStep(current.status)) {
    return update;
  }

  // current 已终态 → 剥离 status 字段，保留其他可更新字段（progress / phase / speed / eta 等元数据）
  const { status: _ignored, ...rest } = update;
  return rest;
}

// ==================== 状态机校验 ====================

/**
 * 状态机校验：基于 VALID_TRANSITIONS 判断 from → to 是否合法。
 *
 * 规则：
 * - from === to → true（同状态幂等，允许）
 * - from 为终态 → false（终态不允许转出，与 to 是否等于 from 无关；
 *   同状态情况已在上一条规则中返回 true）
 * - 否则查 VALID_TRANSITIONS[from].has(to)
 *
 * @param from 当前状态
 * @param to   目标状态
 * @returns true 表示允许转换，false 表示非法
 *
 * @example
 * ```ts
 * validateTransition('pending', 'running')   // true（跳级允许）
 * validateTransition('running', 'success')   // true
 * validateTransition('success', 'running')   // false（终态保护）
 * validateTransition('success', 'success')   // true（同状态幂等）
 * validateTransition('pending', 'success')   // false（pending 不能直接到终态）
 * ```
 */
export function validateTransition(from: StepStatus, to: StepStatus): boolean {
  // 同状态幂等：允许
  if (from === to) return true;

  // 终态不能转出（到任何其他状态都拒绝）
  if (isTerminalStep(from)) return false;

  // 查表
  const allowed = VALID_TRANSITIONS[from];
  return allowed ? allowed.has(to) : false;
}
