/**
 * TaskTimeline 单元测试
 *
 * 覆盖 Task 15 SubTask 15.1-15.7：
 * - 基础渲染：task 有 steps 时渲染对应数量 UnifiedTimelineCard
 * - phase 图标显示（通过 PhaseIcon 子组件）
 * - 进度条显示（step 有 progress 字段时，仅当前 step）
 * - 速率 / ETA 显示（仅当前 step 从 task 级别继承）
 * - 耗时跨度显示 + 最长耗时高亮（isHighlight）
 * - 展开详情卡片化（点击展开显示 outputPath / startedAt / completedAt / duration）
 * - fallback：task.steps 为空时从 phase 序列派生
 * - 状态色：current=蓝(running), completed=绿(success), error=红(failure)
 * - 失败 / 取消态：最后一个事件标记为 failure + 错误信息
 * - 完成态：追加 "completed" 事件
 * - getPhaseLabel 基于 Phase 枚举映射（i18n key）
 */

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it } from "vitest";
import type { EncvTask, TaskStep } from "@encv/shared-components/api/encv";
import PhaseIcon from "@encv/shared-components/components/shared/PhaseIcon.vue";
import UnifiedTimelineCard from "@encv/shared-components/components/shared/UnifiedTimelineCard.vue";
import TaskTimeline from "@encv/shared-components/components/TaskTimeline.vue";
import { Phase } from "@encv/shared-components/lib/workflow/types";

// ion-icon stub：避免 @ionic/vue 全局注册依赖
const IonIconStub = {
  name: "IonIcon",
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" :data-icon="String(icon)" />',
};

/** 构造测试用 TaskStep */
function makeStep(overrides: Partial<TaskStep> = {}): TaskStep {
  return {
    phase: "encrypting",
    startedAt: "2026-06-18T10:00:00Z",
    completedAt: "2026-06-18T10:00:05Z",
    detail: "/storage/emulated/0/output.encv",
    ...overrides,
  };
}

/** 构造测试用 EncvTask（便于覆盖默认值） */
function makeTask(overrides: Partial<EncvTask> = {}): EncvTask {
  return {
    id: "task-1",
    type: "encrypt",
    sourcePath: "/storage/emulated/0/sample.mp4",
    status: "running",
    progress: 50,
    phase: "encrypting",
    speed: "12.5 MB/s",
    eta: "00:01:30",
    createdAt: "2026-06-18T10:00:00Z",
    steps: [],
    ...overrides,
  };
}

function mountTimeline(task: EncvTask) {
  return mount(TaskTimeline, {
    props: { task },
    global: {
      stubs: { "ion-icon": IonIconStub },
    },
  });
}

describe("TaskTimeline - 基础渲染", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('渲染 section-title 为"时间线"', () => {
    const wrapper = mountTimeline(makeTask());
    expect(wrapper.find(".section-title").text()).toBe("时间线");
  });

  it("task 有 steps 时渲染对应数量 UnifiedTimelineCard（steps 数 + created 事件）", () => {
    const task = makeTask({
      steps: [
        makeStep({ phase: "analyzing", startedAt: "2026-06-18T10:00:00Z", completedAt: "2026-06-18T10:00:01Z" }),
        makeStep({ phase: "encrypting", startedAt: "2026-06-18T10:00:01Z", completedAt: "2026-06-18T10:00:05Z" }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // 1 个 created + 2 个 steps = 3
    expect(cards).toHaveLength(3);
  });

  it("task.steps 为空时从 phase 序列派生（7 个 phase + 1 个 created = 8）", () => {
    const task = makeTask({ steps: undefined, phase: "encrypting" });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // 1 个 created + 7 个 fallback phase = 8
    expect(cards).toHaveLength(8);
  });
});

describe("TaskTimeline - phase 图标显示", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("每个 UnifiedTimelineCard 内部渲染 PhaseIcon 子组件", () => {
    const task = makeTask({
      steps: [makeStep({ phase: "analyzing", startedAt: "2026-06-18T10:00:00Z", completedAt: "2026-06-18T10:00:01Z" })],
    });
    const wrapper = mountTimeline(task);
    const phaseIcons = wrapper.findAllComponents(PhaseIcon);
    // created + analyzing = 2
    expect(phaseIcons.length).toBeGreaterThanOrEqual(2);
  });

  it("created 事件使用 Phase.Created 图标", () => {
    const task = makeTask({ steps: [] });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const createdCard = cards[0];
    expect(createdCard.props("entry").phase).toBe(Phase.Created);
  });

  it("encrypting step 使用 Phase.Encrypting 图标", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // 第二个卡片是 encrypting step
    const encryptingCard = cards[1];
    expect(encryptingCard.props("entry").phase).toBe(Phase.Encrypting);
  });
});

describe("TaskTimeline - 进度条显示", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("当前 step 派生 task.progress 字段", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      progress: 65,
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const encryptingCard = cards[1];
    expect(encryptingCard.props("entry").progress).toBe(65);
  });

  it("已完成 step 不派生 progress 字段", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      progress: 65,
      steps: [
        makeStep({
          phase: "analyzing",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:01Z",
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // analyzing step 已完成，不应有 progress
    const analyzingCard = cards[1];
    expect(analyzingCard.props("entry").progress).toBeUndefined();
  });

  it("fallback 模式下当前 phase 派生 progress", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      progress: 42,
      steps: undefined,
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // fallback 顺序：created, analyzing, initializing, preprocessing, encrypting, ...
    // encrypting 是第 5 个（index 4）
    const encryptingCard = cards[4];
    expect(encryptingCard.props("entry").phase).toBe(Phase.Encrypting);
    expect(encryptingCard.props("entry").progress).toBe(42);
  });
});

