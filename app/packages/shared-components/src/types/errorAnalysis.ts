/**
 * 错误分析纯类型（shared 版）
 *
 * 来源：encv-mobile/src/composables/useErrorAnalyzer.ts 的类型定义部分。
 * 这些是零运行时依赖的纯类型（string union + interface），被 shared 的
 * lib/workflow/types.ts（StepRun.errorAnalysis）与自动化报告组件消费。
 *
 * ⚠️ 迁移期临时约定（Phase 3）：
 *   useErrorAnalyzer 的运行时规则仍在 encv-mobile（尚未提升），其类型定义
 *   与本文件当前是「结构一致的双份」。待后续提升 useErrorAnalyzer 时，让
 *   encv-mobile 版 re-export 本文件类型，消除重复。结构必须与
 *   useErrorAnalyzer.ts 保持一致，改一端需同步另一端。
 */

export type ErrorPhase = "submission" | "network" | "http" | "backend" | "plugin" | "unknown";
export type ErrorSeverity = "info" | "warning" | "error";

export type ErrorCategory =
  | "network"
  | "auth"
  | "not_found"
  | "server_error"
  | "unsupported_version"
  | "unsupported_cipher"
  | "unsupported_compression"
  | "wrong_password"
  | "file_not_found"
  | "permission_denied"
  | "oom"
  | "timeout"
  | "plugin_crash"
  | "invalid_request"
  | "unknown";

export interface ErrorChainStep {
  phase: ErrorPhase;
  title: string;
  detail: string;
  severity: ErrorSeverity;
}

export interface FixSuggestion {
  title: string;
  detail: string;
  codeHint?: string;
  docUrl?: string;
}

export interface ErrorAnalysis {
  category: ErrorCategory;
  phase: ErrorPhase;
  summary: string;
  technicalExplanation: string;
  chain: ErrorChainStep[];
  fixes: FixSuggestion[];
}
