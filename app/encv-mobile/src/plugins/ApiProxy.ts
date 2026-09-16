import { registerPlugin } from "@capacitor/core";

/**
 * ApiProxy — Capacitor 插件，绕过 WebView CORS。
 *
 * 背景：WebView origin = `https://localhost`（Capacitor androidScheme），
 *       后端 = `http://127.0.0.1:2025`。跨源 POST /api/chat 触发 preflight，
 *       即使 `X-Agent-Protocol` 已在 server CORS 中放行，preflight 失败仍
 *       概率性出现（依赖 Android WebView 版本 / OkHttp 实现）。
 *
 * 方案：本插件提供 fetchOnce（普通 HTTP）和 streamStart（SSE）两个方法，
 *       native 端用 HttpURLConnection 调后端，从源头消除 WebView CORS 检查。
 *       Web 端（vite dev）fallback 到原生 fetch + dev gateway。
 *
 * 注意：所有路径相对当前页面（`/api/...`），后端地址由 native 端硬编码为
 *       `http://127.0.0.1:2025`，与 EncvGoService.DEFAULT_PORT 对齐。
 */

/** 单次 HTTP 调用的入参（与 web fetch 语义对齐） */
export interface ProxyFetchOptions {
  url: string;
  method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH" | "HEAD" | "OPTIONS";
  headers?: Record<string, string>;
  body?: string | null;
  /** true 表示后端用 chunked/SSE，streamStart 才走 streaming 分支 */
  expectStream?: boolean;
}

/** fetchOnce 返回的响应（body 为完整字符串，不分块） */
export interface ProxyFetchResult {
  status: number;
  statusText: string;
  headers: Record<string, string>;
  body: string;
  /** 后端真实 base URL，便于 DevLogs 诊断（native 端填） */
  resolvedBaseUrl: string;
}

/** streamStart 返回的 stream 句柄 */
export interface ProxyStreamStartResult {
  streamId: string;
  status: number;
  statusText: string;
  headers: Record<string, string>;
  resolvedBaseUrl: string;
}

/** plugin 推送给 JS 的 stream chunk 事件 */
export interface ProxyStreamChunkEvent {
  streamId: string;
  /** 原始字节的 base64 编码（避免 JS 字符串丢字节） */
  dataBase64: string;
}

export interface ProxyStreamEndEvent {
  streamId: string;
  status?: number;
  error?: string;
}

/** 插件接口 */
export interface ApiProxyPlugin {
  /** 单次 fetch，返回完整 body（用于非流式 API） */
  fetchOnce(options: ProxyFetchOptions): Promise<ProxyFetchResult>;
  /** 流式 fetch（用于 SSE），通过 events 回传 chunks */
  streamStart(options: ProxyFetchOptions): Promise<ProxyStreamStartResult>;
  /** 主动取消一个 stream（SSE 断开 / 切 tab） */
  streamCancel(options: { streamId: string }): Promise<void>;
  /** 订阅插件事件（stream:data / stream:end）。 */
  addListener(eventName: string, listener: (event: any) => void): Promise<any>;
  /** 注销某事件的全部监听器。 */
  removeAllListeners(eventName: string): Promise<void>;
}

const ApiProxy = registerPlugin<ApiProxyPlugin>("ApiProxy", {
  web: () => import("./ApiProxy.web").then(m => new m.ApiProxyWeb()),
});

export { ApiProxy };
export default ApiProxy;
