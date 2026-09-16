/**
 * 浮层动效（§2.5.4）：Dialog / BottomSheet / Drawer / Toast / Tooltip / 级联菜单。
 * 统一由 animateOverlay 驱动方向，随 open 状态切换；reduced-motion 下只切 opacity 终态。
 */
import { onMounted, onUnmounted, watch, type Ref } from "vue";
import { motion, type MotionVars } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

type Dir = "scale" | "up" | "left" | "right";

const FROM: Record<Dir, MotionVars> = {
  scale: { scale: 0.95, y: 8, opacity: 0 },
  up: { y: 20, opacity: 0 },
  left: { x: -40, opacity: 0 },
  right: { x: 40, opacity: 0 },
};

function animateOverlay(panel: HTMLElement, open: boolean, dir: Dir, intensity: number): void {
  if (open) {
    motion.fromTo(panel, FROM[dir], {
      x: 0,
      y: 0,
      scale: 1,
      opacity: 1,
      duration: DUR.base * intensity,
      ease: EASE.out,
    });
  } else {
    motion.to(panel, { ...FROM[dir], duration: DUR.fast * intensity, ease: EASE.inOut });
  }
}

function make(dir: Dir) {
  return (panel: Ref<HTMLElement | null>, open: Ref<boolean>) => {
    const run = () => {
      const node = panel.value;
      if (!node) return;
      const p = getMotionProfile();
      if (!p.enabled) {
        motion.set(node, { opacity: open.value ? 1 : 0 });
        return;
      }
      animateOverlay(node, open.value, dir, p.intensity);
    };
    onMounted(run);
    const stop = watch(open, run);
    onUnmounted(() => stop());
  };
}

export const useDialog = make("scale");
export const useSheet = make("up");
export const useToast = make("up");
export const useTooltip = make("scale");
export const useDrawer = make("left");

/** 级联菜单：菜单项 stagger 上移入场 */
export function useMenu(panel: Ref<HTMLElement | null>, open: Ref<boolean>): void {
  const run = () => {
    const node = panel.value;
    if (!node) return;
    const p = getMotionProfile();
    if (!p.enabled) {
      motion.set(node, { opacity: open.value ? 1 : 0 });
      return;
    }
    if (open.value) {
      motion.fromTo(
        Array.from(node.children),
        { y: 6, opacity: 0 },
        { y: 0, opacity: 1, duration: DUR.fast * p.intensity, ease: EASE.out, stagger: 0.03 * p.intensity }
      );
    } else {
      motion.to(node, { opacity: 0, duration: DUR.fast });
    }
  };
  onMounted(run);
  const stop = watch(open, run);
  onUnmounted(() => stop());
}
