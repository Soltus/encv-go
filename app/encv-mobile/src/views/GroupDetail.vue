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
      <!-- 空态：找不到 run -->
      <div v-if="!run" class="empty-state">
        <ion-icon :icon="alertCircleOutline" class="empty-icon" color="medium"></ion-icon>
        <h3>{{ t('tasks.exportNotFoundHeader') }}</h3>
        <p>{{ t('tasks.exportNotFoundMessage') }}</p>
        <ion-button fill="clear" size="small" @click="goBack">{{ t('common.back') }}</ion-button>
      </div>

      <template v-else>
        <PipelineTab
          v-if="activeTab === 'pipeline'"
          :jobs="run.jobs"
          :run-id="run.id"
          :started-at="run.startedAt ?? run.createdAt"
          :duration-ms="run.durationMs ?? 0"
          :total="totals.total"
          :passed="totals.passed"
          :failed="totals.failed"
          :pending="totals.pending"
          :skipped="totals.skipped"
          @select-job="onJobClick"
        />
        <TasksTab
          v-else-if="activeTab === 'tasks'"
          :run-tasks="runTasks"
          @select-task="openTaskDetail"
        />
        <DiagnosticsTab
          v-else-if="activeTab === 'diagnostics'"
          :failed-tasks="failedTasks"
          :running-tasks="runningTasks"
          :jobs="run.jobs"
        />
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonContent, IonButtons,
  IonBackButton, IonButton, IonSegment, IonSegmentButton, IonLabel,
  IonIcon, IonSpinner, alertController, modalController, toastController,
} from '@ionic/vue'
import { Share } from '@capacitor/share'
import { downloadOutline, alertCircleOutline } from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { useWorkflowTaskService } from '@/composables/useWorkflowTaskService'
import { useTasksList } from '@/composables/useTasksList'
import type { EncvTask } from '@/api/encv'
import type { JobRun } from '@/lib/workflow/types'
import PipelineTab from '@/components/group-detail/PipelineTab.vue'
import TasksTab from '@/components/group-detail/TasksTab.vue'
import DiagnosticsTab from '@/components/group-detail/DiagnosticsTab.vue'
import { buildReportZip } from '@/lib/buildReportZip'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const workflowService = useWorkflowTaskService()
const { tasks: allTasks } = useTasksList()

// ============ 路由参数 ============
const runId = computed(() => decodeURIComponent(String(route.params.runId ?? '')))

// ============ Tab 状态（持久化） ============
const activeTab = ref<'pipeline' | 'tasks' | 'diagnostics'>(
  (loadStoredTab() as 'pipeline' | 'tasks' | 'diagnostics' | null) ?? 'pipeline',
)
const TAB_STORAGE_KEY = 'encv_group_detail_active_tab_v1'
function loadStoredTab(): string | null {
  try { return localStorage.getItem(TAB_STORAGE_KEY) } catch { return null }
}
watch(activeTab, (v) => {
  try { localStorage.setItem(TAB_STORAGE_KEY, v) } catch { /* silent */ }
})

// ============ 数据 ============
const run = computed(() => workflowService.getRun(runId.value))

const runTasks = computed<EncvTask[]>(() => {
  const id = runId.value
  if (!id) return []
  return allTasks.value
    .filter((t) => t.runId === id)
    .sort((a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime())
})

const failedTasks = computed(() => runTasks.value.filter((t) => t.status === 'failed'))
const runningTasks = computed(() =>
  runTasks.value.filter((t) => t.status === 'running' || t.status === 'cancelling' || t.status === 'queued'),
)

const totals = computed(() => {
  const list = runTasks.value
  return {
    total: list.length,
    passed: list.filter((t) => t.status === 'completed').length,
    failed: list.filter((t) => t.status === 'failed').length,
    pending: list.filter((t) => t.status === 'running' || t.status === 'queued' || t.status === 'cancelling').length,
    skipped: list.filter((t) => t.status === 'cancelled').length,
  }
})

// ============ 操作 ============
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
  activeTab.value = 'diagnostics'
}

function goBack() {
  router.replace('/tabs/tasks')
}

const exporting = ref(false)
async function blobToDataURL(blob: Blob): Promise<string> {
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
    const r = run.value
    const unifiedLike = {
      id: r.id,
      startedAt: r.startedAt ?? r.createdAt,
      completedAt: r.completedAt,
      totalCases: runTasks.value.length,
      passed: totals.value.passed,
      failed: totals.value.failed,
      skipped: totals.value.skipped,
      results: runTasks.value.map((tk) => ({
        caseId: tk.id,
        status: (tk.status === 'completed' ? 'success' : tk.status === 'failed' ? 'failure' : 'skipped') as 'success' | 'failure' | 'skipped',
        error: tk.error,
        duration: tk.completedAt
          ? String(new Date(tk.completedAt).getTime() - new Date(tk.createdAt).getTime()) + 'ms'
          : undefined,
      })),
      workflowRun: r,
    } as any
    const zipBlob = await buildReportZip(unifiedLike, runTasks.value, t)
    const filename = `encvreport-${r.id.slice(0, 8)}-${new Date().toISOString().slice(0, 10)}.zip`
    const isNative = !!(window as any).Capacitor?.isNativePlatform?.()
    if (isNative) {
      const dataUrl = await blobToDataURL(zipBlob)
      await Share.share({
        title: filename,
        text: t('tasks.exportShareText', {
          runId: r.id.slice(0, 8),
          passed: String(totals.value.passed),
          failed: String(totals.value.failed),
        }),
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
  if (!runId.value) {
    router.replace('/tabs/tasks')
  }
})
</script>

<style scoped>
.group-detail-segment {
  --background: transparent;
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
