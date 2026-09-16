/**
 * 共享工作流类型（供 shared-components 使用）
 *
 * 注意：完整类型定义在 encv-mobile/src/lib/workflow/types.ts
 * 这里只导出 shared-components 中组件需要的最小类型子集。
 * 若需新增字段，请同步更新两端定义。
 */

import type { ErrorAnalysis } from "@encv/shared-components/types/errorAnalysis";
import type { TaskType } from "@encv/shared-components/types/task";

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

/** 终态集合（与 encv-mobile/src/lib/workflow/types.ts 保持一致） */
const TERMINAL_STEP_STATUS: Set<StepStatus> = new Set(["success", "failure", "cancelled", "skipped", "timed_out"]);

/** 判断某状态是否为终态（供 state-machine 终态保护 / 校验使用） */
export function isTerminalStep(s: StepStatus): boolean {
  return TERMINAL_STEP_STATUS.has(s);
}

/**
 * 步骤运行实例（每个对应一个 EncvTask）。
 * 结构与 encv-mobile/src/lib/workflow/types.ts 的 StepRun 保持一致（结构等价即可）。
 */
export interface StepRun {
  id: string;
  stepDefId: string;
  status: StepStatus;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  taskId?: string;
  error?: string;
  errorAnalysis?: ErrorAnalysis;
  output?: Record<string, unknown>;
  /** matrix 展开时的变量快照 */
  matrixVars?: Record<string, string>;
  // 实时进度（由 task:update / task:progress WS 事件驱动）
  progress?: number;
  phase?: string;
  speed?: string;
  eta?: string;
}

// ==================== 触发者 / 状态枚举 ====================

export type TriggeredBy = "user" | "automation" | "ai_agent";

export type WorkflowStatus = "pending" | "running" | "success" | "failure" | "cancelled";

export type JobConclusion = "success" | "failure" | "skipped" | "cancelled";

// ==================== 条件表达式 ====================

export interface ConditionAlways {
  op: "always";
}
export interface ConditionSuccess {
  op: "success";
}
export interface ConditionFailure {
  op: "failure";
}
export interface ConditionEq {
  op: "eq";
  left: string;
  right: string;
}
export interface ConditionNeq {
  op: "neq";
  left: string;
  right: string;
}
export interface ConditionAnd {
  op: "and";
  children: ConditionExpr[];
}
export interface ConditionOr {
  op: "or";
  children: ConditionExpr[];
}
export interface ConditionNot {
  op: "not";
  child: ConditionExpr;
}

export type ConditionExpr =
  | ConditionAlways
  | ConditionSuccess
  | ConditionFailure
  | ConditionEq
  | ConditionNeq
  | ConditionAnd
  | ConditionOr
  | ConditionNot;

// ==================== 动作规格 ====================

export interface EncvTaskActionParams {
  sourcePath?: string;
  targetPath?: string;
  password?: string;
  version?: number;
  cipherMode?: number;
  compressionMode?: "none" | "zstd";
  secondaryPassword?: string;
  extraFields?: Record<string, string>;
}

export interface EncvTaskActionSpec {
  type: "encv_task";
  taskType: TaskType;
  pluginName: string;
  params: EncvTaskActionParams;
}

export interface ShellActionSpec {
  type: "shell";
  command: string;
}
export interface HttpRequestActionSpec {
  type: "http_request";
  method: string;
  url: string;
  body?: Record<string, unknown>;
}

export type ActionSpec = EncvTaskActionSpec | ShellActionSpec | HttpRequestActionSpec;

// ==================== 策略 ====================

export interface MatrixStrategy {
  type: "matrix";
  axes: Record<string, string[]>;
}
export interface ParallelStrategy {
  type: "parallel";
  max: number;
}
export interface SequentialStrategy {
  type: "sequential";
}

export type JobStrategy = MatrixStrategy | ParallelStrategy | SequentialStrategy;

export interface ConcurrencyMaxParallel {
  maxParallel: number;
}
export interface ConcurrencyGroupExclusive {
  group: string;
  cancelInProgress: boolean;
}

export type ConcurrencyConfig = ConcurrencyMaxParallel | ConcurrencyGroupExclusive;

// ==================== 静态定义 ====================

export interface StepDefinition {
  id: string;
  name: string;
  action: ActionSpec;
  if?: ConditionExpr;
  continueOnError?: boolean;
  timeoutSeconds?: number;
}

export interface JobDefinition {
  id: string;
  name: string;
  needs?: string[];
  if?: ConditionExpr;
  strategy?: JobStrategy;
  timeoutMinutes?: number;
  steps: StepDefinition[];
}

