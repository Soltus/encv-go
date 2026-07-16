/**
 * 动效设计令牌 —— 现在是主题 CSS 令牌（theme/tokens.css 的 --motion-*）的运行时读取层。
 *
 * gsap（JS 动画引擎）通过这里在运行时读取主题的 --motion-dur-* / --motion-stagger 令牌：
 * 主题 / 用户片段覆写这些令牌时，GSAP 动画与纯 CSS 动画表现一致（gsap 赋能主题）。
 * 无 DOM（SSR / 单测无 documentElement）时回退到下方常量默认值。
 *
 * 缓动使用「语义键」，由引擎映射为具体库的缓动（见 internal/gsap-engine.ts 的 EASE_MAP）。
 * 组件 / 模块统一从这里取缓动与时长，不要散落魔法数。
 * 强度（intensity）请改从 guard.getMotionProfile() 读取（同样源自 --motion-intensity 令牌）。
 */
import { readMotionSeconds } from "./theme-read";

export const EASE = {
  out: "out",
  inOut: "inOut",
  back: "back",
  linear: "linear",
  follow: "follow",
  float: "float",
} as const;

/** 时长默认值（秒）—— 与 theme/tokens.css 的 --motion-dur-* 同步，作为无 DOM 回退。 */
const DUR_FALLBACK = { fast: 0.16, base: 0.32, slow: 0.52 } as const;

/**
 * 时长令牌（秒）。每次访问都实时读取主题 --motion-dur-* 令牌（带 250ms 节流缓存），
 * 因此主题覆写 --motion-dur-base 等即可同时改变 GSAP 与 CSS 动画时长，消费方零改动。
 */
export const DUR = {
  get fast(): number {
    return readMotionSeconds("--motion-dur-fast", DUR_FALLBACK.fast);
  },
  get base(): number {
    return readMotionSeconds("--motion-dur-base", DUR_FALLBACK.base);
  },
  get slow(): number {
    return readMotionSeconds("--motion-dur-slow", DUR_FALLBACK.slow);
  },
} as const;

/** 列表 / 级联入场的基础 stagger 默认值（秒），作为无 DOM 回退。 */
export const STAGGER = 0.04;

/**
 * 基础 stagger（秒）：读主题 --motion-stagger 令牌（回退 STAGGER 默认），实际会乘以 profile.intensity。
 * 需要 stagger 时请用本函数而非直接引用 STAGGER 常量，以便主题定制。
 */
export function getStagger(): number {
  return readMotionSeconds("--motion-stagger", STAGGER);
}
