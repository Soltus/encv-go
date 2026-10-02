/**
 * useFormFactor — 全局形态档（portrait-phone / portrait-pad / desktop）
 *
 * 机制：把结果写到 `document.documentElement.dataset.formFactor`，
 * CSS 侧用 `[data-form-factor="desktop"]` 等选择器消费（与全局令牌/表面类同一套思路）。
 *
 * 判定规则（desktop 档，见 spec desktop-web-android-pairing P1）：
 *   - 必须**非原生**（Capacitor WebView 上不存在"桌面形态"——原生设备永远 phone/pad）
 *   - 指针为精确指针（pointer: fine，即鼠标/触控板）
 *   - 视口宽 ≥ 1024
 *
 * 注意：`landscape` 是 simverse 世界页的保留值（横屏锁定），本 composable 不产出它；
 * 若检测到 DOM 上已有 `landscape`（simverse 自己写入），安装器**不覆盖**，避免打架。
 */

import { computed, ref } from "vue";
import { getAppCapabilities } from "@encv/shared-components/runtime/appCapabilities";
import type { AppPlatform } from "@encv/shared-components/runtime/appCapabilities";

export type FormFactor = "portrait-phone" | "portrait-pad" | "desktop";

/** desktop 档最小宽度（与 tasks.md P1.1.1 对齐） */
export const DESKTOP_MIN_WIDTH = 1024;
/** pad 档最小宽度（沿用既有 phone/pad 约定） */
export const PAD_MIN_WIDTH = 768;

/** 纯函数：可独立单测的判定核心（不做任何 DOM 操作） */
export interface FormFactorInput {
  width: number;
  height: number;
  /** matchMedia("(pointer: fine)").matches */
  pointerFine: boolean;
  /** Capacitor 原生环境（true = APK/WebView） */
  isNative: boolean;
  /**
   * Capacitor 平台名。给了就以它为准；没给则回退 `!isNative`。
   * ⚠️ 桌面端可能是 `web`（浏览器 / cnb 托管）也可能是 `electron`
   * （Capawesome Electron 平台：isNative=true 但形态仍是桌面）——只看 isNative 会误判。
   */
  platform?: AppPlatform;
}

/** 桌面形态候选平台（官方 Capacitor 无桌面平台；electron 由 Capawesome 平台提供） */
function isDesktopCapablePlatform(input: FormFactorInput): boolean {
  if (input.platform) return input.platform === "web" || input.platform === "electron";
  return !input.isNative;
}

export function computeFormFactor(input: FormFactorInput): FormFactor {
  if (isDesktopCapablePlatform(input) && input.pointerFine && input.width >= DESKTOP_MIN_WIDTH) {
    return "desktop";
  }
  if (input.width >= PAD_MIN_WIDTH && input.height >= input.width) {
    return "portrait-pad";
  }
  return "portrait-phone";
}

// ── 模块级单例（全 app 共享同一份，避免多处 resize 监听） ─────────────

const formFactor = ref<FormFactor>("portrait-phone");
let installed = false;
let rafId = 0;

function isNativeEnv(): boolean {
  try {
    return getAppCapabilities().isNative();
  } catch {
    // shared standalone（无注入）→ 按 web 处理
    return false;
  }
}

/** 平台名（注入优先，未注入按 web）。electron 也算桌面形态候选。 */
function platformEnv(): AppPlatform {
  try {
    return getAppCapabilities().platform?.() ?? (isNativeEnv() ? "android" : "web");
  } catch {
    return "web";
  }
}

function pointerFine(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
  return window.matchMedia("(pointer: fine)").matches;
}

function applyFactor(): void {
  if (typeof document === "undefined") return;
  const current = document.documentElement.dataset.formFactor;
  // simverse 世界页写入的横屏档是它自己的领地，不抢
  if (current === "landscape") return;
  const next = computeFormFactor({
    width: window.innerWidth,
    height: window.innerHeight,
    pointerFine: pointerFine(),
    isNative: isNativeEnv(),
    platform: platformEnv(),
  });
  formFactor.value = next;
  document.documentElement.dataset.formFactor = next;
}

function scheduleApply(): void {
  if (typeof window === "undefined") return;
  if (rafId) cancelAnimationFrame(rafId);
  rafId = requestAnimationFrame(() => {
    rafId = 0;
    applyFactor();
  });
}

/**
 * 安装全局形态档（main.ts 启动期调用一次；幂等）。
 * 监听 resize（rAF 节流）+ pointer 精确性变化，并立即计算一次。
 */
export function installFormFactor(): void {
  if (installed || typeof window === "undefined") return;
  installed = true;
  applyFactor();
  window.addEventListener("resize", scheduleApply, { passive: true });
  const mq = window.matchMedia("(pointer: fine)");
  const onChange = () => scheduleApply();
  if (typeof mq.addEventListener === "function") {
    mq.addEventListener("change", onChange);
  } else if (typeof mq.addListener === "function") {
    mq.addListener(onChange); // 旧 API 兜底
  }
}

/** 组件内消费：响应式形态档 + isDesktop 便捷位 */
export function useFormFactor() {
  return {
    formFactor,
    isDesktop: computed(() => formFactor.value === "desktop"),
    /** 手动重算（测试 / 边界场景用） */
    refresh: applyFactor,
  };
}

/**
 * 测试专用：重置安装态 / 单例值 / DOM 标记。
 * ⚠️ 本测试套运行在 vitest isolate:false 的 fast project 里，**禁止**用
 * `vi.resetModules()` 造新实例（会重置共享模块注册表、污染后续测试文件，
 * 2026-10-02 实测导致 directive-reveal 假红）——必须走这个显式重置。
 */
export function __resetFormFactorForTests(): void {
  installed = false;
  formFactor.value = "portrait-phone";
  if (typeof document !== "undefined") {
    delete document.documentElement.dataset.formFactor;
  }
}
