<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('tasks.taskDetail') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="modalController.dismiss()" fill="clear" size="small" color="medium">
            {{ t('tasks.close') }}
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding">
      <TaskBasicInfo :task="task" />
      <TaskTimeline :task="task" />

      <div class="detail-section" v-if="task.status === 'running' || task.status === 'cancelling'">
        <div class="section-title">{{ t('tasks.progress') }}</div>
        <ion-progress-bar :value="task.progress / 100"></ion-progress-bar>
        <div class="progress-stats">
          <span>{{ task.progress }}%</span>
          <span v-if="task.speed">{{ task.speed }}</span>
          <span v-if="task.eta">ETA: {{ task.eta }}</span>
        </div>
      </div>

      <TaskOutputInfo :task="task" @open="openOutput" @locate="locateOutput" />
      <TaskErrorSection :task="task" />
      <TaskWarningSection :task="task" />
      <TaskActionButtons :task="task" @cancel="dismiss('cancel')" @retry="dismiss('retry')" @remove="dismiss('remove')" />
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonButton,
  IonContent, IonProgressBar, modalController,
} from '@ionic/vue'
import { useI18n } from '@/composables/useI18n'
import type { EncvTask } from '@/api/encv'
import TaskBasicInfo from './TaskBasicInfo.vue'
import TaskTimeline from './TaskTimeline.vue'
import TaskOutputInfo from './TaskOutputInfo.vue'
import TaskErrorSection from './TaskErrorSection.vue'
import TaskWarningSection from './TaskWarningSection.vue'
import TaskActionButtons from './TaskActionButtons.vue'

const props = defineProps<{ task: EncvTask }>()
const { t } = useI18n()
const router = useRouter()

function dismiss(action: 'cancel' | 'retry' | 'remove') {
  return modalController.dismiss({ action, id: props.task.id })
}

function openOutput(outputPath: string) {
  const name = outputPath.split('/').pop() || outputPath
  router.push({ path: '/player', query: { path: outputPath, name } })
  modalController.dismiss({ action: 'opened', id: props.task.id, outputPath })
}

// 🆕 v3 2026-06-18 Task 8：locateOutput 不再做路径转换
//   - 后端 task.outputPath 已统一为虚拟路径 /d/<mount>/<sub>（task_manager.absToVirtualPath）
//   - 前端直接拆 dir + name 塞 route.query，Files.vue onIonViewWillEnter 消费
//   - 旧版逻辑（物理绝对路径 → 前端无法解析）已废弃
function locateOutput(outputPath: string) {
  const trimmed = outputPath.replace(/\/+$/, '')
  const lastSlash = trimmed.lastIndexOf('/')
  const name = lastSlash >= 0 ? trimmed.substring(lastSlash + 1) : trimmed
  const dir = lastSlash >= 0 ? trimmed.substring(0, lastSlash) : '/d'
  router.push({ path: '/tabs/files', query: { path: dir, highlight: name } })
  modalController.dismiss({ action: 'located', id: props.task.id, outputPath })
}
</script>

<style scoped>
.detail-section { margin-bottom: 20px; }
.section-title {
  font-size: 14px; font-weight: 700; color: var(--ion-text-color);
  margin-bottom: 10px; display: flex; align-items: center; gap: 6px;
}
.progress-stats {
  display: flex; gap: 12px; margin-top: 6px;
  font-size: 12px; color: var(--ion-color-medium); font-weight: 500;
}
</style>
