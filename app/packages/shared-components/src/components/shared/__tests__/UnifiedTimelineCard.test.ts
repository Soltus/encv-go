/**
 * UnifiedTimelineCard 单元测试
 *
 * 覆盖 Task 11 SubTask 11.7：
 * - 基础渲染：label / meta / time / duration 显示
 * - 状态色 class：utc--success / utc--failure / utc--running 等
 * - current / highlight class
 * - 进度条渲染（progress 字段存在时显示，缺失时隐藏）
 * - 速率 / ETA 渲染（字段存在时显示，缺失时隐藏）
 * - 展开交互：点击 header 切换 expanded；emit update:expanded + toggle
 * - 默认 detail slot 渲染：startedAt / completedAt / duration / outputPath / error / extra
 * - 自定义 detail slot 渲染
 * - hasExpandableDetail=false 时不显示 chevron 和 detail
 * - 受控 / 非受控展开模式
 * - 错误提示渲染
 * - 自定义 icon / meta slot
 */

import PhaseIcon from "@encv/shared-components/components/shared/PhaseIcon.vue";
import UnifiedTimelineCard from "@encv/shared-components/components/shared/UnifiedTimelineCard.vue";
import { Phase, type StepStatus, type UnifiedTimelineEntry } from "@encv/shared-components/lib/workflow/types";
import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

// ion-icon stub：避免 @ionic/vue 全局注册依赖
const IonIconStub = {
  name: "IonIcon",
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" :data-icon="String(icon)" />',
};

/** 构造测试用 UnifiedTimelineEntry（便于覆盖默认值） */
function makeEntry(overrides: Partial<UnifiedTimelineEntry> = {}): UnifiedTimelineEntry {
  return {
    id: "test-1",
    phase: Phase.Encrypting,
    label: "加密中",
    status: "running",
    ...overrides,
  };
}

/** 构建组件 props 对象（受控 / 非受控模式自动适配） */
function buildProps(
  entry: UnifiedTimelineEntry,
  options: {
    expanded?: boolean;
    defaultExpanded?: boolean;
    highlight?: boolean;
  } = {}
) {
  const props: {
    entry: UnifiedTimelineEntry;
    expanded?: boolean;
    defaultExpanded?: boolean;
    highlight?: boolean;
  } = { entry };
  if (options.expanded !== undefined) props.expanded = options.expanded;
  if (options.defaultExpanded !== undefined) props.defaultExpanded = options.defaultExpanded;
  if (options.highlight !== undefined) props.highlight = options.highlight;
  return props;
}

function mountCard(
  entry: UnifiedTimelineEntry,
  options: {
    expanded?: boolean;
    defaultExpanded?: boolean;
    highlight?: boolean;
    slots?: Record<string, string>;
  } = {}
) {
  return mount(UnifiedTimelineCard, {
    props: buildProps(entry, options),
    global: {
      stubs: { "ion-icon": IonIconStub },
    },
    slots: options.slots,
  });
}

describe("UnifiedTimelineCard - 基础渲染", () => {
  it("渲染 label 文本", () => {
    const wrapper = mountCard(makeEntry({ label: "自定义阶段标签" }));
    expect(wrapper.find(".utc__label").text()).toBe("自定义阶段标签");
  });

  it("渲染 meta 文本（entry.meta 存在时）", () => {
    const wrapper = mountCard(makeEntry({ meta: "mp4 · 1080p" }));
    expect(wrapper.find(".utc__meta").text()).toBe("mp4 · 1080p");
  });

  it("meta 缺失时不渲染 .utc__meta", () => {
    const wrapper = mountCard(makeEntry({ meta: undefined }));
    expect(wrapper.find(".utc__meta").exists()).toBe(false);
  });

  it("渲染 time 文本（entry.time 存在时）", () => {
    const wrapper = mountCard(makeEntry({ time: "14:30:25" }));
    expect(wrapper.find(".utc__time").text()).toBe("14:30:25");
  });

  it("time 缺失时不渲染 .utc__time", () => {
    const wrapper = mountCard(makeEntry({ time: undefined }));
    expect(wrapper.find(".utc__time").exists()).toBe(false);
  });

  it("渲染 duration 文本（entry.duration 存在时）", () => {
    const wrapper = mountCard(makeEntry({ duration: "1.23s" }));
    expect(wrapper.find(".utc__duration").text()).toBe("1.23s");
  });

  it("duration 缺失时不渲染 .utc__duration", () => {
    const wrapper = mountCard(makeEntry({ duration: undefined }));
    expect(wrapper.find(".utc__duration").exists()).toBe(false);
  });
});

