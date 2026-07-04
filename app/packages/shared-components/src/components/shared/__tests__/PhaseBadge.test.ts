/**
 * PhaseBadge 单元测试
 *
 * 覆盖：
 * - 9 个 Phase 值渲染对应 label
 * - 2 个终态 status 值（failed / cancelled）渲染对应 label
 * - 自定义 label 覆盖默认
 * - class 包含 phase 标识
 * - 包含 PhaseIcon 子组件
 */

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import PhaseBadge from "@encv/shared-components/components/shared/PhaseBadge.vue";
import type { PhaseIconValue } from "@encv/shared-components/components/shared/PhaseIcon.vue";
import PhaseIcon from "@encv/shared-components/components/shared/PhaseIcon.vue";
import { ALL_PHASES, Phase } from "@encv/shared-components/lib/workflow/types";

// ion-icon stub：PhaseIcon 内部使用 ion-icon，需 stub 避免 @ionic/vue 全局注册依赖
const IonIconStub = {
  name: "IonIcon",
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" />',
};

function mountPhaseBadge(phase: PhaseIconValue, label?: string) {
  return mount(PhaseBadge, {
    props: label !== undefined ? { phase, label } : { phase },
    global: {
      stubs: { "ion-icon": IonIconStub },
    },
  });
}

// Phase / Status → 期望的默认中文 label
const EXPECTED_LABEL_MAP: Record<string, string> = {
  [Phase.Created]: "已创建",
  [Phase.Analyzing]: "分析中",
  [Phase.Initializing]: "初始化",
  [Phase.Preprocessing]: "预处理",
  [Phase.Encrypting]: "加密中",
  [Phase.Decrypting]: "解密中",
  [Phase.Packing]: "打包中",
  [Phase.Verifying]: "校验中",
  [Phase.Completed]: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

describe("PhaseBadge - 9 个 Phase 值渲染对应默认 label", () => {
  it.each(ALL_PHASES.map(p => [p, p] as const))('Phase.%s 渲染默认 label "%s"', phase => {
    const wrapper = mountPhaseBadge(phase);
    const labelEl = wrapper.find(".phase-badge__label");
    expect(labelEl.exists()).toBe(true);
    expect(labelEl.text()).toBe(EXPECTED_LABEL_MAP[phase]);
  });
});

describe("PhaseBadge - class 包含 phase 标识", () => {
  it.each(ALL_PHASES.map(p => [p, p] as const))("Phase.%s 根元素包含 phase-badge--%s class", phase => {
    const wrapper = mountPhaseBadge(phase);
    const badge = wrapper.find(".phase-badge");
    expect(badge.exists()).toBe(true);
    expect(badge.classes()).toContain("phase-badge");
    expect(badge.classes()).toContain(`phase-badge--${phase}`);
  });
});

describe("PhaseBadge - 自定义 label 覆盖默认", () => {
  it("传入 label prop 时覆盖默认 PHASE_LABEL_MAP", () => {
    const wrapper = mountPhaseBadge(Phase.Encrypting, "自定义加密标签");
    expect(wrapper.find(".phase-badge__label").text()).toBe("自定义加密标签");
  });

  it("传入空字符串 label 时显示空字符串（不 fallback 到默认）", () => {
    const wrapper = mountPhaseBadge(Phase.Created, "");
    expect(wrapper.find(".phase-badge__label").text()).toBe("");
  });

  it("不同 phase 传入相同自定义 label 都生效", () => {
    const w1 = mountPhaseBadge(Phase.Analyzing, "处理中");
    const w2 = mountPhaseBadge(Phase.Packing, "处理中");
    expect(w1.find(".phase-badge__label").text()).toBe("处理中");
    expect(w2.find(".phase-badge__label").text()).toBe("处理中");
  });

  it("label prop 变化时文本跟随更新", async () => {
    const wrapper = mountPhaseBadge(Phase.Created);
    expect(wrapper.find(".phase-badge__label").text()).toBe("已创建");
    await wrapper.setProps({ label: "新标签" });
    expect(wrapper.find(".phase-badge__label").text()).toBe("新标签");
  });
});

describe("PhaseBadge - 包含 PhaseIcon 子组件", () => {
  it("渲染 PhaseIcon 子组件", () => {
    const wrapper = mountPhaseBadge(Phase.Created);
    // PhaseIcon 内部渲染 ion-icon stub
    expect(wrapper.findComponent(PhaseIcon).exists()).toBe(true);
    expect(wrapper.find(".ion-icon-stub").exists()).toBe(true);
  });

  it("PhaseIcon 接收正确的 phase prop", () => {
    const wrapper = mountPhaseBadge(Phase.Encrypting);
    const phaseIcon = wrapper.findComponent(PhaseIcon);
    expect(phaseIcon.props("phase")).toBe(Phase.Encrypting);
  });

  it("phase prop 变化时 PhaseIcon 跟随更新", async () => {
    const wrapper = mountPhaseBadge(Phase.Created);
    expect(wrapper.findComponent(PhaseIcon).props("phase")).toBe(Phase.Created);
    await wrapper.setProps({ phase: Phase.Completed });
    expect(wrapper.findComponent(PhaseIcon).props("phase")).toBe(Phase.Completed);
  });
});

describe("PhaseBadge - 基础渲染", () => {
  it("渲染单个 phase-badge 根元素", () => {
    const wrapper = mountPhaseBadge(Phase.Created);
    expect(wrapper.findAll(".phase-badge")).toHaveLength(1);
  });

  it("渲染单个 phase-badge__label 元素", () => {
    const wrapper = mountPhaseBadge(Phase.Created);
    expect(wrapper.findAll(".phase-badge__label")).toHaveLength(1);
  });
});

describe("PhaseBadge - 2 个终态 status 值渲染对应 label", () => {
  it('failed 渲染默认 label "失败"', () => {
    const wrapper = mountPhaseBadge("failed");
    expect(wrapper.find(".phase-badge__label").text()).toBe("失败");
  });

  it('cancelled 渲染默认 label "已取消"', () => {
    const wrapper = mountPhaseBadge("cancelled");
    expect(wrapper.find(".phase-badge__label").text()).toBe("已取消");
  });

  it("failed 根元素包含 phase-badge--failed class", () => {
    const wrapper = mountPhaseBadge("failed");
    expect(wrapper.find(".phase-badge").classes()).toContain("phase-badge--failed");
  });

  it("cancelled 根元素包含 phase-badge--cancelled class", () => {
    const wrapper = mountPhaseBadge("cancelled");
    expect(wrapper.find(".phase-badge").classes()).toContain("phase-badge--cancelled");
  });
});
