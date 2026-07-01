/**
 * 🆕 2026-06-23 真实架构实现：批量创建 task（替代 client 预占位野路子）
 *
 * 核心问题（user 反馈）：
 *   "聚合任务数量缓慢增加到1000+本身就是问题你没发现吗？运行自动化插件测试应当一下子就显示所有聚会任务才对！"
 *
 *   历史 bug：submitRun 逐个 await createTask → 1000+ task 慢慢累加到 UI 上
 *
 *   旧方案（client 预占位野路子，已废弃）：
 *     前端生成 client UUID → push placeholder → 传给后端 → 后端用 client ID 覆盖 UUID
 *     问题：破坏后端作为 ID 权威源的原则
 *
 *   新方案（真实架构实现）：
 *     前端收集本层所有 step 的 task 定义 → 一次性调 batchCreateTasks API
 *     → 后端批量创建所有 task（后端生成 UUID 作为唯一权威源）→ 一次性返回所有 task
 *     → 前端拿到后一次性 push 到 store → UI 立即显示 1 个 group N task（不慢慢累加）
 *
 * 关键验证点：
 *   1. batchCreateTasks 只调 1 次（不是 N 次 createTask）
 *   2. 后端返回的 task ID 是后端生成的 UUID（不是 client- 前缀）
 *   3. 所有 task 一次性 push 到 store（不慢慢累加）
 *   4. WS task:created 推送不产生重复 task（patch 而非 append）
 *   5. UI groupedTasksByRunId 立即显示 1 个真 group N task
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { useTaskStore } from '@/stores/taskStore'
import { useTasksList, _resetTasksListSingletonForTests } from '@/composables/useTasksList'
import { useWorkflowStore } from '@/composables/useWorkflowStore'
import { useWorkflowTaskService } from '@/composables/useWorkflowTaskService'
import * as encvApi from '@/api/encv'
import type { EncvTask, PluginMeta, TaskType } from '@/api/encv'
import { TaskListDiagSimulator } from './fixtures/TaskListDiagSimulator'

// ==================== Mock localStorage + crypto ====================
const testStorage = new Map<string, string>()
const mockLocalStorage: Storage = {
  get length() { return testStorage.size },
  key: (index: number) => Array.from(testStorage.keys())[index] ?? null,
  getItem: (key: string) => testStorage.get(key) ?? null,
  setItem: (key: string, value: string) => { testStorage.set(key, value) },
  removeItem: (key: string) => { testStorage.delete(key) },
  clear: () => { testStorage.clear() },
} as unknown as Storage
// Polyfill localStorage（直接覆盖 globalThis + vi.stubGlobal 双保险）
Object.defineProperty(globalThis, 'localStorage', {
  value: mockLocalStorage,
  writable: true,
  configurable: true,
})
vi.stubGlobal('localStorage', mockLocalStorage)

// Mock crypto.randomUUID（vitest 环境 crypto.randomUUID 不可靠）
let uuidCounter = 0
const mockCrypto = {
  randomUUID: () => {
    uuidCounter++
    const id = `00000000-0000-4000-8000-${String(uuidCounter).padStart(12, '0')}`
    return id
  },
}
// 覆盖 globalThis.crypto（原生 crypto 是 getter，Object.defineProperty 才能改）
try {
  Object.defineProperty(globalThis, 'crypto', { value: mockCrypto, writable: true, configurable: true })
} catch {
  ;(globalThis as any).crypto = mockCrypto
}

// ==================== Mock 依赖 ====================
vi.mock('@/composables/useTaskEventBridge', () => ({
  useTaskEventBridge: () => {},
}))
vi.mock('@/lib/taskPersistence', () => ({
  loadAllTasks: vi.fn().mockResolvedValue([]),
  bulkPutTasks: vi.fn().mockResolvedValue(undefined),
  putTask: vi.fn().mockResolvedValue(undefined),
  deleteTask: vi.fn().mockResolvedValue(undefined),
  clearPutThrottle: vi.fn(),
  ensureLRUCache: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/composables/useI18n', () => ({
  useI18n: () => ({ t: (_k: string, opts?: any) => opts?.defaultValue ?? _k }),
}))
vi.mock('@/composables/useDateFormat', () => ({
  formatDateTime: (d: string) => d,
}))

// ==================== Mock batchCreateTasks API（后端生成 UUID，一次性返回所有 task）====================
const batchCallLog: Array<{ specCount: number; runId?: string; triggeredBy?: string }> = []
let batchDelayMs = 50  // 模拟网络延迟
let batchShouldFail = false

// 🆕 2026-06-23 Task 8：cancelRun API mock（批量取消整个 run）
const cancelRunCallLog: string[] = []

vi.mock('@/api/encv', async () => {
  const actual = await vi.importActual<typeof encvApi>('@/api/encv')
  return {
    ...actual,
    batchCreateTasks: vi.fn(async (
      specs: any[],
      runId?: string,
      triggeredBy?: string,
    ): Promise<EncvTask[]> => {
      batchCallLog.push({ specCount: specs.length, runId, triggeredBy })
      await new Promise((r) => setTimeout(r, batchDelayMs))
      if (batchShouldFail) {
        throw new Error('mock-batchCreateTasks-failure')
      }
      // 后端生成 UUID（不是 client- 前缀）——后端是 ID 唯一权威源
      return specs.map((spec: any, i: number) => ({
        id: `server-uuid-${i}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        type: spec.type as TaskType,
        sourcePath: spec.sourcePath,
        status: 'queued' as const,
        progress: 0,
        runId: runId ?? '',
        triggeredBy: (triggeredBy ?? 'user') as 'user' | 'automation' | 'ai_agent',
        createdAt: new Date().toISOString(),
      }))
    }),
    getTasks: vi.fn().mockResolvedValue([]),
    cancelTask: vi.fn().mockResolvedValue(undefined),
    removeTask: vi.fn().mockResolvedValue(undefined),
    // 🆕 Task 8：cancelRun mock — 批量取消整个 run（一次 API）
    cancelRun: vi.fn(async (runId: string): Promise<void> => {
      cancelRunCallLog.push(runId)
    }),
  }
})

// ==================== 真 7 个 plugin（视频/音频/图片/WPS/PDF/文本/加密文件，模拟真机）====================
function buildRealPlugins(): PluginMeta[] {
  const makePlugin = (name: string, exts: string[]): PluginMeta => ({
    name,
    supportedExtensions: exts,
    supportedMimePrefixes: [exts[0] === 'mp4' ? 'video/' : 'application/'],
    containerExtension: '.encv',
    taskOptions: {
      passwordStrategy: 'global',
      supportVersionSelect: true,
      supportedVersions: [4],
      defaultVersion: 4,
      extraFields: [
        { key: 'cipher_mode', label: 'cipherMode', type: 'select', required: false, defaultValue: '0',
          options: ['0', '1'], condition: 'encrypt', help: '' },
        { key: 'compression', label: 'compression', type: 'select', required: false, defaultValue: 'none',
          options: ['none', 'zstd'], condition: 'encrypt', help: '' },
        { key: 'stream_preset', label: 'streamPreset', type: 'select', required: false, defaultValue: 'balanced',
          options: ['balanced', 'quality'], condition: 'encrypt', help: '' },
        { key: 'preserve_metadata', label: 'preserveMetadata', type: 'bool', required: false, defaultValue: 'true', help: '' },
      ],
    },
  })
  return [
    makePlugin('video', ['mp4']),
    makePlugin('audio', ['mp3']),
    makePlugin('image', ['png']),
    makePlugin('wps', ['docx']),
    makePlugin('pdf', ['pdf']),
    makePlugin('text', ['txt']),
    makePlugin('alistencrypt', ['encv']),
  ]
}

// ==================== 1:1 复刻模拟器 mock handlers ====================
const noopHandlers = {
  openGroupDetail: vi.fn(),
  openTaskDetail: vi.fn(),
  openGroupActionSheet: vi.fn(),
  openNewTask: vi.fn(),
  handleRefresh: vi.fn(),
  handleClearCompleted: vi.fn(),
  // 🆕 Task 9.1：group card 取消按钮回调
  cancelRun: vi.fn(),
}

// ==================== 构造测试用 WorkflowDefinition ====================
import { buildDynamicWorkflowPure } from '@/lib/workflow/buildDynamicWorkflow'
import type { WorkflowDefinition } from '@/lib/workflow/types'

function buildTestWorkflowDef(plugins: PluginMeta[]): WorkflowDefinition {
  const { wfDef } = buildDynamicWorkflowPure(plugins, '/mock/')
  return wfDef
}

describe('真机"任务逃逸"e2e — 批量创建 task（真实架构实现）', () => {
  let store: ReturnType<typeof useTaskStore>
  let composable: ReturnType<typeof useTasksList>
  let workflowStore: ReturnType<typeof useWorkflowStore>
  let service: ReturnType<typeof useWorkflowTaskService>
  let wrapper: VueWrapper<any> | null = null

  beforeEach(() => {
    setActivePinia(createPinia())
    store = useTaskStore()
    composable = useTasksList()
    workflowStore = useWorkflowStore()
    service = useWorkflowTaskService()
    _resetTasksListSingletonForTests()
    testStorage.clear()
    batchCallLog.length = 0
    uuidCounter = 0
    batchDelayMs = 50
    batchShouldFail = false
    // 🆕 Task 8：清空 cancelRun 调用日志 + 重置 mock 调用记录
    cancelRunCallLog.length = 0
    vi.mocked(encvApi.cancelRun).mockClear()
    vi.mocked(encvApi.cancelTask).mockClear()
    vi.mocked(encvApi.batchCreateTasks).mockClear()
    for (const key of Object.keys(noopHandlers) as (keyof typeof noopHandlers)[]) {
      noopHandlers[key].mockClear()
    }
  })

  afterEach(async () => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
    _resetTasksListSingletonForTests()
    const { __resetServiceForTests: resetService } = await import('@/composables/useWorkflowTaskService')
    resetService()
  })

  /** 挂载 1:1 复刻真机 UI 模拟器 */
  function mountDiag(): VueWrapper<any> {
    return mount(TaskListDiagSimulator, {
      props: { store, composable, handlers: noopHandlers },
    })
  }

  /**
   * 🆕 2026-06-23：等待 submitRun 的 fire-and-forget IIFE 完成
   *
   * Task 3 将 submitRun 改为 fire-and-forget（IIFE 后台执行 batchCreateTasks），
   * submitRun 立即返回 run 对象。测试需要显式等待 IIFE 完成才能检查 store/batchCallLog。
   *
   * 两阶段等待：
   *   1. 等待第一层所有 batchCreateTasks 调用被发起
   *   2. 等待所有 task 被 push 到 store（batchCreateTasks mock 有 50ms 延迟）
   *
   * 只等阶段 1 不够——mock 的 batchCallLog.push() 在 delay 之前执行，
   * 但 task 在 delay 之后才返回并 appendTask 到 store。
   * 如果不等阶段 2，IIFE 会泄漏到下一个 test，导致跨 test 数据污染。
   *
   * 📝 注意：submitRun 只启动第一层 jobs（DAG 分层），第二层由 scheduleDependentJobs
   *   在前一层完成后驱动。所以默认 batchCallLog.length === 第一层 job 数。
   */
  async function waitForSubmitRunComplete(firstLayerJobCount: number = 1): Promise<void> {
    // 阶段 1：等待第一层所有 batchCreateTasks 调用被发起
    await vi.waitFor(() => {
      expect(batchCallLog.length, `等待 batchCreateTasks 调用 ${firstLayerJobCount} 次（第一层）`).toBe(firstLayerJobCount)
    }, { timeout: 3000, interval: 10 })
    // 阶段 2：等待所有 task 被 push 到 store（batchCreateTasks mock 有 50ms 延迟）
    const totalSpecs = batchCallLog.reduce((sum, c) => sum + c.specCount, 0)
    await vi.waitFor(() => {
      expect(store.tasks.length, `等待 store 有所有 ${totalSpecs} 个 task（IIFE 完成）`).toBe(totalSpecs)
    }, { timeout: 3000, interval: 10 })
  }

  // ==================== T1: batchCreateTasks 只调 1 次，所有 task 一次性 push ====================
  it('T1: submitRun 后 batchCreateTasks 只调 1 次（第一层），store 一次性有 N 个真实 task（不慢慢累加）', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    // 🆕 Task 3 fire-and-forget：等待 IIFE 完成（batchCreateTasks mock 有 50ms 延迟）
    await waitForSubmitRunComplete()
    // submitRun 只启动第一层 jobs（DAG 分层），encrypt-all 是第一层（1 个 job）
    const firstLayerJobCount = 1
    expect(batchCallLog.length, `batchCreateTasks 调用 ${firstLayerJobCount} 次（第一层 jobs，不是全部 jobs）`).toBe(firstLayerJobCount)

    // 2. store 一次性有所有 task（不慢慢累加）
    const totalTasks = store.tasks.length
    const totalSpecs = batchCallLog.reduce((sum, c) => sum + c.specCount, 0)
    // eslint-disable-next-line no-console
    console.log(`[T1] batchCreateTasks 调用 ${batchCallLog.length} 次, store 任务数: ${totalTasks}, total specs: ${totalSpecs}`)
    expect(totalTasks, 'store 一次性有所有 task').toBeGreaterThan(0)
    expect(totalTasks, 'store 任务数 == 所有 batch specs 总数').toBe(totalSpecs)

    // 3. 所有 task 都有后端生成的 UUID（不是 client- 前缀）
    const allServerIds = store.tasks.every((t) => !t.id.startsWith('client-'))
    expect(allServerIds, '所有 task ID 是后端生成的（不是 client- 前缀）').toBe(true)

    // 4. 所有 task 共享同一个 runId
    const runIds = new Set(store.tasks.map((t) => t.runId))
    expect(runIds.size, '所有 task 共享同一个 runId').toBe(1)
  })

  // ==================== T2: 后端是 ID 唯一权威源（task ID 不以 client- 开头）====================
  it('T2: 后端是 ID 唯一权威源——store 里所有 task ID 都是后端生成的 UUID', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    // 🆕 Task 3 fire-and-forget：等待 IIFE 完成
    await waitForSubmitRunComplete()

    // 所有 task ID 都不以 client- 开头（后端生成 UUID）
    for (const t of store.tasks) {
      expect(t.id, `task ID "${t.id}" 不应以 client- 开头`).not.toMatch(/^client-/)
      expect(t.id, `task ID "${t.id}" 应非空`).not.toBe('')
    }

    // 所有 task ID 互不相同
    const ids = store.tasks.map((t) => t.id)
    const uniqueIds = new Set(ids)
    expect(uniqueIds.size, '所有 task ID 互不相同').toBe(ids.length)
  })

  // ==================== T3: WS task:created 推相同 id → 不重复 append ====================
  it('T3: 模拟 WS 推 task:created（用后端 ID）→ 不重复 append（patch 而非 append）', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    // 🆕 Task 3 fire-and-forget：等待 IIFE 完成
    await waitForSubmitRunComplete()

    const tasksBefore = store.tasks.length

    // 模拟后端 WS 推 task:created（用后端生成的 ID）
    for (const t of [...store.tasks]) {
      store.applyEvent('created', { ...t, status: 'running' })
    }
    expect(store.tasks.length, 'WS 重推 task:created 不应该 append 重复（patch 而非 append）').toBe(tasksBefore)
  })

  // ==================== T4: UI 1:1 复刻模拟器 — 立即显示 1 个真 group N task ====================
  it('T4: submitRun 后，1:1 复刻 UI 立即显示 1 个真 group N task（不再"慢慢累加"）', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    // 🆕 Task 3 fire-and-forget：等待 IIFE 完成
    await waitForSubmitRunComplete()
    wrapper = mountDiag()
    await nextTick()

    const realGroupCount = Number(wrapper.find('[data-testid="real-group-count"]').text())
    const fakeGroupCount = Number(wrapper.find('[data-testid="fake-group-count"]').text())
    const escapeTaskCount = Number(wrapper.find('[data-testid="escape-task-count"]').text())
    const totalTaskCount = Number(wrapper.find('[data-testid="store-tasks-count"]').text())
    // eslint-disable-next-line no-console
    console.log(`[T4] submitRun 后: totalTask=${totalTaskCount} / 真 group=${realGroupCount} / 伪 group=${fakeGroupCount} / 逃逸=${escapeTaskCount}`)
    expect(totalTaskCount, 'store 一次性有所有 task').toBeGreaterThan(0)
    expect(realGroupCount, '1 个真 group（所有 task 共享 runId）').toBe(1)
    expect(fakeGroupCount, '0 伪 group（所有 task 都有 runId）').toBe(0)
    expect(escapeTaskCount, '0 逃逸').toBe(0)
  })

  // ==================== 阶段一 WS 时序测试（T5-T6）====================

  // ==================== T5: WS task:created 广播带 runId（后端时序修复验证）====================
  it('T5: WS task:created 广播带 runId——store 中所有 task runId 非空 + appendTask 不触发 warn', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    // 捕获 console.warn（appendTask 在 runId 为空时会 warn）
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    await waitForSubmitRunComplete()

    // 1. 所有 task 的 runId 非空（后端 CreateWithRunMeta 广播时序修复后 runId 必带）
    const emptyRunIdTasks = store.tasks.filter((t) => !t.runId)
    expect(emptyRunIdTasks.length, `所有 task runId 非空（实际空 runId task 数: ${emptyRunIdTasks.length}）`).toBe(0)

    // 2. taskStore.appendTask 的 warn 日志不出现（runId 为空的 warn）
    //    taskStore.appendTask 在 runId 为空时打印 '[taskStore.appendTask] 新 task runId 为空'
    const appendTaskWarns = warnSpy.mock.calls.filter(
      (args) => typeof args[0] === 'string' && args[0].includes('[taskStore.appendTask]'),
    )
    expect(appendTaskWarns.length, 'appendTask 不应触发 runId 为空的 warn').toBe(0)

    warnSpy.mockRestore()
  })

  // ==================== T6: WS task:created 不产生孤儿 group ====================
  it('T6: WS task:created 不产生孤儿 group——groupedTasksByRunId 只有 1 个真 group + 0 伪 group', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    await waitForSubmitRunComplete()

    // 1. groupedTasksByRunId 只有 1 个 group（所有 task 共享同一个 runId）
    const groups = store.groupedTasksByRunId
    expect(groups.length, `只有 1 个 group（实际: ${groups.length}）`).toBe(1)

    // 2. 没有 __manual__ 前缀的 group（孤儿 group）
    const orphanGroups = groups.filter((g) => g.runId.startsWith('__manual__'))
    expect(orphanGroups.length, '0 个孤儿 group（__manual__ 前缀）').toBe(0)

    // 3. 唯一的 group 是真 group（runId 非 __manual__ 前缀）
    expect(groups[0].runId, 'group runId 不以 __manual__ 开头').not.toMatch(/^__manual__/)

    // 4. 所有 task 都在该 group 内（无散落 task）
    expect(groups[0].tasks.length, '所有 task 都在唯一 group 内').toBe(store.tasks.length)
  })

  // ==================== 阶段二 非阻塞 submitRun + 批量取消测试（T7-T8）====================

  // ==================== T7: submitRun 非阻塞（fire-and-forget）====================
  it('T7: submitRun 非阻塞——< 100ms 返回 run 对象，batchCreateTasks 后台异步完成', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    // 把 batchCreateTasks 延迟调大到 200ms，确保 submitRun 不等它完成
    batchDelayMs = 200

    const t0 = Date.now()
    const run = await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    const elapsed = Date.now() - t0

    // 1. submitRun 返回时间 < 100ms（fire-and-forget，不等 batchCreateTasks 完成）
    expect(elapsed, `submitRun 应 < 100ms 返回（实际: ${elapsed}ms）`).toBeLessThan(100)

    // 2. run 对象已创建（run.id 非空）
    expect(run.id, 'run.id 非空').toBeTruthy()
    expect(run.status, 'run.status === running（已启动）').toBe('running')

    // 3. service.currentRun 已设置
    expect(service.currentRun.value, 'service.currentRun 已设置').not.toBeNull()
    expect(service.currentRun.value!.id, 'currentRun.id === run.id').toBe(run.id)

    // 4. batchCreateTasks 还在后台执行（此时 store 应该还没有 task）
    //    等 batchDelayMs=200ms 过后 task 才会 push 到 store
    expect(store.tasks.length, 'batchCreateTasks 后台执行中，store 暂无 task').toBe(0)

    // 5. 等待 IIFE 完成（batchCreateTasks 200ms 延迟后 task 才到 store）
    await waitForSubmitRunComplete()
    expect(store.tasks.length, 'IIFE 完成后 store 有所有 task').toBeGreaterThan(0)
  })

  // ==================== T8: cancelRun 批量取消（一次 API）====================
  it('T8: cancelRun 批量取消——只调 1 次 cancelRun API，cancelTask 不被调用', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    const run = await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    await waitForSubmitRunComplete()

    // 取消前：所有 task 都是 queued 状态（非终态）
    const tasksBefore = store.tasks.length
    expect(tasksBefore, '取消前 store 有 task').toBeGreaterThan(0)

    // 调 service.cancelRun（批量取消整个 run）
    await service.cancelRun(run.id)

    // 1. cancelRun API 只被调 1 次（不是 N 次 cancelTask）
    expect(encvApi.cancelRun, 'cancelRun API 只调 1 次').toHaveBeenCalledTimes(1)
    expect(encvApi.cancelRun, 'cancelRun API 参数是 runId').toHaveBeenCalledWith(run.id)

    // 2. cancelTask API 没被调用（批量取消不逐个 cancelTask）
    expect(encvApi.cancelTask, 'cancelTask API 不应被调用').not.toHaveBeenCalled()

    // 3. run 状态变为 cancelled
    expect(service.currentRun.value!.status, 'run.status === cancelled').toBe('cancelled')
    expect(service.isRunning.value, 'isRunning === false').toBe(false)
  })

  // ==================== 阶段三 10 万虚拟滚动测试（T9）====================

  // ==================== T9: 10 万 task store 容量（模拟器不虚拟滚动，验证 store 容量）====================
  it('T9: 10 万 task store 容量——store.tasks.length === 100000 + visible-task-count 派生正确', async () => {
    // 📝 注意：TaskListDiagSimulator 本身不虚拟滚动（v-for 渲染所有 item）。
    //   真机 UI 用 TaskVirtualList.vue（@tanstack/vue-virtual）虚拟滚动，
    //   DOM 节点数恒定 ≤ 30（可见窗口 + overscan）。
    //   本测试验证 store 能容纳 10 万 task + 模拟器 visible-task-count 派生值正确。
    //   真机虚拟滚动行为由 TaskVirtualList.test.ts 覆盖。

    // 1. 往 store 中 push 10 万个 mock task（同一 runId）
    const RUN_ID = 'run-100k-test'
    const TOTAL = 100000
    const mockTasks: EncvTask[] = []
    for (let i = 0; i < TOTAL; i++) {
      mockTasks.push({
        id: `task-100k-${i}`,
        type: 'encrypt',
        sourcePath: `/mock/file-${i}.mp4`,
        status: 'queued',
        progress: 0,
        runId: RUN_ID,
        triggeredBy: 'automation',
        createdAt: new Date().toISOString(),
      })
    }
    // 用 appendTasksPage 批量追加（分页加载路径，不走 appendTask 单个 push）
    store.appendTasksPage(mockTasks)

    // 2. store 容量验证
    expect(store.tasks.length, `store 容纳 10 万 task（实际: ${store.tasks.length}）`).toBe(TOTAL)

    // 3. 挂载模拟器
    wrapper = mountDiag()
    await nextTick()

    // 4. store-tasks-count 显示 10 万
    const storeCount = Number(wrapper.find('[data-testid="store-tasks-count"]').text())
    expect(storeCount, `store-tasks-count === 100000（实际: ${storeCount}）`).toBe(TOTAL)

    // 5. groupedTasksByRunId 只有 1 个真 group（10 万 task 共享同一 runId）
    const realGroupCount = Number(wrapper.find('[data-testid="real-group-count"]').text())
    const fakeGroupCount = Number(wrapper.find('[data-testid="fake-group-count"]').text())
    expect(realGroupCount, '1 个真 group（10 万 task 共享 runId）').toBe(1)
    expect(fakeGroupCount, '0 伪 group').toBe(0)

    // 6. virtual-scroll-container + visible-task-count 标记存在
    expect(wrapper.find('[data-testid="virtual-scroll-container"]').exists(), 'virtual-scroll-container 标记存在').toBe(true)
    const visibleTaskCount = Number(wrapper.find('[data-testid="visible-task-count"]').text())
    expect(visibleTaskCount, `visible-task-count > 0（实际: ${visibleTaskCount}）`).toBeGreaterThan(0)

    // 7. 📝 真机虚拟滚动验证（DOM task card 数 ≤ 30）由 TaskVirtualList.test.ts 覆盖
    //    模拟器不虚拟滚动，visible-task-count 等于 displayedItems 中 kind==='task' 的数量
    // eslint-disable-next-line no-console
    console.log(`[T9] 10万 task: store=${storeCount} / 真 group=${realGroupCount} / visible-task-count=${visibleTaskCount}`)
  })
})
