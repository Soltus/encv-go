/**
 * taskViewCompute.worker — 视图计算 Web Worker（壳）
 *
 * 2026-07-12 Phase 3：纯计算已抽到 `@/lib/taskViewComputeCore`
 * （shared 包，从本文件原内联逻辑去重而来）。本文件只保留
 * Worker 消息壳：接收 compute 请求 → 调 buildDisplayedItems → postMessage。
 *
 * 职责边界（与主线程 useTaskViewCompute 一致）：
 *   - i18n 标签：date section 返回 dateKey，主线程映射为 label
 *   - 响应式：Worker 不持响应式状态，只做纯计算
 *   - 降级：在主线程 useTaskViewCompute 中处理（Worker 不可用 → 同步计算）
 *
 * 通信协议：
 *   主线程 → Worker：{ type: 'compute', tasks, viewMode, sortBy, ...filters, pinnedRunIds, requestId }
 *   Worker → 主线程：{ type: 'result', items, requestId }
 */
import { buildDisplayedItems, type ComputeInput, type ComputeOutput } from "@encv/shared-components/lib/taskViewComputeCore";

self.onmessage = (e: MessageEvent<ComputeInput>) => {
  const input = e.data;
  if (input?.type !== "compute") return;
  try {
    const items = buildDisplayedItems(input);
    const output: ComputeOutput = { type: "result", items, requestId: input.requestId };
    (self as any).postMessage(output);
  } catch (err) {
    // Worker 内计算出错：返回空数组（主线程会保留旧值）
    console.error("[taskViewCompute.worker] compute failed:", err);
    const output: ComputeOutput = { type: "result", items: [], requestId: input.requestId };
    (self as any).postMessage(output);
  }
};
