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
                  v-if="selectedModel && !availableModels.some(m => m.id === selectedModel)"
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
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'
import { IonIcon, modalController, alertController } from '@ionic/vue'
import {
  closeOutline,
  sparklesOutline,
  addOutline,
  sendOutline,
  stopOutline,
  chatbubblesOutline,
  timeOutline,
  attachOutline,
  globeOutline,
  clipboardOutline,
  refreshCircleOutline,
  keyOutline,
  chevronDownOutline,
  flaskOutline,
  trashOutline,
  checkmarkOutline,
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { getDeviceIdSync } from '@/composables/useDeviceId'
import { getAgentApiBase } from '@/composables/useAgentApiBase'
import { useAgent, type Decision, getLanAccess, type LanAddress } from '@/composables/useAgent'
import { useApiBaseProbe } from '@/composables/useApiBaseProbe'
import { useServerStatus } from '@/composables/useServerStatus'
import { useRenderTurnItems } from '@/composables/renderTurnItems'
import { useAttachments } from '@/composables/useAttachments'
import { useSlashMenu } from '@/composables/useSlashMenu'
import { showToast } from '@/composables/useToast'
// Task 8: 缩放 composable + 共享相对时间格式化
import { formatRelativeTime } from '@/composables/relativeTime'
import { usePinchZoom } from '@/composables/usePinchZoom'
// 多渲染引擎架构：引入引擎系统和已注册的引擎实现
import { useChatEngine } from '@/composables/useChatEngine'
// 触发引擎注册（模块副作用自动注册到 EngineRegistry）
import '@/engines/defaultEngine'
import '@/engines/tdesignEngine'
// 引擎渲染包装组件（解决 <component :is="vnode"> 不稳定的问题）
import EngineRenderer from '@/components/agent/EngineRenderer.vue'
// 以下组件现在由 DefaultMessagesView.vue 内部导入（引擎渲染路径）
// AgentChat.vue 作为宿主容器不再直接引用这些组件
import AttachmentTray from '@/components/agent/AttachmentTray.vue'
import MockPresetBar from '@/components/agent/MockPresetBar.vue'
import MockBranchChoiceBar from '@/components/agent/MockBranchChoiceBar.vue'
import AgentDebugPanel from '@/components/agent/AgentDebugPanel.vue'
import V2QuickActions from '@/components/agent/V2QuickActions.vue'
import V2ScenariosMenu from '@/components/agent/V2ScenariosMenu.vue'
import type { V2ScenarioEntry } from '@/components/agent/V2ScenariosMenu.vue'
import SlashMenu from '@/components/agent/SlashMenu.vue'
import ContextIcon from '@/components/agent/ContextIcon.vue'

const { t } = useI18n()

// ── 多渲染引擎架构：引擎切换系统 ─────────────────────
const { currentEngine, currentEngineId, engineList, switchEngine: doSwitchEngine } = useChatEngine()

// 引擎切换器下拉状态
const enginePickerOpen = ref(false)
const enginePickerRef = ref<HTMLElement | null>(null)

/** 当前引擎显示名称 */
const currentEngineDisplayName = computed(() => {
  const id = currentEngineId.value
  const found = engineList.value.find(e => e.id === id)
  return found?.name || id
})

/** 构建传给当前引擎的 renderProps */
const engineRenderProps = computed(() => ({
  messages: messages.value,
  status: status.value,
  onSend: async (text: string) => { send(text) },
  onStop: () => stop(),
  onConfirmTool: async (id: string, decision: string) => confirmTool(id, decision as Decision),
  onCopyMessage: async (messageId: string) => onCopyMessage(messageId),
  onPresetClick: (userText: string) => pickMockPreset({ id: '', label: userText, tooltip: '', userText } as any),
  streaming: status.value === 'streaming',
}))

/** 引擎切换（带 toast 反馈） */
  function handleSwitchEngine(engineId: string): void {
    const ok = doSwitchEngine(engineId)
    if (ok) {
      const name = engineList.value.find(e => e.id === engineId)?.name || engineId
      showToast({ message: `已切换到 ${name}`, duration: 1200, color: 'success' })
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
  const phase = mockPresetsPhase.value
  const sc = mockPresetsScenario.value
  if (sc === 'scenario_picker' || phase === 'picker') {
    return t('agent.mockPresetBarPickerScenario')
  }
  return sc || mockScenario.value || t('agent.mockPresetBarDefaultScenario')
})
// phase 阶段文案：picker 隐藏（已在 scenario 里表达），其他透传
const mockPresetBarPhase = computed(() => {
  const phase = mockPresetsPhase.value
  if (phase === 'picker' || phase === 'off') return ''
  return phase
})

// Agent API 基础路径（与 useAgent.ts 保持一致）
// Agent API 基础 URL（动态解析：dev 走网关 / prod 直连后端）
const AGENT_API_BASE = getAgentApiBase()

const { messages, status, send, confirmTool, resume, stop, newSession, switchSession, deleteSession, sessions, currentSessionId, contextUsage, lastErrorCode, dismissError, activeModel, setApiDefaultModel, isMockMode, isDebugAgent, mockScenario, currentMockMode, loadMockMode, setMockMode, mockPresets, mockPresetsPhase, mockPresetsScenario, pickMockPreset, loadMockPresets, rawSSEEvents, mockBranchChoices, mockBranchPrompt, mockRoundState, mockScenarioPaused, currentMockScenario, pickMockBranch, sendMockRoundResponse } = useAgent()
const router = useRouter()

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
  dismissError()
  try {
    await modalController.dismiss()
  } catch {/* ignore — 可能 modal 已经被关 */}
  router.push('/tabs/settings/agent')
}

