import { applyTerminalGuard, VALID_TRANSITIONS, validateTransition } from "@/lib/workflow/state-machine";
import type { StepStatus } from "@/lib/workflow/types";
import { describe, expect, it } from "vitest";

// ============ applyTerminalGuard ============

describe("applyTerminalGuard", () => {
  it("current 为 null 时原样返回 update（保留 status 字段）", () => {
    const update = { status: "running" as StepStatus, progress: 50 };
    const result = applyTerminalGuard(null, update);
    expect(result).toEqual(update);
    expect(result.status).toBe("running");
  });

  it("current 为 undefined 时原样返回 update（保留 status 字段）", () => {
    const update = { status: "queued" as StepStatus, progress: 10 };
    const result = applyTerminalGuard(undefined, update);
    expect(result).toEqual(update);
    expect(result.status).toBe("queued");
  });

  it("current 为非终态（pending）时保留全部 update", () => {
    const current = { status: "pending" as StepStatus };
    const update = { status: "running" as StepStatus, progress: 30 };
    const result = applyTerminalGuard(current, update);
    expect(result).toEqual(update);
    expect(result.status).toBe("running");
    expect(result.progress).toBe(30);
  });

  it("current 为非终态（running）时保留全部 update", () => {
    const current = { status: "running" as StepStatus };
    const update = { status: "success" as StepStatus, progress: 100 };
    const result = applyTerminalGuard(current, update);
    expect(result).toEqual(update);
    expect(result.status).toBe("success");
  });

  it("current 为终态（success）时剥离 status 字段，保留其他字段", () => {
    const current = { status: "success" as StepStatus };
    const update = { status: "running" as StepStatus, progress: 50 };
    const result = applyTerminalGuard(current, update);
    // status 被剥离
    expect(result.status).toBeUndefined();
    // 其他字段保留
    expect(result.progress).toBe(50);
  });

  it("current 为终态（failure）时剥离 status 字段，保留 progress / phase / speed / eta 元数据", () => {
    const current = { status: "failure" as StepStatus };
    const update = {
      status: "running" as StepStatus,
      progress: 75,
      phase: "encrypting",
      speed: "12.5 MB/s",
      eta: "00:01:30",
    };
    const result = applyTerminalGuard(current, update);
    expect(result.status).toBeUndefined();
    expect(result.progress).toBe(75);
    expect(result.phase).toBe("encrypting");
    expect(result.speed).toBe("12.5 MB/s");
    expect(result.eta).toBe("00:01:30");
  });

  it("current 为终态（cancelled）时剥离 status 字段", () => {
    const current = { status: "cancelled" as StepStatus };
    const update = { status: "running" as StepStatus };
    const result = applyTerminalGuard(current, update);
    expect(result.status).toBeUndefined();
    // 仅 status 字段时返回空对象
    expect(Object.keys(result)).toEqual([]);
  });

  it("current 为终态（skipped）时剥离 status 字段", () => {
    const current = { status: "skipped" as StepStatus };
    const update = { status: "pending" as StepStatus, progress: 0 };
    const result = applyTerminalGuard(current, update);
    expect(result.status).toBeUndefined();
    expect(result.progress).toBe(0);
  });

  it("current 为终态（timed_out）时剥离 status 字段", () => {
    const current = { status: "timed_out" as StepStatus };
    const update = { status: "running" as StepStatus, progress: 60 };
    const result = applyTerminalGuard(current, update);
    expect(result.status).toBeUndefined();
    expect(result.progress).toBe(60);
  });

  it("current 为终态（cancelling 非终态）时保留 status 字段", () => {
    // cancelling 不是终态（终态集合：success/failure/cancelled/skipped/timed_out）
    const current = { status: "cancelling" as StepStatus };
    const update = { status: "cancelled" as StepStatus, progress: 100 };
    const result = applyTerminalGuard(current, update);
    expect(result).toEqual(update);
    expect(result.status).toBe("cancelled");
  });

  it("current 为终态时 update 不含 status 字段则原样返回", () => {
    const current = { status: "success" as StepStatus };
    const update = { progress: 100, phase: "completed" };
    const result = applyTerminalGuard(current, update);
    // update 本身就没 status 字段，原样返回
    expect(result).toEqual(update);
    expect(result.progress).toBe(100);
    expect(result.phase).toBe("completed");
  });
});

// ============ validateTransition ============

