/**
 * 条件表达式求值器 — 评估 GitHub Actions 风格的 if 条件
 *
 * 支持的操作：
 * - always / success / failure（上下文相关）
 * - eq / neq（字符串比较）
 * - and / or / not（逻辑组合）
 *
 * 求值上下文：
 *   context: 上一步骤的 status + 当前 matrix 变量
 */

import type { ConditionExpr, StepStatus } from "./types";

export interface EvalContext {
  /** 上一步骤的状态（用于 success/failure 判断） */
  previousStepStatus?: StepStatus;
  /** matrix 展开的变量绑定 */
  vars?: Record<string, string>;
}

/**
 * 求值条件表达式。返回 true 表示应该执行该步骤。
 */
export function evaluateCondition(expr: ConditionExpr, ctx: EvalContext): boolean {
  switch (expr.op) {
    case "always":
      return true;

    case "success":
      return ctx.previousStepStatus === "success";

    case "failure":
      return ctx.previousStepStatus !== undefined && ctx.previousStepStatus !== "success";

    case "eq": {
      const left = resolveTemplate(expr.left, ctx);
      const right = resolveTemplate(expr.right, ctx);
      return left === right;
    }

    case "neq": {
      const left = resolveTemplate(expr.left, ctx);
      const right = resolveTemplate(expr.right, ctx);
      return left !== right;
    }

    case "and":
      return expr.children.every(child => evaluateCondition(child, ctx));

    case "or":
      return expr.children.some(child => evaluateCondition(child, ctx));

    case "not":
      return !evaluateCondition(expr.child, ctx);

    default:
      // 未知操作符，默认执行（安全侧）
      console.warn(`[ConditionEval] Unknown op: ${(expr as any).op}, defaulting to true`);
      return true;
  }
}

/**
 * 简易模板变量替换。
 * 支持 ${{ varName }} 语法。
 */
function resolveTemplate(template: string, ctx: EvalContext): string {
  if (!template.includes("${{")) return template;
  return template.replace(/\$\{\{\s*(\w+)\s*\}\}/g, (_match, varName) => {
    return ctx.vars?.[varName] ?? template;
  });
}
