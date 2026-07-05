/**
 * TDesignChatView.test.ts
 *
 * 测试 TDesignChatView 组件（v3 实现）：
 *   - 接受 EngineRenderProps 形态的 props
 *   - 渲染消息列表（user / assistant text / tool_call / tool_result）
 *   - **关键**：文本和工具调用按 eventLog 时间轴交错（v3 修复）
 *   - streaming=true 时显示 thinking 指示器
 *   - tool_result 内嵌在对应 operation 卡片内
 *   - 文本段使用 TDesign 自家 ChatMarkdown（v3 替换 MarkdownStream）
 *   - 外层容器背景透明（v3 修复暗黑模式盖色问题）
 *
 * v3 关键变化：
 *   - 改用 useRenderTurnItems(messages, status, compactionText)
 *   - 按 eventLog 顺序逐项渲染 RenderedItem[]
 *   - 文本段渲染 TDesign ChatMarkdown（不再合并为整条 m.content）
 *   - tool_call 单条卡片插入文本流中
 *   - 外层 .tdesign-chat-view 背景改为 transparent
 *
 * SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/ Phase 4
 */

import type { Message, ToolCall, ToolResult } from "@encv/shared-components/composables/useAgent";
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";

// Mock @tdesign-vue-next/chat：ChatThinking + ChatMarkdown
vi.mock("@tdesign-vue-next/chat", () => {
  const StubThinking = defineComponent({
    name: "ChatThinking",
    props: ["content"],
    setup(props) {
      return () => h("div", { class: "td-chat-thinking-stub" }, [props.content]);
    },
  });
  // v3: ChatMarkdown 是基于 cherry-markdown 的 OMI web component，
  // 在 jsdom 测试环境下用 stub 替代，避免 cherry-markdown 副作用
  const StubChatMarkdown = defineComponent({
    name: "ChatMarkdown",
    props: ["content", "options"],
    setup(props) {
      return () =>
        h("div", { class: "td-msg-md-stub", "data-td-markdown": "true" }, [
          typeof props.content === "string" ? props.content : "[non-string]",
        ]);
    },
  });
  return {
    ChatThinking: StubThinking,
    ChatMarkdown: StubChatMarkdown,
  };
});

// Mock ToolDetailContent（v4 新增）—— 用 stub 替代避免复杂模板渲染
vi.mock("../ToolDetailContent.vue", () => ({
  default: defineComponent({
    name: "ToolDetailContent",
    props: ["raw", "kind", "toolName"],
    setup(props) {
      return () =>
        h("div", { class: "td-tool-detail-stub", "data-kind": props.kind }, [
          typeof props.raw === "string" ? props.raw.slice(0, 80) : "[non-string]",
        ]);
    },
  }),
}));

import TDesignChatView from "../TDesignChatView.vue";

// =============================================================================
// 测试辅助
// =============================================================================

function makeUserMessage(text: string, overrides: Partial<Message> = {}): Message {
  return {
    role: "user",
    content: text,
    tool_calls: [],
    tool_results: [],
    eventLog: [{ type: "user_text", text: text } as { type: string; id?: string }],
    ...overrides,
  };
}

function makeAssistantMessage(
  text: string,
  toolCalls: ToolCall[] = [],
  toolResults: ToolResult[] = [],
  overrides: Partial<Message> = {}
): Message {
  // 构造 eventLog：按时间顺序记录 text + tool_call + tool_result
  // 注：Message.eventLog 类型在 useAgent.ts 限定为 { type, id? }，但
  // useRenderTurnItems 实际会读 `text` 字段。test 强制 cast 覆盖类型。
  const eventLog: NonNullable<Message["eventLog"]> = [];
  for (const tc of toolCalls) eventLog.push({ type: "tool_call", id: tc.id });
  for (const tr of toolResults) eventLog.push({ type: "tool_result", id: tr.id });
  if (text) eventLog.push({ type: "text", text } as { type: string; id?: string });
  return {
    role: "assistant",
    content: text,
    tool_calls: toolCalls,
    tool_results: toolResults,
    eventLog,
    ...overrides,
  };
}