describe("TaskTimeline - 速率 / ETA 显示", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("当前 step 派生 task.speed 字段", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      speed: "12.5 MB/s",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[1].props("entry").speed).toBe("12.5 MB/s");
  });

  it("当前 step 派生 task.eta 字段", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      eta: "00:01:30",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[1].props("entry").eta).toBe("00:01:30");
  });

  it("已完成 step 不派生 speed / eta", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      speed: "12.5 MB/s",
      eta: "00:01:30",
      steps: [
        makeStep({
          phase: "analyzing",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:01Z",
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const analyzingCard = cards[1];
    expect(analyzingCard.props("entry").speed).toBeUndefined();
    expect(analyzingCard.props("entry").eta).toBeUndefined();
  });
});

describe("TaskTimeline - 耗时跨度 + 最长耗时高亮", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("已完成 step 派生 duration 字段（耗时跨度）", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "analyzing",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:05Z", // 5s
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const analyzingCard = cards[1];
    expect(analyzingCard.props("entry").duration).toBe("5s");
  });

  it("最长耗时的 step 标记 isHighlight=true", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "analyzing",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:02Z", // 2s
        }),
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:02Z",
          completedAt: "2026-06-18T10:00:10Z", // 8s ← 最长
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // cards[0] = created, cards[1] = analyzing (2s), cards[2] = encrypting (8s)
    expect(cards[1].props("entry").isHighlight).toBeFalsy();
    expect(cards[2].props("entry").isHighlight).toBe(true);
  });

  it("最长耗时高亮传给 UnifiedTimelineCard 的 highlight prop", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:10Z", // 10s
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // cards[0] = created (无 duration), cards[1] = encrypting (10s, 最长)
    expect(cards[0].props("highlight")).toBe(false);
    expect(cards[1].props("highlight")).toBe(true);
  });

  it("所有 step 都无 duration 时不高亮任何条目", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined, // 未完成，无 duration
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards.every(c => c.props("entry").isHighlight !== true)).toBe(true);
  });
});

