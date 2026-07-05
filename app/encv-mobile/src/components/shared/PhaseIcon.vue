<template>
  <ion-icon
    :icon="icon"
    :class="['phase-icon', `phase-icon--${phase}`]"
    :style="size ? { fontSize: `${size}px` } : undefined"
  />
</template>

<script setup lang="ts">
import {
  banOutline,
  checkmarkCircleOutline,
  closeCircleOutline,
  cloudUploadOutline,
  codeSlashOutline,
  cubeOutline,
  helpCircleOutline,
  lockClosedOutline,
  lockOpenOutline,
  playOutline,
  searchOutline,
  shieldCheckmarkOutline,
} from "ionicons/icons";
import { computed } from "vue";
import { Phase } from "@/lib/workflow/types";

/**
 * PhaseIcon 支持的值：
 * - 9 个 Phase 枚举值（created ~ completed）
 * - 2 个终态 status 值（failed / cancelled）
 *
 * 统一用 ion-icon，删除 emoji / Unicode / 自定义 SVG。
 */
export type PhaseIconValue = Phase | "failed" | "cancelled";

const props = withDefaults(
  defineProps<{
    phase: PhaseIconValue;
    /** 图标尺寸（px），未传则继承父级 font-size */
    size?: number;
  }>(),
  {
    size: undefined,
  }
);

// Phase / Status → ion-icon 映射（与后端 Phase 枚举值一一对应）
const PHASE_ICON_MAP: Record<PhaseIconValue, string> = {
  [Phase.Created]: cloudUploadOutline,
  [Phase.Analyzing]: searchOutline,
  [Phase.Initializing]: playOutline,
  [Phase.Preprocessing]: codeSlashOutline,
  [Phase.Encrypting]: lockClosedOutline,
  [Phase.Decrypting]: lockOpenOutline,
  [Phase.Packing]: cubeOutline,
  [Phase.Verifying]: shieldCheckmarkOutline,
  [Phase.Completed]: checkmarkCircleOutline,
  failed: closeCircleOutline,
  cancelled: banOutline,
};

const _icon = computed(() => PHASE_ICON_MAP[props.phase] ?? helpCircleOutline);
</script>

<style scoped>
.phase-icon {
  font-size: 1.1em;
}

/* 暗黑模式适配（项目使用 body.dark 标记暗黑模式，见 useTheme.ts syncDarkClass） */
:global(body.dark) .phase-icon {
  opacity: 0.95;
}
</style>
