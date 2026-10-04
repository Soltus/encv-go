/**
 * useTaskTrigger 单元测试
 *
 * 覆盖：
 * 1. recordTriggeredBy + getTriggeredBy roundtrip
 * 2. 未知 taskId 默认 user
 * 3. localStorage 容量上限 MAX_ENTRIES=500
 * 4. clearTriggeredBy 清空
 * 5. 异常 localStorage 降级
 */

import {
  _reloadTriggeredByCache,
  clearTriggeredBy,
  getTriggeredBy,
  recordTriggeredBy,
} from "@encv/shared-components/composables/useTaskTrigger";
import { beforeEach, describe, expect, it, vi } from "vitest";

// 🆕 2026-06-10 修复：每个 test 都要 reset cacheMap（useTaskTrigger 模块级单例）
// 历史 bug：cacheMap 在 test 之间持续存在 → 后 test 拿前 test 的 cache，
//   localStorage.clear() 只清 localStorage，不清 cacheMap
beforeEach(() => {
  localStorage.clear();
  _reloadTriggeredByCache();
  vi.restoreAllMocks();
});

describe("useTaskTrigger — roundtrip", () => {
  it("record + get 同一个 taskId", () => {
    recordTriggeredBy("task-1", "automation");
    expect(getTriggeredBy("task-1")).toBe("automation");
  });

  it("未登记的 taskId 默认 user", () => {
    expect(getTriggeredBy("unknown")).toBe("user");
  });

  it("空 taskId 返 user", () => {
    expect(getTriggeredBy("")).toBe("user");
  });

  it("三种触发者类型都正确存取", () => {
    recordTriggeredBy("a", "user");
    recordTriggeredBy("b", "automation");
    recordTriggeredBy("c", "ai_agent");
    expect(getTriggeredBy("a")).toBe("user");
    expect(getTriggeredBy("b")).toBe("automation");
    expect(getTriggeredBy("c")).toBe("ai_agent");
  });

  it("同 taskId 后写覆盖前写", () => {
    recordTriggeredBy("x", "user");
    recordTriggeredBy("x", "automation");
    expect(getTriggeredBy("x")).toBe("automation");
  });
});

describe("useTaskTrigger — 容量上限", () => {
  it("写入超过 500 条只保留最新 500", () => {
    // 写 600 条不同 taskId，全部都是 'automation'
    for (let i = 0; i < 600; i++) {
      recordTriggeredBy(`task-${i}`, "automation");
    }
    // 用 public API 验证：最新的 task-599 保留，最早的 task-0 已被裁剪（返 user fallback）
    expect(getTriggeredBy("task-599")).toBe("automation");
    expect(getTriggeredBy("task-100")).toBe("automation");
    // 最早的 100 条已被裁剪（capacity=500，从 task-100 起步，task-0..99 没了）
    expect(getTriggeredBy("task-99")).toBe("user");
    expect(getTriggeredBy("task-0")).toBe("user");
  });
});

describe("useTaskTrigger — clear", () => {
  it("clearTriggeredBy 清空所有记录", () => {
    recordTriggeredBy("a", "automation");
    recordTriggeredBy("b", "ai_agent");
    clearTriggeredBy();
    expect(getTriggeredBy("a")).toBe("user");
    expect(getTriggeredBy("b")).toBe("user");
  });
});

describe("useTaskTrigger — 异常降级", () => {
  it("localStorage.setItem 抛出时不冒泡（内存 map 仍生效，仅丢失跨会话持久化）", () => {
    // 🆕 2026-10-02 契约修订（改**测试**，实现行为是对的）：
    //   实现 `setTaskMetadata` 的顺序是「先写内存 map → trimMapInPlace → writeMap」。
    //   持久化（writeMap）失败**不应该**把已经写进内存的值回滚掉 ——
    //   否则一次 QuotaExceededError 就会让本会话刚创建的 task 全部丢掉 triggeredBy，
    //   反而比"仅丢失跨会话持久化"更糟（UI 立即显示错，还查不出原因）。
    //   所以这里断言：
    //     1) 不抛异常（writeMap 内部已吞掉）
    //     2) 本次会话内仍能读到刚写入的值
    //   真正的"降级"体现在**重新加载后**读不回来（那条语义由 writeMap 的 try/catch 保证）。
    const setItemSpy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("QuotaExceededError");
    });
    let threw = false;
    try {
      recordTriggeredBy("foo", "automation");
    } catch {
      threw = true;
    }
    expect(threw).toBe(false); // 不应抛异常
    expect(getTriggeredBy("foo")).toBe("automation"); // 内存 map 仍生效
    setItemSpy.mockRestore();
  });

  it("localStorage 损坏 JSON 时降级（readMap 失败返 {}）", () => {
    // 🆕 2026-06-10 修复：用 v2 STORAGE_KEY（生产代码 v2 用的 key）
    localStorage.setItem("encv_task_triggered_by_v2", "{not valid json}");
    expect(getTriggeredBy("foo")).toBe("user");
  });

  it("空 taskId 不写入", () => {
    const setItemSpy = vi.spyOn(Storage.prototype, "setItem");
    recordTriggeredBy("", "automation");
    expect(setItemSpy).not.toHaveBeenCalled();
    setItemSpy.mockRestore();
  });
});
