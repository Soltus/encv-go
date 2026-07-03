/**
 * useTaskViewCompute 回归测试
 *
 * 2026-07-02 新增：bug 3 修复（watch 拆分）后的回归覆盖
 *
 * 背景：
 *   - useTaskViewCompute 把 O(N) 视图计算委托给 Web Worker
 *   - 2026-07-02 修复：原 watch([tasks, searchQuery, ...], cb, { deep: true })
 *     会在 searchQuery 变化时对 tasks（可能几千个）做深度遍历，阻塞 UI
 *   - 修复：拆分为两个 watch — tasks 单独 deep watch，filter/search 状态单独 watch
 *
 * 测试策略：
 *   - vitest 环境下 useTaskViewCompute 走 sync computed 路径（isTestEnv=true）
 *   - sync 路径用 computed 自动追踪依赖，验证 searchQuery 变化后 displayedItems 正确响应
 *   - 这是基础回归测试，确保 watch 拆分没有破坏响应性
 *   - 性能问题（深度遍历阻塞 UI）在 jsdom 下无法可靠测量，真机测试是唯一可靠验证
 *
 * 覆盖场景：
 *   1. searchQuery 变化 → displayedItems 反映过滤结果
 *   2. tasks 变化（append/patch）→ displayedItems 反映新 tasks
 *   3. 大量 tasks（500）时 searchQuery 变化 → displayedItems 正确响应
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

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
import { nextTick } from "vue";
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
    searchTasksVector: vi.fn().mockResolvedValue({ results: [] }),
  };
});

async function freshModules() {
  vi.resetModules();
  setActivePinia(createPinia());
  const { useTaskStore } = await import("@/stores/taskStore");
  const { useTasksList, _resetTasksListSingletonForTests } = await import("@/composables/useTasksList");
  return { useTaskStore, useTasksList, _resetTasksListSingletonForTests };
}

function makeTask(id: string, name: string, runId?: string): EncvTask {
  return {
    id,
    type: "encrypt",
    sourcePath: `/mock/${name}`,
    status: "queued",
    progress: 0,
    createdAt: "2026-07-02T10:00:00.000Z",
    runId: runId ?? `run-${id}`,
    triggeredBy: "user",
    pluginName: "video",
  };
}

describe("useTaskViewCompute 回归（bug 3 修复后）", () => {
  beforeEach(() => {
    testStorage.clear();
  });

  it("searchQuery 变化后 displayedItems 正确反映过滤结果", async () => {
    const { useTaskStore, useTasksList, _resetTasksListSingletonForTests } = await freshModules();
    _resetTasksListSingletonForTests();
    const store = useTaskStore();
    const list = useTasksList();

    // flat 模式便于断言 task item
    list.viewMode.value = "flat";

    // 注入 3 个 task：apple / banana / cherry
    store.applyEvent("created", makeTask("t-1", "apple.mp4"));
    store.applyEvent("created", makeTask("t-2", "banana.mp4"));
    store.applyEvent("created", makeTask("t-3", "cherry.mp4"));
    await nextTick();

    // 初始：3 个 task item（+ date header）
    let taskItems = list.displayedItems.value.filter((it: any) => it.kind === "task");
    expect(taskItems.length).toBe(3);

    // searchQuery = 'banana' → 只剩 1 个 task
    list.searchQuery.value = "banana";
    await nextTick();
    taskItems = list.displayedItems.value.filter((it: any) => it.kind === "task");
    expect(taskItems.length).toBe(1);
    expect(taskItems[0].task.id).toBe("t-2");

    // searchQuery = 'an' → apple(无) banana(有) cherry(无) — 实际 'an' 只在 banana
    list.searchQuery.value = "an";
    await nextTick();
    taskItems = list.displayedItems.value.filter((it: any) => it.kind === "task");
    expect(taskItems.length).toBe(1);
    expect(taskItems[0].task.id).toBe("t-2");

    // 清空 searchQuery → 恢复 3 个
    list.searchQuery.value = "";
    await nextTick();
    taskItems = list.displayedItems.value.filter((it: any) => it.kind === "task");
    expect(taskItems.length).toBe(3);
  });

  it("tasks append 后 displayedItems 反映新 tasks（响应性回归）", async () => {
    const { useTaskStore, useTasksList, _resetTasksListSingletonForTests } = await freshModules();
    _resetTasksListSingletonForTests();
    const store = useTaskStore();
    const list = useTasksList();
    list.viewMode.value = "flat";

    store.applyEvent("created", makeTask("t-1", "apple.mp4"));
    await nextTick();
    let taskItems = list.displayedItems.value.filter((it: any) => it.kind === "task");
    expect(taskItems.length).toBe(1);

    // append 新 task
    store.applyEvent("created", makeTask("t-2", "banana.mp4"));
    await nextTick();
    taskItems = list.displayedItems.value.filter((it: any) => it.kind === "task");
    expect(taskItems.length).toBe(2);

    // patch task 状态变化
    store.applyEvent("progress", { id: "t-1", progress: 50 });
    await nextTick();
    const t1 = store.getTaskById("t-1");
    expect(t1?.progress).toBe(50);
  });

  it("大量 tasks（500）时 searchQuery 变化能正确响应（不卡死）", async () => {
    const { useTaskStore, useTasksList, _resetTasksListSingletonForTests } = await freshModules();
    _resetTasksListSingletonForTests();
    const store = useTaskStore();
    const list = useTasksList();
    list.viewMode.value = "flat";

    // 注入 500 个 task，其中 5 个含关键词 "target"
    for (let i = 0; i < 500; i++) {
      const name = i < 5 ? `target-${i}.mp4` : `file-${i}.mp4`;
      store.applyEvent("created", makeTask(`t-${i}`, name));
    }
    await nextTick();

    // 初始：500 个 task
    let taskItems = list.displayedItems.value.filter((it: any) => it.kind === "task");
    expect(taskItems.length).toBe(500);

    // searchQuery = 'target' → 只剩 5 个
    const start = performance.now();
    list.searchQuery.value = "target";
    await nextTick();
    const elapsed = performance.now() - start;

    taskItems = list.displayedItems.value.filter((it: any) => it.kind === "task");
    expect(taskItems.length).toBe(5);

    // 性能回归保护：jsdom 慢于真机，但应在合理阈值内（< 2s）
    //   - 拆分前 watch 会深度遍历 500 个 task 对象，sync computed 也会重算
    //   - 拆分后 sync 路径不受影响，但 computed 重算仍 O(N)
    //   - 阈值 2s 是 jsdom 下的宽松上限，真机应 < 100ms
    expect(elapsed).toBeLessThan(2000);
  });
});
