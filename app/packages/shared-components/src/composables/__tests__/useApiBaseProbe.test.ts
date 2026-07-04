/**
 * useApiBaseProbe.test.ts
 *
 * 验证 AI 路由 baseUrl 探测链的正确性：
 *  1. 优先级链：cached → loopback → LAN 候选
 *  2. 命中即停（不是全探测完）
 *  3. 失败不污染 localStorage（旧值兜底）
 *  4. setManual / resetToDefault 正确
 *  5. 10s 节流
 *
 * 关键 mock 策略：
 *  - global.fetch：模拟各 URL 探活响应
 *  - @/api/encv：用 vi.spyOn 替换 setApiBaseUrl / getApiBaseUrl，
 *    保留原函数写 localStorage 的副作用（spyOn 可以观察调用同时跑原行为）
 *  - useApiBaseProbe：调用 __resetApiBaseProbeForTest() 重置单例，
 *    不使用 vi.resetModules（避免破坏 spy 绑定）
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const fetchMock = vi.fn();
vi.stubGlobal("fetch", fetchMock);

// import 必须在 stubGlobal 之后
import * as encv from "@encv/shared-components/api/encv";
import { __resetApiBaseProbeForTest, useApiBaseProbe } from "@encv/shared-components/composables/useApiBaseProbe";

// 每次测试拿到 fresh 单例：调用 __resetApiBaseProbeForTest() + 取新单例
function freshProbe() {
  __resetApiBaseProbeForTest();
  return useApiBaseProbe();
}

/**
 * 构造一个 fetch Mock：按 url substring 匹配返回不同 Response
 */
function setupFetchMock(
  handlers: Array<{
    match: (url: string) => boolean;
    respond: () => Response;
  }>
) {
  fetchMock.mockReset();
  fetchMock.mockImplementation((url: string) => {
    const handler = handlers.find(h => h.match(url));
    if (handler) return Promise.resolve(handler.respond());
    return Promise.reject(new Error(`unmocked URL: ${url}`));
  });
}

/**
 * 也支持 reject 形式（用于网络错误模拟）
 */
function setupFetchMockWithRejects(
  handlers: Array<{
    match: (url: string) => boolean;
    respond?: () => Response;
    reject?: () => never;
  }>
) {
  fetchMock.mockReset();
  fetchMock.mockImplementation((url: string) => {
    const handler = handlers.find(h => h.match(url));
    if (!handler) return Promise.reject(new Error(`unmocked URL: ${url}`));
    if (handler.reject) return Promise.reject(handler.reject());
    return Promise.resolve(handler.respond!());
  });
}

function okResponse(body: unknown = { ok: true }): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

