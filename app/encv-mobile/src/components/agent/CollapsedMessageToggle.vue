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
    :class="{ 'collapsedToggle--primary': active || expanded, collapsedToggle_active: active }"
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

const chevronUp = chevronUpOutline;
const chevronDown = chevronDownOutline;
</script>

<style scoped>
/* chip 外观以设计令牌内联表达，组件自包含、不依赖共享类词汇 */
.collapsedToggle {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.75rem;
  border-radius: var(--radius-selector, 1rem);
  border: 1px solid var(--color-base-300);
  background: var(--color-base-200);
  color: var(--color-base-content);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease;
}
.collapsedToggle:hover {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--color-primary) 40%, var(--color-base-300));
  box-shadow: 0 2px 6px -2px rgb(0 0 0 / 0.1);
}
.collapsedToggle--primary {
  border-color: color-mix(in srgb, var(--color-primary) 35%, var(--color-base-300));
  background: color-mix(in srgb, var(--color-primary) 12%, var(--color-base-100));
  color: color-mix(in srgb, var(--color-primary) 75%, var(--color-base-content));
}
.collapsedToggle--primary:hover {
  transform: translateY(-1px);
  border-color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 18%, var(--color-base-100));
  box-shadow: 0 2px 6px -2px rgb(0 0 0 / 0.1);
}
.collapsedToggle--primary:active { transform: scale(0.98); }
.collapsedToggle_active {
  animation: collapsedActivePulse 1.4s ease-in-out infinite;
}

.collapsedIcon {
  font-size: 13px;
  color: var(--color-primary);
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
  0%, 100% { background-color: color-mix(in srgb, var(--color-primary) 14%, var(--color-base-100)); }
  50% { background-color: color-mix(in srgb, var(--color-primary) 24%, var(--color-base-100)); }
}
</style>
