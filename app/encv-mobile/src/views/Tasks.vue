<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('tasks.title') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button fill="clear" size="small" @click="toggleSort" class="toolbar-btn">
            <ion-icon :icon="sortBy === 'activity' ? sync : timer" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" @click="handleClearCompleted" class="toolbar-btn" :disabled="!hasCompletedTasks">
            <ion-icon :icon="trashBin" slot="icon-only" color="danger"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>

      <ion-toolbar v-if="showSearch">
        <ion-searchbar
          :value="searchQuery"
          @ionInput="onSearchInput"
          :placeholder="t('tasks.searchPlaceholder')"
          show-cancel-button="focus"
          @ionCancel="showSearch = false; searchQuery = ''"
          :debounce="200"
          class="task-searchbar"
        ></ion-searchbar>
      </ion-toolbar>

      <ion-toolbar v-if="showFilters" class="filter-toolbar">
        <div class="filter-chips">
          <ion-chip :color="filterPlugins.length > 0 ? 'primary' : 'medium'" @click="openPluginPopover($event)">
            <ion-icon :icon="extensionPuzzle" size="small"></ion-icon>
            <ion-label>{{ getPluginChipLabel() }}</ion-label>
            <ion-icon :icon="chevronDown" size="small"></ion-icon>
          </ion-chip>
          <ion-chip :color="filterTypes.length > 0 ? 'primary' : 'medium'" @click="openTypePopover($event)">
            <ion-icon :icon="swapVertical" size="small"></ion-icon>
            <ion-label>{{ getTypeChipLabel() }}</ion-label>
            <ion-icon :icon="chevronDown" size="small"></ion-icon>
          </ion-chip>
          <ion-chip :color="filterStatuses.length > 0 ? 'primary' : 'medium'" @click="openStatusPopover($event)">
            <ion-icon :icon="funnel" size="small"></ion-icon>
            <ion-label>{{ getStatusChipLabel() }}</ion-label>
            <ion-icon :icon="chevronDown" size="small"></ion-icon>
          </ion-chip>
        </div>
      </ion-toolbar>

      <ion-popover
        :is-open="pluginPopoverOpen"
        :event="pluginPopoverEvent"
        @didDismiss="pluginPopoverOpen = false"
        side="bottom"
        alignment="start"
      >
        <div class="popover-filter-content">
          <div class="popover-filter-title">{{ t('tasks.filterByPlugin') }}</div>
          <ion-item
            v-for="plugin in availablePlugins"
            :key="plugin"
            lines="none"
            class="popover-filter-item"
            @click="togglePluginFilter(plugin)"
          >
            <ion-checkbox
              :checked="filterPlugins.includes(plugin)"
              slot="start"
              @ionChange="togglePluginFilter(plugin)"
            ></ion-checkbox>
            <ion-label>{{ plugin }}</ion-label>
          </ion-item>
          <div v-if="availablePlugins.length === 0" class="popover-empty">{{ t('tasks.noPluginsFound') }}</div>
        </div>
      </ion-popover>

      <ion-popover
        :is-open="typePopoverOpen"
        :event="typePopoverEvent"
        @didDismiss="typePopoverOpen = false"
        side="bottom"
        alignment="start"
      >
        <div class="popover-filter-content">
          <div class="popover-filter-title">{{ t('tasks.filterByType') }}</div>
          <ion-item lines="none" class="popover-filter-item" @click="toggleTypeFilter('encrypt')">
            <ion-checkbox :checked="filterTypes.includes('encrypt')" slot="start" @ionChange="toggleTypeFilter('encrypt')"></ion-checkbox>
            <ion-label>{{ t('tasks.encrypt') }}</ion-label>
          </ion-item>
          <ion-item lines="none" class="popover-filter-item" @click="toggleTypeFilter('decrypt')">
            <ion-checkbox :checked="filterTypes.includes('decrypt')" slot="start" @ionChange="toggleTypeFilter('decrypt')"></ion-checkbox>
            <ion-label>{{ t('tasks.decrypt') }}</ion-label>
          </ion-item>
        </div>
      </ion-popover>

      <ion-popover
        :is-open="statusPopoverOpen"
        :event="statusPopoverEvent"
        @didDismiss="statusPopoverOpen = false"
        side="bottom"
        alignment="start"
      >
        <div class="popover-filter-content">
          <div class="popover-filter-title">{{ t('tasks.filterByStatus') }}</div>
          <ion-item v-for="s in statusOptions" :key="s" lines="none" class="popover-filter-item" @click="toggleStatusFilter(s)">
            <ion-checkbox :checked="filterStatuses.includes(s)" slot="start" @ionChange="toggleStatusFilter(s)"></ion-checkbox>
            <ion-label>{{ getStatusLabel(s) }}</ion-label>
          </ion-item>
        </div>
      </ion-popover>
    </ion-header>

    <ion-content>
      <ion-refresher slot="fixed" @ionRefresh="handleRefresh">
        <ion-refresher-content></ion-refresher-content>
      </ion-refresher>

      <div class="toolbar-actions">
        <ion-button fill="clear" size="small" @click="showSearch = !showSearch" class="action-btn">
          <ion-icon :icon="search" slot="icon-only"></ion-icon>
        </ion-button>
        <ion-button fill="clear" size="small" @click="showFilters = !showFilters" class="action-btn">
          <ion-icon :icon="funnel" slot="icon-only" :color="hasActiveFilters ? 'primary' : undefined"></ion-icon>
        </ion-button>
      </div>

      <div v-if="loading" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('tasks.loading') }}</p>
      </div>

      <div v-else-if="filteredTasks.length === 0 && tasks.length > 0" class="empty-state">
        <ion-icon :icon="search" class="empty-icon"></ion-icon>
        <h3>{{ t('tasks.noMatchingTasks') }}</h3>
        <p>{{ t('tasks.noMatchingTasksDesc') }}</p>
        <ion-button fill="clear" size="small" @click="clearFilters">{{ t('tasks.clearFilters') }}</ion-button>
      </div>

      <div v-else-if="tasks.length === 0" class="empty-state">
        <ion-icon :icon="checkmarkCircle" class="empty-icon"></ion-icon>
        <h3>{{ t('tasks.noTasks') }}</h3>
        <p>{{ t('tasks.noTasksDesc') }}</p>
      </div>

      <ion-list v-else>
        <!-- 🆕 2026-06-10 修复 v4：可见的调试计数（让用户能确认 grouping 是否在工作） -->
        <div class="grouping-debug-bar">
          <span>共 <strong>{{ tasks.length }}</strong> 个 task</span>
          <span class="grouping-debug-sep">·</span>
          <span><strong>{{ debugGroupCount }}</strong> 个 run 分组</span>
          <span class="grouping-debug-sep">·</span>
          <span><strong>{{ debugSingletonCount }}</strong> 个单条</span>
          <span class="grouping-debug-sep">·</span>
          <span><strong>{{ debugByTriggeredBy.automation }}</strong> auto / <strong>{{ debugByTriggeredBy.ai_agent }}</strong> ai / <strong>{{ debugByTriggeredBy.user }}</strong> user</span>
          <ion-button size="small" fill="clear" @click="resetGrouping" class="grouping-reset-btn">
            <ion-icon :icon="sync" slot="start"></ion-icon>
            重置分组
          </ion-button>
          <ion-button size="small" fill="clear" @click="showAutomationReports" class="grouping-reset-btn" title="查看所有自动化测试历史报告（localStorage 持久化）">
            <ion-icon :icon="archiveOutline" slot="start"></ion-icon>
            查看报告
          </ion-button>
        </div>
        <template v-for="item in displayedItems" :key="item.key">
          <!-- 🆕 2026-06-10 修复：自动化测试 / AI agent 任务组折叠 -->
          <!-- 历史：自动化测试一次跑 N 个用例 → 污染 task 列表（用户截图的"浪费屏幕空间"）-->
          <!-- 修复：连续 ≥2 个 triggeredBy != 'user' 的 task → 折叠成 1 张 group card -->
          <!--       点 group card 右侧 chevron 展开/折叠详情 -->
          <!-- 🆕 2026-06-10 修复 v2：2 级嵌套 — group 展开时按 pluginName 插 plugin_section 段头 -->
          <ion-item-sliding v-if="item.kind === 'group'">
            <ion-item button detail @click="toggleTaskGroup(item.groupKey!)" :class="['task-group-card', `group-tone-${item.tone}`]">
              <div class="group-icon-bubble" :class="`group-tone-${item.tone}`" slot="start">
                <ion-icon :icon="item.tone === 'ai_agent' ? hardwareChipOutline : cogOutline"></ion-icon>
              </div>
              <ion-label>
                <h2 class="group-title">
                  {{ item.tone === 'ai_agent' ? t('tasks.triggeredBy_ai_agent') : t('tasks.triggeredBy_automation') }}
                  <span class="group-count">· {{ item.tasks.length }} {{ t('tasks.tasksCount') }}</span>
                </h2>
                <p class="card-meta-row group-meta-row">
                  <ion-badge v-if="item.summary.passed > 0" color="success" class="status-badge">
                    <ion-icon :icon="checkmarkCircle" class="badge-icon"></ion-icon>
                    {{ item.summary.passed }}
                  </ion-badge>
                  <ion-badge v-if="item.summary.failed > 0" color="danger" class="status-badge">
                    <ion-icon :icon="closeCircle" class="badge-icon"></ion-icon>
                    {{ item.summary.failed }}
                  </ion-badge>
                  <ion-badge v-if="item.summary.running > 0" color="warning" class="status-badge">
                    <ion-spinner name="dots" class="badge-spinner"></ion-spinner>
                    {{ item.summary.running }}
                  </ion-badge>
                  <ion-badge v-if="item.summary.pending > 0" color="medium" class="status-badge">
                    {{ item.summary.pending }}
                  </ion-badge>
                </p>
                <div class="group-progress-track">
                  <div
                    class="group-progress-fill"
                    :style="{ width: item.summary.percent + '%' }"
                  ></div>
                </div>
                <p class="task-time-info group-time-info">
                  <span class="time-created">{{ formatDateTime(item.summary.latestCreatedAt) }}</span>
                  <span class="group-percent-label">{{ item.summary.percent }}%</span>
                </p>
              </ion-label>
              <ion-button
                slot="end"
                fill="clear"
                size="small"
                @click.stop="toggleTaskGroup(item.groupKey!)"
                :title="isTaskGroupExpanded(item.groupKey!) ? t('tasks.collapse') : t('tasks.expand')"
                class="group-chevron-btn"
              >
                <ion-icon
                  :icon="isTaskGroupExpanded(item.groupKey!) ? chevronBack : chevronForward"
                  slot="icon-only"
                ></ion-icon>
              </ion-button>
            </ion-item>
          </ion-item-sliding>

          <!-- 🆕 2026-06-10 修复 v2：2 级嵌套的 sub_section 段头 -->
          <!-- 在 group card 展开后插入，按 section 维度（v5）桶里每个 section 1 个段头 -->
          <!-- 段头下方是该 section 的所有 task 卡片（紧随其后的 kind='task' 项） -->
          <!-- 🆕 2026-06-11 v5：sub_section 可独立折叠 + sticky 滚动冻结 -->
          <!--
            🆕 2026-06-10 修复 v3：改用 <ion-item> 而不是裸 <div>
            历史：<div> 在 <ion-list> 里 → Ionic 把 <div> 当普通子节点，但 <ion-list> 有自己的
              列表 CSS（display 规则、滚动容器、虚拟化），<div> 子节点不参与，导致：
              - 段头高度计算异常（被压成 0 或被列表 padding 吃掉）
              - 任务卡和段头之间没分隔线
              - "插件没正确识别，任务依旧全部平铺"
            修复：用 <ion-item> + 自定义 class，禁用 button/clickable，让 Ionic 当作「装饰 item」
            🆕 2026-06-11 v5：恢复 button + clickable（要可折叠），用 sticky CSS 解决冻结
          -->
          <ion-item
            v-else-if="item.kind === 'sub_section_header'"
            button
            :detail="false"
            @click="toggleSubSection(item.subKey)"
            :class="['sub-section-header', `sub-dim-${item.meta.dimension}`, `sub-tone-${item.meta.tone}`, { 'is-sticky': true, 'is-collapsed': item.isCollapsed }]"
            :lines="'none'"
          >
            <div class="sub-section-icon-bubble" :class="`sub-tone-${item.meta.tone}`" slot="start">
              <ion-icon :icon="getSubSectionIcon(item.meta.icon)"></ion-icon>
            </div>
            <ion-label class="sub-section-label">
              <h3 class="sub-section-name">{{ item.meta.label }}</h3>
              <p class="sub-section-count">· {{ item.tasks.length }} {{ t('tasks.tasksCount') }}</p>
            </ion-label>
            <div class="sub-section-badges" slot="end">
              <ion-badge v-if="item.subSummary.passed > 0" color="success" class="status-badge">
                <ion-icon :icon="checkmarkCircle" class="badge-icon"></ion-icon>
                {{ item.subSummary.passed }}
              </ion-badge>
              <ion-badge v-if="item.subSummary.failed > 0" color="danger" class="status-badge">
                <ion-icon :icon="closeCircle" class="badge-icon"></ion-icon>
                {{ item.subSummary.failed }}
              </ion-badge>
              <ion-badge v-if="item.subSummary.running > 0" color="warning" class="status-badge">
                <ion-spinner name="dots" class="badge-spinner"></ion-spinner>
                {{ item.subSummary.running }}
              </ion-badge>
              <ion-badge v-if="item.subSummary.pending > 0" color="medium" class="status-badge">
                {{ item.subSummary.pending }}
              </ion-badge>
            </div>
            <ion-button
              slot="end"
              fill="clear"
              size="small"
              :title="item.isCollapsed ? t('tasks.expand') : t('tasks.collapse')"
              class="sub-section-chevron-btn"
              @click.stop="toggleSubSection(item.subKey)"
            >
              <ion-icon
                :icon="item.isCollapsed ? chevronForward : chevronDown"
                slot="icon-only"
              ></ion-icon>
            </ion-button>
            <div class="sub-section-progress-track">
              <div
                class="sub-section-progress-fill"
                :style="{ width: item.subSummary.percent + '%' }"
              ></div>
            </div>
          </ion-item>

          <ion-item-sliding v-else>
            <ion-item
              @click="openTaskDetail(item.task)"
              button
              detail
              v-show="!item.subKey || !isSubSectionCollapsed(item.subKey)"
            >
              <ion-icon
                :icon="getTaskIcon(item.task)"
                :color="getTaskColor(item.task)"
                slot="start"
              ></ion-icon>
              <ion-label>
                <h2>{{ getTaskName(item.task) }}</h2>
                <p class="card-meta-row">
                  <span class="task-id">#{{ item.task.id.slice(0, 6) }}</span>
                  <ion-badge :color="getStatusColor(item.task.status)" class="status-badge">
                    {{ getStatusLabel(item.task.status) }}
                  </ion-badge>
                  <span class="task-type">{{ item.task.type === 'encrypt' ? t('tasks.encrypt') : t('tasks.decrypt') }}</span>
                  <ion-badge v-if="item.task.pluginName" color="primary" class="plugin-badge">
                    {{ item.task.pluginName }}
                  </ion-badge>
                  <ion-badge
                    v-if="getTriggeredBy(item.task.id) !== 'user'"
                    :color="getTriggeredByColor(item.task.id)"
                    class="triggered-by-badge"
                    :title="t('tasks.triggeredBy') + ': ' + t('tasks.triggeredBy_' + getTriggeredBy(item.task.id))"
                  >
                    <ion-icon :icon="getTriggeredByIcon(item.task.id)" class="triggered-by-icon"></ion-icon>
                    {{ t('tasks.triggeredBy_' + getTriggeredBy(item.task.id)) }}
                  </ion-badge>
                </p>
                <p class="task-time-info">
                  <span class="time-created">{{ formatDateTime(item.task.createdAt) }}</span>
                  <span v-if="getTaskDuration(item.task)" class="time-duration">{{ getTaskDuration(item.task) }}</span>
                </p>
                <div v-if="item.task.status === 'running' || item.task.status === 'cancelling'" class="progress-section">
                  <ion-progress-bar
                    :value="item.task.progress / 100"
                    :class="['task-progress', { 'progress-cancelling': item.task.status === 'cancelling' }]"
                  ></ion-progress-bar>
                  <div class="progress-detail">
                    <span v-if="item.task.phase" class="phase-label">{{ getPhaseLabel(item.task.phase) }}</span>
                    <span class="progress-percent">{{ item.task.progress }}%</span>
                    <span v-if="item.task.speed" class="speed-label">{{ item.task.speed }}</span>
                    <span v-if="item.task.eta" class="eta-label">{{ t('tasks.eta') }} {{ item.task.eta }}</span>
                  </div>
                </div>
                <div v-if="item.task.status === 'completed'" class="completed-info">
                  <ion-icon :icon="checkmarkCircle" color="success" class="completed-icon"></ion-icon>
                  <span class="completed-text">{{ t('tasks.phaseCompleted') }}</span>
                  <span v-if="item.task.containerVersion" class="container-version">{{ formatContainerVersion(item.task.containerVersion) }}</span>
                </div>
                <div v-if="item.task.warning" class="task-warning" @click="toggleWarningDetail(item.task)">
                  <ion-icon :icon="warningOutline" class="warning-icon"></ion-icon>
                  <span class="task-warning-text">{{ item.task.warning }}</span>
                </div>
                <div v-if="expandedWarningDetail === item.task.id && item.task.warningDetail" class="task-warning-detail">
                  <pre>{{ formatWarningDetail(item.task.warningDetail) }}</pre>
                </div>
                <p v-if="isPasswordError(item.task)" class="task-error password-error">
                  <ion-icon :icon="lockClosed"></ion-icon>
                  {{ t('tasks.passwordErrorHint') }}
                </p>
                <p v-else-if="item.task.error" class="task-error">{{ item.task.error }}</p>
              </ion-label>
              <ion-button
                v-if="item.task.status === 'running'"
                slot="end"
                fill="clear"
                color="warning"
                size="small"
                @click="cancelTaskById(item.task.id)"
              >
                <ion-icon :icon="closeCircle" slot="icon-only"></ion-icon>
              </ion-button>
              <ion-spinner
                v-if="item.task.status === 'cancelling'"
                slot="end"
                name="crescent"
                color="warning"
                class="cancelling-spinner"
              ></ion-spinner>
            </ion-item>
            <ion-item-options side="end">
              <ion-item-option
                v-if="item.task.status === 'queued'"
                color="warning"
                @click="cancelTaskById(item.task.id)"
              >
                {{ t('tasks.cancel') }}
              </ion-item-option>
              <ion-item-option
                v-if="item.task.status === 'failed'"
                color="primary"
                @click="retryTaskById(item.task.id)"
              >
                {{ t('tasks.retry') }}
              </ion-item-option>
              <ion-item-option
                v-if="item.task.status === 'completed' || item.task.status === 'failed' || item.task.status === 'cancelled'"
                color="danger"
                @click="removeTaskById(item.task.id)"
              >
                {{ t('tasks.remove') }}
              </ion-item-option>
            </ion-item-options>
          </ion-item-sliding>
        </template>
      </ion-list>

      <ion-fab vertical="bottom" horizontal="end" slot="fixed">
        <ion-fab-button @click="openNewTask()">
          <ion-icon :icon="add"></ion-icon>
        </ion-fab-button>
      </ion-fab>

    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { onIonViewWillEnter } from '@ionic/vue'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonContent,
  IonRefresher, IonRefresherContent, IonList, IonItem,
  IonItemSliding, IonItemOptions, IonItemOption, IonIcon,
  IonLabel, IonBadge, IonProgressBar, IonFab, IonFabButton,
  IonSpinner, IonButton, IonButtons, IonSearchbar, IonChip,
  IonPopover, IonCheckbox, alertController, modalController,
} from '@ionic/vue'
import {
  add, closeCircle, checkmarkCircle, timer, sync,
  warningOutline, lockClosed, search, funnel, trashBin,
  extensionPuzzle, swapVertical, chevronDown,
  hardwareChipOutline, cogOutline, person, chevronForward, chevronBack,
  folderOutline, ellipsisHorizontalCircleOutline, archiveOutline,
} from 'ionicons/icons'
import { useRoute, useRouter } from 'vue-router'
import type { EncvTask, TaskType } from '@/api/encv'
import { clearCompletedTasks } from '@/api/encv'
import { useI18n } from '@/composables/useI18n'
import { formatDateTime } from '@/composables/useDateFormat'
import { showToast } from '@/composables/useToast'
import { useNewTaskModal } from '@/composables/useNewTaskModal'
import { useTasksList } from '@/composables/useTasksList'
import { useTaskEventBridge } from '@/composables/useTaskEventBridge'
import { useWorkflowTaskService } from '@/composables/useWorkflowTaskService'
import { getTriggeredBy, getRunIdForTask } from '@/composables/useTaskTrigger'
import { formatContainerVersion } from '@/constants/containerVersion'
import {
  deriveSubSection,
  type SectionDimension,
  type SectionMeta,
} from '@/composables/useSectionDerivation'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { openNewTask } = useNewTaskModal()

