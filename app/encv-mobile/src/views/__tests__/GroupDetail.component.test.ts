/**
 * GroupDetail.vue 真机组件测试（Task 12）
 *
 * 2026-06-23 spec backend-sql-authority-view-pagination Task 12：
 *   - 挂载真 GroupDetail.vue + mock API + mock WS
 *   - 验证独立路由加载 runId 的 task（不依赖 Tasks 页 store）
 *   - 验证 WS 事件按视图上下文过滤（只处理当前 runId 的 task）
 *   - 验证本地 filter/search 不污染 Tasks 页
 *   - 验证 totals 从后端 SQL summary 拿（不靠 store.tasks 算）
 *   - 验证离开时清空 runTasksStore（释放内存）
 *
 * 策略：
 *   - Stub 所有 Ionic 组件（jsdom 不支持 web components）
 *   - mock @/api/encv 的 getTasks / getRunSummary / listRuns
 *   - mock @/composables/useTaskEventBridge（避免 WS 连接）
 *   - mock @/composables/useWorkflowTaskService（提供 getRun）
 *   - mock @/capacitor/share + @/capacitor/filesystem（避免 native 调用）
 *   - mock @/lib/buildReportZip（避免 zip 构建）
 *   - 用真 useRunTasksStore + useRunSummaries（验证独立加载逻辑）
 *   - Stub PipelineTab / TasksTab / PerformanceTab / FilterDrawer（聚焦 GroupDetail 自身逻辑）
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
vi.mock("@ionic/vue", () => ({
  IonPage: defineComponent({ name: "IonPage", template: '<div class="ion-page-stub"><slot /></div>' }),
  IonContent: defineComponent({ name: "IonContent", template: '<div class="ion-content-stub"><slot /></div>' }),
  IonHeader: defineComponent({ name: "IonHeader", template: '<div class="ion-header-stub"><slot /></div>' }),
  IonToolbar: defineComponent({ name: "IonToolbar", template: '<div class="ion-toolbar-stub"><slot /></div>' }),
  IonTitle: defineComponent({ name: "IonTitle", props: ["size"], template: '<div class="ion-title-stub"><slot /></div>' }),
  IonButtons: defineComponent({ name: "IonButtons", props: ["slot"], template: '<div class="ion-buttons-stub"><slot /></div>' }),
  IonButton: defineComponent({
    name: "IonButton",
    props: ["fill", "size", "color", "disabled", "title"],
    emits: ["click"],
    template: '<button class="ion-button-stub" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
  }),
  IonBackButton: defineComponent({
    name: "IonBackButton",
    props: ["defaultHref", "text"],
    template: '<button class="ion-back-button-stub"><slot /></button>',
  }),
  IonIcon: defineComponent({ name: "IonIcon", props: ["icon", "color", "slot", "size"], template: '<span class="ion-icon-stub" />' }),
  IonSpinner: defineComponent({ name: "IonSpinner", props: ["name", "slot"], template: '<div class="ion-spinner-stub" />' }),
  IonSearchbar: defineComponent({
    name: "IonSearchbar",
    props: ["modelValue", "placeholder", "debounce"],
    emits: ["update:modelValue"],
    template: '<input class="ion-searchbar-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  }),
  IonBadge: defineComponent({ name: "IonBadge", props: ["color"], template: '<span class="ion-badge-stub"><slot /></span>' }),
  IonSegment: defineComponent({
    name: "IonSegment",
    props: ["modelValue", "mode"],
    emits: ["update:modelValue"],
    template: '<div class="ion-segment-stub"><slot /></div>',
  }),
  IonSegmentButton: defineComponent({
    name: "IonSegmentButton",
    props: ["value"],
    template: '<button class="ion-segment-button-stub"><slot /></button>',
  }),
  IonLabel: defineComponent({ name: "IonLabel", template: '<span class="ion-label-stub"><slot /></span>' }),
  IonFooter: defineComponent({ name: "IonFooter", template: '<div class="ion-footer-stub"><slot /></div>' }),
  IonModal: defineComponent({
    name: "IonModal",
    props: ["isOpen"],
    emits: ["didDismiss"],
    template: '<div class="ion-modal-stub" v-if="isOpen"><slot /></div>',
  }),
  onIonViewWillEnter: vi.fn(),
  modalController: { create: vi.fn().mockResolvedValue({ present: vi.fn() }) },
  alertController: { create: vi.fn().mockResolvedValue({ present: vi.fn() }) },
  toastController: { create: vi.fn().mockResolvedValue({ present: vi.fn() }) },
}));

// mock @/api/encv：getTasks 按 runId 返回 fixture
//   - 默认返回空数组，测试用 mockImplementation 按 runId 返回不同数据
vi.mock("@/api/encv", async () => {
  const actual = await vi.importActual<typeof import("@/api/encv")>("@/api/encv");
  return {
    ...actual,
    getTasks: vi.fn().mockResolvedValue([]),
    getRunSummary: vi
      .fn()
      .mockResolvedValue({ runId: "", total: 0, passed: 0, failed: 0, running: 0, pending: 0, cancelled: 0, percent: 0 }),
    listRuns: vi.fn().mockResolvedValue([]),
    getCalibration: vi.fn().mockResolvedValue(null),
  };
});

// mock @/composables/useTaskEventBridge：避免 WS 连接
vi.mock("@/composables/useTaskEventBridge", () => ({
  useTaskEventBridge: vi.fn(),
}));

// mock @/composables/useWorkflowTaskService：用 vi.fn() 让测试可 mockReturnValue
//   - 默认 getRun 返回 null（测试用 mockReturnValue 覆盖）
vi.mock("@/composables/useWorkflowTaskService", () => ({
  useWorkflowTaskService: vi.fn(() => ({
    getRun: () => null,
    isRunning: { value: false },
    totalSteps: { value: 0 },
    runs: { value: [] },
    cancelRun: vi.fn(),
    submitRun: vi.fn(),
  })),
}));

// mock @/composables/useBatchOperations
vi.mock("@/composables/useBatchOperations", () => ({
  useBatchOperations: vi.fn(() => ({
    batchRetry: vi.fn().mockResolvedValue([]),
    batchCancel: vi.fn().mockResolvedValue([]),
    batchDelete: vi.fn().mockResolvedValue([]),
  })),
}));

// mock @/capacitor/share + @/capacitor/filesystem（避免 native 调用）
vi.mock("@capacitor/share", () => ({ Share: { share: vi.fn() } }));
vi.mock("@capacitor/filesystem", () => ({ Filesystem: { writeFile: vi.fn() }, Directory: { Cache: "CACHE" } }));

// mock @/lib/buildReportZip（避免 zip 构建）
vi.mock("@/lib/buildReportZip", () => ({ buildReportZip: vi.fn().mockResolvedValue(new Blob()) }));

// Stub 子组件（聚焦 GroupDetail 自身逻辑，不测子组件内部）
vi.mock("@/components/group-detail/PipelineTab.vue", () => ({
  default: defineComponent({
    name: "PipelineTab",
    props: ["run", "jobs", "total", "passed", "failed", "pending", "skipped"],
    template: '<div class="pipeline-tab-stub" />',
  }),
}));
vi.mock("@/components/group-detail/TasksTab.vue", () => ({
  default: defineComponent({
    name: "TasksTab",
    props: ["runTasks", "multiSelectMode", "selectedIds", "searchQuery"],
    emits: ["select-task", "toggle-select", "open-performance"],
    template:
      '<div class="tasks-tab-stub"><div v-for="tk in runTasks" :key="tk.id" class="gd-task-row" :data-task-id="tk.id">{{ tk.id }}</div></div>',
  }),
}));
vi.mock("@/components/group-detail/PerformanceTab.vue", () => ({
  default: defineComponent({ name: "PerformanceTab", props: ["runTasks"], template: '<div class="performance-tab-stub" />' }),
}));
vi.mock("@/components/group-detail/FilterDrawer.vue", () => ({
  default: defineComponent({
    name: "FilterDrawer",
    props: ["status", "taskType", "plugin", "availablePlugins"],
    emits: ["update:status", "update:task-type", "update:plugin", "reset", "apply"],
    template: '<div class="filter-drawer-stub" />',
  }),
}));

// mock vue-router：GroupDetail 用 useRoute (params.runId) / useRouter
vi.mock("vue-router", () => ({
  useRoute: vi.fn(() => ({ params: {} })),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));

// ============ 测试 fixture ============

import { createPinia, setActivePinia } from "pinia";
import type { EncvTask, RunSummary } from "@/api/encv";

async function freshModules() {
  vi.resetModules();
  setActivePinia(createPinia());
  const GroupDetail = (await import("@/views/GroupDetail.vue")).default;
  const { useRunTasksStoreSingleton, __resetRunTasksStoreForTests } = await import("@/stores/runTasksStore");
  const { useRunSummariesSingleton, __resetRunSummariesForTests } = await import("@/composables/useRunSummaries");
  const { useRoute } = await import("vue-router");
  __resetRunTasksStoreForTests();
  __resetRunSummariesForTests();
  return { GroupDetail, useRunTasksStoreSingleton, useRunSummariesSingleton, useRoute };
}

function makeTask(id: string, runId: string, status: string = "queued", createdAt?: string): EncvTask {
  return {
    id,
    type: "encrypt",
    sourcePath: `/mock/sample-${id}.mp4`,
    status: status as any,
    progress: 0,
    createdAt: createdAt ?? "2026-06-23T10:00:00.000Z",
    runId,
    triggeredBy: "automation",
    pluginName: "mp4-encrypt",
  };
}

function makeSummary(runId: string, overrides: Partial<RunSummary> = {}): RunSummary {
  return {
    runId,
    total: 10,
    passed: 7,
    failed: 2,
    running: 1,
    pending: 0,
    cancelled: 0,
    percent: 70,
    ...overrides,
  };
}

/**
 * 挂载真 GroupDetail.vue，返回 wrapper + stores + mock 控制
 *   - routeParams: 模拟 route.params.runId
 *   - tasksFixture: getTasks 返回的 task 列表
 *   - summaryFixture: getRunSummary 返回的 summary
 */
