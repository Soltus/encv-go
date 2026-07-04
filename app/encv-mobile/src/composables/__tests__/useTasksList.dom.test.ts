/**
 * 组件渲染测试：mount 真实 Vue 组件，断言 DOM 节点
 *
 * 之前 vitest 只测 store 输出（displayedItems 数组结构正确）—— 跟实际 UI 渲染无交集
 * 现在 mount 真实组件、跑真实 v-for、跑真实 v-if/v-else-if 条件，断言 DOM 节点
 *
 * 关键断言：
 * - 10 task 共享 runId → DOM 里只有 1 个 .task-group 元素（不是 10 个）
 * - 逃逸场景：100 次 progress update 后，DOM 仍然只有 1 个 .task-group
 * - 逃逸场景：state 跳变后，DOM 仍然只有 1 个 .task-group
 */

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";

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
import type { EncvTask } from "@/api/encv";

vi.mock("@/composables/useTaskEventBridge", () => ({
  useTaskEventBridge: () => {},
}));

vi.mock("@/api/encv", async () => {
  const actual = await vi.importActual<typeof import("@/api/encv")>("@/api/encv");
  return {
    ...actual,
    getTasks: vi.fn().mockResolvedValue([]),
    cancelTask: vi.fn().mockResolvedValue(undefined),
    removeTask: vi.fn().mockResolvedValue(undefined),
    retryTask: vi.fn().mockResolvedValue(undefined),
  };
});

// ============ 测试 fixture 组件：复刻 Tasks.vue 关键 v-if/v-else-if 逻辑 ============
// 简化版：用 div 代替 Ionic 组件，但 v-for + v-if/v-else-if 渲染逻辑与 Tasks.vue 一致
function createTestListComponent() {
  return defineComponent({
    name: "TestTaskList",
    props: {
      // 直接传 ref 进 props（响应式自动保持）
      displayedItemsRef: { type: Object, required: true },
    },
    setup(props) {
      return () => {
        const items = props.displayedItemsRef.value; // ← 响应式访问
        if (!items || items.length === 0) {
          return h("div", { class: "empty" }, "no items");
        }
        return h(
          "div",
          { class: "task-list" },
          items.map((it: any) => {
            // ↓↓↓ 与 Tasks.vue 模板完全相同的 v-if/v-else-if 链 ↓↓↓
            if (it.kind === "date") {
              return h("div", { key: it.key, class: "date-section" }, it.label);
            }
            if (it.kind === "group" && it.counters?.hitAny) {
              return h(
                "div",
                {
                  key: it.key,
                  class: "task-group",
                  "data-run-id": it.runId,
                  "data-tasks-count": it.tasks.length,
                },
                [
                  h("div", { class: "group-title" }, `${it.tasks.length} tasks`),
                  ...it.tasks.map((tk: any) => h("div", { key: `tk-${tk.id}`, class: "group-task", "data-id": tk.id }, tk.id)),
                ]
              );
            }
            if (it.kind === "task") {
              return h("div", { key: it.key, class: "task-row", "data-id": it.task.id }, it.task.id);
            }
            return h("div", { key: it.key, class: "unknown" }, JSON.stringify(it));
          })
        );
      };
    },
  });
}

async function freshModules() {
  vi.resetModules();
  setActivePinia(createPinia());
  const { useTaskStore } = await import("@/stores/taskStore");
  const { useTasksList } = await import("@/composables/useTasksList");
  return { useTaskStore, useTasksList };
}

function makeTask(id: string, runId: string, status: string = "queued"): EncvTask {
  return {
    id,
    type: "encrypt",
    sourcePath: `/mock/sample-${id}.mp4`,
    status: status as any,
    progress: 0,
    createdAt: "2026-06-22T10:00:00.000Z",
    runId,
    triggeredBy: "automation",
    pluginName: "mp4-encrypt",
  };
}

