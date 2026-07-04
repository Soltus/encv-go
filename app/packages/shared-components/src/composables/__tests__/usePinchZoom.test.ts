/**
 * usePinchZoom.test.ts
 *
 * 覆盖：
 * 1. 初始状态 / 默认值
 * 2. zoomIn / zoomOut 步进
 * 3. clamp 到 [minScale, maxScale]
 * 4. resetZoom 回 initialScale
 * 5. applyZoom 写入 CSS zoom style（真布局缩放，不是 transform: scale 视觉变换）
 * 6. bind / unbind 生命周期
 * 7. 双指距离变化 → zoomScale 更新
 * 8. 双击重置（300ms 窗口）
 * 9. 自定义 options
 *
 * 关键 mock 策略：
 * - TouchEvent 在 jsdom 中未完整实现 → 自定义 class 模拟 touches/length/cancelable
 * - 使用 __onTouchStart / __onTouchMove 内部 hook 注入 TouchEvent（避免依赖 addEventListener 触发）
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockTouchList, usePinchZoom } from "@encv/shared-components/composables/usePinchZoom";

/**
 * 构造 mock TouchEvent
 * jsdom 没有 Touch / TouchEvent 完整实现 → 用普通 class 模拟
 */
function makeTouchEvent(opts: { touches: Array<{ clientX: number; clientY: number }>; cancelable?: boolean }): Event {
  const e = new Event("touchstart", { cancelable: opts.cancelable ?? true });
  Object.defineProperty(e, "touches", {
    value: mockTouchList(opts.touches),
    writable: false,
  });
  Object.defineProperty(e, "length", { value: opts.touches.length });
  return e;
}

describe("usePinchZoom - 初始状态 / 默认值", () => {
  it("TestPinch_Defaults: 默认 initialScale=1.0, minScale=0.5, maxScale=1.5, step=0.1", () => {
    const pz = usePinchZoom();
    expect(pz.zoomScale.value).toBe(1.0);
  });

  it("TestPinch_CustomOptions: 自定义 initialScale / minScale / maxScale / step", () => {
    const pz = usePinchZoom({
      initialScale: 0.8,
      minScale: 0.3,
      maxScale: 2.0,
      step: 0.2,
    });
    expect(pz.zoomScale.value).toBe(0.8);
    pz.zoomIn();
    expect(pz.zoomScale.value).toBe(1.0); // 0.8 + 0.2
    pz.zoomIn();
    expect(pz.zoomScale.value).toBe(1.2);
  });
});

describe("usePinchZoom - zoomIn / zoomOut 步进", () => {
  it("TestPinch_ZoomIn_Step: zoomIn 增加 0.1", () => {
    const pz = usePinchZoom();
    pz.zoomIn();
    expect(pz.zoomScale.value).toBe(1.1);
    pz.zoomIn();
    expect(pz.zoomScale.value).toBe(1.2);
  });

  it("TestPinch_ZoomOut_Step: zoomOut 减少 0.1", () => {
    const pz = usePinchZoom();
    pz.zoomOut();
    expect(pz.zoomScale.value).toBe(0.9);
    pz.zoomOut();
    expect(pz.zoomScale.value).toBe(0.8);
  });

  it("TestPinch_ZoomInOut_Alternating: zoomIn/zoomOut 交替", () => {
    const pz = usePinchZoom();
    pz.zoomIn(); // 1.1
    pz.zoomIn(); // 1.2
    pz.zoomOut(); // 1.1
    expect(pz.zoomScale.value).toBe(1.1);
  });
});

describe("usePinchZoom - clamp 范围限制", () => {
  it("TestPinch_Clamp_UpperBound: zoomIn 到 maxScale 后不再增加", () => {
    const pz = usePinchZoom({ initialScale: 1.4, step: 0.1 });
    pz.zoomIn(); // 1.5
    pz.zoomIn(); // clamp 1.5
    pz.zoomIn(); // clamp 1.5
    expect(pz.zoomScale.value).toBe(1.5);
  });

  it("TestPinch_Clamp_LowerBound: zoomOut 到 minScale 后不再减少", () => {
    const pz = usePinchZoom({ initialScale: 0.6, step: 0.1 });
    pz.zoomOut(); // 0.5
    pz.zoomOut(); // clamp 0.5
    pz.zoomOut(); // clamp 0.5
    expect(pz.zoomScale.value).toBe(0.5);
  });

  it("TestPinch_Clamp_DefaultBound: 默认 max=1.5 限制", () => {
    const pz = usePinchZoom({ initialScale: 1.5 });
    pz.zoomIn();
    expect(pz.zoomScale.value).toBe(1.5);
  });

  it("TestPinch_Clamp_DefaultLowerBound: 默认 min=0.5 限制", () => {
    const pz = usePinchZoom({ initialScale: 0.5 });
    pz.zoomOut();
    expect(pz.zoomScale.value).toBe(0.5);
  });
});

