// task.ts - 任务领域核心类型（从 encv-mobile/src/api/encv_tasks 提升为共享类型层）
// 原位置保留 re-export 兼容（见 encv-mobile/src/api/encv_tasks.ts 与 encv_perf.ts）。

export type TaskType =
  | "encrypt"
  | "decrypt"
  | "move"
  | "copy"
  | "rename"
  | "delete"
  | "rollback_encrypt"
  | "rollback_decrypt"
  | "rollback_move"
  | "rollback_copy"
  | "rollback_rename"
  | "rollback_delete"
  | string;

export type TaskStatus = "queued" | "running" | "completed" | "failed" | "cancelled" | "cancelling";

export interface TaskStep {
  phase: string;
  startedAt: string;
  completedAt?: string;
  detail?: string;
}

export interface PerformanceSummary {
  avgThroughput: number;
  grade: "excellent" | "good" | "warn";
  gradeScore: number;
  totalDurationMs: number;
  sourceSize: number;
  outputSize: number;
}

export interface EncvTask {
  id: string;
  type: TaskType;
  sourcePath: string;
  targetPath?: string;
  pluginName?: string;
  status: TaskStatus;
  progress: number;
  phase?: string;
  speed?: string;
  eta?: string;
  error?: string;
  errorDetail?: string;
  warning?: string;
  warningDetail?: string;
  containerVersion?: number;
  outputPath?: string;
  steps?: TaskStep[];
  createdAt: string;
  completedAt?: string;
  // 🆕 2026-06-10 修复 v4：triggeredBy + runId 直接放 task 对象上
  // 历史：分组依赖 localStorage.useTaskTrigger，跨 session / localStorage 清空后全失效
  //   → 「任务组只在一开始的时候正确显示」+「插件没正确识别，任务依旧全部平铺」
  // 修复：这两个字段在 submitAction 返回时就写到 task 对象上，不再只存 localStorage
  //   - 当前 session：直接读 t.triggeredBy / t.runId（O(1) 内存访问）
  //   - 跨 session：localStorage 作 fallback（旧 task 没有这 2 字段，try useTaskTrigger）
  triggeredBy?: "user" | "automation" | "ai_agent";
  runId?: string;
  // 🆕 2026-06-18 Task 17：加解密参数回显字段
  // 后端 Task 16 持久化 cipherMode (0=AES-128-GCM, 1=AES-256-GCM) + compressionMode ('none'|'zstd')
  // 前端 Task 18 任务卡片展示用 — 刷新页面后仍能回显参数
  // optional：旧任务（Task 16 之前创建的）没有这 2 字段，反序列化时 undefined
  cipherMode?: number;
  compressionMode?: "none" | "zstd";
  // extraFields 已存在于 EncvTask 之外（createTask body 传），但后端 MobileTask 也有这个字段
  // 这里加上让前端能读到后端回传的 extraFields（如 plugin_password 等自定义参数）
  extraFields?: Record<string, string>;
  // 🆕 回滚特性：rollbackOf 指向原任务 ID，originalPath 为原始路径（回滚用）
  rollbackOf?: string;
  originalPath?: string;
  // 🆕 性能指标摘要（task:completed 事件推送，仅 completed 状态有值）
  performanceSummary?: PerformanceSummary;
  /** 向量搜索相关度分数（0-1，越大越相似）。仅 /api/search/tasks 返回时填充。 */
  searchScore?: number;

  // ─── 微服务任务字段（2026-07-03 spec microservice-kernel-task-system） ───
  /** 微服务名（如 "fts"、"vector"、"cache"、"db"、"plugin"、"tool"、"system"）。仅微服务任务有值。 */
  serviceName?: string;
  /** 方法名（如 "rebuild"、"search"、"clean"、"backup"）。仅微服务任务有值。 */
  methodName?: string;
  /** 租户 ID（多租户隔离用）。仅微服务任务有值。 */
  tenantId?: string;
  /** 执行耗时（毫秒）。仅微服务任务有值。 */
  durationMs?: number;
  /** 输入参数 JSON。仅微服务任务有值。 */
  inputJSON?: string;
  /** 输出结果 JSON。仅微服务任务有值。 */
  outputJSON?: string;
  /** 重试次数。仅微服务任务有值。 */
  attempts?: number;
  /** 优先级（数字越大越优先）。仅微服务任务有值。 */
  priority?: number;
  /** 自定义标签 JSON（灵活扩展）。仅微服务任务有值。 */
  tagsJSON?: string;
}

// 🆕 2026-06-23：批量创建 task 的输入定义（不含 ID——ID 由后端统一生成）
export interface BatchTaskSpec {
  type: TaskType;
  sourcePath: string;
  targetPath?: string;
  password?: string;
  secondaryPassword?: string;
  version?: number;
  pluginName?: string;
  extraFields?: Record<string, string>;
  cipherMode?: number;
  compressionMode?: "none" | "zstd";
}

/** Run 聚合计数（后端 SQL COUNT + GROUP BY status 出） */
export interface RunSummary {
  runId: string;
  total: number;
  passed: number;
  failed: number;
  running: number;
  pending: number;
  cancelled: number;
  /** 完成百分比（终态 task / total * 100） */
  percent: number;
}

/** Run 列表项（带 summary，避免前端 N+1 调用 /summary） */
export interface RunInfo {
  runId: string;
  startedAt: string;
  triggeredBy: string;
  summary: RunSummary;
}

// ─── 插件 / task 表单领域类型（2026-07-12 从 encv-mobile/src/api/encv_plugins 提升为共享类型层） ───
// 原位置保留 re-export 兼容（见 encv-mobile/src/api/encv_plugins.ts）。

export type PasswordStrategy = "global" | "independent" | "none";

export interface TaskField {
  key: string;
  label: string;
  type: "string" | "password" | "select" | "bool";
  required: boolean;
  defaultValue: string;
  help: string;
  options?: string[];
  optionLabels?: Record<string, string>;
  condition?: "" | "encrypt" | "decrypt";
}

export interface TaskOptions {
  passwordStrategy: PasswordStrategy;
  supportVersionSelect: boolean;
  supportedVersions: number[] | null;
  defaultVersion: number;
  extraFields: TaskField[];
}

export interface PluginCandidate {
  name: string;
  matchType: "mime" | "extension" | "general" | "container";
  priority: number;
  taskOptions: TaskOptions | null;
}

export interface PredictPluginResponse {
  candidates: PluginCandidate[];
  pluginName: string | null;
  error?: string;
  taskOptions: TaskOptions | null;
}
