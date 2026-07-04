/**
 * RelevanceBadge 单元测试
 *
 * 覆盖：
 * - 三档 tier（high >=0.6 / mid >=0.3 / low >0）正确分类
 * - score=0 / undefined 时不渲染
 * - 百分比 label 正确（Math.round）
 * - class 包含对应 tier 标识
 * - 包含 ion-icon 子组件
 *
 * 与 PhaseBadge.test.ts 范式一致（同属 shared 组件）
 */

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RelevanceBadge from "@encv/shared-components/components/shared/RelevanceBadge.vue";

// ion-icon stub：RelevanceBadge 内部使用 ion-icon，需 stub 避免 @ionic/vue 全局注册依赖
const IonIconStub = {
  name: "IonIcon",
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" />',
};

function mountRelevanceBadge(score?: number) {
  return mount(RelevanceBadge, {
    props: score !== undefined ? { score } : {},
    global: {
      stubs: { "ion-icon": IonIconStub },
    },
  });
}

describe("RelevanceBadge", () => {
  it("score 未设置时不渲染", () => {
    const wrapper = mountRelevanceBadge();
    expect(wrapper.find(".relevance-badge").exists()).toBe(false);
  });

  it("score=0 时不渲染", () => {
    const wrapper = mountRelevanceBadge(0);
    expect(wrapper.find(".relevance-badge").exists()).toBe(false);
  });

  it("high tier（>=0.6）正确分类", () => {
    const wrapper = mountRelevanceBadge(0.65);
    const badge = wrapper.find(".relevance-badge");
    expect(badge.exists()).toBe(true);
    expect(badge.classes()).toContain("relevance-badge--high");
    expect(badge.text()).toContain("65%");
  });

  it("high tier 边界值 0.6 正确分类", () => {
    const wrapper = mountRelevanceBadge(0.6);
    const badge = wrapper.find(".relevance-badge");
    expect(badge.classes()).toContain("relevance-badge--high");
  });

  it("mid tier（>=0.3 且 <0.6）正确分类", () => {
    const wrapper = mountRelevanceBadge(0.42);
    const badge = wrapper.find(".relevance-badge");
    expect(badge.exists()).toBe(true);
    expect(badge.classes()).toContain("relevance-badge--mid");
    expect(badge.text()).toContain("42%");
  });

  it("mid tier 边界值 0.3 正确分类", () => {
    const wrapper = mountRelevanceBadge(0.3);
    const badge = wrapper.find(".relevance-badge");
    expect(badge.classes()).toContain("relevance-badge--mid");
  });

  it("low tier（>0 且 <0.3）正确分类", () => {
    const wrapper = mountRelevanceBadge(0.12);
    const badge = wrapper.find(".relevance-badge");
    expect(badge.exists()).toBe(true);
    expect(badge.classes()).toContain("relevance-badge--low");
    expect(badge.text()).toContain("12%");
  });

  it("百分比 label 四舍五入到整数", () => {
    const wrapper = mountRelevanceBadge(0.6494);
    const badge = wrapper.find(".relevance-badge");
    // 0.6494 * 100 = 64.94 → Math.round = 65
    expect(badge.text()).toContain("65%");
  });

  it("包含 ion-icon 子组件", () => {
    const wrapper = mountRelevanceBadge(0.5);
    expect(wrapper.find(".ion-icon-stub").exists()).toBe(true);
  });

  it("score=1.0 显示 100%", () => {
    const wrapper = mountRelevanceBadge(1.0);
    const badge = wrapper.find(".relevance-badge");
    expect(badge.text()).toContain("100%");
  });
});
