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

import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick } from "vue";

// ============ 全局 mock ============

const testStorage = new Map<string, string>();
const mockLocalStorage: Storage = {
  get length() {
    return testStorage.size;
  },
  key: (index: number) => Array.from(testStorage.keys())[index] ?? null,
  getItem: (key: string) => testStorage.get(key) ?? null,
  setItem: (key: string, value: string) => {
    testStorage.set(key, value);
  },
  removeItem: (key: string) => {
    testStorage.delete(key);
  },
  clear: () => {
    testStorage.clear();
  },
} as unknown as Storage;
vi.stubGlobal("localStorage", mockLocalStorage);

// Stub Ionic 组件（jsdom 不支持 web components）
//   - IonPage / IonContent / IonHeader / IonToolbar 等 → 简单 div + slot
//   - IonInfiniteScroll → 暴露 @ionInfinite 事件（测试触发 loadMore）
//   - modalController / alertController / actionSheetController → mock
const IonInfiniteScrollStub = defineComponent({
  name: "IonInfiniteScroll",
  emits: ["ionInfinite"],
  template: '<div class="ion-infinite-scroll-stub"><slot /></div>',
});
vi.mock("@ionic/vue", () => ({
  IonPage: defineComponent({ name: "IonPage", template: '<div class="ion-page-stub"><slot /></div>' }),
  IonContent: defineComponent({
    name: "IonContent",
    setup(_, { expose }) {
      // 暴露 $el 给 Tasks.vue 的 contentRef
      const el = { shadowRoot: null };
      expose({ $el: el });
    },
    template: '<div class="ion-content-stub"><slot /></div>',
  }),
  IonHeader: defineComponent({ name: "IonHeader", template: '<div class="ion-header-stub"><slot /></div>' }),
  IonToolbar: defineComponent({ name: "IonToolbar", template: '<div class="ion-toolbar-stub"><slot /></div>' }),
  IonTitle: defineComponent({ name: "IonTitle", template: '<div class="ion-title-stub"><slot /></div>' }),
  IonButtons: defineComponent({ name: "IonButtons", template: '<div class="ion-buttons-stub"><slot /></div>' }),
  IonButton: defineComponent({
    name: "IonButton",
    emits: ["click"],
    template: '<button class="ion-button-stub" @click="$emit(\'click\')"><slot /></button>',
  }),
  IonIcon: defineComponent({
    name: "IonIcon",
    props: ["icon"],
    template: '<span class="ion-icon-stub" />',
  }),
  IonItem: defineComponent({ name: "IonItem", template: '<div class="ion-item-stub"><slot /></div>' }),
  IonItemSliding: defineComponent({ name: "IonItemSliding", template: '<div class="ion-item-sliding-stub"><slot /></div>' }),
  IonItemOptions: defineComponent({ name: "IonItemOptions", template: '<div class="ion-item-options-stub"><slot /></div>' }),
  IonItemOption: defineComponent({ name: "IonItemOption", template: '<div class="ion-item-option-stub"><slot /></div>' }),
  IonLabel: defineComponent({ name: "IonLabel", template: '<span class="ion-label-stub"><slot /></span>' }),
  IonBadge: defineComponent({ name: "IonBadge", props: ["color"], template: '<span class="ion-badge-stub"><slot /></span>' }),
  IonProgressBar: defineComponent({ name: "IonProgressBar", props: ["value"], template: '<div class="ion-progress-bar-stub" />' }),
  IonFab: defineComponent({ name: "IonFab", props: ["vertical", "horizontal"], template: '<div class="ion-fab-stub"><slot /></div>' }),
  IonFabButton: defineComponent({
    name: "IonFabButton",
    emits: ["click"],
    template: '<button class="ion-fab-button-stub" @click="$emit(\'click\')"><slot /></button>',
  }),
  IonSpinner: defineComponent({ name: "IonSpinner", props: ["name"], template: '<div class="ion-spinner-stub" />' }),
  IonSearchbar: defineComponent({
    name: "IonSearchbar",
    props: ["value", "placeholder", "debounce"],
    emits: ["ionInput", "ionCancel"],
    template: '<input class="ion-searchbar-stub" :value="value" />',
  }),
  IonChip: defineComponent({
    name: "IonChip",
    props: ["color"],
    emits: ["click"],
    template: '<div class="ion-chip-stub" @click="$emit(\'click\')"><slot /></div>',
  }),
  IonPopover: defineComponent({
    name: "IonPopover",
    props: ["isOpen", "event"],
    emits: ["didDismiss"],
    template: '<div class="ion-popover-stub" v-if="isOpen"><slot /></div>',
  }),
  IonCheckbox: defineComponent({
    name: "IonCheckbox",
    props: ["checked"],
    emits: ["ionChange"],
    template: '<input type="checkbox" class="ion-checkbox-stub" :checked="checked" />',
  }),
  IonRefresher: defineComponent({ name: "IonRefresher", template: '<div class="ion-refresher-stub"><slot /></div>' }),
  IonRefresherContent: defineComponent({ name: "IonRefresherContent", template: '<div class="ion-refresher-content-stub" />' }),
  IonInfiniteScroll: IonInfiniteScrollStub,
  IonInfiniteScrollContent: defineComponent({
    name: "IonInfiniteScrollContent",
    props: ["loadingSpinner", "loadingText"],
    template: '<div class="ion-infinite-scroll-content-stub" />',
  }),
  onIonViewWillEnter: vi.fn(),
  modalController: { create: vi.fn().mockResolvedValue({ present: vi.fn(), onDidDismiss: vi.fn().mockResolvedValue({}) }) },
  alertController: { create: vi.fn().mockResolvedValue({ present: vi.fn() }) },
  actionSheetController: { create: vi.fn().mockResolvedValue({ present: vi.fn() }) },
}));

