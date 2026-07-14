/**
 * 并发限流工具（单一真源）。
 *
 * 收敛自 `composables/useBatchOperations.ts` 的 `runWithConcurrency`
 * （批量重试 / 取消 / 删除走 max:5 并发队列）。抽为 lib 工具，便于复用与单测。
 */

/**
 * 限制最大并发数的批量执行。
 *
 * @param items 待处理项
 * @param fn 每项的处理（异步）
 * @param max 最大并发数，默认 5
 * @returns 与 items 等长的数组，顺序对应；单项抛错时该位填入
 *          `{ ok: false, error: string }`（与原实现一致，调用方用 `r?.ok` 判成败）。
 */
export async function runWithConcurrency<T, R>(items: T[], fn: (item: T) => Promise<R>, max = 5): Promise<R[]> {
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
