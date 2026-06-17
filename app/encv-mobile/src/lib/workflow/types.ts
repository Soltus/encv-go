/**
 * DAG 工作流引擎 — 核心类型定义
 *
 * 层次结构：
 *   WorkflowDefinition (静态定义)
 *   └── WorkflowRun (一次运行实例)
 *       └── JobRun[] (作业运行实例)
 *           └── StepRun[] (步骤运行实例，每个对应一个 EncvTask)
 */

import type { TaskType } from '@/api/encv'
import type { TriggeredBy } from '@/composables/useTaskTrigger'
import type { ErrorAnalysis } from '@/composables/useErrorAnalyzer'

// ==================== 状态枚举 ====================

export type StepStatus =
  | 'pending'
  | 'submitted'
  | 'queued'
  | 'running'
  | 'cancelling'
  | 'success'
  | 'failure'
  | 'cancelled'
  | 'skipped'
  | 'timed_out'

export type JobStatus = StepStatus

export type WorkflowStatus = 'pending' | 'running' | 'success' | 'failure' | 'cancelled'

export type JobConclusion = 'success' | 'failure' | 'skipped' | 'cancelled'

/** 终态集合 */
const TERMINAL_STEP_STATUS: Set<StepStatus> = new Set([
  'success', 'failure', 'cancelled', 'skipped', 'timed_out',
])

export function isTerminalStep(s: StepStatus): boolean {
  return TERMINAL_STEP_STATUS.has(s)
}

// ==================== 条件表达式 ====================

export interface ConditionAlways {
  op: 'always'
}
export interface ConditionSuccess {
  op: 'success'
}
export interface ConditionFailure {
  op: 'failure'
}
export interface ConditionEq {
  op: 'eq'
  left: string
  right: string
}
export interface ConditionNeq {
  op: 'neq'
  left: string
  right: string
}
export interface ConditionAnd {
  op: 'and'
  children: ConditionExpr[]
}
export interface ConditionOr {
  op: 'or'
  children: ConditionExpr[]
}
export interface ConditionNot {
  op: 'not'
  child: ConditionExpr
}

export type ConditionExpr =
  | ConditionAlways
  | ConditionSuccess
  | ConditionFailure
  | ConditionEq
  | ConditionNeq
  | ConditionAnd
  | ConditionOr
  | ConditionNot

// ==================== 动作规格 ====================

export interface EncvTaskActionParams {
  sourcePath?: string
  targetPath?: string
  password?: string
  version?: number
  cipherMode?: number
  compressionMode?: 'none' | 'zstd'
  secondaryPassword?: string
  /** 🆕 2026-06-10：插件 ExtraFields 键值对（按 plugin.taskOptions 声明） */
  extraFields?: Record<string, string>
}

export interface EncvTaskActionSpec {
  type: 'encv_task'
  taskType: TaskType
  pluginName: string
  params: EncvTaskActionParams
}

// 未来扩展占位（MVP 不实现）
export interface ShellActionSpec {
  type: 'shell'
  command: string
}
export interface HttpRequestActionSpec {
  type: 'http_request'
  method: string
  url: string
  body?: Record<string, unknown>
}

export type ActionSpec =
  | EncvTaskActionSpec
  | ShellActionSpec
  | HttpRequestActionSpec

// ==================== 静态定义 ====================

export interface MatrixStrategy {
  type: 'matrix'
  axes: Record<string, string[]>
}
export interface ParallelStrategy {
  type: 'parallel'
  max: number
}
export interface SequentialStrategy {
  type: 'sequential'
}

export type JobStrategy = MatrixStrategy | ParallelStrategy | SequentialStrategy

export interface ConcurrencyMaxParallel {
  maxParallel: number
}
export interface ConcurrencyGroupExclusive {
  group: string
  cancelInProgress: boolean
}

export type ConcurrencyConfig = ConcurrencyMaxParallel | ConcurrencyGroupExclusive

