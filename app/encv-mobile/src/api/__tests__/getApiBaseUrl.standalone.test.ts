/**
 * getApiBaseUrl.standalone.test.ts — **独立 vite dev（无 preview-gateway）** 的 base URL 契约锁
 *
 * 【背景 2026-10-04】
 *   :16666 preview-gateway 是**沙箱（trae / OpenPreview）专用**链路。在 CNB 这类非 trae
 *   环境里根本没有网关，而 dev 态 `getApiBaseUrl()` 的兜底值是
 *   `DEV_SANDBOX_ENTRY = http://127.0.0.1:16666` ⇒ 所有 /api/* 打到不存在的端口 ⇒ 断联。
 *
 *   正解（本文件把它钉死）：`ENCV_STANDALONE_VITE=1 vite` —— vite 在 :8100 上自己把
 *   /api、/agent-api、/ws 反代到 encv-go :2025，前端 base 走**同源**（'' = 相对路径）。
 *
 * 契约：
 *   A. dev + VITE_ENCV_API_BASE='' ⇒ 返回 ''（同源），绝不能是 :16666；
 *   B. dev 且未注入该变量（沙箱）⇒ 保持原行为（localStorage → :16666），不被误伤。
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

beforeEach(() => {
  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
});

describe("getApiBaseUrl — 独立 vite dev（ENCV_STANDALONE_VITE=1，无网关）", () => {
  it("dev + VITE_ENCV_API_BASE='' → 同源（''），不再打沙箱网关 :16666", async () => {
    vi.resetModules();
    vi.stubEnv("DEV", true);
    vi.stubEnv("PROD", false);
    vi.stubEnv("VITE_ENCV_API_BASE", "");

    const mod = await import("@encv/shared-components/api/encv");

    expect(mod.getApiBaseUrl()).toBe("");
    // 这就是非 trae 环境断联的元凶
    expect(mod.getApiBaseUrl()).not.toBe(mod.DEV_SANDBOX_ENTRY);
    expect(mod.getApiBaseUrl()).not.toBe("http://127.0.0.1:16666");
  });

  it("拼接结果必须是相对路径（由 vite 反代兜底），不是绝对地址", async () => {
    vi.resetModules();
    vi.stubEnv("DEV", true);
    vi.stubEnv("VITE_ENCV_API_BASE", "");

    const { getApiBaseUrl } = await import("@encv/shared-components/api/encv");

    expect(`${getApiBaseUrl()}/api/config`).toBe("/api/config");
  });
});

describe("getApiBaseUrl — 沙箱 dev 不被误伤（未注入该变量）", () => {
  it("dev 且无 override → localStorage 优先", async () => {
    vi.resetModules();
    vi.stubEnv("DEV", true);
    vi.stubEnv("PROD", false);
    vi.stubEnv("VITE_ENCV_API_BASE", undefined);

    const mod = await import("@encv/shared-components/api/encv");
    mod.setApiBaseUrl("http://127.0.0.1:16666");

    expect(mod.getApiBaseUrl()).toBe("http://127.0.0.1:16666");
  });

  it("dev + localStorage 空 + 无 override → 仍是 DEV_SANDBOX_ENTRY(:16666)", async () => {
    vi.resetModules();
    vi.stubEnv("DEV", true);
    vi.stubEnv("PROD", false);
    vi.stubEnv("VITE_ENCV_API_BASE", undefined);

    const mod = await import("@encv/shared-components/api/encv");

    expect(mod.getApiBaseUrl()).toBe(mod.DEV_SANDBOX_ENTRY);
  });
});
