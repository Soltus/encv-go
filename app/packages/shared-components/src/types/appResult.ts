// AppResult 借鉴自 nuclear-boy AppResult.kt (sealed class AppResult<out T>)。
//
// TS 版用 discriminated union 实现：
//   - { ok: true; data: T }    → Success
//   - { ok: false; error: AppError } → Failure
//
// 配合类型守卫 isOk() / isFailure() 让消费方写链式 .OnSuccess / .OnFailure
// 时编译器能正确收窄类型。

import type { AppError } from "./appError";

/** AppResult<T> —— Success/Failure 联合类型。 */
export type AppResult<T> = { ok: true; data: T } | { ok: false; error: AppError };

/** 构造 Success 结果。 */
export function appOk<T>(data: T): AppResult<T> {
  return { ok: true, data };
}

/** 构造 Failure 结果。 */
export function appErr<T = never>(error: AppError): AppResult<T> {
  return { ok: false, error };
}

/** 类型守卫：是否为 Success。 */
export function isOk<T>(r: AppResult<T>): r is { ok: true; data: T } {
  return r.ok === true;
}

/** 类型守卫：是否为 Failure。 */
export function isFailure<T>(r: AppResult<T>): r is { ok: false; error: AppError } {
  return r.ok === false;
}

/** 取值或 fallback。 */
export function getOrElse<T>(r: AppResult<T>, fallback: T): T {
  return r.ok ? r.data : fallback;
}

/** Map 链式转换（Failure 透传，借鉴 nuclear-boy L24-27）。 */
export function mapResult<T, U>(r: AppResult<T>, transform: (data: T) => U): AppResult<U> {
  return r.ok ? appOk(transform(r.data)) : (r as AppResult<U>);
}

/** OnSuccess 副作用钩子（不改结果，借鉴 nuclear-boy L29-32）。 */
export function onOk<T>(r: AppResult<T>, action: (data: T) => void): AppResult<T> {
  if (r.ok) action(r.data);
  return r;
}

/** OnFailure 副作用钩子（不改结果，借鉴 nuclear-boy L34-37）。 */
export function onErr<T>(r: AppResult<T>, action: (error: AppError) => void): AppResult<T> {
  if (!r.ok) action(r.error);
  return r;
}

/** RunCatching —— 把可能 throw 的 async fn 包成 AppResult。
 *  借鉴 nuclear-boy AppResult.kt L44-53 runCatching。
 *
 *  与 Go RunCatching 行为对齐：
 *   - 返回成功 → { ok: true, data }
 *   - throw AppError → { ok: false, error }（直接复用）
 *   - throw 普通 Error → { ok: false, error: AppErrorUnknown + msg }
 *   - throw ToolError-like 对象 → 由调用方在 fn 内预先转为 AppError
 *
 *  @example
 *    const r = await runCatchingAsync(async () => await fetchXxx())
 *    if (isFailure(r)) showToast(r.error.humanMessage)
 */
export async function runCatchingAsync<T>(fn: () => Promise<T>): Promise<AppResult<T>> {
  try {
    const data = await fn();
    return appOk(data);
  } catch (e: unknown) {
    return appErr(toAppError(e));
  }
}

/** 同步版 runCatching。 */
export function runCatching<T>(fn: () => T): AppResult<T> {
  try {
    return appOk(fn());
  } catch (e: unknown) {
    return appErr(toAppError(e));
  }
}

/** 把任意 thrown 值转 AppError。
 *  - 已是 AppError → 直接返回
 *  - Error 有 name === 'AbortError' → UserCancelled
 *  - Error message 含 timeout/connection refused/401/402/429/HTTP 5xx → 启发式分类
 *  - 兜底 → Unknown
 */
function toAppError(e: unknown): AppError {
  if (isAppError(e)) return e;
  if (e instanceof Error) {
    const msg = e.message;
    const lower = msg.toLowerCase();
    if (e.name === "AbortError") return { type: "UserCancelled", message: msg, humanMessage: "🚫 已取消", isRetryable: false };
    if (lower.includes("timeout")) return { type: "NetworkTimeout", message: msg, humanMessage: "⏱️ 网络超时，请重试", isRetryable: true };
    if (lower.includes("connection refused") || lower.includes("no such host") || lower.includes("ssl"))
      return { type: "NetworkUnavailable", message: msg, humanMessage: "🌐 网络不可用", isRetryable: true };
    if (lower.includes("401")) return { type: "ApiKeyInvalid", message: msg, humanMessage: "🔑 API Key 无效", isRetryable: false };
    if (lower.includes("402")) return { type: "InsufficientBalance", message: msg, humanMessage: "💰 余额不足", isRetryable: false };
    if (lower.includes("429")) return { type: "RateLimited", message: msg, humanMessage: "🚦 请求过于频繁", isRetryable: true };
    if (msg.startsWith("HTTP 5") || lower.includes("internal server error"))
      return { type: "ServerError", message: msg, humanMessage: "🔧 服务端错误", isRetryable: true };
    return { type: "Unknown", message: msg, humanMessage: "❓ 未知错误", isRetryable: false };
  }
  return { type: "Unknown", message: String(e), humanMessage: "❓ 未知错误", isRetryable: false };
}

/** 类型守卫：判断任意值是否为 AppError。
 *  鸭子类型：必须有 type / message / humanMessage / isRetryable 四个字段。
 */
function isAppError(e: unknown): e is AppError {
  return typeof e === "object" && e !== null && "type" in e && "message" in e && "humanMessage" in e && "isRetryable" in e;
}
