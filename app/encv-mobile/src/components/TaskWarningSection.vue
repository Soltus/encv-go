<template>
  <div class="detail-section warning-section" v-if="task.warning">
    <div class="section-title warning-section-title">
      <ion-icon :icon="warningOutline" color="warning"></ion-icon>
      {{ t('tasks.warnings') }}
    </div>
    <p>{{ task.warning }}</p>
    <pre v-if="task.warningDetail" class="warning-detail-pre">{{ formatWarningDetail(task.warningDetail) }}</pre>
  </div>
</template>

<script setup lang="ts">
import type { EncvTask } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";

defineProps<{ task: EncvTask }>();
const { t } = useI18n();

function _formatWarningDetail(detail: string): string {
  try {
    return JSON.stringify(JSON.parse(detail), null, 2);
  } catch {
    return detail;
  }
}
</script>

<style scoped>
.detail-section {
  margin-bottom: 20px;
}

.section-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--ion-text-color);
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.warning-section-title { color: #e65100; }

.warning-section {
  background: rgba(255, 152, 0, 0.06);
  border-radius: 8px;
  padding: 12px;
}

.warning-detail-pre {
  background: var(--ion-color-step-100, #f0f0f0);
  border-radius: 6px;
  padding: 8px 10px;
  margin-top: 6px;
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 120px;
  overflow-y: auto;
  line-height: 1.5;
  color: #666;
}
</style>
