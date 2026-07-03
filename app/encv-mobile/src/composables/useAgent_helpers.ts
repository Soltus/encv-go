// useAgent_helpers.ts - 类型 / 常量 / 解析器 / 辅助函数
// 拆分自 useAgent.ts。useAgent 主函数在 useAgent.ts。
//
// 注意：本文件**纯函数** + 类型。Vue 响应式 API（ref/computed/showToast 等）、
// 业务依赖（getAgentApiBase/useContextUsage 等）都保留在主文件 useAgent.ts，
// 因为这些只在主函数内部使用，搬过来反而冗余。

import { getAgentApiBase } from "./useAgentApiBase";
import type { MessageContentPart } from "./useAttachments";

// =============================================================================
// 类型定义（与 agent Go 服务契约对齐）
// =============================================================================

export type AgentStatus = "idle" | "streaming" | "confirming" | "error";

export interface SessionMeta {
  id: string;
  title: string;
  createdAt: number;
  updatedAt: number;
  messageCount: number;
  rounds: number;
}

export type Decision = "accept" | "accept_for_session" | "decline" | "cancel";

export type AgentEventType =
  | "text_delta"
  | "reasoning_delta"
  | "tool_call"
  | "tool_status"
  | "tool_result"
  | "stream_status" // 后端流式状态（断点续传时推 synced / more_pending）
  | "stream_start" // Mock 模式信号：data = { mock: true, scenario: "..." }
  | "stream_end"
  | "stream_error" // 后端在 SSE 流过程中遇到不可恢复错误时推送
  // 上下文自动压缩事件。Task 7 引入：后端在 messages token 数
  // 越过窗口 80% 时调用 LLM summary 压缩老消息，并推送本事件。
  // 前端收到时插入一条 role='system', content='上下文已自动压缩'
  // 的合成消息（renderTurnItems 把它转成 ContextCompactionDivider）。
  // 这是 7 种 event type 中的第 7 种，原有 6 种契约不变。
  | "compaction"
  // Mock 模式剧本预设：后端 MockEngine 在 stream_start 之后（或 mid-scenario
  // 任意 step 内）推送，data 形状是 { scenario, phase, presets: MockPreset[] }。
  // 前端收到后由 MockPresetBar 渲染为输入框上方的 chip 按钮列表。
  | "mock_presets"
  // Mock 模式预设清空信号：后端在 stream_end 时推，前端 MockPresetBar 收到
  // 后清空 chip。reason 字段仅做调试。
  | "mock_presets_clear"
  // ── AG-UI 协议新增内部类型（useAGUIParser 归一化输出） ──
  // tool_call_args：AG-UI TOOL_CALL_ARGS 事件归一化结果，携带 args 增量
  // （arg 是 String，handleAgentEvent 把它追加到对应 tool_call.args 字段）
  | "tool_call_args"
  // state_snapshot：AG-UI STATE_SNAPSHOT 事件归一化结果
  // （会话级共享状态，前端暂不消费，仅做调试记录 + 持久化兜底）
  | "state_snapshot"
  // messages_snapshot：AG-UI MESSAGES_SNAPSHOT 事件归一化结果
  // （完整消息快照，用于断点续传对齐）
  | "messages_snapshot"
  // ── v2 多轮/分支剧本（参考 .trae/specs/agent-tools-scenarios-v2/spec.md）───
  // mock_branch_choice：剧本在 step.branch_choice=true 时推，由前端
  // MockBranchChoiceBar 渲染为 chip 列表；用户点击 chip / 直接键入
  // 文本都走 pickMockBranch / sendMockRoundResponse 把 userText
  // 送回后端 Resume。
  | "mock_branch_choice"
  // mock_round_state：剧本报告当前 round 进度（round_idx / total_rounds /
  // phase / context）。前端由 mockRoundState 暴露；AgentChat 可选择
  // 渲染 "Round 2/4 · awaiting_user_input" header。
  | "mock_round_state";

/** 单个 mock 预设按钮契约（与后端 internal/server.MockPreset JSON 对应） */
export interface MockPreset {
  id: string;
  label: string;
  userText: string;
  icon?: string;
  tooltip?: string;
}

