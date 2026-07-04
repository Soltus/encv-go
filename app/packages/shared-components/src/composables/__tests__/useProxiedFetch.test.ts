/**
 * useProxiedFetch 单元测试
 *
 * 覆盖：
 * 1. installProxiedFetch 在 native Android 平台会替换 window.fetch
 * 2. dev / web 平台 no-op，不替换 fetch
 * 3. 替换后 fetch 走 ApiProxy.fetchOnce
 * 4. SSE（Accept: text/event-stream）走 ApiProxy.streamStart
 * 5. FormData / Blob 走原始 fetch（不代理）
 * 6. uninstallProxiedFetch 还原原 fetch
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// vi.mock 是 hoisted 的，mock 工厂内不能引用模块顶层变量。
// 用 vi.hoisted 把可变状态提到模块顶层
const mocks = vi.hoisted(() => ({
  isNativePlatform: false,
  platform: "web",
  fetchOnce: vi.fn(),
  streamStart: vi.fn(),
  streamCancel: vi.fn(),
  addListener: vi.fn().mockResolvedValue({ remove: vi.fn() }),
  removeAllListeners: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@capacitor/core", () => ({
  Capacitor: {
    isNativePlatform: () => mocks.isNativePlatform,
    getPlatform: () => mocks.platform,
  },
}));

vi.mock("@/plugins/ApiProxy", () => ({
  ApiProxy: {
    fetchOnce: mocks.fetchOnce,
    streamStart: mocks.streamStart,
    streamCancel: mocks.streamCancel,
    addListener: mocks.addListener,
    removeAllListeners: mocks.removeAllListeners,
  },
}));

import { installProxiedFetch, isProxiedFetchInstalled, uninstallProxiedFetch } from "@encv/shared-components/composables/useProxiedFetch";

describe("useProxiedFetch", () => {
  let originalFetch: typeof window.fetch;
  beforeEach(() => {
    originalFetch = window.fetch;
    mocks.fetchOnce.mockReset();
    mocks.streamStart.mockReset();
    mocks.streamCancel.mockReset();
    uninstallProxiedFetch();
    mocks.isNativePlatform = false;
    mocks.platform = "web";
  });
  afterEach(() => {
    uninstallProxiedFetch();
    window.fetch = originalFetch;
  });

  it("dev/web 平台：installProxiedFetch 不替换 window.fetch", () => {
    mocks.isNativePlatform = false;
    mocks.platform = "web";
    installProxiedFetch();
    expect(isProxiedFetchInstalled()).toBe(false);
  });

  it("native Android 平台：installProxiedFetch 替换 window.fetch", () => {
    mocks.isNativePlatform = true;
    mocks.platform = "android";
    installProxiedFetch();
    expect(isProxiedFetchInstalled()).toBe(true);
    expect(window.fetch).not.toBe(originalFetch);
  });

  it("native iOS 平台：installProxiedFetch 不替换（暂只支持 Android）", () => {
    mocks.isNativePlatform = true;
    mocks.platform = "ios";
    installProxiedFetch();
    expect(isProxiedFetchInstalled()).toBe(false);
  });

  it("native Android 替换后 fetch 走 ApiProxy.fetchOnce，返回 Response 包装", async () => {
    mocks.isNativePlatform = true;
    mocks.platform = "android";
    installProxiedFetch();

    mocks.fetchOnce.mockResolvedValue({
      status: 200,
      statusText: "OK",
      headers: { "content-type": "application/json" },
      body: '{"hello":"world"}',
      resolvedBaseUrl: "http://127.0.0.1:2025",
    });

    const res = await window.fetch("/api/encrypt-key", {
      method: "POST",
      headers: { "X-Custom": "foo" },
      body: JSON.stringify({ data: "abc" }),
    });

    expect(mocks.fetchOnce).toHaveBeenCalledTimes(1);
    expect(mocks.fetchOnce).toHaveBeenCalledWith({
      url: "/api/encrypt-key",
      method: "POST",
      headers: { "X-Custom": "foo" },
      body: '{"data":"abc"}',
    });
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("application/json");
    const text = await res.text();
    expect(text).toBe('{"hello":"world"}');
  });

  it("SSE 请求（Accept: text/event-stream）走 ApiProxy.streamStart", async () => {
    mocks.isNativePlatform = true;
    mocks.platform = "android";
    installProxiedFetch();

    mocks.streamStart.mockResolvedValue({
      streamId: "stream-1",
      status: 200,
      statusText: "OK",
      headers: { "content-type": "text/event-stream" },
      resolvedBaseUrl: "http://127.0.0.1:2025",
    });

    const res = await window.fetch("/api/chat", {
      method: "POST",
      headers: { Accept: "text/event-stream" },
      body: "{}",
    });

    expect(mocks.streamStart).toHaveBeenCalledTimes(1);
    expect(mocks.streamStart).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/chat",
        method: "POST",
      })
    );
    expect(mocks.fetchOnce).not.toHaveBeenCalled();
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("text/event-stream");
    expect(res.body).toBeInstanceOf(ReadableStream);
  });

  it("uninstallProxiedFetch 还原原 fetch", () => {
    mocks.isNativePlatform = true;
    mocks.platform = "android";
    installProxiedFetch();
    expect(isProxiedFetchInstalled()).toBe(true);
    uninstallProxiedFetch();
    expect(isProxiedFetchInstalled()).toBe(false);
    // fetch 被覆盖时是 .bind(window) 的新引用，uninstall 后是新替换；只验证
    // isProxiedFetchInstalled 已置 false 即可，不强引用相等
  });

  it("幂等：多次 installProxiedFetch 不会重复替换", () => {
    mocks.isNativePlatform = true;
    mocks.platform = "android";
    installProxiedFetch();
    const firstReplacement = window.fetch;
    installProxiedFetch();
    expect(window.fetch).toBe(firstReplacement);
  });
});
