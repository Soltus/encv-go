/**
 * 🆕 2026-06-22 Q4：批量任务操作 composable
 *
 * 职责：
 * - 批量重试（已 failed / cancelled 的任务）
 * - 批量取消（运行中的任务）
 * - 批量删除
 *
 * 与单任务的区别：
 * - 走并发队列（max: 5）
 * - 失败任务单独记入 batchOpsResult
 * - 完成后弹 toast 汇总
 */
import { showToast } from "./useToast";

export interface BatchOpResult {
  id: string;
  ok: boolean;
  error?: string;
}

export function useBatchOperations() {
  // 🆕 2026-06-22 并发限流器（max 5 同时）
  async function runWithConcurrency<T, R>(items: T[], fn: (item: T) => Promise<R>, max = 5): Promise<R[]> {
    const results: R[] = [];
    let cursor = 0;
    async function worker() {
      while (cursor < items.length) {
        const idx = cursor++;
        try {
          results[idx] = await fn(items[idx]);
        } catch (err) {
          // 包装错误，让调用方决定如何处理
          results[idx] = { ok: false, error: String(err) } as any;
        }
      }
    }
    const workers = Array.from({ length: Math.min(max, items.length) }, () => worker());
    await Promise.all(workers);
    return results;
  }

  async function batchRetry(taskIds: string[]): Promise<BatchOpResult[]> {
    if (taskIds.length === 0) return [];
    // 动态 import 避免循环依赖
    const { retryTask } = await import("@/api/encv");
    const results = await runWithConcurrency(taskIds, async id => {
      try {
        await retryTask(id);
        return { id, ok: true } as BatchOpResult;
      } catch (err) {
        return { id, ok: false, error: String(err) } as BatchOpResult;
      }
    });
    const success = results.filter(r => r?.ok).length;
    const failed = results.length - success;
    showToast({
      message: `重试完成: ${success} 成功 / ${failed} 失败`,
      duration: 3000,
      color: failed > 0 ? "warning" : "success",
    });
    return results;
  }

  async function batchCancel(taskIds: string[]): Promise<BatchOpResult[]> {
    if (taskIds.length === 0) return [];
    const { cancelTask } = await import("@/api/encv");
    const results = await runWithConcurrency(taskIds, async id => {
      try {
        await cancelTask(id);
        return { id, ok: true } as BatchOpResult;
      } catch (err) {
        return { id, ok: false, error: String(err) } as BatchOpResult;
      }
    });
    const success = results.filter(r => r?.ok).length;
    const failed = results.length - success;
    showToast({
      message: `取消完成: ${success} 成功 / ${failed} 失败`,
      duration: 3000,
      color: failed > 0 ? "warning" : "success",
    });
    return results;
  }

  async function batchDelete(taskIds: string[]): Promise<BatchOpResult[]> {
    if (taskIds.length === 0) return [];
    const { deleteTask } = await import("@/api/encv");
    const results = await runWithConcurrency(taskIds, async id => {
      try {
        await deleteTask(id);
        return { id, ok: true } as BatchOpResult;
      } catch (err) {
        return { id, ok: false, error: String(err) } as BatchOpResult;
      }
    });
    const success = results.filter(r => r?.ok).length;
    const failed = results.length - success;
    showToast({
      message: `删除完成: ${success} 成功 / ${failed} 失败`,
      duration: 3000,
      color: failed > 0 ? "warning" : "success",
    });
    return results;
  }

  return {
    batchRetry,
    batchCancel,
    batchDelete,
  };
}