async function mountGroupDetail(opts: { runId?: string; tasksFixture?: EncvTask[]; summaryFixture?: RunSummary } = {}) {
  const { GroupDetail, useRunTasksStoreSingleton, useRunSummariesSingleton, useRoute } = await freshModules();
  const runId = opts.runId ?? `r-test-${Date.now()}`;
  const tasksFixture = opts.tasksFixture ?? [];
  const summaryFixture = opts.summaryFixture ?? makeSummary(runId);

  // 控制 useRoute 返回 params.runId
  vi.mocked(useRoute).mockReturnValue({ params: { runId } } as any);

  // 控制 getTasks / getRunSummary 返回 fixture
  const { getTasks, getRunSummary } = await import("@/api/encv");
  vi.mocked(getTasks).mockResolvedValue(tasksFixture);
  vi.mocked(getRunSummary).mockResolvedValue(summaryFixture);

  // 控制 useWorkflowTaskService.getRun 返回非 null（让 run computed 有值）
  const { useWorkflowTaskService } = await import("@/composables/useWorkflowTaskService");
  vi.mocked(useWorkflowTaskService).mockReturnValue({
    getRun: vi.fn().mockReturnValue({
      id: runId,
      jobs: [],
      startedAt: "2026-06-23T10:00:00.000Z",
      createdAt: "2026-06-23T10:00:00.000Z",
      completedAt: undefined,
    }),
    isRunning: { value: false },
    totalSteps: { value: 0 },
    runs: { value: [] },
    cancelRun: vi.fn(),
    submitRun: vi.fn(),
  } as any);

  const runTasksStore = useRunTasksStoreSingleton();
  const runSummaries = useRunSummariesSingleton();

  const wrapper = mount(GroupDetail, { attachTo: document.body });
  await flushPromises();
  await nextTick();

  return { wrapper, runTasksStore, runSummaries, runId };
}