describe("TaskTimeline - 展开详情卡片化", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("有 detail 的 step 标记 hasExpandableDetail=true", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:05Z",
          detail: "/storage/emulated/0/output.encv",
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[1].props("entry").hasExpandableDetail).toBe(true);
  });

  it("expandDetail 包含 outputPath / startedAt / completedAt / duration（v3 Task 7：outputPath 来自 task.outputPath）", () => {
    // 🆕 v3 2026-06-18 Task 7：step.detail 不再直接映射到 outputPath
    //   - step.detail === task.outputPath → outputPath（后端任务完成时覆写最后一步）
    //   - step.detail !== task.outputPath → phaseDetail（phase 描述）
    //   - completed 条目 + 最后一步 → 从 task.outputPath 取 outputPath
    const task = makeTask({
      status: "completed",
      phase: "completed",
      outputPath: "/d/primary/output.encv",
      completedAt: "2026-06-18T10:00:05Z",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:05Z",
          detail: "/d/primary/output.encv", // === task.outputPath → 映射到 outputPath
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // cards[0] = created, cards[1] = encrypting step, cards[2] = completed
    const expandDetail = cards[1].props("entry").expandDetail;
    expect(expandDetail).toBeDefined();
    expect(expandDetail?.outputPath).toBe("/d/primary/output.encv");
    expect(expandDetail?.startedAt).toBeTruthy();
    expect(expandDetail?.completedAt).toBeTruthy();
    expect(expandDetail?.duration).toBe("5s");
  });

  it("v3 Task 7：step.detail 不等于 task.outputPath 时映射到 phaseDetail", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          detail: "加密数据流", // phase 描述，不等于 task.outputPath
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const expandDetail = cards[1].props("entry").expandDetail;
    expect(expandDetail).toBeDefined();
    expect(expandDetail?.phaseDetail).toBe("加密数据流");
    expect(expandDetail?.outputPath).toBeUndefined();
  });

  it("v3 Task 7：created 条目展开显示 sourcePath", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      sourcePath: "/d/primary/input.mp4",
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // cards[0] = created
    const createdEntry = cards[0].props("entry");
    expect(createdEntry.hasExpandableDetail).toBe(true);
    expect(createdEntry.expandDetail?.sourcePath).toBe("/d/primary/input.mp4");
  });

  it("v3 Task 7：encrypting step 展开显示 cryptoSummary", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      cipherMode: 1,
      compressionMode: "zstd",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const expandDetail = cards[1].props("entry").expandDetail;
    expect(expandDetail).toBeDefined();
    expect(expandDetail?.cryptoSummary).toBe("AES-256 · Zstd");
  });

  it("v3 Task 7：completed 条目展开显示 outputPath（用 task.outputPath）", () => {
    const task = makeTask({
      status: "completed",
      phase: "completed",
      outputPath: "/d/primary/output.encv",
      completedAt: "2026-06-18T10:00:10Z",
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // 最后一个 card 是 completed
    const completedEntry = cards[cards.length - 1].props("entry");
    expect(completedEntry.hasExpandableDetail).toBe(true);
    expect(completedEntry.expandDetail?.outputPath).toBe("/d/primary/output.encv");
  });

  it("点击展开后渲染自定义 detail slot（输出路径卡片）", async () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:05Z",
          detail: "/storage/emulated/0/output.encv",
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // 初始未展开
    expect(wrapper.find(".timeline-detail-card").exists()).toBe(false);
    // 点击第二个卡片（encrypting step）的 header 展开
    await cards[1].find(".utc__header").trigger("click");
    // 展开后应渲染自定义 detail slot
    const detailCards = wrapper.findAll(".timeline-detail-card");
    expect(detailCards.length).toBeGreaterThan(0);
    // 验证输出路径卡片存在
    const pathCard = wrapper.findAll(".timeline-detail-card--path");
    expect(pathCard.length).toBe(1);
    expect(pathCard[0].text()).toContain("/storage/emulated/0/output.encv");
  });

  it("展开后渲染耗时卡片（最长耗时高亮）", async () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:10Z", // 10s 最长
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    await cards[1].find(".utc__header").trigger("click");
    // 验证耗时卡片存在
    const detailCards = wrapper.findAll(".timeline-detail-card");
    const durationCard = detailCards.find(c => c.text().includes("10s"));
    expect(durationCard).toBeDefined();
    // 最长耗时应高亮
    const highlightCard = wrapper.findAll(".timeline-detail-card--highlight");
    expect(highlightCard.length).toBe(1);
    expect(highlightCard[0].text()).toContain("10s");
  });
});