const {
  tasks, loading, expandedWarningDetail, sortBy,
  showSearch, searchQuery, showFilters,
  filterPlugins, filterTypes, filterStatuses, statusOptions,
  pluginPopoverOpen, typePopoverOpen, statusPopoverOpen,
  pluginPopoverEvent, typePopoverEvent, statusPopoverEvent,
  availablePlugins, hasActiveFilters, hasCompletedTasks, filteredTasks,
  fetchTasks, refresh,
  openPluginPopover, openTypePopover, openStatusPopover,
  togglePluginFilter, toggleTypeFilter, toggleStatusFilter, clearFilters,
  onSearchInput, toggleSort,
  applyTaskUpdate, applyTaskProgress, applyTaskCreated, applyTaskCompleted,
  cancelTaskById, retryTaskById, removeTaskById, clearCompletedWithConfirm,
  getTaskName, getTaskDuration,
  getPluginChipLabel, getTypeChipLabel, getStatusChipLabel, getStatusLabel,
  isPasswordError, toggleWarningDetail, formatWarningDetail,
  getTaskIcon, getTaskColor, getStatusColor, getPhaseLabel,
} = useTasksList()

// 🆕 2026-06-18 Task 16：统一工作流任务服务
// 用途：
//   1. showAutomationReports 从 workflowTaskService.runs 读取历史运行记录（替代旧 encv_automation_results_v1 key）
//   2. 内部通过 useTaskEventBridge 订阅 4 件套 WS 事件，维护 currentRun / runs 状态
//   3. Tasks.vue 仍保留 useTaskEventBridge 4 件套回调（applyTask*）以实时更新 tasks ref
//      —— 因为 Tasks.vue 显示所有任务（含非 workflow 的用户单任务），不能只依赖 workflowTaskService
//      workflowTaskService 只追踪 workflow run 内的 task，单任务不归它管
const workflowTaskService = useWorkflowTaskService()

