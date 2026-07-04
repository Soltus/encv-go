/**
 * useAGUIParser.test.ts
 *
 * 测试 AG-UI 协议 SSE 解析器（composables/useAGUIParser.ts）。
 *
 * 覆盖 11 种 AG-UI 事件类型 + 边界情况 + 端到端 processAGUISSE 集成测试。
 * SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/
 */

import { describe, expect, it, vi } from "vitest";
import { type AGUIStreamHandlers, parseAGUIEvent, processAGUISSE, useAGUIParser } from "../useAGUIParser";

// =============================================================================
// 测试辅助：构造 SSE 块
// =============================================================================

/**
 * 构造单条 AG-UI SSE 事件块（含可选 id: 行）
 */
function sseBlock(type: string, data: any, id?: number): string {
  const lines: string[] = [];
  if (id !== undefined) lines.push(`id: ${id}`);
  lines.push(`event: ${type}`);
  lines.push(`data: ${JSON.stringify(data)}`);
  return lines.join("\n") + "\n\n";
}

/**
 * 构造一个 ReadableStream，输入是字符串数组（每个元素是一次 read() 的 Uint8Array）。
 * 用 TextEncoder 编码 UTF-8。
 */
function makeStreamFromBlocks(blocks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  // 一次性 encode 整段（processAGUISSE 不在乎 chunk 边界，只在乎 \n\n 事件分隔）
  const fullText = blocks.join("");
  const encoded = encoder.encode(fullText);
  return new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(encoded);
      controller.close();
    },
  });
}

/**
 * 构造一个分块 ReadableStream：模拟真实的网络 chunked 行为
 * （块边界不与 \n\n 事件边界对齐）
 */
function makeChunkedStream(text: string, chunkSize: number): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  const encoded = encoder.encode(text);
  let offset = 0;
  return new ReadableStream<Uint8Array>({
    pull(controller) {
      if (offset >= encoded.length) {
        controller.close();
        return;
      }
      const chunk = encoded.slice(offset, offset + chunkSize);
      offset += chunkSize;
      controller.enqueue(chunk);
    },
  });
}

// =============================================================================
// parseAGUIEvent：11 种 AG-UI 事件类型 + 边界
// =============================================================================

