/**
 * ENCV 容器版本常量 + helper
 *
 * 命名沿用 internal/v2/types/container.go 的 ContainerECv3/ContainerECv4
 * 关键事实：
 * - ECV2 已在 SupportedVersions 移除，仅 detector 识别存量文件
 * - ECV3 = deprecated（仍可创建/读取，但强烈建议迁 ECv4）
 * - ECV4 = recommended（默认）
 *
 * 设计原则：
 * - 所有"硬编码 v3/v4 数字"必须从本文件派生
 * - 所有"是否弃用"判断必须用 isDeprecatedVersion() / isRecommendedVersion()
 * - 所有显示文本必须用 formatContainerVersion() 拼出 "ECv3"/"ECv4"
 *
 * 不变量：
 * - isRecommendedVersion(v) = true  ⇔ v === ECV4
 * - isDeprecatedVersion(v)  = true  ⇔ v ∈ {ECV2, ECV3}
 * - 二者都 = false                  ⇔ v 是 SupportedVersions 之外的未知版本
 *
 * 2026-06-11 v2 cleanup：替换所有 `version === 4` / `version <= 3` / `\`v${version}\``
 *   硬编码，统一从本模块派生
 */

export const ECV2 = 2 as const;
export const ECV3 = 3 as const;
export const ECV4 = 4 as const;

export type ContainerVersion = 2 | 3 | 4;

export interface ContainerVersionInfo {
  version: ContainerVersion;
  status: "deprecated" | "recommended";
  label: string;
}

/** 当前支持创建新容器的版本（v3 deprecated + v4 recommended） */
export const CONTAINER_VERSIONS: readonly ContainerVersionInfo[] = [
  { version: ECV3, status: "deprecated", label: "ECv3" },
  { version: ECV4, status: "recommended", label: "ECv4" },
] as const;

export const DEFAULT_CONTAINER_VERSION: ContainerVersion = ECV4;

/**
 * 判断容器版本是否已弃用（v2/v3 都算 deprecated）
 *
 * 注意：v2 已从 SupportedVersions 移除，但语义上仍算 deprecated
 */
export function isDeprecatedVersion(v: number): boolean {
  return v === ECV2 || v === ECV3;
}

/**
 * 判断容器版本是否是当前推荐版本（仅 ECv4）
 */
export function isRecommendedVersion(v: number): boolean {
  return v === ECV4;
}

/**
 * 把数字版本格式化为显示文本，例如：
 *   3 → "ECv3"
 *   4 → "ECv4"
 *   undefined / null → ""
 *   其它数字 → "ECv{数字}"（不报错）
 */
export function formatContainerVersion(v: number | undefined | null): string {
  if (v === undefined || v === null) return "";
  return `ECv${v}`;
}

/**
 * 解析 "ECv3" / "ECv4" 字符串回数字
 * 失败返回 null
 */
export function parseContainerVersion(label: string): ContainerVersion | null {
  const m = /^ECv([2-4])$/.exec(label);
  if (!m || !m[1]) return null;
  const n = Number(m[1]);
  if (n !== ECV2 && n !== ECV3 && n !== ECV4) return null;
  return n as ContainerVersion;
}

/**
 * 给定 plugin 的 supportedVersions 列表，过滤掉已弃用版本（当 includeDeprecated=false）
 */
export function filterRecommendedVersions(versions: readonly number[]): number[] {
  return versions.filter(v => !isDeprecatedVersion(v));
}