export interface StepDefinition {
  id: string
  name: string
  action: ActionSpec
  if?: ConditionExpr
  continueOnError?: boolean
  timeoutSeconds?: number
}

export interface JobDefinition {
  id: string
  name: string
  needs?: string[]
  if?: ConditionExpr
  strategy?: JobStrategy
  timeoutMinutes?: number
  steps: StepDefinition[]
}

export interface WorkflowDefinition {
  id: string
  name: string
  description?: string
  createdAt: string
  updatedAt: string
  trigger: 'manual' | 'on_event' | 'schedule'
  env?: Record<string, string>
  concurrency?: ConcurrencyConfig
  jobs: JobDefinition[]
  /** 是否为内置模板（不可删除） */
  builtin?: boolean
}

// ==================== 运行时实例 ====================

export interface StepRun {
  id: string
  stepDefId: string
  status: StepStatus
  startedAt?: string
  completedAt?: string
  durationMs?: number
  taskId?: string
  error?: string
  errorAnalysis?: ErrorAnalysis
  output?: Record<string, unknown>
  /** matrix 展开时的变量快照 */
  matrixVars?: Record<string, string>
  // 🆕 2026-06-10：实时进度（由 task:update / task:progress WS 事件驱动）
  progress?: number
  phase?: string
  speed?: string
  eta?: string
}

export interface JobRun {
  id: string
  jobDefId: string
  status: JobStatus
  conclusion?: JobConclusion
  startedAt?: string
  completedAt?: string
  durationMs?: number
  steps: StepRun[]
  matrixVars?: Record<string, string>
}

export interface WorkflowRun {
  id: string
  workflowDefId: string
  status: WorkflowStatus
  triggeredBy: TriggeredBy
  createdAt: string
  startedAt?: string
  completedAt?: string
  durationMs?: number
  jobs: JobRun[]
}

// ==================== 存储键名 ====================

export const WORKFLOW_STORE_KEY = 'encv-workflow-definitions'
// 2026-06-18 spec unify-workflow-task-service：WORKFLOW_RUNS_KEY ('encv-workflow-runs') 已删除。
// 旧 runs 持久化由 useWorkflowTaskService 接管，新 key = 'encv_workflow_tasks_v1'（UnifiedRunRecord 格式）。
// useWorkflowStore 不再读写 runs，仅保留 definitions CRUD。

// Re-export from sub-modules for convenience
export type { MatrixBinding } from './matrixExpander'

// ==================== Phase 枚举（前后端同步） ====================

/**
 * 任务执行阶段枚举（与后端 Go 端 Phase 常量字符串值保持一致）
 *
 * 用于 UnifiedTreeNode / UnifiedTimelineEntry / StepRun.phase 等场景，
 * 替代之前裸字符串 phase 字段，避免前后端枚举值漂移。
 */
export enum Phase {
  Created = 'created',
  Analyzing = 'analyzing',
  Initializing = 'initializing',
  Preprocessing = 'preprocessing',
  Encrypting = 'encrypting',
  Decrypting = 'decrypting',
  Packing = 'packing',
  Verifying = 'verifying',
  Completed = 'completed',
}

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
])

/** 类型守卫：判断 unknown 是否为合法 Phase 枚举值 */
export function isPhase(value: unknown): value is Phase {
  return typeof value === 'string' && ALL_PHASES.includes(value as Phase)
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
  id: string
  label: string
  status: StepStatus
  progress?: number
  phase?: Phase
  speed?: string
  eta?: string
  duration?: string
  icon?: string
  meta?: string
  errorHint?: string
  children?: UnifiedTreeNode[]
  /** 父组件可声明的 slot 名称列表，决定展开时渲染哪些详情区块 */
  detailSlots?: string[]
}

/** 类型守卫：判断 unknown 是否为合法 UnifiedTreeNode（必须含 id / label / status 字符串） */
export function isUnifiedTreeNode(value: unknown): value is UnifiedTreeNode {
  if (!value || typeof value !== 'object') return false
  const v = value as Record<string, unknown>
  return typeof v.id === 'string'
    && typeof v.label === 'string'
    && typeof v.status === 'string'
}

