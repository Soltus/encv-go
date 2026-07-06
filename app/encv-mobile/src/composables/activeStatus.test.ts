import { describe, expect, it } from "vitest";
import { compactStatus, isActiveStatus, readTurnStatus } from "./activeStatus";

describe("compactStatus", () => {
  describe("字符串直接输入", () => {
    it("原样返回小写", () => {
      expect(compactStatus("Running")).toBe("running");
      expect(compactStatus("ACTIVE")).toBe("active");
      expect(compactStatus("completed")).toBe("completed");
    });

    it("去除空格 / 下划线 / 连字符（统一 collapse）", () => {
      expect(compactStatus("in progress")).toBe("inprogress");
      // 双重格式化：匹配 [\s_-]+ 一起替换；空格 / _ / - 均被吃
      expect(compactStatus("in_progress")).toBe("inprogress");
      expect(compactStatus("  in   progress  ")).toBe("inprogress");
      expect(compactStatus("not-loaded")).toBe("notloaded");
      expect(compactStatus("not_loaded")).toBe("notloaded");
      expect(compactStatus("a -_ b")).toBe("ab");
    });

    it("空字符串", () => {
      expect(compactStatus("")).toBe("");
    });
  });

  describe("对象输入", () => {
    it("从 type 字段抽取", () => {
      expect(compactStatus({ type: "Running" })).toBe("running");
    });

    it("从 status 字段抽取", () => {
      expect(compactStatus({ status: "in progress" })).toBe("inprogress");
    });

    it("从 state 字段抽取", () => {
      expect(compactStatus({ state: "FAILED" })).toBe("failed");
    });

    it("从 kind 字段抽取", () => {
      expect(compactStatus({ kind: "cancelled" })).toBe("cancelled");
    });

    it("优先 type > status > state > kind", () => {
      expect(compactStatus({ type: "a", status: "b", state: "c", kind: "d" })).toBe("a");
      expect(compactStatus({ status: "b", state: "c", kind: "d" })).toBe("b");
      expect(compactStatus({ state: "c", kind: "d" })).toBe("c");
      expect(compactStatus({ kind: "d" })).toBe("d");
    });

    it("type 为 null 时回退到 status", () => {
      expect(compactStatus({ type: null, status: "running" })).toBe("running");
    });

    it("type 为 undefined 时回退到 status", () => {
      expect(compactStatus({ type: undefined, status: "failed" })).toBe("failed");
    });

    it("所有字段都为空时返回空串", () => {
      expect(compactStatus({ type: null, status: "", state: undefined, kind: null })).toBe("");
    });
  });

  describe("边界输入", () => {
    it("null 返回空串", () => {
      expect(compactStatus(null)).toBe("");
    });

    it("undefined 返回空串", () => {
      expect(compactStatus(undefined)).toBe("");
    });

    it("数字返回空串", () => {
      expect(compactStatus(0)).toBe("");
      expect(compactStatus(42)).toBe("");
    });

    it("布尔值返回空串", () => {
      expect(compactStatus(true)).toBe("");
      expect(compactStatus(false)).toBe("");
    });

    it("数组返回空串（不是普通对象）", () => {
      expect(compactStatus(["running"])).toBe("");
    });

    it("空对象返回空串", () => {
      expect(compactStatus({})).toBe("");
    });
  });
});

