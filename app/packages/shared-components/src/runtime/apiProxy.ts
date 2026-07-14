/**
 * apiProxy — 应用层 → 共享层的「ApiProxy 原生插件 + 平台检测」注入 DI 注册点
 *
 * 背景：shared 作为共享层，不得反向依赖应用层（@/...）内部实现，尤其是
 * Capacitor 原生插件 @/plugins/ApiProxy（绕过 WebView CORS 的 fetch 代理）
 * 以及 @capacitor/core 平台检测。但 useProxiedFetch（window.fetch 覆盖安装器）
 * 运行时需要这些能力。
 *
 * 约定（镜像 nativeBridge / appAssets 范式）：
 *   - shared 内部一律通过 getApiProxy() 取用这些能力，绝不 import @/plugins/ApiProxy
 *     或 @capacitor/core。
 *   - app 在启动期（stores/registerSharedApiProxy）调用 setApiProxy(...) 注入具体实现。
 *   - 未注入时：isAndroid 默认 false（web SPA，安全），其余原生专属函数抛清晰错误
 *     （安装器先 isAndroid() 守卫，web 永不调用）。
 */

export interface ProxyFetchOptions {
  url: string;
  method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH" | "HEAD" | "OPTIONS";
  headers?: Record<string, string>;
  body?: string | null;
  expectStream?: boolean;
}

export interface ProxyFetchResult {
  status: number;
  statusText: string;
  headers: Record<string, string>;
  body: string;
  resolvedBaseUrl: string;
}

export interface ProxyStreamStartResult {
  streamId: string;
  status: number;
  statusText: string;
  headers: Record<string, string>;
  resolvedBaseUrl: string;
}

export type ApiProxyListener = (event: unknown) => void;

export interface ApiProxyBridge {
  /** 是否运行在原生 Android 平台（仅此平台需覆盖 fetch）。默认 false。 */
  isAndroid: () => boolean;
  /** 单次 fetch，返回完整 body（非流式）。 */
  fetchOnce: (options: ProxyFetchOptions) => Promise<ProxyFetchResult>;
  /** 流式 fetch（SSE），通过事件回传 chunks。 */
  streamStart: (options: ProxyFetchOptions) => Promise<ProxyStreamStartResult>;
  /** 主动取消一个 stream。 */
  streamCancel: (options: { streamId: string }) => Promise<void>;
  /** 订阅插件事件（stream:data / stream:end）。 */
  addListener: (eventName: string, listener: ApiProxyListener) => Promise<unknown> | unknown;
  /** 注销某事件的全部监听器。 */
  removeAllListeners: (eventName: string) => Promise<unknown> | unknown;
}

const notInjected = (name: string): never => {
  throw new Error(`[apiProxy] ${name} 未注入（需在 app 启动期调用 setApiProxy）`);
};

const defaults: ApiProxyBridge = {
  isAndroid: () => false,
  fetchOnce: () => notInjected("fetchOnce"),
  streamStart: () => notInjected("streamStart"),
  streamCancel: () => notInjected("streamCancel"),
  addListener: () => notInjected("addListener"),
  removeAllListeners: () => notInjected("removeAllListeners"),
};

let proxy: ApiProxyBridge = { ...defaults };

/** app 启动期调用：注入 / 覆盖 ApiProxy 能力。可多次部分覆盖。 */
export function setApiProxy(partial: Partial<ApiProxyBridge>): void {
  proxy = { ...proxy, ...partial };
}

/** shared 内部取用 ApiProxy 能力。 */
export function getApiProxy(): ApiProxyBridge {
  return proxy;
}
