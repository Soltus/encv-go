/**
 * mockConstants.ts — Mock 数据 + 自动化测试的声明式常量
 *
 * 2026-06-15 重构：禁用一切 .split('/').slice(N) 的隐式推导
 *   - 历史 bug：DEFAULT_AUTOMATION_SOURCE 改前缀 → UI 静默选错子目录 → mount 解析失败
 *   - 现在：mount path 是**声明的常量**，与后端 bootstrap.go 的 "/d/automation" 一一对应
 *   - 改 mount path = 改这一个文件 + 改后端 mount.go 的 NameAutomation 即可，不会漏
 *
 * 三个常量必须保持一致：
 *   - AUTOMATION_MOUNT_NAME  ↔ internal/mount/mount.go 的 NameAutomation
 *   - AUTOMATION_MOUNT_PATH  ↔ internal/mount/bootstrap.go 的 "/d/" + NameAutomation
 *   - AUTOMATION_MOUNT_ROOT  ↔ mount 解析后的实际根（用于 mock 生成器源目录）
 *
 * ⚠️ 防回归：改 mount path 时**必须**同时改这里 + 后端，否则 mock 生成会 403。
 * 详见 __tests__/path-chain-config-regression.test.ts 的「multi-mount 一致性」用例。
 */

import { mountPath } from "@encv/shared-components/lib/mountPath";

/** Mount 名字（与后端 internal/mount/mount.go:NameAutomation 一致） */
export const AUTOMATION_MOUNT_NAME = "automation" as const;

/** Mount 虚拟路径（前端使用的 /d/... 前缀） */
export const AUTOMATION_MOUNT_PATH = mountPath(AUTOMATION_MOUNT_NAME);

/** Mock 数据写入的真实文件系统根（前端用于显示 + 后端用于 Resolve 的目标） */
export const AUTOMATION_MOUNT_ROOT = AUTOMATION_MOUNT_PATH;

/**
 * Mock 数据子目录布局（必须与后端 mock_generator 的 mkdir 路径一致）
 * 历史：曾用 .split('/').slice(0, N) → fragile；现在显式声明每一段
 */
export const MOCK_MEDIA_SUBDIR = "01-plain-media" as const;

/**
 * 给 mock_generator 的 root 参数（**这是 mock API 唯一合法的值**）
 *
 * ❌ 严禁传：/d/automation/01-plain-media/video/  ← 子目录，不是 mount 根
 * ❌ 严禁传：/storage/emulated/0/...             ← 绝对路径，已被 mount 重构拒绝
 * ✅ 必须传：/d/automation/                       ← mount 根（Resolve 必中）
 */
export const MOCK_GENERATE_ROOT = AUTOMATION_MOUNT_PATH + "/";