/**
 * 单个 mock 分支选项契约（与后端 internal/server.MockBranch JSON 对应）。
 * 用于剧本中 step.branch_choice=true 时的选项列表。
 * - id：精确匹配 / 关键词匹配 / 正则匹配时使用
 * - label：chip 上显示
 * - icon / description：可选 UI 增强
 */
export interface MockBranch {
  id: string;
  label: string;
  icon?: string;
  description?: string;
}

/** mock_round_state 事件 payload 形状（后端 MockRoundState JSON 归一化结果） */
export interface MockRoundState {
  /** 当前 round 下标（0-based） */
  roundIdx: number;
  /** 剧本总轮数（v2 8 个剧本里 edit_metadata_wizard=4 / batch_rename=2 / 其他=1） */
  totalRounds: number;
  /** 当前阶段：`running` / `awaiting_user_input` / `awaiting_branch_choice` */
  phase: string;
  /** 跨轮变量：set_context / use_context 写入/读取的任意结构 */
  context: Record<string, unknown>;
  /** 归属 scenario ID（调试用） */
  scenario?: string;
}

/** Agent 推送到 SSE channel 的事件 */
export interface AgentEvent {
  type: AgentEventType;
  /** JSON string —— 前端按 type 自行反序列化 */
  data: string;
}

export type ToolKind = "command" | "fileChange" | "readOnly" | "webSearch" | "plan" | "unknown";
export type ToolStatus = "pending" | "running" | "success" | "failed" | "cancelled";

export interface ToolCall {
  id: string;
  name: string;
  args: string;
  auto_run: boolean;
  kind: ToolKind;
  /** !auto_run —— 需要用户 4-决策确认 */
  needsConfirm: boolean;
  status: ToolStatus;
  /**
   * 失败时的错误码（与后端 ToolError.Code 对齐，如 'ENOENT' / 'TIMEOUT' / 'EXEC_FAILED'）。
   * 仅在 status === 'failed' 时有值。
   */
  errorCode?: string;
  /** 失败时的错误消息（来自后端，已本地化；前端原样透传，不硬编码翻译） */
  errorMessage?: string;
  /** 成功时的输出（来自后端 ToolResult.Output，结构因工具而异） */
  output?: any;
  /** 运行开始时间戳（毫秒）。在 tool_status 收到 'running' 时填入 */
  startedAt?: number;
  /** 运行结束时间戳（毫秒）。在 tool_result 到达 / 状态转为终态时填入 */
  finishedAt?: number;
}

export interface ToolResult {
  id: string;
  name: string;
  result: string;
  is_error: boolean;
  status: string;
  duration_ms: number;
}

