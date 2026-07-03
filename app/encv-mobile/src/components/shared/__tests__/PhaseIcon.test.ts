/**
 * PhaseIcon 单元测试
 *
 * 覆盖：
 * - 9 个 Phase 值渲染对应 ion-icon
 * - 2 个终态 status 值（failed / cancelled）渲染对应 ion-icon
 * - 未知 phase 用 fallback（helpCircleOutline）
 * - class 包含 phase 标识
 * - size props 控制图标尺寸
 */

import { mount } from "@vue/test-utils";
import {
  banOutline,
  checkmarkCircleOutline,
  closeCircleOutline,
  cloudUploadOutline,
  codeSlashOutline,
  cubeOutline,
  helpCircleOutline,
  lockClosedOutline,
  lockOpenOutline,
  playOutline,
  searchOutline,
  shieldCheckmarkOutline,
} from "ionicons/icons";
import { describe, expect, it } from "vitest";
import PhaseIcon from "@/components/shared/PhaseIcon.vue";
import { ALL_PHASES, Phase } from "@/lib/workflow/types";

// ion-icon stub：捕获 icon prop 供断言
const IonIconStub = {
  name: "IonIcon",
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" :data-icon="String(icon)" />',
};

function mountPhaseIcon(options: { phase: any; size?: number }) {
  return mount(PhaseIcon, {
    props: { phase: options.phase, ...(options.size !== undefined ? { size: options.size } : {}) },
    global: {
      stubs: { "ion-icon": IonIconStub },
    },
  });
}

// Phase / Status → 期望的 ionicon 图标常量
const EXPECTED_ICON_MAP: Record<string, string> = {
  [Phase.Created]: cloudUploadOutline,
  [Phase.Analyzing]: searchOutline,
  [Phase.Initializing]: playOutline,
  [Phase.Preprocessing]: codeSlashOutline,
  [Phase.Encrypting]: lockClosedOutline,
  [Phase.Decrypting]: lockOpenOutline,
  [Phase.Packing]: cubeOutline,
  [Phase.Verifying]: shieldCheckmarkOutline,
  [Phase.Completed]: checkmarkCircleOutline,
  failed: closeCircleOutline,
  cancelled: banOutline,
};

describe("PhaseIcon - 9 个 Phase 值渲染对应 ion-icon", () => {
  it.each(ALL_PHASES.map(p => [p, p] as const))("Phase.%s 渲染对应的 ion-icon", phase => {
    const wrapper = mountPhaseIcon({ phase });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.exists()).toBe(true);
    expect(iconEl.attributes("data-icon")).toBe(String(EXPECTED_ICON_MAP[phase]));
  });
});

describe("PhaseIcon - 2 个终态 status 值渲染对应 ion-icon", () => {
  it("failed 渲染 closeCircleOutline", () => {
    const wrapper = mountPhaseIcon({ phase: "failed" });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.attributes("data-icon")).toBe(String(closeCircleOutline));
  });

  it("cancelled 渲染 banOutline", () => {
    const wrapper = mountPhaseIcon({ phase: "cancelled" });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.attributes("data-icon")).toBe(String(banOutline));
  });
});

describe("PhaseIcon - class 包含 phase 标识", () => {
  it.each(ALL_PHASES.map(p => [p, p] as const))("Phase.%s 渲染的 ion-icon 包含 phase-icon--%s class", phase => {
    const wrapper = mountPhaseIcon({ phase });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.classes()).toContain("phase-icon");
    expect(iconEl.classes()).toContain(`phase-icon--${phase}`);
  });

  it("failed 渲染的 ion-icon 包含 phase-icon--failed class", () => {
    const wrapper = mountPhaseIcon({ phase: "failed" });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.classes()).toContain("phase-icon--failed");
  });

  it("cancelled 渲染的 ion-icon 包含 phase-icon--cancelled class", () => {
    const wrapper = mountPhaseIcon({ phase: "cancelled" });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.classes()).toContain("phase-icon--cancelled");
  });
});

describe("PhaseIcon - fallback 处理", () => {
  it("未知 phase 值（强转）使用 helpCircleOutline 作为 fallback", () => {
    // 模拟后端推送了未识别的 phase 字符串（强转为 Phase 类型）
    const wrapper = mountPhaseIcon({ phase: "unknown_phase" as never });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.attributes("data-icon")).toBe(String(helpCircleOutline));
  });

  it("空字符串 phase 使用 fallback", () => {
    const wrapper = mountPhaseIcon({ phase: "" as never });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.attributes("data-icon")).toBe(String(helpCircleOutline));
  });
});

describe("PhaseIcon - 基础渲染", () => {
  it("渲染单个 ion-icon 元素", () => {
    const wrapper = mountPhaseIcon({ phase: Phase.Created });
    expect(wrapper.findAll(".ion-icon-stub")).toHaveLength(1);
  });

  it("phase prop 变化时 icon 跟随更新", async () => {
    const wrapper = mountPhaseIcon({ phase: Phase.Created });
    expect(wrapper.find(".ion-icon-stub").attributes("data-icon")).toBe(String(cloudUploadOutline));
    await wrapper.setProps({ phase: Phase.Encrypting });
    expect(wrapper.find(".ion-icon-stub").attributes("data-icon")).toBe(String(lockClosedOutline));
  });
});

describe("PhaseIcon - size props", () => {
  it("未传 size 时继承父级 font-size（无 inline style）", () => {
    const wrapper = mountPhaseIcon({ phase: Phase.Created });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.attributes("style") ?? "").not.toContain("font-size");
  });

  it("传 size=20 时设置 font-size: 20px", () => {
    const wrapper = mountPhaseIcon({ phase: Phase.Created, size: 20 });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.attributes("style") ?? "").toContain("font-size: 20px");
  });

  it("传 size=16 时设置 font-size: 16px", () => {
    const wrapper = mountPhaseIcon({ phase: Phase.Created, size: 16 });
    const iconEl = wrapper.find(".ion-icon-stub");
    expect(iconEl.attributes("style") ?? "").toContain("font-size: 16px");
  });
});
