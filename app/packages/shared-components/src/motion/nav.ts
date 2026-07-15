/**
 * 导航反馈（§2.5.5）：Tab 切换 / FAB 放射展开 / Search 展开 / 左缘返回手势。
 */
import { onMounted, onUnmounted, type Ref } from "vue";
import { motion } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

/** Tab 切换：图标弹性回弹 */
export function useTabSwitch(icon: Ref<HTMLElement | null>) {
  function bump(): void {
    const node = icon.value;
    if (node && getMotionProfile().enabled) {
      motion.fromTo(node, { scale: 0.8 }, { scale: 1, duration: DUR.fast, ease: EASE.back });
    }
  }
  return { bump };
}

/** FAB 展开：子按钮放射状出现 + 主按钮旋转 */
export function useFab(container: Ref<HTMLElement | null>) {
  function expand(open: boolean): void {
    const node = container.value;
    if (!node || !getMotionProfile().enabled) return;
    const items = Array.from(node.querySelectorAll("[data-fab-item]"));
    motion.to(items, {
      scale: open ? 1 : 0,
      rotate: open ? 0 : -45,
      opacity: open ? 1 : 0,
      duration: DUR.base,
      ease: EASE.back,
      stagger: 0.04,
    });
  }
  return { expand };
}

/** Search 展开：搜索栏从图标态展开为全宽，input 淡入 */
export function useSearchExpand(bar: Ref<HTMLElement | null>, field: Ref<HTMLElement | null>) {
  function toggle(expanded: boolean): void {
    const node = bar.value;
    if (!node || !getMotionProfile().enabled) return;
    motion.to(node, { width: expanded ? "100%" : "44px", duration: DUR.base, ease: EASE.inOut });
    if (field.value) motion.to(field.value, { opacity: expanded ? 1 : 0, duration: DUR.fast });
  }
  return { toggle };
}

/** 左缘返回手势：跟手 x 位移，松手超阈值触发 onCommit */
export function useBackGesture(host: Ref<HTMLElement | null>, onCommit: () => void): void {
  let startX = 0;
  let dragging = false;
  const onDown = (e: PointerEvent) => {
    if (e.clientX > 24) return; // 仅左缘
    dragging = true;
    startX = e.clientX;
  };
  const onMove = (e: PointerEvent) => {
    if (!dragging) return;
    const node = host.value;
    if (node) motion.set(node, { x: Math.max(0, e.clientX - startX) });
  };
  const onUp = (e: PointerEvent) => {
    if (!dragging) return;
    dragging = false;
    const node = host.value;
    if (!node) return;
    if (e.clientX - startX > 120) onCommit();
    motion.to(node, { x: 0, duration: DUR.fast, ease: EASE.out });
  };
  onMounted(() => {
    const node = host.value;
    if (!node || !getMotionProfile().enabled) return;
    node.addEventListener("pointerdown", onDown);
    node.addEventListener("pointermove", onMove);
    node.addEventListener("pointerup", onUp);
  });
  onUnmounted(() => {
    const node = host.value;
    if (!node) return;
    node.removeEventListener("pointerdown", onDown);
    node.removeEventListener("pointermove", onMove);
    node.removeEventListener("pointerup", onUp);
  });
}