// ==================== UnifiedTimelineEntry 通用时间线条目 ====================

/**
 * 通用时间线条目接口（统一 TaskTimeline / MockGenLogCard / 测试报告内联时间线的数据契约）
 *
 * 设计目标：让 UnifiedTimelineCard 组件接收统一结构，由调用方负责从
 * StepRun / MockGenLogEntry / phase 序列等转换为本接口。
 */
export interface UnifiedTimelineEntry {
  id: string
  phase: Phase
  icon?: string
  label: string
  /** 副标题（如文件名 / 编码器 / 插件名等，显示在 label 右侧） */
  meta?: string
  time?: string
  duration?: string
  progress?: number
  speed?: string
  eta?: string
  status: StepStatus
  isCurrent?: boolean
  isHighlight?: boolean
  hasExpandableDetail?: boolean
  /** 展开详情（卡片化渲染，不再是 label:value 网格） */
  expandDetail?: {
    startedAt?: string
    completedAt?: string
    duration?: string
    outputPath?: string
    error?: string
    extra?: Record<string, string>
  }
}

/** 类型守卫：判断 unknown 是否为合法 UnifiedTimelineEntry（必须含 id / label / phase / status 字符串） */
export function isUnifiedTimelineEntry(value: unknown): value is UnifiedTimelineEntry {
  if (!value || typeof value !== 'object') return false
  const v = value as Record<string, unknown>
  return typeof v.id === 'string'
    && typeof v.label === 'string'
    && typeof v.phase === 'string'
    && typeof v.status === 'string'
}

// ==================== UnifiedRunRecord 持久化接口 ====================

/**
 * 统一运行记录持久化接口（localStorage key: encv_workflow_tasks_v1）
 *
 * 设计目标：统一插件测试 / WebDAV 测试 / 通用工作流三套自动化测试的运行记录格式，
 * 替代 useAutomationTests 内的 PersistedRun 与 useWebDavAutomationTests 内的私有结构。
 */
export interface UnifiedRunRecord {
  id: string
  startedAt: string
  completedAt?: string
  totalCases: number
  passed: number
  failed: number
  skipped: number
  results: Array<{
    caseId: string
    status: 'success' | 'failure' | 'skipped'
    error?: string
    duration?: string
  }>
  /** 关联的 WorkflowRun 完整快照（用于历史回放 / 详情展开） */
  workflowRun?: WorkflowRun
}

// ==================== 自动化测试报告类型（迁移自 useAutomationTests） ====================

/**
 * 测试用例规格（单个测试用例的静态定义）
 *
 * 迁移自 useAutomationTests.ts，供 TestCaseFile.vue 等报告 UI 组件使用。
 * 新的测试用例生成逻辑使用 useTestCaseGeneration.generateCases() 返回 GeneratedTestCase，
 * 但报告 UI 仍需此类型展示 cipherMode / compressionMode 等历史字段。
 */
export interface TestCaseSpec {
  id: string
  taskType: TaskType
  pluginName: string
  sourcePath: string
  version: number
  cipherMode?: number
  compressionMode?: 'none' | 'zstd'
  expectedBehavior: 'success' | 'might-fail'
}

/**
 * 测试用例执行结果（单个测试用例的运行时状态）
 *
 * 迁移自 useAutomationTests.ts，供 TestCaseFile.vue / FilterChips.vue 等报告 UI 组件使用。
 */
export interface TestCaseResult {
  spec: TestCaseSpec
  status: 'pending' | 'running' | 'passed' | 'failed' | 'skipped'
  taskId?: string
  error?: string
  durationMs?: number
  /** 错误分析（仅 status === 'failed' 时有值） */
  errorAnalysis?: ErrorAnalysis
  /** 提交时的快照（sourcePath, version, cipher, compression） */
  submittedSourcePath?: string
  submittedAt?: string
}
