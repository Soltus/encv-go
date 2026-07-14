/** 后端请求统一错误类型：携带 HTTP 状态码与可选响应体 */
export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(status: number, message: string, body?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

/** 403 权限拒绝（继承自 ApiError，调用方可用 instanceof ApiError 统一处理） */
export class PermissionDeniedError extends ApiError {
  constructor(message: string, body?: unknown) {
    super(403, message, body);
    this.name = "PermissionDeniedError";
  }
}

/** 404 资源不存在（继承自 ApiError） */
export class NotFoundError extends ApiError {
  constructor(message: string, body?: unknown) {
    super(404, message, body);
    this.name = "NotFoundError";
  }
}

/**
 * 判断未知错误是否为 ApiError 且状态码等于给定值（K18）。
 * 收敛散落的 `e instanceof ApiError && e.status === X` 裸分支。
 */
export function isApiStatus(e: unknown, status: number): e is ApiError {
  return e instanceof ApiError && e.status === status;
}

/**
 * 判断未知错误是否为 ApiError 且状态码 >= 给定下限（K18）。
 * 收敛 `e instanceof ApiError && e.status >= X` 裸分支。
 */
export function isApiStatusAtLeast(e: unknown, min: number): e is ApiError {
  return e instanceof ApiError && e.status >= min;
}
