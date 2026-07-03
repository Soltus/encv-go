/**
 * useProxiedFetch — 在 native 模式下把 window.fetch 替换为 ApiProxy 插件调用，
 * 消除 WebView → 127.0.0.1:2025 跨源触发的 CORS preflight。
 *
 * 触发条件：Capacitor.isNativePlatform() && platform === 'android'。
 * 替换行为：
 *   - 非流式 fetch → ApiProxy.fetchOnce() → 包装为 Response
 *   - 流式 fetch（SSE，需 SSE 端点）→ ApiProxy.streamStart() + 包装为 ReadableStream
 *   - WebSocket、FormData、上传等不走此路径（保持原生）
 *
 * dev 模式（vite）不安装 override：相对路径 /agent-api 走 preview-gateway。
 */

import { Capacitor } from "@capacitor/core";
import { ApiProxy } from "@/plugins/ApiProxy";

let installed = false;
let originalFetch: typeof window.fetch | null = null;

interface InternalRequestInit extends RequestInit {
  /** 标记这是 SSE 请求（流式） */
  isStream?: boolean;
}

/** 解码 SSE chunk (base64) → string */
function decodeChunk(b64: string): string {
  try {
    const binary = atob(b64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return new TextDecoder("utf-8").decode(bytes);
  } catch (e) {
    console.error("[useProxiedFetch] base64 decode failed:", e);
    return "";
  }
}

function headersToObject(h: HeadersInit | undefined): Record<string, string> {
  if (!h) return {};
  if (h instanceof Headers) {
    const obj: Record<string, string> = {};
    h.forEach((v, k) => {
      obj[k] = v;
    });
    return obj;
  }
  if (Array.isArray(h)) {
    const obj: Record<string, string> = {};
    for (const [k, v] of h) obj[k] = v;
    return obj;
  }
  return { ...h } as Record<string, string>;
}

/** 把相对 url 解析为绝对 URL（用于 Response.url 字段） */
function resolveUrlForResponse(url: string, baseOrigin: string): string {
  if (url.startsWith("http://") || url.startsWith("https://")) return url;
  if (url.startsWith("/")) return baseOrigin + url;
  return baseOrigin + "/" + url;
}

/** 给 Response 注入 url 字段（Response constructor 不接受，但 Response.url 是 readonly） */
function withResponseUrl(res: Response, fullUrl: string): Response {
  try {
    Object.defineProperty(res, "url", { value: fullUrl, writable: false, configurable: true });
  } catch (_) {
    // 某些环境 Response.url 是 frozen 的，吞掉
  }
  return res;
}

function buildProxiedResponse(
  status: number,
  statusText: string,
  headers: Record<string, string>,
  body: string,
  resolvedBaseUrl: string,
  originalUrl: string
): Response {
  const h = new Headers();
  for (const [k, v] of Object.entries(headers)) {
    h.set(k, v);
  }
  const fullUrl = resolveUrlForResponse(originalUrl, resolvedBaseUrl);
  const res = new Response(body, { status, statusText, headers: h });
  return withResponseUrl(res, fullUrl);
}

function buildProxiedStreamResponse(
  streamId: string,
  status: number,
  statusText: string,
  headers: Record<string, string>,
  resolvedBaseUrl: string,
  originalUrl: string
): Response {
  const h = new Headers();
  for (const [k, v] of Object.entries(headers)) {
    h.set(k, v);
  }
  const fullUrl = resolveUrlForResponse(originalUrl, resolvedBaseUrl);
  const encoder = new TextEncoder();

  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const dataHandler = (event: any) => {
        if (event.streamId !== streamId) return;
        const text = decodeChunk(event.dataBase64);
        controller.enqueue(encoder.encode(text));
      };
      const endHandler = (event: any) => {
        if (event.streamId !== streamId) return;
        controller.close();
        // 注销监听器
        (ApiProxy as any).removeAllListeners?.("stream:data")?.catch?.(() => {});
        (ApiProxy as any).removeAllListeners?.("stream:end")?.catch?.(() => {});
      };
      // 注册监听器
      (ApiProxy as any).addListener?.("stream:data", dataHandler)?.catch?.((e: unknown) => {
        console.error("[useProxiedFetch] addListener stream:data failed:", e);
      });
      (ApiProxy as any).addListener?.("stream:end", endHandler)?.catch?.((e: unknown) => {
        console.error("[useProxiedFetch] addListener stream:end failed:", e);
      });
    },
    cancel() {
      // 主动取消时调 streamCancel
      ApiProxy.streamCancel({ streamId }).catch(() => {});
    },
  });

  const res = new Response(stream, { status, statusText, headers: h });
  return withResponseUrl(res, fullUrl);
}

const ALLOWED_METHODS = new Set(["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"] as const);
type AllowedMethod = "GET" | "POST" | "PUT" | "DELETE" | "PATCH" | "HEAD" | "OPTIONS";

function asMethod(m: string | undefined): AllowedMethod {
  const upper = (m ?? "GET").toUpperCase() as AllowedMethod;
  return ALLOWED_METHODS.has(upper) ? upper : "GET";
}

/**
 * 把 fetch 调用路由到 ApiProxy（native Android）。
 */
async function proxiedFetch(url: string, init: InternalRequestInit = {}): Promise<Response> {
  const headers = headersToObject(init.headers);
  const method = asMethod(init.method);
  let body: string | null = null;
  if (init.body != null) {
    if (typeof init.body === "string") {
      body = init.body;
    } else if (init.body instanceof URLSearchParams) {
      body = init.body.toString();
    } else {
      // FormData / Blob / ArrayBuffer / ReadableStream — 这些是上传或大文件场景，
      // 不走代理（会丢字节）。fallback 到原始 fetch
      if (originalFetch) {
        return originalFetch.call(window, url, init);
      }
      return fetch(url, init);
    }
  }

  // 流式：要求 Accept: text/event-stream 或 init.isStream
  const isStream =
    init.isStream === true ||
    (headers["Accept"]?.includes("text/event-stream") ?? false) ||
    (headers["accept"]?.includes("text/event-stream") ?? false);

  if (isStream) {
    const result = await ApiProxy.streamStart({ url, method, headers, body });
    return buildProxiedStreamResponse(result.streamId, result.status, result.statusText, result.headers, result.resolvedBaseUrl, url);
  }

  const result = await ApiProxy.fetchOnce({ url, method, headers, body });
  return buildProxiedResponse(result.status, result.statusText, result.headers, result.body, result.resolvedBaseUrl, url);
}

/**
 * 安装 window.fetch 覆盖（幂等）。
 * dev / web 平台不会真覆盖——返回原 fetch。
 */
export function installProxiedFetch(): void {
  if (installed) return;
  if (typeof window === "undefined") return;
  if (!Capacitor.isNativePlatform() || Capacitor.getPlatform() !== "android") {
    // dev/web: 不替换
    return;
  }

  originalFetch = window.fetch.bind(window);
  window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    return proxiedFetch(url, init as InternalRequestInit);
  }) as typeof window.fetch;
  installed = true;
  console.info("[useProxiedFetch] installed — fetch now goes through ApiProxy plugin");
}

/** 卸载（仅测试用） */
export function uninstallProxiedFetch(): void {
  if (!installed || !originalFetch) return;
  window.fetch = originalFetch;
  originalFetch = null;
  installed = false;
}

/** 当前是否已安装（仅测试用） */
export function isProxiedFetchInstalled(): boolean {
  return installed;
}
