/**
 * useRealtimeTransport 单元测试（2026-06-10）
 *
 * 覆盖：
 *  1. 选举 transport 模式：forced / native / OpenPreview / 默认
 *  2. 单例：多次 useRealtimeTransport() 返回同一实例
 *  3. connect/disconnect 切换 connectionState
 *  4. forceReconnect 重置 backend
 *  5. http-poll 模式实际启动 HttpPollBackend
 */

import type { EncvTask } from "@encv/shared-components/api/encv";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// mock api/encv 模块的 isOpenPreviewBrowser / getApiBaseUrl
const _isOpenPreviewBrowser = vi.fn().mockReturnValue(false);
const _isNativePlatform = vi.fn().mockReturnValue(false);

// ⚠️ 2026-10-02 修正：mock 路径必须**与实现 import 的模块路径完全一致**。
//    实现（useRealtimeTransport.ts）import 的是 `@encv/shared-components/api/encv`，
//    而这里原来 mock 的是 `@/api/encv`（encv-mobile 的旧路径，实现根本不 import 它）
//    ⇒ mock 完全没生效，实现调的是真实的 isOpenPreviewBrowser()（返回 false）
//    ⇒ "OpenPreview → http-poll" 这条用例必然失败（拿到 ws）。
//    这是**测试的 mock 打错地方**，不是实现的选举逻辑有问题
//    （实现里 `if (isOpenPreviewBrowser()) return "http-poll"` 一直在）。
vi.mock("@encv/shared-components/api/encv", async () => {
  const actual = await vi.importActual<any>("@encv/shared-components/api/encv");
  return {
    ...actual,
    isOpenPreviewBrowser: () => _isOpenPreviewBrowser(),
    getApiBaseUrl: () => "http://127.0.0.1:2025",
    getWebSocketUrl: () => "ws://127.0.0.1:2025/ws",
    getTasks: vi.fn().mockResolvedValue([] as EncvTask[]),
  };
});

// mock useEventBus（同样要用实现真正 import 的 shared 路径）
vi.mock("@encv/shared-components/composables/useEventBus", () => ({
  eventBus: {
    emit: vi.fn(),
    on: vi.fn(),
    off: vi.fn(),
  },
}));

import {
  getActiveTransportMode,
  getTransportDebugInfo,
  useRealtimeTransport,
} from "@encv/shared-components/composables/useRealtimeTransport";

describe("useRealtimeTransport", () => {
  beforeEach(() => {
    // 重置单例 + forced mode
    const t = useRealtimeTransport();
    t.__resetForTesting();
    _isOpenPreviewBrowser.mockReturnValue(false);
    _isNativePlatform.mockReturnValue(false);
    // 清 Capacitor mock
    delete (window as any).Capacitor;
  });

  afterEach(() => {
    const t = useRealtimeTransport();
    t.__resetForTesting();
    delete (window as any).Capacitor;
  });

  it("returns singleton", () => {
    const t1 = useRealtimeTransport();
    const t2 = useRealtimeTransport();
    expect(t1).toBe(t2);
  });

  it("default mode = ws (when not native and not OpenPreview)", () => {
    const t = useRealtimeTransport();
    t.connect();
    expect(t.transportMode.value).toBe("ws");
  });

  it("OpenPreview browser → http-poll", () => {
    _isOpenPreviewBrowser.mockReturnValue(true);
    const t = useRealtimeTransport();
    t.connect();
    expect(t.transportMode.value).toBe("http-poll");
  });

  it("Capacitor native → ws (NOT native-bridge, 2026-06-11 设计变更)", () => {
    // 2026-06-11 决策：Capacitor 真机走 ws 模式，不用 native-bridge
    // 原因：NativeBridgeBackend 是空壳（start() 只 emit online:true，无事件转发）——
    //       走 native-bridge 意味着真机 task 进度永远不更新
    //       Capacitor WebView = Android System WebView / iOS WKWebView = Chrome/Safari 内核
    //       → WebSocket 原生支持，直连 127.0.0.1:2025 即可
    (window as any).Capacitor = { isNativePlatform: () => true };
    const t = useRealtimeTransport();
    t.connect();
    expect(t.transportMode.value).toBe("ws");
  });

  it("forced mode overrides election", () => {
    _isOpenPreviewBrowser.mockReturnValue(true); // 默认会选 http-poll
    const t = useRealtimeTransport();
    t.__forceMode("ws");
    t.connect();
    expect(t.transportMode.value).toBe("ws");
    t.__forceMode(null); // 恢复默认
  });

  it("isSandboxBrowser reflects isOpenPreviewBrowser", () => {
    _isOpenPreviewBrowser.mockReturnValue(true);
    const t = useRealtimeTransport();
    expect(t.isSandboxBrowser.value).toBe(true);
  });

  it("disconnect resets connectionState", () => {
    const t = useRealtimeTransport();
    t.connect();
    expect(t.connectionState.value).toBe("connecting");
    t.disconnect();
    expect(t.connectionState.value).toBe("disconnected");
    expect(t.transportMode.value).toBe("unknown");
  });

  it("forceReconnect disposes old backend and creates new", () => {
    const t = useRealtimeTransport();
    t.connect();
    expect(t.transportMode.value).toBe("ws");
    t.forceReconnect();
    expect(t.connectionState.value).toBe("connecting");
  });

  it("getActiveTransportMode returns current election (without instance creation)", () => {
    _isOpenPreviewBrowser.mockReturnValue(false);
    expect(getActiveTransportMode()).toBe("ws");
    _isOpenPreviewBrowser.mockReturnValue(true);
    // forced mode 不影响 getActiveTransportMode（但 transport instance 已被 __resetForTesting 清空）
    const t = useRealtimeTransport();
    t.__resetForTesting();
    expect(getActiveTransportMode()).toBe("http-poll");
  });

  it("getTransportDebugInfo returns debug snapshot", () => {
    _isOpenPreviewBrowser.mockReturnValue(false);
    const info = getTransportDebugInfo();
    expect(info).toHaveProperty("mode");
    expect(info).toHaveProperty("baseUrl");
    expect(info).toHaveProperty("isSandboxBrowser");
    expect(info).toHaveProperty("isNative");
  });
});
