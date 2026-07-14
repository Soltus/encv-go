<!--
  ReasoningMessage - 推理过程（折叠展示）
  头部：MessageAuthor(bulbOutline, "推理", meta) + 状态徽章
  折叠态：只显示 "推理" 文字
  展开态：显示完整 reasoning 文本（Markdown 渲染）
  codex_web 风格：左侧 icon 圆头像 + label + meta + 状态徽章 + chevron
-->
<template>
  <div class="reasoningMessage" :class="{ reasoningMessage_streaming: streaming }">
    <div class="reasoningHeader" @click="expanded = !expanded">
      <MessageAuthor
        :icon="icon"
        :label="label"
        :meta="metaText"
        :variant="streaming ? 'streaming' : 'default'"
      />
      <StatusBadge
        v-if="streaming"
        :label="t('agent.thinking')"
        tone="idle"
        pulse
      />
      <StatusBadge
        v-else
        :label="t('agent.thought')"
        tone="ready"
      />
      <ion-icon :icon="expanded ? chevronUp : chevronDown" class="reasoningChevron" />
    </div>
    <div v-if="expanded" class="reasoningBody">
      <MarkdownStream :content="text" :streaming="false" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { bulbOutline, chevronDownOutline, chevronUpOutline } from "ionicons/icons";
import { computed, ref } from "vue";
import MessageAuthor from "@/components/agent/MessageAuthor.vue";
import StatusBadge from "@/components/agent/StatusBadge.vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const props = defineProps<{
  text: string;
  streaming: boolean;
}>();

const { t } = useI18n();
const expanded = ref(props.streaming);
const icon = bulbOutline;
const chevronUp = chevronUpOutline;
const chevronDown = chevronDownOutline;

const label = computed(() => t("agent.reasoning"));
const metaText = computed(() => (props.streaming ? t("agent.thinkingMeta") : ""));
</script>

<style scoped>
.reasoningMessage {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 0 8px 8px;
  border-left: 3px solid rgba(var(--ion-color-medium-rgb), 0.25);
  margin: 6px 0;
  transition: border-color 0.3s ease;
}

.reasoningMessage_streaming {
  border-left-color: rgba(var(--ion-color-primary-rgb), 0.5);
}

.reasoningHeader {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

.reasoningChevron {
  font-size: 14px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
}

.reasoningBody {
  padding-left: 30px;
  font-size: 13px;
  color: var(--encv-text-secondary);
  background: rgba(var(--ion-color-medium-rgb), 0.05);
  border-radius: 4px;
  padding: 8px 10px 8px 30px;
  margin-top: 4px;
}
</style>
