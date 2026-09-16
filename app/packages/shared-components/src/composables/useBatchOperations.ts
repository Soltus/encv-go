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

import { showToast } from "@encv/shared-components/composables/useToast";
import { getTaskServices } from "@encv/shared-components/stores/taskServices";
import { runWithConcurrency } from "@encv/shared-components/lib/concurrency";

export interface BatchOpResult {
  id: string;
  ok: boolean;
  error?: string;
}

export function useBatchOperations() {
  const { retryTask, cancelTask, deleteTask } = getTaskServices();

  async function batchRetry(taskIds: string[]): Promise<BatchOpResult[]> {
    if (taskIds.length === 0) return [];
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
