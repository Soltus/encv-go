/**
 * useAGUIParser.ts - AG-UI 协议 SSE 解析器
 *
 * 封装 AG-UI 事件（11 种类型）→ 内部 AgentEvent 的归一化逻辑。
 *
 * 协议文档参考：AG-UI 标准 SSE 事件流
 *   event: <TYPE>\n
 *   data: <JSON>\n
 *   \n
 *
 * 11 种事件 → 内部 AgentEvent 映射表：
 *   RUN_STARTED          → stream_start      (runId, threadId, protocol:'agui')
 *   TEXT_MESSAGE_START   → null              (纯 meta，前端无内部动作)
 *   TEXT_MESSAGE_CONTENT → text_delta        (text, messageId)
 *   TEXT_MESSAGE_END     → null              (纯 meta)
 *   TOOL_CALL_START      → tool_call         (id, name, args:'', auto_run:true, kind:'unknown')
 *   TOOL_CALL_ARGS       → tool_call_args    (id, argsDelta) — 新增内部类型
 *   TOOL_CALL_END        → tool_status       (id, status:'success')
 *   TOOL_CALL_RESULT     → tool_result       (id, result)
 *   RUN_FINISHED         → stream_end        (runId, threadId)
 *   STATE_SNAPSHOT       → state_snapshot    (state) — 新增内部类型
 *   MESSAGES_SNAPSHOT    → messages_snapshot (messages) — 新增内部类型
 *
 * 关键设计：
 * 1. parseAGUIEvent 纯函数：输入 `event: ...\ndata: ...\n\n` 块字符串 → 输出 AgentEvent | null
 * 2. processAGUISSE 通用 SSE 读取器：通过 handlers 回调与外部 useAgent 集成
 * 3. 协议分发在 useAgent.processSSE 中完成（读 X-Agent-Protocol 响应头）
 *
 * SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/
 */

import type { AgentEvent } from "./useAgent";

// =============================================================================
// 类型定义
// =============================================================================

/** AG-UI 标准 11 种事件类型 */
export type AGUIEventType =
  | "RUN_STARTED"
  | "TEXT_MESSAGE_START"
  | "TEXT_MESSAGE_CONTENT"
  | "TEXT_MESSAGE_END"
  | "TOOL_CALL_START"
  | "TOOL_CALL_ARGS"
  | "TOOL_CALL_END"
  | "TOOL_CALL_RESULT"
  | "RUN_FINISHED"
  | "STATE_SNAPSHOT"
  | "MESSAGES_SNAPSHOT";

/** processAGUISSE 返回的解析结果 */
export interface AGUIProcessResult {
  received: boolean;
  streamEnded: boolean;
  /** AG-UI 无 stream_status 信号（RUN_STARTED..RUN_FINISHED 模式），
   *  此字段恒为 false 保持与 processLegacySSE 形状一致 */
  morePending: boolean;
}

/** processAGUISSE 处理器回调 */
export interface AGUIStreamHandlers {
  /** 分发一个归一化后的 AgentEvent（与 legacy 共用 handleAgentEvent 入口） */
  onEvent: (event: AgentEvent) => void;

  /**
   * SSE 序列号去重：返回 true 表示"新增"（dispatch），
   * 返回 false 表示"重复"（丢弃）。未传则不去重。
   */
  rememberSequence?: (id: number) => boolean;

  /**
   * 原始事件日志：把收到的 AG-UI 事件 type + data 摘要写入调试面板
   */
  onRawEvent?: (type: string, dataSummary: string, seq?: number | null) => void;

  /**
   * 流关闭后回调：useAgent 在这里 finalizeLastAssistant + 修正 status
   */
  onStreamEnd?: () => void;
}

// =============================================================================
// 11 种事件 → 内部 AgentEvent.type 映射表
// =============================================================================

/**
 * 值为 null 表示该 AG-UI 事件无内部对应（纯 meta，前端无业务动作）。
 * 注意：TEXT_MESSAGE_START / TEXT_MESSAGE_END 是消息边界标记，
 * 内部 AgentEvent 状态机靠 TEXT_MESSAGE_CONTENT 的累积自动管理边界，
 * 不需要单独消费这两个事件。
 */
const AGUI_TO_AGENT_TYPE: Readonly<Record<AGUIEventType, AgentEvent["type"] | null>> = {
  RUN_STARTED: "stream_start",
  TEXT_MESSAGE_START: null,
  TEXT_MESSAGE_CONTENT: "text_delta",
  TEXT_MESSAGE_END: null,
  TOOL_CALL_START: "tool_call",
  TOOL_CALL_ARGS: "tool_call_args",
  TOOL_CALL_END: "tool_status",
  TOOL_CALL_RESULT: "tool_result",
  RUN_FINISHED: "stream_end",
  STATE_SNAPSHOT: "state_snapshot",
  MESSAGES_SNAPSHOT: "messages_snapshot",
};

// =============================================================================
// parseAGUIEvent —— 纯函数
// =============================================================================

