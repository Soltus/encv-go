/**
 * useToolCallAccumulator.test.ts — Stage 2 单测
 *
 * 覆盖 nuclear-boy 实战踩坑场景：
 *   1. 单一 tool call 完整累积
 *   2. 同一轮 2-3 个 tool call 不互相覆盖
 *   3. 中断累积不破坏下一个 tool call
 *   4. args JSON 解析失败的容错
 *   5. clear() 在 ReAct 循环**开始**时调用，**不**在 TOOL_CALL_START 时清
 *   6. feedStart 收到重复 id 时不覆盖已累积 args
 */

import { describe, expect, it } from "vitest";
import { createToolCallAccumulator, parseAccumulatedArgs } from "../useToolCallAccumulator";

describe("useToolCallAccumulator", () => {
  it("single tool call completes full lifecycle", () => {
    const acc = createToolCallAccumulator();

    acc.feedStart({ id: "tc-1", name: "read_file" });
    expect(acc.size()).toBe(1);
    expect(acc.hasPartial()).toBe(true);

    acc.feedArgs({ id: "tc-1", argsDelta: '{"path":' });
    acc.feedArgs({ id: "tc-1", argsDelta: '"/a.txt"}' });
    acc.complete("tc-1");

    const completed = acc.getCompleted();
    expect(completed).toHaveLength(1);
    expect(completed[0].name).toBe("read_file");
    expect(completed[0].args).toBe('{"path":"/a.txt"}');
    expect(completed[0].status).toBe("complete");
    expect(acc.hasPartial()).toBe(false);
  });

  it("multiple tool calls in same ReAct round do not overwrite each other", () => {
    // 借鉴 nuclear-boy L707: "Clear once per API call, NOT per individual tool call"
    const acc = createToolCallAccumulator();

    // 第 1 个 tool call
    acc.feedStart({ id: "tc-1", name: "read_file" });
    acc.feedArgs({ id: "tc-1", argsDelta: '{"path":"/a.txt"}' });
    acc.complete("tc-1");

    // 第 2 个 tool call（**不**调用 clear()）
    acc.feedStart({ id: "tc-2", name: "list_files" });
    acc.feedArgs({ id: "tc-2", argsDelta: '{"path":"/"}' });
    acc.complete("tc-2");

    // 第 3 个
    acc.feedStart({ id: "tc-3", name: "stat_file" });
    acc.feedArgs({ id: "tc-3", argsDelta: '{"path":"/a.txt"}' });
    acc.complete("tc-3");

    expect(acc.size()).toBe(3);
    const all = acc.getAll();
    expect(all.map(t => t.name)).toEqual(["read_file", "list_files", "stat_file"]);
    expect(all.map(t => t.args)).toEqual(['{"path":"/a.txt"}', '{"path":"/"}', '{"path":"/a.txt"}']);
  });

  it("feedStart with duplicate id preserves accumulated args", () => {
    // nuclear-boy 实战：协议重传或部分重放时 id 可能重复
    const acc = createToolCallAccumulator();

    acc.feedStart({ id: "tc-1", name: "read_file" });
    acc.feedArgs({ id: "tc-1", argsDelta: '{"path":' });
    acc.feedArgs({ id: "tc-1", argsDelta: '"/a.txt"}' });

    // 重复 start（不应该清掉 args）
    acc.feedStart({ id: "tc-1", name: "read_file_v2" });

    const all = acc.getAll();
    expect(all).toHaveLength(1);
    expect(all[0].args).toBe('{"path":"/a.txt"}'); // args 保留
    expect(all[0].name).toBe("read_file_v2"); // name 更新
  });

  it("aborted accumulation does not break next tool call", () => {
    const acc = createToolCallAccumulator();

    acc.feedStart({ id: "tc-1", name: "read_file" });
    acc.feedArgs({ id: "tc-1", argsDelta: '{"path"' }); // 不完整
    // 假设中断（没有 complete）
    expect(acc.hasPartial()).toBe(true);

    // 下一轮 clear（ReAct 循环开始）
    acc.clear();
    expect(acc.size()).toBe(0);
    expect(acc.hasPartial()).toBe(false);

    acc.feedStart({ id: "tc-2", name: "list_files" });
    acc.feedArgs({ id: "tc-2", argsDelta: "{}" });
    acc.complete("tc-2");

    expect(acc.size()).toBe(1);
    expect(acc.getCompleted()[0].name).toBe("list_files");
  });

  it("feedArgs without prior feedStart is tolerated", () => {
    // 协议异常场景：先收到 args，没收到 start
    const acc = createToolCallAccumulator();
    acc.feedArgs({ id: "tc-orphan", argsDelta: '{"x":1}' });

    expect(acc.size()).toBe(1);
    expect(acc.hasPartial()).toBe(true);
    expect(acc.getAll()[0].status).toBe("accumulating");
  });

  it("setResult marks tool as executed", () => {
    const acc = createToolCallAccumulator();
    acc.feedStart({ id: "tc-1", name: "read_file" });
    acc.feedArgs({ id: "tc-1", argsDelta: "{}" });
    acc.complete("tc-1");
    acc.setResult({ id: "tc-1", result: '{"content":"hello"}' });

    const executed = acc.getExecuted();
    expect(executed).toHaveLength(1);
    expect(executed[0].status).toBe("executed");
    expect(executed[0].result).toBe('{"content":"hello"}');
  });

  it("setResult records error", () => {
    const acc = createToolCallAccumulator();
    acc.feedStart({ id: "tc-1", name: "read_file" });
    acc.feedArgs({ id: "tc-1", argsDelta: "{}" });
    acc.complete("tc-1");
    acc.setResult({ id: "tc-1", result: "", error: "permission denied" });

    const exec = acc.getExecuted()[0];
    expect(exec.status).toBe("executed");
    expect(exec.error).toBe("permission denied");
  });
});

describe("parseAccumulatedArgs", () => {
  it("parses valid JSON", () => {
    expect(parseAccumulatedArgs('{"path":"/a.txt"}')).toEqual({ path: "/a.txt" });
  });

  it("returns empty object for empty string (nuclear-boy L776-798 容错)", () => {
    expect(parseAccumulatedArgs("")).toEqual({});
    expect(parseAccumulatedArgs("   ")).toEqual({});
  });

  it("returns empty object for invalid JSON (nuclear-boy 容错)", () => {
    expect(parseAccumulatedArgs("{not valid")).toEqual({});
  });

  it("returns empty object for array JSON (不是对象)", () => {
    expect(parseAccumulatedArgs("[1,2,3]")).toEqual({});
  });

  it("returns empty object for null JSON", () => {
    expect(parseAccumulatedArgs("null")).toEqual({});
  });
});
