import { onUnmounted, watch, type Ref } from "vue";
import { useSimverse } from "./useSimverse";

export interface LiveRefreshOptions {
  /** WS 实时信号 ref（如 economySignal / chronicleSignal），收到推送时触发刷新（仅 WS 已连接时） */
  signal?: Ref<number>;
  /** WS 未连接时的兜底轮询间隔（ms）。不传则无兜底轮询 */
  pollMs?: number;
  /** 两次刷新最小间隔（ms），防止高频抖动，默认 2000 */
  throttleMs?: number;
}

/**
 * P7 持续演化：让视图随世界演化实时刷新。
 * - 主路径：监听 WS 实时信号（economy:update / chronicle:event），世界运行时自动刷新；
 * - 兜底：WS 未连接时按 pollMs 轮询，连接恢复即停止轮询（"实时推送替代轮询"）。
 */
export function useLiveRefresh(
  refresh: () => void | Promise<void>,
  opts: LiveRefreshOptions = {}
) {
  const { isConnected } = useSimverse();
  const throttleMs = opts.throttleMs ?? 2000;
  let pollTimer: number | null = null;
  let lastRun = 0;
  let stopped = false;

  async function run() {
    if (stopped) return;
    const now = Date.now();
    if (now - lastRun < throttleMs) return;
    lastRun = now;
    try {
      await refresh();
    } catch (e) {
      // 后台刷新忽略网络抖动，避免打断用户
      console.warn("[simverse] live refresh failed:", e);
    }
  }

  // WS 信号驱动刷新（仅连接时）
  if (opts.signal) {
    watch(opts.signal, () => {
      if (isConnected.value) run();
    });
  }

  function startPoll() {
    if (pollTimer != null || !opts.pollMs) return;
    pollTimer = window.setInterval(() => run(), opts.pollMs);
  }
  function stopPoll() {
    if (pollTimer != null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  // WS 未连接 → 兜底轮询；连接恢复 → 停止轮询
  watch(
    isConnected,
    (connected) => {
      if (connected) stopPoll();
      else startPoll();
    },
    { immediate: true }
  );

  onUnmounted(() => {
    stopped = true;
    stopPoll();
  });

  return { run, startPoll, stopPoll };
}
