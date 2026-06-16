/**
 * path-chain-config-regression.test.ts
 *
 * ⚠️ 关键回归测试：mock 数据根路径链路一致性
 *
 * 链路（2026-06-15 multi-mount 改造）：
 *   A. 自动化测试 sourcePath 派生 = useAutomationTests.DEFAULT_AUTOMATION_SOURCE
 *      = /d/automation/01-plain-media/video/sample.mp4（虚拟 mount 路径）
 *      → 后端解析：真机 → /data/user/<uid>/com.encvgo.app/files/encv-automation/01-plain-media/video/sample.mp4
 *                  dev  → $TMPDIR/encv-appdata/encv-automation/01-plain-media/video/sample.mp4
 *   B. withSafetyBoundary 降级为 no-op（spec Phase B5）→ 路径不再客户端改写
 *      命名空间隔离改由后端 mount 系统承担
 *
 * 如果任一处漂移 → Mock 写盘路径 ≠ 任务读盘路径 → "source file not found" 错误
 *
 * 2026-06-10 改造：
 *   - 删 ENCV_MOCK_ROOT 相关测试（ecosystem.config.cjs 不再注入该 env，由 mobile overlay 直接决定 servingDir）
 *   - 删 generate-mock-files.ts 相关测试（Node CLI 脚本已废弃）
 *   - 保留 DEFAULT_AUTOMATION_SOURCE 父目录测试（自动化测试 sourcePath 命名空间硬约束）
 *
 * 2026-06-15 改造（multi-mount）：
 *   - 父目录从 `/storage/emulated/0/encv-automation` 改为 `/d/automation`
 *   - 真实物理路径现在由后端 mount registry 解析（不再是前端能看见的字符串）
 *
 * 文件位置说明：此文件在 /workspace/app/encv-mobile/__tests__/（仓库根级），
 * 不在 src/ 里 — 故 tsconfig.json 的 include 范围（src 之下的 ts）不会扫它，
 * vue-tsc --noEmit 不会因 `node:fs` / `node:path` / `node:url` 报 TS2307
 * （frontend tsconfig 不加载 @types/node）。
 *
 * vitest.config.ts 的 include 第 1 条 `__tests__` 下的 test.ts 仍会扫它，
 * 跑测试时 Node 环境原生支持 node:fs/path/url 协议。
 */

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

// __tests__/ → 仓库根
//  - __tests__/ 在 /workspace/app/encv-mobile/__tests__/
//  - 仓库根 = /workspace
//  - __dirname 是 /workspace/app/encv-mobile/__tests__/，上 3 级 = /workspace
const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const REPO_ROOT = resolve(__dirname, '..', '..', '..')

// 🆕 2026-06-15 multi-mount：父目录断言改为 mount 虚拟路径
//  - 旧值：/storage/emulated/0/encv-automation（绝对路径，前端可读）
//  - 新值：/d/automation（虚拟 mount 路径，运行时由后端 mount registry 解析）
const EXPECTED_AUTOMATION_NS = '/d/automation'

