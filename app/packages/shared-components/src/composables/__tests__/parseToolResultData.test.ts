/**
 * parseToolResultData.test.ts - 回归测试
 *
 * 修 AG-UI 工具结果 name 缺失问题：
 *  v1：`if (!p.id || !p.name) return null` → AG-UI 归一化结果 `{id, result}`（无 name）
 *      被静默丢弃 → 工具调用了但结果不显示
 *  v2：去掉 name 强制要求；name 缺失时返空串；调用方（handleAgentEvent 的
 *      tool_result case）从已存在的 tool_calls 里按 id 反查补齐
 *
 * 修乱码 bug 根因 + 工具渲染缺失 bug 根因
 * SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/ Phase 4
 */
import { describe, expect, it } from "vitest";
import { parseToolResultData } from "../useAgent";

describe("parseToolResultData", () => {
  it("AGUI_OnlyIdAndResult_NoName: AG-UI 归一化 {id, result}（无 name）— 修复渲染缺失", () => {
    // AG-UI TOOL_CALL_RESULT 事件归一化输出（无 name 字段）
    const data = JSON.stringify({ id: "tc-1", result: '{"hits":3}' });
    const result = parseToolResultData(data);
    expect(result).not.toBeNull();
    expect(result!.id).toBe("tc-1");
    expect(result!.name).toBe(""); // 允许为空，调用方负责补齐
    expect(result!.result).toBe('{"hits":3}');
  });

  it("AGUI_OnlyIdAndResult_Object: AG-UI 归一化对象路径", () => {
    // SSE 层已 JSON.parse 走对象路径
    const data = { id: "tc-2", result: "plain string result" };
    const result = parseToolResultData(data);
    expect(result).not.toBeNull();
    expect(result!.id).toBe("tc-2");
    expect(result!.name).toBe("");
    expect(result!.result).toBe("plain string result");
  });

  it("LegacyFormat_WithName: legacy 格式 {id, name, result}（向后兼容）", () => {
    const data = JSON.stringify({
      id: "tc-3",
      name: "list_files",
      result: "file1,file2",
    });
    const result = parseToolResultData(data);
    expect(result).not.toBeNull();
    expect(result!.id).toBe("tc-3");
    expect(result!.name).toBe("list_files");
    expect(result!.result).toBe("file1,file2");
  });

  it("MissingId_ReturnsNull: 缺 id 时仍返回 null（id 是必填）", () => {
    const data = JSON.stringify({ name: "search", result: "whatever" });
    expect(parseToolResultData(data)).toBeNull();
  });

  it("EmptyIdString_ReturnsNull: id 为空字符串时返回 null", () => {
    const data = JSON.stringify({ id: "", result: "whatever" });
    expect(parseToolResultData(data)).toBeNull();
  });

  it("EmptyResultString_Allowed: result 为空字符串时仍正常解析（不报错）", () => {
    const data = JSON.stringify({ id: "tc-empty", result: "" });
    const result = parseToolResultData(data);
    expect(result).not.toBeNull();
    expect(result!.result).toBe("");
  });

  it("NonStringResult_JSONSerialized: result 是对象/数字时 JSON 序列化", () => {
    // 后端偶尔会传非字符串 result（如 boolean / object / number）
    const data = JSON.stringify({ id: "tc-obj", result: { count: 42, items: [1, 2, 3] } });
    const result = parseToolResultData(data);
    expect(result).not.toBeNull();
    expect(typeof result!.result).toBe("string");
    expect(JSON.parse(result!.result)).toEqual({ count: 42, items: [1, 2, 3] });
  });

  it("IsErrorFlag_Preserved: is_error=true 时正确传递", () => {
    const data = JSON.stringify({ id: "tc-err", result: "failed", is_error: true });
    const result = parseToolResultData(data);
    expect(result).not.toBeNull();
    expect(result!.is_error).toBe(true);
  });

  it("DurationMs_Preserved: duration_ms 数字字段保留", () => {
    const data = JSON.stringify({ id: "tc-dur", result: "ok", duration_ms: 1234 });
    const result = parseToolResultData(data);
    expect(result).not.toBeNull();
    expect(result!.duration_ms).toBe(1234);
  });

  it("InvalidJSON_ReturnsNull: 无效 JSON 字符串返回 null", () => {
    expect(parseToolResultData("not json")).toBeNull();
    expect(parseToolResultData("{incomplete")).toBeNull();
  });

  it("NullData_ReturnsNull: null 输入返回 null", () => {
    expect(parseToolResultData(null)).toBeNull();
  });

  it("ArrayData_ReturnsNull: 数组输入返回 null（要求 object）", () => {
    expect(parseToolResultData([])).toBeNull();
    expect(parseToolResultData([1, 2, 3])).toBeNull();
  });
});
