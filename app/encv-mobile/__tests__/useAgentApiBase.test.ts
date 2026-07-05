/**
 * useAgentApiBase 单元测试
 *
 * 关键不变量（Phase X1 改造后）：
 * 1. dev 环境（import.meta.env.DEV=true）→ '/agent-api'（走 preview-gateway）
 * 2. 生产 native APK → ''（相对路径，由 window.fetch override 走 ApiProxy 插件）
 * 3. 生产 web SPA → 走 getApiBaseUrl() 绝对 URL
 * 4. Context 含 source 字段，区分 dev-gateway/native-default/user-configured/web-fallback
 * 5. Context 包含 sampleUrl 字段，便于日志/UI 展示
 *
 * 关键陷阱：
 * - isNative / getApiBaseUrl 都是 ESM 模块导入；测试需要 mock 它们
 * - import.meta.env.DEV 是构建时常量；运行时无法直接修改
 *   但我们可以通过 vi.stubEnv() 在 vitest 中修改 import.meta.env
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// ─── 通用 mock 容器 ──────────────────────────────────
let mockIsNative = false;
let mockApiBase = "http://127.0.0.1:2025";
let mockLocalStorage: Record<string, string> = {};

vi.mock("@/plugins/GoProcess", () => ({
  isNative: () => mockIsNative,
}));

vi.mock("@/api/encv", () => ({
  getApiBaseUrl: () => mockApiBase,
  DEFAULT_API_BASE_URL: "http://127.0.0.1:2025",
}));

beforeEach(() => {
  mockIsNative = false;
  mockApiBase = "http://127.0.0.1:2025";
  mockLocalStorage = {};
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => mockLocalStorage[k] ?? null,
    setItem: (k: string, v: string) => {
      mockLocalStorage[k] = v;
    },
    removeItem: (k: string) => {
      delete mockLocalStorage[k];
    },
    clear: () => {
      mockLocalStorage = {};
    },
  });
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

// ─── dev 环境 ─────────────────────────────────────

describe("getAgentApiBase - dev 环境", () => {
  it('import.meta.env.DEV=true 时返回 "/agent-api"（走 preview-gateway）', async () => {
    vi.stubEnv("DEV", true);
    vi.stubEnv("PROD", false);
    const { getAgentApiBase } = await import("@/composables/useAgentApiBase");
    expect(getAgentApiBase()).toBe("/agent-api");
  });

  it("dev 环境忽略 native / 用户配置（统一走网关）", async () => {
    vi.stubEnv("DEV", true);
    mockIsNative = true;
    mockApiBase = "http://custom.example.com:9999";
    mockLocalStorage["encv-server-url"] = "http://user-set:8888";
    const { getAgentApiBase } = await import("@/composables/useAgentApiBase");
    expect(getAgentApiBase()).toBe("/agent-api");
  });
});

// ─── 生产环境 ─────────────────────────────────────

describe("getAgentApiBase - 生产环境", () => {
  beforeEach(() => {
    vi.stubEnv("DEV", false);
    vi.stubEnv("PROD", true);
  });

  it("APK native → 返回空字符串（相对路径，走 ApiProxy 插件）", async () => {
    mockIsNative = true;
    mockApiBase = "http://127.0.0.1:2025";
    const { getAgentApiBase } = await import("@/composables/useAgentApiBase");
    expect(getAgentApiBase()).toBe("");
  });

  it("web SPA + 用户配置 encv-server-url → 使用用户配置", async () => {
    mockIsNative = false;
    mockApiBase = "http://127.0.0.1:2025";
    mockLocalStorage["encv-server-url"] = "http://user.example.com:9000";
    const { getAgentApiBase } = await import("@/composables/useAgentApiBase");
    expect(getAgentApiBase()).toBe("http://127.0.0.1:2025");
  });

  it("getApiBaseUrl 返回空字符串 → 降级到 DEFAULT_API_BASE_URL", async () => {
    mockIsNative = false;
    mockApiBase = "";
    const { getAgentApiBase } = await import("@/composables/useAgentApiBase");
    expect(getAgentApiBase()).toBe("http://127.0.0.1:2025");
  });
});

// ─── getAgentApiBaseContext ─────────────────────────

describe("getAgentApiBaseContext", () => {
  it("dev: source=dev-gateway, env=dev, sampleUrl 拼装到 /agent-api/api/encrypt-key", async () => {
    vi.stubEnv("DEV", true);
    vi.stubEnv("PROD", false);
    const { getAgentApiBaseContext } = await import("@/composables/useAgentApiBase");
    const ctx = getAgentApiBaseContext();
    expect(ctx).toMatchObject({
      base: "/agent-api",
      source: "dev-gateway",
      env: "dev",
    });
    expect(ctx.sampleUrl).toContain("/agent-api/api/encrypt-key");
  });

  it("prod native: source=native-default, sampleUrl 相对路径（ApiProxy 接管）", async () => {
    vi.stubEnv("DEV", false);
    vi.stubEnv("PROD", true);
    mockIsNative = true;
    mockApiBase = "http://127.0.0.1:2025";
    const { getAgentApiBaseContext } = await import("@/composables/useAgentApiBase");
    const ctx = getAgentApiBaseContext();
    expect(ctx).toMatchObject({
      base: "",
      source: "native-default",
      env: "prod",
      isNative: true,
    });
    expect(ctx.sampleUrl).toBe("/api/encrypt-key");
  });

  it("prod web + 用户配置 encv-server-url: source=user-configured", async () => {
    vi.stubEnv("DEV", false);
    vi.stubEnv("PROD", true);
    mockIsNative = false;
    mockApiBase = "http://127.0.0.1:2025";
    mockLocalStorage["encv-server-url"] = "http://my-config:8888";
    const { getAgentApiBaseContext } = await import("@/composables/useAgentApiBase");
    const ctx = getAgentApiBaseContext();
    expect(ctx.source).toBe("user-configured");
    expect(ctx.isNative).toBe(false);
  });

  it("prod web + 无用户配置: source=web-fallback", async () => {
    vi.stubEnv("DEV", false);
    vi.stubEnv("PROD", true);
    mockIsNative = false;
    mockApiBase = "http://127.0.0.1:2025";
    // mockLocalStorage 空
    const { getAgentApiBaseContext } = await import("@/composables/useAgentApiBase");
    const ctx = getAgentApiBaseContext();
    expect(ctx.source).toBe("web-fallback");
  });
});

// ─── 真实调用方使用：${base}/api/encrypt-key 拼接 ─────────

describe("调用方拼接：${getAgentApiBase()}/api/encrypt-key", () => {
  it("dev: 拼出 /agent-api/api/encrypt-key", async () => {
    vi.stubEnv("DEV", true);
    const { getAgentApiBase } = await import("@/composables/useAgentApiBase");
    const url = `${getAgentApiBase()}/api/encrypt-key`;
    expect(url).toBe("/agent-api/api/encrypt-key");
  });

  it("prod native: 拼出 /api/encrypt-key（相对路径，ApiProxy 插件接管）", async () => {
    vi.stubEnv("DEV", false);
    vi.stubEnv("PROD", true);
    mockIsNative = true;
    mockApiBase = "http://127.0.0.1:2025";
    const { getAgentApiBase } = await import("@/composables/useAgentApiBase");
    const url = `${getAgentApiBase()}/api/encrypt-key`;
    expect(url).toBe("/api/encrypt-key");
  });
});
