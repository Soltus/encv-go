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
            <ion-icon :icon="filterOutline" slot="icon-only" :color="activeFilterCount > 0 ? 'primary' : 'medium'"></ion-icon>
            <ion-badge v-if="activeFilterCount > 0" color="primary" class="filter-badge">{{ activeFilterCount }}</ion-badge>
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
            :run="run"
            :jobs="run.jobs"
            :total="totals.total"
            :passed="totals.passed"
            :failed="totals.failed"
            :pending="totals.pending"
            :skipped="totals.skipped"
            @select-job="onJobClick"
          />
        </div>
        <div v-else-if="activeTab === 'tasks'" class="tab-content">
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
          <ion-button fill="clear" size="small" @click="clearSelection()">
            <ion-icon :icon="closeOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
        <ion-title size="small" class="batch-title">
          {{ t('tasks.batchSelected', { count: String(selectedIds.size) }) }}
        </ion-title>
        <ion-buttons slot="end">
          <ion-button fill="clear" size="small" @click="batchRetrySelected" :title="t('tasks.batchRetry')">
            <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" @click="batchCancelSelected" :title="t('tasks.batchCancel')" color="warning">
            <ion-icon :icon="stopCircleOutline" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" @click="batchDeleteSelected" :title="t('tasks.batchDelete')" color="danger">
            <ion-icon :icon="trashOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-footer>

    <!-- 筛选 drawer（ion-modal 内嵌筛选 UI），绑到 useTaskFilter 状态 -->
    <ion-modal :is-open="showFilterDrawer" @did-dismiss="showFilterDrawer = false" class="filter-modal">
      <FilterDrawer
        :status="filterStatuses"
        :task-type="filterTypes"
        :plugin="filterPlugins"
        :available-plugins="availablePlugins"
        @update:status="onFilterStatus"
        @update:task-type="onFilterType"
        @update:plugin="onFilterPlugin"
        @reset="onFilterReset"
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
  IonIcon, IonSpinner, IonSearchbar, IonBadge, IonFooter,
  IonModal, alertController, modalController, toastController,
} from '@ionic/vue'
import { Share } from '@capacitor/share'
import { Filesystem, Directory } from '@capacitor/filesystem'
import {
  downloadOutline, alertCircleOutline, filterOutline, checkboxOutline,
  chevronDown, chevronForward, closeOutline, refreshOutline,
  stopCircleOutline, trashOutline,
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { useWorkflowTaskService } from '@/composables/useWorkflowTaskService'
import { useTasksList } from '@/composables/useTasksList'
import { useBatchOperations } from '@/composables/useBatchOperations'
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
const { tasks: allTasks, filteredTasks, searchQuery, filterStatuses, filterTypes, filterPlugins, activeFilterCount, clearFilters } = useTasksList()
const batchOps = useBatchOperations()

// ============ UI 局部状态（selection 是 per-view，不进 store） ============
const selectedIds = ref<Set<string>>(new Set())
const multiSelectMode = ref(false)
function toggleSelect(id: string) {
  if (selectedIds.value.has(id)) selectedIds.value.delete(id)
  else selectedIds.value.add(id)
  // 触发响应式（Set mutation 不会自动触发）
  selectedIds.value = new Set(selectedIds.value)
}
function clearSelection() {
  selectedIds.value = new Set()
}

// ============ 派生 ============
const availablePlugins = computed(() => {
  const set = new Set<string>()
  for (const t of allTasks.value) {
    if (t.pluginName) set.add(t.pluginName)
  }
  return Array.from(set).sort()
})

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

// ============ UI 局部状态 ============
const showFilterDrawer = ref(false)
const performanceSectionOpen = ref(false)

// 衍生数据
const run = computed(() => workflowService.getRun(runId.value))

// runTasks 走 useTasksList.filteredTasks（已经是 filter+search 应用后的列表）
//   在 GroupDetail 上下文：再筛一次只显示当前 run 的
const runTasks = computed<EncvTask[]>(() => {
  const id = runId.value
  if (!id) return []
  const list: EncvTask[] = filteredTasks.value
  if (list === allTasks.value) {
    // 快速路径：未应用 filter → 直接过滤 runId
    return allTasks.value
      .filter((t: EncvTask) => t.runId === id)
      .sort((a: EncvTask, b: EncvTask) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime())
  }
  // 已 filter：直接遍历（list 已较小）
  return list
    .filter((t: EncvTask) => t.runId === id)
    .sort((a: EncvTask, b: EncvTask) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime())
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

// ============ 多选 / 批量操作（local state + batchOps） ============
async function batchRetrySelected() {
  const ids = Array.from(selectedIds.value) as string[]
  await batchOps.batchRetry(ids)
  clearSelection()
}
async function batchCancelSelected() {
  const ids = Array.from(selectedIds.value) as string[]
  await batchOps.batchCancel(ids)
  clearSelection()
}
async function batchDeleteSelected() {
  const ids = Array.from(selectedIds.value) as string[]
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

// FilterDrawer 操作转发
function onFilterStatus(v: string[]) { filterStatuses.value = v as any }
function onFilterType(v: string[]) { filterTypes.value = v as any }
function onFilterPlugin(v: string[]) { filterPlugins.value = v }
function onFilterReset() { clearFilters() }

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

/**
 * 🆕 v2 架构：share / download 兜底
 *
 * 三级降级：
 * 1. Share.share({url: dataUrl}) — Capacitor 5 兼容，但 Android 11+ 拒收大 data URL
 * 2. a.download + blob URL — web 通用
 * 3. 错误抛出 → 上面 catch 捕获 → toast 提示
 */
async function shareOrDownloadFallback(blob: Blob, filename: string): Promise<void> {
  // 1) 试 Share.share({url: dataURL})（老 Capacitor 兼容）
  try {
    const reader = new FileReader()
    const dataUrl: string = await new Promise((resolve, reject) => {
      reader.onload = () => resolve(String(reader.result ?? ''))
      reader.onerror = () => reject(reader.error)
      reader.readAsDataURL(blob)
    })
    if ((window as any).Capacitor?.isNativePlatform?.()) {
      await Share.share({ url: dataUrl, dialogTitle: filename, title: filename })
      return
    }
  } catch (e) {
    console.warn('[shareOrDownloadFallback] Share.url 失败:', e)
  }
  // 2) a.download（web 通用）
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
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
      // 🆕 v2 架构：native 双轨 + 探测
      //   - 主路径：Filesystem.writeFile → content:// URI → Share.files（Android 11+ StrictMode 拒收大 data URL）
      //   - 兜底 1：Filesystem 抛 "plugin is not implemented on android" → Share.share({url: dataURL})
      //     （老 Capacitor 5 兼容，data URL 在 Share Intent 实际工作）
      //   - 兜底 2：Share 不可用 → a.download（web 视图）
      try {
        const base64 = await blobToBase64(zipBlob)
        const writeResult = await Filesystem.writeFile({
          path: filename,
          data: base64,
          directory: Directory.Cache,
        })
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
      } catch (nativeErr) {
        console.warn('[exportGroupReport] Filesystem 路径失败，fallback 到 Share.url / a.download:', nativeErr)
        await shareOrDownloadFallback(zipBlob, filename)
      }
    } else {
      await shareOrDownloadFallback(zipBlob, filename)
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