export interface WorkflowDefinition {
  id: string;
  name: string;
  description?: string;
  createdAt: string;
  updatedAt: string;
  trigger: "manual" | "on_event" | "schedule";
  env?: Record<string, string>;
  concurrency?: ConcurrencyConfig;
  jobs: JobDefinition[];
  builtin?: boolean;
}

// ==================== 运行时实例 ====================

export interface JobRun {
  id: string;
  jobDefId: string;
  status: JobStatus;
  conclusion?: JobConclusion;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  steps: StepRun[];
  matrixVars?: Record<string, string>;
}

export interface WorkflowRun {
  id: string;
  workflowDefId: string;
  status: WorkflowStatus;
  triggeredBy: TriggeredBy;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  jobs: JobRun[];
}

// ==================== 存储键名 ====================

export const WORKFLOW_STORE_KEY = "encv-workflow-definitions";

// ==================== UnifiedRunRecord 持久化接口 ====================

export interface UnifiedRunRecord {
  id: string;
  startedAt: string;
  completedAt?: string;
  totalCases: number;
  passed: number;
  failed: number;
  skipped: number;
  results: Array<{
    caseId: string;
    status: "success" | "failure" | "skipped";
    error?: string;
    duration?: string;
  }>;
  workflowRun?: WorkflowRun;
}

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

/** 全部 Phase 枚举值（只读数组，用于运行时校验 / 遍历渲染） */
export const ALL_PHASES: readonly Phase[] = Object.freeze([
  Phase.Created,
  Phase.Analyzing,
  Phase.Initializing,
  Phase.Preprocessing,
  Phase.Encrypting,
  Phase.Decrypting,
  Phase.Packing,
  Phase.Verifying,
  Phase.Completed,
]);

/** 类型守卫：判断 unknown 是否为合法 Phase 枚举值 */
export function isPhase(value: unknown): value is Phase {
  return typeof value === "string" && ALL_PHASES.includes(value as Phase);
}

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

/** 类型守卫：判断 unknown 是否为合法 UnifiedTimelineEntry（必须含 id / label / phase / status 字符串） */
export function isUnifiedTimelineEntry(value: unknown): value is UnifiedTimelineEntry {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return typeof v.id === "string" && typeof v.label === "string" && typeof v.phase === "string" && typeof v.status === "string";
}

// ==================== UnifiedTreeNode 通用树节点 ====================

/**
 * 通用树节点接口（统一 TreeView / 测试报告树 / 任务详情树的数据契约）
 *
 * 设计目标：让 TreeView 不再绑定特定业务字段（stepName / jobDisplayName 等），
 * 而是接收 UnifiedTreeNode[]，由调用方负责从 StepRun / JobRun / MockGenLogEntry
 * 等领域模型转换为本接口。
 */
export interface UnifiedTreeNode {
  id: string;
  label: string;
  status: StepStatus;
  progress?: number;
  phase?: Phase;
  speed?: string;
  eta?: string;
  duration?: string;
  icon?: string;
  meta?: string;
  errorHint?: string;
  children?: UnifiedTreeNode[];
  /** 父组件可声明的 slot 名称列表，决定展开时渲染哪些详情区块 */
  detailSlots?: string[];
}

/** 类型守卫：判断 unknown 是否为合法 UnifiedTreeNode（必须含 id / label / status 字符串） */
export function isUnifiedTreeNode(value: unknown): value is UnifiedTreeNode {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return typeof v.id === "string" && typeof v.label === "string" && typeof v.status === "string";
}

// ==================== 自动化测试报告类型 ====================

/**
 * 测试用例规格（单个测试用例的静态定义）
 *
 * 供 TestCaseFile.vue 等报告 UI 组件使用。
 */
export interface TestCaseSpec {
  id: string;
  taskType: TaskType;
  pluginName: string;
  sourcePath: string;
  version: number;
  cipherMode?: number;
  compressionMode?: "none" | "zstd";
  expectedBehavior: "success" | "might-fail";
}

/**
 * 测试用例执行结果（单个测试用例的运行时状态）
 *
 * 供 TestCaseFile.vue / FilterChips.vue 等报告 UI 组件使用。
 */
export interface TestCaseResult {
  spec: TestCaseSpec;
  status: "pending" | "running" | "passed" | "failed" | "skipped";
  taskId?: string;
  error?: string;
  durationMs?: number;
  /** 错误分析（仅 status === 'failed' 时有值） */
  errorAnalysis?: ErrorAnalysis;
  /** 提交时的快照（sourcePath, version, cipher, compression） */
  submittedSourcePath?: string;
  submittedAt?: string;
}

// Re-export from sub-modules for convenience
export type { MatrixBinding } from "./matrixExpander";
