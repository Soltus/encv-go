/**
 * renderAgentFlow - Task 27：agent 流式时间轴渲染
 *
 * 按 Message.eventLog 的顺序，把文本段和工具调用/结果交错产出为 RenderedItem[]。
 *
 * 算法：
 * 1. 把 displayText（全部 markdown 文本）按 eventLog 中的 'text' 条目数均分。
 *    分割点用自然断点优先（\n\n 双换行 / ## 标题），保证每段是完整语义块。
 * 2. 遍历 eventLog：
 *    - 'text' → 产出 assistantText（取对应 text segment）
 *    - 'tool_call' → 产出 operation（单条工具调用卡片，内嵌 #result slot 渲染结果）
 *    - 'tool_result' → 不产出独立项（结果由 OperationCard #result slot 内嵌渲染）
 * 3. tool_result 紧跟在对应的 tool_call 后面（eventLog 保证顺序）
 *
 * 输出追加到 out 数组（不返回新数组——由调用方 renderTurnItems 统一管理）。
 */
function renderAgentFlow(out: RenderedItem[], msg: Message, messageIndex: number, displayText: string, streaming: boolean): void {
  const log = msg.eventLog!;
  const textEntryCount = log.filter(e => e.type === "text").length;

  // 把 displayText 分成 textEntryCount 段（按自然断点分割）
  const textSegments = splitContentIntoSegments(displayText, textEntryCount);

  // 预扫描：找出所有 text 段在 eventLog 中的全局序号（用于标记首/尾）
  const textGlobalIndices: number[] = [];
  let ti = 0;
  for (let ei = 0; ei < log.length && ti < textSegments.length; ei++) {
    if (log[ei].type === "text") {
      textGlobalIndices.push(ei);
      ti++;
    }
  }

  let textIdx = 0;
  // 预建 id→ToolCall 查找表（O(1) 查找）
  const tcMap = new Map(msg.tool_calls.map(tc => [tc.id, tc]));

  for (let entryIdx = 0; entryIdx < log.length; entryIdx++) {
    const entry = log[entryIdx];
    if (entry.type === "text") {
      const seg = textSegments[textIdx] ?? "";
      const globalPos = textGlobalIndices.indexOf(entryIdx);
      textIdx++;
      if (seg.trim().length > 0) {
        out.push({
          type: "assistantText",
          messageId: `a-${messageIndex}`,
          text: seg,
          streaming,
          firstInGroup: globalPos === 0, // 第一个 text 段 → 显示头像/名字
        });
      }
    } else if (entry.type === "tool_call" && entry.id) {
      const tc = tcMap.get(entry.id);
      if (tc) {
        // plan / webSearch / approval 走特殊路径（与旧逻辑一致）
        if (tc.kind === "plan") {
          const todos = parsePlanArgs(tc.args);
          out.push({
            type: "plan",
            messageId: tc.id,
            toolCallId: tc.id,
            todos,
            streaming: streaming && (tc.status === "pending" || tc.status === "running"),
          });
        } else if (tc.kind === "webSearch") {
          // webSearch 在时间轴模式下也单独渲染（不做 group 合并）
          out.push({ type: "operation", messageId: `a-${messageIndex}`, toolCallId: tc.id, streaming });
        } else if (tc.status === "pending" && tc.needsConfirm) {
          out.push({ type: "approval", toolCallId: tc.id, messageId: tc.id });
        } else {
          // readOnly / command / fileChange / unknown → operation 卡片
          out.push({ type: "operation", messageId: `a-${messageIndex}`, toolCallId: tc.id, streaming });
        }
      }
    } else if (entry.type === "tool_result" && entry.id) {
      // tool_result 不产出独立 RenderedItem——结果已由对应 operation 的
      // OperationCard #result slot 通过 findToolResultById() 内嵌渲染。
      // 此处仅保留 eventLog 中的位置信息（用于未来扩展，如结果折叠/展开状态）。
    }
    // stream_start / stream_end / 其他类型在 eventLog 中但不产生 RenderedItem
  }

  // 独立 Footer 段：时间戳固定为渲染时刻（不随组件 re-render 刷新）
  // 不依赖末段是 text —— 即使最后一段是 operation，footer 也独立展示
  out.push({
    type: "messageFooter",
    messageId: `a-${messageIndex}`,
    timestamp: Date.now(),
  });

  // 如果还有剩余 text 段没消费完（eventLog 不完整时兜底）
  while (textIdx < textSegments.length) {
    const seg = textSegments[textIdx];
    textIdx++;
    if (seg.trim().length > 0) {
      out.push({
        type: "assistantText",
        messageId: `a-${messageIndex}`,
        text: seg,
        streaming,
      });
    }
  }
}

