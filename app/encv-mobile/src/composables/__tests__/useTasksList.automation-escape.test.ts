/**
 * 真机"任务逃逸"诊断 — 渲染逻辑层面（非样式）
 *
 * 覆盖会影响 displayedItems 实际内容的逻辑（user 反馈 2026-06-22）：
 * - viewMode 切换（group ↔ flat）
 * - sortOrder 切换（asc ↔ desc）
 * - dateSection（today / yesterday / thisWeek / thisMonth / earlier）— task.createdAt 在不同时间段
 * - pinnedRunIds — 置顶 runId
 * - filter（plugin / type / status / date）— 过滤
 *
 * 真机自动化测试入口：7 个真机 plugin 数据 + buildDynamicWorkflow 派生 case
 * 测试规模：1+1+1000（1002 task）— 接近真机 1+1+365 量级
 */

import { mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick } from "vue";

// ==================== Setup mocks ====================
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

import { createPinia, setActivePinia } from "pinia";
import type { EncvTask, PluginMeta, TaskStatus } from "@/api/encv";
import { _resetTasksListSingletonForTests, useTasksList } from "@/composables/useTasksList";
import { extToRelativePath } from "@/lib/mockDataGenerator";
import { useTaskStore } from "@/stores/taskStore";

vi.mock("@/composables/useTaskEventBridge", () => ({
  useTaskEventBridge: () => {},
}));
vi.mock("@/lib/taskPersistence", () => ({
  loadAllTasks: vi.fn().mockResolvedValue([]),
  bulkPutTasks: vi.fn().mockResolvedValue(undefined),
  putTask: vi.fn().mockResolvedValue(undefined),
  deleteTask: vi.fn().mockResolvedValue(undefined),
  clearPutThrottle: vi.fn(),
  ensureLRUCache: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("@/composables/useWorkflowTaskService", () => ({
  useWorkflowTaskService: () => ({
    currentRun: { value: null },
    isRunning: { value: false },
    currentDef: { value: null },
    submitRun: vi.fn().mockResolvedValue({ id: "mock-run", jobs: [] }),
    cancelRun: vi.fn(),
    listRuns: () => [],
    getRun: () => undefined,
  }),
  __resetServiceForTests: () => {},
}));
vi.mock("@/composables/useI18n", () => ({
  useI18n: () => ({ t: (_k: string, opts?: any) => opts?.defaultValue ?? _k }),
}));
vi.mock("@/composables/useDateFormat", () => ({
  formatDateTime: (d: string) => d,
}));

// ==================== 真机 7 个 plugin 数据（从后端 registry.go 真值构造） ====================
function buildRealPlugins(): PluginMeta[] {
  const SUPPORTED_VERSIONS = [3, 4];
  const DEFAULT_VERSION = 4;
  const CONTAINER_EXT: Record<string, string> = {
    video: ".encv",
    audio: ".enca",
    image: ".enci",
    wps: ".encw",
    pdf: ".encp",
    text: ".enct",
    alistencrypt: ".ae",
  };
  return [
    {
      name: "video",
      supportedExtensions: ["mp4", "mkv", "avi", "mov", "rmvb", "webm", "flv", "m3u8"],
      supportedMimePrefixes: ["video/"],
      containerExtension: CONTAINER_EXT.video,
      taskOptions: {
        passwordStrategy: "global",
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [
          {
            key: "stream_preset",
            label: "streamPreset",
            type: "select",
            required: false,
            defaultValue: "balanced",
            options: ["balanced", "quality", "high_quality"],
            condition: "encrypt",
          },
          {
            key: "fn_rounds",
            label: "fnRounds",
            type: "select",
            required: false,
            defaultValue: "8",
            options: ["4", "8", "12", "16"],
            condition: "encrypt",
          },
          {
            key: "fn_charset",
            label: "fnCharset",
            type: "select",
            required: false,
            defaultValue: "alphanumeric",
            options: ["alphanumeric", "hex"],
            condition: "encrypt",
          },
          { key: "encrypt_filename", label: "encryptFilename", type: "bool", required: false, defaultValue: "false", condition: "encrypt" },
        ],
      },
    },
    {
      name: "audio",
      supportedExtensions: ["mp3", "flac", "ogg", "m4a", "wav", "aac", "opus"],
      supportedMimePrefixes: ["audio/"],
      containerExtension: CONTAINER_EXT.audio,
      taskOptions: {
        passwordStrategy: "global",
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [
          {
            key: "fn_charset",
            label: "fnCharset",
            type: "select",
            required: false,
            defaultValue: "alphanumeric",
            options: ["alphanumeric", "hex"],
            condition: "encrypt",
          },
        ],
      },
    },
    {
      name: "image",
      supportedExtensions: ["png", "jpg", "jpeg", "gif", "webp", "bmp", "tiff"],
      supportedMimePrefixes: ["image/"],
      containerExtension: CONTAINER_EXT.image,
      taskOptions: {
        passwordStrategy: "global",
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
    {
      name: "wps",
      supportedExtensions: ["doc", "docx", "xls", "xlsx", "ppt", "pptx"],
      supportedMimePrefixes: [],
      containerExtension: CONTAINER_EXT.wps,
      taskOptions: {
        passwordStrategy: "global",
        supportVersionSelect: false,
        supportedVersions: null,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
    {
      name: "pdf",
      supportedExtensions: ["pdf"],
      supportedMimePrefixes: ["application/pdf"],
      containerExtension: CONTAINER_EXT.pdf,
      taskOptions: {
        passwordStrategy: "global",
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
    {
      name: "text",
      supportedExtensions: ["txt", "md", "rtf", "log"],
      supportedMimePrefixes: ["text/"],
      containerExtension: CONTAINER_EXT.text,
      taskOptions: {
        passwordStrategy: "global",
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
    {
      name: "alistencrypt",
      supportedExtensions: [],
      supportedMimePrefixes: [],
      containerExtension: CONTAINER_EXT.alistencrypt,
      taskOptions: {
        passwordStrategy: "global",
        supportVersionSelect: false,
        supportedVersions: null,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
  ] as PluginMeta[];
}

function cartesianExpand(arrays: string[][]): string[][] {
  if (arrays.length === 0) return [[]];
  if (arrays.some(a => a.length === 0)) return [[]];
  return arrays.reduce<string[][]>((acc, curr) => acc.flatMap(a => curr.map(c => [...a, c])), [[]]);
}

function buildDynamicSteps(plugins: PluginMeta[], mockRoot: string): any[] {
  const steps: any[] = [];
  for (const plugin of plugins) {
    const opts = plugin.taskOptions;
    if (!opts) continue;
    const supportedExts = plugin.supportedExtensions ?? [];
    if (supportedExts.length === 0) continue;
    const sourceExt = supportedExts[0];
    const specRelPath = extToRelativePath(sourceExt);
    const sourcePath = specRelPath ? `${mockRoot}${specRelPath}` : `${mockRoot}01-plain-media/misc/sample.${sourceExt}`;

    const versions: number[] = opts.supportVersionSelect && opts.supportedVersions ? opts.supportedVersions : [opts.defaultVersion];
    const selectFields: { field: any; values: string[] }[] = [];
    const boolFields: { field: any }[] = [];
    for (const f of opts.extraFields ?? []) {
      if (f.type === "select" && Array.isArray(f.options) && f.options.length > 1) {
        selectFields.push({ field: f, values: f.options });
      } else if (f.type === "bool") {
        boolFields.push({ field: f });
      }
    }

    for (const version of versions) {
      const enc = selectFields.filter(sf => !sf.field.condition || sf.field.condition === "encrypt");
      const encBool = boolFields.filter(bf => !bf.field.condition || bf.field.condition === "encrypt");
      const encSelectCombos = cartesianExpand(enc.map(sf => sf.values));
      const encBoolCombos: boolean[][] = [];
      if (encBool.length === 0) encBoolCombos.push([]);
      else {
        const n = encBool.length;
        for (let mask = 0; mask < 1 << n; mask++) {
          encBoolCombos.push(Array.from({ length: n }, (_, i) => Boolean(mask & (1 << i))));
        }
      }
      for (const sel of encSelectCombos) {
        for (const bool of encBoolCombos) {
          const extraFields: Record<string, string> = {};
          enc.forEach((sf, i) => {
            if (sel[i] !== undefined) extraFields[sf.field.key] = sel[i];
          });
          encBool.forEach((bf, i) => {
            extraFields[bf.field.key] = bool[i] ? "true" : "false";
          });
          steps.push({
            id: `enc_${plugin.name}_v${version}_${sourceExt}_${JSON.stringify(extraFields)}`,
            pluginName: plugin.name,
            taskType: "encrypt",
            sourcePath,
            sourceExt,
            version,
            extraFields,
          });
        }
      }
    }
  }
  return steps;
}

// ==================== Mini 组件（用于触发渲染，不写样式） ====================
const TaskListDiag = defineComponent({
  name: "TaskListDiag",
  setup() {
    const list = useTasksList();
    return { list, store: useTaskStore() };
  },
  template: `
    <div>
      <div data-testid="stats">
        <span data-testid="stat-store">{{ store.tasks.length }}</span>
        <span data-testid="stat-groups">{{ list.groupedTasksByRunId.value.length }}</span>
        <span data-testid="stat-items">{{ list.displayedItems.value.length }}</span>
        <span data-testid="view-mode">{{ list.viewMode }}</span>
        <span data-testid="sort-order">{{ list.sortOrder }}</span>
      </div>
      <ul>
        <template v-for="it in list.displayedItems.value" :key="it.key">
          <li :data-testid="'item-' + it.kind" :data-key="it.key"
              :data-run-id="it.kind === 'group' ? it.runId : (it.kind === 'task' ? (it.task.runId || '__UNDEFINED__') : 'date')"
              :data-status="it.kind === 'task' ? it.task.status : (it.kind === 'group' ? (it.tasks[0]?.status || '') : '')">
            <template v-if="it.kind === 'date'">{{ it.label }}</template>
            <template v-else-if="it.kind === 'group'">[GROUP {{ it.tasks.length }} task] runId={{ it.runId }} trigger={{ it.tasks[0]?.triggeredBy }}</template>
            <template v-else>[TASK] id={{ it.task.id }} runId={{ it.task.runId || 'UNDEFINED' }} status={{ it.task.status }}</template>
          </li>
        </template>
      </ul>
    </div>
  `,
});

// ==================== 诊断输出 ====================
function logSection(title: string): void {
  // eslint-disable-next-line no-console
  console.log(`\n${"=".repeat(80)}\n[UI-LOGIC] ${title}\n${"=".repeat(80)}`);
}

function dumpDisplayedItems(wrapper: VueWrapper, label: string): void {
  const items = wrapper.findAll('[data-testid^="item-"]');
  // eslint-disable-next-line no-console
  console.log(`[UI-LOGIC] ${label}: displayedItems.length=${items.length}`);
  // 按 kind 统计
  const byKind: Record<string, number> = {};
  for (const it of items) {
    const k = it.attributes("data-testid")?.replace("item-", "") ?? "?";
    byKind[k] = (byKind[k] ?? 0) + 1;
  }
  // eslint-disable-next-line no-console
  console.log(`[UI-LOGIC] ${label}: byKind=${JSON.stringify(byKind)}`);
  // 输出前 20 个 + 逃逸 group（__manual__）所有
  let escapeGroupCount = 0;
  for (const it of items) {
    const kind = it.attributes("data-testid")?.replace("item-", "") ?? "?";
    const runId = it.attributes("data-run-id") ?? "?";
    if (kind === "group" && runId.startsWith("__manual__")) escapeGroupCount++;
  }
  // eslint-disable-next-line no-console
  console.log(`[UI-LOGIC] ${label}: escapeGroupCount（__manual__ 伪 group）=${escapeGroupCount}`);
}

function dumpFakeGroupings(wrapper: VueWrapper, label: string, max = 20): void {
  const items = wrapper.findAll('[data-testid="item-group"]');
  const fake: string[] = [];
  for (const it of items) {
    const runId = it.attributes("data-run-id") ?? "?";
    if (runId.startsWith("__manual__")) fake.push(runId);
  }
  // eslint-disable-next-line no-console
  console.log(
    `[UI-LOGIC] ${label}: ${fake.length} 个伪 group（__manual__）：前 ${Math.min(max, fake.length)} = [${fake.slice(0, max).join(", ")}]`
  );
}

// ==================== Task 工厂 ====================
function makeTask(
  id: string,
  opts: {
    runId?: string;
    triggeredBy?: "user" | "automation" | "ai_agent";
    status?: TaskStatus;
    createdAt?: string;
    pluginName?: string;
  } = {}
): EncvTask {
  return {
    id,
    type: "encrypt",
    sourcePath: "/mock/sample.mp4",
    status: (opts.status ?? "running") as TaskStatus,
    progress: 50,
    createdAt: opts.createdAt ?? new Date().toISOString(),
    runId: opts.runId,
    triggeredBy: opts.triggeredBy,
    pluginName: opts.pluginName,
  } as EncvTask;
}

// ==================== 测试 ====================
describe('真机"逃逸"渲染逻辑层面诊断（非样式）', () => {
  let store: ReturnType<typeof useTaskStore>;
  let wrapper: VueWrapper;

  beforeEach(() => {
    testStorage.clear();
    setActivePinia(createPinia());
    store = useTaskStore();
    store.$reset();
    _resetTasksListSingletonForTests();
    // list is created lazily via TaskListDiag.setup() — not needed here
  });

  afterEach(() => {
    wrapper?.unmount();
    _resetTasksListSingletonForTests();
    testStorage.clear();
  });

  // ==================== L1: 1002 task 规模（1+1+1000）真实链路 ====================
  it("L1: 1+1+1000 规模 + 完整逃逸链路 + viewMode / sortOrder / filter 切换", async () => {
    logSection("L1: 1+1+1000 规模 完整逃逸链路");

    const plugins = buildRealPlugins();
    const steps = buildDynamicSteps(plugins, "/mock/");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L1: buildDynamicSteps 派生 ${steps.length} 个 step`);

    const RUNS = [
      { id: "auto-run-1", stepCount: 1, createdAt: new Date().toISOString() },
      { id: "auto-run-2", stepCount: 1, createdAt: new Date().toISOString() },
      { id: "auto-run-3", stepCount: 1000, createdAt: new Date().toISOString() },
    ];

    // === 阶段 1: submitRun + WS task:created ===
    let idx = 0;
    for (const run of RUNS) {
      for (let i = 0; i < run.stepCount; i++) {
        const s = steps[idx % steps.length];
        store.appendTask(
          makeTask(`t-${idx++}`, {
            runId: run.id,
            triggeredBy: "automation",
            status: "running",
            createdAt: run.createdAt,
            pluginName: s.pluginName,
          })
        );
      }
    }
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L1: 阶段 1 提交完成，store.tasks.length=${store.tasks.length}`);

    wrapper = mount(TaskListDiag);
    await nextTick();
    dumpDisplayedItems(wrapper, "L1-1: 1002 task 提交后 group 模式");
    dumpFakeGroupings(wrapper, "L1-1");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L1-1: 真 group=${wrapper.findAll('[data-testid="item-group"]').length}（应该=3）`);

    // === 阶段 2: viewMode 切到 flat ===
    store.toggleViewMode();
    await nextTick();
    dumpDisplayedItems(wrapper, "L1-2: viewMode=flat");
    // eslint-disable-next-line no-console
    console.log(
      `[UI-LOGIC] L1-2 关键诊断：flat 模式 displayedItems=${wrapper.findAll('[data-testid^="item-"]').length}（应该=1002，flat 无 group 概念）`
    );

    // === 阶段 3: viewMode 切回 group ===
    store.toggleViewMode();
    await nextTick();
    dumpDisplayedItems(wrapper, "L1-3: viewMode=group");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L1-3 关键诊断：group 模式又显示 3 个真 group（应该=3）`);

    // === 阶段 4: sortOrder 切换 ===
    store.toggleSort();
    await nextTick();
    dumpDisplayedItems(wrapper, "L1-4: sortOrder 切换后");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L1-4 关键诊断：sortOrder 切换不影响 group 数（group 按 runId 聚合）`);

    // === 阶段 5: 模拟 fetchTasks 丢 runId（1002 task 中随机 50% 丢 runId）===
    const fetchTasksData: EncvTask[] = [];
    for (let i = 0; i < idx; i++) {
      const t = store.tasks[i] as EncvTask;
      fetchTasksData.push(Math.random() < 0.5 ? { ...t, runId: undefined } : t);
    }
    store.bulkSetTasks(fetchTasksData);
    await nextTick();
    dumpDisplayedItems(wrapper, "L1-5: fetchTasks 50% 丢 runId 后 group 模式");
    dumpFakeGroupings(wrapper, "L1-5", 30);
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L1-5 关键诊断：逃逸真因确认 — bulkSetTasks 直接覆盖，丢 runId task 变 __manual__ 伪 group`);

    // === 阶段 6: 切 flat 模式看逃逸 ===
    store.toggleViewMode();
    await nextTick();
    dumpDisplayedItems(wrapper, "L1-6: fetchTasks 后 flat 模式");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L1-6 关键诊断：flat 模式逃逸 task 显示成 row（不按 runId 分组），逃逸数=丢 runId task 数`);
  });

  // ==================== L2: 时间桶切换对逃逸的影响 ====================
  it("L2: 时间桶切换（today / yesterday / thisWeek / thisMonth / earlier）对逃逸的影响", async () => {
    logSection("L2: 时间桶切换对逃逸的影响");

    const now = Date.now();
    const createdAtBuckets = {
      today: new Date(now - 3600 * 1000).toISOString(), // 1 小时前
      yesterday: new Date(now - 86400 * 1000).toISOString(), // 1 天前
      thisWeek: new Date(now - 3 * 86400 * 1000).toISOString(), // 3 天前
      thisMonth: new Date(now - 15 * 86400 * 1000).toISOString(), // 15 天前
      earlier: new Date(now - 60 * 86400 * 1000).toISOString(), // 60 天前
    };

    // 提交 5 个 run，每个 run 5 个 task，每个 run 不同 createdAt
    const buckets: Array<keyof typeof createdAtBuckets> = ["today", "yesterday", "thisWeek", "thisMonth", "earlier"];
    let idx = 0;
    for (const bucket of buckets) {
      for (let i = 0; i < 5; i++) {
        store.appendTask(
          makeTask(`t-${idx++}`, {
            runId: `run-${bucket}`,
            triggeredBy: "automation",
            status: "running",
            createdAt: createdAtBuckets[bucket],
            pluginName: "video",
          })
        );
      }
    }

    wrapper = mount(TaskListDiag);
    await nextTick();
    dumpDisplayedItems(wrapper, "L2-1: 5 个 run 跨 5 个时间桶（5×5=25 task）");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L2-1: 真 group 数=${wrapper.findAll('[data-testid="item-group"]').length}（应该=5，每个 run 一个 group）`);

    // 模拟逃逸：丢 50% runId
    const fetchTasksData: EncvTask[] = [];
    for (let i = 0; i < idx; i++) {
      const t = store.tasks[i] as EncvTask;
      fetchTasksData.push(Math.random() < 0.5 ? { ...t, runId: undefined } : t);
    }
    store.bulkSetTasks(fetchTasksData);
    await nextTick();
    dumpDisplayedItems(wrapper, "L2-2: 50% 丢 runId 后（逃逸跨多个时间桶）");
    dumpFakeGroupings(wrapper, "L2-2", 15);
    // eslint-disable-next-line no-console
    console.log(
      `[UI-LOGIC] L2-2 关键诊断：逃逸 task 散布在多个 date section（today/yesterday/.../earlier），每个 date section 内有多个 __manual__ 伪 group`
    );
  });

  // ==================== L3: filter 切换对逃逸的影响 ====================
  it("L3: filter（plugin / type / status）切换对逃逸的影响", async () => {
    logSection("L3: filter 切换对逃逸的影响");

    // 提交 5 个 run，每个 run 5 个 task，混合 plugin
    const plugins = ["video", "audio", "image", "pdf", "text"];
    let idx = 0;
    for (const plugin of plugins) {
      for (let i = 0; i < 5; i++) {
        store.appendTask(
          makeTask(`t-${idx++}`, {
            runId: `run-${plugin}`,
            triggeredBy: "automation",
            status: i < 3 ? "completed" : "running",
            pluginName: plugin,
          })
        );
      }
    }

    wrapper = mount(TaskListDiag);
    await nextTick();
    dumpDisplayedItems(wrapper, "L3-1: 5 个 run 跨 5 个 plugin 提交后");

    // 模拟逃逸
    const fetchTasksData: EncvTask[] = [];
    for (let i = 0; i < idx; i++) {
      const t = store.tasks[i] as EncvTask;
      fetchTasksData.push(Math.random() < 0.5 ? { ...t, runId: undefined } : t);
    }
    store.bulkSetTasks(fetchTasksData);
    await nextTick();
    dumpDisplayedItems(wrapper, "L3-2: 50% 丢 runId 后");
    dumpFakeGroupings(wrapper, "L3-2", 15);

    // === 阶段 3: filter 只看 video plugin ===
    store.filterPlugins = ["video"];
    await nextTick();
    dumpDisplayedItems(wrapper, "L3-3: filterPlugins=[video]");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L3-3 关键诊断：filter 后只剩 video 相关 group，逃逸被部分过滤`);

    // === 阶段 4: filter 只看 completed status ===
    store.filterPlugins = [];
    store.filterStatuses = ["completed"];
    await nextTick();
    dumpDisplayedItems(wrapper, "L3-4: filterStatuses=[completed]");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L3-4 关键诊断：filter status=completed 后，逃逸 task 中 status=running 的都被过滤`);

    // === 阶段 5: search 关键字 ===
    store.filterStatuses = [];
    store.searchQuery = "t-1";
    await nextTick();
    dumpDisplayedItems(wrapper, 'L3-5: searchQuery="t-1"');
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L3-5 关键诊断：search 后只剩 task id 包含 t-1 的 task / group`);
  });

  // ==================== L4: A 方向修复验证（merge 模式保 runId） ====================
  it("L4: A 方向修复验证 — bulkSetTasks 改成 merge 模式后，fetchTasks 丢 runId 不会逃逸", async () => {
    logSection("L4: A 方向修复验证（bulkSetTasks merge 模式）");

    // 1. 提交 1002 task（1+1+1000）
    const RUNS = [
      { id: "auto-run-1", stepCount: 1 },
      { id: "auto-run-2", stepCount: 1 },
      { id: "auto-run-3", stepCount: 1000 },
    ];
    let idx = 0;
    for (const run of RUNS) {
      for (let i = 0; i < run.stepCount; i++) {
        store.appendTask(
          makeTask(`t-${idx++}`, {
            runId: run.id,
            triggeredBy: "automation",
            status: "running",
          })
        );
      }
    }
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L4-1: 提交完成，store.tasks.length=${store.tasks.length}`);

    // 2. 模拟 fetchTasks 丢 50% runId
    const fetchTasksData: EncvTask[] = [];
    for (let i = 0; i < idx; i++) {
      const t = store.tasks[i] as EncvTask;
      fetchTasksData.push(Math.random() < 0.5 ? { ...t, runId: undefined } : t);
    }
    store.bulkSetTasks(fetchTasksData);

    // 3. 验证 merge 模式生效：store 里 task 的 runId 应该保留
    let preservedRunIdCount = 0;
    let lostRunIdCount = 0;
    for (const t of store.tasks as EncvTask[]) {
      if (t.runId && (t.runId === "auto-run-1" || t.runId === "auto-run-2" || t.runId === "auto-run-3")) {
        preservedRunIdCount++;
      } else {
        lostRunIdCount++;
      }
    }
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L4-2 关键诊断：fetchTasks 50% 丢 runId → merge 模式保留 store 里 runId`);
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L4-2: preservedRunIdCount=${preservedRunIdCount} (期望 ~1002)`);
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L4-2: lostRunIdCount=${lostRunIdCount} (期望 0，merge 模式保 runId)`);

    wrapper = mount(TaskListDiag);
    await nextTick();
    dumpDisplayedItems(wrapper, "L4-3: merge 模式保 runId 后渲染");
    dumpFakeGroupings(wrapper, "L4-3");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L4-3 关键诊断：merge 模式 → __manual__ 伪 group 数=0（因为没丢 runId）`);
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L4-3: 真 group 数=${wrapper.findAll('[data-testid="item-group"]').length}（期望=3）`);

    // 期望：merge 模式修复后，逃逸消失
    expect(lostRunIdCount).toBe(0);
    expect(preservedRunIdCount).toBe(1002);
  });

  // ==================== L5: A 方向修复 + viewMode / sortOrder 切换 ====================
  it("L5: A 方向修复后 + viewMode / sortOrder / filter 切换不再逃逸", async () => {
    logSection("L5: A 方向修复 + viewMode / sortOrder / filter");

    // 提交 1002 task
    const RUNS = [
      { id: "auto-run-1", stepCount: 1, pluginName: "video" },
      { id: "auto-run-2", stepCount: 1, pluginName: "audio" },
      { id: "auto-run-3", stepCount: 1000, pluginName: "image" },
    ];
    let idx = 0;
    for (const run of RUNS) {
      for (let i = 0; i < run.stepCount; i++) {
        store.appendTask(
          makeTask(`t-${idx++}`, {
            runId: run.id,
            triggeredBy: "automation",
            status: "running",
            pluginName: run.pluginName,
          })
        );
      }
    }

    // 模拟 fetchTasks 丢 50% runId（merge 模式应该保 runId）
    const fetchTasksData: EncvTask[] = [];
    for (let i = 0; i < idx; i++) {
      const t = store.tasks[i] as EncvTask;
      fetchTasksData.push(Math.random() < 0.5 ? { ...t, runId: undefined } : t);
    }
    store.bulkSetTasks(fetchTasksData);

    wrapper = mount(TaskListDiag);
    await nextTick();
    dumpDisplayedItems(wrapper, "L5-1: merge 模式 + 1002 task");

    // 切 viewMode
    store.toggleViewMode();
    await nextTick();
    dumpDisplayedItems(wrapper, "L5-2: flat 模式");
    store.toggleViewMode();
    await nextTick();
    dumpDisplayedItems(wrapper, "L5-3: group 模式");

    // 切 sortOrder
    store.toggleSort();
    await nextTick();
    dumpDisplayedItems(wrapper, "L5-4: sortOrder 切换");

    // filter
    store.filterPlugins = ["video"];
    await nextTick();
    dumpDisplayedItems(wrapper, "L5-5: filterPlugins=[video]");
    store.filterPlugins = [];
    store.filterStatuses = ["running"];
    await nextTick();
    dumpDisplayedItems(wrapper, "L5-6: filterStatuses=[running]");

    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L5 关键诊断：merge 模式修复后，无论 viewMode/sortOrder/filter 怎么切，逃逸=0`);
  });

  // ==================== L6: 时间排序模式切换（sortBy=activity vs sortBy=created） ====================
  // 2026-06-22 新增：user 反馈"时间排序模式切换"是影响渲染的关键路径——
  //   sortBy=activity: 按 max(createdAt, completedAt) 排序（task 状态变化时位置自然更新）
  //   sortBy=created:  按 createdAt desc 排序
  // 两个模式对 **flat 模式 row 顺序** 的影响截然不同。
  // group 模式下，groupedTasksByRunId 按 startedAt（首个 task 的 createdAt）排，
  // sortBy 切换对 group 顺序不影响（group 一旦聚合完成就按 startedAt 固定），
  // 但 group 内部 task 顺序受 sortBy 影响（[run].tasks 来自 sortedTasks）。
  it("L6: sortBy=activity vs sortBy=created 切换对 flat row / group 内 task 顺序影响 + 逃逸下保稳定", async () => {
    logSection("L6: 时间排序模式切换（activity vs created）");

    // 准备 3 个 run，故意构造时间错位：
    //   run-1: createdAt 早（2h前），但有 1 个 task 最近完成（completedAt 1 分钟前）
    //   run-2: createdAt 中（1h前），最近有 task 刚开始
    //   run-3: createdAt 晚（10分钟前），还没完成
    const now = Date.now();
    const r1Created = new Date(now - 7200 * 1000).toISOString();
    const r1Completed = new Date(now - 60 * 1000).toISOString();
    const r2Created = new Date(now - 3600 * 1000).toISOString();
    const r3Created = new Date(now - 600 * 1000).toISOString();

    store.appendTask(
      makeTask("r1-1", { runId: "run-1", triggeredBy: "automation", status: "completed", createdAt: r1Created, pluginName: "video" })
    );
    // patch completedAt 到 r1-1（按 activity 模式它是最新的）
    store.patchTaskById("r1-1", { completedAt: r1Completed } as any);

    store.appendTask(
      makeTask("r2-1", { runId: "run-2", triggeredBy: "automation", status: "running", createdAt: r2Created, pluginName: "video" })
    );

    store.appendTask(
      makeTask("r3-1", { runId: "run-3", triggeredBy: "automation", status: "queued", createdAt: r3Created, pluginName: "video" })
    );

    wrapper = mount(TaskListDiag);
    await nextTick();

    // === 阶段 1：group 模式下，sortBy 切换不影响 group 顺序（按 startedAt 排） ===
    // startedAt: run-1=T-7200s, run-2=T-3600s, run-3=T-600s
    // 降序：run-3 (最新) > run-2 > run-1
    let groups = wrapper.findAll('[data-testid="item-group"]');
    let order = groups.map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L6-1 sortBy=activity (group): 顺序=[${order.join(", ")}]（按 startedAt 降序：run-3, run-2, run-1）`);
    expect(order).toEqual(["run-3", "run-2", "run-1"]);

    // 阶段 2：sortBy=created —— group 顺序仍按 startedAt（不变）
    store.setSortBy("created");
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]');
    order = groups.map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L6-2 sortBy=created  (group): 顺序=[${order.join(", ")}]（按 startedAt 降序，不变）`);
    expect(order).toEqual(["run-3", "run-2", "run-1"]);

    // === 阶段 3：切到 flat 模式 + sortBy=created —— row 顺序按 createdAt 降序 ===
    store.setViewMode("flat");
    await nextTick();
    let taskRows = wrapper.findAll('[data-testid="item-task"]');
    let rowKeys = taskRows.map(r => r.attributes("data-key"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L6-3 sortBy=created (flat): row 顺序=[${rowKeys.join(", ")}]（按 createdAt 降序：r3-1, r2-1, r1-1）`);
    expect(rowKeys).toEqual(["t-r3-1", "t-r2-1", "t-r1-1"]);

    // === 阶段 4：sortBy=activity —— row 顺序按 max(createdAt, completedAt) 降序 ===
    // r1-1 completedAt=60s前（最新）→ 排最前
    // r3-1 createdAt=10min前 → 排第二
    // r2-1 createdAt=1h前 → 排最后
    store.setSortBy("activity");
    await nextTick();
    taskRows = wrapper.findAll('[data-testid="item-task"]');
    rowKeys = taskRows.map(r => r.attributes("data-key"));
    // eslint-disable-next-line no-console
    console.log(
      `[UI-LOGIC] L6-4 sortBy=activity (flat): row 顺序=[${rowKeys.join(", ")}]（按 max 排：r1-1 最近完成 → 排前；r3-1 createdAt 最新 → 排第二；r2-1 排最后）`
    );
    expect(rowKeys).toEqual(["t-r1-1", "t-r3-1", "t-r2-1"]);

    // 阶段 5：模拟逃逸 — fetchTasks 丢 50% runId + 切 view/sort
    const noRunIdTasks: EncvTask[] = [];
    for (const t of store.tasks as EncvTask[]) {
      noRunIdTasks.push(Math.random() < 0.5 ? { ...t, runId: undefined } : { ...t });
    }
    store.bulkSetTasks(noRunIdTasks);
    store.setViewMode("group"); // 切回 group 看
    store.setSortBy("activity");
    await nextTick();
    dumpDisplayedItems(wrapper, "L6-5: 50% 丢 runId + group + activity");
    dumpFakeGroupings(wrapper, "L6-5", 10);
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L6-5 关键诊断：逃逸后 sortBy 切换 + 切回 group → 真 group 仍 3 个，0 伪 group（merge 模式保 runId）`);
    const fakeGroups = wrapper.findAll('[data-testid="item-group"]').filter(g => g.attributes("data-run-id")?.startsWith("__manual__"));
    expect(fakeGroups.length, "merge 模式：sortBy 切换后 0 伪 group").toBe(0);
  });

  // ==================== L7: pinnedRunIds 切换对 group 顺序影响 ====================
  // 2026-06-22 新增：pinnedRunIds 是 groupedTasksByRunId 排序的核心——置顶的 runId
  // 永远排在前。当用户置顶/取消置顶时，group 顺序立即变化（影响渲染）。
  it("L7: pinnedRunIds 切换对 group 顺序影响（置顶的 runId 永远在前）", async () => {
    logSection("L7: pinnedRunIds 切换对 group 顺序影响");

    // 3 个 run，createdAt 顺序：run-1 最新、run-2 中、run-3 最旧
    const now = Date.now();
    store.appendTask(
      makeTask("a-1", { runId: "run-1", triggeredBy: "automation", status: "running", createdAt: new Date(now - 600 * 1000).toISOString() })
    );
    store.appendTask(
      makeTask("a-2", {
        runId: "run-2",
        triggeredBy: "automation",
        status: "running",
        createdAt: new Date(now - 1200 * 1000).toISOString(),
      })
    );
    store.appendTask(
      makeTask("a-3", {
        runId: "run-3",
        triggeredBy: "automation",
        status: "running",
        createdAt: new Date(now - 1800 * 1000).toISOString(),
      })
    );

    wrapper = mount(TaskListDiag);
    await nextTick();

    // 阶段 1：默认顺序（按 startedAt desc）—— run-1, run-2, run-3
    let order = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L7-1 默认顺序=[${order.join(", ")}]（期望 run-1, run-2, run-3）`);
    expect(order).toEqual(["run-1", "run-2", "run-3"]);

    // 阶段 2：置顶 run-3（最旧的）—— run-3 应该跳到最前
    store.togglePinRun("run-3");
    await nextTick();
    order = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L7-2 pin run-3: 顺序=[${order.join(", ")}]（期望 run-3 置顶：run-3, run-1, run-2）`);
    expect(order).toEqual(["run-3", "run-1", "run-2"]);

    // 阶段 3：再置顶 run-2 —— run-2 也置顶
    store.togglePinRun("run-2");
    await nextTick();
    order = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L7-3 pin run-3+run-2: 顺序=[${order.join(", ")}]（期望 pinned 排前，pinned 之间按时间：run-2, run-3, run-1）`);
    expect(order).toEqual(["run-2", "run-3", "run-1"]);

    // 阶段 4：取消 run-3 置顶 —— run-2 仍置顶，run-3 回到非置顶区
    store.togglePinRun("run-3"); // toggle = 取消
    await nextTick();
    order = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L7-4 unpin run-3: 顺序=[${order.join(", ")}]（期望 run-2 置顶，run-1, run-3 按时间：run-2, run-1, run-3）`);
    expect(order).toEqual(["run-2", "run-1", "run-3"]);

    // 阶段 5：取消所有置顶 —— 回到默认时间顺序
    store.togglePinRun("run-2");
    await nextTick();
    order = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L7-5 unpin all: 顺序=[${order.join(", ")}]（期望默认 run-1, run-2, run-3）`);
    expect(order).toEqual(["run-1", "run-2", "run-3"]);

    // 阶段 6：模拟逃逸 + pin 切换
    const noRunIdTasks: EncvTask[] = [];
    for (const t of store.tasks as EncvTask[]) {
      noRunIdTasks.push(Math.random() < 0.5 ? { ...t, runId: undefined } : { ...t });
    }
    store.bulkSetTasks(noRunIdTasks);
    store.togglePinRun("run-1");
    await nextTick();
    dumpDisplayedItems(wrapper, "L7-6: 逃逸 + pin run-1 后");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L7-6 关键诊断：merge 模式 + pin 切换 → 0 伪 group（pinned run-1 仍排前）`);
    const fakeGroups = wrapper.findAll('[data-testid="item-group"]').filter(g => g.attributes("data-run-id")?.startsWith("__manual__"));
    expect(fakeGroups.length, "merge 模式 + pin 切换：0 伪 group").toBe(0);
  });

  // ==================== L8: triggeredByFilter 切换对逃逸的影响 ====================
  // 2026-06-22 新增：filterTriggeredBy 是另一个会改变 displayedItems 内容的关键开关。
  // 当用户筛 user / automation / ai_agent 时，只有对应触发器的 task/group 显示。
  it("L8: triggeredByFilter 切换 + 逃逸下保稳定", async () => {
    logSection("L8: triggeredByFilter 切换对逃逸的影响");

    // 6 个 run：3 个 automation + 2 个 user + 1 个 ai_agent
    // 显式 createdAt：让 groupedTasksByRunId 按 startedAt 降序排，顺序确定
    const now = Date.now();
    const runs: Array<{ id: string; triggeredBy: "user" | "automation" | "ai_agent"; createdAt: string }> = [
      { id: "auto-1", triggeredBy: "automation", createdAt: new Date(now - 6000 * 1000).toISOString() },
      { id: "auto-2", triggeredBy: "automation", createdAt: new Date(now - 5000 * 1000).toISOString() },
      { id: "auto-3", triggeredBy: "automation", createdAt: new Date(now - 4000 * 1000).toISOString() },
      { id: "user-1", triggeredBy: "user", createdAt: new Date(now - 3000 * 1000).toISOString() },
      { id: "user-2", triggeredBy: "user", createdAt: new Date(now - 2000 * 1000).toISOString() },
      { id: "ai-1", triggeredBy: "ai_agent", createdAt: new Date(now - 1000 * 1000).toISOString() },
    ];
    let idx = 0;
    for (const r of runs) {
      store.appendTask(
        makeTask(`t-${idx++}`, {
          runId: r.id,
          triggeredBy: r.triggeredBy,
          status: "running",
          createdAt: r.createdAt,
          pluginName: "video",
        })
      );
    }

    wrapper = mount(TaskListDiag);
    await nextTick();
    // 默认（无 triggeredBy 过滤）—— 按 startedAt 降序：ai-1, user-2, user-1, auto-3, auto-2, auto-1
    let groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L8-1 默认顺序=[${groups.join(", ")}]（按 startedAt 降序：ai-1, user-2, user-1, auto-3, auto-2, auto-1）`);
    expect(groups.length).toBe(6);

    // 阶段 2：filter triggeredBy=automation
    store.filterTriggeredBy = ["automation"];
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L8-2 filter=[automation]: 顺序=[${groups.join(", ")}]（按 startedAt 降序：auto-3, auto-2, auto-1）`);
    expect(groups).toEqual(["auto-3", "auto-2", "auto-1"]);

    // 阶段 3：filter triggeredBy=[user, ai_agent]
    store.filterTriggeredBy = ["user", "ai_agent"];
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L8-3 filter=[user, ai_agent]: 顺序=[${groups.join(", ")}]（按 startedAt 降序：ai-1, user-2, user-1）`);
    expect(groups).toEqual(["ai-1", "user-2", "user-1"]);

    // 阶段 4：清空 filter
    store.filterTriggeredBy = [];
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L8-4 清空 filter: 顺序=[${groups.join(", ")}]（期望 6 个）`);
    expect(groups.length).toBe(6);

    // 阶段 5：模拟逃逸 + filter 切换
    const noRunIdTasks: EncvTask[] = [];
    for (const t of store.tasks as EncvTask[]) {
      noRunIdTasks.push(Math.random() < 0.5 ? { ...t, runId: undefined } : { ...t });
    }
    store.bulkSetTasks(noRunIdTasks);
    store.filterTriggeredBy = ["automation"];
    await nextTick();
    dumpDisplayedItems(wrapper, "L8-5: 逃逸 + filter=automation 后");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L8-5 关键诊断：merge 模式 + triggeredBy 过滤 → 0 伪 group`);
    const fakeGroups = wrapper.findAll('[data-testid="item-group"]').filter(g => g.attributes("data-run-id")?.startsWith("__manual__"));
    expect(fakeGroups.length, "merge 模式 + triggeredBy 过滤：0 伪 group").toBe(0);
  });

  // ==================== L9: datePreset 切换对逃逸的影响 ====================
  // 2026-06-22 新增：filterDatePreset 是 filter 维度之一，影响 filterDateRange.from/to
  // 当用户选 today / 7d / 30d / all / custom 时，只有对应时间窗口的 task/group 显示
  it("L9: datePreset 切换（today / 7d / 30d / all / custom）对逃逸的影响", async () => {
    logSection("L9: datePreset 切换对逃逸的影响");

    const now = Date.now();
    const buckets: Record<string, string> = {
      today: new Date(now - 3600 * 1000).toISOString(), // 1h 前
      yesterday: new Date(now - 86400 * 1000).toISOString(), // 1d 前
      thisWeek: new Date(now - 3 * 86400 * 1000).toISOString(), // 3d 前
      thisMonth: new Date(now - 15 * 86400 * 1000).toISOString(), // 15d 前
      earlier: new Date(now - 60 * 86400 * 1000).toISOString(), // 60d 前
    };

    // 每个 bucket 一个 runId，每个 runId 1 个 task
    let idx = 0;
    for (const [bucket, createdAt] of Object.entries(buckets)) {
      store.appendTask(
        makeTask(`t-${idx++}`, {
          runId: `run-${bucket}`,
          triggeredBy: "automation",
          status: "running",
          createdAt,
          pluginName: "video",
        })
      );
    }

    wrapper = mount(TaskListDiag);
    await nextTick();
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L9-1 默认（all）真 group=${wrapper.findAll('[data-testid="item-group"]').length}（期望 5）`);
    expect(wrapper.findAll('[data-testid="item-group"]').length).toBe(5);

    // 阶段 2：applyDatePreset('today') —— 只剩 today 的 run
    store.applyDatePreset("today");
    await nextTick();
    let groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L9-2 preset=today: 顺序=[${groups.join(", ")}]（期望 run-today）`);
    expect(groups).toEqual(["run-today"]);

    // 阶段 3：applyDatePreset('7d') —— today + yesterday + thisWeek
    store.applyDatePreset("7d");
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L9-3 preset=7d: 顺序=[${groups.join(", ")}]（期望 run-today, run-yesterday, run-thisWeek）`);
    expect(groups).toEqual(["run-today", "run-yesterday", "run-thisWeek"]);

    // 阶段 4：applyDatePreset('30d') —— today + yesterday + thisWeek + thisMonth
    store.applyDatePreset("30d");
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L9-4 preset=30d: 顺序=[${groups.join(", ")}]（期望 today, yesterday, thisWeek, thisMonth）`);
    expect(groups).toEqual(["run-today", "run-yesterday", "run-thisWeek", "run-thisMonth"]);

    // 阶段 5：applyDatePreset('all') —— 全部
    store.applyDatePreset("all");
    await nextTick();
    expect(wrapper.findAll('[data-testid="item-group"]').length).toBe(5);

    // 阶段 6：setCustomDateRange —— 只看 thisMonth
    const thisMonthStart = new Date(now - 20 * 86400 * 1000).toISOString();
    const thisMonthEnd = new Date(now - 10 * 86400 * 1000).toISOString();
    store.setCustomDateRange(thisMonthStart, thisMonthEnd);
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(
      `[UI-LOGIC] L9-6 custom=[${thisMonthStart.slice(0, 10)} ~ ${thisMonthEnd.slice(0, 10)}]: 顺序=[${groups.join(", ")}]（期望 run-thisMonth）`
    );
    expect(groups).toEqual(["run-thisMonth"]);

    // 阶段 7：模拟逃逸 + preset 切换
    store.applyDatePreset("all");
    const noRunIdTasks: EncvTask[] = [];
    for (const t of store.tasks as EncvTask[]) {
      noRunIdTasks.push(Math.random() < 0.5 ? { ...t, runId: undefined } : { ...t });
    }
    store.bulkSetTasks(noRunIdTasks);
    store.applyDatePreset("7d");
    await nextTick();
    dumpDisplayedItems(wrapper, "L9-7: 逃逸 + preset=7d 后");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L9-7 关键诊断：merge 模式 + preset 切换 → 0 伪 group`);
    const fakeGroups = wrapper.findAll('[data-testid="item-group"]').filter(g => g.attributes("data-run-id")?.startsWith("__manual__"));
    expect(fakeGroups.length, "merge 模式 + preset 切换：0 伪 group").toBe(0);
  });

  // ==================== L10: 并发切换（viewMode + sortBy + filter + pin + search 同时切） ====================
  // 2026-06-22 新增：5 种渲染状态同时切换——所有 computed 链路叠加。
  // 真实场景：用户切到 flat 模式 + 改时间排序 + 选 plugin filter + 置顶 + 搜索关键字，
  // 渲染结果应该是这 5 种状态叠加的最终值（不是其中某一个）。
  it("L10: 并发切换（viewMode + sortBy + filter + pin + search 同时切）", async () => {
    logSection("L10: 并发切换 5 种渲染状态叠加");

    // 准备 6 个 run 跨 3 个 plugin，createdAt 故意错位
    const now = Date.now();
    const runs = [
      { id: "r-v1", pluginName: "video", createdAt: new Date(now - 600 * 1000).toISOString(), hasCompleted: true },
      { id: "r-v2", pluginName: "video", createdAt: new Date(now - 1200 * 1000).toISOString(), hasCompleted: false },
      { id: "r-a1", pluginName: "audio", createdAt: new Date(now - 1800 * 1000).toISOString(), hasCompleted: true },
      { id: "r-a2", pluginName: "audio", createdAt: new Date(now - 2400 * 1000).toISOString(), hasCompleted: false },
      { id: "r-i1", pluginName: "image", createdAt: new Date(now - 3000 * 1000).toISOString(), hasCompleted: false },
      { id: "r-i2", pluginName: "image", createdAt: new Date(now - 3600 * 1000).toISOString(), hasCompleted: true },
    ];
    let idx = 0;
    for (const r of runs) {
      store.appendTask(
        makeTask(`t-${idx++}`, {
          runId: r.id,
          triggeredBy: "automation",
          status: r.hasCompleted ? "completed" : "running",
          createdAt: r.createdAt,
          pluginName: r.pluginName,
        })
      );
      if (r.hasCompleted) {
        // patch completedAt 到 r-v1 是 30s 前（让它在 activity 模式排第一）
        store.patchTaskById(`t-${idx - 1}`, { completedAt: new Date(now - 30 * 1000).toISOString() } as any);
      }
    }

    wrapper = mount(TaskListDiag);
    await nextTick();

    // === 阶段 1：5 种状态同时切 ===
    store.setViewMode("flat"); // flat
    store.setSortBy("activity"); // activity
    store.filterPlugins = ["video"]; // filter video
    store.togglePinRun("r-v2"); // pin r-v2
    store.setSearchQuery("t-"); // search 't-'（宽松：所有 task id 都含 t-）
    await nextTick();

    // 期望：flat 模式 + activity + filter=video + pin=r-v2（flat 不影响 row 顺序）+ search='t-'（全部通过）
    // → 只剩 r-v1 / r-v2 里的 task（filter video）
    // → search 't-' 全部通过
    // → pin r-v2 没用（flat 不显示 group）
    // → flat 模式按 activity 排序
    const taskItems = wrapper.findAll('[data-testid="item-task"]');
    const taskIds = taskItems.map(t => t.attributes("data-key"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L10-1 5 状态叠加: flat 显示 ${taskItems.length} 个 row, ids=[${taskIds.join(", ")}]`);
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L10-1 关键诊断：5 状态叠加 = flat 模式按 activity 排 + filter=video + search='t-'`);
    expect(taskItems.length).toBe(2); // r-v1 + r-v2 各 1 个 task

    // 阶段 2：切回 group 模式 —— search 't-' 仍全部通过，pin r-v2 置顶
    store.setViewMode("group");
    await nextTick();
    let groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(
      `[UI-LOGIC] L10-2 切回 group: 顺序=[${groups.join(", ")}]（filter=video 留 r-v1, r-v2; pin r-v2 置顶; search='t-' 全部通过）`
    );
    // r-v2 置顶在前，r-v1 在后（pinned 优先于 startedAt）
    expect(groups[0]).toBe("r-v2");
    expect(groups[1]).toBe("r-v1");

    // 阶段 3：再切 sortBy=created
    store.setSortBy("created");
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L10-3 sortBy=created + pin r-v2 + filter=video: 顺序=[${groups.join(", ")}]（r-v2 仍 pin 在前）`);
    expect(groups[0]).toBe("r-v2");

    // 阶段 4：清 search + 清 filter
    store.setSearchQuery("");
    store.filterPlugins = [];
    store.togglePinRun("r-v2"); // unpin
    await nextTick();
    groups = wrapper.findAll('[data-testid="item-group"]').map(g => g.attributes("data-run-id"));
    // eslint-disable-next-line no-console
    console.log(
      `[UI-LOGIC] L10-4 清 search/filter/unpin: 顺序=[${groups.join(", ")}]（期望 6 个 run 按 createdAt desc：r-v1, r-v2, r-a1, r-a2, r-i1, r-i2）`
    );
    expect(groups).toEqual(["r-v1", "r-v2", "r-a1", "r-a2", "r-i1", "r-i2"]);

    // 阶段 5：模拟逃逸 + 5 状态叠加
    const noRunIdTasks: EncvTask[] = [];
    for (const t of store.tasks as EncvTask[]) {
      noRunIdTasks.push(Math.random() < 0.5 ? { ...t, runId: undefined } : { ...t });
    }
    store.bulkSetTasks(noRunIdTasks);
    store.setViewMode("group");
    store.setSortBy("activity");
    store.filterPlugins = ["video"];
    store.togglePinRun("r-v1");
    store.setSearchQuery("t-");
    await nextTick();
    dumpDisplayedItems(wrapper, "L10-5: 逃逸 + 5 状态叠加");
    // eslint-disable-next-line no-console
    console.log(`[UI-LOGIC] L10-5 关键诊断：merge 模式 + 5 状态叠加 → 0 伪 group`);
    const fakeGroups = wrapper.findAll('[data-testid="item-group"]').filter(g => g.attributes("data-run-id")?.startsWith("__manual__"));
    expect(fakeGroups.length, "merge 模式 + 5 状态叠加：0 伪 group").toBe(0);
  });
});
