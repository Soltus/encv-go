/**
 * 氛围视觉（§2.5.7）：主题光泽过渡 / Aurora 背景 / 光标辉光 / 文字 scramble。
 */
import { onMounted, onUnmounted, watch, type Ref } from "vue";
import { motion, type MotionTween } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

/** 主题光泽过渡：切 data-theme 时整页 brightness 微脉冲（供 theme.js mount 调用） */
export function useThemeIntro() {
  function play(): void {
    if (!getMotionProfile().enabled) return;
    motion.fromTo(
      document.documentElement,
      { filter: "brightness(1.15)" },
      { filter: "brightness(1)", duration: DUR.slow, ease: EASE.out }
    );
  }
  return { play };
}

/** Aurora / 网格背景：多 background-position 慢速漂移 */
export function useAurora(el: Ref<HTMLElement | null>): void {
  let tween: MotionTween | undefined;
  onMounted(() => {
    const node = el.value;
    if (node && getMotionProfile().enabled) {
      tween = motion.to(node, { backgroundPositionX: "200%", duration: 18, ease: EASE.linear, repeat: -1 });
    }
  });
  onUnmounted(() => tween?.kill());
}

/** 光标辉光：跟随指针的径向光斑（引擎 quickTo 跟手，离开淡出由调用方控制） */
export function useSpotlight(el: Ref<HTMLElement | null>): void {
  let xTo: ((v: number) => void) | undefined;
  let yTo: ((v: number) => void) | undefined;
  const onMove = (e: PointerEvent) => {
    const node = el.value;
    if (!node || !getMotionProfile().enabled) return;
    const rect = node.getBoundingClientRect();
    xTo?.(e.clientX - rect.left);
    yTo?.(e.clientY - rect.top);
  };
  onMounted(() => {
    const node = el.value;
    if (!node) return;
    xTo = motion.quickTo(node, "x", { duration: 0.4, ease: EASE.follow });
    yTo = motion.quickTo(node, "y", { duration: 0.4, ease: EASE.follow });
    node.addEventListener("pointermove", onMove);
  });
  onUnmounted(() => {
    const node = el.value;
    if (node) node.removeEventListener("pointermove", onMove);
  });
}

/** 文字 scramble：字符随机乱码 -> 落定（用于主题名 / 品牌位） */
export function useScramble(el: Ref<HTMLElement | null>, text: Ref<string>): void {
  const run = () => {
    const node = el.value;
    if (!node) return;
    const p = getMotionProfile();
    const final = text.value;
    if (!p.enabled) {
      node.textContent = final;
      return;
    }
    const chars = "!<>-_\\/[]{}—=+*^?#________";
    let frame = 0;
    const total = 20;
    const tick = () => {
      const progress = frame / total;
      node.textContent = final
        .split("")
        .map((ch, i) => {
          if (ch === " ") return " ";
          if (i < progress * final.length) return ch;
          return chars[Math.floor(Math.random() * chars.length)];
        })
        .join("");
      frame++;
      if (frame <= total) motion.delayedCall(0.03, tick);
    };
    tick();
  };
  onMounted(run);
  const stop = watch(text, run);
  onUnmounted(stop);
}