export interface Message {
  /**
   * Task 27：事件到达顺序日志（agent 流式时间轴渲染核心）。
   *
   * 后端 SSE 事件流按时间顺序到达：text_delta → tool_call → tool_result → text_delta → ...
   * 但 Message 结构体把所有 text 合并到 content、所有 tool_calls/tool_results 分开存。
   * eventLog 记录原始到达顺序，让 renderTurnItems 能按时间轴交错渲染：
   *   [text, tool_call(id=call_mount), tool_result(id=call_mount), text, tool_call(id=call_files1), ...]
   *
   * 每条 entry 的 type 取值：'text' | 'tool_call' | 'tool_result' | 'stream_start' | 'stream_end'
   * tool_call / tool_result 条目额外带 id 字段用于配对。
   */
  eventLog?: Array<{ type: string; id?: string }>;
  /**
   * Task 22：agent 派发 subagent 拆解任务时插入的"agent task"消息。
   * 与 user/assistant/system 并列，是前端渲染层的合法角色之一。
   * 后端在 SubagentDispatch 事件中构造（content 是 JSON 字符串，
   * 形如 {"subTasks":[{id,status,description}], "reasoning":"..."}）。
   * renderTurnItems 检测到该角色时产出 type='agentTask' 的
   * RenderedItem，由 AgentTaskMessage.vue 渲染子任务列表。
   * 后端持久化 / 上下文回送时该 role 一并保留——见 send() 里的
   * apiMessages 构造循环。
   */
  role: "user" | "assistant" | "system" | "agent_task";
  /**
   * Task 12：附件场景下 content 可能是 OpenAI multimodal 数组
   * （text / image_url / file 元素）。老消息（无附件）保持 string。
   * 持久化层 JSON.parse 后这个 union 仍能正确还原。
   */
  content: string | MessageContentPart[];
  reasoning?: string;
  tool_calls: ToolCall[];
  tool_results: ToolResult[];
  isStreaming?: boolean;
  /**
   * 发送失败时的错误信息（每条消息独立）
   */
  error?: string;
  /**
   * Task 11 (Steer / Queue)：当用户点击「排队下一条」时，
   * 该 user 消息进入 pendingMessages 队列，等待当前 turn
   * 完全结束后由服务端 drain hook 触发新一轮 Chat。pending=true
   * 时 UI 应展示「已排队」标签。首个 text_delta 事件到达时
   * 自动清除（说明服务端已开始处理该消息）。
   */
  pending?: boolean;
  /**
   * 内部 buffer：SSE {seq, text} 排序重建用（不持久化，不回送给后端）
   */
  _contentSeqBuf?: Map<number, string>;
  _reasoningSeqBuf?: Map<number, string>;
}

/**
 * Task 7 引入的「上下文已自动压缩」标记文本。后端在
 * EventCompaction 事件里推送一份 LLM 生成的 summary，前端把它
 * 包裹成一条 role='system' 的合成消息插入 messages 列表。renderTurnItems
 * 检测到 message.role === 'system' && content === CONTEXT_COMPACTION_MARKER
 * 时产出 compaction 类型的 RenderedItem，由 AgentChat.vue
 * 渲染为不可展开的 ContextCompactionDivider 分隔线。
 *
 * 该 marker 是 *单向契约*：前端在 useAgent.ts 收到 compaction 事件
 * 时构造，后端不会直接发这个字符串。如果后端在 messages 数组里看到
 * 这条「system:上下文已自动压缩」消息，会被忽略（不会再次触发压缩）。
 */
export const CONTEXT_COMPACTION_MARKER = "上下文已自动压缩";

// =============================================================================
// 常量
// =============================================================================

/** 持久化到 localStorage 的 key 前缀 */
export const STORAGE_PREFIX = "agent:session:";

/**
 * Agent 服务 API 基础 URL（动态解析，**不在模块加载时缓存**）。
 *
 * 为什么是函数而不是常量：
 *   - 旧实现 `const AGENT_API_BASE = getAgentApiBase()` 在模块首次 import 时
 *     求值一次 → 之后即使用户改了 baseUrl（probe 命中 LAN / 手动设置 / 切前后台），
 *     永远用旧值 → 真实路由失败但 JS 还打旧 URL
 *   - 新实现 getAgentBase() 每次调用都实时读 getApiBaseBase() →
 *     baseUrl 变化立刻生效（与 WS 层 useWebSocket 行为一致）
 *
 * 性能：每次调用 ≈ 1 次 localStorage 读 + 1 个三元判断，可忽略
 * （chat send 不是热路径，且 baseUrl 变化场景只在 probe/手动切换瞬间）
 */
export function getAgentBase(): string {
  return getAgentApiBase();
}

/**
 * 单实例最多追踪的 SSE sequence 编号数。超过此上限时按插入顺序
 * 淘汰最老的 sequence 编号（FIFO 近似 LRU）。参考 codex-web
 * `appServerRealtimeReducer.ts` 的 `MAX_TRACKED_REALTIME_SEQUENCES` 常量。
 */
export const MAX_TRACKED_REALTIME_SEQUENCES = 2_000;

/** ToolCall 状态在 tool_status 事件中可能的取值 */
const TOOL_STATUS_VALUES: ReadonlySet<ToolStatus> = new Set<ToolStatus>(["pending", "running", "success", "failed", "cancelled"]);

// =============================================================================
// 工具函数
// =============================================================================

