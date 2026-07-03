/**
 * parseContentDelta.test.ts - 回归测试
 *
 * 修乱码 bug：AG-UI 归一化路径产出的 data 形如
 *   `JSON.stringify({text: "你", messageId: "msg_..."})`
 * （**无 seq 字段**），原实现无匹配分支，落到末尾
 *   `return { text: data }` → 把整段 JSON 字符串当文本渲染。
 *
 * 本 spec 验证 4 个 case：
 *  1. AG-UI 归一化字符串（{text} 无 seq）— 这是主要 bug 修复
 *  2. 旧格式 {"content":"..."} 字符串
 *  3. 新格式 {seq, text} 字符串（带序号）
 *  4. 纯文本字符串（非 JSON）
 *  5. AG-UI 归一化对象（{text} 无 seq）— 同样修复对象路径
 *
 * SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/
 */
import { describe, expect, it } from "vitest";
import { parseContentDelta } from "../useAgent";

describe("parseContentDelta", () => {
  it("AGUI_String_JustTextNoSeq: AG-UI {text, messageId} 字符串（无 seq）— 修复乱码", () => {
    // 这是 useAGUIParser 对 TEXT_MESSAGE_CONTENT 事件的归一化输出
    const data = JSON.stringify({ text: "你好", messageId: "msg_xxx" });
    const result = parseContentDelta(data);
    expect(result.text).toBe("你好");
    expect(result.seq).toBeUndefined();
    // 关键断言：不能把整段 JSON 当文本返回
    expect(result.text).not.toContain("messageId");
    expect(result.text).not.toContain("msg_xxx");
  });

  it('Legacy_String_ContentField: 旧格式 {"content":"..."} 字符串', () => {
    const data = JSON.stringify({ content: "legacy hello" });
    const result = parseContentDelta(data);
    expect(result.text).toBe("legacy hello");
  });

  it("NewFormat_String_TextAndSeq: 新格式 {text, seq} 字符串", () => {
    const data = JSON.stringify({ text: "streamed", seq: 42 });
    const result = parseContentDelta(data);
    expect(result.text).toBe("streamed");
    expect(result.seq).toBe(42);
  });

  it("PlainString_NonJSON: 纯文本字符串（非 JSON）", () => {
    const data = "just plain text";
    const result = parseContentDelta(data);
    expect(result.text).toBe("just plain text");
  });

  it("AGUI_Object_JustTextNoSeq: AG-UI {text, messageId} 对象（无 seq）— 修复对象路径", () => {
    // SSE 层已 JSON.parse 时走这条路径
    const data = { text: "对象路径你好", messageId: "msg_yyy" };
    const result = parseContentDelta(data);
    expect(result.text).toBe("对象路径你好");
    expect(result.seq).toBeUndefined();
  });

  it('AGUI_String_EmptyText: AG-UI {text: ""} 字符串', () => {
    const data = JSON.stringify({ text: "", messageId: "msg_empty" });
    const result = parseContentDelta(data);
    expect(result.text).toBe("");
  });

  it("NullData: null 输入 → 空文本", () => {
    const result = parseContentDelta(null);
    expect(result.text).toBe("");
  });

  it("UndefinedData: undefined 输入 → 空文本", () => {
    const result = parseContentDelta(undefined);
    expect(result.text).toBe("");
  });

  it("AGUI_RealtimeStream: 模拟真实流式 4 个 chunk 累积", () => {
    // 模拟后端连续推 4 个 TEXT_MESSAGE_CONTENT：data 形状是 {text, messageId}
    const chunks = [
      JSON.stringify({ text: "你", messageId: "m1" }),
      JSON.stringify({ text: "好", messageId: "m1" }),
      JSON.stringify({ text: "，", messageId: "m1" }),
      JSON.stringify({ text: "世界", messageId: "m1" }),
    ];
    const assembled = chunks.map(c => parseContentDelta(c).text).join("");
    expect(assembled).toBe("你好，世界");
  });
});