useTaskEventBridge({
  onUpdate: applyTaskUpdate,
  onProgress: applyTaskProgress,
  onCreate: applyTaskCreated,
  onComplete: applyTaskCompleted,
  onRefresh: fetchTasks,
})

// 任务触发者标签 helpers — Tasks.vue 直接用 useTaskTrigger（因为这是 task 显示的主视图）
function getTriggeredByColor(taskId: string): string {
  const v = getTriggeredBy(taskId)
  return v === 'automation' ? 'primary' : v === 'ai_agent' ? 'secondary' : 'medium'
}
function getTriggeredByIcon(taskId: string): string {
  const v = getTriggeredBy(taskId)
  return v === 'automation' ? cogOutline : v === 'ai_agent' ? hardwareChipOutline : person
}

// 🆕 2026-06-11 v5：sub_section icon name → ionicon 映射
// 升级指南：未来加 dimension 在 SECTION_META 加 icon name 字符串，
//   然后在这个 map 加一条就行，不用动模板
const SUB_SECTION_ICON_MAP: Record<string, string> = {
  'extension-puzzle': extensionPuzzle,
  'swap-vertical': swapVertical,
  'folder': folderOutline,
  'ellipsis-horizontal-circle': ellipsisHorizontalCircleOutline,
}
function getSubSectionIcon(name: string): string {
  return SUB_SECTION_ICON_MAP[name] ?? ellipsisHorizontalCircleOutline
}

async function openTaskDetail(task: EncvTask) {
  const { default: TaskDetailModal } = await import('@/components/TaskDetailModal.vue')
  const modal = await modalController.create({
    component: TaskDetailModal,
    componentProps: { task },
    cssClass: 'task-detail-modal',
  })
  await modal.present()
  const { data, role } = await modal.onDidDismiss()
  if (role === 'dismiss' && data) {
    if (data.action === 'cancel') await cancelTaskById(data.id)
    else if (data.action === 'retry') await retryTaskById(data.id)
    else if (data.action === 'remove') await removeTaskById(data.id)
  }
}

async function handleRefresh(event: CustomEvent) {
  await refresh()
  ;(event.target as any)?.complete?.()
}

