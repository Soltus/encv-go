import { apiRequest, ApiError, isApiStatus } from "@encv/shared-components/api/core";
import { getTaskServices } from "@encv/shared-components/stores/taskServices";
import type {
  BatchTaskSpec,
  EncvTask,
  PerformanceSummary,
  RunInfo,
  RunSummary,
  TaskStatus,
  TaskStep,
  TaskType,
} from "@encv/shared-components/types/task";

// encv_tasks.ts - 拆分自 encv.ts
// 任务领域类型已提升为共享类型层 @encv/shared-components/types/task（见 packages/shared-components/src/types/task.ts），
// 下方 re-export 以兼容现有 `import ... from '@/api/encv_tasks'` / `'@/api/encv'`。

export type {
  BatchTaskSpec,
  EncvTask,
  PerformanceSummary,
  RunInfo,
  RunSummary,
  TaskStatus,
  TaskStep,
  TaskType,
} from "@encv/shared-components/types/task";

// 微服务任务类型判断：{service}.{method} 格式
export function isMicroserviceTask(type: string): boolean {
  return type.includes(".") && !type.startsWith("rollback_");
}

// 🆕 2026-06-23 Task 6.1：支持分页参数（runId / offset / limit）
//   - 后端是唯一权威，任务系统 API 提供给第三方调用，必须支持 SQL 查询
//   - 不传 params → 行为与旧版一致（GET /api/tasks，后端默认 offset=0 limit=100）
//   - 传 params → 拼接 query string
//   - 返回格式兼容：后端返回 { tasks: [...] }，旧代码可能期望数组 → 两种都处理
//   - X-Total-Count 响应头：过滤后、分页前的总数（第三方调用方用于分页 UI）

export async function getTasks(params?: { runId?: string; offset?: number; limit?: number }): Promise<EncvTask[]> {
  const data = await apiRequest<EncvTask[] | { tasks: EncvTask[] }>("/api/tasks", {
    query: {
      runId: params?.runId,
      offset: params?.offset,
      limit: params?.limit,
    },
  });
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
  return apiRequest<EncvTask>("/api/tasks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
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
  const body: Record<string, unknown> = { tasks: specs };
  if (runId) body.runId = runId;
  if (triggeredBy) body.triggeredBy = triggeredBy;

  // 120s 超时：大批量任务后端可能需要较长处理时间（apiRequest timeoutMs 内部 abort）
  try {
    return await apiRequest<EncvTask[]>("/api/tasks/batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      timeoutMs: 120_000,
    });
  } catch (e: unknown) {
    if (isApiStatus(e, 0)) {
      throw new Error(`batchCreateTasks timed out after 120s (${specs.length} tasks)`);
    }
    throw e;
  }
}

// 🆕 2026-06-23：批量创建 task 的输入定义（不含 ID——ID 由后端统一生成）

export async function cancelTask(id: string): Promise<void> {
  await apiRequest<void>(`/api/tasks/${id}/cancel`, { method: "POST" });
}

// 🆕 2026-06-23 Task 4：批量取消整个 run 的所有 task（一次 API 替代逐个 cancelTask）
// 后端路由：POST /api/runs/:runId/cancel（Task 2 实现）
// 调用方：useWorkflowTaskService.cancelRun

export async function cancelRun(runId: string): Promise<void> {
  await apiRequest<void>(`/api/runs/${encodeURIComponent(runId)}/cancel`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  });
}

// 🆕 2026-06-23 spec backend-sql-authority-view-pagination Task 4.1：
//   后端 SQL 权威——聚合计数由后端 SQL COUNT + GROUP BY status 出，不依赖前端 store。
//   前端 group card 显示 summary.total/passed/failed（不靠 store.tasks 算）。
//   store 只持有"当前视图需要的"task（视图分页），不是所有 task。

/** Run 聚合计数（后端 SQL COUNT + GROUP BY status 出） */

/** Run 列表项（带 summary，避免前端 N+1 调用 /summary） */

/** GET /api/runs/:runId/summary — 返回指定 run 的聚合计数 */

export async function getRunSummary(runId: string): Promise<RunSummary> {
  return apiRequest<RunSummary>(`/api/runs/${encodeURIComponent(runId)}/summary`);
}

/** GET /api/runs — 返回所有 run 列表（带 summary） */

export async function listRuns(): Promise<RunInfo[]> {
  const data = await apiRequest("/api/runs");
  return (data as { runs?: RunInfo[] }).runs ?? [];
}

export async function retryTask(id: string): Promise<void> {
  await apiRequest<void>(`/api/tasks/${id}/retry`, { method: "POST" });
}

// 🆕 2026-06-22 v2 架构重写：删除任务（统一走 store.removeTask）
//   历史 Q4 临时引入 removeTaskLocal（仅前端 hide）→ 删了。
//   修法：直接调 store.removeTask，走完整删除流程（store + IndexedDB + 后端 DELETE）

export async function deleteTask(id: string): Promise<void> {
  await getTaskServices().deleteTask(id);
}

export async function removeTask(id: string): Promise<void> {
  console.info("[API] removeTask:", id);
  await apiRequest<void>(`/api/tasks/${id}`, { method: "DELETE" });
}

export async function clearCompletedTasks(): Promise<{ removed: number }> {
  console.info("[API] clearCompletedTasks");
  const data = await apiRequest("/api/tasks", { method: "DELETE" });
  return { removed: (data as { removed?: number }).removed ?? 0 };
}

/**
 * 🆕 回滚特性：触发指定任务的回滚操作。
 * 后端创建一个 rollback_* 类型的反向任务，返回新 task ID。
 */

export async function rollbackTask(taskId: string): Promise<{ taskId: string }> {
  return apiRequest<{ taskId: string }>(`/api/tasks/${encodeURIComponent(taskId)}/rollback`, {
    method: "POST",
  });
}

/** 🆕 回滚特性：回收站项（删除任务移入 trash 后的元数据） */
