/**
 * useWorkflowTaskService 单元测试
 *
 * 覆盖：
 * 1. submitRun 创建 WorkflowRun + 持久化到 localStorage
 * 2. 4 件套事件回调正确更新 StepRun（mock useTaskEventBridge）
 * 3. 终态保护：已终态 StepRun 不被 onTaskUpdate 覆盖
 * 4. 持久化裁剪：超过 50 条时按 startedAt 倒序保留最新 50 条
 * 5. cancelRun 标记 cancelling → cancelled + 调用 cancelTask API
 * 6. listRuns / clearRuns / getRun
 * 7. subscribeRun 订阅运行更新
 * 8. submitRun 拒绝重复运行
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { WorkflowDefinition, UnifiedRunRecord } from '@/lib/workflow/types'

// ==================== Mock 设置 ====================

/** 捕获 useTaskEventBridge 的回调（测试中直接调用） */
const mockBridge = vi.hoisted(() => ({
  options: {} as Record<string, ((data: any) => void) | undefined>,
}))

vi.mock('@/composables/useTaskEventBridge', () => ({
  useTaskEventBridge: (options: any) => {
    mockBridge.options = options
  },
}))

/** mock createTask / cancelTask（用 any 类型避免 Mock 不可调用问题） */
let createTaskMock: (...args: any[]) => any
let cancelTaskMock: (...args: any[]) => any

vi.mock('@/api/encv', () => ({
  createTask: (...args: any[]) => createTaskMock(...args),
  cancelTask: (...args: any[]) => cancelTaskMock(...args),
}))

/** mock setTaskMetadata */
const setTaskMetadataMock = vi.hoisted(() => vi.fn())
vi.mock('@/composables/useTaskTrigger', () => ({
  setTaskMetadata: (...args: any[]) => setTaskMetadataMock(...args),
}))

/** mock analyzeError（返回最小对象） */
vi.mock('@/composables/useErrorAnalyzer', () => ({
  analyzeError: (msg: string) => ({
    category: 'unknown',
    phase: 'backend',
    summary: msg,
    technicalExplanation: '',
    chain: [],
    fixes: [],
  }),
}))

// ==================== 测试夹具 ====================

/** 简单工作流：1 个 job + 1 个 step */
function makeSimpleWorkflow(): WorkflowDefinition {
  return {
    id: 'test-wf',
    name: 'Test Workflow',
    trigger: 'manual',
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
    jobs: [
      {
        id: 'job-1',
        name: 'Job 1',
        steps: [
          {
            id: 'step-1',
            name: 'Step 1',
            action: {
              type: 'encv_task',
              taskType: 'encrypt',
              pluginName: 'test-plugin',
              params: { sourcePath: '/test.mp4' },
            },
          },
        ],
      },
    ],
  }
}

/** 多 step 工作流：1 个 job + 3 个 step（parallel 策略 max=2） */
function makeMultiStepWorkflow(): WorkflowDefinition {
  return {
    id: 'test-wf-multi',
    name: 'Multi Step Workflow',
    trigger: 'manual',
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
    jobs: [
      {
        id: 'job-1',
        name: 'Job 1',
        strategy: { type: 'parallel', max: 2 },
        steps: [
          {
            id: 'step-1',
            name: 'Step 1',
            action: {
              type: 'encv_task',
              taskType: 'encrypt',
              pluginName: 'test-plugin',
              params: { sourcePath: '/test1.mp4' },
            },
          },
          {
            id: 'step-2',
            name: 'Step 2',
            action: {
              type: 'encv_task',
              taskType: 'encrypt',
              pluginName: 'test-plugin',
              params: { sourcePath: '/test2.mp4' },
            },
          },
          {
            id: 'step-3',
            name: 'Step 3',
            action: {
              type: 'encv_task',
              taskType: 'encrypt',
              pluginName: 'test-plugin',
              params: { sourcePath: '/test3.mp4' },
            },
          },
        ],
      },
    ],
  }
}

/** 空工作流（无 jobs） */
function makeEmptyWorkflow(): WorkflowDefinition {
  return {
    id: 'test-empty',
    name: 'Empty',
    trigger: 'manual',
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
    jobs: [],
  }
}

// ==================== beforeEach ====================

