<template>
  <ion-icon :icon="icon" :class="['phase-icon', `phase-icon--${phase}`]" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IonIcon } from '@ionic/vue'
import {
  searchOutline,
  settingsOutline,
  colorFilterOutline,
  lockClosedOutline,
  lockOpenOutline,
  cubeOutline,
  shieldCheckmarkOutline,
  checkmarkCircleOutline,
  flashOutline,
} from 'ionicons/icons'
import { Phase } from '@/lib/workflow/types'

const props = defineProps<{
  phase: Phase
}>()

// Phase → ion-icon 映射（与后端 Phase 枚举值一一对应）
const PHASE_ICON_MAP: Record<Phase, string> = {
  [Phase.Created]: flashOutline,
  [Phase.Analyzing]: searchOutline,
  [Phase.Initializing]: settingsOutline,
  [Phase.Preprocessing]: colorFilterOutline,
  [Phase.Encrypting]: lockClosedOutline,
  [Phase.Decrypting]: lockOpenOutline,
  [Phase.Packing]: cubeOutline,
  [Phase.Verifying]: shieldCheckmarkOutline,
  [Phase.Completed]: checkmarkCircleOutline,
}

const icon = computed(() => PHASE_ICON_MAP[props.phase] ?? flashOutline)
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
