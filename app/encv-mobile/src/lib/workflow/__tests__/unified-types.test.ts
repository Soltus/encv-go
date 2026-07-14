import { describe, expect, it } from "vitest";
import { ALL_PHASES, isPhase, isUnifiedTimelineEntry, isUnifiedTreeNode, Phase } from "@encv/shared-components/lib/workflow/types";

// ============ Phase 枚举值校验 ============

describe("Phase 枚举值", () => {
  it("每个枚举值与预期字符串一致", () => {
    expect(Phase.Created).toBe("created");
    expect(Phase.Analyzing).toBe("analyzing");
    expect(Phase.Initializing).toBe("initializing");
    expect(Phase.Preprocessing).toBe("preprocessing");
    expect(Phase.Encrypting).toBe("encrypting");
    expect(Phase.Decrypting).toBe("decrypting");
    expect(Phase.Packing).toBe("packing");
    expect(Phase.Verifying).toBe("verifying");
    expect(Phase.Completed).toBe("completed");
  });
});

// ============ ALL_PHASES 数组校验 ============

describe("ALL_PHASES", () => {
  it("长度为 9", () => {
    expect(ALL_PHASES.length).toBe(9);
  });

  it("包含所有 Phase 枚举值", () => {
    expect(ALL_PHASES).toContain(Phase.Created);
    expect(ALL_PHASES).toContain(Phase.Analyzing);
    expect(ALL_PHASES).toContain(Phase.Initializing);
    expect(ALL_PHASES).toContain(Phase.Preprocessing);
    expect(ALL_PHASES).toContain(Phase.Encrypting);
    expect(ALL_PHASES).toContain(Phase.Decrypting);
    expect(ALL_PHASES).toContain(Phase.Packing);
    expect(ALL_PHASES).toContain(Phase.Verifying);
    expect(ALL_PHASES).toContain(Phase.Completed);
  });

  it("是只读数组（frozen）", () => {
    expect(Object.isFrozen(ALL_PHASES)).toBe(true);
  });
});

// ============ isPhase 类型守卫 ============

describe("isPhase", () => {
  it("对 9 个有效 Phase 值返回 true", () => {
    expect(isPhase(Phase.Created)).toBe(true);
    expect(isPhase(Phase.Analyzing)).toBe(true);
    expect(isPhase(Phase.Initializing)).toBe(true);
    expect(isPhase(Phase.Preprocessing)).toBe(true);
    expect(isPhase(Phase.Encrypting)).toBe(true);
    expect(isPhase(Phase.Decrypting)).toBe(true);
    expect(isPhase(Phase.Packing)).toBe(true);
    expect(isPhase(Phase.Verifying)).toBe(true);
    expect(isPhase(Phase.Completed)).toBe(true);
  });

  it("对裸字符串形式的有效 Phase 值返回 true", () => {
    expect(isPhase("created")).toBe(true);
    expect(isPhase("analyzing")).toBe(true);
    expect(isPhase("initializing")).toBe(true);
    expect(isPhase("preprocessing")).toBe(true);
    expect(isPhase("encrypting")).toBe(true);
    expect(isPhase("decrypting")).toBe(true);
    expect(isPhase("packing")).toBe(true);
    expect(isPhase("verifying")).toBe(true);
    expect(isPhase("completed")).toBe(true);
  });

  it("对无效字符串返回 false", () => {
    expect(isPhase("pending")).toBe(false);
    expect(isPhase("running")).toBe(false);
    expect(isPhase("success")).toBe(false);
    expect(isPhase("encrypted")).toBe(false); // 易混淆词
    expect(isPhase("decrypted")).toBe(false);
    expect(isPhase("pack")).toBe(false);
    expect(isPhase("verify")).toBe(false);
    expect(isPhase("analyze")).toBe(false);
    expect(isPhase("init")).toBe(false);
    expect(isPhase("")).toBe(false);
    expect(isPhase("CREATED")).toBe(false); // 大小写敏感
    expect(isPhase("Created")).toBe(false);
  });

  it("对非字符串类型返回 false", () => {
    expect(isPhase(undefined)).toBe(false);
    expect(isPhase(null)).toBe(false);
    expect(isPhase(0)).toBe(false);
    expect(isPhase(1)).toBe(false);
    expect(isPhase(true)).toBe(false);
    expect(isPhase(false)).toBe(false);
    expect(isPhase({})).toBe(false);
    expect(isPhase([])).toBe(false);
    expect(isPhase(["created"])).toBe(false);
    expect(isPhase({ phase: "created" })).toBe(false);
  });
});

// ============ isUnifiedTreeNode 类型守卫 ============

