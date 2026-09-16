<!--
  DefaultMessagesView - 默认引擎的消息列表视图组件

  从 AgentChat.vue 的 <main class="agentChatMain"> 区域提取，
  作为 ChatEngine.renderMessages() 的渲染目标。

  接收 EngineRenderProps，内部使用 useRenderTurnItems 计算渲染项，
  包含完整的消息类型分发（user / assistantText / messageFooter / approval /
  operationGroup / operation / webSearchGroup / plan / reasoning / error /
  compaction / agentTask）和虚拟滚动分支（≤120 原生 v-for / >120 MessageVirtualList）。
-->
<template>
  <main class="agentChatMain" ref="mainRef" @scroll="onMainScroll">
    <!-- 空状态（无消息时显示） -->
    <div v-if="renderedItems.length === 0" class="agentChatEmpty">
      <ion-icon :icon="chatbubblesIcon" class="emptyIcon" />
      <p>{{ t('agent.emptyHint') }}</p>
    </div>
    <!-- 短会话（≤ 120）：原生 v-for（无虚拟化开销） -->
    <template v-else-if="renderedItems.length <= VIRTUAL_LIST_THRESHOLD">
      <div
        v-for="(item, idx) in renderedItems"
        :key="item.messageId"
        class="renderedItemWrap"
        :data-msg-idx="idx"
      >
        <UserMessageBubble
          v-if="item.type === 'user'"
          :text="item.text"
        />
        <AssistantMessage
          v-else-if="item.type === 'assistantText'"
          :text="item.text"
          :streaming="item.streaming"
          :status="status"
          :compact="!item.firstInGroup"
          :show-footer="false"
        />
        <!-- 独立 Footer 段：时间戳固定，不依赖末段类型 -->
        <div
          v-else-if="item.type === 'messageFooter'"
          class="messageFooterStandalone"
        >
          <span class="footerTimestamp">{{ formatFooterTime(item.timestamp) }}</span>
          <button
            type="button"
            class="footerCopyBtn"
            :title="'复制内容'"
            @click="copyMessageContent(item.messageId)"
          >
            <ion-icon :icon="copyIconVar" />
          </button>
        </div>
        <ApprovalCard
          v-else-if="item.type === 'approval'"
          :tool-call="findToolCall(item.toolCallId)!"
          :on-decide="handleDecide"
          :is-processing="status === 'streaming'"
        />
        <GroupedOperationMessage
          v-else-if="item.type === 'operationGroup'"
          :items="resolveToolCalls(item.toolCallIds)"
          :results-by-call-id="resolveToolResultsByCallId(item.toolCallIds)"
          :force-complete="item.forceComplete"
        />
        <!-- Task 27：agent 流式时间轴模式 —— 单条工具调用卡片 -->
        <OperationCard
          v-else-if="item.type === 'operation' && findToolCallById(item.toolCallId)"
          :tool-call="findToolCallById(item.toolCallId)!"
          :streaming="item.streaming"
        >
          <!-- toolResultCard 紧跟在 operation 后面（由 renderAgentFlow 保证顺序） -->
          <template v-if="findToolResultById(item.toolCallId)" #result>
            <MountListCard
              v-if="findToolResultById(item.toolCallId)!.name === 'list_mounts'"
              :result-json="findToolResultById(item.toolCallId)!.result"
            />
            <FileListCard
              v-else-if="findToolResultById(item.toolCallId)?.name === 'list_files' || findToolResultById(item.toolCallId)?.name === 'stat_file'"
              :result-json="findToolResultById(item.toolCallId)!.result"
            />
            <FileContentCard
              v-else-if="findToolResultById(item.toolCallId)?.name === 'read_file'"
              :result-json="findToolResultById(item.toolCallId)!.result"
            />
          </template>
        </OperationCard>
        <WebSearchSummaryMessage
          v-else-if="item.type === 'webSearchGroup'"
          :queries="item.queries"
          :tool-calls="resolveToolCalls(item.toolCallIds)"
        />
        <PlanBlock
          v-else-if="item.type === 'plan'"
          :todos="item.todos"
          :streaming="item.streaming"
        />
        <ReasoningMessage
          v-else-if="item.type === 'reasoning'"
          :text="item.text"
          :streaming="item.streaming"
        />
        <ErrorMessage
          v-else-if="item.type === 'error'"
          :text="item.text"
          :on-retry="() => handleRetryError(item)"
        />
        <!-- Task 7：上下文自动压缩分隔线（不可展开） -->
        <ContextCompactionDivider
          v-else-if="item.type === 'compaction'"
          :text="item.text"
        />
        <!-- Task 22: agent task 消息（subagent 拆解的子任务列表） -->
        <AgentTaskMessage
          v-else-if="item.type === 'agentTask'"
          :sub-tasks="item.subTasks"
          :reasoning="item.reasoning"
        />
      </div>
    </template>
    <!-- 长会话（> 120）：虚拟滚动优化 -->
    <MessageVirtualList
      v-else
      ref="virtualListRef"
      :items="renderedItems"
    >
      <template #item="{ item }">
        <div class="renderedItemWrap">
          <UserMessageBubble
            v-if="item.type === 'user'"
            :text="item.text"
          />
          <AssistantMessage
            v-else-if="item.type === 'assistantText'"
            :text="item.text"
            :streaming="item.streaming"
            :status="status"
            :compact="!item.firstInGroup"
            :show-footer="false"
          />
          <!-- 独立 Footer 段（虚拟滚动分支） -->
          <div
            v-else-if="item.type === 'messageFooter'"
            class="messageFooterStandalone"
          >
            <span class="footerTimestamp">{{ formatFooterTime(item.timestamp) }}</span>
            <button
              type="button"
              class="footerCopyBtn"
              :title="'复制内容'"
              @click="copyMessageContent(item.messageId)"
            >
              <ion-icon :icon="copyIconVar" />
            </button>
          </div>
          <ApprovalCard
            v-else-if="item.type === 'approval'"
            :tool-call="findToolCall(item.toolCallId)!"
            :on-decide="handleDecide"
            :is-processing="status === 'streaming'"
          />
          <GroupedOperationMessage
            v-else-if="item.type === 'operationGroup'"
            :items="resolveToolCalls(item.toolCallIds)"
            :results-by-call-id="resolveToolResultsByCallId(item.toolCallIds)"
            :force-complete="item.forceComplete"
          />
          <!-- Task 27：agent 流式时间轴模式 —— 单条工具调用卡片（虚拟滚动分支） -->
          <OperationCard
            v-else-if="item.type === 'operation' && findToolCallById(item.toolCallId)"
            :tool-call="findToolCallById(item.toolCallId)!"
            :streaming="item.streaming"
          >
            <template v-if="findToolResultById(item.toolCallId)" #result>
              <MountListCard
                v-if="findToolResultById(item.toolCallId)!.name === 'list_mounts'"
                :result-json="findToolResultById(item.toolCallId)!.result"
              />
              <FileListCard
                v-else-if="findToolResultById(item.toolCallId)?.name === 'list_files' || findToolResultById(item.toolCallId)?.name === 'stat_file'"
                :result-json="findToolResultById(item.toolCallId)!.result"
              />
              <FileContentCard
                v-else-if="findToolResultById(item.toolCallId)?.name === 'read_file'"
                :result-json="findToolResultById(item.toolCallId)!.result"
              />
            </template>
          </OperationCard>
          <WebSearchSummaryMessage
            v-else-if="item.type === 'webSearchGroup'"
            :queries="item.queries"
            :tool-calls="resolveToolCalls(item.toolCallIds)"
          />
          <PlanBlock
            v-else-if="item.type === 'plan'"
            :todos="item.todos"
            :streaming="item.streaming"
          />
          <ReasoningMessage
            v-else-if="item.type === 'reasoning'"
            :text="item.text"
            :streaming="item.streaming"
          />
          <ErrorMessage
            v-else-if="item.type === 'error'"
            :text="item.text"
            :on-retry="() => handleRetryError(item)"
          />
          <!-- Task 7：虚拟滚动分支同样渲染 ContextCompactionDivider -->
          <ContextCompactionDivider
            v-else-if="item.type === 'compaction'"
            :text="item.text"
          />
          <!-- Task 22: 虚拟滚动分支同样渲染 AgentTaskMessage -->
          <AgentTaskMessage
            v-else-if="item.type === 'agentTask'"
            :sub-tasks="item.subTasks"
            :reasoning="item.reasoning"
          />
        </div>
      </template>
    </MessageVirtualList>
  </main>
