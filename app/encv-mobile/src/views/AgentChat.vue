<!--
  AgentChat - 顶层 AI 助手对话视图
  作为 modalController.create() 的 component 渲染

  流程：
  1. 顶部 header：标题 + 关闭按钮 + Reset
  2. 主体：renderTurnItems(messages, status) → 分发到不同组件
     - renderedItems.length <= 120 → 原生 v-for（性能足够）
     - renderedItems.length >  120 → MessageVirtualList（虚拟滚动）
  3. 底部：输入框（idle 时可用，streaming 时显示停止按钮）

  接收 props: { apiBase: string }  （spec 7.5 要求；当前 useAgent 内部固定 /agent-api，参数保留供以后扩展）
-->
<template>
  <div class="agentChat">
    <header class="agentChatHeader">
      <!--
        v3 修复：关闭按钮放在最左侧（time 历史按钮之前）
        语义：返回上一级（关闭整个 AgentChat modal）
        上一次的残留 + 按钮已迁移到全屏历史界面（v2 改动），此处不再需要。
      -->
      <button type="button" class="headerBtn" @click="handleCloseModal" :title="t('common.close') || '关闭'">
        <ion-icon :icon="closeIcon" />
      </button>
      <button type="button" class="headerBtn" @click="handleOpenHistory" :title="t('agent.history')">
        <ion-icon :icon="timeIcon" />
      </button>
      <div class="headerTitle">
        <ion-icon :icon="sparkleIcon" class="headerTitleIcon" />
        <span>{{ t('agent.title') }}</span>
        <!--
          Mock 模式切换器（用户在会话界面直接配置，无需去 Settings → Agent）。
          三种状态：off / builtin / custom。
          行为：
            - 始终显示（让用户随时知道当前是真实 API 还是 mock）
            - 点击 → 弹 action-sheet 切换模式
            - mode=off 灰色，builtin/custom 强调色 + 文字"模拟"
          currentMockMode 由 useAgent 从后端 /api/config 加载，切换时
          通过 PUT /api/config 持久化（无需重启后端）。
        -->
        <button
          type="button"
          class="mockBadge"
          :class="{
            mockBadge_active: currentMockMode !== 'off',
            mockBadge_clickable: true,
          }"
          :title="mockBadgeTitle"
          @click="toggleMockMode"
        >
          <ion-icon :icon="flaskIcon" class="mockBadgeIcon" />
          <span class="mockBadgeText">{{ mockBadgeText }}</span>
          <ion-icon :icon="chevronDownIcon" class="mockBadgeChevron" />
        </button>
        <!--
          v2 剧本演示入口：常驻在 header 上（mock 徽章旁）。
          点击 → 弹 modal 列出 8 个 v2 剧本（搜索/读/写/分支 4 组）。
          点选某剧本 → 自动切到 builtin mock 模式 + 发送 trigger keyword。
        -->
        <V2ScenariosMenu
          :disabled="status === 'streaming' || status === 'confirming'"
          @pick="onPickV2Scenario"
        />

        <!-- 多渲染引擎切换器（同款模型选择器样式） -->
        <div class="enginePicker" ref="enginePickerRef">
          <button
            type="button"
            class="enginePickerBtn"
            @click="enginePickerOpen = !enginePickerOpen"
            :title="'切换聊天引擎'"
          >
            <span class="enginePickerLabel">{{ currentEngineDisplayName }}</span>
            <ion-icon :icon="chevronDownIcon" class="enginePickerArrow" :class="{ 'enginePickerArrow_open': enginePickerOpen }" />
          </button>
          <Transition name="modelPickerFade">
            <div v-if="enginePickerOpen" class="enginePickerDropdown">
              <button
                v-for="e in engineList"
                :key="e.id"
                type="button"
                class="enginePickerOption"
                :class="{ 'enginePickerOption_active': currentEngineId === e.id }"
                @click="handleSwitchEngine(e.id); enginePickerOpen = false"
              >
                <span class="enginePickerOptionName">{{ e.name }}</span>
              </button>
            </div>
          </Transition>
        </div>
      </div>
      <!-- 上下文使用图标（点击 → 弹窗：todos + 引用文件） -->
      <ContextIcon
        :data="contextUsage.data.value"
        :loading="contextUsage.loading.value"
        class="headerContext"
      />
      <!--
        v3 修复：右侧 + 加号按钮已彻底移除。
        新会话入口已迁移到全屏历史界面（v2 改动的 .historyNewSessionFab）。
        此处不再重复入口，避免用户认知混乱（"为什么两个加号？"）。
      -->
    </header>

    <!--
      no_api_key 自愈 banner：仅在 chat 发送返回 503 {error: "no_api_key"} 时出现。
      原因：用户可能在另一台设备配过 key，本设备的 deviceId 解不开。
      给一个直达 AI 设置的入口，避免"我不知道去哪里修"的卡死循环。
    -->
    <div v-if="lastErrorCode === 'no_api_key'" class="noApiKeyBanner">
      <ion-icon :icon="keyIcon" class="noApiKeyBannerIcon" />
      <div class="noApiKeyBannerText">
        <strong>{{ t('agent.noApiKeyTitle') || '未配置 API Key' }}</strong>
        <span>{{ t('agent.noApiKeyHint2') || '当前设备无法解密已存储的 key，请去 AI 设置重新输入。' }}</span>
      </div>
      <button type="button" class="noApiKeyBannerBtn" @click="goToApiKeySettings">
        {{ t('agent.goToApiKeySettings') || '去设置' }}
      </button>
      <button type="button" class="noApiKeyBannerClose" @click="dismissError" :title="t('common.close') || '关闭'">
        <ion-icon :icon="closeIcon" />
      </button>
    </div>

    <!-- 模型选择已移至输入框内（footerInputRow 左侧） -->

    <!--
      Task 26 (LAN Access)：局域网访问地址折叠面板。
      默认折叠（v-show），点击展开。挂载在 toolbar 下方、main 上方——
      该位置在视觉上属于"次要状态信息"，不会分散对话注意力。
      数据源：useAgent.getLanAccess() → GET /api/network/lan-access。
    -->
    <details class="lanAccessPanel" :open="lanAccessOpen" @toggle="lanAccessOpen = ($event.target as HTMLDetailsElement).open">
      <summary class="lanAccessSummary">
        <ion-icon :icon="globeIcon" class="lanAccessSummaryIcon" />
        <span class="lanAccessSummaryText">{{ t('agent.lanAccess') }}</span>
        <span v-if="lanAccesses.length > 0" class="lanAccessSummaryCount">{{ lanAccesses.length }}</span>
      </summary>
      <div class="lanAccessBody">
        <p class="lanAccessHelp">{{ t('agent.lanAccessHelp') }}</p>
        <div v-if="lanAccessLoading" class="lanAccessLoading">{{ t('settings.loading') }}</div>
        <div v-else-if="lanAccesses.length === 0" class="lanAccessEmpty">
          {{ t('agent.lanAccessEmpty') }}
        </div>
        <ul v-else class="lanAccessList">
          <li v-for="addr in lanAccesses" :key="addr.ip" class="lanAccessItem">
            <div class="lanAccessItemMain">
              <code class="lanAccessUrl">{{ addr.url }}</code>
              <span class="lanAccessInterface">{{ t('agent.lanAccessInterface', { name: addr.interface }) }}</span>
            </div>
            <div class="lanAccessItemActions">
              <button
                type="button"
                class="lanAccessUseBtn"
                :title="t('agent.lanAccessUseTitle') || '使用此地址'"
                :aria-label="t('agent.lanAccessUseTitle') || '使用此地址'"
                @click="handleUseLanAddress(addr.url)"
              >
                <ion-icon :icon="checkmarkIcon" />
                <span>{{ t('agent.lanAccessUse') || '使用' }}</span>
              </button>
              <button
                type="button"
                class="lanAccessCopyBtn"
                :title="t('agent.lanAccessCopy')"
                :aria-label="t('agent.lanAccessCopy')"
                @click="handleCopyLanAccess(addr.url)"
              >
                <ion-icon :icon="clipboardIcon" />
              </button>
            </div>
          </li>
        </ul>
        <button
          type="button"
          class="lanAccessRefresh"
          @click="handleRefreshLanAccess"
          :disabled="lanAccessLoading"
        >
          <ion-icon :icon="refreshCircleIcon" />
          <span>{{ t('agent.lanAccessRefresh') }}</span>
        </button>
      </div>
    </details>

    <!-- 消息区域：圆点导航（左）+ 滚动内容（右） -->
    <div class="agentChatBody">
      <!--
        左侧圆点导航（≥2 条 user 消息时显示）。
        v3 修复：
        1. 按 user 消息计数（不是 renderedItems 块数）
           - 一个 user 消息 = 一次完整的"提问 + assistant 回复 + 工具调用"轮次
           - 不再把每个 tool_call / tool_result / thinking 块都算成 1 个圆点
        2. 垂直居中（top: 50% + transform: translateY(-50%)，避免长列表时挤顶部）
        3. 长按 → 圆点变长条 → 上下滑动可快速跳转 → 松开恢复圆点
           - 长按判定 ≥ LONG_PRESS_MS
           - 拖动用 rAF 节流（防抖，避免高频 scrollIntoView 抖动）
           - pointer capture 防止指针滑出元素丢失事件
      -->
      <div
        v-if="userMessageItems.length >= 2"
        class="dotNavigation"
        :class="{ 'dotNavigation--dragging': isDotDragging }"
        ref="dotNavRef"
        @pointerdown="onDotNavPointerDown"
        @pointermove="onDotNavPointerMove"
        @pointerup="onDotNavPointerUp"
        @pointercancel="onDotNavPointerUp"
      >
        <button
          v-for="(ui, dotIdx) in userMessageItems"
          :key="ui.item.messageId"
          type="button"
          class="dotNavDot"
          :class="{
            dotNavDot_active: !isDotDragging && dotIdx === activeUserMessageIdx,
            dotNavDot_dragged: isDotDragging && dotIdx === draggedDotIdx,
          }"
          :title="`跳转到第 ${dotIdx + 1} 个提问`"
          @click.stop="onDotClick(dotIdx)"
        />
      </div>

      <main class="agentChatMain" ref="mainRef" @scroll="onMainScroll">
      <!--
        多渲染引擎架构：通过 EngineRenderer 组件渲染当前引擎的消息列表。
        EngineRenderer 使用 render() 函数直接输出 VNode，避免 <component :is="vnode"> 不稳定。
      -->
      <EngineRenderer
        v-if="currentEngine"
        :engine="currentEngine"
        :render-props="engineRenderProps"
      />
      <!-- 引擎加载失败时的 fallback（不应触发，但防御性保留） -->
      <div v-else class="agentChatEmpty">
        <ion-icon :icon="chatbubblesIcon" class="emptyIcon" />
        <p>引擎加载失败，请刷新页面</p>
      </div>
    </main>

      <!--
        Task 8：右上角浮动缩放按钮组 "A- / A / A+"
        - 浮于 ion-content（main）之上，不参与缩放事件
        - 程序化控制 pinch.zoomIn / zoomOut / resetZoom
        - i18n label 来自 agent.zoom.{in,out,reset}（已加到 i18n/agent.ts）
        - size="small" + fill="clear" 不占视觉重心
      -->
      <div class="zoomControls" :class="{ zoomControls_zoomed: pinch.zoomScale.value !== 1.0 }">
        <button
          type="button"
          class="zoomBtn"
          :title="t('agent.zoom.out')"
          :aria-label="t('agent.zoom.out')"
          @click="pinch.zoomOut()"
        >
          A−
        </button>
        <button
          type="button"
          class="zoomBtn"
          :title="t('agent.zoom.reset')"
          :aria-label="t('agent.zoom.reset')"
          @click="pinch.resetZoom()"
        >
          A
        </button>
        <button
          type="button"
          class="zoomBtn"
          :title="t('agent.zoom.in')"
          :aria-label="t('agent.zoom.in')"
          @click="pinch.zoomIn()"
        >
          A+
        </button>
      </div>
    </div><!-- /.agentChatBody -->

    <footer class="agentChatFooter">
      <!--
        Task 10: "/" 触发的命令面板（useSlashMenu + SlashMenu 组件）。
        模板挂在 footer 内，组件自身用 Teleport 把 overlay 提升到 body
        以避免被 textarea 滚动裁剪。
      -->
      <SlashMenu
        v-if="slashMenu.isOpen.value"
        :items="slashMenu.items.value"
        :query="slashMenu.query.value"
        :selected-index="slashMenu.selectedIndex.value"
        :on-apply="(id) => slashMenu.applyById(id)"
        :on-close="slashMenu.closeMenu"
        :on-selected-index-change="(n) => (slashMenu.selectedIndex.value = n)"
      />

      <!--
        Task 12: 附件展示行（textarea 上方）
      -->
      <AttachmentTray
        v-if="attachments.length > 0"
        :attachments="attachments"
        :on-remove="removeAttachment"
      />

      <!--
        v2 工具快捷动作 chip 行：让用户一键 pre-fill v2 工具调用 prompt。
        6 个 chip（搜索/读/元数据/改元数据/批量改名/跑命令）覆盖 v2 主力能力。
        点击 chip → inputText 填入示例 prompt，焦点回输入框，用户按 Enter 发送。
      -->
      <V2QuickActions
        :disabled="status === 'streaming' || status === 'confirming'"
        @pick="onPickV2QuickAction"
      />

      <!--
        Mock 模式预设输入控件：覆盖在输入框上方。
        数据由 useAgent().mockPresets 提供（后端 mock_presets 事件驱动）。
        mid-scenario 会再次触发事件 → 完整覆盖 chip 列表（连续会话预设）。
        流式进行中（status === 'streaming'）时禁用 chip，防止重复触发。
      -->
      <MockPresetBar
        v-if="isMockMode && mockPresets.length > 0"
        :presets="mockPresets"
        :scenario="mockPresetBarScenario"
        :phase="mockPresetBarPhase"
        :disabled="status === 'streaming'"
        @pick="(preset) => { void pickMockPreset(preset) }"
      />

      <!--
        v2 多轮/分支剧本：剧本 mid-step 暂停时由 useAgent 推 mock_branch_choice
        事件 → 渲染此 chip 列表；用户点 chip → 走 pickMockBranch(branch.id)
        → send(branchId, { mode: 'mock_resume' }) 通知后端 Resume。
        显隐由 mockScenarioPaused（派生 computed）控制 —— phase 在
        awaiting_user_input / awaiting_branch_choice 时显示。
        优先级高于 MockPresetBar（"剧本等待用户选"必须盖在快捷入口之上）。
      -->
      <MockBranchChoiceBar
        v-if="isMockMode && mockScenarioPaused && mockBranchChoices.length > 0"
        :paused="mockScenarioPaused"
        :scenario="currentMockScenario"
        :round="mockRoundState?.roundIdx"
        :total="mockRoundState?.totalRounds"
        :prompt="mockBranchPrompt"
        :branches="mockBranchChoices"
        :phase="mockRoundState?.phase ?? ''"
        @pick="(branch) => pickMockBranch(branch.id)"
      />

      <!--
        调试面板：mock 模式可手动展开 / URL ?debug=agent 强制开
        v2 修复：默认折叠，避免遮挡对话。需要时手动点开。
      -->
      <AgentDebugPanel
        v-if="isMockMode || isDebugAgent"
        :messages="messages"
        :rendered-items="renderedItems"
        :agent-status="status"
        :raw-sse-events="rawSSEEvents"
        :default-open="false"
      />

      <div class="footerInputRow" :class="{ 'footerInputRow-palette': slashMenu.isOpen.value }">
        <!-- 模型选择器（输入框内嵌，参考 ChatGPT/Claude 主流设计） -->
        <div class="modelPicker" ref="modelPickerRef">
          <button
            type="button"
            class="modelPickerBtn"
            :disabled="status === 'streaming' || modelsLoading"
            @click="modelPickerOpen = !modelPickerOpen"
            :title="t('agent.model')"
          >
            <span class="modelPickerLabel">{{ currentModelDisplayName }}</span>
            <ion-icon :icon="chevronDownIcon" class="modelPickerArrow" :class="{ 'modelPickerArrow_open': modelPickerOpen }" />
          </button>
          <Transition name="modelPickerFade">
            <div v-if="modelPickerOpen" class="modelPickerDropdown">
              <div v-if="modelsLoading" class="modelPickerLoading">{{ t('agent.loadingModels') }}...</div>
              <template v-else-if="modelsError">
                <div class="modelPickerError">{{ modelsError }}</div>
                <button
                  v-if="selectedModel && !isSelectedModelAvailable"
                  type="button"
                  class="modelPickerOption modelPickerOption_active"
                  @click="selectModel(selectedModel); modelPickerOpen = false"
                >{{ selectedModel }}</button>
              </template>
              <template v-else>
                <button
                  v-for="m in availableModels"
                  :key="m.id"
                  type="button"
                  class="modelPickerOption"
                  :class="{ 'modelPickerOption_active': selectedModel === m.id }"
                  @click="selectModel(m.id); modelPickerOpen = false"
                >
                  <span class="modelPickerOptionName">{{ m.name }}</span>
                  <span v-if="m.provider !== 'unknown'" class="modelPickerOptionProvider">{{ m.provider }}</span>
                </button>
              </template>
            </div>
          </Transition>
        </div>

        <!-- Task 12: 附件 `+` 按钮 -->
        <button
          v-if="status !== 'streaming'"
          type="button"
          class="footerAttachBtn"
          :title="t('agent.attach')"
          :aria-label="t('agent.attach')"
          @click="triggerAttach"
        >
          <ion-icon :icon="attachIcon" />
        </button>
        <input
          ref="fileInputRef"
          type="file"
          multiple
          class="footerAttachInput"
          @change="handleAttachChange"
        />
        <textarea
          v-model="inputText"
          class="footerInput"
          rows="1"
          :placeholder="t('agent.placeholder')"
          :disabled="status === 'streaming'"
          @keydown.ctrl.enter.exact.prevent="handleSend"
          @keydown.meta.enter.exact.prevent="handleSend"
          @keydown="onTextareaKeydown"
          @input="onTextareaInput"
          ref="inputRef"
        ></textarea>
        <button
          v-if="status !== 'streaming'"
          type="button"
          class="footerSendBtn"
          :disabled="!canSend"
          @click="handleSend"
          :title="t('agent.send')"
        >
          <ion-icon :icon="sendIcon" />
        </button>
        <button
          v-else
          type="button"
          class="footerStopBtn"
          @click="handleStop"
          :title="t('agent.stop')"
        >
          <ion-icon :icon="stopIcon" />
        </button>
      </div>
      <div class="footerHint">{{ t('agent.inputHint') }}</div>
    </footer>

    <!--
      v2 修复：会话历史改为全屏显示
      - 占用整个 .agentChat 容器（覆盖在主聊天之上）
      - 顶部 header：关闭按钮 + 标题 + 新会话大加号按钮
      - 列表：每条会话显示标题 + 元信息 + 删除按钮（始终可见，不再 hover 才显示）
      - 全屏布局方便用户浏览历史 / 删除 / 切换
    -->
    <div v-if="historyOpen" class="historyOverlay">
      <header class="historyHeader">
        <button type="button" class="headerBtn" @click="handleClose" :title="t('common.close') || '关闭'">
          <ion-icon :icon="closeIcon" />
        </button>
        <div class="historyHeaderTitle">
          <ion-icon :icon="timeIcon" class="historyHeaderIcon" />
          <h3>{{ t('agent.history') || '会话历史' }}</h3>
          <span v-if="sessions.length > 0" class="historyHeaderCount">{{ sessions.length }}</span>
        </div>
        <div class="historyHeaderRight">
          <span class="historyHeaderHint">长按删除</span>
        </div>
      </header>

      <!--
        大加号按钮：放在全屏历史界面的正中央下方，作为醒目入口
        用户从全屏历史中可以直接"新建会话"而无需先关闭
      -->
      <button
        v-if="sessions.length > 0"
        type="button"
        class="historyNewSessionFab"
        @click="handleNewSessionFromHistory"
        :title="t('agent.newSession') || '新会话'"
      >
        <ion-icon :icon="addIcon" class="historyNewSessionFabIcon" />
        <span class="historyNewSessionFabLabel">{{ t('agent.newSession') || '新会话' }}</span>
      </button>

      <div class="historyList">
        <div
          v-for="s in sessions"
          :key="s.id"
          class="historyItem"
          :class="{ historyItemActive: s.id === currentSessionId }"
          @click="switchSession(s.id); historyOpen = false"
        >
          <ion-icon :icon="chatbubblesIcon" class="historyItemIcon" />
          <div class="historyItemMain">
            <p class="historyItemTitle">{{ s.title || '(空)' }}</p>
            <p class="historyItemMeta">
              {{ formatSessionMeta(s) }}
            </p>
          </div>
          <!-- v2 修复：删除按钮始终可见（不依赖 hover），全屏下方便手指操作 -->
          <button
            type="button"
            class="historyItemDelete"
            @click.stop="handleDeleteSession(s.id, $event)"
            :title="t('agent.deleteSession') || '删除会话'"
            :aria-label="t('agent.deleteSession') || '删除会话'"
          >
            <ion-icon :icon="trashIcon" />
          </button>
        </div>
        <div v-if="sessions.length === 0" class="historyEmpty">
          <ion-icon :icon="chatbubblesIcon" class="historyEmptyIcon" />
          <p>{{ t('agent.noHistory') || '暂无历史会话' }}</p>
          <!-- 空状态下大加号按钮 -->
          <button
            type="button"
            class="historyNewSessionFab historyNewSessionFab--empty"
            @click="handleNewSessionFromHistory"
            :title="t('agent.newSession') || '新会话'"
          >
            <ion-icon :icon="addIcon" class="historyNewSessionFabIcon" />
            <span class="historyNewSessionFabLabel">{{ t('agent.newSession') || '新会话' }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// AgentChat.vue 重构后只剩 thin script：调用 useAgentChatView() composable + 必要 imports。
// 原 1061 行 script 逻辑已全部抽到 ./useAgentChatView.ts。
//
// template 内直接使用的 state/handler 都在此解构到局部变量，
// Vue 3 <script setup> 自动暴露顶层 binding 给 template，所以 template 用法保持不变。

import { useAgentChatView } from "./useAgentChatView";
import V2ScenariosMenu from "@/components/agent/V2ScenariosMenu.vue";
import ContextIcon from "@/components/agent/ContextIcon.vue";
import EngineRenderer from "@/components/agent/EngineRenderer.vue";
import SlashMenu from "@/components/agent/SlashMenu.vue";
import AttachmentTray from "@/components/agent/AttachmentTray.vue";
import V2QuickActions from "@/components/agent/V2QuickActions.vue";
import MockPresetBar from "@/components/agent/MockPresetBar.vue";
import MockBranchChoiceBar from "@/components/agent/MockBranchChoiceBar.vue";
import AgentDebugPanel from "@/components/agent/AgentDebugPanel.vue";

const {
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
  // mock preset / scenario
  mockPresetBarScenario,
  mockPresetBarPhase,
  // API base
  goToApiKeySettings,
  // useAgent re-exposed values
  messages,
  status,
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
  // renderTurnItems
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
  // file input
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
  isSelectedModelAvailable,
  modelPickerOpen,
  modelPickerRef,
  currentModelDisplayName,
  selectModel,
  // mock mode toggle
  mockBadgeText,
  mockBadgeTitle,
  toggleMockMode,
  // session meta / send / stop / v2
  formatSessionMeta,
  handleSend,
  handleStop,
  onPickV2QuickAction,
  onPickV2Scenario,
  // history
  handleNewSessionFromHistory,
  handleOpenHistory,
  handleDeleteSession,
  switchSession,
  // close modal
  handleCloseModal,
  handleClose,
  // scroll / dot observer
  onMainScroll,
  // dot nav
  userMessageItems,
  activeUserMessageIdx,
  onDotClick,
  dotNavRef,
  isDotDragging,
  draggedDotIdx,
  onDotNavPointerDown,
  onDotNavPointerMove,
  onDotNavPointerUp,
} = useAgentChatView();

// 暴露给 modal container（可选）
defineExpose({});
</script>

<style scoped>
.agentChat {
  display: flex;
  flex-direction: column;
  height: 100vh;
  max-height: 100vh;
  width: 100vw;
  background: var(--ion-background-color);
  color: var(--ion-text-color);
}

.agentChatHeader {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  background: var(--ion-toolbar-background, var(--ion-background-color));
  border-bottom: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  flex-shrink: 0;
}

.headerBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 0;
  background: transparent;
  border-radius: 8px;
  color: var(--ion-text-color);
  cursor: pointer;
  font-size: 18px;
  padding: 0;
}

