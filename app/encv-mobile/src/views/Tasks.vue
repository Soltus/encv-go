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
          <!-- 🆕 v4 M5：数据源指示器（有多少个 workflow run 正在跑） -->
          <ion-chip
            v-if="workflowService.isRunning.value"
            color="warning"
            class="active-run-chip"
            :title="t('tasks.activeRunIndicator')"
          >
            <ion-spinner name="dots" class="active-run-spinner"></ion-spinner>
            <ion-label>{{ t('tasks.activeRunRunning') }} · {{ workflowService.totalSteps.value }}</ion-label>
          </ion-chip>
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
          <!-- 🆕 v6 2026-06-18：常驻清空筛选按钮（行业惯例：清空操作常驻可见） -->
          <ion-chip
            :color="hasActiveFilters ? 'danger' : 'medium'"
            :disabled="!hasActiveFilters"
            @click="clearFilters"
            :title="t('tasks.clearFilters')"
            class="filter-clear-chip"
          >
            <ion-icon :icon="closeCircle" size="small"></ion-icon>
            <ion-label>{{ t('tasks.clearFilters') }}</ion-label>
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
            <ion-label>{{ plugin === '__unknown__' ? t('tasks.unknownPlugin') : plugin }}</ion-label>
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

      <!-- 🆕 v4 M3：日期区间筛选 popover（不占页面空间，preset + 自定义都内嵌） -->
      <ion-popover
        :is-open="datePopoverOpen"
        :event="datePopoverEvent"
        @didDismiss="datePopoverOpen = false"
        side="bottom"
        alignment="end"
        class="date-popover"
      >
        <div class="date-popover-content">
          <div class="popover-filter-title">{{ t('tasks.filterByDate') }}</div>
          <ion-item
            v-for="preset in datePresets"
            :key="preset.key"
            lines="none"
            button
            class="popover-filter-item"
            :class="{ 'date-preset-active': filterDatePreset === preset.key }"
            @click="onDatePresetClick(preset.key)"
          >
            <ion-icon
              :icon="preset.key === 'custom' ? calendarOutline : timer"
              slot="start"
              :color="filterDatePreset === preset.key ? 'primary' : 'medium'"
            ></ion-icon>
            <ion-label>{{ preset.label }}</ion-label>
          </ion-item>
          <!-- 自定义日期：两个原生 input 紧凑排版，不占大块空间 -->
          <div v-if="filterDatePreset === 'custom'" class="date-custom-range">
            <label class="date-range-label">
              <span>{{ t('tasks.dateFrom') }}</span>
              <input
                type="date"
                :value="customFromInput"
                @change="onCustomFromChange"
                class="date-input"
              />
            </label>
            <label class="date-range-label">
              <span>{{ t('tasks.dateTo') }}</span>
              <input
                type="date"
                :value="customToInput"
                @change="onCustomToChange"
                class="date-input"
              />
            </label>
          </div>
        </div>
      </ion-popover>
    </ion-header>

    <ion-content ref="contentRef">
      <ion-refresher slot="fixed" @ionRefresh="handleRefresh">
        <ion-refresher-content></ion-refresher-content>
      </ion-refresher>

      <!-- 🆕 2026-06-22 任务诊断面板（真机可见） -->
      <!-- user 反馈"加到哪去了"：默认显示 + 默认展开（?debug=tasks 仍可强制显示） -->
      <TaskDebugPanel
        v-if="debugEnabled"
        :tasks="tasks"
        :displayed-items="displayedItems"
        :grouped-tasks-by-run-id="groupedTasksByRunId"
        :view-mode="viewMode"
        :sort-by="sortBy"
        :search-query="searchQuery"
        :filter-plugins="filterPlugins"
        :filter-types="filterTypes"
        :filter-statuses="filterStatuses"
        :filter-triggered-by="filterTriggeredBy"
        :filter-date-preset="filterDatePreset"
        :pinned-run-ids="pinnedRunIds"
        :default-open="true"
      />

      <div class="toolbar-actions">
        <ion-button fill="clear" size="small" @click="showSearch = !showSearch" class="action-btn">
          <ion-icon :icon="search" slot="icon-only"></ion-icon>
        </ion-button>
        <!-- 🆕 v4 M3：日期筛选按钮（与 search / filter 平级） -->
        <ion-button
          fill="clear"
          size="small"
          @click="openDatePopover($event)"
          class="action-btn"
          :title="t('tasks.filterByDate')"
        >
          <ion-icon
            :icon="calendarOutline"
            slot="icon-only"
            :color="filterDatePreset !== 'all' ? 'primary' : undefined"
          ></ion-icon>
        </ion-button>
        <ion-button fill="clear" size="small" @click="showFilters = !showFilters" class="action-btn">
          <ion-icon :icon="funnel" slot="icon-only" :color="hasActiveFilters ? 'primary' : undefined"></ion-icon>
        </ion-button>
        <!-- 🆕 v4 M3：视图模式切换（聚合 / 平铺） -->
        <ion-button
          fill="clear"
          size="small"
          @click="toggleViewMode"
          class="action-btn"
          :title="viewMode === 'group' ? t('tasks.viewModeFlat') : t('tasks.viewModeGroup')"
        >
          <ion-icon
            :icon="viewMode === 'group' ? albumsOutline : listOutline"
            slot="icon-only"
            :color="viewMode === 'group' ? 'primary' : undefined"
          ></ion-icon>
        </ion-button>
      </div>

      <!-- 🆕 v4 M2：仅首屏占位（tasks 为空 + isInitialLoad）显示 loading，已有内容时不闪 -->
      <div v-if="isInitialLoad && tasks.length === 0" class="loading-container">
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

      <!-- 🆕 2026-07-02 搜索模式提示（见 debug-discipline.md §3.5）：
           独立 v-if 块（不参与上方 v-if/v-else-if/v-else 链）
           greedy 模式时显示徽章，告知用户结果是语义近似（可能不完全相关） -->
      <div
        v-if="searchQuery.trim() && filteredTasks.length > 0 && searchMode === 'greedy'"
        class="search-mode-banner search-mode-greedy"
      >
        <ion-icon :icon="pricetagOutline" class="search-mode-icon"></ion-icon>
        <span>{{ t('tasks.searchModeGreedy', { defaultValue: '贪婪匹配：结果为语义近似，可能不完全相关' }) }}</span>
      </div>
      <div
        v-else-if="searchQuery.trim() && filteredTasks.length > 0 && searchMode === 'combined'"
        class="search-mode-banner search-mode-combined"
      >
        <ion-icon :icon="pricetagOutline" class="search-mode-icon"></ion-icon>
        <span>{{ t('tasks.searchModeCombined', { defaultValue: '综合匹配：关键词 + 语义重排序' }) }}</span>
      </div>

      <!-- 🆕 Task 15：用 TaskVirtualList 替换 ion-list + v-for -->
      <!-- 历史：ion-list + v-for 渲染所有 displayedItems → 200+ task 时 DOM 节点爆炸 -->
      <!-- 修复：TaskVirtualList 用 @tanstack/vue-virtual 仅渲染可见窗口 + overscan 10 个 item -->
      <!-- 🆕 v4 M3：displayedItems 是聚合/平铺二选一，包含 date / group / task 3 种 kind -->
      <!-- 🆕 2026-06-23 Task 8：重构为 count + getItem + getKey 接口
        - 旧：:items="displayedItems"（传整个数组，virtualizer 内部 O(N) 遍历）
        - 新：:count + :get-item + :get-key（virtualizer 只对可见窗口调 getItem，O(1)）
        - 配合 Task 10 Web Worker 委托视图计算，主线程不卡顿 -->
      <TaskVirtualList
        v-else
        :count="displayedItems.length"
        :get-item="(i: number) => displayedItems[i]"
        :get-key="(i: number) => displayedItems[i].key"
        :scroll-el="scrollEl"
        ref="virtualListRef"
        class="tasks-virtual-list"
      >
        <template #default="{ item }">
          <!-- ============== Date section header（今/昨/本周/本月/更早） ============== -->
          <!-- 聚合 + 平铺两种模式都显示，按 createdAt 分段 -->
          <div
            v-if="item.kind === 'date'"
            :key="item.key"
            class="tl-date-section"
          >
            <div class="tl-date-section__line"></div>
            <span class="tl-date-section__label">{{ item.label }}</span>
            <div class="tl-date-section__line"></div>
          </div>

          <!-- ============== Group card（聚合模式，1 个 run = 1 张卡片） ============== -->
          <!-- 未命中 group（hitAny=false）按用户选择 C 隐藏 -->
          <!-- 🆕 2026-06-18 v5-bug3fix：整张 card clickable → push 到 L2 GroupDetail -->
          <!-- 🆕 v6 2026-06-18：长按弹出 action-sheet 取消/置顶/删除（TaskVirtualList slot 不支持 ion-item-sliding） -->
          <!-- 🆕 v6 2026-06-22 性能优化：用 item.displayData.xxx 替代 13 次函数调用（预计算） -->
          <div
            v-else-if="item.kind === 'group' && item.counters.hitAny"
            :key="item.key"
            :class="['tl-group-card', 'tl-group-card--clickable', `tl-group-card--${item.displayData.tone}`, item.displayData.moodClass, isRunPinned(item.runId) ? 'tl-group-card--pinned' : '']"
            role="button"
            :aria-label="t('tasks.groupCard.openDetail')"
            @click="openGroupDetail(item.runId)"
            @keydown.enter.prevent="openGroupDetail(item.runId)"
            @keydown.space.prevent="openGroupDetail(item.runId)"
            @contextmenu.prevent="openGroupActionSheet(item)"
            @touchstart="onGroupTouchStart($event, item)"
            @touchend="onGroupTouchEnd($event, item)"
          >
            <!-- 左侧 4px 状态色边 -->
            <div
              :class="['tl-group-card__border', `tl-group-border--${item.displayData.dominantStatus}`]"
            ></div>

            <div class="tl-group-card__main">
              <!-- 标题行：tone icon + 触发器名 + N 个任务 + 进入箭头 -->
              <div class="tl-group-card__head">
                <div :class="['tl-bubble', 'tl-bubble--md', `tl-tone--${item.displayData.tone}`]">
                  <ion-icon :icon="item.displayData.tone === 'ai_agent' ? hardwareChipOutline : cogOutline"></ion-icon>
                </div>
                <div class="tl-group-card__title-block">
                  <h2 class="tl-group-card__title">
                    {{ item.displayData.tone === 'ai_agent' ? t('tasks.triggeredBy_ai_agent') : t('tasks.triggeredBy_automation') }}
                    <span class="tl-group-card__count">· {{ item._summaryTotal ?? item.tasks.length }} {{ t('tasks.tasksCount') }}</span>
                    <!-- 🆕 v6 置顶标记 -->
                    <ion-icon
                      v-if="isRunPinned(item.runId)"
                      :icon="pin"
                      class="tl-group-card__pin"
                      :title="t('tasks.pinnedTitle')"
                    ></ion-icon>
                  </h2>
                  <!-- 🆕 2026-06-22 逃逸诊断：group card 标题下方显示 runId（user 反馈"截图中连 runId 都没显示"） -->
                  <!-- 关键：4 个 group card 显示 4 个不同 runId → 是 4 个独立 run（合理） -->
                  <!--        4 个 group card 显示同一个 runId → 任务逃逸（bug） -->
                  <!-- 伪 group（__manual__${id}）红字标，一眼能看出"逃逸 task 散落" -->
                  <code
                    class="tl-group-card__runid"
                    :class="item.runId.startsWith('__manual__') ? 'tl-group-card__runid--fake' : ''"
                    :title="item.runId.startsWith('__manual__') ? '⚠️ 逃逸 task（runId 丢失）' : 'runId: ' + item.runId"
                    data-testid="group-card-runid"
                  >{{ item.runId }}</code>
                  <!-- plugin badges（前 3 个，超过省略） -->
                  <div class="tl-group-card__plugins">
                    <ion-badge
                      v-for="p in item.displayData.pluginBadges"
                      :key="p"
                      color="primary"
                      class="tl-group-card__plugin-badge"
                    >{{ p }}</ion-badge>
                  </div>
                </div>
                <div class="tl-group-card__actions">
                  <!-- 进入箭头 -->
                  <ion-icon
                    :icon="chevronForward"
                    class="tl-group-card__chevron"
                    :title="t('tasks.groupCard.openDetail')"
                  ></ion-icon>
                </div>
              </div>

              <!-- 自身状态行（智能行：passed/failed/running/pending 紧凑展示） -->
              <div class="tl-group-card__body">
                <div class="tl-meta-row tl-group-card__self">
                  <ion-badge v-if="item.displayData.summary.passed > 0" color="success" class="tl-status-badge">
                    <ion-icon :icon="checkmarkCircle" class="tl-badge-icon"></ion-icon>
                    {{ item.displayData.summary.passed }}
                  </ion-badge>
                  <ion-badge v-if="item.displayData.summary.failed > 0" color="danger" class="tl-status-badge">
                    <ion-icon :icon="closeCircle" class="tl-badge-icon"></ion-icon>
                    {{ item.displayData.summary.failed }}
                  </ion-badge>
                  <ion-badge v-if="item.displayData.summary.running > 0" color="warning" class="tl-status-badge">
                    <ion-spinner name="dots" class="tl-badge-spinner"></ion-spinner>
                    {{ item.displayData.summary.running }}
                  </ion-badge>
                  <ion-badge v-if="item.displayData.summary.pending > 0" color="medium" class="tl-status-badge">
                    {{ item.displayData.summary.pending }}
                  </ion-badge>
                  <span v-if="item.displayData.duration" class="tl-group-card__duration">
                    <ion-icon :icon="timer" class="tl-group-card__duration-icon"></ion-icon>
                    {{ item.displayData.duration }}
                  </span>
                </div>
                <div class="tl-progress tl-progress--md">
                  <div
                    class="tl-progress__fill"
                    :style="{ width: item.displayData.summary.percent + '%' }"
                  ></div>
                </div>
                <p class="tl-time-info">
                  <span class="tl-time-info__created">{{ formatDateTime(new Date(item.startedAt).toISOString()) }}</span>
                  <span class="tl-time-info__percent">{{ item.displayData.summary.percent }}%</span>
                </p>
              </div>

              <!-- 智能命中行：仅在筛选/搜索激活时显示 -->
              <div
                v-if="isGroupFilterActive"
                class="tl-group-card__hit"
              >
                <ion-icon :icon="funnel" class="tl-group-card__hit-icon"></ion-icon>
                <span class="tl-group-card__hit-text">{{ getGroupHitSummary(item) }}</span>
              </div>
            </div>
          </div>

          <!-- ============== Single task card（平铺模式 / 不成组的 task） ============== -->
          <ion-item-sliding v-else-if="item.kind === 'task'" :key="item.key">
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
                  <span v-if="getCryptoSummary(item.task)" class="crypto-summary">
                    <ion-icon :icon="lockClosedOutline" class="crypto-summary-icon"></ion-icon>
                    {{ getCryptoSummary(item.task) }}
                  </span>
                  <ion-badge
                    v-if="item.task.triggeredBy && item.task.triggeredBy !== 'user'"
                    :color="getTriggeredByColor(item.task)"
                    class="triggered-by-badge"
                    :title="t('tasks.triggeredBy') + ': ' + t('tasks.triggeredBy_' + item.task.triggeredBy)"
                  >
                    <ion-icon :icon="getTriggeredByIcon(item.task)" class="triggered-by-icon"></ion-icon>
                    {{ t('tasks.triggeredBy_' + item.task.triggeredBy) }}
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
              <!-- 🆕 2026-07-02：向量搜索相关度徽章（搜索激活 + 命中时显示，复用 Files.vue 同款组件） -->
              <RelevanceBadge
                v-if="searchQuery && getTaskSearchScore(item.task.id)"
                :score="getTaskSearchScore(item.task.id)!"
                slot="end"
              />
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

      <!-- 🆕 2026-06-23 Task 7：ion-infinite-scroll 触发 loadMore
        - 滚动到底部时触发 loadMore 加载下一页
        - hasMore=false 时禁用（没有更多数据）
        - isLoadingMore 时显示 loading spinner
        - ionInfinite 事件回调 complete() 让 infinite-scroll 重置 -->
      <ion-infinite-scroll
        v-if="hasMore"
        @ionInfinite="onInfinite"
        threshold="100px"
      >
        <ion-infinite-scroll-content
          :loading-spinner="isLoadingMore ? 'circular' : undefined"
          :loading-text="isLoadingMore ? t('common.loading') : ''"
        >
        </ion-infinite-scroll-content>
      </ion-infinite-scroll>

      <ion-fab vertical="bottom" horizontal="end" slot="fixed">
        <ion-fab-button @click="openNewTask()">
          <ion-icon :icon="add"></ion-icon>
        </ion-fab-button>
      </ion-fab>

    </ion-content>
  </ion-page>