</template>

<script setup lang="ts">
import { chatbubblesOutline, copyOutline } from "ionicons/icons";
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef, watch } from "vue";
import ApprovalCard from "@/components/agent/ApprovalCard.vue";
import AssistantMessage from "@/components/agent/AssistantMessage.vue";
import FileContentCard from "@/components/agent/FileContentCard.vue";
import FileListCard from "@/components/agent/FileListCard.vue";
import GroupedOperationMessage from "@/components/agent/GroupedOperationMessage.vue";
import MountListCard from "@/components/agent/MountListCard.vue";
import OperationCard from "@/components/agent/OperationCard.vue";
import UserMessageBubble from "@/components/agent/UserMessageBubble.vue";
import type { EngineRenderProps } from "@/composables/chatEngine";
import { useRenderTurnItems } from "@/composables/renderTurnItems";
import type { Decision, Message, ToolCall, ToolResult } from "@/composables/useAgent";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";

const props = defineProps<EngineRenderProps>();
const { t } = useI18n();

// ── 图标常量 ──────────────────────────────────────────────
const chatbubblesIcon = chatbubblesOutline;
const copyIconVar = copyOutline;

// ── 内部状态 ──────────────────────────────────────────────
/** 把 readonly Message[] 包装为 shallowRef，供 useRenderTurnItems 和辅助方法使用 */
const messagesRef = shallowRef<Message[]>([...props.messages]);
const mainRef = ref<HTMLDivElement | null>(null);
const virtualListRef = ref<{ scrollToBottom: (behavior?: "auto" | "smooth") => void } | null>(null);
const nearBottom = ref(true);