.headerBtn:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
}

.headerBtnIcon ion-icon {
  font-size: 20px;
}

.headerTitle {
  flex: 1;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 15px;
  font-weight: 600;
  justify-content: center;
}

.headerTitleIcon {
  color: var(--ion-color-primary);
  font-size: 18px;
}

/* ── Mock 模式切换器（始终可见、可点击、反映当前模式） ── */
.mockBadge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border: 0;
  border-radius: 12px;
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  color: var(--ion-color-medium);
  font-size: 11px;
  font-weight: 500;
  line-height: 1.4;
  user-select: none;
  cursor: pointer;
  font-family: inherit;
  transition: background 0.15s ease, color 0.15s ease, transform 0.1s ease;
}

.mockBadge:hover {
  background: rgba(var(--ion-color-medium-rgb), 0.22);
}

.mockBadge:active {
  transform: scale(0.96);
}

.mockBadge:focus-visible {
  outline: 2px solid var(--ion-color-primary);
  outline-offset: 1px;
}

/* 启用 mock（builtin / custom）时的强调色 */
.mockBadge_active {
  background: rgba(var(--ion-color-primary-rgb), 0.16);
  color: var(--ion-color-primary);
}

.mockBadge_active:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.24);
}

.mockBadgeIcon {
  font-size: 12px;
  color: inherit;
}

