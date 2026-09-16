<!--
  UserMessageBubble - 用户消息气泡
  参照 codex_web .userMessage / .userMessageEditor：
  - 右对齐 + 浅灰背景（#f0f1f3）+ 圆角 18px
  - 长消息自动折叠（>560 字符 或 >9 行）
  - 纯文本渲染（不解析 Markdown）
-->
<template>
  <div class="userBubble">
    <div
      class="userBubbleText ui-bubble ui-bubble--user"
      :class="{ userBubbleText_collapsed: shouldCollapse && !expanded }"
    >
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
import { computed, ref } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const props = defineProps<{
  text: string;
}>();

const { t } = useI18n();
const expanded = ref(false);

const CHAR_LIMIT = 560;
const LINE_LIMIT = 9;

const charCount = computed(() => props.text.length);
const lineCount = computed(() => props.text.split("\n").length);

const shouldCollapse = computed(() => charCount.value > CHAR_LIMIT || lineCount.value > LINE_LIMIT);
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

/* 表面（背景/圆角/字号/前景色）已上提到全局 .ui-bubble.ui-bubble--user（随主题翻转）。
   此处仅留结构与折叠状态（scoped [data-v-x] 特异性胜出，不抢表面）。 */
.userBubbleText {
  max-width: min(72%, 620px);
  min-width: min(100%, 280px);
  word-break: break-word;
  overflow-wrap: anywhere;
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
  color: var(--color-primary);
  cursor: pointer;
  font-weight: 500;
}

.userBubbleToggle:hover {
  text-decoration: underline;
}
</style>
