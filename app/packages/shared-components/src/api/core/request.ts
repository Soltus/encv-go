// request.ts - 统一后端请求封装
// 自动拼接 base URL + 查询参数 + 认证头，并将非 2xx 响应归一化为 ApiError。

import { useApiContext } from "./context";
import { ApiError } from "./errors";

export interface ApiRequestOptions extends RequestInit {
  /** 查询参数，自动拼接到 URL（undefined 值自动跳过） */
  query?: Record<string, string | number | undefined>;
}

/** 拼接 base URL 与路径/查询，返回完整请求 URL */
export function buildUrl(path: string, query?: ApiRequestOptions["query"]): string {
  const ctx = useApiContext();
  const base = ctx.getBaseUrl().replace(/\/$/, "");
  const url = /^(https?:)?\/\//i.test(path) ? path : `${base}${path.startsWith("/") ? "" : "/"}${path}`;
  if (query) {
    const qs = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined) qs.set(key, String(value));
    }
    const serialized = qs.toString();
    if (serialized) return `${url}${url.includes("?") ? "&" : "?"}${serialized}`;
  }
  return url;
}

/** 统一的后端请求封装：自动 base URL + 认证头 + 错误归一化 */
export async function apiRequest<T = unknown>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  const ctx = useApiContext();
  const { query, headers, ...init } = options;
  const url = buildUrl(path, query);
  const finalHeaders: Record<string, string> = {
    ...(ctx.getAuthHeaders ? ctx.getAuthHeaders() : {}),
    ...(headers as Record<string, string> | undefined),
  };
  const response = await fetch(url, { ...init, headers: finalHeaders });
  if (!response.ok) {
    let body: unknown;
    try {
      body = await response.json();
    } catch {
      body = undefined;
    }
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : `HTTP error! status: ${response.status}`;
    throw new ApiError(response.status, message, body);
  }
  return (await response.json()) as T;
}