async function handleClearCompleted() {
  const completedCount = await clearCompletedWithConfirm()
  if (!completedCount) return
  const alert = await alertController.create({
    header: t('tasks.clearConfirmTitle'),
    message: t('tasks.clearConfirmMessage', { count: String(completedCount) }),
    buttons: [
      { text: t('tasks.cancel'), role: 'cancel' },
      {
        text: t('tasks.clearConfirm'),
        role: 'destructive',
        handler: async () => {
          try {
            const result = await clearCompletedTasks()
            showToast({ message: t('tasks.cleared', { count: String(result.removed) }), duration: 2000, color: 'success' })
            await fetchTasks()
          } catch {
            showToast({ message: t('tasks.clearFailed'), duration: 2000, color: 'danger' })
          }
        },
      },
    ],
  })
  await alert.present()
}

// 🆕 2026-06-10 修复：自动化测试 / AI agent 任务组折叠
// 历史：useAutomationTests.runTests() 串行 for 循环逐个调 createTask()，
//   一次跑 N 个用例 → 后端 task 列表被 N 张 task 卡片污染
//   （用户截图"浪费屏幕空间"）。
// 修复思路：纯前端 UI 折叠，**不动后端 API**（后端根本没有 GroupID 概念）。
//   - 扫描 filteredTasks，连续 ≥2 个 triggeredBy != 'user' 的 task → 折叠成 1 张 group card
//   - 用户点 chevron 展开/折叠详情（展开时插入 N 张原始 task card）
//   - 单个非用户 task 不折叠（避免 UI 抖动）
//   - 用户手动搜/筛不受影响（filteredTasks 是折叠前数据）
const GROUP_FOLD_THRESHOLD = 2

// 🆕 2026-06-10 修复 v2：2 级嵌套（Run group card → plugin sub-section → N 张 task 卡片）
// 历史：group 内部只展示扁平 task 列表 → 用户：「单个插件任务下面聚合子任务的显示」
// 修复：group 展开时按 pluginName 再分桶，每个 plugin 渲染一个 plugin_section 段头
//       段头下方是该 plugin 的所有 task 卡片
//
// 🆕 2026-06-10 修复 v3：plugin_section 携带 runId
// 历史：plugin_section key = `plugin-section-${tone}-${pluginName}-${tasks[0]?.id}`
//   → 第一个 task 变化（如新增 / 排序调整）就触发整段 Vue 重建 → 闪烁/消失
// 修复：key 改用 `plugin-section-${runId}-${pluginName}`（runId+pluginName 都稳定）
// 🆕 2026-06-11 修复 v5：section 维度抽象（架构向上兼容）
// 历史：buildPluginSectionItem 硬编码 pluginName → 未来加「下载 / 同步 / 清理」等
//   非 plugin 任务时，task.pluginName 为空 → 全部归到 '(unknown plugin)' → 烂成一锅
// 修复：引入 SubSectionKey 抽象维度（dimension + value），按 task 属性动态派生
//   - 当前支持 4 种 dimension：plugin / type / category / none
//   - 未来加新维度：只需要在 SECTION_META 加一行 + deriveSubSection 加一个 case
//   - task 没 pluginName → fallback 到 'none'（不会丢失，归到「其他任务」section）
//
// 🆕 2026-06-18 Task 6：deriveSubSection / SectionDimension / SectionMeta 已抽取到
//   @/composables/useSectionDerivation，这里只保留 Tasks.vue 专用的 UI 元数据
//   （SubSectionMeta + SECTION_META + sectionKeyToString + buildSubSectionMeta）。

interface SubSectionMeta {
  dimension: SectionDimension
  value: string
  /** 显示名（默认就是 value，特殊 case 可覆盖） */
  label: string
  /** ionicon 名称（用于 sub_section_header 左侧 icon bubble） */
  icon: string
  /** CSS tone class（决定颜色/背景） */
  tone: SectionDimension
}

const SECTION_META: Record<SectionDimension, { icon: string; toneClass: string }> = {
  plugin: { icon: 'extension-puzzle', toneClass: 'plugin' },
  type: { icon: 'swap-vertical', toneClass: 'type' },
  category: { icon: 'folder', toneClass: 'category' },
  none: { icon: 'ellipsis-horizontal-circle', toneClass: 'none' },
}

function sectionKeyToString(meta: SectionMeta): string {
  return `${meta.dimension}:${meta.key}`
}

/**
 * 🆕 2026-06-11 v5：从 task 派生 SubSection（架构核心）
 * 当前规则（按优先级）：
 *   1. task.pluginName 存在 → 'plugin' 维度（按插件分桶）
 *   2. 未来扩展：task.category / task.subType 等可在中间插入 case
 *   3. 都没 → 'none' 维度（统一归到「其他任务」section，不会丢失）
 *
 * 🆕 2026-06-18 Task 6：实际派生逻辑已迁移到 @/composables/useSectionDerivation
 *   deriveSubSection(task, dimension)。这里只保留「按 task 字段 pick 维度」的本地策略，
 *   因为 Tasks.vue 的维度选择是 per-task 的（不是 per-component），不适合用
 *   useSectionDerivation(dimension) 单维度 composable。
 *
 * 升级指南：未来加新维度时
 *   - SECTION_META 加一条（icon + toneClass）
 *   - pickSectionDimension 加一个 if 分支
 *   - i18n 加 'tasks.dimensionXxx' 文案
 *   不需要改 displayedItems / 模板 / CSS
 */
function pickSectionDimension(task: EncvTask): SectionDimension {
  if (task.pluginName) {
    return 'plugin'
  }
  // 未来扩展预留：
  // if (task.category) return 'category'
  // if (task.subType) return 'type'
  return 'none'
}

function buildSubSectionMeta(meta: SectionMeta): SubSectionMeta {
  const sectionMeta = SECTION_META[meta.dimension]
  // label 默认用 composable 派生的 label，特殊 case 可覆盖
  let label = meta.label
  if (meta.dimension === 'none' && meta.key === 'all') {
    label = '其他任务'  // fallback section 标签（i18n key 在模板里覆盖）
  }
  if (meta.dimension === 'type') {
    label = meta.key === 'encrypt' ? '加密任务' : meta.key === 'decrypt' ? '解密任务' : meta.label
  }
  return {
    dimension: meta.dimension,
    value: meta.key,
    label,
    icon: sectionMeta.icon,
    tone: meta.dimension,
  }
}

type DisplayItem =
  | {
      kind: 'group'
      key: string
      groupKey: string
      runId: string
      tone: 'automation' | 'ai_agent'
      tasks: EncvTask[]
      /** 内部 section 桶（用于 sub_section 展开时按桶渲染） */
      sections: Array<{ sectionKeyStr: string; meta: SubSectionMeta; tasks: EncvTask[] }>
      summary: { passed: number; failed: number; running: number; pending: number; percent: number; latestCreatedAt: string }
    }
  | {
      kind: 'sub_section_header'
      key: string
      subKey: string
      runId: string
      meta: SubSectionMeta
      tasks: EncvTask[]
      isCollapsed: boolean
      subSummary: { passed: number; failed: number; running: number; pending: number; percent: number }
    }
  | {
      kind: 'task'
      key: string
      task: EncvTask
      /** 所属 sub_section key（决定 v-show 是否隐藏） */
      subKey: string | null
      /** 所属 group key（决定 v-show 是否隐藏 — group 折叠时整段隐藏） */
      groupKey: string | null
    }

// 🆕 2026-06-10 修复 v2：expandedGroupKeys 持久化到 localStorage
// 历史：ref<Set<string>> 是组件级 state，组件 unmount/remount（如 tab 切换、抽屉开关）会丢
//   → 用户展开 group 后一切 tab 回来，发现 group 又折叠了 → 「展开后一会就消失了」
// 修复：localStorage 持久化 + 启动时恢复 + watch 同步
const EXPANDED_GROUPS_KEY = 'encv_tasks_expanded_groups_v1'
function loadExpandedGroups(): Set<string> {
  try {
    const raw = localStorage.getItem(EXPANDED_GROUPS_KEY)
    if (!raw) return new Set()
    const arr = JSON.parse(raw) as string[]
    return new Set(Array.isArray(arr) ? arr : [])
  } catch {
    return new Set()
  }
}
const expandedGroupKeys = ref<Set<string>>(loadExpandedGroups())
// 同步到 localStorage（用 deep watch 让 Set 内部 add/delete 也能触发）
watch(
  expandedGroupKeys,
  (v) => {
    try {
      localStorage.setItem(EXPANDED_GROUPS_KEY, JSON.stringify(Array.from(v)))
    } catch {
      // quota exceed 等 → silent
    }
  },
  { deep: true },
)