describe('path-chain — 配置文件防回归（跨链路一致）', () => {
  it('【防回归】useAutomationTests.DEFAULT_AUTOMATION_SOURCE 父目录必须 = /d/automation（multi-mount）', () => {
    const src = readFileSync(
      resolve(REPO_ROOT, 'app/encv-mobile/src/composables/useAutomationTests.ts'),
      'utf-8',
    )
    // 提取 DEFAULT_AUTOMATION_SOURCE = '...' 字符串
    const m = src.match(/DEFAULT_AUTOMATION_SOURCE\s*=\s*['"]([^'"]+)['"]/)
    expect(m, 'DEFAULT_AUTOMATION_SOURCE must be present in useAutomationTests.ts').toBeTruthy()
    const sourcePath = m![1]
    // 父目录（去掉 01-plain-media/...）必须是 /d/automation mount 命名空间
    expect(sourcePath.startsWith(`${EXPECTED_AUTOMATION_NS}/`)).toBe(true)
  })

  it('【防回归】ecosystem.config.cjs 不再注入 ENCV_MOCK_ROOT（2026-06-10 废弃）', () => {
    const cfg = readFileSync(resolve(REPO_ROOT, 'ecosystem.config.cjs'), 'utf-8')
    // ENCV_MOCK_ROOT 应该从 ecosystem.config.cjs 移除（mobile overlay 直接决定 servingDir）
    expect(cfg).not.toMatch(/ENCV_MOCK_ROOT/)
  })

  it('【防回归】scripts/generate-mock-files.ts 不应再存在（Node CLI 已废弃）', () => {
    const { existsSync } = require('node:fs') as typeof import('node:fs')
    const exists = existsSync(
      resolve(REPO_ROOT, 'app/encv-mobile/scripts/generate-mock-files.ts'),
    )
    expect(exists, 'generate-mock-files.ts should be removed (2026-06-10)').toBe(false)
  })

  // 🆕 2026-06-15 multi-mount：mockRoot 必须是声明式常量 MOCK_GENERATE_ROOT
  //   禁用 .split('/').slice(N) 的隐式推导（fragile：源路径改前缀就静默错）
  //   正确做法：见 src/lib/mockConstants.ts 的 MOCK_GENERATE_ROOT
  it.each([
    'WorkflowDashboard.vue',
    'AutomationTestsDetail.vue',
  ])('【防回归】%s 必须 import MOCK_GENERATE_ROOT 声明式常量（禁 split/slice 推导）', (viewFile) => {
    const src = readFileSync(
      resolve(REPO_ROOT, `app/encv-mobile/src/views/${viewFile}`),
      'utf-8',
    )
    // ① 必须 import 声明式常量
    expect(
      src.includes("from '@/lib/mockConstants'") &&
        (src.includes('MOCK_GENERATE_ROOT') || src.includes('MOCK_MOUNT')),
      `${viewFile} must import MOCK_GENERATE_ROOT from '@/lib/mockConstants'`,
    ).toBe(true)
    // ② 严禁再出现 .split('/').slice(0, 派生 mockRoot 的隐式逻辑
    expect(
      /\.split\(['"`]\/['"`]\)\.slice\(/.test(src),
      `${viewFile} must NOT use .split('/').slice(...) for mockRoot derivation (2026-06-15 禁用)`,
    ).toBe(false)
    // ③ 严禁再 import DEFAULT_AUTOMATION_SOURCE 用于 mockRoot 派生
    const usesDefaultForMockRoot =
      /import\s*\{[^}]*DEFAULT_AUTOMATION_SOURCE[^}]*\}\s*from\s*['"]@\/composables\/useAutomationTests['"]/.test(src)
    expect(
      usesDefaultForMockRoot,
      `${viewFile} must NOT import DEFAULT_AUTOMATION_SOURCE (used to derive mockRoot via slice)`,
    ).toBe(false)
  })

  // 🆕 2026-06-15 mount path 单一真相源验证
  it('【防回归】mountPath() 必须以 /d/ 前缀构造（与后端 mount.go 一致）', async () => {
    const mod = await import('../src/lib/mountPath')
    expect(mod.mountPath('automation')).toBe('/d/automation')
    expect(mod.mountPath('primary')).toBe('/d/primary')
    expect(mod.unmountPath('/d/automation/foo')).toBe('automation')
    expect(mod.unmountPath('/d')).toBe('')
  })

  // 🆕 2026-06-15 mockConstants 一致性验证
  it('【防回归】MOCK_GENERATE_ROOT 必须 = mountPath("automation") + "/"', async () => {
    const m = await import('../src/lib/mockConstants')
    const mp = await import('../src/lib/mountPath')
    expect(m.MOCK_GENERATE_ROOT).toBe(mp.mountPath(m.AUTOMATION_MOUNT_NAME) + '/')
    expect(m.MOCK_GENERATE_ROOT).toBe('/d/automation/')
  })
})
