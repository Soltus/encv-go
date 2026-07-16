/**
 * 无动画（no-op）引擎：MotionEngine 的「零开销」实现。
 *
 * 用途：
 *   - 测试 / 低性能设备 / SSR 环境下把动效一键关掉，只落终态、不产生任何补间；
 *   - 作为 ACL 的「参考另实现」，证明换底层动画库只需新增一个实现 MotionEngine
 *     的文件并切换 internal/index.ts 的导出（见该文件顶部说明），下游零改动。
 *
 * 语义正确性：composable 在 no-op 下元素停在「自然 CSS 状态」——
 * 对 reveal/transition 这类「从隐藏到可见」的动画，自然态即终态（可见、不闪隐），
 * 故 no-op 是安全的「瞬时落终态」等价物。
 */
import type { MotionEngine, MotionTween, MotionContext, ScrollTriggerHandle, FlipState } from "./types";

const NOOP_TWEEN: MotionTween = { kill() {} };
const NOOP_CONTEXT: MotionContext = { revert() {} };
const NOOP_TRIGGER: ScrollTriggerHandle = { kill() {}, progress: 0 };

class NoopEngine implements MotionEngine {
  registerPlugins(): void {
    /* 无需注册任何插件 */
  }
  set(): void {
    /* 直接停在自然态，不做任何内联覆盖 */
  }
  to(): MotionTween {
    return NOOP_TWEEN;
  }
  from(): MotionTween {
    return NOOP_TWEEN;
  }
  fromTo(): MotionTween {
    return NOOP_TWEEN;
  }
  context(_fn: () => void, _scope: Element): MotionContext {
    return NOOP_CONTEXT;
  }
  quickTo(): (value: number) => void {
    return () => {};
  }
  delayedCall(): MotionTween {
    return NOOP_TWEEN;
  }
  createScrollTrigger(): ScrollTriggerHandle {
    return NOOP_TRIGGER;
  }
  refreshScrollTriggers(): void {
    /* no-op：无滚动触发器需要刷新 */
  }
  flipGetState(): FlipState {
    return undefined;
  }
  flipFrom(): void {
    /* 不播放入场动画，元素保持当前布局 */
  }
}

export const noopMotion: MotionEngine = new NoopEngine();
