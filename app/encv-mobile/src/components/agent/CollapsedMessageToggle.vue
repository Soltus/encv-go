<!--
  CollapsedMessageToggle - 折叠消息切换器
  参照 codex_web CollapsedMessageToggle{icon, label, meta, expanded, active, onToggle}
  - 折叠态：显示 icon + label + meta
  - 活跃态：浅灰脉冲 CSS 动画（active=true）
  - 点击 → onToggle()
-->
<template>
  <!-- 表面（bg/border/fg/圆角/悬停）上提到全局 .ui-chip / .ui-chip--neutral（随主题翻转，用户主题可覆写）。
       scoped 仅留布局 + 活跃脉冲动画（[data-v-x] 特异性胜出，不抢表面）。 -->
  <div
    class="collapsedToggle ui-chip"
    :class="{ 'ui-chip--neutral': !(active || expanded), collapsedToggle_active: active }"
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
/* 表面（bg/border/fg/悬停/按下）由全局 .ui-chip / .ui-chip--neutral 提供（随主题翻转）。
   scoped 仅留布局 + 尺寸（[data-v-x] 胜出）；悬停/按下沿用 .ui-chip:hover/:active。 */
.collapsedToggle {
  gap: 0.375rem;
  padding: 0.25rem 0.75rem;
  border-radius: var(--radius-selector, 1rem);
  font-size: 0.8125rem;
}
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
