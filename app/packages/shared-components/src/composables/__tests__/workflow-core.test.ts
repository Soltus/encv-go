/**
 * 状态机 + 调度器 + matrix 展开器 + 条件求值器 单元测试
 */

import { evaluateCondition } from "@encv/shared-components/lib/workflow/conditionEvaluator";
import { expandMatrix } from "@encv/shared-components/lib/workflow/matrixExpander";
import { getNextReadyJobs, resolveExecutionOrder } from "@encv/shared-components/lib/workflow/scheduler";
import { canTransition, computeJobConclusion, inferWorkflowStatus, transition } from "@encv/shared-components/lib/workflow/stateMachine";
import type { JobRun, StepRun, StepStatus } from "@encv/shared-components/lib/workflow/types";
import { describe, expect, it } from "vitest";

// ==================== State Machine ====================

describe("canTransition", () => {
  it("允许 pending → submitted", () => {
    expect(canTransition("pending", "submitted")).toBe(true);
  });
  it("允许 pending → cancelled", () => {
    expect(canTransition("pending", "cancelled")).toBe(true);
  });
  it("拒绝 pending → running（必须经过 submitted → queued）", () => {
    expect(canTransition("pending", "running")).toBe(false);
  });
  it("拒绝终态转出", () => {
    expect(canTransition("success", "running")).toBe(false);
    expect(canTransition("failure", "queued")).toBe(false);
    expect(canTransition("cancelled", "pending")).toBe(false);
  });
  it("允许 queued → running", () => {
    expect(canTransition("queued", "running")).toBe(true);
  });
  it("允许 running → success/failure/cancelled/timed_out", () => {
    expect(canTransition("running", "success")).toBe(true);
    expect(canTransition("running", "failure")).toBe(true);
    expect(canTransition("running", "cancelled")).toBe(true);
    expect(canTransition("running", "timed_out")).toBe(true);
  });
});

describe("transition", () => {
  it("合法转换返回新状态", () => {
    expect(transition("pending", "submitted")).toBe("submitted");
    expect(transition("running", "success")).toBe("success");
  });
  it("非法转换抛出错误", () => {
    expect(() => transition("success", "running")).toThrow();
  });
});

describe("computeJobConclusion", () => {
  function makeSteps(statuses: StepStatus[]): StepRun[] {
    return statuses.map((s, i) => ({
      id: `s${i}`,
      stepDefId: `sd${i}`,
      status: s,
    }));
  }

  it("空 steps → skipped", () => {
    expect(computeJobConclusion([])).toBe("skipped");
  });
  it("全 success → success", () => {
    expect(computeJobConclusion(makeSteps(["success", "success"]))).toBe("success");
  });
  it("有 failure（无 continueOnError）→ failure", () => {
    const coMap = new Map<string, boolean>();
    coMap.set("sd0", false);
    expect(computeJobConclusion(makeSteps(["success", "failure"]), coMap)).toBe("failure");
  });
  it("failure 但 continueOnError=true 且有 success → success", () => {
    const coMap = new Map<string, boolean>();
    coMap.set("sd1", true); // failure 的 step 允许继续
    expect(computeJobConclusion(makeSteps(["success", "failure"]), coMap)).toBe("success");
  });
  it("timed_out 视为 failure", () => {
    const coMap = new Map<string, boolean>();
    coMap.set("sd0", false);
    expect(computeJobConclusion(makeSteps(["timed_out"]), coMap)).toBe("failure");
  });
  it("有 cancelled → cancelled", () => {
    expect(computeJobConclusion(makeSteps(["success", "cancelled"]))).toBe("cancelled");
  });
  it("全 skipped → skipped", () => {
    expect(computeJobConclusion(makeSteps(["skipped", "skipped"]))).toBe("skipped");
  });
  it("mixed: success + skipped → success", () => {
    expect(computeJobConclusion(makeSteps(["success", "skipped"]))).toBe("success");
  });
});

describe("inferWorkflowStatus", () => {
  function makeJobs(conclusions: Array<{ status: string; conclusion?: string }>): JobRun[] {
    return conclusions.map((c, i) => ({
      id: `j${i}`,
      jobDefId: `jd${i}`,
      status: c.status as any,
      conclusion: c.conclusion as any,
      steps: [],
    }));
  }

  it("空 jobs → pending", () => {
    expect(inferWorkflowStatus([])).toBe("pending");
  });
  it("有 running job → running", () => {
    expect(inferWorkflowStatus(makeJobs([{ status: "running" }]))).toBe("running");
  });
  it("全 success → success", () => {
    expect(
      inferWorkflowStatus(
        makeJobs([
          { status: "success", conclusion: "success" },
          { status: "success", conclusion: "success" },
        ])
      )
    ).toBe("success");
  });
  it("有 failure → failure", () => {
    expect(
      inferWorkflowStatus(
        makeJobs([
          { status: "success", conclusion: "success" },
          { status: "failure", conclusion: "failure" },
        ])
      )
    ).toBe("failure");
  });
});

// ==================== Scheduler ====================

