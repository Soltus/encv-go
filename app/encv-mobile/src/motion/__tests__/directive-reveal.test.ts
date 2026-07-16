import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// 复现优先（.codebuddy/rules/复现优先.mdc）：v-reveal 指令此前与 useScrollReveal
// 共用同一条「ScrollTrigger.onEnter 揭示」逻辑，在 Ionic ion-content 内滚下永不触发，
// 同样会造成「整页空白但可点击」。本测试**先红后绿**锁死其 IntersectionObserver 修复：
// 挂载后走 IO（不再用 window-scoped ScrollTrigger），相交才揭示，无 IO 时直接落可见态。

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

import type { ObjectDirective } from "vue";
import { vReveal } from "@encv/shared-components/directives/motion";

// vReveal 是 Directive 联合类型，直接 .mounted 在静态类型上不可达；收窄为 ObjectDirective 以调用钩子。
const reveal = vReveal as ObjectDirective<HTMLElement, { stagger?: boolean } | undefined>;
// mounted 在 ObjectDirective 上可能为 undefined 且签名含 vnode 等额外参数，这里收窄为测试所需的 2 参调用。
const mountFn = reveal.mounted as unknown as (el: HTMLElement, binding: { value?: { stagger?: boolean } | undefined }) => void;
function mountReveal(el: HTMLElement, stagger = true): void {
  mountFn(el, { value: stagger ? { stagger: true } : undefined });
}

interface FakeIOEntry {
  target: Element;
  isIntersecting: boolean;
}
class FakeIntersectionObserver {
  observed: Element[] = [];
  disconnected = false;
  static instances: FakeIntersectionObserver[] = [];
  constructor(public cb: (entries: FakeIOEntry[], obs: FakeIntersectionObserver) => void) {
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
  fire(isIntersecting: boolean): void {
    this.cb(
      this.observed.map(target => ({ target, isIntersecting })),
      this
    );
  }
}

function makeEl(childCount: number): HTMLElement {
  const el = document.createElement("section");
  for (let i = 0; i < childCount; i++) {
    const c = document.createElement("div");
    c.className = "ui-list";
    el.appendChild(c);
  }
  document.body.appendChild(el);
  return el;
}

describe("v-reveal 指令 — 复现 Ionic 内滚空白并锁死 IntersectionObserver 修复", () => {
  beforeEach(() => {
    for (const k of Object.keys(engine) as (keyof typeof engine)[]) {
      (engine[k] as unknown as { mockClear?: () => void }).mockClear?.();
    }
    FakeIntersectionObserver.instances = [];
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: false,
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

  it("mounted 后走 IntersectionObserver 观察元素，相交才揭示", () => {
    const el = makeEl(3);
    mountReveal(el);

    expect(engine.set).toHaveBeenCalledTimes(1);
    const setVars = engine.set.mock.calls[0][1] as Record<string, unknown>;
    expect(setVars.opacity).toBe(0);

    expect(FakeIntersectionObserver.instances).toHaveLength(1);
    expect(engine.createScrollTrigger).not.toHaveBeenCalled();
    expect(FakeIntersectionObserver.instances[0].observed).toContain(el);

    expect(engine.to).not.toHaveBeenCalled();
    FakeIntersectionObserver.instances[0].fire(true);
    expect(engine.to).toHaveBeenCalledTimes(1);
    const toVars = engine.to.mock.calls[0][1] as Record<string, unknown>;
    expect(toVars.opacity).toBe(1);
    expect(FakeIntersectionObserver.instances[0].disconnected).toBe(true);
  });

  it("不相交不揭示", () => {
    const el = makeEl(2);
    mountReveal(el);
    FakeIntersectionObserver.instances[0].fire(false);
    expect(engine.to).not.toHaveBeenCalled();
  });

  it("无 IntersectionObserver 环境 => 直接落可见态", () => {
    vi.stubGlobal("IntersectionObserver", undefined as unknown as typeof IntersectionObserver);
    const el = makeEl(2);
    mountReveal(el);
    expect(engine.to).toHaveBeenCalledTimes(1);
    const toVars = engine.to.mock.calls[0][1] as Record<string, unknown>;
    expect(toVars.opacity).toBe(1);
  });
});