</template>
<script setup lang="ts">
import {
  add,
  albumsOutline,
  calendarOutline,
  checkmarkCircle,
  chevronDown,
  chevronForward,
  closeCircle,
  funnel,
  listOutline,
  lockClosed,
  lockClosedOutline,
  pin,
  pricetagOutline,
  search,
  swapVertical,
  sync,
  timer,
  trashBin,
  warningOutline,
} from "ionicons/icons";

// Tasks.vue 重构后只剩 thin script：调用 useTasksView() composable + 必要 imports。
// 原 633 行 script 逻辑已全部抽到 ./useTasksView.ts。
//
// template 内直接使用的 state/handler 都在此解构到局部变量，
// Vue 3 <script setup> 自动暴露顶层 binding 给 template，所以 template 用法保持不变。

import { useTasksView } from "./useTasksView";

const {
  // i18n
  t,
  // virtual scroll
  contentRef,
  scrollEl,
  virtualListRef,
  // useTasksList re-exposed values
  tasks,
  isInitialLoad,
  expandedWarningDetail,
  sortBy,
  showSearch,
  searchQuery,
  showFilters,
  filterPlugins,
  filterTypes,
  filterStatuses,
  filterTriggeredBy,
  statusOptions,
  pluginPopoverOpen,
  typePopoverOpen,
  statusPopoverOpen,
  datePopoverOpen,
  datePopoverEvent,
  pluginPopoverEvent,
  typePopoverEvent,
  statusPopoverEvent,
  availablePlugins,
  hasActiveFilters,
  hasCompletedTasks,
  filteredTasks,
  openPluginPopover,
  openTypePopover,
  openStatusPopover,
  openDatePopover,
  togglePluginFilter,
  toggleTypeFilter,
  toggleStatusFilter,
  clearFilters,
  onSearchInput,
  toggleSort,
  cancelTaskById,
  retryTaskById,
  removeTaskById,
  getTaskName,
  getTaskDuration,
  getPluginChipLabel,
  getTypeChipLabel,
  getStatusChipLabel,
  getStatusLabel,
  isPasswordError,
  toggleWarningDetail,
  formatWarningDetail,
  getTaskIcon,
  getTaskColor,
  getStatusColor,
  getPhaseLabel,
  getTaskSearchScore,
  searchMode,
  viewMode,
  filterDatePreset,
  displayedItems,
  groupedTasksByRunId,
  pinnedRunIds,
  toggleViewMode,
  workflowService,
  // infinite scroll
  hasMore,
  isLoadingMore,
  // group card actions
  isRunPinned,
  // modal / new task
  openNewTask,
  // triggeredBy icons
  hardwareChipOutline,
  cogOutline,
  // inline helpers
  debugEnabled,
  getTriggeredByColor,
  getTriggeredByIcon,
  getCryptoSummary,
  openTaskDetail,
  handleRefresh,
  onInfinite,
  handleClearCompleted,
  datePresets,
  customFromInput,
  customToInput,
  onDatePresetClick,
  onCustomFromChange,
  onCustomToChange,
  isGroupFilterActive,
  getGroupHitSummary,
  openGroupDetail,
  onGroupTouchStart,
  onGroupTouchEnd,
  openGroupActionSheet,
} = useTasksView();
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

