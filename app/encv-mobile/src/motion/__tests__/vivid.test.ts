// 臻彩显示（Vivid / P3 宽色域）真实生效回归测试。
//
// 历史 bug（复现优先：本测试在修复前为 RED）：
//   1. 开启 vivid 从不加 .encv-vivid 根类 → guard.ts / tokens.css 的动效强度 1.3 boost 永不触发；
//   2. 默认强度 100 时 --encv-vivid-filter = contrast(1) saturate(1) = 恒等，肉眼零变化；
//   3. P3 模式写的是 `color-gamut` 属性（非作者可写属性）→ 宽色域从未真正生效。
//
// 修复后：vivid 经 .encv-vivid 类 + --encv-vivid-amount 驱动（gsap 平滑）；P3 经 .encv-p3 类
//   + @media (color-gamut: p3) 把品牌色换成 color(display-p3 ...) 真实宽色域。
import { vi } from "vitest";
import { beforeEach, describe, expect, it } from "vitest";

// 与 scroll-reveal / directive-reveal 同款：hoist 引擎 mock，to() 同步把 --xx 写到元素 style。
const engine = vi.hoisted(() => {
  const applyVars = (target: any, vars?: Record<string, unknown>) => {
    if (target && target.style && vars) {
      for (const [k, v] of Object.entries(vars)) {
        if (k.startsWith("--")) target.style.setProperty(k, String(v));
      }
    }
  };
  return {
    registerPlugins: vi.fn(),
    set: vi.fn(),
    to: vi.fn((target: any, vars?: Record<string, unknown>) => {
      applyVars(target, vars);
      return { kill: vi.fn() };
    }),
    from: vi.fn((_t: unknown, _v?: unknown) => ({ kill: vi.fn() })),
    fromTo: vi.fn((_t: unknown, _f?: unknown, _v?: unknown) => ({ kill: vi.fn() })),
    context: vi.fn(() => ({ revert: vi.fn() })),
    quickTo: vi.fn(() => () => {}),
    delayedCall: vi.fn(() => ({ kill: vi.fn() })),
    createScrollTrigger: vi.fn(() => ({ kill: vi.fn(), progress: 0 })),
    flipGetState: vi.fn(),
    flipFrom: vi.fn(),
  };
});

vi.mock("@encv/shared-components/motion/internal", () => ({ motion: engine, noopMotion: engine }));

import { useTheme } from "@encv/shared-components/composables/useTheme";

