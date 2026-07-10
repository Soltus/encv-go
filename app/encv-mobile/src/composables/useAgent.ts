/**
 * useAgent - Vue 复合式：与 Go agent 服务进行 SSE 流式对话
 *
 * 核心能力：
 * 1. reactive<Message[]> 消息列表
 * 2. processSSE 解析 6 种事件类型
 * 3. 4-决策 ConfirmTool（accept / accept_for_session / decline / cancel）
 * 4. 启动时自动续传（基于 localStorage）
 * 5. 持久化（每次事件后写 localStorage `agent:session:{sessionId}`）
 *
 * API 路径（由 preview-gateway :16666 转发）：
 *   - POST /agent-api/api/chat    发起对话（SSE）
 *   - POST /agent-api/api/resume  断点续传（SSE）
 *   - POST /agent-api/api/confirm 4-决策确认（SSE）
 *
 * 文件拆分（2026-07-02）：
 *   - useAgent.ts（本文件）：composable 主体（useAgent 函数）
 *   - useAgent_helpers.ts：类型 / 常量 / 解析器 / 辅助函数
 *   下面 re-export 所有 helper 以保持向后兼容（`import { ... } from '@/composables/useAgent'`）
 */

// 完整 import helpers（主体使用）+ re-export（保持向后兼容）
import {
  // 类型（按需）
  type AgentStatus,
  appendSequencedChunk,
  buildHttpError,
  CONTEXT_COMPACTION_MARKER,
  type Decision,
  extractUserTitle,
  generateSessionId,
  getAgentBase,
  MAX_TRACKED_REALTIME_SEQUENCES,
  type Message,
  type MockBranch,
  type MockPreset,
  type MockRoundState,
  parseCompactionData,
  // 工具函数
  parseContentDelta,
  parseStreamStatusData,
  parseToolCallData,
  parseToolResultData,
  parseToolStatus,
  type SessionMeta,
  STORAGE_PREFIX,
  type ToolCall,
} from "./useAgent_helpers";

// 重新导出所有 helpers 以保持向后兼容（`import { ... } from '@/composables/useAgent'`）
export * from "./useAgent_helpers";

import { computed, ref } from "vue";
import { showToast } from "@/composables/useToast";
import { getAgentApiBaseContext, shouldSendAGUIHeader } from "./useAgentApiBase";
// SSE 解析器 + 事件分发（拆分到 useAgentStream.ts，通过 ctx 访问闭包状态）
import { type AgentStreamContext, createAgentStream } from "./useAgentStream";
import { type Attachment, type MessageContentPart, serializeAttachments } from "./useAttachments";
import { useContextUsage } from "./useContextUsage";
import { getDeviceIdSync } from "./useDeviceId";

