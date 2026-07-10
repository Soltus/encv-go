<!--
  V2QuickActions - v2 工具快捷动作 chip 行

  位置：AgentChat 输入框上方
  作用：让用户一键触发 v2 工具演示（不依赖 LLM 选择行为）
  设计：
  - 横向滚动 chip 行（移动端窄屏友好）
  - 每个 chip = 图标 + 中文标签
  - 点击 chip → 向上 emit 'pick'，由父组件（AgentChat）把 prompt 填到输入框
  - streaming 时禁用 chip（避免干扰正在进行的请求）
-->
<template>
  <div class="v2QuickActions" v-if="actions.length > 0">
    <div class="v2QuickActionsScroll">
      <button
        v-for="a in actions"
        :key="a.id"
        type="button"
        class="v2Chip"
        :class="{ 'v2Chip_disabled': disabled }"
        :disabled="disabled"
        :title="a.title"
        @click="emitPick(a)"
      >
        <ion-icon :icon="a.icon" class="v2ChipIcon" />
        <span class="v2ChipLabel">{{ a.label }}</span>
        <span class="v2ChipTag">v2</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  documentTextOutline,
  informationCircleOutline,
  pricetagOutline,
  searchOutline,
  swapHorizontalOutline,
  terminalOutline,
} from "ionicons/icons";
import { useI18n } from "@/composables/useI18n";

export interface V2QuickAction {
  id: string;
  label: string;
  title: string;
  icon: string;
  /** 注入到输入框的 prompt（中文示例，会调对应 v2 工具） */
  prompt: string;
}

defineProps<{
  /** streaming / confirming 时禁用 */
  disabled?: boolean;
}>();

const emit = defineEmits<{
  pick: [action: V2QuickAction];
}>();

const { t } = useI18n();

/** 6 个 v2 工具快捷入口（按用户使用频率排序：搜索 > 读 > 元数据 > 写 > 跑命令） */
const actions: V2QuickAction[] = [
  {
    id: "search",
    label: t("agent.v2Chip.search"),
    title: t("agent.v2Chip.searchTitle"),
    icon: searchOutline,
    prompt: t("agent.v2Chip.searchPrompt"),
  },
  {
    id: "read",
    label: t("agent.v2Chip.read"),
    title: t("agent.v2Chip.readTitle"),
    icon: documentTextOutline,
    prompt: t("agent.v2Chip.readPrompt"),
  },
  {
    id: "metadata",
    label: t("agent.v2Chip.metadata"),
    title: t("agent.v2Chip.metadataTitle"),
    icon: informationCircleOutline,
    prompt: t("agent.v2Chip.metadataPrompt"),
  },
  {
    id: "editMetadata",
    label: t("agent.v2Chip.editMetadata"),
    title: t("agent.v2Chip.editMetadataTitle"),
    icon: pricetagOutline,
    prompt: t("agent.v2Chip.editMetadataPrompt"),
  },
  {
    id: "batchRename",
    label: t("agent.v2Chip.batchRename"),
    title: t("agent.v2Chip.batchRenameTitle"),
    icon: swapHorizontalOutline,
    prompt: t("agent.v2Chip.batchRenamePrompt"),
  },
  {
    id: "command",
    label: t("agent.v2Chip.command"),
    title: t("agent.v2Chip.commandTitle"),
    icon: terminalOutline,
    prompt: t("agent.v2Chip.commandPrompt"),
  },
];

function emitPick(a: V2QuickAction): void {
  emit("pick", a);
}
</script>

<style scoped>
.v2QuickActions {
  padding: 4px 12px 6px;
  border-top: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.08));
}
.v2QuickActionsScroll {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: none;
  padding-bottom: 2px;
}
.v2QuickActionsScroll::-webkit-scrollbar {
  display: none;
}
.v2Chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 9px;
  background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.08));
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.18));
  border-radius: 14px;
  font-size: 11.5px;
  color: var(--ion-text-color);
  white-space: nowrap;
  flex-shrink: 0;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s, transform 0.1s;
}
.v2Chip:hover:not(.v2Chip_disabled) {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  border-color: rgba(var(--ion-color-primary-rgb), 0.4);
}
.v2Chip:active:not(.v2Chip_disabled) {
  transform: scale(0.97);
}
.v2Chip_disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.v2ChipIcon {
  font-size: 13px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}
.v2ChipLabel {
  font-weight: 500;
}
.v2ChipTag {
  font-size: 9px;
  padding: 1px 4px;
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
  border-radius: 4px;
  font-weight: 600;
  letter-spacing: 0.04em;
  line-height: 1.2;
}
</style>
