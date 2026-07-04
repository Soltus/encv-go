import { WebPlugin } from "@capacitor/core";
import type { ApiProxyPlugin, ProxyFetchOptions, ProxyFetchResult, ProxyStreamStartResult } from "./ApiProxy";

/**
 * Web 端 fallback（vite dev / 浏览器）：直接用原生 fetch。
 * Dev 模式下 useAgentApiBase 返回 `/agent-api`（preview-gateway 转发到 Go），
 * 走原生 fetch 即可，不被插件拦截。
 */
export class ApiProxyWeb extends WebPlugin implements ApiProxyPlugin {
  async fetchOnce(options: ProxyFetchOptions): Promise<ProxyFetchResult> {
    const res = await fetch(options.url, {
      method: options.method ?? "GET",
      headers: options.headers,
      body: options.body,
    });
    const headers: Record<string, string> = {};
    res.headers.forEach((v, k) => {
      headers[k] = v;
    });
    const body = await res.text();
    return {
      status: res.status,
      statusText: res.statusText,
      headers,
      body,
      resolvedBaseUrl: new URL(options.url, location.href).origin,
    };
  }

  async streamStart(_options: ProxyFetchOptions): Promise<ProxyStreamStartResult> {
    // dev 不走这里；useAgentApiBase 走 /agent-api
    throw this.unimplemented("dev mode uses native fetch, not the plugin");
  }

  async streamCancel(_options: { streamId: string }): Promise<void> {
    // no-op
  }
}