/**
 * splitContentIntoSegments - 把全文按目标段数 + 自然断点切分。
 *
 * 策略：
 * 1. 先按 \n\n（双换行，markdown 段落分隔）预切成候选段
 * 2. 如果候选段数 >= targetCount → 直接取前 targetCount-1 段 + 剩余合并为最后一段
 * 3. 如果候选段数 < targetCount → 从最长段中间再切（保证段数够）
 * 4. targetCount <= 1 时直接返回 [fullText]
 */
function splitContentIntoSegments(fullText: string, targetCount: number): string[] {
  if (!fullText || fullText.trim().length === 0) return [];
  if (targetCount <= 1) return [fullText];

  // 按双换行预切，但保护 markdown 表格块（| 开头的连续行不应在中间断开）
  const candidates = splitPreservingTables(fullText);

  if (candidates.length >= targetCount) {
    // 够分：前 n-1 段各取一段，剩余全归最后一段
    const result = candidates.slice(0, targetCount - 1);
    const tail = candidates.slice(targetCount - 1).join("\n\n");
    result.push(tail);
    return result;
  }

  // 不够分：从最长的一段中间再切（避开表格区域）
  const result = [...candidates];
  while (result.length < targetCount) {
    // 找当前最长的段（跳过表格段）
    let longestIdx = -1;
    let longestLen = 0;
    for (let i = 0; i < result.length; i++) {
      const len = result[i].length;
      // 跳过表格段（以 | 开头）和太短的段
      const isTable = /^\s*\|/.test(result[i]);
      if (len > longestLen && len >= 20 && !isTable) {
        longestLen = len;
        longestIdx = i;
      }
    }
    if (longestIdx === -1) break; // 所有剩余段都是表格或太短，无法继续切分
    // 从中间位置切（尽量在换行或空格处）
    const mid = Math.floor(result[longestIdx].length / 2);
    let cutPos = mid;
    const nlBefore = result[longestIdx].lastIndexOf("\n", mid);
    const nlAfter = result[longestIdx].indexOf("\n", mid);
    if (nlBefore > mid - 100) cutPos = nlBefore + 1;
    else if (nlAfter > 0 && nlAfter < mid + 100) cutPos = nlAfter + 1;
    const head = result[longestIdx].slice(0, cutPos);
    const tail = result[longestIdx].slice(cutPos);
    result.splice(longestIdx, 1, head, tail);
  }

  return result;
}

/**
 * 按双换行分割文本，但保持 markdown 表格块完整。
 * 表格块 = 连续的以 | 开头的行（含分隔行 |---|---|）。
 * 这些行之间只用 \n 分隔，不应被 \n\n 切分点拆散。
 */
function splitPreservingTables(text: string): string[] {
  const lines = text.split("\n");
  const segments: string[] = [];
  let current: string[] = [];
  let inTable = false;

  for (const line of lines) {
    const isTableRow = /^\s*\|/.test(line);
    if (isTableRow && !inTable) {
      // 进入表格：先把之前累积的非表格内容作为一个 segment
      if (current.length > 0) {
        segments.push(current.join("\n"));
        current = [];
      }
      inTable = true;
      current.push(line);
    } else if (!isTableRow && inTable) {
      // 离开表格：表格内容作为独立 segment
      inTable = false;
      current.push(line); // 表格最后一行后的空行也归入表格段
      segments.push(current.join("\n"));
      current = [];
    } else {
      current.push(line);
    }
  }
  // 处理末尾残余
  if (current.length > 0) {
    segments.push(current.join("\n"));
  }

  // 对非表格段再做 \n\n 二次切分
  const result: string[] = [];
  for (const seg of segments) {
    if (/^\s*\|/m.test(seg)) {
      // 表格段：保持完整
      result.push(seg);
    } else {
      // 非表格段：按 \n\n 再细分
      const parts = seg.split(/\n\n+/);
      for (const p of parts) {
        if (p.trim().length > 0) result.push(p);
      }
    }
  }

  return result.length > 0 ? result : [text];
}

