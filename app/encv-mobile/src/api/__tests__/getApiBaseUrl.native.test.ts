/**
 * getApiBaseUrl.native.test.ts — 安卓真机 WebView 的 base URL 契约锁
 *
 * 【真机事故 2026-10-03】（本文件存在的唯一理由，勿删勿放宽断言）
 *   现象：安卓真机起来后后端连不上，日志
 *     `[ENCV] Failed to load config: Error: proxy fetch failed:
 *      Failed to connect to localhost/127.0.0.1:443`
 *   而通知栏里后端明明跑在 :2025。
 *
 *   链路：
 *     1. `capacitor.config.ts` 配 `server.androidScheme: 'https'`
 *        ⇒ 真机 WebView 的 `window.location.origin` 就是 **https://localhost**
 *        （不是很多人以为的 `capacitor://`）
 *     2. 2026-10-02（spec desktop-web-android-pairing / Task 1.4 R16）给 prod 分支
 *        加了「http/https 形态默认同源」——只判 protocol，没排除原生壳，
 *        于是 WebView **自己的** origin 被当成后端 base
 *     3. fetch('https://localhost/api/config') 被 useProxiedFetch override 交给 ApiProxy，
 *        而 ApiProxy 对绝对 URL 原样转发 ⇒ HttpURLConnection 打 https://localhost
 *        ⇒ 解析 127.0.0.1 + 默认端口 **443** ⇒ connect refused（就是日志里的 :443）
 *
 *   契约（本文件把它钉死）：
 *     A. 原生壳（Capacitor android/ios）里 getApiBaseUrl() **绝不返回 WebView origin**，
 *        必须回落到设备本地后端（DEFAULT_API_BASE_URL = http://127.0.0.1:2025）。
 *     B. 真实服务器托管的 web SPA（`https://xxx`）仍默认同源——10-02 的意图不被误伤。
 *     C. `capacitor://` scheme 的原生壳同样回落 :2025（历史契约保留）。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

/** 把 window.location 伪装成指定 origin 的页面（模拟真机 WebView / 托管页） */
function stubLocationOrigin(origin: string): void {
  const u = new URL(origin);
  const fake = {
    href: `${u.origin}/`,
    origin: u.origin,
    protocol: u.protocol,
    host: u.host,
    hostname: u.hostname,
    port: u.port,
    pathname: "/",
    search: "",
    hash: "",
  };
  vi.spyOn(window, "location", "get").mockReturnValue(fake as unknown as Location);
}

/**
 * 按给定运行形态加载 api 模块：
 *   native=true  → 注入 appCapabilities（isNative/platform），等价于 main.ts 启动期注入
 *   再动态 import，保证拿到注入后的模块实例。
 */
async function loadApiWith(
  options: { origin: string; native: boolean; platform?: "android" | "ios" | "web" | "electron" },
  env: { dev?: boolean } = {},
) {
  vi.resetModules();
  // dev=true 用于复现"APK 内置资源是 dev 构建"的真机场景（DEV=true）
  vi.stubEnv("DEV", env.dev === true);
  vi.stubEnv("PROD", env.dev !== true);
  stubLocationOrigin(options.origin);

  const { setAppCapabilities } = await import("@encv/shared-components/runtime/appCapabilities");
  setAppCapabilities({
    isNative: () => options.native,
    platform: () => options.platform ?? "web",
  });

  // Capacitor 全局桥也要装上：shared 层的 native 判定不能只依赖 DI
  // （DI 注入晚于此处时会漏判，真线上这就是保护壳失效的口子）。
  if (options.native) {
    (globalThis as Record<string, unknown>).Capacitor = {
      isNativePlatform: () => true,
      getPlatform: () => options.platform ?? "android",
    };
  } else {
    delete (globalThis as Record<string, unknown>).Capacitor;
    Reflect.deleteProperty(window as unknown as Record<string, unknown>, "Capacitor");
  }

  return await import("@encv/shared-components/api/encv");
}

beforeEach(() => {
  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
  delete (globalThis as Record<string, unknown>).Capacitor;
  Reflect.deleteProperty(window as unknown as Record<string, unknown>, "Capacitor");
});

