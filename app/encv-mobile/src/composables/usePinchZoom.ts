/**
 * usePinchZoom.ts — AI 会话区域双指缩放 composable
 *
 * 用途：接管安卓 webview 的"双指缩放"手势，让业务控制缩放比例。
 *
 * 关键认知：viewport meta 已默认 user-scalable=no + maximum-scale=1.0
 *   - WebView 不再拦截双指捏合做整页缩放（避免破坏 UI 布局）
 *   - AgentChat 区域显式接管手势：双指距离变化 → 更新 zoomScale → 应用 CSS zoom
 *
 * 为什么用 CSS `zoom` 而不是 `transform: scale()`：
 *   - `transform: scale()` 是视觉变换：元素看起来变大但 layout box 不变
 *     → 父容器 overflow 区域不扩展 → 大内容"被裁切"，无法滚动浏览
 *     → 元素的 hit-test 区域仍是原大小（点击/滚动行为错位）
 *   - `zoom` 是真布局缩放：元素 layout box 真的变大
 *     → 大内容**溢出**父容器，可被父容器滚动浏览
 *     → hit-test 区域随缩放调整
 *     → 这是用户期望的"直接缩放比例溢出屏幕"行为
 *
 * 设计原则：
 * - bind / unbind 用 addEventListener / removeEventListener（不用 onMounted 内部绑定）
 *   调用方在 onMounted 调 bind(targetRef.value)，onUnmounted 调 unbind()
 * - 缩放严格 clamp 到 [minScale, maxScale]，超出范围不抛错
 *
 * SPEC: /workspace/.trae/specs/mobile-agent-polish-2026q2/spec.md "usePinchZoom composable"
 */

import { type Ref, ref } from "vue";

export interface UsePinchZoomOptions {
  /** 最小缩放比例，默认 0.5 */
  minScale?: number;
  /** 最大缩放比例，默认 1.5 */
  maxScale?: number;
  /** 初始缩放比例（也是 resetZoom 目标），默认 1.0 */
  initialScale?: number;
  /** 每次 zoomIn / zoomOut 步进量，默认 0.1 */
  step?: number;
  /** 双击重置的时间窗口（ms），默认 300 */
  doubleTapMs?: number;
}

export interface UsePinchZoomReturn {
  /** 当前缩放比例（响应式） */
  zoomScale: Ref<number>;
  /** 程序化放大（zoomScale += step） */
  zoomIn: () => void;
  /** 程序化缩小（zoomScale -= step） */
  zoomOut: () => void;
  /** 重置回 initialScale */
  resetZoom: () => void;
  /** 手动应用 transform（zoomIn/Out/Reset 已自动调用） */
  applyZoom: () => void;
  /** 绑定目标元素 touch 事件（onMounted 后调） */
  bind: (target: HTMLElement) => void;
  /** 解绑（onUnmounted 调） */
  unbind: () => void;
  /** 手动构造 TouchEvent（单测用） */
  __onTouchStart: (e: TouchEvent) => void;
  /** 手动构造 TouchEvent（单测用） */
  __onTouchMove: (e: TouchEvent) => void;
}

const DEFAULT_OPTIONS = {
  minScale: 0.5,
  maxScale: 1.5,
  initialScale: 1.0,
  step: 0.1,
  doubleTapMs: 300,
};

/**
 * 构造一个 mock TouchList-like 对象（单测用，避免 jsdom 不实现 Touch）
 */
function mockTouchList(items: Array<{ clientX: number; clientY: number }>): TouchList {
  // 真实 TouchList 是只读 + 类数组；用 array 直接 mock 即可
  return items as unknown as TouchList;
}

