<template>
  <div class="badge" :class="`badge--${status}`">
    <span class="badge__icon">{{ icon }}</span>
    <span v-if="showName" class="badge__name">{{ name }}</span>
  </div>
</template>

<script setup lang="ts">
import type { StepStatus } from "@/lib/workflow/types";
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    status: StepStatus;
    name?: string;
    showName?: boolean;
  }>(),
  {
    showName: true,
  }
);

const icon = computed(() => {
  switch (props.status) {
    case "success":
      return "✓";
    case "failure":
      return "✕";
    case "cancelled":
      return "⊘";
    case "skipped":
      return "⊘";
    case "timed_out":
      return "⏱";
    case "running":
      return "◉";
    case "queued":
      return "◌";
    case "submitted":
      return "◐";
    default:
      return "○";
  }
});
</script>

<style scoped>
.badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 7px;
  border-radius: 10px;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.04em;
  white-space: nowrap;
}

.badge__icon {
  font-size: 11px;
  line-height: 1;
}

.badge__name {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 状态颜色 */
.badge--pending { background: #F4EFE6; color: #6B5D4C; }
.badge--submitted { background: #E8DFF0; color: #6B4C8B; }
.badge--queued { background: #FFF8E1; color: #8B7500; }
.badge--running { background: #E3F2FD; color: #1565C0; animation: pulse 1.5s ease-in-out infinite; }
.badge--success { background: #E8F5E9; color: #1B5E20; }
.badge--failure { background: #FCE4EC; color: #880E4F; }
.badge--cancelled { background: #F5F5F5; color: #616161; }
.badge--skipped { background: #F5F5F5; color: #9E9E9E; }
.badge--timed_out { background: #FFF3E0; color: #E65100; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
