// TokenSnapshot TS 类型对齐 Go TokenTracker (internal/agent/token_tracker.go)。
//
// 设计要点：
//   - 所有字段都是 number（前端不做 null 检查）
//   - JSON 字段名与 Go 端对齐（snake_case → 直接走 JSON.stringify）
//   - 缺失的字段默认为 0（init snapshot）

/** 实时 token 用量快照（来自 SSE token_stats 事件）。 */
export interface TokenSnapshot {
  /** 当前流式速率（tokens/s） */
  tokensPerSecond: number;
  /** 思考 token 速率 */
  reasoningTokensPerSecond: number;
  /** 累计 prompt tokens */
  promptTokensTotal: number;
  /** 累计 completion tokens */
  completionTokensTotal: number;
  /** 累计缓存命中 tokens */
  cachedTokensTotal: number;
  /** 累计思考 tokens */
  reasoningTokensTotal: number;
  /** 当前请求 prompt tokens */
  promptTokensThisRequest: number;
  /** 当前请求 completion tokens */
  completionTokensThisRequest: number;
  /** 当前请求缓存命中 tokens */
  cachedTokensThisRequest: number;
  /** 0.0 - 1.0，per-request 命中率 */
  cacheHitRate: number;
  /** 当前请求占用 context tokens */
  contextUsed: number;
  /** 剩余 context tokens */
  contextRemaining: number;
  /** 0.0 - 1.0+ */
  contextUsagePercent: number;
  /** 累计费用 USD */
  estimatedCostUsd: number;
  /** 累计请求数 */
  requestCount: number;
  /** 平均延迟 ms */
  averageLatencyMs: number;
}

/** 构造空 snapshot（用于初始状态）。 */
export function emptyTokenSnapshot(): TokenSnapshot {
  return {
    tokensPerSecond: 0,
    reasoningTokensPerSecond: 0,
    promptTokensTotal: 0,
    completionTokensTotal: 0,
    cachedTokensTotal: 0,
    reasoningTokensTotal: 0,
    promptTokensThisRequest: 0,
    completionTokensThisRequest: 0,
    cachedTokensThisRequest: 0,
    cacheHitRate: 0,
    contextUsed: 0,
    contextRemaining: 1_000_000,
    contextUsagePercent: 0,
    estimatedCostUsd: 0,
    requestCount: 0,
    averageLatencyMs: 0,
  };
}

/** 5 级预警等级。 */
export type TokenWarningLevel = "ok" | "green" | "yellow" | "red" | "force";

/** 根据 contextUsagePercent 计算预警等级。 */
export function tokenWarningLevel(pct: number): TokenWarningLevel {
  if (pct >= 0.98) return "force";
  if (pct >= 0.95) return "red";
  if (pct >= 0.8) return "yellow";
  if (pct >= 0.3) return "green";
  return "ok";
}
