/**
 * 微交互（§2.5.3）：按压 / 悬浮 / 波纹 / 开关 / 分段控件 / 复选框描边。
 * 全部受 guard 闸门管控，reduced-motion 下不播动画。
 */
import { onMounted, onUnmounted, watch, type Ref } from "vue";
import { motion } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

function disabled(): boolean {
  return !getMotionProfile().enabled;
}

/** 按钮按压：active 时 scale 0.96 弹性回弹 */
export function usePress(el: Ref<HTMLElement | null>): void {
  const onDown = () => {
    const node = el.value;
    if (node && !disabled()) motion.to(node, { scale: 0.96, duration: 0.08, ease: EASE.out });
  };
  const onUp = () => {
    const node = el.value;
    if (node && !disabled()) motion.to(node, { scale: 1, duration: 0.24, ease: EASE.back });
  };
  onMounted(() => {
    const node = el.value;
    if (!node) return;
    node.addEventListener("pointerdown", onDown);
    node.addEventListener("pointerup", onUp);
    node.addEventListener("pointerleave", onUp);
  });
  onUnmounted(() => {
    const node = el.value;
    if (!node) return;
    node.removeEventListener("pointerdown", onDown);
    node.removeEventListener("pointerup", onUp);
    node.removeEventListener("pointerleave", onUp);
  });
}

/** 卡片点击波纹：pointer 位置 scale 0->1 / opacity->0 圆形 ripple */
export function useRipple(el: Ref<HTMLElement | null>): void {
  onMounted(() => {
    const node = el.value;
    if (!node) return;
    node.addEventListener("pointerdown", (e: PointerEvent) => {
      if (disabled()) return;
      const rect = node.getBoundingClientRect();
      const size = Math.max(rect.width, rect.height);
      const ripple = document.createElement("span");
      ripple.className = "encv-ripple";
      motion.set(ripple, {
        position: "absolute",
        left: e.clientX - rect.left - size / 2,
        top: e.clientY - rect.top - size / 2,
        width: size,
        height: size,
        borderRadius: "9999px",
        background: "currentColor",
        opacity: 0.28,
        scale: 0,
        pointerEvents: "none",
      });
      node.appendChild(ripple);
      motion.to(ripple, {
        scale: 1,
        opacity: 0,
        duration: 0.5,
        ease: EASE.out,
        onComplete: () => ripple.remove(),
      });
    });
  });
}

/** 卡片悬浮：hover 抬升 y -4 */
export function useHover(el: Ref<HTMLElement | null>): void {
  onMounted(() => {
    const node = el.value;
    if (!node) return;
    node.addEventListener("pointerenter", () => {
      if (!disabled()) motion.to(node, { y: -4, duration: DUR.fast, ease: EASE.out });
    });
    node.addEventListener("pointerleave", () => {
      if (!disabled()) motion.to(node, { y: 0, duration: DUR.fast, ease: EASE.out });
    });
  });
}

/** 开关 Toggle：滑块 x 位移 + 弹性回弹（随 on 状态变化） */
export function useToggle(el: Ref<HTMLElement | null>, on: Ref<boolean>): void {
  const apply = (value: boolean) => {
    const node = el.value;
    if (!node) return;
    const p = getMotionProfile();
    const x = value ? 18 : 0;
    if (!p.enabled) {
      motion.set(node, { x });
      return;
    }
    motion.to(node, { x, duration: DUR.fast * p.intensity, ease: EASE.out });
  };
  onMounted(() => apply(on.value));
  const stop = watch(on, v => apply(v));
  onUnmounted(() => stop());
}

/** 分段控件：下划线 / 滑块在选项间滑动（morph 指示条） */
export function useSegmented(indicator: Ref<HTMLElement | null>, target: Ref<HTMLElement | null>) {
  function move(): void {
    const node = indicator.value;
    const to = target.value;
    if (!node || !to || disabled()) return;
    const rect = to.getBoundingClientRect();
    motion.to(node, { width: rect.width, x: to.offsetLeft, duration: DUR.fast, ease: EASE.out });
  }
  return { move };
}

/** 复选框：对勾 SVG 用 strokeDashoffset 一笔画出 */
export function useCheckbox(path: Ref<SVGPathElement | null>, checked: Ref<boolean>): void {
  const draw = (value: boolean) => {
    const node = path.value;
    if (!node) return;
    const len = node.getTotalLength();
    const p = getMotionProfile();
    if (!p.enabled) {
      motion.set(node, { strokeDasharray: len, strokeDashoffset: value ? 0 : len });
      return;
    }
    motion.fromTo(
      node,
      { strokeDasharray: len, strokeDashoffset: value ? len : 0 },
      { strokeDashoffset: value ? 0 : len, duration: DUR.fast, ease: EASE.out }
    );
  };
  onMounted(() => draw(checked.value));
  const stop = watch(checked, v => draw(v));
  onUnmounted(() => stop());
}