function toggleTaskGroup(key: string) {
  const next = new Set(expandedGroupKeys.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedGroupKeys.value = next
}

function isTaskGroupExpanded(key: string): boolean {
  return expandedGroupKeys.value.has(key)
}

// 🆕 2026-06-11 v5：sub_section 折叠状态（每个 section header 可独立折叠）
// 持久化 key v2（v1 用 plugin-section 前缀，已废弃）
const COLLAPSED_SUBSECTIONS_KEY = 'encv_tasks_collapsed_subsections_v1'
function loadCollapsedSubSections(): Set<string> {
  try {
    const raw = localStorage.getItem(COLLAPSED_SUBSECTIONS_KEY)
    if (!raw) return new Set()
    const arr = JSON.parse(raw) as string[]
    return new Set(Array.isArray(arr) ? arr : [])
  } catch {
    return new Set()
  }
}
const collapsedSubSectionKeys = ref<Set<string>>(loadCollapsedSubSections())
watch(
  collapsedSubSectionKeys,
  (v) => {
    try {
      localStorage.setItem(COLLAPSED_SUBSECTIONS_KEY, JSON.stringify(Array.from(v)))
    } catch {
      // silent
    }
  },
  { deep: true },
)
function toggleSubSection(key: string) {
  const next = new Set(collapsedSubSectionKeys.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsedSubSectionKeys.value = next
}
function isSubSectionCollapsed(key: string): boolean {
  return collapsedSubSectionKeys.value.has(key)
}

const displayedItems = computed<DisplayItem[]>(() => {
  const tasks = filteredTasks.value
  if (tasks.length === 0) return []

  // 🆕 2026-06-11 v5：按 SubSection 维度分桶（不再硬编码 pluginName）
  // 历史：之前 plugins: Map<pluginName, tasks[]> 假设 task.pluginName 必存在
  // 修复：sections: Map<sectionKeyStr, { meta, tasks }> 通用化，未来加 category 维度
  //   只需要在 deriveSubSection 加一个 case
  interface Group {
    runId: string
    tone: 'automation' | 'ai_agent'
    sections: Array<{ sectionKeyStr: string; meta: SubSectionMeta; tasks: EncvTask[] }>
  }
  const groupsByRun = new Map<string, Group>()
  const singletonTasks: EncvTask[] = []

  for (const t of tasks) {
    // 🆕 2026-06-10 修复 v4：直接读 task 对象上的 triggeredBy / runId
    const by = t.triggeredBy ?? getTriggeredBy(t.id)
    if (by === 'user') {
      singletonTasks.push(t)
      continue
    }
    const runId = t.runId ?? getRunIdForTask(t.id)
    if (!runId) {
      singletonTasks.push(t)
      continue
    }
    const tone: 'automation' | 'ai_agent' = by === 'ai_agent' ? 'ai_agent' : 'automation'
    // 🆕 v5：用 deriveSubSection 动态派生（兼容 task 没 pluginName 的情况）
    // 🆕 2026-06-18 Task 6：deriveSubSection 从 composable 导入，pickSectionDimension 决定维度
    const section = deriveSubSection(t, pickSectionDimension(t))
    const sectionKeyStr = sectionKeyToString(section)
    const meta = buildSubSectionMeta(section)
    const g = groupsByRun.get(runId)
    if (g) {
      const sec = g.sections.find((s) => s.sectionKeyStr === sectionKeyStr)
      if (sec) sec.tasks.push(t)
      else g.sections.push({ sectionKeyStr, meta, tasks: [t] })
    } else {
      groupsByRun.set(runId, {
        runId,
        tone,
        sections: [{ sectionKeyStr, meta, tasks: [t] }],
      })
    }
  }

  // 把 singletonTasks 按 filteredTasks 顺序插入；group 按最早 createdAt 排序
  const allGroups: Group[] = Array.from(groupsByRun.values())
  allGroups.sort((a, b) => {
    const aEarliest = Math.min(
      ...a.sections.flatMap((s) => s.tasks.map((t) => new Date(t.createdAt).getTime())),
    )
    const bEarliest = Math.min(
      ...b.sections.flatMap((s) => s.tasks.map((t) => new Date(t.createdAt).getTime())),
    )
    return bEarliest - aEarliest
  })

  // 输出：singleton tasks + group cards
  const result: DisplayItem[] = []
  for (const t of singletonTasks) {
    result.push({ kind: 'task', key: t.id, task: t, subKey: null, groupKey: null })
  }
  for (const g of allGroups) {
    // 拉平 group 内所有 task 用于 group summary
    const allGroupTasks: EncvTask[] = g.sections.flatMap((s) => s.tasks)
    if (allGroupTasks.length >= GROUP_FOLD_THRESHOLD) {
      const groupKey = `${g.tone}-${g.runId}`
      const groupExpanded = expandedGroupKeys.value.has(groupKey)
      // 始终构造 group card
      result.push(buildGroupItem(groupKey, g.runId, allGroupTasks, g.tone, g.sections))
      if (groupExpanded) {
        // 🆕 v5：group 展开时按 section 维度插 sub_section_header（可折叠 + sticky）
        for (const sec of g.sections) {
          const subKey = `sub-${groupKey}-${sec.sectionKeyStr}`
          const isCollapsed = collapsedSubSectionKeys.value.has(subKey)
          result.push(buildSubSectionHeader(subKey, g.runId, sec.meta, sec.tasks, isCollapsed))
          // sub_section 内的 task 跟随折叠
          for (const t of sec.tasks) {
            result.push({ kind: 'task', key: t.id, task: t, subKey, groupKey })
          }
        }
      }
    } else {
      // 不足阈值 → 全部展开为 task（保留顺序 + sub_section header 让用户看到分组）
      for (const sec of g.sections) {
        const subKey = `sub-${g.tone}-${g.runId}-${sec.sectionKeyStr}`
        result.push(buildSubSectionHeader(subKey, g.runId, sec.meta, sec.tasks, false))
        for (const t of sec.tasks) {
          result.push({ kind: 'task', key: t.id, task: t, subKey, groupKey: null })
        }
      }
    }
  }
  return result
})

/**
 * 构造 group card item（外层折叠段，≥2 个 task 折叠为 1 张卡片）
 */
function buildGroupItem(
  groupKey: string,
  runId: string,  // 🆕 用于上层 buildPluginSectionItem
  seg: EncvTask[],
  tone: 'automation' | 'ai_agent',
  sections: Array<{ sectionKeyStr: string; meta: SubSectionMeta; tasks: EncvTask[] }>,
): DisplayItem {
  let passed = 0, failed = 0, running = 0, pending = 0
  let latest = seg[0]
  for (const t of seg) {
    if (t.status === 'completed') passed++
    else if (t.status === 'failed') failed++
    else if (t.status === 'running' || t.status === 'cancelling') running++
    else pending++
    if (new Date(t.createdAt).getTime() > new Date(latest.createdAt).getTime()) {
      latest = t
    }
  }
  // 完成度 = (passed + failed) / total（不算 running/pending）
  const finished = passed + failed
  const percent = seg.length > 0 ? Math.round((finished / seg.length) * 100) : 0
  return {
    kind: 'group',
    key: groupKey,
    groupKey,
    runId,  // 🆕 携带 runId 给模板 / 子项用
    tone,
    tasks: seg,
    sections,  // 🆕 v5：携带 sections 给子 sub_section_header
    summary: { passed, failed, running, pending, percent, latestCreatedAt: latest.createdAt },
  }
}

/**
 * 🆕 2026-06-11 v5：构造 sub_section_header item（取代 v4 buildPluginSectionItem）
 *
 * 通用 section 段头（不再硬编码 pluginName），按 section 维度（plugin/type/category/none）渲染：
 *   - 左侧 icon bubble（按 dimension 显示对应 icon）
 *   - 中间：section 名称 + task 数
 *   - 右侧：4 个 status badge + 折叠 chevron
 *   - 整段可点击 → toggle 折叠/展开该 section 内的 task
 *   - sticky 行为：滚动时冻结在 group card 顶部
 *
 * 升级：未来加新维度只需要在 SECTION_META + deriveSubSection 加一行
 */
function buildSubSectionHeader(
  subKey: string,
  runId: string,
  meta: SubSectionMeta,
  tasks: EncvTask[],
  isCollapsed: boolean,
): DisplayItem {
  let passed = 0, failed = 0, running = 0, pending = 0
  for (const t of tasks) {
    if (t.status === 'completed') passed++
    else if (t.status === 'failed') failed++
    else if (t.status === 'running' || t.status === 'cancelling') running++
    else pending++
  }
  const finished = passed + failed
  const percent = tasks.length > 0 ? Math.round((finished / tasks.length) * 100) : 0
  return {
    kind: 'sub_section_header',
    key: subKey,
    subKey,
    runId,
    meta,
    tasks,
    isCollapsed,
    subSummary: { passed, failed, running, pending, percent },
  }
}

// 🆕 2026-06-10 修复：测试报告卡运行中（Tasks 页面卡 running）
// 根因：Tasks.vue 之前**完全没订阅 WS 事件**。task:update / task:progress / task:completed
//   推过来时没人调 applyTask*，tasks.value 永远是首次拉取的快照。
// 修复：useTaskEventBridge 已在 line 374-380 订阅 4 件套 WS 事件（mount 注册，unmount 注销），
//   **不要在这里再写一份 eventBus.on**，否则同一个事件会被触发 2 次，state 错乱。
// 修复 v2（2026-06-10 同日）：删除下方手写的 handleTask* + onMounted 重复订阅块。

// 🆕 onMounted：只处理路由 query（长按菜单跳转过来时打开 new task modal）
// 首次 fetchTasks 由 onIonViewWillEnter 接管（每次切回 tab 智能刷新）。
onMounted(() => {
  if (route.query.action === 'new') {
    const sourcePath = route.query.source as string
    const taskType = (route.query.type || 'encrypt') as TaskType
    router.replace({ path: '/tabs/tasks', query: {} })
    if (sourcePath) {
      openNewTask(sourcePath, taskType)
    } else {
      openNewTask()
    }
  }
})

// 🆕 onIonViewWillEnter：参考 Files.vue 实现切回 tab 自动刷新
//   智能条件：如果存在 running/queued task 立即拉一次最新列表；否则只靠 WS 增量更新
//   避免无谓的 GET /api/tasks 调用
onIonViewWillEnter(() => {
  if (tasks.value.length === 0) {
    fetchTasks()
    return
  }
  // 存在 running/queued → 立即拉一次最新
  const hasActive = tasks.value.some(
    (t) => t.status === 'running' || t.status === 'queued' || t.status === 'cancelling',
  )
  if (hasActive) {
    fetchTasks()
  }
})

// 🆕 2026-06-10 修复 v4：可见的调试计数（让用户能直接看到 grouping 是否在工作）
// 历史：用户报「毫无变化，我非常失望」—— HMR 没生效 / localStorage v2 数据 stale / 用户没刷新页面
// 修复：在 task 列表顶部显示 group / singleton / by triggeredBy 计数，让用户一眼看出问题在哪
const debugGroupCount = computed(() => {
  // 统计当前 displayedItems 里 group card 数量
  return displayedItems.value.filter((i) => i.kind === 'group').length
})
const debugSingletonCount = computed(() => {
  return displayedItems.value.filter((i) => i.kind === 'task').length
})
const debugByTriggeredBy = computed(() => {
  const acc: { automation: number; ai_agent: number; user: number } = {
    automation: 0,
    ai_agent: 0,
    user: 0,
  }
  for (const t of tasks.value) {
    const by = t.triggeredBy ?? getTriggeredBy(t.id)
    if (by === 'automation') acc.automation++
    else if (by === 'ai_agent') acc.ai_agent++
    else acc.user++
  }
  return acc
})

// 🆕 2026-06-10 修复 v4：手动重置分组（强制清空 localStorage + 重新拉取）
// 用法：调试栏右上「重置分组」按钮 → 调这个 → 所有 task 变 'user' → 重新跑 workflow
//   强制丢弃 stale localStorage（v2 数据残留让 task 分散到不同 runId，永远凑不到 1 个 group）
async function resetGrouping() {
  const { clearTriggeredBy } = await import('@/composables/useTaskTrigger')
  clearTriggeredBy()
  // 🆕 2026-06-11 v5：同时清 sub_section 折叠状态
  try {
    localStorage.removeItem(COLLAPSED_SUBSECTIONS_KEY)
  } catch {
    // silent
  }
  collapsedSubSectionKeys.value = new Set()
  showToast({ message: '已清空任务触发者缓存，刷新页面后生效', duration: 2000, color: 'medium' })
  await fetchTasks()
}

// 🆕 2026-06-11 v7：自动化测试报告分析器
// 设计原则（用户原话「测试报告是给你看的不是给我看的」）：
//   - 不弹 alert / showToast 烦用户
//   - 写 console.group 输出结构化分析（dev console 直接看）
//   - 自动按失败率 / 错误模式分类，输出可疑 bug 列表
//   - 关键失败 → 上报后端 /api/dev/automation-report（让后端聚合分析）
//   - 用户视角：调试栏按钮触发，但**用户不用等结果**，console + 后端都看得到
//
// 🆕 2026-06-18 Task 16：数据源迁移
//   - 旧：localStorage key `encv_automation_results_v1`（useAutomationTests 持久化，Task 8 已删除）
//   - 新：workflowTaskService.runs（UnifiedRunRecord[]，localStorage key `encv_workflow_tasks_v1`）
//   - 字段映射：UnifiedRunRecord.results[].caseId 替代旧 caseName；category 从 workflowRun.triggeredBy 派生
function showAutomationReports() {
  // 🆕 Task 16：从 workflowTaskService.runs 读取（响应式 ref → 取 .value）
  const runs = workflowTaskService.runs.value
  if (runs.length === 0) {
    console.info('[automation-report] no runs in workflowTaskService.runs (key=encv_workflow_tasks_v1)')
    return
  }

  // 自动分析
  // 🆕 Task 16：category 从 workflowRun.triggeredBy 派生（旧字段 r.category 已不存在）
  //   - triggeredBy === 'ai_agent' → 归到 'ai_agent' 桶
  //   - 其他（user / automation）→ 归到 'plugin' 桶
  //   注：旧版 webdav category 已合并到统一 workflow 体系，不再单独区分
  const aiAgentRuns = runs.filter((r) => r.workflowRun?.triggeredBy === 'ai_agent')
  const pluginRuns = runs.filter((r) => r.workflowRun?.triggeredBy !== 'ai_agent')
  const totalPassed = runs.reduce((acc, r) => acc + (r.passed ?? 0), 0)
  const totalFailed = runs.reduce((acc, r) => acc + (r.failed ?? 0), 0)
  const totalSkipped = runs.reduce((acc, r) => acc + (r.skipped ?? 0), 0)
  const totalCases = totalPassed + totalFailed + totalSkipped
  const failureRate = totalCases > 0 ? (totalFailed / totalCases) * 100 : 0

  // 错误聚类：相同 caseId 失败多次 → 可疑 bug
  // 🆕 Task 16：UnifiedRunRecord.results[].status 用 'failure'（旧版用 'failed'）
  const errorMap = new Map<string, { count: number; firstError: string; runs: string[] }>()
  for (const r of runs) {
    for (const c of r.results ?? []) {
      if (c.status === 'failure') {
        const key = `case:${c.caseId ?? '?'}`
        const prev = errorMap.get(key)
        if (prev) {
          prev.count++
          prev.runs.push(r.id?.slice(0, 12) ?? '?')
        } else {
          errorMap.set(key, { count: 1, firstError: c.error ?? '', runs: [r.id?.slice(0, 12) ?? '?'] })
        }
      }
    }
  }
  const suspiciousBugs = Array.from(errorMap.entries())
    .filter(([, v]) => v.count >= 2)
    .sort((a, b) => b[1].count - a[1].count)

  // 最近一次失败的 run
  const lastRun = runs[0]
  const lastRunFailed = (lastRun?.results ?? []).filter((c) => c.status === 'failure')

  // 输出结构化报告
  console.group('[automation-report] 自动化测试历史分析')
  console.log('data source: workflowTaskService.runs (localStorage key=encv_workflow_tasks_v1)')
  console.log('run 总数:', runs.length, '(ai_agent=' + aiAgentRuns.length + ', plugin=' + pluginRuns.length + ')')
  console.log('总用例:', totalCases, '· 通过:', totalPassed, '· 失败:', totalFailed, '· 跳过:', totalSkipped)
  console.log('总失败率:', failureRate.toFixed(2) + '%')
  if (lastRun) {
    console.log('最近 run:', {
      id: lastRun.id,
      startedAt: lastRun.startedAt,
      totalCases: lastRun.totalCases,
      passed: lastRun.passed,
      failed: lastRun.failed,
      skipped: lastRun.skipped,
      triggeredBy: lastRun.workflowRun?.triggeredBy ?? 'user',
      workflowDefId: lastRun.workflowRun?.workflowDefId,
    })
    if (lastRunFailed.length > 0) {
      console.warn('最近 run 失败用例:', lastRunFailed)
    }
  }
  if (suspiciousBugs.length > 0) {
    console.error('🚨 可疑 bug（多次失败的用例）:')
    for (const [name, info] of suspiciousBugs) {
      console.error(`  ${name} — 失败 ${info.count} 次`)
      console.error(`    错误: ${info.firstError}`)
      console.error(`    出现在 run: ${info.runs.join(', ')}`)
    }
  } else if (totalFailed === 0) {
    console.info('✅ 所有 run 均无失败')
  }
  console.groupEnd()

  // 上报后端（fire-and-forget，不阻塞 UI）
  reportAutomationToBackend({
    storageKey: 'encv_workflow_tasks_v1',
    runCount: runs.length,
    aiAgentRunCount: aiAgentRuns.length,
    pluginRunCount: pluginRuns.length,
    totalCases,
    totalPassed,
    totalFailed,
    totalSkipped,
    failureRate: Number(failureRate.toFixed(2)),
    suspiciousBugs: suspiciousBugs.map(([name, info]) => ({ name, count: info.count, firstError: info.firstError })),
    lastRunFailed: lastRunFailed.map((c) => ({ caseId: c.caseId, error: c.error, duration: c.duration })),
    timestamp: new Date().toISOString(),
  }).catch((e) => {
    console.debug('[automation-report] backend report failed (silent):', e)
  })
}

/**
 * 上报自动化测试分析结果到后端（fire-and-forget）
 * 失败不阻塞 UI，silent
 */
async function reportAutomationToBackend(payload: object): Promise<void> {
  try {
    const { getApiBaseUrl } = await import('@/api/encv')
    const baseUrl = getApiBaseUrl()
    await fetch(`${baseUrl}/api/dev/automation-report`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
  } catch {
    // 后端没接这个 endpoint 没关系，console 已经输出了
    throw new Error('backend report endpoint unavailable')
  }
}
</script>

<style scoped>
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 50%;
  color: var(--encv-text-secondary);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 50%;
  padding: 24px;
  text-align: center;
  color: var(--encv-text-secondary);
}

.empty-icon {
  font-size: 64px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.toolbar-actions {
  display: flex;
  justify-content: flex-end;
  gap: 4px;
  padding: 4px 16px 0;
}

.action-btn {
  --color: var(--ion-color-medium);
  --padding-start: 8px;
  --padding-end: 8px;
  font-size: 18px;
}

.toolbar-btn {
  --color: var(--ion-color-medium);
  --padding-start: 8px;
  --padding-end: 8px;
  font-size: 20px;
}

.task-searchbar {
  --padding-start: 12px;
  --padding-end: 12px;
  padding-top: 4px;
  padding-bottom: 4px;
}

.filter-toolbar {
  --padding-start: 8px;
  --padding-end: 8px;
  --min-height: 44px;
}

.filter-chips {
  display: flex;
  gap: 6px;
  padding: 4px 8px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.filter-chips ion-chip {
  flex-shrink: 0;
  font-size: 12px;
  --padding-start: 8px;
  --padding-end: 10px;
}

.task-id {
  font-size: 11px;
  font-family: monospace;
  color: var(--encv-text-secondary);
  opacity: 0.7;
  margin-right: 2px;
}

.status-badge {
  margin-right: 8px;
  font-size: 11px;
}

.task-type {
  font-size: 12px;
  color: var(--encv-text-secondary);
  margin-left: 6px;
}

.card-meta-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.plugin-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
  --padding-top: 2px;
  --padding-bottom: 2px;
  font-weight: 500;
}

.triggered-by-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
  --padding-top: 2px;
  --padding-bottom: 2px;
  font-weight: 500;
  margin-left: 4px;
}
.triggered-by-icon {
  font-size: 11px;
  margin-right: 3px;
  vertical-align: middle;
}

.task-time-info {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
  font-size: 11px;
  color: var(--encv-text-secondary);
}

/* ============================================================
   🆕 2026-06-10 修复：自动化测试 / AI agent 任务组卡片美化
   ============================================================ */
.task-group-card {
  --background: var(--ion-color-light);
  border-left: 4px solid var(--ion-color-primary);
  margin: 8px 0;
  border-radius: 8px;
  overflow: hidden;
  transition: background 0.2s ease;
}
.task-group-card.group-tone-ai_agent {
  border-left-color: var(--ion-color-secondary);
  --background: linear-gradient(135deg, rgba(139, 92, 246, 0.08), rgba(139, 92, 246, 0.02));
}
.task-group-card.group-tone-automation {
  border-left-color: var(--ion-color-primary);
  --background: linear-gradient(135deg, rgba(79, 140, 255, 0.08), rgba(79, 140, 255, 0.02));
}

.group-icon-bubble {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  margin-right: 4px;
}
.group-icon-bubble.group-tone-ai_agent {
  background: var(--ion-color-secondary);
  color: white;
}
.group-icon-bubble.group-tone-automation {
  background: var(--ion-color-primary);
  color: white;
}
.group-icon-bubble ion-icon {
  font-size: 22px;
}

.group-title {
  font-size: 15px;
  font-weight: 600;
  margin: 0 0 4px;
  color: var(--ion-color-dark);
}
.group-count {
  font-size: 13px;
  font-weight: 500;
  color: var(--ion-color-medium-shade);
  margin-left: 4px;
}

.group-meta-row {
  margin: 6px 0;
}
.group-meta-row .status-badge {
  font-size: 11px;
  padding: 3px 8px;
  margin-right: 4px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.badge-icon {
  font-size: 12px;
}
.badge-spinner {
  width: 10px;
  height: 10px;
  --color: currentColor;
}

.group-progress-track {
  height: 6px;
  background: var(--ion-color-step-100, rgba(0, 0, 0, 0.06));
  border-radius: 3px;
  overflow: hidden;
  margin: 6px 0 4px;
}
.group-progress-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--ion-color-primary), var(--ion-color-primary-shade));
  border-radius: 3px;
  transition: width 0.3s ease;
}
.group-tone-ai_agent .group-progress-fill {
  background: linear-gradient(90deg, var(--ion-color-secondary), var(--ion-color-secondary-shade));
}

