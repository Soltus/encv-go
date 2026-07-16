/**
 * ENCV 动效中枢入口（src/motion）。
 *
 * 引入此模块即注册底层动画引擎插件（幂等）。所有动画原语都经由
 * internal/ 的引擎抽象，调用方无需、也不应直接接触具体动画库。
 * 用法：
 *   import { usePageTransition, useScrollReveal } from "@encv/shared-components/motion";
 * 或按需子路径：
 *   import { useRipple } from "@encv/shared-components/motion/micro";
 */
import { motion } from "./internal";

/** 注册底层动画引擎插件（幂等）。引入本模块时已自动调用一次。 */
export function installMotion(): void {
  motion.registerPlugins();
}

installMotion();

export * from "./guard";
export * from "./tokens";
// 主题切换后可调用 invalidateMotionTokenCache() 让 GSAP 立即读到新的 --motion-* 令牌。
export { invalidateMotionTokenCache } from "./theme-read";
export * from "./registry";
export * from "./transition";
export * from "./reveal";
export * from "./flip";
export * from "./micro";
export * from "./magnetic";
export * from "./overlay";
export * from "./nav";
export * from "./data";
export * from "./ambient";
export * from "./guide";
export * from "./gesture";
