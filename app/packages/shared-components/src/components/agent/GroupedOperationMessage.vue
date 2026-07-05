<!--
  GroupedOperationMessage - 累积操作摘要（两级折叠）
  参照 codex_web GroupedOperationMessage{items, forceComplete}
  - 外层 OperationGroupSummary 卡片：显示"已运行/正在运行 N 条命令"等汇总
  - 内层 OperationItemDetail 列表：默认显示前 3 条，"显示更多 (N)" 展开后显示全部
  - 摘要文本规则：
    * 全 command → "已运行 N 条命令，Xms"
    * 全 fileChange → 委托 FileChangeSummaryMessage
    * 混合 → "已执行 N 个操作（X 命令 + Y 文件变更）"
    * 全 toolOutput → "已执行 N 个工具"
  - active 跟随最末 item 的状态
-->
<template>
  <FileChangeSummaryMessage
    v-if="allFileChange && fileItems.length > 0"
    :items="fileItems"
    :force-complete="forceComplete"
  />
  <div v-else class="groupedOp">
    <!-- 外层：OperationGroupSummary 摘要卡片（点击展开/折叠） -->
    <button
      type="button"
      class="groupedOpHeader"
      :class="{ groupedOpHeader_active: isActive, groupedOpHeader_expanded: groupExpanded }"
      :aria-expanded="groupExpanded"
      @click="toggleGroup"
    >
      <ion-icon :icon="icon" class="groupedOpIcon" />
      <span class="groupedOpSummary">{{ summary }}</span>
      <StatusBadge
        v-if="status"
        :label="status"
        :tone="statusTone"
      />
      <ion-icon
        :icon="chevronIcon"
        class="groupedOpChevron"
        :class="{ groupedOpChevron_open: groupExpanded }"
      />
    </button>

    <!-- 内层：OperationItemDetail 列表（两级折叠） -->
    <div v-if="hasDetail && groupExpanded" class="groupedOpList">
      <div
        v-for="(it, idx) in visibleItems"
        :key="`${it.id}-${idx}`"
        class="groupedOpItem"
        @click="toggleItem(idx)"
      >
        <ion-icon :icon="itemIcon(it.kind)" class="groupedOpItemIcon" />
        <span class="groupedOpItemName">{{ it.name || t('agent.tool.unknown') }}</span>
        <span class="groupedOpItemArgs">
          {{ expandedItems.has(idx) ? it.args : truncateArgs(it.args) }}
        </span>
        <ion-icon
          v-if="it.args && it.args.length > 80"
          :icon="chevronIcon"
          class="groupedOpItemChevron"
          :class="{ groupedOpItemChevron_open: expandedItems.has(idx) }"
        />
        <!--
          tool_result 结构化卡片：按 toolCall.name 分发到对应卡片组件
            - list_mounts → MountListCard
            - list_files  → FileListCard
            - read_file   → FileContentCard
            - stat_file   → FileListCard（单条记录模式）
          resultsByCallId 由 AgentChat 父组件传过来（id → ToolResult）。
          没有 result 的 item（pending / 失败）→ 不渲染卡片，原有 row 保留。
        -->
        <div v-if="resultFor(it.id, it.name)" class="groupedOpItemCard" @click.stop>
          <MountListCard
            v-if="it.name === 'list_mounts'"
            :result-json="resultFor(it.id, it.name)!.result"
          />
          <FileListCard
            v-else-if="it.name === 'list_files' || it.name === 'stat_file'"
            :result-json="resultFor(it.id, it.name)!.result"
          />
          <FileContentCard
            v-else-if="it.name === 'read_file'"
            :result-json="resultFor(it.id, it.name)!.result"
          />
        </div>
      </div>
      <button
        v-if="canExpand"
        type="button"
        class="groupedOpMore"
        @click.stop="expandGroup"
      >
        {{ showMoreLabel }}
      </button>
      <button
        v-else-if="canCollapse"
        type="button"
        class="groupedOpMore"
        @click.stop="collapseGroup"
      >
        {{ t('agent.ops.collapseAll') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ToolCall, ToolKind, ToolResult, ToolStatus } from "@encv/shared-components/composables/useAgent";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  documentTextOutline,
  ellipsisHorizontalCircleOutline,
  eyeOutline,
  helpCircleOutline,
  searchOutline,
  terminalOutline,
} from "ionicons/icons";
import { computed, ref } from "vue";
import { OPERATION_COLLAPSE_INITIAL_COUNT } from "./twoLevelGrouping";