describe("usePinchZoom - resetZoom 重置", () => {
  it("TestPinch_Reset_BackToInitial: resetZoom 回到 initialScale", () => {
    const pz = usePinchZoom({ initialScale: 0.9 });
    pz.zoomIn(); // 1.0
    pz.zoomIn(); // 1.1
    pz.resetZoom();
    expect(pz.zoomScale.value).toBe(0.9);
  });

  it("TestPinch_Reset_Default: 默认 initialScale=1.0", () => {
    const pz = usePinchZoom();
    pz.zoomOut(); // 0.9
    pz.zoomOut(); // 0.8
    pz.resetZoom();
    expect(pz.zoomScale.value).toBe(1.0);
  });
});

describe("usePinchZoom - applyZoom 写 CSS zoom", () => {
  let el: HTMLElement;

  beforeEach(() => {
    el = document.createElement("div");
    document.body.appendChild(el);
  });

  afterEach(() => {
    document.body.removeChild(el);
  });

  it("TestPinch_ApplyZoom_WritesZoom: applyZoom 把 scale 写到 style.zoom", () => {
    const pz = usePinchZoom();
    pz.bind(el);
    pz.zoomIn(); // 1.1
    expect(el.style.zoom).toBe("1.1");
  });

  it("TestPinch_ApplyZoom_NoOpWhenNotBound: 未 bind 时 applyZoom 是 no-op（不抛错）", () => {
    const pz = usePinchZoom();
    // 直接调 applyZoom 不应抛错
    expect(() => pz.applyZoom()).not.toThrow();
    // zoomScale 仍然更新（ref 不依赖 target）
    pz.zoomIn();
    expect(pz.zoomScale.value).toBe(1.1);
  });

  it("TestPinch_ApplyZoom_ResetWritesZoom: resetZoom 把 zoom 重置", () => {
    const pz = usePinchZoom({ initialScale: 1.0 });
    pz.bind(el);
    pz.zoomIn();
    pz.resetZoom();
    expect(el.style.zoom).toBe("1");
  });
});

describe("usePinchZoom - bind / unbind 生命周期", () => {
  let el: HTMLElement;

  beforeEach(() => {
    el = document.createElement("div");
    document.body.appendChild(el);
  });

  afterEach(() => {
    document.body.removeChild(el);
  });

  it("TestPinch_Bind_RegistersListeners: bind 注册 touchstart / touchmove 监听器", () => {
    const pz = usePinchZoom();
    const startSpy = vi.spyOn(el, "addEventListener");
    pz.bind(el);
    expect(startSpy).toHaveBeenCalledWith("touchstart", expect.any(Function), expect.objectContaining({ passive: false }));
    expect(startSpy).toHaveBeenCalledWith("touchmove", expect.any(Function), expect.objectContaining({ passive: false }));
  });

  it("TestPinch_Unbind_RemovesListeners: unbind 移除监听器", () => {
    const pz = usePinchZoom();
    pz.bind(el);
    const removeSpy = vi.spyOn(el, "removeEventListener");
    pz.unbind();
    expect(removeSpy).toHaveBeenCalledWith("touchstart", expect.any(Function));
    expect(removeSpy).toHaveBeenCalledWith("touchmove", expect.any(Function));
  });

  it("TestPinch_Bind_Replaces: 重复 bind 自动解绑旧的", () => {
    const pz = usePinchZoom();
    const el2 = document.createElement("div");
    document.body.appendChild(el2);
    try {
      pz.bind(el);
      const removeSpy = vi.spyOn(el, "removeEventListener");
      pz.bind(el2); // 重复 bind
      expect(removeSpy).toHaveBeenCalled(); // 旧的 el 被解绑
    } finally {
      document.body.removeChild(el2);
    }
  });

  it("TestPinch_Unbind_AfterUnbind_NoEffect: 多次 unbind 安全", () => {
    const pz = usePinchZoom();
    pz.bind(el);
    pz.unbind();
    expect(() => pz.unbind()).not.toThrow();
  });
});