onMounted(() => {
  // 启动 Context 图标的轮询（5s/30s 周期自适应当前 streaming 状态）
  contextUsage.start()
})
onUnmounted(() => {
  // 卸载时清理 timer，避免内存泄漏
  contextUsage.stop()
})

// Task 12：附件管理（Composer `+` 按钮）
const {
  attachments,
  addFiles,
  removeAttachment,
  clearAttachments,
} = useAttachments({
  onError: (msg) => showToast({ message: msg, duration: 2400, color: 'warning' }),
})

// Task 7：把 i18n 解析后的 "上下文已自动压缩" 文本通过 computed
// 注入到 renderTurnItems，renderTurnItems 把它塞进 RenderedItem
// 供 ContextCompactionDivider 直接渲染。这里用 computed 而非
// t('agent.contextCompaction') 直接调用——renderTurnItems 的
// 第三个参数要 Ref/ComputedRef，让语言切换时自动重渲染。
const compactionText = computed(() => t('agent.contextCompaction'))

const renderedItems = useRenderTurnItems(messages, status, compactionText)

const inputText = ref('')
const inputRef = ref<HTMLTextAreaElement | null>(null)
const mainRef = ref<HTMLDivElement | null>(null)
const virtualListRef = ref<{ scrollToBottom: (behavior?: 'auto' | 'smooth') => void } | null>(null)
const nearBottom = ref(true)
const activeMessageIndex = ref(0)

// ─── Task 8: usePinchZoom 集成 ──────────────────────────
// 关键认知：android webview 默认 user-scalable=yes 时会拦截双指捏合
// 整体缩放页面 → 破坏 UI 布局。这里显式接管手势：
//   - 双指捏合 → 计算 distance ratio → 更新 zoomScale → 应用 transform
//   - 右上角 A-/A/A+ 浮动按钮 → 程序化控制缩放
//   - 绑定的 targetRef 是 mainRef（.agentChatMain），即会话内容容器
//   - 缩放范围严格 clamp 到 [0.5, 1.5]，双击重置回 1.0
const pinch = usePinchZoom({ minScale: 0.5, maxScale: 1.5, step: 0.1 })

/** 触发虚拟滚动的阈值（renderedItems 数量 > 此值时切换） */
const VIRTUAL_LIST_THRESHOLD = 120

const closeIcon = closeOutline
const sparkleIcon = sparklesOutline
const addIcon = addOutline
const sendIcon = sendOutline
const stopIcon = stopOutline
const keyIcon = keyOutline
const chatbubblesIcon = chatbubblesOutline
const timeIcon = timeOutline
const attachIcon = attachOutline
const globeIcon = globeOutline
const clipboardIcon = clipboardOutline
const refreshCircleIcon = refreshCircleOutline
const chevronDownIcon = chevronDownOutline
const flaskIcon = flaskOutline
const trashIcon = trashOutline
const checkmarkIcon = checkmarkOutline
// copyIconVar 已移至 DefaultMessagesView.vue（引擎渲染路径内的复制按钮）
const historyOpen = ref(false)

// ── Task 26 (LAN Access) ───────────────────────────────────
// 折叠面板状态：默认收起。数据由 useAgent.getLanAccess() 拉取。
// 展开时才拉取（按需），关闭后保留缓存，避免反复网络请求。
const lanAccessOpen = ref(false)
const lanAccesses = ref<LanAddress[]>([])
const lanAccessLoading = ref(false)
const lanAccessLoaded = ref(false)