/**
 * 解析单条 AG-UI SSE 事件块为内部 AgentEvent。
 *
 * 输入格式：
 *   event: <TYPE>\n
 *   data: <JSON>\n
 *   \n
 *
 * 也容忍同一块内的 `id: N` 行（断点续传用），但不在返回的 AgentEvent 中暴露——
 * rememberSequence 由 processAGUISSE 负责处理。
 *
 * @returns AgentEvent | null
 *   - null：事件类型未识别 / 类型无内部对应（TEXT_MESSAGE_* / 未知）
 *   - AgentEvent：data 字段是 JSON 字符串（与现有 AgentEvent 契约一致）
 */
export function parseAGUIEvent(raw: string): AgentEvent | null {
  if (!raw?.trim()) return null;

  let eventType = "";
  const dataLines: string[] = [];

  // 按行扫描：event: / data: 多行 SSE 规范（data: 行可重复，自动 join）
  const lines = raw.split("\n");
  for (const line of lines) {
    if (line.startsWith("event:")) {
      eventType = line.slice(6).trim();
    } else if (line.startsWith("data:")) {
      // data: 后可接一个空格（标准 SSE），trim 后是 payload
      dataLines.push(line.slice(5).trim());
    }
    // id: 行由 processAGUISSE 单独处理（与 legacy parser 共享去重逻辑）
  }

  if (!eventType) return null;

  // 未知类型直接 null（未来可扩展）
  if (!isKnownAGUIEventType(eventType)) return null;

  const target = AGUI_TO_AGENT_TYPE[eventType as AGUIEventType];
  if (!target) return null;

  // data 多行 join 后尝试 JSON 解析
  const dataStr = dataLines.join("\n");
  let parsedData: unknown = null;
  if (dataStr) {
    try {
      parsedData = JSON.parse(dataStr);
    } catch {
      // 解析失败：把原始字符串塞到 data，标记 _raw=true 让下游识别「未归一化」
      // 这样调试时能完整看到后端到底推了什么（不丢信息）。
      return {
        type: target,
        data: JSON.stringify({ _raw: dataStr, _parseError: true }),
      };
    }
  }

  // 按事件类型提取业务字段，归一化为下游 handleAgentEvent 期望的形状
  const extracted = extractAGUIEventData(eventType as AGUIEventType, parsedData);
  return {
    type: target,
    // 与现有 AgentEvent.data 契约一致：JSON 字符串
    data: JSON.stringify(extracted),
  };
}

/**
 * 检查字符串是否是已知的 AG-UI 事件类型。
 * 未识别事件返回 false（不抛错、不 panic）。
 */
function isKnownAGUIEventType(t: string): t is AGUIEventType {
  return Object.hasOwn(AGUI_TO_AGENT_TYPE, t);
}

/**
 * 按 AG-UI 事件类型从 data 对象中提取业务字段。
 *
 * 所有事件统一提取规则：
 * - RUN_STARTED:     { runId, threadId, protocol: 'agui' }
 * - TEXT_MESSAGE_*:  TEXT_MESSAGE_START/END 由 parseAGUIEvent 直接 return null
 * - TOOL_CALL_START: { id, name, args:'', auto_run:true, kind:'unknown' }  （与 legacy tool_call 形状对齐）
 * - TOOL_CALL_ARGS:  { id, argsDelta }
 * - TOOL_CALL_END:   { id, status:'success' }
 * - TOOL_CALL_RESULT:{ id, result }
 * - RUN_FINISHED:    { runId, threadId }
 * - STATE_SNAPSHOT:  { state }
 * - MESSAGES_SNAPSHOT:{ messages }
 */
function extractAGUIEventData(eventType: AGUIEventType, data: unknown): Record<string, unknown> {
  const obj = data && typeof data === "object" && !Array.isArray(data) ? (data as Record<string, unknown>) : {};

  switch (eventType) {
    case "RUN_STARTED":
      return {
        runId: typeof obj.runId === "string" ? obj.runId : "",
        threadId: typeof obj.threadId === "string" ? obj.threadId : "",
        protocol: "agui",
      };
    case "TEXT_MESSAGE_CONTENT":
      return {
        text: typeof obj.delta === "string" ? obj.delta : "",
        messageId: typeof obj.messageId === "string" ? obj.messageId : "",
      };
    case "TOOL_CALL_START":
      // 与 legacy tool_call 形状对齐：{id, name, args, auto_run, kind}
      // AG-UI 完整 args 在 TOOL_CALL_ARGS 事件里流式累积
      return {
        id: typeof obj.toolCallId === "string" ? obj.toolCallId : "",
        name: typeof obj.toolCallName === "string" ? obj.toolCallName : "",
        args: "",
        auto_run: true,
        kind: "unknown",
      };
    case "TOOL_CALL_ARGS":
      return {
        id: typeof obj.toolCallId === "string" ? obj.toolCallId : "",
        argsDelta: typeof obj.delta === "string" ? obj.delta : "",
      };
    case "TOOL_CALL_END":
      return {
        id: typeof obj.toolCallId === "string" ? obj.toolCallId : "",
        status: "success",
      };
    case "TOOL_CALL_RESULT":
      return {
        id: typeof obj.toolCallId === "string" ? obj.toolCallId : "",
        // AG-UI 规范中 content 是 string
        result: typeof obj.content === "string" ? obj.content : "",
      };
    case "RUN_FINISHED":
      return {
        runId: typeof obj.runId === "string" ? obj.runId : "",
        threadId: typeof obj.threadId === "string" ? obj.threadId : "",
      };
    case "STATE_SNAPSHOT":
      return {
        state: obj.state && typeof obj.state === "object" ? obj.state : {},
      };
    case "MESSAGES_SNAPSHOT":
      return {
        messages: Array.isArray(obj.messages) ? obj.messages : [],
      };
    default:
      return obj;
  }
}

