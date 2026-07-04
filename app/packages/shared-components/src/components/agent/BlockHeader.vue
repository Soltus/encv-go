<!--
  BlockHeader - 块头（tool / approval / plan / error 块）
  参照 codex_web BlockHeader{icon, title, status, statusTone, copyText, expanded, onToggleExpanded}
  - 默认折叠，只显示 icon + title + status
  - 展开后由父组件控制显示完整内容
-->
<template>
  <div class="blockHeader" :class="{ blockHeader_expanded: expanded }">
    <div class="blockTitle" @click="onToggleExpanded">
      <ion-icon :icon="icon" class="blockIcon"></ion-icon>
      <span class="blockTitleText">{{ title }}</span>
      <StatusBadge v-if="status" :label="status" :tone="statusTone || 'idle'" />
    </div>
    <div class="blockActions" @click.stop>
      <button
        v-if="copyText"
        type="button"
        class="blockActionBtn"
        :title="t('agent.copy')"
        @click="handleCopy"
      >
        <ion-icon :icon="copyIcon" />
      </button>
      <button
        v-if="onToggleExpanded"
        type="button"
        class="blockActionBtn"
        :title="expanded ? t('agent.collapse') : t('agent.expand')"
        @click="onToggleExpanded"
      >
        <ion-icon :icon="expanded ? chevronUp : chevronDown" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { IonIcon } from "@ionic/vue";
import { chevronDownOutline, chevronUpOutline, copyOutline } from "ionicons/icons";
import type { Component } from "vue";
import { copyToClipboard } from "@encv/shared-components/composables/useClipboard";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";
import StatusBadge from "./StatusBadge.vue";

const props = defineProps<{
  icon: Component | string;
  title: string;
  status?: string;
  statusTone?: "ready" | "warn" | "idle";
  copyText?: string;
  expanded: boolean;
  onToggleExpanded?: () => void;
}>();

const { t } = useI18n();

const copyIcon = copyOutline;
const chevronUp = chevronUpOutline;
const chevronDown = chevronDownOutline;

async function handleCopy() {
  if (!props.copyText) return;
  const ok = await copyToClipboard(props.copyText);
  if (ok) {
    showToast({ message: t("agent.copied"), duration: 1200, color: "success" });
  } else {
    showToast({ message: t("agent.copyFailed"), duration: 1500, color: "danger" });
  }
}
</script>

<style scoped>
.blockHeader {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: rgba(var(--ion-color-primary-rgb), 0.06);
  border-radius: 8px;
  border: 1px solid rgba(var(--ion-color-primary-rgb), 0.12);
  min-height: 30px;
}

.blockHeader_expanded {
  background: rgba(var(--ion-color-primary-rgb), 0.1);
}

.blockTitle {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
  cursor: pointer;
  user-select: none;
}

.blockIcon {
  font-size: 14px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.blockTitleText {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--ion-text-color);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.blockActions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

.blockActionBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: 0;
  background: transparent;
  border-radius: 5px;
  color: var(--encv-text-secondary);
  cursor: pointer;
  padding: 0;
  transition: background-color 0.15s, color 0.15s;
}

.blockActionBtn:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  color: var(--ion-color-primary);
}

.blockActionBtn ion-icon {
  font-size: 14px;
}
</style>
