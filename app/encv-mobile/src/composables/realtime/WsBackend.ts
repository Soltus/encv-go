/**
 * WsBackend — WebSocket transport 实现
 *
 * 历史：
 *   - 之前 [useWebSocket.ts](file:///workspace/app/encv-mobile/src/composables/useWebSocket.ts) 整个文件
 *   - 现在迁到 realtime/WsBackend.ts 内部 backend
 *   - 公共 useRealtimeTransport() 单例负责生命周期
 *
 * 行为：
 *   - 选举 ws 模式后 → 调 start() 连 WS
 *   - 收到消息 → 解析后 emit('task:update', data) 等
 *   - 断线 → exp backoff 重连（1s → 2s → 4s → ... → max 30s）+ jitter
 *   - 心跳：30s 一次 ping，10s 内没回 pong → 强制重连
 *   - 详细日志：onclose 的 event.code / event.reason / event.wasClean 全部打印
 *
 * 2026-06-14 增强（安卓真机 WS 稳定性）：
 *   1. **详细 close 日志**：event.code (1006=异常, 1001=going away, 1000=正常) +
 *      event.reason + event.wasClean — 排查 readyState=3 必须有这些
 *   2. **visibilitychange 处理**：Android WebView 切后台后系统可能杀掉长连接，
 *      切回前台时强制 forceReconnect
 *   3. **pagehide 处理**：浏览器/WebView 切 tab/关页时主动 close
 *   4. **jitter 抖动**：重连 delay 加 ±25% 随机抖动，避免多个客户端同时重连
 *   5. **connect guard 修复**：旧代码只跳过 OPEN/CONNECTING，没处理 CLOSING
 *   6. **stop() 状态重置**：彻底清理所有 timer + 监听器，避免内存泄漏
 *
 * 防御（不应到达，但保留）：
 *   - OpenPreview 浏览器理论上被 electMode 分流到 http-poll
 *   - 但如果 _forcedMode = 'ws' 强制切过来，调用 start() 时再 emit online:true 静默退出
 */

import { getWebSocketUrl, isOpenPreviewBrowser } from "@/api/encv";
import type { Backend, EventEmitter } from "./Backend";

interface WsMessage {
  type: string;
  data: any;
}

/** ws 已知事件集合（其他事件也通过 ws:message 透传） */
const KNOWN_WS_EVENTS = new Set([
  "task:update",
  "task:progress",
  "task:created",
  "task:completed",
  "file:change",
  "server:status",
  "server:connection-error",
  "log:message",
]);

export interface WsBackendOptions {
  /** WS 连接成功回调（给 transport 改 connectionState） */
  onConnected?: () => void;
  /** WS 断开回调（不区分 clean close） */
  onDisconnected?: () => void;
  /** 注入 fetch（测试用） */
  fetch?: typeof fetch;
}

const HEARTBEAT_INTERVAL = 30000;
const PONG_TIMEOUT = 10000;
const MAX_RECONNECT_DELAY = 30000;
const RECONNECT_JITTER_RATIO = 0.25; // ±25% 抖动

/**
 * 给定一个毫秒数，加 ±25% 随机抖动避免雷鸣群。
 * 输入 1000 → 输出 750~1250；输入 30000 → 输出 22500~37500
 */
function withJitter(ms: number): number {
  const jitter = ms * RECONNECT_JITTER_RATIO;
  return Math.round(ms + (Math.random() * 2 - 1) * jitter);
}

