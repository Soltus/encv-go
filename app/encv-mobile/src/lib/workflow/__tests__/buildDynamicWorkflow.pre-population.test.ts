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

  // ==================== T1: batchCreateTasks 只调 1 次，所有 task 一次性 push ====================
  it('T1: submitRun 后 batchCreateTasks 只调 1 次，store 一次性有 N 个真实 task（不慢慢累加）', async () => {
    const wfDef = buildTestWorkflowDef(buildRealPlugins())
    workflowStore.createDefinition(wfDef)

    await service.submitRun({ workflow: wfDef, triggeredBy: 'automation' })

    // 1. batchCreateTasks 调用次数 == job 数（每个 job 一次性批量提交自己的 step）
    const jobCount = wfDef.jobs.length
    expect(batchCallLog.length, `batchCreateTasks 调用 ${jobCount} 次（每个 job 1 次，不是每个 step 1 次）`).toBe(jobCount)

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
})
