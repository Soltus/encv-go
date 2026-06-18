<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/tasks" :text="t('common.back')"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('tasks.groupDetail.title') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button
            fill="clear"
            size="small"
            @click="exportGroupReport"
            :disabled="!run || exporting"
            :title="t('tasks.groupDetail.exportZip')"
          >
            <ion-spinner v-if="exporting" name="dots" slot="icon-only"></ion-spinner>
            <ion-icon v-else :icon="downloadOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>

      <ion-toolbar>
        <ion-segment v-model="activeTab" mode="md" class="group-detail-segment">
          <ion-segment-button value="pipeline">
            <ion-label>{{ t('tasks.groupDetail.tabPipeline') }}</ion-label>
          </ion-segment-button>
          <ion-segment-button value="tasks">
            <ion-label>{{ t('tasks.groupDetail.tabTasks') }}</ion-label>
          </ion-segment-button>
          <ion-segment-button value="diagnostics">
            <ion-label>{{ t('tasks.groupDetail.tabDiagnostics') }}</ion-label>
          </ion-segment-button>
        </ion-segment>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- 空态：找不到 run（已被清理 / 旧 runId 失效） -->
      <div v-if="!run" class="empty-state">
        <ion-icon :icon="alertCircleOutline" class="empty-icon" color="medium"></ion-icon>
        <h3>{{ t('tasks.exportNotFoundHeader') }}</h3>
        <p>{{ t('tasks.exportNotFoundMessage') }}</p>
        <ion-button fill="clear" size="small" @click="goBack">{{ t('common.back') }}</ion-button>
      </div>

      <!-- Pipeline tab：TestReportHeader + JobPipelineCard 列表 -->
      <div v-else-if="activeTab === 'pipeline'" class="gd-pipeline">
        <TestReportHeader
          v-if="reportHeaderProps"
          v-bind="reportHeaderProps"
        />
        <div v-if="run.jobs.length === 0" class="empty-state">
          <ion-icon :icon="gitNetworkOutline" class="empty-icon" color="medium"></ion-icon>
          <h3>{{ t('tasks.groupDetail.noJobs') }}</h3>
          <p>{{ t('tasks.groupDetail.noJobsDesc') }}</p>
        </div>
        <div v-else class="gd-pipeline__list">
          <JobPipelineCard
            v-for="job in run.jobs"
            :key="job.id"
            :job="job"
            :step-names="stepNamesMap"
            :display-name="getJobDisplayName(job)"
            @click="onJobClick(job)"
          />
        </div>
      </div>

      <!-- Tasks tab：run 关联的 EncvTask 列表（点击 → L3 TaskDetail modal） -->
      <div v-else-if="activeTab === 'tasks'" class="gd-tasks">
        <div v-if="runTasks.length === 0" class="empty-state">
          <ion-icon :icon="listOutline" class="empty-icon" color="medium"></ion-icon>
          <h3>{{ t('tasks.groupDetail.emptyTasks') }}</h3>
          <p>{{ t('tasks.groupDetail.emptyTasksDesc') }}</p>
        </div>
        <ion-list v-else class="gd-tasks__list">
          <ion-item-sliding v-for="tk in runTasks" :key="tk.id">
            <ion-item
              class="gd-task-item"
              button
              detail
              @click="openTaskDetail(tk)"
            >
              <ion-icon
                :icon="getTaskIcon(tk)"
                :color="getTaskColor(tk)"
                slot="start"
              ></ion-icon>
              <ion-label>
                <h2>{{ getTaskName(tk) }}</h2>
                <p class="gd-task-item__meta">
                  <ion-badge :color="getStatusColor(tk.status)" class="tl-status-badge">
                    {{ getStatusLabel(tk.status) }}
                  </ion-badge>
                  <span class="gd-task-item__type">{{ tk.type === 'encrypt' ? t('tasks.encrypt') : t('tasks.decrypt') }}</span>
                  <ion-badge v-if="tk.pluginName" color="primary" class="gd-task-item__plugin">{{ tk.pluginName }}</ion-badge>
                </p>
                <p class="gd-task-item__time">
                  <span>{{ formatDateTime(tk.createdAt) }}</span>
                  <span v-if="getTaskDuration(tk)">· {{ getTaskDuration(tk) }}</span>
                </p>
                <p v-if="tk.error" class="gd-task-item__error">{{ tk.error }}</p>
              </ion-label>
            </ion-item>
          </ion-item-sliding>
        </ion-list>
      </div>

      <!-- Diagnostics tab：失败任务优先 + StepDetailPanel（点击 job 展开） -->
      <div v-else-if="activeTab === 'diagnostics'" class="gd-diagnostics">
        <div v-if="failedTasks.length === 0 && runningTasks.length === 0" class="empty-state">
          <ion-icon :icon="checkmarkCircleOutline" class="empty-icon" color="success"></ion-icon>
          <h3>{{ t('tasks.groupDetail.emptyDiagnostics') }}</h3>
          <p>{{ t('tasks.groupDetail.emptyDiagnosticsDesc') }}</p>
        </div>
        <template v-else>
          <!-- 失败任务列表（点击展开 step 详情） -->
          <div v-if="failedTasks.length > 0" class="gd-diag-section">
            <h3 class="gd-diag-section__title gd-diag-section__title--fail">
              {{ t('tasks.groupDetail.diagnosticsFailed') }} ({{ failedTasks.length }})
            </h3>
            <ion-list>
              <ion-item
                v-for="(diag, idx) in failedDiagnostics"
                :key="'f-' + diag.task.id"
                :class="['gd-diag-item', { 'gd-diag-item--active': selectedDiagIdx === idx }]"
                button
                @click="selectedDiagIdx = idx"
              >
                <ion-icon :icon="closeCircle" color="danger" slot="start"></ion-icon>
                <ion-label>
                  <h2>{{ getTaskName(diag.task) }}</h2>
                  <p class="gd-task-item__meta">
                    <ion-badge :color="getStatusColor(diag.task.status)" class="tl-status-badge">
                      {{ getStatusLabel(diag.task.status) }}
                    </ion-badge>
                    <ion-badge v-if="diag.task.pluginName" color="primary" class="gd-task-item__plugin">{{ diag.task.pluginName }}</ion-badge>
                  </p>
                  <p class="gd-task-item__error">{{ diag.task.error }}</p>
                </ion-label>
              </ion-item>
            </ion-list>
            <StepDetailPanel
              v-if="selectedFailedDiag"
              :step-run="selectedFailedDiag.step"
              :job-run="selectedFailedDiag.job"
            />
          </div>

          <!-- 运行中任务列表 -->
          <div v-if="runningTasks.length > 0" class="gd-diag-section">
            <h3 class="gd-diag-section__title gd-diag-section__title--run">
              {{ t('tasks.groupDetail.diagnosticsRunning') }} ({{ runningTasks.length }})
            </h3>
            <ion-list>
              <ion-item v-for="tk in runningTasks" :key="'r-' + tk.id" class="gd-diag-item">
                <ion-spinner name="dots" slot="start" color="warning"></ion-spinner>
                <ion-label>
                  <h2>{{ getTaskName(tk) }}</h2>
                  <p class="gd-task-item__meta">
                    <ion-badge color="warning" class="tl-status-badge">{{ t('tasks.running') }}</ion-badge>
                    <ion-badge v-if="tk.pluginName" color="primary" class="gd-task-item__plugin">{{ tk.pluginName }}</ion-badge>
                  </p>
                  <p v-if="tk.progress !== undefined" class="gd-task-item__time">
                    <ion-progress-bar :value="(tk.progress ?? 0) / 100" class="gd-diag-progress"></ion-progress-bar>
                    <span class="gd-diag-progress__pct">{{ tk.progress }}%</span>
                    <span v-if="tk.phase">· {{ tk.phase }}</span>
                  </p>
                </ion-label>
              </ion-item>
            </ion-list>
          </div>
        </template>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonContent, IonButtons,
  IonBackButton, IonButton, IonSegment, IonSegmentButton, IonLabel,
  IonIcon, IonList, IonItem, IonItemSliding, IonBadge, IonProgressBar,
  IonSpinner, alertController, modalController, toastController,
} from '@ionic/vue'
import { Share } from '@capacitor/share'
import {
  downloadOutline, alertCircleOutline, gitNetworkOutline, listOutline,
  checkmarkCircleOutline, closeCircle,
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { formatDateTime } from '@/composables/useDateFormat'
import { useWorkflowTaskService } from '@/composables/useWorkflowTaskService'
import { useTasksList } from '@/composables/useTasksList'
import type { EncvTask } from '@/api/encv'
import type { JobRun, StepRun } from '@/lib/workflow/types'
import TestReportHeader from '@/components/automation/TestReportHeader.vue'
import JobPipelineCard from '@/components/automation/JobPipelineCard.vue'
import StepDetailPanel from '@/components/automation/StepDetailPanel.vue'
import { buildReportZip } from '@/lib/buildReportZip'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const workflowService = useWorkflowTaskService()
const {
  tasks: allTasks,
  getTaskName, getTaskDuration, getStatusLabel, getStatusColor,
  getTaskIcon, getTaskColor,
} = useTasksList()

// ==================== 路由参数 ====================
const runId = computed(() => decodeURIComponent(String(route.params.runId ?? '')))

// ==================== 状态 ====================
const activeTab = ref<'pipeline' | 'tasks' | 'diagnostics'>(
  (loadStoredTab() as 'pipeline' | 'tasks' | 'diagnostics' | null) ?? 'pipeline',
)
const exporting = ref(false)
const selectedDiagIdx = ref(0)

const TAB_STORAGE_KEY = 'encv_group_detail_active_tab_v1'
function loadStoredTab(): string | null {
  try {
    return localStorage.getItem(TAB_STORAGE_KEY)
  } catch {
    return null
  }
}
watch(activeTab, (v) => {
  try {
    localStorage.setItem(TAB_STORAGE_KEY, v)
  } catch {
    // silent
  }
})

// ==================== 数据 ====================
const run = computed(() => workflowService.getRun(runId.value))

/** 该 run 关联的 EncvTask 列表（按 createdAt 升序） */
const runTasks = computed<EncvTask[]>(() => {
  const id = runId.value
  if (!id) return []
  return allTasks.value
    .filter((t) => t.runId === id)
    .sort((a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime())
})

/** 失败 task（Diagnostics tab 用） */
const failedTasks = computed(() => runTasks.value.filter((t) => t.status === 'failed'))

/** 运行中 task（Diagnostics tab 用） */
const runningTasks = computed(() =>
  runTasks.value.filter((t) => t.status === 'running' || t.status === 'cancelling' || t.status === 'queued'),
)

/** 失败 task 对应的 (task, job, step) 三元组（按 step 失败时间倒序） */
const failedDiagnostics = computed(() => {
  const result: { task: EncvTask; job: JobRun; step: StepRun }[] = []
  if (!run.value) return result
  for (const tk of failedTasks.value) {
    for (const job of run.value.jobs) {
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

// ==================== 派生 props ====================
const stepNamesMap = computed<Map<string, string> | undefined>(() => {
  if (!run.value) return undefined
  const m = new Map<string, string>()
  for (const job of run.value.jobs) {
    m.set(job.jobDefId, getJobDisplayName(job))
    for (const step of job.steps) {
      m.set(step.id, step.stepDefId)
    }
  }
  return m
})

const reportHeaderProps = computed(() => {
  if (!run.value) return null
  const total = runTasks.value.length
  const passed = runTasks.value.filter((t) => t.status === 'completed').length
  const failed = runTasks.value.filter((t) => t.status === 'failed').length
  const pending = runTasks.value.filter((t) =>
    t.status === 'running' || t.status === 'queued' || t.status === 'cancelling',
  ).length
  const skipped = runTasks.value.filter((t) => t.status === 'cancelled').length
  return {
    runId: run.value.id.slice(0, 8),
    openedAt: run.value.startedAt ?? run.value.createdAt,
    durationMs: run.value.durationMs ?? 0,
    total,
    passed,
    failed,
    skipped,
    pending,
    platform: 'encv-automation',
  }
})

function getJobDisplayName(job: JobRun): string {
  // 简单规则：去除前缀 + 美化
  return job.jobDefId
    .replace(/^[a-z]+-/, '')
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

// ==================== 操作 ====================
async function openTaskDetail(task: EncvTask) {
  const { default: TaskDetailModal } = await import('@/components/TaskDetailModal.vue')
  const modal = await modalController.create({
    component: TaskDetailModal,
    componentProps: { task },
    cssClass: 'task-detail-modal',
  })
  await modal.present()
}

function onJobClick(_job: JobRun) {
  // 简单处理：点击 job 切到 diagnostics tab
  activeTab.value = 'diagnostics'
}

function goBack() {
  router.replace('/tabs/tasks')
}

/** 把 Blob 转为 base64 dataURL（Capacitor Share API 在 Android 端需要） */
function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error('blobToDataURL failed'))
    reader.readAsDataURL(blob)
  })
}

async function exportGroupReport() {
  if (!run.value) return
  try {
    exporting.value = true
    // 包装成 UnifiedRunRecord 形态给 buildReportZip（totalCases 必填 → 用 task 总数）
    const unifiedLike = {
      id: run.value.id,
      startedAt: run.value.startedAt ?? run.value.createdAt,
      completedAt: run.value.completedAt,
      totalCases: runTasks.value.length,
      passed: runTasks.value.filter((t) => t.status === 'completed').length,
      failed: runTasks.value.filter((t) => t.status === 'failed').length,
      skipped: runTasks.value.filter((t) => t.status === 'cancelled').length,
      results: runTasks.value.map((t) => ({
        caseId: t.id,
        status: (t.status === 'completed' ? 'success' : t.status === 'failed' ? 'failure' : 'skipped') as 'success' | 'failure' | 'skipped',
        error: t.error,
        duration: t.completedAt
          ? String(new Date(t.completedAt).getTime() - new Date(t.createdAt).getTime()) + 'ms'
          : undefined,
      })),
      workflowRun: run.value,
    } as any
    const zipBlob = await buildReportZip(unifiedLike, runTasks.value, t)
    const filename = `encvreport-${run.value.id.slice(0, 8)}-${new Date().toISOString().slice(0, 10)}.zip`
    const isNative = !!(window as any).Capacitor?.isNativePlatform?.()
    if (isNative && Share) {
      const file = new File([zipBlob], filename, { type: 'application/zip' })
      const dataUrl = await blobToDataURL(file)
      await Share.share({
        title: filename,
        text: t('tasks.exportShareText', {
          runId: run.value.id.slice(0, 8),
          passed: String(unifiedLike.passed),
          failed: String(unifiedLike.failed),
        }),
        files: undefined,
        url: dataUrl,
        dialogTitle: t('tasks.exportShareDialogTitle'),
      })
    } else {
      const url = URL.createObjectURL(zipBlob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    }
    const toast = await toastController.create({
      message: t('tasks.exportSuccess', { filename }),
      duration: 2500,
      color: 'success',
    })
    await toast.present()
  } catch (error) {
    console.error('[GroupDetail.exportGroupReport] failed:', error)
    const alert = await alertController.create({
      header: t('tasks.exportFailedHeader'),
      message: String((error as any)?.message ?? error),
      buttons: [t('common.ok')],
    })
    await alert.present()
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  // 如果 runId 无效 → 跳回
  if (!runId.value) {
    router.replace('/tabs/tasks')
  }
})
</script>

<style scoped>
.group-detail-segment {
  --background: transparent;
}
.gd-pipeline__list,
.gd-tasks__list {
  padding: 12px;
  background: transparent;
}
.gd-task-item {
  --padding-start: 8px;
  --padding-end: 8px;
  --min-height: 64px;
  margin-bottom: 6px;
}
.gd-task-item__meta {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 4px;
  font-size: 12px;
}
.gd-task-item__plugin {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
}
.gd-task-item__type {
  font-size: 11px;
  color: var(--ion-color-medium);
}
.gd-task-item__time {
  font-size: 11px;
  color: var(--ion-color-medium-shade);
  margin-top: 4px;
  font-family: var(--tl-card-font-mono, monospace);
}
.gd-task-item__error {
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
.gd-diag-section {
  margin-top: 12px;
  padding: 0 12px 16px;
}
.gd-diag-section__title {
  font-size: 13px;
  font-weight: 600;
  margin: 8px 0 8px;
  padding: 4px 8px;
  border-radius: 4px;
  letter-spacing: 0.04em;
}
.gd-diag-section__title--fail {
  color: var(--ion-color-danger-shade);
  background: rgba(var(--ion-color-danger-rgb), 0.08);
}
.gd-diag-section__title--run {
  color: var(--ion-color-warning-shade);
  background: rgba(var(--ion-color-warning-rgb), 0.08);
}
.gd-diag-item {
  --padding-start: 8px;
  --padding-end: 8px;
  --min-height: 60px;
  margin-bottom: 4px;
  border-radius: 6px;
}
.gd-diag-item--active {
  --background: rgba(var(--ion-color-danger-rgb), 0.06);
  border-left: 3px solid var(--ion-color-danger);
}
.gd-diag-progress {
  margin-top: 4px;
  height: 4px;
}
.gd-diag-progress__pct {
  font-size: 11px;
  font-weight: 600;
  margin-left: 4px;
  color: var(--ion-color-warning-shade);
}
</style>
