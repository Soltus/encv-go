/**
 * 动效统一闸门（guard）。
 *
 * 所有 motion 模块从这里读取 MotionProfile，决定动画是否启用 / 强度多少。
 * 规则（见 ENCV前端主题重构方案.md §2.6）：
 *   - prefers-reduced-motion => 关闭动画，调用方必须直接落终态，绝不播放入场动画
 *   - 根节点带 .encv-vivid 或 .encv-p3 => 强度上浮到 1.3
 */
import { motion } from "./internal";
import { readMotionNumber } from "./theme-read";

export interface MotionProfile {
  /** reduced-motion 时为 false：调用方应直接 set 终态，不要播放入场动画 */
  enabled: boolean;
  /** vivid / P3 时 1.3，否则 1；用于缩放时长与位移幅度 */
  intensity: number;
  /** 是否处于 reduced-motion 环境 */
  respectsReduced: boolean;
}

/**
 * 运行时「一键关动效」全局开关（覆盖 reduced-motion 探测）。
 *   - null（默认）：跟随系统 prefers-reduced-motion；
 *   - true：强制关闭所有动效（低端设备 / 测试 / 省电模式），composable 直接落终态；
 *   - false：强制开启（即使用户系统偏好 reduced-motion，用于无障碍「显式开启」）。
 * 与 noopMotion 引擎二选一即可：setMotionDisabled(true) 是运行时按调用点读取的闸门，
 * 改 internal/index.ts 导出 noopMotion 是构建期全局替换。
 * 注意：与 registry.ts 的 setMotionEnabled(name, enabled)（按命名动画开关）不冲突，
 * 这里是「全局」总闸。
 */
const MOTION_DISABLED_KEY = "encv-motion-disabled";

/** 模块加载即水合：从 localStorage 恢复「减少动效」持久偏好（应用级全局生效）。 */
function readStoredDisabled(): boolean | null {
  if (typeof localStorage === "undefined") return null;
  const v = localStorage.getItem(MOTION_DISABLED_KEY);
  if (v === null) return null;
  return v === "true";
}

let forcedDisabled: boolean | null = readStoredDisabled();

export function setMotionDisabled(disabled: boolean | null): void {
  forcedDisabled = disabled;
  if (typeof localStorage === "undefined") return;
  if (disabled === null) localStorage.removeItem(MOTION_DISABLED_KEY);
  else localStorage.setItem(MOTION_DISABLED_KEY, disabled ? "true" : "false");
}

export function getMotionDisabled(): boolean | null {
  return forcedDisabled;
}

export function getMotionProfile(): MotionProfile {
  if (typeof window === "undefined" || !window.matchMedia) {
    return { enabled: forcedDisabled === null ? true : !forcedDisabled, intensity: 1, respectsReduced: false };
  }
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const root = document.documentElement;
  const vivid = root.classList.contains("encv-vivid") || root.classList.contains("encv-p3");
  const enabled = forcedDisabled !== null ? !forcedDisabled : !reduced;
  // 强度源自主题 --motion-intensity 令牌（gsap 赋能主题）：默认 1、vivid/P3 上调 1.3、
  // 主题可自定义（如 0.8 克制 / 1.5 张扬）。令牌读不到时回退 vivid 类判断。
  // reduced-motion 下 tokens.css 把该令牌置 0——但若被 setMotionDisabled(false) 显式开启，
  // enabled=true 而令牌=0 会导致「开启却零幅度」的矛盾，此时钳制回 vivid?1.3:1。
  let intensity = readMotionNumber("--motion-intensity", vivid ? 1.3 : 1);
  if (enabled && intensity <= 0) intensity = vivid ? 1.3 : 1;
  return {
    enabled,
    intensity,
    respectsReduced: reduced,
  };
}

/**
 * reduced-motion 下把元素直接清回终态，避免任何入场动画残留的内联样式。
 */
export function settleToFinalState(el: HTMLElement | null | undefined): void {
  if (el) motion.set(el, { clearProps: "all" });
}