describe("UnifiedTimelineCard - 状态色 class", () => {
  const statusCases: Array<[StepStatus, string]> = [
    ["success", "utc--success"],
    ["failure", "utc--failure"],
    ["running", "utc--running"],
    ["cancelled", "utc--cancelled"],
    ["skipped", "utc--skipped"],
    ["timed_out", "utc--timed_out"],
    ["queued", "utc--queued"],
    ["pending", "utc--pending"],
  ];

  it.each(statusCases)("status=%s 时根元素包含 class %s", (status, expectedClass) => {
    const wrapper = mountCard(makeEntry({ status }));
    expect(wrapper.find(".utc").classes()).toContain(expectedClass);
  });
});

describe("UnifiedTimelineCard - current / highlight class", () => {
  it("isCurrent=true 时根元素包含 utc--current class", () => {
    const wrapper = mountCard(makeEntry({ isCurrent: true }));
    expect(wrapper.find(".utc").classes()).toContain("utc--current");
  });

  it("isCurrent=false 时根元素不包含 utc--current class", () => {
    const wrapper = mountCard(makeEntry({ isCurrent: false }));
    expect(wrapper.find(".utc").classes()).not.toContain("utc--current");
  });

  it("highlight=true 时根元素包含 utc--highlight class", () => {
    const wrapper = mountCard(makeEntry(), { highlight: true });
    expect(wrapper.find(".utc").classes()).toContain("utc--highlight");
  });

  it("highlight=true 且 duration 存在时 duration 元素包含 utc__duration--highlight class", () => {
    const wrapper = mountCard(makeEntry({ duration: "5.67s" }), { highlight: true });
    expect(wrapper.find(".utc__duration").classes()).toContain("utc__duration--highlight");
  });

  it("highlight=false 时 duration 元素不包含 utc__duration--highlight class", () => {
    const wrapper = mountCard(makeEntry({ duration: "5.67s" }), { highlight: false });
    expect(wrapper.find(".utc__duration").classes()).not.toContain("utc__duration--highlight");
  });
});

describe("UnifiedTimelineCard - 进度条渲染", () => {
  it("progress 字段存在时渲染进度条", () => {
    const wrapper = mountCard(makeEntry({ progress: 65 }));
    expect(wrapper.find(".utc__progress-wrap").exists()).toBe(true);
    expect(wrapper.find(".utc__progress-text").text()).toBe("65%");
  });

  it("progress=0 时也渲染进度条（0 是有效值）", () => {
    const wrapper = mountCard(makeEntry({ progress: 0 }));
    expect(wrapper.find(".utc__progress-wrap").exists()).toBe(true);
    expect(wrapper.find(".utc__progress-text").text()).toBe("0%");
  });

  it("progress 缺失时不渲染进度条", () => {
    const wrapper = mountCard(makeEntry({ progress: undefined }));
    expect(wrapper.find(".utc__progress-wrap").exists()).toBe(false);
  });

  it("progress-fill 宽度跟随 progress 值", () => {
    const wrapper = mountCard(makeEntry({ progress: 75 }));
    const fill = wrapper.find(".utc__progress-fill");
    expect(fill.attributes("style")).toContain("width: 75%");
  });
});

