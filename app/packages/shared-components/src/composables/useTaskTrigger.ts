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
  /**
   * 写入序号（单调递增）。recordedAt 相同时用它决出先后 —— 见 sortEntriesNewestFirst
   * 的注释：没有它，同一毫秒内写入的一批条目裁剪时会保留**最早**的，把刚写的丢掉。
   * 老数据没有这个字段（读回来时按原顺序补号），缺省 0 ⇒ 不影响存量。
   */
  seq?: number;
}

type TriggeredByMap = Record<string, TriggeredByEntry>;

// 单一数据源：reactive 容器（reactive 让 Vue 追踪到 mutation）
const triggeredByMap = reactive<TriggeredByMap>({});

let initialized = false;
/** 单调递增的写入序号（见 TriggeredByEntry.seq 的注释） */
let seqCounter = 0;

function ensureLoaded(): void {
  if (initialized) return;
  initialized = true;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return;
    const parsed = JSON.parse(raw) as TriggeredByMap;
    if (!parsed || typeof parsed !== "object") return;
    // 老数据没有 seq ⇒ 按 JSON 里的顺序补号（越靠后越新），保持裁剪语义正确
    let seq = seqCounter;
    for (const [k, v] of Object.entries(parsed)) {
      if (typeof v.seq !== "number") v.seq = ++seq;
      triggeredByMap[k] = v;
    }
    seqCounter = seq;
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
  const trimmed = trimMapInPlace();
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(trimmed));
  } catch (e) {
    // localStorage 写失败（quota 等）→ 不阻塞前端，警告即可
    console.warn("[useTaskTrigger] localStorage write failed:", e);
  }
}

// sortEntriesNewestFirst 把条目按「新 → 旧」排序。
//
// ⚠️ 为什么要 seq 这个 tie-breaker（2026-10-02 修的真 bug）：
//   只按 recordedAt（ISO 字符串）排序时，**同一毫秒内**写入的条目时间戳完全相同，
//   排序退化为"保持原顺序"（稳定排序）⇒ 降序 slice(0, MAX_ENTRIES) 取到的其实是
//   **最早插入的那批**，刚写入的反而被裁掉。
//   真机表现：批量创建任务（自动化/workflow 一口气建几百个）时，
//   `getTriggeredBy()` 随机返回默认值 user ⇒ UI 的"触发者"分类/统计错，且极难复现
//   （取决于循环跑得快不快 —— 单测里表现为 flaky：600 条用例时红时绿）。
function sortEntriesNewestFirst(entries: Array<[string, TriggeredByEntry]>): Array<[string, TriggeredByEntry]> {
  return [...entries].sort((a, b) => {
    const byTime = b[1].recordedAt.localeCompare(a[1].recordedAt);
    if (byTime !== 0) return byTime;
    return (b[1].seq ?? 0) - (a[1].seq ?? 0);
  });
}

/** trimMapInPlace 裁剪到 MAX_ENTRIES（保留最新的），返回裁剪后的 map */
function trimMapInPlace(): TriggeredByMap {
  const entries = sortEntriesNewestFirst(Object.entries(triggeredByMap)).slice(0, MAX_ENTRIES);
  const trimmed: TriggeredByMap = Object.fromEntries(entries);
  for (const k of Object.keys(triggeredByMap)) delete triggeredByMap[k];
  Object.assign(triggeredByMap, trimmed);
  return trimmed;
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
    seq: ++seqCounter,
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