describe("validateTransition", () => {
  it("相同状态返回 true（幂等）", () => {
    expect(validateTransition("pending", "pending")).toBe(true);
    expect(validateTransition("running", "running")).toBe(true);
    expect(validateTransition("success", "success")).toBe(true);
    expect(validateTransition("failure", "failure")).toBe(true);
    expect(validateTransition("cancelled", "cancelled")).toBe(true);
  });

  it("pending → submitted/queued/running/cancelled/skipped 合法", () => {
    expect(validateTransition("pending", "submitted")).toBe(true);
    expect(validateTransition("pending", "queued")).toBe(true);
    expect(validateTransition("pending", "running")).toBe(true);
    expect(validateTransition("pending", "cancelled")).toBe(true);
    expect(validateTransition("pending", "skipped")).toBe(true);
  });

  it("pending → success/failure/timed_out 非法（不能直接跳到终态）", () => {
    expect(validateTransition("pending", "success")).toBe(false);
    expect(validateTransition("pending", "failure")).toBe(false);
    expect(validateTransition("pending", "timed_out")).toBe(false);
    expect(validateTransition("pending", "cancelling")).toBe(false);
  });

  it("submitted → queued/running/cancelled/skipped 合法", () => {
    expect(validateTransition("submitted", "queued")).toBe(true);
    expect(validateTransition("submitted", "running")).toBe(true);
    expect(validateTransition("submitted", "cancelled")).toBe(true);
    expect(validateTransition("submitted", "skipped")).toBe(true);
  });

  it("submitted → success/failure/timed_out/cancelling 非法", () => {
    expect(validateTransition("submitted", "success")).toBe(false);
    expect(validateTransition("submitted", "failure")).toBe(false);
    expect(validateTransition("submitted", "timed_out")).toBe(false);
    expect(validateTransition("submitted", "cancelling")).toBe(false);
  });

  it("queued → running/cancelling/cancelled/skipped 合法", () => {
    expect(validateTransition("queued", "running")).toBe(true);
    expect(validateTransition("queued", "cancelling")).toBe(true);
    expect(validateTransition("queued", "cancelled")).toBe(true);
    expect(validateTransition("queued", "skipped")).toBe(true);
  });

  it("queued → success/failure/timed_out 非法", () => {
    expect(validateTransition("queued", "success")).toBe(false);
    expect(validateTransition("queued", "failure")).toBe(false);
    expect(validateTransition("queued", "timed_out")).toBe(false);
  });

  it("running → cancelling/success/failure/cancelled/timed_out 合法", () => {
    expect(validateTransition("running", "cancelling")).toBe(true);
    expect(validateTransition("running", "success")).toBe(true);
    expect(validateTransition("running", "failure")).toBe(true);
    expect(validateTransition("running", "cancelled")).toBe(true);
    expect(validateTransition("running", "timed_out")).toBe(true);
  });

  it("running → pending/submitted/queued/skipped 非法（不能回退）", () => {
    expect(validateTransition("running", "pending")).toBe(false);
    expect(validateTransition("running", "submitted")).toBe(false);
    expect(validateTransition("running", "queued")).toBe(false);
    expect(validateTransition("running", "skipped")).toBe(false);
  });

  it("cancelling → cancelled/failure/success 合法", () => {
    expect(validateTransition("cancelling", "cancelled")).toBe(true);
    expect(validateTransition("cancelling", "failure")).toBe(true);
    expect(validateTransition("cancelling", "success")).toBe(true);
  });

  it("cancelling → running/pending/queued/skipped/timed_out 非法", () => {
    expect(validateTransition("cancelling", "running")).toBe(false);
    expect(validateTransition("cancelling", "pending")).toBe(false);
    expect(validateTransition("cancelling", "queued")).toBe(false);
    expect(validateTransition("cancelling", "skipped")).toBe(false);
    expect(validateTransition("cancelling", "timed_out")).toBe(false);
  });

  it("终态 → 任何其他状态返回 false（终态保护）", () => {
    const terminals: StepStatus[] = ["success", "failure", "cancelled", "skipped", "timed_out"];
    const allStates: StepStatus[] = [
      "pending",
      "submitted",
      "queued",
      "running",
      "cancelling",
      "success",
      "failure",
      "cancelled",
      "skipped",
      "timed_out",
    ];
    for (const from of terminals) {
      for (const to of allStates) {
        if (from === to) continue; // 同状态幂等已在另一测试覆盖
        expect(validateTransition(from, to)).toBe(false);
      }
    }
  });
});

// ============ VALID_TRANSITIONS 表完整性 ============

describe("VALID_TRANSITIONS", () => {
  it("包含全部 10 个 StepStatus 键", () => {
    const expectedKeys: StepStatus[] = [
      "pending",
      "submitted",
      "queued",
      "running",
      "cancelling",
      "success",
      "failure",
      "cancelled",
      "skipped",
      "timed_out",
    ];
    for (const key of expectedKeys) {
      expect(VALID_TRANSITIONS).toHaveProperty(key);
      expect(VALID_TRANSITIONS[key]).toBeInstanceOf(Set);
    }
  });

  it("5 个终态的转换集合为空", () => {
    expect(VALID_TRANSITIONS.success.size).toBe(0);
    expect(VALID_TRANSITIONS.failure.size).toBe(0);
    expect(VALID_TRANSITIONS.cancelled.size).toBe(0);
    expect(VALID_TRANSITIONS.skipped.size).toBe(0);
    expect(VALID_TRANSITIONS.timed_out.size).toBe(0);
  });

  it("pending 允许 5 个目标状态（submitted/queued/running/cancelled/skipped）", () => {
    expect(VALID_TRANSITIONS.pending.size).toBe(5);
    expect(VALID_TRANSITIONS.pending.has("submitted")).toBe(true);
    expect(VALID_TRANSITIONS.pending.has("queued")).toBe(true);
    expect(VALID_TRANSITIONS.pending.has("running")).toBe(true);
    expect(VALID_TRANSITIONS.pending.has("cancelled")).toBe(true);
    expect(VALID_TRANSITIONS.pending.has("skipped")).toBe(true);
  });
});
