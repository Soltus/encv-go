/**
 * useFormFactor.test.ts — 全局形态档判定（spec desktop-web-android-pairing P1.1）
 *
 * 覆盖：
 *  1. computeFormFactor 纯函数全档位边界（desktop / portrait-pad / portrait-phone）
 *  2. desktop 档三个必要条件缺一不可：非原生 + pointer:fine + ≥1024
 *  3. installFormFactor 的 DOM 行为：写 `data-form-factor`、尊重 simverse 的 `landscape`、幂等
 *
 * ⚠️ fast project 是 isolate:false —— 用 `__resetFormFactorForTests()` 显式重置单例，
 *    **禁止** `vi.resetModules()`（会污染共享模块注册表，实测让 directive-reveal 假红）。
 *    对 window.innerWidth/innerHeight 的 stub 在 afterEach 一并还原，防跨文件泄漏。
 */

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  __resetFormFactorForTests,
  computeFormFactor,
  DESKTOP_MIN_WIDTH,
  installFormFactor,
  useFormFactor,
  PAD_MIN_WIDTH,
} from "@encv/shared-components/composables/useFormFactor";

function f(partial: Partial<Parameters<typeof computeFormFactor>[0]>) {
  return computeFormFactor({
    width: 390,
    height: 844,
    pointerFine: false,
    isNative: false,
    ...partial,
  });
}

describe("computeFormFactor — desktop 档（三条件缺一不可）", () => {
  it("web + pointer:fine + 1440 → desktop", () => {
    expect(f({ width: 1440, height: 900, pointerFine: true })).toBe("desktop");
  });

  it(`宽度恰为 ${DESKTOP_MIN_WIDTH} → desktop（含边界）`, () => {
    expect(f({ width: DESKTOP_MIN_WIDTH, height: 900, pointerFine: true })).toBe("desktop");
  });

  it(`宽度 ${DESKTOP_MIN_WIDTH - 1} 不进 desktop（1300x1366 → portrait-pad）`, () => {
    expect(f({ width: DESKTOP_MIN_WIDTH - 1, height: 1366, pointerFine: true })).toBe("portrait-pad");
  });

  it("原生环境（APK WebView）即使 1440 + 鼠标也不判 desktop", () => {
    expect(f({ width: 1440, height: 900, pointerFine: true, isNative: true })).not.toBe("desktop");
  });

  it("粗指针（触屏）即使 1440 也不判 desktop", () => {
    expect(f({ width: 1440, height: 900, pointerFine: false })).not.toBe("desktop");
  });
});

describe("computeFormFactor — 平台维度（Capacitor 官方只有 web/android/ios）", () => {
  it("platform=electron + 1440 + 鼠标 → desktop（Capawesome Electron 桌面平台；isNative 也是 true）", () => {
    expect(f({ width: 1440, height: 900, pointerFine: true, isNative: true, platform: "electron" })).toBe("desktop");
  });

  it("platform=android 即使 1440 + 鼠标也永不 desktop", () => {
    expect(f({ width: 1440, height: 900, pointerFine: true, isNative: true, platform: "android" })).not.toBe("desktop");
  });

  it("platform=ios 即使 1440 + 鼠标也永不 desktop", () => {
    expect(f({ width: 1440, height: 900, pointerFine: true, isNative: true, platform: "ios" })).not.toBe("desktop");
  });

  it("未给 platform 时回退 isNative（向后兼容旧调用方）", () => {
    expect(f({ width: 1440, height: 900, pointerFine: true, isNative: true })).not.toBe("desktop");
    expect(f({ width: 1440, height: 900, pointerFine: true, isNative: false })).toBe("desktop");
  });
});

describe("computeFormFactor — phone/pad 档", () => {
  it(`≥${PAD_MIN_WIDTH} 且竖屏（高≥宽）→ portrait-pad`, () => {
    expect(f({ width: 900, height: 1400 })).toBe("portrait-pad");
  });

  it(`恰为 ${PAD_MIN_WIDTH} 且竖屏 → portrait-pad（含边界）`, () => {
    expect(f({ width: PAD_MIN_WIDTH, height: PAD_MIN_WIDTH })).toBe("portrait-pad");
  });

  it("宽≥pad 但横屏（高<宽）→ portrait-phone（非 desktop 前提下的兜底）", () => {
    expect(f({ width: 1280, height: 800, pointerFine: false })).toBe("portrait-phone");
  });

  it("390x844 手机 → portrait-phone", () => {
    expect(f({ width: 390, height: 844 })).toBe("portrait-phone");
  });
});

describe("installFormFactor — DOM 行为", () => {
  // stub 前的原值（afterEach 还原，防 isolate:false 跨文件泄漏）
  let origWidth = 0;
  let origHeight = 0;

  function stubViewport(width: number, height: number, pointerFine = true) {
    origWidth = window.innerWidth;
    origHeight = window.innerHeight;
    Object.defineProperty(window, "innerWidth", { configurable: true, value: width });
    Object.defineProperty(window, "innerHeight", { configurable: true, value: height });
    vi.spyOn(window, "matchMedia").mockReturnValue({
      matches: pointerFine,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
    } as unknown as MediaQueryList);
  }

  afterEach(() => {
    vi.restoreAllMocks();
    Object.defineProperty(window, "innerWidth", { configurable: true, value: origWidth });
    Object.defineProperty(window, "innerHeight", { configurable: true, value: origHeight });
    __resetFormFactorForTests();
  });

  it("1440x900 web → 写 data-form-factor=desktop", () => {
    stubViewport(1440, 900);
    installFormFactor();
    expect(document.documentElement.dataset.formFactor).toBe("desktop");
    const { isDesktop } = useFormFactor();
    expect(isDesktop.value).toBe(true);
  });

  it("390x844 → 写 data-form-factor=portrait-phone", () => {
    stubViewport(390, 844, false);
    installFormFactor();
    expect(document.documentElement.dataset.formFactor).toBe("portrait-phone");
  });

  it("已有 landscape（simverse 领地）时不覆盖", () => {
    document.documentElement.dataset.formFactor = "landscape";
    stubViewport(1440, 900);
    installFormFactor();
    expect(document.documentElement.dataset.formFactor).toBe("landscape");
  });

  it("幂等：重复 install 不抛错且结果一致", () => {
    stubViewport(1440, 900);
    installFormFactor();
    installFormFactor();
    expect(document.documentElement.dataset.formFactor).toBe("desktop");
  });
});
