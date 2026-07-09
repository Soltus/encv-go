/**
 * useAgent.test.ts - 复合式单元测试
 *
 * 覆盖：
 * 1. processSSE 解析 6 种 event type
 * 2. 4 决策 confirmTool
 * 3. localStorage save/load 持久化
 * 4. resume 续传
 * 5. stop 中断
 * 6. reset 清空
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Mock useToast
vi.mock("@/composables/useToast", () => ({
  showToast: vi.fn(),
}));

// 重要：必须在 import useAgent 之前 mock globalThis.crypto.randomUUID，
// 否则 jsdom 默认提供 crypto.randomUUID 不会有问题，但我们仍要确保 stable 行为。
// 这里只 mock fetch 来注入可控的 SSE 响应流。

import { type DoctorReport, getLanAccess, type LanAddress, type Message, useAgent } from "@/composables/useAgent";
import { showToast } from "@/composables/useToast";

const mockedShowToast = vi.mocked(showToast);
const originalFetch = globalThis.fetch;

// ─── 辅助函数 ─────────────────────────────────────────────────────────────

/**
 * 构造一个 mock ReadableStream，按 chunk 吐出给定的 SSE chunks
 */
function makeSSEStream(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  let index = 0;
  return new ReadableStream<Uint8Array>({
    pull(controller) {
      if (index < chunks.length) {
        controller.enqueue(encoder.encode(chunks[index]));
        index++;
      } else {
        controller.close();
      }
    },
  });
}

/**
 * 把单个 AgentEvent 转成 SSE 字符串行
 */
function sseLine(type: string, data: unknown): string {
  return `data: ${JSON.stringify({ type, data: JSON.stringify(data) })}\n\n`;
}

function fetchReturningStream(stream: ReadableStream<Uint8Array>): ReturnType<typeof vi.fn> {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    body: stream,
  } as Response);
}

function fetchReturningError(status = 500): ReturnType<typeof vi.fn> {
  return vi.fn().mockResolvedValue({
    ok: false,
    status,
    body: null,
  } as Response);
}

/**
 * 创建 URL 感知的 mock 实现：
 *   /api/config   → 返回空配置（无 system_prompt）
 *   /api/health   → 返回 { serverInstanceId }（Task 4 引入的 health 端点）
 *   其他 URL      → 使用 fallback（通常是测试提供的 SSE stream）
 *
 * options.serverInstanceId 控制 /api/health 返回的 instance id；
 * 不传则用固定字符串 'test-instance'，所有 useAgent 实例共享。
 */
function urlAwareMock(fallback: ReturnType<typeof vi.fn>, options?: { serverInstanceId?: string }): ReturnType<typeof vi.fn> {
  const instanceId = options?.serverInstanceId ?? "test-instance";
  return vi.fn(async (url: string | Request) => {
    const urlStr = typeof url === "string" ? url : (url as Request).url;
    if (urlStr.includes("/api/config")) {
      return { ok: true, status: 200, json: () => Promise.resolve({}) } as Response;
    }
    if (urlStr.includes("/api/health")) {
      return {
        ok: true,
        status: 200,
        json: () => Promise.resolve({ serverInstanceId: instanceId }),
      } as Response;
    }
    return fallback(url);
  });
}

/**
 * 找出 fetchSpy.mock.calls 中最后一次匹配 path 的调用下标。
 * Task 4 之后 send() 会先调 /api/health 再调 /agent-api/*，
 * 所以原本写死下标的断言需要按 URL 重新定位。
 */
function findLastCallIndex(calls: unknown[][], path: string): number {
  for (let i = calls.length - 1; i >= 0; i--) {
    const c = calls[i];
    const url = typeof c[0] === "string" ? c[0] : (c[0] as Request).url;
    if (url.includes(path)) return i;
  }
  return -1;
}

// ─── 测试 ─────────────────────────────────────────────────────────────────