/** 触发虚拟滚动的阈值（renderedItems 数量 > 此值时切换） */
const VIRTUAL_LIST_THRESHOLD = 120;

// 同步外部 messages 变化到内部 ref
watch(
  () => props.messages,
  newMessages => {
    messagesRef.value = [...newMessages];
  },
  { deep: true }
);

// ── 渲染项计算 ────────────────────────────────────────────
const compactionText = computed(() => t("agent.contextCompaction"));
const renderedItems = useRenderTurnItems(
  computed(() => messagesRef.value),
  computed(() => props.status),
  compactionText
);

// ── 工具调用 / 结果查找 ───────────────────────────────────
function findToolCall(id: string): ToolCall | null {
  for (const msg of messagesRef.value) {
    const tc = msg.tool_calls.find((t: ToolCall) => t.id === id);
    if (tc) return tc;
  }
  return null;
}

function findToolResult(id: string): ToolResult | null {
  for (const msg of messagesRef.value) {
    const tr = msg.tool_results.find((r: ToolResult) => r.id === id);
    if (tr) return tr;
  }
  return null;
}

function findToolCallById(id: string): ToolCall | null {
  return findToolCall(id);
}
function findToolResultById(id: string): ToolResult | null {
  return findToolResult(id);
}

/** 格式化 Footer 固定时间戳为 HH:mm */
function formatFooterTime(timestamp: number): string {
  const d = new Date(timestamp);
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${hh}:${mm}`;
}

/** 复制 messageFooter 对应消息的全文内容 */
async function copyMessageContent(messageId: string): Promise<void> {
  const idx = parseInt(messageId.replace(/^[au]-/, ""), 10);
  const msg = messagesRef.value[idx];
  if (!msg?.content) return;
  const text = typeof msg.content === "string" ? msg.content : "";
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.left = "-9999px";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    showToast({ message: "已复制", duration: 1200, color: "success" });
  } catch {
    showToast({ message: "复制失败", duration: 1600, color: "danger" });
  }
}

function resolveToolCalls(ids: string[]): ToolCall[] {
  const out: ToolCall[] = [];
  for (const id of ids) {
    const tc = findToolCall(id);
    if (tc) out.push(tc);
  }
  return out;
}

/** 按 id 查 tool result，构造成 name→Result 的 record 给结构化卡片用 */
function resolveToolResultsByCallId(ids: string[]): Record<string, ToolResult> {
  const out: Record<string, ToolResult> = {};
  for (const id of ids) {
    const tr = findToolResult(id);
    if (tr) out[id] = tr;
  }
  return out;
}

// ── 事件处理 ──────────────────────────────────────────────
function handleDecide(toolCallId: string, decision: Decision) {
  props.onConfirmTool(toolCallId, decision);
}

/**
 * 重试一条出错的消息：清除 error 标记 + 删除关联的 assistant 消息 + 重新发送
 */
function handleRetryError(item: { type: "error"; messageIndex: number }) {
  const idx = item.messageIndex;
  if (idx < 0 || idx >= messagesRef.value.length) return;

  const targetMsg = messagesRef.value[idx];
  if (targetMsg?.role !== "user") return;

  let text = "";
  if (typeof targetMsg.content === "string") {
    text = targetMsg.content;
  } else {
    for (const part of targetMsg.content) {
      if (part.type === "text") {
        text += part.text;
      }
    }
    text = text.trim();
  }

  delete targetMsg.error;
  messagesRef.value.splice(idx);
  props.onSend(text);
  nextTick(() => scrollToBottom());
}

// ── 滚动管理 ──────────────────────────────────────────────
function scrollToBottom(behavior: "auto" | "smooth" = "smooth") {
  nextTick(() => {
    if (renderedItems.value.length > VIRTUAL_LIST_THRESHOLD && virtualListRef.value) {
      virtualListRef.value.scrollToBottom(behavior);
      return;
    }
    const el = mainRef.value;
    if (!el) return;
    el.scrollTo({ top: el.scrollHeight, behavior });
  });
}

function onMainScroll() {
  const el = mainRef.value;
  if (!el) return;
  const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
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

const activeMessageIndex = ref(0);

watch(renderedItems, () => nextTick(setupDotObserver), { flush: "post" });
onMounted(() => nextTick(setupDotObserver));
onUnmounted(cleanupDotObserver);

// 监听 status 变化 → streaming 开始时滚动到底部
watch(
  () => props.status,
  newStatus => {
    if (newStatus === "streaming") {
      scrollToBottom();
    }
  }
);

// 监听 messages 变化 → 接近底部时自动滚
watch(
  () => messagesRef.value.length,
  () => {
    if (nearBottom.value) scrollToBottom();
  }
);

watch(
  () => messagesRef.value[messagesRef.value.length - 1]?.content,
  () => {
    if (nearBottom.value) scrollToBottom("auto");
  }
);

defineExpose({
  scrollToBottom,
});
</script>

<style scoped>
.agentChatMain {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px 12px 12px 36px;
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
  color: color-mix(in srgb, var(--color-primary) 30%, transparent);
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
  border-top: 1px solid color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 12%, transparent);
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
  color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
}
</style>
