<!--
  TDesignChatView.vue - 腾讯 TDesign 视觉风格的 turn-items 渲染器

  修复记录：
  - v1: 用 <ChatList :data="Message[]"> 把整条 m.content 当一个 ChatItem 渲染
       → 文本累积成单块 markdown；tool_calls 全部排在底部 → 用户痛点
  - v2: 改用 useRenderTurnItems(messages, status, compactionText) 拿 RenderedItem[]
       按 eventLog 时间轴逐项渲染（与 Default 引擎同源数据，差异化视觉）
       → 文本段和工具调用交错显示，符合 agent 流式预期
  - v3: 文本段改用 TDesign 自家的 ChatMarkdown 组件（@tdesign-vue-next/chat 内置，
       基于 cherry-markdown 引擎），保持 TDesign 一致的 markdown 视觉。
       外层 .tdesign-chat-view 背景改为 transparent，融入宿主页面（Ionic ion-content）。

  渲染策略：
  1. 文本段：TDesign ChatMarkdown + 套 TDesign 风格气泡
  2. 工具调用：复用 OperationCard（项目内已存在，跨引擎共享） + TDesign 配色
  3. 工具结果：内嵌在对应 OperationCard 的 #result slot
  4. 审批卡片：复用 ApprovalCard + TDesign 风格
  5. 消息 footer：复用时间戳 + 复制按钮，TDesign 风格

  数据源：从 useAgent 通过 EngineRenderProps 拿到的 messages: readonly Message[]
  协议：AG-UI（与 Default 引擎共享同一份数据）

  SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/ Phase 4
