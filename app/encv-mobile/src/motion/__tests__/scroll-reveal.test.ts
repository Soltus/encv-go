import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createApp, defineComponent, h, ref } from "vue";

// 复现优先（.codebuddy/rules/复现优先.mdc）：本测试**先红后绿**锁死
// 「Settings 一级页面整页空白但可点击」的根因——
//   useScrollReveal 把 ion-content 的每个直接子元素（各 <ion-list>）立即置 opacity:0，
//   然后仅靠 gsap ScrollTrigger 的 onEnter 揭示；但 Ionic 的 ion-content 在内部
//   .inner-scroll 滚动，window 默认 scroller 收不到滚动 → onEnter 永不触发 →
//   所有 list 永久停在 opacity:0 = 空白但可点。
// 修复：改用 IntersectionObserver（观察视口相交，与具体滚动容器无关；挂载即在视口内的
//   元素会立即回调揭示），并在无 IO 环境下直接落可见态，保证内容【绝不】永久隐藏。

// Mock 动画引擎（ACL 唯一出口），确定性观察 set/to 调用，隔离 gsap 计时抖动（严禁乐观测试）。
// 用 vi.hoisted 让 engine 与 vi.mock 一同提升，避免「Cannot access 'engine' before initialization」。
const engine = vi.hoisted(() => ({
  registerPlugins: vi.fn(),
  set: vi.fn(),
  to: vi.fn((_t: unknown, _v?: unknown) => ({ kill: vi.fn() })),
  from: vi.fn((_t: unknown, _v?: unknown) => ({ kill: vi.fn() })),
  fromTo: vi.fn((_t: unknown, _f?: unknown, _v?: unknown) => ({ kill: vi.fn() })),
  context: vi.fn(() => ({ revert: vi.fn() })),
  quickTo: vi.fn(() => () => {}),
  delayedCall: vi.fn(() => ({ kill: vi.fn() })),
  createScrollTrigger: vi.fn(() => ({ kill: vi.fn(), progress: 0 })),
  flipGetState: vi.fn(),
  flipFrom: vi.fn(),
}));
vi.mock("@encv/shared-components/motion/internal", () => ({ motion: engine, noopMotion: engine }));

import { useScrollReveal } from "@encv/shared-components/motion/reveal";

interface FakeIOEntry {
  target: Element;
  isIntersecting: boolean;
}
class FakeIntersectionObserver {
  cb: (entries: FakeIOEntry[], obs: FakeIntersectionObserver) => void;
  observed: Element[] = [];
  disconnected = false;
  static instances: FakeIntersectionObserver[] = [];
  constructor(cb: (entries: FakeIOEntry[], obs: FakeIntersectionObserver) => void) {
    this.cb = cb;
    FakeIntersectionObserver.instances.push(this);
  }
  observe(el: Element): void {
    this.observed.push(el);
  }
  disconnect(): void {
    this.disconnected = true;
  }
  unobserve(): void {}
  takeRecords(): FakeIOEntry[] {
    return [];
  }
  /** 测试驱动：模拟元素进入 / 未进入视口。 */
  fire(isIntersecting: boolean): void {
    this.cb(
      this.observed.map(target => ({ target, isIntersecting })),
      this
    );
  }
}

function mountReveal(node: HTMLElement, opts: { stagger?: boolean } = {}): { unmount: () => void } {
  const app = createApp(
    defineComponent({
      setup() {
        const r = ref<HTMLElement | null>(node);
        useScrollReveal(r, opts);
        return () => h("div");
      },
    })
  );
  const host = document.createElement("div");
  document.body.appendChild(host);
  app.mount(host);
  return { unmount: () => app.unmount() };
}

function makeContainer(childCount: number): HTMLElement {
  const container = document.createElement("div");
  for (let i = 0; i < childCount; i++) {
    const c = document.createElement("div");
    c.className = "ui-list";
    container.appendChild(c);
  }
  document.body.appendChild(container);
  return container;
}

describe("useScrollReveal — 复现「Ionic 内滚导致整页空白」并锁死 IntersectionObserver 修复", () => {
  beforeEach(() => {
    for (const k of Object.keys(engine) as (keyof typeof engine)[]) {
      (engine[k] as unknown as { mockClear?: () => void }).mockClear?.();
    }
    FakeIntersectionObserver.instances = [];
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: false, // 非 reduced-motion => 动效开启，会落隐藏初始态
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }));
    vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  it("挂载后用 IntersectionObserver 观察容器（而非 window-scoped ScrollTrigger），相交时才揭示", () => {
    const container = makeContainer(3);
    mountReveal(container, { stagger: true });

    // 立即落隐藏初始态（否则会「滚到才闪一下」）
    expect(engine.set).toHaveBeenCalledTimes(1);
    const setVars = engine.set.mock.calls[0][1] as Record<string, unknown>;
    expect(setVars.opacity).toBe(0);

    // 关键：走 IntersectionObserver，不再用会在 Ionic 内滚下永不触发的 ScrollTrigger
    expect(FakeIntersectionObserver.instances).toHaveLength(1);
    expect(engine.createScrollTrigger).not.toHaveBeenCalled();
    expect(FakeIntersectionObserver.instances[0].observed).toContain(container);

    // 相交前不揭示
    expect(engine.to).not.toHaveBeenCalled();
    // 相交后揭示到 opacity:1
    FakeIntersectionObserver.instances[0].fire(true);
    expect(engine.to).toHaveBeenCalledTimes(1);
    const toVars = engine.to.mock.calls[0][1] as Record<string, unknown>;
    expect(toVars.opacity).toBe(1);
    // once 语义：揭示后断开观察，避免重复
    expect(FakeIntersectionObserver.instances[0].disconnected).toBe(true);
  });

  it("不相交回调不揭示（内容保持隐藏直到真正进入视口，非乐观揭示）", () => {
    const container = makeContainer(2);
    mountReveal(container, { stagger: true });
    FakeIntersectionObserver.instances[0].fire(false);
    expect(engine.to).not.toHaveBeenCalled();
  });

  it("无 IntersectionObserver 环境 => 立即落可见态（内容绝不永久隐藏，兜底不变形）", () => {
    vi.stubGlobal("IntersectionObserver", undefined as unknown as typeof IntersectionObserver);
    const container = makeContainer(2);
    mountReveal(container, { stagger: true });
    // 兜底：直接揭示，不依赖任何 scroll/IO 事件
    expect(engine.to).toHaveBeenCalledTimes(1);
    const toVars = engine.to.mock.calls[0][1] as Record<string, unknown>;
    expect(toVars.opacity).toBe(1);
  });
});
