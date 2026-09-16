/**
 * 主题动效令牌的「运行时读取层」——让 gsap（JS 动画引擎）赋能主题。
 *
 * 背景：theme/tokens.css 定义了 --motion-dur-* / --motion-stagger / --motion-intensity，
 * 但此前只有「纯 CSS 动画」读它们；GSAP 侧的时长 / stagger / 强度是写死的 JS 常量，
 * 主题 / 用户片段覆写这些令牌对 GSAP 动画毫无影响 —— 主题无法真正定制动效。
 *
 * 本模块在运行时读取 documentElement 上的 --motion-* 计算值，供 tokens.ts / guard.ts 使用，
 * 于是「覆写 --motion-* 令牌」能同时作用于纯 CSS 动画与 GSAP 动画（gsap 赋能主题）。
 *
 * 性能：getComputedStyle().getPropertyValue 在样式脏时会触发同步样式重算。磁性跟手等
 * 高频路径每帧调 getMotionProfile()，若每次都读会造成回流风暴。故这里带 250ms 节流缓存
 * （按变量名缓存），拖拽期间至多每 250ms 读一次，主题切换也能在 250ms 内反映。
 *
 * 无 DOM（SSR / 单测无 documentElement）时读不到值，交由调用方回退到常量默认。
 */

const CACHE_TTL_MS = 250;

let cache: { at: number; values: Map<string, string> } | null = null;

function now(): number {
  return typeof performance !== "undefined" ? performance.now() : Date.now();
}

/** 读取根节点某个 CSS 自定义属性的计算值（带节流缓存），读不到返回空串。 */
function readRootVar(varName: string): string {
  if (typeof document === "undefined" || !document.documentElement) return "";
  if (typeof getComputedStyle === "undefined") return "";
  const t = now();
  if (!cache || t - cache.at >= CACHE_TTL_MS) {
    cache = { at: t, values: new Map() };
  }
  const cached = cache.values.get(varName);
  if (cached !== undefined) return cached;
  const value = getComputedStyle(document.documentElement).getPropertyValue(varName).trim();
  cache.values.set(varName, value);
  return value;
}

/** 解析 CSS 时间值为秒：支持 "160ms" / "0.32s" / 裸数字（视为秒）。非法返回 null。 */
function parseSeconds(raw: string): number | null {
  const m = raw.match(/^(-?[\d.]+)\s*(ms|s)?$/);
  if (!m) return null;
  const n = Number.parseFloat(m[1]);
  if (Number.isNaN(n)) return null;
  return m[2] === "ms" ? n / 1000 : n; // 无单位或 s 视为秒
}

/** 读取 --motion-* 时长令牌（秒）；读不到 / 非法时用 fallback。 */
export function readMotionSeconds(varName: string, fallback: number): number {
  const parsed = parseSeconds(readRootVar(varName));
  return parsed ?? fallback;
}

/** 读取 --motion-* 数值令牌（无单位，如 --motion-intensity）；读不到 / 非法时用 fallback。 */
export function readMotionNumber(varName: string, fallback: number): number {
  const raw = readRootVar(varName);
  if (!raw) return fallback;
  const n = Number.parseFloat(raw);
  return Number.isNaN(n) ? fallback : n;
}

/**
 * 清空节流缓存，让下次读取立即反映最新令牌值。
 * 主题切换（切 data-theme / 注入用户片段）后可显式调用以即时生效（否则最多 250ms 后自动生效）；
 * 单测里改 --motion-* 后也应调用以获得确定性。
 */
export function invalidateMotionTokenCache(): void {
  cache = null;
}
