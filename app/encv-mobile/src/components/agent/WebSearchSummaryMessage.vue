<!--
  WebSearchSummaryMessage - 联网搜索摘要
  头部：MessageAuthor(searchOutline, "搜索", query count) + 状态徽章
  折叠态：显示首条 query
  展开态：显示所有 query 列表
  codex_web 风格：Globe2-like icon + avatar + 状态徽章
-->
<template>
  <div class="webSearchMessage">
    <div class="webSearchHeader" @click="expanded = !expanded">
      <MessageAuthor :icon="icon" :label="label" :meta="metaText" />
      <StatusBadge
        v-if="totalHits != null"
        :label="`${totalHits} 命中`"
        tone="ready"
      />
      <ion-icon :icon="expanded ? chevronUp : chevronDown" class="webSearchChevron" />
    </div>
    <div v-if="expanded && queries.length > 0" class="webSearchList">
      <div v-for="(q, idx) in queries" :key="idx" class="webSearchItem">
        <ion-icon :icon="searchOutline" class="webSearchItemIcon" />
        <span class="webSearchItemText">{{ q }}</span>
        <span v-if="results && results[idx] != null" class="webSearchItemHits">
          {{ results[idx] }} 命中
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ToolCall } from "@/composables/useAgent";
import { useI18n } from "@/composables/useI18n";
import { chevronDownOutline, chevronUpOutline, searchOutline } from "ionicons/icons";
import { computed, ref } from "vue";

const props = defineProps<{
  queries: string[];
  toolCalls: ToolCall[];
  /** 每个查询的命中数（与 queries 一一对应，可选） */
  results?: number[];
}>();

const { t } = useI18n();
const expanded = ref(false);
const icon = searchOutline;
const chevronUp = chevronUpOutline;
const chevronDown = chevronDownOutline;
const label = computed(() => t("agent.webSearch"));

const metaText = computed(() => {
  const n = props.queries.length;
  return n > 1 ? `${n} ${t("agent.queries")}` : `${n} ${t("agent.query")}`;
});

const totalHits = computed(() => {
  if (!props.results) return null;
  let s = 0;
  for (const r of props.results) s += r;
  return s > 0 ? s : null;
});
</script>

<style scoped>
.webSearchMessage {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 6px 0;
  max-width: 100%;
}

.webSearchHeader {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

.webSearchChevron {
  font-size: 14px;
  color: var(--encv-text-secondary);
}

.webSearchList {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 4px;
  padding: 6px 8px;
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-radius: 6px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.16);
}

.webSearchItem {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--ion-text-color);
}

.webSearchItemIcon {
  font-size: 12px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
}

.webSearchItemText {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.webSearchItemHits {
  font-size: 10.5px;
  color: var(--encv-text-secondary);
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  padding: 1px 5px;
  border-radius: 6px;
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
}
</style>
