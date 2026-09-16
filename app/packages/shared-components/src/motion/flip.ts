/**
 * 共享元素转场（§2.5.1，Flip 插件）。
 * 在状态切换前 record()，切换后 play()：Flip 自动做尺寸 / 位置 morph。
 */
import { type Ref } from "vue";
import { motion, type FlipState } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

export function useSharedElement(el: Ref<HTMLElement | null>) {
  let state: FlipState | null = null;

  function record(): void {
    const node = el.value;
    if (node) state = motion.flipGetState(node);
  }

  function play(): void {
    const node = el.value;
    if (!node || !state) return;
    const p = getMotionProfile();
    if (!p.enabled) {
      state = null;
      return;
    }
    motion.flipFrom(state, {
      duration: DUR.base * p.intensity,
      ease: EASE.inOut,
      absolute: true,
    });
    state = null;
  }

  return { record, play };
}