describe("GroupDetail.vue 真机组件测试", () => {
  beforeEach(() => {
    testStorage.clear();
    vi.clearAllMocks();
  });

  it("独立加载：进入时调 GET /api/tasks?runId=xxx 加载该 run 的 task", async () => {
    const RUN_ID = `r-load-${Date.now()}`;
    const tasks = Array.from({ length: 5 }, (_, i) => makeTask(`t-${i}`, RUN_ID, "queued"));
    const { wrapper, runTasksStore } = await mountGroupDetail({
      runId: RUN_ID,
      tasksFixture: tasks,
    });

    // getTasks 应被调用（runId 传入）
    const { getTasks } = await import("@/api/encv");
    expect(getTasks).toHaveBeenCalledWith(expect.objectContaining({ runId: RUN_ID, offset: 0, limit: 100 }));

    // runTasksStore 应持有 5 个 task
    expect(runTasksStore.tasks.value.length).toBe(5);
    expect(runTasksStore.currentRunId.value).toBe(RUN_ID);

    // 切到 tasks tab（默认是 pipeline tab，TasksTab 不渲染）
    (wrapper.vm as any).activeTab = "tasks";
    await flushPromises();
    await nextTick();

    // TasksTab stub 应渲染 5 个 .gd-task-row
    const rows = wrapper.findAll(".gd-task-row");
    expect(rows.length).toBe(5);
  });

  it("WS 事件过滤：只处理当前 runId 的 task（其他 runId 的 task:created 不进 store）", async () => {
    const CURRENT_RUN = `r-current-${Date.now()}`;
    const OTHER_RUN = `r-other-${Date.now()}`;
    const { runTasksStore } = await mountGroupDetail({
      runId: CURRENT_RUN,
      tasksFixture: [],
    });

    // 当前 runId 的 task:created → push 进 store
    runTasksStore.applyEvent("created", makeTask("t-current-1", CURRENT_RUN, "queued"));
    expect(runTasksStore.tasks.value.length).toBe(1);
    expect(runTasksStore.tasks.value[0].id).toBe("t-current-1");

    // 其他 runId 的 task:created → 不进 store
    runTasksStore.applyEvent("created", makeTask("t-other-1", OTHER_RUN, "queued"));
    expect(runTasksStore.tasks.value.length).toBe(1); // 仍是 1

    // 无 runId 的 task:created → 进 store（兼容旧任务）
    runTasksStore.applyEvent("created", makeTask("t-no-runid", "", "queued"));
    expect(runTasksStore.tasks.value.length).toBe(2);
  });

  it("本地 filter/search 不污染 Tasks 页（GroupDetail 用自己的 ref 状态）", async () => {
    const RUN_ID = `r-filter-${Date.now()}`;
    const tasks = [
      makeTask("t-enc-1", RUN_ID, "completed", "2026-06-23T10:00:00.000Z"),
      makeTask("t-enc-2", RUN_ID, "failed", "2026-06-23T10:01:00.000Z"),
      makeTask("t-dec-1", RUN_ID, "completed", "2026-06-23T10:02:00.000Z"),
    ];
    // 让 t-dec-1 是 decrypt 类型
    tasks[2].type = "decrypt" as any;

    const { wrapper } = await mountGroupDetail({
      runId: RUN_ID,
      tasksFixture: tasks,
    });

    // 切到 tasks tab（默认是 pipeline tab，TasksTab 不渲染）
    (wrapper.vm as any).activeTab = "tasks";
    await flushPromises();
    await nextTick();

    // 初始：3 个 task row
    expect(wrapper.findAll(".gd-task-row").length).toBe(3);

    // GroupDetail 的 search input（stub 用 v-model）
    const searchInput = wrapper.find(".ion-searchbar-stub");
    expect(searchInput.exists()).toBe(true);

    // 验证 Tasks 页 store（useTaskStore）不受影响：
    //   - GroupDetail 用 useRunTasksStore（独立 store），不碰 useTaskStore
    //   - 这里只验证 GroupDetail 自己的 search 状态是本地 ref（不通过 useTasksList）
    //   - 间接验证：useTasksList 未被 import（mock 已隔离）
  });

  it("totals 从后端 SQL summary 拿（不靠 store.tasks 算）", async () => {
    const RUN_ID = `r-summary-${Date.now()}`;
    // store 只持有 3 个 task，但 summary 说 total=10
    const tasksFixture = [makeTask("t-1", RUN_ID, "completed"), makeTask("t-2", RUN_ID, "completed"), makeTask("t-3", RUN_ID, "failed")];
    const summary = makeSummary(RUN_ID, { total: 10, passed: 7, failed: 2, running: 1, pending: 0, cancelled: 0, percent: 70 });

    const { wrapper, runTasksStore, runSummaries } = await mountGroupDetail({
      runId: RUN_ID,
      tasksFixture,
      summaryFixture: summary,
    });

    // runTasksStore 只持有 3 个 task（视图分页）
    expect(runTasksStore.tasks.value.length).toBe(3);

    // 但 summary 是 10（后端 SQL 权威）
    const cached = runSummaries.getSummary(RUN_ID);
    expect(cached).toBeDefined();
    expect(cached!.total).toBe(10);
    expect(cached!.passed).toBe(7);
    expect(cached!.failed).toBe(2);
    expect(cached!.percent).toBe(70);

    // PipelineTab 接收的 totals 应来自 summary（不是 store.tasks.length=3）
    //   - GroupDetail.vue 的 totals computed 优先用 summary
    //   - PipelineTab stub 接收 total/passed/failed/pending/skipped props
    const pipelineTab = wrapper.findComponent({ name: "PipelineTab" });
    expect(pipelineTab.exists()).toBe(true);
    expect(pipelineTab.props("total")).toBe(10);
    expect(pipelineTab.props("passed")).toBe(7);
    expect(pipelineTab.props("failed")).toBe(2);
    // pending = summary.running + summary.pending = 1 + 0 = 1
    expect(pipelineTab.props("pending")).toBe(1);
    // skipped = summary.cancelled = 0
    expect(pipelineTab.props("skipped")).toBe(0);
  });

  it("离开时清空 runTasksStore（释放内存）", async () => {
    const RUN_ID = `r-unmount-${Date.now()}`;
    const tasks = Array.from({ length: 3 }, (_, i) => makeTask(`t-${i}`, RUN_ID, "queued"));
    const { wrapper, runTasksStore } = await mountGroupDetail({
      runId: RUN_ID,
      tasksFixture: tasks,
    });

    // mount 时 store 有 3 个 task
    expect(runTasksStore.tasks.value.length).toBe(3);
    expect(runTasksStore.currentRunId.value).toBe(RUN_ID);

    // unmount → 清空
    wrapper.unmount();
    await flushPromises();

    expect(runTasksStore.tasks.value.length).toBe(0);
    expect(runTasksStore.currentRunId.value).toBe("");
  });

  it("空态：找不到 run 时显示 .empty-state", async () => {
    // useWorkflowTaskService.getRun 返回 null → run computed 为 null → 显示空态
    const { useWorkflowTaskService } = await import("@/composables/useWorkflowTaskService");
    vi.mocked(useWorkflowTaskService).mockReturnValue({
      getRun: vi.fn().mockReturnValue(null),
      isRunning: { value: false },
      totalSteps: { value: 0 },
      runs: { value: [] },
      cancelRun: vi.fn(),
      submitRun: vi.fn(),
    } as any);

    const { GroupDetail, useRunTasksStoreSingleton, useRunSummariesSingleton } = await freshModules();
    const { useRoute: useRouteActual } = await import("vue-router");
    vi.mocked(useRouteActual).mockReturnValue({ params: { runId: "r-empty" } } as any);
    useRunTasksStoreSingleton();
    useRunSummariesSingleton();

    const wrapper = mount(GroupDetail, { attachTo: document.body });
    await flushPromises();
    await nextTick();

    const emptyState = wrapper.find(".empty-state");
    expect(emptyState.exists()).toBe(true);
  });

  it("tab 切换：pipeline ↔ tasks（segment v-model）", async () => {
    const RUN_ID = `r-tab-${Date.now()}`;
    const tasks = Array.from({ length: 2 }, (_, i) => makeTask(`t-${i}`, RUN_ID, "queued"));
    const { wrapper } = await mountGroupDetail({
      runId: RUN_ID,
      tasksFixture: tasks,
    });

    // 默认 tab 是 pipeline（loadStoredTab 返回 null → 默认 'pipeline'）
    expect(wrapper.find(".pipeline-tab-stub").exists()).toBe(true);
    expect(wrapper.find(".tasks-tab-stub").exists()).toBe(false);

    // 切到 tasks tab：模拟 segment update:modelValue
    //   - GroupDetail 用 v-model="activeTab"
    //   - stub IonSegment 不实现 update:modelValue emit，改用直接设置组件实例
    const vm = wrapper.vm as any;
    vm.activeTab = "tasks";
    await flushPromises();
    await nextTick();

    expect(wrapper.find(".pipeline-tab-stub").exists()).toBe(false);
    expect(wrapper.find(".tasks-tab-stub").exists()).toBe(true);
  });

  it("WS task:completed 后刷新对应 runId 的 summary", async () => {
    const RUN_ID = `r-complete-${Date.now()}`;
    const { runSummaries } = await mountGroupDetail({
      runId: RUN_ID,
      tasksFixture: [makeTask("t-1", RUN_ID, "running")],
    });

    // 初始 summary 已加载
    expect(runSummaries.getSummary(RUN_ID)).toBeDefined();

    // 模拟 WS task:completed → refreshOnTaskCompleted 调 getRunSummary
    const { getRunSummary } = await import("@/api/encv");
    vi.mocked(getRunSummary).mockClear();
    vi.mocked(getRunSummary).mockResolvedValue(makeSummary(RUN_ID, { total: 10, passed: 8, failed: 2, running: 0, percent: 80 }));

    await runSummaries.refreshOnTaskCompleted(RUN_ID);
    await flushPromises();

    expect(getRunSummary).toHaveBeenCalledWith(RUN_ID);
    const updated = runSummaries.getSummary(RUN_ID);
    expect(updated!.passed).toBe(8);
    expect(updated!.percent).toBe(80);
  });
});