beforeEach(() => {
  // 清空 localStorage
  localStorage.clear()
  // 🆕 v4 M5：重置单例（每个测试拿到全新 service 实例，避免 isRunning 状态串扰）
  __resetServiceForTests()
  // 重置 mock 回调
  mockBridge.options = {}
  // 重置 createTask mock（每次返回不同 taskId）
  createTaskMock = vi.fn().mockImplementation(async (_type: string, sourcePath: string) => ({
    id: `task-${Math.random().toString(36).slice(2, 10)}`,
    type: _type,
    sourcePath,
    status: 'queued',
    progress: 0,
  }))
  cancelTaskMock = vi.fn().mockResolvedValue(undefined)
  setTaskMetadataMock.mockClear()
  vi.restoreAllMocks()
  // 重新设置 mock（vi.restoreAllMocks 会清除实现）
  createTaskMock = vi.fn().mockImplementation(async (_type: string, sourcePath: string) => ({
    id: `task-${Math.random().toString(36).slice(2, 10)}`,
    type: _type,
    sourcePath,
    status: 'queued',
    progress: 0,
  }))
  cancelTaskMock = vi.fn().mockResolvedValue(undefined)
})

// ==================== 测试用例 ====================

// 需要 import useWorkflowTaskService（在 mock 之后）
import { useWorkflowTaskService, __resetServiceForTests } from '@/composables/useWorkflowTaskService'

describe('useWorkflowTaskService — submitRun 基本流程', () => {
  it('submitRun 创建 WorkflowRun + 设置 currentRun', async () => {
    const service = useWorkflowTaskService()
    const run = await service.submitRun({ workflow: makeSimpleWorkflow() })
    expect(run.id).toMatch(/^run-/)
    expect(run.status).toBe('running')
    expect(service.currentRun.value).not.toBeNull()
    expect(service.currentRun.value!.id).toBe(run.id)
    expect(service.isRunning.value).toBe(true)
  })

  it('submitRun 持久化到 localStorage（encv_workflow_tasks_v1）', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const raw = localStorage.getItem('encv_workflow_tasks_v1')
    expect(raw).not.toBeNull()
    const records: UnifiedRunRecord[] = JSON.parse(raw!)
    expect(records.length).toBe(1)
    expect(records[0].id).toBe(service.currentRun.value!.id)
    expect(records[0].workflowRun).toBeDefined()
    expect(records[0].workflowRun!.id).toBe(service.currentRun.value!.id)
  })

  it('submitRun 空 workflow（无 jobs）→ 直接标记 success', async () => {
    const service = useWorkflowTaskService()
    const run = await service.submitRun({ workflow: makeEmptyWorkflow() })
    expect(run.status).toBe('success')
    expect(run.completedAt).toBeDefined()
    expect(service.isRunning.value).toBe(false)
  })

  it('submitRun 调用 createTask 为每个 step 提交任务', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeMultiStepWorkflow() })
    expect(createTaskMock).toHaveBeenCalledTimes(3)
  })

  it('submitRun 调用 createTask 时传入 runId 和 triggeredBy（单一数据源）', async () => {
    const service = useWorkflowTaskService()
    const run = await service.submitRun({
      workflow: makeSimpleWorkflow(),
      triggeredBy: 'automation',
    })
    // 🆕 v6 2026-06-22：runId/triggeredBy 直接传给 createTask（不再用 setTaskMetadata）
    // createTask 签名：(..., runId?, triggeredBy?) — 倒数第 2、第 1 参数
    expect(createTaskMock).toHaveBeenCalledTimes(1)
    const callArgs = (createTaskMock as any).mock.calls[0]
    const passedRunId = callArgs[callArgs.length - 2]   // runId
    const passedTriggeredBy = callArgs[callArgs.length - 1]  // triggeredBy
    expect(passedRunId).toBe(run.id)
    expect(passedTriggeredBy).toBe('automation')
  })

  it('submitRun 拒绝重复运行（isRunning 时抛错）', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    // currentRun.status === 'running' → isRunning = true
    await expect(
      service.submitRun({ workflow: makeSimpleWorkflow() }),
    ).rejects.toThrow('A workflow is already running')
  })
})