describe("UnifiedTimelineCard - 速率 / ETA 渲染", () => {
  it("speed 字段存在时渲染速率", () => {
    const wrapper = mountCard(makeEntry({ speed: "12.5 MB/s" }));
    const metrics = wrapper.findAll(".utc__metric");
    const speedMetric = metrics.find(m => m.text().includes("12.5 MB/s"));
    expect(speedMetric).toBeDefined();
  });

  it("speed 缺失时不渲染速率", () => {
    const wrapper = mountCard(makeEntry({ speed: undefined, eta: undefined, progress: undefined }));
    const metrics = wrapper.findAll(".utc__metric");
    expect(metrics).toHaveLength(0);
  });

  it("eta 字段存在时渲染 ETA", () => {
    const wrapper = mountCard(makeEntry({ eta: "00:01:30" }));
    const metrics = wrapper.findAll(".utc__metric");
    const etaMetric = metrics.find(m => m.text().includes("00:01:30"));
    expect(etaMetric).toBeDefined();
  });

  it("eta 缺失时不渲染 ETA", () => {
    const wrapper = mountCard(makeEntry({ eta: undefined, speed: undefined, progress: undefined }));
    expect(wrapper.findAll(".utc__metric")).toHaveLength(0);
  });

  it("progress / speed / eta 全部缺失时不渲染 .utc__metrics", () => {
    const wrapper = mountCard(makeEntry({ progress: undefined, speed: undefined, eta: undefined }));
    expect(wrapper.find(".utc__metrics").exists()).toBe(false);
  });
});

describe("UnifiedTimelineCard - 展开交互", () => {
  it("点击 header 切换 expanded（非受控模式）", async () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }));
    // 初始未展开
    expect(wrapper.find(".utc__detail").exists()).toBe(false);
    // 点击展开
    await wrapper.find(".utc__header").trigger("click");
    expect(wrapper.find(".utc__detail").exists()).toBe(true);
    // 再次点击收起
    await wrapper.find(".utc__header").trigger("click");
    expect(wrapper.find(".utc__detail").exists()).toBe(false);
  });

  it("点击 header 时 emit update:expanded 事件", async () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }));
    await wrapper.find(".utc__header").trigger("click");
    const updateEvents = wrapper.emitted("update:expanded");
    expect(updateEvents).toHaveLength(1);
    expect(updateEvents![0]).toEqual([true]);
  });

  it("点击 header 时 emit toggle 事件", async () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }));
    await wrapper.find(".utc__header").trigger("click");
    const toggleEvents = wrapper.emitted("toggle");
    expect(toggleEvents).toHaveLength(1);
    expect(toggleEvents![0]).toEqual([true]);
  });

  it("hasExpandableDetail=false 时点击 header 不切换展开", async () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: false }));
    await wrapper.find(".utc__header").trigger("click");
    expect(wrapper.emitted("update:expanded")).toBeUndefined();
    expect(wrapper.emitted("toggle")).toBeUndefined();
  });

  it("hasExpandableDetail=false 时不渲染 chevron 图标", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: false }));
    expect(wrapper.find(".utc__chevron").exists()).toBe(false);
  });

  it("hasExpandableDetail=true 时渲染 chevron 图标", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }));
    expect(wrapper.find(".utc__chevron").exists()).toBe(true);
  });

  it("hasExpandableDetail=false 时不渲染 detail 区域", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: false, expandDetail: { duration: "1s" } }));
    expect(wrapper.find(".utc__detail").exists()).toBe(false);
  });
});

describe("UnifiedTimelineCard - 受控 / 非受控展开模式", () => {
  it("非受控模式：defaultExpanded=true 时初始展开", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }), { defaultExpanded: true });
    expect(wrapper.find(".utc__detail").exists()).toBe(true);
  });

  it("非受控模式：defaultExpanded=false 时初始收起", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }), { defaultExpanded: false });
    expect(wrapper.find(".utc__detail").exists()).toBe(false);
  });

  it("受控模式：expanded=true 时展开", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }), { expanded: true });
    expect(wrapper.find(".utc__detail").exists()).toBe(true);
  });

  it("受控模式：expanded=false 时收起", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }), { expanded: false });
    expect(wrapper.find(".utc__detail").exists()).toBe(false);
  });

  it("受控模式：点击 header 不改变内部状态，只 emit 事件", async () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }), { expanded: false });
    await wrapper.find(".utc__header").trigger("click");
    // 受控模式下，prop 没变，detail 仍然不显示
    expect(wrapper.find(".utc__detail").exists()).toBe(false);
    // 但事件已 emit，父组件应响应
    expect(wrapper.emitted("update:expanded")![0]).toEqual([true]);
  });
});

