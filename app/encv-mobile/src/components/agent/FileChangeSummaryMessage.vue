<!--
  FileChangeSummaryMessage - 文件变更特化摘要（两级折叠）
  - 外层 FileChangeSummary 卡片：显示"已编辑 N 个文件"汇总
  - 内层文件路径列表：默认显示前 3 条，"显示更多 (N)" 展开后显示全部
  - 当 operationGroup 全是 fileChange 时由 GroupedOperationMessage 委托渲染
-->
<template>
  <div class="fileChangeSummary">
    <!-- 外层：FileChangeSummary 摘要卡片（点击展开/折叠） -->
    <button
      type="button"
      class="fileChangeHeader"
      :class="{ fileChangeHeader_active: isActive, fileChangeHeader_expanded: headerExpanded }"
      :aria-expanded="headerExpanded"
      @click="toggleHeader"
    >
      <ion-icon :icon="icon" class="fileChangeIcon" />
      <span class="fileChangeSummaryText">{{ summary }}</span>
      <StatusBadge
        v-if="status"
        :label="status"
        :tone="statusTone"
      />
      <ion-icon
        :icon="chevronIcon"
        class="fileChangeChevron"
        :class="{ fileChangeChevron_open: headerExpanded }"
      />
    </button>

    <!-- 内层：文件路径列表（两级折叠） -->
    <div v-if="hasDetail && headerExpanded" class="fileChangeList">
      <div
        v-for="(p, idx) in visiblePaths"
        :key="`${idx}-${p}`"
        class="fileChangeItem"
        :title="p"
      >
        <ion-icon :icon="documentOutline" class="fileChangeItemIcon" />
        <span class="fileChangeItemPath">{{ truncate(p) }}</span>
      </div>
      <button
        v-if="canExpand"
        type="button"
        class="fileChangeMore"
        @click.stop="expandList"
      >
        {{ showMoreLabel }}
      </button>
      <button
        v-else-if="canCollapse"
        type="button"
        class="fileChangeMore"
        @click.stop="collapseList"
      >
        {{ t('agent.ops.collapseAll') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { documentTextOutline } from "ionicons/icons";
import { computed, ref } from "vue";
import type { ToolCall, ToolStatus } from "@/composables/useAgent";
import { useI18n } from "@/composables/useI18n";
import { OPERATION_COLLAPSE_INITIAL_COUNT } from "./twoLevelGrouping";

const props = defineProps<{
  items: ToolCall[];
  forceComplete?: boolean;
}>();

const { t } = useI18n();
const listExpanded = ref(false);
const headerExpanded = ref(false);

function _toggleHeader() {
  headerExpanded.value = !headerExpanded.value;
}

const _icon = documentTextOutline;
const _documentOutline = documentTextOutline;

const paths = computed<string[]>(() => {
  const out: string[] = [];
  for (const it of props.items) {
    try {
      const args = JSON.parse(it.args) as Record<string, unknown>;
      if (Array.isArray(args.changedFiles)) {
        for (const f of args.changedFiles) {
          if (typeof f === "string") out.push(f);
          else if (f && typeof f === "object" && typeof (f as Record<string, unknown>).path === "string") {
            out.push((f as Record<string, string>).path);
          }
        }
      } else if (typeof args.path === "string") {
        out.push(args.path);
      }
    } catch {
      // ignore
    }
  }
  return out;
});

const lastItem = computed<ToolCall | null>(() => props.items[props.items.length - 1] ?? null);

const _summary = computed(() => t("agent.ops.files", { n: String(paths.value.length) }));

const _status = computed(() => {
  const s = lastItem.value?.status;
  if (!s) return "";
  if (s === "success") return t("agent.completed");
  if (s === "failed") return t("agent.failed");
  if (s === "cancelled") return t("agent.cancelled");
  if (s === "running") return t("agent.running");
  return "";
});

const _statusTone = computed<"ready" | "warn" | "idle">(() => {
  const s: ToolStatus | undefined = lastItem.value?.status;
  if (s === "success") return "ready";
  if (s === "failed" || s === "cancelled") return "warn";
  if (s === "running" || s === "pending") return "idle";
  return "idle";
});

const _isActive = computed(() => lastItem.value?.status === "running" || lastItem.value?.status === "pending");

// 两级折叠：hasDetail / visiblePaths / canExpand / canCollapse
const _hasDetail = computed(() => paths.value.length > 0);
const _visiblePaths = computed(() => {
  if (listExpanded.value) return paths.value;
  return paths.value.slice(0, OPERATION_COLLAPSE_INITIAL_COUNT);
});
const _canExpand = computed(() => !listExpanded.value && paths.value.length > OPERATION_COLLAPSE_INITIAL_COUNT);
const _canCollapse = computed(() => listExpanded.value && paths.value.length > OPERATION_COLLAPSE_INITIAL_COUNT);
const _showMoreLabel = computed(() =>
  t("agent.ops.showMore", {
    n: String(paths.value.length - OPERATION_COLLAPSE_INITIAL_COUNT),
  })
);

function _expandList() {
  listExpanded.value = true;
}

function _collapseList() {
  listExpanded.value = false;
}

function _truncate(p: string): string {
  if (p.length <= 60) return p;
  return "…" + p.slice(p.length - 59);
}
</script>

<style scoped>
.fileChangeSummary {
  margin: 6px 0;
}

.fileChangeHeader {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  border-radius: 14px;
  border: 1px solid rgba(var(--ion-color-primary-rgb), 0.2);
  font-size: 12px;
  color: var(--ion-text-color);
  max-width: 100%;
  flex-wrap: wrap;
  /* button reset */
  font-family: inherit;
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease;
}

.fileChangeHeader:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.14);
  border-color: rgba(var(--ion-color-primary-rgb), 0.3);
}

.fileChangeHeader_active {
  animation: fileChangeActivePulse 1.4s ease-in-out infinite;
}

.fileChangeHeader_expanded {
  background: rgba(var(--ion-color-primary-rgb), 0.14);
  border-color: rgba(var(--ion-color-primary-rgb), 0.3);
}

.fileChangeIcon {
  font-size: 13px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.fileChangeSummaryText {
  font-weight: 500;
}

.fileChangeChevron {
  font-size: 12px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
  margin-inline-start: 2px;
  transition: transform 0.2s ease;
}

.fileChangeChevron_open {
  transform: rotate(90deg);
}

.fileChangeList {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 6px;
  padding: 6px 8px;
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-radius: 6px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.16);
}

.fileChangeItem {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  color: var(--ion-text-color);
  min-width: 0;
}

.fileChangeItemIcon {
  font-size: 12px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
}

.fileChangeItemPath {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fileChangeMore {
  align-self: flex-start;
  margin-top: 4px;
  background: transparent;
  border: 0;
  padding: 2px 0;
  font-size: 11.5px;
  color: var(--ion-color-primary);
  cursor: pointer;
  font-family: inherit;
}

.fileChangeMore:hover {
  text-decoration: underline;
}

@keyframes fileChangeActivePulse {
  0%, 100% { background-color: rgba(var(--ion-color-primary-rgb), 0.08); }
  50% { background-color: rgba(var(--ion-color-primary-rgb), 0.18); }
}
</style>