describe("getApiBaseUrl — 安卓真机 WebView（androidScheme: https）", () => {
  it("【事故回归锁】原生壳 + origin=https://localhost → 回落 :2025，绝不能返回 https://localhost", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" });

    const actual = mod.getApiBaseUrl();

    // 这就是打到 443 的元凶：https://localhost 是 WebView 自己，不是后端
    expect(actual).not.toBe("https://localhost");
    expect(actual).toBe(mod.DEFAULT_API_BASE_URL);
    expect(actual).toBe("http://127.0.0.1:2025");
  });

  it("原生壳 + WebView origin → /api/config 拼出来的 URL 指向设备本地后端（非 443）", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" });

    const url = `${mod.getApiBaseUrl()}/api/config`;

    expect(url).toBe("http://127.0.0.1:2025/api/config");
    expect(new URL(url).port).not.toBe("");
    expect(new URL(url).port).toBe("2025");
  });

  it("用户在 Settings 手配了 URL → 原生壳下仍以用户配置优先", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" });
    mod.setApiBaseUrl("http://127.0.0.1:2031");

    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:2031");
  });

  it("capacitor:// scheme 的原生壳 → 同样回落 :2025（历史契约）", async () => {
    const mod = await loadApiWith({ origin: "capacitor://localhost", native: true, platform: "android" });

    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:2025");
  });
});

describe("真机链路端到端（WebView → window.fetch override → ApiProxy 桥）", () => {
  it("checkServerStatus 必须打到 :2025/ping，而不是 WebView origin 的 :443", async () => {
    // 这条是事故链路的最小复现：真机上第一个挂掉的就是 checkServerStatus / fetchConfig
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" });
    const proxyMod = await import("@encv/shared-components/runtime/apiProxy");
    const proxiedFetchMod = await import("@encv/shared-components/composables/useProxiedFetch");

    const capturedUrls: string[] = [];
    proxyMod.setApiProxy({
      isAndroid: () => true,
      fetchOnce: async (opts: { url: string }) => {
        capturedUrls.push(opts.url);
        return {
          status: 200,
          statusText: "OK",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ status: "ok", version: "9.9.9", instance_id: "inst-native-1" }),
          resolvedBaseUrl: "http://127.0.0.1:2025",
        };
      },
      streamStart: async () => ({
        streamId: "s1",
        status: 200,
        statusText: "OK",
        headers: {},
        resolvedBaseUrl: "http://127.0.0.1:2025",
      }),
      streamCancel: async () => {},
      addListener: () => ({}) as unknown,
      removeAllListeners: () => ({}) as unknown,
    });
    proxiedFetchMod.installProxiedFetch();

    try {
      const result = await mod.checkServerStatus();

      expect(result.online).toBe(true);
      expect(capturedUrls.length).toBeGreaterThan(0);
      // 事故态这里会是 https://localhost/ping（→ 443 refused）
      expect(capturedUrls[0]).toBe("http://127.0.0.1:2025/ping");
      expect(capturedUrls[0]).not.toContain("https://localhost");
    } finally {
      proxiedFetchMod.uninstallProxiedFetch();
    }
  });
});

describe("getApiBaseUrl — 服务器托管 web SPA（10-02 同��默认不被误伤）", () => {
  it("非原生 + https 托管页 → 默认同源", async () => {
    const mod = await loadApiWith({ origin: "https://encv.example.com", native: false });

    expect(mod.getApiBaseUrl()).toBe("https://encv.example.com");
  });
});

