// baseUrl.ts - 服务端 URL / 标识 / 持久化常量
// 从 encv-mobile/src/api/encv_core.ts 提升为共享基座（不依赖任何应用层 @/ 配置）。
// 仅使用浏览器全局（localStorage / window.location / import.meta.env）+ shared 层的
// 能力注入点（runtime/appCapabilities），符合 shared 边界约束。

import { getAppCapabilities } from "../../runtime/appCapabilities";

export const SERVER_URL_KEY = "encv-server-url";
// 🆕 2026-06-15：跨会话持久化 backend instance_id，用于防"端口被劫持/换进程"误判
//   后端 performPingCheck (internal/register/server_start.go:103) 启动期就比对
//   instance_id 防劫持，移动端必须复用同一机制——单看 200 + JSON 不够。
//   任何"返 200 application/json" 的进程（mock / 旧 encv-go / 上游代理 / 中间人）
//   都会骗过老的 checkServerStatus。InstanceID 是 encv-go 启动时唯一生成的
//   UUID4，进程内常驻不变，重启即换。
export const SERVER_INSTANCE_ID_KEY = "encv-server-instance-id";
export const SERVER_VERSION_KEY = "encv-server-version";
export const DEFAULT_API_BASE_URL = "http://127.0.0.1:2025";
// 🆕 2026-06-10 沙箱 OpenPreview 浏览器专用：必须用**同源** fetch。
//   - OpenPreview 浏览器在 agent-tool-host 上，访问 127.0.0.1/localhost
//     解析到 agent-tool-host 自己的端口（不存在 :16666）→ 永远 connect refused
//   - trae 反代已经把 trae.cn/api/* 完整代理到 :16000 → :16666 → :2025
//     （curl -s http://127.0.0.1:16000/api/config 直接 200，proxy 链路 OK）
//   - 所以 sandbox 浏览器下 baseUrl 必须是**同源**（window.location.origin），
//     fetch 走同源相对 URL，让 trae 反代处理；或者直接返回 '' 让浏览器补 origin
//   - 沙箱本地（非 OpenPreview）的 loopback 浏览器走原 127.0.0.1:16666 路径
//   - APK 真机（capacitor://）直连 127.0.0.1:2025
export const DEV_SANDBOX_ENTRY = "http://127.0.0.1:16666";

/**
 * 判断当前是否在 OpenPreview 浏览器（agent-tool-host 提供的 trae 域名 mock 浏览器）
 *
 * 🆕 2026-06-10 修复：把 trae 反代端口 16000 也算上
 *   背景：trae 反代 16000 不支持 WebSocket upgrade，OpenPreview 工具激活时
 *   location 可能是 `http://127.0.0.1:16000`（trae 把页面代理到 16000），
 *   这种情况下 origin.hostname === '127.0.0.1'，原 trae 域名正则匹配不到。
 *   必须靠端口 16000 嗅探。
 */
/**
 * 是否运行在 **Capacitor 原生壳**（APK / iOS App）里。
 *
 * ⚠️ 这里判定的是「是不是原生壳」，不是「是不是移动端」：
 *   - `capacitor.config.ts` 配了 `server.androidScheme: 'https'`，所以安卓 WebView 的
 *     `window.location` 是 **`https://localhost`**（不是很多人以为的 `capacitor://`），
 *     协议/形态都长得像「一个普通 https 托管页」。
 *   - 一旦把 WebView **自己的** origin 当成后端 base，所有 `${base}/api/...` 都会打到
 *     `https://localhost` ⇒ 端口默许 **443** ⇒ ApiProxy（对绝对 URL 原样转发）
 *     ⇒ `Failed to connect to localhost/127.0.0.1:443`（2026-10-03 真机事故）。
 *   - 桌面形态若走 Capawesome Electron（isNative=true 但形态仍是桌面托管），按
 *     appCapabilities 的约定要看 platform，不当原生壳处理。
 *
 * 判定顺序：DI（app 启动期注入，唯一真源）→ Capacitor 全局桥（DI 未覆盖时的防线，
 * 防止注入时序被漂移后就漏判）。
 */
function isNativeShell(): boolean {
  try {
    const caps = getAppCapabilities();
    if (caps.isNative()) {
      return caps.platform?.() !== "electron";
    }
  } catch {
    // 能力未注入 / 取值异常 → 落到下面的全局桥探测，不放大成崩溃
  }
  const cap =
    typeof globalThis !== "undefined"
      ? ((globalThis as Record<string, unknown>).Capacitor as { isNativePlatform?: () => boolean; getPlatform?: () => string } | undefined)
      : undefined;
  if (!cap || typeof cap.isNativePlatform !== "function") return false;
  return cap.isNativePlatform() === true && cap.getPlatform?.() !== "electron";
}