async function handleRefreshLanAccess(): Promise<void> {
  lanAccessLoading.value = true
  try {
    lanAccesses.value = await getLanAccess(0)
    lanAccessLoaded.value = true
  } finally {
    lanAccessLoading.value = false
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
 * 与 ServerUrlDetail.vue "使用" 按钮的区别：本处是"立即生效"，不进入配置页
 * （适合用户已经看到 LAN 列表想直接切换的场景）
 */
async function handleUseLanAddress(url: string): Promise<void> {
  try {
    useApiBaseProbe().setManual(url)
    const result = await useServerStatus().manualReconnect()
    if (result.ok) {
      showToast({
        message: t('agent.lanAccessUseSuccess', { url }) || `已切换到 ${url}`,
        duration: 1600,
        color: 'success',
      })
    } else {
      showToast({
        message: `${t('agent.lanAccessUseFailed') || '切换失败'}：${result.error || 'unknown'}`,
        duration: 2000,
        color: 'danger',
      })
    }
  } catch (e) {
    showToast({
      message: t('agent.lanAccessUseFailed') || '切换失败' + ': ' + (e instanceof Error ? e.message : String(e)),
      duration: 2000,
      color: 'danger',
    })
  }
}

async function handleCopyLanAccess(url: string): Promise<void> {
  try {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      await navigator.clipboard.writeText(url)
      showToast({ message: t('agent.lanAccessCopied', { url }), duration: 1600, color: 'success' })
    } else {
      // Fallback：临时 textarea + execCommand（老 webview 兼容）
      const ta = document.createElement('textarea')
      ta.value = url
      ta.style.position = 'fixed'
      ta.style.left = '-9999px'
      document.body.appendChild(ta)
      ta.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(ta)
      if (ok) {
        showToast({ message: t('agent.lanAccessCopied', { url }), duration: 1600, color: 'success' })
      } else {
        showToast({ message: t('agent.lanAccessCopyFailed'), duration: 1800, color: 'danger' })
      }
    }
  } catch {
    showToast({ message: t('agent.lanAccessCopyFailed'), duration: 1800, color: 'danger' })
  }
}

// 监听展开事件：用户首次展开时拉取一次。后续点击「刷新」按钮
// 可强制重拉。watch 比 onMounted 触发更精准——避免用户在折叠
// 面板被滚动出视野前白白消耗一次网络请求。
watch(lanAccessOpen, async (open) => {
  if (open && !lanAccessLoaded.value && !lanAccessLoading.value) {
    await handleRefreshLanAccess()
  }
})

// Task 12：隐藏 file input 的引用
const fileInputRef = ref<HTMLInputElement | null>(null)

function triggerAttach() {
  // 复用同一个 input：每次点击重置 value，确保选同一文件也能触发 change
  const el = fileInputRef.value
  if (!el) return
  el.value = ''
  el.click()
}

async function handleAttachChange(e: Event) {
  const target = e.target as HTMLInputElement
  const files = target.files
  if (!files || files.length === 0) return
  const result = await addFiles(files)
  if (result.rejected.length > 0) {
    const names = result.rejected.map((r) => r.name).join(', ')
    const sample = result.rejected[0]?.reason || '文件超限'
    showToast({
      message: `已跳过 ${result.rejected.length} 个文件（${names}）：${sample}`,
      duration: 3000,
      color: 'warning',
    })
  }
  // 清空 input.value 允许重复选同一文件
  target.value = ''
}

const canSend = computed(() => {
  if (status.value === 'streaming') return false
  // 文本非空 OR 至少一个附件都可以发送
  return inputText.value.trim().length > 0 || attachments.value.length > 0
})

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
    triggerAttach()
  },
  onTogglePlanMode: () => {
    showToast({ message: 'Plan 模式：开发中', duration: 1600, color: 'medium' })
  },
  onTogglePermissionMode: () => {
    showToast({ message: '权限模式：开发中', duration: 1600, color: 'medium' })
  },
  onSelectSkill: (id, label) => {
    // 选中技能 → 在输入框中插入 "@<label> "，等用户继续编辑
    void id // 技能 id 当前仅用于日志/未来埋点；label 用于填充输入
    inputText.value = `@${label} `
    autoResize()
    nextTick(() => inputRef.value?.focus())
  },
})

/**
 * textarea @input 入口：先走原生 autoResize 维持高度，
 * 再把当前文本传给 slashMenu.handleInput 决定开关。
 */
