<!--
  CollapsedMessageToggle - 折叠消息切换器
  参照 codex_web CollapsedMessageToggle{icon, label, meta, expanded, active, onToggle}
  - 折叠态：显示 icon + label + meta
  - 活跃态：浅灰脉冲 CSS 动画（active=true）
  - 点击 → onToggle()
-->
<template>
  <div
    class="collapsedToggle"
    :class="{ collapsedToggle_active: active, collapsedToggle_expanded: expanded }"
    @click="onToggle"
  >
    <ion-icon :icon="icon" class="collapsedIcon" />
    <span class="collapsedLabel">{{ label }}</span>
    <span v-if="meta" class="collapsedMeta">{{ meta }}</span>
    <ion-icon
      v-if="onToggle != null"
      :icon="expanded ? chevronUp : chevronDown"
      class="collapsedChevron"
    />
  </div>
</template>

<script setup lang="ts">
import { chevronDownOutline, chevronUpOutline } from "ionicons/icons";
import type { Component } from "vue";

defineProps<{
  icon: Component | string;
  label: string;
  meta?: string;
  expanded: boolean;
  active?: boolean;
  onToggle: () => void;
}>();

const _chevronUp = chevronUpOutline;
const _chevronDown = chevronDownOutline;
</script>

<style scoped>
.collapsedToggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  border-radius: 14px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.18);
  font-size: 12px;
  color: var(--ion-text-color);
  cursor: pointer;
  user-select: none;
  transition: background-color 0.15s;
  max-width: 100%;
  min-height: 24px;
}

.collapsedToggle:hover {
  background: rgba(var(--ion-color-medium-rgb), 0.16);
}

.collapsedToggle_expanded {
  background: rgba(var(--ion-color-primary-rgb), 0.1);
  border-color: rgba(var(--ion-color-primary-rgb), 0.22);
}

.collapsedToggle_active {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  border-color: rgba(var(--ion-color-primary-rgb), 0.3);
  animation: collapsedActivePulse 1.4s ease-in-out infinite;
}

.collapsedIcon {
  font-size: 13px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.collapsedLabel {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.collapsedMeta {
  font-size: 11px;
  color: var(--encv-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 220px;
}

.collapsedChevron {
  font-size: 12px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
}

@keyframes collapsedActivePulse {
  0%, 100% { background-color: rgba(var(--ion-color-primary-rgb), 0.12); }
  50% { background-color: rgba(var(--ion-color-primary-rgb), 0.22); }
}
</style>
