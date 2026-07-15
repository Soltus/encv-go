<!--
  StatusBadge - 多色调状态徽章
  参照 codex_web StatusBadge{label, tone}
  tone 取值：
    - ready  = 绿 (success)        → 工具调用成功 / 子任务完成
    - warn   = 橙 (warning)        → 工具失败 / 取消（默认）
    - idle   = 灰 (medium)         → 等待中 / 静默
    - danger = 红 (error)          → 严重错误（Task 4 新增；当前 OperationCard 暂未用，保留供未来扩展）
  pulse: 是否显示脉冲（流式 / 进行中状态）
-->
<template>
  <span class="statusBadge" :class="[badgeToneClass, { statusBadge_pulse: pulse }]">{{ label }}</span>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  label: string;
  tone: "ready" | "warn" | "idle" | "danger";
  pulse?: boolean;
}>();

const toneClassMap = {
  ready: "tone-success",
  warn: "tone-warning",
  idle: "tone-neutral",
  danger: "tone-error",
} as const;
const badgeToneClass = computed(() => toneClassMap[props.tone]);
</script>

<style scoped>
/* 尺寸/字重等 bespoke 细节归本组件；色调由下方 tone-* 以设计令牌表达，不依赖共享词汇 */
.statusBadge {
  display: inline-flex;
  align-items: center;
  height: 18px;
  padding: 0 8px;
  border-radius: 9px;
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
  letter-spacing: 0.02em;
  white-space: nowrap;
}

.statusBadge_pulse {
  animation: statusBadgePulse 1.4s ease-in-out infinite;
}

/* 色调：前景朝主题自有 --color-base-content、背景朝 --color-base-100 派生，
   明暗随主题自动翻转，无需 body.dark；任意用户主题免费获得正确对比（§6.3 tint 提级已非必需） */
.tone-success { background: color-mix(in srgb, var(--color-success) 14%, var(--color-base-100)); color: color-mix(in srgb, var(--color-success) 75%, var(--color-base-content)); }
.tone-warning { background: color-mix(in srgb, var(--color-warning) 16%, var(--color-base-100)); color: color-mix(in srgb, var(--color-warning) 72%, var(--color-base-content)); }
.tone-error   { background: color-mix(in srgb, var(--color-error) 16%, var(--color-base-100));   color: color-mix(in srgb, var(--color-error) 72%, var(--color-base-content)); }
.tone-neutral { background: color-mix(in srgb, var(--color-base-content) 14%, var(--color-base-100)); color: var(--color-base-content); }

@keyframes statusBadgePulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
