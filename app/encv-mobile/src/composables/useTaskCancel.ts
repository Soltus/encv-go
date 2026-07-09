import { getApiBaseUrl } from "@/api/encv_core";
import { cancelTask } from "@/api/encv_tasks";
import { enqueueCancelWorker, isNative } from "@/plugins/GoProcess";
import { ref } from "vue";

/**
 * useTaskCancel — 任务取消的双写 composable。
 *
 * 2026-07-03 spec android-workmanager-split-start-stop Phase 3.4
 *
 * 设计：
 *   取消任务时同时发起：
 *     1. HTTP POST /api/tasks/:id/cancel（同步，期望立即生效）
 *     2. enqueueCancelWorker(taskId)（WorkManager 持久化，兜底）
 *
 * ⚠️ 注意：不要直接导入 GoProcess plugin 对象，使用包装函数 enqueueCancelWorker
 *         详见 @/plugins/GoProcess.ts 顶部的架构守卫注释
 *
 * 容错策略：
 *   - HTTP 成功：直接返回成功，Worker 后续执行时发现 task 已取消则 noop
 *   - HTTP 失败：不抛错给调用方（UI 显示失败），但 Worker 仍会重试
 *                （Go 进程可能已死导致 HTTP 失败，Worker 会在 Go 重启后重试）
 *   - Web 端：只走 HTTP，WorkManager 不可用（web plugin 不实现，返回 noop）
 *
 * 与 useTasksList.cancelTaskById 的区别：
 *   - useTasksList.cancelTaskById 只走 HTTP，且会乐观地把 task status 改为 cancelling
 *   - useTaskCancel.cancel 走 HTTP + WorkManager 双写
 *   - 未来可以把 useTasksList.cancelTaskById 替换为 useTaskCancel.cancel
 */
export function useTaskCancel() {
  const isCancelling = ref(false);

  /**
   * 取消任务（双写：HTTP + WorkManager）。
   * @param taskId 任务 ID
   * @returns Promise<boolean> true 表示 HTTP 成功；false 表示 HTTP 失败但 Worker 已入队
   * @throws 仅在非 native 且 HTTP 失败时抛出（没有 Worker 兜底）
   */
  async function cancel(taskId: string): Promise<boolean> {
    if (!taskId) return false;
    isCancelling.value = true;

    // 1. 先入队 WorkManager（即使 HTTP 失败也有兜底）
    //    native 平台才有 WorkManager，web 端走 web plugin 返回 success:false 但不影响
    if (isNative()) {
      try {
        const result = await enqueueCancelWorker(taskId);
        console.debug("[useTaskCancel] enqueued cancel worker:", result);
      } catch (e) {
        console.warn("[useTaskCancel] enqueueCancelWorker failed:", e);
        // Worker 入队失败不影响 HTTP 路径，继续
      }
    }

    // 2. 发起 HTTP cancel
    try {
      await cancelTask(taskId);
      return true;
    } catch (err) {
      console.warn("[useTaskCancel] HTTP cancel failed:", err);
      if (isNative()) {
        // native 模式：HTTP 失败但 Worker 已入队，返回 false 但不抛错
        // （Worker 会在 Go 进程恢复后重试
        return false;
      }
      // web 模式：没有 Worker 兜底，直接抛错
      throw err;
    } finally {
      isCancelling.value = false;
    }
  }

  /**
   * 轮询取消状态（用于 HTTP 失败时，前端可以轮询 Worker 结果）。
   * 简化版：直接查 GET /api/tasks/:id 看 status 是否变为 cancelled。
   */
  async function pollCancelStatus(taskId: string, maxAttempts = 10, intervalMs = 2000): Promise<boolean> {
    for (let i = 0; i < maxAttempts; i++) {
      try {
        const baseUrl = getApiBaseUrl();
        const resp = await fetch(`${baseUrl}/api/tasks/${encodeURIComponent(taskId)}`);
        if (resp.ok) {
          const data = await resp.json();
          if (data.status === "cancelled" || data.status === "canceled") {
            return true;
          }
        }
      } catch {
        // Go 进程可能还没起来，继续等
      }
      await new Promise(r => setTimeout(r, intervalMs));
    }
    return false;
  }

  return {
    isCancelling,
    cancel,
    pollCancelStatus,
  };
}