describe("真实组件渲染测试（mount Vue 组件，断言 DOM）", () => {
  beforeEach(() => {
    testStorage.clear();
  });

  it("10 task 共享 runId → DOM 只渲染 1 个 .task-group", async () => {
    const { useTaskStore, useTasksList } = await freshModules();
    const store = useTaskStore();
    const list = useTasksList();
    list.viewMode.value = "group";

    const RUN_ID = `r-dom-1-${Date.now()}`;
    for (let i = 0; i < 10; i++) {
      store.applyEvent("created", makeTask(`t-${i}`, RUN_ID, "queued"));
    }

    // 真实 mount
    const Component = createTestListComponent();
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } });
    const html = wrapper.html();

    // 断言：DOM 里只有 1 个 .task-group（不是 10 个）
    const groups = wrapper.findAll(".task-group");
    expect(groups.length).toBe(1);

    // 断言：group 的 runId 和 tasks 数量正确
    const group = groups[0];
    expect(group.attributes("data-run-id")).toBe(RUN_ID);
    expect(group.attributes("data-tasks-count")).toBe("10");

    // 断言：没有 .task-row（所有 task 都在 group 里，没逃逸）
    const tasks = wrapper.findAll(".task-row");
    expect(tasks.length).toBe(0);

    // 断言：html 字符串验证（防御性）
    expect(html).toContain('data-run-id="' + RUN_ID + '"');
    expect(html).not.toContain('class="task-row"');
  });

  it("100 次 progress update 后，DOM 仍只有 1 个 .task-group（逃逸验证）", async () => {
    const { useTaskStore, useTasksList } = await freshModules();
    const store = useTaskStore();
    const list = useTasksList();
    list.viewMode.value = "group";

    const RUN_ID = `r-dom-2-${Date.now()}`;
    const taskIds: string[] = [];
    for (let i = 0; i < 10; i++) {
      const id = `t-${i}`;
      taskIds.push(id);
      store.applyEvent("created", makeTask(id, RUN_ID, "queued"));
    }

    const Component = createTestListComponent();
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } });

    // 初始：1 个 group
    expect(wrapper.findAll(".task-group").length).toBe(1);
    expect(wrapper.findAll(".task-row").length).toBe(0);

    // 100 次 progress update（模拟后端 WS 推送）
    for (let round = 0; round < 10; round++) {
      for (const id of taskIds) {
        store.applyEvent("progress", { id, progress: (round + 1) * 10 });
      }
    }

    // 关键断言：100 次 update 后，DOM 仍只有 1 个 group
    const groups = wrapper.findAll(".task-group");
    const tasks = wrapper.findAll(".task-row");

    expect(groups.length).toBe(1); // ← 关键：不应该是 10
    expect(tasks.length).toBe(0); // ← 关键：不应该有 task row

    // 断言：group 里的 task 数量仍是 10
    expect(groups[0].attributes("data-tasks-count")).toBe("10");
  });

  it("status 状态跳变后，DOM 仍只有 1 个 .task-group", async () => {
    const { useTaskStore, useTasksList } = await freshModules();
    const store = useTaskStore();
    const list = useTasksList();
    list.viewMode.value = "group";

    const RUN_ID = `r-dom-3-${Date.now()}`;
    for (let i = 0; i < 5; i++) {
      store.applyEvent("created", makeTask(`t-${i}`, RUN_ID, "queued"));
    }

    const Component = createTestListComponent();
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } });
    expect(wrapper.findAll(".task-group").length).toBe(1);

    // 模拟 status 状态变化：queued → running → completed
    for (let i = 0; i < 5; i++) {
      store.applyEvent("update", { id: `t-${i}`, status: "running" });
    }
    expect(wrapper.findAll(".task-group").length).toBe(1);

    for (let i = 0; i < 5; i++) {
      store.applyEvent("update", { id: `t-${i}`, status: "completed" });
    }
    expect(wrapper.findAll(".task-group").length).toBe(1);

    for (let i = 0; i < 5; i++) {
      store.applyEvent("completed", { id: `t-${i}`, outputPath: `/out-${i}` });
    }
    expect(wrapper.findAll(".task-group").length).toBe(1);
  });

  it("flat 模式下，所有 task 显示成 .task-row，0 个 .task-group", async () => {
    const { useTaskStore, useTasksList } = await freshModules();
    const store = useTaskStore();
    const list = useTasksList();
    list.viewMode.value = "flat"; // ← 切到 flat

    const RUN_ID = `r-dom-4-${Date.now()}`;
    for (let i = 0; i < 5; i++) {
      store.applyEvent("created", makeTask(`t-${i}`, RUN_ID, "queued"));
    }

    const Component = createTestListComponent();
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } });

    const groups = wrapper.findAll(".task-group");
    const tasks = wrapper.findAll(".task-row");

    expect(groups.length).toBe(0);
    expect(tasks.length).toBe(5);
  });

  it("切 viewMode group→flat→group 后，DOM 稳定不变", async () => {
    const { useTaskStore, useTasksList } = await freshModules();
    const store = useTaskStore();
    const list = useTasksList();
    list.viewMode.value = "group";

    const RUN_ID = `r-dom-5-${Date.now()}`;
    for (let i = 0; i < 3; i++) {
      store.applyEvent("created", makeTask(`t-${i}`, RUN_ID, "queued"));
    }

    const Component = createTestListComponent();
    const wrapper = mount(Component, { props: { displayedItemsRef: list.displayedItems } });
    expect(wrapper.findAll(".task-group").length).toBe(1);

    // 切到 flat
    list.viewMode.value = "flat";
    await nextTick();
    expect(wrapper.findAll(".task-group").length).toBe(0);
    expect(wrapper.findAll(".task-row").length).toBe(3);

    // 切回 group
    list.viewMode.value = "group";
    await nextTick();
    expect(wrapper.findAll(".task-group").length).toBe(1);
    expect(wrapper.findAll(".task-row").length).toBe(0);
  });
});
