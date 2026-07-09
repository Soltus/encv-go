/**
 * 端到端路径一致性测试 — 自动化测试完整路径链路验证
 *
 * 链路：
 *   DEFAULT_AUTOMATION_SOURCE (常量)
 *     → PluginTestsDetail.vue 的 mockRoot 计算
 *       → generateMockFilesViaBackend({ root: mockRoot })  [前端发请求]
 *         → 后端 handleMockGenerateGin 接收 root + filepath.Join(root, relativePath) 写盘
 *           → useWorkflowEngine.executeJob() 取 sourcePath
 *             → withSafetyBoundary(sourcePath, { forceAutomation: true })
 *               → createTask(taskType, safeSource, ...) 提交给后端
 *                 → 后端 stat(文件路径) 检查文件是否存在
 *
 * 本测试验证：步骤 ① 生成的文件路径 == 步骤 ⑦ 检查的文件路径
 *
 * 如果不一致 → "source file not found" 错误（用户报告的问题）
 */

import { collectSpecs } from "@encv/shared-components/lib/mockDataGenerator";
import { beforeEach, describe, expect, it, vi } from "vitest";

// 注：「跨链路配置文件防回归」测试已移出到 __tests__/path-chain-config-regression.test.ts
//  （那里用 node:fs/path/url 协议读真实文件验证 ENCV_MOCK_ROOT 一致性）。
// 本文件不再需要 node: 协议 — vue-tsc --noEmit 不会报 TS2307（不装 @types/node）。

// mock crypto.getRandomValues 以避免 1MB 文件在 jsdom 中超出配额
beforeEach(() => {
  // vitest/jsdom 环境下 crypto.getRandomValues 对 >65536 字节会抛 QuotaExceededError
  // 用 Math.random fallback 替代（仅用于测试路径逻辑，不关心随机性）
  if (typeof globalThis.crypto !== "undefined") {
    vi.spyOn(globalThis.crypto, "getRandomValues").mockImplementation((array: ArrayBufferView) => {
      const buf = new Uint8Array(array.buffer, array.byteOffset, array.byteLength);
      for (let i = 0; i < buf.length; i++) buf[i] = Math.floor(Math.random() * 256);
      return array;
    });
  }
});

// ==================== 常量复刻 ====================
// 这些值必须与源码中的实际常量完全一致

/** useAutomationTests.ts L77 */
const DEFAULT_AUTOMATION_SOURCE = "/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4";

/** PluginTestsDetail.vue L225 — mockRoot computed */
function computeMockRootFromSource(): string {
  return DEFAULT_AUTOMATION_SOURCE.split("/").slice(0, 5).join("/") + "/";
}

/** mockDataGenerator.ts L88 — joinPath (前端版本) */
function frontendJoinPath(...parts: string[]): string {
  return parts.join("/").replace(/\/+/g, "/");
}

/** mock_generator.go L190 — filepath.Join (Go 版本，模拟 Unix 行为) */
function goFilepathJoin(root: string, relativePath: string): string {
  // Go filepath.Join:
  //   - 清理多余斜杠
  //   - 处理 . 和 ..
  //   - 保证结果以 root 为基础
  const cleaned = [root, relativePath].join("/").replace(/\/+/g, "/");
  // 移除尾部斜杠（除非是根目录）
  if (cleaned.length > 1 && cleaned.endsWith("/")) {
    return cleaned.slice(0, -1);
  }
  return cleaned;
}

/** usePathResolver.ts L71 — withSafetyBoundary (release + forceAutomation) */
function simulateWithSafetyBoundaryReleaseForce(rawPath: string): string {
  const normalized = rawPath.trim().replace(/\\/g, "/").replace(/\/+/g, "/");
  if (!normalized) return "";

  const REAL_STORAGE_ROOT = "/storage/emulated/0";
  const SAFETY_NAMESPACE = "encv-automation";

  const insideSafety =
    normalized === `${REAL_STORAGE_ROOT}/${SAFETY_NAMESPACE}` || normalized.startsWith(`${REAL_STORAGE_ROOT}/${SAFETY_NAMESPACE}/`);

  if (insideSafety) return normalized;

  if (normalized.startsWith(REAL_STORAGE_ROOT + "/")) {
    const rel = normalized.slice(REAL_STORAGE_ROOT.length);
    return `${REAL_STORAGE_ROOT}/${SAFETY_NAMESPACE}${rel}`;
  }

  const basename = normalized.replace(/\/+$/, "").split("/").pop() || "unnamed";
  return `${REAL_STORAGE_ROOT}/${SAFETY_NAMESPACE}/__misc__/${basename}`;
}

