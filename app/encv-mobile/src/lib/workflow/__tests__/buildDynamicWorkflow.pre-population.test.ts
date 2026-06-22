/**
 * 🆕 2026-06-22 真因修复 5：submitRun 同步阶段预占位 1000+ placeholder task
 *
 * 核心问题（user 反馈）：
 *   "聚合任务数量缓慢增加到1000+本身就是问题你没发现吗？运行自动化插件测试应当一下子就显示所有聚会任务才对！"
 *
 *   历史 bug：submitRun 逐个 await createTask → 1000+ task 慢慢累加到 UI 上
 *   修法：submitRun 阶段同步 push 1000+ placeholder EncvTask 到 store（client 生成 ID）→
 *         调 createTask API 把 client ID 传给后端 → 后端用 client ID 覆盖默认 UUID →
 *         WS 推 task:created 时 task.id == placeholder.id → 找到 placeholder patch 更新（不 append）
 *         → submitRun 同步阶段 UI 立即显示 1 个 group 1000+ task
 *
 * 关键验证点：
 *   1. submitRun 同步阶段（不 await）store.tasks 已经有 N 个 placeholder（不依赖 createTask API）
 *   2. placeholder.id 跟 createTask API 返回的 task.id 完全一致（client UUID 覆盖）
 *   3. WS task:created 推送不产生重复 task（_taskIndex.has 找到 placeholder → patch 而非 append）
 *   4. UI groupedTasksByRunId 在 submitRun 同步阶段后已经 1 个真 group N task
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
const uuidByIndex = new Map<number, string>()
const mockCrypto = {
  randomUUID: () => {
    uuidCounter++
    const id = `00000000-0000-4000-8000-${String(uuidCounter).padStart(12, '0')}`
    uuidByIndex.set(uuidCounter, id)
    return id
  },
}
// 覆盖 globalThis.crypto（原生 crypto 是 getter，Object.defineProperty 才能改）
try {
  Object.defineProperty(globalThis, 'crypto', { value: mockCrypto, writable: true, configurable: true })
} catch {
  // jsdom / happy-dom 可能拦截 → 改用 globalThis 顶层覆盖
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

// ==================== Mock createTask API（追踪 clientTaskId + 延迟 + 返回同 ID）====================
const createTaskCallLog: Array<{ clientTaskId?: string; type: TaskType; sourcePath: string; runId?: string }> = []
let createTaskDelayMs = 50  // 每个 createTask 50ms 延迟（模拟网络）
let createTaskShouldFail = false

vi.mock('@/api/encv', async () => {
  const actual = await vi.importActual<typeof encvApi>('@/api/encv')
  return {
    ...actual,
    createTask: vi.fn(async (
      type: TaskType,
      sourcePath: string,
      _targetPath?: string,
      _password?: string,
      _version?: number,
      _pluginName?: string,
      _extraFields?: Record<string, string>,
      _secondaryPassword?: string,
      _cipherMode?: number,
      _compressionMode?: 'none' | 'zstd',
      runId?: string,
      _triggeredBy?: 'user' | 'automation' | 'ai_agent',
      clientTaskId?: string,
    ): Promise<EncvTask> => {
      createTaskCallLog.push({ clientTaskId, type, sourcePath, runId })
      await new Promise((r) => setTimeout(r, createTaskDelayMs))
      if (createTaskShouldFail) {
        throw new Error('mock-createTask-failure')
      }
      // 返回 task 时 ID 必须 == clientTaskId（模拟后端用 client ID 覆盖默认 UUID）
      return {
        id: clientTaskId ?? `server-generated-${createTaskCallLog.length}`,
        type,
        sourcePath,
        status: 'running',
        progress: 0,
        runId,
        triggeredBy: 'automation',
        createdAt: new Date().toISOString(),
      }
    }),
    getTasks: vi.fn().mockResolvedValue([]),
    cancelTask: vi.fn().mockResolvedValue(undefined),
    removeTask: vi.fn().mockResolvedValue(undefined),
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
}

// ==================== 构造测试用 WorkflowDefinition ====================
import { buildDynamicWorkflowPure } from '@/lib/workflow/buildDynamicWorkflow'
import type { WorkflowDefinition } from '@/lib/workflow/types'

function buildTestWorkflowDef(plugins: PluginMeta[]): WorkflowDefinition {
  const { wfDef } = buildDynamicWorkflowPure(plugins, '/mock/')
  return wfDef
}

describe('真机"任务逃逸"e2e — submitRun 同步预占位 1000+ placeholder', () => {
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
    createTaskCallLog.length = 0
    uuidCounter = 0
    uuidByIndex.clear()
    createTaskDelayMs = 50
    createTaskShouldFail = false
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
    // 🆕 2026-06-22
    const { __resetServiceForTests: resetService } = await import('@/composables/useWorkflowTaskService')
    resetService()
  })

  /** 挂载 1:1 复刻真机 UI 模拟器 */
  function mountDiag(): VueWrapper<any> {
    return mount(TaskListDiagSimulator, {
      props: { store, composable, handlers: noopHandlers },
    })
  }

  // ==================== T1: 核心场景：submitRun 同步阶段 placeholder 已就位 ====================
  it('T1: submitRun 同步阶段后，store 立即有 N 个 placeholder（不依赖 createTask API 返回）', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    // 注册 wfDef 到 store（executeJob 内部会用）
    workflowStore.createDefinition(wfDef)

    // 调 submitRun（不要 await，让它跑）
    const runPromise = service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    // 关键断言：submitRun 同步阶段后（microtask 之前）就 push 1000+ placeholder
    //   submitRun 是 async function，调用立即进入 Promise 构造函数 → 同步代码到第一个 await
    //   我们的 placeholder 推入代码在 submitRun 同步阶段（for 循环 + taskStore.appendTask）
    //   第一次 await 是 Promise.all(workers) → 所以 placeholder 数量在同步阶段就确定
    await nextTick()  // 让 microtask 跑（submitRun 内部第一个 await 之前）
    const placeholderCountAfterSync = store.tasks.length
    // eslint-disable-next-line no-console
    console.log(`[T1] placeholder 同步阶段数量: ${placeholderCountAfterSync}, jobs: ${wfDef.jobs.length}`)
    for (const j of wfDef.jobs) {
      // eslint-disable-next-line no-console
      console.log(`  job ${j.id}: strategy=${JSON.stringify(j.strategy)}, steps=${j.steps.length}`)
    }
    expect(placeholderCountAfterSync, 'submitRun 同步阶段后 store 立即有 placeholder（不依赖 createTask API）').toBeGreaterThanOrEqual(50)
    // 同步阶段所有 placeholder 都应该有 runId + status='queued'
    const allHaveRunId = store.tasks.every((t) => !!t.runId)
    const allQueued = store.tasks.every((t) => t.status === 'queued')
    expect(allHaveRunId, '同步阶段 placeholder 都应该有 runId').toBe(true)
    expect(allQueued, '同步阶段 placeholder 都应该是 queued').toBe(true)

    // 等等 runPromise 完成
    await runPromise
    // eslint-disable-next-line no-console
    console.log(`[T1] 全部完成后 store 任务数: ${store.tasks.length} / createTask API 调用次数: ${createTaskCallLog.length}`)
  })

  // ==================== T2: placeholder.id == createTask API 返回的 task.id ====================
  it('T2: placeholder.id 跟 createTask API 返回的 task.id 完全一致（client UUID 覆盖后端默认 UUID）', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    await new Promise((r) => setTimeout(r, 500))  // 等等所有 createTask 完成

    // 1. createTask 都被调用，且都传了 clientTaskId
    expect(createTaskCallLog.length).toBeGreaterThan(0)
    const allCallsWithClientId = createTaskCallLog.every((c) => !!c.clientTaskId)
    expect(allCallsWithClientId, '所有 createTask 调用都必须传 clientTaskId').toBe(true)

    // 2. createTask 返回的 task.id 跟 placeholder.id 完全一致（client UUID 覆盖后端默认 UUID）
    //    我们 mock createTask 返回 { id: clientTaskId }，验证 store 里 placeholder 没被 append 重复
    //    实际上 store.tasks.length 应该 == createTaskCallLog.length（不重复）
    //    因为 placeholder 被 patch（不是 append）→ store 里没多出来的 task
    expect(store.tasks.length, 'placeholder 跟返回 task 是同一个 ID，store 里没重复').toBe(createTaskCallLog.length)

    // 3. 每个 store.tasks 里的 task 的 id 都来自 placeholder（client-${uuid}）
    const allClientIds = store.tasks.every((t) => t.id.startsWith('client-'))
    expect(allClientIds, 'store 里所有 task.id 都以 client- 开头（placeholder ID）').toBe(true)
  })

  // ==================== T3: WS task:created 推相同 id → 不重复 append ====================
  it('T3: 模拟 WS 推 task:created（用 placeholder.id）→ 不重复 append（patch 而非 append）', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    await new Promise((r) => setTimeout(r, 500))

    // 此时 store.tasks.length == createTaskCallLog.length（已确认 T2）
    const tasksBefore = store.tasks.length

    // 模拟后端 WS 推 task:created（用 placeholder.id）—— 模拟客户端 HTTP poll 收到的 task
    // 这个场景：后端 createTask 返回后，WS 又推一次 task:created（带同样 id）
    //   store.applyEvent('created', task) → appendTask → _taskIndex.has(id) === true → patchTaskById
    //   store.tasks.length 不变
    for (const t of [...store.tasks]) {
      store.applyEvent('created', { ...t, status: 'running' })
    }
    expect(store.tasks.length, 'WS 重推 task:created 不应该 append 重复（patch 而非 append）').toBe(tasksBefore)
  })

  // ==================== T4: UI 1:1 复刻模拟器 — submitRun 同步阶段后立即显示 1 个真 group 1000+ task ====================
  it('T4: submitRun 同步阶段后，1:1 复刻 UI 立即显示 1 个真 group N task（不再"慢慢累加"）', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    const runPromise = service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })
    await nextTick()
    wrapper = mountDiag()
    await nextTick()

    const realGroupCount = Number(wrapper.find('[data-testid="real-group-count"]').text())
    const fakeGroupCount = Number(wrapper.find('[data-testid="fake-group-count"]').text())
    const escapeTaskCount = Number(wrapper.find('[data-testid="escape-task-count"]').text())
    const totalTaskCount = Number(wrapper.find('[data-testid="store-tasks-count"]').text())
    // eslint-disable-next-line no-console
    console.log(`[T4] submitRun 同步阶段后: totalTask=${totalTaskCount} / 真 group=${realGroupCount} / 伪 group=${fakeGroupCount} / 逃逸=${escapeTaskCount}`)
    expect(totalTaskCount, '同步阶段后 store 立即有 placeholder 任务').toBeGreaterThanOrEqual(50)
    expect(realGroupCount, '同步阶段后 1 个真 group（所有 placeholder 共享 runId）').toBe(1)
    expect(fakeGroupCount, '同步阶段后 0 伪 group（placeholder 都有 runId）').toBe(0)
    expect(escapeTaskCount, '同步阶段后 0 逃逸').toBe(0)

    await runPromise
    await new Promise((r) => setTimeout(r, 200))
    // 等等 createTask 完成后仍 0 逃逸
    const fakeGroupCountAfter = Number(wrapper.find('[data-testid="fake-group-count"]').text())
    const escapeTaskCountAfter = Number(wrapper.find('[data-testid="escape-task-count"]').text())
    expect(fakeGroupCountAfter, 'createTask 完成后仍 0 伪 group').toBe(0)
    expect(escapeTaskCountAfter, 'createTask 完成后仍 0 逃逸').toBe(0)
  })
})