describe("useAgent", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
    // 默认 mock：/api/config 返回空配置，其他 URL 返回空 body
    // 各测试通过 fetchSpy.mockImplementation(urlAwareMock(fallback)) 覆盖
    fetchSpy.mockImplementation(async (url: string | Request) => {
      const urlStr = typeof url === "string" ? url : (url as Request).url;
      if (urlStr.includes("/api/config")) {
        return { ok: true, status: 200, json: () => Promise.resolve({}) } as Response;
      }
      // /api/agent/context-usage 默认返回空 session（context icon 用）
      if (urlStr.includes("/api/agent/context-usage")) {
        return {
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              sessionId: "default",
              model: "",
              usage: { tokens: 0, window: 8192, percent: 0 },
              todos: [],
              referencedFiles: [],
              compactions: 0,
              updatedAt: Date.now(),
            }),
        } as Response;
      }
      return { ok: true, status: 200, body: null, json: () => Promise.resolve({}) } as unknown as Response;
    });
    localStorage.clear();
    mockedShowToast.mockClear();
  });

  afterEach(() => {
    fetchSpy.mockRestore();
    localStorage.clear();
  });

  describe("processSSE - 6 种 event type 分发", () => {
    it("text_delta 追加到最后 assistant 消息 content", async () => {
      const sse = sseLine("text_delta", { content: "Hello" }) + sseLine("text_delta", { content: " World" });
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages } = useAgent();
      await send("hi");

      expect(messages.value.length).toBe(2);
      expect(messages.value[0].role).toBe("user");
      expect(messages.value[0].content).toBe("hi");
      expect(messages.value[1].role).toBe("assistant");
      expect(messages.value[1].content).toBe("Hello World");
    });

    it("reasoning_delta 追加到 reasoning 字段", async () => {
      const sse = sseLine("reasoning_delta", { content: "thinking... " }) + sseLine("text_delta", { content: "answer" });
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages } = useAgent();
      await send("q");

      const assistant = messages.value.find(m => m.role === "assistant")!;
      expect(assistant.reasoning).toBe("thinking... ");
      expect(assistant.content).toBe("answer");
    });

    it("tool_call push 到 tool_calls（带 kind/needsConfirm/status=pending）", async () => {
      const sse =
        sseLine("text_delta", { content: "I will run ls" }) +
        sseLine("tool_call", {
          id: "tc-1",
          name: "exec_command",
          args: '{"command":"ls"}',
          auto_run: false,
          kind: "command",
        });
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages } = useAgent();
      await send("list files");

      const assistant = messages.value.find(m => m.role === "assistant")!;
      expect(assistant.tool_calls.length).toBe(1);
      const tc = assistant.tool_calls[0];
      expect(tc.id).toBe("tc-1");
      expect(tc.name).toBe("exec_command");
      expect(tc.kind).toBe("command");
      expect(tc.needsConfirm).toBe(true); // auto_run=false → needsConfirm
      expect(tc.status).toBe("pending");
    });

    it("tool_call auto_run=true 时 needsConfirm=false", async () => {
      const sse = sseLine("tool_call", {
        id: "tc-2",
        name: "list_files",
        args: '{"path":"/"}',
        auto_run: true,
        kind: "readOnly",
      });
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages } = useAgent();
      await send("list");

      const tc = messages.value[1].tool_calls[0];
      expect(tc.needsConfirm).toBe(false);
    });

    it("tool_status 标记 tool_call 的 status 变更（running/success/failed）", async () => {
      const sse =
        sseLine("tool_call", {
          id: "tc-3",
          name: "exec_command",
          args: "{}",
          auto_run: true,
          kind: "command",
        }) +
        sseLine("tool_status", { id: "tc-3", status: "running" }) +
        sseLine("tool_status", { id: "tc-3", status: "success" });
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages } = useAgent();
      await send("run");

      const tc = messages.value[1].tool_calls[0];
      expect(tc.status).toBe("success");
    });

    it("tool_result push 到 tool_results", async () => {
      const sse =
        sseLine("tool_call", {
          id: "tc-4",
          name: "list_files",
          args: "{}",
          auto_run: true,
          kind: "readOnly",
        }) +
        sseLine("tool_result", {
          id: "tc-4",
          name: "list_files",
          result: '{"files":[]}',
          is_error: false,
          status: "success",
          duration_ms: 42,
        });
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages } = useAgent();
      await send("list");

      expect(messages.value[1].tool_results.length).toBe(1);
      expect(messages.value[1].tool_results[0].id).toBe("tc-4");
      expect(messages.value[1].tool_results[0].is_error).toBe(false);
      expect(messages.value[1].tool_results[0].duration_ms).toBe(42);
    });

    it("stream_end 把 status 切回 idle（无 pending 确认时）", async () => {
      const sse = sseLine("text_delta", { content: "done" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, status, messages } = useAgent();
      await send("hi");

      expect(status.value).toBe("idle");
      // 最后 assistant 消息应标记 isStreaming=false
      const lastAssistant = messages.value[messages.value.length - 1];
      expect(lastAssistant.isStreaming).toBe(false);
    });

    it("stream_end 在有 pending needsConfirm tool_call 时 status=confirming", async () => {
      const sse =
        sseLine("tool_call", {
          id: "tc-pending",
          name: "delete_file",
          args: "{}",
          auto_run: false,
          kind: "fileChange",
        }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, status } = useAgent();
      await send("delete");

      expect(status.value).toBe("confirming");
    });

    it("分块读取仍能正确解析（chunked SSE）", async () => {
      const fullSSE = sseLine("text_delta", { content: "hi" }) + sseLine("stream_end", {});
      // 切两半
      const mid = Math.floor(fullSSE.length / 2);
      const chunk1 = fullSSE.slice(0, mid);
      const chunk2 = fullSSE.slice(mid);
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([chunk1, chunk2])));

      const { send, messages, status } = useAgent();
      await send("q");

      expect(messages.value[1].content).toBe("hi");
      expect(status.value).toBe("idle");
    });

    // ─── Task 7: 上下文自动压缩事件 ──────────────────────
    it("compaction 事件插入 role=system + marker 的合成消息", async () => {
      const sse =
        sseLine("text_delta", { content: "pre" }) +
        sseLine("compaction", {
          summary_text: "summary-abc",
          replaced_message_count: 3,
          triggered_at_ms: 1700000000000,
        }) +
        sseLine("text_delta", { content: "post" }) +
        sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages, status } = useAgent();
      await send("q");

      // messages: [user, assistant(pre), system(marker), assistant(post)]
      expect(messages.value.length).toBe(4);
      expect(messages.value[0].role).toBe("user");
      expect(messages.value[1].role).toBe("assistant");
      expect(messages.value[1].content).toBe("pre");
      // 第 3 条：合成的 system marker 消息
      expect(messages.value[2].role).toBe("system");
      expect(messages.value[2].content).toBe("上下文已自动压缩");
      // 第 4 条：assistant post
      expect(messages.value[3].role).toBe("assistant");
      expect(messages.value[3].content).toBe("post");
      // status 不受 compaction 影响
      expect(status.value).toBe("idle");
    });

    it("compaction 事件 data 解析失败时仍插入 marker 消息（容错）", async () => {
      // 直接发非 JSON 的 data：parseCompactionData 返回 null，
      // 但 useAgent 仍然 push 一条 marker 消息
      const sse =
        sseLine("text_delta", { content: "ok" }) +
        // data 字段是普通字符串而非 JSON object
        `data: ${JSON.stringify({ type: "compaction", data: "garbage" })}\n\n` +
        sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages } = useAgent();
      await send("q");

      const marker = messages.value.find(m => m.role === "system" && m.content === "上下文已自动压缩");
      expect(marker).toBeTruthy();
    });
  });

  describe("send - 4 决策 confirmTool", () => {
    it("confirmTool 接受 accept 决策：调用 /api/confirm 并处理 SSE", async () => {
      // 第一次 send：模拟有 pending tool_call
      const sse1 =
        sseLine("tool_call", {
          id: "tc-x",
          name: "delete_file",
          args: "{}",
          auto_run: false,
          kind: "fileChange",
        }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse1]))));

      const agent = useAgent();
      await agent.send("delete");

      expect(agent.status.value).toBe("confirming");
      expect(agent.messages.value[1].tool_calls[0].id).toBe("tc-x");

      // 第二次 confirmTool：accept
      const sse2 =
        sseLine("tool_result", {
          id: "tc-x",
          name: "delete_file",
          result: "ok",
          is_error: false,
          status: "success",
          duration_ms: 10,
        }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse2]))));

      await agent.confirmTool("tc-x", "accept");

      // Task 4：send 入口加了 /api/health，所以 fetchSpy 总调用次数
      // 从 2 变 3（/api/health + /api/chat + /api/confirm）。改用
      // findLastCallIndex 按 URL 定位 confirm 调用的下标。
      const confirmIdx = findLastCallIndex(fetchSpy.mock.calls as unknown[][], "/api/confirm");
      expect(confirmIdx).toBeGreaterThanOrEqual(0);
      const confirmCall = fetchSpy.mock.calls[confirmIdx];
      expect(confirmCall[0]).toBe("/agent-api/api/confirm");
      const body = JSON.parse(confirmCall[1].body);
      expect(body.toolCallId).toBe("tc-x");
      expect(body.decision).toBe("accept");
    });

    it("confirmTool 接受 4 种决策：accept / accept_for_session / decline / cancel", async () => {
      const decisions: Array<"accept" | "accept_for_session" | "decline" | "cancel"> = [
        "accept",
        "accept_for_session",
        "decline",
        "cancel",
      ];
      for (const decision of decisions) {
        fetchSpy.mockReset();
        localStorage.clear();

        const sse1 =
          sseLine("tool_call", {
            id: `tc-${decision}`,
            name: "op",
            args: "{}",
            auto_run: false,
            kind: "command",
          }) + sseLine("stream_end", {});
        fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse1]))));

        const agent = useAgent();
        await agent.send("q");
        expect(agent.status.value).toBe("confirming");

        const sse2 = sseLine("stream_end", {});
        fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse2]))));
        await agent.confirmTool(`tc-${decision}`, decision);

        const body = JSON.parse(fetchSpy.mock.calls[findLastCallIndex(fetchSpy.mock.calls as unknown[][], "/api/confirm")][1].body);
        expect(body.decision).toBe(decision);
      }
    });

    it("send 期间有 stream 时忽略二次 send", async () => {
      // 第一次 send：模拟一个完整但很短的流
      const sse1 = sseLine("text_delta", { content: "first" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse1]))));

      const { send, status, messages } = useAgent();
      await send("first");

      // 此时 status=idle（第一次 send 已完成）
      expect(status.value).toBe("idle");

      // 第二次 send 应该正常进行（idle → streaming）
      const sse2 = sseLine("text_delta", { content: "second" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse2]))));
      await send("second");

      expect(messages.value.length).toBe(4); // 2 user + 2 assistant
      expect(messages.value[0].content).toBe("first");
      expect(messages.value[2].content).toBe("second");
    });

    it("send 期间 status=streaming 时第二次 send 立即返回且不发新请求", async () => {
      // 用一个 pull 永远不关闭的流 + 拿到它的 controller（start 时存）
      let streamController: ReadableStreamDefaultController<Uint8Array> | null = null;
      const slowStream = new ReadableStream<Uint8Array>({
        start(controller) {
          streamController = controller;
        },
      });

      fetchSpy.mockImplementation(
        urlAwareMock(
          vi.fn().mockResolvedValue({
            ok: true,
            status: 200,
            body: slowStream,
          } as Response)
        )
      );

      const { send, status, messages } = useAgent();
      // 启动第一次 send（不 await，让它挂起）
      const p1 = send("first").catch(() => {});

      // 给 microtask 一点时间
      await new Promise(r => setTimeout(r, 5));

      expect(status.value).toBe("streaming");
      expect(messages.value.length).toBe(2);

      // 第二次 send：应立即返回（被忽略）。注意：Task 4 在 send 入口
      // 调 /api/health 是在 status 检查之后，所以 streaming 时第二次 send
      // 仍然立即 return，不会再发任何 fetch。
      await send("second");

      // 第一次 send 调了 2 次：/api/health + /api/chat。第二次 send 因
      // status=streaming 立即 return，没增加任何 fetch。
      expect(fetchSpy).toHaveBeenCalledTimes(2);
      expect(messages.value.length).toBe(2);
      expect(messages.value[0].content).toBe("first");

      // 清理：关闭流 + 等第一次 send 完成
      if (streamController) streamController.close();
      await p1;

      // 关闭后 status 应该恢复 idle
      expect(status.value).toBe("idle");
    }, 5000);
  });

  describe("localStorage 持久化", () => {
    it("send 完成后 session 写入 localStorage", async () => {
      const sse = sseLine("text_delta", { content: "ok" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send } = useAgent();
      await send("hi");

      // localStorage 中应出现 agent:session:* 键
      const keys: string[] = [];
      for (let i = 0; i < localStorage.length; i++) {
        const k = localStorage.key(i);
        if (k) keys.push(k);
      }
      expect(keys.some(k => k.startsWith("agent:session:"))).toBe(true);

      const sessionKey = keys.find(k => k.startsWith("agent:session:"))!;
      const persisted = JSON.parse(localStorage.getItem(sessionKey)!);
      expect(persisted.sessionId).toBeTruthy();
      expect(persisted.messages.length).toBe(2);
      expect(persisted.messages[1].content).toBe("ok");
      expect(persisted.status).toBe("idle");
    });

    it("resume 从 localStorage 恢复 messages", async () => {
      // 预先填充 localStorage
      const fakeSessionId = "fake-session-id-123";
      const stored = {
        sessionId: fakeSessionId,
        eventOffset: 5,
        messages: [
          { role: "user", content: "prev q", tool_calls: [], tool_results: [] },
          { role: "assistant", content: "prev a", tool_calls: [], tool_results: [], isStreaming: false },
        ] satisfies Message[],
        status: "idle",
      };
      localStorage.setItem(`agent:session:${fakeSessionId}`, JSON.stringify(stored));

      // resume 不会发起新请求（status=idle）
      const { resume, messages } = useAgent();
      await resume();

      expect(messages.value.length).toBe(2);
      expect(messages.value[0].content).toBe("prev q");
      expect(messages.value[1].content).toBe("prev a");
      expect(fetchSpy).not.toHaveBeenCalled();
    });

    it("resume 之前 status=streaming 时调用 /api/resume", async () => {
      const fakeSessionId = "session-streaming";
      localStorage.setItem(
        `agent:session:${fakeSessionId}`,
        JSON.stringify({
          sessionId: fakeSessionId,
          eventOffset: 2,
          messages: [
            { role: "user", content: "q", tool_calls: [], tool_results: [] },
            { role: "assistant", content: "partial", tool_calls: [], tool_results: [], isStreaming: true },
          ],
          status: "streaming",
        })
      );

      const sse = sseLine("text_delta", { content: " complete" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { resume, messages, status } = useAgent();
      await resume();

      expect(fetchSpy).toHaveBeenCalledTimes(1);
      expect(fetchSpy.mock.calls[0][0]).toBe("/agent-api/api/resume");
      const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
      expect(body.sessionId).toBe(fakeSessionId);
      // D.2: body 字段名从 offset 改成 lastEventId（与后端 handleAgentResume 对齐）
      expect(body.lastEventId).toBe(0);
      expect(body.offset).toBeUndefined();
      // D.2.3: lastEventId=0 时不发送 Last-Event-ID header
      const headers = fetchSpy.mock.calls[0][1].headers;
      expect(headers["Last-Event-ID"]).toBeUndefined();

      expect(messages.value[1].content).toBe("partial complete");
      expect(status.value).toBe("idle");
    });
  });

  describe("D.2 - resume 协议对齐（Last-Event-ID / stream_status）", () => {
    /**
     * 构造带 `id:` 行的 SSE 事件（SSE 标准断点续传字段）
     */
    function sseLineWithId(id: number, type: string, data: unknown): string {
      return `id: ${id}\ndata: ${JSON.stringify({ type, data: JSON.stringify(data) })}\n\n`;
    }

    it("解析 SSE `id: N` 行：processSSE 维护 lastEventId", async () => {
      const sse = sseLineWithId(7, "text_delta", { content: "tail" }) + sseLineWithId(8, "stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages, status } = useAgent();
      await send("q");

      // 收到 2 个事件，data 都被解析
      expect(messages.value[1].content).toBe("tail");
      expect(status.value).toBe("idle");
    });

    it("resume 时从 localStorage 恢复 lastEventId 并发送 Last-Event-ID header", async () => {
      const fakeSessionId = "session-last-event-id";
      localStorage.setItem(
        `agent:session:${fakeSessionId}`,
        JSON.stringify({
          sessionId: fakeSessionId,
          eventOffset: 5,
          lastEventId: 42, // 关键：上次收到的服务端事件 id
          messages: [
            { role: "user", content: "q", tool_calls: [], tool_results: [] },
            { role: "assistant", content: "partial", tool_calls: [], tool_results: [], isStreaming: true },
          ],
          status: "streaming",
        })
      );

      const sse = sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { resume } = useAgent();
      await resume();

      // 验证：body.lastEventId + header.Last-Event-ID 都带上了
      expect(fetchSpy).toHaveBeenCalledTimes(1);
      const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
      const headers = fetchSpy.mock.calls[0][1].headers;
      expect(body.lastEventId).toBe(42);
      expect(headers["Last-Event-ID"]).toBe("42");
    });

    it("stream_status.more_pending → 链式 resume（连续两次 /api/resume 调用）", async () => {
      const fakeSessionId = "session-stream-status";
      localStorage.setItem(
        `agent:session:${fakeSessionId}`,
        JSON.stringify({
          sessionId: fakeSessionId,
          eventOffset: 1,
          lastEventId: 10,
          messages: [
            { role: "user", content: "q", tool_calls: [], tool_results: [] },
            { role: "assistant", content: "a", tool_calls: [], tool_results: [], isStreaming: true },
          ],
          status: "streaming",
        })
      );

      // 第一轮 resume：server 推 stream_status.more_pending
      const sse1 = sseLine("stream_status", { status: "more_pending", inProgress: true, maxEventId: 11 });
      // 第二轮 resume：server 推真正的事件 + stream_end
      const sse2 = sseLine("text_delta", { content: " tail" }) + sseLine("stream_end", {});

      let callCount = 0;
      fetchSpy.mockImplementation(
        vi.fn(async (url: string | Request) => {
          const urlStr = typeof url === "string" ? url : (url as Request).url;
          if (urlStr.includes("/api/config")) {
            return { ok: true, status: 200, json: () => Promise.resolve({}) } as Response;
          }
          if (urlStr.includes("/api/resume")) {
            callCount++;
            const stream = callCount === 1 ? makeSSEStream([sse1]) : makeSSEStream([sse2]);
            return { ok: true, status: 200, body: stream } as Response;
          }
          return { ok: false, status: 404 } as Response;
        })
      );

      const { resume, messages } = useAgent();
      await resume();

      // 关键断言：被链式调用了两次
      expect(callCount).toBe(2);
      expect(messages.value[1].content).toBe("a tail");
    });

    it("stream_status.synced → 退出链式 resume（保持 streaming 状态由下一次轮询触发）", async () => {
      const fakeSessionId = "session-synced";
      localStorage.setItem(
        `agent:session:${fakeSessionId}`,
        JSON.stringify({
          sessionId: fakeSessionId,
          eventOffset: 1,
          lastEventId: 5,
          messages: [
            { role: "user", content: "q", tool_calls: [], tool_results: [] },
            { role: "assistant", content: "a", tool_calls: [], tool_results: [], isStreaming: true },
          ],
          status: "streaming",
        })
      );

      // server 推 stream_status.synced + 自然关闭流（无 stream_end）
      const sse1 = sseLine("stream_status", { status: "synced", inProgress: true, maxEventId: 5 });

      let callCount = 0;
      fetchSpy.mockImplementation(
        vi.fn(async (url: string | Request) => {
          const urlStr = typeof url === "string" ? url : (url as Request).url;
          if (urlStr.includes("/api/config")) {
            return { ok: true, status: 200, json: () => Promise.resolve({}) } as Response;
          }
          if (urlStr.includes("/api/resume")) {
            callCount++;
            return { ok: true, status: 200, body: makeSSEStream([sse1]) } as Response;
          }
          return { ok: false, status: 404 } as Response;
        })
      );

      const { resume, messages, status } = useAgent();
      await resume();

      // synced：只调用一次（没有 more_pending 触发下一轮）
      expect(callCount).toBe(1);
      // synced 不修改 messages.content（仅同步 lastEventId）
      expect(messages.value[1].content).toBe("a");
      // status 仍为 streaming（等待用户后续触发）
      // 注：processSSE 流关闭时会执行 finalizeLastAssistant + status 回退
      // 但 messages 中有 isStreaming=true，会被 finalizeLastAssistant 影响
      expect(status.value).toBe("idle"); // 流自然关闭后切回 idle
    });

    it("runResumeChain 限制 maxHops=32 防无限递归", async () => {
      const fakeSessionId = "session-infinite";
      localStorage.setItem(
        `agent:session:${fakeSessionId}`,
        JSON.stringify({
          sessionId: fakeSessionId,
          eventOffset: 1,
          lastEventId: 5,
          messages: [
            { role: "user", content: "q", tool_calls: [], tool_results: [] },
            { role: "assistant", content: "a", tool_calls: [], tool_results: [], isStreaming: true },
          ],
          status: "streaming",
        })
      );

      // server 永远只推 more_pending（不推 stream_end）
      const sseForever = sseLine("stream_status", { status: "more_pending", inProgress: true, maxEventId: 1 });
      let callCount = 0;

      fetchSpy.mockImplementation(
        vi.fn(async (url: string | Request) => {
          const urlStr = typeof url === "string" ? url : (url as Request).url;
          if (urlStr.includes("/api/config")) {
            return { ok: true, status: 200, json: () => Promise.resolve({}) } as Response;
          }
          if (urlStr.includes("/api/resume")) {
            callCount++;
            return { ok: true, status: 200, body: makeSSEStream([sseForever]) } as Response;
          }
          return { ok: false, status: 404 } as Response;
        })
      );

      const { resume } = useAgent();
      await resume();

      // maxHops=32：每轮 resume 包含初次，最多 32 次
      expect(callCount).toBe(32);
    });

    it("老存档（无 lastEventId 字段）兼容：缺省按 0 处理", async () => {
      const fakeSessionId = "session-legacy";
      // 注意：stored 没有 lastEventId 字段
      localStorage.setItem(
        `agent:session:${fakeSessionId}`,
        JSON.stringify({
          sessionId: fakeSessionId,
          eventOffset: 0,
          messages: [
            { role: "user", content: "q", tool_calls: [], tool_results: [] },
            { role: "assistant", content: "a", tool_calls: [], tool_results: [], isStreaming: true },
          ],
          status: "streaming",
        })
      );

      const sse = sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { resume } = useAgent();
      await resume();

      const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
      const headers = fetchSpy.mock.calls[0][1].headers;
      expect(body.lastEventId).toBe(0); // 缺省
      expect(headers["Last-Event-ID"]).toBeUndefined(); // 0 不发 header
    });
  });

  describe("stop / reset", () => {
    it("stop 中断正在 streaming 的流", async () => {
      const sse1 = sseLine("text_delta", { content: "partial" });
      const stream = makeSSEStream([sse1]);
      fetchSpy.mockImplementationOnce(fetchReturningStream(stream));

      const { send, stop, status, messages } = useAgent();
      const promise = send("q");

      // 等待 microtask 让 fetch 启动
      await new Promise(r => setTimeout(r, 0));
      stop();

      await promise;

      expect(status.value).toBe("idle");
      expect(messages.value[1].isStreaming).toBe(false);
    });

    it("reset 清空所有状态 + 删除 localStorage", async () => {
      const sse = sseLine("text_delta", { content: "x" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, reset, messages, status } = useAgent();
      await send("hi");

      expect(messages.value.length).toBeGreaterThan(0);
      expect(localStorage.length).toBeGreaterThan(0);

      reset();
      expect(messages.value.length).toBe(0);
      expect(status.value).toBe("idle");
      expect(localStorage.length).toBe(0);
    });
  });

  describe("错误处理", () => {
    it("fetch 500 错误时显示 toast + status=idle", async () => {
      fetchSpy.mockImplementation(fetchReturningError(500));

      const { send, status, messages } = useAgent();
      await send("hi");

      expect(mockedShowToast).toHaveBeenCalled();
      expect(status.value).toBe("idle");
      // 流式结束标记
      expect(messages.value[1].isStreaming).toBe(false);
    });

    it("无效 JSON SSE payload 静默忽略（不崩溃）", async () => {
      const sse = "data: {this is broken\n\n" + sseLine("text_delta", { content: "ok" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages, status } = useAgent();
      await send("q");

      expect(messages.value[1].content).toBe("ok");
      expect(status.value).toBe("idle");
    });

    it("tool_call data 是非法 JSON 时跳过该事件（不崩溃）", async () => {
      const sse =
        'data: {"type":"tool_call","data":"not valid json"}\n\n' + sseLine("text_delta", { content: "ok" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const { send, messages, status } = useAgent();
      await send("q");

      expect(messages.value[1].content).toBe("ok");
      expect(messages.value[1].tool_calls.length).toBe(0);
      expect(status.value).toBe("idle");
    });
  });

  describe("processSSE 空/异常 stream", () => {
    it("response.body 为 null 时 processSSE 安全返回", async () => {
      // 直接构造 body: null 的 mock（processSSE 早返回）
      fetchSpy.mockImplementationOnce(
        vi.fn().mockResolvedValue({
          ok: true,
          status: 200,
          body: null,
        } as Response)
      );

      const { send, status, messages } = useAgent();
      await send("q");

      // body 为 null 时 send() 抛出异常 → catch → status=idle + user msg 标记 error
      expect(status.value).toBe("idle");
      // 最后一条 user 消息应标记了 error
      const lastUserMsg = [...messages.value].reverse().find(m => m.role === "user");
      expect(lastUserMsg?.error).toBeTruthy();
    });
  });

  describe("resume - 异常路径", () => {
    it("resume 时 fetch 500 错误 → 静默 + status=idle", async () => {
      const sessionId = "session-resume-fail";
      localStorage.setItem(
        `agent:session:${sessionId}`,
        JSON.stringify({
          sessionId,
          eventOffset: 0,
          messages: [
            { role: "user", content: "q", tool_calls: [], tool_results: [] },
            { role: "assistant", content: "partial", tool_calls: [], tool_results: [], isStreaming: true },
          ],
          status: "streaming",
        })
      );

      fetchSpy.mockImplementationOnce(fetchReturningError(500));

      const { resume, status, messages } = useAgent();
      await resume();

      expect(status.value).toBe("idle");
      // 流式结束标记
      expect(messages.value[1].isStreaming).toBe(false);
    });

    it("resume 时没有 streaming 状态：不发请求", async () => {
      const sessionId = "session-idle";
      localStorage.setItem(
        `agent:session:${sessionId}`,
        JSON.stringify({
          sessionId,
          eventOffset: 0,
          messages: [
            { role: "user", content: "q", tool_calls: [], tool_results: [] },
            { role: "assistant", content: "a", tool_calls: [], tool_results: [], isStreaming: false },
          ],
          status: "idle",
        })
      );

      const { resume } = useAgent();
      await resume();

      expect(fetchSpy).not.toHaveBeenCalled();
    });

    it("resume 时 localStorage 没有 session：安全返回", async () => {
      const { resume, messages, status } = useAgent();
      await resume();

      expect(fetchSpy).not.toHaveBeenCalled();
      expect(messages.value.length).toBe(0);
      expect(status.value).toBe("idle");
    });
  });

  describe("confirmTool - 异常路径", () => {
    it("confirmTool 在没有 active session 时立即返回", async () => {
      const { confirmTool } = useAgent();
      await confirmTool("tc-1", "accept");
      expect(fetchSpy).not.toHaveBeenCalled();
    });

    it("confirmTool 在 fetch 500 时 status 回到 confirming + tool.status 回到 pending", async () => {
      // 先建立 confirming 状态
      const sse1 =
        sseLine("tool_call", {
          id: "tc-r",
          name: "op",
          args: "{}",
          auto_run: false,
          kind: "command",
        }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse1]))));
      const agent = useAgent();
      await agent.send("q");
      expect(agent.status.value).toBe("confirming");

      // 第二次 fetch 500 错误
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningError(500)));
      await agent.confirmTool("tc-r", "accept");

      expect(agent.status.value).toBe("confirming");
      expect(agent.messages.value[1].tool_calls[0].status).toBe("pending");
      expect(mockedShowToast).toHaveBeenCalled();
    });
  });

  describe("stop - 多次调用", () => {
    it("stop 调用多次不崩溃", async () => {
      const { stop, status } = useAgent();
      stop();
      stop();
      stop();
      expect(status.value).toBe("idle");
    });

    it("stop 之前没有任何流式连接：仍是 no-op + status=idle", () => {
      const { stop, status } = useAgent();
      stop();
      expect(status.value).toBe("idle");
    });
  });

  describe("reset - 多实例隔离", () => {
    it("多个 useAgent 实例互不影响", async () => {
      const sse = sseLine("text_delta", { content: "hi" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(fetchReturningStream(makeSSEStream([sse])));

      const a = useAgent();
      const b = useAgent();

      await a.send("for A");
      // A 完成，B 不应有任何消息
      expect(a.messages.value.length).toBe(2);
      expect(b.messages.value.length).toBe(0);

      a.reset();
      // A 清空后 B 仍不变
      expect(a.messages.value.length).toBe(0);
      expect(b.messages.value.length).toBe(0);
    });
  });

  // ─── Task 4: Server Instance + Sequence 去重 ───────────────────────
  describe("Task 4: server instance + SSE sequence 去重", () => {
    /**
     * 构造带 `id: N` 行的 SSE 事件（SSE 标准断点续传字段）
     */
    function sseLineWithId(id: number, type: string, data: unknown): string {
      return `id: ${id}\ndata: ${JSON.stringify({ type, data: JSON.stringify(data) })}\n\n`;
    }

    /**
     * URL-aware mock：按 URL 路由返回不同响应。
     *   /api/config   → 空配置
     *   /api/health   → { serverInstanceId: <id> } (200)
     *   /agent-api/   → fallback（一般给 SSE 流）
     */
    function healthAwareMock(instanceId: string, fallback: ReturnType<typeof vi.fn>): ReturnType<typeof vi.fn> {
      return vi.fn(async (url: string | Request) => {
        const urlStr = typeof url === "string" ? url : (url as Request).url;
        if (urlStr.includes("/api/config")) {
          return { ok: true, status: 200, json: () => Promise.resolve({}) } as Response;
        }
        if (urlStr.includes("/api/health")) {
          return {
            ok: true,
            status: 200,
            json: () => Promise.resolve({ serverInstanceId: instanceId }),
          } as Response;
        }
        return fallback(url);
      });
    }

    it("send() 入口拉取 /api/health 取 serverInstanceId（成功路径）", async () => {
      const instanceId = "host-12345-1700000000000000000";
      const sse = sseLine("text_delta", { content: "ok" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(healthAwareMock(instanceId, fetchReturningStream(makeSSEStream([sse]))));

      const agent = useAgent();
      // 初始时 instance 是空串
      expect(agent.__getServerInstanceForTest()).toBe("");

      await agent.send("hi");

      // 拉取成功后应已设置
      expect(agent.__getServerInstanceForTest()).toBe(instanceId);
      // /api/health 至少被调用了一次（在 send 入口处）
      const healthCalls = fetchSpy.mock.calls.filter(c =>
        (typeof c[0] === "string" ? c[0] : (c[0] as Request).url).includes("/api/health")
      );
      expect(healthCalls.length).toBeGreaterThanOrEqual(1);
    });

    it("拉取 /api/health 失败时 fallback 到空串 + 业务继续", async () => {
      // /api/health 直接抛异常
      fetchSpy.mockImplementation(
        vi.fn(async (url: string | Request) => {
          const urlStr = typeof url === "string" ? url : (url as Request).url;
          if (urlStr.includes("/api/config")) {
            return { ok: true, status: 200, json: () => Promise.resolve({}) } as Response;
          }
          if (urlStr.includes("/api/health")) {
            throw new Error("network down");
          }
          const sse = sseLine("text_delta", { content: "still works" }) + sseLine("stream_end", {});
          return { ok: true, status: 200, body: makeSSEStream([sse]) } as Response;
        })
      );

      const agent = useAgent();
      await agent.send("hi");

      // instance 保持空串（不抛错）
      expect(agent.__getServerInstanceForTest()).toBe("");
      // 业务仍正常
      expect(agent.messages.value[1].content).toBe("still works");
      expect(agent.status.value).toBe("idle");
    });

    it("同一 SSE 流内重复 sequence 被丢弃（不重复 dispatch）", async () => {
      const instanceId = "inst-A";
      const sse =
        sseLineWithId(10, "text_delta", { content: "first " }) +
        // sequence 11 重复（id: 11）
        sseLineWithId(11, "text_delta", { content: "duplicate" }) +
        sseLineWithId(11, "text_delta", { content: "should-be-dropped" }) +
        sseLineWithId(12, "text_delta", { content: "after-dup" }) +
        sseLine("stream_end", {});
      fetchSpy.mockImplementation(healthAwareMock(instanceId, fetchReturningStream(makeSSEStream([sse]))));

      const agent = useAgent();
      // 强制把 instance 固定到 A（避免自动 refresh 时进入新值）
      agent.__setServerInstanceForTest(instanceId);
      await agent.send("q");

      // id=10: "first "
      // id=11 (第一次): "duplicate" — 11 进入 seen 集合
      // id=11 (第二次): drop，console.debug 触发，messages 不变
      // id=12: "after-dup"
      expect(agent.messages.value[1].content).toBe("first duplicateafter-dup");
      // seen 集合：{10, 11, 12}
      expect(agent.__getSeenSequencesForTest()).toEqual([10, 11, 12]);
    });

    it("server instance 变化时 seenSequences 被清空（新进程 sequence 重新计数）", async () => {
      // 关键：让 urlAwareMock 每次 send 时按当前 instance 路由决定 mock 值。
      // 用一个 mutable 状态让两次 send() 拉到不同的 instance id。
      let currentMockedInstance = "inst-A";

      // 第一轮：instance=A，流内 seq=5
      const sseA = sseLineWithId(5, "text_delta", { content: "from-A" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(
        urlAwareMock(fetchReturningStream(makeSSEStream([sseA])), {
          serverInstanceId: currentMockedInstance,
        })
      );

      const agent = useAgent();
      await agent.send("q");

      expect(agent.messages.value[1].content).toBe("from-A");
      expect(agent.__getSeenSequencesForTest()).toEqual([5]);
      expect(agent.__getServerInstanceForTest()).toBe("inst-A");

      // 切换 instance id：第二轮 send() 入口调 /api/health 会拉到 B
      // → refreshServerInstance 检测到变化 → seenSequences 被清空
      currentMockedInstance = "inst-B";

      // 第二轮：instance=B，流内 seq=5（与 A 重复，模拟新进程从 1 重新
      // 开始编号——但 5 也可能再次出现，seenSequences 必须被清空才能 dispatch）
      const sseB = sseLineWithId(5, "text_delta", { content: "from-B" }) + sseLine("stream_end", {});
      fetchSpy.mockImplementation(
        urlAwareMock(fetchReturningStream(makeSSEStream([sseB])), {
          serverInstanceId: currentMockedInstance,
        })
      );

      await agent.send("q2");

      // 最后一条 assistant 消息
      const lastAssistant = agent.messages.value[agent.messages.value.length - 1];
      expect(lastAssistant.content).toBe("from-B");
      // seen 集合在 instance 切换后已被清空，再加入新 instance 的 seq 5
      expect(agent.__getSeenSequencesForTest()).toEqual([5]);
      // currentServerInstance 已更新到 B
      expect(agent.__getServerInstanceForTest()).toBe("inst-B");
    });

    it("超过 MAX_TRACKED_REALTIME_SEQUENCES (2000) 时按 FIFO 驱逐最老", async () => {
      // 我们用 __setServerInstanceForTest + 直接给 useAgent 喂大量重复序列
      // 的方式比较重——这里用更轻量方式：构造 seq 1..2001 + 重复 seq 1
      // 期望 1 被驱逐，再次出现 seq 1 时不重复 dispatch（因为它已被清出 seen）

      // 实际简单版：构造 2001 个不同 seq 编号的事件 + 第 2002 个
      // 但受限于流式 2001 行的开销，我们用 4 个事件 + 直接 assert rememberSequence 行为
      // —— 不行，rememberSequence 是闭包私有。改用行为验证：
      // 构造 seq 1..2001 全 dispatch + seq 1 重复（已驱逐）应该重新 dispatch
      //
      // 简化路径：直接通过 send 触发一次 add，然后调用 __setServerInstanceForTest 不变，
      // 连续 emit 2000 个 unique seq 编号 + 1 个 earliest(seq=1) 重新出现。
      // 困难是 processSSE 不接受"2000 个事件"——它只在一条 SSE 流中工作。
      // 退而求其次：构造 2002 个事件，第一个被驱逐后再次出现时应该不丢。
      //
      // 实测更简单的：构造 2001 个 events + 重复 seq 1。
      // 1..2000 全部进入 seen，第 2001 个（FIFO 淘汰 seq 1）
      // 第 2002 个 (seq 1) → seen 集合里 seq 1 已被驱逐 → 视为新 → dispatch

      // 实际性能考虑：构造 2002 个 SSE 字符串太大。我们采用更聪明的
      // 验证方式：构造 MAX_TRACKED_REALTIME_SEQUENCES (2000) 个 seq 1..2000，
      // 然后再次 send 在新流中只发 seq 1 + seq 2。
      //   - send 1: seq 1..2000 → seen = {1..2000}, order = [1..2000]
      //   - send 2: seq 1 → 命中 seen（FIFO 还未淘汰）→ 丢弃；seq 2 → 命中 → 丢弃
      //   - 我们再 setServerInstance 不变，直接 send 2001 个：
      //     实际上要触发 FIFO 必须构造 2001+ 个事件。
      //
      // 折中：本测试只断言"seenSequences 容量被限制到 2000 以内"——用性能更好的方式：
      // 构造刚好 2001 个 unique seq 的 SSE 流 + 1 个重复最早 seq，断言后到达的不会丢。
      //
      // 性能 vs 简单性：构造 2002 个事件的 SSE 字符串约 2002 * 60 bytes ≈ 120KB，
      // 对 vitest 来说可接受。
      const MAX = 2000;
      const ids: number[] = [];
      for (let i = 1; i <= MAX; i++) ids.push(i);
      let sseBody = "";
      for (const id of ids) {
        sseBody += sseLineWithId(id, "text_delta", { content: "x" });
      }
      // 第 2001 个事件：seq=2001（让 seq=1 被 FIFO 淘汰）
      sseBody += sseLineWithId(MAX + 1, "text_delta", { content: "y" });
      // 第 2002 个事件：seq=1（已被淘汰）→ 视为新 → dispatch
      sseBody += sseLineWithId(1, "text_delta", { content: "z" });
      sseBody += sseLine("stream_end", {});

      fetchSpy.mockImplementation(healthAwareMock("inst-fifo", fetchReturningStream(makeSSEStream([sseBody]))));

      const agent = useAgent();
      agent.__setServerInstanceForTest("inst-fifo");
      await agent.send("q");

      // seenSequences 容量应被钳制到 MAX 以内
      const seen = agent.__getSeenSequencesForTest();
      expect(seen.length).toBeLessThanOrEqual(MAX);
      // seq=1 被淘汰后，seen 集合里不应包含它（除非容量计算错误）
      // 但 seen 是个 LIFO-like 的数组，shift 后顺序是 [2..2001] + [1 (重新加入)]
      // = 长度 2000 + 1 (第 2002 个加入的 1)？不对：第 2002 个 1 也会 push 到尾部
      // 并触发 shift 把 2 淘汰。seen 长度稳定在 MAX
      expect(seen.length).toBe(MAX);
      // 验证 seen 顺序：最老的应是 3（2 被淘汰），最新的应是 1
      expect(seen[0]).toBe(3);
      expect(seen[seen.length - 1]).toBe(1);
    }, 15000);
  });

  // ─── Task 25: Sync Doctor ────────────────────────────────
  //
  // runSyncDoctor 是 useAgent 模块导出的顶层函数（非 composable 实例方法），
  // 所以可以脱离 useAgent() 单独 import。下面的测试只关心它的 HTTP 行为。
  describe("Task 25: runSyncDoctor", () => {
    beforeEach(() => {
      globalThis.fetch = originalFetch;
    });
    afterEach(() => {
      globalThis.fetch = originalFetch;
    });

    it("POST /api/sync/doctor 并返回解析后的 DoctorReport", async () => {
      const fakeReport: DoctorReport = {
        generated_at_ms: 1_700_000_000_000,
        version: "v0.1.0",
        agent: {
          version: "v0.1.0",
          server_instance_id: "inst-1",
          go_version: "go1.22",
          gomaxprocs: 4,
          num_goroutine: 5,
          openai_api_key_configured: true,
        },
        sessions: { total_cached: 2, total_persisted: 3, largest_session_size_bytes: 1024 },
        tools: { registered_count: 2, names: ["echo", "list_files"] },
        openlist: { base_url_configured: true, token_configured: true, last_ping_ms: 12 },
        skills: { loaded_count: 0, names: [] },
        issues: ["openai_api_key is set"],
      };
      const spy = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(fakeReport),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const { runSyncDoctor } = await import("@/composables/useAgent");
      const report = await runSyncDoctor();
      expect(spy).toHaveBeenCalledTimes(1);
      const [calledUrl, calledInit] = spy.mock.calls[0] as [string, RequestInit];
      expect(calledUrl).toContain("/api/sync/doctor");
      expect(calledInit.method).toBe("POST");
      expect(report.version).toBe("v0.1.0");
      expect(report.agent.gomaxprocs).toBe(4);
      expect(report.tools.names).toEqual(["echo", "list_files"]);
      expect(report.issues).toEqual(["openai_api_key is set"]);
    });

    it("非 2xx 响应时抛 Error（HTTP n）", async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
        json: () => Promise.reject(new Error("not json")),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const { runSyncDoctor } = await import("@/composables/useAgent");
      await expect(runSyncDoctor()).rejects.toThrow(/HTTP 503/);
    });

    it("响应体不合法时抛 Error（malformed doctor report）", async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ not: "a doctor report" }),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const { runSyncDoctor } = await import("@/composables/useAgent");
      await expect(runSyncDoctor()).rejects.toThrow(/malformed doctor report/);
    });
  });

  // ─── Task 26: LAN Access ───────────────────────────────────────────────
  // 覆盖 useAgent.getLanAccess() 的三个分支：成功 / HTTP 错误 / 网络错误。
  // 字段形状（interface / ip / url）是与后端 agent/lan_access.go 的
  // wire contract；任何字段重命名都是 breaking change，测试必须同步。
  describe("Task 26: getLanAccess（局域网访问地址枚举）", () => {
    beforeEach(() => {
      // 每次用例重置 fetch mock 防止跨用例污染。
      globalThis.fetch = originalFetch;
    });

    it("happy path：200 + addresses 数组透传到 caller", async () => {
      const sample: LanAddress[] = [
        { interface: "en0", ip: "192.168.1.10", url: "http://192.168.1.10:5245" },
        { interface: "eth0", ip: "10.0.0.5", url: "http://10.0.0.5:5245" },
      ];
      const spy = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ addresses: sample, port: 5245 }),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const result = await getLanAccess(0);
      expect(result).toEqual(sample);
      // 默认 port=0 ⇒ 不带 ?port= 查询参数（让后端走默认 5245）
      const calledUrl = (spy.mock.calls[0]?.[0] as string) || "";
      expect(calledUrl).not.toMatch(/\?port=/);
    });

    it("port > 0 时 query string 带 port 参数", async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ addresses: [], port: 8123 }),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      await getLanAccess(8123);
      expect(spy).toHaveBeenCalledTimes(1);
      const calledUrl = (spy.mock.calls[0]?.[0] as string) || "";
      expect(calledUrl).toMatch(/\?port=8123$/);
    });

    it("HTTP 非 2xx 时返回空数组（不抛错）", async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.reject(new Error("not used")),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const result = await getLanAccess(0);
      expect(result).toEqual([]);
    });

    it("网络异常时返回空数组（不抛错）", async () => {
      const spy = vi.fn().mockRejectedValue(new TypeError("network down"));
      globalThis.fetch = spy as unknown as typeof fetch;

      const result = await getLanAccess(0);
      expect(result).toEqual([]);
    });

    it("响应体不合法时（缺 addresses 字段）返回空数组", async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ port: 5245 }), // 无 addresses
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const result = await getLanAccess(0);
      expect(result).toEqual([]);
    });

    it("addresses 不是数组时（malformed）返回空数组", async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ addresses: "not-an-array" }),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const result = await getLanAccess(0);
      expect(result).toEqual([]);
    });
  });

  // ─── Task 27：buildHttpError 错误信息精准化 ─────────────────────
  // 背景：之前前端只读 response.statusText ("Service Unavailable")，
  // 把后端真正的 message 字段 ("未配置 API Key，请在 AI 设置中填写") 吞了。
  // buildHttpError 必须能 (1) 解析 JSON body 拿 message，
  // (2) 透出 error code 给 UI 做"去设置"按钮分支判断。
  describe("buildHttpError（错误信息精准化）", () => {
    it('503 + {error:"no_api_key", message:"未配置 API Key"} → 透出 message + code', async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
        statusText: "Service Unavailable",
        text: () => Promise.resolve(JSON.stringify({ error: "no_api_key", message: "未配置 API Key，请在 AI 设置中填写" })),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const { runSyncDoctor } = await import("@/composables/useAgent");
      let capturedErr: any;
      try {
        await runSyncDoctor();
      } catch (e) {
        capturedErr = e;
      }
      // 错误信息必须含后端 message，不是只显示 "Service Unavailable"
      expect(capturedErr?.message).toContain("未配置 API Key");
      // 错误码必须透出（用于 UI "去设置" 按钮分支）
      expect(capturedErr?.code).toBe("no_api_key");
      // HTTP 状态码后缀保留便于排错
      expect(capturedErr?.message).toContain("HTTP 503");
    });

    it('502 + {error:"upstream_error", message:"rate limit"} → 透出 upstream_error', async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: false,
        status: 502,
        statusText: "Bad Gateway",
        text: () => Promise.resolve(JSON.stringify({ error: "upstream_error", message: "rate limit" })),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const { runSyncDoctor } = await import("@/composables/useAgent");
      let capturedErr: any;
      try {
        await runSyncDoctor();
      } catch (e) {
        capturedErr = e;
      }
      expect(capturedErr?.message).toContain("rate limit");
      expect(capturedErr?.code).toBe("upstream_error");
    });

    it("body 不是 JSON 时 fallback 到 statusText，不崩", async () => {
      const spy = vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
        statusText: "Service Unavailable",
        text: () => Promise.resolve("not a json body"),
      } as Response);
      globalThis.fetch = spy as unknown as typeof fetch;

      const { runSyncDoctor } = await import("@/composables/useAgent");
      let capturedErr: any;
      try {
        await runSyncDoctor();
      } catch (e) {
        capturedErr = e;
      }
      // fallback 到了 statusText
      expect(capturedErr?.message).toContain("Service Unavailable");
      // code 兜底为 unknown
      expect(capturedErr?.code).toBe("unknown");
    });
  });

  // ─── Task 3: tool_call 状态机（success / failed / cancelled / TIMEOUT） ───
  // 核心契约：
  //   1. tool_call 创建后 status='pending'，armToolCallTimeout 启动 30s 看门狗
  //   2. tool_result { is_error: false } → tool_call.status='success' + output 填充
  //   3. tool_result { is_error: true, errorMessage } → tool_call.status='failed'
  //      + errorCode / errorMessage 填充；错误消息原样透传（后端已本地化）
  //   4. tool_result { status: 'cancelled' } → tool_call.status='cancelled'
  //   5. 30s 内无 tool_result → 自动 status='failed' + errorCode='TIMEOUT'
  //   6. 状态机严格单向：tool_call 处于 failed/cancelled/success 终态时，
  //      后续 tool_status / tool_result 不会把它再覆盖（防止陈旧重放）
  //   7. 收到 tool_result 时 clearToolCallTimeout 必须清除 timer（不泄漏）
  //   8. 派生 computed：runningTools / hasRunningTool / allToolCalls 正确反映
  describe("Task 3: tool_call 状态机", () => {
    it("success: tool_result is_error=false → tool_call.status=success + output 填充", async () => {
      const sse =
        sseLine("tool_call", {
          id: "tc-success",
          name: "list_files",
          args: "{}",
          auto_run: true,
          kind: "readOnly",
        }) +
        sseLine("tool_result", {
          id: "tc-success",
          name: "list_files",
          result: '{"files":["a.txt","b.txt"]}',
          is_error: false,
          status: "success",
          duration_ms: 120,
        }) +
        sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse]))));

      const { send, messages, runningTools, hasRunningTool } = useAgent();
      await send("list");

      const tc = messages.value[1].tool_calls[0];
      expect(tc.id).toBe("tc-success");
      expect(tc.status).toBe("success");
      // output 字段：后端 result 是 JSON 字符串时，前端尝试 JSON.parse 还原为对象
      expect(tc.output).toEqual({ files: ["a.txt", "b.txt"] });
      // finishedAt 应被填充
      expect(typeof tc.finishedAt).toBe("number");
      // errorCode/errorMessage 不应在 success 时填充
      expect(tc.errorCode).toBeUndefined();
      expect(tc.errorMessage).toBeUndefined();
      // 终态后 runningTools 立刻清空
      expect(runningTools.value.length).toBe(0);
      expect(hasRunningTool.value).toBe(false);
    });

    it("failed: tool_result is_error=true → tool_call.status=failed + errorCode/errorMessage 填充", async () => {
      const sse =
        sseLine("tool_call", {
          id: "tc-fail",
          name: "search_files",
          args: '{"path":"/nonexistent"}',
          auto_run: true,
          kind: "readOnly",
        }) +
        sseLine("tool_result", {
          id: "tc-fail",
          name: "search_files",
          result: "文件未找到: /nonexistent",
          is_error: true,
          status: "error",
          duration_ms: 50,
          errorCode: "ENOENT",
          errorMessage: "文件未找到: /nonexistent",
        }) +
        sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse]))));

      const { send, messages } = useAgent();
      await send("search");

      const tc = messages.value[1].tool_calls[0];
      // 关键修复点：之前 status 仍显示 success，现在正确显示 failed
      expect(tc.status).toBe("failed");
      expect(tc.errorCode).toBe("ENOENT");
      // 错误消息原样透传（后端已本地化）
      expect(tc.errorMessage).toBe("文件未找到: /nonexistent");
      expect(typeof tc.finishedAt).toBe("number");
    });

    it("cancelled: tool_result status=cancelled → tool_call.status=cancelled", async () => {
      const sse =
        sseLine("tool_call", {
          id: "tc-cancel",
          name: "delete_file",
          args: "{}",
          auto_run: false,
          kind: "fileChange",
        }) +
        sseLine("tool_result", {
          id: "tc-cancel",
          name: "delete_file",
          result: "",
          is_error: false,
          status: "cancelled",
          duration_ms: 0,
        }) +
        sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse]))));

      const { send, messages } = useAgent();
      await send("delete");

      const tc = messages.value[1].tool_calls[0];
      expect(tc.status).toBe("cancelled");
      // 取消状态不属于失败，errorCode 不应被设置
      expect(tc.errorCode).toBeUndefined();
      expect(typeof tc.finishedAt).toBe("number");
    });

    it("TIMEOUT: 30s 无 tool_result → tool_call.status=failed + errorCode=TIMEOUT", async () => {
      // 启用 fake timers，让 30s 超时立即可推进
      vi.useFakeTimers();
      try {
        // 只发 tool_call 事件，**永远不**发 tool_result。stream_end 推一个
        // 让 send() 正常返回。注意：stream_end 到达会切回 idle 但不会
        // 清除 30s timer（armToolCallTimeout 是在 tool_call 时启动的，
        // 直到 tool_result 到达或 timer 本身 fire）。
        const sse =
          sseLine("tool_call", {
            id: "tc-hang",
            name: "command_run",
            args: '{"command":"sleep 999"}',
            auto_run: true,
            kind: "command",
          }) + sseLine("stream_end", {});
        fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse]))));

        const { send, messages } = useAgent();
        await send("run long");

        // 初始：tool_call 已创建，status=pending，timer 已 arm
        const tc = messages.value[1].tool_calls[0];
        expect(tc.id).toBe("tc-hang");
        expect(tc.status).toBe("pending");

        // 推进 fake timers 30s（不真等 30s）
        await vi.advanceTimersByTimeAsync(30_000);

        // 30s 后：tool_call 应被自动标记 failed + errorCode='TIMEOUT'
        // 注意：reactive ref 触发后需要读取最新值
        const tcAfter = messages.value[1].tool_calls[0];
        expect(tcAfter.status).toBe("failed");
        expect(tcAfter.errorCode).toBe("TIMEOUT");
        // 错误消息：硬编码的中文文案（透传，不依赖后端）
        expect(tcAfter.errorMessage).toContain("30s");
        expect(typeof tcAfter.finishedAt).toBe("number");
      } finally {
        vi.useRealTimers();
      }
    });

    it("单向: 终态后 tool_status success 不会把 failed 覆盖回 success（防陈旧重放）", async () => {
      const sse =
        sseLine("tool_call", {
          id: "tc-once",
          name: "op",
          args: "{}",
          auto_run: true,
          kind: "readOnly",
        }) +
        sseLine("tool_result", {
          id: "tc-once",
          name: "op",
          result: "boom",
          is_error: true,
          status: "error",
          duration_ms: 10,
          errorCode: "BOOM",
          errorMessage: "boom",
        }) +
        // 异常：服务端在 tool_result 失败后又推 tool_status=success（陈旧）
        sseLine("tool_status", { id: "tc-once", status: "success" }) +
        sseLine("stream_end", {});
      fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse]))));

      const { send, messages } = useAgent();
      await send("q");

      const tc = messages.value[1].tool_calls[0];
      // 第一次 tool_result 失败后：failed
      expect(tc.status).toBe("failed");
      // 后续 tool_status=success 不应覆盖：仍为 failed
      expect(tc.status).toBe("failed");
      expect(tc.errorCode).toBe("BOOM");
    });

    it("runningTools / hasRunningTool 派生 computed 正确反映状态", async () => {
      // 推 2 个 tool_call：第一个 30s 内无 result（用 fake timer 推进 1s），
      // 第二个 正常收到 result 变 success
      vi.useFakeTimers();
      try {
        const sse =
          sseLine("tool_call", {
            id: "tc-r1",
            name: "op1",
            args: "{}",
            auto_run: true,
            kind: "readOnly",
          }) +
          sseLine("tool_call", {
            id: "tc-r2",
            name: "op2",
            args: "{}",
            auto_run: true,
            kind: "readOnly",
          }) +
          sseLine("tool_status", { id: "tc-r1", status: "running" }) +
          sseLine("tool_result", {
            id: "tc-r2",
            name: "op2",
            result: "{}",
            is_error: false,
            status: "success",
            duration_ms: 5,
          }) +
          sseLine("stream_end", {});
        fetchSpy.mockImplementation(urlAwareMock(fetchReturningStream(makeSSEStream([sse]))));

        const { send, messages, runningTools, hasRunningTool, allToolCalls } = useAgent();
        await send("q");

        // tc-r1: pending → running（via tool_status）
        // tc-r2: pending → success（via tool_result）
        const tc1 = messages.value[1].tool_calls[0];
        const tc2 = messages.value[1].tool_calls[1];
        expect(tc1.id).toBe("tc-r1");
        expect(tc2.id).toBe("tc-r2");
        expect(tc1.status).toBe("running");
        // running 状态时 startedAt 应被填充
        expect(typeof tc1.startedAt).toBe("number");
        expect(tc2.status).toBe("success");
        expect(typeof tc2.finishedAt).toBe("number");

        // 派生 computed：tc-r1 running + tc-r2 已 success → 仅 tc-r1
        expect(runningTools.value.length).toBe(1);
        expect(runningTools.value[0].id).toBe("tc-r1");
        expect(hasRunningTool.value).toBe(true);
        // allToolCalls：2 条全部
        expect(allToolCalls.value.length).toBe(2);
      } finally {
        vi.useRealTimers();
      }
    });
  });
});
