/**
 * 复现：自动化测试任务聚合模式显示 100+ 张"自动化"卡片
 * 根因验证：task.runId 缺失时，useTasksList.groupedItems 会把每个 task 单独成组
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// 提供一个隔离的 localStorage，避免污染其他并行测试
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

function makeTask(id: string, sourcePath: string): EncvTask {
  return {
    id,
    type: 'encrypt',
    sourcePath,
    status: 'completed',
    progress: 100,
    createdAt: '2026-06-18T10:00:00.000Z',
    completedAt: '2026-06-18T10:01:00.000Z',
  }
}

describe('useTasksList — 自动化测试任务分组', () => {
  beforeEach(() => {
    testStorage.clear()
    _reloadTriggeredByCache()
    setActivePinia(createPinia())
  })
  afterEach(() => {
    testStorage.clear()
  })

  it('同一 runId 的 12 个 task 应该聚合成 1 个 group', () => {
    const store = useTaskStore()
    const runId = 'run-automation-001'
    const tasks: EncvTask[] = Array.from({ length: 12 }, (_, i) =>
      makeTask(`task-${i}`, `/mock/sample-${i}.mp4`),
    )

    // 模拟 workflow service 提交时做的关联：setTaskMetadata(taskId, 'automation', runId)
    for (const t of tasks) {
      setTaskMetadata(t.id, 'automation', runId)
    }

    store.bulkSetTasks(tasks)

    const list = useTasksList()
    const groups = list.groupedItems.value

    // ❌ 当前行为：12 个 group（每个 task 单独一组，因为 task.runId undefined）
    // ✅ 期望行为：1 个 group
    expect(groups.length).toBe(1)
    expect(groups[0].runId).toBe(runId)
    expect(groups[0].tasks.length).toBe(12)
  })
})
