/**
 * 引导与空态（§2.5.8）：首屏 splash / Onboarding / 新功能高亮 / 空状态漂浮。
 */
import { onMounted, type Ref } from "vue";
import { motion } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

/** 首屏 Logo 入场：mark scale/rotate + slogan 字符 stagger 上移 */
export function useSplash(logo: Ref<HTMLElement | null>, slogan: Ref<HTMLElement | null>): void {
  onMounted(() => {
    if (!getMotionProfile().enabled) return;
    if (logo.value) motion.from(logo.value, { scale: 0.8, rotate: -8, duration: DUR.slow, ease: EASE.back });
    if (slogan.value) {
      motion.from(Array.from(slogan.value.children), {
        y: 12,
        opacity: 0,
        duration: DUR.base,
        ease: EASE.out,
        stagger: 0.05,
      });
    }
  });
}

/** Onboarding 步骤卡：横移淡入 */
export function useOnboarding(card: Ref<HTMLElement | null>): void {
  onMounted(() => {
    const node = card.value;
    if (node && getMotionProfile().enabled) {
      motion.from(node, { x: 24, opacity: 0, duration: DUR.base, ease: EASE.out });
    }
  });
}

/** 新功能高亮：目标元素 outline 脉冲 + 提示气泡上移 */
export function useFeatureHint(target: Ref<HTMLElement | null>, bubble: Ref<HTMLElement | null>) {
  function play(): void {
    const t = target.value;
    const b = bubble.value;
    if ((!t && !b) || !getMotionProfile().enabled) return;
    if (t) {
      motion.fromTo(t, { outlineColor: "rgba(139,92,246,0.8)" }, { outlineColor: "rgba(139,92,246,0)", duration: 1.2, repeat: 2 });
    }
    if (b) motion.from(b, { y: 8, opacity: 0, duration: DUR.base, ease: EASE.out });
  }
  return { play };
}

/** 空状态插画：SVG 局部循环漂浮 */
export function useEmptyState(art: Ref<HTMLElement | null>): void {
  onMounted(() => {
    const node = art.value;
    if (node && getMotionProfile().enabled) {
      motion.to(node, { y: -6, duration: 1.6, ease: EASE.float, yoyo: true, repeat: -1 });
    }
  });
}