describe("isUnifiedTreeNode", () => {
  it("对完整合法对象返回 true", () => {
    expect(
      isUnifiedTreeNode({
        id: "node-1",
        label: "加密步骤",
        status: "running",
      })
    ).toBe(true);
  });

  it("对含全部可选字段的对象返回 true", () => {
    expect(
      isUnifiedTreeNode({
        id: "node-1",
        label: "加密步骤",
        status: "success",
        progress: 100,
        phase: Phase.Encrypting,
        speed: "12.5 MB/s",
        eta: "00:01:30",
        duration: "5s",
        icon: "lockClosedOutline",
        meta: "mp4 → encv",
        errorHint: undefined,
        children: [],
        detailSlots: ["output", "error"],
      })
    ).toBe(true);
  });

  it("对含 children 嵌套结构的对象返回 true", () => {
    expect(
      isUnifiedTreeNode({
        id: "root",
        label: "Job",
        status: "running",
        children: [
          { id: "child-1", label: "Step 1", status: "success" },
          { id: "child-2", label: "Step 2", status: "pending" },
        ],
      })
    ).toBe(true);
  });

  it("对缺 id 字段的对象返回 false", () => {
    expect(
      isUnifiedTreeNode({
        label: "加密步骤",
        status: "running",
      })
    ).toBe(false);
  });

  it("对缺 label 字段的对象返回 false", () => {
    expect(
      isUnifiedTreeNode({
        id: "node-1",
        status: "running",
      })
    ).toBe(false);
  });

  it("对缺 status 字段的对象返回 false", () => {
    expect(
      isUnifiedTreeNode({
        id: "node-1",
        label: "加密步骤",
      })
    ).toBe(false);
  });

  it("对 id 类型非字符串的对象返回 false", () => {
    expect(
      isUnifiedTreeNode({
        id: 123,
        label: "加密步骤",
        status: "running",
      })
    ).toBe(false);
  });

  it("对 label 类型非字符串的对象返回 false", () => {
    expect(
      isUnifiedTreeNode({
        id: "node-1",
        label: 42,
        status: "running",
      })
    ).toBe(false);
  });

  it("对 status 类型非字符串的对象返回 false", () => {
    expect(
      isUnifiedTreeNode({
        id: "node-1",
        label: "加密步骤",
        status: 0,
      })
    ).toBe(false);
  });

  it("对 null / undefined / 原始类型返回 false", () => {
    expect(isUnifiedTreeNode(null)).toBe(false);
    expect(isUnifiedTreeNode(undefined)).toBe(false);
    expect(isUnifiedTreeNode("string")).toBe(false);
    expect(isUnifiedTreeNode(42)).toBe(false);
    expect(isUnifiedTreeNode(true)).toBe(false);
    expect(isUnifiedTreeNode([])).toBe(false); // 数组也是 object，但缺字段
  });
});

// ============ isUnifiedTimelineEntry 类型守卫 ============

describe("isUnifiedTimelineEntry", () => {
  it("对完整合法对象返回 true", () => {
    expect(
      isUnifiedTimelineEntry({
        id: "entry-1",
        phase: Phase.Encrypting,
        label: "加密中",
        status: "running",
      })
    ).toBe(true);
  });

  it("对含全部可选字段的对象返回 true", () => {
    expect(
      isUnifiedTimelineEntry({
        id: "entry-1",
        phase: Phase.Encrypting,
        icon: "lockClosedOutline",
        label: "加密中",
        time: "12:34:56",
        duration: "5s",
        progress: 75,
        speed: "12.5 MB/s",
        eta: "00:00:30",
        status: "running",
        isCurrent: true,
        isHighlight: false,
        hasExpandableDetail: true,
        expandDetail: {
          startedAt: "12:34:51",
          completedAt: "12:34:56",
          duration: "5s",
          outputPath: "/storage/output/sample.encv",
          error: undefined,
          extra: { cipherMode: "0", version: "4" },
        },
      })
    ).toBe(true);
  });

  it("对 phase 为裸字符串形式的有效对象返回 true", () => {
    expect(
      isUnifiedTimelineEntry({
        id: "entry-1",
        phase: "encrypting",
        label: "加密中",
        status: "running",
      })
    ).toBe(true);
  });

  it("对缺 id 字段的对象返回 false", () => {
    expect(
      isUnifiedTimelineEntry({
        phase: Phase.Encrypting,
        label: "加密中",
        status: "running",
      })
    ).toBe(false);
  });

  it("对缺 label 字段的对象返回 false", () => {
    expect(
      isUnifiedTimelineEntry({
        id: "entry-1",
        phase: Phase.Encrypting,
        status: "running",
      })
    ).toBe(false);
  });

  it("对缺 phase 字段的对象返回 false", () => {
    expect(
      isUnifiedTimelineEntry({
        id: "entry-1",
        label: "加密中",
        status: "running",
      })
    ).toBe(false);
  });

  it("对缺 status 字段的对象返回 false", () => {
    expect(
      isUnifiedTimelineEntry({
        id: "entry-1",
        phase: Phase.Encrypting,
        label: "加密中",
      })
    ).toBe(false);
  });

  it("对 phase 类型非字符串的对象返回 false", () => {
    expect(
      isUnifiedTimelineEntry({
        id: "entry-1",
        phase: 0,
        label: "加密中",
        status: "running",
      })
    ).toBe(false);
  });

  it("对 status 类型非字符串的对象返回 false", () => {
    expect(
      isUnifiedTimelineEntry({
        id: "entry-1",
        phase: Phase.Encrypting,
        label: "加密中",
        status: 1,
      })
    ).toBe(false);
  });

  it("对 null / undefined / 原始类型返回 false", () => {
    expect(isUnifiedTimelineEntry(null)).toBe(false);
    expect(isUnifiedTimelineEntry(undefined)).toBe(false);
    expect(isUnifiedTimelineEntry("string")).toBe(false);
    expect(isUnifiedTimelineEntry(42)).toBe(false);
    expect(isUnifiedTimelineEntry(true)).toBe(false);
    expect(isUnifiedTimelineEntry([])).toBe(false);
  });
});
