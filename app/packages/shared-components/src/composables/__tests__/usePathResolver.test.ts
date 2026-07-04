/**
 * usePathResolver 单元测试
 *
 * 🆕 2026-06-15 multi-mount 重构：withSafetyBoundary 已降级为 no-op（spec Phase B5）
 *   - 旧行为：dev 原样返回，release 把 /storage/emulated/0/* 改写到 encv-automation
 *   - 新行为：始终原样返回（只走 normalize，**不**改写）
 *   - 命名空间隔离改由后端 mount 系统承担（/d/automation → appdata）
 *
 * 本测试覆盖新行为：
 *   1. withSafetyBoundary 永远原样返回（除 normalize）
 *   2. 基础 API：normalize / isAbsolutePath / getMockPaths 不变
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePathResolver } from "@encv/shared-components/composables/usePathResolver";

describe("usePathResolver.withSafetyBoundary (no-op mode)", () => {
  beforeEach(() => {
    vi.stubEnv("DEV", true);
  });
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  function withProd(): void {
    vi.stubEnv("DEV", false);
    vi.stubEnv("PROD", true);
  }
  function withDev(): void {
    vi.stubEnv("DEV", true);
    vi.stubEnv("PROD", false);
  }

  // 🆕 2026-06-15：所有调用都走 normalize 后原样返回
  it("dev 模式 + 普通调用：原路径不变（仅 normalize）", () => {
    withDev();
    const { withSafetyBoundary } = usePathResolver();
    expect(withSafetyBoundary("/storage/emulated/0/Download/foo.txt")).toBe("/storage/emulated/0/Download/foo.txt");
  });

  it("dev 模式 + 普通调用：mock 路径不变", () => {
    withDev();
    const { withSafetyBoundary } = usePathResolver();
    expect(withSafetyBoundary("/mock/video.mp4")).toBe("/mock/video.mp4");
  });

  it("真机 + 普通调用：不再改写（原 spec 真机会改写到 encv-automation）", () => {
    withProd();
    const { withSafetyBoundary } = usePathResolver();
    // 🆕 no-op：原路径保留
    expect(withSafetyBoundary("/storage/emulated/0/Download/photo.jpg")).toBe("/storage/emulated/0/Download/photo.jpg");
  });

  it("真机 + 普通调用：mount 路径原样返回", () => {
    withProd();
    const { withSafetyBoundary } = usePathResolver();
    expect(withSafetyBoundary("/d/automation/01-plain-media/video/sample.mp4")).toBe("/d/automation/01-plain-media/video/sample.mp4");
  });

  it("真机 + 普通调用：encv-automation 旧绝对路径保留（migration 期兼容）", () => {
    withProd();
    const { withSafetyBoundary } = usePathResolver();
    expect(withSafetyBoundary("/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4")).toBe(
      "/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4"
    );
  });

  it("真机 + 普通调用：非 storage 路径（/tmp, /data, /mock）不变", () => {
    withProd();
    const { withSafetyBoundary } = usePathResolver();
    expect(withSafetyBoundary("/tmp/test.bin")).toBe("/tmp/test.bin");
    expect(withSafetyBoundary("/data/local/tmp/foo")).toBe("/data/local/tmp/foo");
    expect(withSafetyBoundary("/mock/video.mp4")).toBe("/mock/video.mp4");
  });

  it("真机 + forceAutomation：也 no-op（原 spec 强制改写）", () => {
    withProd();
    const { withSafetyBoundary } = usePathResolver();
    expect(withSafetyBoundary("/storage/emulated/0/Download/important.jpg", { forceAutomation: true })).toBe(
      "/storage/emulated/0/Download/important.jpg"
    );
  });

  it("dev + forceAutomation：也 no-op（原 spec 强制改写）", () => {
    withDev();
    const { withSafetyBoundary } = usePathResolver();
    expect(withSafetyBoundary("/storage/emulated/0/Download/foo", { forceAutomation: true })).toBe("/storage/emulated/0/Download/foo");
  });

  it("空字符串原样返回", () => {
    withProd();
    const { withSafetyBoundary } = usePathResolver();
    expect(withSafetyBoundary("")).toBe("");
    expect(withSafetyBoundary("   ")).toBe("");
  });

  it("路径含反斜杠被规范化（Windows 风格）", () => {
    withProd();
    const { withSafetyBoundary } = usePathResolver();
    // 🆕 no-op：只做 normalize，不再改写
    expect(withSafetyBoundary("\\storage\\emulated\\0\\Download\\foo")).toBe("/storage/emulated/0/Download/foo");
  });
});

describe("usePathResolver 基础 API", () => {
  beforeEach(() => {
    vi.stubEnv("DEV", true);
  });
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("normalize：去前后空格、统一正斜杠、去重", () => {
    const { normalize } = usePathResolver();
    expect(normalize("  /a//b  ")).toBe("/a/b");
    expect(normalize("a/b")).toBe("/a/b");
    expect(normalize("")).toBe("");
  });

  it("isAbsolutePath", () => {
    const { isAbsolutePath } = usePathResolver();
    expect(isAbsolutePath("/abs")).toBe(true);
    expect(isAbsolutePath("rel")).toBe(false);
  });

  it("getMockPaths：dev 模式返 mock 路径数组", () => {
    vi.stubEnv("DEV", true);
    const { getMockPaths } = usePathResolver();
    expect(getMockPaths()).toEqual(["/mock/video.mp4", "/mock/doc.txt", "/mock/report.pdf", "/mock/data.csv"]);
  });

  it("getMockPaths：prod 模式返 null", () => {
    vi.stubEnv("DEV", false);
    const { getMockPaths } = usePathResolver();
    expect(getMockPaths()).toBeNull();
  });
});
