/**
 * 任务 WS 事件归一化（单一真源）
 *
 * Tasks 页（taskStore）与 GroupDetail 页（runTasksStore）各自实现 applyEvent，
 * 但「事件归一化」逻辑必须共用，否则同一条 WS 事件在两页产生不一致的状态。
 *
 * 历史上 taskStore 的 `completed` 分支会派生
 *   - status: data.error ? "failed" : "completed"
 *   - completedAt
 *   - 无错时 progress: 100
 * 而 runTasksStore 的 `completed` 直接 patchTaskById(data) 不做任何归一化 → 两页分歧。
 * 本函数把该归一化收敛为单一真源，两 store 的 completed 分支统一调用，消除状态不一致。
 */
import type { EncvTask } from "@encv/shared-components/types/task";

export type TaskEventType = "created" | "update" | "progress" | "completed";

export function normalizeTaskEvent(type: TaskEventType, data: any): Partial<EncvTask> {
  if (type === "completed") {
    return {
      ...data,
      status: data.error ? "failed" : "completed",
      completedAt: new Date().toISOString(),
      ...(data.error ? {} : { progress: 100 }),
    };
  }
  return { ...data };
}
