/**
 * 路由 / 页面转场（§2.5.1）。
 * 挂载时元素从下方淡入（surface 微缩放 + 内容上移）；reduced-motion 下直接落终态。
 */
import { onMounted, onUnmounted, type Ref } from "vue";
import { motion, type MotionContext } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

export function usePageTransition(el: Ref<HTMLElement | null>): void {
  let ctx: MotionContext | undefined;
  onMounted(() => {
    const node = el.value;
    if (!node) return;
    const p = getMotionProfile();
    if (!p.enabled) {
      motion.set(node, { clearProps: "all" });
      return;
    }
    ctx = motion.context(() => {
      motion.from(node, {
        y: 12 * p.intensity,
        opacity: 0,
        duration: DUR.base * p.intensity,
        ease: EASE.out,
        // 结束后清除内联 opacity/transform，让元素完全回到 CSS 控制，
        // 杜绝任何「卡在 opacity:0」导致整页空白但可点击的残留。
        clearProps: "opacity,transform",
      });
    }, node);
  });
  onUnmounted(() => ctx?.revert());
}
