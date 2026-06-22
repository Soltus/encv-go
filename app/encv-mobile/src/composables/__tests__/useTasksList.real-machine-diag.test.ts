/**
 * 真机 1+1+365 拆 3 个 group 场景的端到端诊断
 *
 * 设计目标：跑完 5 个场景 + 输出每个阶段的详细 UI 诊断，让真因自己浮现。
 *
 * 场景列表：
 * S1: 3 次 submitRun（1+1+365 个 step）+ WS 4 件套 + fetchTasks
 *     → 应该显示 3 个 group 标签"自动化"（这是正常，还是逃逸？）
 * S2: WS task:update 事件带 null runId → patchTaskById merge 模式
 * S3: fetchTasks 返回无 runId task + bulkSetTasks 直接覆盖
 * S4: WS task:created 事件无 runId 字段（Go omitempty）
 * S5: useTasksList module-level singleton + Pinia $reset 跨测试 isolation
 *
 * 诊断输出：每个场景输出「store.tasks 列表 + 每个 task 的 runId/triggeredBy/来源」
 *            + 「groupedTasksByRunId 实际分组 + displayedItems UI 实际显示」
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
import type { EncvTask, TaskStatus } from '@/api/encv'

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

// ============ 诊断输出工具 ============
function logSection(title: string): void {
  // eslint-disable-next-line no-console
  console.log(`\n${'='.repeat(80)}\n[DIAG] ${title}\n${'='.repeat(80)}`)
}

function logStoreTasks(store: ReturnType<typeof useTaskStore>, label: string): void {
  const tasks = store.tasks as unknown as EncvTask[]
  // eslint-disable-next-line no-console
  console.log(`[DIAG] ${label}: store.tasks.length=${tasks.length}`)
  for (const tk of tasks) {
    // eslint-disable-next-line no-console
    console.log(
      `  - id=${tk.id} runId=${JSON.stringify(tk.runId)} triggeredBy=${JSON.stringify(tk.triggeredBy)} status=${tk.status}`,
    )
  }
}

function logGroupedTasks(list: ReturnType<typeof useTasksList>, label: string): void {
  const groups = list.groupedTasksByRunId.value
  // eslint-disable-next-line no-console
  console.log(`[DIAG] ${label}: list.groupedTasksByRunId.length=${groups.length}`)
  for (const g of groups) {
    // eslint-disable-next-line no-console
    console.log(
      `  - runId=${JSON.stringify(g.runId)} key=${g.key} tasks.length=${g.tasks.length} taskIds=[${g.tasks.map((t) => t.id).join(',')}]`,
    )
  }
}

function logDisplayedItems(list: ReturnType<typeof useTasksList>, label: string): void {
  const items = list.displayedItems.value
  // eslint-disable-next-line no-console
  console.log(`[DIAG] ${label}: list.displayedItems.length=${items.length}`)
  for (const it of items) {
    if (it.kind === 'date') {
      // eslint-disable-next-line no-console
      console.log(`  - [date] ${it.label}`)
    } else if (it.kind === 'group') {
      // eslint-disable-next-line no-console
      console.log(
        `  - [group] runId=${JSON.stringify(it.runId)} tasks.length=${it.tasks.length} counter.hitAny=${it.counters?.hitAny}`,
      )
    } else if (it.kind === 'task') {
      // eslint-disable-next-line no-console
      console.log(
        `  - [task] id=${it.task.id} runId=${JSON.stringify(it.task.runId)} triggeredBy=${JSON.stringify(it.task.triggeredBy)}`,
      )
    }
  }
}

function logComputedEquivalence(
  store: ReturnType<typeof useTaskStore>,
  list: ReturnType<typeof useTasksList>,
  label: string,
): void {
  // eslint-disable-next-line no-console
  console.log(`[DIAG] ${label}: same computed? store.groupedTasksByRunId === list.groupedTasksByRunId →`,
    // @ts-ignore
    store.groupedTasksByRunId === list.groupedTasksByRunId.value,
  )
}

// ============ Task 工厂 ============
function makeTask(
  id: string,
  opts: {
    runId?: string
    triggeredBy?: 'user' | 'automation' | 'ai_agent'
    status?: TaskStatus
    source?: 'ws-created' | 'ws-update' | 'ws-progress' | 'ws-completed' | 'fetch' | 'hydrate'
  } = {},
): EncvTask {
  return {
    id,
    type: 'encrypt',
    sourcePath: `/mock/sample-${id}.mp4`,
    status: (opts.status ?? 'running') as TaskStatus,
    progress: 50,
    createdAt: '2026-06-18T10:00:00.000Z',
    runId: opts.runId,
    triggeredBy: opts.triggeredBy,
    // @ts-ignore 测试内部标记，运行时不影响
    __source: opts.source,
  } as EncvTask
}

// ============ 测试 ============
describe('真机 1+1+365 拆 3 个 group 端到端诊断', () => {
  let store: ReturnType<typeof useTaskStore>
  let list: ReturnType<typeof useTasksList>

  beforeEach(() => {
    testStorage.clear()
    setActivePinia(createPinia())
    store = useTaskStore()
    store.$reset()
    list = useTasksList()
  })

  afterEach(() => {
    testStorage.clear()
  })

  // ==================== S1: 3 次 submitRun（1+1+365）真实链路模拟 ====================
  it('S1: 3 次 submitRun（1+1+365 个 step）+ WS 4 件套 + fetchTasks 后的 UI 诊断', () => {
    logSection('S1: 3 次 submitRun + WS 4 件套 + fetchTasks')

    // 模拟 run-1：1 个 step
    const RUN_1 = 'run-1-uuid'
    const t1 = makeTask('t-1-0', { runId: RUN_1, triggeredBy: 'automation', status: 'running', source: 'ws-created' })
    store.appendTask(t1)
    logStoreTasks(store, 'S1-1: 提交 run-1 后')
    logGroupedTasks(list, 'S1-1: 提交 run-1 后')
    logComputedEquivalence(store, list, 'S1-1')

    // 模拟 run-2：1 个 step
    const RUN_2 = 'run-2-uuid'
    const t2 = makeTask('t-2-0', { runId: RUN_2, triggeredBy: 'automation', status: 'running', source: 'ws-created' })
    store.appendTask(t2)
    logStoreTasks(store, 'S1-2: 提交 run-2 后')
    logGroupedTasks(list, 'S1-2: 提交 run-2 后')

    // 模拟 run-3：365 个 step
    const RUN_3 = 'run-3-uuid'
    const run3Tasks: EncvTask[] = []
    for (let i = 0; i < 365; i++) {
      const t = makeTask(`t-3-${i}`, { runId: RUN_3, triggeredBy: 'automation', status: 'running', source: 'ws-created' })
      run3Tasks.push(t)
      store.appendTask(t)
    }
    logStoreTasks(store, 'S1-3: 提交 run-3 后（store 持有 367 个 task）')

    const groupsAfterAllSubmit = list.groupedTasksByRunId.value
    logGroupedTasks(list, 'S1-3: 提交 run-3 后')
    logDisplayedItems(list, 'S1-3: 提交 run-3 后')

    // 模拟 WS 4 件套：run-3 的 100 个 task 跑 update + progress
    for (let i = 0; i < 100; i++) {
      store.applyTaskUpdate({ id: `t-3-${i}`, type: 'encrypt', status: 'running' })
    }
    logGroupedTasks(list, 'S1-4: WS task:update 100 次后')
    for (let i = 0; i < 100; i++) {
      store.applyTaskProgress({ id: `t-3-${i}`, progress: 80, phase: 'encoding' })
    }
    logGroupedTasks(list, 'S1-5: WS task:progress 100 次后')

    // 模拟 fetchTasks：后端返回 367 个 task（带 runId/triggeredBy，因为后端持久化）
    const fetchTasksData: EncvTask[] = []
    fetchTasksData.push(makeTask('t-1-0', { runId: RUN_1, triggeredBy: 'automation', status: 'completed', source: 'fetch' }))
    fetchTasksData.push(makeTask('t-2-0', { runId: RUN_2, triggeredBy: 'automation', status: 'completed', source: 'fetch' }))
    for (let i = 0; i < 365; i++) {
      fetchTasksData.push(
        makeTask(`t-3-${i}`, {
          runId: RUN_3,
          triggeredBy: 'automation',
          status: i < 200 ? 'completed' : 'running',
          source: 'fetch',
        }),
      )
    }
    store.bulkSetTasks(fetchTasksData)
    logStoreTasks(store, 'S1-6: fetchTasks 后（store 持有 367 个 task 来自后端）')
    logGroupedTasks(list, 'S1-6: fetchTasks 后')
    logDisplayedItems(list, 'S1-6: fetchTasks 后')

    // 模拟 hydrate：store 从 SQLite 加载历史 task（同样带 runId/triggeredBy）
    const hydrateData: EncvTask[] = []
    hydrateData.push(makeTask('t-1-0', { runId: RUN_1, triggeredBy: 'automation', status: 'completed', source: 'hydrate' }))
    hydrateData.push(makeTask('t-2-0', { runId: RUN_2, triggeredBy: 'automation', status: 'completed', source: 'hydrate' }))
    for (let i = 0; i < 365; i++) {
      hydrateData.push(
        makeTask(`t-3-${i}`, {
          runId: RUN_3,
          triggeredBy: 'automation',
          status: 'completed',
          source: 'hydrate',
        }),
      )
    }
    store.bulkSetTasks(hydrateData)
    logStoreTasks(store, 'S1-7: hydrate 后')
    logGroupedTasks(list, 'S1-7: hydrate 后')
    logDisplayedItems(list, 'S1-7: hydrate 后')

    // 期望：3 个 group（3 个 run）
    expect(groupsAfterAllSubmit.length).toBe(3)
  })

  // ==================== S2: WS task:update 事件带 null runId ====================
  it('S2: WS task:update 事件 payload 无 runId 字段 + 模拟后端结构体序列化带 null runId', () => {
    logSection('S2: WS task:update + 模拟后端 null runId 序列化')

    // 1. 初始化 12 个 task 共享 run-A
    const RUN_A = 'run-A'
    const initialTasks: EncvTask[] = []
    for (let i = 0; i < 12; i++) {
      initialTasks.push(makeTask(`t-A-${i}`, { runId: RUN_A, triggeredBy: 'automation', status: 'running' }))
    }
    store.bulkSetTasks(initialTasks)
    logStoreTasks(store, 'S2-1: 12 个 task 共享 run-A')
    logGroupedTasks(list, 'S2-1: 12 个 task 共享 run-A')

    // 2. 模拟后端 broadcast("task:update", {id, status}) — 真机后端结构体 RunId="" 序列化为 omitempty 省略
    //    测试 S2a：update payload 完全无 runId 字段
    store.applyTaskUpdate({ id: 't-A-0', type: 'encrypt', status: 'completed', progress: 100 })
    logStoreTasks(store, 'S2-2: WS update payload 完全无 runId 字段后')
    logGroupedTasks(list, 'S2-2: WS update payload 完全无 runId 字段后')

    // 3. 模拟后端结构体 RunId="", Go omitempty 省略 → 但 patchTaskById merge 模式应该保留 runId
    //    测试 S2b：update payload 含 runId=null（理论上 Go omitempty 不会出现 null，但保险起见测试）
    store.applyTaskUpdate({ id: 't-A-1', type: 'encrypt', status: 'completed', progress: 100, runId: null as any })
    logStoreTasks(store, 'S2-3: WS update payload 含 runId=null 后')
    logGroupedTasks(list, 'S2-3: WS update payload 含 runId=null 后')

    // 4. 验证：所有 12 个 task 仍应在 1 个 group 里（merge 模式保留 prev.runId）
    const groups = list.groupedTasksByRunId.value
    expect(groups.length).toBe(1)
    expect(groups[0].runId).toBe(RUN_A)
  })

  // ==================== S3: bulkSetTasks 直接覆盖 ====================
  it('S3: store 持有 12 个 task（带 runId）+ fetchTasks 返回 12 个无 runId → bulkSetTasks 直接覆盖', () => {
    logSection('S3: bulkSetTasks 直接覆盖')

    // 1. 初始化：store 持有 12 个 task（带 runId），通过 appendTask（模拟 ws-created）
    const RUN_X = 'run-X'
    for (let i = 0; i < 12; i++) {
      const t = makeTask(`t-X-${i}`, { runId: RUN_X, triggeredBy: 'automation', status: 'running', source: 'ws-created' })
      store.appendTask(t)
    }
    logStoreTasks(store, 'S3-1: 12 个 task 带 runId')
    logGroupedTasks(list, 'S3-1: 12 个 task 带 runId')

    // 2. 模拟 fetchTasks：后端 List() 返回的 task 无 runId（极端假设：后端 SQLite runId 是空字符串）
    const fetchTasksNoRunId: EncvTask[] = []
    for (let i = 0; i < 12; i++) {
      fetchTasksNoRunId.push(makeTask(`t-X-${i}`, { status: 'running', source: 'fetch' }))
      // 不传 runId → 字段 undefined
    }
    store.bulkSetTasks(fetchTasksNoRunId)
    logStoreTasks(store, 'S3-2: fetchTasks 无 runId 后')
    logGroupedTasks(list, 'S3-2: fetchTasks 无 runId 后')
    logDisplayedItems(list, 'S3-2: fetchTasks 无 runId 后')

    // 3. 验证：12 个 task 变孤儿（每 task 一个伪 group）
    const groups = list.groupedTasksByRunId.value
    // eslint-disable-next-line no-console
    console.log(`[DIAG] S3 真因分析：bulkSetTasks 直接覆盖 → store 里 task.runId 全部变 undefined`)
    // eslint-disable-next-line no-console
    console.log(`[DIAG] S3 期望：1 个 group（如果有兜底） 或 12 个伪 group（如果没兜底）`)

    // 期望：12 个伪 group（按 `__manual__${id}` 兜底）
    expect(groups.length).toBe(12)
  })

  // ==================== S4: WS task:created 事件无 runId 字段 ====================
  it('S4: WS task:created 事件无 runId 字段（Go omitempty + 后端 runId=""）', () => {
    logSection('S4: WS task:created 无 runId 字段')

    // 模拟 12 次 WS task:created，但 payload 不含 runId/triggeredBy
    // 这是真机场景：后端 create task 时没传 runId → Go 结构体 RunId="" → JSON 省略
    for (let i = 0; i < 12; i++) {
      const t = makeTask(`t-Y-${i}`, { status: 'running', source: 'ws-created' })
      // 不传 runId/triggeredBy
      store.applyTaskCreated(t)
    }
    logStoreTasks(store, 'S4-1: 12 个 task 无 runId（WS created 事件）')
    logGroupedTasks(list, 'S4-1: 12 个 task 无 runId（WS created 事件）')
    logDisplayedItems(list, 'S4-1: 12 个 task 无 runId（WS created 事件）')

    // 验证：12 个伪 group（按 `__manual__${id}` 兜底）
    const groups = list.groupedTasksByRunId.value
    expect(groups.length).toBe(12)
  })

  // ==================== S5: module-level singleton + Pinia $reset ====================
  it('S5: useTasksList module-level singleton + Pinia $reset 跨测试 isolation', () => {
    logSection('S5: useTasksList singleton isolation')

    // 1. test 阶段 1：创建 store-A + useTasksList
    const RUN_OLD = 'run-OLD'
    for (let i = 0; i < 5; i++) {
      const t = makeTask(`t-old-${i}`, { runId: RUN_OLD, triggeredBy: 'automation', status: 'running' })
      store.appendTask(t)
    }
    const listA = useTasksList()
    logGroupedTasks(listA, 'S5-1: test1 阶段（store-A + list-A 持有 5 个 task）')
    logComputedEquivalence(store, listA, 'S5-1')

    // 2. test 阶段 2：setActivePinia(createPinia()) + store.$reset() + 重新创建 store
    //    模拟"用户切换 tab / 重启 / $reset 场景"
    setActivePinia(createPinia())
    const storeB = useTaskStore()
    logStoreTasks(storeB, 'S5-2: $reset 后 store-B 状态')

    // 3. listA 是 module-level singleton 复用的旧实例
    //    storeB 是新 store
    //    storeB.groupedTasksByRunId.length = 0（空）
    logGroupedTasks(listA, 'S5-3: list-A（module-level singleton 持有旧 store 引用？）')
    logComputedEquivalence(storeB, listA, 'S5-3')

    // eslint-disable-next-line no-console
    console.log(`[DIAG] S5 关键诊断：list.groupedTasksByRunId === store.groupedTasksByRunId →`,
      // @ts-ignore
      storeB.groupedTasksByRunId === listA.groupedTasksByRunId.value,
    )
    // eslint-disable-next-line no-console
    console.log(`[DIAG] S5 关键诊断：list.tasks === store.tasks →`,
      // @ts-ignore
      storeB.tasks === listA.tasks,
    )

    // 4. 如果 module-level singleton 复用了旧实例：listA 仍持有 store-A 的 ref
    //    → listA.groupedTasksByRunId 显示 1 个 group（5 个 task），但 storeB 是空的
    //    → 视图显示旧数据，但 Pinia 状态已重置
    const listB = useTasksList()
    logGroupedTasks(listB, 'S5-4: list-B（重新调 useTasksList）')
  })

  // ==================== S6: 真机 1+1+365 拆 3 个 group 的"逃逸"模拟 ====================
  it('S6: 真机"逃逸"模拟 — 1 个 run 提交 367 个 task，但 fetchTasks 时部分 task runId 变 undefined', () => {
    logSection('S6: 真机"逃逸"模拟')

    // 模拟真机：
    // 1. 提交阶段：1+1+365 个 task，每个 task 带正确 runId
    const RUNS = ['run-1-uuid', 'run-2-uuid', 'run-3-uuid']
    const SUBMIT_COUNTS = [1, 1, 365]
    let tIdx = 0
    for (let r = 0; r < RUNS.length; r++) {
      for (let i = 0; i < SUBMIT_COUNTS[r]; i++) {
        const t = makeTask(`t-${tIdx++}`, { runId: RUNS[r], triggeredBy: 'automation', status: 'running', source: 'ws-created' })
        store.appendTask(t)
      }
    }
    logStoreTasks(store, 'S6-1: 提交 367 个 task 后')
    logGroupedTasks(list, 'S6-1: 提交 367 个 task 后')

    // 2. 模拟真机 fetchTasks（部分 task 丢 runId — 这是核心怀疑场景）
    //    假设：run-3 的 365 个 task 中有 100 个 task 在 fetchTasks 时 runId 变 undefined
    //    原因候选：后端 SQLite 部分 row 的 runId 字段是空字符串（早期版本没存）
    const fetchTasksData: EncvTask[] = []
    let tIdx2 = 0
    for (let r = 0; r < RUNS.length; r++) {
      for (let i = 0; i < SUBMIT_COUNTS[r]; i++) {
        const isRun3Drop = r === 2 && i < 100  // run-3 的前 100 个 task 模拟丢 runId
        const t = makeTask(`t-${tIdx2++}`, {
          runId: isRun3Drop ? undefined : RUNS[r],
          triggeredBy: 'automation',
          status: 'completed',
          source: 'fetch',
        })
        fetchTasksData.push(t)
      }
    }
    store.bulkSetTasks(fetchTasksData)
    logStoreTasks(store, 'S6-2: fetchTasks 后（run-3 前 100 个 task 丢 runId）')
    logGroupedTasks(list, 'S6-2: fetchTasks 后')
    logDisplayedItems(list, 'S6-2: fetchTasks 后')

    // eslint-disable-next-line no-console
    console.log(`[DIAG] S6 真因候选：fetchTasks 时部分 task 丢 runId → bulkSetTasks 直接覆盖`)
    // eslint-disable-next-line no-console
    console.log(`[DIAG] S6 → 100 个孤儿 task（run-3 拆 1 + 100）`)
    // eslint-disable-next-line no-console
    console.log(`[DIAG] S6 期望：1+1+1+1+100 = 104 个 group（1 个 run-1 + 1 个 run-2 + 2 个 run-3 group + 100 个伪 group）`)
  })
})
