import { describe, it, expect } from "vitest";
import { createTaskCollection, type TaskCollectionHooks } from "./taskCollection";
import type { EncvTask } from "@encv/shared-components/types/task";

function makeTask(over: Partial<EncvTask> = {}): EncvTask {
  return {
    id: "t1",
    runId: "r1",
    type: "encrypt",
    pluginName: "p1",
    triggeredBy: "user",
    status: "running",
    progress: 10,
    createdAt: "2026-01-01T00:00:00.000Z",
    sourcePath: "/a",
    ...over,
  } as EncvTask;
}

describe("createTaskCollection", () => {
  it("getTaskById 返回正确任务 / 未知返回 undefined", () => {
    const c = createTaskCollection();
    c.appendTask(makeTask({ id: "a" }));
    c.appendTask(makeTask({ id: "b" }));
    expect(c.getTaskById("a")?.id).toBe("a");
    expect(c.getTaskById("b")?.id).toBe("b");
    expect(c.getTaskById("z")).toBeUndefined();
  });

  it("patchTaskById 合并字段，IDENTITY_FIELDS 守卫（null/'' 不覆盖）", () => {
    const c = createTaskCollection();
    c.appendTask(makeTask({ id: "a", runId: "r1", type: "encrypt", pluginName: "p1", triggeredBy: "user" }));
    const ok = c.patchTaskById("a", {
      progress: 50,
      runId: "", // 实体字段：空串不应覆盖
      type: null as any, // 实体字段：null 不应覆盖
      status: "completed",
    });
    expect(ok).toBe(true);
    const t = c.getTaskById("a")!;
    expect(t.progress).toBe(50);
    expect(t.status).toBe("completed");
    expect(t.runId).toBe("r1"); // 保留原值
    expect(t.type).toBe("encrypt"); // 保留原值
    expect(t.pluginName).toBe("p1"); // 保留原值
  });

  it("patchTaskById 未知 id 返回 false", () => {
    const c = createTaskCollection();
    expect(c.patchTaskById("nope", { progress: 1 })).toBe(false);
  });

  it("appendTask 新任务 prepend；已存在则按 patch 合并", () => {
    const c = createTaskCollection();
    c.appendTask(makeTask({ id: "a", progress: 1 }));
    c.appendTask(makeTask({ id: "b", progress: 2 }));
    expect(c.tasks.value.map(t => t.id)).toEqual(["b", "a"]); // prepend
    // 已存在 → patch 而非重复
    c.appendTask(makeTask({ id: "a", progress: 99 }));
    expect(c.tasks.value.filter(t => t.id === "a")).toHaveLength(1);
    expect(c.getTaskById("a")!.progress).toBe(99);
  });

  it("applyEvent created → 插入；update/progress → patch；completed → 归一化", () => {
    const c = createTaskCollection();
    c.applyEvent("created", makeTask({ id: "a", status: "running", progress: 0 }));
    expect(c.getTaskById("a")?.status).toBe("running");

    c.applyEvent("update", { id: "a", progress: 40 });
    expect(c.getTaskById("a")?.progress).toBe(40);

    c.applyEvent("progress", { id: "a", progress: 70 });
    expect(c.getTaskById("a")?.progress).toBe(70);

    // completed（无错）→ status=completed, progress=100, completedAt 设置
    c.applyEvent("completed", { id: "a" });
    const done = c.getTaskById("a")!;
    expect(done.status).toBe("completed");
    expect(done.progress).toBe(100);
    expect(typeof done.completedAt).toBe("string");
  });

  it("applyEvent completed（带 error）→ status=failed", () => {
    const c = createTaskCollection();
    c.applyEvent("created", makeTask({ id: "a", status: "running" }));
    c.applyEvent("completed", { id: "a", error: "boom" });
    expect(c.getTaskById("a")!.status).toBe("failed");
  });

  it("hooks：acceptCreated=false 丢弃；onCreated/onPatched 触发", () => {
    const calls: string[] = [];
    const hooks: TaskCollectionHooks = {
      acceptCreated: () => {
        calls.push("acceptCreated");
        return false; // 丢弃
      },
      onCreated: () => calls.push("onCreated"),
      onPatched: type => calls.push(`onPatched:${type}`),
    };
    const c = createTaskCollection(hooks);
    c.applyEvent("created", makeTask({ id: "x" }));
    expect(c.getTaskById("x")).toBeUndefined(); // 被丢弃
    expect(calls).toContain("acceptCreated");
    expect(calls).not.toContain("onCreated");

    // 直接 appendTask 绕过 acceptCreated（与 store 的 appendTask 公开方法一致）
    c.appendTask(makeTask({ id: "x" }));
    expect(c.getTaskById("x")).toBeDefined();
    expect(calls).toContain("onCreated");

    c.applyEvent("update", { id: "x", progress: 1 });
    expect(calls).toContain("onPatched:update");
    c.applyEvent("completed", { id: "x" });
    expect(calls).toContain("onPatched:completed");
  });

  it("rebuildIndex 在批量替换 tasks.value 后保持 getTaskById 正确", () => {
    const c = createTaskCollection();
    c.appendTask(makeTask({ id: "a" }));
    c.appendTask(makeTask({ id: "b" }));
    // 模拟 loadRun 批量替换
    c.tasks.value = [makeTask({ id: "c" }), makeTask({ id: "d" })];
    c.rebuildIndex();
    expect(c.getTaskById("a")).toBeUndefined();
    expect(c.getTaskById("c")?.id).toBe("c");
    expect(c.getTaskById("d")?.id).toBe("d");
    // patch 仍生效
    expect(c.patchTaskById("c", { progress: 5 })).toBe(true);
    expect(c.getTaskById("c")!.progress).toBe(5);
  });
});
