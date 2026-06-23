/**
 * 集成测试：模拟一组自动化 task 的全生命周期，断言 displayedItems 始终聚合
 *
 * 逃逸场景：10 个 task 共享 runId='r-1'，每个 task 收到 status update 后
 * 跑 patchTaskById；旧实现 spread undefined 覆盖 runId → task 失去 runId →
 * displayedItems 把它当 10 个伪 group（每个 task 单独成 group）→ 视觉上"逃出"聚合
 *
 * 修复后：patchTaskById 跳过 undefined 字段 → runId 保留 → 始终 1 个 group
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'

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

// 动态 import：每个测试用全新 pinia + 全新 useTasksList 单例
async function freshModules() {
  vi.resetModules()
  setActivePinia(createPinia())
  const { useTaskStore } = await import('@/stores/taskStore')
  const { useTasksList } = await import('@/composables/useTasksList')
  return { useTaskStore, useTasksList }
}

function makeTask(id: string, runId: string, status: string = 'queued'): EncvTask {
  return {
    id,
    type: 'encrypt',
    sourcePath: `/mock/sample-${id}.mp4`,
    status: status as any,
    progress: 0,
    createdAt: '2026-06-22T10:00:00.000Z',
    runId,
    triggeredBy: 'automation',
    pluginName: 'mp4-encrypt',
  }
}

describe('一组自动化 task 状态变化不逃逸（v7 修复验证）', () => {
  beforeEach(() => {
    testStorage.clear()
  })

  it('10 个 task 共享 runId，created → 100 次 update 后仍聚合为 1 个 group', async () => {
    const { useTaskStore, useTasksList } = await freshModules()
    const store = useTaskStore()
    const list = useTasksList()
    list.viewMode.value = 'group'

    // 1) created 一组 10 个 task
    const RUN_ID = `r-${Date.now()}-1`
    const taskIds: string[] = []
    for (let i = 0; i < 10; i++) {
      const id = `t-${i.toString().padStart(2, '0')}`
      taskIds.push(id)
      store.applyEvent('created', makeTask(id, RUN_ID, 'queued'))
    }

    // 断言：10 个 task 全部加载
    expect(store.tasks.length).toBe(10)
    // 断言：聚合为 1 个 group
    expect(list.groupedTasksByRunId.value.length).toBe(1)
    expect(list.groupedTasksByRunId.value[0].runId).toBe(RUN_ID)
    expect(list.groupedTasksByRunId.value[0].tasks.length).toBe(10)

    // 2) 模拟后端 WS 推 100 次 update（每个 task 一次 progress 更新）
    //    关键：后端 update payload 不带 runId（按 v7 设计，update 只发变化字段）
    for (let round = 0; round < 10; round++) {
      for (const id of taskIds) {
        store.applyEvent('progress', { id, progress: (round + 1) * 10 })
      }
    }

    // 3) 断言：所有 task 的 runId 仍然存在（关键：未逃逸）
    for (const id of taskIds) {
      const t = store.getTaskById(id)
      expect(t).toBeDefined()
      expect(t!.runId).toBe(RUN_ID)
    }

    // 4) 断言：聚合仍然是 1 个 group（关键：未逃逸成 10 个伪 group）
    const groups = list.groupedTasksByRunId.value
    expect(groups.length).toBe(1)
    expect(groups[0].runId).toBe(RUN_ID)
    expect(groups[0].tasks.length).toBe(10)

    // 5) 断言：displayedItems 渲染为 1 个 kind='group' + 1 个 kind='date'
    const items = list.displayedItems.value
    const dateItems = items.filter((it: any) => it.kind === 'date')
    const groupItems = items.filter((it: any) => it.kind === 'group')
    const taskItems = items.filter((it: any) => it.kind === 'task')
    expect(dateItems.length).toBe(1)
    expect(groupItems.length).toBe(1)  // ← 不应该是 0 或 10
    expect(taskItems.length).toBe(0)   // ← 不会有"逃逸"成独立 task row
  })

  it('update payload 含 runId 时，正常更新且不破坏聚合', async () => {
    const { useTaskStore, useTasksList } = await freshModules()
    const store = useTaskStore()
    const list = useTasksList()
    list.viewMode.value = 'group'

    const RUN_ID = `r-${Date.now()}-2`
    store.applyEvent('created', makeTask('t-1', RUN_ID, 'queued'))
    store.applyEvent('created', makeTask('t-2', RUN_ID, 'queued'))
    const beforeGroup = list.groupedTasksByRunId.value.find((g: any) => g.runId === RUN_ID)!
    expect(beforeGroup.tasks.length).toBe(2)

    // 后端 update payload 含完整元数据
    store.applyEvent('update', {
      id: 't-1',
      type: 'encrypt',
      status: 'running',
      progress: 50,
      runId: RUN_ID,
      triggeredBy: 'automation',
      pluginName: 'mp4-encrypt',
    })

    const afterGroup = list.groupedTasksByRunId.value.find((g: any) => g.runId === RUN_ID)!
    expect(afterGroup.tasks.length).toBe(2)
    expect(store.getTaskById('t-1')!.runId).toBe(RUN_ID)
  })

  it('completed 事件触发后 group 仍存在，task 收尾状态正确', async () => {
    const { useTaskStore, useTasksList } = await freshModules()
    const store = useTaskStore()
    const list = useTasksList()
    list.viewMode.value = 'group'

    const RUN_ID = `r-${Date.now()}-3`
    for (let i = 0; i < 5; i++) {
      store.applyEvent('created', makeTask(`t-${i}`, RUN_ID, 'running'))
    }
    const beforeGroup = list.groupedTasksByRunId.value.find((g: any) => g.runId === RUN_ID)!
    expect(beforeGroup.tasks.length).toBe(5)

    for (let i = 0; i < 5; i++) {
      store.applyEvent('completed', {
        id: `t-${i}`,
        outputPath: `/mock/out-${i}.mp4`,
      })
    }

    for (let i = 0; i < 5; i++) {
      const t = store.getTaskById(`t-${i}`)
      expect(t!.status).toBe('completed')
      expect(t!.runId).toBe(RUN_ID)
      expect(t!.progress).toBe(100)
    }

    const afterGroup = list.groupedTasksByRunId.value.find((g: any) => g.runId === RUN_ID)!
    expect(afterGroup.tasks.length).toBe(5)
  })

  it('乱序事件：progress 在 created 之前到达也不应破坏', async () => {
    const { useTaskStore, useTasksList } = await freshModules()
    const store = useTaskStore()
    const list = useTasksList()
    list.viewMode.value = 'group'

    // 模拟乱序：progress 先到
    store.applyEvent('progress', { id: 'late-1', progress: 50 })
    store.applyEvent('progress', { id: 'late-2', progress: 50 })

    // 此时 task 还不存在 → patchTaskById 找不到 id → 返回 false → 不报错
    // 然后 created 才到
    const RUN_ID = `r-${Date.now()}-late`
    store.applyEvent('created', makeTask('late-1', RUN_ID, 'running'))
    store.applyEvent('created', makeTask('late-2', RUN_ID, 'running'))

    const t1 = store.getTaskById('late-1')!
    const t2 = store.getTaskById('late-2')!
    expect(t1.status).toBe('running')
    expect(t1.progress).toBe(0)
    expect(t2.progress).toBe(0)

    // 后续 progress 更新能正常工作
    store.applyEvent('progress', { id: 'late-1', progress: 80 })
    expect(store.getTaskById('late-1')!.progress).toBe(80)
    expect(store.getTaskById('late-1')!.runId).toBe(RUN_ID)
  })
})