/**
 * 解析 `text_delta` / `reasoning_delta` 的 data 字段
 * 后端格式：json.Marshal(plainString) → 经外层 JSON 解码后 data 就是纯文本
 * 兼容旧格式：{"content": "..."} 包装
 */
/**
 * 解析 text_delta / reasoning_delta 的 data 字段。
 *
 * 后端架构升级后事件格式从裸字符串变为 {seq: number, text: string}，
 * 此函数兼容两种格式：
 *   - 旧格式：data 是 string → { text, seq: undefined }
 *   - 新格式：data 是 {seq, text} 对象 → { text, seq }
 */
interface ParsedContentDelta {
  text: string;
  seq?: number;
}
export function parseContentDelta(data: unknown): ParsedContentDelta {
  if (!data) return { text: "" };
  if (typeof data === "string") {
    try {
      const parsed = JSON.parse(data);
      if (typeof parsed === "string") return { text: parsed };
      if (parsed && typeof parsed === "object") {
        // 新格式 {seq, text}
        if ("text" in parsed && "seq" in parsed) {
          return { text: String(parsed.text ?? ""), seq: Number(parsed.seq) };
        }
        // AG-UI 归一化格式：{text, messageId}（useAGUIParser 输出，**无 seq**）
        // 修乱码 bug：之前这种情况会落到末尾 return {text: data}，把整段 JSON 字符串当文本渲染
        if ("text" in parsed) {
          return { text: String((parsed as { text: unknown }).text ?? "") };
        }
        // 旧格式兼容 {"content":"..."}
        if ("content" in parsed) {
          return { text: String((parsed as { content: unknown }).content ?? "") };
        }
      }
    } catch {
      // 不是有效 JSON → 纯文本，直接使用
    }
    return { text: data };
  }
  // data 已经是对象（新格式，SSE 层已 JSON.parse）
  if (data && typeof data === "object") {
    if ("text" in data && "seq" in data) {
      return { text: String((data as { text: unknown }).text ?? ""), seq: Number((data as { seq: unknown }).seq) };
    }
    // AG-UI 归一化格式：{text, messageId}
    if ("text" in data) {
      return { text: String((data as { text: unknown }).text ?? "") };
    }
  }
  return { text: String(data ?? "") };
}

/**
 * 按 seq 序列号追加文本块到消息字段，保证乱序到达时也能正确排序显示。
 *
 * 内部维护 msg._contentSeqBuf / msg._reasoningSeqBuf（Map<number, string>），
 * 每次写入后按 key 排序重建 msg.content / msg.reasoning。
 */
export function appendSequencedChunk(msg: Message, field: "content" | "reasoning", seq: number | undefined, text: string) {
  const bufKey = field === "content" ? "_contentSeqBuf" : "_reasoningSeqBuf";
  let buf = msg[bufKey] as Map<number, string> | undefined;
  if (!buf) {
    buf = new Map<number, string>();
    msg[bufKey] = buf;
  }

  if (seq !== undefined) {
    // 有序号模式：存入 buffer，按 seq 排序后重建
    buf.set(seq, text);
    let rebuilt = "";
    const sortedKeys = Array.from(buf.keys()).sort((a, b) => a - b);
    for (const k of sortedKeys) rebuilt += buf.get(k);
    msg[field] = rebuilt;

    // 检测乱序/丢包（seq 不连续）
    if (buf.size > 1 && sortedKeys.length > 1) {
      for (let i = 1; i < sortedKeys.length; i++) {
        if (sortedKeys[i] - sortedKeys[i - 1] > 1) {
          console.warn(
            `[useAgent] seq gap detected in ${field}:`,
            `missing ${sortedKeys[i - 1] + 1}..${sortedKeys[i] - 1}`,
            `(got ${sortedKeys[i - 1]} → ${sortedKeys[i]}, total=${buf.size})`
          );
          break; // 只报一次
        }
      }
    }
  } else {
    // 无序号（旧格式 fallback）：直接追加
    (msg[field] as string) = ((msg[field] as string) || "") + text;
  }
}