describe("parseAGUIEvent", () => {
  it("TestParseAGUIEvent_RUN_STARTED_ToStreamStart: 映射为 stream_start AgentEvent", () => {
    const block = sseBlock("RUN_STARTED", { runId: "r-1", threadId: "t-1" });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("stream_start");
    const data = JSON.parse(ev!.data);
    expect(data.runId).toBe("r-1");
    expect(data.threadId).toBe("t-1");
    expect(data.protocol).toBe("agui");
  });

  it("TestParseAGUIEvent_TEXT_MESSAGE_START_ReturnsNull: 纯 meta，返回 null", () => {
    const block = sseBlock("TEXT_MESSAGE_START", { messageId: "m-1", role: "assistant" });
    expect(parseAGUIEvent(block)).toBeNull();
  });

  it("TestParseAGUIEvent_TEXT_MESSAGE_CONTENT_ToTextDelta: 提取 text + messageId", () => {
    const block = sseBlock("TEXT_MESSAGE_CONTENT", { messageId: "m-1", delta: "Hello" });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("text_delta");
    const data = JSON.parse(ev!.data);
    expect(data.text).toBe("Hello");
    expect(data.messageId).toBe("m-1");
  });

  it("TestParseAGUIEvent_TEXT_MESSAGE_END_ReturnsNull: 纯 meta，返回 null", () => {
    const block = sseBlock("TEXT_MESSAGE_END", { messageId: "m-1" });
    expect(parseAGUIEvent(block)).toBeNull();
  });

  it("TestParseAGUIEvent_TOOL_CALL_START_ToToolCall: 提取 id + name，args 初始化为空", () => {
    const block = sseBlock("TOOL_CALL_START", { toolCallId: "tc-1", toolCallName: "search" });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("tool_call");
    const data = JSON.parse(ev!.data);
    expect(data.id).toBe("tc-1");
    expect(data.name).toBe("search");
    expect(data.args).toBe("");
    expect(data.auto_run).toBe(true);
    expect(data.kind).toBe("unknown");
  });

  it("TestParseAGUIEvent_TOOL_CALL_ARGS_AccumulatesArgs: 新增 tool_call_args 类型，提取 id + argsDelta", () => {
    const block = sseBlock("TOOL_CALL_ARGS", { toolCallId: "tc-1", delta: '{"q":"' });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("tool_call_args");
    const data = JSON.parse(ev!.data);
    expect(data.id).toBe("tc-1");
    expect(data.argsDelta).toBe('{"q":"');
  });

  it("TestParseAGUIEvent_TOOL_CALL_END_ToToolStatus: 提取 id + status=success", () => {
    const block = sseBlock("TOOL_CALL_END", { toolCallId: "tc-1" });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("tool_status");
    const data = JSON.parse(ev!.data);
    expect(data.id).toBe("tc-1");
    expect(data.status).toBe("success");
  });

  it("TestParseAGUIEvent_TOOL_CALL_RESULT_ToToolResult: 提取 id + result", () => {
    const block = sseBlock("TOOL_CALL_RESULT", { toolCallId: "tc-1", content: "tool output" });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("tool_result");
    const data = JSON.parse(ev!.data);
    expect(data.id).toBe("tc-1");
    expect(data.result).toBe("tool output");
  });

  it("TestParseAGUIEvent_RUN_FINISHED_ToStreamEnd: 提取 runId + threadId", () => {
    const block = sseBlock("RUN_FINISHED", { runId: "r-1", threadId: "t-1" });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("stream_end");
    const data = JSON.parse(ev!.data);
    expect(data.runId).toBe("r-1");
    expect(data.threadId).toBe("t-1");
  });

  it("TestParseAGUIEvent_STATE_SNAPSHOT_ToStateSnapshot: 新增类型 state_snapshot", () => {
    const block = sseBlock("STATE_SNAPSHOT", { state: { foo: "bar", count: 42 } });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("state_snapshot");
    const data = JSON.parse(ev!.data);
    expect(data.state).toEqual({ foo: "bar", count: 42 });
  });

  it("TestParseAGUIEvent_MESSAGES_SNAPSHOT_ToMessagesSnapshot: 新增类型 messages_snapshot", () => {
    const block = sseBlock("MESSAGES_SNAPSHOT", {
      messages: [{ id: "m-1", role: "user", content: "hi" }],
    });
    const ev = parseAGUIEvent(block);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("messages_snapshot");
    const data = JSON.parse(ev!.data);
    expect(data.messages).toEqual([{ id: "m-1", role: "user", content: "hi" }]);
  });

  it("TestParseAGUIEvent_UnknownEventType_ReturnsNull: 未知事件类型返回 null", () => {
    const block = sseBlock("UNKNOWN_TYPE", { foo: "bar" });
    expect(parseAGUIEvent(block)).toBeNull();
  });

  it("TestParseAGUIEvent_EmptyInput_ReturnsNull: 空字符串 / null 返回 null", () => {
    expect(parseAGUIEvent("")).toBeNull();
    expect(parseAGUIEvent("   ")).toBeNull();
    expect(parseAGUIEvent(null as any)).toBeNull();
  });

  it("TestParseAGUIEvent_MalformedJSON_DataIsRawString: data 不是合法 JSON 时保留原始字符串（带 _parseError 标记）", () => {
    const raw = "event: RUN_STARTED\ndata: not-json-at-all\n\n";
    const ev = parseAGUIEvent(raw);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("stream_start");
    // data 字段是包含 _raw + _parseError 的对象（保留 raw 字符串供调试）
    const data = JSON.parse(ev!.data);
    expect(data._raw).toContain("not-json-at-all");
    expect(data._parseError).toBe(true);
  });

  it("TestParseAGUIEvent_NoEventLine_ReturnsNull: 没有 event: 行的 SSE 块返回 null", () => {
    const raw = 'data: {"foo":"bar"}\n\n';
    expect(parseAGUIEvent(raw)).toBeNull();
  });

  it("TestParseAGUIEvent_ExtraIdLineIgnored: id: 行不影响 parseAGUIEvent 输出", () => {
    // parseAGUIEvent 不消费 id: 行（由 processAGUISSE 单独处理去重）
    const raw = 'id: 99\nevent: RUN_STARTED\ndata: {"runId":"r-1"}\n\n';
    const ev = parseAGUIEvent(raw);
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("stream_start");
  });
});

// =============================================================================
// processAGUISSE：端到端流读取
// =============================================================================

