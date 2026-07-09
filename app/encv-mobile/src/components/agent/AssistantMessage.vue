<!--
  AssistantMessage - AI 回复文本块
  参照 codex_web assistantMessage / plainAssistantMessage：
  - 无背景气泡（纯文本，与页面背景融合）
  - MessageAuthor(28px 圆形头像, label, meta)
  - MarkdownStream(source, streaming)
  - markdownBody: 14px / line-height 1.62 / 正确间距
  - 底部栏：时间戳（左）+ 复制按钮（右）
-->
<template>
  <div class="assistantMessage" :class="{ assistantMessage_compact: compact }" ref="msgRef">
    <MessageAuthor
      v-if="!compact"
      :icon="icon"
      :label="label"
      :meta="meta"
      :variant="streaming ? 'streaming' : 'default'"
    />
    <div class="assistantMessageBody">
      <MarkdownStream :content="text" :streaming="streaming" />
    </div>
    <!-- 底部栏：时间戳 + 复制（仅 showFooter=true 且非流式时显示） -->
    <div v-if="showFooter && !streaming" class="assistantMessageFooter">
      <span class="footerTimestamp">{{ displayTime }}</span>
      <button
        type="button"
        class="footerCopyBtn"
        :title="'复制内容'"
        :aria-label="'复制消息内容'"
        @click="handleCopy"
      >
        <ion-icon :icon="copyIconVar" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { AgentStatus } from "@/composables/useAgent";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import MessageAuthor from "@/components/agent/MessageAuthor.vue";
import MarkdownStream from "markstream-vue";
import { copyOutline, sparklesOutline } from "ionicons/icons";
import { computed } from "vue";

const props = defineProps<{
  text: string;
  streaming: boolean;
  status?: AgentStatus;
  /** 时间戳（Unix ms），不传则用当前时间 */
  timestamp?: number;
  /**
   * 紧凑模式：隐藏头像/名字。
   * 用于 agent 时间轴模式——同轮消息只有第一个 text 段显示完整 header，
   * 后续 text 段用 compact=true 只渲染 markdown body。
   */
  compact?: boolean;
  /**
   * 强制显示 footer（时间戳+复制按钮）。
   * 用于 agent 时间轴模式——只有最后一个 text 段显示 footer，
   * 即使在 compact 模式下也展示。
   */
  showFooter?: boolean;
}>();

const { t } = useI18n();
const icon = sparklesOutline;
const copyIconVar = copyOutline;

const label = computed(() => "AI 助手");
const meta = computed(() => {
  if (props.streaming) return t("agent.thinking");
  return "";
});

/** 格式化时间戳为 HH:mm */
const displayTime = computed(() => {
  const ts = props.timestamp ?? Date.now();
  const d = new Date(ts);
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${hh}:${mm}`;
});

/** 复制全文到剪贴板 */
async function handleCopy() {
  try {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
      await navigator.clipboard.writeText(props.text);
    } else {
      // Fallback：临时 textarea + execCommand
      const ta = document.createElement("textarea");
      ta.value = props.text;
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
</script>

<style scoped>
/* ── 参照 codex_web .assistantMessage ─────────────────────── */
.assistantMessage {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 18px;
  max-width: 100%;
}

/* 紧凑模式：agent 时间轴后续 text 段——无头像/无 footer / 更紧凑 */
.assistantMessage_compact {
  margin-bottom: 6px;
}

.assistantMessage_compact .assistantMessageBody {
  padding-left: 0;
}

.assistantMessageBody {
  padding-left: 36px; /* 28px avatar + 8px gap */
  min-width: 0;
}

/* ── markdownBody：参照 codex_web .markdownBody ──────────── */
.assistantMessageBody :deep(.markdownStream) {
  display: block;
  max-width: 100%;
  color: var(--ion-text-color);
  font-size: 14px;
  line-height: 1.62;
  overflow-wrap: break-word;
}

/* 段落/列表/代码块间距（codex_web: margin 0 0 12px） */
.assistantMessageBody :deep(.markdownStream) :deep(.node-slot) {
  margin-bottom: 12px;
}

.assistantMessageBody :deep(.markdownStream) :deep(.node-slot:last-child) {
  margin-bottom: 0;
}

/* 行内代码：圆角 + 浅灰底（codex_web 风格） */
.assistantMessageBody :deep(.markdownStream) :deep(.inline-code) {
  border-radius: 5px;
  padding: 1px 5px;
  background: var(--ion-color-light);
  font-size: 0.9em;
}

/* 打字光标动画（流式期间显示） */
.assistantMessageBody :deep(.markdownStream_streaming)::after {
  content: '';
  display: inline-block;
  width: 2px;
  height: 1em;
  background: var(--ion-color-primary);
  margin-left: 2px;
  vertical-align: text-bottom;
  animation: cursorBlink 1s step-end infinite;
  border-radius: 1px;
}

@keyframes cursorBlink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

/* ── 底部栏：时间戳 + 复制 ─────────────────────────────── */
.assistantMessageFooter {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-left: 36px;
  margin-top: 4px;
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
  padding-top: 4px;
}

.footerTimestamp {
  font-size: 11px;
  color: var(--encv-text-secondary, rgba(127,127,127,0.6));
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
  user-select: none;
}

.footerCopyBtn {
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

.footerCopyBtn:hover,
.footerCopyBtn:active {
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}
</style>
