/**
 * useAgentStream - SSE 解析器 + 事件分发（从 useAgent.ts 拆分）
 *
 * 拆分原因：useAgent.ts 单文件超过 2000 行（自定义 vite 插件 file-size-limit
 * 强制要求拆分）。本文件承载 SSE 流式解析（legacy / AG-UI 双协议）与
 * `handleAgentEvent` 事件分发逻辑，是体积最大的两块。
 *
 * 设计：由于这些函数强依赖 useAgent() 闭包内的 reactive 状态
 * （messages / status / lastEventId / eventOffset / 各类 mock ref …），
 * 不再直接闭包捕获，而是通过一个显式 `AgentStreamContext` 上下文对象访问。
 * useAgent() 在定义好所有闭包函数后，构造 ctx 并调用 createAgentStream(ctx)
 * 取回 processSSE / handleAgentEvent 等函数，行为与原实现完全一致。
 */
import { type Ref } from "vue";
import {
  appendSequencedChunk,
  CONTEXT_COMPACTION_MARKER,
  MAX_TRACKED_REALTIME_SEQUENCES,
  parseCompactionData,
  parseContentDelta,
  parseStreamStatusData,
  parseToolCallData,
  parseToolResultData,
  parseToolStatus,
  type AgentEvent,
  type AgentStatus,
  type Message,
  type MockBranch,
  type MockPreset,
  type MockRoundState,
  type ToolCall,
} from "./useAgent_helpers";
import { processAGUISSE as processAGUISSEImpl } from "./useAGUIParser";

/** useAgent() 闭包状态 / 内部函数的显式访问上下文 */
export interface AgentStreamContext {
  /** 消息列表 reactive ref */
  messages: Ref<Message[]>;
  /** 当前 agent 状态 reactive ref */
  status: Ref<AgentStatus>;
  /** 错误文案 reactive ref */
  lastError: Ref<string>;
  /** 错误语义码 reactive ref */
  lastErrorCode: Ref<"" | "no_api_key" | "upstream_error" | "invalid_json" | "unknown">;
  /** lastEventId 闭包变量 getter/setter（断点续传用） */
  getLastEventId: () => number;
  setLastEventId: (n: number) => void;
  /** eventOffset 递增（每次事件处理后 +1） */
  bumpEventOffset: () => void;
  /** 内部闭包函数（已绑定 useAgent 闭包状态，直接透传） */
  rememberSequence: (seq: number) => boolean;
  pushRawEvent: (type: string, dataSummary: string, seq?: number | null) => void;
  finalizeLastAssistant: () => void;
  saveState: () => void;
  armToolCallTimeout: (toolCallId: string) => void;
  clearToolCallTimeout: (toolCallId: string) => void;
  /** Task 11：排队消息列表 */
  pendingMessages: Ref<Message[]>;
  /** Mock 模式相关 reactive ref */
  isMockMode: Ref<boolean>;
  mockScenario: Ref<string>;
  mockPresets: Ref<MockPreset[]>;
  mockPresetsPhase: Ref<string>;
  mockPresetsScenario: Ref<string>;
  mockBranchChoices: Ref<MockBranch[]>;
  mockBranchPrompt: Ref<string>;
  mockRoundState: Ref<MockRoundState | null>;
  currentMockScenario: Ref<string>;
}

export interface ProcessSSEResult {
  received: boolean;
  streamEnded: boolean;
  morePending: boolean;
}

/**
 * 构造 SSE 解析器 + 事件分发函数集合。
 * 所有函数通过 ctx 访问 useAgent 闭包状态，与原闭包实现行为一致。
 */
