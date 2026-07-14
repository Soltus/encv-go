/**
 * useRealtimeTransport — 统一实时传输单例（2026-06-10 重构）
 *
 * 目标：
 *   - 集中 transport 模式选举（ws / http-poll / native-bridge）
 *   - 集中 fallback 逻辑（不再每个用到 WS 的地方写 isOpenPreviewBrowser）
 *   - 消费方继续 eventBus.on，不感知 transport 变化
 *   - 新增 WS 事件类型 → 只改 backend 实现，消费方零改动
 *
 * 用法：
 *   - App.vue: const transport = useRealtimeTransport(); transport.connect()
 *   - 消费方: eventBus.on('task:update', cb)（不变）
 *
 * 选举规则（按优先级）：
 *   1. _forcedMode（测试用强制模式）
 *   2. isOpenPreviewBrowser()    → http-poll
 *   3. 默认                       → ws
 *
 * 2026-06-14 增强（安卓真机 WS 稳定性）：
 *   - **WS → HttpPoll 自动降级**：WS 连续失败 N 次后自动切换到 http-poll，
 *     避免 Android 系统层静默杀长连接导致 UI 永远卡在「连接中」状态。
 *   - **降级后定期尝试回升**：每 5min 尝试用 WS 重连一次，成功则切回。
 *
 * 历史 bug 根因：
 *   - 沙箱 OpenPreview 浏览器 trae 反代 :16000 不支持 WebSocket upgrade
 *   - 之前每个用到 WS 的地方都自己写 isOpenPreviewBrowser / isSandboxBrowser 判断
 *   - 散落在 useWebSocket.ts / useServerStatus.ts / useFrontendLogs.ts / DevLogs.vue
 *   - 加新 WS 事件类型 = 改 4+ 文件
 *
 * 验证：
 *   - ws 模式：沙箱本地 dev (localhost:16666) / 真机浏览器
 *   - http-poll 模式：OpenPreview 浏览器 / WS 持续失败降级
 *   - native-bridge 模式：APK 真机（暂未实现）
 */

import { type Ref, ref } from "vue";
import { getApiBaseUrl, isOpenPreviewBrowser } from "@encv/shared-components/api/encv";
import { eventBus } from "@encv/shared-components/composables/useEventBus";
import type { Backend, ConnectionState } from "./realtime/Backend";
import { createHttpPollBackend } from "./realtime/HttpPollBackend";
import { createNativeBridgeBackend } from "./realtime/NativeBridgeBackend";
import { createWsBackend } from "./realtime/WsBackend";

export type TransportMode = "ws" | "http-poll" | "native-bridge" | "unknown";

export interface RealtimeTransport {
  /** 启动 transport（幂等：重复调用只触发一次） */
  connect(): void;
  /** 停止 transport */
  disconnect(): void;
  /** 强制重连（先 disconnect 再 connect） */
  forceReconnect(): void;
  /** 当前连接状态 */
  readonly connectionState: Readonly<Ref<ConnectionState>>;
  /** 当前 transport 模式（connect 后才会变） */
  readonly transportMode: Readonly<Ref<TransportMode>>;
  /** 当前是否在 OpenPreview 浏览器（只读） */
  readonly isSandboxBrowser: Readonly<Ref<boolean>>;
  /**
   * 🆕 v4 2026-06-18 M1：file:change 事件 tab active gate
   * - true（默认）：转发 file:change 给消费者
   * - false：丢弃 file:change（tab 不可见时用，避免 Files tab 在后台被狂刷）
   * @param active tab 是否可见
   * @param onActive 切回可见时的回调（用于触发一次性 loadFiles）
   */
  setFileChangeGate(active: boolean, onActive?: () => void): void;
  /** 测试用：强制 transport 模式（传 null 恢复默认选举） */
  __forceMode(mode: TransportMode | null): void;
  /** 测试用：重置单例（仅在测试 setup/teardown 用） */
  __resetForTesting(): void;
}

