/**
 * 动效设计令牌（与 ENCV前端主题重构方案.md §2.6 的 CSS 变量对应）。
 *
 * 缓动使用「语义键」，由引擎映射为具体库的缓动（见 internal/gsap-engine.ts 的 EASE_MAP）。
 * 组件 / 模块统一从这里取缓动与时长，不要散落魔法数。
 * 强度（intensity）请改从 guard.getMotionProfile() 读取，避免与 CSS 变量重复。
 */
export const EASE = {
  out: "out",
  inOut: "inOut",
  back: "back",
  linear: "linear",
  follow: "follow",
  float: "float",
} as const;

export const DUR = {
  fast: 0.16,
  base: 0.32,
  slow: 0.52,
} as const;

/** 列表 / 级联入场的基础 stagger（秒），实际会乘以 profile.intensity */
export const STAGGER = 0.04;