function onTextareaInput() {
  autoResize()
  slashMenu.handleInput(inputText.value)
}

/**
 * textarea @keydown 入口：先让 slashMenu 拦截 ↑
 * ↓ / Enter / Escape（菜单打开时）；未拦截时放行原生行为。
 */
function onTextareaKeydown(e: KeyboardEvent) {
  // slashMenu.handleKeydown 内部决定是否拦截
  if (slashMenu.handleKeydown(e)) return
  // 菜单未打开时：菜单不处理，留给浏览器默认（如 Tab、Backspace 等）
}

// ─── 模型选择（动态从 API 获取） ────────────────────────────
interface ModelOption {
  id: string
  name: string
  provider: string
}

const availableModels = ref<ModelOption[]>([])
const modelsLoading = ref(true)
const modelsError = ref('')

async function fetchModels() {
  modelsLoading.value = true
  modelsError.value = ''
  const did = getDeviceIdSync()
  let url = `${AGENT_API_BASE}/api/models?deviceId=${encodeURIComponent(did)}`
  try {
    const res = await fetch(url)
    if (!res.ok) throw new Error(`HTTP ${res.status} ${res.statusText}`)
    const data = await res.json()
    // 处理各种错误状态
    if (data.error === 'no_api_key') {
      modelsError.value = t('agent.noApiKeyHint') || '未配置 API Key'
      return
    }
    if (data.error || !Array.isArray(data.models)) {
      modelsError.value = data.note || t('agent.modelsError')
      return
    }
    availableModels.value = (data.models || []).map((m: any) => ({
      id: m.id,
      name: m.name || m.id,
      provider: m.provider || 'unknown',
    }))
    // 保存 API 返回的默认模型（新会话时使用）
    if (data.defaultModel) {
      setApiDefaultModel(data.defaultModel)
    }
    // 如果当前选中的模型不在列表中，切换到默认值
    if (availableModels.value.length > 0 && !availableModels.value.some(m => m.id === selectedModel.value)) {
      selectedModel.value = data.defaultModel || availableModels.value[0].id
    }
  } catch (e: any) {
    const errInfo = (() => {
      if (!e) return '(null)'
      if (e instanceof Error) return `${e.name}: ${e.message}`
      try { return JSON.stringify(e) } catch { return String(e) }
    })()
    console.error(`[AgentChat] fetchModels failed: url=${url} error=${errInfo}`)
    // 网络错误等：不阻断用户使用，显示提示但保留已存储的模型选择
    modelsError.value = `${t('agent.modelsError')} (${errInfo})`
  } finally {
    modelsLoading.value = false
  }
}

const SELECTED_MODEL_KEY = 'encv-agent-selected-model'
const TEMPERATURE_KEY = 'encv-agent-temperature'
const storedModel = (() => {
  try {
    return localStorage.getItem(SELECTED_MODEL_KEY) || 'gpt-4o-mini'
  } catch {
    return 'gpt-4o-mini'
  }
})()
const storedTemp = (() => {
  try {
    const v = localStorage.getItem(TEMPERATURE_KEY)
    const n = v == null ? 0.7 : Number(v)
    return Number.isFinite(n) ? n : 0.7
  } catch {
    return 0.7
  }
})()
const selectedModel = ref<string>(storedModel)
const temperature = ref<number>(storedTemp)
watch(selectedModel, (v) => {
  try { localStorage.setItem(SELECTED_MODEL_KEY, v) } catch { /* ignore */ }
  // 同步到 useAgent 的 activeModel（send/sendQueued 读取此值）
  activeModel.value = v
})
watch(temperature, (v) => {
  try { localStorage.setItem(TEMPERATURE_KEY, String(v)) } catch { /* ignore */ }
})

// ─── 模型选择器（输入框内嵌） ─────────────────────────────
const modelPickerOpen = ref(false)
const modelPickerRef = ref<HTMLElement | null>(null)

/** 当前模型的显示名称（从 availableModels 查找，找不到则用 id 本身） */
const currentModelDisplayName = computed(() => {
  const id = selectedModel.value
  const found = availableModels.value.find(m => m.id === id)
  return found?.name || id
})

function selectModel(id: string) {
  selectedModel.value = id
}

// ─── Mock 模式切换器（在会话界面直接配置，弹 action-sheet） ────────
/**
 * 徽章文本：根据当前模式显示对应文案
 *  - off     → "真实 API"   （灰色，提示"未启用 mock"）
 *  - builtin → "模拟·内置"
 *  - custom  → "模拟·自定义"
 */