describe("UnifiedTimelineCard - 默认 detail slot 渲染", () => {
  it("渲染 startedAt 卡片", () => {
    const wrapper = mountCard(
      makeEntry({
        hasExpandableDetail: true,
        expandDetail: { startedAt: "2026-06-18 14:30:00" },
      }),
      { defaultExpanded: true }
    );
    const cards = wrapper.findAll(".utc__detail-card");
    const startedCard = cards.find(c => c.find(".utc__detail-label").text() === "开始时间");
    expect(startedCard).toBeDefined();
    expect(startedCard!.find(".utc__detail-value").text()).toBe("2026-06-18 14:30:00");
  });

  it("渲染 completedAt 卡片", () => {
    const wrapper = mountCard(
      makeEntry({
        hasExpandableDetail: true,
        expandDetail: { completedAt: "2026-06-18 14:31:00" },
      }),
      { defaultExpanded: true }
    );
    const cards = wrapper.findAll(".utc__detail-card");
    const completedCard = cards.find(c => c.find(".utc__detail-label").text() === "完成时间");
    expect(completedCard).toBeDefined();
    expect(completedCard!.find(".utc__detail-value").text()).toBe("2026-06-18 14:31:00");
  });

  it("渲染 duration 卡片", () => {
    const wrapper = mountCard(
      makeEntry({
        hasExpandableDetail: true,
        expandDetail: { duration: "1m 30s" },
      }),
      { defaultExpanded: true }
    );
    const cards = wrapper.findAll(".utc__detail-card");
    const durationCard = cards.find(c => c.find(".utc__detail-label").text() === "耗时");
    expect(durationCard).toBeDefined();
    expect(durationCard!.find(".utc__detail-value").text()).toBe("1m 30s");
  });

  it("渲染 outputPath 卡片（mono 字体）", () => {
    const wrapper = mountCard(
      makeEntry({
        hasExpandableDetail: true,
        expandDetail: { outputPath: "/storage/emulated/0/output.encv" },
      }),
      { defaultExpanded: true }
    );
    const cards = wrapper.findAll(".utc__detail-card");
    const outputCard = cards.find(c => c.find(".utc__detail-label").text() === "输出路径");
    expect(outputCard).toBeDefined();
    expect(outputCard!.find(".utc__detail-value").text()).toBe("/storage/emulated/0/output.encv");
    expect(outputCard!.find(".utc__detail-value").classes()).toContain("utc__detail-value--mono");
  });

  it("渲染 error 卡片（带 error 样式）", () => {
    const wrapper = mountCard(
      makeEntry({
        hasExpandableDetail: true,
        expandDetail: { error: "FFMPEG exited with code 1" },
      }),
      { defaultExpanded: true }
    );
    const errorCard = wrapper.find(".utc__detail-card--error");
    expect(errorCard.exists()).toBe(true);
    expect(errorCard.find(".utc__detail-label").text()).toBe("错误信息");
    expect(errorCard.find(".utc__detail-value").text()).toBe("FFMPEG exited with code 1");
  });

  it("渲染 extra 键值对卡片", () => {
    const wrapper = mountCard(
      makeEntry({
        hasExpandableDetail: true,
        expandDetail: { extra: { 编码器: "libx264", 码率: "2M" } },
      }),
      { defaultExpanded: true }
    );
    const cards = wrapper.findAll(".utc__detail-card");
    const encoderCard = cards.find(c => c.find(".utc__detail-label").text() === "编码器");
    const bitrateCard = cards.find(c => c.find(".utc__detail-label").text() === "码率");
    expect(encoderCard).toBeDefined();
    expect(encoderCard!.find(".utc__detail-value").text()).toBe("libx264");
    expect(bitrateCard).toBeDefined();
    expect(bitrateCard!.find(".utc__detail-value").text()).toBe("2M");
  });

  it("expandDetail 缺失时不渲染任何 detail 卡片", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: undefined }), { defaultExpanded: true });
    expect(wrapper.findAll(".utc__detail-card")).toHaveLength(0);
  });
});

