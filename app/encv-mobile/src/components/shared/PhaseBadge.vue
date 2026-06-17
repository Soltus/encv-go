<template>
  <span :class="['phase-badge', `phase-badge--${phase}`]">
    <PhaseIcon :phase="phase" />
    <span class="phase-badge__label">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Phase } from '@/lib/workflow/types'
import PhaseIcon from './PhaseIcon.vue'

const props = defineProps<{
  phase: Phase
  /** 自定义 label，未传则使用 PHASE_LABEL_MAP 默认值 */
  label?: string
}>()

// Phase → 中文 label 映射（与后端 Phase 枚举值一一对应）
const PHASE_LABEL_MAP: Record<Phase, string> = {
  [Phase.Created]: '已创建',
  [Phase.Analyzing]: '分析中',
  [Phase.Initializing]: '初始化',
  [Phase.Preprocessing]: '预处理',
  [Phase.Encrypting]: '加密中',
  [Phase.Decrypting]: '解密中',
  [Phase.Packing]: '打包中',
  [Phase.Verifying]: '校验中',
  [Phase.Completed]: '已完成',
}

const label = computed(() => props.label ?? PHASE_LABEL_MAP[props.phase] ?? props.phase)
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
  background: var(--ion-color-light, #f4f5f8);
  color: var(--ion-color-medium, #92949c);
  white-space: nowrap;
}

.phase-badge__label {
  line-height: 1;
}

/* Phase 状态色（基于 Ionic 调色板 CSS 变量） */
.phase-badge--created {
  background: rgba(var(--ion-color-medium-rgb, 146, 148, 156), 0.12);
  color: var(--ion-color-medium, #92949c);
}
.phase-badge--analyzing {
  background: rgba(var(--ion-color-primary-rgb, 79, 140, 255), 0.12);
  color: var(--ion-color-primary, #4f8cff);
}
.phase-badge--initializing {
  background: rgba(var(--ion-color-primary-rgb, 79, 140, 255), 0.12);
  color: var(--ion-color-primary, #4f8cff);
}
.phase-badge--preprocessing {
  background: rgba(var(--ion-color-warning-rgb, 255, 196, 9), 0.12);
  color: var(--ion-color-warning, #ffc409);
}
.phase-badge--encrypting {
  background: rgba(var(--ion-color-success-rgb, 45, 211, 111), 0.12);
  color: var(--ion-color-success, #2dd36f);
}
.phase-badge--decrypting {
  background: rgba(var(--ion-color-success-rgb, 45, 211, 111), 0.12);
  color: var(--ion-color-success, #2dd36f);
}
.phase-badge--packing {
  background: rgba(var(--ion-color-tertiary-rgb, 112, 102, 255), 0.12);
  color: var(--ion-color-tertiary, #7066ff);
}
.phase-badge--verifying {
  background: rgba(var(--ion-color-warning-rgb, 255, 196, 9), 0.12);
  color: var(--ion-color-warning, #ffc409);
}
.phase-badge--completed {
  background: rgba(var(--ion-color-success-rgb, 45, 211, 111), 0.15);
  color: var(--ion-color-success, #2dd36f);
}

/* 暗黑模式适配（项目使用 body.dark 标记暗黑模式，见 useTheme.ts syncDarkClass） */
:global(body.dark) .phase-badge {
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.75);
}
</style>