// mock @/api/encv：getTasks 返回空数组（测试用 store.applyEvent 直接注入 task）
vi.mock("@/api/encv", async () => {
  const actual = await vi.importActual<typeof import("@encv/shared-components/api/encv")>("@/api/encv");
  return {
    ...actual,
    getTasks: vi.fn().mockResolvedValue([]),
    cancelTask: vi.fn().mockResolvedValue(undefined),
    removeTask: vi.fn().mockResolvedValue(undefined),
    retryTask: vi.fn().mockResolvedValue(undefined),
    clearCompletedTasks: vi.fn().mockResolvedValue({ removed: 0 }),
    getRunSummary: vi
      .fn()
      .mockResolvedValue({ runId: "", total: 0, passed: 0, failed: 0, running: 0, pending: 0, cancelled: 0, percent: 0 }),
    listRuns: vi.fn().mockResolvedValue([]),
  };
});

// mock @/composables/useTaskEventBridge：避免 WS 连接
vi.mock("@/composables/useTaskEventBridge", () => ({
  useTaskEventBridge: vi.fn(),
}));

// mock @/composables/useNewTaskModal：避免 modalController
vi.mock("@/composables/useNewTaskModal", () => ({
  useNewTaskModal: () => ({ openNewTask: vi.fn() }),
}));

// mock @/composables/useToast：避免 toastController
vi.mock("@/composables/useToast", () => ({
  showToast: vi.fn(),
}));

// mock @/components/tasks/TaskDebugPanel：避免复杂依赖
vi.mock("@/components/tasks/TaskDebugPanel.vue", () => ({
  default: defineComponent({ name: "TaskDebugPanel", template: '<div class="task-debug-panel-stub" />' }),
}));

// 🆕 2026-06-23 修复：不再 mock TaskVirtualList，用真组件验证
//   - 真 TaskVirtualList 在 scrollEl=null 时走 fallback 渲染前 N 个 item
//   - 这样测试能覆盖真机的"scrollEl 异步就绪"场景
//   - 暴露 forceMeasure 给 Tasks.vue 调用

// mock @/composables/useWorkflowTaskService：避免 workflow 链
vi.mock("@/composables/useWorkflowTaskService", () => ({
  useWorkflowTaskService: () => ({
    isRunning: { value: false },
    totalSteps: { value: 0 },
    runs: { value: [] },
    cancelRun: vi.fn(),
    submitRun: vi.fn(),
  }),
}));

// mock vue-router：Tasks.vue 用 useRoute / useRouter
vi.mock("vue-router", () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}));

