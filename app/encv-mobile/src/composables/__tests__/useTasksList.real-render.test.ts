/**
 * 真机"任务逃逸"诊断：真实组件渲染 + 详细 UI DOM 诊断
 *
 * 核心诊断方向（user 反馈 2026-06-22）：
 * - 1+1+365 不是固定拆分（动态变化：1+1+1+555, 1+1+1+1+1+888 等）
 * - 数量不稳定 → 不是固定数据问题
 * - 重启后逃逸消失 → 怀疑 module-level singleton 状态在 session 间累积
 * - 逃逸 = 渲染问题（不是 store 数据问题）
 *
 * 测试策略：
 * 1. mount 真实 mini TaskListDiag 组件（用 useTasksList + store）
 * 2. 注入各种 task 数据 + 触发各种 session 操作
 * 3. 输出 DOM 实际渲染的 group + row + 每个 group 的内容
 * 4. 覆盖"重启后逃逸消失"：session 1 跑出逃逸 → 重启 → escape 消失
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'

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
import { useTasksList, _resetTasksListSingletonForTests } from '@/composables/useTasksList'
import { __resetServiceForTests } from '@/composables/useWorkflowTaskService'
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
// stub i18n 避免依赖 vue-i18n 复杂 setup
vi.mock('@/composables/useI18n', () => ({
  useI18n: () => ({ t: (_k: string, opts?: any) => opts?.defaultValue ?? _k }),
}))
// stub workflowService（不需要真实逻辑）
vi.mock('@/composables/useWorkflowTaskService', () => ({
  useWorkflowTaskService: () => ({
    currentRun: { value: null },
    isRunning: { value: false },
    currentDef: { value: null },
    submitRun: vi.fn().mockResolvedValue({ id: 'mock-run', jobs: [] }),
    cancelRun: vi.fn(),
    listRuns: () => [],
    getRun: () => undefined,
  }),
  __resetServiceForTests: () => {},
}))
// 避免 useDateFormat 真实依赖
vi.mock('@/composables/useDateFormat', () => ({
  formatDateTime: (d: string) => d,
}))

// ============ 真实 mini 组件：直接用 useTasksList + store 渲染 displayedItems ============
const TaskListDiag = defineComponent({
  name: 'TaskListDiag',
  setup() {
    const list = useTasksList()
    return { list, store: useTaskStore() }
  },
  template: `
    <div class="diag-root">
      <div class="diag-counts" data-testid="counts">
        <span data-testid="store-count">{{ store.tasks.length }}</span>
        <span data-testid="group-count">{{ list.groupedTasksByRunId.value.length }}</span>
        <span data-testid="item-count">{{ list.displayedItems.value.length }}</span>
      </div>
      <ul class="diag-list">
        <li
          v-for="it in list.displayedItems.value"
          :key="it.key"
          :data-testid="'item-' + it.kind"
          :data-run-id="it.kind === 'group' ? it.runId : (it.kind === 'task' ? (it.task.runId || '__UNDEFINED__') : 'date')"
        >
          <template v-if="it.kind === 'date'">
            <span data-testid="date-label">{{ it.label }}</span>
          </template>
          <template v-else-if="it.kind === 'group'">
            <span data-testid="group-runid">{{ it.runId }}</span>
            <span data-testid="group-size">×{{ it.tasks.length }}</span>
            <span data-testid="group-taskids">[{{ it.tasks.map(t => t.id).join(',') }}]</span>
          </template>
          <template v-else-if="it.kind === 'task'">
            <span data-testid="row-id">{{ it.task.id }}</span>
            <span data-testid="row-runid">runId={{ it.task.runId || 'UNDEFINED' }}</span>
          </template>
        </li>
      </ul>
    </div>
  `,
})

// ============ 诊断输出工具 ============
function logSection(title: string): void {
  // eslint-disable-next-line no-console
  console.log(`\n${'='.repeat(80)}\n[DOM-DIAG] ${title}\n${'='.repeat(80)}`)
}

function logDomDump(wrapper: VueWrapper, label: string): void {
  const counts = wrapper.find('[data-testid="counts"]').text()
  const items = wrapper.findAll('[data-testid^="item-"]')
  // eslint-disable-next-line no-console
  console.log(`[DOM-DIAG] ${label}: counts=[${counts.replace(/\s+/g, ' ').trim()}]`)
  // eslint-disable-next-line no-console
  console.log(`[DOM-DIAG] ${label}: DOM items count=${items.length}`)
  for (const it of items) {
    const kind = it.attributes('data-testid')?.replace('item-', '')
    const runId = it.attributes('data-run-id') ?? '?'
    let detail = ''
    if (kind === 'group') {
      const runidEl = it.find('[data-testid="group-runid"]')
      const sizeEl = it.find('[data-testid="group-size"]')
      const idsEl = it.find('[data-testid="group-taskids"]')
      detail = `runId=${runidEl.text()} size=${sizeEl.text()} ids=${idsEl.text()}`
    } else if (kind === 'task') {
      const idEl = it.find('[data-testid="row-id"]')
      const runidEl = it.find('[data-testid="row-runid"]')
      detail = `id=${idEl.text()} ${runidEl.text()}`
    } else if (kind === 'date') {
      const labelEl = it.find('[data-testid="date-label"]')
      detail = `label=${labelEl.text()}`
    }
    // eslint-disable-next-line no-console
    console.log(`  - [${kind}] data-run-id=${runId} ${detail}`)
  }
}

function logStoreSnapshot(store: ReturnType<typeof useTaskStore>, label: string): void {
  const tasks = store.tasks as unknown as EncvTask[]
  // eslint-disable-next-line no-console
  console.log(`[DOM-DIAG] ${label}: store.tasks.length=${tasks.length}`)
  // 按 runId 分组统计
  const byRun = new Map<string, number>()
  let undefinedRun = 0
  for (const tk of tasks) {
    const k = tk.runId ?? '__UNDEFINED__'
    byRun.set(k, (byRun.get(k) ?? 0) + 1)
    if (!tk.runId) undefinedRun++
  }
  for (const [k, n] of byRun.entries()) {
    // eslint-disable-next-line no-console
    console.log(`  - runId=${JSON.stringify(k)} taskCount=${n}`)
  }
  // eslint-disable-next-line no-console
  console.log(`[DOM-DIAG] ${label}: undefined-runId taskCount=${undefinedRun}`)
}

// ============ Task 工厂 ============
function makeTask(
  id: string,
  opts: {
    runId?: string
    triggeredBy?: 'user' | 'automation' | 'ai_agent'
    status?: TaskStatus
    createdAt?: string
  } = {},
): EncvTask {
  return {
    id,
    type: 'encrypt',
    sourcePath: `/mock/sample-${id}.mp4`,
    status: (opts.status ?? 'running') as TaskStatus,
    progress: 50,
    createdAt: opts.createdAt ?? '2026-06-18T10:00:00.000Z',
    runId: opts.runId,
    triggeredBy: opts.triggeredBy,
  } as EncvTask
}

// ============ 测试 ============
describe('真机"逃逸"真实组件渲染诊断', () => {
  let store: ReturnType<typeof useTaskStore>
  let wrapper: VueWrapper

  beforeEach(() => {
    testStorage.clear()
    setActivePinia(createPinia())
    store = useTaskStore()
    store.$reset()
  })

  afterEach(() => {
    wrapper?.unmount()
    _resetTasksListSingletonForTests()
    __resetServiceForTests()
    testStorage.clear()
  })

  // ==================== D1: 基础渲染 — 3 个 group + 期望 1+1+365 ====================
  it('D1: 真实组件渲染 — 3 个 submitRun (1+1+365) 应该显示 3 个 group card', async () => {
    logSection('D1: 3 个 submitRun (1+1+365) 真实组件渲染')

    // 1. 注入数据
    const RUNS = ['run-1', 'run-2', 'run-3']
    const COUNTS = [1, 1, 365]
    const initialTasks: EncvTask[] = []
    let idx = 0
    for (let r = 0; r < RUNS.length; r++) {
      for (let i = 0; i < COUNTS[r]; i++) {
        initialTasks.push(
          makeTask(`t-${idx++}`, {
            runId: RUNS[r],
            triggeredBy: 'automation',
            status: i < COUNTS[r] / 2 ? 'completed' : 'running',
          }),
        )
      }
    }
    store.bulkSetTasks(initialTasks)
    logStoreSnapshot(store, 'D1-1: 注入 367 个 task 后')

    // 2. mount 真实组件
    wrapper = mount(TaskListDiag)
    await nextTick()
    logDomDump(wrapper, 'D1-2: mount 后 DOM 实际渲染')

    // 3. 期望：3 个 group card (run-1:1, run-2:1, run-3:365)
    const groupItems = wrapper.findAll('[data-testid="item-group"]')
    expect(groupItems.length).toBe(3)
  })

  // ==================== D2: "重启后逃逸消失" — 核心诊断场景 ====================
  it('D2: 重启后逃逸消失 — session 1 跑出逃逸 → 重启 → escape 消失', async () => {
    logSection('D2: 重启后逃逸消失')

    // ===== session 1：跑出"逃逸" =====
    // 1.1 注入 367 个 task（3 个 run, 1+1+365）
    const RUNS = ['run-1', 'run-2', 'run-3']
    const COUNTS = [1, 1, 365]
    const session1Tasks: EncvTask[] = []
    let idx = 0
    for (let r = 0; r < RUNS.length; r++) {
      for (let i = 0; i < COUNTS[r]; i++) {
        session1Tasks.push(
          makeTask(`s1-t-${idx++}`, {
            runId: RUNS[r],
            triggeredBy: 'automation',
            status: 'completed',
          }),
        )
      }
    }
    store.bulkSetTasks(session1Tasks)
    logStoreSnapshot(store, 'D2-1: session 1 注入 367 个 task')

    wrapper = mount(TaskListDiag)
    await nextTick()
    logDomDump(wrapper, 'D2-2: session 1 mount 后 DOM')

    const session1GroupCount = wrapper.findAll('[data-testid="item-group"]').length
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D2-2: session 1 group card count=${session1GroupCount}`)

    // 1.2 模拟 session 1 内的 mutation（fetchTasks + WS 事件）
    for (let i = 0; i < 50; i++) {
      store.applyTaskProgress({ id: `s1-t-${i}`, progress: 80, phase: 'encoding' })
    }
    await nextTick()
    logDomDump(wrapper, 'D2-3: session 1 + WS progress 50 次后 DOM')

    // 1.3 模拟 fetchTasks：后端返回同样数据（merge 模式应保持 runId）
    const fetchTasks1: EncvTask[] = session1Tasks.map((t) => ({
      ...t,
      // 不传 runId 模拟"后端 SQLite 部分 row 的 runId 字段是空字符串"
      ...(Math.random() < 0.1 ? { runId: undefined } : {}),
    })) as EncvTask[]
    store.bulkSetTasks(fetchTasks1)
    await nextTick()
    logDomDump(wrapper, 'D2-4: session 1 + fetchTasks 10% 丢 runId 后 DOM')
    logStoreSnapshot(store, 'D2-4: store 实际状态')

    const session1FinalGroupCount = wrapper.findAll('[data-testid="item-group"]').length
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D2 关键诊断：session 1 注入 367 task，bulkSetTasks 后 DOM group count=${session1FinalGroupCount}`)

    wrapper.unmount()
    _resetTasksListSingletonForTests()
    __resetServiceForTests()

    // ===== session 2：模拟"重启" =====
    // 2.1 销毁旧 Pinia + store + useTasksList singleton
    setActivePinia(createPinia())
    const store2 = useTaskStore()
    store2.$reset()

    // 2.2 hydrate：从 SQLite 加载 367 个 task（带 runId）
    const hydrateTasks: EncvTask[] = []
    idx = 0
    for (let r = 0; r < RUNS.length; r++) {
      for (let i = 0; i < COUNTS[r]; i++) {
        hydrateTasks.push(
          makeTask(`s1-t-${idx++}`, {
            runId: RUNS[r],
            triggeredBy: 'automation',
            status: 'completed',
          }),
        )
      }
    }
    store2.bulkSetTasks(hydrateTasks)
    logStoreSnapshot(store2, 'D2-5: session 2 hydrate 367 个 task')

    // 2.3 mount 新组件
    const wrapper2 = mount(TaskListDiag)
    await nextTick()
    logDomDump(wrapper2, 'D2-6: session 2 mount 后 DOM')

    const session2GroupCount = wrapper2.findAll('[data-testid="item-group"]').length
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D2 关键诊断：session 2 hydrate 后 DOM group count=${session2GroupCount}`)

    // 诊断：session 1 vs session 2 group 数量对比
    // eslint-disable-next-line no-console
    console.log(`\n[DOM-DIAG] D2 关键诊断结论：`)
    // eslint-disable-next-line no-console
    console.log(`  - session 1 跑出逃逸：DOM group count=${session1FinalGroupCount}`)
    // eslint-disable-next-line no-console
    console.log(`  - session 2 重启后：DOM group count=${session2GroupCount}`)
    // eslint-disable-next-line no-console
    console.log(`  - 如果 session1 !== session2 → 真因在 session 状态累积（useTasksList singleton 跨 session）`)
    // eslint-disable-next-line no-console
    console.log(`  - 如果 session1 === session2 → 真因不在 singleton，需要更深挖`)

    wrapper2.unmount()
  })

  // ==================== D3: 跨 session singleton 复用 ====================
  it('D3: 跨 session singleton 复用诊断 — list 持有旧 store 引用', async () => {
    logSection('D3: 跨 session singleton 复用诊断')

    // session A: 注入 5 个 task + mount
    const tasksA: EncvTask[] = []
    for (let i = 0; i < 5; i++) {
      tasksA.push(makeTask(`A-t-${i}`, { runId: 'A-run', triggeredBy: 'automation', status: 'running' }))
    }
    store.bulkSetTasks(tasksA)

    wrapper = mount(TaskListDiag)
    await nextTick()
    logDomDump(wrapper, 'D3-1: session A mount 后')

    // session A 的 store 和 list
    const listA = (wrapper.vm as any).list
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D3-1: listA === useTasksList() result? (singleton cached)`,
      listA === (await import('@/composables/useTasksList')).useTasksList(),
    )

    // 模拟用户操作：多次 group/flat 切换 + 滚动
    for (let i = 0; i < 3; i++) {
      store.toggleViewMode()
      await nextTick()
      store.toggleViewMode()
      await nextTick()
    }
    logDomDump(wrapper, 'D3-2: 切 6 次 view mode 后')

    wrapper.unmount()
    _resetTasksListSingletonForTests()
    __resetServiceForTests()

    // session B: 模拟"重启"（setActivePinia + 重新创建 store）
    setActivePinia(createPinia())
    const storeB = useTaskStore()
    storeB.$reset()

    // 注入新数据（runId 不同）
    const tasksB: EncvTask[] = []
    for (let i = 0; i < 3; i++) {
      tasksB.push(makeTask(`B-t-${i}`, { runId: 'B-run', triggeredBy: 'user', status: 'running' }))
    }
    storeB.bulkSetTasks(tasksB)

    const wrapperB = mount(TaskListDiag)
    await nextTick()
    logDomDump(wrapperB, 'D3-3: session B mount 后')

    const listB = (wrapperB.vm as any).list
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D3-3: listB === listA? (singleton 跨 session 复用？)`, listB === listA)
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D3-3: listB.groupedTasksByRunId === listA.groupedTasksByRunId?`,
      listB.groupedTasksByRunId.value === listA.groupedTasksByRunId.value,
    )
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D3-3: listA.groupedTasksByRunId.value.length =`, listA.groupedTasksByRunId.value.length)
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D3-3: listB.groupedTasksByRunId.value.length =`, listB.groupedTasksByRunId.value.length)

    wrapperB.unmount()
  })

  // ==================== D4: view mode 切换是否导致逃逸 ====================
  it('D4: 频繁切换 view mode (group ↔ flat) 是否导致逃逸', async () => {
    logSection('D4: 频繁切换 view mode')

    // 注入 12 个 task 共享 run-X
    const tasks: EncvTask[] = []
    for (let i = 0; i < 12; i++) {
      tasks.push(makeTask(`t-${i}`, { runId: 'run-X', triggeredBy: 'automation', status: 'running' }))
    }
    store.bulkSetTasks(tasks)

    wrapper = mount(TaskListDiag)
    await nextTick()
    logDomDump(wrapper, 'D4-1: mount 后')

    // 频繁切换
    for (let i = 0; i < 10; i++) {
      store.toggleViewMode()
      await nextTick()
    }
    logDomDump(wrapper, 'D4-2: 切 10 次 view mode 后')

    // 模拟 WS 事件
    for (let i = 0; i < 12; i++) {
      store.applyTaskUpdate({ id: `t-${i}`, type: 'encrypt', status: 'completed', progress: 100 })
    }
    await nextTick()
    logDomDump(wrapper, 'D4-3: + WS update 12 次后')

    const finalGroupCount = wrapper.findAll('[data-testid="item-group"]').length
    // eslint-disable-next-line no-console
    console.log(`[DOM-DIAG] D4 关键诊断：12 个 task 共享 run-X，频繁切 view mode + WS update 后 DOM group count=${finalGroupCount}`)
    // 期望：1 个 group（因为 12 个 task 共享 runId）
  })
})