.mockBadgeText {
  letter-spacing: 0.02em;
}

.mockBadgeChevron {
  font-size: 10px;
  color: inherit;
  opacity: 0.7;
}

/* ── 多渲染引擎切换器（同款模型选择器样式） ─────────── */
.enginePicker {
  position: relative;
  flex-shrink: 0;
}

.enginePickerBtn {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  height: 26px;
  padding: 0 8px;
  border: 1px solid rgba(var(--ion-color-primary-rgb), 0.25);
  border-radius: 8px;
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  color: var(--ion-color-primary);
  cursor: pointer;
  font-size: 11px;
  font-weight: 500;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.enginePickerBtn:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.14);
  border-color: rgba(var(--ion-color-primary-rgb), 0.3);
}

.enginePickerLabel {
  max-width: 80px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.enginePickerArrow {
  font-size: 12px;
  transition: transform 0.2s ease;
  color: var(--ion-color-primary);
  opacity: 0.7;
}

.enginePickerArrow_open {
  transform: rotate(180deg);
}

.enginePickerDropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 50%;
  transform: translateX(-50%);
  min-width: 140px;
  max-width: 220px;
  background: var(--ion-background-color);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.2);
  border-radius: 10px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.14);
  z-index: 50;
  padding: 4px;
}

.enginePickerOption {
  display: flex;
  align-items: center;
  width: 100%;
  padding: 7px 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--ion-text-color);
  cursor: pointer;
  font-size: 12px;
  text-align: left;
  transition: background 0.12s;
}

