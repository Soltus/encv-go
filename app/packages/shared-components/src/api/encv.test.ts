/**
 * checkServerStatus.test.ts
 *
 * 验证 instance_id 防劫持机制：
 *  1. 200 + JSON + 完整 PingResponse → online，持久化 instance_id
 *  2. 200 + JSON + 缺 instance_id → offline（不是 encv-go）
 *  3. 200 + text/html → offline（Vite SPA fallback，保留老逻辑）
 *  4. 200 + JSON + instance_id 与 localStorage 不一致 → "backend instance changed"（hijack 警告）
 *  5. 首次探测（无 localStorage）→ online + 持久化
 *  6. status != 'ok' → offline
 *
 * Mock 策略：
 *  - global.fetch 模拟 /ping 响应
 *  - localStorage 用真实实现（happy-dom 提供）
 */

import { checkServerStatus, getPersistedBackendIdentity } from "@encv/shared-components/api/encv";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

function makePingResponse(
  overrides: Partial<{
    status: string;
    version: string;
    instance_id: string;
    server_dir: string;
    webdav_dir: string;
  }> = {}
) {
  return {
    status: "ok",
    version: "0.0.0-test",
    instance_id: "inst-test-uuid-001",
    server_dir: "/tmp/encv-test",
    webdav_dir: "",
    ...overrides,
  };
}

function makeResponse(body: object | string, opts: { status?: number; contentType?: string } = {}): Response {
  const { status = 200, contentType = "application/json" } = opts;
  const isJson = typeof body === "object";
  return new Response(isJson ? JSON.stringify(body) : body, { status, headers: { "Content-Type": contentType } });
}

describe("checkServerStatus — instance_id hijack detection", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    localStorage.clear();
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("200 + JSON + 完整 PingResponse → online + 持久化 instance_id", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse(makePingResponse()));
    const r = await checkServerStatus();
    expect(r.online).toBe(true);
    expect(r.instanceId).toBe("inst-test-uuid-001");
    expect(r.version).toBe("0.0.0-test");
    const persisted = getPersistedBackendIdentity();
    expect(persisted?.instanceId).toBe("inst-test-uuid-001");
    expect(persisted?.version).toBe("0.0.0-test");
  });

  it("200 + JSON + 缺 instance_id → offline（不是 encv-go）", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse({ status: "ok", version: "1.0" })); // 没 instance_id
    const r = await checkServerStatus();
    expect(r.online).toBe(false);
    expect(r.error).toMatch(/missing required fields/);
  });

  it("200 + text/html → offline（Vite SPA fallback 保留）", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse("<!doctype html><html>vite</html>", { contentType: "text/html" }));
    const r = await checkServerStatus();
    expect(r.online).toBe(false);
    expect(r.error).toMatch(/vite SPA fallback/);
  });

  it("200 + JSON + 缺 status 字段 → offline（不是 encv-go）", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse({ instance_id: "x" }));
    const r = await checkServerStatus();
    expect(r.online).toBe(false);
    expect(r.error).toMatch(/missing required fields/);
  });

  it("200 + JSON + status !== ok → offline", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse(makePingResponse({ status: "error" })));
    const r = await checkServerStatus();
    expect(r.online).toBe(false);
    expect(r.error).toMatch(/ping status=error/);
  });

  it("5xx → offline（HTTP 错误）", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse("Internal Server Error", { status: 500, contentType: "text/plain" }));
    const r = await checkServerStatus();
    expect(r.online).toBe(false);
    expect(r.error).toBe("HTTP 500");
  });

  it("跨会话：localStorage 有 instance_id，新响应 instance_id 不同 → hijack 警告", async () => {
    // 模拟上一进程
    localStorage.setItem("encv-server-instance-id", "inst-old-process-999");
    localStorage.setItem("encv-server-version", "0.0.0-old");
    // 当前进程（新响应）
    fetchMock.mockResolvedValueOnce(makeResponse(makePingResponse({ instance_id: "inst-new-process-111", version: "0.0.0-new" })));
    const r = await checkServerStatus();
    expect(r.online).toBe(false);
    expect(r.error).toMatch(/backend instance changed/);
    expect(r.error).toMatch(/possible port hijack/);
    // **不应** 覆盖 localStorage 旧值（hijack 模式下保留 forensic evidence）
    expect(localStorage.getItem("encv-server-instance-id")).toBe("inst-old-process-999");
  });

  it("跨会话：localStorage instance_id 一致 → online（正常续连）", async () => {
    localStorage.setItem("encv-server-instance-id", "inst-same-process-777");
    fetchMock.mockResolvedValueOnce(makeResponse(makePingResponse({ instance_id: "inst-same-process-777" })));
    const r = await checkServerStatus();
    expect(r.online).toBe(true);
    expect(r.instanceId).toBe("inst-same-process-777");
  });

  it("首次探测（localStorage 空）→ online + 持久化", async () => {
    // 明确清空
    expect(localStorage.getItem("encv-server-instance-id")).toBeNull();
    fetchMock.mockResolvedValueOnce(makeResponse(makePingResponse({ instance_id: "first-time-123" })));
    const r = await checkServerStatus();
    expect(r.online).toBe(true);
    expect(localStorage.getItem("encv-server-instance-id")).toBe("first-time-123");
  });

  it("JSON 解析失败 → offline（不是 encv-go）", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse("not json{", { contentType: "application/json" }));
    const r = await checkServerStatus();
    expect(r.online).toBe(false);
    expect(r.error).toMatch(/missing required fields/);
  });

  it("fetch reject → offline（网络层错误）", async () => {
    fetchMock.mockRejectedValueOnce(new Error("NetworkError"));
    const r = await checkServerStatus();
    expect(r.online).toBe(false);
    expect(r.error).toMatch(/NetworkError/);
  });

  it("fetch 命中 /ping 路径（不是 /api/config）", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse(makePingResponse()));
    await checkServerStatus();
    const calledUrl = fetchMock.mock.calls[0]?.[0] as string;
    expect(calledUrl).toMatch(/\/ping$/);
    // 显式确认：不再用 /api/config
    expect(calledUrl).not.toMatch(/\/api\/config/);
  });

  it("fetch 强制 no-cache 头（避免 HTTP 缓存让旧 instance_id 错认）", async () => {
    fetchMock.mockResolvedValueOnce(makeResponse(makePingResponse()));
    await checkServerStatus();
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.cache).toBe("no-store");
    expect((init.headers as Record<string, string>)?.Accept).toBe("application/json");
  });
});
