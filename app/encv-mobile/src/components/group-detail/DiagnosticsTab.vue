<template>
  <div class="diagnostics-tab">
    <div v-if="failedTasks.length === 0 && runningTasks.length === 0" class="empty-state">
      <ion-icon :icon="checkmarkCircleOutline" class="empty-icon" color="success"></ion-icon>
      <h3>{{ t('tasks.groupDetail.emptyDiagnostics') }}</h3>
      <p>{{ t('tasks.groupDetail.emptyDiagnosticsDesc') }}</p>
    </div>
    <template v-else>
      <!-- 失败任务列表（点击展开 step 详情） -->
      <div v-if="failedTasks.length > 0" class="diagnostics-section">
        <h3 class="diagnostics-section__title diagnostics-section__title--fail">
          {{ t('tasks.groupDetail.diagnosticsFailed') }} ({{ failedTasks.length }})
        </h3>
        <ion-list>
          <ion-item
            v-for="(diag, idx) in failedDiagnostics"
            :key="'f-' + diag.task.id"
            :class="['diagnostics-item', { 'diagnostics-item--active': selectedDiagIdx === idx }]"
            button
            @click="selectedDiagIdx = idx"
          >
            <ion-icon :icon="closeCircle" color="danger" slot="start"></ion-icon>
            <ion-label>
              <h2>{{ getTaskName(diag.task) }}</h2>
              <p class="diagnostics-item__meta">
                <ion-badge :color="getStatusColor(diag.task.status)" class="tl-status-badge">
                  {{ getStatusLabel(diag.task.status) }}
                </ion-badge>
                <ion-badge v-if="diag.task.pluginName" color="primary" class="diagnostics-item__plugin">{{ diag.task.pluginName }}</ion-badge>
              </p>
              <p class="diagnostics-item__error">{{ diag.task.error }}</p>
            </ion-label>
          </ion-item>
        </ion-list>
        <StepDetailPanel
          v-if="selectedFailedDiag"
          :step-run="selectedFailedDiag.step"
          :job-run="selectedFailedDiag.job"
        />
      </div>

      <!-- 运行中任务 -->
      <div v-if="runningTasks.length > 0" class="diagnostics-section">
        <h3 class="diagnostics-section__title diagnostics-section__title--run">
          {{ t('tasks.groupDetail.diagnosticsRunning') }} ({{ runningTasks.length }})
        </h3>
        <ion-list>
          <ion-item v-for="tk in runningTasks" :key="'r-' + tk.id" class="diagnostics-item">
            <ion-spinner name="dots" slot="start" color="warning"></ion-spinner>
            <ion-label>
              <h2>{{ getTaskName(tk) }}</h2>
              <p class="diagnostics-item__meta">
                <ion-badge color="warning" class="tl-status-badge">{{ t('tasks.running') }}</ion-badge>
                <ion-badge v-if="tk.pluginName" color="primary" class="diagnostics-item__plugin">{{ tk.pluginName }}</ion-badge>
              </p>
              <p v-if="tk.progress !== undefined" class="diagnostics-item__time">
                <ion-progress-bar :value="(tk.progress ?? 0) / 100" class="diagnostics-progress"></ion-progress-bar>
                <span class="diagnostics-progress__pct">{{ tk.progress }}%</span>
                <span v-if="tk.phase">· {{ tk.phase }}</span>
              </p>
            </ion-label>
          </ion-item>
        </ion-list>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { IonIcon, IonList, IonItem, IonLabel, IonBadge, IonProgressBar, IonSpinner } from '@ionic/vue'
import { checkmarkCircleOutline, closeCircle } from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import StepDetailPanel from '@/components/automation/StepDetailPanel.vue'
import type { EncvTask } from '@/api/encv'
import type { JobRun, StepRun } from '@/lib/workflow/types'

const props = defineProps<{
  failedTasks: EncvTask[]
  runningTasks: EncvTask[]
  jobs: JobRun[]
}>()

const { t } = useI18n()

const selectedDiagIdx = ref(0)

// 失败 task 对应的 (task, job, step) 三元组
const failedDiagnostics = computed(() => {
  const result: { task: EncvTask; job: JobRun; step: StepRun }[] = []
  for (const tk of props.failedTasks) {
    for (const job of props.jobs) {
      for (const step of job.steps) {
        if (step.taskId === tk.id && (step.status === 'failure' || step.status === 'timed_out')) {
          result.push({ task: tk, job, step })
        }
      }
    }
  }
  return result.sort((a, b) => {
    const aEnd = a.step.completedAt ? new Date(a.step.completedAt).getTime() : 0
    const bEnd = b.step.completedAt ? new Date(b.step.completedAt).getTime() : 0
    return bEnd - aEnd
  })
})

const selectedFailedDiag = computed(() => failedDiagnostics.value[selectedDiagIdx.value] ?? null)

// 当 failedDiagnostics 变化时（新增/删除失败 task）→ 重置 idx
watch(failedDiagnostics, (v) => {
  if (selectedDiagIdx.value >= v.length) {
    selectedDiagIdx.value = 0
  }
})

function getTaskName(task: EncvTask): string {
  if (task.targetPath) return task.targetPath.split('/').pop() || task.targetPath
  if (task.sourcePath) return task.sourcePath.split('/').pop() || task.sourcePath
  return task.id.slice(0, 8)
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
</script>

<style scoped>
.diagnostics-section {
  margin-top: 12px;
  padding: 0 12px 16px;
}
.diagnostics-section__title {
  font-size: 13px;
  font-weight: 600;
  margin: 8px 0 8px;
  padding: 4px 8px;
  border-radius: 4px;
  letter-spacing: 0.04em;
}
.diagnostics-section__title--fail {
  color: var(--ion-color-danger-shade);
  background: rgba(var(--ion-color-danger-rgb), 0.08);
}
.diagnostics-section__title--run {
  color: var(--ion-color-warning-shade);
  background: rgba(var(--ion-color-warning-rgb), 0.08);
}
.diagnostics-item {
  --padding-start: 8px;
  --padding-end: 8px;
  --min-height: 60px;
  margin-bottom: 4px;
  border-radius: 6px;
}
.diagnostics-item--active {
  --background: rgba(var(--ion-color-danger-rgb), 0.06);
  border-left: 3px solid var(--ion-color-danger);
}
.diagnostics-item__meta {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 4px;
  font-size: 12px;
}
.diagnostics-item__plugin {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
}
.diagnostics-item__error {
  font-size: 11px;
  color: var(--ion-color-danger-shade);
  margin-top: 4px;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.diagnostics-item__time {
  font-size: 11px;
  color: var(--ion-color-medium-shade);
  margin-top: 4px;
  font-family: monospace;
}
.diagnostics-progress {
  margin-top: 4px;
  height: 4px;
}
.diagnostics-progress__pct {
  font-size: 11px;
  font-weight: 600;
  margin-left: 4px;
  color: var(--ion-color-warning-shade);
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