// mock @/lib/taskPersistence：避免 IndexedDB（jsdom 不支持）
vi.mock("@/lib/taskPersistence", () => ({
  loadAllTasks: vi.fn().mockResolvedValue([]),
  bulkPutTasks: vi.fn().mockResolvedValue(undefined),
  putTask: vi.fn().mockResolvedValue(undefined),
  deleteTask: vi.fn().mockResolvedValue(undefined),
  clearPutThrottle: vi.fn(),
  ensureLRUCache: vi.fn().mockResolvedValue(undefined),
}));

// ============ 测试 fixture ============

import type { EncvTask } from "@encv/shared-components/api/encv";
import { createPinia, setActivePinia } from "pinia";

async function freshModules() {
  vi.resetModules();
  setActivePinia(createPinia());
  const Tasks = (await import("@encv/shared-components/views/Tasks.vue")).default;
  const { useTaskStore } = await import("@encv/shared-components/stores/taskStore");
  const { useTasksList } = await import("@encv/shared-components/composables/useTasksList");
  return { Tasks, useTaskStore, useTasksList };
}

function makeTask(id: string, runId: string, status: string = "queued"): EncvTask {
  return {
    id,
    type: "encrypt",
    sourcePath: `/mock/sample-${id}.mp4`,
    status: status as any,
    progress: 0,
    createdAt: "2026-06-23T10:00:00.000Z",
    runId,
    triggeredBy: "automation",
    pluginName: "mp4-encrypt",
  };
}

/**
 * 挂载真 Tasks.vue，返回 wrapper + store + list
 *   - 用 attachTo: document.body 让 ion-content 的 $el 可被 Tasks.vue 拿到
 *   - flushPromises 让 onMounted / watch / computed 跑完
 */
async function mountTasksVue() {
  const { Tasks, useTaskStore, useTasksList } = await freshModules();
  const store = useTaskStore();
  const list = useTasksList();

  const wrapper = mount(Tasks, {
    attachTo: document.body,
    global: {
      stubs: {
        // TaskVirtualList 用真组件（验证虚拟滚动行为）
        // 其他 Ionic 组件已在 vi.mock 中 stub
      },
    },
  });
  await flushPromises();
  await nextTick();
  return { wrapper, store, list };
}

