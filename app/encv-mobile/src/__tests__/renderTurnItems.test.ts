/**
 * renderTurnItems 单元测试
 *
 * 覆盖：
 * - 单条 user → user item
 * - 单条 assistant → assistantText item
 * - reasoning → reasoning item
 * - 累积 command → operationGroup
 * - 累积混合（command + fileChange）→ operationGroup
 * - 累积 webSearch → webSearchGroup
 * - needsConfirm + pending → approval item
 * - 流式状态保留未完结 group
 * - 非流式状态强制 flush
 */
import { describe, expect, it } from "vitest";
import { renderTurnItems } from "@/composables/renderTurnItems";
import type { Message } from "@/composables/useAgent";

const u = (_i: number, content: string): Message => ({
  role: "user",
  content,
  tool_calls: [],
  tool_results: [],
});

const a = (_i: number, content: string, extra: Partial<Message> = {}): Message => ({
  role: "assistant",
  content,
  tool_calls: [],
  tool_results: [],
  isStreaming: false,
  ...extra,
});

const tc = (
  id: string,
  kind: Message["tool_calls"][number]["kind"],
  status: "pending" | "running" | "success" | "failed" = "success",
  needsConfirm = false
): Message["tool_calls"][number] => ({
  id,
  name: `tool_${id}`,
  args:
    kind === "webSearch"
      ? JSON.stringify({ query: `q-${id}` })
      : kind === "command"
        ? JSON.stringify({ command: "ls" })
        : JSON.stringify({ path: `/tmp/${id}` }),
  auto_run: !needsConfirm,
  kind,
  needsConfirm,
  status,
});

