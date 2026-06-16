/**
 * useAutomationTests 单元测试
 *
 * 重点覆盖：
 * 1. 动态笛卡尔积正确性（v4 encrypt 包含 cipher × compression）
 * 2. v2/v3 不带 cipher/compression
 * 3. includeDeprecated=false 跳过 v2/v3
 * 4. 真机安全边界 forceAutomation 改写 source
 * 5. recordTriggeredBy 触发者标签登记
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useAutomationTests, DEFAULT_AUTOMATION_SOURCE } from '@/composables/useAutomationTests'
import { _reloadTriggeredByCache, getTriggeredBy } from '@/composables/useTaskTrigger'
import { ECV2, ECV3, ECV4 } from '@/constants/containerVersion'

// mock createTask 和 fetchPlugins
let createTaskMock: any
let fetchPluginsMock: any

vi.mock('@/api/encv', () => {
  return {
    createTask: (...args: any[]) => createTaskMock(...args),
    fetchPlugins: (...args: any[]) => fetchPluginsMock(...args),
  }
})

function makePlugin(name: string, supportedVersions: number[] | null, defaultVersion: number): any {
  return {
    name,
    supportedExtensions: ['.mp4'],
    taskOptions: {
      passwordStrategy: 'global',
      supportVersionSelect: supportedVersions !== null,
      supportedVersions,
      defaultVersion,
      extraFields: [],
    },
  }
}

beforeEach(() => {
  localStorage.clear()
  _reloadTriggeredByCache()  // 🆕 2026-06-10：reset cacheMap 防 test 间状态泄漏
  createTaskMock = vi.fn().mockImplementation(async (_type, sourcePath) => {
    return { id: `task-${Math.random().toString(36).slice(2, 10)}`, sourcePath }
  })
  fetchPluginsMock = vi.fn().mockResolvedValue([])
  // 默认 prod 模式（自动化测试的 forceAutomation 行为）
  Object.defineProperty(import.meta, 'env', { configurable: true, get: () => ({ DEV: false }) })
})

describe('useAutomationTests.loadPlugins', () => {
  it('从后端拉取 plugin 列表', async () => {
    const plugins = [makePlugin('video-v4', [4], 4), makePlugin('audio-v3', [3], 3)]
    fetchPluginsMock.mockResolvedValue(plugins)
    const t = useAutomationTests()
    await t.loadPlugins()
    expect(t.plugins.value).toEqual(plugins)
    expect(t.isLoadingPlugins.value).toBe(false)
  })

  it('拉取失败记录 lastError 不抛', async () => {
    fetchPluginsMock.mockRejectedValue(new Error('network'))
    const t = useAutomationTests()
    await t.loadPlugins()
    expect(t.lastError.value).toBe('network')
    expect(t.isLoadingPlugins.value).toBe(false)
  })
})

describe('useAutomationTests.generateTestCases 笛卡尔积', () => {
  it('单 v4 plugin：encrypt 2×2×2 + decrypt 2 = 10 个用例', () => {
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('video-v4', [4], 4)]
    const cases = t.generateTestCases({ sourceFile: '/foo.mp4' })
    // encrypt: 2 cipher × 2 compression = 4
    // decrypt: cipherMode/compressionMode 跳过 = 1
    // 共 4 + 1 = 5
    expect(cases.length).toBe(5)
    const encrypts = cases.filter((c) => c.taskType === 'encrypt')
    expect(encrypts.length).toBe(4)
    expect(encrypts.some((c) => c.cipherMode === 0 && c.compressionMode === 'none')).toBe(true)
    expect(encrypts.some((c) => c.cipherMode === 0 && c.compressionMode === 'zstd')).toBe(true)
    expect(encrypts.some((c) => c.cipherMode === 1 && c.compressionMode === 'none')).toBe(true)
    expect(encrypts.some((c) => c.cipherMode === 1 && c.compressionMode === 'zstd')).toBe(true)
    const decrypts = cases.filter((c) => c.taskType === 'decrypt')
    expect(decrypts.length).toBe(1)
    expect(decrypts[0].cipherMode).toBeUndefined()
    expect(decrypts[0].compressionMode).toBeUndefined()
  })

  it('单 v3 plugin：encrypt 1 + decrypt 1 = 2 个用例，无 cipher/compression', () => {
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('audio-v3', [3], 3)]
    const cases = t.generateTestCases({ sourceFile: '/foo.mp3' })
    expect(cases.length).toBe(2)
    for (const c of cases) {
      expect(c.version).toBe(3)
      expect(c.cipherMode).toBeUndefined()
      expect(c.compressionMode).toBeUndefined()
      expect(c.expectedBehavior).toBe('might-fail') // ECv3 已废弃
    }
  })

  it('多版本 plugin：[2,3,4]：includeDeprecated=true 全包含', () => {
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('multi', [2, 3, 4], 4)]
    const cases = t.generateTestCases({ sourceFile: '/foo' })
    // ECv2: encrypt 1 + decrypt 1 = 2
    // ECv3: encrypt 1 + decrypt 1 = 2
    // ECv4: encrypt 4 + decrypt 1 = 5
    // 总 9
    expect(cases.length).toBe(9)
    expect(cases.filter((c) => c.version === ECV2).length).toBe(2)
    expect(cases.filter((c) => c.version === ECV3).length).toBe(2)
    expect(cases.filter((c) => c.version === ECV4).length).toBe(5)
  })

  it('多版本 plugin：includeDeprecated=false 跳过 ECv2/ECv3', () => {
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('multi', [2, 3, 4], 4)]
    const cases = t.generateTestCases({ sourceFile: '/foo', includeDeprecated: false })
    // 只剩 ECv4
    expect(cases.length).toBe(5)
    expect(cases.every((c) => c.version === ECV4)).toBe(true)
  })

  it('无 taskOptions 的 plugin 跳过', () => {
    const t = useAutomationTests()
    t.plugins.value = [
      makePlugin('ok', [4], 4),
      { name: 'broken', taskOptions: null },
    ]
    const cases = t.generateTestCases({ sourceFile: '/foo' })
    expect(cases.every((c) => c.pluginName === 'ok')).toBe(true)
    expect(cases.length).toBe(5)
  })

  it('空 plugin 列表返空数组', () => {
    const t = useAutomationTests()
    t.plugins.value = []
    const cases = t.generateTestCases({ sourceFile: '/foo' })
    expect(cases).toEqual([])
  })

  it('用例 id 包含完整选项', () => {
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('mp4', [4], 4)]
    const cases = t.generateTestCases({ sourceFile: '/foo' })
    const example = cases.find((c) => c.cipherMode === 1 && c.compressionMode === 'zstd')
    expect(example?.id).toBe('mp4-encrypt-ECv4-c1-zstd')
  })
})

describe('useAutomationTests.runTests 真实行为', () => {
  it('真机：forceAutomation no-op 后调 createTask（multi-mount 改造）', async () => {
    Object.defineProperty(import.meta, 'env', { configurable: true, get: () => ({ DEV: false }) })
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('p', [4], 4)]
    const cases = t.generateTestCases({ sourceFile: '/d/automation/01-plain-media/video/sample.mp4' })

    await t.runTests(cases)

    // createTask 的第二个参数（sourcePath）应该是 no-op 后的原路径
    expect(createTaskMock).toHaveBeenCalledTimes(cases.length)
    for (const call of createTaskMock.mock.calls) {
      const sourcePath = call[1]
      expect(sourcePath.startsWith('/d/automation/')).toBe(true)
      // multi-mount 改造后 withSafetyBoundary 降级为 no-op
      expect(sourcePath).toBe('/d/automation/01-plain-media/video/sample.mp4')
    }
  })

  it('登记所有任务触发者为 automation', async () => {
    Object.defineProperty(import.meta, 'env', { configurable: true, get: () => ({ DEV: false }) })
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('p', [4], 4)]
    const cases = t.generateTestCases({ sourceFile: '/foo' })

    await t.runTests(cases)

    // 通过公共 API getTriggeredBy() 验证每个 task 都被登记为 automation
    // （v7 简化后 useTaskTrigger 不再导出内部 map getter，改用公共查询 API）
    const triggers: string[] = []
    for (const r of t.results.value) {
      if (r.taskId) triggers.push(getTriggeredBy(r.taskId))
    }
    expect(triggers.length).toBe(cases.length)
    expect(triggers.every((v) => v === 'automation')).toBe(true)
  })

  it('progress 计数正确：提交成功 → pending（等 WS 回调）', async () => {
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('p', [4], 4)]
    const cases = t.generateTestCases({ sourceFile: '/foo' })

    const runPromise = t.runTests(cases)
    expect(t.isRunning.value).toBe(true)
    expect(t.progress.value.total).toBe(cases.length)
    await runPromise
    expect(t.isRunning.value).toBe(false)
    expect(t.progress.value.completed).toBe(cases.length)
    // 提交成功后全部为 pending（WS 回调才会改为 passed/failed）
    expect(t.progress.value.pending).toBe(cases.length)
    expect(t.progress.value.passed).toBe(0)
    expect(t.progress.value.failed).toBe(0)
    // 所有 result 的初始状态应为 pending
    for (const r of t.results.value) {
      expect(r.status).toBe('pending')
      expect(r.taskId).toBeTruthy()
    }
  })

  it('createTask 抛出：单个用例 failed，其余 pending', async () => {
    let callCount = 0
    createTaskMock.mockImplementation(async () => {
      callCount++
      if (callCount === 2) throw new Error('boom')
      return { id: `task-${callCount}` }
    })
    const t = useAutomationTests()
    t.plugins.value = [makePlugin('p', [4], 4)]
    const cases = t.generateTestCases({ sourceFile: '/foo' })

    await t.runTests(cases)

    expect(t.progress.value.completed).toBe(cases.length)
    // 第 2 个用例 createTask 抛异常 → failed
    expect(t.progress.value.failed).toBe(1)
    // 其余提交成功 → pending（不是 passed！）
    expect(t.progress.value.pending).toBe(cases.length - 1)
    expect(t.progress.value.passed).toBe(0)
    // results 中第 2 个用例 status='failed'
    expect(t.results.value[1].status).toBe('failed')
    expect(t.results.value[1].error).toBe('boom')
    // 其余为 pending
    for (let i = 0; i < cases.length; i++) {
      if (i === 1) continue
      expect(t.results.value[i].status).toBe('pending')
    }
  })
})

describe('useAutomationTests 常量', () => {
  it('DEFAULT_AUTOMATION_SOURCE 指向 /d/automation mount（multi-mount 改造）', () => {
    // 旧值：/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4
    // 新值：/d/automation/01-plain-media/video/sample.mp4
    //   → 后端解析为 /data/user/<uid>/com.encvgo.app/files/encv-automation/01-plain-media/video/sample.mp4
    expect(DEFAULT_AUTOMATION_SOURCE).toBe('/d/automation/01-plain-media/video/sample.mp4')
  })
})