describe("usePinchZoom - 双指距离变化", () => {
  let el: HTMLElement;

  beforeEach(() => {
    el = document.createElement("div");
    document.body.appendChild(el);
  });

  afterEach(() => {
    document.body.removeChild(el);
  });

  it("TestPinch_PinchOut_Increases: 双指距离变大 → zoomScale 变大", () => {
    const pz = usePinchZoom();
    pz.bind(el);
    // 双指起始：距离 = sqrt((100-0)^2 + 0) = 100
    pz.__onTouchStart(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 100, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    expect(pz.zoomScale.value).toBe(1.0); // 起始不变

    // 双指移动：距离 = 200 → ratio = 2 → scale = 1.0 * 2 = 2.0 → clamp 1.5
    pz.__onTouchMove(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 200, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    expect(pz.zoomScale.value).toBe(1.5);
  });

  it("TestPinch_PinchIn_Decreases: 双指距离变小 → zoomScale 变小", () => {
    const pz = usePinchZoom();
    pz.bind(el);
    // 起始距离 = 100
    pz.__onTouchStart(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 100, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    // 缩到 50 → ratio = 0.5 → scale = 0.5
    pz.__onTouchMove(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 50, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    expect(pz.zoomScale.value).toBe(0.5);
  });

  it("TestPinch_Pinch_ClampUpper: 双指放大到 3 倍 → clamp 1.5", () => {
    const pz = usePinchZoom();
    pz.bind(el);
    pz.__onTouchStart(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 100, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    pz.__onTouchMove(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 300, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    expect(pz.zoomScale.value).toBe(1.5);
  });

  it("TestPinch_Pinch_ClampLower: 双指缩小到 0.1 倍 → clamp 0.5", () => {
    const pz = usePinchZoom();
    pz.bind(el);
    pz.__onTouchStart(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 100, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    pz.__onTouchMove(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 10, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    expect(pz.zoomScale.value).toBe(0.5);
  });

  it("TestPinch_Move_BeforeStart: 没 start 就 move → 忽略", () => {
    const pz = usePinchZoom({ initialScale: 1.0 });
    pz.bind(el);
    // 没有 __onTouchStart
    pz.__onTouchMove(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 200, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    expect(pz.zoomScale.value).toBe(1.0); // 没变化
  });

  it("TestPinch_Move_SingleFinger: 单指 move → 忽略", () => {
    const pz = usePinchZoom();
    pz.bind(el);
    // 双指 start
    pz.__onTouchStart(
      makeTouchEvent({
        touches: [
          { clientX: 0, clientY: 0 },
          { clientX: 100, clientY: 0 },
        ],
      }) as unknown as TouchEvent
    );
    // 单指 move
    pz.__onTouchMove(makeTouchEvent({ touches: [{ clientX: 50, clientY: 50 }] }) as unknown as TouchEvent);
    expect(pz.zoomScale.value).toBe(1.0);
  });
});

describe("usePinchZoom - 双击重置", () => {
  let el: HTMLElement;

  beforeEach(() => {
    el = document.createElement("div");
    document.body.appendChild(el);
  });

  afterEach(() => {
    document.body.removeChild(el);
  });

  it("TestPinch_DoubleTap_Resets: 300ms 内两次单指 tap → 重置", () => {
    vi.useFakeTimers();
    try {
      const pz = usePinchZoom({ initialScale: 1.0 });
      pz.bind(el);
      // 第一次缩放
      pz.zoomIn(); // 1.1
      pz.zoomIn(); // 1.2
      expect(pz.zoomScale.value).toBe(1.2);

      const t0 = 1_000_000;
      vi.setSystemTime(t0);
      // 第一次 tap
      pz.__onTouchStart(makeTouchEvent({ touches: [{ clientX: 50, clientY: 50 }] }) as unknown as TouchEvent);
      expect(pz.zoomScale.value).toBe(1.2); // 第一次 tap 不重置

      // 200ms 后第二次 tap（在 300ms 窗口内）
      vi.setSystemTime(t0 + 200);
      pz.__onTouchStart(makeTouchEvent({ touches: [{ clientX: 50, clientY: 50 }] }) as unknown as TouchEvent);
      expect(pz.zoomScale.value).toBe(1.0); // 重置成功
    } finally {
      vi.useRealTimers();
    }
  });

  it("TestPinch_DoubleTap_TimeoutIgnored: 400ms 后第二次 tap → 忽略（窗口外）", () => {
    vi.useFakeTimers();
    try {
      const pz = usePinchZoom({ initialScale: 1.0, doubleTapMs: 300 });
      pz.bind(el);
      pz.zoomIn(); // 1.1

      const t0 = 1_000_000;
      vi.setSystemTime(t0);
      pz.__onTouchStart(makeTouchEvent({ touches: [{ clientX: 50, clientY: 50 }] }) as unknown as TouchEvent);
      // 400ms 后（超过 300ms 窗口）
      vi.setSystemTime(t0 + 400);
      pz.__onTouchStart(makeTouchEvent({ touches: [{ clientX: 50, clientY: 50 }] }) as unknown as TouchEvent);
      // 不应重置
      expect(pz.zoomScale.value).toBe(1.1);
    } finally {
      vi.useRealTimers();
    }
  });
});