export function usePinchZoom(options: UsePinchZoomOptions = {}): UsePinchZoomReturn {
  const minScale = options.minScale ?? DEFAULT_OPTIONS.minScale;
  const maxScale = options.maxScale ?? DEFAULT_OPTIONS.maxScale;
  const initialScale = options.initialScale ?? DEFAULT_OPTIONS.initialScale;
  const step = options.step ?? DEFAULT_OPTIONS.step;
  const doubleTapMs = options.doubleTapMs ?? DEFAULT_OPTIONS.doubleTapMs;

  const zoomScale = ref(initialScale);

  // 闭包状态：目标元素 + 当前手势
  let target: HTMLElement | null = null;
  let initialDistance = 0;
  let initialScaleAtTouch = 1.0;
  let lastTapTime = 0;

  function touchDistance(touches: TouchList): number {
    if (!touches || touches.length < 2) return 0;
    const dx = touches[0].clientX - touches[1].clientX;
    const dy = touches[0].clientY - touches[1].clientY;
    return Math.sqrt(dx * dx + dy * dy);
  }

  function clamp(v: number): number {
    // 用 3 位小数 round 避免浮点累加误差（0.1+0.1+0.1=0.30000000000000004 → 0.3）
    // step=0.05 时也只到 2 位小数，3 位足够
    const rounded = Math.round(v * 1000) / 1000;
    return Math.max(minScale, Math.min(maxScale, rounded));
  }

  function applyZoom(): void {
    if (!target) return;
    // CSS zoom：真布局缩放，元素 layout box 变大 → 大内容溢出父容器 → 父容器可滚动浏览
    // 浏览器支持：Chrome / Edge / Safari / Firefox（Firefox 126+ 已默认启用）
    target.style.zoom = String(zoomScale.value);
  }

  function onTouchStart(e: TouchEvent): void {
    if (e.touches.length === 2) {
      // 双指：开始记录基准
      initialDistance = touchDistance(e.touches);
      initialScaleAtTouch = zoomScale.value;
      if (e.cancelable) e.preventDefault();
    } else if (e.touches.length === 1) {
      // 单指：检测双击
      const now = Date.now();
      if (lastTapTime > 0 && now - lastTapTime < doubleTapMs) {
        resetZoom();
        if (e.cancelable) e.preventDefault();
        lastTapTime = 0;
      } else {
        lastTapTime = now;
      }
    }
  }

  function onTouchMove(e: TouchEvent): void {
    if (e.touches.length !== 2) return;
    if (initialDistance === 0) return;
    if (e.cancelable) e.preventDefault();
    const currentDistance = touchDistance(e.touches);
    if (currentDistance === 0) return;
    const ratio = currentDistance / initialDistance;
    zoomScale.value = clamp(initialScaleAtTouch * ratio);
    applyZoom();
  }

  function bind(t: HTMLElement): void {
    if (target) {
      // 重复绑定：先解旧的（避免内存泄漏）
      unbind();
    }
    target = t;
    t.addEventListener("touchstart", onTouchStart as EventListener, { passive: false });
    t.addEventListener("touchmove", onTouchMove as EventListener, { passive: false });
  }

  function unbind(): void {
    if (!target) return;
    target.removeEventListener("touchstart", onTouchStart as EventListener);
    target.removeEventListener("touchmove", onTouchMove as EventListener);
    target = null;
    initialDistance = 0;
    initialScaleAtTouch = 1.0;
    lastTapTime = 0;
  }

  function zoomIn(): void {
    zoomScale.value = clamp(zoomScale.value + step);
    applyZoom();
  }

  function zoomOut(): void {
    zoomScale.value = clamp(zoomScale.value - step);
    applyZoom();
  }

  function resetZoom(): void {
    zoomScale.value = initialScale;
    applyZoom();
  }

  return {
    zoomScale,
    zoomIn,
    zoomOut,
    resetZoom,
    applyZoom,
    bind,
    unbind,
    // 单测 / 内部 hook
    __onTouchStart: onTouchStart,
    __onTouchMove: onTouchMove,
  };
}

// 导出工具函数供单测复用
export { mockTouchList };
