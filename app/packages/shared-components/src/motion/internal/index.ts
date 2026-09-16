/**
 * 动效引擎桶导出。
 *
 * 这是「换技术栈」的唯一开关：把下面这行指向新的实现即可。
 * 例如改用 anime.js 时：
 *   export { motion } from "./anime-engine";
 * composable 与下游代码无需任何改动。
 */
export { motion } from "./gsap-engine";
/**
 * 无动画引擎（参考实现 / 测试 / 低端设备一键关动效）。
 * 要全局改用 no-op，把上面那行改为 `export { noopMotion as motion } from "./noop-engine"` 即可。
 */
export { noopMotion } from "./noop-engine";
export type * from "./types";
