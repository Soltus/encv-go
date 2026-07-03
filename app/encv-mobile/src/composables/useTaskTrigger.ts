/**
 * 任务触发者标签 + workflow run 关联 — localStorage 持久化
 *
 * 用法：
 *   - 自动化测试入口：`setTaskMetadata(task.id, 'automation', runId)`
 *   - AI 智能体入口：`setTaskMetadata(task.id, 'ai_agent', runId)`
 *   - 用户手动创建：默认 'user'（无需显式登记）
 *   - 显示：`getTriggeredBy(task.id)` / `getRunIdForTask(task.id)`
 *
 * 设计原则（2026-06-11 v7 简化）：
 *   - 单一 localStorage key，没有 v1/v2/v3 兼容
 *   - 单一数据源 taskMetadata Map（不再 reactive() 同步双写）
 *   - 调试/重置时手动调用 _reloadTriggeredByCache()
 *   - localStorage 异常时直接清空（开发环境，可接受）
 */

import { reactive } from "vue";

export type TriggeredBy = "user" | "automation" | "ai_agent";

const STORAGE_KEY = "encv_task_triggered_by";
const MAX_ENTRIES = 500;

interface TriggeredByEntry {
  triggeredBy: TriggeredBy;
  /** workflow run 关联 ID（同一 run 的 task 共享） */
  runId?: string;
  recordedAt: string;
}

type TriggeredByMap = Record<string, TriggeredByEntry>;

// 单一数据源：reactive 容器（reactive 让 Vue 追踪到 mutation）
const triggeredByMap = reactive<TriggeredByMap>({});

let initialized = false;

function ensureLoaded(): void {
  if (initialized) return;
  initialized = true;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return;
    const parsed = JSON.parse(raw) as TriggeredByMap;
    if (!parsed || typeof parsed !== "object") return;
    for (const [k, v] of Object.entries(parsed)) {
      triggeredByMap[k] = v;
    }
  } catch (e) {
    // localStorage 异常 → 清空（开发环境，可接受）
    console.warn("[useTaskTrigger] localStorage read failed, starting empty:", e);
    try {
      localStorage.removeItem(STORAGE_KEY);
    } catch {
      // silent
    }
  }
}

function writeMap(): void {
  const entries = Object.entries(triggeredByMap)
    .sort((a, b) => b[1].recordedAt.localeCompare(a[1].recordedAt))
    .slice(0, MAX_ENTRIES);
  const trimmed: TriggeredByMap = Object.fromEntries(entries);
  for (const k of Object.keys(triggeredByMap)) delete triggeredByMap[k];
  Object.assign(triggeredByMap, trimmed);
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(trimmed));
  } catch (e) {
    // localStorage 写失败（quota 等）→ 不阻塞前端，警告即可
    console.warn("[useTaskTrigger] localStorage write failed:", e);
  }
}

function trimMapInPlace(): void {
  const entries = Object.entries(triggeredByMap)
    .sort((a, b) => b[1].recordedAt.localeCompare(a[1].recordedAt))
    .slice(0, MAX_ENTRIES);
  const trimmed: TriggeredByMap = Object.fromEntries(entries);
  for (const k of Object.keys(triggeredByMap)) delete triggeredByMap[k];
  Object.assign(triggeredByMap, trimmed);
}

/**
 * 记录 task 的触发者 + workflow run 关联
 */
export function setTaskMetadata(taskId: string, triggeredBy: TriggeredBy, runId?: string): void {
  if (!taskId) return;
  ensureLoaded();
  triggeredByMap[taskId] = {
    triggeredBy,
    recordedAt: new Date().toISOString(),
    ...(runId ? { runId } : {}),
  };
  trimMapInPlace();
  writeMap();
}

/**
 * 兼容旧 API（之前叫 recordTriggeredBy，现在统一叫 setTaskMetadata）
 */
export const recordTriggeredBy = setTaskMetadata;

/**
 * 取 task 的元数据
 */
export function getTaskMetadata(taskId: string): { triggeredBy: TriggeredBy; runId?: string } | undefined {
  if (!taskId) return undefined;
  ensureLoaded();
  const entry = triggeredByMap[taskId];
  if (!entry) return undefined;
  return { triggeredBy: entry.triggeredBy, runId: entry.runId };
}

/**
 * 读 task 的 triggeredBy（'user' 默认）
 */
export function getTriggeredBy(taskId: string): TriggeredBy {
  if (!taskId) return "user";
  ensureLoaded();
  return triggeredByMap[taskId]?.triggeredBy ?? "user";
}

/**
 * 读 task 关联的 workflow run.id
 */
export function getRunIdForTask(taskId: string): string | undefined {
  if (!taskId) return undefined;
  ensureLoaded();
  return triggeredByMap[taskId]?.runId;
}

/**
 * 用户主动重置所有触发者记录
 * 用法：Tasks.vue 「重置分组」按钮 → 调这个 → 所有 task 重新变 'user'
 */
export function clearTriggeredBy(): void {
  for (const k of Object.keys(triggeredByMap)) delete triggeredByMap[k];
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // silent
  }
  initialized = false;
  ensureLoaded();
}

/**
 * 清除进程级缓存（用于调试）
 */
export function _reloadTriggeredByCache(): void {
  initialized = false;
  for (const k of Object.keys(triggeredByMap)) delete triggeredByMap[k];
}
