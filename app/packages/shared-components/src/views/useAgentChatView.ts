// useAgentChatView.ts - AgentChat.vue 的 script 逻辑拆出（composable）
// 拆分自 AgentChat.vue。所有 reactive state / handler / lifecycle 集中在此。
// AgentChat.vue 只剩 template + 调 useAgentChatView() 拿到返回值后解构使用。

import { alertController, modalController } from "@ionic/vue";
import {
  addOutline,
  attachOutline,
  chatbubblesOutline,
  checkmarkOutline,
  chevronDownOutline,
  clipboardOutline,
  closeOutline,
  flaskOutline,
  globeOutline,
  keyOutline,
  refreshCircleOutline,
  sendOutline,
  sparklesOutline,
  stopOutline,
  timeOutline,
  trashOutline,
} from "ionicons/icons";
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
// Task 8: 缩放 composable + 共享相对时间格式化
import { formatRelativeTime } from "@encv/shared-components/composables/relativeTime";
import { useRenderTurnItems } from "@encv/shared-components/composables/renderTurnItems";
import { type Decision, getLanAccess, type LanAddress, useAgent } from "@encv/shared-components/composables/useAgent";
import { getAgentApiBase } from "@encv/shared-components/composables/useAgentApiBase";
import { useApiBaseProbe } from "@encv/shared-components/composables/useApiBaseProbe";
import { useAttachments } from "@encv/shared-components/composables/useAttachments";
// 多渲染引擎架构：引入引擎系统和已注册的引擎实现
import { useChatEngine } from "@encv/shared-components/composables/useChatEngine";
import { getDeviceIdSync } from "@encv/shared-components/composables/useDeviceId";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { usePinchZoom } from "@encv/shared-components/composables/usePinchZoom";
import { useServerStatus } from "@encv/shared-components/composables/useServerStatus";
import { useSlashMenu } from "@encv/shared-components/composables/useSlashMenu";
import { showToast } from "@encv/shared-components/composables/useToast";
// 触发引擎注册（模块副作用自动注册到 EngineRegistry）
import "@/engines/defaultEngine";
import "@/engines/tdesignEngine";
import type { V2ScenarioEntry } from "@encv/shared-components/components/agent/V2ScenariosMenu.vue";
// 引擎渲染包装组件（解决 <component :is="vnode"> 不稳定的问题） - 只在 AgentChat.vue 模板内使用
// 其他子组件（AttachmentTray/MockPresetBar/MockBranchChoiceBar/AgentDebugPanel/V2QuickActions/
// V2ScenariosMenu/SlashMenu/ContextIcon/EngineRenderer）由 AgentChat.vue 模板内 import。
export function useAgentChatView() {
  const { t } = useI18n();

  // ── 多渲染引擎架构：引擎切换系统 ─────────────────────
  const { currentEngine, currentEngineId, engineList, switchEngine: doSwitchEngine } = useChatEngine();

  // 引擎切换器下拉状态
  const enginePickerOpen = ref(false);
  const enginePickerRef = ref<HTMLElement | null>(null);

  /** 当前引擎显示名称 */
  const currentEngineDisplayName = computed(() => {
    const id = currentEngineId.value;
    const found = engineList.value.find(e => e.id === id);
    return found?.name || id;
  });

  /** 构建传给当前引擎的 renderProps */
  const engineRenderProps = computed(() => ({
    messages: messages.value,
    status: status.value,
    onSend: async (text: string) => {
      send(text);
    },
    onStop: () => stop(),
    onConfirmTool: async (id: string, decision: string) => confirmTool(id, decision as Decision),
    onCopyMessage: async (messageId: string) => onCopyMessage(messageId),
    onPresetClick: (userText: string) => pickMockPreset({ id: "", label: userText, tooltip: "", userText } as any),
    streaming: status.value === "streaming",
  }));

  /** 引擎切换（带 toast 反馈） */
  function handleSwitchEngine(engineId: string): void {
    const ok = doSwitchEngine(engineId);
    if (ok) {
      const name = engineList.value.find(e => e.id === engineId)?.name || engineId;
      showToast({ message: `已切换到 ${name}`, duration: 1200, color: "success" });
    }
  }

  /** 复制消息内容（引擎回调 —— 实际复制逻辑在 DefaultMessagesView 内部实现） */
  async function onCopyMessage(_messageId: string): Promise<void> {
    // 由 DefaultMessagesView 内部处理，此处仅作为引擎接口的桥接
    // 如果未来其他引擎也需要此回调，可在此统一实现
  }

  // Mock 预设输入栏头部显示：
  // - picker 阶段（首次进 AgentChat）→ "剧本库"（i18n mockPresetBarPickerScenario）
  // - 实际剧本阶段 → 当前 scenario ID
  // - 都不匹配 → "剧本"（默认）
  const mockPresetBarScenario = computed(() => {
    const phase = mockPresetsPhase.value;
    const sc = mockPresetsScenario.value;
    if (sc === "scenario_picker" || phase === "picker") {
      return t("agent.mockPresetBarPickerScenario");
    }
    return sc || mockScenario.value || t("agent.mockPresetBarDefaultScenario");
  });
  // phase 阶段文案：picker 隐藏（已在 scenario 里表达），其他透传
  const mockPresetBarPhase = computed(() => {
    const phase = mockPresetsPhase.value;
    if (phase === "picker" || phase === "off") return "";
    return phase;
  });

  // Agent API 基础路径（与 useAgent.ts 保持一致）
  // Agent API 基础 URL（动态解析：dev 走网关 / prod 直连后端）
  const AGENT_API_BASE = getAgentApiBase();

  const {
    messages,
    status,
    send,
    confirmTool,
    resume,
    stop,
    newSession,
    switchSession,
    deleteSession,
    sessions,
    currentSessionId,
    contextUsage,
    lastErrorCode,
    dismissError,
    activeModel,
    setApiDefaultModel,
    isMockMode,
    isDebugAgent,
    mockScenario,
    currentMockMode,
    loadMockMode,
    setMockMode,
    mockPresets,
    mockPresetsPhase,
    mockPresetsScenario,
    pickMockPreset,
    loadMockPresets,
    rawSSEEvents,
    mockBranchChoices,
    mockBranchPrompt,
    mockRoundState,
    mockScenarioPaused,
    currentMockScenario,
    pickMockBranch,
    sendMockRoundResponse,
  } = useAgent();
  const router = useRouter();

  /**
   * 跳转到 AI 设置页面（让用户重新输入 API Key）。
   *
   * 触发场景：no_api_key banner 出现时（后端 readAgentConfig(deviceId)
   * 返回空，说明当前 deviceId 派生不出 AES key 解开存储密文）。
   *
   * 行为：
   *   1. 先 dismiss banner（避免下次进来还显示）
   *   2. 关闭当前 AgentChat modal（modalController.dismiss）
   *   3. 用 vue-router 跳到 /tabs/settings/agent
   *
   * 为什么不直接 router.push：AgentChat 是 modal，路由跳转不会自动关 modal，
   * 用户回到 home 还会看到飘着的对话窗口。必须先 dismiss。
   */
  async function goToApiKeySettings(): Promise<void> {
    dismissError();
    try {
      await modalController.dismiss();
    } catch {
      /* ignore — 可能 modal 已经被关 */
    }
    router.push("/tabs/settings/agent");
  }

  onMounted(() => {
    // 启动 Context 图标的轮询（5s/30s 周期自适应当前 streaming 状态）
    contextUsage.start();
  });
  onUnmounted(() => {
    // 卸载时清理 timer，避免内存泄漏
    contextUsage.stop();
  });

  // Task 12：附件管理（Composer `+` 按钮）
  const { attachments, addFiles, removeAttachment, clearAttachments } = useAttachments({
    onError: msg => showToast({ message: msg, duration: 2400, color: "warning" }),
  });

  // Task 7：把 i18n 解析后的 "上下文已自动压缩" 文本通过 computed
  // 注入到 renderTurnItems，renderTurnItems 把它塞进 RenderedItem
  // 供 ContextCompactionDivider 直接渲染。这里用 computed 而非
  // t('agent.contextCompaction') 直接调用——renderTurnItems 的
  // 第三个参数要 Ref/ComputedRef，让语言切换时自动重渲染。
  const compactionText = computed(() => t("agent.contextCompaction"));

  const renderedItems = useRenderTurnItems(messages, status, compactionText);

  const inputText = ref("");
  const inputRef = ref<HTMLTextAreaElement | null>(null);
  const mainRef = ref<HTMLDivElement | null>(null);
  const virtualListRef = ref<{ scrollToBottom: (behavior?: "auto" | "smooth") => void } | null>(null);
  const nearBottom = ref(true);
  const activeMessageIndex = ref(0);

  // ─── Task 8: usePinchZoom 集成 ──────────────────────────
  // 关键认知：android webview 默认 user-scalable=yes 时会拦截双指捏合
  // 整体缩放页面 → 破坏 UI 布局。这里显式接管手势：
  //   - 双指捏合 → 计算 distance ratio → 更新 zoomScale → 应用 transform
  //   - 右上角 A-/A/A+ 浮动按钮 → 程序化控制缩放
  //   - 绑定的 targetRef 是 mainRef（.agentChatMain），即会话内容容器
  //   - 缩放范围严格 clamp 到 [0.5, 1.5]，双击重置回 1.0
  const pinch = usePinchZoom({ minScale: 0.5, maxScale: 1.5, step: 0.1 });

  /** 触发虚拟滚动的阈值（renderedItems 数量 > 此值时切换） */
  const VIRTUAL_LIST_THRESHOLD = 120;

  const closeIcon = closeOutline;
  const sparkleIcon = sparklesOutline;
  const addIcon = addOutline;
  const sendIcon = sendOutline;
  const stopIcon = stopOutline;
  const keyIcon = keyOutline;
  const chatbubblesIcon = chatbubblesOutline;
  const timeIcon = timeOutline;
  const attachIcon = attachOutline;
  const globeIcon = globeOutline;
  const clipboardIcon = clipboardOutline;
  const refreshCircleIcon = refreshCircleOutline;
  const chevronDownIcon = chevronDownOutline;
  const flaskIcon = flaskOutline;
  const trashIcon = trashOutline;
  const checkmarkIcon = checkmarkOutline;
  // copyIconVar 已移至 DefaultMessagesView.vue（引擎渲染路径内的复制按钮）
  const historyOpen = ref(false);

  // ── Task 26 (LAN Access) ───────────────────────────────────
  // 折叠面板状态：默认收起。数据由 useAgent.getLanAccess() 拉取。
  // 展开时才拉取（按需），关闭后保留缓存，避免反复网络请求。
  const lanAccessOpen = ref(false);
  const lanAccesses = ref<LanAddress[]>([]);
  const lanAccessLoading = ref(false);
  const lanAccessLoaded = ref(false);

  async function handleRefreshLanAccess(): Promise<void> {
    lanAccessLoading.value = true;
    try {
      lanAccesses.value = await getLanAccess(0);
      lanAccessLoaded.value = true;
    } finally {
      lanAccessLoading.value = false;
    }
  }

  /**
   * LAN 候选「使用此地址」按钮：把 URL 设为当前 baseUrl + 重建连接。
   *
   * 行为：
   *  1. useApiBaseProbe.setManual(url) 写 localStorage + 内存
   *  2. useServerStatus.manualReconnect() 重新探测 + 重建 WS
   *  3. 失败 → toast 红色；成功 → toast 绿色 + 1.6s 后自动隐藏
   *
   * 与 ServerSettings.vue "使用" 按钮的区别：本处是"立即生效"，不进入配置页
   * （适合用户已经看到 LAN 列表想直接切换的场景）
   */
  async function handleUseLanAddress(url: string): Promise<void> {
    try {
      useApiBaseProbe().setManual(url);
      const result = await useServerStatus().manualReconnect();
      if (result.ok) {
        showToast({
          message: t("agent.lanAccessUseSuccess", { url }) || `已切换到 ${url}`,
          duration: 1600,
          color: "success",
        });
      } else {
        showToast({
          message: `${t("agent.lanAccessUseFailed") || "切换失败"}：${result.error || "unknown"}`,
          duration: 2000,
          color: "danger",
        });
      }
    } catch (e) {
      showToast({
        message: t("agent.lanAccessUseFailed") || "切换失败" + ": " + (e instanceof Error ? e.message : String(e)),
        duration: 2000,
        color: "danger",
      });
    }
  }

  async function handleCopyLanAccess(url: string): Promise<void> {
    try {
      if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
        await navigator.clipboard.writeText(url);
        showToast({ message: t("agent.lanAccessCopied", { url }), duration: 1600, color: "success" });
      } else {
        // Fallback：临时 textarea + execCommand（老 webview 兼容）
        const ta = document.createElement("textarea");
        ta.value = url;
        ta.style.position = "fixed";
        ta.style.left = "-9999px";
        document.body.appendChild(ta);
        ta.select();
        const ok = document.execCommand("copy");
        document.body.removeChild(ta);
        if (ok) {
          showToast({ message: t("agent.lanAccessCopied", { url }), duration: 1600, color: "success" });
        } else {
          showToast({ message: t("agent.lanAccessCopyFailed"), duration: 1800, color: "danger" });
        }
      }
    } catch {
      showToast({ message: t("agent.lanAccessCopyFailed"), duration: 1800, color: "danger" });
    }
  }

  // 监听展开事件：用户首次展开时拉取一次。后续点击「刷新」按钮
  // 可强制重拉。watch 比 onMounted 触发更精准——避免用户在折叠
  // 面板被滚动出视野前白白消耗一次网络请求。
  watch(lanAccessOpen, async open => {
    if (open && !lanAccessLoaded.value && !lanAccessLoading.value) {
      await handleRefreshLanAccess();
    }
  });

  // Task 12：隐藏 file input 的引用
  const fileInputRef = ref<HTMLInputElement | null>(null);

  function triggerAttach() {
    // 复用同一个 input：每次点击重置 value，确保选同一文件也能触发 change
    const el = fileInputRef.value;
    if (!el) return;
    el.value = "";
    el.click();
  }

  async function handleAttachChange(e: Event) {
    const target = e.target as HTMLInputElement;
    const files = target.files;
    if (!files || files.length === 0) return;
    const result = await addFiles(files);
    if (result.rejected.length > 0) {
      const names = result.rejected.map(r => r.name).join(", ");
      const sample = result.rejected[0]?.reason || "文件超限";
      showToast({
        message: `已跳过 ${result.rejected.length} 个文件（${names}）：${sample}`,
        duration: 3000,
        color: "warning",
      });
    }
    // 清空 input.value 允许重复选同一文件
    target.value = "";
  }

  const canSend = computed(() => {
    if (status.value === "streaming") return false;
    // 文本非空 OR 至少一个附件都可以发送
    return inputText.value.trim().length > 0 || attachments.value.length > 0;
  });

  // ─── Task 10: "/" 命令面板（useSlashMenu） ─────────────────
  // 取代旧版内联 tool palette：现在支持功能 + 技能两类。
  // 静态功能项（attach / plan-mode / permission-mode）由 composable 内部定义。
  // 技能项从后端 /api/skills 拉取，mount 时拉一次缓存。
  // apply 回调在这里桥接："添加附件" → triggerAttach 打开 file picker；
  // "Plan 模式" / "权限模式" → 留作未来扩展，目前仅 toast 提示。
  // 技能选中 → 在输入框中插入 "@<skill-name> " 让用户继续编辑。
  const slashMenu = useSlashMenu({
    onAttach: () => {
      // 复用 Task 12 的 + 按钮逻辑
      triggerAttach();
    },
    onTogglePlanMode: () => {
      showToast({ message: "Plan 模式：开发中", duration: 1600, color: "medium" });
    },
    onTogglePermissionMode: () => {
      showToast({ message: "权限模式：开发中", duration: 1600, color: "medium" });
    },
    onSelectSkill: (id, label) => {
      // 选中技能 → 在输入框中插入 "@<label> "，等用户继续编辑
      void id; // 技能 id 当前仅用于日志/未来埋点；label 用于填充输入
      inputText.value = `@${label} `;
      autoResize();
      nextTick(() => inputRef.value?.focus());
    },
  });

  /**
   * textarea @input 入口：先走原生 autoResize 维持高度，
   * 再把当前文本传给 slashMenu.handleInput 决定开关。
   */
  function onTextareaInput() {
    autoResize();
    slashMenu.handleInput(inputText.value);
  }

  /**
   * textarea @keydown 入口：先让 slashMenu 拦截 ↑
   * ↓ / Enter / Escape（菜单打开时）；未拦截时放行原生行为。
   */
  function onTextareaKeydown(e: KeyboardEvent) {
    // slashMenu.handleKeydown 内部决定是否拦截
    if (slashMenu.handleKeydown(e)) return;
    // 菜单未打开时：菜单不处理，留给浏览器默认（如 Tab、Backspace 等）
  }

  // ─── 模型选择（动态从 API 获取） ────────────────────────────
  interface ModelOption {
    id: string;
    name: string;
    provider: string;
  }

  const availableModels = ref<ModelOption[]>([]);
  const modelsLoading = ref(true);
  const modelsError = ref("");

  async function fetchModels() {
    modelsLoading.value = true;
    modelsError.value = "";
    const did = getDeviceIdSync();
    const url = `${AGENT_API_BASE}/api/models?deviceId=${encodeURIComponent(did)}`;
    try {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`HTTP ${res.status} ${res.statusText}`);
      const data = await res.json();
      // 处理各种错误状态
      if (data.error === "no_api_key") {
        modelsError.value = t("agent.noApiKeyHint") || "未配置 API Key";
        return;
      }
      if (data.error || !Array.isArray(data.models)) {
        modelsError.value = data.note || t("agent.modelsError");
        return;
      }
      availableModels.value = (data.models || []).map((m: any) => ({
        id: m.id,
        name: m.name || m.id,
        provider: m.provider || "unknown",
      }));
      // 保存 API 返回的默认模型（新会话时使用）
      if (data.defaultModel) {
        setApiDefaultModel(data.defaultModel);
      }
      // 如果当前选中的模型不在列表中，切换到默认值
      if (availableModels.value.length > 0 && !availableModels.value.some(m => m.id === selectedModel.value)) {
        selectedModel.value = data.defaultModel || availableModels.value[0].id;
      }
    } catch (e: any) {
      const errInfo = (() => {
        if (!e) return "(null)";
        if (e instanceof Error) return `${e.name}: ${e.message}`;
        try {
          return JSON.stringify(e);
        } catch {
          return String(e);
        }
      })();
      console.error(`[AgentChat] fetchModels failed: url=${url} error=${errInfo}`);
      // 网络错误等：不阻断用户使用，显示提示但保留已存储的模型选择
      modelsError.value = `${t("agent.modelsError")} (${errInfo})`;
    } finally {
      modelsLoading.value = false;
    }
  }

  const SELECTED_MODEL_KEY = "encv-agent-selected-model";
  const TEMPERATURE_KEY = "encv-agent-temperature";
  const storedModel = (() => {
    try {
      return localStorage.getItem(SELECTED_MODEL_KEY) || "gpt-4o-mini";
    } catch {
      return "gpt-4o-mini";
    }
  })();
  const storedTemp = (() => {
    try {
      const v = localStorage.getItem(TEMPERATURE_KEY);
      const n = v == null ? 0.7 : Number(v);
      return Number.isFinite(n) ? n : 0.7;
    } catch {
      return 0.7;
    }
  })();
  const selectedModel = ref<string>(storedModel);
  const temperature = ref<number>(storedTemp);
  watch(selectedModel, v => {
    try {
      localStorage.setItem(SELECTED_MODEL_KEY, v);
    } catch {
      /* ignore */
    }
    // 同步到 useAgent 的 activeModel（send/sendQueued 读取此值）
    activeModel.value = v;
  });
  watch(temperature, v => {
    try {
      localStorage.setItem(TEMPERATURE_KEY, String(v));
    } catch {
      /* ignore */
    }
  });

  // ─── 模型选择器（输入框内嵌） ─────────────────────────────
  const modelPickerOpen = ref(false);
  const modelPickerRef = ref<HTMLElement | null>(null);

  /** 当前模型的显示名称（从 availableModels 查找，找不到则用 id 本身） */
  const currentModelDisplayName = computed(() => {
    const id = selectedModel.value;
    const found = availableModels.value.find(m => m.id === id);
    return found?.name || id;
  });

  function selectModel(id: string) {
    selectedModel.value = id;
  }

  /**
   * 当前 selectedModel 是否在 availableModels 列表里。
   * 当列表加载完成后用户选了一个已不存在于列表的 model（罕见，但可能）时，
   * 模板里仍然要显示这个 selectedModel（用户过往选择），但要标记它是 "fallback"。
   */
  function isSelectedModelAvailable(): boolean {
    if (!selectedModel.value) return true;
    return availableModels.value.some(m => m.id === selectedModel.value);
  }

  // ─── Mock 模式切换器（在会话界面直接配置，弹 action-sheet） ────────
  /**
   * 徽章文本：根据当前模式显示对应文案
   *  - off     → "真实 API"   （灰色，提示"未启用 mock"）
   *  - builtin → "模拟·内置"
   *  - custom  → "模拟·自定义"
   */
  const mockBadgeText = computed(() => {
    if (currentMockMode.value === "builtin") return `${t("agent.mockBadge")}·${t("agent.mockModeBuiltin")}`;
    return t("agent.mockModeOff");
  });

  /**
   * 徽章 tooltip：
   *  - active 时显示当前 scenario id（来自最近一次 SSE 响应）
   *  - off 时显示"点击切换模式"
   */
  const mockBadgeTitle = computed(() => {
    if (currentMockMode.value === "off") return t("agent.mockMode");
    if (isMockMode.value && mockScenario.value) {
      return t("agent.mockBadgeTooltip", { scenario: mockScenario.value });
    }
    return t("agent.mockMode");
  });

  /**
   * 点击徽章 → 直接切换 off ↔ builtin（无 action-sheet）
   * 切换经由 useAgent.setMockMode() 走 PUT /api/config 持久化
   */
  async function toggleMockMode(): Promise<void> {
    const next = currentMockMode.value === "off" ? "builtin" : "off";
    try {
      await setMockMode(next);
      showToast({
        message: next === "off" ? t("agent.mockModeOff") || "真实 API" : t("agent.mockModeBuiltin") || "模拟·内置",
        duration: 1200,
        color: next === "off" ? "medium" : "success",
      });
    } catch (e) {
      showToast({
        message: `${t("agent.mockModeSetFailed") || "切换失败"}: ${e instanceof Error ? e.message : String(e)}`,
        duration: 2400,
        color: "danger",
      });
    }
  }

  /** 点击外部关闭下拉 */
  function handleModelPickerOutsideClick(e: MouseEvent) {
    if (modelPickerOpen.value && modelPickerRef.value && !modelPickerRef.value.contains(e.target as Node)) {
      modelPickerOpen.value = false;
    }
    if (enginePickerOpen.value && enginePickerRef.value && !enginePickerRef.value.contains(e.target as Node)) {
      enginePickerOpen.value = false;
    }
  }

  // ── 工具调用/结果查找已移至 DefaultMessagesView.vue（引擎渲染路径）──
  // AgentChat 作为宿主容器不再直接操作消息渲染细节

  /**
   * 格式化会话历史列表项的元信息（时间 + 消息数 + 轮次）
   *
   * Task 8：相对时间改用 composables/relativeTime.ts 共享实现
   * （与 sessionList 完全一致的逻辑，自动 30s 刷新由 useRelativeTime 控制，
   *  本处直接接受硬编码中文格式）
   */
  function formatSessionMeta(s: { messageCount: number; rounds: number; updatedAt: number }): string {
    const time = formatRelativeTime(s.updatedAt);
    const parts = [time];
    if (s.rounds > 0) {
      parts.push(`${s.rounds} ${t("agent.rounds") || "轮"}`);
    }
    parts.push(`${s.messageCount} ${t("agent.messages")}`);
    return parts.join(" · ");
  }

  // ─── 输入框处理 ──────────────────────────────────────────
  function autoResize() {
    const el = inputRef.value;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 120) + "px";
  }

  function handleSend() {
    if (!canSend.value) return;
    const text = inputText.value.trim();
    const atts = attachments.value.slice(); // 拍快照：避免 send 异步期间被清空后引用空数组
    inputText.value = "";
    autoResize();
    // v2 多轮/分支剧本暂停时：把文本走 sendMockRoundResponse → 后端
    // MockEngineV2 走"恢复"分支（带 scenario ID），而非开新 session。
    // 原因：send() 默认 mode='start' 会让后端重新匹配 scenario，
    //       而 v2 的"恢复"必须显式 mode='mock_resume' + scenario。
    if (mockScenarioPaused.value) {
      sendMockRoundResponse(text);
      nextTick(() => scrollToBottom());
      return;
    }
    send(text, { attachments: atts });
    // 发送后清空 tray（避免下次发送重复附带）
    clearAttachments();
    nextTick(() => scrollToBottom());
  }

  function handleStop() {
    stop();
  }

  /**
   * v2 工具快捷动作 chip 点击：把示例 prompt 注入输入框 + 聚焦
   *
   * 不直接 send —— 让用户能修改/补充上下文，避免"我都不知道它发了什么"的失控感。
   * 若用户已经在 mock 模式里被 v2 剧本暂停，则走 sendMockRoundResponse 路径。
   */
  function onPickV2QuickAction(action: { prompt: string }): void {
    inputText.value = action.prompt;
    nextTick(() => {
      inputRef.value?.focus();
      autoResize();
    });
  }

  /**
   * v2 剧本演示入口点击：自动切到 builtin mock + 发送 trigger keyword
   *
   * 流程：
   * 1. 若 mock 模式为 off，切到 builtin（持久化），toast 提示
   * 2. 调用 send(triggerKeyword) —— 后端按 keyword 匹配到对应 v2 剧本并启动
   * 3. 多轮剧本的 mid-step 暂停由 mock_branch_choice / mock_round_state 事件驱动
   */
  async function onPickV2Scenario(scenario: V2ScenarioEntry): Promise<void> {
    if (status.value === "streaming" || status.value === "confirming") {
      showToast({
        message: t("agent.v2Scenarios.busy") || "当前正在请求中，请稍候",
        duration: 1500,
        color: "warning",
      });
      return;
    }
    if (currentMockMode.value === "off") {
      try {
        await setMockMode("builtin");
      } catch (e) {
        showToast({
          message: (t("agent.mockModeSetFailed") || "切换 mock 模式失败") + ": " + (e instanceof Error ? e.message : String(e)),
          duration: 2000,
          color: "danger",
        });
        return;
      }
    }
    // 短暂延迟让 mock 模式切换 toast 显示 + UI 更新
    await new Promise(resolve => setTimeout(resolve, 80));
    void send(scenario.triggerKeyword);
    nextTick(() => scrollToBottom());
  }

  /**
   * v2 修复：从全屏历史界面直接新建会话
   * - 不需要关闭历史界面再操作 → 流畅体验
   * - 自动关闭全屏历史 → 回到主聊天（新会话已就绪）
   */
  async function handleNewSessionFromHistory(): Promise<void> {
    // 来自全屏历史时直接创建（不弹确认，因为历史界面本身就是"切走"的语义）
    newSession();
    historyOpen.value = false;
  }

  async function handleOpenHistory() {
    await Promise.resolve();
    historyOpen.value = true;
  }

  async function handleDeleteSession(sessionId: string, event: Event) {
    event.stopPropagation();
    const alert = await alertController.create({
      header: t("agent.deleteSession"),
      message: t("agent.confirmDeleteSession"),
      buttons: [
        { text: t("common.cancel"), role: "cancel" },
        { text: t("common.confirm"), role: "destructive" },
      ],
    });
    await alert.present();
    const { role } = await alert.onDidDismiss();
    if (role === "destructive") {
      deleteSession(sessionId);
    }
  }

  /**
   * v3 修复：关闭整个 AgentChat modal
   * - 上一次 modal 没有 dismiss 入口，用户只能点系统返回键，体验割裂
   * - 拆弹点：从外部标签页 / App 内入口 / 系统返回键进来时都能从这里退
   * - 与"返回上一级"语义对齐：modal pop → 回到原页面
   * - 容错：重复 dismiss 静默吞错（避免 alert 弹窗干扰）
   */
  async function handleCloseModal(): Promise<void> {
    try {
      await modalController.dismiss();
    } catch {
      // ignore — modal 可能已经被外部代码 dismiss
    }
  }

  function handleClose() {
    // v2 修复：全屏历史界面上的"关闭"按钮只关闭历史面板，
    // 不再 dismiss 整个 modal —— 用户希望回到主聊天继续对话
    historyOpen.value = false;
  }

  function scrollToBottom(behavior: "auto" | "smooth" = "smooth") {
    nextTick(() => {
      // 长会话走虚拟列表的 scrollToItem
      if (renderedItems.value.length > VIRTUAL_LIST_THRESHOLD && virtualListRef.value) {
        virtualListRef.value.scrollToBottom(behavior);
        return;
      }
      // 短会话走原生 container 滚动
      const el = mainRef.value;
      if (!el) return;
      el.scrollTo({ top: el.scrollHeight, behavior });
    });
  }

  /**
   * 监听 main 容器滚动，更新 nearBottom
   * 虚拟列表模式下滚动源是 RecycleScroller 内部 wrapper，
   * 但其 scroll 事件会冒泡到 main 容器，逻辑统一处理
   */
  function onMainScroll() {
    const el = mainRef.value;
    if (!el) return;
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    // 80px 阈值内视为"接近底部"——避免长消息末尾抖动
    nearBottom.value = distanceFromBottom < 80;
  }

  /** IntersectionObserver：追踪当前视口中最接近中心的消息项 */
  let dotObserver: IntersectionObserver | null = null;

  function setupDotObserver() {
    cleanupDotObserver();
    const el = mainRef.value;
    if (!el) return;
    dotObserver = new IntersectionObserver(
      entries => {
        // 找到相交比例最大的元素（最接近视口中心）
        let maxRatio = 0;
        let targetIdx = activeMessageIndex.value;
        for (const entry of entries) {
          if (entry.intersectionRatio > maxRatio) {
            maxRatio = entry.intersectionRatio;
            const idx = Number((entry.target as HTMLElement).dataset.msgIdx ?? -1);
            if (idx >= 0) targetIdx = idx;
          }
        }
        if (maxRatio > 0) activeMessageIndex.value = targetIdx;
      },
      { root: el, threshold: [0, 0.25, 0.5, 0.75, 1] }
    );
    // 观察所有消息项
    nextTick(() => {
      el.querySelectorAll(".renderedItemWrap").forEach(wrap => {
        dotObserver?.observe(wrap);
      });
    });
  }

  function cleanupDotObserver() {
    dotObserver?.disconnect();
    dotObserver = null;
  }

  // 消息列表变化时重建 Observer
  watch(renderedItems, () => nextTick(setupDotObserver), { flush: "post" });
  onMounted(() => nextTick(setupDotObserver));
  onUnmounted(() => {
    cleanupDotObserver();
    // v3 新增：清理圆点导航的长按 timer / rAF
    clearDotPressTimer();
    clearDotDragRaf();
  });

  /**
   * v3 修复：左侧圆点导航核心逻辑
   *
   * 三件事：
   * 1) userMessageItems：过滤出所有 type === 'user' 的渲染项
   *    - 每个 user 消息 = 1 个圆点（不是每个 tool_call 块）
   * 2) activeUserMessageIdx：当前最接近视口中心的 user 消息索引
   *    - 用于给"非拖动状态"下的当前圆点上色
   * 3) 长按拖动：圆点 → 长条 → 拖动 → 松开恢复
   *    - 250ms 长按后激活"拖动模式"
   *    - 拖动用 rAF 节流，避免高频 scrollIntoView 抖动
   *    - pointer capture 防止指针滑出元素丢失事件
   *    - 组件卸载时清理 timer / rAF（防内存泄漏）
   */

  /** user 消息条目（包含在 renderedItems 中的原始下标） */
  interface UserMessageItem {
    item: { type: "user"; messageId: string; text: string };
    idx: number; // 在 renderedItems 中的下标
  }

  /** 过滤出所有 user 类型的渲染项 */
  const userMessageItems = computed<UserMessageItem[]>(() => {
    const out: UserMessageItem[] = [];
    const items = renderedItems.value;
    for (let i = 0; i < items.length; i++) {
      const it = items[i];
      if (it.type === "user") {
        out.push({ item: it, idx: i });
      }
    }
    return out;
  });

  /**
   * 当前激活的 user 消息索引（自然滚动时）
   * 算法：在 userMessageItems 中找 idx <= activeMessageIndex 的最后一个
   */
  const activeUserMessageIdx = computed(() => {
    const ai = activeMessageIndex.value;
    const list = userMessageItems.value;
    if (list.length === 0) return -1;
    let result = 0;
    for (let i = 0; i < list.length; i++) {
      if (list[i].idx <= ai) {
        result = i;
      } else {
        break;
      }
    }
    return result;
  });

  /** 跳转到指定 user 消息（点击圆点时调用） */
  function onDotClick(dotIdx: number) {
    const ui = userMessageItems.value[dotIdx];
    if (!ui) return;
    scrollToUserMessage(ui.idx);
  }

  /** 跳转到 user 消息在 renderedItems 中的下标 */
  function scrollToUserMessage(renderedIdx: number) {
    const el = mainRef.value;
    if (!el) return;
    const target = el.querySelector(`[data-msg-idx="${renderedIdx}"]`) as HTMLElement | null;
    if (!target) return;
    target.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  // ── 长按 → 长条 → 拖动 → 松开（瞬态） ──────────────────────
  const dotNavRef = ref<HTMLElement | null>(null);
  const isDotDragging = ref(false);
  const draggedDotIdx = ref<number | null>(null);

  /** 长按判定阈值（ms） */
  const DOT_LONG_PRESS_MS = 280;
  /** 拖动前最大允许位移（px）—— 超过则判定为"快速滑动/误触"而取消长按 */
  const DOT_DRAG_THRESHOLD_PX = 5;
  /** 单个圆点占用的总高度（8 圆点 + 5 gap） */
  const DOT_ITEM_HEIGHT = 13;

  let dotPressTimer: ReturnType<typeof setTimeout> | null = null;
  let dotDragRafId: number | null = null;
  let dotPressStartY = 0;
  let dotPressStartX = 0;
  let dotPressActivePointerId: number | null = null;
  let pendingDragY = 0;

  function clearDotPressTimer() {
    if (dotPressTimer !== null) {
      clearTimeout(dotPressTimer);
      dotPressTimer = null;
    }
  }

  function clearDotDragRaf() {
    if (dotDragRafId !== null) {
      cancelAnimationFrame(dotDragRafId);
      dotDragRafId = null;
    }
  }

  /**
   * 在拖动模式下，根据指针 clientY 找出对应的圆点索引
   * - 圆点列中心 = dotNavRef.getBoundingClientRect() 的 top + height/2
   * - 每个圆点占 13px，按偏移/13 估算索引
   */
  function dotIdxFromClientY(clientY: number): number {
    const navEl = dotNavRef.value;
    if (!navEl) return 0;
    const rect = navEl.getBoundingClientRect();
    // 圆点 nav 的 padding-top 是 8px；按"第一个圆点中心位于 padding 后 4px"估算
    const firstDotCenter = rect.top + 8 + 4;
    const offset = clientY - firstDotCenter;
    const idx = Math.round(offset / DOT_ITEM_HEIGHT);
    return Math.max(0, Math.min(userMessageItems.value.length - 1, idx));
  }

  /**
   * 处理 pointermove：长按未触发前如果移动过多则取消；触发后用 rAF 节流更新 draggedDotIdx
   */
  function handleDotNavPointerMove(e: PointerEvent) {
    if (dotPressTimer === null && !isDotDragging.value) return;
    if (dotPressActivePointerId !== null && e.pointerId !== dotPressActivePointerId) return;

    // 长按未触发：检查是否超过位移阈值
    if (!isDotDragging.value) {
      const dy = Math.abs(e.clientY - dotPressStartY);
      const dx = Math.abs(e.clientX - dotPressStartX);
      if (dy > DOT_DRAG_THRESHOLD_PX || dx > DOT_DRAG_THRESHOLD_PX) {
        // 取消长按 → 视为轻触滚动（不做事）
        clearDotPressTimer();
        return;
      }
      return;
    }

    // 拖动模式：rAF 节流更新
    pendingDragY = e.clientY;
    if (dotDragRafId === null) {
      dotDragRafId = requestAnimationFrame(() => {
        dotDragRafId = null;
        const idx = dotIdxFromClientY(pendingDragY);
        if (idx !== draggedDotIdx.value) {
          draggedDotIdx.value = idx;
          // 滚动主内容到对应的 user 消息
          const ui = userMessageItems.value[idx];
          if (ui) scrollToUserMessage(ui.idx);
        }
      });
    }
  }

  function onDotNavPointerDown(e: PointerEvent) {
    // 只响应主指针（左键 / 单点触摸）
    if (e.pointerType === "mouse" && e.button !== 0) return;
    // 防止在已经激活的拖动上叠加新的指针
    if (dotPressActivePointerId !== null) return;

    const navEl = e.currentTarget as HTMLElement;
    dotPressActivePointerId = e.pointerId;
    dotPressStartY = e.clientY;
    dotPressStartX = e.clientX;
    pendingDragY = e.clientY;
    draggedDotIdx.value = null;
    isDotDragging.value = false;

    // 捕获指针：后续 move/up 仍由本元素接收（即使指针滑出元素）
    try {
      navEl.setPointerCapture(e.pointerId);
    } catch {
      // 部分平台不支持，静默继续
    }

    // 启动长按计时
    clearDotPressTimer();
    dotPressTimer = setTimeout(() => {
      dotPressTimer = null;
      if (dotPressActivePointerId === null) return;
      isDotDragging.value = true;
      // 进入拖动模式：初始 idx 按当前指针位置计算
      const idx = dotIdxFromClientY(dotPressStartY);
      draggedDotIdx.value = idx;
      const ui = userMessageItems.value[idx];
      if (ui) scrollToUserMessage(ui.idx);
    }, DOT_LONG_PRESS_MS);
  }

  function onDotNavPointerMove(e: PointerEvent) {
    handleDotNavPointerMove(e);
  }

  function onDotNavPointerUp(e: PointerEvent) {
    handleDotNavPointerUp(e);
  }

  function handleDotNavPointerUp(e: PointerEvent) {
    // 清理状态
    clearDotPressTimer();
    clearDotDragRaf();

    // 释放指针捕获
    const navEl = e.currentTarget as HTMLElement | null;
    if (navEl && dotPressActivePointerId !== null) {
      try {
        navEl.releasePointerCapture(dotPressActivePointerId);
      } catch {
        // ignore
      }
    }

    const wasDragging = isDotDragging.value;
    isDotDragging.value = false;
    draggedDotIdx.value = null;
    dotPressActivePointerId = null;

    // 拖动模式松开 → 已经在拖动中跳转过了，无需再做
    if (wasDragging) {
      e.preventDefault();
    }
    // 否则走 @click（短按 = 跳转）→ 已绑定 onDotClick
  }

  // 监听 status 变化 → streaming 开始时滚动到底部
  watch(
    () => status.value,
    newStatus => {
      if (newStatus === "streaming") {
        scrollToBottom();
      }
    }
  );

  // 监听 messages 变化（长度/最后一条）→ 接近底部时自动滚
  watch(
    () => messages.value.length,
    () => {
      if (nearBottom.value) scrollToBottom();
    }
  );

  watch(
    () => messages.value[messages.value.length - 1]?.content,
    () => {
      if (nearBottom.value) scrollToBottom("auto");
    }
  );

  onMounted(async () => {
    // 动态获取可用模型列表（不阻塞 UI）
    fetchModels();
    // 启动时尝试恢复最近 session
    await resume();
    // 加载当前 mock 模式（用户主动控制 → action-sheet 切换）
    await loadMockMode();
    // mock 模式开启时 → 拉"全局剧本选择器"覆盖在输入框上方
    // 用户首次进入就能看到 chip，不必先发消息触发流
    if (isMockMode.value) {
      void loadMockPresets();
    }
    nextTick(() => scrollToBottom("auto"));
    // 模型选择器：点击外部关闭下拉
    document.addEventListener("click", handleModelPickerOutsideClick);
    // Task 8: 绑定双指缩放到会话内容容器（mainRef = .agentChatMain）
    // 必须在 nextTick 之后绑定 —— mainRef 在 onMounted 时可能还没渲染
    nextTick(() => {
      if (mainRef.value) {
        pinch.bind(mainRef.value);
      }
    });
  });

  // 用户在 Settings/其他位置切换 mock 模式后 → 重新拉/清空 chip
  // off → 清空（v-if 自然不渲染，无需手动）
  // builtin/custom → 拉新选择器覆盖当前 chip
  watch(currentMockMode, (newMode, _oldMode) => {
    console.debug("[AgentChat] mock mode changed →", newMode);
    if (newMode === "builtin" || newMode === "custom") {
      void loadMockPresets();
    }
    // 'off' 不需要清空 —— isMockMode.value = false → v-if 不渲染
  });

  onUnmounted(() => {
    document.removeEventListener("click", handleModelPickerOutsideClick);
    // Task 8: 解绑双指缩放事件监听器（避免内存泄漏）
    pinch.unbind();
  });

  // 暴露给 modal container（可选）

  // Return to caller — caller will destructure as needed
  // Note: vue-tsc 推断 useAgentChatView() return type 时遇到某些 shorthand property
  // 容易丢失。调试经验：把"易丢"字段用 Object.assign 显式注入 + spread 模式
  // 能确保它们都在 return type 中（避免 destructure 出现 not exist on type 错误）。
  const _explicitKeys = {
    isSelectedModelAvailable: isSelectedModelAvailable as unknown as () => boolean,
    switchSession: switchSession as unknown as (id: string) => void,
    lanAccessLoaded: lanAccessLoaded as unknown as boolean,
    fetchModels: fetchModels as unknown as () => Promise<void>,
    temperature: temperature as unknown as number,
  };
  return Object.assign(_explicitKeys, {
    // i18n
    t,
    // 引擎系统
    currentEngine,
    currentEngineId,
    engineList,
    enginePickerOpen,
    enginePickerRef,
    currentEngineDisplayName,
    engineRenderProps,
    handleSwitchEngine,
    onCopyMessage,
    // mock preset / scenario
    mockPresetBarScenario,
    mockPresetBarPhase,
    // API base
    goToApiKeySettings,
    // useAgent re-exposed values (30+ fields)
    messages,
    status,
    send,
    confirmTool,
    resume,
    stop,
    sessions,
    currentSessionId,
    contextUsage,
    lastErrorCode,
    dismissError,
    isMockMode,
    isDebugAgent,
    currentMockMode,
    mockPresets,
    pickMockPreset,
    rawSSEEvents,
    mockBranchChoices,
    mockBranchPrompt,
    mockRoundState,
    mockScenarioPaused,
    currentMockScenario,
    pickMockBranch,
    // attachments
    attachments,
    removeAttachment,
    // renderTurnItems + compaction
    renderedItems,
    // input refs
    inputText,
    inputRef,
    mainRef,
    // pinch
    pinch,
    // icons
    closeIcon,
    sparkleIcon,
    addIcon,
    sendIcon,
    stopIcon,
    keyIcon,
    chatbubblesIcon,
    timeIcon,
    attachIcon,
    globeIcon,
    clipboardIcon,
    refreshCircleIcon,
    chevronDownIcon,
    flaskIcon,
    trashIcon,
    checkmarkIcon,
    // history / LAN
    historyOpen,
    lanAccessOpen,
    lanAccesses,
    lanAccessLoading,
    handleRefreshLanAccess,
    handleUseLanAddress,
    handleCopyLanAccess,
    // file input + attach
    fileInputRef,
    triggerAttach,
    handleAttachChange,
    // send
    canSend,
    // slash menu
    slashMenu,
    onTextareaInput,
    onTextareaKeydown,
    // models
    availableModels,
    modelsLoading,
    modelsError,
    selectedModel,
    modelPickerOpen,
    modelPickerRef,
    currentModelDisplayName,
    selectModel,
    handleModelPickerOutsideClick,
    // mock mode toggle
    mockBadgeText,
    mockBadgeTitle,
    toggleMockMode,
    // session meta / send / stop / v2
    formatSessionMeta,
    autoResize,
    handleSend,
    handleStop,
    onPickV2QuickAction,
    onPickV2Scenario,
    // history
    handleNewSessionFromHistory,
    handleOpenHistory,
    handleDeleteSession,
    // close modal
    handleCloseModal,
    handleClose,
    // dot nav
    userMessageItems,
    activeUserMessageIdx,
    onDotClick,
    scrollToUserMessage,
    dotNavRef,
    isDotDragging,
    draggedDotIdx,
    DOT_LONG_PRESS_MS,
    DOT_DRAG_THRESHOLD_PX,
    DOT_ITEM_HEIGHT,
    onDotNavPointerDown,
    onDotNavPointerMove,
    onDotNavPointerUp,
    // scroll observer
    scrollToBottom,
    onMainScroll,
    setupDotObserver,
    cleanupDotObserver,
    VIRTUAL_LIST_THRESHOLD,
  });
}