.group-time-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 2px 0 0;
  font-size: 11px;
}
.group-percent-label {
  font-weight: 600;
  color: var(--ion-color-primary);
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
}
.group-tone-ai_agent .group-percent-label {
  color: var(--ion-color-secondary);
}

.group-chevron-btn {
  --color: var(--ion-color-medium-shade);
  margin: 0;
}

/* ============================================================
   🆕 2026-06-11 v5：sub_section_header（取代 v4 plugin-sub-section）
   - 4 种 dimension tone：plugin / type / category / none
   - sticky 滚动冻结（top: 0）
   - 商业级视觉：subtle shadow + 1px border + backdrop-filter
   - 可折叠：整段 button + 右侧 chevron
   ============================================================ */
ion-item.sub-section-header {
  --padding-start: 56px;       /* 左侧缩进（对应 group card 4px border + 40px icon + 12px 间距） */
  --padding-end: 12px;
  --padding-top: 10px;
  --padding-bottom: 12px;
  --min-height: 52px;
  --background: var(--sub-bg, rgba(79, 140, 255, 0.05));
  --background-hover: var(--sub-bg-hover, rgba(79, 140, 255, 0.08));
  --background-activated: var(--sub-bg-activated, rgba(79, 140, 255, 0.12));
  --border-color: var(--sub-border, rgba(79, 140, 255, 0.12));
  --color: var(--ion-color-dark);
  --inner-padding-end: 0;
  position: sticky;
  top: 0;
  z-index: 5;
  font-size: 13px;
  /* 商业级视觉：backdrop-filter 让 sticky 时半透明 */
  backdrop-filter: blur(10px) saturate(140%);
  -webkit-backdrop-filter: blur(10px) saturate(140%);
  background-color: rgba(255, 255, 255, 0.92);
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.04), 0 4px 12px -4px rgba(0, 0, 0, 0.06);
  transition: background 0.18s ease, box-shadow 0.18s ease;
}
ion-item.sub-section-header.is-collapsed {
  /* 折叠时视觉上「轻」一点：subtle hint 让用户知道里面有内容 */
  background-color: rgba(250, 250, 252, 0.92);
}

