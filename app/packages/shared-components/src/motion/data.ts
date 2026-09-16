/**
 * 数据 / 状态反馈（§2.5.6）：数字滚动 / 进度条 / 骨架 shimmer / 成功对勾 / Confetti。
 */
import { onMounted, onUnmounted, watch, type Ref } from "vue";
import { motion } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

/** 数字滚动 count-up：数值变化时缓动写入 DOM */
export function useCountUp(el: Ref<HTMLElement | null>, to: Ref<number> | number, dur = 0.8): void {
  const target = () => (typeof to === "number" ? to : to.value);
  const run = () => {
    const node = el.value;
    if (!node) return;
    const p = getMotionProfile();
    if (!p.enabled) {
      node.textContent = target().toLocaleString();
      return;
    }
    const obj = { v: 0 };
    motion.to(obj, {
      v: target(),
      duration: dur * p.intensity,
      ease: EASE.inOut,
      onUpdate: () => (node.textContent = Math.round(obj.v).toLocaleString()),
    });
  };
  onMounted(run);
  if (typeof to !== "number") {
    const stop = watch(to, run);
    onUnmounted(stop);
  }
}

/** 进度条：scaleX 0->1，可中断续跑 */
export function useProgress(el: Ref<HTMLElement | null>, value: Ref<number>): void {
  const run = () => {
    const node = el.value;
    if (!node) return;
    const p = getMotionProfile();
    const t = Math.max(0, Math.min(1, value.value));
    if (!p.enabled) {
      motion.set(node, { scaleX: t });
      return;
    }
    motion.to(node, { scaleX: t, duration: DUR.base * p.intensity, ease: EASE.inOut, transformOrigin: "left center" });
  };
  onMounted(run);
  const stop = watch(value, run);
  onUnmounted(stop);
}

/** 骨架屏 shimmer：渐变高光循环漂移 */
export function useShimmer(el: Ref<HTMLElement | null>): void {
  onMounted(() => {
    const node = el.value;
    if (node && getMotionProfile().enabled) {
      motion.to(node, { backgroundPositionX: "100%", duration: 1.2, ease: EASE.linear, repeat: -1 });
    }
  });
}

/** 成功对勾：外圈 scale 弹 + 对勾描边一笔画出 */
export function useSuccess(circle: Ref<HTMLElement | null>, check: Ref<SVGPathElement | null>) {
  function play(): void {
    const c = circle.value;
    const path = check.value;
    if (!c || !path) return;
    const p = getMotionProfile();
    if (!p.enabled) {
      motion.set(c, { scale: 1 });
      const len = path.getTotalLength();
      motion.set(path, { strokeDasharray: len, strokeDashoffset: 0 });
      return;
    }
    const len = path.getTotalLength();
    motion.fromTo(c, { scale: 0.6 }, { scale: 1, duration: DUR.base, ease: EASE.back });
    motion.fromTo(path, { strokeDasharray: len, strokeDashoffset: len }, { strokeDashoffset: 0, duration: DUR.slow, ease: EASE.out });
  }
  return { play };
}

/** Confetti 成就：粒子从中心喷射（轻量自写 physics，非外挂） */
export function useConfetti(host: Ref<HTMLElement | null>) {
  function burst(): void {
    const node = host.value;
    if (!node || !getMotionProfile().enabled) return;
    const p = getMotionProfile();
    const colors = ["#8b5cf6", "#06b6d4", "#ec4899", "#22c55e"];
    const count = Math.round(24 * p.intensity);
    for (let i = 0; i < count; i++) {
      const dot = document.createElement("span");
      dot.className = "encv-confetti";
      motion.set(dot, {
        position: "absolute",
        left: "50%",
        top: "50%",
        width: 8,
        height: 8,
        background: colors[i % colors.length],
        borderRadius: "2px",
        x: 0,
        y: 0,
        opacity: 1,
      });
      node.appendChild(dot);
      const angle = (Math.PI * 2 * i) / count;
      motion.to(dot, {
        x: Math.cos(angle) * 120,
        y: Math.sin(angle) * 120,
        opacity: 0,
        rotation: 360,
        duration: 0.9,
        ease: EASE.out,
        onComplete: () => dot.remove(),
      });
    }
  }
  return { burst };
}
