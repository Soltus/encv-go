/**
 * 状态机工具函数 — 终态保护 + 状态转换校验（shared 版）
 *
 * 设计目标：
 * - 纯函数，无副作用，可被任意 composable / 测试直接复用
 * - 配合 useTaskEventBridge 的 4 件套 WS 事件订阅，提供状态机校验底座
 * - 对齐 automation-workflow 规则 §四 的状态转换矩阵
 *
 * 本文件是状态机「官方」工具函数，服务于 useTaskEventBridge + useWorkflowTaskService。
 *
 * 严格度可配置（用户拍板：默认宽松，允许配置为严格）：
 * - 默认宽松 `VALID_TRANSITIONS`（对齐 automation-workflow 规则 §四，允许 pending → queued/running/skipped
 *   等跳级转换，以兼容后端可能跳发事件：task:created 后直接 task:completed，中间不发 task:update）。
 * - 传 `{ strict: true }` 走 `VALID_TRANSITIONS_STRICT`（对齐旧 camelCase 版语义，pending 只能 → submitted/cancelled）。
 */

import {
  isTerminalStep,
  type JobConclusion,
  type JobRun,
  type StepRun,
  type StepStatus,
  type WorkflowStatus,
} from "@encv/shared-components/lib/workflow/types";

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
 * 默认宽松：允许跳级转换（pending → queued / running / skipped 等），以兼容后端事件丢失场景。
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

/**
 * 严格版转换表（对齐旧 camelCase stateMachine.ts 语义）。
 *
 * 与 VALID_TRANSITIONS 的差异：禁止跳级——
 *   pending 只能 → submitted / cancelled（不能直接 queued / running / skipped）；
 *   submitted 只能 → queued / cancelled / skipped（不能直接 running）。
 * 其余中间态仅允许相邻推进，终态规则与宽松版一致。
 *
 * 通过 `validateTransition(from, to, { strict: true })` 启用。
 */
export const VALID_TRANSITIONS_STRICT: Record<StepStatus, Set<StepStatus>> = {
  pending: new Set<StepStatus>(["submitted", "cancelled"]),
  submitted: new Set<StepStatus>(["queued", "cancelled", "skipped"]),
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
 * 状态机校验：基于 VALID_TRANSITIONS（默认宽松）或 VALID_TRANSITIONS_STRICT（严格）判断 from → to 是否合法。
 *
 * 规则：
 * - from === to → true（同状态幂等，允许）
 * - from 为终态 → false（终态不允许转出，与 to 是否等于 from 无关；
 *   同状态情况已在上一条规则中返回 true）
 * - 否则查对应转换表[from].has(to)
 *
 * @param from 当前状态
 * @param to   目标状态
 * @param opts.strict 为 true 时走 VALID_TRANSITIONS_STRICT（禁止跳级，对齐旧 camelCase 版语义）；缺省 false（宽松）
 * @returns true 表示允许转换，false 表示非法
 *
 * @example
 * ```ts
 * validateTransition('pending', 'running')              // true（宽松，跳级允许）
 * validateTransition('pending', 'running', {strict:true}) // false（严格，禁止跳级）
 * validateTransition('running', 'success')              // true
 * validateTransition('success', 'running')              // false（终态保护）
 * validateTransition('success', 'success')              // true（同状态幂等）
 * validateTransition('pending', 'success')              // false（pending 不能直接到终态）
 * ```
 */
export function validateTransition(from: StepStatus, to: StepStatus, opts?: { strict?: boolean }): boolean {
  // 同状态幂等：允许
  if (from === to) return true;

  // 终态不能转出（到任何其他状态都拒绝）
  if (isTerminalStep(from)) return false;

  // 查表（严格 / 宽松）
  const table = opts?.strict ? VALID_TRANSITIONS_STRICT : VALID_TRANSITIONS;
  const allowed = table[from];
  return allowed ? allowed.has(to) : false;
}

/**
 * 执行状态转换：合法时返回目标状态，非法时抛错。
 *
 * 与 validateTransition 共用同一套转换表与 strict 选项；适用于「必须成功否则中断」的场景
 * （如调度器推进步骤、调用方已假定转换合法）。
 *
 * @param from 当前状态
 * @param to   目标状态
 * @param opts.strict 同 validateTransition
 * @throws 当 validateTransition(from, to, opts) 为 false
 *
 * @example
 * ```ts
 * transition('pending', 'submitted')          // 'submitted'
 * transition('running', 'success')            // 'success'
 * transition('success', 'running')             // 抛错（终态保护）
 * ```
 */
export function transition(from: StepStatus, to: StepStatus, opts?: { strict?: boolean }): StepStatus {
  if (!validateTransition(from, to, opts)) {
    throw new Error(`Illegal state transition: ${from} -> ${to}`);
  }
  return to;
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