describe("isActiveStatus", () => {
  describe("7 个 active 集合值", () => {
    it.each([
      ["active", "active"],
      ["inprogress", "inprogress"],
      ["running", "running"],
      ["editing", "editing"],
      ["thinking", "thinking"],
      ["in_progress", "in_progress"],
      ["streaming", "streaming"],
    ])("%s → true", input => {
      expect(isActiveStatus(input)).toBe(true);
    });

    it("大写 / 空格变体也命中", () => {
      expect(isActiveStatus("RUNNING")).toBe(true);
      expect(isActiveStatus("In Progress")).toBe(true);
      expect(isActiveStatus("IN_PROGRESS")).toBe(true);
      expect(isActiveStatus("Thinking")).toBe(true);
    });

    it('对象形式 status="running" 也命中', () => {
      expect(isActiveStatus({ status: "running" })).toBe(true);
      expect(isActiveStatus({ type: "Thinking" })).toBe(true);
    });
  });

  describe("4 个集合边界（不命中）", () => {
    it("completed 集合不命中", () => {
      expect(isActiveStatus("completed")).toBe(false);
      expect(isActiveStatus("done")).toBe(false);
      expect(isActiveStatus("success")).toBe(false);
    });

    it("failed 集合不命中", () => {
      expect(isActiveStatus("failed")).toBe(false);
      expect(isActiveStatus("error")).toBe(false);
    });

    it("interrupted 集合不命中", () => {
      expect(isActiveStatus("cancelled")).toBe(false);
      expect(isActiveStatus("interrupted")).toBe(false);
    });

    it("未知值不命中", () => {
      expect(isActiveStatus("idle")).toBe(false);
      expect(isActiveStatus("pending")).toBe(false);
      expect(isActiveStatus("")).toBe(false);
      expect(isActiveStatus(null)).toBe(false);
      expect(isActiveStatus(undefined)).toBe(false);
      expect(isActiveStatus(42)).toBe(false);
    });
  });
});

describe("readTurnStatus", () => {
  describe("active 集合", () => {
    it.each(["active", "inprogress", "running", "editing", "thinking", "in_progress", "streaming"])('%s → "active"', input => {
      expect(readTurnStatus(input)).toBe("active");
    });

    it("大写 / 空格变体归到 active", () => {
      expect(readTurnStatus("RUNNING")).toBe("active");
      expect(readTurnStatus("In Progress")).toBe("active");
      expect(readTurnStatus("Editing")).toBe("active");
    });
  });

  describe("completed 集合", () => {
    it.each(["completed", "complete", "done", "success", "succeeded"])('%s → "completed"', input => {
      expect(readTurnStatus(input)).toBe("completed");
    });

    it('对象 { state: "DONE" } → "completed"', () => {
      expect(readTurnStatus({ state: "DONE" })).toBe("completed");
    });
  });

  describe("failed 集合", () => {
    it.each(["failed", "failure", "error"])('%s → "failed"', input => {
      expect(readTurnStatus(input)).toBe("failed");
    });

    it('对象 { kind: "ERROR" } → "failed"', () => {
      expect(readTurnStatus({ kind: "ERROR" })).toBe("failed");
    });
  });

  describe("interrupted 集合", () => {
    it.each(["interrupted", "interrupt", "canceled", "cancelled"])('%s → "interrupted"', input => {
      expect(readTurnStatus(input)).toBe("interrupted");
    });

    it('对象 { type: "Cancelled" } → "interrupted"', () => {
      expect(readTurnStatus({ type: "Cancelled" })).toBe("interrupted");
    });
  });

  describe("unknown 兜底", () => {
    it('未知字符串 → "unknown"', () => {
      expect(readTurnStatus("idle")).toBe("unknown");
      expect(readTurnStatus("pending")).toBe("unknown");
      expect(readTurnStatus("something-else")).toBe("unknown");
    });

    it('空串 → "unknown"', () => {
      expect(readTurnStatus("")).toBe("unknown");
    });

    it('null / undefined → "unknown"', () => {
      expect(readTurnStatus(null)).toBe("unknown");
      expect(readTurnStatus(undefined)).toBe("unknown");
    });

    it('数字 / 布尔 → "unknown"', () => {
      expect(readTurnStatus(0)).toBe("unknown");
      expect(readTurnStatus(1)).toBe("unknown");
      expect(readTurnStatus(true)).toBe("unknown");
    });

    it('空对象 → "unknown"', () => {
      expect(readTurnStatus({})).toBe("unknown");
    });

    it('数组 → "unknown"', () => {
      expect(readTurnStatus(["running"])).toBe("unknown");
    });
  });

  describe("类型契约", () => {
    it("返回值始终是 ActiveStatus 联合类型之一", () => {
      const values: unknown[] = [
        "running",
        "done",
        "failed",
        "cancelled",
        "idle",
        "",
        null,
        undefined,
        42,
        true,
        [],
        { type: "running" },
        { status: "failed" },
        { state: "done" },
        { kind: "cancelled" },
      ];
      const allowed = new Set(["active", "completed", "failed", "interrupted", "unknown"]);
      for (const v of values) {
        expect(allowed.has(readTurnStatus(v))).toBe(true);
      }
    });
  });
});