describe("UnifiedTimelineCard - 自定义 detail slot", () => {
  it("提供 detail slot 时覆盖默认渲染", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }), {
      defaultExpanded: true,
      slots: {
        detail: '<div class="custom-detail">自定义详情内容</div>',
      },
    });
    expect(wrapper.find(".custom-detail").exists()).toBe(true);
    expect(wrapper.find(".custom-detail").text()).toBe("自定义详情内容");
    // 默认卡片不应出现
    expect(wrapper.find(".utc__detail-card").exists()).toBe(false);
  });

  it("detail slot 接收 entry 作为 slot prop", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, label: "原始标签" }), {
      defaultExpanded: true,
      slots: {
        detail: '<template #detail="{ entry }"><div class="slot-entry">{{ entry.label }}</div></template>',
      },
    });
    expect(wrapper.find(".slot-entry").text()).toBe("原始标签");
  });
});

describe("UnifiedTimelineCard - 错误提示渲染", () => {
  it("expandDetail.error 存在时渲染 error-hint（无需展开）", () => {
    const wrapper = mountCard(
      makeEntry({
        hasExpandableDetail: true,
        expandDetail: { error: "快速错误预览" },
      })
    );
    expect(wrapper.find(".utc__error-hint").exists()).toBe(true);
    expect(wrapper.find(".utc__error-hint").text()).toContain("快速错误预览");
  });

  it("expandDetail.error 缺失时不渲染 error-hint", () => {
    const wrapper = mountCard(makeEntry({ hasExpandableDetail: true, expandDetail: { duration: "1s" } }));
    expect(wrapper.find(".utc__error-hint").exists()).toBe(false);
  });
});

describe("UnifiedTimelineCard - 自定义 icon / meta slot", () => {
  it("提供 icon slot 时覆盖默认 PhaseIcon", () => {
    const wrapper = mountCard(makeEntry({ phase: Phase.Encrypting }), {
      slots: {
        icon: '<span class="custom-icon">🎬</span>',
      },
    });
    expect(wrapper.find(".custom-icon").exists()).toBe(true);
    expect(wrapper.find(".custom-icon").text()).toBe("🎬");
    // 默认 PhaseIcon 不应渲染
    expect(wrapper.findComponent(PhaseIcon).exists()).toBe(false);
  });

  it("未提供 icon slot 时使用默认 PhaseIcon", () => {
    const wrapper = mountCard(makeEntry({ phase: Phase.Encrypting }));
    expect(wrapper.findComponent(PhaseIcon).exists()).toBe(true);
    expect(wrapper.findComponent(PhaseIcon).props("phase")).toBe(Phase.Encrypting);
  });

  it("提供 meta slot 时覆盖默认 meta 文本", () => {
    const wrapper = mountCard(makeEntry({ meta: "默认 meta" }), {
      slots: {
        meta: '<span class="custom-meta">自定义 meta</span>',
      },
    });
    expect(wrapper.find(".custom-meta").exists()).toBe(true);
    expect(wrapper.find(".custom-meta").text()).toBe("自定义 meta");
    // 默认 .utc__meta 不应渲染
    expect(wrapper.find(".utc__meta").exists()).toBe(false);
  });
});

describe("UnifiedTimelineCard - PhaseIcon 集成", () => {
  it("默认渲染 PhaseIcon 子组件并传入 phase prop", () => {
    const wrapper = mountCard(makeEntry({ phase: Phase.Packing }));
    const phaseIcon = wrapper.findComponent(PhaseIcon);
    expect(phaseIcon.exists()).toBe(true);
    expect(phaseIcon.props("phase")).toBe(Phase.Packing);
  });

  it("phase prop 变化时 PhaseIcon 跟随更新", async () => {
    const wrapper = mountCard(makeEntry({ phase: Phase.Created }));
    expect(wrapper.findComponent(PhaseIcon).props("phase")).toBe(Phase.Created);
    await wrapper.setProps({ entry: makeEntry({ phase: Phase.Completed }) });
    expect(wrapper.findComponent(PhaseIcon).props("phase")).toBe(Phase.Completed);
  });
});
