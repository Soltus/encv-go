<template>
  <div class="tasks-tab">
    <div v-if="runTasks.length === 0" class="empty-state">
      <ion-icon :icon="listOutline" class="empty-icon" color="medium"></ion-icon>
      <h3>{{ t('tasks.groupDetail.emptyTasks') }}</h3>
      <p>{{ t('tasks.groupDetail.emptyTasksDesc') }}</p>
    </div>
    <ion-list v-else class="tasks-tab__list">
      <ion-item-sliding v-for="tk in runTasks" :key="tk.id">
        <ion-item
          class="tasks-tab__item"
          button
          detail
          @click="emit('select-task', tk)"
        >
          <ion-icon
            :icon="getTaskIcon(tk)"
            :color="getStatusColor(tk.status)"
            slot="start"
          ></ion-icon>
          <ion-label>
            <h2>{{ getTaskName(tk) }}</h2>
            <p class="tasks-tab__meta">
              <ion-badge :color="getStatusColor(tk.status)" class="tl-status-badge">
                {{ getStatusLabel(tk.status) }}
              </ion-badge>
              <span class="tasks-tab__type">{{ tk.type === 'encrypt' ? t('tasks.encrypt') : t('tasks.decrypt') }}</span>
              <ion-badge v-if="tk.pluginName" color="primary" class="tasks-tab__plugin">{{ tk.pluginName }}</ion-badge>
            </p>
            <p class="tasks-tab__time">
              <span>{{ formatDateTime(tk.createdAt) }}</span>
              <span v-if="getTaskDuration(tk)">· {{ getTaskDuration(tk) }}</span>
            </p>
            <p v-if="tk.error" class="tasks-tab__error">{{ tk.error }}</p>
          </ion-label>
        </ion-item>
      </ion-item-sliding>
    </ion-list>
  </div>
</template>

<script setup lang="ts">
import { IonIcon, IonList, IonItem, IonItemSliding, IonBadge, IonLabel } from '@ionic/vue'
import { listOutline } from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { formatDateTime } from '@/composables/useDateFormat'
import type { EncvTask } from '@/api/encv'

const { t } = useI18n()

defineProps<{ runTasks: EncvTask[] }>()
const emit = defineEmits<{ (e: 'select-task', task: EncvTask): void }>()

function getTaskName(task: EncvTask): string {
  if (task.targetPath) return task.targetPath.split('/').pop() || task.targetPath
  if (task.sourcePath) return task.sourcePath.split('/').pop() || task.sourcePath
  return task.id.slice(0, 8)
}

function getTaskIcon(task: EncvTask): string {
  return task.type === 'encrypt' ? 'lock-closed' : 'lock-open'
}

function getStatusLabel(status: EncvTask['status']): string {
  return t(`tasks.status.${status}`)
}

function getStatusColor(status: EncvTask['status']): string {
  switch (status) {
    case 'completed': return 'success'
    case 'failed': return 'danger'
    case 'running': case 'cancelling': return 'warning'
    case 'cancelled': return 'medium'
    case 'queued': return 'primary'
    default: return 'medium'
  }
}

function getTaskDuration(task: EncvTask): string {
  if (!task.completedAt) return ''
  const ms = new Date(task.completedAt).getTime() - new Date(task.createdAt).getTime()
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}
</script>

<style scoped>
.tasks-tab__list {
  padding: 12px;
  background: transparent;
}
.tasks-tab__item {
  --padding-start: 8px;
  --padding-end: 8px;
  --min-height: 64px;
  margin-bottom: 6px;
}
.tasks-tab__meta {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 4px;
  font-size: 12px;
}
.tasks-tab__plugin {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
}
.tasks-tab__type {
  font-size: 11px;
  color: var(--ion-color-medium);
}
.tasks-tab__time {
  font-size: 11px;
  color: var(--ion-color-medium-shade);
  margin-top: 4px;
  font-family: monospace;
}
.tasks-tab__error {
  font-size: 11px;
  color: var(--ion-color-danger-shade);
  margin-top: 4px;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.empty-state {
  text-align: center;
  padding: 48px 16px;
}
.empty-icon {
  font-size: 56px;
  margin-bottom: 8px;
  display: block;
  margin-left: auto;
  margin-right: auto;
}
</style>