const mockBadgeText = computed(() => {
  if (currentMockMode.value === 'builtin') return `${t('agent.mockBadge')}·${t('agent.mockModeBuiltin')}`
  return t('agent.mockModeOff')
})

/**
 * 徽章 tooltip：
 *  - active 时显示当前 scenario id（来自最近一次 SSE 响应）
 *  - off 时显示"点击切换模式"
 */
const mockBadgeTitle = computed(() => {
  if (currentMockMode.value === 'off') return t('agent.mockMode')
  if (isMockMode.value && mockScenario.value) {
    return t('agent.mockBadgeTooltip', { scenario: mockScenario.value })
  }
  return t('agent.mockMode')
})

/**
 * 点击徽章 → 直接切换 off ↔ builtin（无 action-sheet）
 * 切换经由 useAgent.setMockMode() 走 PUT /api/config 持久化
 */
async function toggleMockMode(): Promise<void> {
  const next = currentMockMode.value === 'off' ? 'builtin' : 'off'
  try {
    await setMockMode(next)
    showToast({
      message: next === 'off'
        ? (t('agent.mockModeOff') || '真实 API')
        : (t('agent.mockModeBuiltin') || '模拟·内置'),
      duration: 1200,
      color: next === 'off' ? 'medium' : 'success',
    })
  } catch (e) {
    showToast({
      message: `${t('agent.mockModeSetFailed') || '切换失败'}: ${e instanceof Error ? e.message : String(e)}`,
      duration: 2400,
      color: 'danger',
    })
  }
}

/** 点击外部关闭下拉 */
function handleModelPickerOutsideClick(e: MouseEvent) {
  if (modelPickerOpen.value && modelPickerRef.value && !modelPickerRef.value.contains(e.target as Node)) {
    modelPickerOpen.value = false
  }
  if (enginePickerOpen.value && enginePickerRef.value && !enginePickerRef.value.contains(e.target as Node)) {
    enginePickerOpen.value = false
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
  const time = formatRelativeTime(s.updatedAt)
  const parts = [time]
  if (s.rounds > 0) {
    parts.push(`${s.rounds} ${t('agent.rounds') || '轮'}`)
  }
  parts.push(`${s.messageCount} ${t('agent.messages')}`)
  return parts.join(' · ')
}

// ─── 输入框处理 ──────────────────────────────────────────
function autoResize() {
  const el = inputRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 120) + 'px'
}

function handleSend() {
  if (!canSend.value) return
  const text = inputText.value.trim()
  const atts = attachments.value.slice() // 拍快照：避免 send 异步期间被清空后引用空数组
  inputText.value = ''
  autoResize()
  // v2 多轮/分支剧本暂停时：把文本走 sendMockRoundResponse → 后端
  // MockEngineV2 走"恢复"分支（带 scenario ID），而非开新 session。
  // 原因：send() 默认 mode='start' 会让后端重新匹配 scenario，
  //       而 v2 的"恢复"必须显式 mode='mock_resume' + scenario。
  if (mockScenarioPaused.value) {
    sendMockRoundResponse(text)
    nextTick(() => scrollToBottom())
    return
  }
  send(text, { attachments: atts })
  // 发送后清空 tray（避免下次发送重复附带）
  clearAttachments()
  nextTick(() => scrollToBottom())
}

function handleStop() {
  stop()
}

/**
 * v2 工具快捷动作 chip 点击：把示例 prompt 注入输入框 + 聚焦
 *
 * 不直接 send —— 让用户能修改/补充上下文，避免"我都不知道它发了什么"的失控感。
 * 若用户已经在 mock 模式里被 v2 剧本暂停，则走 sendMockRoundResponse 路径。
 */
function onPickV2QuickAction(action: { prompt: string }): void {
  inputText.value = action.prompt
  nextTick(() => {
    inputRef.value?.focus()
    autoResize()
  })
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
  if (status.value === 'streaming' || status.value === 'confirming') {
    showToast({
      message: t('agent.v2Scenarios.busy') || '当前正在请求中，请稍候',
      duration: 1500,
      color: 'warning',
    })
    return
  }
  if (currentMockMode.value === 'off') {
    try {
      await setMockMode('builtin')
    } catch (e) {
      showToast({
        message: (t('agent.mockModeSetFailed') || '切换 mock 模式失败') + ': ' + (e instanceof Error ? e.message : String(e)),
        duration: 2000,
        color: 'danger',
      })
      return
    }
  }
  // 短暂延迟让 mock 模式切换 toast 显示 + UI 更新
  await new Promise((resolve) => setTimeout(resolve, 80))
  void send(scenario.triggerKeyword)
  nextTick(() => scrollToBottom())
}

