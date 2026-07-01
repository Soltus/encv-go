<template>
  <span v-if="scoreValue > 0" :class="['relevance-badge', `relevance-badge--${tier}`]">
    <ion-icon :icon="tierIcon" class="relevance-badge__icon" />
    <span class="relevance-badge__label">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IonIcon } from '@ionic/vue'
import { sparkles, checkmarkCircle, ellipsisHorizontalCircle } from 'ionicons/icons'

const props = defineProps<{
  /** 相关度分数（0-1，越大越相似）。0 或未设置时不显示。 */
  score?: number
}>()

// 规范化为非 undefined 数值（默认 0），后续所有计算都基于此
const scoreValue = computed(() => props.score ?? 0)

// 按分数分三档：高(>=0.6) / 中(>=0.3) / 低(>0)
const tier = computed<'high' | 'mid' | 'low'>(() => {
  const s = scoreValue.value
  if (s >= 0.6) return 'high'
  if (s >= 0.3) return 'mid'
  return 'low'
})

const tierIcon = computed(() => {
  switch (tier.value) {
    case 'high':
      return sparkles
    case 'mid':
      return checkmarkCircle
    default:
      return ellipsisHorizontalCircle
  }
})

// 显示百分比，保留整数
const label = computed(() => `${Math.round(scoreValue.value * 100)}%`)
</script>

<style scoped>
.relevance-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 2px 7px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 500;
  white-space: nowrap;
  line-height: 1.4;
}

.relevance-badge__icon {
  font-size: 12px;
}

.relevance-badge__label {
  line-height: 1;
}

/* 三档颜色：高=primary，中=success，低=medium */
.relevance-badge--high {
  background: rgba(var(--ion-color-primary-rgb), 0.15);
  color: var(--ion-color-primary);
}

.relevance-badge--mid {
  background: rgba(var(--ion-color-success-rgb), 0.15);
  color: var(--ion-color-success);
}

.relevance-badge--low {
  background: rgba(var(--ion-color-medium-rgb), 0.15);
  color: var(--ion-color-medium);
}
</style>