-->
<template>
  <div class="tdesign-chat-view" :data-streaming="streaming">
    <!-- 空状态：欢迎文案 -->
    <div v-if="renderedItems.length === 0" class="empty-state">
      <ChatThinking v-if="streaming" :content="'正在思考...'" />
      <div v-else class="welcome">
        <h3>{{ welcomeTitle }}</h3>
        <p>{{ welcomeSubtitle }}</p>
      </div>
    </div>

    <!--
      渲染 RenderedItem 列表（按 eventLog 时间轴交错）:
      - user → user 气泡
      - assistantText → MarkdownStream（避免整块 markdown 显示）
      - operation → OperationCard（带 #result slot 渲染对应 tool_result）
      - messageFooter → 时间戳 + 复制按钮
      - 其他类型 → 通用 TDesign 风格卡片
    -->
    <template v-else>
      <div
        v-for="(item, idx) in renderedItems"
        :key="`${item.messageId}-${idx}`"
        class="renderedItemWrap"
        :data-msg-idx="idx"
      >
        <!-- 用户消息气泡 -->
        <div v-if="item.type === 'user'" class="td-msg-row td-msg-row--user">
          <div class="td-msg-bubble td-msg-bubble--user">
            <span class="td-msg-avatar">🧑</span>
            <div class="td-msg-body">{{ item.text }}</div>
          </div>
        </div>

        <!-- 助手文本段（按 eventLog 切分，单段渲染） -->
        <div
          v-else-if="item.type === 'assistantText'"
          class="td-msg-row td-msg-row--assistant"
          :data-first-in-group="item.firstInGroup ? 'true' : 'false'"
        >
          <div class="td-msg-bubble td-msg-bubble--assistant">
            <span v-if="item.firstInGroup" class="td-msg-avatar">🤖</span>
            <div class="td-msg-body">
              <!--
                TDesign 自家的 ChatMarkdown 组件（cherry-markdown 引擎）。
                相比项目内 MarkdownStream：
                - 样式与 TDesign 主题色系一致
                - 支持 cherry-markdown 的代码高亮、表格、LaTeX 等扩展
                - 通过 :content prop 接收 markdown 字符串
              -->
              <ChatMarkdown
                v-if="item.text"
                :content="item.text"
                class="td-msg-md"
              />
            </div>
          </div>
        </div>

        <!--
          工具调用（按 eventLog 顺序单条） — TDesign 风格 v4 适配：
          - 默认折叠（<details>），head 显示图标+名称+状态（折叠态）→ 不打扰主流程
          - streaming=true 时强制展开（用户需看到实时进度）
          - 展开内容：
            1. 参数：尝试解析 args 为对象 → 渲染为 kv 表；解析失败 → TDesign 配色 pre 块
            2. 结果：尝试解析为 JSON：
                 - mount list（mounts[]）→ 简洁卡片列表
                 - file list（files[]/items[]）→ 简洁卡片列表
                 - file content（content/text）→ 等宽字体块
                 - 普通对象 → 格式化 JSON（带 TDesign 颜色 key/value）
                 - 字符串 → 等宽字体块
            3. 错误：红色 banner
        -->
        <div
          v-else-if="item.type === 'operation' && findToolCallById(item.toolCallId)"
          class="td-msg-row td-msg-row--tool"
        >
          <details
            class="td-tool-card"
            :class="{ 'td-tool-card--open': isOpen(item.toolCallId) }"
            :open="isOpen(item.toolCallId)"
            @toggle="onToolToggle(item.toolCallId, $event)"
          >
            <summary class="td-tool-card-head">
              <span class="td-tool-card-icon">🔧</span>
              <span class="td-tool-card-name">
                {{ findToolCallById(item.toolCallId)!.name }}
              </span>
              <span
                class="td-tool-card-status"
                :data-status="findToolCallById(item.toolCallId)!.status"
              >
                {{ statusText(findToolCallById(item.toolCallId)!.status) }}
              </span>
              <span class="td-tool-card-chevron">▾</span>
            </summary>
            <div class="td-tool-card-body">
              <!-- 参数 -->
              <div v-if="findToolCallById(item.toolCallId)!.args" class="td-tool-card-section">
                <div class="td-tool-card-section-label">参数</div>
                <ToolDetailContent
                  :raw="findToolCallById(item.toolCallId)!.args"
                  kind="args"
                />
              </div>

              <!-- 结果 / 错误 -->
              <div
                v-if="findToolResultById(item.toolCallId)"
                class="td-tool-card-section"
              >
                <div class="td-tool-card-section-label">
                  {{ findToolResultById(item.toolCallId)!.is_error ? '错误' : '结果' }}
                </div>
                <ToolDetailContent
                  v-if="!findToolResultById(item.toolCallId)!.is_error"
                  :raw="findToolResultById(item.toolCallId)!.result"
                  kind="result"
                  :tool-name="findToolCallById(item.toolCallId)!.name"
                />
                <div v-else class="td-tool-card-error">
                  {{ findToolResultById(item.toolCallId)!.result }}
                </div>
              </div>
            </div>
          </details>
        </div>

        <!-- 工具结果独立卡片（备用，目前 OperationCard #result slot 已覆盖） -->
        <div
          v-else-if="item.type === 'toolResultCard'"
          class="td-msg-row td-msg-row--tool-result"
        >
          <div class="td-tool-result-card">
            <div class="td-tool-card-head">
              <span class="td-tool-card-icon">📋</span>
              <span class="td-tool-card-name">{{ item.name }}</span>
            </div>
            <pre class="td-tool-card-result-body">{{ item.result }}</pre>
          </div>
        </div>

        <!-- 消息 footer（时间戳 + 复制） -->
        <div v-else-if="item.type === 'messageFooter'" class="td-msg-footer">
          <span class="td-msg-footer-time">{{ formatFooterTime(item.timestamp) }}</span>
          <button
            v-if="onCopyMessage"
            type="button"
            class="td-msg-footer-copy"
            :title="'复制内容'"
            @click="onCopyMessage(item.messageId)"
          >
            复制
          </button>
        </div>

        <!-- Plan 块（write_todos） -->
        <div v-else-if="item.type === 'plan'" class="td-msg-row td-msg-row--plan">
          <div class="td-plan-card">
            <div class="td-plan-head">📋 计划</div>
            <ol class="td-plan-list">
              <li
                v-for="t in item.todos"
                :key="t.id"
                class="td-plan-item"
                :data-status="t.status"
              >
                <span class="td-plan-status">{{ t.status }}</span>
                <span class="td-plan-content">{{ t.content }}</span>
              </li>
            </ol>
          </div>
        </div>

        <!-- 审批卡片（needsConfirm 工具） -->
        <div v-else-if="item.type === 'approval' && findToolCallById(item.toolCallId)" class="td-msg-row td-msg-row--approval">
          <div class="td-approval-card">
            <div class="td-approval-head">⚠️ 需确认</div>
            <div class="td-approval-body">
              工具 <code>{{ findToolCallById(item.toolCallId)!.name }}</code> 等待授权
            </div>
            <div v-if="onConfirmTool" class="td-approval-actions">
              <button
                class="td-approval-btn td-approval-btn--approve"
                @click="onConfirmTool(item.toolCallId, 'approve')"
              >批准</button>
              <button
                class="td-approval-btn td-approval-btn--reject"
                @click="onConfirmTool(item.toolCallId, 'reject')"
              >拒绝</button>
            </div>
          </div>
        </div>

        <!-- Reasoning（思维链） -->
        <div v-else-if="item.type === 'reasoning'" class="td-msg-row td-msg-row--reasoning">
          <details class="td-reasoning">
            <summary>💭 思维链</summary>
            <pre>{{ item.text }}</pre>
          </details>
        </div>

        <!-- Error -->
        <div v-else-if="item.type === 'error'" class="td-msg-row td-msg-row--error">
          <div class="td-error-card">⚠️ {{ item.text }}</div>
        </div>

        <!-- Compaction（上下文压缩） -->
        <div v-else-if="item.type === 'compaction'" class="td-msg-row td-msg-row--compaction">
          <div class="td-compaction-divider">
            <span>— {{ item.text }} —</span>
          </div>
        </div>

        <!-- Agent Task（subagent 拆解） -->
        <div v-else-if="item.type === 'agentTask'" class="td-msg-row td-msg-row--agent-task">
          <div class="td-agent-task-card">
            <div class="td-agent-task-head">🎯 子任务</div>
            <ul class="td-agent-task-list">
              <li
                v-for="t in item.subTasks"
                :key="t.id"
                :data-status="t.status"
              >{{ t.description }}</li>
            </ul>
          </div>
        </div>

        <!-- WebSearch 组合 / OperationGroup（多 tool 一起）-->
        <div
          v-else-if="item.type === 'webSearchGroup' || item.type === 'operationGroup'"
          class="td-msg-row td-msg-row--tool-group"
        >
          <div class="td-tool-group-card">
            <div v-if="item.type === 'webSearchGroup'" class="td-tool-group-head">🔍 搜索</div>
            <div v-else class="td-tool-group-head">🔧 操作组</div>
            <div
              v-for="tcid in ('toolCallIds' in item ? item.toolCallIds : [])"
              :key="tcid"
              class="td-tool-group-item"
            >
              <span v-if="findToolCallById(tcid)" class="td-tool-card-name">
                {{ findToolCallById(tcid)!.name }}
              </span>
              <span
                v-if="findToolCallById(tcid)"
                class="td-tool-card-status"
                :data-status="findToolCallById(tcid)!.status"
              >
                {{ statusText(findToolCallById(tcid)!.status) }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- 流式时尾部显示 thinking 指示器（覆盖在列表底部） -->
    <ChatThinking
      v-if="streaming && renderedItems.length > 0"
      class="streaming-thinking"
      :content="'正在思考...'"
    />
  </div>
</template>

<script setup lang="ts">
// TDesign Chat 自家组件：ChatThinking + ChatMarkdown（cherry-markdown 引擎）
// 统一 TDesign 视觉风格

import { useRenderTurnItems } from "@/composables/renderTurnItems";
import type { AgentStatus, Message, ToolCall, ToolResult } from "@/composables/useAgent";
import { type ComputedRef, computed, ref } from "vue";

/**
 * 适配 TDesign 视觉的 turn-items 渲染器
 * 接收 messages: readonly Message[]，使用 useRenderTurnItems
 * 按 eventLog 时间轴逐项渲染（与 Default 引擎同源）
 */

interface Props {
  messages: readonly Message[];
  status: AgentStatus | string;
  streaming: boolean;
  onSend?: (text: string) => Promise<void>;
  onStop?: () => void;
  onConfirmTool?: (toolCallId: string, decision: string) => Promise<void>;
  onCopyMessage?: (messageId: string) => Promise<void>;
  onPresetClick?: (userText: string) => void;
}

const props = withDefaults(defineProps<Props>(), {
  messages: () => [] as readonly Message[],
});

const welcomeTitle = "TDesign 风格引擎";
const welcomeSubtitle = "使用腾讯 TDesign 视觉组件渲染 Agent 对话。";

/**
 * 按 eventLog 时间轴拆分 messages 为 RenderedItem[]。
 * 与 Default 引擎（DefaultMessagesView）共用同一份拆分逻辑，保证：
 * 1. 文本段和工具调用交错（不会出现"全文 markdown + 底部工具栏"）
 * 2. 同一份 message 跨多次 text_delta 按 \n\n 切段
 * 3. plan / approval / webSearch 走特殊路径
 *
 * SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/ Phase 4
 */
const messagesRef = computed(() => [...props.messages]) as ComputedRef<Message[]>;
const statusRef = computed(() => props.status as AgentStatus);
const renderedItems = useRenderTurnItems(messagesRef, statusRef);

/** O(1) 工具调用查找：跨 messages 全局 id 索引 */
const allToolCalls = computed<Map<string, ToolCall>>(() => {
  const m = new Map<string, ToolCall>();
  for (const msg of props.messages) {
    if (!msg.tool_calls) continue;
    for (const tc of msg.tool_calls) {
      m.set(tc.id, tc);
    }
  }
  return m;
});

/** O(1) 工具结果查找 */
const allToolResults = computed<Map<string, ToolResult>>(() => {
  const m = new Map<string, ToolResult>();
  for (const msg of props.messages) {
    if (!msg.tool_results) continue;
    for (const tr of msg.tool_results) {
      m.set(tr.id, tr);
    }
  }
  return m;
});

function findToolCallById(id: string): ToolCall | undefined {
  return allToolCalls.value.get(id);
}

function findToolResultById(id: string): ToolResult | undefined {
  return allToolResults.value.get(id);
}

function statusText(status: string | undefined): string {
  switch (status) {
    case "pending":
      return "待执行";
    case "running":
      return "执行中...";
    case "success":
      return "完成";
    case "error":
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    default:
      return status || "";
  }
}

function formatFooterTime(timestamp: number): string {
  const d = new Date(timestamp);
  const pad = (n: number) => n.toString().padStart(2, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// ── 工具卡片展开状态管理 ─────────────────────────────────
// 规则：
//   - 用户手动点击 → 记住状态（即使切到其他 message）
//   - 初始状态：running/pending → 自动展开；success/error → 默认折叠
//   - 一个 set 存用户主动展开的 ids；一个 set 存用户主动折叠的 ids
//     （避免被自动规则覆盖）
const userExpandedIds = ref(new Set<string>());
const userCollapsedIds = ref(new Set<string>());

/** 当前 id 是否展开 */
function isOpen(toolCallId: string): boolean {
  if (userExpandedIds.value.has(toolCallId)) return true;
  if (userCollapsedIds.value.has(toolCallId)) return false;
  // 默认规则：running/pending → 展开
  const tc = findToolCallById(toolCallId);
  if (!tc) return false;
  return tc.status === "running" || tc.status === "pending";
}

/**
 * <details> @toggle 事件 → 同步用户意图到 ref Set
 * 注意：组件挂载时 <details :open="isOpen"> 会触发一次 toggle event，
 * 此时要避免把"自动展开"误记为"用户主动展开"。
 * 解决：检查 event.target.open 与 isOpen(id) 一致才记为用户操作。
 */
function onToolToggle(toolCallId: string, e: Event) {
  const target = e.target as HTMLDetailsElement;
  if (!target) return;
  // 首次挂载导致的 toggle 不记
  // （依靠 userExpandedIds/userCollapsedIds 初始为空来过滤）
  if (target.open) {
    userCollapsedIds.value.delete(toolCallId);
    userExpandedIds.value.add(toolCallId);
  } else {
    userExpandedIds.value.delete(toolCallId);
    userCollapsedIds.value.add(toolCallId);
  }
}
</script>

<style scoped>
.tdesign-chat-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  height: 100%;
  overflow-y: auto;
  /* v3 fix: 背景透明，融入宿主页面（Ionic ion-content 默认有主题背景色）。
     之前用 --td-bg-color-page (#f7f7f7) 在暗黑模式下会盖住整个聊天区。 */
  background: transparent;
}

.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-secondary, #666);
  text-align: center;
}
.empty-state .welcome h3 {
  margin: 0 0 8px 0;
  color: var(--td-brand-color, #4f8cff);
}
.empty-state .welcome p {
  margin: 0;
  color: var(--td-text-color-secondary, #999);
}

.streaming-thinking {
  align-self: flex-start;
  margin-top: 8px;
}

/* ── 通用消息行（user / assistant）── */
.td-msg-row {
  display: flex;
  width: 100%;
  margin-bottom: 8px;
}
.td-msg-row--user { justify-content: flex-end; }
.td-msg-row--assistant { justify-content: flex-start; }
.td-msg-row--tool,
.td-msg-row--tool-result,
.td-msg-row--plan,
.td-msg-row--approval,
.td-msg-row--reasoning,
.td-msg-row--error,
.td-msg-row--compaction,
.td-msg-row--agent-task,
.td-msg-row--tool-group {
  justify-content: stretch;
}

.td-msg-bubble {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  max-width: 85%;
  padding: 10px 14px;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  word-wrap: break-word;
}
.td-msg-bubble--user {
  background: var(--td-brand-color, #4f8cff);
  color: #fff;
  border-bottom-right-radius: 4px;
}
.td-msg-bubble--assistant {
  background: var(--td-bg-color-container, #fff);
  color: var(--td-text-color-primary, #333);
  border-bottom-left-radius: 4px;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
}
.td-msg-avatar {
  font-size: 18px;
  flex-shrink: 0;
}
.td-msg-body {
  flex: 1;
  min-width: 0;
}
/* TDesign ChatMarkdown 容器：背景透明，无内边距（由气泡承担） */
.td-msg-body :deep(.td-msg-md) {
  background: transparent;
}
.td-msg-body :deep(p) {
  margin: 0 0 6px 0;
}
.td-msg-body :deep(p:last-child) {
  margin-bottom: 0;
}

/* ── 工具调用卡片（按 eventLog 顺序单条渲染，不再堆在底部）── */
/* v4: 改为 <details> 折叠态 + TDesign 风格 */
.td-tool-card {
  width: 100%;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  overflow: hidden;
  transition: border-color 0.2s, box-shadow 0.2s;
}
.td-tool-card[open] {
  border-color: rgba(var(--td-brand-color-rgb, 79, 140, 255), 0.4);
  box-shadow: 0 2px 8px rgba(var(--td-brand-color-rgb, 79, 140, 255), 0.12);
}
.td-tool-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 10px 12px;
  cursor: pointer;
  user-select: none;
  -webkit-tap-highlight-color: transparent;
  list-style: none;
  outline: none;
}
.td-tool-card-head::-webkit-details-marker {
  display: none;
}
.td-tool-card-head::marker {
  content: '';
}
.td-tool-card-head:hover {
  background: rgba(var(--td-brand-color-rgb, 79, 140, 255), 0.04);
}
.td-tool-card-icon { font-size: 14px; }
.td-tool-card-name {
  font-weight: 500;
  color: var(--td-text-color-primary, #333);
  font-size: 13px;
  font-family: 'SF Mono', Monaco, monospace;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.td-tool-card-status {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  background: var(--td-bg-color-secondarycomponent, #f3f3f3);
  color: var(--td-text-color-secondary, #666);
  flex-shrink: 0;
}
.td-tool-card-status[data-status='running'] {
  background: var(--td-brand-color, #4f8cff);
  color: #fff;
}
.td-tool-card-status[data-status='success'] {
  background: var(--td-success-color, #2ba471);
  color: #fff;
}
.td-tool-card-status[data-status='error'],
.td-tool-card-status[data-status='failed'] {
  background: var(--td-error-color, #d54941);
  color: #fff;
}
.td-tool-card-chevron {
  font-size: 12px;
  color: var(--td-text-color-secondary, #999);
  flex-shrink: 0;
  transition: transform 0.2s ease;
  display: inline-block;
  margin-left: 2px;
}
.td-tool-card[open] .td-tool-card-chevron {
  transform: rotate(180deg);
  color: var(--td-brand-color, #4f8cff);
}
.td-tool-card-body {
  padding: 0 12px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  border-top: 1px dashed var(--td-component-stroke, #e7e7e7);
  padding-top: 10px;
}
.td-tool-card-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.td-tool-card-section-label {
  font-size: 10px;
  color: var(--td-text-color-secondary, #999);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}
.td-tool-card-error {
  padding: 6px 8px;
  background: var(--td-error-color-light, #fff1f0);
  border: 1px solid var(--td-error-color, #d54941);
  color: var(--td-error-color, #d54941);
  border-radius: 4px;
  font-size: 12px;
  font-family: 'SF Mono', Monaco, monospace;
  white-space: pre-wrap;
  word-break: break-word;
}

/* ── 工具结果独立卡片（备用）── */
.td-tool-result-card {
  width: 100%;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  padding: 10px 12px;
}

/* ── Plan 块 ── */
.td-plan-card {
  width: 100%;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  padding: 10px 14px;
}
.td-plan-head {
  font-weight: 500;
  color: var(--td-brand-color, #4f8cff);
  margin-bottom: 8px;
}
.td-plan-list {
  margin: 0;
  padding-left: 20px;
}
.td-plan-item {
  margin-bottom: 4px;
  font-size: 13px;
}
.td-plan-status {
  display: inline-block;
  padding: 0 6px;
  margin-right: 6px;
  background: var(--td-bg-color-secondarycomponent, #f3f3f3);
  color: var(--td-text-color-secondary, #666);
  font-size: 11px;
  border-radius: 3px;
}

/* ── 审批卡片 ── */
.td-approval-card {
  width: 100%;
  background: var(--td-warning-color-light, #fff7e8);
  border: 1px solid var(--td-warning-color, #e37318);
  border-radius: 8px;
  padding: 10px 14px;
}
.td-approval-head {
  font-weight: 500;
  color: var(--td-warning-color, #e37318);
  margin-bottom: 8px;
}
.td-approval-body {
  font-size: 13px;
  color: var(--td-text-color-primary, #333);
  margin-bottom: 10px;
}
.td-approval-body code {
  background: var(--td-bg-color-secondarycomponent, #f3f3f3);
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 12px;
}
.td-approval-actions {
  display: flex;
  gap: 8px;
}
.td-approval-btn {
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 12px;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  background: #fff;
  cursor: pointer;
}
.td-approval-btn--approve {
  background: var(--td-success-color, #2ba471);
  color: #fff;
  border-color: var(--td-success-color, #2ba471);
}
.td-approval-btn--reject {
  background: var(--td-error-color, #d54941);
  color: #fff;
  border-color: var(--td-error-color, #d54941);
}

/* ── Reasoning ── */
.td-reasoning {
  width: 100%;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
}
.td-reasoning summary {
  cursor: pointer;
  color: var(--td-text-color-secondary, #666);
  font-weight: 500;
}
.td-reasoning pre {
  margin: 8px 0 0 0;
  white-space: pre-wrap;
  word-break: break-word;
}

/* ── Error ── */
.td-error-card {
  width: 100%;
  background: var(--td-error-color-light, #fff1f0);
  border: 1px solid var(--td-error-color, #d54941);
  color: var(--td-error-color, #d54941);
  border-radius: 8px;
  padding: 10px 14px;
  font-size: 13px;
}

/* ── Compaction ── */
.td-compaction-divider {
  width: 100%;
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-secondary, #999);
  padding: 8px 0;
  border-top: 1px dashed var(--td-component-stroke, #e7e7e7);
  border-bottom: 1px dashed var(--td-component-stroke, #e7e7e7);
}

/* ── Agent Task ── */
.td-agent-task-card {
  width: 100%;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  padding: 10px 14px;
}
.td-agent-task-head {
  font-weight: 500;
  color: var(--td-brand-color, #4f8cff);
  margin-bottom: 8px;
}
.td-agent-task-list {
  margin: 0;
  padding-left: 20px;
  font-size: 13px;
}

/* ── Tool group (webSearch / operationGroup) ── */
.td-tool-group-card {
  width: 100%;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  padding: 10px 12px;
}
.td-tool-group-head {
  font-weight: 500;
  color: var(--td-brand-color, #4f8cff);
  margin-bottom: 6px;
}
.td-tool-group-item {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
  font-size: 13px;
}

/* ── Footer ── */
.td-msg-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 4px 36px;
  font-size: 11px;
  color: var(--td-text-color-secondary, #999);
}
.td-msg-footer-copy {
  background: transparent;
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 3px;
  padding: 1px 6px;
  font-size: 11px;
  color: var(--td-text-color-secondary, #666);
  cursor: pointer;
}
.td-msg-footer-copy:hover {
  background: var(--td-bg-color-secondarycomponent, #f3f3f3);
}
</style>