describe("臻彩显示（vivid / P3）真实生效", () => {
  beforeEach(() => {
    const root = document.documentElement;
    root.classList.remove("encv-vivid", "encv-p3", "encv-force-p3");
    root.style.removeProperty("--encv-vivid-sat");
    root.style.removeProperty("--encv-vivid-contrast");
    root.style.removeProperty("--encv-vivid-amount");
    root.style.removeProperty("--encv-vivid-filter");
    root.style.removeProperty("--encv-color-gamut");
    root.style.removeProperty("--color-primary-p3");
    localStorage.clear();
    engine.to.mockClear();
  });

  it("开启 vivid 加 .encv-vivid 根类（此前从未添加 → 动效强度 boost 失效）", () => {
    const { setVividMode } = useTheme();
    setVividMode("on");
    expect(document.documentElement.classList.contains("encv-vivid")).toBe(true);
  });

  it("默认浓度/对比度 100 即产生可见滤镜（两变量>0），而非恒等滤镜", () => {
    const { setVividMode, vividSaturation, vividContrast } = useTheme();
    expect(vividSaturation.value).toBe(100); // 默认值
    expect(vividContrast.value).toBe(100);
    setVividMode("on");
    const sat = parseFloat(document.documentElement.style.getPropertyValue("--encv-vivid-sat") || "0");
    const con = parseFloat(document.documentElement.style.getPropertyValue("--encv-vivid-contrast") || "0");
    expect(sat).toBeGreaterThan(0);
    expect(con).toBeGreaterThan(0);
  });

  it("色彩浓度与对比度可独立调节（拆分，互不影响）", () => {
    const { setVividMode, setVividSaturation, setVividContrast, vividSaturation, vividContrast } = useTheme();
    setVividMode("on");
    setVividSaturation(200);
    setVividContrast(50);
    expect(vividSaturation.value).toBe(200);
    expect(vividContrast.value).toBe(50);
    const sat = parseFloat(document.documentElement.style.getPropertyValue("--encv-vivid-sat") || "0");
    const con = parseFloat(document.documentElement.style.getPropertyValue("--encv-vivid-contrast") || "0");
    // satTarget = (200-50)/150 = 1；conTarget = (50-50)/150 = 0
    expect(sat).toBeCloseTo(1, 2);
    expect(con).toBeCloseTo(0, 2);
  });

  it("关闭 vivid：移除根类且两变量归零", () => {
    const { setVividMode } = useTheme();
    setVividMode("on");
    expect(document.documentElement.classList.contains("encv-vivid")).toBe(true);
    setVividMode("off");
    expect(document.documentElement.classList.contains("encv-vivid")).toBe(false);
    const sat = parseFloat(document.documentElement.style.getPropertyValue("--encv-vivid-sat") || "0");
    const con = parseFloat(document.documentElement.style.getPropertyValue("--encv-vivid-contrast") || "0");
    expect(sat).toBe(0);
    expect(con).toBe(0);
  });

  it("开启 P3 加 .encv-p3 根类（此前加的是无效 color-gamut / encv-force-p3）", () => {
    const { setP3Mode } = useTheme();
    setP3Mode("on");
    expect(document.documentElement.classList.contains("encv-p3")).toBe(true);
    expect(document.documentElement.classList.contains("encv-force-p3")).toBe(false);
    expect(document.documentElement.style.getPropertyValue("--encv-color-gamut")).toBe("");
  });

  it("P3 开启且为已知主题色时，写入 display-p3 宽色域令牌", () => {
    const { setP3Mode, setThemeColor } = useTheme();
    setP3Mode("on");
    setThemeColor("#4f8cff"); // Blue preset
    const p3 = document.documentElement.style.getPropertyValue("--color-primary-p3");
    expect(p3).toContain("display-p3");
  });

  it("P3 任意自定义色也自动派生 display-p3 令牌（开关对全色域全自动，不再限内置 7 色）", () => {
    const { setP3Mode, setThemeColor } = useTheme();
    setP3Mode("on");
    setThemeColor("#123456"); // 非内置表色
    const p3 = document.documentElement.style.getPropertyValue("--color-primary-p3");
    expect(p3).toContain("display-p3");
    expect(p3).toBe("color(display-p3 0.0706 0.2039 0.3373)");
  });

  it("P3 非法色不写 -p3 令牌（回退 srgb，不报错）", () => {
    const { setP3Mode, setThemeColor } = useTheme();
    setP3Mode("on");
    setThemeColor("#zzz"); // 非法 hex
    expect(document.documentElement.style.getPropertyValue("--color-primary-p3")).toBe("");
    // 合法色后仍可被正确派生（验证回退不是永久脏状态）
    setThemeColor("#abcdef");
    expect(document.documentElement.style.getPropertyValue("--color-primary-p3")).toContain("display-p3");
  });

  it("initTheme 重新应用 vivid 根类（修复：此前只置 ref 未调 applyVividMode，刷新后 .encv-vivid 丢失 → 滤镜不挂）", () => {
    const { initTheme, setVividMode } = useTheme();
    setVividMode("on");
    expect(document.documentElement.classList.contains("encv-vivid")).toBe(true);
    // 模拟「刷新」：清掉运行时根类，仅保留 localStorage 中的偏好（含浓度/对比度，
    // 否则 module 级 ref 会被其它用例改过的值泄漏、initTheme 回落到非默认）。
    document.documentElement.classList.remove("encv-vivid");
    document.documentElement.style.removeProperty("--encv-vivid-sat");
    document.documentElement.style.removeProperty("--encv-vivid-contrast");
    localStorage.setItem("encv-vivid-mode", "on");
    localStorage.setItem("encv-vivid-mode-saturation", "100");
    localStorage.setItem("encv-vivid-mode-contrast", "100");
    initTheme();
    // 修复后 initTheme 应重新挂上 .encv-vivid 根类 + 滤镜变量
    expect(document.documentElement.classList.contains("encv-vivid")).toBe(true);
    const sat = parseFloat(document.documentElement.style.getPropertyValue("--encv-vivid-sat") || "0");
    const con = parseFloat(document.documentElement.style.getPropertyValue("--encv-vivid-contrast") || "0");
    expect(sat).toBeGreaterThan(0);
    expect(con).toBeGreaterThan(0);
  });
});
