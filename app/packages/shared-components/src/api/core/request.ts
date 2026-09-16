// request.ts - 统一后端请求封装
// 自动拼接 base URL + 查询参数 + 认证头，并将非 2xx 响应归一化为 ApiError（或其子类）。

import { useApiContext } from "./context";
import { ApiError, PermissionDeniedError, NotFoundError } from "./errors";

export interface ApiRequestOptions extends RequestInit {
  /** 查询参数，自动拼接到 URL（undefined 值自动跳过） */
  query?: Record<string, string | number | undefined>;
  /**
   * 响应解析方式（默认 json）。
   * - `json`：解析 JSON；2xx 但返回 HTML 视为 SPA-fallback 报错（防 json() 抛 SyntaxError）。
   * - `text`：返回 `response.text()`。
   * - `blob`：返回 `response.blob()`（如文件导出下载）。
   * - `response`：返回原始 `Response` 对象（需读响应头 / 自定义解析时使用，如导出下载）。
   */
  responseType?: "json" | "text" | "blob" | "response";
  /** 请求超时（毫秒）。超时后 abort 并抛 ApiError(status=0) */
  timeoutMs?: number;
  /**
   * 非 2xx 时按 HTTP 状态码映射到具体错误子类（覆盖默认映射）。
   * 默认：403 → PermissionDeniedError、404 → NotFoundError。
   */
  statusErrorMap?: Record<number, new (message: string, body?: unknown) => ApiError>;
}

/** 默认状态码 → 错误子类映射（typed error 收敛到 ApiError 体系） */
const DEFAULT_STATUS_ERROR_MAP: Record<number, new (message: string, body?: unknown) => ApiError> = {
  403: PermissionDeniedError,
  404: NotFoundError,
};

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

/** 统一的后端请求封装：自动 base URL + 认证头 + 超时 + 错误归一化（typed error） */
export async function apiRequest<T = unknown>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  const ctx = useApiContext();
  const { query, headers, timeoutMs, statusErrorMap, ...init } = options;
  const url = buildUrl(path, query);
  const finalHeaders: Record<string, string> = {
    ...(ctx.getAuthHeaders ? ctx.getAuthHeaders() : {}),
    ...(headers as Record<string, string> | undefined),
  };

  // 超时控制（AbortController）
  const controller = timeoutMs ? new AbortController() : undefined;
  if (controller) {
    init.signal = controller.signal;
    setTimeout(() => controller.abort(), timeoutMs);
  }

  let response: Response;
  try {
    response = await fetch(url, { ...init, headers: finalHeaders });
  } catch (err) {
    if (controller?.signal.aborted) {
      throw new ApiError(0, `Request timed out after ${timeoutMs}ms`, undefined);
    }
    throw err;
  }

  const contentType = response.headers.get("content-type") ?? "";
  const isJson = contentType.includes("application/json");

  if (!response.ok) {
    let body: unknown;
    try {
      body = isJson ? await response.json() : await response.text();
    } catch {
      body = undefined;
    }
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : `HTTP error! status: ${response.status}`;
    const errorMap = { ...DEFAULT_STATUS_ERROR_MAP, ...statusErrorMap };
    const Ctor = errorMap[response.status];
    if (Ctor) throw new Ctor(message, body);
    throw new ApiError(response.status, message, body);
  }

  const responseType = options.responseType ?? "json";

  if (responseType === "text") {
    return (await response.text()) as T;
  }
  if (responseType === "blob") {
    return (await response.blob()) as T;
  }
  if (responseType === "response") {
    return response as T;
  }

  // json：统一读 text 后再解析，避免空响应体（204 / 空 200）让 response.json() 抛 SyntaxError
  const text = await response.text();
  if (text.trim().length === 0) {
    return undefined as T;
  }
  // SPA-fallback 防护：2xx 但返回 HTML（如登录页）视为后端不通，而非让 JSON.parse 抛错
  if (contentType.includes("text/html") || /^\s*<(!doctype|html)/i.test(text)) {
    throw new ApiError(response.status, "Backend returned HTML instead of JSON (SPA fallback?)", text.slice(0, 200));
  }
  try {
    return JSON.parse(text) as T;
  } catch {
    throw new ApiError(response.status, "Invalid JSON response from backend", text.slice(0, 200));
  }
}