/**
 * renderTurnItems - 把 messages 数组转换为渲染块
 * 参照 codex_web renderTurnItems()
 * 累积 operationGroup（command/fileChange/toolOutput）和 webSearchGroup
 * flush 时按 group 类型返回不同的渲染规格
 *
 * 输入：messages 数组（来自 useAgent）+ status（'idle' | 'streaming' | 'confirming'）
 * 输出：RenderedItem[] - 每项包含 type + data，AgentChat 据此分发到不同组件
 *
 * 适配 useAgent.ts 的 Message 类型：
 * - Message.role: 'user' | 'assistant'
 * - Message.content: string
 * - Message.reasoning?: string
 * - Message.tool_calls: ToolCall[]
 * - Message.tool_results: ToolResult[]
 * - Message.isStreaming?: boolean
 * - ToolCall.kind: 'command' | 'fileChange' | 'readOnly' | 'unknown'
 * - ToolCall.needsConfirm: boolean（替代 requiresApproval）
 * - ToolCall.status: 'pending' | 'running' | 'success' | 'failed' | 'cancelled'
 */

import { type ComputedRef, computed, type Ref } from "vue";
import { type AgentStatus, CONTEXT_COMPACTION_MARKER, type Message, type ToolCall } from "./useAgent";
import type { MessageContentPart } from "./useAttachments";

/**
 * stripToolCallJSON — 从消息文本中剥离工具调用 JSON 片段（安全网）。
 *
 * 参考 LangChain agent-chat-ui 的 getContentString() 设计：
 * - LobeChat: 协议级分离（chunkType 区分 text/tools_calling），content 永远不含 tool JSON
 * - LangChain: content 可能含 tool_use block，渲染时 getContentString() 过滤只取 text 类型
 * - 我们: 后端可能泄漏 tool JSON 到 content（gptgod 代理不发送标准 tool_call_chunk 事件），
 *        渲染前需清理，避免在 GroupedOperationMessage 旁边重复显示原始 JSON
 *
 * 匹配 OpenAI function calling 格式:
 *   [{"name":"xxx","arguments":{...}}]  — 数组形式（最常见）
 *   {"name":"xxx","arguments":{...}}      — 单对象形式
 */