const props = defineProps<{
  items: ToolCall[];
  /**
   * toolCallId → ToolResult 的映射（由 AgentChat 用 findToolResult 配对后传过来）。
   * 用于在每个 item 下方条件渲染结构化卡片（MountListCard / FileListCard / FileContentCard）。
   * 可选：未传 / 没结果时不渲染卡片，原有 row 保留。
   */
  resultsByCallId?: Record<string, ToolResult>;
  forceComplete?: boolean;
}>();

const { t } = useI18n();
const groupExpanded = ref(false);
const expandedItems = ref<Set<number>>(new Set());

/**
 * 取 item 对应的 ToolResult。
 * 返回 null 时模板不渲染结构化卡片（item 仍按原 row 显示）。
 */
function _resultFor(id: string, name: string | undefined): ToolResult | null {
  if (!props.resultsByCallId || !name) return null;
  // 只对"已知支持结构化卡片"的工具名查 result —— 其他工具（video_encrypt 等）
  // 的 result 不在 resultsByCallId 也不影响 row 渲染。
  if (name !== "list_mounts" && name !== "list_files" && name !== "stat_file" && name !== "read_file") {
    return null;
  }
  return props.resultsByCallId[id] ?? null;
}

function _toggleGroup() {
  groupExpanded.value = !groupExpanded.value;
}

function _toggleItem(idx: number) {
  if (expandedItems.value.has(idx)) {
    expandedItems.value.delete(idx);
  } else {
    expandedItems.value.add(idx);
  }
  // trigger reactivity
  expandedItems.value = new Set(expandedItems.value);
}

const kinds = computed(() => props.items.map(it => it.kind));
const allFileChange = computed(() => kinds.value.length > 0 && kinds.value.every(k => k === "fileChange"));
const allCommand = computed(() => kinds.value.length > 0 && kinds.value.every(k => k === "command"));

const _fileItems = computed(() => props.items);

const lastItem = computed<ToolCall | null>(() => {
  return props.items.length > 0 ? props.items[props.items.length - 1] : null;
});

const totalDuration = computed(() => {
  // 累加 args.durationMs（如有）作为粗略估计；无值则按 0 显示
  let total = 0;
  for (const it of props.items) {
    try {
      const parsed = JSON.parse(it.args);
      const v = (parsed as Record<string, unknown>).durationMs;
      if (typeof v === "number") total += v;
    } catch {
      // ignore
    }
  }
  return total;
});

const _summary = computed(() => {
  const n = props.items.length;
  const cmd = props.items.filter(i => i.kind === "command").length;
  const file = props.items.filter(i => i.kind === "fileChange").length;
  if (allCommand.value) {
    return t("agent.ops.commands", { n: String(n), ms: String(totalDuration.value || 0) });
  }
  if (allFileChange.value) {
    return t("agent.ops.files", { n: String(n) });
  }
  if (cmd > 0 && file > 0) {
    return t("agent.ops.mixed", { n: String(n), cmd: String(cmd), file: String(file) });
  }
  return t("agent.ops.toolOutputs", { n: String(n) });
});

const _icon = computed(() => {
  if (allCommand.value) return terminalOutline;
  return ellipsisHorizontalCircleOutline;
});

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

// 两级折叠：hasDetail / visibleItems / canExpand / canCollapse
const _hasDetail = computed(() => props.items.length > 0);
const _visibleItems = computed(() => {
  if (groupExpanded.value) return props.items;
  return props.items.slice(0, OPERATION_COLLAPSE_INITIAL_COUNT);
});
const _canExpand = computed(() => !groupExpanded.value && props.items.length > OPERATION_COLLAPSE_INITIAL_COUNT);
const _canCollapse = computed(() => groupExpanded.value && props.items.length > OPERATION_COLLAPSE_INITIAL_COUNT);
const _showMoreLabel = computed(() =>
  t("agent.ops.showMore", {
    n: String(props.items.length - OPERATION_COLLAPSE_INITIAL_COUNT),
  })
);

