import { onUnmounted, ref, type Ref } from "vue";
import { eventBus, type EventKey } from "@encv/shared-components/composables/useEventBus";

export interface UsePollOptions {
  /** 轮询间隔（毫秒）。可传函数动态返回（如按 status 切频率） */
  intervalMs: number | (() => number);
  /** 启动后立即执行一次（默认 true） */
  immediate?: boolean;
  /** 并发守卫：上一次 fetcher 未返回则跳过本次（默认 true） */
  guardConcurrency?: boolean;
  /** 监听 eventBus 事件名 → 触发一次 refresh（可选） */
  onEvent?: EventKey | EventKey[];
}

export interface UsePollReturn {
  start: () => void;
  stop: () => void;
  /** 立即执行一次 fetcher（受并发守卫约束），不重置定时 */
  refresh: () => Promise<void>;
  /** 按当前 interval 重新排期（不立即 fetch），用于 interval 变化 */
  reschedule: () => void;
  isPolling: Readonly<Ref<boolean>>;
}

/**
 * usePoll — 统一「周期轮询端点 → 更新响应式状态」样板。
 *
 * 消除 useVectorSearchStatus / useContextUsage 各自手搓的
 * setInterval/setTimeout 自调度 + 并发守卫 + onUnmounted 清理 + 事件触发刷新。
 * onUnmounted 自动 stop + 注销 eventBus 监听，调用方无需手动清理。
 *
 * 设计：递归 setTimeout（非 setInterval），每次循环重新读取 interval，
 * 天然支持动态 interval（如 useContextUsage 按 streaming/idle 切频率）。
 */
export function usePoll(fetcher: () => Promise<void> | void, options: UsePollOptions): UsePollReturn {
  const { intervalMs, immediate = true, guardConcurrency = true, onEvent } = options;

  const isPolling = ref(false);
  let timer: ReturnType<typeof setTimeout> | null = null;
  let inFlight = false;
  let stopped = false;

  const resolveInterval = (): number => (typeof intervalMs === "function" ? intervalMs() : intervalMs);

  async function run(): Promise<void> {
    if (guardConcurrency && inFlight) return;
    inFlight = true;
    try {
      await fetcher();
    } finally {
      inFlight = false;
    }
  }

  function schedule(): void {
    if (stopped) return;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      void run().finally(schedule);
    }, resolveInterval());
  }

  function start(): void {
    if (isPolling.value) return;
    isPolling.value = true;
    stopped = false;
    if (immediate) void run();
    schedule();
  }

  function stop(): void {
    isPolling.value = false;
    stopped = true;
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function refresh(): Promise<void> {
    return run();
  }

  function reschedule(): void {
    if (!isPolling.value) return;
    schedule();
  }

  const events = onEvent ? (Array.isArray(onEvent) ? onEvent : [onEvent]) : [];
  for (const ev of events) eventBus.on(ev, refresh);

  onUnmounted(() => {
    stop();
    for (const ev of events) eventBus.off(ev, refresh);
  });

  return { start, stop, refresh, reschedule, isPolling };
}
