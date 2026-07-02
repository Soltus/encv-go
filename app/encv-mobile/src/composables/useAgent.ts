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
  // 工具函数
  parseContentDelta,
  parseToolResultData,
  parseStreamStatusData,
  parseToolCallData,
  parseToolStatus,
  parseCompactionData,
  appendSequencedChunk,
  getAgentBase,
  generateSessionId,
  extractUserTitle,
  buildHttpError,
  CONTEXT_COMPACTION_MARKER,
  STORAGE_PREFIX,
  MAX_TRACKED_REALTIME_SEQUENCES,
  // 类型（按需）
  type AgentStatus,
  type SessionMeta,
  type Decision,
  type MockPreset,
  type MockBranch,
  type MockRoundState,
  type AgentEvent,
  type ToolCall,
  type Message,
} from './useAgent_helpers'

// 重新导出所有 helpers 以保持向后兼容（`import { ... } from '@/composables/useAgent'`）
export * from './useAgent_helpers'

import { ref, computed } from 'vue'
import { showToast } from '@/composables/useToast'
import { getDeviceIdSync } from './useDeviceId'
import { getAgentApiBaseContext, shouldSendAGUIHeader } from './useAgentApiBase'
import {
  serializeAttachments,
  type Attachment,
  type MessageContentPart,
} from './useAttachments'
import { useContextUsage } from './useContextUsage'
import { processAGUISSE as processAGUISSEImpl } from './useAGUIParser'