export function stripToolCallJSON(text: string): string {
  if (!text) return text;

  let cleaned = text;

  // 模式 1: [...{"name":"...",...}] 数组形式（OpenAI function calling 标准格式）
  cleaned = cleaned.replace(/\[\s*\{\s*"name"\s*:\s*"[^"]*"(?:\s*,\s*"[^"]*"\s*:\s*(?:\{[^}]*\}|"[^"]*"))*\s*\}\s*\]/g, "");

  // 模式 2: {"name":"...",...} 单对象形式（独立行）
  cleaned = cleaned.replace(/^\s*\{\s*"name"\s*:\s*"[^"]*"(?:\s*,\s*"[^"]*"\s*:\s*(?:\{[^}]*\}|"[^"]*"))*\s*\}\s*/gm, "");

  // 清理产生的多余空行（连续 3+ 个换行合并为最多 2 个）
  cleaned = cleaned.replace(/\n{3,}/g, "\n\n");

  return cleaned.trim();
}

/** 单条 todo，源自 plan tool (write_todos) 的 args JSON。 */
export interface PlanTodo {
  id: string;
  status: "pending" | "in_progress" | "completed" | string;
  content: string;
}

/**
 * Task 22：agent task 消息中的子任务（subagent 拆解）。
 * 与 plan/todo 不同——子任务来源是后端 agent 框架（codex-web 形态），
 * 不是 write_todos 工具。状态集合固定为四态，避免后端传来未知
 * 字符串导致渲染分支爆炸；非合法值由 parseAgentTaskContent 防御性降级
 * 为 'pending'。
 */
export interface SubTask {
  id: string;
  status: "pending" | "in_progress" | "completed" | "failed";
  description: string;
}

/** 单条渲染项 - 由 AgentChat 分发到对应组件 */
export type RenderedItem =
  | { type: "user"; messageId: string; text: string }
  | { type: "assistantText"; messageId: string; text: string; streaming: boolean; firstInGroup?: boolean }
  | { type: "messageFooter"; messageId: string; timestamp: number }
  | { type: "approval"; toolCallId: string; messageId: string }
  | { type: "operationGroup"; messageId: string; toolCallIds: string[]; forceComplete: boolean }
  // Task 27：单条工具调用卡片（agent 流式时间轴模式）。
  // 当 Message.eventLog 存在时，renderTurnItems 不再聚合所有 tool_call 到 operationGroup，
  // 而是按 eventLog 顺序逐个产出 operation（内嵌 #result slot 渲染结果），实现真正的 agent 步骤流。
  | { type: "operation"; messageId: string; toolCallId: string; streaming: boolean }
  // Task 27：单条工具结果数据卡片（紧跟对应 operation 后）。
  // 注意：当前 renderAgentFlow 不再产出此类型（结果由 OperationCard #result slot 内嵌渲染）。
  // 保留类型定义以备未来独立渲染 tool_result 之需。
  | { type: "toolResultCard"; messageId: string; toolResultId: string; name: string; result: string }
  | { type: "webSearchGroup"; messageId: string; queries: string[]; toolCallIds: string[] }
  | { type: "reasoning"; messageId: string; text: string; streaming: boolean }
  | { type: "error"; messageId: string; text: string; messageIndex: number }
  | { type: "plan"; messageId: string; toolCallId: string; todos: PlanTodo[]; streaming: boolean }
  // Task 7：上下文自动压缩分隔线。position 字段是分隔线在
  // RenderedItem[] 数组中的下标（用 idx 衍生），用于 key
  // 生成；text 是 i18n 文本（"上下文已自动压缩"）。
  | { type: "compaction"; messageId: string; text: string }
  // Task 22：agent task 消息。subTasks 是子任务列表；reasoning
  // 是可选的"为什么派发子任务"的高层说明（来自后端 SubagentDispatch
  // 事件）。渲染端 AgentTaskMessage.vue 负责折叠展示（默认按
  // AGENT_TASK_COLLAPSE_LINE_COUNT / _CHAR_COUNT 阈值）。
  | { type: "agentTask"; messageId: string; subTasks: SubTask[]; reasoning?: string };

/**
 * useRenderTurnItems - 组合式接口
 * 入参：messages ref + status ref
 * 返回：RenderedItem[] computed
 *
 * Task 7：可选第三个参数 compactionText —— i18n 键解析后的
 * "上下文已自动压缩" 文本。传空串时直接用 CONTEXT_COMPACTION_MARKER
 * 兜底（中文 hardcode），保证 renderTurnItems 在没有 i18n 调用上下文
 * （如 unit test）的场景下也能产出可读的 divider。
 */
export function useRenderTurnItems(
  messages: Ref<Message[]> | ComputedRef<Message[]>,
  status: Ref<AgentStatus> | ComputedRef<AgentStatus>,
  compactionText?: ComputedRef<string> | Ref<string>
): ComputedRef<RenderedItem[]> {
  return computed(() => renderTurnItems(messages.value, status.value, compactionText?.value));
}

/** 累积状态 */
interface OpGroup {
  anchorId: string;
  toolCallIds: string[];
  kinds: string[];
  lastStatus: string | null;
}

interface WebSearchGroup {
  anchorId: string;
  queries: string[];
  toolCallIds: string[];
}

// 8 个合并窗常量（已删除，模板里不直接用）
//   旧代码: const FLUSH_GAP_MS = 800

/**
 * Task 22：agent task 消息的折叠阈值常量。参考
 * codex-web `MessageBlocks.tsx:68-69` 的同名常量。
 *
 *  - AGENT_TASK_COLLAPSE_LINE_COUNT：子任务行数 ≤ 此值时
 *    AgentTaskMessage.vue 默认展开（视为"短列表"）
 *  - AGENT_TASK_COLLAPSE_CHAR_COUNT：所有 description 拼接的
 *    字符数 ≤ 此值时也默认展开；任一条件触发折叠
 *
 * 这两个常量只用于"是否自动展开"决策，不限制 UI 实际渲染
 * 的最大行数/字符数——用户可手动展开查看全部。
 */
export const AGENT_TASK_COLLAPSE_LINE_COUNT = 7;
export const AGENT_TASK_COLLAPSE_CHAR_COUNT = 520;

/**
 * 把 Message.content（string | MessageContentPart[] | undefined）规整为字符串。
 * 仅取首个 text 段；其余类型（image_url / file）以占位符 [附件] 兜底。
 * 缺失或非文本时返回空字符串。
 *
 * 对于 assistant 消息，额外清理 LLM 在文本内容中泄露的工具调用 JSON 前缀
 *（如 `{ "queries":[...], "source_filter":[...], "intent":"nav" }`），
 * 避免用户看到原始工具调用元数据混在回复正文中。
 */
function contentToText(content: string | MessageContentPart[] | undefined, role?: string): string {
  const raw = (() => {
    if (!content) return "";
    if (typeof content === "string") return content;
    for (const part of content) {
      if (part.type === "text" && part.text) return part.text;
    }
    return "";
  })();

  // 仅对 assistant 消息做工具调用 JSON 清理
  if (role === "assistant" && raw) {
    return stripLeadingToolCallJson(raw);
  }
  return raw;
}

/**
 * 清理 assistant 消息中 LLM 泄露的工具调用 JSON 前缀。
 *
 * 某些 LLM 提供商（尤其是 function calling 模式）会在 text_delta 中把
 * 工具调用的参数 JSON 作为响应正文的前缀输出，例如：
 *   { "queries":[""], "source_filter":["file_library"], "intent":"nav" }当前工作区共有以下文件：
 *
 * 本函数检测并剥离这类前缀，保留后续的自然语言/Markdown 正文。
 *
 * 匹配策略：
 *   1. 行首的 `{ "key": ... }` JSON 对象（可能跨多行）
 *   2. JSON 后紧跟非空白字符（说明 JSON 是前缀而非独立内容）
 *   3. JSON 内包含已知工具调用 key 名（queries / source_filter / intent /
 *      path / command / file_path 等）
 */
const TOOL_CALL_JSON_KEYS = new Set([
  "queries",
  "source_filter",
  "intent",
  "path",
  "command",
  "file_path",
  "directory",
  "pattern",
  "query",
  "operation",
  "arguments",
  "tool_name",
  "name",
]);

export function stripLeadingToolCallJson(text: string): string {
  const trimmed = text.trimStart();
  if (!trimmed.startsWith("{")) return text;

  // 尝试从行首解析一个完整 JSON 对象
  let depth = 0;
  let jsonEnd = -1;
  for (let i = 0; i < trimmed.length; i++) {
    const ch = trimmed[i];
    if (ch === "{") depth++;
    else if (ch === "}") {
      depth--;
      if (depth === 0) {
        jsonEnd = i + 1;
        break;
      }
    }
    // 简单跳过字符串字面量（不处理转义引号——够用即可）
    else if (ch === '"') {
      const close = trimmed.indexOf('"', i + 1);
      if (close === -1) break;
      i = close;
    }
  }

  if (jsonEnd <= 0) return text; // 不完整 JSON，不处理

  const candidate = trimmed.slice(0, jsonEnd);
  let hasToolKey = false;
  // 快速扫描：JSON 中是否包含已知工具调用 key
  const keyRegex = /"([a-z_]+)"\s*:/g;
  let match: RegExpExecArray | null;
  while ((match = keyRegex.exec(candidate)) !== null) {
    if (TOOL_CALL_JSON_KEYS.has(match[1])) {
      hasToolKey = true;
      break;
    }
  }

  if (!hasToolKey) return text; // 不是工具调用 JSON

  // JSON 后是否有非空白内容？有 → 剥离前缀；无 → 整段都是 JSON，保留原样
  const rest = trimmed.slice(jsonEnd).trimStart();
  if (rest.length > 0) {
    return rest;
  }

  return text;
}

/**
 * renderTurnItems 纯函数
 * 1. 单条 user / assistant / error / reasoning → 直接产出 item
 * 2. 累积连续 toolCall（command/fileChange/readOnly）→ operationGroup
 * 3. 累积连续 web_search 类的 toolCall → webSearchGroup
 * 4. 流式结束或超过 FLUSH_GAP_MS 时强制 flush
 *
 * 这里按"消息 + 该消息内 toolCalls 顺序遍历"的方式处理，
 * 不在 Message 上做时间戳假设；用 opGroup 之间的"非 tool_call 事件"作为 flush 触发点。
 *
 * Task 7：在遍历 messages 时检测 role='system' + content=CONTEXT_COMPACTION_MARKER
 * 的合成消息，产出 { type: 'compaction' } 项。compactionText 可选
 * （i18n 解析后的 "上下文已自动压缩" 文本），未传时回退到 marker
 * 本身，保证测试 / 旧调用方也能正常工作。
 */
export function renderTurnItems(messages: Message[], status: AgentStatus, compactionText?: string): RenderedItem[] {
  const out: RenderedItem[] = [];
  let opGroup: OpGroup | null = null;
  let webGroup: WebSearchGroup | null = null;
  // Task 7：compaction 分隔线的展示文本。优先用 i18n 解析结果
  // （传进来的 compactionText），缺失时回退到 marker 本身
  // （"上下文已自动压缩"），保证未配置 i18n 也能渲染。
  const effectiveCompactionText = compactionText && compactionText.trim().length > 0 ? compactionText : CONTEXT_COMPACTION_MARKER;

  function flushOpGroup(force: boolean) {
    if (!opGroup) return;
    if (!force && (opGroup.lastStatus === "running" || opGroup.lastStatus === "pending")) {
      return;
    }
    out.push({
      type: "operationGroup",
      messageId: opGroup.anchorId,
      toolCallIds: opGroup.toolCallIds.slice(),
      forceComplete: force,
    });
    opGroup = null;
  }

  function flushWebGroup() {
    if (!webGroup) return;
    out.push({
      type: "webSearchGroup",
      messageId: webGroup.anchorId,
      queries: webGroup.queries.slice(),
      toolCallIds: webGroup.toolCallIds.slice(),
    });
    webGroup = null;
  }

  function tryAppendToGroup(tc: ToolCall) {
    if (tc.kind === "webSearch") {
      // webSearch 不属于 operationGroup
      return false;
    }
    if (!opGroup) {
      opGroup = { anchorId: tc.id, toolCallIds: [], kinds: [], lastStatus: null };
    }
    opGroup.toolCallIds.push(tc.id);
    opGroup.kinds.push(tc.kind);
    opGroup.lastStatus = tc.status;
    return true;
  }

  let messageIndex = 0;
  // Task 27：记录已通过 eventLog 时间轴模式处理的消息索引。
  // 这些消息的 tool_calls 已在 renderAgentFlow 中逐条产出 operation（内嵌结果 slot），
  // 必须跳过下面（行 ~593）的旧累积循环，否则每条工具调用会被渲染两份。
  const eventLogHandledIndices = new Set<number>();

  for (const msg of messages) {
    const idx = messageIndex++;
    // ── 主消息体 ──────────────────────────────────────────
    if (msg.role === "user") {
      flushOpGroup(true);
      flushWebGroup();
      out.push({ type: "user", messageId: `u-${idx}`, text: contentToText(msg.content) });
      // user 消息携带 error → 紧跟一个错误项（每条消息独立错误状态）
      if (msg.error) {
        out.push({ type: "error", messageId: `uerr-${idx}`, text: msg.error, messageIndex: idx });
      }
    } else if (msg.role === "system" && msg.content === CONTEXT_COMPACTION_MARKER) {
      // Task 7：上下文自动压缩分隔线
      //
      // 角色为 'system' 且 content 严格等于 CONTEXT_COMPACTION_MARKER
      // 的合成消息，由 useAgent 在收到后端 EventCompaction 事件时
      // 插入 messages 列表；这里是它唯一的合法渲染出口。
      //
      // 设计要点：
      //  - 紧跟前一条消息尾部插入，不打断 operationGroup / webSearchGroup
      //    累积（先 flush 完旧 group，再 push compaction 项，下一轮
      //    tool_call 重新开 group）
      //  - 不进入 assistant 的 content / reasoning 渲染路径，避免
      //    AssistantMessage.vue 把 "上下文已自动压缩" 当成对话内容
      //    渲染成 markdown 气泡
      //  - text 字段直接用 effectiveCompactionText（i18n 解析后），
      //    让 ContextCompactionDivider 不必再调 i18n
      flushOpGroup(true);
      flushWebGroup();
      out.push({
        type: "compaction",
        messageId: `c-${idx}`,
        text: effectiveCompactionText,
      });
    } else if (msg.role === "agent_task") {
      // Task 22：agent task 消息。content 形如
      //   {"subTasks":[{id,status,description}, ...], "reasoning":"..."}
      // 解析失败时退化为空 subTasks（UI 渲染"无子任务"提示，不崩溃）。
      // 紧跟前一条消息尾部插入，先 flush 完旧 group（防止 agent task
      // 跟 operationGroup 串在一起——它们语义独立）。
      flushOpGroup(true);
      flushWebGroup();
      const parsed = parseAgentTaskContent(msg.content);
      out.push({
        type: "agentTask",
        messageId: `at-${idx}`,
        subTasks: parsed.subTasks,
        ...(parsed.reasoning ? { reasoning: parsed.reasoning } : {}),
      });
    } else if (msg.role === "assistant") {
      flushOpGroup(true);
      flushWebGroup();
      const streaming = !!msg.isStreaming && status === "streaming";
      // 错误优先
      if (msg.tool_results.length > 0) {
        const lastErr = [...msg.tool_results].reverse().find(r => r.is_error);
        if (lastErr && !streaming) {
          out.push({ type: "error", messageId: `a-${idx}`, text: lastErr.result, messageIndex: idx });
          continue;
        }
      }

      // ── Task 27：agent 流式时间轴渲染（eventLog 模式）────────
      // 当 Message 存在 eventLog 时，按事件到达顺序交错产出：
      //   text → operation(tool_call, 内嵌结果) → text → ...
      // 不再聚合为"一大段文本 + 一个折叠 group"。
      //
      // 无 eventLog 时（旧消息 / 非 agent 场景）走 fallback 路径（原逻辑）。
      const rawContentText = contentToText(msg.content, "assistant");
      const displayText = (msg.tool_calls?.length ?? 0) > 0 ? stripToolCallJSON(rawContentText) : rawContentText;

      if (msg.eventLog && msg.eventLog.length > 0 && msg.tool_calls.length > 0) {
        // 时间轴模式：把 content 按 eventLog 中的 text 条目数均分，
        // 每个 text 段与紧随其后的 tool_call / tool_result 配对。
        renderAgentFlow(out, msg, idx, displayText, streaming);
        eventLogHandledIndices.add(idx); // 标记：此消息的 tool_calls 已由时间轴处理完
      } else {
        // fallback：旧聚合模式（向后兼容）
        if (displayText && displayText.trim().length > 0) {
          out.push({
            type: "assistantText",
            messageId: `a-${idx}`,
            text: displayText,
            streaming,
          });
        }
      }

      if (msg.reasoning && msg.reasoning.trim().length > 0) {
        out.push({
          type: "reasoning",
          messageId: `r-${idx}`,
          text: msg.reasoning,
          streaming: streaming && !msg.content,
        });
      }
    }

    // ── tool_calls 累积 ──────────────────────────────────
    // Task 27：eventLog 时间轴模式已处理过的消息，跳过旧累积循环（避免双重渲染）
    if (!eventLogHandledIndices.has(idx)) {
      for (const tc of msg.tool_calls || []) {
        // approval 单独成项
        if (tc.status === "pending" && tc.needsConfirm) {
          flushOpGroup(true);
          flushWebGroup();
          out.push({ type: "approval", toolCallId: tc.id, messageId: tc.id });
          continue;
        }

        // plan tool (write_todos) 单独渲染为 plan block，**不能**
        // 与 operationGroup / webSearchGroup 合并——plan block
        // 是顶层 plan 视图，必须有独立的空间和生命周期。
        if (tc.kind === "plan") {
          flushOpGroup(true);
          flushWebGroup();
          const streaming = !!msg.isStreaming && status === "streaming" && (tc.status === "pending" || tc.status === "running");
          const todos = parsePlanArgs(tc.args);
          out.push({
            type: "plan",
            messageId: tc.id,
            toolCallId: tc.id,
            todos,
            streaming,
          });
          continue;
        }

        if (tc.kind === "webSearch") {
          flushOpGroup(true);
          if (!webGroup) webGroup = { anchorId: tc.id, queries: [], toolCallIds: [] };
          webGroup.toolCallIds.push(tc.id);
          try {
            const args = JSON.parse(tc.args) as Record<string, unknown>;
            if (typeof args.query === "string") webGroup.queries.push(args.query);
          } catch {
            // ignore
          }
          continue;
        }
        if (tc.kind === "readOnly") {
          // webSearch 走 webSearch 合并窗；readOnly 单独走 operationGroup
          flushWebGroup();
          tryAppendToGroup(tc);
          continue;
        }

        flushWebGroup();
        tryAppendToGroup(tc);
      }
    } // end eventLogHandledIndices skip

    // ── tool_results 错误回灌（仅当本条消息内已有内容）──
    if (msg.tool_results && msg.tool_results.length > 0 && !msg.content) {
      const lastErr = [...msg.tool_results].reverse().find(r => r.is_error);
      if (lastErr) {
        flushOpGroup(true);
        flushWebGroup();
        out.push({ type: "error", messageId: `err-${idx}`, text: lastErr.result, messageIndex: idx });
      }
    }
  }

  // 收尾：流式状态保留未完结 group（让 UI 持续显示 running 状态）
  // 非流式时强制 flush 全部
  if (status !== "streaming") {
    flushOpGroup(true);
    flushWebGroup();
  } else {
    flushOpGroup(false);
    flushWebGroup();
  }

  return out;
}

/**
 * parsePlanArgs 解析 plan tool (write_todos) 的 args JSON。
 *
 * 接受以下两种形状：
 *  1. 完整 schema: `{"todos":[{"id":"1","status":"in_progress","content":"..."}, ...]}`
 *  2. 兼容退化: `[{"id":"1",...}]` （裸数组，LLM 偶尔会省略外层对象）
 *
 * 解析失败时返回空数组——UI 会渲染 "planEmpty" 提示而不是崩溃。
 * 任何不是字符串的字段会被丢弃（防御性，避免 null.length 这类 bug）。
 */
export function parsePlanArgs(args: string): PlanTodo[] {
  if (!args || typeof args !== "string") return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(args);
  } catch {
    return [];
  }
  let rawList: unknown[];
  if (Array.isArray(parsed)) {
    rawList = parsed;
  } else if (parsed && typeof parsed === "object" && Array.isArray((parsed as { todos?: unknown }).todos)) {
    rawList = (parsed as { todos: unknown[] }).todos;
  } else {
    return [];
  }
  const out: PlanTodo[] = [];
  for (const item of rawList) {
    if (!item || typeof item !== "object") continue;
    const t = item as { id?: unknown; status?: unknown; content?: unknown };
    if (typeof t.id !== "string") continue;
    if (typeof t.content !== "string") continue;
    const status = typeof t.status === "string" ? t.status : "pending";
    out.push({ id: t.id, status, content: t.content });
  }
  return out;
}

/**
 * Task 22：解析 agent_task 消息的 content JSON。
 *
 * 接受以下形状：
 *   {
 *     "subTasks": [{ id, status, description }, ...],
 *     "reasoning"?: string
 *   }
 *
 * 解析失败 / 字段缺失时退化为空 subTasks + 无 reasoning——UI
 * 渲染"无子任务"提示而不崩溃（与 parsePlanArgs 失败时返回 [] 的
 * 防御策略一致）。status 字段非合法四态时降级为 'pending'，避免
 * 后端版本演进时引入新状态导致前端空白。
 */
export function parseAgentTaskContent(content: string | unknown): { subTasks: SubTask[]; reasoning?: string } {
  let parsed: unknown;
  if (typeof content === "string") {
    if (!content) return { subTasks: [] };
    try {
      parsed = JSON.parse(content);
    } catch {
      return { subTasks: [] };
    }
  } else {
    parsed = content;
  }
  if (!parsed || typeof parsed !== "object") return { subTasks: [] };
  const obj = parsed as { subTasks?: unknown; reasoning?: unknown };
  const rawList = Array.isArray(obj.subTasks) ? obj.subTasks : [];
  const out: SubTask[] = [];
  for (const item of rawList) {
    if (!item || typeof item !== "object") continue;
    const t = item as { id?: unknown; status?: unknown; description?: unknown };
    if (typeof t.id !== "string") continue;
    if (typeof t.description !== "string") continue;
    const status: SubTask["status"] =
      t.status === "pending" || t.status === "in_progress" || t.status === "completed" || t.status === "failed" ? t.status : "pending";
    out.push({ id: t.id, status, description: t.description });
  }
  const reasoning = typeof obj.reasoning === "string" ? obj.reasoning : undefined;
  return reasoning ? { subTasks: out, reasoning } : { subTasks: out };
}
