/**
 * 磁性按钮（§2.5.3，useMagnetic）。
 * 鼠标靠近时朝指针微移（引擎 quickTo 跟手），离开归位。
 */
import { onMounted, onUnmounted, type Ref } from "vue";
import { motion } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

export function useMagnetic(el: Ref<HTMLElement | null>, strength = 0.3): void {
  let xTo: ((v: number) => void) | undefined;
  let yTo: ((v: number) => void) | undefined;

  const onMove = (e: PointerEvent) => {
    const node = el.value;
    if (!node || !getMotionProfile().enabled) return;
    const rect = node.getBoundingClientRect();
    const mx = e.clientX - (rect.left + rect.width / 2);
    const my = e.clientY - (rect.top + rect.height / 2);
    xTo?.(mx * strength);
    yTo?.(my * strength);
  };
  const onLeave = () => {
    xTo?.(0);
    yTo?.(0);
  };

  onMounted(() => {
    const node = el.value;
    if (!node) return;
    xTo = motion.quickTo(node, "x", { duration: DUR.fast, ease: EASE.out });
    yTo = motion.quickTo(node, "y", { duration: DUR.fast, ease: EASE.out });
    node.addEventListener("pointermove", onMove);
    node.addEventListener("pointerleave", onLeave);
  });
  onUnmounted(() => {
    const node = el.value;
    if (!node) return;
    node.removeEventListener("pointermove", onMove);
    node.removeEventListener("pointerleave", onLeave);
  });
}
