/**
 * 活跃态归一化
 *
 * 参考 codex-web-repo/apps/web/src/app/appServerRealtimeReducer.ts:57-89
 * 的 `compactStatus` / `isActiveStatus` / `readTurnStatus` 设计模式。
 *
 * 设计要点：
 * 1. 归一化 (compact) —— 去除大小写、空格、下划线、连字符的差异
 * 2. 把后端可能下发的多种状态字符串归到 4 个语义集合（active / completed / failed / interrupted）
 * 3. 对象输入支持从 `type / status / state / kind` 四个常见字段抽取
 * 4. 兜底 'unknown' —— 永不抛错，方便 UI 直接走 `v-if="status === 'active'"` 分支
 */

export type ActiveStatus = "active" | "completed" | "failed" | "interrupted" | "unknown";

const ACTIVE_VALUES = ["active", "inprogress", "running", "editing", "thinking", "in_progress", "streaming"];

const COMPLETED_VALUES = ["completed", "complete", "done", "success", "succeeded"];

const FAILED_VALUES = ["failed", "failure", "error"];

const INTERRUPTED_VALUES = ["interrupted", "interrupt", "canceled", "cancelled"];

/**
 * 把任意输入归一化为小写且去除 [空格 / 下划线 / 连字符] 的字符串。
 * 对象输入会尝试 `type / status / state / kind` 四个字段。
 * 任何非字符串/非对象输入都返回空串 —— 永不抛错。
 */
export function compactStatus(value: unknown): string {
  if (typeof value === "string") {
    return value.toLowerCase().replace(/[\s_-]+/g, "");
  }
  if (value && typeof value === "object") {
    const r = value as Record<string, unknown>;
    return compactStatus(r.type ?? r.status ?? r.state ?? r.kind);
  }
  return "";
}

/**
 * 判断输入是否属于「正在执行」语义。
 * 用于 UI 决定是否渲染 spinner / 「正在… 文案」。
 */
export function isActiveStatus(value: unknown): boolean {
  return ACTIVE_VALUES.includes(compactStatus(value));
}

/**
 * 把任意状态输入映射到 4 个语义集合 + unknown 兜底。
 * UI 直接 `switch (readTurnStatus(x))` 即可，无需关心后端字段名差异。
 */
export function readTurnStatus(value: unknown): ActiveStatus {
  const s = compactStatus(value);
  if (ACTIVE_VALUES.includes(s)) return "active";
  if (COMPLETED_VALUES.includes(s)) return "completed";
  if (FAILED_VALUES.includes(s)) return "failed";
  if (INTERRUPTED_VALUES.includes(s)) return "interrupted";
  return "unknown";
}
