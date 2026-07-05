/**
 * renderTurnItems.compaction.test.ts - Task 7 单元测试
 *
 * 覆盖：
 * 1. role='system' + content=CONTEXT_COMPACTION_MARKER 的消息
 *    产出 type='compaction' 的 RenderedItem
 * 2. 文本默认用 i18n 解析后的文本（compactionText 参数）
 * 3. 缺省 compactionText 时回退到 marker 本身
 * 4. 多次 compaction 事件（多条 marker 消息）都能正确产出独立项
 * 5. compaction 之后 / 之前穿插 user/assistant 消息不影响分组累积
 */
import { describe, expect, it } from "vitest";
import { type RenderedItem, renderTurnItems } from "@/composables/renderTurnItems";
import { CONTEXT_COMPACTION_MARKER, type Message } from "@/composables/useAgent";

function msg(partial: Partial<Message>): Message {
  return {
    role: "user",
    content: "",
    tool_calls: [],
    tool_results: [],
    ...partial,
  } as Message;
}

describe("renderTurnItems - Task 7 compaction divider", () => {
  it("role=system + marker content 产出 type=compaction 项", () => {
    const messages: Message[] = [
      msg({ role: "user", content: "hi" }),
      msg({ role: "assistant", content: "reply" }),
      msg({ role: "system", content: CONTEXT_COMPACTION_MARKER }),
    ];
    const items = renderTurnItems(messages, "idle");
    const compactionItems = items.filter(i => i.type === "compaction");
    expect(compactionItems.length).toBe(1);
    const ci = compactionItems[0] as Extract<RenderedItem, { type: "compaction" }>;
    expect(ci.text).toBe(CONTEXT_COMPACTION_MARKER);
    expect(ci.messageId).toBeTruthy();
  });

  it("compactionText 优先于 marker 本身（i18n 覆盖）", () => {
    const messages: Message[] = [msg({ role: "system", content: CONTEXT_COMPACTION_MARKER })];
    const items = renderTurnItems(messages, "idle", "Context auto-compressed");
    const ci = items.find(i => i.type === "compaction") as Extract<RenderedItem, { type: "compaction" }>;
    expect(ci.text).toBe("Context auto-compressed");
  });

  it("未传 compactionText 时回退到 marker 本身", () => {
    const messages: Message[] = [msg({ role: "system", content: CONTEXT_COMPACTION_MARKER })];
    const items = renderTurnItems(messages, "idle");
    const ci = items.find(i => i.type === "compaction") as Extract<RenderedItem, { type: "compaction" }>;
    expect(ci.text).toBe(CONTEXT_COMPACTION_MARKER);
  });

  it("多次 compaction 事件产出多个独立项（不合并）", () => {
    const messages: Message[] = [
      msg({ role: "user", content: "m1" }),
      msg({ role: "system", content: CONTEXT_COMPACTION_MARKER }),
      msg({ role: "user", content: "m2" }),
      msg({ role: "system", content: CONTEXT_COMPACTION_MARKER }),
      msg({ role: "user", content: "m3" }),
    ];
    const items = renderTurnItems(messages, "idle");
    expect(items.filter(i => i.type === "compaction").length).toBe(2);
    // 3 个 user 都正常产出
    expect(items.filter(i => i.type === "user").length).toBe(3);
  });

  it("compaction 出现在 user/assistant 之间的混合消息流中", () => {
    const messages: Message[] = [
      msg({ role: "user", content: "q1" }),
      msg({ role: "assistant", content: "a1" }),
      msg({ role: "system", content: CONTEXT_COMPACTION_MARKER }),
      msg({ role: "user", content: "q2" }),
      msg({ role: "assistant", content: "a2" }),
    ];
    const items = renderTurnItems(messages, "idle");
    const types = items.map(i => i.type);
    // 期望顺序：user, assistantText, compaction, user, assistantText
    expect(types).toEqual(["user", "assistantText", "compaction", "user", "assistantText"]);
  });

  it("compaction 标记外的 system 消息不会被当作 compaction 项", () => {
    const messages: Message[] = [msg({ role: "system", content: "some other system message" })];
    const items = renderTurnItems(messages, "idle");
    expect(items.filter(i => i.type === "compaction").length).toBe(0);
  });
});
