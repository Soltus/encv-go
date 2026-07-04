/**
 * Reasoning effort 归一化 + i18n
 *
 * 后端在 reasoning_delta 事件中可能附带 `reasoningEffort` 字段，
 * 取值不统一：
 *   - "low" / "minimal" / "small"
 *   - "medium" / "med" / "normal"
 *   - "high" / "large"
 *   - "xhigh" / "extra" / "extra max" / "max"
 *   - 缺省 / 未知 → 归到 "default"
 *
 * 归一化后用于查找 i18n 键：
 *   agent.reasoningEffort.{low|medium|high|xhigh|default}
 *
 * 设计参考 `composables/activeStatus.ts` 的 `compactStatus`：
 *   - 永远不抛错（undefined / null / 数字 / 布尔都回退 'default'）
 *   - 大小写不敏感
 *   - 去除空格 / 下划线 / 连字符 / 中划线
 */

export type ReasoningEffort = "low" | "medium" | "high" | "xhigh" | "default" | string;

/**
 * 把任意输入归一化为 5 个 effort bucket 之一 + 'default' 兜底。
 *
 * - 非字符串 / 空串 → 'default'
 * - 大小写不敏感：toLowerCase()
 * - 去除 [\s_-]+：把 "extra max" / "Extra-Max" / "extra_max" 都归一为 "extramax"
 */
export function normalizeReasoningEffort(value: unknown): ReasoningEffort {
  if (typeof value !== "string") return "default";
  const s = value.toLowerCase().replace(/[\s_-]+/g, "");
  if (["low", "minimal", "small"].includes(s)) return "low";
  if (["medium", "med", "normal"].includes(s)) return "medium";
  if (["high", "large"].includes(s)) return "high";
  if (["xhigh", "extra", "extramax", "max"].includes(s)) return "xhigh";
  return "default";
}

/**
 * 把归一化后的 effort 映射为 i18n 键。
 * ReasoningMessage 拿到这个键后交给 useI18n().t() 渲染。
 */
export function i18nKeyFor(effort: ReasoningEffort): string {
  return `agent.reasoningEffort.${effort}` as const;
}