describe("TaskTimeline - 状态色", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("当前 step 状态为 running（蓝色边框）", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[1].props("entry").status).toBe("running");
    expect(cards[1].props("entry").isCurrent).toBe(true);
  });

  it("已完成 step 状态为 success（绿色边框）", () => {
    const task = makeTask({
      status: "running",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "analyzing",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:01Z",
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[1].props("entry").status).toBe("success");
  });

  it("失败任务最后一个 step 状态为 failure（红色边框）", () => {
    const task = makeTask({
      status: "failed",
      phase: "encrypting",
      error: "FFMPEG exited with code 1",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const lastCard = cards[cards.length - 1];
    expect(lastCard.props("entry").status).toBe("failure");
  });

  it("失败任务的错误信息附加到最后一个事件的 expandDetail.error", () => {
    const task = makeTask({
      status: "failed",
      phase: "encrypting",
      error: "FFMPEG exited with code 1",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const lastCard = cards[cards.length - 1];
    expect(lastCard.props("entry").expandDetail?.error).toBe("FFMPEG exited with code 1");
    expect(lastCard.props("entry").hasExpandableDetail).toBe(true);
  });

  it('取消任务最后一个 step 状态为 failure + label 为"已取消"', () => {
    const task = makeTask({
      status: "cancelled",
      phase: "encrypting",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const lastCard = cards[cards.length - 1];
    expect(lastCard.props("entry").status).toBe("failure");
    expect(lastCard.props("entry").label).toBe("已取消");
  });
});

describe("TaskTimeline - 完成态追加事件", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("task.status=completed 时追加 Phase.Completed 事件", () => {
    const task = makeTask({
      status: "completed",
      phase: "completed",
      completedAt: "2026-06-18T10:01:00Z",
      steps: [
        makeStep({
          phase: "encrypting",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: "2026-06-18T10:00:30Z",
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // created + encrypting + completed = 3
    expect(cards).toHaveLength(3);
    const completedCard = cards[2];
    expect(completedCard.props("entry").phase).toBe(Phase.Completed);
    expect(completedCard.props("entry").status).toBe("success");
  });
});

describe("TaskTimeline - getPhaseLabel 基于 Phase 枚举映射", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("analyzing phase 映射到 i18n key tasks.phaseAnalyzing", () => {
    const task = makeTask({
      status: "running",
      phase: "analyzing",
      steps: [
        makeStep({
          phase: "analyzing",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // cards[1] 是 analyzing step
    expect(cards[1].props("entry").label).toBe("分析文件中...");
  });

  it("packing phase 映射到 i18n key tasks.phasePacking", () => {
    const task = makeTask({
      status: "running",
      phase: "packing",
      steps: [
        makeStep({
          phase: "packing",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[1].props("entry").label).toBe("打包中...");
  });

  it("未知 phase 字符串原样返回 label", () => {
    const task = makeTask({
      status: "running",
      phase: "unknown_phase",
      steps: [
        makeStep({
          phase: "unknown_phase",
          startedAt: "2026-06-18T10:00:00Z",
          completedAt: undefined,
        }),
      ],
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // 未知 phase 降级为 Created 枚举，但 label 原样返回字符串
    expect(cards[1].props("entry").phase).toBe(Phase.Created);
    expect(cards[1].props("entry").label).toBe("unknown_phase");
  });
});

describe("TaskTimeline - fallback 模式", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("task.steps 为空 + status=completed 时所有 phase 标记为 success", () => {
    const task = makeTask({
      status: "completed",
      phase: "completed",
      steps: undefined,
      completedAt: "2026-06-18T10:01:00Z",
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // 所有 fallback phase 应为 success（因为 isTerminal=true）
    for (const card of cards) {
      expect(card.props("entry").status).toBe("success");
    }
  });

  it("fallback 模式下当前 phase 标记 isCurrent=true", () => {
    const task = makeTask({
      status: "running",
      phase: "preprocessing",
      steps: undefined,
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    // fallback 顺序：created, analyzing, initializing, preprocessing, ...
    // preprocessing 是第 4 个（index 3）
    const preprocessingCard = cards[3];
    expect(preprocessingCard.props("entry").phase).toBe(Phase.Preprocessing);
    expect(preprocessingCard.props("entry").isCurrent).toBe(true);
    expect(preprocessingCard.props("entry").status).toBe("running");
  });
});

// 🆕 2026-06-18 Task 18：crypto params 摘要显示在 created 条目的 meta 字段
describe("TaskTimeline - crypto params 摘要 (Task 18)", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('cipherMode=1 (AES-256) + compressionMode=zstd → meta 为 "AES-256 · Zstd"', () => {
    const task = makeTask({
      cipherMode: 1,
      compressionMode: "zstd",
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    const createdCard = cards[0];
    expect(createdCard.props("entry").meta).toBe("AES-256 · Zstd");
  });

  it('cipherMode=0 (AES-128) + compressionMode=none → meta 为 "AES-128 · none"', () => {
    const task = makeTask({
      cipherMode: 0,
      compressionMode: "none",
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[0].props("entry").meta).toBe("AES-128 · none");
  });

  it('仅 cipherMode=1（无 compressionMode）→ meta 为 "AES-256"', () => {
    const task = makeTask({
      cipherMode: 1,
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[0].props("entry").meta).toBe("AES-256");
  });

  it('仅 compressionMode=zstd（无 cipherMode）→ meta 为 "Zstd"', () => {
    const task = makeTask({
      compressionMode: "zstd",
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[0].props("entry").meta).toBe("Zstd");
  });

  it("旧任务无 crypto 字段 → meta 为 undefined（不显示空摘要）", () => {
    const task = makeTask();
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[0].props("entry").meta).toBeUndefined();
  });

  it("cipherMode=null（旧任务显式 null）→ meta 不包含 cipher", () => {
    const task = makeTask({
      cipherMode: null as unknown as number,
      compressionMode: "zstd",
    });
    const wrapper = mountTimeline(task);
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[0].props("entry").meta).toBe("Zstd");
  });
});