/* 🆕 2026-07-02 搜索模式提示横幅（见 debug-discipline.md §3.5）
   - greedy：橙色调，虚线边框，告知用户结果是语义近似
   - combined：蓝绿色调，实线边框，告知用户是关键词+向量综合 */
.search-mode-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 12px 4px;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.4;
}

.search-mode-banner .search-mode-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.search-mode-greedy {
  background: rgba(var(--ion-color-warning-rgb), 0.12);
  border: 1px dashed var(--ion-color-warning);
  color: var(--ion-color-warning-shade);
}

.search-mode-combined {
  background: rgba(var(--ion-color-tertiary-rgb), 0.10);
  border: 1px solid var(--ion-color-tertiary);
  color: var(--ion-color-tertiary-shade);
}

@media (prefers-color-scheme: dark) {
  .search-mode-greedy {
    color: var(--ion-color-warning-tint);
  }
  .search-mode-combined {
    color: var(--ion-color-tertiary-tint);
  }
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
  flex-wrap: wrap;
  align-items: center;
  padding: 4px 0;
}

/* 🆕 v4 M5：active run chip 样式（与 filter chip 视觉区分） */
.active-run-chip {
  font-weight: 600;
  --color: var(--ion-color-warning);
  background: rgba(var(--ion-color-warning-rgb), 0.1);
  flex-shrink: 0;
}
.active-run-spinner {
  width: 12px;
  height: 12px;
  margin-right: 4px;
}

