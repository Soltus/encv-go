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

    <ion-content ref="contentRef">
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

      <!-- 🆕 Task 15：用 TaskVirtualList 替换 ion-list + v-for -->
      <!-- 历史：ion-list + v-for 渲染所有 displayedItems → 200+ task 时 DOM 节点爆炸 -->
      <!-- 修复：TaskVirtualList 用 @tanstack/vue-virtual 仅渲染可见窗口 + overscan 20 个 item -->
      <!-- 注意：collapsed sub_section 的 task 不在 displayedItems 里（v-show 与虚拟滚动冲突） -->
      <TaskVirtualList
        v-else
        :items="displayedItems"
        :scroll-el="scrollEl"
        ref="virtualListRef"
        class="tasks-virtual-list"
      >
        <template #default="{ item }">
          <!-- 🆕 2026-06-10 修复：自动化测试 / AI agent 任务组折叠 -->
          <!-- 历史：自动化测试一次跑 N 个用例 → 污染 task 列表（用户截图的"浪费屏幕空间"）-->
          <!-- 修复：连续 ≥2 个 triggeredBy != 'user' 的 task → 折叠成 1 张 group card -->
          <!--       点 group card 右侧 chevron 展开/折叠详情 -->
          <!-- 🆕 2026-06-10 修复 v2：2 级嵌套 — group 展开时按 pluginName 插 plugin_section 段头 -->
          <ion-item-sliding v-if="item.kind === 'group'">
            <ion-item button detail @click="toggleTaskGroup(item.groupKey!)" :class="['tl-item-card', 'tl-item-card--group', `tl-tone--${item.tone}`]">
              <div :class="['tl-bubble', 'tl-bubble--lg', `tl-tone--${item.tone}`]" slot="start">
                <ion-icon :icon="item.tone === 'ai_agent' ? hardwareChipOutline : cogOutline"></ion-icon>
              </div>
              <ion-label>
                <h2 :class="['tl-title', 'tl-title--lg']">
                  {{ item.tone === 'ai_agent' ? t('tasks.triggeredBy_ai_agent') : t('tasks.triggeredBy_automation') }}
                  <span class="tl-title__count">· {{ item.tasks.length }} {{ t('tasks.tasksCount') }}</span>
                </h2>
                <p class="tl-meta-row">
                  <ion-badge v-if="item.summary.passed > 0" color="success" class="tl-status-badge">
                    <ion-icon :icon="checkmarkCircle" class="tl-badge-icon"></ion-icon>
                    {{ item.summary.passed }}
                  </ion-badge>
                  <ion-badge v-if="item.summary.failed > 0" color="danger" class="tl-status-badge">
                    <ion-icon :icon="closeCircle" class="tl-badge-icon"></ion-icon>
                    {{ item.summary.failed }}
                  </ion-badge>
                  <ion-badge v-if="item.summary.running > 0" color="warning" class="tl-status-badge">
                    <ion-spinner name="dots" class="tl-badge-spinner"></ion-spinner>
                    {{ item.summary.running }}
                  </ion-badge>
                  <ion-badge v-if="item.summary.pending > 0" color="medium" class="tl-status-badge">
                    {{ item.summary.pending }}
                  </ion-badge>
                </p>
                <div :class="['tl-progress', 'tl-progress--lg', `tl-tone--${item.tone}`]">
                  <div
                    class="tl-progress__fill"
                    :style="{ width: item.summary.percent + '%' }"
                  ></div>
                </div>
                <p :class="['tl-time-info', `tl-tone--${item.tone}`]">
                  <span class="tl-time-info__created">{{ formatDateTime(item.summary.latestCreatedAt) }}</span>
                  <span class="tl-time-info__percent">{{ item.summary.percent }}%</span>
                </p>
              </ion-label>
              <ion-button
                v-if="item.runId"
                slot="end"
                fill="clear"
                size="small"
                @click.stop="viewGroupReport(item.runId)"
                :title="t('tasks.viewReport')"
                class="group-report-btn"
              >
                <ion-icon
                  :icon="documentTextOutline"
                  slot="icon-only"
                ></ion-icon>
              </ion-button>
              <ion-button
                slot="end"
                fill="clear"
                size="small"
                @click.stop="toggleTaskGroup(item.groupKey!)"
                :title="isTaskGroupExpanded(item.groupKey!) ? t('tasks.collapse') : t('tasks.expand')"
                class="tl-chevron-btn"
              >
                <ion-icon
                  :icon="isTaskGroupExpanded(item.groupKey!) ? chevronBack : chevronForward"
                  slot="icon-only"
                ></ion-icon>
              </ion-button>
            </ion-item>
          </ion-item-sliding>

          <!-- 🆕 2026-06-11 v5：sub_section 段头（可独立折叠） -->
          <!-- 🆕 Task 15：移除 position: sticky（与虚拟滚动 absolute 定位冲突） -->
          <ion-item
            v-else-if="item.kind === 'sub_section_header'"
            button
            :detail="false"
            @click="toggleSubSection(item.subKey)"
            :class="['tl-item-card', 'tl-item-card--subsection', `tl-tone--${item.meta.dimension}`, { 'is-collapsed': item.isCollapsed }]"
            :lines="'none'"
          >
            <div :class="['tl-bubble', 'tl-bubble--md', `tl-tone--${item.meta.dimension}`]" slot="start">
              <ion-icon :icon="getSubSectionIcon(item.meta.icon)"></ion-icon>
            </div>
            <ion-label class="sub-section-label">
              <h3 :class="['tl-title', 'tl-title--md']">{{ item.meta.label }}</h3>
              <p class="tl-title__count">· {{ item.tasks.length }} {{ t('tasks.tasksCount') }}</p>
            </ion-label>
            <div class="sub-section-badges" slot="end">
              <ion-badge v-if="item.subSummary.passed > 0" color="success" class="tl-status-badge tl-status-badge--sm">
                <ion-icon :icon="checkmarkCircle" class="tl-badge-icon"></ion-icon>
                {{ item.subSummary.passed }}
              </ion-badge>
              <ion-badge v-if="item.subSummary.failed > 0" color="danger" class="tl-status-badge tl-status-badge--sm">
                <ion-icon :icon="closeCircle" class="tl-badge-icon"></ion-icon>
                {{ item.subSummary.failed }}
              </ion-badge>
              <ion-badge v-if="item.subSummary.running > 0" color="warning" class="tl-status-badge tl-status-badge--sm">
                <ion-spinner name="dots" class="tl-badge-spinner"></ion-spinner>
                {{ item.subSummary.running }}
              </ion-badge>
              <ion-badge v-if="item.subSummary.pending > 0" color="medium" class="tl-status-badge tl-status-badge--sm">
                {{ item.subSummary.pending }}
              </ion-badge>
            </div>
            <ion-button
              slot="end"
              fill="clear"
              size="small"
              :title="item.isCollapsed ? t('tasks.expand') : t('tasks.collapse')"
              class="tl-chevron-btn"
              @click.stop="toggleSubSection(item.subKey)"
            >
              <ion-icon
                :icon="item.isCollapsed ? chevronForward : chevronDown"
                slot="icon-only"
              ></ion-icon>
            </ion-button>
            <div :class="['tl-progress', 'tl-progress--sm', `tl-tone--${item.meta.dimension}`, 'sub-section-progress-track']">
              <div
                class="tl-progress__fill"
                :style="{ width: item.subSummary.percent + '%' }"
              ></div>
            </div>
          </ion-item>

          <!-- 🆕 Task 15：移除 v-show（虚拟滚动下 display:none 会导致 measureElement 测量 0px） -->
          <!-- collapsed sub_section 的 task 不在 displayedItems 里（buildDisplayedItems 过滤） -->
          <ion-item-sliding v-else>
            <ion-item
              :class="['tl-item-card']"
              @click="openTaskDetail(item.task)"
              button
              detail
            >
              <ion-icon
                :icon="getTaskIcon(item.task)"
                :color="getTaskColor(item.task)"
                slot="start"
              ></ion-icon>
              <ion-label>
                <h2>{{ getTaskName(item.task) }}</h2>
                <p class="tl-meta-row">
                  <span class="task-id">#{{ item.task.id.slice(0, 6) }}</span>
                  <ion-badge :color="getStatusColor(item.task.status)" class="tl-status-badge">
                    {{ getStatusLabel(item.task.status) }}
                  </ion-badge>
                  <span class="task-type">{{ item.task.type === 'encrypt' ? t('tasks.encrypt') : t('tasks.decrypt') }}</span>
                  <ion-badge v-if="item.task.pluginName" color="primary" class="plugin-badge">
                    {{ item.task.pluginName }}
                  </ion-badge>
                  <!-- 🆕 2026-06-18 Task 18：crypto params 摘要 badge（仅当有 cipherMode/compressionMode 时显示） -->
                  <span v-if="getCryptoSummary(item.task)" class="crypto-summary">
                    <ion-icon :icon="lockClosedOutline" class="crypto-summary-icon"></ion-icon>
                    {{ getCryptoSummary(item.task) }}
                  </span>
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
                <p class="tl-time-info">
                  <span class="tl-time-info__created">{{ formatDateTime(item.task.createdAt) }}</span>
                  <span v-if="getTaskDuration(item.task)" class="tl-time-info__duration">{{ getTaskDuration(item.task) }}</span>
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
      </TaskVirtualList>

      <ion-fab vertical="bottom" horizontal="end" slot="fixed">
        <ion-fab-button @click="openNewTask()">
          <ion-icon :icon="add"></ion-icon>
        </ion-fab-button>
      </ion-fab>

    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onIonViewWillEnter } from '@ionic/vue'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonContent,
  IonRefresher, IonRefresherContent, IonItem,
  IonItemSliding, IonItemOptions, IonItemOption, IonIcon,
  IonLabel, IonBadge, IonProgressBar, IonFab, IonFabButton,
  IonSpinner, IonButton, IonButtons, IonSearchbar, IonChip,
  IonPopover, IonCheckbox, alertController, modalController,
} from '@ionic/vue'
import {
  add, closeCircle, checkmarkCircle, timer, sync,
  warningOutline, lockClosed, lockClosedOutline, search, funnel, trashBin,
  extensionPuzzle, swapVertical, chevronDown,
  hardwareChipOutline, cogOutline, person, chevronForward, chevronBack,
  folderOutline, ellipsisHorizontalCircleOutline,
  documentTextOutline,
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
import { getTriggeredBy, getRunIdForTask } from '@/composables/useTaskTrigger'
import { formatContainerVersion } from '@/constants/containerVersion'
import {
  deriveSubSection,
  type SectionDimension,
  type SectionMeta,
} from '@/composables/useSectionDerivation'
// 🆕 Task 15：虚拟滚动组件
import TaskVirtualList from '@/components/tasks/TaskVirtualList.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { openNewTask } = useNewTaskModal()

// 🆕 Task 15：虚拟滚动所需的 ion-content 滚动容器引用
// ion-content 内部用 shadow DOM 渲染 .inner-scroll，需要通过 shadowRoot.querySelector 获取
// 参考 DevLogs.vue 的 ensureScrollEl() 模式
const contentRef = ref<any>(null)
const scrollEl = ref<HTMLElement | null>(null)
const virtualListRef = ref<{ forceMeasure: () => void } | null>(null)

/**
 * 🆕 Task 15：从 ion-content shadow DOM 获取 .inner-scroll 滚动容器
 *
 * ion-content 是 Ionic 的 shadow DOM 组件，实际滚动发生在内部 .inner-scroll 元素上，
 * 不是 ion-content host 本身。@tanstack/vue-virtual 需要拿到这个真实滚动元素才能
 * 监听 scroll 事件 + 测量视口高度。
 *
 * 时序问题：onMounted 时 ion-content 可能还没完成 shadow DOM 渲染 → scrollEl=null
 * 修法：多次重试（rAF + setTimeout 指数退避）+ ResizeObserver 兜底监听 host 尺寸变化
 */
function ensureScrollEl(): HTMLElement | null {
  if (!contentRef.value) return null
  const hostEl = (contentRef.value.$el || contentRef.value) as HTMLElement | undefined
  if (!hostEl || !hostEl.shadowRoot) return null
  const el = hostEl.shadowRoot.querySelector('.inner-scroll') as HTMLElement | null
  if (el && el !== scrollEl.value) scrollEl.value = el
  return scrollEl.value
}

let scrollElRetryTimer: ReturnType<typeof setTimeout> | null = null
let scrollElRO: ResizeObserver | null = null

function initScrollElWithRetry(): void {
  let retryCount = 0
  const maxRetries = 8
  const tryInit = (): void => {
    const el = ensureScrollEl()
    if (el) {
      // 拿到 .inner-scroll 后，强制 virtualizer 重算（首次 watch 已触发 measure()，
      // 这里再 measure 一次确保虚拟列表渲染首屏 items）
      virtualListRef.value?.forceMeasure?.()
      return
    }
    retryCount++
    if (retryCount < maxRetries) {
      // 指数退避：50ms → 100ms → 150ms → 200ms → 250ms → 300ms
      const delay = Math.min(50 * retryCount, 300)
      scrollElRetryTimer = setTimeout(tryInit, delay)
    }
  }
  tryInit()

  // 兜底：ResizeObserver 监听 contentRef 尺寸变化（ion-content 完成渲染时会触发）
  if (typeof ResizeObserver !== 'undefined' && contentRef.value) {
    const hostEl = (contentRef.value.$el || contentRef.value) as HTMLElement | undefined
    if (hostEl) {
      scrollElRO = new ResizeObserver(() => {
        if (!scrollEl.value) tryInit()
      })
      scrollElRO.observe(hostEl)
    }
  }
}

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

// 🆕 2026-06-18 Task 18：任务卡片副标题 crypto params 摘要
// 返回 "AES-256 · zstd" / "AES-128" / "zstd" / ""（旧任务无 crypto 字段时返回空串）
function getCryptoSummary(task: EncvTask): string {
  const parts: string[] = []
  if (task.cipherMode !== undefined && task.cipherMode !== null) {
    parts.push(task.cipherMode === 1 ? t('tasks.cipherMode256') : t('tasks.cipherMode128'))
  }
  if (task.compressionMode === 'zstd') {
    parts.push('Zstd')
  } else if (task.compressionMode === 'none') {
    parts.push(t('tasks.compressionNone'))
  }
  return parts.join(' · ')
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

// 🆕 v3 2026-06-18 Task 10：group card 跳转按钮 → PluginTestsDetail（带 runId query）
// 把任务系统的 group card 与插件测试报告系统打通：用户点击「查看报告」直接跳到
// /tabs/settings/devtools/plugin-tests?runId=xxx，PluginTestsDetail 读取 query 自动选中 run
function viewGroupReport(runId: string) {
  router.push({
    path: '/tabs/settings/devtools/plugin-tests',
    query: { runId },
  })
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
        // 🆕 v5：group 展开时按 section 维度插 sub_section_header（可独立折叠）
        // 🆕 Task 15：collapsed sub_section 的 task 不放入 displayedItems
        //   历史：v-show 隐藏 task → 虚拟滚动 measureElement 测到 display:none 元素高度为 0
        //   修复：collapsed 时只输出 sub_section_header，不输出其下 task
        //   用户展开 sub_section 时 collapsedSubSectionKeys 变化 → displayedItems 重算 → task 出现
        for (const sec of g.sections) {
          const subKey = `sub-${groupKey}-${sec.sectionKeyStr}`
          const isCollapsed = collapsedSubSectionKeys.value.has(subKey)
          result.push(buildSubSectionHeader(subKey, g.runId, sec.meta, sec.tasks, isCollapsed))
          if (!isCollapsed) {
            // sub_section 展开时才输出其下 task
            for (const t of sec.tasks) {
              result.push({ kind: 'task', key: t.id, task: t, subKey, groupKey })
            }
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
  // 🆕 Task 15：初始化虚拟滚动所需的 ion-content .inner-scroll 引用
  // ion-content shadow DOM 异步渲染，需要重试 + ResizeObserver 兜底
  initScrollElWithRetry()

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

// 🆕 Task 15：组件卸载时清理 scrollEl 重试定时器 + ResizeObserver
onBeforeUnmount(() => {
  if (scrollElRetryTimer) {
    clearTimeout(scrollElRetryTimer)
    scrollElRetryTimer = null
  }
  if (scrollElRO) {
    scrollElRO.disconnect()
    scrollElRO = null
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
  font-family: var(--tl-card-font-mono);
  color: var(--tl-card-text-secondary);
  opacity: 0.7;
  margin-right: 2px;
}

.task-type {
  font-size: 12px;
  color: var(--tl-card-text-secondary);
  margin-left: 6px;
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

/* 🆕 2026-06-18 Task 18：crypto params 摘要 badge */
.crypto-summary {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 10px;
  font-family: var(--tl-card-font-mono);
  color: var(--tl-card-text-secondary);
  background: rgba(var(--tl-state-created-rgb), 0.1);
  padding: 2px 6px;
  border-radius: var(--tl-card-radius-sm);
  margin-left: 4px;
  white-space: nowrap;
}
.crypto-summary-icon {
  font-size: 11px;
  flex-shrink: 0;
}

/* ============================================================
   🆕 v3 2026-06-18 Task 4：group / sub_section / task card 视觉
   已迁移到 timeline-utilities.css 的 .tl-item-card / .tl-bubble /
   .tl-status-badge / .tl-progress / .tl-title / .tl-time-info /
   .tl-meta-row / .tl-chevron-btn utility class。
   本文件只保留 Tasks.vue 特有的布局覆盖。
   ============================================================ */

/* 🆕 v3 2026-06-18 Task 10：group card 查看报告按钮 */
.group-report-btn {
  --color: var(--tl-trigger-automation);
  margin: 0 4px 0 0;
}
.group-report-btn:hover {
  --color: var(--tl-state-analyzing);
}
.tl-tone--ai_agent .group-report-btn {
  --color: var(--tl-trigger-ai-agent);
}

/* sub_section 布局覆盖（utility class 不含的特定布局） */
.sub-section-label {
  margin: 0 !important;
  display: flex;
  flex-direction: column;
  gap: 0;
  min-width: 0;
}
.sub-section-badges {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
  align-items: center;
  margin-right: 4px;
}
/* sub_section 底部进度条需要 absolute 定位（贴底） */
.sub-section-progress-track {
  position: absolute;
  left: var(--tl-item-padding-start-subsection, 56px);
  right: var(--tl-space-lg, 12px);
  bottom: 0;
  pointer-events: none;
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
  color: var(--tl-card-text-secondary);
  flex-wrap: wrap;
}

.phase-label {
  color: var(--tl-state-analyzing);
  font-weight: 500;
}

.progress-percent {
  font-weight: 600;
  color: var(--tl-card-text-secondary);
}

.speed-label {
  color: var(--tl-card-text-secondary);
}

.eta-label {
  color: var(--tl-card-text-secondary);
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
   🆕 Task 15：TaskVirtualList 容器样式
   - 虚拟列表接管 ion-content 内的滚动渲染
   - 移除 ion-list 默认 padding（虚拟列表自己管理布局）
   ============================================================ */
.tasks-virtual-list {
  width: 100%;
  /* 给虚拟列表一点底部留白，避免 FAB 遮挡最后一个 item */
  padding-bottom: 80px;
  box-sizing: border-box;
}
</style>
