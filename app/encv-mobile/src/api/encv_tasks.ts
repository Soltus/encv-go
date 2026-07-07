import { getApiBaseUrl } from "./encv_core";
import type { PerformanceSummary } from "./encv_perf";

// encv_tasks.ts - 拆分自 encv.ts

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

// 微服务任务类型判断：{service}.{method} 格式
export function isMicroserviceTask(type: string): boolean {
  return type.includes(".") && !type.startsWith("rollback_");
}

export type TaskStatus = "queued" | "running" | "completed" | "failed" | "cancelled" | "cancelling";

export interface TaskStep {
  phase: string;
  startedAt: string;
  completedAt?: string;
  detail?: string;
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

// 🆕 2026-06-23 Task 6.1：支持分页参数（runId / offset / limit）
//   - 后端是唯一权威，任务系统 API 提供给第三方调用，必须支持 SQL 查询
//   - 不传 params → 行为与旧版一致（GET /api/tasks，后端默认 offset=0 limit=100）
//   - 传 params → 拼接 query string
//   - 返回格式兼容：后端返回 { tasks: [...] }，旧代码可能期望数组 → 两种都处理
//   - X-Total-Count 响应头：过滤后、分页前的总数（第三方调用方用于分页 UI）

export async function getTasks(params?: { runId?: string; offset?: number; limit?: number }): Promise<EncvTask[]> {
  const baseUrl = getApiBaseUrl();
  const query = new URLSearchParams();
  if (params?.runId) query.set("runId", params.runId);
  if (params?.offset !== undefined) query.set("offset", String(params.offset));
  if (params?.limit !== undefined) query.set("limit", String(params.limit));
  const qs = query.toString();
  const url = qs ? `${baseUrl}/api/tasks?${qs}` : `${baseUrl}/api/tasks`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  const data = await response.json();
  return Array.isArray(data) ? data : (data.tasks ?? []);
}

/**
 * 🆕 2026-06-16：拉取后端 ring buffer 最近 N 条日志
 *   - http-poll 模式：每次 tick 拉一次
 *   - WS 模式：onMounted 冷启动时拉一次历史（WS 推的实时日志不补历史）
 *   - since 参数：增量拉取（时间戳字符串，HH:MM:SS 格式）
 */

export async function createTask(
  type: TaskType,
  sourcePath: string,
  targetPath?: string,
  password?: string,
  version?: number,
  pluginName?: string,
  extraFields?: Record<string, string>,
  secondaryPassword?: string,
  cipherMode?: number,
  compressionMode?: "none" | "zstd",
  runId?: string,
  triggeredBy?: "user" | "automation" | "ai_agent"
): Promise<EncvTask> {
  console.info(
    "[API] createTask:",
    type,
    sourcePath,
    targetPath || "",
    "hasPassword:",
    !!password,
    "version:",
    version ?? "default",
    "pluginName:",
    pluginName ?? "auto",
    "hasExtraFields:",
    extraFields && Object.keys(extraFields).length > 0,
    "hasSecondaryPassword:",
    !!secondaryPassword,
    "cipherMode:",
    cipherMode ?? 0,
    "compressionMode:",
    compressionMode ?? "none",
    "runId:",
    runId ?? "",
    "triggeredBy:",
    triggeredBy ?? "user"
  );
  const baseUrl = getApiBaseUrl();
  const body: Record<string, unknown> = { type, sourcePath };
  if (targetPath) body.targetPath = targetPath;
  if (password) body.password = password;
  if (version) body.version = version;
  if (pluginName) body.pluginName = pluginName;
  if (extraFields && Object.keys(extraFields).length > 0) body.extraFields = extraFields;
  if (secondaryPassword) body.secondaryPassword = secondaryPassword;
  if (cipherMode !== undefined) body.cipherMode = cipherMode;
  if (compressionMode !== undefined) body.compressionMode = compressionMode;
  if (runId) body.runId = runId;
  if (triggeredBy) body.triggeredBy = triggeredBy;
  const response = await fetch(`${baseUrl}/api/tasks`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  return await response.json();
}

// 🆕 2026-06-23 真实架构实现：批量创建 task API
//
// 架构原则（替代 client 预占位野路子）：
//   - 前端 submitRun 阶段收集本层所有 step 的 task 定义 → 一次性调 batchCreateTasks
//   - 后端批量创建所有 task（后端生成 UUID 作为唯一权威源）→ 一次性返回所有 task
//   - 前端拿到后一次性 push 到 store → UI 立即显示 1 个 group N task（不慢慢累加）
//   - 不存在 client ID 覆盖后端 ID 的野路子
//
// 🆕 2026-07-01 P1 修复：
//   - 大批量任务分批提交（每批 50 个），避免单次请求体过大超时
//   - 增加 120s 超时（之前无超时，浏览器默认 30s 容易超时）
//   - 分批之间不 await 所有结果，聚合后统一返回（保持 API 签名不变）
//
// 调用方：useWorkflowTaskService.executeJob（每层一次性批量提交）

export async function batchCreateTasks(
  specs: BatchTaskSpec[],
  runId?: string,
  triggeredBy?: "user" | "automation" | "ai_agent"
): Promise<EncvTask[]> {
  console.info("[API] batchCreateTasks:", specs.length, "tasks", "runId:", runId ?? "", "triggeredBy:", triggeredBy ?? "user");

  // 分批：每批 50 个，避免单次请求体过大或处理时间过长
  const BATCH_SIZE = 50;
  if (specs.length <= BATCH_SIZE) {
    return batchCreateTasksSingle(specs, runId, triggeredBy);
  }

  // 分批并行提交（Promise.all），最后聚合结果
  const batches: BatchTaskSpec[][] = [];
  for (let i = 0; i < specs.length; i += BATCH_SIZE) {
    batches.push(specs.slice(i, i + BATCH_SIZE));
  }
  console.info("[API] batchCreateTasks: split into", batches.length, "batches");

  const results = await Promise.all(batches.map(batch => batchCreateTasksSingle(batch, runId, triggeredBy)));
  return results.flat();
}

/** 单批批量创建（内部函数） */
async function batchCreateTasksSingle(
  specs: BatchTaskSpec[],
  runId?: string,
  triggeredBy?: "user" | "automation" | "ai_agent"
): Promise<EncvTask[]> {
  const baseUrl = getApiBaseUrl();
  const body: Record<string, unknown> = { tasks: specs };
  if (runId) body.runId = runId;
  if (triggeredBy) body.triggeredBy = triggeredBy;

  // 120s 超时：大批量任务后端可能需要较长处理时间
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 120_000);

  try {
    const response = await fetch(`${baseUrl}/api/tasks/batch`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: controller.signal,
    });
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return await response.json();
  } catch (e: unknown) {
    if (e instanceof Error && e.name === "AbortError") {
      throw new Error(`batchCreateTasks timed out after 120s (${specs.length} tasks)`);
    }
    throw e;
  } finally {
    clearTimeout(timeoutId);
  }
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

export async function cancelTask(id: string): Promise<void> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/tasks/${id}/cancel`, {
    method: "POST",
  });
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
}

// 🆕 2026-06-23 Task 4：批量取消整个 run 的所有 task（一次 API 替代逐个 cancelTask）
// 后端路由：POST /api/runs/:runId/cancel（Task 2 实现）
// 调用方：useWorkflowTaskService.cancelRun

export async function cancelRun(runId: string): Promise<void> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/runs/${encodeURIComponent(runId)}/cancel`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  });
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
}

