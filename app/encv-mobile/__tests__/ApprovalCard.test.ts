/**
 * ApprovalCard 单元测试
 *
 * 覆盖 F 阶段需求：
 * F.1 - 4 决策按钮文案（i18n）
 * F.2 - 按钮处理中态（spinner + 禁用其他按钮）
 *
 * 覆盖矩阵：
 * 1. 渲染：4 个按钮 + 正确 i18n 文案
 * 2. readOnly 时隐藏 "本轮批准" 按钮
 * 3. 点击按钮触发 onDecide(toolCallId, decision)
 * 4. 点击后：当前按钮显示 spinner，4 个按钮全部 disabled
 * 5. isProcessing 变化：true→false 时按钮恢复
 * 6. 5s 兜底 timer：网络挂起时强制清空 processingDecision
 */

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import ApprovalCard from "@/components/agent/ApprovalCard.vue";
import type { ToolCall } from "@/composables/useAgent";

// ─── 工具函数：构造 mock ToolCall ─────────────────────────────────

function makeToolCall(overrides: Partial<ToolCall> = {}): ToolCall {
  return {
    id: "tc-1",
    name: "exec_command",
    args: JSON.stringify({ command: "ls -la", reason: "list files" }),
    auto_run: false,
    kind: "command",
    needsConfirm: true,
    status: "pending",
    ...overrides,
  };
}

function findButtonByText(wrapper: ReturnType<typeof mount>, text: string) {
  const buttons = wrapper.findAll(".approvalBtn");
  return buttons.find(b => b.text().includes(text));
}

const STUB_ICONS = {
  IonIcon: { template: '<span class="ion-icon-stub" />' },
};

// ─── 测试 ──────────────────────────────────────────────────────────

describe("ApprovalCard - F.1 4 决策按钮文案", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("渲染 4 个决策按钮（命令类型）", () => {
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall({ kind: "command" }),
        onDecide: vi.fn(),
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });
    const buttons = wrapper.findAll(".approvalBtn");
    expect(buttons.length).toBe(4);
    // 按顺序：accept / accept_for_session / decline / cancel
    expect(buttons[0].text()).toContain("批准"); // modals.approve
    expect(buttons[1].text()).toContain("本轮批准"); // modals.approveForSession
    expect(buttons[2].text()).toContain("拒绝"); // modals.decline
    expect(buttons[3].text()).toContain("拒绝并停止"); // modals.cancel
  });

  it('kind=readOnly 时隐藏"本轮批准"按钮（spec F.1 要求）', () => {
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall({ kind: "readOnly" }),
        onDecide: vi.fn(),
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });
    const buttons = wrapper.findAll(".approvalBtn");
    // readOnly 不展示本轮批准 → 3 个按钮
    expect(buttons.length).toBe(3);
    // 顺序：accept / decline / cancel
    expect(buttons[0].text()).toContain("批准");
    expect(buttons[1].text()).toContain("拒绝");
    expect(buttons[2].text()).toContain("拒绝并停止");
  });

  it("kind=fileChange 时仍展示 4 个按钮", () => {
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall({ kind: "fileChange" }),
        onDecide: vi.fn(),
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });
    const buttons = wrapper.findAll(".approvalBtn");
    expect(buttons.length).toBe(4);
  });

  it("kind=webSearch 时仍展示 4 个按钮", () => {
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall({ kind: "webSearch" }),
        onDecide: vi.fn(),
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });
    const buttons = wrapper.findAll(".approvalBtn");
    expect(buttons.length).toBe(4);
  });
});

