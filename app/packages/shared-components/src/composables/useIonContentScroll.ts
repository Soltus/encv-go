import { onUnmounted, ref, type Ref } from "vue";

/**
 * 获取 ion-content 内部滚动元素（`.inner-scroll`）的通用 composable。
 *
 * 收敛自两处重复实现（K7）：
 *   - `useTasksView` 的 `ensureScrollEl` + `initScrollElWithRetry`
 *     （shadowRoot.querySelector(".inner-scroll") + 指数退避重试 + ResizeObserver 兜底）。
 *   - `DevLogsViewer` 的 `ensureScrollEl`
 *     （Ionic 官方 `getScrollElement()` + try/catch，无重试 / 无 ResizeObserver）。
 *
 * 时序问题：onMounted 时 ion-content 可能还没完成 shadow DOM 渲染 → scrollEl=null。
 * 修法：多次重试（rAF + setTimeout 指数退避）+ ResizeObserver 兜底监听 host 尺寸变化。
 */
export interface IonContentHost {
  $el?: unknown;
  getScrollElement?: () => Promise<HTMLElement> | HTMLElement;
}

export function useIonContentScroll(contentRef: Ref<IonContentHost | null>) {
  const scrollEl = ref<HTMLElement | null>(null);
  let retryTimer: ReturnType<typeof setTimeout> | null = null;
  let resizeObserver: ResizeObserver | null = null;

  function ensureScrollEl(): HTMLElement | null {
    if (!contentRef.value) return null;
    const hostEl = (contentRef.value.$el || contentRef.value) as HTMLElement | undefined;
    let el: HTMLElement | null = null;
    // 主路径：shadowRoot 内 .inner-scroll（虚拟列表依赖此同步拿到元素）
    if (hostEl?.shadowRoot) {
      el = hostEl.shadowRoot.querySelector(".inner-scroll") as HTMLElement | null;
    }
    // 兜底：Ionic 官方 getScrollElement()（仅同步返回时采纳，异步分支交由重试处理）
    if (!el && typeof contentRef.value.getScrollElement === "function") {
      const r = contentRef.value.getScrollElement();
      if (!(r instanceof Promise)) el = r as HTMLElement | null;
    }
    if (el && el !== scrollEl.value) scrollEl.value = el;
    return scrollEl.value;
  }

  function initScrollElWithRetry(): void {
    let retryCount = 0;
    const maxRetries = 8;
    const tryInit = (): void => {
      const el = ensureScrollEl();
      if (el) return;
      retryCount++;
      if (retryCount < maxRetries) {
        // 指数退避：50ms → 100ms → 150ms → 200ms → 250ms → 300ms
        const delay = Math.min(50 * retryCount, 300);
        retryTimer = setTimeout(tryInit, delay);
      }
    };
    tryInit();

    // 兜底：ResizeObserver 监听 contentRef 尺寸变化（ion-content 完成渲染时会触发）
    if (typeof ResizeObserver !== "undefined" && contentRef.value) {
      const hostEl = (contentRef.value.$el || contentRef.value) as HTMLElement | undefined;
      if (hostEl) {
        resizeObserver = new ResizeObserver(() => {
          if (!scrollEl.value) tryInit();
        });
        resizeObserver.observe(hostEl);
      }
    }
  }

  function dispose(): void {
    if (retryTimer) {
      clearTimeout(retryTimer);
      retryTimer = null;
    }
    if (resizeObserver) {
      resizeObserver.disconnect();
      resizeObserver = null;
    }
  }

  onUnmounted(dispose);

  return { scrollEl, ensureScrollEl, initScrollElWithRetry, dispose };
}