// =============================================================================
// processAGUISSE —— SSE 读取器
// =============================================================================

/**
 * 包装 SSE 流读取循环，逐行解析 AG-UI 事件并通过 handlers 回调分发。
 *
 * 与 processLegacySSE 行为对比：
 *   相同：分块读取（stream chunked 处理） / \n\n 切分事件 / TextDecoder 流式解码
 *   不同：按 `event:` 行分类型而非统一 `data:` JSON 解析（AG-UI 用 `event:` + `data:`）
 *
 * 返回 { received, streamEnded, morePending } 与 legacy parser 同形，
 * 调用方可统一处理。
 */
export async function processAGUISSE(stream: ReadableStream<Uint8Array> | null, handlers: AGUIStreamHandlers): Promise<AGUIProcessResult> {
  if (!stream) return { received: false, streamEnded: false, morePending: false };

  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let received = false;
  let streamEnded = false;

  /**
   * 处理一个完整 SSE 事件（已被 \n\n 分隔）
   * 与 processLegacySSE.consumeEvent 形态相似但解析逻辑不同
   */
  function consumeEvent(rawEvent: string): void {
    if (!rawEvent.trim()) return;

    // ① 提取 id: 行（去重） + event: + data: 行
    let currentEventId: number | null = null;
    let eventType = "";
    const dataLines: string[] = [];

    const lines = rawEvent.split("\n");
    for (const line of lines) {
      if (line.startsWith("id:")) {
        const n = parseInt(line.slice(3).trim(), 10);
        if (Number.isFinite(n) && n >= 0) currentEventId = n;
        continue;
      }
      if (line.startsWith("event:")) {
        eventType = line.slice(6).trim();
        continue;
      }
      if (line.startsWith("data:")) {
        dataLines.push(line.slice(5).trim());
      }
    }

    if (!eventType) return;

    // ② 序列号去重（与 legacy 共用 rememberSequence 闭包）
    if (currentEventId !== null && handlers.rememberSequence) {
      if (!handlers.rememberSequence(currentEventId)) {
        // 重复 → 跳过该事件
        return;
      }
    }

    // ③ 解析 + 归一化
    // 重构一个最小化 rawEvent 喂给 parseAGUIEvent：
    // 它内部会重新扫描 event: / data: 行，结果与上面一致
    // 但为了避免重复扫描，这里直接 inline 归一化
    if (!isKnownAGUIEventType(eventType)) return;
    const target = AGUI_TO_AGENT_TYPE[eventType as AGUIEventType];
    if (!target) return;

    const dataStr = dataLines.join("\n");
    let parsedData: unknown = null;
    if (dataStr) {
      try {
        parsedData = JSON.parse(dataStr);
      } catch {
        parsedData = dataStr;
      }
    }
    const extracted = extractAGUIEventData(eventType as AGUIEventType, parsedData);

    received = true;
    if (target === "stream_end") streamEnded = true;

    // ④ 原始事件日志
    if (handlers.onRawEvent) {
      const summary = dataStr.length > 200 ? dataStr.slice(0, 200) : dataStr;
      handlers.onRawEvent(eventType, summary, currentEventId);
    }

    // ⑤ 分发到下游
    handlers.onEvent({
      type: target,
      data: JSON.stringify(extracted),
    });
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
  } finally {
    try {
      reader.releaseLock();
    } catch {
      // already released
    }
  }

  // 流关闭后回调（useAgent 在这里 finalizeLastAssistant + 修正 status）
  if (handlers.onStreamEnd) {
    try {
      handlers.onStreamEnd();
    } catch (e) {
      console.debug("[useAGUIParser] onStreamEnd handler threw:", e);
    }
  }

  return {
    received,
    streamEnded,
    morePending: false, // AG-UI 无 stream_status 信号
  };
}

// =============================================================================
// useAGUIParser —— composable 形式（仅暴露纯函数 + reader，不持有状态）
// =============================================================================

/**
 * 暴露 useAGUIParser 工具。
 * 不持有任何 reactive state（保持纯函数语义）。
 *
 * 调用方：
 *   const { parseAGUIEvent, processAGUISSE } = useAGUIParser()
 */
export function useAGUIParser() {
  return {
    parseAGUIEvent,
    processAGUISSE,
  };
}