export function useAgent() {
  const messages = ref<Message[]>([]);
  const status = ref<AgentStatus>("idle");
  const lastError = ref<string>("");
  // 错误语义码：后端 buildHttpError 在 Error 上挂 .code 字段（'no_api_key' / 'upstream_error' / 'unknown'），
  // 前端 chat UI 据此做分支判断（如展示"去 AI 设置"按钮）。
  // 与 lastError 的区别：lastError 是给人看的字符串，lastErrorCode 是给程序判断的结构化字段。
  const lastErrorCode = ref<"" | "no_api_key" | "upstream_error" | "invalid_json" | "unknown">("");
  const lastUserInput = ref<string>("");
  const sessions = ref<SessionMeta[]>([]);
  const currentSessionId = ref<string>("");
  /**
   * Task 11 (Steer / Queue)：跟踪已通过「排队下一条」按钮提交、
   * 但服务端尚未开始处理的 user 消息。pendingMessages 数组的元素
   * 是 messages.value 中对应 user 消息的引用（同一对象），这样
   * UI 可以直接从 pendingMessages 取出 text 渲染「已排队：xxx」
   * 提示，并在服务端开始处理时（首个 text_delta 到达）实时清除
   * 对应条目的 pending 标记。
   */
  const pendingMessages = ref<Message[]>([]);
  /** 本地已处理事件计数（与 lastEventId 解耦：eventOffset 永远递增，lastEventId 由 SSE id 行决定） */
  let eventOffset = 0;
  /**
   * Context 图标：从 /api/agent/context-usage 周期拉取的实时数据
   * （tokens/window/percent、todos、referencedFiles、compactions）
   * 由 ContextIcon / ContextPopover 直接读取。
   *
   * **不自动 start**：测试中 useAgent() 不应触发任何 fetch。
   * AgentChat 视图在 onMounted 时调 start()，onUnmounted 时调 stop()。
   */
  const contextUsage = useContextUsage({
    sessionId: currentSessionId,
    status,
  });
  /**
   * SSE 标准 Last-Event-ID：服务端为每个事件分配的全局递增 ID。
   * 用于断点续传——前端解析 `id: N` 行维护此字段，resume() 时回传给后端。
   * 0 = 尚未收到任何事件 id。
   */
  let lastEventId = 0;
  let abortController: AbortController | null = null;

  // ─── Task 3: tool_call 状态机 — 30s 无响应超时保护 ────────────────
  // 当 tool_call 创建后 30s 内未收到 tool_result（且未进入 failed/cancelled
  // 终态），自动标记为 failed + errorCode='TIMEOUT'。这是 user 反馈"工具
  // 卡住没有任何反馈"的根因——之前前端没有超时机制，spinner 转 60s+ 仍然
  // 停在 running。timer 必须在 tool_result 到达或 tool_call 进入终态时清除
  // （clearToolCallTimeout），否则会泄漏。
  const TOOL_CALL_TIMEOUT_MS = 30_000;
  const toolCallTimeoutMap = new Map<string, ReturnType<typeof setTimeout>>();

  function clearToolCallTimeout(toolCallId: string): void {
    const t = toolCallTimeoutMap.get(toolCallId);
    if (t !== undefined) {
      clearTimeout(t);
      toolCallTimeoutMap.delete(toolCallId);
    }
  }

  function armToolCallTimeout(toolCallId: string): void {
    clearToolCallTimeout(toolCallId); // 防重入
    const t = setTimeout(() => {
      toolCallTimeoutMap.delete(toolCallId);
      // 找到对应 tool_call，标 failed
      for (const msg of messages.value) {
        const tc = msg.tool_calls.find(c => c.id === toolCallId);
        if (!tc) continue;
        // 单向：终态（failed/cancelled/success）不再覆盖
        if (tc.status === "failed" || tc.status === "cancelled" || tc.status === "success") {
          break;
        }
        tc.status = "failed";
        tc.errorCode = "TIMEOUT";
        tc.errorMessage = "工具执行超过 30s 无响应";
        tc.finishedAt = Date.now();
        console.warn(`[useAgent] tool_call ${toolCallId} (${tc.name}) timed out after ${TOOL_CALL_TIMEOUT_MS}ms`);
        break;
      }
    }, TOOL_CALL_TIMEOUT_MS);
    toolCallTimeoutMap.set(toolCallId, t);
  }

  // ─── Server Instance + Sequence 去重（Task 4） ────────────────────────
  // 后端 /api/health 返回 process-wide 唯一的 serverInstanceId。同一进程
  // 启动期间它恒定；进程重启（OS 分配新 PID / 启动时间不同）就会变化。
  // 前端每次拉取发现 instance 变化时，必须清空 seenSequences——因为新进程
  // 的 SSE sequence 编号可能与旧进程"撞号"，不复用旧的去重集合会造成
  // 真实事件被错误丢弃。
  /** 当前进程已知的 serverInstanceId；空串 = 尚未拉取过 /api/health */
  let currentServerInstance = "";
  /** 已见 SSE sequence 编号集合；超过 MAX_TRACKED_REALTIME_SEQUENCES 时按 FIFO 驱逐 */
  const seenSequences = new Set<number>();
  /** 配合 Set 实现的 FIFO 驱逐顺序表 */
  const seenSequencesOrder: number[] = [];

  // 模型/温度从 localStorage 读取（AgentChat 顶部 UI 选择会同步写入这里）
  const MODEL_STORAGE_KEY = "encv-agent-selected-model";
  const TEMP_STORAGE_KEY = "encv-agent-temperature";
  const activeModel = ref<string>(
    (() => {
      try {
        return localStorage.getItem(MODEL_STORAGE_KEY) || "gpt-4o-mini";
      } catch {
        return "gpt-4o-mini";
      }
    })()
  );
  /**
   * API 返回的默认模型（来自 /api/models 的 defaultModel 字段）。
   * AgentChat 在 fetchModels 成功后通过 setApiDefaultModel() 写入，
   * newSession 时用于重置 activeModel。
   */
  const apiDefaultModel = ref<string>("");
  /** 当前会话的创建时间戳（毫秒），用于持久化和历史列表排序 */
  const sessionCreatedAt = ref<number>(Date.now());
  // 之前怀疑 gpt-4o-mini 在 gptgod 代理下不发 tools，临时加了 safeModel
  // 白名单做硬编码降级。实测 gpt-4o-mini 完全能用工具（3 轮 list_mounts →
  // list_files → 输出 4 个目录），根因不在模型上。回退这段逻辑，
  // 直接用 activeModel —— 真正的问题要去看后端日志 / 实际请求体。
  const activeTemperature = ref<number>(
    (() => {
      try {
        const v = localStorage.getItem(TEMP_STORAGE_KEY);
        const n = v == null ? 0.7 : Number(v);
        return Number.isFinite(n) ? n : 0.7;
      } catch {
        return 0.7;
      }
    })()
  );

  // ─── Mock 模式检测 ────────────────────────────────────────
  // 后端在 cfg.Agent.MockMode != "off" 时启用 mock 模式：
  //   - HTTP response header: X-Mock-Mode: builtin|custom
  //   - HTTP response header: X-Mock-Scenario: <scenario_id>
  //   - SSE 首个 stream_start 事件 data: { mock: true, scenario: "..." }
  // 前端拿到任一信号就置 isMockMode=true，让 AgentChat 顶部展示"🧪 模拟"badge。
  const isMockMode = ref(false);
  const mockScenario = ref<string>("");

  // 调试开关：URL 带 ?debug=agent 时为 true，强制显示 AgentDebugPanel。
  // 浏览器端用 window.location，SSR 时降级为 false。
  const isDebugAgent = computed(() => {
    if (typeof window === "undefined") return false;
    try {
      return new URLSearchParams(window.location.search).get("debug") === "agent";
    } catch {
      return false;
    }
  });

  // ─── 原始 SSE 事件日志（调试用：AgentDebugPanel ⑦ 区展示） ──
  // 每个进 processSSE 的 event 都追加一条（含 type + data 摘要 + 时间戳）。
  // 不自动清理——用户手动"清空"或新建会话时重置。
  const rawSSEEvents = ref<{ ts: string; type: string; dataSummary: string; seq?: number | null }[]>([]);

  /** 追加一条原始事件到日志（最多保留 200 条防内存爆炸） */
  function pushRawEvent(type: string, dataSummary: string, seq?: number | null) {
    rawSSEEvents.value.push({
      ts: new Date().toISOString().slice(11, 23), // HH:MM:SS.mmm
      type,
      dataSummary,
      seq: seq ?? null,
    });
    if (rawSSEEvents.value.length > 200) {
      rawSSEEvents.value = rawSSEEvents.value.slice(-150);
    }
  }

  // ─── Mock 模式预设按钮（覆盖在输入框上方，由 mock_presets 事件驱动） ──
  // 三个 ref 状态：
  //   - mockPresets：当前激活的预设 chip 列表
  //   - mockPresetsPhase：当前阶段（initial / after_round_2 / ...，调试用）
  //   - mockPresetsScenario：当前预设归属的 scenario ID（调试用）
  // 后端会在 stream_start 之后立刻推一次 mock_presets 事件初始化，
  // 并在 stream_end 时推 mock_presets_clear 清空。
  // 中级/高级剧本可在 mid-scenario step 内再次推 mock_presets 实现"随进度更新"。
  const mockPresets = ref<MockPreset[]>([]);
  const mockPresetsPhase = ref<string>("");
  const mockPresetsScenario = ref<string>("");

  // ─── v2 多轮/分支剧本（参考 .trae/specs/agent-tools-scenarios-v2/spec.md） ───
  // 与上面 mockPresets 的区别：
  //   - mockPresets：scenario 顶层覆盖在输入框上方的"快捷入口"，与剧本进度无强关联
  //   - mockBranchChoices：剧本 mid-step 暂停时推的选项 chip，必须等待用户
  //     点击 / 键入才能继续；AgentChat 据此把 v-if 打开并禁用 send 按钮
  //   - mockBranchPrompt：当前 step 的提问文案（"请选择操作："/"你想改名哪些字段？"）
  //   - mockRoundState：当前 round 进度 + 阶段（驱动 MockBranchChoiceBar header）
  //   - mockScenarioPaused：派生 computed；phase 在 awaiting_user_input 或
  //     awaiting_branch_choice 时为 true。AgentChat 用它控制 MockBranchChoiceBar
  //     显隐 + send 按钮 disabled
  //   - currentMockScenario：当前激活的 scenario ID（pickMockBranch /
  //     sendMockRoundResponse 必须带上，供后端 MockEngineV2 知道是哪个剧本）
  // 后端 stream_end 时（或者推 mock_branch_choice_clear / mock_round_state_clear
  // 显式清空时）本 composable 把所有 ref 复位。
  const mockBranchChoices = ref<MockBranch[]>([]);
  const mockBranchPrompt = ref<string>("");
  const mockRoundState = ref<MockRoundState | null>(null);
  const currentMockScenario = ref<string>("");

  const mockScenarioPaused = computed(() => {
    const phase = mockRoundState.value?.phase;
    return phase === "awaiting_user_input" || phase === "awaiting_branch_choice";
  });

  // ─── Task 3: tool_call 状态机派生 computed ────────────────────────
  /**
   * 正在运行 / 等待中的 tool_call 扁平列表。
   *  - running：tool_status 收到 'running' 后保持
   *  - pending：刚 tool_call 事件到达、还没收到 tool_status 'running'
   * 命中这两个状态的 tool_call 全部列入，供 AgentChat footer 渲染
   * 「🔄 工具执行中…」指示 + OperationCard 渲染脉冲动画。
   */
  const runningTools = computed(() => {
    const list: ToolCall[] = [];
    for (const m of messages.value) {
      for (const tc of m.tool_calls) {
        if (tc.status === "running" || tc.status === "pending") {
          list.push(tc);
        }
      }
    }
    return list;
  });

  /** 是否有任意 tool_call 仍在运行 / 等待中（UI 状态指示用） */
  const hasRunningTool = computed(() => runningTools.value.length > 0);

  /**
   * 当前会话中所有 tool_call 的扁平引用（按到达顺序）。
   * 供 UI 一次性遍历渲染——避免在模板里再嵌套 .tool_calls。
   */
  const allToolCalls = computed(() => {
    const list: ToolCall[] = [];
    for (const m of messages.value) {
      for (const tc of m.tool_calls) {
        list.push(tc);
      }
    }
    return list;
  });

  /**
   * 多轮剧本中点 chip 继续：把 chip id 当作 userText 送回后端 Resume。
   * 关键点：mode='mock_resume' —— 后端 MockEngineV2 据此区分"新 session 启动"
   * 和"在暂停点恢复"，并用 currentMockScenario 找到正确的剧本状态机。
   */
  function pickMockBranch(branchId: string): void {
    if (typeof branchId !== "string" || branchId.length === 0) {
      console.debug("[useAgent] pickMockBranch: invalid branchId", branchId);
      return;
    }
    if (!currentMockScenario.value) {
      console.debug("[useAgent] pickMockBranch: no currentMockScenario — dropped");
      return;
    }
    if (status.value === "streaming" || status.value === "confirming") {
      console.debug("[useAgent] pickMockBranch: ignored (busy)");
      return;
    }
    console.debug("[useAgent] pickMockBranch →", branchId, "| scenario =", currentMockScenario.value);
    void send(branchId, { mode: "mock_resume", scenario: currentMockScenario.value });
  }

  /**
   * 多轮剧本中键入文本继续：等价于 pickMockBranch，但 userText 是用户键入的。
   * 用于：chip 列表里没覆盖到的细粒度控制（比如用正则编辑 metadata 字段）。
   */
  function sendMockRoundResponse(userText: string): void {
    if (typeof userText !== "string" || userText.trim().length === 0) {
      console.debug("[useAgent] sendMockRoundResponse: empty text — dropped");
      return;
    }
    if (!currentMockScenario.value) {
      console.debug("[useAgent] sendMockRoundResponse: no currentMockScenario — dropped");
      return;
    }
    if (status.value === "streaming" || status.value === "confirming") {
      console.debug("[useAgent] sendMockRoundResponse: ignored (busy)");
      return;
    }
    console.debug("[useAgent] sendMockRoundResponse →", userText.slice(0, 40), "| scenario =", currentMockScenario.value);
    void send(userText.trim(), { mode: "mock_resume", scenario: currentMockScenario.value });
  }

  // ─── Mock 模式控制（用户从 AgentChat 顶栏的"🧪 模拟"badge 切换） ─────
  // 字段语义与后端 cfg.Agent.MockMode 一一对应：
  //   - 'off'     → 真实 LLM 调用（默认）
  //   - 'builtin' → 内置 12 个剧本
  //   - 'custom'  → config.user.json 中 agent_settings.mock_scenarios
  //
  // 修改后会立刻调 PUT /api/config 持久化到后端 config.user.json，
  // 下次会话起立即生效（无需重启后端）。
  type MockMode = "off" | "builtin" | "custom";
  const currentMockMode = ref<MockMode>("off");

  /**
   * 模拟模式预设 chip 点击：直接把 preset.userText 喂给 send()。
   * 与用户在输入框里打字的区别：
   *   - 不会先填到 input.value（用户点击 chip 的预期是"立即发"，不是"先看再改"）
   *   - 会触发和正常 send 完全一样的流程（后端按 userText 关键词重新匹配 scenario）
   * 用 mode='start'（而非 'steer' / 'queue'）：mock 模式是单次请求流。
   */
  async function pickMockPreset(preset: MockPreset): Promise<void> {
    if (!preset || typeof preset.userText !== "string" || preset.userText.length === 0) {
      console.debug("[useAgent] pickMockPreset: invalid preset", preset);
      return;
    }
    // 状态检查：跟 send 保持一致 —— 正在 streaming/confirming 时丢弃
    if (status.value === "streaming" || status.value === "confirming") {
      console.debug("[useAgent] pickMockPreset: ignored (busy)");
      return;
    }
    console.debug("[useAgent] pickMockPreset →", preset.id, "| userText =", preset.userText);
    await send(preset.userText, { mode: "start" });
  }

  /**
   * 首次进入 AgentChat 时拉取"全局剧本选择器"覆盖在输入框上方。
   * 由 AgentChat.vue 的 onMounted 调用（仅一次）。
   * 后端 mock 模式关闭时返回空 presets → v-if 自然不渲染。
   * 后端流内 mock_presets 事件会**覆盖**本函数写入的 presets。
   */
  async function loadMockPresets(): Promise<void> {
    try {
      const resp = await fetch(`${getAgentBase()}/api/agent/mock/presets`);
      if (!resp.ok) {
        console.debug("[useAgent] loadMockPresets: HTTP", resp.status);
        return;
      }
      const data = (await resp.json()) as {
        scenario?: string;
        phase?: string;
        presets?: MockPreset[];
        mockMode?: string;
      };
      const list = Array.isArray(data.presets) ? data.presets : [];
      // 标准化：scenario_picker 后端 ID 是 "pick_xxx" 风格，但前端 type
      // 要求必须有 id+label+userText。后端已经保证。
      mockPresets.value = list.filter(
        (p): p is MockPreset =>
          !!p && typeof p === "object" && typeof p.id === "string" && typeof p.label === "string" && typeof p.userText === "string"
      );
      mockPresetsPhase.value = String(data.phase ?? "picker");
      mockPresetsScenario.value = String(data.scenario ?? "scenario_picker");
      console.debug(
        "[useAgent] loadMockPresets →",
        mockPresets.value.length,
        "presets | mode =",
        data.mockMode,
        "| phase =",
        mockPresetsPhase.value
      );
    } catch (e) {
      console.debug("[useAgent] loadMockPresets failed:", e);
    }
  }

  async function loadMockMode() {
    try {
      const resp = await fetch(`${getAgentBase()}/api/config`);
      if (!resp.ok) {
        console.debug("[MockMode] fetch /api/config failed: HTTP", resp.status);
        return;
      }
      const cfg = (await resp.json()) as {
        agent_settings?: { mock_mode?: string };
      };
      const m = String(cfg?.agent_settings?.mock_mode ?? "off").toLowerCase();
      currentMockMode.value = m === "builtin" || m === "custom" ? (m as MockMode) : "off";
      // 覆盖式 UI：mock 模式配置开启时，isMockMode 必须**预先**置 true，
      // 否则用户首次进 AgentChat（还没发过消息）chip 不会显示。
      // 后续发消息触发流时，stream_start 事件会再次确认 isMockMode=true。
      isMockMode.value = currentMockMode.value !== "off";
      console.debug("[MockMode] load → mode =", currentMockMode.value, "| isMockMode =", isMockMode.value);
    } catch (e) {
      console.debug("[MockMode] load failed:", e);
    }
  }

  async function setMockMode(mode: MockMode) {
    if (mode === currentMockMode.value) return;
    try {
      // 必须整张 config 一并 PUT（后端会保留非 agent_settings 字段）。
      const getResp = await fetch(`${getAgentBase()}/api/config`);
      if (!getResp.ok) throw new Error(`fetch /api/config → HTTP ${getResp.status}`);
      const cfg = (await getResp.json()) as Record<string, unknown>;
      const agentSettings = (cfg.agent_settings as Record<string, unknown> | undefined) ?? {};
      agentSettings.mock_mode = mode;
      cfg.agent_settings = agentSettings;
      const putResp = await fetch(`${getAgentBase()}/api/config`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(cfg),
      });
      if (!putResp.ok) {
        const errText = await putResp.text();
        throw new Error(`PUT /api/config → HTTP ${putResp.status}: ${errText}`);
      }
      currentMockMode.value = mode;
      // 立即重置 isMockMode：下次 send 时再由 SSE stream_start 事件重新置位
      isMockMode.value = false;
      mockScenario.value = "";
      console.info("[MockMode] set to", mode);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      console.error("[MockMode] setMockMode failed:", msg);
      throw e;
    }
  }

  // ─── 内部辅助 ───────────────────────────────────────────────────────────

  /**
   * 把最后一条正在 streaming 的 assistant 消息标记为流式结束
   * 用于 catch 块 / stop() / 错误恢复场景
   */
  function finalizeLastAssistant(): void {
    for (let i = messages.value.length - 1; i >= 0; i--) {
      if (messages.value[i].role === "assistant" && messages.value[i].isStreaming) {
        messages.value[i].isStreaming = false;
        break;
      }
    }
  }

  /**
   * 拉取后端 /api/health 取 serverInstanceId，并按规则同步到本地状态：
   *   - 拉取失败 / 解析失败 / 字段缺失 → 静默保留 currentServerInstance 不动
   *     （fallback 到空串 + console.warn 提示，但不抛错）
   *   - 拉到新 id 且 ≠ currentServerInstance → 清空 seenSequences（关键！新进程
   *     编号从 1 开始，旧 instance 的 sequence 集合必须丢弃避免误丢事件）
   *   - 拉到与 currentServerInstance 相同的 id → 保持 sequence 去重集合不变
   *
   * 安全：仅在 init() / send() 入口处被调用；不会在 SSE 流过程中触发，
   * 所以不会与正在进行的 sequence 编号检查产生竞态。
   */
  async function refreshServerInstance(): Promise<void> {
    try {
      const response = await fetch(`${getAgentBase()}/api/health`, { method: "GET" });
      if (!response.ok) {
        console.warn("[useAgent] refreshServerInstance: /api/health returned", response.status);
        return;
      }
      const data = await response.json();
      const newId = typeof data?.serverInstanceId === "string" ? data.serverInstanceId : "";
      if (!newId) {
        console.warn("[useAgent] refreshServerInstance: response missing serverInstanceId");
        return;
      }
      if (newId !== currentServerInstance) {
        console.debug("[useAgent] server instance changed:", currentServerInstance || "(none)", "->", newId);
        currentServerInstance = newId;
        // 清空去重状态：旧 instance 的 sequence 编号集合在物理上属于旧进程，
        // 复用它们会导致新进程的真实事件被错误判为重复。
        seenSequences.clear();
        seenSequencesOrder.length = 0;
      }
    } catch (e) {
      // 网络/CORS 等错误：保留旧值不丢业务，但提示一次
      console.warn(
        "[useAgent] refreshServerInstance: fetch /api/health failed:",
        e instanceof Error ? `${e.name}: ${e.message}` : String(e)
      );
    }
  }

  /**
   * 记录一个 sequence 编号为"已见"。超过 MAX_TRACKED_REALTIME_SEQUENCES
   * 时按插入顺序淘汰最老的编号。返回 true 表示"新增"（未见过），false
   * 表示"重复"（已在集合中）。
   */
  function rememberSequence(seq: number): boolean {
    if (seenSequences.has(seq)) {
      return false;
    }
    seenSequences.add(seq);
    seenSequencesOrder.push(seq);
    if (seenSequencesOrder.length > MAX_TRACKED_REALTIME_SEQUENCES) {
      const evict = seenSequencesOrder.shift();
      if (evict !== undefined) seenSequences.delete(evict);
    }
    return true;
  }

  // ─── 持久化 ─────────────────────────────────────────────────────────────

  function saveState() {
    if (!currentSessionId.value) return;
    try {
      const payload = {
        sessionId: currentSessionId.value,
        eventOffset,
        lastEventId,
        messages: JSON.parse(JSON.stringify(messages.value)),
        status: status.value,
        createdAt: sessionCreatedAt.value,
        updatedAt: Date.now(),
      };
      localStorage.setItem(STORAGE_PREFIX + currentSessionId.value, JSON.stringify(payload));
    } catch (e) {
      console.debug("[useAgent] saveState failed:", e);
    }
  }

  function loadState(sessionId: string): {
    sessionId: string;
    eventOffset: number;
    /** 兼容老存档：无 lastEventId 字段时默认为 0 */
    lastEventId?: number;
    messages: Message[];
    status: AgentStatus;
    createdAt?: number;
    updatedAt?: number;
  } | null {
    try {
      const raw = localStorage.getItem(STORAGE_PREFIX + sessionId);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      // 恢复会话创建时间（兼容老存档无此字段）
      if (typeof parsed?.createdAt === "number") {
        sessionCreatedAt.value = parsed.createdAt;
      }
      return parsed;
    } catch (e) {
      console.debug("[useAgent] loadState failed:", e);
      return null;
    }
  }

  function findLatestPersistedSession(): string | null {
    try {
      let latest: { id: string; ts: number } | null = null;
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (!key?.startsWith(STORAGE_PREFIX)) continue;
        const raw = localStorage.getItem(key);
        if (!raw) continue;
        try {
          const parsed = JSON.parse(raw) as { sessionId?: string; messages?: Message[] };
          // 启发式：取有消息的最后一次会话
          if (parsed?.messages && parsed.messages.length > 0 && parsed.sessionId) {
            const ts = parsed.messages[parsed.messages.length - 1]?.content?.length ?? 0;
            if (!latest || ts > latest.ts) {
              latest = { id: parsed.sessionId, ts };
            }
          }
        } catch {
          // skip malformed
        }
      }
      return latest?.id ?? null;
    } catch (e) {
      console.debug("[useAgent] findLatestPersistedSession failed:", e);
      return null;
    }
  }

  /**
   * 扫描 localStorage 中所有 `agent:session:*` 键，返回按 updatedAt 倒序的 session 列表
   * 用于 UI 渲染"会话历史"
   */
  function refreshSessions(): void {
    const list: SessionMeta[] = [];
    try {
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (!key?.startsWith(STORAGE_PREFIX)) continue;
        const raw = localStorage.getItem(key);
        if (!raw) continue;
        try {
          const parsed = JSON.parse(raw) as {
            sessionId?: string;
            messages?: Message[];
            createdAt?: number;
            updatedAt?: number;
          };
          if (!parsed?.sessionId) continue;
          const msgs = parsed.messages || [];
          const firstUser = msgs.find(m => m.role === "user");
          // Task 12：content 可能是 multimodal 数组（带附件的 user 消息），
          //          从中抽出首段 text 元素作为会话标题。
          const title = extractUserTitle(firstUser?.content) || "(空会话)";
          // 优先使用持久化的真实时间戳，兼容老存档无此字段
          const createdAt = parsed.createdAt || Date.now();
          const updatedAt = parsed.updatedAt || createdAt;
          // 轮次 = 用户消息数量（每条 user 消息代表一轮对话）
          const rounds = msgs.filter(m => m.role === "user").length;
          list.push({
            id: parsed.sessionId,
            title,
            createdAt,
            updatedAt,
            messageCount: msgs.length,
            rounds,
          });
        } catch {
          // skip
        }
      }
    } catch (e) {
      console.debug("[useAgent] refreshSessions failed:", e);
    }
    list.sort((a, b) => b.updatedAt - a.updatedAt);
    sessions.value = list;
  }

  /**
   * 切换到指定 session：先 stop 当前流，再加载目标 session 消息
   */
  function switchSession(sessionId: string): void {
    if (sessionId === currentSessionId.value) return;
    if (abortController) {
      abortController.abort();
      abortController = null;
    }
    const saved = loadState(sessionId);
    if (!saved) {
      console.debug("[useAgent] switchSession: no saved state for", sessionId);
      return;
    }
    currentSessionId.value = saved.sessionId;
    eventOffset = saved.eventOffset;
    messages.value = saved.messages.map(m => ({ ...m }));
    status.value = "idle";
    lastError.value = "";
    lastErrorCode.value = "";
    saveState();
  }

  /**
   * 创建新 session 并切到它（不删除原 session，留作历史）
   */
  function newSession(): void {
    if (abortController) {
      abortController.abort();
      abortController = null;
    }
    if (currentSessionId.value && messages.value.length > 0) {
      // 当前会话已存在消息 → 持久化保留作为历史
      saveState();
    }
    currentSessionId.value = generateSessionId();
    eventOffset = 0;
    lastEventId = 0;
    messages.value = [];
    status.value = "idle";
    lastError.value = "";
    lastUserInput.value = "";
    sessionCreatedAt.value = Date.now();
    // 如果 API 返回了默认模型，新会话时重置为该默认模型
    if (apiDefaultModel.value) {
      activeModel.value = apiDefaultModel.value;
    }
    saveState();
    refreshSessions();
  }

  /**
   * 删除一个 session（不可恢复）
   */
  function deleteSession(sessionId: string): void {
    try {
      localStorage.removeItem(STORAGE_PREFIX + sessionId);
    } catch {
      // ignore
    }
    if (sessionId === currentSessionId.value) {
      currentSessionId.value = "";
      messages.value = [];
      eventOffset = 0;
      lastEventId = 0;
    }
    refreshSessions();
  }

  // ─── SSE 解析器（拆分到 useAgentStream.ts） ───────────────────────────────
  // 闭包状态通过 AgentStreamContext 显式传入 createAgentStream，行为与原实现一致。
  const streamCtx: AgentStreamContext = {
    messages,
    status,
    lastError,
    lastErrorCode,
    getLastEventId: () => lastEventId,
    setLastEventId: n => {
      lastEventId = n;
    },
    bumpEventOffset: () => {
      eventOffset++;
    },
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
  };
  const { processSSE, processLegacySSE, processAGUISSEWithHandlers, handleAgentEvent } = createAgentStream(streamCtx);
  // ─── 公共 API ───────────────────────────────────────────────────────────

  /**
   * 发送用户消息，发起对话
   *
   * Task 11 (Steer / Queue)：新增 `mode` 选项，支持三种发送模式：
   *   - "start"  默认行为：常规发起一轮新 turn，blocking 等待 SSE 流。
   *   - "steer"  与 start 等价的 server-side 行为，语义上标记"修正当前
   *              turn"。在 UI 上用「引导当前」按钮触发，行为对用户透明。
   *   - "queue"  排队下一条：仅在 status='streaming' 时合法。消息被
   *              加入 pendingMessages 队列，立即在 messages.value 中
   *              渲染（带 pending 标记），同时 POST /api/chat with
   *              mode='queue'。服务端在当前 turn 完全结束后由 drain
   *              hook 启动新一轮 Chat，结果通过现有 SSE 连接回流。
   *
   * Task 12：可选接受 `attachments`（图片 / 文件）。如果提供，会和 text
   * 一起被编码成 OpenAI multimodal content 数组塞进 Message.content
   * （参见 useAttachments.serializeAttachments）。
   */
  async function send(
    text: string,
    options?: { mode?: "start" | "steer" | "queue" | "mock_resume"; scenario?: string; attachments?: Attachment[] }
  ): Promise<void> {
    const mode = options?.mode ?? "start";
    if (mode === "queue") {
      await sendQueued(text, options?.attachments ?? []);
      return;
    }

    if (status.value === "streaming" || status.value === "confirming") {
      console.debug("[useAgent] send: ignored (busy)");
      return;
    }

    // 关闭上一轮的错误条
    lastError.value = "";
    lastErrorCode.value = "";
    lastUserInput.value = text;

    // Task 4：先拉取 /api/health 取 serverInstanceId。每次 send 都刷一次——
    // 开销可忽略（一次 GET，且 health 不带 SSE body），但能在"中途服务器
    // 重启"时立刻把 seenSequences 清空，避免后续事件被错误判为重复。
    await refreshServerInstance();

    // 第一次发送：分配新 session
    if (!currentSessionId.value) {
      currentSessionId.value = generateSessionId();
    }
    eventOffset = 0;
    lastEventId = 0;

    // 推 user 消息 + 空 assistant 占位
    // Task 12：如果有 attachments，content 编为 multimodal 数组；
    //          否则保持原样（纯字符串）以保持向后兼容。
    const attachments = options?.attachments ?? [];
    const userContent: string | MessageContentPart[] = attachments.length > 0 ? serializeAttachments(text, attachments) : text;

    messages.value.push({
      role: "user",
      content: userContent,
      tool_calls: [],
      tool_results: [],
    });
    messages.value.push({
      role: "assistant",
      content: "",
      tool_calls: [],
      tool_results: [],
      isStreaming: true,
    });

    status.value = "streaming";

    // T15 unblock：mock_resume 模式在 fetch 前清空 chip + 把 round state
    // 切到 "running" —— UI 立即从 paused 切到 spinner，避免视觉残留
    // （stale "awaiting_user_input" 与新事件流冲突）。
    // 后续 mock_round_state{phase:resumed/in_progress} 事件会覆盖此处写入。
    if (mode === "mock_resume") {
      mockBranchChoices.value = [];
      mockBranchPrompt.value = "";
      const curRound = mockRoundState.value?.roundIdx ?? 0;
      const totalRounds = mockRoundState.value?.totalRounds ?? 0;
      mockRoundState.value = {
        scenario: currentMockScenario.value,
        roundIdx: curRound,
        totalRounds,
        phase: "running",
        context: { ...(mockRoundState.value?.context ?? {}) },
      };
    }

    saveState();
    refreshSessions();

    abortController = new AbortController();
    // 30s 超时保护：如果后端长时间无响应，自动中断
    let timedOut = false;
    const timeoutId = setTimeout(() => {
      timedOut = true;
      if (abortController) abortController.abort();
    }, 30_000);

    // ── 构建消息列表：历史对话（system prompt 由后端从 config 注入） ──
    // Task 12：content 可能是 string 或 multimodal 数组，原样透传。
    const apiMessages: Array<{ role: string; content: string | MessageContentPart[] }> = [];

    // 追加历史消息（不含空的 assistant 占位消息）。
    // Task 7：跳过 role='system' + content=CONTEXT_COMPACTION_MARKER
    // 的合成消息——后端在触发压缩时已经把这段 summary 写进了
    // 自己的 session 持久化层，前端再回送一份就是重复。
    for (const m of messages.value) {
      if (m.role === "assistant" && !m.content && !m.reasoning && m.tool_calls.length === 0) continue;
      // Task 7：跳过 system + CONTEXT_COMPACTION_MARKER 的合成消息。
      // Task 12：content 可能是 multimodal 数组，这种情况下不可能 ==
      // CONTEXT_COMPACTION_MARKER（string），TS 也允许此比较但用 typeof
      // 守卫更安全。
      if (m.role === "system" && typeof m.content === "string" && m.content === CONTEXT_COMPACTION_MARKER) continue;
      apiMessages.push({ role: m.role, content: m.content });
    }

    try {
      console.debug("[useAgent] send() starting fetch to", `${getAgentBase()}/api/chat`, "mode=", mode);
      // AG-UI 协议协商：根据 useAgentApiBase.shouldSendAGUIHeader() 决定
      // 是否带 X-Agent-Protocol: agui header（默认 'auto' → 带）。
      // 后端看到 header 后用 AG-UI parser 解析 LLM 响应；不带则按
      // legacy 自定义 SSE 返回。
      //
      // Accept: text/event-stream —— 必传。Android 真机上 useProxiedFetch
      // 已替换 window.fetch，见 isStream 判断（useProxiedFetch.ts#L166-169）：
      //   命中此 header 才走 ApiProxy.streamStart()，否则走 fetchOnce()，
      //   会把整个 SSE body 一次性塞进 new Response(body)，processLegacySSE
      //   reader.read() 同步读完所有 chunk，**没有逐字流式效果**。
      // dev 模式 useProxiedFetch 不安装，原生 fetch 走 WebView 自带 SSE 拆分，
      // 加此 header 无副作用（CORS 不拦 Accept）。
      const sendAGUIHeader = shouldSendAGUIHeader();
      const fetchHeaders: Record<string, string> = {
        "Content-Type": "application/json",
        Accept: "text/event-stream",
        ...(sendAGUIHeader ? { "X-Agent-Protocol": "agui" } : {}),
      };
      // T15 unblock：mode === 'mock_resume' 时把 scenario 透传给后端，
      // 否则 MockEngineV2 找不到对应的 stateful 实例 → 400 错误。
      // 后端 handleMockResume 据此：(1) 在 mockScenariosV2 中查找剧本；
      // (2) 调 mockV2SessionEngines 取出 / 创建 stateful 引擎；
      // (3) 调 engine.Resume(userText) 推下一轮事件。
      const scenarioForBody = mode === "mock_resume" ? (options?.scenario ?? currentMockScenario.value ?? undefined) : undefined;
      const response = await fetch(`${getAgentBase()}/api/chat`, {
        method: "POST",
        headers: fetchHeaders,
        body: JSON.stringify({
          sessionId: currentSessionId.value,
          model: activeModel.value,
          temperature: activeTemperature.value,
          messages: apiMessages,
          deviceId: getDeviceIdSync(),
          // Task 11：把 mode 字段透传给后端。后端 ChatMode 根据 mode
          // 走 start/steer/queue 三种分支（"" 视为 start）。
          mode,
          // T15 unblock：mock_resume 时把 scenario 字段一并发出。
          // 非 mock_resume 模式不发送（保持 backward-compat：后端
          // struct 字段是 omitempty，未传则忽略）。
          ...(scenarioForBody ? { scenario: scenarioForBody } : {}),
        }),
        signal: abortController.signal,
      });

      if (!response.ok) {
        // 关键：解析后端 JSON body 拿到真正的 message 字段，而不是只显示
        // "HTTP 503: Service Unavailable"。后端 handleAgentChat 在缺 API Key
        // 等场景下会返回 {error, message, ...}，前端要把 message 透给用户。
        throw await buildHttpError(response, "/api/chat");
      }

      if (!response.body) {
        throw new Error("响应体为空（可能被代理或网络中间层截断）");
      }

      // Mock 模式 header 检测（备份信号）：SSE stream_start 事件是主信号，
      // 但如果首条事件到达前 header 已被读取，这里先把状态置好，避免 UI
      // 看到一段无 badge 的"普通"回复再被刷成 mock。
      // response.headers 可能为 undefined（部分代理 / 测试 mock），用 ?. 兼容。
      const mockHeader = response.headers?.get("X-Mock-Mode");
      if (mockHeader) {
        isMockMode.value = true;
        mockScenario.value = response.headers?.get("X-Mock-Scenario") ?? "";
      }

      // 协议分发：根据后端响应 X-Agent-Protocol 决定走 AG-UI parser 还是 legacy
      // response.headers 可能为 undefined（部分代理 / 测试 mock），用 ?. 兼容。
      const responseProtocol = response.headers?.get("X-Agent-Protocol") === "agui" ? "agui" : "legacy";
      const result = await processSSE(response.body, responseProtocol);

      // 流结束但未收到任何事件 → 后端无响应
      if (!result.received) {
        throw new Error("服务端无响应：连接已关闭但未返回任何数据");
      }

      // 收到了事件但 assistant 内容仍为空且无工具调用 → 异常空回复
      const lastAssistant = [...messages.value].reverse().find(m => m.role === "assistant");
      if (lastAssistant && !lastAssistant.content && lastAssistant.tool_calls.length === 0) {
        console.error("[useAgent] WARNING: stream ended with empty assistant reply — marking user msg as errored");
        const lastUserMsg = [...messages.value].reverse().find(m => m.role === "user");
        if (lastUserMsg) lastUserMsg.error = "服务端返回空回复，请重试";
        status.value = "idle";
      }
    } catch (e: any) {
      // 找到本次发送的 user 消息，标记错误（每条消息独立错误状态）
      const lastUserMsg = [...messages.value].reverse().find(m => m.role === "user");
      if (e?.name === "AbortError") {
        if (timedOut) {
          console.error("[useAgent] send timed out (30s)");
          if (lastUserMsg) lastUserMsg.error = "请求超时（30秒内服务端无响应），请检查网络或稍后重试";
          status.value = "idle";
        } else {
          console.debug("[useAgent] send aborted by user");
          // 用户主动停止：不标记错误
        }
        finalizeLastAssistant();
        if (status.value !== "idle") status.value = "idle";
      } else {
        let detail = e?.message || String(e);
        // 区分 CORS 预检失败 / 网络断开 / 服务器返回：
        //   TypeError: Failed to fetch（或 iOS Safari 的"Load failed"）通常是
        //     CORS 预检失败 / mixed content blocked / 端口不通 — 浏览器拒绝跨域 POST
        //   这里把诊断信息 dump 到 console.error，下次出问题时 DevLogs 一眼能定位
        if (e?.name === "TypeError" && /Failed to fetch|Load failed/i.test(detail)) {
          const ctx = getAgentApiBaseContext();
          console.error("[useAgent] send failed (likely CORS preflight / network / mixed content):", {
            base: ctx.base,
            source: ctx.source,
            isNative: ctx.isNative,
            env: ctx.env,
            sampleUrl: ctx.sampleUrl,
            pageOrigin: typeof location !== "undefined" ? location.origin : "(no location)",
            requestUrl: `${ctx.base}/api/chat`,
            aguiHeaderSent: shouldSendAGUIHeader(),
          });
          detail = `无法连接 Agent API (${ctx.base}) — 检查 CORS 预检 / 网络 / 服务器可达性`;
        }
        console.error("[useAgent] send failed:", detail);
        if (lastUserMsg) lastUserMsg.error = detail;
        // 把后端 buildHttpError 挂的 .code 提取出来（'no_api_key' / 'upstream_error' / 等）。
        // chat UI 据此可以展示"去 AI 设置"快捷按钮，让用户从对话流直达修复点，
        // 避免"我保存了 key 但 chat 还是 503"的卡死循环（用户的 6 条日志就是这个场景）。
        const errCode = e?.code as "no_api_key" | "upstream_error" | "invalid_json" | "unknown" | undefined;
        lastErrorCode.value = errCode ?? "unknown";
        showToast({ message: detail, duration: 3000, color: "danger" });
        status.value = "idle";
        finalizeLastAssistant();
      }
    } finally {
      clearTimeout(timeoutId);
      abortController = null;
      saveState();
    }
  }

  /**
   * Task 11：「排队下一条」按钮的内部实现。仅在 status='streaming' 或
   * 'confirming' 时合法。流程：
   *   1. 推一条带 pending=true 的 user 消息到 messages.value，并
   *      加入 pendingMessages 队列（同一对象引用，便于 UI 双向引用）。
   *   2. POST /api/chat with mode='queue'，body 包含完整消息列表
   *      （含刚 push 的 user 消息，与 start/steer 一致）。
   *   3. 期望服务端返回 202 Accepted —— 不读取 SSE（queued turn 的
   *      事件会通过现有 SSE 连接回流，由 handleAgentEvent 接管）。
   *   4. 出错时把 user 消息从 pendingMessages 移除并标记 error。
   *
   * 注意：排队期间不打断当前 SSE 流、不创建新 assistant 占位——等
   * 排队的 turn 真正启动时，由 handleAgentEvent 的 lastAssistant()
   * 在首个 text_delta 时按需创建。
   */
  async function sendQueued(text: string, attachments: Attachment[]): Promise<void> {
    if (status.value !== "streaming" && status.value !== "confirming") {
      console.debug("[useAgent] sendQueued: ignored (no active turn to queue after)");
      return;
    }
    if (!currentSessionId.value) {
      console.debug("[useAgent] sendQueued: ignored (no active session)");
      return;
    }

    lastError.value = "";

    // Task 12：multimodal content 数组 vs 纯字符串
    const userContent: string | MessageContentPart[] = attachments.length > 0 ? serializeAttachments(text, attachments) : text;

    // Push user 消息（带 pending=true），并把它登记到 pendingMessages。
    // 两个数组共享同一对象引用：messages.value 渲染聊天气泡，
    // pendingMessages 暴露给 UI 渲染「已排队：xxx」提示。
    const userMsg: Message = {
      role: "user",
      content: userContent,
      tool_calls: [],
      tool_results: [],
      pending: true,
    };
    messages.value.push(userMsg);
    pendingMessages.value.push(userMsg);
    saveState();
    refreshSessions();

    // 复用 send() 的 apiMessages 构建逻辑（不能调 send() 本身——
    // 那会重置 eventOffset 并破坏当前 SSE 流的状态机）。
    const apiMessages: Array<{ role: string; content: string | MessageContentPart[] }> = [];
    for (const m of messages.value) {
      if (m.role === "assistant" && !m.content && !m.reasoning && m.tool_calls.length === 0) continue;
      if (m.role === "system" && typeof m.content === "string" && m.content === CONTEXT_COMPACTION_MARKER) continue;
      apiMessages.push({ role: m.role, content: m.content });
    }

    try {
      console.debug("[useAgent] sendQueued() POST /api/chat mode=queue");
      const response = await fetch(`${getAgentBase()}/api/chat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sessionId: currentSessionId.value,
          model: activeModel.value,
          temperature: activeTemperature.value,
          messages: apiMessages,
          deviceId: getDeviceIdSync(),
          mode: "queue",
        }),
      });
      // queue 模式服务端约定返回 202 Accepted（chat.go 中由
      // HandleChat 在 mode=='queue' 时显式 WriteHeader 202）。
      if (response.status !== 202) {
        throw new Error(`HTTP ${response.status}: ${response.statusText || "排队失败"}`);
      }
      console.debug("[useAgent] sendQueued: 202 Accepted, message parked on server");
    } catch (e: any) {
      const detail = e?.message || String(e);
      console.error("[useAgent] sendQueued failed:", detail);
      // 出错：把 user 消息从 pendingMessages 摘除并标记 error（保留在
      // messages.value 以便用户看到失败提示）。
      const idx = pendingMessages.value.indexOf(userMsg);
      if (idx !== -1) pendingMessages.value.splice(idx, 1);
      userMsg.error = "排队失败：" + detail;
      userMsg.pending = false;
      showToast({ message: "排队失败", duration: 3000, color: "danger" });
      saveState();
    }
  }

  /**
   * 4-决策确认工具调用
   */
  async function confirmTool(toolCallId: string, decision: Decision): Promise<void> {
    if (!currentSessionId.value) {
      console.debug("[useAgent] confirmTool: no active session");
      return;
    }

    if (abortController) {
      abortController.abort();
      abortController = null;
    }

    // 找到对应的 tool_call，把它的 status 标记为 'running' 表示处理中
    let targetTool: ToolCall | null = null;
    for (const msg of messages.value) {
      const tc = msg.tool_calls.find(t => t.id === toolCallId);
      if (tc) {
        targetTool = tc;
        break;
      }
    }
    if (targetTool) {
      // 用户做决策期间：保持 needsConfirm 但 status 标记 'running' 表示处理中
      targetTool.status = "running";
    }

    status.value = "streaming";
    saveState();

    abortController = new AbortController();
    try {
      // AG-UI 协议协商（与 send() 一致）
      // Accept: text/event-stream —— 必传，触发 useProxiedFetch 走 streamStart，
      // 否则 native 端走 fetchOnce 一次性读完所有 chunk，无流式效果。
      // 详见 useAgent.send() 注释。
      const sendAGUIHeader = shouldSendAGUIHeader();
      const response = await fetch(`${getAgentBase()}/api/confirm`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "text/event-stream",
          ...(sendAGUIHeader ? { "X-Agent-Protocol": "agui" } : {}),
        },
        body: JSON.stringify({
          sessionId: currentSessionId.value,
          toolCallId,
          decision,
          // 关键：必须传 deviceId！
          // 后端 handleAgentConfirm 在 accept/accept_for_session 分支
          // 会调 readAgentConfig(body.DeviceId) 派生 AES 解密 key 来读 API Key。
          // 不传 deviceId 会用错的 salt，永远解不出设备绑定的密文，
          // 真实执行工具时 Authorization header 会是空 Bearer，OpenAI 返回 401。
          deviceId: getDeviceIdSync(),
        }),
        signal: abortController.signal,
      });

      if (!response.ok) {
        throw await buildHttpError(response, "/api/confirm");
      }

      // 协议分发
      const responseProtocol = response.headers?.get("X-Agent-Protocol") === "agui" ? "agui" : "legacy";
      await processSSE(response.body, responseProtocol);
    } catch (e: any) {
      if (e?.name === "AbortError") {
        console.debug("[useAgent] confirmTool aborted");
        if (targetTool) targetTool.status = "pending";
        status.value = "confirming";
      } else {
        console.error("[useAgent] confirmTool failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
        if (targetTool) targetTool.status = "pending";
        showToast({ message: "Confirm request failed", duration: 2000, color: "danger" });
        status.value = "confirming";
      }
    } finally {
      abortController = null;
      saveState();
    }
  }

  /**
   * 启动时自动续传：恢复最近的 session 并继续追平进度
   *
   * 协议：
   *   ① 优先用 SSE 标准 `Last-Event-ID` HTTP header（与 EventSource 一致）
   *   ② 同时在 body 携带 `lastEventId` 字段（兼容 Go handler 当前解析路径）
   *   ③ 处理后端 `stream_status` 事件：
   *      - synced: 服务端已追平，等待新事件 → 短间隔轮询
   *      - more_pending: 服务端还有事件未推完 → 立即发起下一轮 resume
   *   ④ 收尾：服务端流自然结束（stream_end）→ 状态切回 idle
   */
  async function resume(): Promise<void> {
    const sessionId = findLatestPersistedSession();
    if (!sessionId) return;

    const saved = loadState(sessionId);
    if (!saved) return;

    currentSessionId.value = saved.sessionId;
    eventOffset = saved.eventOffset || 0;
    lastEventId = saved.lastEventId || 0;
    // 恢复 messages
    messages.value.splice(0, messages.value.length, ...(saved.messages || []));
    status.value = saved.status || "idle";

    // 如果上次是 streaming 状态，主动 resume 追平进度
    if (status.value === "streaming") {
      await runResumeChain();
    }
  }

  /**
   * 实际跑一轮 resume，处理 `stream_status` 事件决定是否链式续传。
   * 与 resume() 分离是为了支持 stream_status.more_pending 时递归调用。
   */
  async function runResumeChain(maxHops = 32): Promise<void> {
    if (abortController) {
      abortController.abort();
      abortController = null;
    }
    abortController = new AbortController();
    const controller = abortController;
    let hopsLeft = maxHops;
    try {
      // 链式 resume：每轮处理 processSSE 的信号决定下一步
      //   - streamEnded=true       → 退出循环（服务端显式 stream_end）
      //   - morePending=true       → 立刻下一轮（hopsLeft-- 防止无限递归）
      //   - 完全没收到事件          → 退出（流自然关闭且没新事件，等同于 synced）
      //   - 收到 synced            → 退出（保持 streaming 状态由后续触发）
      //   - status 切到非 streaming → 退出
      while (hopsLeft-- > 0) {
        const headerLastEventId = lastEventId > 0 ? String(lastEventId) : undefined;
        // AG-UI 协议协商（与 send() / confirmTool() 一致）
        // Accept: text/event-stream —— 必传，触发 useProxiedFetch 走 streamStart，
        // 否则 native 端走 fetchOnce 一次性读完所有 chunk，无流式效果。
        // 详见 useAgent.send() 注释。
        const sendAGUIHeader = shouldSendAGUIHeader();
        const response = await fetch(`${getAgentBase()}/api/resume`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Accept: "text/event-stream",
            // SSE 标准 Last-Event-ID：与 EventSource 协议一致
            ...(headerLastEventId ? { "Last-Event-ID": headerLastEventId } : {}),
            ...(sendAGUIHeader ? { "X-Agent-Protocol": "agui" } : {}),
          },
          body: JSON.stringify({
            sessionId: currentSessionId.value,
            // body 字段名：与后端 handleAgentResume 的 lastEventId 字段对齐
            // （不再使用旧的 offset 字段）
            lastEventId: lastEventId,
          }),
          signal: controller.signal,
        });

        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }

        // 协议分发：response.headers 可能为 undefined（部分代理 / 测试 mock），
        // 用 ?. 兼容。后端正常响应总会带 X-Agent-Protocol header。
        const responseProtocol = response.headers?.get("X-Agent-Protocol") === "agui" ? "agui" : "legacy";
        const { received, streamEnded, morePending } = await processSSE(response.body, responseProtocol);

        // 收尾判定
        if (streamEnded) break; // 服务端显式收尾 → 退出
        if (morePending) continue; // 服务端还有事件未推完 → 立刻下一轮
        if (!received) break; // 流自然关闭且无事件 → 退出
        if (status.value !== "streaming") break; // 已被 confirm/cancel 切走 → 退出
        // 收到事件但 neither morePending nor streamEnded：保守退出，
        // 避免无限循环（实际上 server 应该会推 stream_status 或 stream_end）
        break;
      }
    } catch (e: any) {
      if (e?.name !== "AbortError") {
        console.error("[useAgent] resume failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
        finalizeLastAssistant();
        status.value = "idle";
      }
    } finally {
      // 仅当 controller 没被换掉时才清空（避免覆盖新一轮 runResumeChain 的 controller）
      if (abortController === controller) {
        abortController = null;
      }
      saveState();
    }
  }

  /**
   * 停止当前流式（SSE 连接 abort）
   */
  function stop(): void {
    if (abortController) {
      abortController.abort();
      abortController = null;
    }
    finalizeLastAssistant();
    status.value = "idle";
    saveState();
  }

  /**
   * 重置 session（清空消息、状态、持久化）
   * ⚠️ 旧版是 destroy；现在 newSession 替代 reset 的语义——
   *    reset 仅用于内部紧急回退（UI 入口已切换为 newSession）
   */
  function reset(): void {
    stop();
    // 清除所有 tool_call 超时 timer（避免泄漏 + reset 后误标记）
    for (const t of toolCallTimeoutMap.values()) clearTimeout(t);
    toolCallTimeoutMap.clear();
    if (currentSessionId.value) {
      try {
        localStorage.removeItem(STORAGE_PREFIX + currentSessionId.value);
      } catch {
        // ignore
      }
    }
    currentSessionId.value = "";
    eventOffset = 0;
    messages.value.splice(0, messages.value.length);
    status.value = "idle";
    lastError.value = "";
    lastUserInput.value = "";
    refreshSessions();
  }

  /**
   * 关闭错误条
   */
  function dismissError(): void {
    lastError.value = "";
    lastErrorCode.value = "";
    if (status.value === "error") status.value = "idle";
  }

  /**
   * 重发上一次失败的用户消息
   */
  function retryLast(): void {
    const text = lastUserInput.value;
    if (!text) return;
    lastError.value = "";
    status.value = "idle";
    void send(text);
  }

  /**
   * 设置 API 返回的默认模型（由 AgentChat.fetchModels 调用）。
   * 写入后 newSession() 会自动使用此值重置 activeModel。
   */
  function setApiDefaultModel(m: string): void {
    apiDefaultModel.value = m;
  }

  // 构造时同步一次 session 列表（供 UI 立即显示）
  refreshSessions();

  return {
    messages,
    status,
    send,
    confirmTool,
    resume,
    stop,
    reset,
    newSession,
    switchSession,
    deleteSession,
    refreshSessions,
    sessions,
    currentSessionId,
    lastError,
    lastErrorCode,
    dismissError,
    retryLast,
    activeModel,
    activeTemperature,
    // Issue 1: API 默认模型（新会话时使用）
    apiDefaultModel,
    setApiDefaultModel,
    // Task 11 (Steer / Queue)：UI 用它渲染「已排队：xxx」提示。
    pendingMessages,
    // Context 图标：实时上下文使用 + todos + referenced files
    contextUsage,
    // Mock 模式：后端 cfg.Agent.MockMode != "off" 时由 SSE stream_start
    // 事件或 X-Mock-Mode header 触发，UI 据此展示"🧪 模拟"badge。
    isMockMode,
    mockScenario,
    // 用户主动切换（AgentChat 顶栏的"🧪 模拟"badge → action-sheet 触发）
    currentMockMode,
    loadMockMode,
    setMockMode,
    // Mock 模式预设按钮：覆盖在输入框上方的 chip 列表。
    // - mockPresets：当前 chip 列表（mock_presets 事件 / loadMockPresets 驱动）
    // - mockPresetsPhase：当前阶段（initial / after_round_2 / picker / ...）
    // - mockPresetsScenario：当前 scenario ID
    // - pickMockPreset：点击 chip → send(preset.userText)
    // - loadMockPresets：AgentChat onMounted 调一次拉"全局剧本选择器"
    mockPresets,
    mockPresetsPhase,
    mockPresetsScenario,
    pickMockPreset,
    loadMockPresets,
    // v2 多轮/分支剧本（参考 .trae/specs/agent-tools-scenarios-v2/spec.md）。
    // - mockBranchChoices：当前 step 的 chip 列表（mock_branch_choice 事件驱动）
    // - mockBranchPrompt：当前 step 的 prompt 文案（供 MockBranchChoiceBar 渲染）
    // - mockRoundState：当前 round 进度 + 阶段（mock_round_state 事件驱动）
    // - mockScenarioPaused：派生 computed，phase 为 awaiting_user_input 或
    //   awaiting_branch_choice 时为 true。AgentChat 用它控制 MockBranchChoiceBar
    //   的 v-if 显隐。
    // - currentMockScenario：当前激活的 scenario ID（pickMockBranch /
    //   sendMockRoundResponse 必须带上，供后端 MockEngineV2 知道是哪个剧本）
    // - pickMockBranch(branchId)：点击 chip → send(branchId, {mode: mock_resume})
    // - sendMockRoundResponse(userText)：键入文本 → send(userText, {mode: mock_resume})
    mockBranchChoices,
    mockBranchPrompt,
    mockRoundState,
    mockScenarioPaused,
    currentMockScenario,
    pickMockBranch,
    sendMockRoundResponse,
    // 调试开关：URL ?debug=agent 时强制显示 AgentDebugPanel（mock 模式时也自动开）。
    // 便于排查"SSE 事件 → messages → renderedItems → UI 组件"全链路断点。
    isDebugAgent,
    // 调试：原始 SSE 事件日志（AgentDebugPanel ⑦ 区展示）
    rawSSEEvents,
    // ── Task 3: tool_call 状态机派生 ─────────────────────────────
    // - runningTools：当前正在 running/pending 的 tool_call 扁平列表
    //   （供 AgentChat footer 渲染「🔄 工具执行中…」+ OperationCard 动画）
    // - hasRunningTool：runningTools 非空标志位（UI 状态指示）
    // - allToolCalls：所有 tool_call 扁平列表（供 UI 一次性遍历渲染）
    runningTools,
    hasRunningTool,
    allToolCalls,
    // Task 4：以下为测试专用钩子。生产代码不应调用——所有 serverInstance
    // 同步都由 useAgent 内部 await refreshServerInstance() 完成。
    __refreshServerInstanceForTest: refreshServerInstance,
    __getServerInstanceForTest: () => currentServerInstance,
    __setServerInstanceForTest: (id: string) => {
      currentServerInstance = id;
    },
    __getSeenSequencesForTest: () => seenSequencesOrder.slice(),
  };
}