// =============================================================================
// 自动降级参数（2026-06-14 新增）
// =============================================================================
// 触发条件：WS 模式下，短时间内累计 N 次"ws 关闭"事件 → 切到 http-poll
// 理由：Android 系统层 / Capacitor WebView 在某些情况下会静默杀长连接，
//       WS 端可能长时间无法重连成功 → 用 http-poll 兜底
// =============================================================================
const WS_FAILURE_WINDOW_MS = 60_000; // 失败统计窗口（60s）
const WS_FAILURE_THRESHOLD = 3; // 窗口内 N 次失败触发降级
const WS_RECOVERY_CHECK_MS = 5 * 60_000; // 降级后每 5min 尝试回升 WS

// 模块级单例
let _instance: RealtimeTransport | null = null;
let _forcedMode: TransportMode | null = null;

/**
 * 检测是否在 Capacitor native 环境（APK / iOS）
 *
 * 关键决策（2026-06-11）：native 真机仍走 ws 模式，**不**用 native-bridge。
 *
 * 原因：
 *   1. NativeBridgeBackend 现状是空壳（start() 只 emit online:true，无事件转发）——
 *      走 native-bridge 意味着真机 task 进度永远不更新、mock 写盘永远不触发
 *   2. Capacitor WebView = Android System WebView / iOS WKWebView = Chrome/Safari 内核
 *      → WebSocket 原生支持，直连 127.0.0.1:2025 即可
 *   3. ws 模式比 native-bridge 简单得多（不用写 native plugin）
 *
 * native-bridge 留作未来 SSE / 设备本地 socket 实现（见 NativeBridgeBackend.ts TODO）。
 */
function isNative(): boolean {
  if (typeof window === "undefined") return false;
  // Capacitor 在 window 上挂 capacitor 全局对象
  const cap = (window as any).Capacitor;
  return Boolean(cap && typeof cap.isNativePlatform === "function" && cap.isNativePlatform());
}

/** 选举 transport 模式
 *
 * 顺序（2026-06-11 v3 修正）：
 *   1. _forcedMode（测试用强制模式）
 *   2. isOpenPreviewBrowser()    → http-poll（沙箱 trae 反代 :16000 不支持 WS upgrade）
 *   3. 默认                       → ws（**包括真机** Capacitor WebView，NativeBridgeBackend 暂未实现）
 *
 * 历史：原顺序是 isNative() → native-bridge（空壳）→ 真机进度永远不更新
 */
function electMode(): TransportMode {
  if (_forcedMode) return _forcedMode;
  // 关键：OpenPreview 浏览器判断放在 isNative 之前
  // 因为 trae OpenPreview 也可能在 webview 内跑（Capacitor 检测可能误报）
  if (isOpenPreviewBrowser()) return "http-poll";
  return "ws";
}

/** baseUrl 变更监听（当 serverUrl 改变时强制重连） */
function ensureApiBaseListeners(transport: RealtimeTransport): () => void {
  if (typeof window === "undefined") return () => {};
  const onApiBaseConnected = () => {
    console.info("[RealtimeTransport] api-base:connected → forceReconnect");
    setTimeout(() => transport.forceReconnect(), 100);
  };
  const onStorage = (e: StorageEvent) => {
    if (e.key !== "encv-server-url") return;
    if (!e.newValue) return;
    console.info("[RealtimeTransport] storage event: encv-server-url changed → forceReconnect");
    setTimeout(() => transport.forceReconnect(), 100);
  };
  eventBus.on("api-base:connected", onApiBaseConnected);
  window.addEventListener("storage", onStorage);
  return () => {
    eventBus.off("api-base:connected", onApiBaseConnected);
    window.removeEventListener("storage", onStorage);
  };
}