describe('useWorkflowTaskService — 4 件套事件回调', () => {
  it('onTaskCreated 将 step 从 pending 升级到 queued', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    // submitRun 后 step.status = 'running'（createTask 已 resolve）
    // 手动重置为 pending 测试 onTaskCreated
    const step = service.currentRun.value!.jobs[0].steps[0]
    step.status = 'pending'
    mockBridge.options.onCreate!({ id: step.taskId, type: 'encrypt', sourcePath: '/test.mp4' })
    expect(step.status).toBe('queued')
  })

  it('onTaskUpdate 将 step 从 queued 升级到 running', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    step.status = 'queued'
    mockBridge.options.onUpdate!({
      id: step.taskId!,
      type: 'encrypt',
      status: 'running',
      progress: 10,
    })
    expect(step.status).toBe('running')
    expect(step.progress).toBe(10)
  })

  it('onTaskProgress 更新 progress / phase / speed / eta', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    mockBridge.options.onProgress!({
      id: step.taskId!,
      progress: 50,
      phase: 'encrypting',
      speed: '12.5 MB/s',
      eta: '00:01:30',
    })
    expect(step.progress).toBe(50)
    expect(step.phase).toBe('encrypting')
    expect(step.speed).toBe('12.5 MB/s')
    expect(step.eta).toBe('00:01:30')
  })

  it('onTaskCompleted 无 error → step = success', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    mockBridge.options.onComplete!({ id: step.taskId! })
    expect(step.status).toBe('success')
    expect(step.completedAt).toBeDefined()
    expect(step.durationMs).toBeDefined()
  })

  it('onTaskCompleted 有 error → step = failure + error 字段', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    mockBridge.options.onComplete!({ id: step.taskId!, error: 'password wrong' })
    expect(step.status).toBe('failure')
    expect(step.error).toBe('password wrong')
    expect(step.errorAnalysis).toBeDefined()
  })

  it('onTaskCompleted 触发 workflow 完成检查（单 step → run success）', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    mockBridge.options.onComplete!({ id: step.taskId! })
    expect(service.currentRun.value!.status).toBe('success')
    expect(service.currentRun.value!.completedAt).toBeDefined()
    expect(service.isRunning.value).toBe(false)
  })
})

describe('useWorkflowTaskService — 终态保护 + 状态机校验', () => {
  it('终态保护：success step 不被 onTaskUpdate 覆盖', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    // 先标记为 success
    mockBridge.options.onComplete!({ id: step.taskId! })
    expect(step.status).toBe('success')
    // 尝试用 onTaskUpdate 降级
    mockBridge.options.onUpdate!({
      id: step.taskId!,
      type: 'encrypt',
      status: 'running',
      progress: 50,
    })
    expect(step.status).toBe('success') // 仍然是 success
    expect(step.progress).toBe(50) // progress 仍可刷新
  })

  it('终态保护：success step 不被 onTaskProgress 覆盖 status', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    mockBridge.options.onComplete!({ id: step.taskId! })
    mockBridge.options.onProgress!({
      id: step.taskId!,
      progress: 99,
      phase: 'decrypting',
      speed: '1 MB/s',
      eta: '00:00:01',
    })
    expect(step.status).toBe('success')
    expect(step.progress).toBe(99)
  })

  it('状态机校验：running → queued 非法转换不生效（onTaskCreated）', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    // step 已是 running（createTask resolve 后设置）
    expect(step.status).toBe('running')
    // onTaskCreated 尝试升级到 queued → 非法（running 不能回退到 queued）
    mockBridge.options.onCreate!({ id: step.taskId!, type: 'encrypt', sourcePath: '/test.mp4' })
    expect(step.status).toBe('running') // 不变
  })

  it('状态机校验：onTaskUpdate cancelled 转换生效', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    // step 是 running
    mockBridge.options.onUpdate!({
      id: step.taskId!,
      type: 'encrypt',
      status: 'cancelled',
      progress: 0,
    })
    expect(step.status).toBe('cancelled')
    expect(step.completedAt).toBeDefined()
  })
})

