<!--
  UserMessageBubble - 用户消息气泡
  参照 codex_web .userMessage / .userMessageEditor：
  - 右对齐 + 浅灰背景（#f0f1f3）+ 圆角 18px
  - 长消息自动折叠（>560 字符 或 >9 行）
  - 纯文本渲染（不解析 Markdown）
-->
<template>
  <div class="userBubble">
    <div class="userBubbleText" :class="{ userBubbleText_collapsed: shouldCollapse && !expanded }">
      <pre class="userBubblePre">{{ text }}</pre>
    </div>
    <button
      v-if="shouldCollapse"
      type="button"
      class="userBubbleToggle"
      @click="expanded = !expanded"
    >
      {{ expanded ? t('agent.collapse') : t('agent.expand') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "@/composables/useI18n";
import { computed, ref } from "vue";

const props = defineProps<{
  text: string;
}>();

const { t } = useI18n();
const _expanded = ref(false);

const CHAR_LIMIT = 560;
const LINE_LIMIT = 9;

const charCount = computed(() => props.text.length);
const lineCount = computed(() => props.text.split("\n").length);

const _shouldCollapse = computed(() => charCount.value > CHAR_LIMIT || lineCount.value > LINE_LIMIT);
</script>

<style scoped>
/* ── 参照 codex_web .userMessage（右对齐）──────────────────── */
.userBubble {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  margin: 8px 0 10px;
  max-width: 100%;
}

/* 参照 codex_web .userMessageEditor: 浅灰底 + 大圆角 */
.userBubbleText {
  max-width: min(72%, 620px);
  min-width: min(100%, 280px);
  padding: 10px 14px 8px;
  background: #f0f1f3;
  color: #111827;
  border-radius: 18px;
  font-size: 14px;
  line-height: 1.48;
  word-break: break-word;
  overflow-wrap: anywhere;
}

/* 暗黑模式适配 */
@media (prefers-color-scheme: dark) {
  .userBubbleText {
    background: rgba(var(--ion-color-medium-rgb), 0.15);
    color: var(--ion-text-color);
  }
}

.userBubbleText_collapsed {
  max-height: 200px;
  overflow: hidden;
  position: relative;
  -webkit-mask-image: linear-gradient(to bottom, black 70%, transparent 100%);
  mask-image: linear-gradient(to bottom, black 70%, transparent 100%);
}

.userBubblePre {
  margin: 0;
  font-family: inherit;
  font-size: inherit;
  white-space: pre-wrap;
  word-break: break-word;
}

.userBubbleToggle {
  margin-top: 4px;
  background: transparent;
  border: 0;
  padding: 2px 4px;
  font-size: 11.5px;
  color: var(--ion-color-primary);
  cursor: pointer;
  font-weight: 500;
}

.userBubbleToggle:hover {
  text-decoration: underline;
}
</style>