/* 4 种 dimension tone：plugin (primary 蓝) / type (warning 黄) / category (success 绿) / none (medium 灰) */
ion-item.sub-section-header.sub-tone-plugin {
  --sub-bg: rgba(79, 140, 255, 0.05);
  --sub-bg-hover: rgba(79, 140, 255, 0.08);
  --sub-bg-activated: rgba(79, 140, 255, 0.12);
  --sub-border: rgba(79, 140, 255, 0.14);
}
ion-item.sub-section-header.sub-tone-type {
  --sub-bg: rgba(255, 167, 38, 0.05);
  --sub-bg-hover: rgba(255, 167, 38, 0.08);
  --sub-bg-activated: rgba(255, 167, 38, 0.12);
  --sub-border: rgba(255, 167, 38, 0.14);
}
ion-item.sub-section-header.sub-tone-category {
  --sub-bg: rgba(54, 175, 110, 0.05);
  --sub-bg-hover: rgba(54, 175, 110, 0.08);
  --sub-bg-activated: rgba(54, 175, 110, 0.12);
  --sub-border: rgba(54, 175, 110, 0.14);
}
ion-item.sub-section-header.sub-tone-none {
  --sub-bg: rgba(158, 158, 158, 0.04);
  --sub-bg-hover: rgba(158, 158, 158, 0.07);
  --sub-bg-activated: rgba(158, 158, 158, 0.1);
  --sub-border: rgba(158, 158, 158, 0.12);
}

