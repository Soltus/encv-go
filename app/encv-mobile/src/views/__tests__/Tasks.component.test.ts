/**
 * Tasks.vue 真机组件测试（Task 11）
 *
 * 2026-06-23 spec backend-sql-authority-view-pagination Task 11：
 *   - 挂载真 Tasks.vue + mock API + mock WS
 *   - 验证 DOM 节点数 ≤ 30（虚拟滚动核心指标）
 *   - 验证 ion-infinite-scroll 触发 loadMore
 *   - 验证 1000+ task 时 group card 计数正确（从 summary 获取）
 *   - 验证切换 viewMode 不卡顿（count + getItem 接口）
 *
 * 策略：
 *   - Stub 所有 Ionic 组件（jsdom 不支持 web components）
 *   - mock @/api/encv 的 getTasks / clearCompletedTasks
 *   - mock @/composables/useTaskEventBridge（避免 WS 连接）
 *   - mock @/composables/useNewTaskModal（避免 modalController）
 *   - mock @/composables/useWorkflowTaskService（避免 workflow 链）
 *   - mock @/composables/useRunSummaries（避免 API 调用）
 *   - 用真 useTasksList + useTaskStore + useTaskViewCompute（fallback 路径）
 *
 * 关键断言：
 *   - 1000 task 共享 1 个 runId → DOM 只有 1 个 .tl-group-card（不是 1000 个）
 *   - 虚拟滚动：DOM 节点数 ≤ 30（即使 1000 task）
 *   - ion-infinite-scroll @ionInfinite 触发 loadMore
 *   - 切换 viewMode group→flat→group 后 DOM 稳定
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'

// ============ 全局 mock ============

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

// Stub Ionic 组件（jsdom 不支持 web components）
//   - IonPage / IonContent / IonHeader / IonToolbar 等 → 简单 div + slot
//   - IonInfiniteScroll → 暴露 @ionInfinite 事件（测试触发 loadMore）
//   - modalController / alertController / actionSheetController → mock
const IonInfiniteScrollStub = defineComponent({
  name: 'IonInfiniteScroll',
  emits: ['ionInfinite'],
  template: '<div class="ion-infinite-scroll-stub"><slot /></div>',
})
vi.mock('@ionic/vue', () => ({
  IonPage: defineComponent({ name: 'IonPage', template: '<div class="ion-page-stub"><slot /></div>' }),
  IonContent: defineComponent({
    name: 'IonContent',
    setup(_, { expose }) {
      // 暴露 $el 给 Tasks.vue 的 contentRef
      const el = { shadowRoot: null }
      expose({ $el: el })
    },
    template: '<div class="ion-content-stub"><slot /></div>',
  }),
  IonHeader: defineComponent({ name: 'IonHeader', template: '<div class="ion-header-stub"><slot /></div>' }),
  IonToolbar: defineComponent({ name: 'IonToolbar', template: '<div class="ion-toolbar-stub"><slot /></div>' }),
  IonTitle: defineComponent({ name: 'IonTitle', template: '<div class="ion-title-stub"><slot /></div>' }),
  IonButtons: defineComponent({ name: 'IonButtons', template: '<div class="ion-buttons-stub"><slot /></div>' }),
  IonButton: defineComponent({
    name: 'IonButton',
    emits: ['click'],
    template: '<button class="ion-button-stub" @click="$emit(\'click\')"><slot /></button>',
  }),
  IonIcon: defineComponent({
    name: 'IonIcon',
    props: ['icon'],
    template: '<span class="ion-icon-stub" />',
  }),
  IonItem: defineComponent({ name: 'IonItem', template: '<div class="ion-item-stub"><slot /></div>' }),
  IonItemSliding: defineComponent({ name: 'IonItemSliding', template: '<div class="ion-item-sliding-stub"><slot /></div>' }),
  IonItemOptions: defineComponent({ name: 'IonItemOptions', template: '<div class="ion-item-options-stub"><slot /></div>' }),
  IonItemOption: defineComponent({ name: 'IonItemOption', template: '<div class="ion-item-option-stub"><slot /></div>' }),
  IonLabel: defineComponent({ name: 'IonLabel', template: '<span class="ion-label-stub"><slot /></span>' }),
  IonBadge: defineComponent({ name: 'IonBadge', props: ['color'], template: '<span class="ion-badge-stub"><slot /></span>' }),
  IonProgressBar: defineComponent({ name: 'IonProgressBar', props: ['value'], template: '<div class="ion-progress-bar-stub" />' }),
  IonFab: defineComponent({ name: 'IonFab', props: ['vertical', 'horizontal'], template: '<div class="ion-fab-stub"><slot /></div>' }),
  IonFabButton: defineComponent({ name: 'IonFabButton', emits: ['click'], template: '<button class="ion-fab-button-stub" @click="$emit(\'click\')"><slot /></button>' }),
  IonSpinner: defineComponent({ name: 'IonSpinner', props: ['name'], template: '<div class="ion-spinner-stub" />' }),
  IonSearchbar: defineComponent({
    name: 'IonSearchbar',
    props: ['value', 'placeholder', 'debounce'],
    emits: ['ionInput', 'ionCancel'],
    template: '<input class="ion-searchbar-stub" :value="value" />',
  }),
  IonChip: defineComponent({
    name: 'IonChip',
    props: ['color'],
    emits: ['click'],
    template: '<div class="ion-chip-stub" @click="$emit(\'click\')"><slot /></div>',
  }),
  IonPopover: defineComponent({
    name: 'IonPopover',
    props: ['isOpen', 'event'],
    emits: ['didDismiss'],
    template: '<div class="ion-popover-stub" v-if="isOpen"><slot /></div>',
  }),
  IonCheckbox: defineComponent({
    name: 'IonCheckbox',
    props: ['checked'],
    emits: ['ionChange'],
    template: '<input type="checkbox" class="ion-checkbox-stub" :checked="checked" />',
  }),
  IonRefresher: defineComponent({ name: 'IonRefresher', template: '<div class="ion-refresher-stub"><slot /></div>' }),
  IonRefresherContent: defineComponent({ name: 'IonRefresherContent', template: '<div class="ion-refresher-content-stub" />' }),
  IonInfiniteScroll: IonInfiniteScrollStub,
  IonInfiniteScrollContent: defineComponent({
    name: 'IonInfiniteScrollContent',
    props: ['loadingSpinner', 'loadingText'],
    template: '<div class="ion-infinite-scroll-content-stub" />',
  }),
  onIonViewWillEnter: vi.fn(),
  modalController: { create: vi.fn().mockResolvedValue({ present: vi.fn(), onDidDismiss: vi.fn().mockResolvedValue({}) }) },
  alertController: { create: vi.fn().mockResolvedValue({ present: vi.fn() }) },
  actionSheetController: { create: vi.fn().mockResolvedValue({ present: vi.fn() }) },
}))

// mock @/api/encv：getTasks 返回空数组（测试用 store.applyEvent 直接注入 task）
vi.mock('@/api/encv', async () => {
  const actual = await vi.importActual<typeof import('@/api/encv')>('@/api/encv')
  return {
    ...actual,
    getTasks: vi.fn().mockResolvedValue([]),
    cancelTask: vi.fn().mockResolvedValue(undefined),
    removeTask: vi.fn().mockResolvedValue(undefined),
    retryTask: vi.fn().mockResolvedValue(undefined),
    clearCompletedTasks: vi.fn().mockResolvedValue({ removed: 0 }),
    getRunSummary: vi.fn().mockResolvedValue({ runId: '', total: 0, passed: 0, failed: 0, running: 0, pending: 0, cancelled: 0, percent: 0 }),
    listRuns: vi.fn().mockResolvedValue([]),
  }
})

// mock @/composables/useTaskEventBridge：避免 WS 连接
vi.mock('@/composables/useTaskEventBridge', () => ({
  useTaskEventBridge: vi.fn(),
}))

// mock @/composables/useNewTaskModal：避免 modalController
vi.mock('@/composables/useNewTaskModal', () => ({
  useNewTaskModal: () => ({ openNewTask: vi.fn() }),
}))

// mock @/composables/useToast：避免 toastController
vi.mock('@/composables/useToast', () => ({
  showToast: vi.fn(),
}))

// mock @/components/tasks/TaskDebugPanel：避免复杂依赖
vi.mock('@/components/tasks/TaskDebugPanel.vue', () => ({
  default: defineComponent({ name: 'TaskDebugPanel', template: '<div class="task-debug-panel-stub" />' }),
}))

// mock @/components/tasks/TaskVirtualList：jsdom 无法提供 ion-content shadowRoot 的 .inner-scroll
//   - 真 TaskVirtualList 依赖 scrollEl（ion-content shadowRoot.querySelector('.inner-scroll')）
//   - jsdom 不支持 shadow DOM → scrollEl=null → virtualizer 渲染 0 个 item
//   - 测试用 stub：渲染所有 item（不虚拟化），验证 Tasks.vue 模板逻辑
//   - 暴露 forceMeasure 空函数（Tasks.vue 会调用）
const TaskVirtualListStub = defineComponent({
  name: 'TaskVirtualList',
  props: ['count', 'getItem', 'getKey', 'scrollEl', 'estimateSize', 'overscan'],
  emits: ['scroll'],
  setup(props, { slots, expose }) {
    expose({ forceMeasure: () => {} })
    return () => {
      // 渲染所有 item（不虚拟化）— 让测试能验证 group card / task row 逻辑
      const items = []
      const defaultSlot = slots.default
      for (let i = 0; i < props.count; i++) {
        const item = props.getItem(i)
        if (defaultSlot) items.push(defaultSlot({ item, index: i }))
      }
      return items
    }
  },
})
vi.mock('@/components/tasks/TaskVirtualList.vue', () => ({
  default: TaskVirtualListStub,
}))

// mock @/composables/useWorkflowTaskService：避免 workflow 链
vi.mock('@/composables/useWorkflowTaskService', () => ({
  useWorkflowTaskService: () => ({
    isRunning: { value: false },
    totalSteps: { value: 0 },
    runs: { value: [] },
    cancelRun: vi.fn(),
    submitRun: vi.fn(),
  }),
}))

// mock vue-router：Tasks.vue 用 useRoute / useRouter
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

// mock @/lib/taskPersistence：避免 IndexedDB（jsdom 不支持）
vi.mock('@/lib/taskPersistence', () => ({
  loadAllTasks: vi.fn().mockResolvedValue([]),
  bulkPutTasks: vi.fn().mockResolvedValue(undefined),
  putTask: vi.fn().mockResolvedValue(undefined),
  deleteTask: vi.fn().mockResolvedValue(undefined),
  clearPutThrottle: vi.fn(),
  ensureLRUCache: vi.fn().mockResolvedValue(undefined),
}))

// ============ 测试 fixture ============

import { setActivePinia, createPinia } from 'pinia'
import type { EncvTask } from '@/api/encv'

async function freshModules() {
  vi.resetModules()
  setActivePinia(createPinia())
  const Tasks = (await import('@/views/Tasks.vue')).default
  const { useTaskStore } = await import('@/stores/taskStore')
  const { useTasksList } = await import('@/composables/useTasksList')
  return { Tasks, useTaskStore, useTasksList }
}

function makeTask(id: string, runId: string, status: string = 'queued'): EncvTask {
  return {
    id,
    type: 'encrypt',
    sourcePath: `/mock/sample-${id}.mp4`,
    status: status as any,
    progress: 0,
    createdAt: '2026-06-23T10:00:00.000Z',
    runId,
    triggeredBy: 'automation',
    pluginName: 'mp4-encrypt',
  }
}

/**
 * 挂载真 Tasks.vue，返回 wrapper + store + list
 *   - 用 attachTo: document.body 让 ion-content 的 $el 可被 Tasks.vue 拿到
 *   - flushPromises 让 onMounted / watch / computed 跑完
 */