function _expandGroup() {
  groupExpanded.value = true;
}

function _collapseGroup() {
  groupExpanded.value = false;
}

function _itemIcon(kind: ToolKind | undefined) {
  switch (kind) {
    case "command":
      return terminalOutline;
    case "fileChange":
      return documentTextOutline;
    case "readOnly":
      return eyeOutline;
    case "webSearch":
      return searchOutline;
    default:
      return helpCircleOutline;
  }
}

function _truncateArgs(raw: string): string {
  if (!raw) return "";
  if (raw.length <= 80) return raw;
  return raw.slice(0, 77) + "…";
}
</script>

<style scoped>
.groupedOp {
  margin: 6px 0;
}

.groupedOpHeader {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  border-radius: 14px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  font-size: 12px;
  color: var(--ion-text-color);
  max-width: 100%;
  flex-wrap: wrap;
  /* button reset */
  font-family: inherit;
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease;
}

.groupedOpHeader:hover {
  background: rgba(var(--ion-color-medium-rgb), 0.14);
  border-color: rgba(var(--ion-color-medium-rgb), 0.28);
}

.groupedOpHeader_active {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  border-color: rgba(var(--ion-color-primary-rgb), 0.3);
  animation: groupedOpPulse 1.4s ease-in-out infinite;
}

.groupedOpHeader_expanded {
  background: rgba(var(--ion-color-medium-rgb), 0.14);
  border-color: rgba(var(--ion-color-medium-rgb), 0.28);
}

.groupedOpIcon {
  font-size: 13px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.groupedOpSummary {
  font-weight: 500;
  word-break: break-word;
}

.groupedOpChevron {
  font-size: 12px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
  margin-inline-start: 2px;
  transition: transform 0.2s ease;
}

.groupedOpChevron_open {
  transform: rotate(90deg);
}

.groupedOpList {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 6px;
  padding: 6px 8px;
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-radius: 6px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.16);
  font-size: 11.5px;
}

.groupedOpItem {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  padding: 2px 4px;
  border-radius: 4px;
  cursor: pointer;
  transition: background 0.15s ease;
}

.groupedOpItem:hover {
  background: rgba(var(--ion-color-medium-rgb), 0.1);
}

.groupedOpItemIcon {
  font-size: 12px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
}

.groupedOpItemName {
  font-weight: 500;
  color: var(--ion-text-color);
  flex-shrink: 0;
}

.groupedOpItemArgs {
  color: var(--encv-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex: 1;
  font-size: 11px;
}

/* Args 展开后：多行显示 + 自动换行 + 浅灰背景 */
.groupedOpItem:has(.groupedOpItemChevron_open) .groupedOpItemArgs {
  white-space: pre-wrap;
  word-break: break-all;
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  padding: 4px 6px;
  border-radius: 3px;
  font-size: 10.5px;
  max-height: 200px;
  overflow-y: auto;
}

.groupedOpItemChevron {
  font-size: 11px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
  transition: transform 0.2s ease;
}

.groupedOpItemChevron_open {
  transform: rotate(90deg);
}

/*
  tool_result 结构化卡片容器：
  - grid-column: 1 / -1 → 跨整行（不与 name/args/chevron 同行）
  - padding-left 缩进让卡片视觉上"属于" item
  - @click.stop 防止点击卡片内部时触发外层 toggleItem（避免反复折叠 args）
*/
.groupedOpItemCard {
  grid-column: 1 / -1;
  margin: 4px 0 0 24px;
  min-width: 0;
}

.groupedOpMore {
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

.groupedOpMore:hover {
  text-decoration: underline;
}

@keyframes groupedOpPulse {
  0%, 100% { background-color: rgba(var(--ion-color-primary-rgb), 0.12); }
  50% { background-color: rgba(var(--ion-color-primary-rgb), 0.22); }
}
</style>