.sub-section-icon-bubble {
  width: 28px;
  height: 28px;
  border-radius: 8px;          /* 商业级：圆角方形（vs 圆形） */
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
  flex-shrink: 0;
  margin-left: -40px;
  background: var(--ion-color-primary);
  color: white;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}
.sub-section-icon-bubble.sub-tone-plugin {
  background: linear-gradient(135deg, #5b9dff, #2f7ce0);
  color: white;
}
.sub-section-icon-bubble.sub-tone-type {
  background: linear-gradient(135deg, #ffb74d, #f57c00);
  color: white;
}
.sub-section-icon-bubble.sub-tone-category {
  background: linear-gradient(135deg, #66bb6a, #388e3c);
  color: white;
}
.sub-section-icon-bubble.sub-tone-none {
  background: linear-gradient(135deg, #bdbdbd, #9e9e9e);
  color: white;
}
.sub-section-icon-bubble ion-icon {
  font-size: 16px;
}

.sub-section-label {
  margin: 0 !important;
  display: flex;
  flex-direction: column;
  gap: 0;
  min-width: 0;
}
.sub-section-label h3.sub-section-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--ion-color-dark);
  margin: 0;
  line-height: 1.3;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  letter-spacing: -0.01em;       /* 商业级：tight letter-spacing 提升精致感 */
}
.sub-section-label p.sub-section-count {
  font-size: 11px;
  color: var(--encv-text-secondary);
  margin: 0;
  line-height: 1.3;
  font-weight: 500;
}

.sub-section-badges {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
  align-items: center;
  margin-right: 4px;
}
.sub-section-badges .status-badge {
  font-size: 10px;
  --padding-start: 5px;
  --padding-end: 6px;
  --padding-top: 1px;
  --padding-bottom: 1px;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-weight: 600;             /* 商业级：徽章文字加粗 */
}
.sub-section-badges .badge-icon {
  font-size: 10px;
}
.sub-section-badges .badge-spinner {
  width: 8px;
  height: 8px;
  --color: currentColor;
}

.sub-section-chevron-btn {
  --color: var(--ion-color-medium-shade);
  margin: 0;
  transition: transform 0.2s ease;   /* 商业级：旋转动画 */
}
.sub-section-chevron-btn ion-icon {
  transition: transform 0.2s ease;
}
.is-collapsed .sub-section-chevron-btn ion-icon {
  transform: rotate(-90deg);
}

.sub-section-progress-track {
  position: absolute;
  left: 56px;
  right: 12px;
  bottom: 0;
  height: 2px;
  background: rgba(0, 0, 0, 0.05);
  overflow: hidden;
  pointer-events: none;
}
.sub-section-progress-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--ion-color-primary), var(--ion-color-primary-shade));
  transition: width 0.3s ease;
}
.sub-tone-type .sub-section-progress-fill {
  background: linear-gradient(90deg, #ffb74d, #f57c00);
}
.sub-tone-category .sub-section-progress-fill {
  background: linear-gradient(90deg, #66bb6a, #388e3c);
}
.sub-tone-none .sub-section-progress-fill {
  background: linear-gradient(90deg, #bdbdbd, #9e9e9e);
}

.time-created {
  color: var(--encv-text-secondary);
}

.time-duration {
  color: var(--ion-color-primary);
  font-weight: 500;
}

.progress-section {
  margin-top: 6px;
}

.task-progress {
  margin-top: 2px;
}

.progress-cancelling {
  --progress-background: var(--ion-color-warning);
}

.progress-detail {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  font-size: 11px;
  color: var(--encv-text-secondary);
  flex-wrap: wrap;
}

.phase-label {
  color: var(--ion-color-primary);
  font-weight: 500;
}

.progress-percent {
  font-weight: 600;
  color: var(--encv-text-secondary);
}

.speed-label {
  color: var(--encv-text-secondary);
}

.eta-label {
  color: var(--encv-text-secondary);
}

.completed-info {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
}

.completed-icon {
  font-size: 16px;
}

.completed-text {
  font-size: 12px;
  color: var(--ion-color-success);
}

.container-version {
  font-size: 11px;
  font-weight: 600;
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  padding: 1px 6px;
  border-radius: 4px;
  margin-left: 6px;
}

.task-error {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin-top: 4px;
}

.password-error {
  display: flex;
  align-items: center;
  gap: 4px;
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  padding: 6px 10px;
  border-radius: 6px;
  border-left: 3px solid var(--ion-color-danger);
}

.password-error ion-icon {
  font-size: 14px;
  flex-shrink: 0;
}

.task-warning {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  margin-top: 4px;
  background: rgba(255, 152, 0, 0.1);
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  color: #e65100;
}

.warning-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.task-warning-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-warning-detail {
  padding: 8px 12px;
  margin-top: 4px;
  background: var(--ion-color-step-100, #f0f0f0);
  border-radius: 4px;
  max-height: 150px;
  overflow-y: auto;
}

.task-warning-detail pre {
  margin: 0;
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
  color: #666;
}

.cancelling-spinner {
  width: 20px;
  height: 20px;
}

.popover-filter-content {
  padding: 8px 0;
  min-width: 180px;
  max-height: 320px;
  overflow-y: auto;
}

.popover-filter-title {
  font-size: 13px;
  font-weight: 600;
  padding: 4px 16px 8px;
  color: var(--encv-text-secondary);
}

.popover-filter-item {
  --min-height: 40px;
  --padding-start: 12px;
  --padding-end: 12px;
  cursor: pointer;
}

.popover-filter-item ion-checkbox {
  margin-right: 8px;
}

.popover-empty {
  padding: 12px 16px;
  font-size: 13px;
  color: var(--encv-text-secondary);
}

/* ============================================================
   🆕 2026-06-10 修复 v4：可见的调试计数栏
   用途：让用户能直接看到 grouping 是否在工作
   历史：用户报「毫无变化，我非常失望」时排查卡住 — HMR 没生效 / localStorage v2 stale
        都没法让用户自查。修这个栏 → 任何时候用户都能看到 group / singleton / by 计数。
   ============================================================ */
.grouping-debug-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 10px;
  padding: 8px 14px;
  margin: 8px 12px 4px;
  background: linear-gradient(135deg, rgba(79, 140, 255, 0.06), rgba(139, 92, 246, 0.06));
  border: 1px dashed rgba(79, 140, 255, 0.3);
  border-radius: 6px;
  font-size: 11px;
  color: var(--encv-text-secondary);
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  line-height: 1.4;
}
.grouping-debug-bar strong {
  color: var(--ion-color-dark);
  font-weight: 700;
  font-family: inherit;
  margin: 0 2px;
}
.grouping-debug-sep {
  color: var(--ion-color-medium-shade);
  opacity: 0.5;
  font-weight: 300;
}
.grouping-reset-btn {
  margin-left: auto;
  --padding-start: 8px;
  --padding-end: 8px;
  font-size: 11px;
  height: 28px;
}
</style>