// ────────────────────────────────────────────────────────────────
// 【真机事故 2026-10-05】落盘的服务器地址被无条件信任
//
// 现象（用户杀后台重启后必现，重启无效）：
//   `[ENCV] Failed to load config: Error: proxy fetch failed:
//    Failed to connect to /127.0.0.1:16666`
//   `wss://localhost/ws` 连续失败 → 降级 http-poll → 同样打 :16666
//
// 链路：
//   1. prod 分支第一行就是 `if (stored) return stored`，**没有任何校验**，
//      且它排在 `isNativeShell()` 判定之前；
//   2. :16666 是**沙箱 preview-gateway 专用端口**（DEV_SANDBOX_ENTRY），
//      真机/托管形态上根本没有这个监听 ⇒ 必然 connect refused；
//   3. 值存在 localStorage ⇒ **杀后台、重启 APP 都清不掉**，表现就是"怎么都连不上"。
//
// 契约（钉死）：
//   A. 沙箱网关端口 :16666 的落盘值一律无效（任何形态），丢弃并回落；
//   B. 原生壳里后端永远跑在**设备 loopback** ⇒ 非 loopback 的落盘值同样无效；
//   C. 用户手配的 loopback 端口（如 :2031）**必须继续优先**（不被误伤）。
// ────────────────────────────────────────────────────────────────
describe("getApiBaseUrl — 落盘地址的有效性（2026-10-05 真机事故）", () => {
  it("【事故回归锁】原生壳 + 落盘 :16666（沙箱端口）→ 丢弃并回落 :2025", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" });
    mod.setApiBaseUrl("http://127.0.0.1:16666");

    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:2025");
    // 坏值必须被清掉，否则下次启动还是它（这就是"重启也连不上"的根因）
    expect(localStorage.getItem("encv-server-url")).toBeNull();
  });

  it("原生壳 + 落盘非 loopback（如局域网地址）→ 无效，回落 :2025", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" });
    mod.setApiBaseUrl("http://192.168.1.9:2025");

    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:2025");
  });

  it("托管 web 形态 + 落盘 :16666 → 同样丢弃（沙箱端口不是任何形态的后端）", async () => {
    const mod = await loadApiWith({ origin: "https://encv.example.com", native: false });
    mod.setApiBaseUrl("http://127.0.0.1:16666");

    expect(mod.getApiBaseUrl()).toBe("https://encv.example.com");
  });

  it("反向锁：手配的 loopback 端口（:2031）必须仍然优先，不被新校验误伤", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" });
    mod.setApiBaseUrl("http://127.0.0.1:2031");

    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:2031");
  });
});

// ────────────────────────────────────────────────────────────────
// 【真机事故 2026-10-05 · 第二次】dev 构建跑在原生壳里
//
// 现象（回滚 web 包无效 ⇒ 说明不是"包"的问题）：
//   proxy fetch failed: Failed to connect to /127.0.0.1:16666
//   wss://localhost/ws 连续失败（WS 打到 WebView 自己的 origin）
//
// 根因：`import.meta.env.DEV === true`（APK 内置资源常是 dev/debug 构建）
//   ⇒ getApiBaseUrl() 走 **dev 分支**，stored 为空时直接 fallback 到
//      DEV_SANDBOX_ENTRY(:16666)；getWebSocketUrl() 的 dev 分支还用 location.host。
//   第一次修复只改了 prod 分支 ⇒ 这条路仍在漏。
//
// 契约：
//   A. dev + 原生壳 ⇒ HTTP 与 WS 都指向设备 loopback 后端，**绝不能是 :16666 / WebView origin**；
//   B. dev + 沙箱浏览器（trae/OpenPreview）⇒ 保持既有行为，不被误伤。
// ────────────────────────────────────────────────────────────────
describe("getApiBaseUrl — dev 构建跑在原生壳里（2026-10-05 第二次事故）", () => {
  it("【事故回归锁】dev + 原生壳 + 无落盘 ⇒ :2025，绝不是 :16666", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" }, { dev: true });

    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:2025");
    expect(mod.getApiBaseUrl()).not.toBe(mod.DEV_SANDBOX_ENTRY);
  });

  it("dev + 原生壳 ⇒ WS 与 HTTP 同 base（不能是 wss://localhost/ws）", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" }, { dev: true });

    expect(mod.getWebSocketUrl()).toBe("ws://127.0.0.1:2025/ws");
    expect(mod.getWebSocketUrl()).not.toContain("localhost/ws");
  });

  it("dev + 原生壳 + 落盘 :16666 ⇒ 丢弃并回落 :2025", async () => {
    const mod = await loadApiWith({ origin: "https://localhost", native: true, platform: "android" }, { dev: true });
    mod.setApiBaseUrl("http://127.0.0.1:16666");

    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:2025");
    expect(localStorage.getItem("encv-server-url")).toBeNull();
  });

  it("反向锁：dev + 沙箱浏览器（非原生）⇒ 保持既有行为（同源），不被新分支误伤", async () => {
    const mod = await loadApiWith({ origin: "https://run-agent-abc.trae.cn", native: false }, { dev: true });

    expect(mod.getApiBaseUrl()).toBe("https://run-agent-abc.trae.cn");
  });
});