describe("renderTurnItems", () => {
  it("renders single user message", () => {
    const out = renderTurnItems([u(0, "hello")], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("user");
    if (out[0].type === "user") {
      expect(out[0].text).toBe("hello");
    }
  });

  it("renders single assistant message with streaming flag", () => {
    const out = renderTurnItems([a(0, "world", { isStreaming: true })], "streaming");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("assistantText");
    if (out[0].type === "assistantText") {
      expect(out[0].text).toBe("world");
      expect(out[0].streaming).toBe(true);
    }
  });

  it("renders reasoning as a separate item", () => {
    const out = renderTurnItems([a(0, "", { reasoning: "hmm" })], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("reasoning");
  });

  it("accumulates consecutive commands into operationGroup", () => {
    const msg = a(0, "", { tool_calls: [tc("c1", "command"), tc("c2", "command"), tc("c3", "command")] });
    const out = renderTurnItems([msg], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("operationGroup");
    if (out[0].type === "operationGroup") {
      expect(out[0].toolCallIds).toEqual(["c1", "c2", "c3"]);
      expect(out[0].forceComplete).toBe(true);
    }
  });

  it("accumulates mixed command+fileChange into operationGroup", () => {
    const msg = a(0, "", { tool_calls: [tc("c1", "command"), tc("f1", "fileChange")] });
    const out = renderTurnItems([msg], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("operationGroup");
    if (out[0].type === "operationGroup") {
      expect(out[0].toolCallIds).toEqual(["c1", "f1"]);
    }
  });

  it("accumulates webSearch into webSearchGroup", () => {
    const msg = a(0, "", { tool_calls: [tc("w1", "webSearch"), tc("w2", "webSearch")] });
    const out = renderTurnItems([msg], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("webSearchGroup");
    if (out[0].type === "webSearchGroup") {
      expect(out[0].queries).toEqual(["q-w1", "q-w2"]);
      expect(out[0].toolCallIds).toEqual(["w1", "w2"]);
    }
  });

  it("renders pending+needsConfirm tool call as approval item", () => {
    const msg = a(0, "", { tool_calls: [tc("a1", "command", "pending", true)] });
    const out = renderTurnItems([msg], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("approval");
  });

  it("keeps group open during streaming with lastStatus=running", () => {
    const msg = a(0, "", {
      tool_calls: [tc("c1", "command", "running"), tc("c2", "command", "running")],
      isStreaming: true,
    });
    const out = renderTurnItems([msg], "streaming");
    // forceComplete=false → 流式时 group 不被 flush（保持为可继续累积的"活跃"状态）
    // 但本实现中流式时 lastStatus=running → 跳过 flush，所以 out 应为空
    expect(out).toHaveLength(0);
  });

  it("flushes group when non-streaming", () => {
    const msg = a(0, "", {
      tool_calls: [tc("c1", "command", "success")],
    });
    const out = renderTurnItems([msg], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("operationGroup");
  });

  it("handles user+assistant+tools sequence", () => {
    const messages: Message[] = [
      u(0, "hi"),
      a(1, "thinking...", { tool_calls: [tc("c1", "command", "success")] }),
      u(2, "again"),
      a(3, "done", { tool_calls: [tc("a1", "command", "pending", true)] }),
    ];
    const out = renderTurnItems(messages, "idle");
    // 当前实现会同时输出 assistantText + operationGroup
    // （让用户既看到 AI 思考文本，又看到工具调用结果）
    // u-0 user → a-1 assistantText + c-1 group → u-2 user → a-3 assistantText + a-3 approval
    expect(out.map(o => o.type)).toEqual(["user", "assistantText", "operationGroup", "user", "assistantText", "approval"]);
  });

  it("handles tool_result.is_error as error item", () => {
    const msg: Message = a(0, "", {
      tool_results: [
        {
          id: "r1",
          name: "cmd",
          result: "command failed",
          is_error: true,
          status: "failed",
          duration_ms: 100,
        },
      ],
    });
    const out = renderTurnItems([msg], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("error");
  });
});

// =============================================================================
// stripLeadingToolCallJson 测试（LLM 工具调用 JSON 前缀清理）
// =============================================================================
describe("stripLeadingToolCallJson", () => {
  it("exports the function", async () => {
    const { stripLeadingToolCallJson } = await import("@/composables/renderTurnItems");
    expect(typeof stripLeadingToolCallJson).toBe("function");
  });

  // 需要动态 import 因为 vitest esbuild 模式下顶层 await 不稳定
  async function getStripFn() {
    const { stripLeadingToolCallJson } = await import("@/composables/renderTurnItems");
    return stripLeadingToolCallJson;
  }

  it("strips file_search tool call JSON prefix", async () => {
    const fn = await getStripFn();
    const input = '{ "queries":[""], "source_filter": ["file_library"], "intent": "nav" }当前工作区共有以下文件：';
    expect(fn(input)).toBe("当前工作区共有以下文件：");
  });

  it("preserves normal markdown content without JSON prefix", async () => {
    const fn = await getStripFn();
    const input = "# Hello\n\n这是正常的回复内容。";
    expect(fn(input)).toBe("# Hello\n\n这是正常的回复内容。");
  });

  it("returns original when JSON has no known tool keys", async () => {
    const fn = await getStripFn();
    const input = '{ "foo": "bar", "baz": [1,2] }some text';
    // JSON with non-tool keys should not be stripped
    expect(fn(input)).toBe('{ "foo": "bar", "baz": [1,2] }some text');
  });

  it("returns original when JSON is not at start", async () => {
    const fn = await getStripFn();
    const input = '查看文件列表：\n{ "queries": [""] }';
    expect(fn(input)).toBe('查看文件列表：\n{ "queries": [""] }');
  });

  it("returns original when JSON is the entire content (no trailing text)", async () => {
    const fn = await getStripFn();
    const input = '{ "queries": [""], "intent": "nav" }';
    expect(fn(input)).toBe('{ "queries": [""], "intent": "nav" }');
  });

  it("handles nested braces correctly", async () => {
    const fn = await getStripFn();
    const input = '{ "queries": [{"path": "/tmp"}], "source_filter": ["file_library"], "intent": "nav" }结果如下：';
    expect(fn(input)).toBe("结果如下：");
  });

  it("handles empty string", async () => {
    const fn = await getStripFn();
    expect(fn("")).toBe("");
  });

  it("handles non-object start (plain text)", async () => {
    const fn = await getStripFn();
    expect(fn("Hello world")).toBe("Hello world");
  });

  it("integration: assistant message with tool JSON gets cleaned via renderTurnItems", () => {
    const msg: Message = {
      role: "assistant",
      content: '{ "queries":[""], "source_filter":["file_library"] }\n# 文件列表\n\n1. test.py',
      tool_calls: [],
      tool_results: [],
    };
    const out = renderTurnItems([msg], "idle");
    expect(out).toHaveLength(1);
    expect(out[0].type).toBe("assistantText");
    // JSON prefix should be stripped
    expect((out[0] as { text: string }).text).not.toContain('"queries"');
    expect((out[0] as { text: string }).text).toContain("# 文件列表");
  });

  it("integration: user message with tool JSON is NOT cleaned", () => {
    const msg: Message = {
      role: "user",
      content: '{ "queries":["test"] }请帮我找文件',
      tool_calls: [],
      tool_results: [],
    };
    const out = renderTurnItems([msg], "idle");
    expect(out).toHaveLength(1);
    // user messages are NOT stripped
    expect((out[0] as { text: string }).text).toContain('"queries"');
  });
});
