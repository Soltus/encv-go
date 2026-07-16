/**
 * 动效引擎契约（Anti-Corruption Layer 的对外接口）。
 *
 * 本文件刻意不 import 任何动画库（gsap / animejs / ...），仅声明
 * 「引擎无关」的原语类型。所有消费方（motion/* composable）只依赖
 * 这里的类型与 `MotionEngine` 接口。因此更换底层动画库时，下游应用
 * 与插件零改动——只需另写一个实现同一接口的引擎，并切换
 * internal/index.ts 的导出即可（见 gsap-engine.ts 顶部说明）。
 */

/** 可被动画驱动的目标：DOM 元素，或供补间用的纯数值对象（如 count-up 的 {v}） */
export type MotionTarget = HTMLElement | SVGElement | Element | Record<string, number>;

/** 补间变量（gsap 的 TweenVars 等价物，但作为不透明记录暴露给调用方） */
export type MotionVars = Record<string, unknown>;

/** 一个可取消的补间 / 延时回调 */
export interface MotionTween {
  kill(): void;
}

/** gsap.context 的等价物，用于作用域化 + 卸载时回滚 */
export interface MotionContext {
  revert(): void;
}

/** ScrollTrigger 句柄（仅暴露消费方用到的能力） */
export interface ScrollTriggerHandle {
  kill(): void;
  readonly progress: number;
}

/** Flip 状态快照（具体形态对调用方不透明） */
export type FlipState = unknown;

export interface ScrollTriggerConfig {
  trigger: Element;
  /** 滚动容器。不传则默认 window。注意：Ionic 的 ion-content 在内部 shadow DOM 的
   *  .inner-scroll 滚动（非 window），直接以 window 为 scroller 的 ScrollTrigger 在 Ionic
   *  页面内永不触发 onEnter。揭示类动效（useScrollReveal / v-reveal）已改用 IntersectionObserver
   *  规避此问题；本参数保留给真正需要自定义 scroller 的 ScrollTrigger 场景（如 useScrollParallax）。 */
  scroller?: Element;
  start?: string;
  end?: string;
  once?: boolean;
  scrub?: boolean;
  onEnter?: () => void;
  onUpdate?: (self: ScrollTriggerHandle) => void;
}

/**
 * 动效引擎接口——底层动画库的统一抽象。
 * 实现见 ./gsap-engine.ts；未来换库时新增一个实现并改 ./index.ts 导出。
 */
export interface MotionEngine {
  /** 注册所需插件（ScrollTrigger / Flip 等），幂等 */
  registerPlugins(): void;
  set(target: MotionTarget | MotionTarget[], vars: MotionVars): void;
  to(target: MotionTarget | MotionTarget[], vars: MotionVars): MotionTween;
  from(target: MotionTarget | MotionTarget[], vars: MotionVars): MotionTween;
  fromTo(target: MotionTarget | MotionTarget[], fromVars: MotionVars, toVars: MotionVars): MotionTween;
  context(fn: () => void, scope: Element): MotionContext;
  /** 跟手：返回设置某属性值的函数（gsap.quickTo 语义） */
  quickTo(target: MotionTarget, prop: string, vars: MotionVars): (value: number) => void;
  /** 延时回调，返回可取消句柄 */
  delayedCall(delay: number, cb: () => void): MotionTween;
  createScrollTrigger(config: ScrollTriggerConfig): ScrollTriggerHandle;
  /** 重新量测所有 ScrollTrigger（页面进场 transition 结束后调用，修正被缓存的「未进入」位置）。 */
  refreshScrollTriggers(): void;
  flipGetState(target: MotionTarget): FlipState;
  flipFrom(state: FlipState, vars: MotionVars): void;
}