.filter-chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  align-items: center;
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

/* ============================================================
   🆕 v4 2026-06-18 M3：group card / date section / hit counter chips
   替代 v3 的 sub_section_header + 双层折叠；按用户反馈"看 group 整体状态"
   把 status / 进度 / hit counter 平铺在 group card 头部。
   ============================================================ */

/* Date section header（今/昨/本周/本月/更早） */
.tl-date-section {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 12px 6px;
  position: relative;
  pointer-events: none;
}
.tl-date-section__line {
  flex: 1;
  height: 1px;
  background: var(--ion-color-step-200, rgba(128, 128, 128, 0.2));
}
.tl-date-section__label {
  font-size: 11px;
  font-weight: 600;
  color: var(--ion-color-medium);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  flex-shrink: 0;
}

/* Group card 容器 */
.tl-group-card {
  display: flex;
  position: relative;
  margin: 8px 8px 10px;
  border-radius: 10px;
  background: var(--ion-color-step-50, #fafafa);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  overflow: hidden;
  min-height: 80px;
}
.tl-group-card--ai_agent {
  background: linear-gradient(135deg,
    rgba(139, 92, 246, 0.06) 0%,
    var(--ion-color-step-50, #fafafa) 50%);
}
.tl-group-card--automation {
  background: linear-gradient(135deg,
    rgba(79, 140, 255, 0.06) 0%,
    var(--ion-color-step-50, #fafafa) 50%);
}

/* 左侧 4px 状态色边（group 主态决定） */
.tl-group-card__border {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
}
.tl-group-border--failed { background: var(--ion-color-danger, #ef4444); }
.tl-group-border--running {
  background: var(--ion-color-warning, #f59e0b);
  /* 运行时呼吸效果（避免静态感） */
  animation: tl-group-border-pulse 1.5s ease-in-out infinite;
}
.tl-group-border--completed { background: var(--ion-color-success, #10b981); }
.tl-group-border--cancelled { background: var(--ion-color-medium, #6b7280); }
.tl-group-border--queued { background: var(--ion-color-primary, #3b82f6); }

@keyframes tl-group-border-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.tl-group-card__main {
  flex: 1;
  padding: 10px 12px 8px 14px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 标题行 */
.tl-group-card__head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tl-group-card__title-block {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.tl-group-card__title {
  font-size: 14px;
  font-weight: 600;
  margin: 0;
  color: var(--ion-color-dark, #111);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tl-group-card__count {
  font-size: 12px;
  font-weight: 500;
  color: var(--ion-color-medium);
  margin-left: 4px;
}
.tl-group-card__plugins {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.tl-group-card__plugin-badge {
  font-size: 9px;
  --padding-start: 5px;
  --padding-end: 5px;
  --padding-top: 1px;
  --padding-bottom: 1px;
}

/* 🆕 2026-06-22 逃逸诊断：group card 显示 runId（user 反馈"截图中连 runId 都没显示"） */
.tl-group-card__runid {
  display: inline-block;
  font-family: ui-monospace, 'SF Mono', Menlo, monospace;
  font-size: 10px;
  color: var(--ion-color-medium-shade);
  background: var(--ion-color-light-shade, #e6e7eb);
  padding: 1px 5px;
  border-radius: 3px;
  margin-top: 2px;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}
.tl-group-card__runid--fake {
  color: var(--ion-color-danger-shade, #b30000);
  background: var(--ion-color-danger-tint, #f5b3b3);
  font-weight: 700;
}
.tl-group-card__actions {
  display: flex;
  align-items: center;
  gap: 0;
  flex-shrink: 0;
}

/* 状态汇总 + 进度条 */
.tl-group-card__body {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.tl-group-card__children {
  border-top: 1px solid var(--ion-color-step-200, rgba(128, 128, 128, 0.15));
  padding: 4px 0 0;
  margin: 4px -12px -8px -14px;
  background: rgba(0, 0, 0, 0.02);
}
.tl-item-card--child {
  --background: transparent;
  --min-height: 56px;
}
.tl-item-card--child h2 {
  font-size: 13px;
  font-weight: 500;
}

/* Hit counter chips（点击 toggle 筛选） */
.tl-counter-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}
.tl-counter-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 12px;
  border: 1px solid var(--ion-color-step-200, rgba(128, 128, 128, 0.25));
  background: var(--ion-color-step-50, #fafafa);
  font-size: 10px;
  font-weight: 500;
  color: var(--ion-color-dark, #111);
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
  white-space: nowrap;
  font-family: inherit;
}
.tl-counter-chip:hover {
  background: var(--ion-color-step-100, #f0f0f0);
  border-color: var(--ion-color-step-300, rgba(128, 128, 128, 0.4));
}
.tl-counter-chip__icon {
  font-size: 11px;
  flex-shrink: 0;
}
.tl-counter-chip__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 80px;
}
.tl-counter-chip__ratio {
  font-family: var(--tl-card-font-mono, monospace);
  font-size: 9px;
  font-weight: 600;
  color: var(--ion-color-medium);
  flex-shrink: 0;
}
/* hit 状态 */
.tl-counter-chip--zero {
  opacity: 0.5;
  /* 🆕 2026-06-18 v4 Bug7 修复：0-hit chip 不可点击
   *   - 旧行为：0-hit chip 点击触发 toggleFilterFromCounter，但因无 task 命中 filter，UI 无变化
   *   - 用户感受："点 chip 没反应"
   *   - 修法：pointer-events: none + cursor: not-allowed
   *   - 保留 active 状态可点击（已选中的筛选条件可点击取消）
   */
  pointer-events: none;
  cursor: not-allowed;
}
.tl-counter-chip--zero .tl-counter-chip__ratio {
  color: var(--ion-color-danger);
}
.tl-counter-chip--partial {
  background: var(--ion-color-step-100, #fff5e6);
}
.tl-counter-chip--partial .tl-counter-chip__ratio {
  color: var(--ion-color-warning-shade);
}
/* active（已选中筛选） */
.tl-counter-chip--active {
  background: var(--ion-color-primary);
  color: #fff;
  border-color: var(--ion-color-primary);
}
.tl-counter-chip--active .tl-counter-chip__ratio {
  color: rgba(255, 255, 255, 0.85);
}

/* 适配 group report 按钮颜色（与 tone 对应） */
.tl-group-card--ai_agent .group-report-btn {
  --color: var(--tl-trigger-ai-agent, #8b5cf6);
}
.tl-group-card--automation .group-report-btn {
  --color: var(--tl-trigger-automation, #4f8cff);
}

/* date popover 自定义范围紧凑排版 */
.date-custom-range {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 12px 12px;
  border-top: 1px solid var(--ion-color-step-100, rgba(128, 128, 128, 0.1));
}

/* ============================================================
   🆕 2026-06-18 v5-bug3fix L1 group card 智能样式
   - 整张 card clickable（cursor pointer + hover lift）
   - mood class：100% passed → 淡绿；失败率 > 50% → 红色警告
   - 智能命中行：仅筛选激活时显示，紧凑单行
   - 移除展开态（chevron btn / 子 task 列表 → 进入箭头）
   ============================================================ */
.tl-group-card--clickable {
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
  outline: none;
  -webkit-tap-highlight-color: rgba(var(--ion-color-primary-rgb), 0.1);
}
.tl-group-card--clickable:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}
.tl-group-card--clickable:active {
  transform: translateY(0);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.06);
}
.tl-group-card--clickable:focus-visible {
  outline: 2px solid var(--ion-color-primary);
  outline-offset: 2px;
}

/* mood 色调（不冲突：基础 tone（automation/ai_agent）保持，仅背景微调） */
.tl-group-card--mood-success {
  background: linear-gradient(135deg,
    rgba(var(--ion-color-success-rgb, 16, 185, 129), 0.04) 0%,
    rgba(var(--ion-color-success-rgb, 16, 185, 129), 0.08) 100%);
}
.tl-group-card--mood-danger {
  background: linear-gradient(135deg,
    rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.04) 0%,
    rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.1) 100%);
  border-color: rgba(var(--ion-color-danger-rgb, 239, 68, 68), 0.3);
}
.tl-group-card--mood-neutral {
  /* 默认无变化 */
}

/* 进入箭头（替代展开 chevron） */
.tl-group-card__chevron {
  color: var(--ion-color-medium);
  font-size: 22px;
  margin: 0 4px 0 0;
  flex-shrink: 0;
  transition: transform 0.15s ease, color 0.15s ease;
}
.tl-group-card--clickable:hover .tl-group-card__chevron {
  color: var(--ion-color-primary);
  transform: translateX(2px);
}

/* 自身状态行（紧凑展示 + duration） */
.tl-group-card__self {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 4px;
}
.tl-group-card__duration {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  color: var(--tl-card-text-secondary, var(--ion-color-medium-shade));
  margin-left: auto;
  font-family: var(--tl-card-font-mono, monospace);
  white-space: nowrap;
}
.tl-group-card__duration-icon {
  font-size: 11px;
  flex-shrink: 0;
}

/* 智能命中行（仅筛选激活时显示） */
.tl-group-card__hit {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  padding: 5px 10px;
  border-radius: 6px;
  background: rgba(var(--ion-color-primary-rgb, 59, 130, 246), 0.08);
  border-left: 3px solid var(--ion-color-primary);
  font-size: 11px;
  color: var(--ion-color-primary-shade, #1e40af);
  font-weight: 500;
  line-height: 1.4;
}
.tl-group-card__hit-icon {
  font-size: 12px;
  flex-shrink: 0;
  color: var(--ion-color-primary);
}
.tl-group-card__hit-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.date-range-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: var(--ion-color-medium);
  gap: 8px;
}
.date-range-label span {
  flex-shrink: 0;
  min-width: 40px;
}
.date-input {
  flex: 1;
  padding: 4px 8px;
  border: 1px solid var(--ion-color-step-200, rgba(128, 128, 128, 0.25));
  border-radius: 4px;
  font-size: 12px;
  font-family: var(--tl-card-font-mono, monospace);
  background: var(--ion-color-step-50, #fafafa);
  color: var(--ion-color-dark, #111);
  outline: none;
  min-width: 0;
}
.date-input:focus {
  border-color: var(--ion-color-primary);
}
.date-preset-active {
  --background: var(--ion-color-primary-tint, rgba(79, 140, 255, 0.1));
  font-weight: 600;
}
</style>
