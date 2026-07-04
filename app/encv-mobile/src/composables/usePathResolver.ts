import type { FileItem } from "@/api/encv";

const MOCK_PATHS = ["/mock/video.mp4", "/mock/doc.txt", "/mock/report.pdf", "/mock/data.csv"] as const;

/**
 * 真机安全边界常量
 *
 * 🆕 2026-06-15 multi-mount 重构：**已删除**。
 *   - 旧常量：REAL_STORAGE_ROOT = '/storage/emulated/0'，SAFETY_NAMESPACE = 'encv-automation'
 *   - 旧用途：release 构建上强制改写 source 路径到 encv-automation 命名空间
 *   - 新机制：路径用 /d/automation 虚拟 mount + 后端解析到 appdata
 *     → 天然隔离用户数据，无需客户端改写
 *   - withSafetyBoundary() 函数保留（降级为 no-op）→ 调用方零改动（migration 期兼容）
 *   - 计划删除 withSafetyBoundary 本身：spec Phase F2
 */

/**
 * 测试环境 / dev 模式下把 isNative/withSafetyBoundary 的真实行为替换为 no-op。
 * - DEV 模式：保持原路径（vite 走 /mock/* 路径或本地磁盘）
 * - 真机 release：拦截 /storage/emulated/0/* 改写到 encv-automation 命名空间
 *
 * `forceAutomation: true` 用于"无论路径在哪里都强制改写"的场景，
 * 适用于自动化测试入口（即使开发者在 dev 设置了 /tmp/real.txt 也改写）。
 */
export interface WithSafetyBoundaryOptions {
  forceAutomation?: boolean;
}

export function usePathResolver() {
  function normalize(rawPath: string): string {
    const trimmed = rawPath.trim();
    if (!trimmed) return "";
    const slashesReplaced = trimmed.replace(/\\/g, "/");
    const deduped = slashesReplaced.replace(/\/+/g, "/");
    if (!deduped.startsWith("/")) {
      return "/" + deduped;
    }
    return deduped;
  }

  function resolveFileItem(file: FileItem): string {
    if (!file?.path) return "";
    return normalize(file.path);
  }

  function isAbsolutePath(path: string): boolean {
    return path.startsWith("/");
  }

  function getMockPaths(): string[] | null {
    if (import.meta.env.DEV) {
      return [...MOCK_PATHS];
    }
    return null;
  }

  /**
   * 真机安全边界（**已降级为 no-op**）
   *
   * 🆕 2026-06-15 multi-mount 重构（spec Phase B5）：
   *   - 旧行为：dev 原样返回，release 把 /storage/emulated/0/* 改写到 encv-automation 命名空间
   *   - 新行为：始终原样返回（只走 normalize，**不**改写）
   *   - 命名空间隔离改由后端 mount 系统承担：
   *     - 测试路径用 /d/automation/... 虚拟 mount → 后端解析到 appdata
   *     - 用户数据用 /d/primary/... 虚拟 mount → 后端解析到 /storage/emulated/0
   *   - 函数签名保留 → 调用方无需修改（migration 期兼容）
   *
   * 计划删除：spec Phase F2（清理旧代码）
   *
   * @deprecated since 2026-06-15 — use mount path /d/<mount>/... directly
   */
  function withSafetyBoundary(rawPath: string, _opts?: WithSafetyBoundaryOptions): string {
    return normalize(rawPath);
  }

  return {
    normalize,
    resolveFileItem,
    isAbsolutePath,
    getMockPaths,
    withSafetyBoundary,
  };
}