describe("ApprovalCard - F.2 按钮处理中态", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('点击"批准"按钮 → 触发 onDecide(id, "accept")', async () => {
    const onDecide = vi.fn();
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall({ id: "tc-42" }),
        onDecide,
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    const acceptBtn = findButtonByText(wrapper, "批准")!;
    expect(acceptBtn.exists()).toBe(true);
    await acceptBtn.trigger("click");

    expect(onDecide).toHaveBeenCalledTimes(1);
    expect(onDecide).toHaveBeenCalledWith("tc-42", "accept");
  });

  it('点击"本轮批准" → 触发 onDecide(id, "accept_for_session")', async () => {
    const onDecide = vi.fn();
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide,
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    const sessionBtn = findButtonByText(wrapper, "本轮批准")!;
    await sessionBtn.trigger("click");

    expect(onDecide).toHaveBeenCalledWith("tc-1", "accept_for_session");
  });

  it('点击"拒绝" → 触发 onDecide(id, "decline")', async () => {
    const onDecide = vi.fn();
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide,
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    const declineBtn = findButtonByText(wrapper, "拒绝")!;
    await declineBtn.trigger("click");

    expect(onDecide).toHaveBeenCalledWith("tc-1", "decline");
  });

  it('点击"拒绝并停止" → 触发 onDecide(id, "cancel")', async () => {
    const onDecide = vi.fn();
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide,
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    // 拒绝并停止 是以 "拒绝并停止" 完整匹配
    const cancelBtn = wrapper.findAll(".approvalBtn").find(b => b.text().trim() === "拒绝并停止")!;
    await cancelBtn.trigger("click");

    expect(onDecide).toHaveBeenCalledWith("tc-1", "cancel");
  });

  it("点击后：当前按钮带 approvalBtn_processing class + spinner", async () => {
    const onDecide = vi.fn();
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide,
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    const acceptBtn = findButtonByText(wrapper, "批准")!;
    await acceptBtn.trigger("click");
    await nextTick();

    // 处理的按钮带 processing class
    expect(acceptBtn.classes()).toContain("approvalBtn_processing");
    // spinner 渲染
    expect(acceptBtn.find(".approvalBtnSpinner").exists()).toBe(true);
  });

  it("点击后：所有 4 个按钮全部 disabled（禁用其他 3 个）", async () => {
    const onDecide = vi.fn();
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide,
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    const acceptBtn = findButtonByText(wrapper, "批准")!;
    await acceptBtn.trigger("click");
    await nextTick();

    const buttons = wrapper.findAll(".approvalBtn");
    buttons.forEach(btn => {
      // native disabled attr 应为 true
      const disabled = (btn.element as HTMLButtonElement).disabled;
      expect(disabled).toBe(true);
    });
  });

  it("isProcessing=true 时所有按钮预先 disabled", () => {
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide: vi.fn(),
        isProcessing: true,
      },
      global: { stubs: STUB_ICONS },
    });

    const buttons = wrapper.findAll(".approvalBtn");
    buttons.forEach(btn => {
      const disabled = (btn.element as HTMLButtonElement).disabled;
      expect(disabled).toBe(true);
    });
  });

  it("isProcessing 从 true 变 false 时立即清空 processingDecision（spec F.2: 等 SSE 流返回后按钮恢复）", async () => {
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide: vi.fn(),
        isProcessing: true,
      },
      global: { stubs: STUB_ICONS },
    });

    // 初始：isProcessing=true → 按钮 disabled
    const acceptBtn = findButtonByText(wrapper, "批准")!;
    expect((acceptBtn.element as HTMLButtonElement).disabled).toBe(true);

    // 父组件通知流结束 → isProcessing=false
    await wrapper.setProps({ isProcessing: false });
    await nextTick();

    // 按钮立即恢复
    const buttons = wrapper.findAll(".approvalBtn");
    buttons.forEach(btn => {
      expect((btn.element as HTMLButtonElement).disabled).toBe(false);
    });
  });

  it("isProcessing 保持 false 但用户点击过 → 模拟 SSE 结束前 isProcessing 仍为 true，按钮 disabled", async () => {
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide: vi.fn(),
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    // 模拟点击 accept → processingDecision=accept，但 isProcessing 仍 false（异常路径）
    const acceptBtn = findButtonByText(wrapper, "批准")!;
    await acceptBtn.trigger("click");
    await nextTick();
    expect((acceptBtn.element as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("ApprovalCard - 5s 兜底 timer", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("点击按钮后 5s 内未恢复 → 强制清空 processingDecision", async () => {
    const onDecide = vi.fn();
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide,
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    const acceptBtn = findButtonByText(wrapper, "批准")!;
    await acceptBtn.trigger("click");
    await nextTick();

    // 5s 内：按钮 disabled
    expect((acceptBtn.element as HTMLButtonElement).disabled).toBe(true);

    // 时间快进 5s
    vi.advanceTimersByTime(5000);
    await nextTick();

    // 5s 后：processingDecision 被清空 → 按钮恢复
    const buttons = wrapper.findAll(".approvalBtn");
    buttons.forEach(btn => {
      expect((btn.element as HTMLButtonElement).disabled).toBe(false);
    });
  });

  it("5s 兜底前再次点击同按钮 → 不会重复触发 onDecide（已 disabled）", async () => {
    const onDecide = vi.fn();
    const wrapper = mount(ApprovalCard, {
      props: {
        toolCall: makeToolCall(),
        onDecide,
        isProcessing: false,
      },
      global: { stubs: STUB_ICONS },
    });

    const acceptBtn = findButtonByText(wrapper, "批准")!;
    await acceptBtn.trigger("click");
    await nextTick();
    expect(onDecide).toHaveBeenCalledTimes(1);

    // 第二次点击：disabled 应为 true，handler 提前 return
    await acceptBtn.trigger("click");
    expect(onDecide).toHaveBeenCalledTimes(1);
  });
});
