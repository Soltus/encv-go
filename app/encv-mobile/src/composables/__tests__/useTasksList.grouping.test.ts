/**
 * 复现：自动化测试任务聚合模式显示 100+ 张"自动化"卡片
 * 根因验证：task.runId 缺失时，useTasksList.groupedTasksByRunId 会把每个 task 单独成组
 *
 * 🆕 v6 2026-06-22：runId 现在是 task 一等字段（后端持久化），不再需要 setTaskMetadata
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// 提供一个隔离的 localStorage，避免污染其他并行测试
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
import { useTasksList } from "@/composables/useTasksList";
import { useTaskStore } from "@/stores/taskStore";

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

function makeTask(id: string, sourcePath: string, runId?: string, triggeredBy?: EncvTask["triggeredBy"]): EncvTask {
  return {
    id,
    type: "encrypt",
    sourcePath,
    status: "completed",
    progress: 100,
    createdAt: "2026-06-18T10:00:00.000Z",
    completedAt: "2026-06-18T10:01:00.000Z",
    runId,
    triggeredBy,
  };
}

describe("useTasksList — 自动化测试任务分组", () => {
  beforeEach(() => {
    testStorage.clear();
    setActivePinia(createPinia());
  });
  afterEach(() => {
    testStorage.clear();
  });

  it("同一 runId 的 12 个 task 应该聚合成 1 个 group", () => {
    const store = useTaskStore();
    const runId = "run-automation-001";
    // 🆕 v6 2026-06-22：runId 直接设在 task 上（后端持久化，单一数据源）
    const tasks: EncvTask[] = Array.from({ length: 12 }, (_, i) => makeTask(`task-${i}`, `/mock/sample-${i}.mp4`, runId, "automation"));

    store.bulkSetTasks(tasks);

    const list = useTasksList();
    const groups = list.groupedTasksByRunId.value;

    // ✅ 期望行为：1 个 group（runId 是 task 一等字段，按 runId 聚合）
    expect(groups.length).toBe(1);
    expect(groups[0].runId).toBe(runId);
    expect(groups[0].tasks.length).toBe(12);
  });
});
