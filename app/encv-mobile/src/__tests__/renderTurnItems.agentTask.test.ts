/**
 * renderTurnItems.agentTask.test.ts - Task 22 单元测试
 *
 * 覆盖：
 * 1. role='agent_task' + 合法 content 产出 type='agentTask' 的
 *    RenderedItem
 * 2. subTasks 数组正确解析（id/status/description）
 * 3. reasoning 字段可选，缺省时不写入 RenderedItem
 * 4. content 解析失败（非法 JSON）时退化为空 subTasks，**不崩溃**
 * 5. AGENT_TASK_COLLAPSE_LINE_COUNT / _CHAR_COUNT 常量值与 spec
 *    对齐（7 / 520，参照 codex-web MessageBlocks.tsx:68-69）
 * 6. agent_task 之前的 operationGroup 在到达时被正确 flush
 *    （agent task 不应与 operationGroup 合并）
 */
import { describe, expect, it } from "vitest";
import {
  AGENT_TASK_COLLAPSE_CHAR_COUNT,
  AGENT_TASK_COLLAPSE_LINE_COUNT,
  parseAgentTaskContent,
  type RenderedItem,
  renderTurnItems,
} from "@/composables/renderTurnItems";
import type { Message } from "@/composables/useAgent";

function msg(partial: Partial<Message>): Message {
  return {
    role: "user",
    content: "",
    tool_calls: [],
    tool_results: [],
    ...partial,
  } as Message;
}

describe("renderTurnItems - Task 22 agentTask", () => {
  it("role=agent_task + 合法 content 产出 type=agentTask 项", () => {
    const messages: Message[] = [
      msg({
        role: "agent_task",
        content: JSON.stringify({
          subTasks: [
            { id: "s1", status: "in_progress", description: "读取项目结构" },
            { id: "s2", status: "pending", description: "分析依赖关系" },
          ],
          reasoning: "需要拆解为两个独立子任务",
        }),
      }),
    ];
    const items = renderTurnItems(messages, "idle");
    expect(items).toHaveLength(1);
    const at = items[0] as Extract<RenderedItem, { type: "agentTask" }>;
    expect(at.type).toBe("agentTask");
    expect(at.subTasks).toHaveLength(2);
    expect(at.subTasks[0].id).toBe("s1");
    expect(at.subTasks[0].status).toBe("in_progress");
    expect(at.subTasks[0].description).toBe("读取项目结构");
    expect(at.reasoning).toBe("需要拆解为两个独立子任务");
  });

  it("reasoning 字段缺省时不写入 RenderedItem（不污染 props）", () => {
    const messages: Message[] = [
      msg({
        role: "agent_task",
        content: JSON.stringify({
          subTasks: [{ id: "s1", status: "pending", description: "sub1" }],
        }),
      }),
    ];
    const items = renderTurnItems(messages, "idle");
    const at = items[0] as Extract<RenderedItem, { type: "agentTask" }>;
    expect(at.reasoning).toBeUndefined();
    expect("reasoning" in at).toBe(false);
  });

  it("content 解析失败时退化为空 subTasks（不崩溃）", () => {
    const messages: Message[] = [msg({ role: "agent_task", content: "{invalid json" })];
    const items = renderTurnItems(messages, "idle");
    const at = items[0] as Extract<RenderedItem, { type: "agentTask" }>;
    expect(at.type).toBe("agentTask");
    expect(at.subTasks).toEqual([]);
  });

  it("content 为空字符串时退化为空 subTasks", () => {
    const messages: Message[] = [msg({ role: "agent_task", content: "" })];
    const items = renderTurnItems(messages, "idle");
    const at = items[0] as Extract<RenderedItem, { type: "agentTask" }>;
    expect(at.subTasks).toEqual([]);
  });

  it("subTasks 字段为非数组时退化为空", () => {
    const messages: Message[] = [msg({ role: "agent_task", content: JSON.stringify({ subTasks: "not-an-array" }) })];
    const items = renderTurnItems(messages, "idle");
    const at = items[0] as Extract<RenderedItem, { type: "agentTask" }>;
    expect(at.subTasks).toEqual([]);
  });

  it("非法 status 字符串降级为 pending", () => {
    const messages: Message[] = [
      msg({
        role: "agent_task",
        content: JSON.stringify({
          subTasks: [
            { id: "a", status: "unknown-state", description: "foo" },
            { id: "b", status: 123, description: "bar" },
          ],
        }),
      }),
    ];
    const items = renderTurnItems(messages, "idle");
    const at = items[0] as Extract<RenderedItem, { type: "agentTask" }>;
    expect(at.subTasks[0].status).toBe("pending");
    expect(at.subTasks[1].status).toBe("pending");
  });

  it("subTask 缺 id 或 description 时被过滤", () => {
    const messages: Message[] = [
      msg({
        role: "agent_task",
        content: JSON.stringify({
          subTasks: [
            { id: "valid", status: "completed", description: "ok" },
            { id: 123, status: "pending", description: "bad-id" },
            { id: "no-desc", status: "pending" },
            null,
            "not-an-object",
          ],
        }),
      }),
    ];
    const items = renderTurnItems(messages, "idle");
    const at = items[0] as Extract<RenderedItem, { type: "agentTask" }>;
    expect(at.subTasks).toHaveLength(1);
    expect(at.subTasks[0].id).toBe("valid");
  });

  it("agent_task 之前的 operationGroup 仍正确 flush（不合并）", () => {
    const tcSuccess = {
      id: "c1",
      name: "cmd",
      args: JSON.stringify({ command: "ls" }),
      auto_run: true,
      kind: "command" as const,
      needsConfirm: false,
      status: "success" as const,
    };
    const messages: Message[] = [
      msg({
        role: "assistant",
        content: "",
        tool_calls: [tcSuccess],
      }),
      msg({
        role: "agent_task",
        content: JSON.stringify({
          subTasks: [{ id: "s1", status: "pending", description: "sub1" }],
        }),
      }),
    ];
    const items = renderTurnItems(messages, "idle");
    const types = items.map(i => i.type);
    // 期望：operationGroup + agentTask（agentTask 不会合并到 group）
    expect(types).toContain("operationGroup");
    expect(types).toContain("agentTask");
  });

  it("多个 agent_task 各自产出独立项（不合并）", () => {
    const messages: Message[] = [
      msg({
        role: "agent_task",
        content: JSON.stringify({
          subTasks: [{ id: "a1", status: "pending", description: "a" }],
        }),
      }),
      msg({
        role: "agent_task",
        content: JSON.stringify({
          subTasks: [{ id: "b1", status: "pending", description: "b" }],
        }),
      }),
    ];
    const items = renderTurnItems(messages, "idle");
    expect(items.filter(i => i.type === "agentTask").length).toBe(2);
  });
});