.enginePickerOption:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}

.enginePickerOption_active {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  font-weight: 600;
  color: var(--ion-color-primary);
}

.enginePickerOptionName {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agentChatMain {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px 12px 12px 36px; /* 左侧留出圆点导航空间 (4px gap + ~24px nav + 8px margin) */
  display: flex;
  flex-direction: column;
  gap: 6px;
  -webkit-overflow-scrolling: touch;
  position: relative;
}

.agentChatEmpty {
  margin: auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--encv-text-secondary);
  font-size: 13px;
}

.emptyIcon {
  font-size: 40px;
  color: rgba(var(--ion-color-primary-rgb), 0.3);
}

.renderedItemWrap {
  display: flex;
  flex-direction: column;
}

/* 独立 Footer 段：时间戳固定，与 AssistantMessage 解耦 */
.messageFooterStandalone {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-left: 36px;
  margin-top: 2px;
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
  padding-top: 4px;
}

.messageFooterStandalone .footerTimestamp {
  font-size: 11px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.6));
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
  user-select: none;
}

.messageFooterStandalone .footerCopyBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--encv-text-secondary, rgba(127,127,127,0.45));
  cursor: pointer;
  font-size: 14px;
  padding: 0;
  transition: color 0.15s, background 0.15s;
}

.messageFooterStandalone .footerCopyBtn:hover,
.messageFooterStandalone .footerCopyBtn:active {
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}