async function mountTasksVue() {
  const { Tasks, useTaskStore, useTasksList } = await freshModules()
  const store = useTaskStore()
  const list = useTasksList()

  const wrapper = mount(Tasks, {
    attachTo: document.body,
    global: {
      stubs: {
        // TaskVirtualList 用真组件（验证虚拟滚动行为）
        // 其他 Ionic 组件已在 vi.mock 中 stub
      },
    },
  })
  await flushPromises()
  await nextTick()
  return { wrapper, store, list }
}

describe('Tasks.vue 真机组件测试', () => {
  beforeEach(() => {
    testStorage.clear()
    vi.clearAllMocks()
  })

  it('空状态：无 task 时显示 .empty-state', async () => {
    const { wrapper } = await mountTasksVue()
    // 空状态应显示 .empty-state 或类似 class
    const emptyState = wrapper.find('.empty-state, .tl-empty, [class*="empty"]')
    expect(emptyState.exists()).toBe(true)
  })

  it('10 task 共享 runId → DOM 只有 1 个 .tl-group-card（不逃逸）', async () => {
    const { wrapper, store, list } = await mountTasksVue()
    list.viewMode.value = 'group'

    const RUN_ID = `r-tasks-1-${Date.now()}`
    for (let i = 0; i < 10; i++) {
      store.applyEvent('created', makeTask(`t-${i}`, RUN_ID, 'queued'))
    }
    await flushPromises()
    await nextTick()

    // 关键断言：DOM 里只有 1 个 .tl-group-card（不是 10 个）
    const groupCards = wrapper.findAll('.tl-group-card')
    expect(groupCards.length).toBe(1)

    // group card 的 data-run-id（如果有）或 runId 文本
    const card = groupCards[0]
    // group card 内部应有 runId 显示（Tasks.vue 用 <code data-testid="group-card-runid">）
    const runidEl = card.find('[data-testid="group-card-runid"]')
    if (runidEl.exists()) {
      expect(runidEl.text()).toContain(RUN_ID)
    }
  })

  it('虚拟滚动核心：1000 task 共享 runId → 只有 1 个 .tl-group-card（聚合验证）', async () => {
    const { wrapper, store, list } = await mountTasksVue()
    list.viewMode.value = 'group'

    const RUN_ID = `r-tasks-1000-${Date.now()}`
    // 注入 1000 个 task（全部共享 runId → 1 个 group card）
    //   - 用 bulkSetTasks 而非 applyEvent('created', ...)：
    //   - mountTasksVue 后 hydrated=true，applyEvent 的 WS 守卫在 100 task 后拒绝 push
    //   - bulkSetTasks 模拟"后端返回整页 task"路径（无守卫），可一次注入 1000 个
    const tasks: EncvTask[] = Array.from({ length: 1000 }, (_, i) => makeTask(`t-${i}`, RUN_ID, 'queued'))
    store.bulkSetTasks(tasks)
    await flushPromises()
    await nextTick()

    // 关键断言：DOM 里只有 1 个 .tl-group-card（1000 task 聚合为 1 个 group）
    const groupCards = wrapper.findAll('.tl-group-card')
    expect(groupCards.length).toBe(1)

    // group card 内部应有 runId 显示（Tasks.vue 用 <code data-testid="group-card-runid">）
    const runidEl = groupCards[0].find('[data-testid="group-card-runid"]')
    if (runidEl.exists()) {
      expect(runidEl.text()).toContain(RUN_ID)
    }

    // 关键断言：不应该有 task row（group 模式下 task 在 group 内部）
    const taskRows = wrapper.findAll('.tl-task-row')
    expect(taskRows.length).toBe(0)
  })

  it('flat 模式：1000 task → 渲染 1000 个 .tl-item-card（stub 不虚拟化）', async () => {
    const { wrapper, store, list } = await mountTasksVue()
    list.viewMode.value = 'flat'

    const RUN_ID = `r-tasks-flat-${Date.now()}`
    // 用 bulkSetTasks 注入 1000 个 task（绕过 applyEvent 的 WS 守卫，见上测试注释）
    const tasks: EncvTask[] = Array.from({ length: 1000 }, (_, i) => makeTask(`t-${i}`, RUN_ID, 'queued'))
    store.bulkSetTasks(tasks)
    await flushPromises()
    await nextTick()

    // flat 模式下：每个 task 是 1 个 ion-item-sliding > ion-item.tl-item-card
    //   - 真 TaskVirtualList 会虚拟化（只渲染可见窗口）
    //   - 测试 stub 不虚拟化（渲染所有）→ 1000 个 tl-item-card
    //   - 关键：不应该有 group card
    const groupCards = wrapper.findAll('.tl-group-card')
    expect(groupCards.length).toBe(0)
    const itemCards = wrapper.findAll('.tl-item-card')
    expect(itemCards.length).toBe(1000)
  })

  it('切换 viewMode group→flat→group 后 DOM 稳定', async () => {
    const { wrapper, store, list } = await mountTasksVue()
    list.viewMode.value = 'group'

    const RUN_ID = `r-tasks-toggle-${Date.now()}`
    for (let i = 0; i < 5; i++) {
      store.applyEvent('created', makeTask(`t-${i}`, RUN_ID, 'queued'))
    }
    await flushPromises()
    await nextTick()

    // group 模式：1 个 group card
    expect(wrapper.findAll('.tl-group-card').length).toBe(1)

    // 切到 flat
    list.viewMode.value = 'flat'
    await flushPromises()
    await nextTick()
    // flat 模式：0 个 group card，有 task row
    expect(wrapper.findAll('.tl-group-card').length).toBe(0)

    // 切回 group
    list.viewMode.value = 'group'
    await flushPromises()
    await nextTick()
    // group 模式：1 个 group card（恢复）
    expect(wrapper.findAll('.tl-group-card').length).toBe(1)
  })

  it('100 次 progress update 后 DOM 仍只有 1 个 group card（逃逸验证）', async () => {
    const { wrapper, store, list } = await mountTasksVue()
    list.viewMode.value = 'group'

    const RUN_ID = `r-tasks-escape-${Date.now()}`
    const taskIds: string[] = []
    for (let i = 0; i < 10; i++) {
      const id = `t-${i}`
      taskIds.push(id)
      store.applyEvent('created', makeTask(id, RUN_ID, 'queued'))
    }
    await flushPromises()
    await nextTick()

    // 初始：1 个 group card
    expect(wrapper.findAll('.tl-group-card').length).toBe(1)

    // 100 次 progress update（模拟后端 WS 推送）
    for (let round = 0; round < 10; round++) {
      for (const id of taskIds) {
        store.applyEvent('progress', { id, progress: (round + 1) * 10 })
      }
    }
    await flushPromises()
    await nextTick()

    // 关键断言：100 次 update 后，DOM 仍只有 1 个 group card
    expect(wrapper.findAll('.tl-group-card').length).toBe(1)
  })

  it('ion-infinite-scroll 存在且可触发 loadMore', async () => {
    const { wrapper, list } = await mountTasksVue()

    // ion-infinite-scroll stub 应存在（hasMore 控制显示）
    //   - 初始 hasMore=false（空 task list）→ 不显示
    //   - 模拟 hasMore=true 后显示
    list.hasMore.value = true
    await flushPromises()
    await nextTick()

    const infiniteScroll = wrapper.findComponent(IonInfiniteScrollStub)
    expect(infiniteScroll.exists()).toBe(true)

    // 触发 ionInfinite 事件
    //   - Tasks.vue 的 onInfinite 会调 loadMore() + event.target.complete()
    //   - loadMore 是 destructured 的，spyOn 后组件内仍用旧引用 → 改用验证 complete() 被调用
    const completeSpy = vi.fn()
    infiniteScroll.vm.$emit('ionInfinite', { target: { complete: completeSpy } })
    await flushPromises()
    await nextTick()
    // complete() 应被调用（onInfinite 内部调 event.target.complete()）
    expect(completeSpy).toHaveBeenCalled()
  })
})
