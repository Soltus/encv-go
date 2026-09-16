import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiRequest: vi.fn(),
  on: vi.fn(),
  off: vi.fn(),
  emit: vi.fn(),
}));

vi.mock("@encv/shared-components/api/core/request", () => ({
  apiRequest: mocks.apiRequest,
}));

vi.mock("@encv/shared-components/composables/useEventBus", () => ({
  eventBus: { on: mocks.on, off: mocks.off, emit: mocks.emit },
}));

import { useVectorSearchStatus } from "@encv/shared-components/composables/useVectorSearchStatus";

describe("useVectorSearchStatus", () => {
  beforeEach(() => {
    mocks.apiRequest.mockReset();
    mocks.apiRequest.mockResolvedValue({});
    mocks.on.mockClear();
    mocks.off.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('探测返回 vector_search_available=true, degraded=false → status="available"', async () => {
    mocks.apiRequest.mockResolvedValue({
      vector_search_available: true,
      vector_search_degraded: false,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("available");
  });

  it('探测返回 degraded=true → status="degraded"', async () => {
    mocks.apiRequest.mockResolvedValue({
      vector_search_available: true,
      vector_search_degraded: true,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("degraded");
  });

  it('探测返回 available=false → status="unavailable"', async () => {
    mocks.apiRequest.mockResolvedValue({
      vector_search_available: false,
      vector_search_degraded: false,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("unavailable");
  });

  it("探测失败：状态不应变为 available/degraded（保持 unknown 或前值）", async () => {
    mocks.apiRequest.mockRejectedValue(new Error("network error"));
    const { status, refresh } = useVectorSearchStatus();
    const before = status.value;
    await refresh();
    expect(status.value).toBe(before);
    expect(status.value).not.toBe("available");
    expect(status.value).not.toBe("degraded");
  });

  it('探测返回 available=undefined → "unavailable"（保守）', async () => {
    mocks.apiRequest.mockResolvedValue({});
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("unavailable");
  });

  it("同时只能有一个 in-flight 请求（并发守卫保护）", async () => {
    let resolveFn: (v: any) => void = () => {};
    mocks.apiRequest.mockReturnValue(
      new Promise<any>(r => {
        resolveFn = r;
      })
    );
    const { refresh } = useVectorSearchStatus();
    const p1 = refresh();
    const p2 = refresh();
    const p3 = refresh();
    // 三个并发 refresh 只触发一次 apiRequest
    expect(mocks.apiRequest).toHaveBeenCalledTimes(1);
    resolveFn({ vector_search_available: true, vector_search_degraded: false });
    await Promise.all([p1, p2, p3]);
  });

  it("状态切换：available → degraded 后再次 refresh 仍能正确反映", async () => {
    mocks.apiRequest.mockResolvedValueOnce({
      vector_search_available: true,
      vector_search_degraded: false,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("available");

    mocks.apiRequest.mockResolvedValueOnce({
      vector_search_available: true,
      vector_search_degraded: true,
    });
    await refresh();
    expect(status.value).toBe("degraded");
  });

  it("HTTP 4xx/5xx（apiRequest 抛错）：保持原状态不变", async () => {
    // 先把状态变为 available
    mocks.apiRequest.mockResolvedValueOnce({
      vector_search_available: true,
      vector_search_degraded: false,
    });
    const { status, refresh } = useVectorSearchStatus();
    await refresh();
    expect(status.value).toBe("available");

    // 然后下一次 apiRequest 抛错（非 2xx 归一化为 ApiError）
    mocks.apiRequest.mockRejectedValueOnce(new Error("HTTP 500"));
    const before = status.value;
    await refresh();
    expect(status.value).toBe(before);
  });

  it("请求路径为 /api/runtime", async () => {
    mocks.apiRequest.mockResolvedValue({
      vector_search_available: true,
      vector_search_degraded: false,
    });
    const { refresh } = useVectorSearchStatus();
    await refresh();
    expect(mocks.apiRequest).toHaveBeenCalledWith("/api/runtime", expect.anything());
  });
});
