<template>
  <span :class="['phase-badge', `phase-badge--${phase}`]">
    <PhaseIcon :phase="phase" />
    <span class="phase-badge__label">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Phase } from "@/lib/workflow/types";
import PhaseIcon, { type PhaseIconValue } from "./PhaseIcon.vue";

const props = defineProps<{
  phase: PhaseIconValue;
  /** 自定义 label，未传则使用 PHASE_LABEL_MAP 默认值 */
  label?: string;
}>();

// Phase → 中文 label 映射（与后端 Phase 枚举值一一对应）
const PHASE_LABEL_MAP: Record<string, string> = {
  [Phase.Created]: "已创建",
  [Phase.Analyzing]: "分析中",
  [Phase.Initializing]: "初始化",
  [Phase.Preprocessing]: "预处理",
  [Phase.Encrypting]: "加密中",
  [Phase.Decrypting]: "解密中",
  [Phase.Packing]: "打包中",
  [Phase.Verifying]: "校验中",
  [Phase.Completed]: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

const label = computed(() => props.label ?? PHASE_LABEL_MAP[props.phase] ?? props.phase);
</script>

<style scoped>
.phase-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}

.phase-badge__label {
  line-height: 1;
}

/* Phase 状态色（使用 design token，light/dark 双主题均保留状态色） */
.phase-badge--created {
  background: rgba(var(--tl-state-created-rgb), 0.15);
  color: var(--tl-state-created);
}
.phase-badge--analyzing {
  background: rgba(var(--tl-state-analyzing-rgb), 0.15);
  color: var(--tl-state-analyzing);
}
.phase-badge--initializing {
  background: rgba(var(--tl-state-initializing-rgb), 0.15);
  color: var(--tl-state-initializing);
}
.phase-badge--preprocessing {
  background: rgba(var(--tl-state-preprocessing-rgb), 0.15);
  color: var(--tl-state-preprocessing);
}
.phase-badge--encrypting {
  background: rgba(var(--tl-state-encrypting-rgb), 0.15);
  color: var(--tl-state-encrypting);
}
.phase-badge--decrypting {
  background: rgba(var(--tl-state-decrypting-rgb), 0.15);
  color: var(--tl-state-decrypting);
}
.phase-badge--packing {
  background: rgba(var(--tl-state-packing-rgb), 0.15);
  color: var(--tl-state-packing);
}
.phase-badge--verifying {
  background: rgba(var(--tl-state-verifying-rgb), 0.15);
  color: var(--tl-state-verifying);
}
.phase-badge--completed {
  background: rgba(var(--tl-state-completed-rgb), 0.18);
  color: var(--tl-state-completed);
}
.phase-badge--failed {
  background: rgba(var(--tl-state-failed-rgb), 0.15);
  color: var(--tl-state-failed);
}
.phase-badge--cancelled {
  background: rgba(var(--tl-state-cancelled-rgb), 0.15);
  color: var(--tl-state-cancelled);
}
</style>
