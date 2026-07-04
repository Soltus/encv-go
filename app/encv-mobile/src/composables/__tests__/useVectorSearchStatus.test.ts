import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  fetchResponse: {
    ok: true as boolean,
    json: vi.fn(),
  },
  fetch: vi.fn(),
  on: vi.fn(),
  off: vi.fn(),
  emit: vi.fn(),
  apiBaseUrl: "",
}));

vi.mock("@/api/encv", () => ({
  getApiBaseUrl: () => mocks.apiBaseUrl,
}));

vi.mock("@/composables/useEventBus", () => ({
  eventBus: { on: mocks.on, off: mocks.off, emit: mocks.emit },
}));

// 全局 fetch 用我们的 mock（useVectorSearchStatus 内部用 fetch 调用 /api/runtime）
(globalThis as any).fetch = mocks.fetch;

import { useVectorSearchStatus } from "@/composables/useVectorSearchStatus";

describe("useVectorSearchStatus", () => {
  beforeEach(() => {
    mocks.fetch.mockReset();
    mocks.fetch.mockResolvedValue(mocks.fetchResponse);
    mocks.fetchResponse.ok = true;
    mocks.fetchResponse.json.mockReset();
    mocks.fetchResponse.json.mockResolvedValue({});
    mocks.on.mockClear();
    mocks.off.mockClear();
    mocks.apiBaseUrl = "";
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('探测返回 vector_search_available=true, degraded=false → status="available"', async () => {
    mocks.fetchResponse.json.mockResolvedValue({
      vector_search_available: true,
      vector_search_degraded: false,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("available");
  });

  it('探测返回 degraded=true → status="degraded"', async () => {
    mocks.fetchResponse.json.mockResolvedValue({
      vector_search_available: true,
      vector_search_degraded: true,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("degraded");
  });

  it('探测返回 available=false → status="unavailable"', async () => {
    mocks.fetchResponse.json.mockResolvedValue({
      vector_search_available: false,
      vector_search_degraded: false,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("unavailable");
  });

  it("探测失败：状态不应变为 available/degraded（保持 unknown 或前值）", async () => {
    mocks.fetch.mockRejectedValue(new Error("network error"));
    const { status, refresh } = useVectorSearchStatus();
    const before = status.value;
    await refresh();
    expect(status.value).toBe(before);
    expect(status.value).not.toBe("available");
    expect(status.value).not.toBe("degraded");
  });

  it('探测返回 available=undefined → "unavailable"（保守）', async () => {
    mocks.fetchResponse.json.mockResolvedValue({});
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("unavailable");
  });

  it("同时只能有一个 in-flight 请求（pollInFlight 保护）", async () => {
    let resolveFn: (v: any) => void = () => {};
    mocks.fetchResponse.json.mockReturnValue(
      new Promise<any>(r => {
        resolveFn = r;
      })
    );
    const { refresh } = useVectorSearchStatus();
    const p1 = refresh();
    const p2 = refresh();
    const p3 = refresh();
    // 三个并发 refresh 只触发一次 fetch
    expect(mocks.fetch).toHaveBeenCalledTimes(1);
    resolveFn({ vector_search_available: true, vector_search_degraded: false });
    await Promise.all([p1, p2, p3]);
  });

  it("状态切换：available → degraded 后再次 refresh 仍能正确反映", async () => {
    mocks.fetchResponse.json.mockResolvedValueOnce({
      vector_search_available: true,
      vector_search_degraded: false,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("available");

    mocks.fetchResponse.json.mockResolvedValueOnce({
      vector_search_available: true,
      vector_search_degraded: true,
    });
    await refresh();
    expect(status.value).toBe("degraded");
  });

  it("HTTP 4xx/5xx：保持原状态不变", async () => {
    // 先把状态变为 available
    mocks.fetchResponse.json.mockResolvedValueOnce({
      vector_search_available: true,
      vector_search_degraded: false,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("available");

    // 然后下一次响应非 ok
    mocks.fetchResponse.ok = false;
    mocks.fetchResponse.json.mockResolvedValueOnce({});
    const before = status.value;
    await refresh();
    expect(status.value).toBe(before);
  });

  it("请求 URL 包含 /api/runtime", async () => {
    mocks.apiBaseUrl = "/test-base";
    mocks.fetchResponse.json.mockResolvedValue({
      vector_search_available: true,
      vector_search_degraded: false,
    });
    const { refresh } = useVectorSearchStatus();
    await refresh();
    expect(mocks.fetch).toHaveBeenCalledWith("/test-base/api/runtime", expect.objectContaining({ method: "GET" }));
  });
});
