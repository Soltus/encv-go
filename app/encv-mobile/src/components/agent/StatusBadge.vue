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
  <span class="statusBadge" :class="[`statusBadge_${tone}`, { statusBadge_pulse: pulse }]">{{ label }}</span>
</template>

<script setup lang="ts">
defineProps<{
  label: string;
  tone: "ready" | "warn" | "idle" | "danger";
  pulse?: boolean;
}>();
</script>

<style scoped>
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

.statusBadge_ready {
  background: rgba(45, 211, 111, 0.14);
  color: var(--ion-color-success-shade, #28ba62);
}

.statusBadge_warn {
  background: rgba(255, 196, 9, 0.16);
  color: var(--ion-color-warning-shade, #e0ac08);
}

.statusBadge_idle {
  background: rgba(146, 148, 156, 0.16);
  color: var(--ion-color-medium-shade, #808289);
}

/* Task 4：危险 / 失败状态红色 badge（保留供未来扩展，
   当前 OperationCard 仍把 failed 映射到 warn 以保持视觉一致；
   新组件想表达"严重错误"时可直接用 danger）。 */
.statusBadge_danger {
  background: rgba(239, 68, 68, 0.16);
  color: var(--ion-color-danger-shade, #c53030);
}

.statusBadge_pulse {
  animation: statusBadgePulse 1.4s ease-in-out infinite;
}

@keyframes statusBadgePulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

body.dark .statusBadge_ready {
  background: rgba(47, 223, 117, 0.18);
  color: #3de283;
}
body.dark .statusBadge_warn {
  background: rgba(255, 213, 72, 0.18);
  color: #ffda5a;
}
body.dark .statusBadge_danger {
  background: rgba(239, 68, 68, 0.22);
  color: #ff5e5e;
}
</style>