function createTransport(): RealtimeTransport {
  const connectionState = ref<ConnectionState>("disconnected");
  const transportMode = ref<TransportMode>("unknown");
  const isSandboxBrowser = ref(isOpenPreviewBrowser());
  let backend: Backend | null = null;
  let cleanupListeners: (() => void) | null = null;
  // 🆕 v4 2026-06-18 M1：file:change 事件 tab active gate
  //   - 默认 true（tab 未注册 gate 时不丢事件，向后兼容）
  //   - Files.vue 切到不可见时调 setFileChangeGate(false) → emit 时丢 file:change
  //   - 切回时 setFileChangeGate(true) → 立刻补一次 onTabActive 回调（由 caller 实现）
  let fileChangeGateActive = true;
  let onFilesTabActiveCallback: (() => void) | null = null;

  // 2026-06-14 增强：自动降级状态
  // 注意：这些 state 闭包在 ensureBackend 内被引用，ensureBackend 又是 createTransport 内
  // 闭包 → 整个 transport 生命周期内只有一份
  let wsFailureTimestamps: number[] = []; // 滑动窗口内的失败时间戳
  let downgradeTimer: ReturnType<typeof setTimeout> | null = null; // 降级后定期尝试回升 WS
  let isAutoDowngraded = false; // 当前是否处于自动降级状态

  function recordWsFailure() {
    const now = Date.now();
    wsFailureTimestamps.push(now);
    // 清理窗口外的旧时间戳
    wsFailureTimestamps = wsFailureTimestamps.filter(t => now - t < WS_FAILURE_WINDOW_MS);
    console.info(
      `[RealtimeTransport] WS failure recorded: ${wsFailureTimestamps.length}/${WS_FAILURE_THRESHOLD} in last ${WS_FAILURE_WINDOW_MS}ms`
    );
    if (wsFailureTimestamps.length >= WS_FAILURE_THRESHOLD) {
      console.warn(`[RealtimeTransport] WS 连续失败 ${wsFailureTimestamps.length} 次，自动降级到 http-poll`);
      isAutoDowngraded = true;
      // 强制重启 — 这次 start() 走 http-poll
      backend?.stop();
      backend = null;
      connectionState.value = "disconnected";
      transportMode.value = "unknown";
      // 重置失败统计（避免一直升级失败）
      wsFailureTimestamps = [];
      // 安排定期回升检查
      scheduleWsRecoveryCheck();
      // 触发 connect（这次 electMode 仍然返回 ws，但 transport 内部逻辑要走 http-poll）
      setTimeout(() => reconnectInternal(), 0);
    }
  }

  /**
   * 降级后定期尝试切回 WS — 每 WS_RECOVERY_CHECK_MS 探一次。
   * 注意：探的时候清空 isAutoDowngraded 标记，让 electMode 决定走 ws 还是 http-poll。
   * 探成功（ws.onopen 触发）→ 继续 ws；探失败 → 再次 recordWsFailure → 重新降级
   */
  function scheduleWsRecoveryCheck() {
    if (downgradeTimer) return;
    console.info(`[RealtimeTransport] scheduling WS recovery check in ${WS_RECOVERY_CHECK_MS}ms`);
    downgradeTimer = setTimeout(() => {
      downgradeTimer = null;
      if (!isAutoDowngraded) return;
      console.info("[RealtimeTransport] attempting WS recovery");
      isAutoDowngraded = false; // 临时解除降级标记，让 electMode 选 ws
      backend?.stop();
      backend = null;
      connectionState.value = "disconnected";
      transportMode.value = "unknown";
      wsFailureTimestamps = []; // 给 recovery 一次干净的机会
      setTimeout(() => reconnectInternal(), 0);
    }, WS_RECOVERY_CHECK_MS);
  }

  function clearDowngradeTimer() {
    if (downgradeTimer) {
      clearTimeout(downgradeTimer);
      downgradeTimer = null;
    }
  }

  /** 决定当前实际 mode（electMode + 自动降级叠加） */
  function resolveMode(): TransportMode {
    if (isAutoDowngraded) {
      // 降级状态：用 electMode 但强制 http-poll
      return "http-poll";
    }
    return electMode();
  }

  function ensureBackend(): Backend {
    if (backend) return backend;
    const mode = resolveMode();
    transportMode.value = mode;
    const emit = (type: string, data: any) => {
      // 🆕 v4 M1：tab 不在 Files 时丢 file:change（Files.vue 300ms 防抖兜底）
      if (type === "file:change" && !fileChangeGateActive) {
        return;
      }
      eventBus.emit(type as any, data);
      // 2026-06-14 增强：ws 模式下的关闭事件 → 记录失败，可能触发降级
      if (type === "server:connection-error" && mode === "ws") {
        recordWsFailure();
      }
    };
    switch (mode) {
      case "native-bridge":
        backend = createNativeBridgeBackend(emit);
        break;
      case "http-poll":
        backend = createHttpPollBackend(emit, {
          onConnected: () => {
            connectionState.value = "connected";
          },
        });
        break;
      case "ws":
        backend = createWsBackend(emit, {
          onConnected: () => {
            connectionState.value = "connected";
          },
          onDisconnected: () => {
            connectionState.value = "disconnected";
          },
        });
        break;
    }
    return backend!;
  }

  // 首次 connect 注册 baseUrl 监听
  function ensureListeners() {
    if (cleanupListeners) return;
    cleanupListeners = ensureApiBaseListeners({
      connect: () => {},
      disconnect: () => {},
      forceReconnect: () => {},
      connectionState,
      transportMode,
      isSandboxBrowser,
      setFileChangeGate: () => {},
      __forceMode: () => {},
      __resetForTesting: () => {},
    });
  }

  // 2026-06-14 修复：原代码在 setTimeout 回调中调用 connect()，但 connect
  // 是返回对象的方法名，闭包捕获不到（undefined）。改为内部函数 doConnect。
  function doConnect() {
    if (connectionState.value === "connecting" || connectionState.value === "connected") return;
    ensureListeners();
    connectionState.value = "connecting";
    ensureBackend().start();
  }

  function doDisconnect() {
    backend?.stop();
    backend = null;
    connectionState.value = "disconnected";
    transportMode.value = "unknown";
    // 2026-06-14：清理降级 timer，避免内存泄漏
    clearDowngradeTimer();
  }

  function doForceReconnect() {
    backend?.stop();
    backend = null;
    doConnect();
  }

  function doForceMode(mode: TransportMode | null) {
    _forcedMode = mode;
    backend?.stop();
    backend = null;
    transportMode.value = "unknown";
    connectionState.value = "disconnected";
  }

  // 供 setTimeout 调用的内部重连（不重复 start 检查）
  function reconnectInternal() {
    backend?.stop();
    backend = null;
    doConnect();
  }

  return {
    connect: doConnect,
    disconnect: doDisconnect,
    forceReconnect: doForceReconnect,
    connectionState: connectionState as Readonly<Ref<ConnectionState>>,
    transportMode: transportMode as Readonly<Ref<TransportMode>>,
    isSandboxBrowser: isSandboxBrowser as Readonly<Ref<boolean>>,
    /**
     * 🆕 v4 M1：设置 file:change 事件 tab active gate
     * - true（默认）：转发 file:change 给消费者
     * - false：丢弃 file:change（tab 不可见时用，避免 Files tab 在后台被狂刷）
     * @param active tab 是否可见
     * @param onActive 切回可见时的回调（用于触发一次性 loadFiles）
     */
    setFileChangeGate(active: boolean, onActive?: () => void) {
      const wasActive = fileChangeGateActive;
      fileChangeGateActive = active;
      onFilesTabActiveCallback = onActive ?? null;
      // 切回 true 立即触发 onActive（让 caller 补一次 reload）
      if (!wasActive && active && onFilesTabActiveCallback) {
        try {
          onFilesTabActiveCallback();
        } catch (e) {
          console.warn("[RealtimeTransport] onFilesTabActive callback error:", e);
        }
      }
    },
    __forceMode: doForceMode,
    __resetForTesting() {
      backend?.stop();
      backend = null;
      cleanupListeners?.();
      cleanupListeners = null;
      transportMode.value = "unknown";
      connectionState.value = "disconnected";
      clearDowngradeTimer();
      isAutoDowngraded = false;
      wsFailureTimestamps = [];
      _instance = null;
      _forcedMode = null;
    },
  };
}

export function useRealtimeTransport(): RealtimeTransport {
  if (!_instance) _instance = createTransport();
  return _instance;
}

/** 内部使用：test/console log 拿当前 transport mode（不影响生产逻辑） */
export function getActiveTransportMode(): TransportMode {
  return electMode();
}

/** 内部使用：debug 打印当前 baseUrl + transport mode */
export function getTransportDebugInfo(): { mode: TransportMode; baseUrl: string; isSandboxBrowser: boolean; isNative: boolean } {
  return {
    mode: electMode(),
    baseUrl: getApiBaseUrl(),
    isSandboxBrowser: isOpenPreviewBrowser(),
    isNative: isNative(),
  };
}
