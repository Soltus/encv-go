/**
 * devlogApiError 单元测试
 *
 * 验证：
 * 1. devlogApiError 通过 console.error 输出结构化日志（hijackConsole 会镜像到 DevLogs）
 * 2. devlogApiInfo 通过 console.info 输出（DevLogs 默认显示）
 * 3. 日志包含完整 context（base/source/env/status/body/err）
 * 4. 单行格式（不会让 DevLogs 表格被多行 object 撑爆）
 * 5. context 缺省时优雅降级（status=undefined → 'n/a'，body=undefined → 'n/a'）
 * 6. err 是 Error 对象时取 .message，是 string 时直接用
 * 7. extra 字段正确拼接到日志尾部
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// 同样 mock useAgentApiBase（devlogApiError 内部 import）
let mockIsNative = false;
let mockApiBase = "http://127.0.0.1:2025";

vi.mock("@/plugins/GoProcess", () => ({
  isNative: () => mockIsNative,
}));

vi.mock("@/api/encv", () => ({
  getApiBaseUrl: () => mockApiBase,
  DEFAULT_API_BASE_URL: "http://127.0.0.1:2025",
}));

vi.stubEnv("DEV", false);
vi.stubEnv("PROD", true);

beforeEach(() => {
  mockIsNative = true;
  mockApiBase = "http://127.0.0.1:2025";
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("devlogApiError", () => {
  it("通过 console.error 输出一行格式化字符串（包含 kind/endpoint/base/source/status）", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { devlogApiError } = await import("@/composables/devlogApiError");

    devlogApiError(new Error("boom"), {
      kind: "encrypt",
      endpoint: "/api/encrypt-key",
      status: 500,
      body: "internal error",
      deviceId: "abc123def",
    });

    expect(errorSpy).toHaveBeenCalledTimes(1);
    const msg = errorSpy.mock.calls[0][0] as string;
    expect(msg).toContain("[AGENT-API]");
    expect(msg).toContain("encrypt");
    expect(msg).toContain("/api/encrypt-key");
    expect(msg).toContain("status=500");
    // Phase X1: native 模式下 base 是空字符串（相对路径），由 ApiProxy 接管
    // 实际目标 base URL 通过 baseSource='native-default' + native=true 体现
    expect(msg).toMatch(/base=\s*\(native-default\)/);
    expect(msg).toContain("native=true");
    expect(msg).toContain("boom");
  });

  it("Error 对象时取 .message，string 时直接用", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { devlogApiError } = await import("@/composables/devlogApiError");

    devlogApiError("plain string error", { kind: "decrypt", endpoint: "/api/decrypt-key" });
    expect(errorSpy.mock.calls[0][0]).toContain("plain string error");

    errorSpy.mockClear();
    devlogApiError(new Error("typed error"), { kind: "decrypt", endpoint: "/api/decrypt-key" });
    expect(errorSpy.mock.calls[0][0]).toContain("typed error");
  });

  it('context 缺省时优雅降级（status=undefined → "n/a"）', async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { devlogApiError } = await import("@/composables/devlogApiError");

    devlogApiError(new Error("test"), { kind: "test", endpoint: "/test" });
    const msg = errorSpy.mock.calls[0][0] as string;
    expect(msg).toContain("status=n/a");
    expect(msg).toContain("body=n/a");
    expect(msg).toContain("device=n/a"); // deviceId 缺省
  });

  it("deviceId 在日志中只显示前 8 字符（隐私）", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { devlogApiError } = await import("@/composables/devlogApiError");

    devlogApiError(new Error("test"), {
      kind: "encrypt",
      endpoint: "/api/encrypt-key",
      deviceId: "abcdefghijklmnop",
    });
    const msg = errorSpy.mock.calls[0][0] as string;
    expect(msg).toContain("device=abcdefgh…");
    expect(msg).not.toContain("abcdefghijklmnop");
  });

  it("extra 字段拼接到 extra=... 行", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { devlogApiError } = await import("@/composables/devlogApiError");

    devlogApiError(new Error("mismatch"), {
      kind: "roundtrip",
      endpoint: "/api/decrypt-key",
      extra: { originalLen: 51, decryptedLen: 49 },
    });
    const msg = errorSpy.mock.calls[0][0] as string;
    expect(msg).toContain("extra=");
    expect(msg).toContain('"originalLen":51');
    expect(msg).toContain('"decryptedLen":49');
  });

  it("body > 200 字符时截断（防止 DevLogs 表格被刷爆）", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { devlogApiError } = await import("@/composables/devlogApiError");

    const hugeBody = "X".repeat(5000);
    devlogApiError(new Error("test"), {
      kind: "encrypt",
      endpoint: "/api/encrypt-key",
      status: 500,
      body: hugeBody,
    });
    const msg = errorSpy.mock.calls[0][0] as string;
    // body 段最多 200 字符
    const bodyMatch = msg.match(/body=([^\n]+)/);
    expect(bodyMatch).not.toBeNull();
    expect(bodyMatch![1].length).toBeLessThanOrEqual(200);
    expect(msg).not.toContain("X".repeat(500));
  });

  it("native 标志被 DevLog 反映（isNative=true）", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    mockIsNative = true;
    const { devlogApiError } = await import("@/composables/devlogApiError");

    devlogApiError(new Error("test"), { kind: "test", endpoint: "/test" });
    const msg = errorSpy.mock.calls[0][0] as string;
    expect(msg).toContain("native=true");
  });
});

describe("devlogApiInfo", () => {
  it("通过 console.info 输出", async () => {
    const infoSpy = vi.spyOn(console, "info").mockImplementation(() => {});
    const { devlogApiInfo } = await import("@/composables/devlogApiError");

    devlogApiInfo("roundtrip OK");
    expect(infoSpy).toHaveBeenCalledTimes(1);
    const msg = infoSpy.mock.calls[0][0] as string;
    expect(msg).toContain("[AGENT-API]");
    expect(msg).toContain("roundtrip OK");
  });

  it("带 kind 时输出 [AGENT-API] kind ... 前缀", async () => {
    const infoSpy = vi.spyOn(console, "info").mockImplementation(() => {});
    const { devlogApiInfo } = await import("@/composables/devlogApiError");

    devlogApiInfo("decrypt-key OK", { kind: "decrypt" });
    const msg = infoSpy.mock.calls[0][0] as string;
    expect(msg).toContain("[AGENT-API] decrypt");
    expect(msg).toContain("decrypt-key OK");
  });

  it("带 status 时附加 status=... 段", async () => {
    const infoSpy = vi.spyOn(console, "info").mockImplementation(() => {});
    const { devlogApiInfo } = await import("@/composables/devlogApiError");

    devlogApiInfo("OK", { kind: "test", status: 200 });
    const msg = infoSpy.mock.calls[0][0] as string;
    expect(msg).toContain("status=200");
  });
});