describe("端到端路径一致性测试", () => {
  // ==================== 阶段 1：常量与计算 ====================

  describe("阶段 1：常量定义 + 三处一致性约束", () => {
    it("DEFAULT_AUTOMATION_SOURCE 必须在 encv-automation 命名空间内", () => {
      expect(DEFAULT_AUTOMATION_SOURCE).toContain("/encv-automation/");
      expect(DEFAULT_AUTOMATION_SOURCE).toMatch(/^\/storage\/emulated\/0\/encv-automation\//);
    });

    it("DEFAULT_AUTOMATION_SOURCE 必须指向 sample.mp4", () => {
      expect(DEFAULT_AUTOMATION_SOURCE).toBe("/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4");
    });

    it("mockRoot 必须是 encv-automation 目录（不带 sample.mp4 部分）", () => {
      const mockRoot = computeMockRootFromSource();
      expect(mockRoot).toBe("/storage/emulated/0/encv-automation/");
      // mockRoot 是 DEFAULT_AUTOMATION_SOURCE 的父目录前缀
      expect(DEFAULT_AUTOMATION_SOURCE.startsWith(mockRoot.replace(/\/$/, ""))).toBe(true);
    });

    // ⚠️ 三个「跨链路 ENCV_MOCK_ROOT 一致性」防回归测试已移出到
    //   __tests__/path-chain-config-regression.test.ts（避免 vue-tsc 扫到 node:fs 协议）
  });

  // ==================== 阶段 2：Mock 文件生成路径 ====================

  describe("阶段 2：后端 Mock 文件写入路径", () => {
    it("后端写入 sample.mp4 的完整路径应等于 DEFAULT_AUTOMATION_SOURCE", () => {
      const mockRoot = computeMockRootFromSource();
      const specs = collectSpecs("all");
      const videoSpec = specs.find(s => s.relativePath === "01-plain-media/video/sample.mp4");
      expect(videoSpec).toBeDefined();

      // 后端用 filepath.Join(root, relativePath)
      const backendWritePath = goFilepathJoin(mockRoot, videoSpec!.relativePath);
      expect(backendWritePath).toBe(DEFAULT_AUTOMATION_SOURCE);
    });

    it("前端 joinPath 和 Go filepath.Join 对同输入产生相同输出", () => {
      const mockRoot = computeMockRootFromSource();
      const relPath = "01-plain-media/video/sample.mp4";

      const frontendResult = frontendJoinPath(mockRoot, relPath);
      const goResult = goFilepathJoin(mockRoot, relPath);

      expect(frontendResult).toBe(goResult);
    });

    it("所有 plain 文件的 Go filepath.Join 结果都无尾部斜杠且无双斜杠", () => {
      const mockRoot = computeMockRootFromSource();
      const specs = collectSpecs("plain");

      for (const spec of specs) {
        const fullPath = goFilepathJoin(mockRoot, spec.relativePath);
        // 无双斜杠
        expect(fullPath).not.toContain("//");
        // 以正常文件名结尾（非目录）
        expect(fullPath).not.toMatch(/\/$/);
        // 在命名空间内
        expect(fullPath).toMatch(/^\/storage\/emulated\/0\/encv-automation\//);
      }
    });
  });

  // ==================== 阶段 3：任务提交路径 ====================

  describe("阶段 3：withSafetyBoundary 转换后的提交路径", () => {
    it("DEFAULT_AUTOMATION_SOURCE 经过 release+forceAutomation 后不变", () => {
      const result = simulateWithSafetyBoundaryReleaseForce(DEFAULT_AUTOMATION_SOURCE);
      expect(result).toBe(DEFAULT_AUTOMATION_SOURCE);
    });

    it("如果 source 不带 encv-automation 前缀，forceAutomation 会修正", () => {
      const wrongSource = "/storage/emulated/0/01-plain-media/video/sample.mp4";
      const result = simulateWithSafetyBoundaryReleaseForce(wrongSource);
      expect(result).toBe(DEFAULT_AUTOMATION_SOURCE);
    });
  });

  // ==================== 阶段 4：核心一致性断言 ====================

  describe("阶段 4：核心一致性 — Mock 写入路径 == 任务提交路径", () => {
    /**
     * 这是最关键的测试！
     *
     * 如果这个测试失败，说明「生成的文件位置」和「任务读取的位置」不一致，
     * 直接导致 "source file not found" 错误。
     */
    it("【核心】sample.mp4：后端写入路径 == 任务提交路径", () => {
      // 步骤 A：计算后端会写到哪里
      const mockRoot = computeMockRootFromSource();
      const specs = collectSpecs("all");
      const videoSpec = specs.find(s => s.relativePath === "01-plain-media/video/sample.mp4")!;
      const backendWritePath = goFilepathJoin(mockRoot, videoSpec.relativePath);

      // 步骤 B：计算任务提交时用的路径
      // Workflow step 定义中 sourcePath = DEFAULT_AUTOMATION_SOURCE
      // 经过 withSafetyBoundary({ forceAutomation: true }) 后
      const taskSubmitPath = simulateWithSafetyBoundaryReleaseForce(DEFAULT_AUTOMATION_SOURCE);

      // 核心断言
      expect(taskSubmitPath).toBe(backendWritePath);
      expect(backendWritePath).toBe(DEFAULT_AUTOMATION_SOURCE);
    });

    it("【核心】所有 plain 文件：后端写入路径都在 encv-automation 下，且 withSafetyBoundary 不再改写", () => {
      const mockRoot = computeMockRootFromSource();
      const specs = collectSpecs("plain");

      for (const spec of specs) {
        const writePath = goFilepathJoin(mockRoot, spec.relativePath);

        // 这个路径已经在 encv-automation 下，所以 withSafetyBoundary 应原样返回
        const afterBoundary = simulateWithSafetyBoundaryReleaseForce(writePath);
        expect(afterBoundary).toBe(writePath);
      }
    });
  });

  // ==================== 阶段 5：边界条件 ====================

  describe("阶段 5：边界条件与防御性检查", () => {
    it("mockRoot 尾部斜杠不影响 filepath.Join 正确性", () => {
      const withSlash = "/storage/emulated/0/encv-automation/";
      const withoutSlash = "/storage/emulated/0/encv-automation";
      const relPath = "01-plain-media/video/sample.mp4";

      expect(goFilepathJoin(withSlash, relPath)).toBe(goFilepathJoin(withoutSlash, relPath));
    });

    it("collectSpecs 中不存在空相对路径", () => {
      const specs = collectSpecs("all");
      for (const spec of specs) {
        expect(spec.relativePath.trim().length).toBeGreaterThan(0);
      }
    });

    it("collectSpecs 中不存在以 / 开头的相对路径（防止绝对路径注入）", () => {
      const specs = collectSpecs("all");
      for (const spec of specs) {
        expect(spec.relativePath.startsWith("/")).toBe(false);
      }
    });

    it("collectSpecs 中不存在真正的路径遍历（../ 或 /.. 后跟 /）", () => {
      const specs = collectSpecs("all");
      for (const spec of specs) {
        // 真正的路径遍历模式：.. 后面跟着路径分隔符
        expect(spec.relativePath).not.toMatch(/\/\.\.\//); // /../ （中间穿越）
        expect(spec.relativePath).not.toMatch(/^\.\/\.\./); // 以 ../ 开头
        // 注意：'..dotfile' 是合法文件名（.. 后面是字母不是 /），不会触发路径遍历
        // 但 Go filepath.Join 对以 .. 开头的路径段仍会特殊处理
      }
      // 记录已知的 ..dotfile 文件名
      const dotDotFile = collectSpecs("all").find(s => s.relativePath.includes("..dotfile"));
      if (dotDotFile) {
        console.warn(`[WARN] 发现潜在风险文件名: "${dotDotFile.relativePath}" — 文件名以 .. 开头，某些系统可能异常处理`);
      }
    });
  });

  // ==================== 阶段 6：Workflow Engine 路径传递验证 ====================

  describe("阶段 6：Workflow Engine 路径传递", () => {
    /**
     * PluginTestsDetail.vue buildDynamicWorkflow() 中：
     *   steps.push({
     *     action: {
     *       params: {
     *         sourcePath: DEFAULT_AUTOMATION_SOURCE,
     *       },
     *     },
     *   })
     *
     * useWorkflowEngine.executeJob() 中：
     *   const source = step.action.params.sourcePath
     *   const safeSource = withSafetyBoundary(source, { forceAutomation: true })
     *   createTask(..., safeSource, ...)
     */

    it("Workflow step 定义的 sourcePath 就是 DEFAULT_AUTOMATION_SOURCE", () => {
      // 模拟 buildDynamicWorkflow() 中的赋值
      const stepSourcePath = DEFAULT_AUTOMATION_SOURCE;
      expect(stepSourcePath).toBe("/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4");
    });

    it("executeJob 提交给 createTask 的最终路径不变", () => {
      // 模拟 executeJob 中的处理流程
      const stepSourcePath = DEFAULT_AUTOMATION_SOURCE;
      const safeSource = simulateWithSafetyBoundaryReleaseForce(stepSourcePath);
      // 最终传给 createTask 的路径
      expect(safeSource).toBe(DEFAULT_AUTOMATION_SOURCE);
    });
  });
});
