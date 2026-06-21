/**
 * 复现：任务状态刷新后 task 从 group 逃逸到顶层
 * 测试 WS 4 件套 + fetchTasks 是否导致 runId 丢失
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

const testStorage = new Map<string, string>()
const mockLocalStorage: Storage = {
  get length() { return testStorage.size },
  key: (index: number) => Array.from(testStorage.keys())[index] ?? null,
  getItem: (key: string) => testStorage.get(key) ?? null,
  setItem: (key: string, value: string) => { testStorage.set(key, value) },
  removeItem: (key: string) => { testStorage.delete(key) },
  clear: () => { testStorage.clear() },
} as unknown as Storage
vi.stubGlobal('localStorage', mockLocalStorage)

import { setActivePinia, createPinia } from 'pinia'
import { useTaskStore } from '@/stores/taskStore'
import { useTasksList } from '@/composables/useTasksList'
import { setTaskMetadata, _reloadTriggeredByCache } from '@/composables/useTaskTrigger'
import type { EncvTask } from '@/api/encv'

vi.mock('@/composables/useTaskEventBridge', () => ({
  useTaskEventBridge: () => {},
}))

vi.mock('@/api/encv', async () => {
  const actual = await vi.importActual<typeof import('@/api/encv')>('@/api/encv')
  return {
    ...actual,
    getTasks: vi.fn().mockResolvedValue([]),
    cancelTask: vi.fn().mockResolvedValue(undefined),
    removeTask: vi.fn().mockResolvedValue(undefined),
    retryTask: vi.fn().mockResolvedValue(undefined),
  }
})

function makeTask(id: string, runId: string, status: string = 'running'): EncvTask {
  return {
    id,
    type: 'encrypt',
    sourcePath: `/mock/sample-${id}.mp4`,
    status: status as any,
    progress: 50,
    createdAt: '2026-06-18T10:00:00.000Z',
    runId,
    triggeredBy: 'automation',
  }
}

/** 模拟后端返回的 task（无 runId / triggeredBy） */
function makeBackendTask(id: string, status: string = 'running'): EncvTask {
  return {
    id,
    type: 'encrypt',
    sourcePath: `/mock/sample-${id}.mp4`,
    status: status as any,
    progress: 50,
    createdAt: '2026-06-18T10:00:00.000Z',
  }
}

describe('任务逃逸复现', () => {
  let store: ReturnType<typeof useTaskStore>
  let list: ReturnType<typeof useTasksList>
  const RUN_ID = 'run-automation-001'

  beforeEach(() => {
    testStorage.clear()
    _reloadTriggeredByCache()
    setActivePinia(createPinia())
    store = useTaskStore()
    store.$reset()
    // 模拟 workflow service 提交 12 个 task
    for (let i = 0; i < 12; i++) {
      setTaskMetadata(`task-${i}`, 'automation', RUN_ID)
    }
    // 用带 runId 的 task 初始化 store
    const tasks = Array.from({ length: 12 }, (_, i) => makeTask(`task-${i}`, RUN_ID))
    store.bulkSetTasks(tasks)
    list = useTasksList()
  })
  afterEach(() => {
    testStorage.clear()
  })

  it('初始状态：12 个 task 聚合成 1 个 group', () => {
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
    expect(groups[0].tasks.length).toBe(12)
  })

  it('WS task:update 不应导致逃逸', () => {
    store.applyTaskUpdate({ id: 'task-0', type: 'encrypt', status: 'completed', progress: 100 })
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
    expect(groups[0].tasks.length).toBe(12)
  })

  it('WS task:progress 不应导致逃逸', () => {
    store.applyTaskProgress({ id: 'task-0', progress: 80, phase: 'encoding', speed: '12MB/s', eta: '5s' })
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
  })

  it('WS task:completed 不应导致逃逸', () => {
    store.applyTaskCompleted({ id: 'task-0' })
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
  })

  it('fetchTasks（后端无 runId）不应导致逃逸', () => {
    // 模拟 fetchTasks：后端返回的 task 无 runId，bulkSetTasks merge 模式保留 store 里已有 runId
    const backendTasks = Array.from({ length: 12 }, (_, i) => makeBackendTask(`task-${i}`, 'completed'))
    store.bulkSetTasks(backendTasks)
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
    expect(groups[0].tasks.length).toBe(12)
    expect(groups[0].runId).toBe(RUN_ID)
  })

  it('fetchTasks 后再次 WS 事件不应导致逃逸', () => {
    // 1. fetchTasks（后端无 runId，但 store 有 → merge 保留）
    const backendTasks = Array.from({ length: 12 }, (_, i) => makeBackendTask(`task-${i}`, 'running'))
    store.bulkSetTasks(backendTasks)
    // 2. WS update
    store.applyTaskUpdate({ id: 'task-0', type: 'encrypt', status: 'completed', progress: 100 })
    // 3. 再次 fetchTasks
    store.bulkSetTasks(backendTasks.map((t, i) => ({ ...t, status: i === 0 ? 'completed' : 'running' })))
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
    expect(groups[0].tasks.length).toBe(12)
  })

  it('冷启动模拟：useTaskTrigger 无数据时，store 已有 runId 不应丢失', () => {
    // 模拟冷启动后 useTaskTrigger 被清空
    _reloadTriggeredByCache()
    testStorage.clear()
    // store 里已有 task（带 runId，来自 hydrate）
    // fetchTasks → bulkSetTasks → merge 模式从 store 回填
    const backendTasks = Array.from({ length: 12 }, (_, i) => makeBackendTask(`task-${i}`, 'completed'))
    store.bulkSetTasks(backendTasks)
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
    expect(groups[0].runId).toBe(RUN_ID)
  })

  it('连续 WS 事件 + fetchTasks 不应导致逃逸', () => {
    // 1. WS update
    store.applyTaskUpdate({ id: 'task-0', type: 'encrypt', status: 'completed', progress: 100 })
    // 2. WS progress
    store.applyTaskProgress({ id: 'task-1', progress: 80, phase: 'encoding', speed: '12MB/s', eta: '5s' })
    // 3. WS completed
    store.applyTaskCompleted({ id: 'task-2' })
    // 4. fetchTasks
    const backendTasks = Array.from({ length: 12 }, (_, i) => makeBackendTask(`task-${i}`, i <= 2 ? 'completed' : 'running'))
    store.bulkSetTasks(backendTasks)
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
    expect(groups[0].tasks.length).toBe(12)
  })

  it('HttpPollBackend task:created（完整 task 无 runId）不应导致逃逸', () => {
    // 模拟 HttpPollBackend emit task:created，payload 是后端完整 task（无 runId）
    const backendTask = makeBackendTask('task-0', 'completed')
    store.applyTaskCreated(backendTask as any)
    const groups = list.groupedItems.value
    expect(groups.length).toBe(1)
    expect(groups[0].tasks.length).toBe(12)
  })
})