.agentChatFooter {
  flex-shrink: 0;
  padding: 8px 12px 12px;
  background: var(--ion-toolbar-background, var(--ion-background-color));
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
}

.footerInputRow {
  display: flex;
  align-items: flex-end;
  gap: 6px;
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.2);
  border-radius: 12px;
  padding: 4px 6px;
}

.footerInput {
  flex: 1;
  resize: none;
  background: transparent;
  border: 0;
  outline: none;
  font-size: 14px;
  line-height: 1.45;
  font-family: inherit;
  color: var(--ion-text-color);
  max-height: 120px;
  padding: 6px 8px;
  word-break: break-word;
}

.footerInput:disabled {
  opacity: 0.55;
}

.footerSendBtn,
.footerStopBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 0;
  border-radius: 10px;
  cursor: pointer;
  font-size: 18px;
  flex-shrink: 0;
  color: #fff;
}

/* Task 12：附件 `+` 按钮（与发送按钮同尺寸，无背景色） */
.footerAttachBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--ion-color-primary);
  cursor: pointer;
  font-size: 18px;
  flex-shrink: 0;
  padding: 0;
  transition: background 0.12s;
}

.footerAttachBtn:hover,
.footerAttachBtn:active {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
}

/* 隐藏原生 file input —— 用按钮触发 */
.footerAttachInput {
  display: none;
}

/* ── 模型选择器（输入框内嵌） ──────────────────────────── */
.modelPicker {
  position: relative;
  flex-shrink: 0;
}

.modelPickerBtn {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  height: 30px;
  padding: 0 8px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.25);
  border-radius: 8px;
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  color: var(--ion-color-primary);
  cursor: pointer;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.modelPickerBtn:hover:not(:disabled) {
  background: rgba(var(--ion-color-primary-rgb), 0.14);
  border-color: rgba(var(--ion-color-primary-rgb), 0.3);
}

