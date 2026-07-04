/**
 * getApiBaseUrl.test.ts
 *
 * 🆕 2026-06-10 沙箱 dev 模式防回归（用户报告"断联+配置全空"）：
 *  验证 getApiBaseUrl() 在 dev 模式下必须读 localStorage 并 fallback 到 DEV_SANDBOX_ENTRY
 *  （127.0.0.1:16666），而不是返回空字符串（空字符串让 fetch 走相对路径，依赖
 *  window.location.origin —— sandbox 浏览器 origin 是 trae 域名被网关拦 → 全 403）。
 *
 * 验证点：
 *  1. 常量契约：DEFAULT_API_BASE_URL = :2025，DEV_SANDBOX_ENTRY = :16666
 *  2. dev 模式（stubEnv DEV=true）：
 *     - localStorage 有值 → 用 localStorage
 *     - localStorage 空 → fallback 到 DEV_SANDBOX_ENTRY（:16666）
 *  3. 非 dev 模式（stubEnv DEV=false）：
 *     - localStorage 有值 → 用 localStorage
 *     - localStorage 空 → fallback 到 DEFAULT_API_BASE_URL（:2025）
 *
 * 测试策略：每个 describe 块先 vi.stubEnv + vi.resetModules + 动态 re-import，
 * 保证 import.meta.env.DEV 真的等于预期。
 */

import { beforeEach, describe, expect, it, vi } from "vitest";

beforeEach(() => {
  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
});

describe("常量契约", () => {
  it("DEFAULT_API_BASE_URL 是 encv-go 直连 :2025（APK 模式用）", async () => {
    const mod = await import("@encv/shared-components/api/encv");
    expect(mod.DEFAULT_API_BASE_URL).toBe("http://127.0.0.1:2025");
  });

  it("DEV_SANDBOX_ENTRY 是 preview-gateway 入口 :16666（沙箱 dev 浏览器用）", async () => {
    const mod = await import("@encv/shared-components/api/encv");
    expect(mod.DEV_SANDBOX_ENTRY).toBe("http://127.0.0.1:16666");
  });
});

describe("getApiBaseUrl — DEV 模式（沙箱 dev 浏览器）", () => {
  beforeEach(async () => {
    vi.resetModules();
    vi.stubEnv("DEV", true);
  });

  it("localStorage 有值 → 用 localStorage（probe commit 后）", async () => {
    const mod = await import("@encv/shared-components/api/encv");
    mod.setApiBaseUrl("http://127.0.0.1:16666");
    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:16666");
  });

  it("localStorage 空 → fallback 到 DEV_SANDBOX_ENTRY (:16666)，不是空字符串", async () => {
    const mod = await import("@encv/shared-components/api/encv");
    // ❌ 旧 dev 行为：返回空字符串 → fetch 走相对路径 → origin 是 trae 域名 → 403
    // ✅ 新 dev 行为：fallback 到 :16666 → fetch 走沙箱内 preview-gateway 入口 → 200
    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:16666");
    expect(mod.getApiBaseUrl()).not.toBe("");
  });

  it("localStorage 写任何 URL → 用 localStorage（不限制必须 :16666）", async () => {
    const mod = await import("@encv/shared-components/api/encv");
    mod.setApiBaseUrl("http://10.0.0.5:9999");
    expect(mod.getApiBaseUrl()).toBe("http://10.0.0.5:9999");
  });
});

describe("getApiBaseUrl — 非 DEV 模式（生产/真机）", () => {
  beforeEach(async () => {
    vi.resetModules();
    vi.stubEnv("DEV", false);
  });

  it("localStorage 有值 → 用 localStorage", async () => {
    const mod = await import("@encv/shared-components/api/encv");
    mod.setApiBaseUrl("http://192.168.1.99:2025");
    expect(mod.getApiBaseUrl()).toBe("http://192.168.1.99:2025");
  });

  it("localStorage 空 → fallback 到 DEFAULT_API_BASE_URL (:2025)", async () => {
    const mod = await import("@encv/shared-components/api/encv");
    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:2025");
  });
});