/**
 * v2 修复：从全屏历史界面直接新建会话
 * - 不需要关闭历史界面再操作 → 流畅体验
 * - 自动关闭全屏历史 → 回到主聊天（新会话已就绪）
 */
async function handleNewSessionFromHistory(): Promise<void> {
  // 来自全屏历史时直接创建（不弹确认，因为历史界面本身就是"切走"的语义）
  newSession()
  historyOpen.value = false
}

async function handleOpenHistory() {
  await Promise.resolve()
  historyOpen.value = true
}

async function handleDeleteSession(sessionId: string, event: Event) {
  event.stopPropagation()
  const alert = await alertController.create({
    header: t('agent.deleteSession'),
    message: t('agent.confirmDeleteSession'),
    buttons: [
      { text: t('common.cancel'), role: 'cancel' },
      { text: t('common.confirm'), role: 'destructive' },
    ],
  })
  await alert.present()
  const { role } = await alert.onDidDismiss()
  if (role === 'destructive') {
    deleteSession(sessionId)
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
    await modalController.dismiss()
  } catch {
    // ignore — modal 可能已经被外部代码 dismiss
  }
}

function handleClose() {
  // v2 修复：全屏历史界面上的"关闭"按钮只关闭历史面板，
  // 不再 dismiss 整个 modal —— 用户希望回到主聊天继续对话
  historyOpen.value = false
}

function scrollToBottom(behavior: 'auto' | 'smooth' = 'smooth') {
  nextTick(() => {
    // 长会话走虚拟列表的 scrollToItem
    if (renderedItems.value.length > VIRTUAL_LIST_THRESHOLD && virtualListRef.value) {
      virtualListRef.value.scrollToBottom(behavior)
      return
    }
    // 短会话走原生 container 滚动
    const el = mainRef.value
    if (!el) return
    el.scrollTo({ top: el.scrollHeight, behavior })
  })
}

/**
 * 监听 main 容器滚动，更新 nearBottom
 * 虚拟列表模式下滚动源是 RecycleScroller 内部 wrapper，
 * 但其 scroll 事件会冒泡到 main 容器，逻辑统一处理
 */
function onMainScroll() {
  const el = mainRef.value
  if (!el) return
  const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
  // 80px 阈值内视为"接近底部"——避免长消息末尾抖动
  nearBottom.value = distanceFromBottom < 80
}

/** IntersectionObserver：追踪当前视口中最接近中心的消息项 */
let dotObserver: IntersectionObserver | null = null

function setupDotObserver() {
  cleanupDotObserver()
  const el = mainRef.value
  if (!el) return
  dotObserver = new IntersectionObserver(
    (entries) => {
      // 找到相交比例最大的元素（最接近视口中心）
      let maxRatio = 0
      let targetIdx = activeMessageIndex.value
      for (const entry of entries) {
        if (entry.intersectionRatio > maxRatio) {
          maxRatio = entry.intersectionRatio
          const idx = Number((entry.target as HTMLElement).dataset.msgIdx ?? -1)
          if (idx >= 0) targetIdx = idx
        }
      }
      if (maxRatio > 0) activeMessageIndex.value = targetIdx
    },
    { root: el, threshold: [0, 0.25, 0.5, 0.75, 1] },
  )
  // 观察所有消息项
  nextTick(() => {
    el.querySelectorAll('.renderedItemWrap').forEach((wrap) => {
      dotObserver?.observe(wrap)
    })
  })
}

function cleanupDotObserver() {
  dotObserver?.disconnect()
  dotObserver = null
}

// 消息列表变化时重建 Observer
watch(renderedItems, () => nextTick(setupDotObserver), { flush: 'post' })
onMounted(() => nextTick(setupDotObserver))
onUnmounted(() => {
  cleanupDotObserver()
  // v3 新增：清理圆点导航的长按 timer / rAF
  clearDotPressTimer()
  clearDotDragRaf()
})

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
  item: { type: 'user'; messageId: string; text: string }
  idx: number // 在 renderedItems 中的下标
}

/** 过滤出所有 user 类型的渲染项 */
const userMessageItems = computed<UserMessageItem[]>(() => {
  const out: UserMessageItem[] = []
  const items = renderedItems.value
  for (let i = 0; i < items.length; i++) {
    const it = items[i]
    if (it.type === 'user') {
      out.push({ item: it, idx: i })
    }
  }
  return out
})