export function useAgent() {
  const messages = ref<Message[]>([])
  const status = ref<AgentStatus>('idle')
  const lastError = ref<string>('')
  // 错误语义码：后端 buildHttpError 在 Error 上挂 .code 字段（'no_api_key' / 'upstream_error' / 'unknown'），
  // 前端 chat UI 据此做分支判断（如展示"去 AI 设置"按钮）。
  // 与 lastError 的区别：lastError 是给人看的字符串，lastErrorCode 是给程序判断的结构化字段。
  const lastErrorCode = ref<'' | 'no_api_key' | 'upstream_error' | 'invalid_json' | 'unknown'>('')
  const lastUserInput = ref<string>('')
  const sessions = ref<SessionMeta[]>([])
  const currentSessionId = ref<string>('')
  /**
   * Task 11 (Steer / Queue)：跟踪已通过「排队下一条」按钮提交、
   * 但服务端尚未开始处理的 user 消息。pendingMessages 数组的元素
   * 是 messages.value 中对应 user 消息的引用（同一对象），这样
   * UI 可以直接从 pendingMessages 取出 text 渲染「已排队：xxx」
   * 提示，并在服务端开始处理时（首个 text_delta 到达）实时清除
   * 对应条目的 pending 标记。
   */
  const pendingMessages = ref<Message[]>([])
  /** 本地已处理事件计数（与 lastEventId 解耦：eventOffset 永远递增，lastEventId 由 SSE id 行决定） */
  let eventOffset = 0
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
  })
  /**
   * SSE 标准 Last-Event-ID：服务端为每个事件分配的全局递增 ID。
   * 用于断点续传——前端解析 `id: N` 行维护此字段，resume() 时回传给后端。
   * 0 = 尚未收到任何事件 id。
   */
  let lastEventId = 0
  let abortController: AbortController | null = null

  // ─── Task 3: tool_call 状态机 — 30s 无响应超时保护 ────────────────
  // 当 tool_call 创建后 30s 内未收到 tool_result（且未进入 failed/cancelled
  // 终态），自动标记为 failed + errorCode='TIMEOUT'。这是 user 反馈"工具
  // 卡住没有任何反馈"的根因——之前前端没有超时机制，spinner 转 60s+ 仍然
  // 停在 running。timer 必须在 tool_result 到达或 tool_call 进入终态时清除
  // （clearToolCallTimeout），否则会泄漏。
  const TOOL_CALL_TIMEOUT_MS = 30_000
  const toolCallTimeoutMap = new Map<string, ReturnType<typeof setTimeout>>()

  function clearToolCallTimeout(toolCallId: string): void {
    const t = toolCallTimeoutMap.get(toolCallId)
    if (t !== undefined) {
      clearTimeout(t)
      toolCallTimeoutMap.delete(toolCallId)
    }
  }

  function armToolCallTimeout(toolCallId: string): void {
    clearToolCallTimeout(toolCallId) // 防重入
    const t = setTimeout(() => {
      toolCallTimeoutMap.delete(toolCallId)
      // 找到对应 tool_call，标 failed
      for (const msg of messages.value) {
        const tc = msg.tool_calls.find((c) => c.id === toolCallId)
        if (!tc) continue
        // 单向：终态（failed/cancelled/success）不再覆盖
        if (tc.status === 'failed' || tc.status === 'cancelled' || tc.status === 'success') {
          break
        }
        tc.status = 'failed'
        tc.errorCode = 'TIMEOUT'
        tc.errorMessage = '工具执行超过 30s 无响应'
        tc.finishedAt = Date.now()
        console.warn(
          `[useAgent] tool_call ${toolCallId} (${tc.name}) timed out after ${TOOL_CALL_TIMEOUT_MS}ms`,
        )
        break
      }
    }, TOOL_CALL_TIMEOUT_MS)
    toolCallTimeoutMap.set(toolCallId, t)
  }

  // ─── Server Instance + Sequence 去重（Task 4） ────────────────────────
  // 后端 /api/health 返回 process-wide 唯一的 serverInstanceId。同一进程
  // 启动期间它恒定；进程重启（OS 分配新 PID / 启动时间不同）就会变化。
  // 前端每次拉取发现 instance 变化时，必须清空 seenSequences——因为新进程
  // 的 SSE sequence 编号可能与旧进程"撞号"，不复用旧的去重集合会造成
  // 真实事件被错误丢弃。
  /** 当前进程已知的 serverInstanceId；空串 = 尚未拉取过 /api/health */
  let currentServerInstance = ''
  /** 已见 SSE sequence 编号集合；超过 MAX_TRACKED_REALTIME_SEQUENCES 时按 FIFO 驱逐 */
  const seenSequences = new Set<number>()
  /** 配合 Set 实现的 FIFO 驱逐顺序表 */
  const seenSequencesOrder: number[] = []

  // 模型/温度从 localStorage 读取（AgentChat 顶部 UI 选择会同步写入这里）
  const MODEL_STORAGE_KEY = 'encv-agent-selected-model'
  const TEMP_STORAGE_KEY = 'encv-agent-temperature'
  const activeModel = ref<string>(
    (() => {
      try { return localStorage.getItem(MODEL_STORAGE_KEY) || 'gpt-4o-mini' }
      catch { return 'gpt-4o-mini' }
    })(),
  )
  /**
   * API 返回的默认模型（来自 /api/models 的 defaultModel 字段）。
   * AgentChat 在 fetchModels 成功后通过 setApiDefaultModel() 写入，
   * newSession 时用于重置 activeModel。
   */
  const apiDefaultModel = ref<string>('')
  /** 当前会话的创建时间戳（毫秒），用于持久化和历史列表排序 */
  const sessionCreatedAt = ref<number>(Date.now())
  // 之前怀疑 gpt-4o-mini 在 gptgod 代理下不发 tools，临时加了 safeModel
  // 白名单做硬编码降级。实测 gpt-4o-mini 完全能用工具（3 轮 list_mounts →
  // list_files → 输出 4 个目录），根因不在模型上。回退这段逻辑，
  // 直接用 activeModel —— 真正的问题要去看后端日志 / 实际请求体。
  const activeTemperature = ref<number>(
    (() => {
      try {
        const v = localStorage.getItem(TEMP_STORAGE_KEY)
        const n = v == null ? 0.7 : Number(v)
        return Number.isFinite(n) ? n : 0.7
      } catch { return 0.7 }
    })(),
  )

  // ─── Mock 模式检测 ────────────────────────────────────────
  // 后端在 cfg.Agent.MockMode != "off" 时启用 mock 模式：
  //   - HTTP response header: X-Mock-Mode: builtin|custom
  //   - HTTP response header: X-Mock-Scenario: <scenario_id>
  //   - SSE 首个 stream_start 事件 data: { mock: true, scenario: "..." }
  // 前端拿到任一信号就置 isMockMode=true，让 AgentChat 顶部展示"🧪 模拟"badge。
  const isMockMode = ref(false)
  const mockScenario = ref<string>('')

  // 调试开关：URL 带 ?debug=agent 时为 true，强制显示 AgentDebugPanel。
  // 浏览器端用 window.location，SSR 时降级为 false。
  const isDebugAgent = computed(() => {
    if (typeof window === 'undefined') return false
    try {
      return new URLSearchParams(window.location.search).get('debug') === 'agent'
    } catch {
      return false
    }
  })

  // ─── 原始 SSE 事件日志（调试用：AgentDebugPanel ⑦ 区展示） ──
  // 每个进 processSSE 的 event 都追加一条（含 type + data 摘要 + 时间戳）。
  // 不自动清理——用户手动"清空"或新建会话时重置。
  const rawSSEEvents = ref<{ ts: string; type: string; dataSummary: string; seq?: number | null }[]>([])

  /** 追加一条原始事件到日志（最多保留 200 条防内存爆炸） */
  function pushRawEvent(type: string, dataSummary: string, seq?: number | null) {
    rawSSEEvents.value.push({
      ts: new Date().toISOString().slice(11, 23), // HH:MM:SS.mmm
      type,
      dataSummary,
      seq: seq ?? null,
    })
    if (rawSSEEvents.value.length > 200) {
      rawSSEEvents.value = rawSSEEvents.value.slice(-150)
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
  const mockPresets = ref<MockPreset[]>([])
  const mockPresetsPhase = ref<string>('')
  const mockPresetsScenario = ref<string>('')

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
  const mockBranchChoices = ref<MockBranch[]>([])
  const mockBranchPrompt = ref<string>('')
  const mockRoundState = ref<MockRoundState | null>(null)
  const currentMockScenario = ref<string>('')

  const mockScenarioPaused = computed(() => {
    const phase = mockRoundState.value?.phase
    return phase === 'awaiting_user_input' || phase === 'awaiting_branch_choice'
  })

  // ─── Task 3: tool_call 状态机派生 computed ────────────────────────
  /**
   * 正在运行 / 等待中的 tool_call 扁平列表。
   *  - running：tool_status 收到 'running' 后保持
   *  - pending：刚 tool_call 事件到达、还没收到 tool_status 'running'
   * 命中这两个状态的 tool_call 全部列入，供 AgentChat footer 渲染
   * 「🔄 工具执行中…」指示 + OperationCard 渲染脉冲动画。
   */
  const runningTools = computed(() => {
    const list: ToolCall[] = []
    for (const m of messages.value) {
      for (const tc of m.tool_calls) {
        if (tc.status === 'running' || tc.status === 'pending') {
          list.push(tc)
        }
      }
    }
    return list
  })

  /** 是否有任意 tool_call 仍在运行 / 等待中（UI 状态指示用） */
  const hasRunningTool = computed(() => runningTools.value.length > 0)

  /**
   * 当前会话中所有 tool_call 的扁平引用（按到达顺序）。
   * 供 UI 一次性遍历渲染——避免在模板里再嵌套 .tool_calls。
   */
  const allToolCalls = computed(() => {
    const list: ToolCall[] = []
    for (const m of messages.value) {
      for (const tc of m.tool_calls) {
        list.push(tc)
      }
    }
    return list
  })

  /**
   * 多轮剧本中点 chip 继续：把 chip id 当作 userText 送回后端 Resume。
   * 关键点：mode='mock_resume' —— 后端 MockEngineV2 据此区分"新 session 启动"
   * 和"在暂停点恢复"，并用 currentMockScenario 找到正确的剧本状态机。
   */
  function pickMockBranch(branchId: string): void {
    if (typeof branchId !== 'string' || branchId.length === 0) {
      console.debug('[useAgent] pickMockBranch: invalid branchId', branchId)
      return
    }
    if (!currentMockScenario.value) {
      console.debug('[useAgent] pickMockBranch: no currentMockScenario — dropped')
      return
    }
    if (status.value === 'streaming' || status.value === 'confirming') {
      console.debug('[useAgent] pickMockBranch: ignored (busy)')
      return
    }
    console.debug(
      '[useAgent] pickMockBranch →',
      branchId,
      '| scenario =',
      currentMockScenario.value,
    )
    void send(branchId, { mode: 'mock_resume', scenario: currentMockScenario.value })
  }

  /**
   * 多轮剧本中键入文本继续：等价于 pickMockBranch，但 userText 是用户键入的。
   * 用于：chip 列表里没覆盖到的细粒度控制（比如用正则编辑 metadata 字段）。
   */
  function sendMockRoundResponse(userText: string): void {
    if (typeof userText !== 'string' || userText.trim().length === 0) {
      console.debug('[useAgent] sendMockRoundResponse: empty text — dropped')
      return
    }
    if (!currentMockScenario.value) {
      console.debug('[useAgent] sendMockRoundResponse: no currentMockScenario — dropped')
      return
    }
    if (status.value === 'streaming' || status.value === 'confirming') {
      console.debug('[useAgent] sendMockRoundResponse: ignored (busy)')
      return
    }
    console.debug(
      '[useAgent] sendMockRoundResponse →',
      userText.slice(0, 40),
      '| scenario =',
      currentMockScenario.value,
    )
    void send(userText.trim(), { mode: 'mock_resume', scenario: currentMockScenario.value })
  }

  // ─── Mock 模式控制（用户从 AgentChat 顶栏的"🧪 模拟"badge 切换） ─────
  // 字段语义与后端 cfg.Agent.MockMode 一一对应：
  //   - 'off'     → 真实 LLM 调用（默认）
  //   - 'builtin' → 内置 12 个剧本
  //   - 'custom'  → config.user.json 中 agent_settings.mock_scenarios
  //
  // 修改后会立刻调 PUT /api/config 持久化到后端 config.user.json，
  // 下次会话起立即生效（无需重启后端）。
  type MockMode = 'off' | 'builtin' | 'custom'
  const currentMockMode = ref<MockMode>('off')

  /**
   * 模拟模式预设 chip 点击：直接把 preset.userText 喂给 send()。
   * 与用户在输入框里打字的区别：
   *   - 不会先填到 input.value（用户点击 chip 的预期是"立即发"，不是"先看再改"）
   *   - 会触发和正常 send 完全一样的流程（后端按 userText 关键词重新匹配 scenario）
   * 用 mode='start'（而非 'steer' / 'queue'）：mock 模式是单次请求流。
   */
  async function pickMockPreset(preset: MockPreset): Promise<void> {
    if (!preset || typeof preset.userText !== 'string' || preset.userText.length === 0) {
      console.debug('[useAgent] pickMockPreset: invalid preset', preset)
      return
    }
    // 状态检查：跟 send 保持一致 —— 正在 streaming/confirming 时丢弃
    if (status.value === 'streaming' || status.value === 'confirming') {
      console.debug('[useAgent] pickMockPreset: ignored (busy)')
      return
    }
    console.debug('[useAgent] pickMockPreset →', preset.id, '| userText =', preset.userText)
    await send(preset.userText, { mode: 'start' })
  }

  /**
   * 首次进入 AgentChat 时拉取"全局剧本选择器"覆盖在输入框上方。
   * 由 AgentChat.vue 的 onMounted 调用（仅一次）。
   * 后端 mock 模式关闭时返回空 presets → v-if 自然不渲染。
   * 后端流内 mock_presets 事件会**覆盖**本函数写入的 presets。
   */
  async function loadMockPresets(): Promise<void> {
    try {
      const resp = await fetch(`${getAgentBase()}/api/agent/mock/presets`)
      if (!resp.ok) {
        console.debug('[useAgent] loadMockPresets: HTTP', resp.status)
        return
      }
      const data = (await resp.json()) as {
        scenario?: string
        phase?: string
        presets?: MockPreset[]
        mockMode?: string
      }
      const list = Array.isArray(data.presets) ? data.presets : []
      // 标准化：scenario_picker 后端 ID 是 "pick_xxx" 风格，但前端 type
      // 要求必须有 id+label+userText。后端已经保证。
      mockPresets.value = list.filter(
        (p): p is MockPreset =>
          !!p &&
          typeof p === 'object' &&
          typeof p.id === 'string' &&
          typeof p.label === 'string' &&
          typeof p.userText === 'string',
      )
      mockPresetsPhase.value = String(data.phase ?? 'picker')
      mockPresetsScenario.value = String(data.scenario ?? 'scenario_picker')
      console.debug(
        '[useAgent] loadMockPresets →',
        mockPresets.value.length,
        'presets | mode =',
        data.mockMode,
        '| phase =',
        mockPresetsPhase.value,
      )
    } catch (e) {
      console.debug('[useAgent] loadMockPresets failed:', e)
    }
  }

  async function loadMockMode() {
    try {
      const resp = await fetch(`${getAgentBase()}/api/config`)
      if (!resp.ok) {
        console.debug('[MockMode] fetch /api/config failed: HTTP', resp.status)
        return
      }
      const cfg = (await resp.json()) as {
        agent_settings?: { mock_mode?: string }
      }
      const m = String(cfg?.agent_settings?.mock_mode ?? 'off').toLowerCase()
      currentMockMode.value =
        m === 'builtin' || m === 'custom' ? (m as MockMode) : 'off'
      // 覆盖式 UI：mock 模式配置开启时，isMockMode 必须**预先**置 true，
      // 否则用户首次进 AgentChat（还没发过消息）chip 不会显示。
      // 后续发消息触发流时，stream_start 事件会再次确认 isMockMode=true。
      isMockMode.value = currentMockMode.value !== 'off'
      console.debug(
        '[MockMode] load → mode =',
        currentMockMode.value,
        '| isMockMode =',
        isMockMode.value,
      )
    } catch (e) {
      console.debug('[MockMode] load failed:', e)
    }
  }

  async function setMockMode(mode: MockMode) {
    if (mode === currentMockMode.value) return
    try {
      // 必须整张 config 一并 PUT（后端会保留非 agent_settings 字段）。
      const getResp = await fetch(`${getAgentBase()}/api/config`)
      if (!getResp.ok) throw new Error(`fetch /api/config → HTTP ${getResp.status}`)
      const cfg = (await getResp.json()) as Record<string, unknown>
      const agentSettings = (cfg.agent_settings as Record<string, unknown> | undefined) ?? {}
      agentSettings.mock_mode = mode
      cfg.agent_settings = agentSettings
      const putResp = await fetch(`${getAgentBase()}/api/config`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(cfg),
      })
      if (!putResp.ok) {
        const errText = await putResp.text()
        throw new Error(`PUT /api/config → HTTP ${putResp.status}: ${errText}`)
      }
      currentMockMode.value = mode
      // 立即重置 isMockMode：下次 send 时再由 SSE stream_start 事件重新置位
      isMockMode.value = false
      mockScenario.value = ''
      console.info('[MockMode] set to', mode)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      console.error('[MockMode] setMockMode failed:', msg)
      throw e
    }
  }

  // ─── 内部辅助 ───────────────────────────────────────────────────────────

  /**
   * 把最后一条正在 streaming 的 assistant 消息标记为流式结束
   * 用于 catch 块 / stop() / 错误恢复场景
   */
  function finalizeLastAssistant(): void {
    for (let i = messages.value.length - 1; i >= 0; i--) {
      if (messages.value[i].role === 'assistant' && messages.value[i].isStreaming) {
        messages.value[i].isStreaming = false
        break
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
      const response = await fetch(`${getAgentBase()}/api/health`, { method: 'GET' })
      if (!response.ok) {
        console.warn('[useAgent] refreshServerInstance: /api/health returned', response.status)
        return
      }
      const data = await response.json()
      const newId = typeof data?.serverInstanceId === 'string' ? data.serverInstanceId : ''
      if (!newId) {
        console.warn('[useAgent] refreshServerInstance: response missing serverInstanceId')
        return
      }
      if (newId !== currentServerInstance) {
        console.debug('[useAgent] server instance changed:', currentServerInstance || '(none)', '->', newId)
        currentServerInstance = newId
        // 清空去重状态：旧 instance 的 sequence 编号集合在物理上属于旧进程，
        // 复用它们会导致新进程的真实事件被错误判为重复。
        seenSequences.clear()
        seenSequencesOrder.length = 0
      }
    } catch (e) {
      // 网络/CORS 等错误：保留旧值不丢业务，但提示一次
      console.warn('[useAgent] refreshServerInstance: fetch /api/health failed:', e instanceof Error ? `${e.name}: ${e.message}` : String(e))
    }
  }

  /**
   * 记录一个 sequence 编号为"已见"。超过 MAX_TRACKED_REALTIME_SEQUENCES
   * 时按插入顺序淘汰最老的编号。返回 true 表示"新增"（未见过），false
   * 表示"重复"（已在集合中）。
   */
  function rememberSequence(seq: number): boolean {
    if (seenSequences.has(seq)) {
      return false
    }
    seenSequences.add(seq)
    seenSequencesOrder.push(seq)
    if (seenSequencesOrder.length > MAX_TRACKED_REALTIME_SEQUENCES) {
      const evict = seenSequencesOrder.shift()
      if (evict !== undefined) seenSequences.delete(evict)
    }
    return true
  }

  // ─── 持久化 ─────────────────────────────────────────────────────────────

  function saveState() {
    if (!currentSessionId.value) return
    try {
      const payload = {
        sessionId: currentSessionId.value,
        eventOffset,
        lastEventId,
        messages: JSON.parse(JSON.stringify(messages.value)),
        status: status.value,
        createdAt: sessionCreatedAt.value,
        updatedAt: Date.now(),
      }
      localStorage.setItem(STORAGE_PREFIX + currentSessionId.value, JSON.stringify(payload))
    } catch (e) {
      console.debug('[useAgent] saveState failed:', e)
    }
  }

  function loadState(sessionId: string): {
    sessionId: string
    eventOffset: number
    /** 兼容老存档：无 lastEventId 字段时默认为 0 */
    lastEventId?: number
    messages: Message[]
    status: AgentStatus
    createdAt?: number
    updatedAt?: number
  } | null {
    try {
      const raw = localStorage.getItem(STORAGE_PREFIX + sessionId)
      if (!raw) return null
      const parsed = JSON.parse(raw)
      // 恢复会话创建时间（兼容老存档无此字段）
      if (typeof parsed?.createdAt === 'number') {
        sessionCreatedAt.value = parsed.createdAt
      }
      return parsed
    } catch (e) {
      console.debug('[useAgent] loadState failed:', e)
      return null
    }
  }

  function findLatestPersistedSession(): string | null {
    try {
      let latest: { id: string; ts: number } | null = null
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i)
        if (!key || !key.startsWith(STORAGE_PREFIX)) continue
        const raw = localStorage.getItem(key)
        if (!raw) continue
        try {
          const parsed = JSON.parse(raw) as { sessionId?: string; messages?: Message[] }
          // 启发式：取有消息的最后一次会话
          if (parsed?.messages && parsed.messages.length > 0 && parsed.sessionId) {
            const ts = parsed.messages[parsed.messages.length - 1]?.content?.length ?? 0
            if (!latest || ts > latest.ts) {
              latest = { id: parsed.sessionId, ts }
            }
          }
        } catch {
          // skip malformed
        }
      }
      return latest?.id ?? null
    } catch (e) {
      console.debug('[useAgent] findLatestPersistedSession failed:', e)
      return null
    }
  }

  /**
   * 扫描 localStorage 中所有 `agent:session:*` 键，返回按 updatedAt 倒序的 session 列表
   * 用于 UI 渲染"会话历史"
   */
  function refreshSessions(): void {
    const list: SessionMeta[] = []
    try {
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i)
        if (!key || !key.startsWith(STORAGE_PREFIX)) continue
        const raw = localStorage.getItem(key)
        if (!raw) continue
        try {
          const parsed = JSON.parse(raw) as {
            sessionId?: string
            messages?: Message[]
            createdAt?: number
            updatedAt?: number
          }
          if (!parsed?.sessionId) continue
          const msgs = parsed.messages || []
          const firstUser = msgs.find((m) => m.role === 'user')
          // Task 12：content 可能是 multimodal 数组（带附件的 user 消息），
          //          从中抽出首段 text 元素作为会话标题。
          const title = extractUserTitle(firstUser?.content) || '(空会话)'
          // 优先使用持久化的真实时间戳，兼容老存档无此字段
          const createdAt = parsed.createdAt || Date.now()
          const updatedAt = parsed.updatedAt || createdAt
          // 轮次 = 用户消息数量（每条 user 消息代表一轮对话）
          const rounds = msgs.filter((m) => m.role === 'user').length
          list.push({
            id: parsed.sessionId,
            title,
            createdAt,
            updatedAt,
            messageCount: msgs.length,
            rounds,
          })
        } catch {
          // skip
        }
      }
    } catch (e) {
      console.debug('[useAgent] refreshSessions failed:', e)
    }
    list.sort((a, b) => b.updatedAt - a.updatedAt)
    sessions.value = list
  }

  /**
   * 切换到指定 session：先 stop 当前流，再加载目标 session 消息
   */
  function switchSession(sessionId: string): void {
    if (sessionId === currentSessionId.value) return
    if (abortController) {
      abortController.abort()
      abortController = null
    }
    const saved = loadState(sessionId)
    if (!saved) {
      console.debug('[useAgent] switchSession: no saved state for', sessionId)
      return
    }
    currentSessionId.value = saved.sessionId
    eventOffset = saved.eventOffset
    messages.value = saved.messages.map((m) => ({ ...m }))
    status.value = 'idle'
    lastError.value = ''
    lastErrorCode.value = ''
    saveState()
  }

  /**
   * 创建新 session 并切到它（不删除原 session，留作历史）
   */
  function newSession(): void {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
    if (currentSessionId.value && messages.value.length > 0) {
      // 当前会话已存在消息 → 持久化保留作为历史
      saveState()
    }
    currentSessionId.value = generateSessionId()
    eventOffset = 0
    lastEventId = 0
    messages.value = []
    status.value = 'idle'
    lastError.value = ''
    lastUserInput.value = ''
    sessionCreatedAt.value = Date.now()
    // 如果 API 返回了默认模型，新会话时重置为该默认模型
    if (apiDefaultModel.value) {
      activeModel.value = apiDefaultModel.value
    }
    saveState()
    refreshSessions()
  }

  /**
   * 删除一个 session（不可恢复）
   */
  function deleteSession(sessionId: string): void {
    try {
      localStorage.removeItem(STORAGE_PREFIX + sessionId)
    } catch {
      // ignore
    }
    if (sessionId === currentSessionId.value) {
      currentSessionId.value = ''
      messages.value = []
      eventOffset = 0
      lastEventId = 0
    }
    refreshSessions()
  }

  // ─── SSE 解析器 ─────────────────────────────────────────────────────────

  /**
   * 解析 SSE 响应流并 dispatch 事件到 reactive state
   *
   * SSE 格式：
   *   data: {"type": "text_delta", "data": "..."}\n
   *   data: {"type": "stream_end", "data": ""}\n
   *   \n
   *
   * 注：agent Go 服务推送的每行已经是一个 `{type, data}` JSON；
   *     data 字段是 stringified payload。
   */
  /**
   * 解析 SSE 流，返回是否收到过至少一个有效事件
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
  /**
   * SSE 协议分发器。
   *
   * 行为：
   *   - protocol='agui'  → 走 AG-UI parser（composables/useAGUIParser.ts）
   *   - protocol='legacy'→ 走原始自定义 SSE（保持原 processSSE 行为）
   *
   * 调用方（send / confirmTool / runResumeChain）从 response.headers 读
   * `X-Agent-Protocol` 后再传入本函数：
   *   - 'agui'  → useAGUIParser
   *   - 缺失/其他值 → legacy
   *
   * 返回值结构 { received, streamEnded, morePending } 与原 processSSE 一致，
   * 链式 resume / 错误处理路径不需要修改。
   */
  async function processSSE(
    stream: ReadableStream<Uint8Array> | null,
    protocol: 'agui' | 'legacy' = 'legacy',
  ): Promise<{
    received: boolean
    streamEnded: boolean
    morePending: boolean
  }> {
    if (protocol === 'agui') {
      return processAGUISSEWithHandlers(stream)
    }
    return processLegacySSE(stream)
  }

  /**
   * 原始自定义 SSE 事件流解析（AG-UI 未启用时的回退路径）。
   *
   * 行为与重构前完全一致：
   *   - 逐行扫描 `data: ` 字段、尝试 JSON.parse
   *   - sequence id 去重（rememberSequence）
   *   - lastEventId 同步
   *   - 原始事件日志（pushRawEvent + Task 27 DOM counter）
   *   - 链式 resume 决策（lastStreamStatus = synced / more_pending）
   *   - 流关闭后 finalize + 修正 status
   */
  async function processLegacySSE(stream: ReadableStream<Uint8Array> | null): Promise<{
    received: boolean
    streamEnded: boolean
    morePending: boolean
  }> {
    if (!stream) return { received: false, streamEnded: false, morePending: false }

    const reader = stream.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let received = false
    let streamEnded = false
    /** 最后一个 stream_status 事件：synced 还是 more_pending（用于 chain 决策） */
    let lastStreamStatus: 'synced' | 'more_pending' | null = null

    /**
     * 处理一个完整 SSE 事件（已被 \n\n 分隔）
     * 维护 currentEventId 用于关联同一事件内的 data 行
     */
    function consumeEvent(rawEvent: string): void {
      if (!rawEvent.trim()) return
      let currentEventId: number | null = null
      const lines = rawEvent.split('\n')
      for (const line of lines) {
        // SSE 标准：id: N —— 用于断点续传
        if (line.startsWith('id:')) {
          const n = parseInt(line.slice(3).trim(), 10)
          if (Number.isFinite(n) && n >= 0) currentEventId = n
          continue
        }
        if (!line.startsWith('data: ')) continue
        const payload = line.slice(6).trim()
        if (!payload) continue
        received = true
        try {
          const event = JSON.parse(payload) as AgentEvent
          // ─── Task 27 调试：每个 SSE 事件立即 console.error 打印 ───
          // 看后端到底推了哪些 event.type + data 摘要（特别关注 tool_call / tool_result）
          const dataSummary =
            event.data == null
              ? 'null'
              : typeof event.data === 'string'
                ? event.data.slice(0, 120)
                : JSON.stringify(event.data).slice(0, 200)
          console.error(
            `[useAgent][SSE] type=${event.type} id=${currentEventId ?? '-'} data=${dataSummary}`,
          )
          // ⑦ 区：追加到原始事件日志（AgentDebugPanel 可视化展示）
          pushRawEvent(event.type, dataSummary, currentEventId)
          // Task 4.3：sequence 去重。若事件声明了 SSE id (currentEventId)，
          // 且该 id 已在当前 serverInstance 见过 → 整条事件丢弃，不再 dispatch。
          // 注意：未声明 id 的事件（如后端断点续传 stream_status 边界事件）跳过
          // 去重逻辑，避免误丢无 id 的合法事件。
          if (currentEventId !== null) {
            if (!rememberSequence(currentEventId)) {
              console.debug('[useAgent] drop duplicate seq', currentEventId)
              // 重置 currentEventId 避免下一行误用
              currentEventId = null
              continue
            }
            // 关联 SSE id：若同一事件声明了 id，则覆盖到 lastEventId
            // （一个事件只能有一个 data，多个 id 行以最后一个为准）
            lastEventId = currentEventId
            // 重置 currentEventId 避免下一行误用
            currentEventId = null
          }
          if (event.type === 'stream_end') streamEnded = true
          if (event.type === 'stream_status') {
            const payload = parseStreamStatusData(event.data)
            if (payload?.status === 'synced' || payload?.status === 'more_pending') {
              lastStreamStatus = payload.status
            }
          }
          handleAgentEvent(event)
        } catch (e) {
          console.debug('[useAgent] malformed SSE payload:', payload, e)
        }
      }
    }

    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        // SSE 事件以 \n\n 分隔
        const events = buffer.split('\n\n')
        buffer = events.pop() || ''

        for (const rawEvent of events) {
          consumeEvent(rawEvent)
        }
      }
      // 处理 trailing buffer（不带 \n\n 结尾的最后一段）
      if (buffer.trim()) {
        consumeEvent(buffer)
        buffer = ''
      }

      // 流结束：清理未结束的 assistant 消息 + 恢复 idle 状态
      // （防止 server 端未发 stream_end 但连接关闭的边角场景）
      finalizeLastAssistant()
      if (status.value === 'streaming') {
        // 决定 status：是否有 pending 确认？
        const hasPendingConfirm = messages.value.some((m) =>
          m.tool_calls.some((tc) => tc.needsConfirm && tc.status === 'pending'),
        )
        status.value = hasPendingConfirm ? 'confirming' : 'idle'
        saveState()
      }
    } finally {
      try {
        reader.releaseLock()
      } catch {
        // already released
      }
    }

    return {
      received,
      streamEnded,
      morePending: lastStreamStatus === 'more_pending',
    }
  }

  /**
   * AG-UI 协议路径：包装 useAGUIParser.processAGUISSE 把
   * 11 种 AG-UI 事件归一化为内部 AgentEvent，再走 handleAgentEvent。
   *
   * handlers 桥接：
   *   - onEvent      → handleAgentEvent
   *   - rememberSequence → useAgent 内部的去重闭包（与 legacy 共用）
   *   - onRawEvent   → pushRawEvent (⑦ 区调试日志)
   *   - onStreamEnd  → 复用 legacy 的 finalizeLastAssistant + status 修正
   */
  async function processAGUISSEWithHandlers(
    stream: ReadableStream<Uint8Array> | null,
  ): Promise<{ received: boolean; streamEnded: boolean; morePending: boolean }> {
    return processAGUISSEImpl(stream, {
      onEvent: (event) => {
        // 与 processLegacySSE 一致：Task 27 调试 console.error
        const dataSummary =
          event.data == null
            ? 'null'
            : typeof event.data === 'string'
              ? event.data.slice(0, 120)
              : JSON.stringify(event.data).slice(0, 200)
        console.error(
          `[useAgent][AGUI] type=${event.type} data=${dataSummary}`,
        )
        handleAgentEvent(event)
      },
      rememberSequence: (id: number) => {
        // 与 legacy 共用 rememberSequence 闭包（serverInstance 切换时会被清空）
        return rememberSequence(id)
      },
      onRawEvent: (type: string, dataSummary: string, seq?: number | null) => {
        pushRawEvent(type, dataSummary, seq ?? null)
        // 同步 lastEventId（与 legacy 同形）
        if (seq !== null && seq !== undefined && seq > lastEventId) {
          lastEventId = seq
        }
      },
      onStreamEnd: () => {
        // 与 legacy onFinalize 行为一致
        finalizeLastAssistant()
        if (status.value === 'streaming') {
          const hasPendingConfirm = messages.value.some((m) =>
            m.tool_calls.some((tc) => tc.needsConfirm && tc.status === 'pending'),
          )
          status.value = hasPendingConfirm ? 'confirming' : 'idle'
          saveState()
        }
      },
    })
  }

  /**
   * 单个 event type → reactive state dispatch
   */
  function handleAgentEvent(event: AgentEvent): void {
    // 取最后一条 *正在 streaming* 的 assistant 消息作为流式追加目标。
    //
    // Task 7 之前：实现是"最后一条 assistant 消息"，因为 send() 每次
    // 都会推一条新的空 assistant(isStreaming=true)，老的已经
    // 被 stream_end / finalizeLastAssistant 标记为 isStreaming=false。
    //
    // Task 7 之后：compaction 事件会在 messages 中插入 system marker
    // 标记，*并 finalize 掉当前 streaming 的 assistant*。如果下一个
    // text_delta 仍走"最后一条 assistant 消息"路径，它会找到刚被
    // finalize 的老 assistant，把 post-compaction 的文本 append 到
    // pre-compaction 文本后面，marker 漂到消息尾部。
    //
    // 修正：把"messages 尾部为 system 消息"作为 turn 边界的硬信号——
    // 说明上一个 turn 已被边界事件（compaction、未来的 user 切分等）
    // 显式终结，后续 text_delta 应当开一条新 assistant。resume 路径
    // 不触发此分支（resume 不会在 messages 尾部插入 system 消息），
    // 因此原"追加到最后一条 assistant"行为保留。
    const lastAssistant = (): Message => {
      // Turn 边界检测：尾部是 system 消息 → 必须新开 assistant
      const tail = messages.value[messages.value.length - 1]
      const isTurnBoundary = tail && tail.role === 'system'
      if (!isTurnBoundary) {
        // 非 turn 边界：优先 streaming → fallback 到任何 assistant
        for (let i = messages.value.length - 1; i >= 0; i--) {
          const m = messages.value[i]
          if (m.role === 'assistant' && m.isStreaming) return m
        }
        for (let i = messages.value.length - 1; i >= 0; i--) {
          if (messages.value[i].role === 'assistant') return messages.value[i]
        }
      }
      // turn 边界 or 没有 assistant → 创建新 assistant
      const newMsg: Message = {
        role: 'assistant',
        content: '',
        tool_calls: [],
        tool_results: [],
        isStreaming: true,
        eventLog: [], // Task 27：初始化事件顺序日志
      }
      messages.value.push(newMsg)
      return messages.value[messages.value.length - 1]
    }

    switch (event.type) {
      case 'text_delta': {
        const m = lastAssistant()
        const parsed = parseContentDelta(event.data)
        appendSequencedChunk(m, 'content', parsed.seq, parsed.text)
        // Task 27：记录文本事件到达顺序
        if (!m.eventLog) m.eventLog = []
        m.eventLog.push({ type: 'text' })
        break
      }
      case 'reasoning_delta': {
        const m = lastAssistant()
        const parsed = parseContentDelta(event.data)
        appendSequencedChunk(m, 'reasoning', parsed.seq, parsed.text)
        break
      }
      case 'tool_call': {
        const tool = parseToolCallData(event.data)
        if (tool) {
          const m = lastAssistant()
          m.tool_calls.push(tool)
          // Task 27：记录工具调用事件到达顺序
          if (!m.eventLog) m.eventLog = []
          m.eventLog.push({ type: 'tool_call', id: tool.id })
          // Task 3：启动 30s 超时 watchdog。若 30s 内未收到 tool_result，
          // 自动标记 failed + errorCode='TIMEOUT'。timer 会在 tool_result
          // 到达时由 clearToolCallTimeout 清除。
          armToolCallTimeout(tool.id)
        }
        break
      }
      case 'tool_status': {
        const ts = parseToolStatus(event.data)
        if (ts) {
          // 找到对应的 tool_call 并更新 status
          for (const msg of messages.value) {
            const tc = msg.tool_calls.find((t) => t.id === ts.id)
            if (tc) {
              // 状态机严格单向：failed/cancelled 是终态，后续 tool_status 不可降级
              // （典型场景：失败后服务端还推了个 success 给陈旧连接）
              if (tc.status === 'failed' || tc.status === 'cancelled' || tc.status === 'success') {
                break
              }
              tc.status = ts.status
              // 进入 running 状态时记录 startedAt
              if (ts.status === 'running' && tc.startedAt === undefined) {
                tc.startedAt = Date.now()
              }
              break
            }
          }
        }
        break
      }
      case 'tool_result': {
        const result = parseToolResultData(event.data)
        if (result) {
          // AG-UI 协议适配：TOOL_CALL_RESULT 归一化结果只有 {id, result}（无 name），
          // 从前面累积的 tool_calls 按 id 反查补齐 name
          let matchedTc: ToolCall | null = null
          if (!result.name) {
            for (let i = messages.value.length - 1; i >= 0; i--) {
              const m = messages.value[i]
              if (m.role !== 'assistant') continue
              const tc = m.tool_calls.find((t) => t.id === result.id)
              if (tc?.name) {
                result.name = tc.name
                matchedTc = tc
                break
              }
            }
          } else {
            // 已有 name：同样按 id 反查 tool_call，准备联动 status
            for (let i = messages.value.length - 1; i >= 0; i--) {
              const m = messages.value[i]
              if (m.role !== 'assistant') continue
              const tc = m.tool_calls.find((t) => t.id === result.id)
              if (tc) {
                matchedTc = tc
                break
              }
            }
          }
          const m = lastAssistant()
          m.tool_results.push(result)
          // Task 27：记录工具结果事件到达顺序
          if (!m.eventLog) m.eventLog = []
          m.eventLog.push({ type: 'tool_result', id: result.id })

          // ── 状态机联动：tool_result → tool_call.status ──
          // 关键修复：之前 tool_call.status 只看 tool_status 事件，failed 时
          // 仍然显示 success。spec / 用户反馈：tool_result 到达时必须按
          // is_error + status 字段联动外层 tool_call.status。
          // 状态机严格单向：failed/cancelled 是终态（已被 tool_status 设过），
          // 不再被 tool_result 覆盖（防止陈旧重放把 failed 改回 success）。
          if (matchedTc && matchedTc.status !== 'failed' && matchedTc.status !== 'cancelled') {
            const finishedAt = Date.now()
            matchedTc.finishedAt = finishedAt

            // ── errorCode / errorMessage 提取 ──
            // ToolResult 类型契约不包含这两个字段（避免与后端 wire format 紧耦合），
            // 但后端实际可能通过原始 event.data 传 errorCode / errorMessage /
            // message 字段。这里就地再 JSON.parse 一次 raw data 拿这些可选字段。
            // 容错：解析失败 / 字段缺失时退到 fallback。
            let rawErrorCode: string | undefined
            let rawErrorMessage: string | undefined
            let rawOutput: any
            try {
              const raw = typeof event.data === 'string' ? JSON.parse(event.data) : event.data
              if (raw && typeof raw === 'object') {
                const r = raw as Record<string, unknown>
                if (typeof r.errorCode === 'string') rawErrorCode = r.errorCode
                else if (typeof (r as any).code === 'string') rawErrorCode = (r as any).code
                if (typeof r.errorMessage === 'string') rawErrorMessage = r.errorMessage
                else if (typeof (r as any).message === 'string') rawErrorMessage = (r as any).message
                if ('output' in r) rawOutput = (r as any).output
              }
            } catch {
              // raw data 解析失败 → 用 fallback（不阻塞主流程）
            }

            // 后端契约：is_error=true 表示执行失败；status='cancelled' 表示用户取消
            if (result.status === 'cancelled') {
              matchedTc.status = 'cancelled'
            } else if (result.is_error === true) {
              matchedTc.status = 'failed'
              matchedTc.errorCode = rawErrorCode || 'EXEC_FAILED'
              // 错误消息原样透传（后端已本地化），不要在前端硬编码翻译
              matchedTc.errorMessage = rawErrorMessage
                || (typeof result.result === 'string' ? result.result : '')
                || '工具执行失败'
            } else {
              matchedTc.status = 'success'
              // output 字段：后端 AG-UI 格式可能挂在 raw.output 或 result.result
              const output = rawOutput !== undefined
                ? rawOutput
                : (typeof result.result === 'string'
                    ? (() => { try { return JSON.parse(result.result) } catch { return result.result } })()
                    : result.result)
              matchedTc.output = output
            }
            // 收到 result 时清除 30s 超时 timer（避免泄漏 + 误标记 failed）
            clearToolCallTimeout(result.id)
          }
        }
        break
      }
      case 'stream_status': {
        // 后端断点续传信号：
        //   { status: 'synced',       inProgress: bool, maxEventId?: number }
        //   { status: 'more_pending', inProgress: bool, maxEventId?: number }
        // 前端拿到信号后由 runResumeChain 决定下一轮，handleAgentEvent 本身
        // 不修改 status（status 由 stream_end / confirm 决策驱动），但要：
        //   1. 同步服务端声明的 maxEventId 到 lastEventId（覆盖，避免漂移）
        //   2. 通过 console.debug 留痕便于排查
        const payload = parseStreamStatusData(event.data)
        if (payload?.maxEventId !== undefined) {
          if (payload.maxEventId > lastEventId) {
            lastEventId = payload.maxEventId
          }
        }
        if (payload?.status === 'synced') {
          console.debug('[useAgent] stream_status: synced, awaiting new events')
        } else if (payload?.status === 'more_pending') {
          console.debug('[useAgent] stream_status: more_pending, will re-resume')
        } else {
          console.debug('[useAgent] stream_status:', payload)
        }
        break
      }
      case 'stream_end': {
        // 标记最后 assistant 消息流式结束
        for (let i = messages.value.length - 1; i >= 0; i--) {
          if (messages.value[i].role === 'assistant' && messages.value[i].isStreaming) {
            messages.value[i].isStreaming = false
            break
          }
        }
        // 决定下一个 status：
        //   - 如果有 pending tool_call（needsConfirm=true 且 status=pending）→ confirming
        //   - 否则 → idle
        const hasPendingConfirm = messages.value.some((m) =>
          m.tool_calls.some((tc) => tc.needsConfirm && tc.status === 'pending'),
        )
        status.value = hasPendingConfirm ? 'confirming' : 'idle'
        // Task 11：每个 stream_end 清掉队首 pending 消息。
        // 时序说明：当 in-flight turn 结束时，stream_end 触发
        // 队首清空（即使这个 turn 实际处理的是该消息，UI 端
        // 「已排队」标签与实际处理节拍同步）；后续 queued turn
        // 各自的 stream_end 同样按 FIFO 清队。
        if (pendingMessages.value.length > 0) {
          const first = pendingMessages.value.shift()!
          first.pending = false
        }
        // v2 多轮/分支剧本清理：stream_end 到达意味着整个 session 结束
        // （不管是正常结束、超时、还是用户中断）。把 v2 state 复位，
        // 否则用户开新会话时 MockBranchChoiceBar 会"残留显示"。
        // mockPresets 不在此处清空（它走"chip 永远覆盖显示"语义）。
        mockBranchChoices.value = []
        mockBranchPrompt.value = ''
        mockRoundState.value = null
        currentMockScenario.value = ''
        break
      }
      case 'stream_error': {
        // 后端在 SSE 流过程中遇到不可恢复错误时推送此事件。
        // data 字段包含错误信息（JSON 字符串或纯文本）。
        // 前端收到后应：
        //   1. finalize 当前 streaming 的 assistant 消息
        //   2. 记录错误信息到 lastError / lastErrorCode
        //   3. 切换 status 为 'error'
        //   4. 不再继续处理后续事件（连接即将关闭）
        finalizeLastAssistant()
        const errorMsg = parseContentDelta(event.data).text || '服务端流式传输发生未知错误'
        lastError.value = errorMsg
        lastErrorCode.value = 'upstream_error'
        status.value = 'error'
        // v2 多轮/分支剧本清理：异常路径下也要清掉 chip 状态，
        // 否则用户关闭错误弹窗后 MockBranchChoiceBar 还会显示。
        mockBranchChoices.value = []
        mockBranchPrompt.value = ''
        mockRoundState.value = null
        currentMockScenario.value = ''
        console.error('[useAgent] stream_error:', errorMsg)
        break
      }
      case 'stream_start': {
        // Mock 模式信号：后端在 cfg.Agent.MockMode != "off" 时，SSE 流首
        // 个事件就是 stream_start，data 形状是 { mock: true, scenario: "..." }。
        // 拿到就置 isMockMode = true，AgentChat 顶部展示"🧪 模拟"badge。
        // 容忍旧后端/不 mock 模式：data 可能为空 / 不含 mock 字段 → 不动状态。
        try {
          const raw = JSON.parse(event.data) as { mock?: unknown; scenario?: unknown } | null
          if (raw && raw.mock === true) {
            isMockMode.value = true
            mockScenario.value = String(raw.scenario ?? '')
          }
        } catch {
          // data 不是 JSON → 非 mock 信号，忽略
        }
        break
      }
      case 'mock_presets': {
        // Mock 模式剧本预设推送：
        //   - 后端在 stream_start 之后立即推一次（phase=initial）
        //   - 高级剧本可在 mid-scenario 任意 step 再推（phase=after_round_2 等）
        // data 形状：{ scenario, phase, presets: MockPreset[] }
        //
        // 前端覆盖行为：每次 mock_presets 事件都**完整替换**当前 mockPresets。
        // 这样 mid-scenario 更新（高级剧本的"随进度更新"）天然就是覆盖语义。
        try {
          const raw = JSON.parse(event.data) as
            | { scenario?: unknown; phase?: unknown; presets?: unknown }
            | null
          if (!raw) break
          const list = Array.isArray(raw.presets) ? (raw.presets as MockPreset[]) : []
          // 兼容后端发送 MockPreset 时字段大小写：id/label/userText/tooltip
          // 一律 lowercase 归一化（后端 json tag 是 lowercase，前端契约保持一致）。
          mockPresets.value = list
            .filter((p): p is MockPreset => !!p && typeof p === 'object' && typeof p.id === 'string' && typeof p.userText === 'string')
            .map((p) => ({
              id: p.id,
              label: String(p.label ?? p.id),
              userText: p.userText,
              icon: typeof p.icon === 'string' ? p.icon : undefined,
              tooltip: typeof p.tooltip === 'string' ? p.tooltip : undefined,
            }))
          mockPresetsPhase.value = String(raw.phase ?? '')
          mockPresetsScenario.value = String(raw.scenario ?? '')
          console.debug(
            '[useAgent] mock_presets:',
            mockPresets.value.length,
            'presets, phase=',
            mockPresetsPhase.value,
            ', scenario=',
            mockPresetsScenario.value,
          )
        } catch (e) {
          console.debug('[useAgent] mock_presets parse failed:', e)
        }
        break
      }

      case 'mock_presets_clear': {
        // 故意 noop：chip 在 mock 模式开启期间**永远覆盖显示**（覆盖式 UI）。
        // 收到 clear 事件**不**清空 mockPresets —— 仅当用户**主动**退出 mock 模式
        // （setMockMode("off") 触发 X-Mock-Mode header 变化）时，isMockMode 变 false，
        // AgentChat 的 v-if 自然不再渲染 MockPresetBar。
        // 后端未来若需要主动清 chip（极少见），可以走一个新的 `mock_presets_reset` 事件。
        console.debug('[useAgent] mock_presets_clear (ignored, chip 永远覆盖显示)')
        break
      }

      case 'mock_branch_choice': {
        // v2 多轮/分支剧本：剧本在 step.branch_choice=true 时推。
        // data 形状：{ scenario, prompt, choices: MockBranch[], phase }
        // 关键不变量：
        //   1. mockBranchChoices 一旦被设置，AgentChat 必须显示 MockBranchChoiceBar
        //      并禁用 send 按钮（用户必须点 chip 或键入文本才能继续）
        //   2. currentMockScenario 同步更新，否则 pickMockBranch 不知道推回哪个剧本
        //   3. phase 写入 mockRoundState，使 mockScenarioPaused = true
        // 不会在此处清理 mockRoundState —— 它的"运行中"状态可能仍有用。
        try {
          const raw = JSON.parse(event.data) as
            | {
                scenario?: unknown
                prompt?: unknown
                choices?: unknown
                phase?: unknown
              }
            | null
          if (!raw) break
          const list = Array.isArray(raw.choices) ? (raw.choices as MockBranch[]) : []
          mockBranchChoices.value = list
            .filter((b): b is MockBranch =>
              !!b && typeof b === 'object' && typeof (b as MockBranch).id === 'string',
            )
            .map((b) => ({
              id: b.id,
              label: String(b.label ?? b.id),
              icon: typeof b.icon === 'string' ? b.icon : undefined,
              description: typeof b.description === 'string' ? b.description : undefined,
            }))
          mockBranchPrompt.value = String(raw.prompt ?? '')
          if (typeof raw.scenario === 'string' && raw.scenario.length > 0) {
            currentMockScenario.value = raw.scenario
          }
          // 强制 paused：即使后端没推 mock_round_state 事件，单凭 mock_branch_choice
          // 到达也意味着剧本在等用户选 branch。
          mockRoundState.value = {
            roundIdx: mockRoundState.value?.roundIdx ?? 0,
            totalRounds: mockRoundState.value?.totalRounds ?? 1,
            phase: 'awaiting_branch_choice',
            context: mockRoundState.value?.context ?? {},
            scenario: currentMockScenario.value,
          }
          console.debug(
            '[useAgent] mock_branch_choice:',
            mockBranchChoices.value.length,
            'branches, prompt="',
            mockBranchPrompt.value.slice(0, 40),
            '..., scenario=',
            currentMockScenario.value,
          )
        } catch (e) {
          console.debug('[useAgent] mock_branch_choice parse failed:', e)
        }
        break
      }

      case 'mock_round_state': {
        // v2 多轮/分支剧本：剧本报告当前 round 进度。
        // data 形状：{ roundIdx, totalRounds, phase, context, scenario }
        // 与 mock_branch_choice 配合使用：round_state 永远先到（宣告"我现在到
        // round N 了"），branch_choice 后到（"我在这一步等你"）。
        // 也可在 phase='running' 时单独到达，告知前端 round 推进但不需要用户操作。
        try {
          const raw = JSON.parse(event.data) as
            | {
                roundIdx?: unknown
                totalRounds?: unknown
                phase?: unknown
                context?: unknown
                scenario?: unknown
              }
            | null
          if (!raw) break
          const next: MockRoundState = {
            roundIdx: typeof raw.roundIdx === 'number' ? raw.roundIdx : 0,
            totalRounds: typeof raw.totalRounds === 'number' ? raw.totalRounds : 1,
            phase: typeof raw.phase === 'string' ? raw.phase : 'running',
            context:
              raw.context && typeof raw.context === 'object'
                ? (raw.context as Record<string, unknown>)
                : {},
            scenario:
              typeof raw.scenario === 'string' && raw.scenario.length > 0
                ? raw.scenario
                : mockRoundState.value?.scenario,
          }
          mockRoundState.value = next
          // scenario 也要单独缓存（mock_branch_choice 也写一次）
          if (typeof raw.scenario === 'string' && raw.scenario.length > 0) {
            currentMockScenario.value = raw.scenario
          }
          console.debug(
            '[useAgent] mock_round_state: round',
            next.roundIdx,
            '/',
            next.totalRounds,
            'phase=',
            next.phase,
            'scenario=',
            currentMockScenario.value,
          )
        } catch (e) {
          console.debug('[useAgent] mock_round_state parse failed:', e)
        }
        break
      }

      case 'compaction': {
        // Task 7：上下文自动压缩事件。
        //
        // 后端在 messages token 数越过 80% 窗口阈值时，调用 LLM
        // 生成老消息的 summary，并在 messages 头部插入一条
        // role='system', name='compaction' 的合成消息。
        // 本事件只是告知前端"这里发生了压缩"，附带：
        //   - summary_text:   LLM 生成的 summary 文本（前端不渲染）
        //   - replaced_message_count: 被替换的旧消息数
        //   - triggered_at_ms: unix 毫秒时间戳
        //
        // 前端在 messages 列表里也插入一条 role='system' 的合成
        // 消息，content 固定为 CONTEXT_COMPACTION_MARKER。
        // renderTurnItems 会把这条消息转成 ContextCompactionDivider
        // （不可展开的水平分隔线），位置恰好对应被压缩掉的旧
        // 消息所在位置。summary 文本本身不展示——压缩是不可逆的，
        // 用户看到「这里有 N 条消息被合并为一段总结」即可。
        //
        // 关键时序：先 finalize 当前 streaming 的 assistant 消息
        // （mark isStreaming=false），再 push system marker。如果
        // 不 finalize，下一个 text_delta 仍会找到这条"还在流式"
        // 的老 assistant，把 post-compaction 的内容 append 上去，
        // marker 漂到消息尾部——视觉上分隔线就不在压缩点的位置了。
        // 配合 Task 7 改的 lastAssistant 行为（只取 streaming
        // assistant），post-compaction 的 text_delta 会创建一条新
        // 的 assistant 消息，marker 自然落在两条 assistant 之间。
        //
        // 容错：compaction 事件不应让 status 切换；不影响
        // streaming 状态。重复到达时直接再 push 一条 marker
        // （后端保证只发一次）。
        finalizeLastAssistant()
        const data = parseCompactionData(event.data)
        // 不论 data 是否解析成功都插入 marker（即使解析失败也
        // 给用户一个 divider 提示此处发生了压缩）。
        messages.value.push({
          role: 'system',
          content: CONTEXT_COMPACTION_MARKER,
          tool_calls: [],
          tool_results: [],
        })
        if (data) {
          console.debug(
            '[useAgent] compaction: replaced',
            data.replaced_message_count,
            'messages @',
            data.triggered_at_ms
              ? new Date(data.triggered_at_ms).toISOString()
              : '(no timestamp)',
          )
        }
        break
      }

      // ====== AG-UI 协议新增事件类型（useAGUIParser 归一化输出） ======

      case 'tool_call_args': {
        // AG-UI TOOL_CALL_ARGS：把 args 增量追加到已存在的 tool_call.args 字段
        // 找最后一条 assistant 消息里 id 匹配的 tool_call
        try {
          const payload = typeof event.data === 'string' ? JSON.parse(event.data) : (event.data as any)
          if (!payload || typeof payload !== 'object' || !payload.id || typeof payload.argsDelta !== 'string') break
          // 遍历所有 assistant 消息找到匹配的 tool_call（FIFO 顺序匹配，
          // 保留 processSSE 阶段的 lastAssistant 选择语义）
          for (let i = messages.value.length - 1; i >= 0; i--) {
            const m = messages.value[i]
            if (m.role !== 'assistant') continue
            const tc = m.tool_calls.find((t) => t.id === payload.id)
            if (tc) {
              tc.args = (tc.args || '') + payload.argsDelta
              break
            }
          }
        } catch {
          // 解析失败：忽略此增量 args（不破坏已存在的 tc.args）
        }
        break
      }

      case 'state_snapshot': {
        // AG-UI STATE_SNAPSHOT：会话级共享状态，目前仅做调试记录 + 持久化兜底
        try {
          const payload = typeof event.data === 'string' ? JSON.parse(event.data) : (event.data as any)
          if (typeof console.debug === 'function') {
            const keys = payload && typeof payload === 'object' && payload.state && typeof payload.state === 'object'
              ? Object.keys(payload.state)
              : []
            console.debug('[useAgent] agui state_snapshot keys:', keys)
          }
        } catch {
          // 静默
        }
        break
      }

      case 'messages_snapshot': {
        // AG-UI MESSAGES_SNAPSHOT：完整消息快照（断点续传对齐用）
        // 当前实现：仅记录关键信息，不直接覆盖前端 messages（避免与本地编辑冲突）。
        try {
          const payload = typeof event.data === 'string' ? JSON.parse(event.data) : (event.data as any)
          if (typeof console.debug === 'function') {
            const count = payload && Array.isArray(payload.messages) ? payload.messages.length : 0
            console.debug('[useAgent] agui messages_snapshot count:', count)
          }
        } catch {
          // 静默
        }
        break
      }

      default:
        // 未知 type 静默忽略
        break
    }

    eventOffset++
    saveState()
  }

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
    options?: { mode?: 'start' | 'steer' | 'queue' | 'mock_resume'; scenario?: string; attachments?: Attachment[] },
  ): Promise<void> {
    const mode = options?.mode ?? 'start'
    if (mode === 'queue') {
      await sendQueued(text, options?.attachments ?? [])
      return
    }

    if (status.value === 'streaming' || status.value === 'confirming') {
      console.debug('[useAgent] send: ignored (busy)')
      return
    }

    // 关闭上一轮的错误条
    lastError.value = ''
    lastErrorCode.value = ''
    lastUserInput.value = text

    // Task 4：先拉取 /api/health 取 serverInstanceId。每次 send 都刷一次——
    // 开销可忽略（一次 GET，且 health 不带 SSE body），但能在"中途服务器
    // 重启"时立刻把 seenSequences 清空，避免后续事件被错误判为重复。
    await refreshServerInstance()

    // 第一次发送：分配新 session
    if (!currentSessionId.value) {
      currentSessionId.value = generateSessionId()
    }
    eventOffset = 0
    lastEventId = 0

    // 推 user 消息 + 空 assistant 占位
    // Task 12：如果有 attachments，content 编为 multimodal 数组；
    //          否则保持原样（纯字符串）以保持向后兼容。
    const attachments = options?.attachments ?? []
    const userContent: string | MessageContentPart[] =
      attachments.length > 0
        ? serializeAttachments(text, attachments)
        : text

    messages.value.push({
      role: 'user',
      content: userContent,
      tool_calls: [],
      tool_results: [],
    })
    messages.value.push({
      role: 'assistant',
      content: '',
      tool_calls: [],
      tool_results: [],
      isStreaming: true,
    })

    status.value = 'streaming'

    // T15 unblock：mock_resume 模式在 fetch 前清空 chip + 把 round state
    // 切到 "running" —— UI 立即从 paused 切到 spinner，避免视觉残留
    // （stale "awaiting_user_input" 与新事件流冲突）。
    // 后续 mock_round_state{phase:resumed/in_progress} 事件会覆盖此处写入。
    if (mode === 'mock_resume') {
      mockBranchChoices.value = []
      mockBranchPrompt.value = ''
      const curRound = mockRoundState.value?.roundIdx ?? 0
      const totalRounds = mockRoundState.value?.totalRounds ?? 0
      mockRoundState.value = {
        scenario: currentMockScenario.value,
        roundIdx: curRound,
        totalRounds,
        phase: 'running',
        context: { ...(mockRoundState.value?.context ?? {}) },
      }
    }

    saveState()
    refreshSessions()

    abortController = new AbortController()
    // 30s 超时保护：如果后端长时间无响应，自动中断
    let timedOut = false
    const timeoutId = setTimeout(() => {
      timedOut = true
      if (abortController) abortController.abort()
    }, 30_000)

    // ── 构建消息列表：历史对话（system prompt 由后端从 config 注入） ──
    // Task 12：content 可能是 string 或 multimodal 数组，原样透传。
    const apiMessages: Array<{ role: string; content: string | MessageContentPart[] }> = []

    // 追加历史消息（不含空的 assistant 占位消息）。
    // Task 7：跳过 role='system' + content=CONTEXT_COMPACTION_MARKER
    // 的合成消息——后端在触发压缩时已经把这段 summary 写进了
    // 自己的 session 持久化层，前端再回送一份就是重复。
    for (const m of messages.value) {
      if (m.role === 'assistant' && !m.content && !m.reasoning && m.tool_calls.length === 0) continue
      // Task 7：跳过 system + CONTEXT_COMPACTION_MARKER 的合成消息。
      // Task 12：content 可能是 multimodal 数组，这种情况下不可能 ==
      // CONTEXT_COMPACTION_MARKER（string），TS 也允许此比较但用 typeof
      // 守卫更安全。
      if (m.role === 'system' && typeof m.content === 'string' && m.content === CONTEXT_COMPACTION_MARKER) continue
      apiMessages.push({ role: m.role, content: m.content })
    }

    try {
      console.debug('[useAgent] send() starting fetch to', `${getAgentBase()}/api/chat`, 'mode=', mode)
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
      const sendAGUIHeader = shouldSendAGUIHeader()
      const fetchHeaders: Record<string, string> = {
        'Content-Type': 'application/json',
        'Accept': 'text/event-stream',
        ...(sendAGUIHeader ? { 'X-Agent-Protocol': 'agui' } : {}),
      }
      // T15 unblock：mode === 'mock_resume' 时把 scenario 透传给后端，
      // 否则 MockEngineV2 找不到对应的 stateful 实例 → 400 错误。
      // 后端 handleMockResume 据此：(1) 在 mockScenariosV2 中查找剧本；
      // (2) 调 mockV2SessionEngines 取出 / 创建 stateful 引擎；
      // (3) 调 engine.Resume(userText) 推下一轮事件。
      const scenarioForBody =
        mode === 'mock_resume'
          ? options?.scenario ?? currentMockScenario.value ?? undefined
          : undefined
      const response = await fetch(`${getAgentBase()}/api/chat`, {
        method: 'POST',
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
      })

      if (!response.ok) {
        // 关键：解析后端 JSON body 拿到真正的 message 字段，而不是只显示
        // "HTTP 503: Service Unavailable"。后端 handleAgentChat 在缺 API Key
        // 等场景下会返回 {error, message, ...}，前端要把 message 透给用户。
        throw await buildHttpError(response, '/api/chat')
      }

      if (!response.body) {
        throw new Error('响应体为空（可能被代理或网络中间层截断）')
      }

      // Mock 模式 header 检测（备份信号）：SSE stream_start 事件是主信号，
      // 但如果首条事件到达前 header 已被读取，这里先把状态置好，避免 UI
      // 看到一段无 badge 的"普通"回复再被刷成 mock。
      // response.headers 可能为 undefined（部分代理 / 测试 mock），用 ?. 兼容。
      const mockHeader = response.headers?.get('X-Mock-Mode')
      if (mockHeader) {
        isMockMode.value = true
        mockScenario.value = response.headers?.get('X-Mock-Scenario') ?? ''
      }

      // 协议分发：根据后端响应 X-Agent-Protocol 决定走 AG-UI parser 还是 legacy
      // response.headers 可能为 undefined（部分代理 / 测试 mock），用 ?. 兼容。
      const responseProtocol = response.headers?.get('X-Agent-Protocol') === 'agui' ? 'agui' : 'legacy'
      const result = await processSSE(response.body, responseProtocol)

      // 流结束但未收到任何事件 → 后端无响应
      if (!result.received) {
        throw new Error('服务端无响应：连接已关闭但未返回任何数据')
      }

      // 收到了事件但 assistant 内容仍为空且无工具调用 → 异常空回复
      const lastAssistant = [...messages.value].reverse().find((m) => m.role === 'assistant')
      if (lastAssistant && !lastAssistant.content && lastAssistant.tool_calls.length === 0) {
        console.error('[useAgent] WARNING: stream ended with empty assistant reply — marking user msg as errored')
        const lastUserMsg = [...messages.value].reverse().find((m) => m.role === 'user')
        if (lastUserMsg) lastUserMsg.error = '服务端返回空回复，请重试'
        status.value = 'idle'
      }
    } catch (e: any) {
      // 找到本次发送的 user 消息，标记错误（每条消息独立错误状态）
      const lastUserMsg = [...messages.value].reverse().find((m) => m.role === 'user')
      if (e?.name === 'AbortError') {
        if (timedOut) {
          console.error('[useAgent] send timed out (30s)')
          if (lastUserMsg) lastUserMsg.error = '请求超时（30秒内服务端无响应），请检查网络或稍后重试'
          status.value = 'idle'
        } else {
          console.debug('[useAgent] send aborted by user')
          // 用户主动停止：不标记错误
        }
        finalizeLastAssistant()
        if (status.value !== 'idle') status.value = 'idle'
      } else {
        let detail = e?.message || String(e)
        // 区分 CORS 预检失败 / 网络断开 / 服务器返回：
        //   TypeError: Failed to fetch（或 iOS Safari 的"Load failed"）通常是
        //     CORS 预检失败 / mixed content blocked / 端口不通 — 浏览器拒绝跨域 POST
        //   这里把诊断信息 dump 到 console.error，下次出问题时 DevLogs 一眼能定位
        if (e?.name === 'TypeError' && /Failed to fetch|Load failed/i.test(detail)) {
          const ctx = getAgentApiBaseContext()
          console.error('[useAgent] send failed (likely CORS preflight / network / mixed content):', {
            base: ctx.base,
            source: ctx.source,
            isNative: ctx.isNative,
            env: ctx.env,
            sampleUrl: ctx.sampleUrl,
            pageOrigin: typeof location !== 'undefined' ? location.origin : '(no location)',
            requestUrl: `${ctx.base}/api/chat`,
            aguiHeaderSent: shouldSendAGUIHeader(),
          })
          detail = `无法连接 Agent API (${ctx.base}) — 检查 CORS 预检 / 网络 / 服务器可达性`
        }
        console.error('[useAgent] send failed:', detail)
        if (lastUserMsg) lastUserMsg.error = detail
        // 把后端 buildHttpError 挂的 .code 提取出来（'no_api_key' / 'upstream_error' / 等）。
        // chat UI 据此可以展示"去 AI 设置"快捷按钮，让用户从对话流直达修复点，
        // 避免"我保存了 key 但 chat 还是 503"的卡死循环（用户的 6 条日志就是这个场景）。
        const errCode = e?.code as 'no_api_key' | 'upstream_error' | 'invalid_json' | 'unknown' | undefined
        lastErrorCode.value = errCode ?? 'unknown'
        showToast({ message: detail, duration: 3000, color: 'danger' })
        status.value = 'idle'
        finalizeLastAssistant()
      }
    } finally {
      clearTimeout(timeoutId)
      abortController = null
      saveState()
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
    if (status.value !== 'streaming' && status.value !== 'confirming') {
      console.debug('[useAgent] sendQueued: ignored (no active turn to queue after)')
      return
    }
    if (!currentSessionId.value) {
      console.debug('[useAgent] sendQueued: ignored (no active session)')
      return
    }

    lastError.value = ''

    // Task 12：multimodal content 数组 vs 纯字符串
    const userContent: string | MessageContentPart[] =
      attachments.length > 0
        ? serializeAttachments(text, attachments)
        : text

    // Push user 消息（带 pending=true），并把它登记到 pendingMessages。
    // 两个数组共享同一对象引用：messages.value 渲染聊天气泡，
    // pendingMessages 暴露给 UI 渲染「已排队：xxx」提示。
    const userMsg: Message = {
      role: 'user',
      content: userContent,
      tool_calls: [],
      tool_results: [],
      pending: true,
    }
    messages.value.push(userMsg)
    pendingMessages.value.push(userMsg)
    saveState()
    refreshSessions()

    // 复用 send() 的 apiMessages 构建逻辑（不能调 send() 本身——
    // 那会重置 eventOffset 并破坏当前 SSE 流的状态机）。
    const apiMessages: Array<{ role: string; content: string | MessageContentPart[] }> = []
    for (const m of messages.value) {
      if (m.role === 'assistant' && !m.content && !m.reasoning && m.tool_calls.length === 0) continue
      if (m.role === 'system' && typeof m.content === 'string' && m.content === CONTEXT_COMPACTION_MARKER) continue
      apiMessages.push({ role: m.role, content: m.content })
    }

    try {
      console.debug('[useAgent] sendQueued() POST /api/chat mode=queue')
      const response = await fetch(`${getAgentBase()}/api/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          sessionId: currentSessionId.value,
          model: activeModel.value,
          temperature: activeTemperature.value,
          messages: apiMessages,
          deviceId: getDeviceIdSync(),
          mode: 'queue',
        }),
      })
      // queue 模式服务端约定返回 202 Accepted（chat.go 中由
      // HandleChat 在 mode=='queue' 时显式 WriteHeader 202）。
      if (response.status !== 202) {
        throw new Error(`HTTP ${response.status}: ${response.statusText || '排队失败'}`)
      }
      console.debug('[useAgent] sendQueued: 202 Accepted, message parked on server')
    } catch (e: any) {
      const detail = e?.message || String(e)
      console.error('[useAgent] sendQueued failed:', detail)
      // 出错：把 user 消息从 pendingMessages 摘除并标记 error（保留在
      // messages.value 以便用户看到失败提示）。
      const idx = pendingMessages.value.indexOf(userMsg)
      if (idx !== -1) pendingMessages.value.splice(idx, 1)
      userMsg.error = '排队失败：' + detail
      userMsg.pending = false
      showToast({ message: '排队失败', duration: 3000, color: 'danger' })
      saveState()
    }
  }

  /**
   * 4-决策确认工具调用
   */
  async function confirmTool(toolCallId: string, decision: Decision): Promise<void> {
    if (!currentSessionId.value) {
      console.debug('[useAgent] confirmTool: no active session')
      return
    }

    if (abortController) {
      abortController.abort()
      abortController = null
    }

    // 找到对应的 tool_call，把它的 status 标记为 'running' 表示处理中
    let targetTool: ToolCall | null = null
    for (const msg of messages.value) {
      const tc = msg.tool_calls.find((t) => t.id === toolCallId)
      if (tc) {
        targetTool = tc
        break
      }
    }
    if (targetTool) {
      // 用户做决策期间：保持 needsConfirm 但 status 标记 'running' 表示处理中
      targetTool.status = 'running'
    }

    status.value = 'streaming'
    saveState()

    abortController = new AbortController()
    try {
      // AG-UI 协议协商（与 send() 一致）
      // Accept: text/event-stream —— 必传，触发 useProxiedFetch 走 streamStart，
      // 否则 native 端走 fetchOnce 一次性读完所有 chunk，无流式效果。
      // 详见 useAgent.send() 注释。
      const sendAGUIHeader = shouldSendAGUIHeader()
      const response = await fetch(`${getAgentBase()}/api/confirm`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Accept': 'text/event-stream',
          ...(sendAGUIHeader ? { 'X-Agent-Protocol': 'agui' } : {}),
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
      })

      if (!response.ok) {
        throw await buildHttpError(response, '/api/confirm')
      }

      // 协议分发
      const responseProtocol = response.headers?.get('X-Agent-Protocol') === 'agui' ? 'agui' : 'legacy'
      await processSSE(response.body, responseProtocol)
    } catch (e: any) {
      if (e?.name === 'AbortError') {
        console.debug('[useAgent] confirmTool aborted')
        if (targetTool) targetTool.status = 'pending'
        status.value = 'confirming'
      } else {
        console.error('[useAgent] confirmTool failed:', e instanceof Error ? `${e.name}: ${e.message}` : String(e))
        if (targetTool) targetTool.status = 'pending'
        showToast({ message: 'Confirm request failed', duration: 2000, color: 'danger' })
        status.value = 'confirming'
      }
    } finally {
      abortController = null
      saveState()
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
    const sessionId = findLatestPersistedSession()
    if (!sessionId) return

    const saved = loadState(sessionId)
    if (!saved) return

    currentSessionId.value = saved.sessionId
    eventOffset = saved.eventOffset || 0
    lastEventId = saved.lastEventId || 0
    // 恢复 messages
    messages.value.splice(0, messages.value.length, ...(saved.messages || []))
    status.value = saved.status || 'idle'

    // 如果上次是 streaming 状态，主动 resume 追平进度
    if (status.value === 'streaming') {
      await runResumeChain()
    }
  }

  /**
   * 实际跑一轮 resume，处理 `stream_status` 事件决定是否链式续传。
   * 与 resume() 分离是为了支持 stream_status.more_pending 时递归调用。
   */
  async function runResumeChain(maxHops = 32): Promise<void> {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
    abortController = new AbortController()
    const controller = abortController
    let hopsLeft = maxHops
    try {
      // 链式 resume：每轮处理 processSSE 的信号决定下一步
      //   - streamEnded=true       → 退出循环（服务端显式 stream_end）
      //   - morePending=true       → 立刻下一轮（hopsLeft-- 防止无限递归）
      //   - 完全没收到事件          → 退出（流自然关闭且没新事件，等同于 synced）
      //   - 收到 synced            → 退出（保持 streaming 状态由后续触发）
      //   - status 切到非 streaming → 退出
      while (hopsLeft-- > 0) {
        const headerLastEventId = lastEventId > 0 ? String(lastEventId) : undefined
        // AG-UI 协议协商（与 send() / confirmTool() 一致）
        // Accept: text/event-stream —— 必传，触发 useProxiedFetch 走 streamStart，
        // 否则 native 端走 fetchOnce 一次性读完所有 chunk，无流式效果。
        // 详见 useAgent.send() 注释。
        const sendAGUIHeader = shouldSendAGUIHeader()
        const response = await fetch(`${getAgentBase()}/api/resume`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Accept': 'text/event-stream',
            // SSE 标准 Last-Event-ID：与 EventSource 协议一致
            ...(headerLastEventId ? { 'Last-Event-ID': headerLastEventId } : {}),
            ...(sendAGUIHeader ? { 'X-Agent-Protocol': 'agui' } : {}),
          },
          body: JSON.stringify({
            sessionId: currentSessionId.value,
            // body 字段名：与后端 handleAgentResume 的 lastEventId 字段对齐
            // （不再使用旧的 offset 字段）
            lastEventId: lastEventId,
          }),
          signal: controller.signal,
        })

        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`)
        }

        // 协议分发：response.headers 可能为 undefined（部分代理 / 测试 mock），
        // 用 ?. 兼容。后端正常响应总会带 X-Agent-Protocol header。
        const responseProtocol = response.headers?.get('X-Agent-Protocol') === 'agui' ? 'agui' : 'legacy'
        const { received, streamEnded, morePending } = await processSSE(response.body, responseProtocol)

        // 收尾判定
        if (streamEnded) break                  // 服务端显式收尾 → 退出
        if (morePending) continue               // 服务端还有事件未推完 → 立刻下一轮
        if (!received) break                    // 流自然关闭且无事件 → 退出
        if (status.value !== 'streaming') break // 已被 confirm/cancel 切走 → 退出
        // 收到事件但 neither morePending nor streamEnded：保守退出，
        // 避免无限循环（实际上 server 应该会推 stream_status 或 stream_end）
        break
      }
    } catch (e: any) {
      if (e?.name !== 'AbortError') {
        console.error('[useAgent] resume failed:', e instanceof Error ? `${e.name}: ${e.message}` : String(e))
        finalizeLastAssistant()
        status.value = 'idle'
      }
    } finally {
      // 仅当 controller 没被换掉时才清空（避免覆盖新一轮 runResumeChain 的 controller）
      if (abortController === controller) {
        abortController = null
      }
      saveState()
    }
  }

  /**
   * 停止当前流式（SSE 连接 abort）
   */
  function stop(): void {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
    finalizeLastAssistant()
    status.value = 'idle'
    saveState()
  }

  /**
   * 重置 session（清空消息、状态、持久化）
   * ⚠️ 旧版是 destroy；现在 newSession 替代 reset 的语义——
   *    reset 仅用于内部紧急回退（UI 入口已切换为 newSession）
   */
  function reset(): void {
    stop()
    // 清除所有 tool_call 超时 timer（避免泄漏 + reset 后误标记）
    for (const t of toolCallTimeoutMap.values()) clearTimeout(t)
    toolCallTimeoutMap.clear()
    if (currentSessionId.value) {
      try {
        localStorage.removeItem(STORAGE_PREFIX + currentSessionId.value)
      } catch {
        // ignore
      }
    }
    currentSessionId.value = ''
    eventOffset = 0
    messages.value.splice(0, messages.value.length)
    status.value = 'idle'
    lastError.value = ''
    lastUserInput.value = ''
    refreshSessions()
  }

  /**
   * 关闭错误条
   */
  function dismissError(): void {
    lastError.value = ''
    lastErrorCode.value = ''
    if (status.value === 'error') status.value = 'idle'
  }

  /**
   * 重发上一次失败的用户消息
   */
  function retryLast(): void {
    const text = lastUserInput.value
    if (!text) return
    lastError.value = ''
    status.value = 'idle'
    void send(text)
  }

  /**
   * 设置 API 返回的默认模型（由 AgentChat.fetchModels 调用）。
   * 写入后 newSession() 会自动使用此值重置 activeModel。
   */
  function setApiDefaultModel(m: string): void {
    apiDefaultModel.value = m
  }

  // 构造时同步一次 session 列表（供 UI 立即显示）
  refreshSessions()

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
      currentServerInstance = id
    },
    __getSeenSequencesForTest: () => seenSequencesOrder.slice(),
  }
}
