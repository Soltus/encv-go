import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// 回归锁（2026-10-03，真实浏览器复现后固化）：
//   v-page-transition 曾用 from() 把「当前计算值」当终态 —— 挂在 <ion-page> 上时，
//   Ionic 转场开始前会给页面写内联 opacity:0，from() 采集到 0 ⇒ 动画 0→0，
//   y 正常归位但透明度永久卡 0 ⇒ 整页空白但可点击（Files / AgentChat 在桌面壳下全白）。
//   契约：必须 fromTo 显式终态 opacity:1，且结束后 clearProps 释放内联样式。
//
// ⚠️ FAST 项目是 isolate:false，模块级状态跨文件共享：
//   - 这里必须 **mock** engine 与 guard，**绝不调用真实 setMotionDisabled**（会污染后续文件，
//     实测把 directive-reveal.test.ts 打成假红，且失败在两个文件间来回跳）。
//   - 与 directive-reveal.test.ts 同款：vi.hoisted + vi.mock 完全自给自足。

const state = vi.hoisted(() => ({ enabled: true, intensity: 1 }));
const engine = vi.hoisted(() => ({
  registerPlugins: vi.fn(),
  set: vi.fn(),
  to: vi.fn((_t: unknown, _v?: unknown) => ({ kill: vi.fn() })),
  from: vi.fn((_t: unknown, _v?: unknown) => ({ kill: vi.fn() })),
  fromTo: vi.fn((_t: unknown, _f?: unknown, _v?: unknown) => ({ kill: vi.fn() })),
  context: vi.fn((fn: () => void) => {
    fn();
    return { revert: vi.fn() };
  }),
  quickTo: vi.fn(() => () => {}),
  delayedCall: vi.fn(() => ({ kill: vi.fn() })),
  createScrollTrigger: vi.fn(() => ({ kill: vi.fn(), progress: 0 })),
  flipGetState: vi.fn(),
  flipFrom: vi.fn(),
}));
const guard = vi.hoisted(() => ({
  getMotionProfile: vi.fn(() => ({ enabled: state.enabled, intensity: state.intensity, respectsReduced: false })),
  setMotionDisabled: vi.fn(),
  getMotionDisabled: vi.fn(() => null),
}));
vi.mock("@encv/shared-components/motion/internal", () => ({ motion: engine, noopMotion: engine }));
vi.mock("@encv/shared-components/motion/guard", () => guard);

import { vPageTransition } from "@encv/shared-components/directives/motion";

type MountHook = (el: HTMLElement, binding: unknown) => void;

function mountDirectiveWithIonicPreInlineOpacity(): HTMLElement {
  const el = document.createElement("ion-page");
  el.className = "ion-page";
  // 模拟 Ionic 转场前置状态：页面在指令 mounted 瞬间带内联 opacity:0
  el.style.opacity = "0";
  document.body.appendChild(el);
  // Directive 是「函数指令 | 对象指令」联合类型，这里取对象形态的 mounted 钩子
  const mounted = (vPageTransition as unknown as { mounted?: MountHook }).mounted;
  mounted?.(el, {});
  return el;
}

beforeEach(() => {
  state.enabled = true;
  state.intensity = 1;
  engine.fromTo.mockClear();
  engine.from.mockClear();
  engine.set.mockClear();
});

afterEach(() => {
  document.body.innerHTML = "";
});

describe("v-page-transition 终态契约（防「整页空白但可点击」复发）", () => {
  it("Ionic 前置 opacity:0 时必须用 fromTo 显式终态 opacity:1，不得用 from()", () => {
    mountDirectiveWithIonicPreInlineOpacity();

    expect(engine.fromTo).toHaveBeenCalledTimes(1);
    expect(engine.from).not.toHaveBeenCalled();
    const toVars = (engine.fromTo.mock.calls[0]?.[2] ?? {}) as Record<string, unknown>;
    expect(toVars.opacity).toBe(1);
    expect(String(toVars.clearProps)).toContain("opacity");
  });

  it("结束后 clearProps 释放 opacity/transform（回到 CSS 控制，杜绝内联残留）", () => {
    mountDirectiveWithIonicPreInlineOpacity();

    const toVars = (engine.fromTo.mock.calls[0]?.[2] ?? {}) as Record<string, unknown>;
    expect(String(toVars.clearProps)).toContain("transform");
  });

  it("关动效（reduced-motion）时不写动画，直接落终态", () => {
    state.enabled = false;

    mountDirectiveWithIonicPreInlineOpacity();

    expect(engine.fromTo).not.toHaveBeenCalled();
    expect(engine.set).toHaveBeenCalled();
    const setVars = (engine.set.mock.calls[0]?.[1] ?? {}) as Record<string, unknown>;
    expect(String(setVars.clearProps)).toContain("all");
  });
});
