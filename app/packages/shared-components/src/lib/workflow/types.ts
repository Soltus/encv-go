/**
 * 共享工作流类型（供 shared-components 使用）
 *
 * 注意：完整类型定义在 encv-mobile/src/lib/workflow/types.ts
 * 这里只导出 shared-components 中组件需要的最小类型子集。
 * 若需新增字段，请同步更新两端定义。
 */

export type StepStatus =
  | "pending"
  | "submitted"
  | "queued"
  | "running"
  | "cancelling"
  | "success"
  | "failure"
  | "cancelled"
  | "skipped"
  | "timed_out";

export type JobStatus = StepStatus;

export const Phase = {
  Created: "created",
  Analyzing: "analyzing",
  Initializing: "initializing",
  Preprocessing: "preprocessing",
  Encrypting: "encrypting",
  Decrypting: "decrypting",
  Packing: "packing",
  Verifying: "verifying",
  Completed: "completed",
} as const;

export type Phase = (typeof Phase)[keyof typeof Phase];

export interface UnifiedTimelineEntry {
  id: string;
  phase: Phase;
  icon?: string;
  label: string;
  meta?: string;
  time?: string;
  duration?: string;
  progress?: number;
  speed?: string;
  eta?: string;
  status: StepStatus;
  isCurrent?: boolean;
  isHighlight?: boolean;
  hasExpandableDetail?: boolean;
  expandDetail?: {
    startedAt?: string;
    completedAt?: string;
    duration?: string;
    outputPath?: string;
    error?: string;
    extra?: Record<string, string>;
    sourcePath?: string;
    phaseDetail?: string;
    cryptoSummary?: string;
  };
}