.modelPickerBtn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.modelPickerLabel {
  max-width: 90px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.modelPickerArrow {
  font-size: 12px;
  transition: transform 0.2s ease;
  color: var(--ion-color-primary);
  opacity: 0.7;
}

.modelPickerArrow_open {
  transform: rotate(180deg);
}

.modelPickerDropdown {
  position: absolute;
  bottom: calc(100% + 6px);
  left: 0;
  min-width: 180px;
  max-width: 260px;
  max-height: 240px;
  overflow-y: auto;
  background: var(--ion-background-color);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.2);
  border-radius: 10px;
  box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.14);
  z-index: 50;
  padding: 4px;
}

.modelPickerLoading,
.modelPickerError {
  padding: 10px 12px;
  font-size: 12px;
  color: var(--ion-text-color-step-400, #888);
  text-align: center;
}

.modelPickerOption {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--ion-text-color);
  cursor: pointer;
  font-size: 13px;
  text-align: left;
  transition: background 0.12s;
}

.modelPickerOption:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}

.modelPickerOption_active {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  font-weight: 600;
  color: var(--ion-color-primary);
}

.modelPickerOptionName {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.modelPickerOptionProvider {
  font-size: 10px;
  color: var(--ion-text-color-step-400, #999);
  flex-shrink: 0;
  margin-left: 8px;
}

/* 下拉动画 */
.modelPickerFade-enter-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.modelPickerFade-leave-active {
  transition: opacity 0.1s ease, transform 0.1s ease;
}
.modelPickerFade-enter-from {
  opacity: 0;
  transform: translateY(4px);
}
.modelPickerFade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

.footerSendBtn {
  background: var(--ion-color-primary);
}

.footerSendBtn:disabled {
  background: rgba(var(--ion-color-medium-rgb), 0.4);
  cursor: not-allowed;
}

.footerStopBtn {
  background: var(--ion-color-danger);
}

/* ── no_api_key 自愈 banner ─────────────────────────────
   触发条件：chat 发送返回 503 {error: "no_api_key"}（设备解密不开存的密文）。
   设计要点：
   - 高对比红色（与 chat 顶部 toolbar 区分，避免被误以为是普通状态条）
   - icon + 文案 + 主操作按钮 + 关闭按钮四件套，缺一不可
   - 文案给两个句号：短句放强解释，长句放行动指引
*/
.noApiKeyBanner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: linear-gradient(
    90deg,
    rgba(var(--ion-color-danger-rgb), 0.16),
    rgba(var(--ion-color-danger-rgb), 0.08)
  );
  border-bottom: 1px solid rgba(var(--ion-color-danger-rgb), 0.4);
  color: var(--ion-color-danger-shade);
  font-size: 13px;
  flex-shrink: 0;
}
.noApiKeyBannerIcon {
  font-size: 18px;
  flex-shrink: 0;
}
.noApiKeyBannerText {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  line-height: 1.35;
}
.noApiKeyBannerText strong {
  font-size: 13px;
  font-weight: 600;
}
.noApiKeyBannerText span {
  font-size: 12px;
  opacity: 0.85;
}
.noApiKeyBannerBtn {
  background: var(--ion-color-danger);
  color: var(--ion-color-danger-contrast, #fff);
  border: none;
  border-radius: 6px;
  padding: 5px 12px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  flex-shrink: 0;
}
.noApiKeyBannerBtn:hover {
  opacity: 0.9;
}
.noApiKeyBannerClose {
  background: transparent;
  border: none;
  color: var(--ion-color-danger-shade);
  cursor: pointer;
  padding: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  flex-shrink: 0;
}
.noApiKeyBannerClose ion-icon {
  font-size: 18px;
}

/* ── Toolbar (model / temperature) ─────────────────── */
.agentChatToolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 12px;
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-bottom: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
  flex-shrink: 0;
}

/* ── LAN Access 折叠面板（Task 26） ─────────────────── */
.lanAccessPanel {
  background: var(--ion-toolbar-background, var(--ion-background-color));
  border-bottom: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
  flex-shrink: 0;
  font-size: 13px;
}

.lanAccessPanel[open] {
  padding-bottom: 8px;
}

.lanAccessSummary {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  cursor: pointer;
  user-select: none;
  list-style: none;
  outline: none;
}

.lanAccessSummary::-webkit-details-marker {
  display: none;
}

.lanAccessSummary::marker {
  content: '';
}

.lanAccessSummaryIcon {
  font-size: 16px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.lanAccessSummaryText {
  flex: 1;
  font-weight: 500;
  color: var(--ion-text-color);
}

.lanAccessSummaryCount {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
}

.lanAccessBody {
  padding: 0 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.lanAccessHelp {
  margin: 0 0 2px;
  font-size: 11px;
  color: var(--ion-text-color-step-400, #888);
}

.lanAccessEmpty,
.lanAccessLoading {
  font-size: 12px;
  color: var(--ion-text-color-step-400, #888);
  padding: 4px 0;
}

.lanAccessList {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.lanAccessItem {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-radius: 8px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
}

.lanAccessItemMain {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.lanAccessUrl {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 12px;
  color: var(--ion-color-primary);
  word-break: break-all;
  line-height: 1.35;
}

.lanAccessInterface {
  font-size: 10px;
  color: var(--ion-text-color-step-400, #888);
}

.lanAccessCopyBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--ion-color-primary);
  cursor: pointer;
  font-size: 16px;
  padding: 0;
  flex-shrink: 0;
  transition: background 0.12s;
}

.lanAccessCopyBtn:hover,
.lanAccessCopyBtn:active {
  background: rgba(var(--ion-color-primary-rgb), 0.14);
}

/* Task 26 扩展：LAN 列表项右侧动作组（使用 + 复制） */
.lanAccessItemActions {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.lanAccessUseBtn {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  height: 30px;
  padding: 0 10px;
  border: 1px solid rgba(var(--ion-color-primary-rgb), 0.4);
  border-radius: 8px;
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  color: var(--ion-color-primary);
  cursor: pointer;
  font-size: 11.5px;
  font-weight: 500;
  transition: background 0.12s, border-color 0.12s;
}

.lanAccessUseBtn ion-icon {
  font-size: 14px;
}

.lanAccessUseBtn:hover,
.lanAccessUseBtn:active {
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  border-color: rgba(var(--ion-color-primary-rgb), 0.6);
}

.lanAccessRefresh {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  margin-top: 4px;
  padding: 4px 10px;
  font-size: 11px;
  color: var(--ion-text-color-step-400, #888);
  background: transparent;
  border: 0;
  border-radius: 6px;
  cursor: pointer;
  align-self: flex-start;
}

.lanAccessRefresh:hover:not(:disabled),
.lanAccessRefresh:active:not(:disabled) {
  background: rgba(var(--ion-color-medium-rgb), 0.1);
  color: var(--ion-text-color);
}

.lanAccessRefresh:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.toolbarField {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
}

.toolbarFieldNarrow {
  min-width: 70px;
}

.toolbarLabel {
  color: var(--ion-text-color-step-400, #888);
  font-size: 11px;
  white-space: nowrap;
}

.toolbarSelect {
  font-size: 12px;
  padding: 2px 4px;
  border-radius: 6px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.25);
  background: var(--ion-background-color);
  color: var(--ion-text-color);
  outline: none;
  max-width: 160px;
}

.toolbarInput {
  width: 52px;
  font-size: 12px;
  padding: 2px 4px;
  border-radius: 6px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.25);
  background: var(--ion-background-color);
  color: var(--ion-text-color);
  outline: none;
  text-align: center;
}

/* ── Footer hint ─────────────────────────────────────── */
.footerHint {
  text-align: center;
  font-size: 11px;
  color: var(--ion-text-color-step-350, #999);
  padding: 2px 0 0;
  user-select: none;
}

/* ── Tool Palette ("/" 命令面板) ───────────────────────── */
.tool-palette {
  background: var(--ion-background-color);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.25);
  border-radius: 10px 10px 0 0;
  margin: 0 12px;
  max-height: 220px;
  overflow-y: auto;
  box-shadow: 0 -2px 12px rgba(0, 0, 0, 0.08);
  z-index: 10;
}

.tool-palette-header {
  padding: 8px 12px 4px;
  border-bottom: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
}

.tool-palette-title {
  font-size: 11px;
  font-weight: 600;
  color: var(--ion-text-color-step-400, #888);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.tool-palette-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  cursor: pointer;
  transition: background 0.12s;
}

.tool-palette-item:hover,
.tool-palette-item:active,
.tool-palette-active {
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}

.tool-palette-active {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
}

.tool-palette-icon {
  font-size: 18px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.tool-palette-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.tool-palette-name {
  font-size: 13px;
  font-weight: 600;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  color: var(--ion-text-color);
}

.tool-palette-desc {
  font-size: 11px;
  color: var(--ion-text-color-step-400, #888);
}

.tool-palette-empty {
  padding: 16px 12px;
  text-align: center;
  font-size: 12px;
  color: var(--ion-text-color-step-350, #999);
}

.footerInputRow-palette {
  border-radius: 0 0 12px 12px;
  border-top-left-radius: 0;
  border-top-right-radius: 0;
}

/* ── History full-screen layout（v2 重构） ────────────────────── */
/* 关键：覆盖整个 .agentChat 容器（position: absolute + inset: 0） */
.historyOverlay {
  position: absolute;
  inset: 0;
  z-index: 100;
  background: var(--ion-background-color);
  display: flex;
  flex-direction: column;
  animation: historyFadeIn 0.18s ease-out;
}

@keyframes historyFadeIn {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

/* 全屏头部：左关闭 / 中标题 / 右 hint */
.historyHeader {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  background: var(--ion-toolbar-background, var(--ion-background-color));
  border-bottom: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  flex-shrink: 0;
}
.historyHeaderTitle {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
}
.historyHeaderIcon {
  font-size: 18px;
  color: var(--ion-color-primary);
}
.historyHeaderTitle h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.historyHeaderCount {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
  padding: 0 8px;
  border-radius: 11px;
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
}
.historyHeaderRight {
  display: flex;
  align-items: center;
}
.historyHeaderHint {
  font-size: 11px;
  color: var(--ion-text-color-step-400, #999);
  user-select: none;
}

/* 列表区：占据全屏剩余空间 */
.historyList {
  overflow-y: auto;
  flex: 1;
  padding: 8px 0 100px; /* 底部留 100px 给 FAB */
}

/* 列表项（卡片化） */
.historyItem {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  margin: 0 12px 8px;
  background: var(--ion-toolbar-background, var(--ion-background-color));
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.15s;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}
.historyItem:hover,
.historyItem:active {
  border-color: rgba(var(--ion-color-primary-rgb), 0.4);
  transform: translateY(-1px);
  box-shadow: 0 3px 10px rgba(var(--ion-color-primary-rgb), 0.12);
}
.historyItemActive {
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  border-color: var(--ion-color-primary);
}
.historyItemActive .historyItemTitle {
  font-weight: 600;
  color: var(--ion-color-primary);
}

.historyItemIcon {
  font-size: 24px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.historyItemMain {
  flex: 1;
  min-width: 0;
}
.historyItemTitle {
  margin: 0;
  font-size: 14px;
  color: var(--ion-text-color);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.historyItemMeta {
  margin: 3px 0 0;
  font-size: 11px;
  color: var(--ion-text-color-step-400);
}

/* 删除按钮：v2 始终可见（不依赖 hover），全屏下方便手指操作 */
.historyItemDelete {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 0;
  border-radius: 10px;
  background: rgba(var(--ion-color-danger-rgb), 0.1);
  color: var(--ion-color-danger);
  cursor: pointer;
  flex-shrink: 0;
  font-size: 18px;
  padding: 0;
  transition: background 0.12s, transform 0.1s;
}
.historyItemDelete:hover,
.historyItemDelete:active {
  background: rgba(var(--ion-color-danger-rgb), 0.22);
  transform: scale(1.05);
}
.historyItemDelete ion-icon {
  font-size: 18px;
}

/* 空状态：垂直居中 + 大加号按钮 */
.historyEmpty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 60px 16px 0;
  color: var(--ion-text-color-step-400);
}
.historyEmptyIcon {
  font-size: 56px;
  color: rgba(var(--ion-color-primary-rgb), 0.25);
}
.historyEmpty p {
  margin: 0;
  font-size: 13px;
}

/* ── 大加号按钮（v2 关键 UI）── */
/*
  设计要点：
  - 固定在底部右侧（不挡列表）
  - 主色背景（高识别度）
  - 圆角大（FAB 风格）
  - 阴影立体
  - 含图标 + 文字（不只是 FAB 圆点）
*/
.historyNewSessionFab {
  position: fixed;
  bottom: max(20px, env(safe-area-inset-bottom, 20px));
  right: 16px;
  z-index: 10;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 48px;
  padding: 0 20px;
  border: 0;
  border-radius: 24px;
  background: var(--ion-color-primary);
  color: #fff;
  cursor: pointer;
  font-size: 14px;
  font-weight: 600;
  box-shadow: 0 4px 16px rgba(var(--ion-color-primary-rgb), 0.4);
  transition: all 0.15s;
  user-select: none;
}
.historyNewSessionFab:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(var(--ion-color-primary-rgb), 0.5);
}
.historyNewSessionFab:active {
  transform: scale(0.97);
}
.historyNewSessionFabIcon {
  font-size: 20px;
}
.historyNewSessionFabLabel {
  letter-spacing: 0.02em;
}

/* 空状态时按钮变正中央 + 稍大（强调"开始第一次会话"） */
.historyNewSessionFab--empty {
  position: relative;
  bottom: auto;
  right: auto;
  margin-top: 8px;
  height: 52px;
  padding: 0 24px;
  font-size: 15px;
}

/* ── Dot Navigation（左侧圆点导航） ──────────────────────── */
/* 位于 .agentChatBody(flex row) 内，是 main(scroll) 的兄弟元素 */
.agentChatBody {
  display: flex;
  flex: 1;
  min-height: 0;
  position: relative; /* 为绝对定位的圆点导航提供定位上下文 */
}

/*
  v3 修复：
  1) 垂直居中：top: 50% + transform: translateY(-50%)
     - 不再用 top: 8px / bottom: 8px 顶天立地式
     - 圆点列高度由内容决定（max-height 80vh 兜底防溢出）
  2) 不再 overflow-y: auto —— 我们已经过滤到 user 消息，圆点数天然不会爆炸
     - 之前 block 模式 30+ 圆点能溢出滚动；现在 1 个 user 消息 = 1 圆点
  3) min-width 加大（10px → 16px）提高长按时手指命中区
*/
.dotNavigation {
  position: absolute;
  left: 2px;
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 5px;
  z-index: 20;
  padding: 10px 8px;
  min-width: 16px;
  max-height: 80vh;
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.9);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  border-radius: 10px;
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  box-shadow: 0 2px 14px rgba(0, 0, 0, 0.12);
  touch-action: none; /* 防止 pointermove 时浏览器误判为页面滚动 */
  user-select: none;
  -webkit-user-select: none;
}

/* 拖动模式：加阴影 + 放大 + 触觉反馈（vibrate if available） */
.dotNavigation--dragging {
  box-shadow: 0 4px 20px rgba(var(--ion-color-primary-rgb), 0.28);
  border-color: rgba(var(--ion-color-primary-rgb), 0.4);
}

/*
  v3 修复：单圆点样式
  - 默认：8x8 圆形
  - active：放大 1.35x + primary 色 + 光晕
  - dragged（长按拖动中）：变 5x34 长条，圆角 2.5px（scrollbar thumb 风格）
  - transition all：保证圆点↔长条过渡平滑
*/
.dotNavDot {
  display: block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  border: none;
  background: rgba(var(--ion-color-medium-rgb), 0.45);
  cursor: pointer;
  padding: 0;
  flex-shrink: 0;
  transition:
    width 0.18s cubic-bezier(0.4, 0, 0.2, 1),
    height 0.18s cubic-bezier(0.4, 0, 0.2, 1),
    border-radius 0.18s ease,
    background 0.18s ease,
    transform 0.18s ease,
    box-shadow 0.18s ease;
  outline: none;
  -webkit-tap-highlight-color: transparent;
}

.dotNavDot:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.55);
  transform: scale(1.25);
}

/* 自然滚动时的当前 user 消息对应圆点 */
.dotNavDot_active {
  background: var(--ion-color-primary);
  box-shadow: 0 0 0 2px rgba(var(--ion-color-primary-rgb), 0.28);
  transform: scale(1.35);
}

/*
  长按拖动中的圆点 → 变成长条（scrollbar 拇指）
  - width: 5px, height: 34px（≈ 4 个圆点高度之和 + gap）
  - border-radius: 2.5px（半圆端点）
  - 颜色与 active 保持一致（primary + 光晕）
*/
.dotNavDot_dragged {
  width: 5px;
  height: 34px;
  border-radius: 2.5px;
  background: var(--ion-color-primary);
  box-shadow:
    0 0 0 2px rgba(var(--ion-color-primary-rgb), 0.32),
    0 0 8px rgba(var(--ion-color-primary-rgb), 0.55);
  transform: scale(1);
  cursor: grabbing;
}

/* ── Task 8: Zoom Controls（右上角浮动按钮组 A- / A / A+） ── */
/* 浮于 .agentChatBody 右上角（agentChatBody 是 position: relative）。
   不影响 main(scroll) 的滚动，不参与双指缩放事件（按钮在 main 之外）。 */
.zoomControls {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 10;
  display: inline-flex;
  flex-direction: column;
  gap: 4px;
  padding: 4px;
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.78);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border-radius: 10px;
  border: 1px solid rgba(var(--ion-text-color-rgb), 0.08);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  /* 关键：让按钮自身不接收双指缩放（缩放事件由 main 接管） */
  touch-action: manipulation;
}

body.dark .zoomControls {
  background: rgba(30, 30, 30, 0.78);
  border-color: rgba(255, 255, 255, 0.06);
}

.zoomBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 36px;
  height: 28px;
  padding: 0 8px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--ion-text-color);
  font-family: ui-monospace, Menlo, monospace;
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
  cursor: pointer;
  user-select: none;
  -webkit-user-select: none;
  -webkit-tap-highlight-color: transparent;
  transition: background 0.12s ease, color 0.12s ease, transform 0.1s ease;
}

.zoomBtn:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  color: var(--ion-color-primary);
}

.zoomBtn:active {
  transform: scale(0.92);
}

/* 非默认缩放时高亮 reset 按钮：提示用户当前偏离 1.0 */
.zoomControls_zoomed .zoomBtn:nth-child(2) {
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
}
</style>