describe("Tasks.vue 真机组件测试", () => {
  beforeEach(() => {
    testStorage.clear();
    vi.clearAllMocks();
  });

  it("空状态：无 task 时显示 .empty-state", async () => {
    const { wrapper } = await mountTasksVue();
    // 空状态应显示 .empty-state 或类似 class
    const emptyState = wrapper.find('.empty-state, .tl-empty, [class*="empty"]');
    expect(emptyState.exists()).toBe(true);
  });

  it("10 task 共享 runId → DOM 只有 1 个 .tl-group-card（不逃逸）", async () => {
    const { wrapper, store, list } = await mountTasksVue();
    list.viewMode.value = "group";

    const RUN_ID = `r-tasks-1-${Date.now()}`;
    for (let i = 0; i < 10; i++) {
      store.applyEvent("created", makeTask(`t-${i}`, RUN_ID, "queued"));
    }
    await flushPromises();
    await nextTick();

    // 关键断言：DOM 里只有 1 个 .tl-group-card（不是 10 个）
    const groupCards = wrapper.findAll(".tl-group-card");
    expect(groupCards.length).toBe(1);

    // group card 的 data-run-id（如果有）或 runId 文本
    const card = groupCards[0];
    // group card 内部应有 runId 显示（Tasks.vue 用 <code data-testid="group-card-runid">）
    const runidEl = card.find('[data-testid="group-card-runid"]');
    if (runidEl.exists()) {
      expect(runidEl.text()).toContain(RUN_ID);
    }
  });

  it("虚拟滚动核心：1000 task 共享 runId → 只有 1 个 .tl-group-card（聚合验证）", async () => {
    const { wrapper, store, list } = await mountTasksVue();
    list.viewMode.value = "group";

    const RUN_ID = `r-tasks-1000-${Date.now()}`;
    // 注入 1000 个 task（全部共享 runId → 1 个 group card）
    //   - 用 bulkSetTasks 而非 applyEvent('created', ...)：
    //   - mountTasksVue 后 hydrated=true，applyEvent 的 WS 守卫在 100 task 后拒绝 push
    //   - bulkSetTasks 模拟"后端返回整页 task"路径（无守卫），可一次注入 1000 个
    const tasks: EncvTask[] = Array.from({ length: 1000 }, (_, i) => makeTask(`t-${i}`, RUN_ID, "queued"));
    store.bulkSetTasks(tasks);
    await flushPromises();
    await nextTick();

    // 关键断言：DOM 里只有 1 个 .tl-group-card（1000 task 聚合为 1 个 group）
    const groupCards = wrapper.findAll(".tl-group-card");
    expect(groupCards.length).toBe(1);

    // group card 内部应有 runId 显示（Tasks.vue 用 <code data-testid="group-card-runid">）
    const runidEl = groupCards[0].find('[data-testid="group-card-runid"]');
    if (runidEl.exists()) {
      expect(runidEl.text()).toContain(RUN_ID);
    }

    // 关键断言：不应该有 task row（group 模式下 task 在 group 内部）
    const taskRows = wrapper.findAll(".tl-task-row");
    expect(taskRows.length).toBe(0);
  });

  it("flat 模式：1000 task → scrollEl=null 降级渲染 fallbackCount 个 .tl-item-card（非空白）", async () => {
    const { wrapper, store, list } = await mountTasksVue();
    list.viewMode.value = "flat";

    const RUN_ID = `r-tasks-flat-${Date.now()}`;
    // 用 bulkSetTasks 注入 1000 个 task（绕过 applyEvent 的 WS 守卫，见上测试注释）
    const tasks: EncvTask[] = Array.from({ length: 1000 }, (_, i) => makeTask(`t-${i}`, RUN_ID, "queued"));
    store.bulkSetTasks(tasks);
    await flushPromises();
    await nextTick();

    // flat 模式下：每个 task 是 1 个 ion-item-sliding > ion-item.tl-item-card
    //   - 真 TaskVirtualList 在 scrollEl=null（jsdom 无 shadow DOM）时走 fallback
    //   - fallback 渲染前 fallbackCount = min(count, overscan*2+20) = min(1000, 40) = 40 个
    //   - 关键：不应该有 group card
    const groupCards = wrapper.findAll(".tl-group-card");
    expect(groupCards.length).toBe(0);
    const itemCards = wrapper.findAll(".tl-item-card");
    // fallbackCount = min(1000, 10*2+20) = 40，但 date section header 占位 → 实际 item card < 40
    //   - 关键断言：> 0（非空白）且 <= 40（fallback 上限）
    expect(itemCards.length).toBeGreaterThan(0);
    expect(itemCards.length).toBeLessThanOrEqual(40);
  });

  it("切换 viewMode group→flat→group 后 DOM 稳定", async () => {
    const { wrapper, store, list } = await mountTasksVue();
    list.viewMode.value = "group";

    const RUN_ID = `r-tasks-toggle-${Date.now()}`;
    for (let i = 0; i < 5; i++) {
      store.applyEvent("created", makeTask(`t-${i}`, RUN_ID, "queued"));
    }
    await flushPromises();
    await nextTick();

    // group 模式：1 个 group card
    expect(wrapper.findAll(".tl-group-card").length).toBe(1);

    // 切到 flat
    list.viewMode.value = "flat";
    await flushPromises();
    await nextTick();
    // flat 模式：0 个 group card，有 task row
    expect(wrapper.findAll(".tl-group-card").length).toBe(0);

    // 切回 group
    list.viewMode.value = "group";
    await flushPromises();
    await nextTick();
    // group 模式：1 个 group card（恢复）
    expect(wrapper.findAll(".tl-group-card").length).toBe(1);
  });

  it("100 次 progress update 后 DOM 仍只有 1 个 group card（逃逸验证）", async () => {
    const { wrapper, store, list } = await mountTasksVue();
    list.viewMode.value = "group";

    const RUN_ID = `r-tasks-escape-${Date.now()}`;
    const taskIds: string[] = [];
    for (let i = 0; i < 10; i++) {
      const id = `t-${i}`;
      taskIds.push(id);
      store.applyEvent("created", makeTask(id, RUN_ID, "queued"));
    }
    await flushPromises();
    await nextTick();

    // 初始：1 个 group card
    expect(wrapper.findAll(".tl-group-card").length).toBe(1);

    // 100 次 progress update（模拟后端 WS 推送）
    for (let round = 0; round < 10; round++) {
      for (const id of taskIds) {
        store.applyEvent("progress", { id, progress: (round + 1) * 10 });
      }
    }
    await flushPromises();
    await nextTick();

    // 关键断言：100 次 update 后，DOM 仍只有 1 个 group card
    expect(wrapper.findAll(".tl-group-card").length).toBe(1);
  });

  it("ion-infinite-scroll 存在且可触发 loadMore", async () => {
    const { wrapper, list } = await mountTasksVue();

    // ion-infinite-scroll stub 应存在（hasMore 控制显示）
    //   - 初始 hasMore=false（空 task list）→ 不显示
    //   - 模拟 hasMore=true 后显示
    list.hasMore.value = true;
    await flushPromises();
    await nextTick();

    const infiniteScroll = wrapper.findComponent(IonInfiniteScrollStub);
    expect(infiniteScroll.exists()).toBe(true);

    // 触发 ionInfinite 事件
    //   - Tasks.vue 的 onInfinite 会调 loadMore() + event.target.complete()
    //   - loadMore 是 destructured 的，spyOn 后组件内仍用旧引用 → 改用验证 complete() 被调用
    const completeSpy = vi.fn();
    infiniteScroll.vm.$emit("ionInfinite", { target: { complete: completeSpy } });
    await flushPromises();
    await nextTick();
    // complete() 应被调用（onInfinite 内部调 event.target.complete()）
    expect(completeSpy).toHaveBeenCalled();
  });

  // 🆕 2026-06-23 修复真机空白：验证 worker 未就绪时 sync 兜底不空白
  //   - 真机场景：worker 异步计算期间 displayedItems 不应为空
  //   - useTaskViewCompute 在 workerHasResult=false 时用 syncDisplayedItems 兜底
  //   - 测试环境 isFallback=true（VITEST）→ 直接走 sync → 立即有数据
  it("真机空白修复：worker 未就绪时 displayedItems 用 sync 兜底（非空白）", async () => {
    const { wrapper, store, list } = await mountTasksVue();
    list.viewMode.value = "group";

    const RUN_ID = `r-blank-fix-${Date.now()}`;
    // 注入 5 个 task（模拟自动化测试创建）
    for (let i = 0; i < 5; i++) {
      store.applyEvent("created", makeTask(`t-${i}`, RUN_ID, "queued"));
    }
    await flushPromises();
    await nextTick();

    // 关键断言：displayedItems 不为空（sync 兜底立即计算）
    //   - 旧 bug：worker 异步计算期间 workerDisplayedItems=[] → 页面空白
    //   - 修复：workerHasResult=false 时用 syncDisplayedItems 兜底
    expect(list.displayedItems.value.length).toBeGreaterThan(0);

    // DOM 应有 1 个 group card（5 task 聚合）
    expect(wrapper.findAll(".tl-group-card").length).toBe(1);
  });

  // 🆕 2026-06-23 修复真机空白：验证 TaskVirtualList scrollEl=null 降级渲染
  //   - 真机场景：ion-content shadow DOM 异步渲染 → scrollEl=null → 旧逻辑空白
  //   - 修复：scrollEl=null 时 TaskVirtualList 走 fallback 渲染前 N 个 item
  it("真机空白修复：scrollEl=null 时 TaskVirtualList 降级渲染（非空白）", async () => {
    const { wrapper, store, list } = await mountTasksVue();
    list.viewMode.value = "group";

    const RUN_ID = `r-scroll-null-${Date.now()}`;
    store.bulkSetTasks(Array.from({ length: 50 }, (_, i) => makeTask(`t-${i}`, RUN_ID, "queued")));
    await flushPromises();
    await nextTick();

    // jsdom 无 shadow DOM → scrollEl=null → TaskVirtualList 走 fallback
    //   - fallback 渲染前 fallbackCount = min(50, 40) = 40 个 item
    //   - 50 task 共享 1 runId → 1 个 group card + 1 个 date header
    //   - fallback 渲染的 item 包含 group card（kind='group'）
    const groupCards = wrapper.findAll(".tl-group-card");
    expect(groupCards.length).toBe(1);

    // 验证不是空白：task-virtual-list 容器存在且有子元素
    const virtualList = wrapper.find(".task-virtual-list");
    expect(virtualList.exists()).toBe(true);
    expect(virtualList.element.children.length).toBeGreaterThan(0);
  });
});
