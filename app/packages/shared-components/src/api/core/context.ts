// context.ts - 共享 API 上下文（依赖注入）
// 应用层在启动时调用 provideApiContext 注入 base URL / 认证来源；
// 未注入时回退到历史默认实现（从 localStorage / DEV 沙箱解析），
// 保证现有行为不变、且 api 函数可在组件 setup 之外安全调用。

import { getApiBaseUrl } from "./baseUrl";

export interface ApiContext {
  /** 返回后端 base URL（不含结尾斜杠） */
  getBaseUrl: () => string;
  /** 可选：返回需附加到每个请求的认证头（如 token） */
  getAuthHeaders?: () => Record<string, string>;
}

// 默认上下文 = 历史行为（从 localStorage / DEV 沙箱解析 base URL）
const defaultContext: ApiContext = {
  getBaseUrl: () => getApiBaseUrl(),
};

// 模块级注入覆盖（轻量 DI，不依赖 Vue 组件树，可在任意异步调用点读取）
let injected: ApiContext | null = null;

/** 应用层在启动时为共享 api 注入 base URL / 认证来源（依赖注入） */
export function provideApiContext(ctx: ApiContext): void {
  injected = ctx;
}

/** 读取当前 api 上下文；未注入时回退到历史默认实现 */
export function useApiContext(): ApiContext {
  return injected ?? defaultContext;
}