/**
 * 当前激活的 user 消息索引（自然滚动时）
 * 算法：在 userMessageItems 中找 idx <= activeMessageIndex 的最后一个
 */
const activeUserMessageIdx = computed(() => {
  const ai = activeMessageIndex.value
  const list = userMessageItems.value
  if (list.length === 0) return -1
  let result = 0
  for (let i = 0; i < list.length; i++) {
    if (list[i].idx <= ai) {
      result = i
    } else {
      break
    }
  }
  return result
})

/** 跳转到指定 user 消息（点击圆点时调用） */
function onDotClick(dotIdx: number) {
  const ui = userMessageItems.value[dotIdx]
  if (!ui) return
  scrollToUserMessage(ui.idx)
}

/** 跳转到 user 消息在 renderedItems 中的下标 */
function scrollToUserMessage(renderedIdx: number) {
  const el = mainRef.value
  if (!el) return
  const target = el.querySelector(`[data-msg-idx="${renderedIdx}"]`) as HTMLElement | null
  if (!target) return
  target.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// ── 长按 → 长条 → 拖动 → 松开（瞬态） ──────────────────────
const dotNavRef = ref<HTMLElement | null>(null)
const isDotDragging = ref(false)
const draggedDotIdx = ref<number | null>(null)

/** 长按判定阈值（ms） */
const DOT_LONG_PRESS_MS = 280
/** 拖动前最大允许位移（px）—— 超过则判定为"快速滑动/误触"而取消长按 */
const DOT_DRAG_THRESHOLD_PX = 5
/** 单个圆点占用的总高度（8 圆点 + 5 gap） */
const DOT_ITEM_HEIGHT = 13

let dotPressTimer: ReturnType<typeof setTimeout> | null = null
let dotDragRafId: number | null = null
let dotPressStartY = 0
let dotPressStartX = 0
let dotPressActivePointerId: number | null = null
let pendingDragY = 0

function clearDotPressTimer() {
  if (dotPressTimer !== null) {
    clearTimeout(dotPressTimer)
    dotPressTimer = null
  }
}

function clearDotDragRaf() {
  if (dotDragRafId !== null) {
    cancelAnimationFrame(dotDragRafId)
    dotDragRafId = null
  }
}

/**
 * 在拖动模式下，根据指针 clientY 找出对应的圆点索引
 * - 圆点列中心 = dotNavRef.getBoundingClientRect() 的 top + height/2
 * - 每个圆点占 13px，按偏移/13 估算索引
 */
function dotIdxFromClientY(clientY: number): number {
  const navEl = dotNavRef.value
  if (!navEl) return 0
  const rect = navEl.getBoundingClientRect()
  // 圆点 nav 的 padding-top 是 8px；按"第一个圆点中心位于 padding 后 4px"估算
  const firstDotCenter = rect.top + 8 + 4
  const offset = clientY - firstDotCenter
  const idx = Math.round(offset / DOT_ITEM_HEIGHT)
  return Math.max(0, Math.min(userMessageItems.value.length - 1, idx))
}

/**
 * 处理 pointermove：长按未触发前如果移动过多则取消；触发后用 rAF 节流更新 draggedDotIdx
 */
function handleDotNavPointerMove(e: PointerEvent) {
  if (dotPressTimer === null && !isDotDragging.value) return
  if (dotPressActivePointerId !== null && e.pointerId !== dotPressActivePointerId) return

  // 长按未触发：检查是否超过位移阈值
  if (!isDotDragging.value) {
    const dy = Math.abs(e.clientY - dotPressStartY)
    const dx = Math.abs(e.clientX - dotPressStartX)
    if (dy > DOT_DRAG_THRESHOLD_PX || dx > DOT_DRAG_THRESHOLD_PX) {
      // 取消长按 → 视为轻触滚动（不做事）
      clearDotPressTimer()
      return
    }
    return
  }

  // 拖动模式：rAF 节流更新
  pendingDragY = e.clientY
  if (dotDragRafId === null) {
    dotDragRafId = requestAnimationFrame(() => {
      dotDragRafId = null
      const idx = dotIdxFromClientY(pendingDragY)
      if (idx !== draggedDotIdx.value) {
        draggedDotIdx.value = idx
        // 滚动主内容到对应的 user 消息
        const ui = userMessageItems.value[idx]
        if (ui) scrollToUserMessage(ui.idx)
      }
    })
  }
}

function onDotNavPointerDown(e: PointerEvent) {
  // 只响应主指针（左键 / 单点触摸）
  if (e.pointerType === 'mouse' && e.button !== 0) return
  // 防止在已经激活的拖动上叠加新的指针
  if (dotPressActivePointerId !== null) return

  const navEl = e.currentTarget as HTMLElement
  dotPressActivePointerId = e.pointerId
  dotPressStartY = e.clientY
  dotPressStartX = e.clientX
  pendingDragY = e.clientY
  draggedDotIdx.value = null
  isDotDragging.value = false

  // 捕获指针：后续 move/up 仍由本元素接收（即使指针滑出元素）
  try {
    navEl.setPointerCapture(e.pointerId)
  } catch {
    // 部分平台不支持，静默继续
  }

  // 启动长按计时
  clearDotPressTimer()
  dotPressTimer = setTimeout(() => {
    dotPressTimer = null
    if (dotPressActivePointerId === null) return
    isDotDragging.value = true
    // 进入拖动模式：初始 idx 按当前指针位置计算
    const idx = dotIdxFromClientY(dotPressStartY)
    draggedDotIdx.value = idx
    const ui = userMessageItems.value[idx]
    if (ui) scrollToUserMessage(ui.idx)
  }, DOT_LONG_PRESS_MS)
}

function onDotNavPointerMove(e: PointerEvent) {
  handleDotNavPointerMove(e)
}

function onDotNavPointerUp(e: PointerEvent) {
  handleDotNavPointerUp(e)
}

function handleDotNavPointerUp(e: PointerEvent) {
  // 清理状态
  clearDotPressTimer()
  clearDotDragRaf()

  // 释放指针捕获
  const navEl = e.currentTarget as HTMLElement | null
  if (navEl && dotPressActivePointerId !== null) {
    try {
      navEl.releasePointerCapture(dotPressActivePointerId)
    } catch {
      // ignore
    }
  }

  const wasDragging = isDotDragging.value
  isDotDragging.value = false
  draggedDotIdx.value = null
  dotPressActivePointerId = null

  // 拖动模式松开 → 已经在拖动中跳转过了，无需再做
  if (wasDragging) {
    e.preventDefault()
  }
  // 否则走 @click（短按 = 跳转）→ 已绑定 onDotClick
}

// 监听 status 变化 → streaming 开始时滚动到底部
watch(
  () => status.value,
  (newStatus) => {
    if (newStatus === 'streaming') {
      scrollToBottom()
    }
  },
)

// 监听 messages 变化（长度/最后一条）→ 接近底部时自动滚
watch(
  () => messages.value.length,
  () => {
    if (nearBottom.value) scrollToBottom()
  },
)

watch(
  () => messages.value[messages.value.length - 1]?.content,
  () => {
    if (nearBottom.value) scrollToBottom('auto')
  },
)

onMounted(async () => {
  // 动态获取可用模型列表（不阻塞 UI）
  fetchModels()
  // 启动时尝试恢复最近 session
  await resume()
  // 加载当前 mock 模式（用户主动控制 → action-sheet 切换）
  await loadMockMode()
  // mock 模式开启时 → 拉"全局剧本选择器"覆盖在输入框上方
  // 用户首次进入就能看到 chip，不必先发消息触发流
  if (isMockMode.value) {
    void loadMockPresets()
  }
  nextTick(() => scrollToBottom('auto'))
  // 模型选择器：点击外部关闭下拉
  document.addEventListener('click', handleModelPickerOutsideClick)
  // Task 8: 绑定双指缩放到会话内容容器（mainRef = .agentChatMain）
  // 必须在 nextTick 之后绑定 —— mainRef 在 onMounted 时可能还没渲染
  nextTick(() => {
    if (mainRef.value) {
      pinch.bind(mainRef.value)
    }
  })
})

// 用户在 Settings/其他位置切换 mock 模式后 → 重新拉/清空 chip
// off → 清空（v-if 自然不渲染，无需手动）
// builtin/custom → 拉新选择器覆盖当前 chip
watch(currentMockMode, (newMode, _oldMode) => {
  console.debug('[AgentChat] mock mode changed →', newMode)
  if (newMode === 'builtin' || newMode === 'custom') {
    void loadMockPresets()
  }
  // 'off' 不需要清空 —— isMockMode.value = false → v-if 不渲染
})

onUnmounted(() => {
  document.removeEventListener('click', handleModelPickerOutsideClick)
  // Task 8: 解绑双指缩放事件监听器（避免内存泄漏）
  pinch.unbind()
})

// 暴露给 modal container（可选）
defineExpose({})
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
