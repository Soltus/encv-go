/**
 * useWorkflowStore 单元测试 — 防回归 createDefinition 覆盖 partial.id 的 bug
 *
 * 历史 bug：
 *   createDefinition 内部强制覆盖 partial.id 为 generateId()，
 *   导致 PluginTestsDetail.vue 的 buildDynamicWorkflow() 传
 *   { id: 'dynamic-auto-test', ... } 时，实际 store 里的 id 变成
 *   'wf-xxx-yyy' 随机串，runWorkflow('dynamic-auto-test') 找不到 def。
 *
 * 修复：createDefinition 改用 partial.id ?? generateId()，尊重调用方指定的 id。
 */
import { beforeEach, describe, expect, it } from "vitest";
import { useWorkflowStore } from "@encv/shared-components/composables/useWorkflowStore";
import type { WorkflowDefinition } from "@encv/shared-components/lib/workflow/types";

// mock localStorage
const storage = new Map<string, string>();
beforeEach(() => {
  storage.clear();
  globalThis.localStorage = {
    getItem: (k: string) => storage.get(k) ?? null,
    setItem: (k: string, v: string) => {
      storage.set(k, v);
    },
    removeItem: (k: string) => {
      storage.delete(k);
    },
    clear: () => {
      storage.clear();
    },
    key: () => null,
    length: 0,
  } as Storage;
});

const baseDef: Omit<WorkflowDefinition, "id" | "createdAt" | "updatedAt"> = {
  name: "test",
  description: "test",
  trigger: "manual",
  jobs: [],
};

describe("useWorkflowStore — createDefinition partial.id 行为", () => {
  it("不传 id 时应自动生成 id（与原版兼容）", () => {
    const store = useWorkflowStore();
    const def = store.createDefinition(baseDef);
    expect(def.id).toMatch(/^wf-/);
    expect(def.id).not.toBe("");
  });

  it("传 partial.id 时必须尊重调用方指定的 id（防 dynamic-auto-test 失踪）", () => {
    const store = useWorkflowStore();
    const def = store.createDefinition({ ...baseDef, id: "dynamic-auto-test" });
    expect(def.id).toBe("dynamic-auto-test");
  });

  it("PluginTestsDetail.vue 场景：build→run 必须能 lookup 回来", () => {
    const store = useWorkflowStore();
    // 模拟 PluginTestsDetail.vue buildDynamicWorkflow
    const existingIdx = store.definitions.value.findIndex(d => d.id === "dynamic-auto-test");
    expect(existingIdx).toBe(-1);
    store.createDefinition({ ...baseDef, id: "dynamic-auto-test" });
    // 模拟 handleRunWorkflow
    const lookup = store.getDefinition("dynamic-auto-test");
    expect(lookup).toBeDefined();
    expect(lookup!.id).toBe("dynamic-auto-test");
  });

  it("同时支持多次 createDefinition 不同 id（regression）", () => {
    const store = useWorkflowStore();
    const a = store.createDefinition({ ...baseDef, id: "wf-a" });
    const b = store.createDefinition({ ...baseDef, id: "wf-b" });
    expect(a.id).toBe("wf-a");
    expect(b.id).toBe("wf-b");
    expect(store.getDefinition("wf-a")!.id).toBe("wf-a");
    expect(store.getDefinition("wf-b")!.id).toBe("wf-b");
  });
});