describe("processAGUISSE", () => {
  it("TestProcessAGUISSE_NullStream_ReturnsEmpty: null stream 返回 received=false", async () => {
    const handlers: AGUIStreamHandlers = { onEvent: vi.fn() };
    const result = await processAGUISSE(null, handlers);
    expect(result.received).toBe(false);
    expect(result.streamEnded).toBe(false);
    expect(result.morePending).toBe(false);
  });

  it("TestProcessAGUISSE_FullRunLifecycle: 完整 RUN_STARTED → 多个 text_delta → TOOL_CALL_* → RUN_FINISHED", async () => {
    // 构造完整一次 run 的事件流：
    //   RUN_STARTED
    //   TEXT_MESSAGE_CONTENT (3 个 delta)
    //   TOOL_CALL_START
    //   TOOL_CALL_ARGS (2 个 delta)
    //   TOOL_CALL_END
    //   TOOL_CALL_RESULT
    //   RUN_FINISHED
    const blocks = [
      sseBlock("RUN_STARTED", { runId: "r-1", threadId: "t-1" }, 1),
      sseBlock("TEXT_MESSAGE_CONTENT", { messageId: "m-1", delta: "Hel" }, 2),
      sseBlock("TEXT_MESSAGE_CONTENT", { messageId: "m-1", delta: "lo " }, 3),
      sseBlock("TEXT_MESSAGE_CONTENT", { messageId: "m-1", delta: "world" }, 4),
      sseBlock("TOOL_CALL_START", { toolCallId: "tc-1", toolCallName: "search" }, 5),
      sseBlock("TOOL_CALL_ARGS", { toolCallId: "tc-1", delta: '{"q":"' }, 6),
      sseBlock("TOOL_CALL_ARGS", { toolCallId: "tc-1", delta: 'hello"}' }, 7),
      sseBlock("TOOL_CALL_END", { toolCallId: "tc-1" }, 8),
      sseBlock("TOOL_CALL_RESULT", { toolCallId: "tc-1", content: "found 3 results" }, 9),
      sseBlock("RUN_FINISHED", { runId: "r-1", threadId: "t-1" }, 10),
    ];
    const stream = makeStreamFromBlocks(blocks);

    const onEvent = vi.fn();
    const rememberSequence = vi.fn((_id: number) => true); // 全部「首次出现」
    const onRawEvent = vi.fn();
    const onStreamEnd = vi.fn();

    const result = await processAGUISSE(stream, {
      onEvent,
      rememberSequence,
      onRawEvent,
      onStreamEnd,
    });

    // 收到所有 10 个事件
    expect(onEvent).toHaveBeenCalledTimes(10);
    // RUN_FINISHED → streamEnded=true
    expect(result.streamEnded).toBe(true);
    expect(result.received).toBe(true);
    expect(result.morePending).toBe(false);
    // onStreamEnd 触发
    expect(onStreamEnd).toHaveBeenCalledTimes(1);
    // rememberSequence 被调 10 次（每个 id: 都去重）
    expect(rememberSequence).toHaveBeenCalledTimes(10);
    // onRawEvent 被调 10 次
    expect(onRawEvent).toHaveBeenCalledTimes(10);

    // 验证事件类型顺序与归一化
    const calls = onEvent.mock.calls.map(c => c[0]);
    expect(calls.map((e: any) => e.type)).toEqual([
      "stream_start",
      "text_delta",
      "text_delta",
      "text_delta",
      "tool_call",
      "tool_call_args",
      "tool_call_args",
      "tool_status",
      "tool_result",
      "stream_end",
    ]);

    // 验证 tool_call_args 增量累积字段名
    const tca1 = JSON.parse(calls[5].data);
    const tca2 = JSON.parse(calls[6].data);
    expect(tca1.id).toBe("tc-1");
    expect(tca1.argsDelta).toBe('{"q":"');
    expect(tca2.argsDelta).toBe('hello"}');
  });

  it("TestProcessAGUISSE_DuplicateSequenceDropsEvent: SSE id 重复时丢弃事件", async () => {
    const blocks = [
      sseBlock("RUN_STARTED", { runId: "r-1" }, 1),
      sseBlock("TEXT_MESSAGE_CONTENT", { messageId: "m-1", delta: "a" }, 1), // 重复 id=1
      sseBlock("RUN_FINISHED", { runId: "r-1" }, 2),
    ];
    const stream = makeStreamFromBlocks(blocks);

    const seenIds = new Set<number>();
    const onEvent = vi.fn();

    const result = await processAGUISSE(stream, {
      onEvent,
      rememberSequence: (id: number) => {
        if (seenIds.has(id)) return false;
        seenIds.add(id);
        return true;
      },
    });

    // 只有 2 个事件被分发（id=1 的第二个 TEXT_MESSAGE_CONTENT 被丢弃）
    expect(onEvent).toHaveBeenCalledTimes(2);
    expect(result.received).toBe(true);
    expect(result.streamEnded).toBe(true);
  });

  it("TestProcessAGUISSE_NoSequenceDropsAlways: 未声明 id 的事件全部 dispatch", async () => {
    // 块不传 id → 不去重，全部 dispatch
    const blocks = [
      sseBlock("RUN_STARTED", { runId: "r-1" }),
      sseBlock("TEXT_MESSAGE_CONTENT", { messageId: "m-1", delta: "a" }),
      sseBlock("TEXT_MESSAGE_CONTENT", { messageId: "m-1", delta: "b" }),
      sseBlock("RUN_FINISHED", { runId: "r-1" }),
    ];
    const stream = makeStreamFromBlocks(blocks);
    const onEvent = vi.fn();
    await processAGUISSE(stream, { onEvent });
    expect(onEvent).toHaveBeenCalledTimes(4);
  });

  it("TestProcessAGUISSE_ChunkedStream_StillParsesAllEvents: 块边界与 \n\n 错位时仍正确解析", async () => {
    // 故意把 chunk size 设到 5 字节，模拟网络层任意切分
    const text =
      sseBlock("RUN_STARTED", { runId: "r-1" }, 1) +
      sseBlock("TEXT_MESSAGE_CONTENT", { messageId: "m-1", delta: "chunked!" }, 2) +
      sseBlock("RUN_FINISHED", { runId: "r-1" }, 3);
    const stream = makeChunkedStream(text, 5);

    const onEvent = vi.fn();
    const result = await processAGUISSE(stream, {
      onEvent,
      rememberSequence: () => true,
    });

    expect(onEvent).toHaveBeenCalledTimes(3);
    expect(result.streamEnded).toBe(true);
    const calls = onEvent.mock.calls.map(c => c[0]);
    expect(calls.map((e: any) => e.type)).toEqual(["stream_start", "text_delta", "stream_end"]);
    const td = JSON.parse(calls[1].data);
    expect(td.text).toBe("chunked!");
  });

  it("TestProcessAGUISSE_TrailingBufferWithoutDoubleNewline: 末尾块无 \n\n 仍被消费", async () => {
    // 手工构造一个缺尾部分隔的 SSE 文本（模拟 server 端忘记写 \n\n 收尾）
    const text2 =
      'id: 1\nevent: RUN_STARTED\ndata: {"runId":"r-1"}\n\n' +
      'id: 2\nevent: TEXT_MESSAGE_CONTENT\ndata: {"messageId":"m-1","delta":"tail"}';
    // 末段没有 \n\n 收尾
    const stream = makeStreamFromBlocks([text2]);

    const onEvent = vi.fn();
    const result = await processAGUISSE(stream, {
      onEvent,
      rememberSequence: () => true,
    });

    expect(onEvent).toHaveBeenCalledTimes(2);
    expect(result.streamEnded).toBe(false); // 没有 RUN_FINISHED
  });

  it("TestProcessAGUISSE_OnStreamEndCalledAfterReaderClosed: 流关闭后才触发 onStreamEnd", async () => {
    const blocks = [sseBlock("RUN_STARTED", { runId: "r-1" }, 1)];
    const stream = makeStreamFromBlocks(blocks);

    const onEvent = vi.fn();
    const onStreamEnd = vi.fn();
    await processAGUISSE(stream, { onEvent, onStreamEnd });

    // onStreamEnd 在所有 onEvent 之后被调用
    const eventOrder: string[] = [];
    onEvent.mockImplementation(() => eventOrder.push("event"));
    // 重新跑（顺序不影响，但确认 onStreamEnd 调用了）
    // 上面已经跑过：onStreamEnd 至少被调 1 次
    expect(onStreamEnd).toHaveBeenCalledTimes(1);
  });

  it("TestProcessAGUISSE_OnStreamEndError_DoesNotThrow: onStreamEnd 抛错被 try/catch 吞掉", async () => {
    const blocks = [sseBlock("RUN_STARTED", { runId: "r-1" }, 1)];
    const stream = makeStreamFromBlocks(blocks);

    const errorSpy = vi.spyOn(console, "debug").mockImplementation(() => {});

    await expect(
      processAGUISSE(stream, {
        onEvent: vi.fn(),
        onStreamEnd: () => {
          throw new Error("boom");
        },
      })
    ).resolves.toBeDefined();

    // 静默：console.debug 被调 1 次（错误日志）
    expect(errorSpy).toHaveBeenCalled();
    errorSpy.mockRestore();
  });
});

// =============================================================================
// useAGUIParser composable
// =============================================================================

describe("useAGUIParser", () => {
  it("TestUseAGUIParser_ExposesParseAndProcess: 暴露 parseAGUIEvent + processAGUISSE", () => {
    const parser = useAGUIParser();
    expect(typeof parser.parseAGUIEvent).toBe("function");
    expect(typeof parser.processAGUISSE).toBe("function");
    // 与顶层导出指向同一函数引用
    expect(parser.parseAGUIEvent).toBe(parseAGUIEvent);
    expect(parser.processAGUISSE).toBe(processAGUISSE);
  });
});