describe("resolveExecutionOrder", () => {
  const jobA = { id: "a", name: "A", steps: [] };
  const jobB = { id: "b", name: "B", needs: ["a"], steps: [] };
  const jobC = { id: "c", name: "C", needs: ["a"], steps: [] };
  const jobD = { id: "d", name: "D", needs: ["b", "c"], steps: [] };

  it("无依赖 → 单层", () => {
    const result = resolveExecutionOrder([jobA]);
    expect(result).toEqual([["a"]]);
  });

  it("线性依赖 a→b → 两层", () => {
    const result = resolveExecutionOrder([jobA, jobB]);
    expect(result).toEqual([["a"], ["b"]]);
  });

  it("a→(b,c) 并行 → b,c 同层", () => {
    const result = resolveExecutionOrder([jobA, jobB, jobC]);
    expect(result).toEqual([["a"], ["b", "c"]]);
  });

  it("a→(b,c)→d 三层", () => {
    const result = resolveExecutionOrder([jobA, jobB, jobC, jobD]);
    expect(result).toEqual([["a"], ["b", "c"], ["d"]]);
  });

  it("空数组 → 空", () => {
    expect(resolveExecutionOrder([])).toEqual([]);
  });

  it("循环依赖抛出错误", () => {
    const cycleA = { id: "x", name: "X", needs: ["y"], steps: [] };
    const cycleB = { id: "y", name: "Y", needs: ["x"], steps: [] };
    expect(() => resolveExecutionOrder([cycleA, cycleB])).toThrow(/Circular/);
  });

  it("忽略不存在的依赖", () => {
    const j = { id: "z", name: "Z", needs: ["nonexistent"], steps: [] };
    // 不存在的依赖被忽略，z 可以直接运行
    const result = resolveExecutionOrder([j]);
    expect(result).toEqual([["z"]]);
  });
});

describe("getNextReadyJobs", () => {
  const jobs = [
    { id: "setup", name: "Setup", steps: [] },
    { id: "test", name: "Test", needs: ["setup"], steps: [] },
    { id: "cleanup", name: "Cleanup", needs: ["test"], steps: [] },
  ];

  it("初始无完成 → 只有 setup 就绪", () => {
    expect(getNextReadyJobs(jobs, new Set())).toEqual(["setup"]);
  });

  it("setup 完成 → test 就绪", () => {
    expect(getNextReadyJobs(jobs, new Set(["setup"]))).toEqual(["test"]);
  });

  it("全部完成 → 无就绪", () => {
    expect(getNextReadyJobs(jobs, new Set(["setup", "test", "cleanup"]))).toEqual([]);
  });
});

// ==================== Matrix Expander ====================

describe("expandMatrix", () => {
  it("单轴展开", () => {
    const result = expandMatrix({ type: "matrix", axes: { plugin: ["a", "b"] } });
    expect(result).toEqual([{ plugin: "a" }, { plugin: "b" }]);
  });

  it("双轴笛卡尔积", () => {
    const result = expandMatrix({
      type: "matrix",
      axes: { cipher: ["0", "1"], compress: ["none", "zstd"] },
    });
    expect(result).toEqual([
      { cipher: "0", compress: "none" },
      { cipher: "0", compress: "zstd" },
      { cipher: "1", compress: "none" },
      { cipher: "1", compress: "zstd" },
    ]);
  });

  it("三轴笛卡尔积", () => {
    const result = expandMatrix({
      type: "matrix",
      axes: { a: ["1"], b: ["2"], c: ["3"] },
    });
    expect(result).toHaveLength(1 * 1 * 1);
    expect(result[0]).toEqual({ a: "1", b: "2", c: "3" });
  });

  it("空 axes → 单个空绑定", () => {
    expect(expandMatrix({ type: "matrix", axes: {} })).toEqual([{}]);
  });

  it("某轴为空值 → 0 个组合", () => {
    expect(expandMatrix({ type: "matrix", axes: { x: [] } })).toEqual([]);
  });
});

// ==================== Condition Evaluator ====================

describe("evaluateCondition", () => {
  it("always → true", () => {
    expect(evaluateCondition({ op: "always" }, {})).toBe(true);
  });

  it("success — 上一步成功 → true", () => {
    expect(evaluateCondition({ op: "success" }, { previousStepStatus: "success" })).toBe(true);
  });

  it("success — 上一步失败 → false", () => {
    expect(evaluateCondition({ op: "success" }, { previousStepStatus: "failure" })).toBe(false);
  });

  it("failure — 上一步失败 → true", () => {
    expect(evaluateCondition({ op: "failure" }, { previousStepStatus: "failure" })).toBe(true);
  });

  it("eq 匹配", () => {
    expect(evaluateCondition({ op: "eq", left: "${{type}}", right: "encrypt" }, { vars: { type: "encrypt" } })).toBe(true);
  });

  it("eq 不匹配", () => {
    expect(evaluateCondition({ op: "eq", left: "${{type}}", right: "decrypt" }, { vars: { type: "encrypt" } })).toBe(false);
  });

  it("neq", () => {
    expect(evaluateCondition({ op: "neq", left: "a", right: "b" }, {})).toBe(true);
  });

  it("and 全 true → true", () => {
    expect(evaluateCondition({ op: "and", children: [{ op: "always" }, { op: "always" }] }, {})).toBe(true);
  });

  it("and 有 false → false", () => {
    expect(evaluateCondition({ op: "and", children: [{ op: "always" }, { op: "eq", left: "a", right: "b" }] }, {})).toBe(false);
  });

  it("or 有 true → true", () => {
    expect(evaluateCondition({ op: "or", children: [{ op: "always" }, { op: "eq", left: "a", right: "b" }] }, {})).toBe(true);
  });

  it("not 取反", () => {
    expect(evaluateCondition({ op: "not", child: { op: "always" } }, {})).toBe(false);
    expect(evaluateCondition({ op: "not", child: { op: "eq", left: "a", right: "b" } }, {})).toBe(true);
  });

  it("未知操作符默认 true（安全侧）", () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const expr = { op: "unknown" as any };
    // 控制台会 warn，但返回 true
    expect(evaluateCondition(expr, {})).toBe(true);
  });
});
