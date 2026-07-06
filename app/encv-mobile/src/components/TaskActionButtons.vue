<template>
  <div class="action-buttons">
    <ion-button
      v-if="task.status === 'running'"
      expand="block"
      color="warning"
      @click="emit('cancel')"
    >
      <ion-icon :icon="closeCircle" slot="start"></ion-icon>
      {{ t('tasks.cancel') }}
    </ion-button>
    <ion-button
      v-if="task.status === 'failed'"
      expand="block"
      color="primary"
      @click="emit('retry')"
    >
      <ion-icon :icon="refresh" slot="start"></ion-icon>
      {{ t('tasks.retry') }}
    </ion-button>
    <ion-button
      v-if="['completed', 'failed', 'cancelled'].includes(task.status)"
      expand="block"
      color="danger"
      fill="outline"
      @click="emit('remove')"
    >
      <ion-icon :icon="trash" slot="start"></ion-icon>
      {{ t('tasks.remove') }}
    </ion-button>
  </div>
</template>

<script setup lang="ts">
import {
  closeCircle,
  refresh,
  trash,
} from "ionicons/icons";

import type { EncvTask } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";

defineProps<{ task: EncvTask }>();
const emit = defineEmits<{
  (e: "cancel"): void;
  (e: "retry"): void;
  (e: "remove"): void;
}>();
const { t } = useI18n();
</script>

<style scoped>
.action-buttons {
  margin-top: 24px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.action-buttons ion-button {
  --border-radius: 10px;
  height: 44px;
  font-weight: 600;
}
</style>
