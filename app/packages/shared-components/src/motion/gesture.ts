/**
 * 手势驱动（§2.5.9，跟手动画）：下拉刷新 / 侧滑返回 / 拖拽排序 / 滑动消除。
 */
import { onMounted, onUnmounted, type Ref } from "vue";
import { motion } from "./internal";
import { getMotionProfile } from "./guard";
import { DUR, EASE } from "./tokens";

/** 下拉刷新：下拉距离跟手，超阈值触发 onRefresh 并回弹 */
export function usePullRefresh(host: Ref<HTMLElement | null>, onRefresh: () => void): void {
  let startY = 0;
  let pulling = false;
  const onDown = (e: PointerEvent) => {
    const node = host.value;
    if (node && node.scrollTop <= 0) {
      pulling = true;
      startY = e.clientY;
    }
  };
  const onMove = (e: PointerEvent) => {
    if (!pulling) return;
    const node = host.value;
    if (!node) return;
    const dy = Math.max(0, e.clientY - startY);
    motion.set(node, { y: dy * 0.4 });
    if (dy > 80) {
      pulling = false;
      motion.to(node, { y: 0, duration: DUR.base, ease: EASE.out });
      onRefresh();
    }
  };
  const onUp = () => {
    if (!pulling) return;
    pulling = false;
    const node = host.value;
    if (node) motion.to(node, { y: 0, duration: DUR.fast, ease: EASE.out });
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

/** 侧滑返回：左缘跟手 x，松手超阈值触发 onCommit */
export function useSwipeBack(host: Ref<HTMLElement | null>, onCommit: () => void): void {
  let startX = 0;
  let dragging = false;
  const onDown = (e: PointerEvent) => {
    if (e.clientX > 24) return;
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

/** 拖拽排序：列表项跟手 + 占位让位（reorder） */
export function useDragSort(item: Ref<HTMLElement | null>): void {
  let startY = 0;
  let dragging = false;
  const onDown = (e: PointerEvent) => {
    dragging = true;
    startY = e.clientY;
  };
  const onMove = (e: PointerEvent) => {
    if (!dragging) return;
    const node = item.value;
    if (node) motion.set(node, { y: e.clientY - startY, zIndex: 10 });
  };
  const onUp = () => {
    if (!dragging) return;
    dragging = false;
    const node = item.value;
    if (node) motion.to(node, { y: 0, zIndex: 1, duration: DUR.fast, ease: EASE.out });
  };
  onMounted(() => {
    const node = item.value;
    if (!node || !getMotionProfile().enabled) return;
    node.addEventListener("pointerdown", onDown);
    node.addEventListener("pointermove", onMove);
    node.addEventListener("pointerup", onUp);
  });
  onUnmounted(() => {
    const node = item.value;
    if (!node) return;
    node.removeEventListener("pointerdown", onDown);
    node.removeEventListener("pointermove", onMove);
    node.removeEventListener("pointerup", onUp);
  });
}

/** 滑动消除：行内 x 跟手，超阈值飞出 + 高度塌缩 */
export function useSwipeDismiss(row: Ref<HTMLElement | null>, onDismiss: () => void): void {
  let startX = 0;
  let dragging = false;
  const onDown = (e: PointerEvent) => {
    dragging = true;
    startX = e.clientX;
  };
  const onMove = (e: PointerEvent) => {
    if (!dragging) return;
    const node = row.value;
    if (node) motion.set(node, { x: e.clientX - startX });
  };
  const onUp = (e: PointerEvent) => {
    if (!dragging) return;
    dragging = false;
    const node = row.value;
    if (!node) return;
    if (e.clientX - startX > 100) {
      motion.to(node, { x: 400, opacity: 0, duration: DUR.base, ease: EASE.inOut, onComplete: onDismiss });
    } else {
      motion.to(node, { x: 0, duration: DUR.fast, ease: EASE.out });
    }
  };
  onMounted(() => {
    const node = row.value;
    if (!node || !getMotionProfile().enabled) return;
    node.addEventListener("pointerdown", onDown);
    node.addEventListener("pointermove", onMove);
    node.addEventListener("pointerup", onUp);
  });
  onUnmounted(() => {
    const node = row.value;
    if (!node) return;
    node.removeEventListener("pointerdown", onDown);
    node.removeEventListener("pointermove", onMove);
    node.removeEventListener("pointerup", onUp);
  });
}