export function isOpenPreviewBrowser(): boolean {
  if (typeof window === "undefined" || !window.location) return false;
  const origin = window.location.origin;
  const port = window.location.port;
  return (
    /trae\.cn$/i.test(origin) || /agent-sandbox/i.test(origin) || /^run-agent-/i.test(origin) || port === "16000" // 🆕 trae 反代端口
  );
}

export function getApiBaseUrl(): string {
  if (import.meta.env.DEV) {
    // OpenPreview 浏览器（trae 域名）→ 必须同源，让 trae 反代处理
    if (isOpenPreviewBrowser()) {
      return typeof window !== "undefined" ? window.location.origin : "";
    }
    const stored = localStorage.getItem(SERVER_URL_KEY);
    if (stored) return stored;
    return DEV_SANDBOX_ENTRY;
  }

  // ── prod ──
  const stored = localStorage.getItem(SERVER_URL_KEY);
  if (stored) return stored;

  // 🆕 2026-10-02（spec desktop-web-android-pairing / Task 1.4 R16）：
  //   **web 形态（http/https）默认同源**——页面是谁托管的，后端就在谁背后。
  //   旧默认 `http://127.0.0.1:2025` 隐含"用户本机跑着 encv-go"，对
  //   **服务器托管形态（cnb：preview-gateway 把 /api、/agent-api 代理到 :2025）是错的**
  //   ——浏览器会去找用户自己机器上的 2025，而不是托管页的那台服务器。
  //   与 useApiBaseProbe 的探测链一致：它早就把 [1.5] current-origin 排在
  //   [2] loopback 之前（浏览器模式优先同源）。这里只把**默认值**对齐同一意图。
  //   安全性：若托管方没代理 /api，探测链 [1.5] 失败后会继续回落 loopback / LAN，
  //   行为与修改前一致（不会比现在更差）。
  //
  // 🚨 2026-10-03 真机事故修复（回归锁：src/api/__tests__/getApiBaseUrl.native.test.ts）：
  //   上一版这条分支只判 protocol（http/https），没先排除**原生壳**。但
  //   `server.androidScheme: 'https'` 让真机 WebView 的页面协议就是 https、
  //   hostname 就是 localhost ⇒ WebView 自己的 origin 被当成后端 base
  //   ⇒ fetch('https://localhost/api/config') ⇒ ApiProxy 对绝对 URL 原样转发
  //   ⇒ HttpURLConnection 打 127.0.0.1:443 ⇒ connect refused
  //   （用户侧现象：通知栏后端端口明明是 2025，页面却报 :443）。
  //   原生壳里后端永远跑在设备 loopback 上，必须回落 DEFAULT_API_BASE_URL。
  if (isNativeShell()) {
    return DEFAULT_API_BASE_URL;
  }

  if (typeof window !== "undefined" && /^https?:$/.test(window.location.protocol)) {
    return window.location.origin;
  }
  // 非 http(s) 场景（如古老 capacitor:// scheme）：保持原绝对地址
  return DEFAULT_API_BASE_URL;
}

export function setApiBaseUrl(url: string) {
  localStorage.setItem(SERVER_URL_KEY, url);
}

export function getServerUrl(): string {
  return getApiBaseUrl();
}

export function resetServerUrl() {
  localStorage.removeItem(SERVER_URL_KEY);
}

export function getWebSocketUrl(): string {
  if (import.meta.env.DEV) {
    const wsProtocol = location.protocol === "https:" ? "wss:" : "ws:";
    return `${wsProtocol}//${location.host}/ws`;
  }
  const baseUrl = getApiBaseUrl();
  const wsUrl = baseUrl.replace(/^https:\/\//, "wss://").replace(/^http:\/\//, "ws://");
  return `${wsUrl}/ws`;
}

export function proxySafeEncode(value: string): string {
  return encodeURIComponent(encodeURIComponent(value));
}

export function getPersistedBackendIdentity(): { instanceId: string; version: string } | null {
  if (typeof localStorage === "undefined") return null;
  const id = localStorage.getItem(SERVER_INSTANCE_ID_KEY);
  if (!id) return null;
  return { instanceId: id, version: localStorage.getItem(SERVER_VERSION_KEY) || "" };
}