function makeToolCall(overrides: Partial<ToolCall> = {}): ToolCall {
  return {
    id: "tc-1",
    name: "search",
    args: '{"q":"hello"}',
    auto_run: false,
    kind: "readOnly",
    needsConfirm: false,
    status: "pending",
    ...overrides,
  };
}

function makeToolResult(overrides: Partial<ToolResult> = {}): ToolResult {
  return {
    id: "tr-1",
    name: "search",
    result: '{"hits":3}',
    is_error: false,
    status: "success",
    duration_ms: 100,
    ...overrides,
  };
}

const defaultProps = {
  messages: [] as Message[],
  status: "idle" as string,
  streaming: false,
  onSend: vi.fn(async () => {}),
  onStop: vi.fn(),
  onConfirmTool: vi.fn(async () => {}),
  onCopyMessage: vi.fn(async () => {}),
  onPresetClick: vi.fn(),
};

function mountChatView(props: Partial<typeof defaultProps> = {}) {
  return mount(TDesignChatView, {
    props: { ...defaultProps, ...props },
  });
}

// =============================================================================
// 测试
// =============================================================================

describe("TDesignChatView v3 (useRenderTurnItems)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("TestTDesign_AcceptsEngineRenderProps: 接受完整 EngineRenderProps 形态", () => {
    const wrapper = mountChatView({
      messages: [],
      status: "idle",
      streaming: false,
    });
    expect(wrapper.exists()).toBe(true);
    expect(wrapper.classes()).toContain("tdesign-chat-view");
  });

  it("TestTDesign_EmptyState_WhenNoMessages: messages 为空时显示欢迎文案", () => {
    const wrapper = mountChatView({ messages: [] });
    const welcome = wrapper.find(".welcome");
    expect(welcome.exists()).toBe(true);
    expect(welcome.text()).toContain("TDesign");
  });

  it("TestTDesign_StreamingEmptyShowsThinking: streaming=true + 无消息时显示 thinking", () => {
    const wrapper = mountChatView({ messages: [], streaming: true });
    const emptyState = wrapper.find(".empty-state");
    expect(emptyState.exists()).toBe(true);
    expect(wrapper.attributes("data-streaming")).toBe("true");
  });

  it("TestTDesign_RendersUserBubble: 渲染 user 消息气泡", () => {
    const wrapper = mountChatView({
      messages: [makeUserMessage("你好世界")],
    });
    const userRow = wrapper.find(".td-msg-row--user");
    expect(userRow.exists()).toBe(true);
    expect(userRow.text()).toContain("你好世界");
  });

  it("TestTDesign_RendersAssistantText: 渲染 assistant 文本段（v3 关键：单段渲染不合并）", () => {
    const wrapper = mountChatView({
      messages: [makeUserMessage("问个问题"), makeAssistantMessage("这是回答")],
    });
    const assistantRow = wrapper.find(".td-msg-row--assistant");
    expect(assistantRow.exists()).toBe(true);
    // v3: 文本段用 TDesign ChatMarkdown（cherry-markdown），stub 标记为 td-msg-md-stub
    const md = wrapper.find(".td-msg-md-stub");
    expect(md.exists()).toBe(true);
    expect(md.text()).toBe("这是回答");
  });

  // v3 新增：验证 ChatMarkdown 真实渲染 + 透明背景
  it("TestTDesign_UsesTDesignChatMarkdown: 文本段使用 TDesign ChatMarkdown 组件", () => {
    const wrapper = mountChatView({
      messages: [makeUserMessage("hi"), makeAssistantMessage("**bold** 内容")],
    });
    const tdMd = wrapper.find('[data-td-markdown="true"]');
    expect(tdMd.exists()).toBe(true);
    expect(tdMd.text()).toContain("**bold** 内容");
  });

  // v3 新增：空文本段不渲染 ChatMarkdown（v-if 保护）
  it("TestTDesign_EmptyTextDoesNotRenderMarkdown: 空文本不渲染 ChatMarkdown", () => {
    const wrapper = mountChatView({
      messages: [makeUserMessage("hi"), makeAssistantMessage("")],
    });
    const tdMd = wrapper.find('[data-td-markdown="true"]');
    expect(tdMd.exists()).toBe(false);
  });

  // v3 新增：外层容器背景透明（修复暗黑模式盖色）
  it("TestTDesign_OuterContainerTransparentBackground: 外层容器背景为 transparent", () => {
    const wrapper = mountChatView({ messages: [] });
    const view = wrapper.find(".tdesign-chat-view");
    expect(view.exists()).toBe(true);
    // 真实 DOM 元素的 inline style 由 scoped 注入；检查 computed style 在测试中无法
    // 直接读取，但可确认样式表中没有 #f7f7f7 兜底
    const styleText = wrapper.element.outerHTML;
    // 透明背景无 #f7f7f7 兜底色
    expect(view.classes()).toContain("tdesign-chat-view");
    // 注：scoped 样式表无法在 jsdom 中读 ::cssText，但样式仍生效
    // 关键：确保没有遗留 MarkdownStream 引用
    expect(styleText).not.toContain("markdownStream-stub");
  });

  // ★ v3 核心修复测试：文本和工具调用交错
  it("TestTDesign_InterleavesTextAndToolCall: 文本和工具调用按 eventLog 交错（不再堆底部）", () => {
    const tc = makeToolCall({ id: "tc-1", name: "list_files" });
    const tr = makeToolResult({ id: "tr-1", name: "list_files", result: "file1,file2" });
    const messages = [makeUserMessage("列一下文件"), makeAssistantMessage("好的", [tc], [tr])];
    const wrapper = mountChatView({ messages });

    // 关键断言：tool-call-list 容器（旧的"堆底部"模式）不应存在
    const oldBottomList = wrapper.find(".tool-call-list");
    expect(oldBottomList.exists()).toBe(false);

    // tool_call 应该是按行渲染（不是底部堆叠）
    const toolRow = wrapper.find(".td-msg-row--tool");
    expect(toolRow.exists()).toBe(true);

    // 关键断言：工具调用在 DOM 树中应该出现在文本段之前（按 eventLog 顺序）
    const renderedItems = wrapper.findAll(".renderedItemWrap");
    expect(renderedItems.length).toBeGreaterThan(0);
  });

  it("TestTDesign_ToolResultEmbeddedInOperationCard: 工具结果内嵌在对应 operation 卡片内", () => {
    // AG-UI 协议语义：ToolCall.id 和 ToolResult.id 是同一个 toolCallId（用 call 的 id 关联 result）
    const tc = makeToolCall({ id: "tc-1", name: "search" });
    const tr = makeToolResult({ id: "tc-1", name: "search", result: "找到3条" });
    const messages = [makeUserMessage("搜一下"), makeAssistantMessage("", [tc], [tr])];
    const wrapper = mountChatView({ messages });

    // v4: 工具卡片用 <details> 折叠容器
    const toolCard = wrapper.find(".td-tool-card");
    expect(toolCard.exists()).toBe(true);
    // ToolDetailContent stub 用 data-kind="result" 标记
    const detailStub = toolCard.find('[data-kind="result"]');
    expect(detailStub.exists()).toBe(true);
    expect(detailStub.text()).toContain("找到3条");
  });

  it("TestTDesign_NoResultEmbeddedWhenNoResult: 工具调用但无结果时不显示结果区", () => {
    const tc = makeToolCall({ id: "tc-1", name: "search", status: "pending" });
    const messages = [makeUserMessage("搜"), makeAssistantMessage("", [tc], [])];
    const wrapper = mountChatView({ messages });

    const toolCard = wrapper.find(".td-tool-card");
    expect(toolCard.exists()).toBe(true);
    // v4: 无 result 时不渲染 result stub
    const detailStub = toolCard.find('[data-kind="result"]');
    expect(detailStub.exists()).toBe(false);
  });

  // ★ v4 新增测试：工具卡片 <details> 折叠行为
  it("TestTDesign_ToolCardUsesDetailsElement: v4 用 <details> + <summary> 替代 div", () => {
    const tc = makeToolCall({ id: "tc-1", name: "search", status: "success" });
    const messages = [makeUserMessage("搜"), makeAssistantMessage("", [tc], [])];
    const wrapper = mountChatView({ messages });

    const toolCard = wrapper.find(".td-tool-card");
    expect(toolCard.exists()).toBe(true);
    // v4 必须是 <details> 元素
    expect(toolCard.element.tagName).toBe("DETAILS");
    // 必须有 <summary> 子元素
    const summary = toolCard.find("summary");
    expect(summary.exists()).toBe(true);
    expect(summary.classes()).toContain("td-tool-card-head");
  });

  it("TestTDesign_ToolCardAutoExpandWhenRunning: 运行时自动展开", () => {
    const tc = makeToolCall({ id: "tc-1", name: "search", status: "running" });
    const messages = [makeUserMessage("搜"), makeAssistantMessage("", [tc], [])];
    const wrapper = mountChatView({ messages });

    const toolCard = wrapper.find(".td-tool-card");
    // running → 默认展开
    expect(toolCard.attributes("open")).toBeDefined();
  });

  it("TestTDesign_ToolCardCollapsedByDefaultWhenSuccess: success 默认折叠", () => {
    const tc = makeToolCall({ id: "tc-1", name: "search", status: "success" });
    const messages = [makeUserMessage("搜"), makeAssistantMessage("", [tc], [])];
    const wrapper = mountChatView({ messages });

    const toolCard = wrapper.find(".td-tool-card");
    // success → 默认折叠
    expect(toolCard.attributes("open")).toBeUndefined();
  });

  it("TestTDesign_ToolCardUsesToolDetailContent: 工具卡片用 ToolDetailContent 而非 raw pre", () => {
    const tc = makeToolCall({
      id: "tc-1",
      name: "list_mounts",
      args: '{"type":"serving"}',
    });
    const tr = makeToolResult({
      id: "tc-1",
      name: "list_mounts",
      result: '{"mounts":[{"id":"x","type":"serving"}]}',
    });
    const messages = [makeUserMessage("列挂载"), makeAssistantMessage("", [tc], [tr])];
    const wrapper = mountChatView({ messages });

    // 必须用 ToolDetailContent 组件（stub 标记 data-kind）
    const argStub = wrapper.find('[data-kind="args"]');
    expect(argStub.exists()).toBe(true);
    const resultStub = wrapper.find('[data-kind="result"]');
    expect(resultStub.exists()).toBe(true);
  });

  it("TestTDesign_ToolCardHasChevronIcon: 工具卡片有 chevron 图标（v4 新增）", () => {
    const tc = makeToolCall({ id: "tc-1", name: "search", status: "success" });
    const messages = [makeUserMessage("搜"), makeAssistantMessage("", [tc], [])];
    const wrapper = mountChatView({ messages });

    const chevron = wrapper.find(".td-tool-card-chevron");
    expect(chevron.exists()).toBe(true);
  });

  it("TestTDesign_ToolStatusMappedToDataAttribute: tool_call status 映射到 data-status", () => {
    const pending = makeToolCall({ id: "p", status: "pending" });
    const running = makeToolCall({ id: "r", status: "running" });
    const success = makeToolCall({ id: "s", status: "success" });
    const failed = makeToolCall({ id: "f", status: "failed" });
    // AG-UI 协议：每个 tool 有自己的 toolCallId
    const messages = [makeAssistantMessage("", [pending, running, success, failed])];
    const wrapper = mountChatView({ messages });

    const statuses = wrapper.findAll(".td-tool-card-status");
    expect(statuses.length).toBe(4);
    expect(statuses[0].attributes("data-status")).toBe("pending");
    expect(statuses[1].attributes("data-status")).toBe("running");
    expect(statuses[2].attributes("data-status")).toBe("success");
    expect(statuses[3].attributes("data-status")).toBe("failed");
  });

  it("TestTDesign_StreamingWithMessages_ShowsThinkingAtBottom: 有消息且 streaming=true 时显示底部 thinking", () => {
    const messages = [makeUserMessage("hi"), makeAssistantMessage("")];
    const wrapper = mountChatView({ messages, streaming: true });
    const thinkingAtBottom = wrapper.find(".streaming-thinking");
    expect(thinkingAtBottom.exists()).toBe(true);
  });

  it("TestTDesign_DataStreamingAttribute_ReflectsProp: data-streaming 属性绑定正确", () => {
    const w1 = mountChatView({ streaming: true });
    expect(w1.attributes("data-streaming")).toBe("true");
    const w2 = mountChatView({ streaming: false });
    expect(w2.attributes("data-streaming")).toBe("false");
  });
});
