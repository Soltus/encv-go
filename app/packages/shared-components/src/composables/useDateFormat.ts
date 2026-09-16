import { useI18n } from "./useI18n";

export interface FormatDateTimeOptions {
  /** 是否包含秒，默认 false（仅 年/月/日/时/分）。 */
  withSeconds?: boolean;
  /** 显式 locale，默认取 useI18n 的 locale。 */
  locale?: string;
}

export function formatDateTime(isoStr: string | undefined | null, opts: FormatDateTimeOptions = {}): string {
  if (!isoStr) return "";
  const { withSeconds = false, locale } = opts;
  try {
    const d = new Date(isoStr);
    if (Number.isNaN(d.getTime())) return "";
    const { getLocale } = useI18n();
    const loc = locale ?? getLocale();
    return new Intl.DateTimeFormat(loc, {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      ...(withSeconds ? { second: "2-digit" } : {}),
      hour12: false,
    }).format(d);
  } catch {
    return "";
  }
}

export interface FormatDurationOptions {
  /** <1000ms 时显示 "Xms"，默认 false（归入秒）。 */
  showMs?: boolean;
  /** 秒显示一位小数 "X.Ys"，默认 false（整数 "Xs"）。 */
  showDecimals?: boolean;
  /** null/undefined/负数 时返回的占位符，默认 ""。 */
  invalid?: string;
}

/**
 * 把毫秒耗时格式化为人类可读字符串（单一真源）。
 *
 * 通过选项保留各调用方原有输出，避免「收敛 = 丢精度」的错误取舍：
 * - 默认：`<0 → ""`、`<60s → "Xs"`、`<60m → "XmYs"`、否则 `"XhYm"`（无 ms、无小数）。
 * - `showMs + showDecimals`：还原 buildReportZip / TreeView 的 `"500ms"` / `"45.0s"`。
 * - `invalid: "N/A"`：还原 buildReportZip 对 `null` 的占位。
 */
export function formatDuration(ms?: number, opts: FormatDurationOptions = {}): string {
  const { showMs = false, showDecimals = false, invalid = "" } = opts;
  if (ms === null || ms === undefined || ms < 0) return invalid;
  if (showMs && ms < 1000) return `${ms}ms`;
  const totalSec = Math.floor(ms / 1000);
  if (totalSec < 60) return showDecimals ? `${totalSec.toFixed(1)}s` : `${totalSec}s`;
  const min = Math.floor(totalSec / 60);
  const sec = totalSec % 60;
  if (min < 60) return `${min}m${sec}s`;
  const hr = Math.floor(min / 60);
  const rm = min % 60;
  return `${hr}h${rm}m`;
}
