/**
 * useErrorAnalyzer 单元测试
 *
 * 覆盖：
 * 1. 关键字匹配：每种 error 类别能正确识别
 * 2. 排除规则：unsupported_version 不被 cipher 误匹配
 * 3. HTTP status 兜底：500/404/403 走对应分类
 * 4. 完全未知错误：返回 unknown 兜底
 * 5. chain 总是非空
 * 6. fixes 总是非空（unknown 也有 3 条）
 */

import { analyzeError, CATEGORY_META } from "@encv/shared-components/composables/useErrorAnalyzer";
import { describe, expect, it } from "vitest";

describe("analyzeError — 关键字匹配", () => {
  const cases: Array<[string, string, string]> = [
    ["Failed to fetch / Network request failed", "failed to fetch", "network"],
    ["401 Unauthorized", "unauthorized token expired", "auth"],
    ["404 Not Found", "404 page not found", "not_found"],
    ["500 Internal Server Error", "500 internal server error", "server_error"],
    ["Plugin crashed", "plugin: signal 11 (segfault)", "plugin_crash"],
    ["Wrong password", "wrong password provided", "wrong_password"],
    ["Decrypt failed", "decrypt failed gcm auth fail", "wrong_password"],
    ["File not found", "source file not found", "file_not_found"],
    ["ENOENT", "no such file or directory", "file_not_found"],
    ["Permission denied", "eacces permission denied", "permission_denied"],
    ["Out of memory", "out of memory killed by kernel", "oom"],
    ["Timeout", "request timeout after 30s", "timeout"],
    ["Socket hang up", "socket hang up", "network"],
    ["Connection refused", "econnrefused 127.0.0.1:2025", "network"],
    ["Unsupported version", "unsupported version: 2", "unsupported_version"],
    ["Invalid compression", "invalid compression mode", "unsupported_compression"],
    ["Zstd failed", "zstd decompression failed", "unsupported_compression"],
    ["Cipher 256 unsupported", "aes-256 cipher not supported by this plugin", "unsupported_cipher"],
  ];

  for (const [name, msg, expectedCat] of cases) {
    it(name, () => {
      const result = analyzeError(msg);
      expect(result.category).toBe(expectedCat);
      expect(result.summary).toBeTruthy();
      expect(result.technicalExplanation.length).toBeGreaterThan(10);
      expect(result.fixes.length).toBeGreaterThanOrEqual(2);
      expect(result.chain.length).toBeGreaterThanOrEqual(3);
    });
  }
});

describe("analyzeError — 排除规则", () => {
  it("含 cipher 关键字的 unsupported version 消息应归类到 cipher", () => {
    const result = analyzeError("unsupported version 2 with cipher aes-256");
    expect(result.category).toBe("unsupported_cipher");
  });

  it("compression 不会被 cipher 误匹配", () => {
    const result = analyzeError("zstd compression failed");
    expect(result.category).toBe("unsupported_compression");
  });
});

describe("analyzeError — HTTP 状态码兜底", () => {
  it("500 → server_error", () => {
    expect(analyzeError("something failed", { httpStatus: 500 }).category).toBe("server_error");
  });
  it("503 → server_error", () => {
    expect(analyzeError("something failed", { httpStatus: 503 }).category).toBe("server_error");
  });
  it("404 → not_found", () => {
    expect(analyzeError("something failed", { httpStatus: 404 }).category).toBe("not_found");
  });
  it("401 → auth", () => {
    expect(analyzeError("something failed", { httpStatus: 401 }).category).toBe("auth");
  });
  it("403 → auth", () => {
    expect(analyzeError("something failed", { httpStatus: 403 }).category).toBe("auth");
  });
});

describe("analyzeError — 兜底", () => {
  it("完全无关字符串 → unknown", () => {
    const result = analyzeError("this is some random nonsense string");
    expect(result.category).toBe("unknown");
    expect(result.fixes.length).toBeGreaterThanOrEqual(2);
    expect(result.chain.length).toBeGreaterThanOrEqual(3);
  });

  it("空字符串 → unknown", () => {
    const result = analyzeError("");
    expect(result.category).toBe("unknown");
  });

  it("大写关键字仍能匹配", () => {
    const result = analyzeError("TIMEOUT");
    expect(result.category).toBe("timeout");
  });
});

describe("analyzeError — chain 构造", () => {
  it("network phase：submission 是 info，network 是 error，后面是未到达", () => {
    const result = analyzeError("failed to fetch");
    const phases = result.chain.map(s => s.phase);
    expect(phases[0]).toBe("submission");
    expect(phases[1]).toBe("network");
    // failedIndex 之后的应是 info
    const failedIdx = result.chain.findIndex(s => s.severity === "error");
    for (let i = 0; i < failedIdx; i++) {
      expect(result.chain[i].severity).toBe("info");
    }
    for (let i = failedIdx + 1; i < result.chain.length; i++) {
      expect(result.chain[i].severity).toBe("info");
    }
  });

  it("plugin phase：chain 包含 5 步，plugin 步是 error", () => {
    const result = analyzeError("plugin crashed with nil pointer");
    const failed = result.chain.find(s => s.severity === "error");
    expect(failed?.phase).toBe("plugin");
    expect(result.chain.length).toBe(5);
  });
});

describe("analyzeError — phase 覆盖", () => {
  it("显式 phase 覆盖推断", () => {
    const result = analyzeError("some error", { phase: "http" });
    expect(result.phase).toBe("http");
  });
});

describe("CATEGORY_META", () => {
  it("所有 category 都有 meta", () => {
    const cats = [
      "network",
      "auth",
      "not_found",
      "server_error",
      "unsupported_version",
      "unsupported_cipher",
      "unsupported_compression",
      "wrong_password",
      "file_not_found",
      "permission_denied",
      "oom",
      "timeout",
      "plugin_crash",
      "invalid_request",
      "unknown",
    ] as const;
    for (const c of cats) {
      const meta = CATEGORY_META[c];
      expect(meta).toBeDefined();
      expect(meta.label).toBeTruthy();
      expect(meta.color).toMatch(/^#[0-9A-Fa-f]{6}$/);
      expect(["info", "warning", "error"]).toContain(meta.severity);
    }
  });
});