// 🆕 2026-06-23 spec backend-sql-authority-view-pagination Task 4.1：
//   后端 SQL 权威——聚合计数由后端 SQL COUNT + GROUP BY status 出，不依赖前端 store。
//   前端 group card 显示 summary.total/passed/failed（不靠 store.tasks 算）。
//   store 只持有"当前视图需要的"task（视图分页），不是所有 task。

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

/** GET /api/runs/:runId/summary — 返回指定 run 的聚合计数 */

export async function getRunSummary(runId: string): Promise<RunSummary> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/runs/${encodeURIComponent(runId)}/summary`);
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  return (await response.json()) as RunSummary;
}

/** GET /api/runs — 返回所有 run 列表（带 summary） */

export async function listRuns(): Promise<RunInfo[]> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/runs`);
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  const data = await response.json();
  return data.runs ?? [];
}

export async function retryTask(id: string): Promise<void> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/tasks/${id}/retry`, {
    method: "POST",
  });
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
}

// 🆕 2026-06-22 v2 架构重写：删除任务（统一走 store.removeTask）
//   历史 Q4 临时引入 removeTaskLocal（仅前端 hide）→ 删了。
//   修法：直接调 store.removeTask，走完整删除流程（store + IndexedDB + 后端 DELETE）

export async function deleteTask(id: string): Promise<void> {
  const { useTaskStore } = await import("@/stores/taskStore");
  const store = useTaskStore();
  await store.removeTask(id);
}

export async function removeTask(id: string): Promise<void> {
  console.info("[API] removeTask:", id);
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/tasks/${id}`, {
    method: "DELETE",
  });
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
}

export async function clearCompletedTasks(): Promise<{ removed: number }> {
  console.info("[API] clearCompletedTasks");
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/tasks`, {
    method: "DELETE",
  });
  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }
  const data = await response.json();
  return { removed: data.removed ?? 0 };
}

/**
 * 🆕 回滚特性：触发指定任务的回滚操作。
 * 后端创建一个 rollback_* 类型的反向任务，返回新 task ID。
 */

export async function rollbackTask(taskId: string): Promise<{ taskId: string }> {
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/tasks/${encodeURIComponent(taskId)}/rollback`, {
    method: "POST",
  });
  if (!response.ok) {
    const err = await response.json().catch(() => ({}));
    throw new Error(err.error || `HTTP error! status: ${response.status}`);
  }
  return response.json();
}

/** 🆕 回滚特性：回收站项（删除任务移入 trash 后的元数据） */