describe("parseAgentTaskContent", () => {
  it("解析完整 schema（含 reasoning）", () => {
    const out = parseAgentTaskContent(
      JSON.stringify({
        subTasks: [{ id: "1", status: "in_progress", description: "desc" }],
        reasoning: "r",
      })
    );
    expect(out.subTasks).toHaveLength(1);
    expect(out.reasoning).toBe("r");
  });

  it("解析仅 subTasks 数组", () => {
    const out = parseAgentTaskContent(JSON.stringify({ subTasks: [{ id: "1", status: "pending", description: "d" }] }));
    expect(out.subTasks).toHaveLength(1);
    expect(out.reasoning).toBeUndefined();
  });

  it("解析非法 JSON 返回空 subTasks", () => {
    expect(parseAgentTaskContent("not-json").subTasks).toEqual([]);
  });

  it("解析空字符串返回空 subTasks", () => {
    expect(parseAgentTaskContent("").subTasks).toEqual([]);
  });

  it("支持对象直接传入（不仅是 JSON 字符串）", () => {
    const out = parseAgentTaskContent({
      subTasks: [{ id: "1", status: "completed", description: "d" }],
    });
    expect(out.subTasks).toHaveLength(1);
  });
});

describe("AGENT_TASK collapse constants", () => {
  it("AGENT_TASK_COLLAPSE_LINE_COUNT = 7（对齐 codex-web MessageBlocks.tsx:68）", () => {
    expect(AGENT_TASK_COLLAPSE_LINE_COUNT).toBe(7);
  });
  it("AGENT_TASK_COLLAPSE_CHAR_COUNT = 520（对齐 codex-web MessageBlocks.tsx:69）", () => {
    expect(AGENT_TASK_COLLAPSE_CHAR_COUNT).toBe(520);
  });
});