describe('useWorkflowTaskService — 持久化裁剪', () => {
  it('超过 50 条时按 startedAt 倒序保留最新 50 条', async () => {
    // 预填充 55 条历史记录
    const records: UnifiedRunRecord[] = []
    for (let i = 0; i < 55; i++) {
      records.push({
        id: `old-run-${i}`,
        startedAt: new Date(2026, 0, 1, 0, i).toISOString(),
        totalCases: 0,
        passed: 0,
        failed: 0,
        skipped: 0,
        results: [],
      })
    }
    localStorage.setItem('encv_workflow_tasks_v1', JSON.stringify(records))

    const service = useWorkflowTaskService()
    // 加载后应有 55 条
    expect(service.runs.value.length).toBe(55)

    // 提交一个空 workflow 触发持久化（会裁剪到 50）
    await service.submitRun({ workflow: makeEmptyWorkflow() })

    // 裁剪后应有 50 条
    expect(service.runs.value.length).toBe(50)
    // 最旧的 5 条被裁剪
    expect(service.runs.value.find((r) => r.id === 'old-run-0')).toBeUndefined()
    expect(service.runs.value.find((r) => r.id === 'old-run-4')).toBeUndefined()
    // 最新的旧记录保留
    expect(service.runs.value.find((r) => r.id === 'old-run-54')).toBeDefined()
    // 新提交的 run 也在
    expect(service.runs.value.find((r) => r.id === service.currentRun.value!.id)).toBeDefined()
  })

  it('自定义 maxRuns 选项生效', async () => {
    // 提交 5 次空 workflow
    for (let i = 0; i < 5; i++) {
      // 每次需要新实例（因为 isRunning 会阻止重复）
      const s = useWorkflowTaskService({ maxRuns: 3 })
      await s.submitRun({ workflow: makeEmptyWorkflow() })
    }
    // 最后一个实例的 runs 应该 ≤ 3
    const finalService = useWorkflowTaskService({ maxRuns: 3 })
    expect(finalService.runs.value.length).toBeLessThanOrEqual(3)
  })
})

describe('useWorkflowTaskService — cancelRun', () => {
  it('cancelRun 标记 cancelling → cancelled + 调用 cancelTask API', async () => {
    const service = useWorkflowTaskService()
    const run = await service.submitRun({ workflow: makeSimpleWorkflow() })
    const step = service.currentRun.value!.jobs[0].steps[0]
    expect(step.taskId).toBeDefined()

    await service.cancelRun(run.id)

    expect(cancelTaskMock).toHaveBeenCalledWith(step.taskId)
    expect(step.status).toBe('cancelled')
    expect(service.currentRun.value!.status).toBe('cancelled')
    expect(service.isRunning.value).toBe(false)
  })

  it('cancelRun 对非当前 run 不操作', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    await service.cancelRun('nonexistent-run-id')
    expect(cancelTaskMock).not.toHaveBeenCalled()
    expect(service.currentRun.value!.status).toBe('running')
  })

  it('cancelRun 对已完成 run 不操作', async () => {
    const service = useWorkflowTaskService()
    const run = await service.submitRun({ workflow: makeSimpleWorkflow() })
    // 完成 step → run 完成
    const step = service.currentRun.value!.jobs[0].steps[0]
    mockBridge.options.onComplete!({ id: step.taskId! })
    expect(service.currentRun.value!.status).toBe('success')
    // 尝试取消
    await service.cancelRun(run.id)
    expect(cancelTaskMock).not.toHaveBeenCalled()
    expect(service.currentRun.value!.status).toBe('success')
  })
})