beforeEach(() => {
  fetchMock.mockReset();

  // 用 vi.spyOn 替换 setApiBaseUrl：保留 localStorage 写副作用
  // 这里 spy 是用 mockImplementation 替换实现，但内部仍写 localStorage
  // 这样 spy 调用记录可断言，同时 production 行为（写 localStorage）保留
  vi.spyOn(encv, "setApiBaseUrl").mockImplementation((url: string) => {
    try {
      localStorage.setItem("encv-server-url", url);
    } catch {
      /* ignore */
    }
  });
  // getApiBaseUrl 在测试里返回 loopback（import.meta.env.DEV 在测试中为 false，
  // 所以原函数会读 localStorage，spy 后直接返回 loopback）
  vi.spyOn(encv, "getApiBaseUrl").mockReturnValue("http://127.0.0.1:2025");

  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
  // 隐藏 console.debug 噪音
  vi.spyOn(console, "debug").mockImplementation(() => {});
  vi.spyOn(console, "info").mockImplementation(() => {});
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("useApiBaseProbe — 优先级链", () => {
  it("[1] cached 命中 → 立刻返回，不试 loopback", async () => {
    localStorage.setItem("encv-server-url", "http://192.168.1.5:2025");
    setupFetchMock([{ match: u => u.includes("192.168.1.5"), respond: () => okResponse() }]);
    const probe = freshProbe();
    const result = await probe.probe({ force: true });

    expect(result.baseUrl).toBe("http://192.168.1.5:2025");
    expect(result.source).toBe("cached");
    expect(fetchMock).toHaveBeenCalledTimes(1); // 只 1 次探活
    expect(encv.setApiBaseUrl).toHaveBeenCalledWith("http://192.168.1.5:2025");
  });

  it("[2] cached 失败 + loopback 命中 → 走 loopback", async () => {
    localStorage.setItem("encv-server-url", "http://192.168.1.99:2025"); // 不通
    setupFetchMock([
      { match: u => u.includes("192.168.1.99"), respond: () => new Response("", { status: 502 }) },
      { match: u => u.includes("127.0.0.1"), respond: () => okResponse() },
    ]);
    const probe = freshProbe();
    const result = await probe.probe({ force: true });

    expect(result.baseUrl).toBe("http://127.0.0.1:2025");
    expect(result.source).toBe("loopback");
    // 至少 2 次：cached 探活 + loopback 探活
    expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("[3] cached 失败 + loopback 失败 + LAN 候选命中 → 走 lan-candidate", async () => {
    localStorage.setItem("encv-server-url", "http://192.168.1.99:2025");

    const probe = freshProbe();
    // 第一次跑会失败（无 lanAccess 兜底），但会建立 lastResult
    setupFetchMock([{ match: () => true, respond: () => new Response("", { status: 502 }) }]);
    await expect(probe.probe({ force: true })).rejects.toThrow("all-candidates-failed");

    // 第二次：注入 lastResult.lanAccess（模拟前一轮的缓存）
    probe.lastResult.value = {
      baseUrl: "",
      lanAccess: { addresses: ["192.168.1.10", "192.168.1.20"], preferred: "http://192.168.1.10:2025" },
      source: "lan-candidate",
      latencyMs: 0,
      log: [],
    };

    // 现在 192.168.1.10 通，loopback 还是不通
    setupFetchMockWithRejects([
      { match: u => u.includes("192.168.1.99"), respond: () => new Response("", { status: 502 }) },
      {
        match: u => u.includes("127.0.0.1"),
        reject: () => {
          throw new TypeError("Failed to fetch");
        },
      },
      { match: u => u.includes("192.168.1.10"), respond: () => okResponse() },
    ]);
    const result2 = await probe.probe({ force: true });
    expect(result2.baseUrl).toBe("http://192.168.1.10:2025");
    expect(result2.source).toBe("lan-candidate");
  });

  it("[4] 全部失败 → 抛 all-candidates-failed，不写 localStorage", async () => {
    localStorage.setItem("encv-server-url", "http://192.168.1.99:2025");
    setupFetchMockWithRejects([
      {
        match: () => true,
        reject: () => {
          throw new TypeError("Failed to fetch");
        },
      },
    ]);
    const probe = freshProbe();

    // 第一次：建立空 lanAccess 的 lastResult
    await expect(probe.probe({ force: true })).rejects.toThrow();
    expect(localStorage.getItem("encv-server-url")).toBe("http://192.168.1.99:2025"); // 旧值保留
    expect(encv.setApiBaseUrl).not.toHaveBeenCalled(); // commit 没被调
  });
});

describe("useApiBaseProbe — setManual / resetToDefault", () => {
  it("setManual 接受合法 URL", async () => {
    const probe = freshProbe();
    probe.setManual("http://192.168.1.5:9999");
    expect(localStorage.getItem("encv-server-url")).toBe("http://192.168.1.5:9999");
    expect(encv.setApiBaseUrl).toHaveBeenCalledWith("http://192.168.1.5:9999");
  });

  it("setManual 拒绝不合法 URL", async () => {
    const probe = freshProbe();
    expect(() => probe.setManual("not-a-url")).toThrow(/invalid baseUrl format/);
    expect(() => probe.setManual("ftp://foo")).toThrow(/invalid baseUrl format/);
  });

  it("resetToDefault 清 localStorage 后再探测命中 loopback", async () => {
    localStorage.setItem("encv-server-url", "http://192.168.1.5:2025");
    setupFetchMock([{ match: u => u.includes("127.0.0.1"), respond: () => okResponse() }]);
    const probe = freshProbe();
    const result = await probe.resetToDefault();
    // 探测成功后会写回 localStorage（commit 调用 setApiBaseUrl）
    expect(localStorage.getItem("encv-server-url")).toBe("http://127.0.0.1:2025");
    expect(result.baseUrl).toBe("http://127.0.0.1:2025");
    // resetToDefault 内部先 remove，commit 内部 setApiBaseUrl
    // setApiBaseUrl 被调一次（commit 写回）
    expect(encv.setApiBaseUrl).toHaveBeenCalledWith("http://127.0.0.1:2025");
  });
});

describe("useApiBaseProbe — 节流", () => {
  it("10s 内重复 probe 默认走 throttle（无 force）", async () => {
    setupFetchMock([{ match: () => true, respond: () => okResponse() }]);
    const probe = freshProbe();

    const r1 = await probe.probe({ force: true });
    fetchMock.mockClear();

    // 第二次不强制 → 应走 throttle，复用 lastResult
    const r2 = await probe.probe();
    expect(r2).toEqual(r1);
    expect(fetchMock).not.toHaveBeenCalled(); // 节流命中
  });

  it("force: true 跳过节流", async () => {
    setupFetchMock([{ match: () => true, respond: () => okResponse() }]);
    const probe = freshProbe();

    await probe.probe({ force: true });
    fetchMock.mockClear();
    await probe.probe({ force: true });
    expect(fetchMock).toHaveBeenCalled(); // 强制重探
  });
});

/**
 * 🆕 2026-06-10 沙箱 mock 浏览器防回归（用户报告"断联+配置全空"）：
 *  trae sandbox origin 探测 401/403/non-JSON 时**绝对不能** commit trae origin，
 *  也不应 throw（会让 [App] onErrorCaptured 渲染错误边界），必须继续走后续探测
 *  或 fallback 到沙箱内 127.0.0.1:16666。
 */
describe("useApiBaseProbe — 沙箱 mock 浏览器 trae gateway 拦截（防回归）", () => {
  function mockWindowOrigin(origin: string): void {
    Object.defineProperty(window, "location", {
      configurable: true,
      writable: true,
      value: { ...window.location, origin, protocol: "https:", host: origin.replace(/^https?:\/\//, "") },
    });
  }

  it("[1.5] sandbox origin 返 401 → 不 commit trae origin，继续 [2] loopback 命中", async () => {
    mockWindowOrigin("https://run-agent-test.trae.cn");
    setupFetchMock([
      // trae origin 返 401（CORS / 网关拦截）
      { match: u => u.includes("run-agent-test.trae.cn"), respond: () => new Response("", { status: 401 }) },
      // [2] loopback 通
      { match: u => u.includes("127.0.0.1:2025"), respond: () => okResponse() },
    ]);
    const probe = freshProbe();
    const result = await probe.probe({ force: true });

    // ❌ 绝不能 commit trae origin
    expect(result.baseUrl).not.toContain("run-agent-test.trae.cn");
    // ✅ 落到 [2] loopback
    expect(result.baseUrl).toBe("http://127.0.0.1:2025");
    expect(result.source).toBe("loopback");
  });

  it("[1.5] sandbox origin 返 403 → 不 commit trae origin，继续 [2] loopback 命中", async () => {
    mockWindowOrigin("https://run-agent-abc.trae.cn");
    setupFetchMock([
      { match: u => u.includes("run-agent-abc.trae.cn"), respond: () => new Response("", { status: 403 }) },
      { match: u => u.includes("127.0.0.1:2025"), respond: () => okResponse() },
    ]);
    const probe = freshProbe();
    const result = await probe.probe({ force: true });

    expect(result.baseUrl).toBe("http://127.0.0.1:2025");
    expect(result.source).toBe("loopback");
  });

  it("[1.5] sandbox origin 返 HTML（non-JSON） → 不 commit trae origin", async () => {
    mockWindowOrigin("https://run-agent-xyz.trae.cn");
    setupFetchMock([
      {
        match: u => u.includes("run-agent-xyz.trae.cn"),
        respond: () => new Response("<!DOCTYPE html>", { status: 200, headers: { "content-type": "text/html" } }),
      },
      { match: u => u.includes("127.0.0.1:2025"), respond: () => okResponse() },
    ]);
    const probe = freshProbe();
    const result = await probe.probe({ force: true });

    // trae origin content-type 不对（HTML）→ 走 sandbox fallback → 命中 [2]
    expect(result.baseUrl).toBe("http://127.0.0.1:2025");
  });

  it("[4] sandbox 全失败 fallback → commit 127.0.0.1:16666（沙箱内 preview-gateway 入口），不 commit trae origin", async () => {
    mockWindowOrigin("https://run-agent-fallback.trae.cn");
    // cached 空、sandbox origin 全拒、loopback 不通（沙箱内 encv-go 偶尔挂）
    setupFetchMockWithRejects([
      { match: u => u.includes("run-agent-fallback.trae.cn"), respond: () => new Response("", { status: 401 }) },
      {
        match: u => u.includes("127.0.0.1:2025"),
        reject: () => {
          throw new TypeError("NetworkError");
        },
      },
    ]);
    const probe = freshProbe();
    const result = await probe.probe({ force: true });

    // ❌ 绝不能 commit trae origin（trae 网关不通）
    expect(result.baseUrl).not.toContain("trae.cn");
    // ❌ 绝不能 throw（会触发 [App] onErrorCaptured 渲染错误边界）
    // ✅ 必须 fallback 到沙箱内 127.0.0.1:16666（preview-gateway 入口）
    expect(result.baseUrl).toBe("http://127.0.0.1:16666");
    expect(result.source).toBe("current-origin");
  });
});
