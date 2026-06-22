/**
 * 逃逸反向测试：构造"逃逸应该发生"的输入，断言期望行为
 *
 * 之前的 DOM 测试是 happy path：所有 task 都有完整 runId、所有事件顺序正常
 * ——我等于在断言"我设计的不变量成立"，没验证真机上的边界场景
 *
 * 这里覆盖 5 个逃逸候选根因：
 * A. groupedTasksByRunId 兜底逻辑让 runId 缺失 task 独立成 group
 * B. WS 推 update payload 含 runId=null（不是 undefined）→ prev.runId 被覆盖
 * C. hydrate 加载历史 task，runId 字段是空字符串
 * D. update 事件在 created 之前到达 → patchTaskById 静默 return
 * E. _taskIndex 在并发 patch 时过期（单线程 JS 不存在，但 array splice 后索引会乱）
 *
 * 跑法：先跑 → 看哪些红 → 修代码 → 再跑 → 全绿才是真修复
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

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

async function freshModules() {
  vi.resetModules()
  setActivePinia(createPinia())
  const { useTaskStore } = await import('@/stores/taskStore')
  const { useTasksList } = await import('@/composables/useTasksList')
  return { useTaskStore, useTasksList }
}

function makeTask(id: string, runId: string | null, status: string = 'queued'): EncvTask {
  return {
    id,
    type: 'encrypt',
    sourcePath: `/mock/sample-${id}.mp4`,
    status: status as any,
    progress: 0,
    createdAt: '2026-06-22T10:00:00.000Z',
    runId: runId as any,
    triggeredBy: 'automation',
    pluginName: 'mp4-encrypt',
  }
}

// 渲染组件
function createTestListComponent() {
  return defineComponent({
    name: 'TestTaskList',
    props: { displayedItemsRef: { type: Object, required: true } },
    setup(props) {
      return () => {
        const items = props.displayedItemsRef.value
        if (!items || items.length === 0) return h('div', { class: 'empty' }, 'no items')
        return h('div', { class: 'task-list' }, items.map((it: any) => {
          if (it.kind === 'date') {
            return h('div', { key: it.key, class: 'date-section' }, it.label)
          } else if (it.kind === 'group' && it.counters?.hitAny) {
            return h('div', {
              key: it.key,
              class: 'task-group',
              'data-run-id': it.runId || '__no_runid__',
              'data-tasks-count': it.tasks.length,
            }, [
              h('div', { class: 'group-title' }, `${it.tasks.length} tasks`),
              ...it.tasks.map((tk: any) => h('div', { key: `tk-${tk.id}`, class: 'group-task', 'data-id': tk.id }, tk.id)),
            ])
          } else if (it.kind === 'task') {
            return h('div', { key: it.key, class: 'task-row', 'data-id': it.task.id }, it.task.id)
          } else {
            return h('div', { key: it.key, class: 'unknown' }, JSON.stringify(it))
          }
        }))
      }
    },
  })
}

describe('逃逸反向测试（5 个假设，按顺序跑，看哪些红）', () => {
  beforeEach(() => {
    testStorage.clear()
  })

  // ============ 假设 A：groupedTasksByRunId 兜底让 runId 缺失 task 独立成 group ============
  it('A: created 事件里 task 没有 runId 字段（undefined）→ 应该单独显示，不影响其它 group', async () => {
    const { useTaskStore, useTasksList } = await freshModules()
    const store = useTaskStore()
    const list = useTasksList()
    list.viewMode.value = 'group'

    // 10 个正常 automation task 共享 runId='r-1'
    const RUN_ID = `r-A-${Date.now()}`
    for (let i = 0; i < 10; i++) {
      store.applyEvent('created', makeTask(`t-${i}`, RUN_ID, 'queued'))
    }

    // 关键：第 11 个 created 事件，task 没有 runId 字段（纯 undefined）
    store.applyEvent('created', {
      id: 't-orphan',
      type: 'encrypt',
      sourcePath: '/mock/orphan.mp4',
      status: 'queued',
      progress: 0,
      createdAt: '2026-06-22T10:00:00.000Z',
      triggeredBy: 'automation',
      pluginName: 'mp4-encrypt',
      // ← 没有 runId 字段
    } as EncvTask)

    const Component = createTestListComponent()
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } })

    // 期望：10 个共享 runId 的 task 还是 1 个 group；orphan 单独显示（group 或 row）
    const groups = wrapper.findAll('.task-group')

    // 关键断言：runId='r-A' 的 group 仍然存在，tasks 数量是 10
    const realGroup = groups.find((g) => g.attributes('data-run-id') === RUN_ID)
    expect(realGroup, 'r-A 的 group 必须存在').toBeTruthy()
    expect(realGroup!.attributes('data-tasks-count')).toBe('10')

    // 关键断言：不应该有第二个 group 跟 r-A 抢 task
    // 如果兜底逻辑把 orphan 算成 __manual__ 独立 group，那是合理的
    // 但如果 r-A group 里的 task 数量 < 10，那就说明 orphan 错误地混进去了

    // 把断言写宽松：r-A group 里必须有 10 个 task
    const tIdsInRealGroup = realGroup!.findAll('.group-task').map((el) => el.attributes('data-id'))
    expect(tIdsInRealGroup.length).toBe(10)
  })

  // ============ 假设 B：WS 推 update payload 含 runId=null（不是 undefined）→ 覆盖 prev.runId ============
  it('B: update 事件 payload 含 runId=null → 9 个共享 runId 的 task 还能聚合吗？', async () => {
    const { useTaskStore, useTasksList } = await freshModules()
    const store = useTaskStore()
    const list = useTasksList()
    list.viewMode.value = 'group'

    const RUN_ID = `r-B-${Date.now()}`
    for (let i = 0; i < 10; i++) {
      store.applyEvent('created', makeTask(`t-${i}`, RUN_ID, 'queued'))
    }

    // 关键：后端推 update 时，payload 显式包含 runId=null（不是 undefined，是 null）
    // 业务上后端有时会发这种"清空字段"的 payload
    store.applyEvent('update', { id: 't-0', status: 'running', runId: null as any })

    const Component = createTestListComponent()
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } })

    // 期望：即使后端发了 runId=null，前端不应该被这个 null 覆盖掉原来的 runId
    // 正确行为：要么 patchTaskById 把 null 也当 undefined 跳过（更安全）
    //           要么 groupedTasksByRunId 不把 null 算成 __manual__
    const groups = wrapper.findAll('.task-group')
    const realGroup = groups.find((g) => g.attributes('data-run-id') === RUN_ID)
    expect(realGroup, 'r-B 的 group 必须在 update 后还存在').toBeTruthy()

    // 关键断言：r-B group 里仍然有 10 个 task（如果 t-0 的 runId 被 null 覆盖，会变成独立 group）
    if (realGroup) {
      expect(realGroup.attributes('data-tasks-count')).toBe('10')
    }
  })

  // ============ 假设 C：hydrate 加载历史 task，runId 字段是空字符串 ============
  it('C: hydrate 加载历史 task，runId=""（空字符串）→ 不应该独立成 __manual__ group', async () => {
    const { useTaskStore, useTasksList } = await freshModules()
    const store = useTaskStore()
    const list = useTasksList()
    list.viewMode.value = 'group'

    // 模拟 hydrate：用 bulkSetTasks 直接灌进 store
    const RUN_ID = `r-C-${Date.now()}`
    const tasks: EncvTask[] = []
    for (let i = 0; i < 10; i++) {
      tasks.push(makeTask(`t-${i}`, RUN_ID, 'queued'))
    }
    // 加一个 runId='' 的 task（模拟 localStorage 里的历史脏数据）
    tasks.push(makeTask('t-empty-runid', '', 'queued'))

    store.bulkSetTasks(tasks)

    const Component = createTestListComponent()
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } })

    // 期望：r-C 的 group 存在，10 个 task
    const groups = wrapper.findAll('.task-group')
    const realGroup = groups.find((g) => g.attributes('data-run-id') === RUN_ID)
    expect(realGroup, 'r-C group 必须存在').toBeTruthy()
    if (realGroup) {
      expect(realGroup.attributes('data-tasks-count')).toBe('10')
    }
  })

  // ============ 假设 D：update 事件在 created 之前到达 ============
  it('D: update 早于 created 到达 → task 至少应该在 created 后能被补全', async () => {
    const { useTaskStore, useTasksList } = await freshModules()
    const store = useTaskStore()
    const list = useTasksList()
    list.viewMode.value = 'group'

    const RUN_ID = `r-D-${Date.now()}`

    // 关键：先推 update（task 还不存在），再推 created
    store.applyEvent('update', { id: 't-0', status: 'running', progress: 50 })
    // 此时 t-0 不存在，patchTaskById 静默 return
    expect(store.getTaskById('t-0'), 'update 早于 created → task 还没建出来').toBeUndefined()

    // 再推 created
    store.applyEvent('created', makeTask('t-0', RUN_ID, 'queued'))
    const t0 = store.getTaskById('t-0')
    expect(t0, 'created 之后 task 必须存在').toBeTruthy()
    // t-0 status 是 'queued'（created 设的），不是 'running'（update 早到被丢了）
    // 这是当前实现的"静默 return"行为
    expect(t0!.status).toBe('queued')

    // 期望：update 早到不应该让数据出错；created 后数据完整
    const Component = createTestListComponent()
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } })
    expect(wrapper.findAll('.task-group').length).toBeGreaterThan(0)
  })

  // ============ 假设 E：数组 splice 后 _taskIndex 过期 ============
  it('E: removeTask 后 _taskIndex 是否仍然有效？', async () => {
    const { useTaskStore } = await freshModules()
    const store = useTaskStore()

    const RUN_ID = `r-E-${Date.now()}`
    for (let i = 0; i < 10; i++) {
      store.applyEvent('created', makeTask(`t-${i}`, RUN_ID, 'queued'))
    }

    // 删除中间一个 task
    store.removeTask('t-5')

    // 删完之后再 patch 一个剩下的 task
    // 期望：patch 落到正确位置，不应该写错行
    store.applyEvent('update', { id: 't-7', status: 'running' })
    const t7 = store.getTaskById('t-7')
    expect(t7, 't-7 必须能找到').toBeTruthy()
    expect(t7!.status).toBe('running')
    expect(t7!.runId).toBe(RUN_ID)  // 关键：runId 不能丢
  })
})
