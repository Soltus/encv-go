import { addOpenListStatusListener, getOpenListRuntime, isNative } from "@/plugins/GoProcess";
import { onMounted, onUnmounted, ref } from "vue";
import { eventBus } from "./useEventBus";

/**
 * Phase 22: 事件驱动替代 3s 轮询。
 *
 * 设计思路：
 * - plugin-openlist 用系统广播 + setPackage 跨进程投递状态变更（不再是 LocalBroadcastManager 死代码）
 * - host (GoProcessPlugin) 注册 RECEIVER_EXPORTED 接收 → notifyListeners('openlist:status', ...) 推 Capacitor
 * - 前端 addOpenListStatusListener() 订阅，emit 到 eventBus 供 LocalOpenListStatusCard 消费
 *
 * 保留一次性的 getOpenListRuntime() 快照（不是轮询）：
 * - 解决 race condition：view 挂载时 plugin 可能已运行，第一次 broadcast 早于 listener 注册
 * - 一次性调用 <10ms，无持续网络/IO 压力
 */
export function useOpenListBridge() {
  const runtime = ref<{
    isInstalled: boolean;
    running: boolean;
    port: number;
    pid: number;
    dataSizeBytes: number;
    lastError: string;
    lastUpdateTs: number;
  }>({
    isInstalled: false,
    running: false,
    port: 0,
    pid: 0,
    dataSizeBytes: 0,
    lastError: "",
    lastUpdateTs: 0,
  });

  let listenerHandle: { remove: () => Promise<void> } | null = null;

  /**
   * 一次性的初始快照（避免 view 挂载晚于 plugin 已运行导致漏掉第一个 broadcast）。
   * 失败静默：拿不到快照时仍依赖 listener 后续推送。
   */
  async function fetchInitialSnapshot() {
    if (!isNative()) return;
    try {
      const snap = await getOpenListRuntime();
      console.error("[SAT-DBG][OpenList][Frontend] initial snapshot:", JSON.stringify(snap));
      applyRuntime(snap);
    } catch (e: any) {
      console.error("[SAT-DBG][OpenList][Frontend] initial snapshot FAILED:", e?.message || e);
    }
  }

  /**
   * 把状态应用到 runtime + eventBus（前端用 Vue3 reactive，ref 替换整个对象）。
   */
  function applyRuntime(snap: {
    isInstalled: boolean;
    running: boolean;
    port: number;
    pid: number;
    dataSizeBytes: number;
    lastError: string;
    lastUpdateTs: number;
  }) {
    runtime.value = snap;
    eventBus.emit("openlist:status", snap);
    if (snap.lastError) {
      eventBus.emit("openlist:error", { type: "runtime_error", message: snap.lastError });
    }
  }

  /**
   * Phase 22 主路径：订阅 Capacitor listener（由 host push 过来）。
   */
  async function subscribeToStatus() {
    if (!isNative()) return;
    try {
      listenerHandle = await addOpenListStatusListener(status => {
        console.error("[SAT-DBG][OpenList][Frontend] listener received:", JSON.stringify(status));
        applyRuntime(status);
      });
      console.error("[SAT-DBG][OpenList][Frontend] listener subscribed OK");
    } catch (e: any) {
      console.error("[SAT-DBG][OpenList][Frontend] subscribeToStatus FAILED:", e?.message || e);
    }
  }

  onMounted(async () => {
    console.error("[SAT-DBG][OpenList][Frontend] useOpenListBridge mounted");
    // 顺序：先订阅 listener（避免漏 broadcast），再取初始快照
    await subscribeToStatus();
    await fetchInitialSnapshot();
  });

  onUnmounted(async () => {
    console.error("[SAT-DBG][OpenList][Frontend] useOpenListBridge unmounted");
    if (listenerHandle) {
      try {
        await listenerHandle.remove();
        console.error("[SAT-DBG][OpenList][Frontend] listener removed");
      } catch (e: any) {
        console.error("[SAT-DBG][OpenList][Frontend] listener remove FAILED:", e?.message || e);
      }
      listenerHandle = null;
    }
  });

  return { runtime, fetchInitialSnapshot };
}