export function createWsBackend(emit: EventEmitter, options: WsBackendOptions = {}): Backend {
  let ws: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  let pongTimeoutTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectDelay = 1000;
  let running = false;

  // visibilitychange / pagehide 监听器引用（用于 stop 时清理）
  let visibilityHandler: (() => void) | null = null;
  let pageHideHandler: (() => void) | null = null;

  function clearReconnectTimer() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  }

  function stopHeartbeat() {
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer);
      heartbeatTimer = null;
    }
    if (pongTimeoutTimer) {
      clearTimeout(pongTimeoutTimer);
      pongTimeoutTimer = null;
    }
  }

  function startHeartbeat() {
    stopHeartbeat();
    heartbeatTimer = setInterval(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        try {
          ws.send(JSON.stringify({ type: "ping" }));
          pongTimeoutTimer = setTimeout(() => {
            console.warn("[WsBackend] pong timeout — forcing reconnect (Android 网络层可能静默断连)");
            forceReconnect("pong-timeout");
          }, PONG_TIMEOUT);
        } catch (e) {
          console.warn("[WsBackend] heartbeat send failed:", e instanceof Error ? e.message : String(e));
          forceReconnect("heartbeat-send-failed");
        }
      }
    }, HEARTBEAT_INTERVAL);
  }

  function handleMessage(event: MessageEvent) {
    try {
      const msg: WsMessage = JSON.parse(event.data);

      if (msg.type === "pong") {
        if (pongTimeoutTimer) {
          clearTimeout(pongTimeoutTimer);
          pongTimeoutTimer = null;
        }
        return;
      }

      if (KNOWN_WS_EVENTS.has(msg.type)) {
        emit(msg.type, msg.data);
      }
      emit("ws:message", { type: msg.type, data: msg.data });
    } catch (e) {
      console.error(
        "[WsBackend] Failed to parse message:",
        e instanceof Error ? `${e.name}: ${e.message}` : String(e),
        "raw=",
        String(event.data).slice(0, 200)
      );
    }
  }

  function connect() {
    if (!running) return;
    // 关键修复（2026-06-14）：旧代码只跳过 OPEN/CONNECTING，没处理 CLOSING
    // CLOSING 状态下 ws 已调用 close() 但 server 没收到 close 帧的过渡态，
    // 此时如果再 new WebSocket，会出现两个 ws 共存，新旧 ws 抢通道。
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.CLOSING)) {
      return;
    }

    // 🆕 2026-06-10 沙箱 OpenPreview 浏览器：trae 反代 :16000 不支持 WebSocket upgrade
    //   防御：理论上 electMode 已分流，但 _forcedMode 强制时可能落到这里
    if (isOpenPreviewBrowser()) {
      console.info("[WsBackend] OpenPreview browser detected, skipping WebSocket.");
      emit("server:status", { online: true });
      return;
    }

    const url = getWebSocketUrl();
    const origin = typeof location !== "undefined" ? location.origin : "n/a";
    const ua = typeof navigator !== "undefined" ? (navigator.userAgent || "").slice(0, 80) : "n/a";
    console.info(`[WsBackend] connecting to ${url} (origin=${origin} ua=${ua})`);

    try {
      ws = new WebSocket(url);
    } catch (e) {
      const msg = e instanceof Error ? `${e.name}: ${e.message}` : String(e);
      console.error(`[WsBackend] Failed to create WebSocket: ${msg} url=${url}`);
      scheduleReconnect();
      return;
    }

    ws.onopen = () => {
      reconnectDelay = 1000;
      startHeartbeat();
      console.info(`[WsBackend] connected to ${url}`);
      options.onConnected?.();
      emit("server:status", { online: true });
    };

    ws.onmessage = handleMessage;

    // 🆕 2026-06-14：详细 close 日志 — 排查 readyState=3 必备
    ws.onclose = event => {
      // 关键诊断信息：
      //   - code: 1006 = abnormal closure (网络层断)
      //   - code: 1001 = going away (server shutdown / client navigate)
      //   - code: 1000 = normal closure
      //   - reason: server 发来的 close reason
      //   - wasClean: 是否发送了 close 帧
      const diagnostic = `code=${event.code} reason='${event.reason || "(empty)"}' wasClean=${event.wasClean} readyState=${ws?.readyState}`;
      console.warn(`[WsBackend] closed url=${url} ${diagnostic}`);

      options.onDisconnected?.();
      emit("server:status", { online: false });

      if (running) {
        stopHeartbeat();
        scheduleReconnect();
      }

      // 只在非正常关闭时弹 connection-error（避免正常关闭的噪音）
      // wasClean=true 且 code=1000 是 server 主动 close 或 client disconnect
      if (!event.wasClean || event.code !== 1000) {
        emit("server:connection-error", { error: `Connection closed (${diagnostic})` });
      }
    };

    ws.onerror = () => {
      // onerror 触发时 readyState 已经是 CLOSING 或 CLOSED（3 是 CLOSED）
      // 真正的诊断信息在 onclose 里 — 这里只记 URL + readyState
      console.error(`[WsBackend] WebSocket error: url=${url} readyState=${ws?.readyState}`);
      // 不要在这里 emit server:connection-error — onclose 会发，避免重复
    };
  }

  function disconnect() {
    clearReconnectTimer();
    stopHeartbeat();
    if (ws) {
      // 清空 handler 避免 onclose 触发 scheduleReconnect
      ws.onclose = null;
      ws.onerror = null;
      ws.onmessage = null;
      ws.onopen = null;
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        try {
          ws.close(1000, "Client disconnect");
        } catch {
          // 某些异常 readyState 下 close() 会抛 — 静默吞
        }
      }
      ws = null;
    }
  }

  function scheduleReconnect() {
    if (!running) return;
    if (reconnectTimer) return;
    const delay = withJitter(reconnectDelay);
    console.info(`[WsBackend] scheduling reconnect in ${delay}ms (base=${reconnectDelay}ms, jittered)`);
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      connect();
    }, delay);
    // 指数退避：1s → 2s → 4s → 8s → 16s → 30s (cap)
    reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY);
  }

  function forceReconnect(reason: string) {
    console.warn(`[WsBackend] forceReconnect reason=${reason}`);
    disconnect();
    reconnectDelay = 1000;
    if (running) {
      connect();
    }
  }

  /**
   * 🆕 2026-06-14：visibilitychange 监听 — Android WebView 切后台后系统
   * 可能杀掉长连接，切回前台时强制重连。
   */
  function setupVisibilityHandlers() {
    if (typeof document === "undefined") return;

    visibilityHandler = () => {
      if (document.visibilityState === "visible" && running) {
        const state = ws?.readyState;
        // 不可见时如果 ws 还在 / 切回时 ws 已关 / 切回时 ws 在 CLOSING 都需要重连
        if (state === WebSocket.CLOSED || state === WebSocket.CLOSING || state === undefined) {
          console.info(`[WsBackend] visibilitychange → visible, ws state=${state}, forcing reconnect`);
          forceReconnect("visibilitychange-to-visible");
        } else {
          console.info(`[WsBackend] visibilitychange → visible, ws state=${state}, no action needed`);
        }
      }
    };
    document.addEventListener("visibilitychange", visibilityHandler);

    // pagehide：用户切到其他 app / 关页 — 主动 close，节省资源
    pageHideHandler = () => {
      if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
        try {
          ws.close(1000, "page hidden");
        } catch {
          // 忽略
        }
      }
    };
    if (typeof window !== "undefined") {
      window.addEventListener("pagehide", pageHideHandler);
    }
  }

  function teardownVisibilityHandlers() {
    if (typeof document !== "undefined" && visibilityHandler) {
      document.removeEventListener("visibilitychange", visibilityHandler);
    }
    if (typeof window !== "undefined" && pageHideHandler) {
      window.removeEventListener("pagehide", pageHideHandler);
    }
    visibilityHandler = null;
    pageHideHandler = null;
  }

  return {
    start() {
      if (running) return;
      running = true;
      reconnectDelay = 1000;
      setupVisibilityHandlers();
      connect();
    },
    stop() {
      running = false;
      teardownVisibilityHandlers();
      disconnect();
    },
    reset() {
      disconnect();
      reconnectDelay = 1000;
    },
  };
}
