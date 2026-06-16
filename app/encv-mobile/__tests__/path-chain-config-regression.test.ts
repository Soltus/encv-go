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

  // 🆕 2026-06-15 multi-mount：mockRoot 计算必须 .slice(0, 3) = '/d/automation'
  //   旧 .slice(0, 5) = '/d/automation/01-plain-media/video/' → mount 解析失败
  //   触发：23:50 真机 mock generate "invalid mount path" bug
  it.each([
    ['WorkflowDashboard.vue'],
    ['AutomationTestsDetail.vue'],
  ])('【防回归】%s 的 mockRoot 计算必须 .slice(0, 3) = /d/automation（multi-mount）', (viewFile) => {
    const src = readFileSync(
      resolve(REPO_ROOT, `app/encv-mobile/src/views/${viewFile}`),
      'utf-8',
    )
    // 匹配：DEFAULT_AUTOMATION_SOURCE.split('/').slice(0, N).join('/') + '/'
    const m = src.match(/DEFAULT_AUTOMATION_SOURCE\.split\(['"`]\/['"`]\)\.slice\(0,\s*(\d+)\)/)
    expect(m, `mockRoot slice() in ${viewFile}`).toBeTruthy()
    const n = Number(m![1])
    // N 必须是 3（'/d/automation'），不是 5（'/d/automation/01-plain-media/video/'）
    expect(n, `${viewFile} mockRoot slice(0, N) N must be 3 (not 5)`).toBe(3)
  })
})