/**
 * 解析 `tool_call` 的 data 字段 —— ToolCallData
 */
export function parseToolCallData(data: unknown): ToolCall | null {
  try {
    // event.data 可能是已解析的对象（processSSE 中 JSON.parse 后）或字符串（旧代码路径）
    const parsed: Partial<ToolCall> = typeof data === "string" ? JSON.parse(data) : (data as Partial<ToolCall>);
    if (!parsed.id || !parsed.name) return null;
    const autoRun = parsed.auto_run !== false;
    return {
      id: String(parsed.id),
      name: String(parsed.name),
      args: typeof parsed.args === "string" ? parsed.args : JSON.stringify(parsed.args ?? {}),
      auto_run: autoRun,
      kind: (parsed.kind as ToolKind) ?? "unknown",
      needsConfirm: !autoRun,
      status: "pending",
    };
  } catch {
    return null;
  }
}

/**
 * 解析 `tool_status` 的 data 字段 —— 包含 id 和 status
 */
export function parseToolStatus(data: unknown): { id: string; status: ToolStatus } | null {
  try {
    const parsed = typeof data === "string" ? JSON.parse(data) : data;
    if (!parsed || typeof parsed !== "object") return null;
    const { id, status: rawStatus } = parsed as { id?: string; status?: string };
    if (!id || !rawStatus) return null;
    const status = rawStatus as ToolStatus;
    if (!TOOL_STATUS_VALUES.has(status)) return null;
    return { id, status };
  } catch {
    return null;
  }
}

/**
 * 解析 `tool_result` 的 data 字段 —— ToolResultData
 *
 * 适配 AG-UI 协议：AG-UI `TOOL_CALL_RESULT` 事件归一化后只有
 *   `{ id, result }`（**无 name** 字段——name 来自前面的 `TOOL_CALL_START`）
 * legacy 格式有 `name` 字段（来自后端 sendAndCache 的 tool_result 事件）
 * 本函数**不强制**要求 name，由调用方在拿到 result 后从已存在的
 * tool_calls 里按 id 查找补齐 name。
 */
export function parseToolResultData(data: unknown): ToolResult | null {
  try {
    const parsed = typeof data === "string" ? JSON.parse(data) : data;
    if (!parsed || typeof parsed !== "object") return null;
    const p = parsed as Partial<ToolResult>;
    if (!p.id) return null;
    return {
      id: String(p.id),
      // name 可能为空（AG-UI 归一化格式）——调用方负责补齐
      name: typeof p.name === "string" ? p.name : "",
      result: typeof p.result === "string" ? p.result : JSON.stringify(p.result ?? ""),
      is_error: p.is_error === true,
      status: String(p.status ?? "success"),
      duration_ms: typeof p.duration_ms === "number" ? p.duration_ms : 0,
    };
  } catch {
    return null;
  }
}

/**
 * 解析 `stream_status` 的 data 字段 —— StreamStatusData
 * 后端格式（internal/server/agent_api.go::sendAndCache stream_status 分支）：
 *   { "status": "synced" | "more_pending", "inProgress": bool, "maxEventId"?: number }
 *
 * 此类型在 D.2 引入：断点续传链路的前端信号
 */
export interface StreamStatusData {
  status: "synced" | "more_pending" | string;
  inProgress?: boolean;
  maxEventId?: number;
}

export function parseStreamStatusData(data: string): StreamStatusData | null {
  try {
    const parsed = JSON.parse(data) as Partial<StreamStatusData>;
    if (!parsed || typeof parsed.status !== "string") return null;
    return {
      status: parsed.status,
      inProgress: parsed.inProgress,
      maxEventId: typeof parsed.maxEventId === "number" ? parsed.maxEventId : undefined,
    };
  } catch {
    return null;
  }
}