describe('useWorkflowTaskService — 查询方法', () => {
  it('listRuns 返回历史运行副本', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    const list = service.listRuns()
    expect(list.length).toBe(1)
    expect(list[0].id).toBe(service.currentRun.value!.id)
    // 修改副本不影响内部状态
    list.length = 0
    expect(service.runs.value.length).toBe(1)
  })

  it('clearRuns 清空历史运行 + localStorage', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    expect(service.runs.value.length).toBe(1)
    service.clearRuns()
    expect(service.runs.value.length).toBe(0)
    const raw = localStorage.getItem('encv_workflow_tasks_v1')
    expect(JSON.parse(raw!)).toEqual([])
  })

  it('getRun 返回当前运行', async () => {
    const service = useWorkflowTaskService()
    const run = await service.submitRun({ workflow: makeSimpleWorkflow() })
    const got = service.getRun(run.id)
    expect(got).not.toBeNull()
    expect(got!.id).toBe(run.id)
  })

  it('getRun 从历史记录返回 workflowRun 快照', async () => {
    // 预填充历史记录
    const historicalRun = {
      id: 'historical-run-1',
      workflowDefId: 'test-wf',
      status: 'success' as const,
      triggeredBy: 'user' as const,
      createdAt: '2026-01-01T00:00:00.000Z',
      startedAt: '2026-01-01T00:00:00.000Z',
      completedAt: '2026-01-01T00:01:00.000Z',
      durationMs: 60000,
      jobs: [],
    }
    const records: UnifiedRunRecord[] = [
      {
        id: 'historical-run-1',
        startedAt: '2026-01-01T00:00:00.000Z',
        completedAt: '2026-01-01T00:01:00.000Z',
        totalCases: 0,
        passed: 0,
        failed: 0,
        skipped: 0,
        results: [],
        workflowRun: historicalRun,
      },
    ]
    localStorage.setItem('encv_workflow_tasks_v1', JSON.stringify(records))

    const service = useWorkflowTaskService()
    const got = service.getRun('historical-run-1')
    expect(got).not.toBeNull()
    expect(got!.id).toBe('historical-run-1')
    expect(got!.status).toBe('success')
  })

  it('getRun 不存在时返回 null', () => {
    const service = useWorkflowTaskService()
    expect(service.getRun('nonexistent')).toBeNull()
  })
})

describe('useWorkflowTaskService — subscribeRun', () => {
  it('subscribeRun 在事件回调时收到更新', async () => {
    const service = useWorkflowTaskService()
    const run = await service.submitRun({ workflow: makeSimpleWorkflow() })
    const updates: string[] = []
    const unsub = service.subscribeRun(run.id, (r) => {
      updates.push(r.status)
    })
    const step = service.currentRun.value!.jobs[0].steps[0]
    // 触发 onTaskProgress → notifySubscribers
    mockBridge.options.onProgress!({
      id: step.taskId!,
      progress: 50,
      phase: 'encrypting',
      speed: '',
      eta: '',
    })
    expect(updates.length).toBeGreaterThan(0)
    expect(updates[0]).toBe('running')
    unsub()
  })

  it('subscribeRun 取消订阅后不再收到更新', async () => {
    const service = useWorkflowTaskService()
    const run = await service.submitRun({ workflow: makeSimpleWorkflow() })
    const updates: string[] = []
    const unsub = service.subscribeRun(run.id, () => {
      updates.push('called')
    })
    unsub()
    const step = service.currentRun.value!.jobs[0].steps[0]
    mockBridge.options.onProgress!({
      id: step.taskId!,
      progress: 50,
      phase: 'encrypting',
      speed: '',
      eta: '',
    })
    expect(updates.length).toBe(0)
  })
})

describe('useWorkflowTaskService — 计算属性', () => {
  it('totalSteps / completedSteps / successSteps / failedSteps 正确计算', async () => {
    const service = useWorkflowTaskService()
    await service.submitRun({ workflow: makeMultiStepWorkflow() })
    // 3 个 step 都已提交（status = running）
    expect(service.totalSteps.value).toBe(3)
    expect(service.completedSteps.value).toBe(0) // 都不是终态
    expect(service.successSteps.value).toBe(0)
    expect(service.failedSteps.value).toBe(0)

    // 完成 2 个 step
    const steps = service.currentRun.value!.jobs[0].steps
    mockBridge.options.onComplete!({ id: steps[0].taskId! })
    mockBridge.options.onComplete!({ id: steps[1].taskId!, error: 'failed' })

    expect(service.completedSteps.value).toBe(2)
    expect(service.successSteps.value).toBe(1)
    expect(service.failedSteps.value).toBe(1)
  })
})

describe('useWorkflowTaskService — 自定义 storageKey', () => {
  it('自定义 storageKey 生效', async () => {
    const customKey = 'encv_custom_workflow_tasks'
    const service = useWorkflowTaskService({ storageKey: customKey })
    await service.submitRun({ workflow: makeSimpleWorkflow() })
    expect(localStorage.getItem(customKey)).not.toBeNull()
    expect(localStorage.getItem('encv_workflow_tasks_v1')).toBeNull()
  })
})
