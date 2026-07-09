/**
 * relativeTime.ts — 统一相对时间格式化函数
 *
 * 5 档边界：
 *   < 60s        → "刚刚"
 *   < 60min      → "X 分钟前"
 *   < 24h        → "X 小时前"
 *   < 7d         → "X 天前"
 *   >= 7d        → "YYYY-MM-DD"（绝对日期 fallback）
 *
 * 关键决策：
 * - 保持纯函数（不依赖 useI18n / 不在内部 setInterval 刷新）：
 *   调用方按当前语言环境传 i18n 字符串，或直接接受中文。
 *   简化方案：直接返回中文字符串。如需多语言，调用方在外面 wrap。
 * - 接受 `now` 参数注入：让单测可以构造确定性时间点。
 * - 未来时间（diff < 0）使用 `Math.abs` 同样处理（"3 分钟后" 会落入 < 60s）。
 *
 * 调用方迁移：
 *   - AgentChat.vue L1037 的局部 formatRelativeTime → 改为 import 这个
 *   - sessionList 列表项时间 → 改为 import 这个
 *   - 30s 自动刷新：调用方用 setInterval 触发重新计算（参考 Task 8 集成）
 */

// 各档阈值（毫秒）
const MIN_MS = 60_000;
const HOUR_MS = 3_600_000;
const DAY_MS = 86_400_000;
const WEEK_MS = 604_800_000;

/**
 * 相对时间格式化（zh-CN 字符串；调用方需要 i18n 时自行 wrap）
 *
 * @param ts 目标时间戳（毫秒）
 * @param now 当前时间戳（毫秒），默认 Date.now()，单测可注入
 * @returns zh-CN 格式的相对时间 / 绝对日期
 */
export function formatRelativeTime(ts: number, now: number = Date.now()): string {
  // 防御：0 / undefined / null / NaN
  if (!ts || typeof ts !== "number" || Number.isNaN(ts)) return "";
  if (!now || typeof now !== "number" || Number.isNaN(now)) return "";

  const diff = now - ts;
  const abs = Math.abs(diff);

  if (abs < MIN_MS) return "刚刚";
  if (abs < HOUR_MS) return `${Math.floor(abs / MIN_MS)} 分钟前`;
  if (abs < DAY_MS) return `${Math.floor(abs / HOUR_MS)} 小时前`;
  if (abs < WEEK_MS) return `${Math.floor(abs / DAY_MS)} 天前`;

  // >= 7d → 绝对日期 YYYY-MM-DD（避免显示 11 个月前这种精度问题）
  const d = new Date(ts);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export default formatRelativeTime;