/**
 * CompactionData 是后端 EventCompaction 事件的 payload 形状（Task 7）：
 *   {
 *     summary_text: string,            // LLM 生成的 summary
 *     replaced_message_count: number,  // 被替换的旧消息数
 *     triggered_at_ms: number,          // unix 毫秒时间戳
 *   }
 *
 * parseCompactionData 容忍后端包装格式（data 字段是 JSON 字符串）：
 *   - 直接 JSON.parse(data) 拿到 CompactionData
 *   - 旧后端可能把整个 CompactionData 直接 stringify 在 data 里
 *   - 极端场景：data 损坏 → 返回 null，让上层用 fallback 行为
 */
export interface CompactionData {
  summary_text?: string;
  replaced_message_count?: number;
  triggered_at_ms?: number;
}

export function parseCompactionData(data: string): CompactionData | null {
  if (!data) return null;
  try {
    const parsed = JSON.parse(data) as Partial<CompactionData>;
    if (!parsed || typeof parsed !== "object") return null;
    return {
      summary_text: typeof parsed.summary_text === "string" ? parsed.summary_text : undefined,
      replaced_message_count: typeof parsed.replaced_message_count === "number" ? parsed.replaced_message_count : undefined,
      triggered_at_ms: typeof parsed.triggered_at_ms === "number" ? parsed.triggered_at_ms : undefined,
    };
  } catch {
    return null;
  }
}