export function createAgentStream(ctx: AgentStreamContext) {
  const {
    messages,
    status,
    lastError,
    lastErrorCode,
    getLastEventId,
    setLastEventId,
    bumpEventOffset,
    rememberSequence,
    pushRawEvent,
    finalizeLastAssistant,
    saveState,
    armToolCallTimeout,
    clearToolCallTimeout,
    pendingMessages,
    isMockMode,
    mockScenario,
    mockPresets,
    mockPresetsPhase,
    mockPresetsScenario,
    mockBranchChoices,
    mockBranchPrompt,
    mockRoundState,
    currentMockScenario,
  } = ctx;

  /**
   * SSE 协议分发器。
   *
   * 行为：
   *   - protocol='agui'  → 走 AG-UI parser（composables/useAGUIParser.ts）
   *   - protocol='legacy'→ 走原始自定义 SSE（保持原 processSSE 行为）
   *
   * 支持 SSE 标准 `id: N` 字段（后端断点续传时用），用于维护 lastEventId。
   * 多个 `id:` 行在同一事件中以最后一个为准。
   *
   * 返回结构：
   *   - received:      是否收到过任何 data 事件
   *   - streamEnded:   是否收到过 stream_end 事件（用于 runResumeChain 决定是否链式续传）
   *   - morePending:   最后一个有意义事件是否为 stream_status.more_pending
   *                    （若 true 且 !streamEnded，runResumeChain 应继续下一轮）
   */
  async function processSSE(
    stream: ReadableStream<Uint8Array> | null,
    protocol: "agui" | "legacy" = "legacy"
  ): Promise<ProcessSSEResult> {
    if (protocol === "agui") {
      return processAGUISSEWithHandlers(stream);
    }
    return processLegacySSE(stream);
  }

  /**
   * 原始自定义 SSE 事件流解析（AG-UI 未启用时的回退路径）。
   */
  async function processLegacySSE(stream: ReadableStream<Uint8Array> | null): Promise<ProcessSSEResult> {
    if (!stream) return { received: false, streamEnded: false, morePending: false };

    const reader = stream.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let received = false;
    let streamEnded = false;
    /** 最后一个 stream_status 事件：synced 还是 more_pending（用于 chain 决策） */
    let lastStreamStatus: "synced" | "more_pending" | null = null;

    /**
     * 处理一个完整 SSE 事件（已被 \n\n 分隔）
     * 维护 currentEventId 用于关联同一事件内的 data 行
     */
    function consumeEvent(rawEvent: string): void {
      if (!rawEvent.trim()) return;
      let currentEventId: number | null = null;
      const lines = rawEvent.split("\n");
      for (const line of lines) {
        // SSE 标准：id: N —— 用于断点续传
        if (line.startsWith("id:")) {
          const n = parseInt(line.slice(3).trim(), 10);
          if (Number.isFinite(n) && n >= 0) currentEventId = n;
          continue;
        }
        if (!line.startsWith("data: ")) continue;
        const payload = line.slice(6).trim();
        if (!payload) continue;
        received = true;
        try {
          const event = JSON.parse(payload) as AgentEvent;
          // ─── Task 27 调试：每个 SSE 事件立即 console.error 打印 ───
          const dataSummary =
            event.data == null
              ? "null"
              : typeof event.data === "string"
                ? event.data.slice(0, 120)
                : JSON.stringify(event.data).slice(0, 200);
          console.error(`[useAgent][SSE] type=${event.type} id=${currentEventId ?? "-"} data=${dataSummary}`);
          // ⑦ 区：追加到原始事件日志（AgentDebugPanel 可视化展示）
          pushRawEvent(event.type, dataSummary, currentEventId);
          // Task 4.3：sequence 去重。
          if (currentEventId !== null) {
            if (!rememberSequence(currentEventId)) {
              console.debug("[useAgent] drop duplicate seq", currentEventId);
              currentEventId = null;
              continue;
            }
            // 关联 SSE id：若同一事件声明了 id，则覆盖到 lastEventId
            setLastEventId(currentEventId);
            // 重置 currentEventId 避免下一行误用
            currentEventId = null;
          }
          if (event.type === "stream_end") streamEnded = true;
          if (event.type === "stream_status") {
            const payload = parseStreamStatusData(event.data);
            if (payload?.status === "synced" || payload?.status === "more_pending") {
              lastStreamStatus = payload.status;
            }
          }
          handleAgentEvent(event);
        } catch (e) {
          console.debug("[useAgent] malformed SSE payload:", payload, e);
        }
      }
    }

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        // SSE 事件以 \n\n 分隔
        const events = buffer.split("\n\n");
        buffer = events.pop() || "";

        for (const rawEvent of events) {
          consumeEvent(rawEvent);
        }
      }
      // 处理 trailing buffer（不带 \n\n 结尾的最后一段）
      if (buffer.trim()) {
        consumeEvent(buffer);
        buffer = "";
      }

      // 流结束：清理未结束的 assistant 消息 + 恢复 idle 状态
      finalizeLastAssistant();
      if (status.value === "streaming") {
        const hasPendingConfirm = messages.value.some(m => m.tool_calls.some(tc => tc.needsConfirm && tc.status === "pending"));
        status.value = hasPendingConfirm ? "confirming" : "idle";
        saveState();
      }
    } finally {
      try {
        reader.releaseLock();
      } catch {
        // already released
      }
    }

    return {
      received,
      streamEnded,
      morePending: lastStreamStatus === "more_pending",
    };
  }

  /**
   * AG-UI 协议路径：包装 useAGUIParser.processAGUISSE 把
   * 11 种 AG-UI 事件归一化为内部 AgentEvent，再走 handleAgentEvent。
   */
  async function processAGUISSEWithHandlers(
    stream: ReadableStream<Uint8Array> | null
  ): Promise<ProcessSSEResult> {
    return processAGUISSEImpl(stream, {
      onEvent: event => {
        const dataSummary =
          event.data == null
            ? "null"
            : typeof event.data === "string"
              ? event.data.slice(0, 120)
              : JSON.stringify(event.data).slice(0, 200);
        console.error(`[useAgent][AGUI] type=${event.type} data=${dataSummary}`);
        handleAgentEvent(event);
      },
      rememberSequence: (id: number) => {
        return rememberSequence(id);
      },
      onRawEvent: (type: string, dataSummary: string, seq?: number | null) => {
        pushRawEvent(type, dataSummary, seq ?? null);
        if (seq !== null && seq !== undefined && seq > getLastEventId()) {
          setLastEventId(seq);
        }
      },
      onStreamEnd: () => {
        finalizeLastAssistant();
        if (status.value === "streaming") {
          const hasPendingConfirm = messages.value.some(m => m.tool_calls.some(tc => tc.needsConfirm && tc.status === "pending"));
          status.value = hasPendingConfirm ? "confirming" : "idle";
          saveState();
        }
      },
    });
  }

  /**
   * 单个 event type → reactive state dispatch
   */
  function handleAgentEvent(event: AgentEvent): void {
    const lastAssistant = (): Message => {
      // Turn 边界检测：尾部是 system 消息 → 必须新开 assistant
      const tail = messages.value[messages.value.length - 1];
      const isTurnBoundary = tail && tail.role === "system";
      if (!isTurnBoundary) {
        // 非 turn 边界：优先 streaming → fallback 到任何 assistant
        for (let i = messages.value.length - 1; i >= 0; i--) {
          const m = messages.value[i];
          if (m.role === "assistant" && m.isStreaming) return m;
        }
        for (let i = messages.value.length - 1; i >= 0; i--) {
          if (messages.value[i].role === "assistant") return messages.value[i];
        }
      }
      // turn 边界 or 没有 assistant → 创建新 assistant
      const newMsg: Message = {
        role: "assistant",
        content: "",
        tool_calls: [],
        tool_results: [],
        isStreaming: true,
        eventLog: [], // Task 27：初始化事件顺序日志
      };
      messages.value.push(newMsg);
      return messages.value[messages.value.length - 1];
    };

    switch (event.type) {
      case "text_delta": {
        const m = lastAssistant();
        const parsed = parseContentDelta(event.data);
        appendSequencedChunk(m, "content", parsed.seq, parsed.text);
        if (!m.eventLog) m.eventLog = [];
        m.eventLog.push({ type: "text" });
        break;
      }
      case "reasoning_delta": {
        const m = lastAssistant();
        const parsed = parseContentDelta(event.data);
        appendSequencedChunk(m, "reasoning", parsed.seq, parsed.text);
        break;
      }
      case "tool_call": {
        const tool = parseToolCallData(event.data);
        if (tool) {
          const m = lastAssistant();
          m.tool_calls.push(tool);
          if (!m.eventLog) m.eventLog = [];
          m.eventLog.push({ type: "tool_call", id: tool.id });
          armToolCallTimeout(tool.id);
        }
        break;
      }
      case "tool_status": {
        const ts = parseToolStatus(event.data);
        if (ts) {
          for (const msg of messages.value) {
            const tc = msg.tool_calls.find(t => t.id === ts.id);
            if (tc) {
              if (tc.status === "failed" || tc.status === "cancelled" || tc.status === "success") {
                break;
              }
              tc.status = ts.status;
              if (ts.status === "running" && tc.startedAt === undefined) {
                tc.startedAt = Date.now();
              }
              break;
            }
          }
        }
        break;
      }
      case "tool_result": {
        const result = parseToolResultData(event.data);
        if (result) {
          let matchedTc: ToolCall | null = null;
          if (!result.name) {
            for (let i = messages.value.length - 1; i >= 0; i--) {
              const m = messages.value[i];
              if (m.role !== "assistant") continue;
              const tc = m.tool_calls.find(t => t.id === result.id);
              if (tc?.name) {
                result.name = tc.name;
                matchedTc = tc;
                break;
              }
            }
          } else {
            for (let i = messages.value.length - 1; i >= 0; i--) {
              const m = messages.value[i];
              if (m.role !== "assistant") continue;
              const tc = m.tool_calls.find(t => t.id === result.id);
              if (tc) {
                matchedTc = tc;
                break;
              }
            }
          }
          const m = lastAssistant();
          m.tool_results.push(result);
          if (!m.eventLog) m.eventLog = [];
          m.eventLog.push({ type: "tool_result", id: result.id });

          if (matchedTc && matchedTc.status !== "failed" && matchedTc.status !== "cancelled") {
            const finishedAt = Date.now();
            matchedTc.finishedAt = finishedAt;

            let rawErrorCode: string | undefined;
            let rawErrorMessage: string | undefined;
            let rawOutput: any;
            try {
              const raw = typeof event.data === "string" ? JSON.parse(event.data) : event.data;
              if (raw && typeof raw === "object") {
                const r = raw as Record<string, unknown>;
                if (typeof r.errorCode === "string") rawErrorCode = r.errorCode;
                else if (typeof (r as any).code === "string") rawErrorCode = (r as any).code;
                if (typeof r.errorMessage === "string") rawErrorMessage = r.errorMessage;
                else if (typeof (r as any).message === "string") rawErrorMessage = (r as any).message;
                if ("output" in r) rawOutput = (r as any).output;
              }
            } catch {
              // raw data 解析失败 → 用 fallback（不阻塞主流程）
            }

            if (result.status === "cancelled") {
              matchedTc.status = "cancelled";
            } else if (result.is_error === true) {
              matchedTc.status = "failed";
              matchedTc.errorCode = rawErrorCode || "EXEC_FAILED";
              matchedTc.errorMessage = rawErrorMessage || (typeof result.result === "string" ? result.result : "") || "工具执行失败";
            } else {
              matchedTc.status = "success";
              const output =
                rawOutput !== undefined
                  ? rawOutput
                  : typeof result.result === "string"
                    ? (() => {
                        try {
                          return JSON.parse(result.result);
                        } catch {
                          return result.result;
                        }
                      })()
                    : result.result;
              matchedTc.output = output;
            }
            clearToolCallTimeout(result.id);
          }
        }
        break;
      }
      case "stream_status": {
        const payload = parseStreamStatusData(event.data);
        if (payload?.maxEventId !== undefined) {
          if (payload.maxEventId > getLastEventId()) {
            setLastEventId(payload.maxEventId);
          }
        }
        if (payload?.status === "synced") {
          console.debug("[useAgent] stream_status: synced, awaiting new events");
        } else if (payload?.status === "more_pending") {
          console.debug("[useAgent] stream_status: more_pending, will re-resume");
        } else {
          console.debug("[useAgent] stream_status:", payload);
        }
        break;
      }
      case "stream_end": {
        for (let i = messages.value.length - 1; i >= 0; i--) {
          if (messages.value[i].role === "assistant" && messages.value[i].isStreaming) {
            messages.value[i].isStreaming = false;
            break;
          }
        }
        const hasPendingConfirm = messages.value.some(m => m.tool_calls.some(tc => tc.needsConfirm && tc.status === "pending"));
        status.value = hasPendingConfirm ? "confirming" : "idle";
        if (pendingMessages.value.length > 0) {
          const first = pendingMessages.value.shift()!;
          first.pending = false;
        }
        mockBranchChoices.value = [];
        mockBranchPrompt.value = "";
        mockRoundState.value = null;
        currentMockScenario.value = "";
        break;
      }
      case "stream_error": {
        finalizeLastAssistant();
        const errorMsg = parseContentDelta(event.data).text || "服务端流式传输发生未知错误";
        lastError.value = errorMsg;
        lastErrorCode.value = "upstream_error";
        status.value = "error";
        mockBranchChoices.value = [];
        mockBranchPrompt.value = "";
        mockRoundState.value = null;
        currentMockScenario.value = "";
        console.error("[useAgent] stream_error:", errorMsg);
        break;
      }
      case "stream_start": {
        try {
          const raw = JSON.parse(event.data) as { mock?: unknown; scenario?: unknown } | null;
          if (raw && raw.mock === true) {
            isMockMode.value = true;
            mockScenario.value = String(raw.scenario ?? "");
          }
        } catch {
          // data 不是 JSON → 非 mock 信号，忽略
        }
        break;
      }
      case "mock_presets": {
        try {
          const raw = JSON.parse(event.data) as { scenario?: unknown; phase?: unknown; presets?: unknown } | null;
          if (!raw) break;
          const list = Array.isArray(raw.presets) ? (raw.presets as MockPreset[]) : [];
          mockPresets.value = list
            .filter((p): p is MockPreset => !!p && typeof p === "object" && typeof p.id === "string" && typeof p.userText === "string")
            .map(p => ({
              id: p.id,
              label: String(p.label ?? p.id),
              userText: p.userText,
              icon: typeof p.icon === "string" ? p.icon : undefined,
              tooltip: typeof p.tooltip === "string" ? p.tooltip : undefined,
            }));
          mockPresetsPhase.value = String(raw.phase ?? "");
          mockPresetsScenario.value = String(raw.scenario ?? "");
          console.debug(
            "[useAgent] mock_presets:",
            mockPresets.value.length,
            "presets, phase=",
            mockPresetsPhase.value,
            ", scenario=",
            mockPresetsScenario.value
          );
        } catch (e) {
          console.debug("[useAgent] mock_presets parse failed:", e);
        }
        break;
      }

      case "mock_presets_clear": {
        console.debug("[useAgent] mock_presets_clear (ignored, chip 永远覆盖显示)");
        break;
      }

      case "mock_branch_choice": {
        try {
          const raw = JSON.parse(event.data) as {
            scenario?: unknown;
            prompt?: unknown;
            choices?: unknown;
            phase?: unknown;
          } | null;
          if (!raw) break;
          const list = Array.isArray(raw.choices) ? (raw.choices as MockBranch[]) : [];
          mockBranchChoices.value = list
            .filter((b): b is MockBranch => !!b && typeof b === "object" && typeof (b as MockBranch).id === "string")
            .map(b => ({
              id: b.id,
              label: String(b.label ?? b.id),
              icon: typeof b.icon === "string" ? b.icon : undefined,
              description: typeof b.description === "string" ? b.description : undefined,
            }));
          mockBranchPrompt.value = String(raw.prompt ?? "");
          if (typeof raw.scenario === "string" && raw.scenario.length > 0) {
            currentMockScenario.value = raw.scenario;
          }
          mockRoundState.value = {
            roundIdx: mockRoundState.value?.roundIdx ?? 0,
            totalRounds: mockRoundState.value?.totalRounds ?? 1,
            phase: "awaiting_branch_choice",
            context: mockRoundState.value?.context ?? {},
            scenario: currentMockScenario.value,
          };
          console.debug(
            "[useAgent] mock_branch_choice:",
            mockBranchChoices.value.length,
            'branches, prompt="',
            mockBranchPrompt.value.slice(0, 40),
            "..., scenario=",
            currentMockScenario.value
          );
        } catch (e) {
          console.debug("[useAgent] mock_branch_choice parse failed:", e);
        }
        break;
      }

      case "mock_round_state": {
        try {
          const raw = JSON.parse(event.data) as {
            roundIdx?: unknown;
            totalRounds?: unknown;
            phase?: unknown;
            context?: unknown;
            scenario?: unknown;
          } | null;
          if (!raw) break;
          const next: MockRoundState = {
            roundIdx: typeof raw.roundIdx === "number" ? raw.roundIdx : 0,
            totalRounds: typeof raw.totalRounds === "number" ? raw.totalRounds : 1,
            phase: typeof raw.phase === "string" ? raw.phase : "running",
            context: raw.context && typeof raw.context === "object" ? (raw.context as Record<string, unknown>) : {},
            scenario: typeof raw.scenario === "string" && raw.scenario.length > 0 ? raw.scenario : mockRoundState.value?.scenario,
          };
          mockRoundState.value = next;
          if (typeof raw.scenario === "string" && raw.scenario.length > 0) {
            currentMockScenario.value = raw.scenario;
          }
          console.debug(
            "[useAgent] mock_round_state: round",
            next.roundIdx,
            "/",
            next.totalRounds,
            "phase=",
            next.phase,
            "scenario=",
            currentMockScenario.value
          );
        } catch (e) {
          console.debug("[useAgent] mock_round_state parse failed:", e);
        }
        break;
      }

      case "compaction": {
        finalizeLastAssistant();
        const data = parseCompactionData(event.data);
        messages.value.push({
          role: "system",
          content: CONTEXT_COMPACTION_MARKER,
          tool_calls: [],
          tool_results: [],
        });
        if (data) {
          console.debug(
            "[useAgent] compaction: replaced",
            data.replaced_message_count,
            "messages @",
            data.triggered_at_ms ? new Date(data.triggered_at_ms).toISOString() : "(no timestamp)"
          );
        }
        break;
      }

      // ====== AG-UI 协议新增事件类型（useAGUIParser 归一化输出） ======

      case "tool_call_args": {
        try {
          const payload = typeof event.data === "string" ? JSON.parse(event.data) : (event.data as any);
          if (!payload || typeof payload !== "object" || !payload.id || typeof payload.argsDelta !== "string") break;
          for (let i = messages.value.length - 1; i >= 0; i--) {
            const m = messages.value[i];
            if (m.role !== "assistant") continue;
            const tc = m.tool_calls.find(t => t.id === payload.id);
            if (tc) {
              tc.args = (tc.args || "") + payload.argsDelta;
              break;
            }
          }
        } catch {
          // 解析失败：忽略此增量 args
        }
        break;
      }

      case "state_snapshot": {
        try {
          const payload = typeof event.data === "string" ? JSON.parse(event.data) : (event.data as any);
          if (typeof console.debug === "function") {
            const keys =
              payload && typeof payload === "object" && payload.state && typeof payload.state === "object"
                ? Object.keys(payload.state)
                : [];
            console.debug("[useAgent] agui state_snapshot keys:", keys);
          }
        } catch {
          // 静默
        }
        break;
      }

      case "messages_snapshot": {
        try {
          const payload = typeof event.data === "string" ? JSON.parse(event.data) : (event.data as any);
          if (typeof console.debug === "function") {
            const count = payload && Array.isArray(payload.messages) ? payload.messages.length : 0;
            console.debug("[useAgent] agui messages_snapshot count:", count);
          }
        } catch {
          // 静默
        }
        break;
      }

      default:
        // 未知 type 静默忽略
        break;
    }

    bumpEventOffset();
    saveState();
  }

  return { processSSE, processLegacySSE, processAGUISSEWithHandlers, handleAgentEvent };
}
