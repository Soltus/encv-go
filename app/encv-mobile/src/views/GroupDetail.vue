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

      <!-- 🆕 2026-06-22 Q4：顶层 action bar（segment + 搜索 + 筛选 + 多选） -->
      <ion-toolbar class="top-action-bar">
        <ion-segment v-model="activeTab" mode="md" class="group-detail-segment">
          <ion-segment-button value="pipeline">
            <ion-label>{{ t('tasks.groupDetail.tabPipeline') }}</ion-label>
          </ion-segment-button>
          <ion-segment-button value="tasks">
            <ion-label>{{ t('tasks.groupDetail.tabTasks') }}</ion-label>
          </ion-segment-button>
        </ion-segment>

        <!-- tasks tab 才显示搜索 + 筛选 + 多选 -->
        <div v-if="activeTab === 'tasks'" class="top-actions">
          <ion-searchbar
            v-model="searchQuery"
            :placeholder="t('tasks.searchPlaceholder')"
            class="top-searchbar"
            :debounce="200"
          />
          <ion-button
            fill="clear"
            size="small"
            @click="showFilterDrawer = true"
            :title="t('tasks.filterTitle')"
            class="top-icon-btn"
          >
            <ion-icon :icon="filterOutline" slot="icon-only" :color="filterChipCount > 0 ? 'primary' : 'medium'"></ion-icon>
            <ion-badge v-if="filterChipCount > 0" color="primary" class="filter-badge">{{ filterChipCount }}</ion-badge>
          </ion-button>
          <ion-button
            fill="clear"
            size="small"
            @click="multiSelectMode = !multiSelectMode"
            :title="t('tasks.multiSelectTitle')"
            :color="multiSelectMode ? 'primary' : 'medium'"
            class="top-icon-btn"
          >
            <ion-icon :icon="checkboxOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </div>
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
        <!-- 🆕 Q4：PerformanceTab 折叠在 tasks tab 内（不再是独立 tab） -->
        <div v-if="activeTab === 'pipeline'" class="tab-content">
          <PipelineTab
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
        </div>
        <div v-else-if="activeTab === 'tasks'" class="tab-content">
          <!-- 已应用的筛选 chip 行 -->
          <div v-if="filterChipCount > 0" class="filter-chip-row">
            <ion-chip
              v-for="chip in activeFilterChips"
              :key="chip.key"
              @click="removeFilter(chip.key)"
              :title="t('tasks.filterRemoveChip')"
            >
              <ion-label>{{ chip.label }}</ion-label>
              <ion-icon :icon="closeCircle"></ion-icon>
            </ion-chip>
            <ion-button fill="clear" size="small" @click="clearFilters">
              {{ t('tasks.filterClearAll') }}
            </ion-button>
          </div>

          <TasksTab
            :run-tasks="filteredTasks"
            :multi-select-mode="multiSelectMode"
            :selected-ids="selectedIds"
            :search-query="searchQuery"
            @select-task="openTaskDetail"
            @toggle-select="toggleSelect"
            @open-performance="performanceSectionOpen = !performanceSectionOpen"
          />

          <!-- 🆕 Q4：性能汇总作为 collapsible section（仅当有数据时） -->
          <div v-if="runTasks.some((tk) => tk.performanceSummary)" class="performance-section-collapsible">
            <ion-button
              fill="clear"
              expand="block"
              @click="performanceSectionOpen = !performanceSectionOpen"
            >
              <ion-icon :icon="performanceSectionOpen ? chevronDown : chevronForward" slot="start"></ion-icon>
              {{ t('tasks.performance.sectionTitle') }}
            </ion-button>
            <PerformanceTab v-if="performanceSectionOpen" :run-tasks="runTasks" />
          </div>
        </div>
      </template>
    </ion-content>

    <!-- 🆕 Q4：底部 sticky 批量操作栏（仅在 multiSelectMode + 有选中时显示） -->
    <ion-footer v-if="multiSelectMode && selectedIds.size > 0" class="batch-action-bar">
      <ion-toolbar color="light">
        <ion-buttons slot="start">
          <ion-button fill="clear" size="small" @click="clearSelection">
            <ion-icon :icon="closeOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
        <ion-title size="small" class="batch-title">
          {{ t('tasks.batchSelected', { count: String(selectedIds.size) }) }}
        </ion-title>
        <ion-buttons slot="end">
          <ion-button fill="clear" size="small" @click="batchRetry" :title="t('tasks.batchRetry')">
            <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" @click="batchCancel" :title="t('tasks.batchCancel')" color="warning">
            <ion-icon :icon="stopCircleOutline" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" @click="batchDelete" :title="t('tasks.batchDelete')" color="danger">
            <ion-icon :icon="trashOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-footer>

    <!-- 🆕 Q4：筛选 drawer（ion-modal 内嵌筛选 UI） -->
    <ion-modal :is-open="showFilterDrawer" @did-dismiss="showFilterDrawer = false" class="filter-modal">
      <FilterDrawer
        v-model:status="filterStatus"
        v-model:task-type="filterTaskType"
        v-model:plugin="filterPlugin"
        :available-plugins="availablePlugins"
        @reset="resetFilters"
        @apply="showFilterDrawer = false"
      />
    </ion-modal>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonContent, IonButtons,
  IonBackButton, IonButton, IonSegment, IonSegmentButton, IonLabel,
  IonIcon, IonSpinner, IonSearchbar, IonChip, IonBadge, IonFooter,
  IonModal, alertController, modalController, toastController,
} from '@ionic/vue'
import { Share } from '@capacitor/share'
import { Filesystem, Directory } from '@capacitor/filesystem'
import {
  downloadOutline, alertCircleOutline, filterOutline, checkboxOutline,
  closeCircle, chevronDown, chevronForward, closeOutline, refreshOutline,
  stopCircleOutline, trashOutline,
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { useWorkflowTaskService } from '@/composables/useWorkflowTaskService'
import { useTasksList } from '@/composables/useTasksList'
import { useBatchOperations } from '@/composables/useBatchOperations'
import { useTaskFiltering } from '@/composables/useTaskFiltering'
import type { EncvTask } from '@/api/encv'
import { getCalibration } from '@/api/encv'
import type { JobRun } from '@/lib/workflow/types'
import PipelineTab from '@/components/group-detail/PipelineTab.vue'
import TasksTab from '@/components/group-detail/TasksTab.vue'
import PerformanceTab from '@/components/group-detail/PerformanceTab.vue'
import FilterDrawer from '@/components/group-detail/FilterDrawer.vue'
import { buildReportZip } from '@/lib/buildReportZip'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const workflowService = useWorkflowTaskService()
const { tasks: allTasks } = useTasksList()
const batchOps = useBatchOperations()

// ============ 路由参数 ============
const runId = computed(() => decodeURIComponent(String(route.params.runId ?? '')))

// ============ Tab 状态（持久化）🆕 Q4：4→2 ============
const activeTab = ref<'pipeline' | 'tasks'>(
  (loadStoredTab() as 'pipeline' | 'tasks' | null) ?? 'pipeline',
)
const TAB_STORAGE_KEY = 'encv_group_detail_active_tab_v2'
function loadStoredTab(): string | null {
  try { return localStorage.getItem(TAB_STORAGE_KEY) } catch { return null }
}
watch(activeTab, (v) => {
  try { localStorage.setItem(TAB_STORAGE_KEY, v) } catch { /* silent */ }
})

// ============ 🆕 Q4：搜索 / 筛选 / 多选 状态 ============
const searchQuery = ref('')
const showFilterDrawer = ref(false)
const multiSelectMode = ref(false)
const selectedIds = ref<Set<string>>(new Set())
const performanceSectionOpen = ref(false)

// 筛选状态
const filterStatus = ref<Set<string>>(new Set())
const filterTaskType = ref<Set<string>>(new Set())
const filterPlugin = ref<Set<string>>(new Set())

// 衍生数据
const run = computed(() => workflowService.getRun(runId.value))

const runTasks = computed<EncvTask[]>(() => {
  const id = runId.value
  if (!id) return []
  return allTasks.value
    .filter((t) => t.runId === id)
    .sort((a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime())
})

const availablePlugins = computed<string[]>(() => {
  const set = new Set<string>()
  for (const t of runTasks.value) {
    if (t.pluginName) set.add(t.pluginName)
  }
  return Array.from(set).sort()
})

// 🆕 Q4：useTaskFiltering 应用筛选 + 搜索
const { filteredTasks, filterChipCount, activeFilterChips } = useTaskFiltering({
  tasks: runTasks,
  searchQuery,
  statusFilter: filterStatus,
  taskTypeFilter: filterTaskType,
  pluginFilter: filterPlugin,
})

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
  activeTab.value = 'tasks'
  // 默认打开后筛选到 failed + 选中当前 job 的 task
}

function goBack() {
  router.replace('/tabs/tasks')
}

// ============ 🆕 Q4：多选 / 批量操作 ============
function toggleSelect(taskId: string) {
  const next = new Set(selectedIds.value)
  if (next.has(taskId)) next.delete(taskId)
  else next.add(taskId)
  selectedIds.value = next
}

function clearSelection() {
  selectedIds.value = new Set()
  multiSelectMode.value = false
}

async function batchRetry() {
  const ids = Array.from(selectedIds.value)
  await batchOps.batchRetry(ids)
  clearSelection()
}

async function batchCancel() {
  const ids = Array.from(selectedIds.value)
  await batchOps.batchCancel(ids)
  clearSelection()
}

async function batchDelete() {
  const ids = Array.from(selectedIds.value)
  const confirm = await alertController.create({
    header: t('tasks.batchDeleteConfirmHeader'),
    message: t('tasks.batchDeleteConfirmMessage', { count: String(ids.length) }),
    buttons: [
      { text: t('common.cancel'), role: 'cancel' },
      { text: t('common.confirm'), role: 'destructive', handler: async () => {
        await batchOps.batchDelete(ids)
        clearSelection()
      } },
    ],
  })
  await confirm.present()
}

// ============ 🆕 Q4：筛选操作 ============
function removeFilter(key: string) {
  if (key.startsWith('status:')) filterStatus.value.delete(key.slice(7))
  else if (key.startsWith('type:')) filterTaskType.value.delete(key.slice(5))
  else if (key.startsWith('plugin:')) filterPlugin.value.delete(key.slice(7))
  // 触发响应式更新
  filterStatus.value = new Set(filterStatus.value)
  filterTaskType.value = new Set(filterTaskType.value)
  filterPlugin.value = new Set(filterPlugin.value)
}

function clearFilters() {
  filterStatus.value = new Set()
  filterTaskType.value = new Set()
  filterPlugin.value = new Set()
}

function resetFilters() {
  clearFilters()
}

// ============ 导出报告（🆕 Q7C native Filesystem + web download） ============
const exporting = ref(false)
async function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = String(reader.result ?? '')
      // data:application/zip;base64,XXXX → XXXX
      const idx = result.indexOf(',')
      resolve(idx >= 0 ? result.slice(idx + 1) : result)
    }
    reader.onerror = () => reject(reader.error ?? new Error('blobToBase64 failed'))
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
    const zipBlob = await buildReportZip(unifiedLike, runTasks.value, t, {
      calibration: await getCalibration().catch(() => null),
    })
    const filename = `encvreport-${r.id.slice(0, 8)}-${new Date().toISOString().slice(0, 10)}.zip`
    const isNative = !!(window as any).Capacitor?.isNativePlatform?.()
    if (isNative) {
      // 🆕 Q7C：native 走 Filesystem → content:// URI → Share.files
      const base64 = await blobToBase64(zipBlob)
      const writeResult = await Filesystem.writeFile({
        path: filename,
        data: base64,
        directory: Directory.Cache,
      })
      // writeResult.uri 是 string | undefined，兜底重读
      const fileUri = (writeResult.uri ?? `content://${filename}`) as string
      await Share.share({
        title: filename,
        text: t('tasks.exportShareText', {
          runId: r.id.slice(0, 8),
          passed: String(totals.value.passed),
          failed: String(totals.value.failed),
        }),
        files: [fileUri],
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
    // 🆕 Q7C：失败提示更详细（错误分类 + 建议）
    const message = String((error as any)?.message ?? error)
    const hint = message.includes('Unsupported')
      ? t('tasks.exportFailedUnsupportedHint')
      : message.includes('permission') || message.includes('Permission')
        ? t('tasks.exportFailedPermissionHint')
        : ''
    const alert = await alertController.create({
      header: t('tasks.exportFailedHeader'),
      message: hint ? `${message}\n\n${hint}` : message,
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
.top-action-bar {
  --padding-start: 8px;
  --padding-end: 8px;
}
.top-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 0;
}
.top-searchbar {
  flex: 1;
  --background: var(--ion-color-light, #f4f5f8);
  --border-radius: 16px;
  --box-shadow: none;
  padding: 0;
  min-height: 32px;
}
.top-icon-btn {
  position: relative;
  margin: 0;
}
.filter-badge {
  position: absolute;
  top: 0;
  right: 0;
  font-size: 10px;
  min-width: 16px;
  height: 16px;
}
.tab-content {
  padding-bottom: 60px;
}
.filter-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 4px 8px;
  align-items: center;
}
.filter-chip-row ion-chip {
  margin: 0;
  height: 24px;
  font-size: 11px;
}
.performance-section-collapsible {
  margin: 12px 0;
  padding: 0 8px;
}
.batch-action-bar ion-toolbar {
  --background: var(--ion-color-light, #f4f5f8);
}
.batch-title {
  font-size: 14px;
  font-weight: 600;
}
.filter-modal {
  --height: 80%;
  --border-radius: 12px;
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