export function generateSessionId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  // 极端 fallback（老浏览器 / 测试环境无 crypto）
  return `sess-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}

/**
 * 把 fetch 失败响应构造成带 .code 标记的 Error。
 *
 * 背景：后端在缺 API Key / 解密失败 / 上游错误等场景下，body 是
 *   { "error": "no_api_key", "message": "未配置 API Key，请在 AI 设置中填写" }
 *  之前前端只读 statusText = "Service Unavailable"，把"未配置 API Key"
 *  这层用户语义吞了——用户只看到 "HTTP 503: Service Unavailable"，完全
 *  不知道为什么。
 *
 * 此函数：
 *   1. 尝试 JSON.parse(body) → 取 message / error 字段
 *   2. 拼成 "后端文案（HTTP 503）" 给用户看
 *   3. 在 Error 上挂 .code 字段（'no_api_key' / 'upstream_error' / 'unknown'），
 *      供上游 UI 做分支判断（如 chat 显示"去设置"按钮）
 */
export async function buildHttpError(response: Response, endpoint: string): Promise<Error & { code?: string; status?: number }> {
  let bodyText = "";
  let parsed: any = null;
  try {
    bodyText = await response.text();
    if (bodyText) {
      try {
        parsed = JSON.parse(bodyText);
      } catch {
        /* not JSON */
      }
    }
  } catch {
    // 读 body 失败也无所谓，落到 fallback
  }
  const userMessage =
    parsed && typeof parsed.message === "string" && parsed.message.trim()
      ? parsed.message
      : parsed && typeof parsed.error === "string" && parsed.error !== "unknown" && parsed.error.trim()
        ? parsed.error
        : response.statusText || "请求失败";
  const code = parsed && typeof parsed.error === "string" ? parsed.error : "unknown";
  const detail = `${userMessage}（HTTP ${response.status}）`;
  console.error("[useAgent]", endpoint, "failed:", detail);
  const err = new Error(detail) as Error & { code?: string; status?: number };
  err.code = code;
  err.status = response.status;
  return err;
}

// =============================================================================
// Task 26 (LAN Access) —— 与后端 /api/network/lan-access 对齐的类型
// =============================================================================
//
// 后端 agent/lan_access.go::LanAddress 的 JSON 形状。字段重命名是
// breaking change —— 任何修改都必须同步更新 AgentChat.vue 的渲染层。
// 保持后端字段 tag 与前端 interface 字段名一致：interface / ip / url。
export interface LanAddress {
  interface: string;
  ip: string;
  url: string;
}

/**
 * Task 26：拉取当前后端可访问的 LAN URL 列表。
 *
 * 调用后端 GET /api/network/lan-access?port=... ，失败时返回空数组并
 * 留痕一条 console.debug（不抛错、不显示 toast —— 该功能是辅助性的，
 * UI 应当自己处理空列表状态：折叠面板显示 "未发现可用网络接口"）。
 *
 * @param port 监听端口，传 0 让后端用默认 5245
 */
export async function getLanAccess(port: number = 0): Promise<LanAddress[]> {
  try {
    const qs = port > 0 ? `?port=${port}` : "";
    const response = await fetch(`${getAgentBase()}/api/network/lan-access${qs}`, {
      method: "GET",
    });
    if (!response.ok) {
      console.debug("[getLanAccess] HTTP", response.status, "— returning empty list");
      return [];
    }
    const data = (await response.json()) as { addresses?: LanAddress[] };
    if (!data || !Array.isArray(data.addresses)) {
      console.debug("[getLanAccess] malformed response:", data);
      return [];
    }
    return data.addresses;
  } catch (e) {
    console.debug("[getLanAccess] fetch failed:", e);
    return [];
  }
}

/**
 * Task 12：从 Message.content 抽取会话标题用的纯文本。
 *  - string  → 第一行前 40 字符
 *  - array   → 取首个 text 元素
 *  - empty   → 空串
 */
export function extractUserTitle(content: string | MessageContentPart[] | undefined): string {
  if (!content) return "";
  if (typeof content === "string") {
    return content.split("\n")[0]?.slice(0, 40) || "";
  }
  // multimodal 数组：找第一个 text 元素
  for (const part of content) {
    if (part.type === "text" && part.text) {
      return part.text.split("\n")[0]?.slice(0, 40) || "";
    }
  }
  // 全是附件没文本：返回一个占位提示
  return "[附件]";
}

// =============================================================================
// Task 25 (Sync Doctor) —— 与后端 /api/sync/doctor 对齐的类型
// =============================================================================
//
// DoctorReport 的 JSON 形状由 agent/sync_doctor.go::DoctorReport 定义。
// 前端不依赖任何后端内部结构 —— 只消费这个 doctor 报告的 wire 字段。
// 后端已经在生成报告前对所有错误信息和配置做 Redact 处理，所以
// 前端把它原样塞进 <pre> 块 / 剪贴板 / 截图分享都是安全的。
export interface DoctorReport {
  generated_at_ms: number;
  version: string;
  agent: {
    version: string;
    server_instance_id: string;
    go_version: string;
    gomaxprocs: number;
    num_goroutine: number;
    openai_api_key_configured: boolean;
  };
  sessions: {
    total_cached: number;
    total_persisted: number;
    largest_session_size_bytes: number;
  };
  tools: {
    registered_count: number;
    names: string[];
  };
  openlist: {
    base_url_configured: boolean;
    token_configured: boolean;
    last_ping_ms: number;
    last_error?: string;
  };
  skills: {
    loaded_count: number;
    names: string[];
  };
  issues: string[];
}

/**
 * Task 25：调用后端 /api/sync/doctor 拉取一次脱敏诊断报告。
 *
 * 用途：AgentSettingsDetail.vue 面板的「运行 sync 诊断」按钮，
 * 拿到 JSON 后展示给用户（<pre> 块 + 复制按钮）。
 *
 * 行为：
 *  - 成功：返回解析后的 DoctorReport，调用方自行 JSON.stringify 展示。
 *  - 失败：抛 Error，调用方负责 toast / 弹窗。
 *
 * 副作用：无（HTTP 只读）。AbortSignal 透传给 fetch 以便 UI 能取消
 * 一个长尾的 doctor 请求（实际后端超时是 2 秒，不会真等很久）。
 */
export async function runSyncDoctor(signal?: AbortSignal): Promise<DoctorReport> {
  const response = await fetch(`${getAgentBase()}/api/sync/doctor`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    // 没有 body 也合法：handler 接受 GET/POST 两种 method。
    body: JSON.stringify({}),
    ...(signal ? { signal } : {}),
  });
  if (!response.ok) {
    throw await buildHttpError(response, "/api/sync/doctor");
  }
  const report = (await response.json()) as DoctorReport;
  if (!report || typeof report !== "object" || !Array.isArray(report.issues)) {
    throw new Error("malformed doctor report");
  }
  return report;
}

// =============================================================================
// 复合式主体
// =============================================================================
