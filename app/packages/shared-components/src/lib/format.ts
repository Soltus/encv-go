/**
 * 格式化工具单一真源。
 *
 * 收敛自三处语义一致的 1024 进制「文件大小 → 人类可读」实现：
 *   - `api/encv_files.ts` 的 `formatFileSize`
 *   - `lib/buildReportZip.ts` 的本地 `formatBytes`
 *   - `components/TaskPerformanceSection.vue` 的本地 `formatBytes`
 *
 * 三者单位序（B/KB/MB/GB/TB）、进制（1024）一致，仅健壮性/精度微差。
 * 此处统一为最健壮版本（处理 undefined/null、clamp 越界单位），作为 shared 内唯一实现。
 */

export interface FormatBytesOptions {
  /** 小数位数，默认 1。调用方可用 `decimals: 2` 保留更高精度（不丢精度）。 */
  decimals?: number;
  /** 非有限值 / 负数 / null / undefined 时返回的占位符，默认 ""。 */
  invalid?: string;
  /** 最大单位上限，默认 "TB"（设为 "GB" 可禁用 TB 显示）。 */
  maxUnit?: "B" | "KB" | "MB" | "GB" | "TB";
}

/**
 * 把字节数格式化为人类可读字符串（1024 进制，单一真源）。
 *
 * 通过选项保留各调用方原有精度，避免「收敛 = 丢精度」的错误取舍：
 * - 默认：`undefined/null/负数 → ""`、`0 → "0 B"`、`toFixed(1)`、含 TB。
 * - `decimals: 2`：GB/TB 也保留两位小数（FullTextIndex / SparseContainer 原行为）。
 * - `invalid: "?"`：非法值返回 "?"（SparseContainer 原 sentinel）。
 * - `maxUnit: "GB"`：禁用 TB（历史报告契约需固定单位时）。
 */
export function formatBytes(bytes?: number | string | null, opts: FormatBytesOptions = {}): string {
  const { decimals = 1, invalid = "", maxUnit = "TB" } = opts;
  const num = typeof bytes === "string" ? Number(bytes) : bytes;
  if (num === undefined || num === null || !Number.isFinite(num) || num < 0) {
    return invalid;
  }
  if (num === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const maxIdx = units.indexOf(maxUnit);
  const k = 1024;
  const i = Math.min(Math.floor(Math.log(num) / Math.log(k)), maxIdx);
  return `${parseFloat((num / k ** i).toFixed(decimals))} ${units[i]}`;
}

/**
 * 截断路径/字符串（保留末尾，避免长内容破坏排版）。
 *
 * 收敛自两处语义一致的本地实现：
 *   - `lib/buildReportZip.ts` 的 `truncatePath(p, max=60)`（省略符 "..."）
 *   - `app/.../ApprovalCard.vue` 的 `truncatePath(p)`（max=28，省略符 "…"）
 *
 * 两者均保留总长度 = max，仅省略符与默认 max 不同 → 用入参收敛。
 *
 * @param p 原字符串
 * @param max 保留总长度（含省略符），默认 60
 * @param ellipsis 省略符，默认 "..."
 */
/**
 * 截断文本（保留一端，避免长内容破坏排版）。
 *
 * 通用化「截断 + 省略符」模式，供路径 / 错误 / 任意文本复用：
 *   - `mode: "tail"`（默认）：保留**末尾**（如路径，省略符在前）；
 *   - `mode: "head"`：保留**开头**（如错误首行，省略符在后）。
 *
 * @param text 原字符串
 * @param max 保留总长度（含省略符），默认 60
 * @param ellipsis 省略符，默认 "..."
 * @param mode 保留方向，默认 "tail"
 */
export type TruncateMode = "tail" | "head";

export function truncateText(text: string, max = 60, ellipsis = "...", mode: TruncateMode = "tail"): string {
  if (!text) return "";
  if (text.length <= max) return text;
  const keep = Math.max(0, max - ellipsis.length);
  if (mode === "head") {
    return `${text.slice(0, keep)}${ellipsis}`;
  }
  return `${ellipsis}${text.slice(-keep)}`;
}

export function truncatePath(p: string, max = 60, ellipsis = "..."): string {
  return truncateText(p, max, ellipsis);
}
