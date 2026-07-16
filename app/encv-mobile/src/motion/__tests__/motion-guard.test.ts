import { describe, it, expect, afterEach, vi } from "vitest";
import { createApp, defineComponent, h } from "vue";
import { getMotionProfile, setMotionDisabled, getMotionDisabled } from "@encv/shared-components/motion/guard";
import { EASE, DUR, STAGGER, getStagger } from "@encv/shared-components/motion/tokens";
import { invalidateMotionTokenCache } from "@encv/shared-components/motion/theme-read";
import {
  installMotionDirectives,
  motionDirectives,
  vReveal,
  vPageTransition,
  vRipple,
  vPress,
  vHover,
  vMagnetic,
  vCountUp,
} from "@encv/shared-components/directives/motion";

// 动效契约测试（§2.5 / §2.6）：锁死 guard 统一闸门行为 + 设计令牌导出。
// 这是「换 gsap+daisyui 技术栈、下游零改动」保证的运行时兜底——只要本测试通过，
// 换引擎（改 internal/index.ts 一行）不会改变闸门口径；组件层也无从感知具体动画库。

function stubMatchMedia(reduced: boolean): void {
  vi.stubGlobal(
    "matchMedia",
    (query: string) =>
      ({
        matches: reduced && query.includes("reduce"),
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }) as unknown as MediaQueryList
  );
}

describe("motion guard — ACL 统一闸门", () => {
  afterEach(() => {
    setMotionDisabled(null);
    document.documentElement.classList.remove("encv-vivid", "encv-p3");
    vi.unstubAllGlobals();
  });

  it("prefers-reduced-motion => 关闭动画且 respectsReduced", () => {
    stubMatchMedia(true);
    const p = getMotionProfile();
    expect(p.enabled).toBe(false);
    expect(p.respectsReduced).toBe(true);
  });

  it("普通环境 => 开启，intensity=1", () => {
    stubMatchMedia(false);
    const p = getMotionProfile();
    expect(p.enabled).toBe(true);
    expect(p.intensity).toBe(1);
  });

  it("encv-vivid / encv-p3 根类 => intensity 1.3", () => {
    stubMatchMedia(false);
    document.documentElement.classList.add("encv-vivid");
    expect(getMotionProfile().intensity).toBe(1.3);
    document.documentElement.classList.remove("encv-vivid");
    document.documentElement.classList.add("encv-p3");
    expect(getMotionProfile().intensity).toBe(1.3);
  });

  it("setMotionDisabled 总闸覆盖 reduced-motion 探测", () => {
    stubMatchMedia(true); // 系统偏好 reduced
    setMotionDisabled(true);
    expect(getMotionProfile().enabled).toBe(false);
    setMotionDisabled(false); // 显式开启，覆盖 reduced
    expect(getMotionProfile().enabled).toBe(true);
    expect(getMotionDisabled()).toBe(false);
    setMotionDisabled(null); // 回到跟随系统
    expect(getMotionProfile().enabled).toBe(false); // 系统 reduced => 关
  });

  it("设计令牌导出（语义缓动 + 时长 + stagger）", () => {
    expect(EASE.out).toBe("out");
    expect(EASE.back).toBe("back");
    expect(DUR.base).toBeGreaterThan(0);
    expect(STAGGER).toBeGreaterThan(0);
  });
});

// gsap 赋能主题：GSAP（JS 动画）运行时读取主题的 --motion-* CSS 令牌。
// 这是「主题不只是换色，也能定制动效（时长 / 节奏 / 强度）」的运行时兜底——
// 主题 / 用户片段覆写这些令牌，GSAP 与纯 CSS 动画表现一致，消费方零改动。
describe("motion tokens — gsap 赋能主题（GSAP 读主题 CSS 令牌）", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    invalidateMotionTokenCache();
    setMotionDisabled(null);
    document.documentElement.classList.remove("encv-vivid", "encv-p3");
  });

  function stubTokens(map: Record<string, string>): void {
    vi.stubGlobal("getComputedStyle", () => ({
      getPropertyValue: (p: string) => map[p] ?? "",
    }));
    invalidateMotionTokenCache(); // 丢弃节流缓存，立即读新值（确定性）
  }

  it("DUR 从 --motion-dur-* 令牌实时读取（ms/s 皆可），令牌缺失回退默认", () => {
    stubTokens({ "--motion-dur-fast": "100ms", "--motion-dur-base": "0.5s" });
    expect(DUR.fast).toBeCloseTo(0.1);
    expect(DUR.base).toBeCloseTo(0.5);
    expect(DUR.slow).toBeCloseTo(0.52); // 未覆写 => 回退默认
  });

  it("getStagger 从 --motion-stagger 令牌读取，缺失回退默认", () => {
    stubTokens({ "--motion-stagger": "0.1s" });
    expect(getStagger()).toBeCloseTo(0.1);
    stubTokens({});
    expect(getStagger()).toBeCloseTo(STAGGER);
  });

  it("intensity 从 --motion-intensity 令牌读取（主题可克制 / 张扬）", () => {
    stubMatchMedia(false);
    stubTokens({ "--motion-intensity": "1.5" });
    expect(getMotionProfile().intensity).toBeCloseTo(1.5);
  });

  it("令牌 <=0 但被 setMotionDisabled(false) 强制开启时钳制为正（避免开启却零幅度）", () => {
    stubMatchMedia(true); // 系统 reduced
    setMotionDisabled(false); // 显式强制开启，覆盖 reduced
    stubTokens({ "--motion-intensity": "0" });
    const p = getMotionProfile();
    expect(p.enabled).toBe(true);
    expect(p.intensity).toBeGreaterThan(0);
  });
});

// 指令层（应用层 · 自助接入）契约：锁死 7 个指令的 API 与全局注册。
// 与 guard 测试同责——保证「换 gsap+daisyui 技术栈、下游零改动」：指令内部仍走
// internal/ 引擎 + guard 闸门，注册名稳定，组件层用 v-reveal 等一行接入。
describe("motion directives — 应用层自助接入", () => {
  const all = { vReveal, vPageTransition, vRipple, vPress, vHover, vMagnetic, vCountUp };

  it("7 个指令全部导出且含 mounted 钩子（指令生命周期驱动，不依赖 composable 的 onMounted）", () => {
    type AnyDir = { mounted?: unknown };
    for (const d of Object.values(all)) {
      expect(typeof (d as AnyDir).mounted).toBe("function");
    }
    expect(Object.keys(motionDirectives)).toHaveLength(7);
  });

  it("installMotionDirectives 把 7 个指令注册到 app（全局可用，组件一行接入）", () => {
    const app = createApp(defineComponent({ render: () => h("div") }));
    installMotionDirectives(app);
    expect(app.directive("reveal")).toBe(vReveal);
    expect(app.directive("page-transition")).toBe(vPageTransition);
    expect(app.directive("ripple")).toBe(vRipple);
    expect(app.directive("press")).toBe(vPress);
    expect(app.directive("hover")).toBe(vHover);
    expect(app.directive("magnetic")).toBe(vMagnetic);
    expect(app.directive("count-up")).toBe(vCountUp);
  });
});
