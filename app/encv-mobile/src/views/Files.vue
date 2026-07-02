<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button v-if="currentPath !== '/d'" @click="goUp">
            <ion-icon :icon="arrowBack" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
        <ion-buttons slot="end">
          <ion-button fill="clear" @click="openSideDrawer()">
            <ion-icon :icon="menuOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
        <ion-title>{{ t('files.title') }}</ion-title>
      </ion-toolbar>
      <ion-toolbar v-if="currentPath !== '/d' && !isSearching">
        <div class="breadcrumb-scroll">
          <div class="breadcrumb">
            <span class="breadcrumb-item" @click="navigateTo('/d')">{{ t('files.mountRoot') || '挂载点' }}</span>
            <span v-for="(segment, index) in pathSegments" :key="index" class="breadcrumb-segment">
              <ion-icon :icon="chevronForward" class="breadcrumb-sep"></ion-icon>
              <span class="breadcrumb-item" @click="navigateTo(segment.path)">{{ segment.name }}</span>
            </span>
          </div>
        </div>
      </ion-toolbar>
      <ion-toolbar>
        <ion-searchbar
          v-model="searchQuery"
          :placeholder="t('files.searchPlaceholder')"
          @ionInput="handleSearchInput"
          @ionClear="handleSearchClear"
        ></ion-searchbar>
        <ion-toggle
          v-if="searchQuery"
          slot="end"
          v-model="searchRecursive"
          @ionChange="handleSearchToggle"
          class="recursive-toggle"
        >
          {{ t('files.recursive') }}
        </ion-toggle>
      </ion-toolbar>
    </ion-header>

    <ion-menu side="start" menu-id="plugin-menu" content-id="main-content">
      <ion-header>
        <ion-toolbar>
          <ion-title>插件分类</ion-title>
        </ion-toolbar>
      </ion-header>
      <ion-content>
        <ion-list>
          <ion-item button @click="exitPluginMode()" detail>
            <ion-icon :icon="folder" slot="start" color="primary"></ion-icon>
            <ion-label><h2>所有文件</h2><p>{{ currentPath || '/' }}</p></ion-label>
          </ion-item>
          <ion-list-header>文件类型</ion-list-header>
          <ion-item v-for="plugin in plugins" :key="plugin.name" button detail @click="openPluginView(plugin)">
            <ion-icon :icon="getPluginIcon(plugin)" slot="start" color="primary" />
            <ion-label>
              <h2>{{ plugin.name }}</h2>
              <p>{{ plugin.supportedExtensions?.length ?? 0 }} 种格式 · 容器 {{ plugin.containerExtension }}</p>
            </ion-label>
          </ion-item>
        </ion-list>

        <ion-list v-if="tags.length > 0" style="margin-top: 16px">
          <ion-list-header>标签</ion-list-header>
          <ion-item v-for="tag in tags" :key="tag.name" button @click="handleTagFilter(tag.name)">
            <ion-icon :icon="pricetagOutline" slot="start" color="success" />
            <ion-label>
              <h2>{{ tag.name }}</h2>
              <p>{{ tag.count }} 个文件</p>
            </ion-label>
          </ion-item>
        </ion-list>
      </ion-content>
    </ion-menu>

    <ion-content id="main-content" ref="mainContentRef">
      <ion-refresher slot="fixed" @ionRefresh="handleRefresh" v-if="!searchQuery">
        <ion-refresher-content></ion-refresher-content>
      </ion-refresher>

      <!-- 🆕 v4 Bug1 修复：自动更新顶栏细 indicator（不阻塞老数据渲染，不清空列表） -->
      <div v-if="refreshing" class="files-refresh-bar" aria-live="polite">
        <ion-spinner name="dots" class="files-refresh-bar__spinner"></ion-spinner>
        <span class="files-refresh-bar__label">{{ t('files.autoRefreshing') }}</span>
      </div>

      <!-- 播放错误展示区域 -->
      <div v-if="playError" class="play-error-banner">
        <div class="play-error-header">
          <ion-icon :icon="alertCircle" color="danger"></ion-icon>
          <span class="play-error-file">{{ playErrorFile }}</span>
          <ion-button fill="clear" size="small" color="medium" @click="clearPlayError">
            <ion-icon :icon="close" slot="icon-only"></ion-icon>
          </ion-button>
        </div>
        <p class="play-error-message">{{ playError }}</p>
        <div v-if="playErrorDetail" class="play-error-detail-row">
          <ion-button fill="clear" size="small" color="medium" @click="togglePlayErrorDetail">
            {{ t('common.showDetail') }}
          </ion-button>
        </div>
      </div>

      <template v-if="(loading || isSearching || noPermission || !serverOnline || displayFiles.length === 0) && !selectedPlugin">
        <div v-if="loading || isSearching" class="loading-container">
          <!--
            搜索中 loading 三态区分（debug-discipline.md §1.6）：
              - 'available'   → spinner（旋转，索引完全可用）
              - 'degraded'    → dots（安静等待，部分降级）
              - 'unavailable' / 'unknown' → pulse（脉冲闪动，索引不可用，仅关键词）
          -->
          <ion-spinner v-if="isSearching && vectorSearchStatus === 'available'" name="crescent"></ion-spinner>
          <ion-spinner v-else-if="isSearching && vectorSearchStatus === 'degraded'" name="dots"></ion-spinner>
          <ion-spinner v-else-if="isSearching" name="lines" class="search-spinner-pulse"></ion-spinner>
          <ion-spinner v-else name="crescent"></ion-spinner>
          <p>{{ isSearching ? t('files.searching') : (connecting ? t('files.connecting') : t('files.loading')) }}</p>
        </div>
        <div v-else-if="noPermission" class="empty-state">
          <ion-icon :icon="lockClosed" class="empty-icon"></ion-icon>
          <h3>{{ t('files.noPermission') }}</h3>
          <p>{{ t('files.noPermissionDesc') }}</p>
          <ion-button @click="handleRequestStorage">
            <ion-icon :icon="folderOpen" slot="start"></ion-icon>
            {{ t('files.grantPermission') }}
          </ion-button>
        </div>
        <div v-else-if="!serverOnline" class="empty-state">
          <ion-icon :icon="cloudOffline" class="empty-icon"></ion-icon>
          <h3>{{ t('files.serverOffline') }}</h3>
          <p>{{ t('files.serverOfflineDesc') }}</p>
          <ion-button @click="retryConnection">
            <ion-icon :icon="refresh" slot="start"></ion-icon>
            {{ t('files.retry') }}
          </ion-button>
        </div>
        <div v-else class="empty-state">
          <ion-icon :icon="searchQuery ? search : folderOpen" class="empty-icon"></ion-icon>
          <h3>{{ searchQuery ? t('files.noSearchResults') : t('files.emptyDir') }}</h3>
          <p>{{ searchQuery ? t('files.noSearchResultsDesc') : t('files.emptyDirDesc') }}</p>
        </div>
      </template>

      <template v-else>
        <div v-if="selectedPlugin" class="plugin-view">
            <div class="plugin-header">
              <div class="plugin-header-top">
                <ion-button fill="clear" size="small" class="plugin-back-btn" @click="exitPluginMode()">
                  <ion-icon :icon="arrowBack" slot="icon-only"></ion-icon>
                </ion-button>
                <span class="plugin-title">{{ selectedPlugin?.name }} 文件</span>
                <ion-segment v-model="pluginTab" value="source" class="plugin-segment">
                  <ion-segment-button value="source">未加密</ion-segment-button>
                  <ion-segment-button value="container">已加密</ion-segment-button>
                </ion-segment>
              </div>
              <ion-item button detail @click="showPluginFilters = !showPluginFilters" class="filter-toggle-item">
                <ion-icon :icon="filterOutline" slot="start"></ion-icon>
                <ion-label>筛选与排序</ion-label>
                <ion-badge v-if="activeFilterCount > 0" slot="end" color="primary">{{ activeFilterCount }}</ion-badge>
                <ion-badge slot="end" color="medium">{{ pluginSortLabel }}</ion-badge>
              </ion-item>
            </div>
            <div v-if="!pluginLoaded" class="loading-container">
              <ion-spinner name="crescent"></ion-spinner>
              <p>{{ t('files.loading') }}</p>
            </div>
            <template v-else>
            <div v-if="pluginFiles.length === 0" class="empty-state">
              <ion-icon :icon="folderOpen" class="empty-icon"></ion-icon>
              <h3>{{ t('files.emptyDir') }}</h3>
              <p>{{ t('settings.emptyPluginDesc', { name: selectedPlugin?.name ?? '' }) || '该类型下暂无文件' }}</p>
            </div>
            <template v-else>
            <ion-list v-if="showPluginFilters" :inset="true">
              <ion-item>
                <ion-label position="stacked">大小范围</ion-label>
                <div style="display:flex;gap:8px;align-items:center;width:100%">
                  <ion-input type="number" placeholder="最小"
                    :value="sizeFilterMin !== null ? String(sizeFilterMin) : ''"
                    @ionInput="sizeFilterMin = $event.detail.value ? Number($event.detail.value) : null">
                  </ion-input>
                  <span>~</span>
                  <ion-input type="number" placeholder="最大"
                    :value="sizeFilterMax !== null ? String(sizeFilterMax) : ''"
                    @ionInput="sizeFilterMax = $event.detail.value ? Number($event.detail.value) : null">
                  </ion-input>
                  <ion-button fill="clear" size="small" @click="sizeFilterMin=null;sizeFilterMax=null">
                    <ion-icon :icon="closeCircleOutline" slot="icon-only"></ion-icon>
                  </ion-button>
                </div>
                <div class="filter-chips">
                  <ion-chip v-for="p in SIZE_PRESETS" :key="p.label" :button="true" outline @click.stop="applySizePreset(p)">{{ p.label }}</ion-chip>
                </div>
              </ion-item>
              <ion-item>
                <ion-label position="stacked">修改时间</ion-label>
                <div style="display:flex;gap:8px;align-items:center;width:100%">
                  <ion-input type="date" placeholder="起始"
                    :value="timeFilterFrom || ''"
                    @ionInput="timeFilterFrom = ($event.detail.value as string) || null">
                  </ion-input>
                  <span>~</span>
                  <ion-input type="date" placeholder="结束"
                    :value="timeFilterTo || ''"
                    @ionInput="timeFilterTo = ($event.detail.value as string) || null">
                  </ion-input>
                  <ion-button fill="clear" size="small" @click="timeFilterFrom=null;timeFilterTo=null">
                    <ion-icon :icon="closeCircleOutline" slot="icon-only"></ion-icon>
                  </ion-button>
                </div>
                <div class="filter-chips">
                  <ion-chip v-for="p in TIME_PRESETS" :key="p.label" :button="true" outline @click.stop="applyTimePreset(p)">{{ p.label }}</ion-chip>
                </div>
              </ion-item>
              <ion-item>
                <ion-label position="stacked">排序方式</ion-label>
                <div class="filter-chips">
                  <ion-chip v-for="s in ['name', 'size', 'time'] as const" :key="s"
                    :button="true" :outline="pluginSortBy !== s" :color="pluginSortBy === s ? 'primary' : undefined"
                    @click.stop="pluginSortBy = s">
                    {{ s === 'name' ? '名称' : s === 'size' ? '大小' : '时间' }}
                  </ion-chip>
                </div>
                <div class="filter-chips" style="margin-top:4px">
                  <ion-chip :button="true" :outline="!!pluginSortDesc" :color="!pluginSortDesc ? 'primary' : undefined" @click.stop="pluginSortDesc = false">升序 ↑</ion-chip>
                  <ion-chip :button="true" :outline="!pluginSortDesc" :color="!!pluginSortDesc ? 'primary' : undefined" @click.stop="pluginSortDesc = true">降序 ↓</ion-chip>
                </div>
              </ion-item>
              <ion-item button @click="clearAllPluginFilters">
                <ion-icon :icon="closeCircleOutline" slot="start" color="danger"></ion-icon>
                <ion-label color="danger">清除所有筛选</ion-label>
              </ion-item>
            </ion-list>
            <ion-list :inset="true">
            <ion-item v-for="file in filteredPluginFiles" :key="file.path" button @click="handleFileClick(file)" v-longpress="() => handleLongPress(file)">
              <div slot="start" class="file-thumbnail-slot lazy-thumb-target" :data-file-path="file.path">
                <img
                  v-if="isImageFile(file) && thumbnailUrls[file.path]"
                  :src="thumbnailUrls[file.path]"
                  class="file-thumb"
                  loading="lazy"
                  @error="onThumbError(file.path)"
                />
                <ion-icon
                  v-else
                  :icon="getFileIcon(file)"
                  :color="getFileIconColor(file)"
                  :class="{ 'thumb-fallback': isImageFile(file) }"
                ></ion-icon>
              </div>
              <ion-label>
                <h2>{{ file.display_name || file.name }}</h2>
                <p v-if="!file.isDirectory && file.size">{{ formatFileSize(file.size) }}<span v-if="file.modified"> · {{ formatDateTime(file.modified) }}</span></p>
                <p v-else-if="file.isDirectory">{{ t('files.directory') }}</p>
                <div v-if="!file.isDirectory && file._tags && file._tags.length > 0" class="file-tag-chips">
                  <ion-chip v-for="tag in file._tags" :key="tag" size="small" color="tertiary" outline>{{ tag }}</ion-chip>
                </div>
              </ion-label>
              <ion-badge v-if="file.isEncrypted" color="warning" slot="end">
                ENCV
              </ion-badge>
            </ion-item>
            <ion-item v-if="filteredPluginFiles.length === 0">
              <ion-label>无匹配文件</ion-label>
            </ion-item>
          </ion-list>
          </template>
          </template>
        </div>

        <!--
          排序控件显示条件：
          - 非插件模式：始终显示
          - 搜索时：仅在有搜索结果时显示（避免空结果时显示无意义控件）
        -->
        <div v-if="!selectedPlugin && (!searchQuery || (searchQuery && displayFiles.length > 0))" class="main-sort-bar">
          <ion-item button detail @click="showMainSort = !showMainSort">
            <ion-icon :icon="swapVerticalOutline" slot="start"></ion-icon>
            <ion-label>{{ t('files.sortBy') || '排序' }}</ion-label>
            <ion-badge slot="end" color="medium">{{ mainSortLabel }}</ion-badge>
          </ion-item>
          <ion-list v-if="showMainSort" :inset="true">
            <ion-item>
              <ion-label position="stacked">{{ t('files.sortMethod') || '排序方式' }}</ion-label>
              <div class="filter-chips">
                <!--
                  排序选项：
                  - 搜索时：额外提供「相关度」选项（用后端返回的 hybrid score）
                    默认选中，且是降序（高相关度在前）
                  - 非搜索：name / size / time 三选一
                -->
                <ion-chip v-if="searchQuery && searchResults && searchResults.length > 0"
                  :button="true" :outline="sortBy !== 'relevance'" :color="sortBy === 'relevance' ? 'primary' : undefined"
                  @click.stop="setSortBy('relevance')">
                  {{ t('files.sortByRelevance') || '相关度' }}
                </ion-chip>
                <ion-chip v-for="s in (['name', 'size', 'time'] as const)" :key="s"
                  :button="true" :outline="sortBy !== s" :color="sortBy === s ? 'primary' : undefined"
                  @click.stop="setSortBy(s)">
                  {{ s === 'name' ? t('files.sortByName') || '名称' : s === 'size' ? t('files.sortBySize') || '大小' : t('files.sortByTime') || '时间' }}
                </ion-chip>
              </div>
              <!--
                相关度排序强制为降序（高相关度在前），不显示升降选择器。
                非相关度排序才显示升/降切换。
              -->
              <div v-if="sortBy !== 'relevance'" class="filter-chips" style="margin-top:4px">
                <ion-chip :button="true" :outline="!!sortDesc" :color="!sortDesc ? 'primary' : undefined" @click.stop="sortDesc = false">{{ t('files.sortAsc') || '升序 ↑' }}</ion-chip>
                <ion-chip :button="true" :outline="!sortDesc" :color="!!sortDesc ? 'primary' : undefined" @click.stop="sortDesc = true">{{ t('files.sortDesc') || '降序 ↓' }}</ion-chip>
              </div>
              <div v-else class="filter-chips" style="margin-top:4px">
                <ion-chip :button="false" :outline="false" color="primary">{{ t('files.sortDesc') || '降序 ↓' }}（{{ t('files.sortByRelevance') || '相关度' }}）</ion-chip>
              </div>
            </ion-item>
          </ion-list>
        </div>

        <!-- 🆕 2026-07-02 搜索模式提示（见 debug-discipline.md §3.5）：
             greedy 模式时显示徽章，告知用户结果是宽松匹配（可能不完全相关） -->
        <div v-if="searchQuery && searchResults && searchResults.length > 0 && searchMode === 'greedy'" class="search-mode-banner search-mode-greedy">
          <ion-icon :icon="pricetagOutline" class="search-mode-icon"></ion-icon>
          <span>{{ t('files.searchModeGreedy', { defaultValue: '贪婪匹配：结果为语义近似，可能不完全相关' }) }}</span>
        </div>
        <div v-else-if="searchQuery && searchResults && searchResults.length > 0 && searchMode === 'combined'" class="search-mode-banner search-mode-combined">
          <ion-icon :icon="pricetagOutline" class="search-mode-icon"></ion-icon>
          <span>{{ t('files.searchModeCombined', { defaultValue: '综合匹配：关键词 + 语义重排序' }) }}</span>
        </div>

        <ion-list>
          <ion-item
            v-for="file in displayFiles"
            :key="file.path"
            @click="handleFileClick(file)"
            v-longpress="() => handleLongPress(file)"
            :data-highlight-path="file.path"
            :class="{ 'file-highlight': highlightedPath === file.path, 'greedy-match': searchMode === 'greedy' }"
          >
            <div slot="start" class="file-thumbnail-slot lazy-thumb-target" :data-file-path="file.path">
                <img
                  v-if="isImageFile(file) && thumbnailUrls[file.path]"
                  :src="thumbnailUrls[file.path]"
                  class="file-thumb"
                  loading="lazy"
                  @error="onThumbError(file.path)"
                />
                <ion-icon
                  v-else
                  :icon="getFileIcon(file)"
                  :color="getFileIconColor(file)"
                  :class="{ 'thumb-fallback': isImageFile(file) }"
                ></ion-icon>
            </div>
            <ion-label>
              <h2>{{ file.display_name || file.name }}</h2>
              <p v-if="searchQuery && !file.isDirectory" class="search-path">{{ file.path }}</p>
              <p v-if="!file.isDirectory && file.size">{{ formatFileSize(file.size) }}<span v-if="file.modified && !searchQuery"> · {{ formatDateTime(file.modified) }}</span></p>
              <!-- 🆕 2026-06-15 multi-mount 适配：mount 伪 item 在根目录展示
                   driver badge + 真实 mount_path + resolved root_path
                   让用户能看到 "这是个 mount，不是普通目录" -->
              <p v-else-if="file.isDirectory && mountDriverOf(file)">
                <ion-badge color="primary" class="mount-driver-badge">{{ mountDriverOf(file) }}</ion-badge>
                <code class="mount-path-inline">{{ mountPathOf(file) }}</code>
                <span class="mount-root-inline">→ {{ mountRootOf(file) }}</span>
              </p>
              <p v-else-if="file.isDirectory">{{ t('files.directory') }}</p>
              <p v-for="sub in fileSubtitles[file.path]" :key="'sub-' + sub.text" class="real-name" :style="{ color: sub.color || 'var(--ion-color-danger)' }">{{ sub.text }}</p>
              <div v-if="!file.isDirectory && !searchQuery && file._tags && file._tags.length > 0" class="file-tag-chips">
                <ion-chip v-for="tag in file._tags" :key="tag" size="small" color="tertiary" outline>{{ tag }}</ion-chip>
              </div>
            </ion-label>
            <RelevanceBadge v-if="searchQuery && file.score" :score="file.score" slot="end" />
            <ion-badge v-if="file.isEncrypted" color="warning" slot="end">
              ENCV
            </ion-badge>
            <ion-badge v-for="badge in fileBadges[file.path]" :key="'badge-' + badge.text" :color="badge.color" slot="end">
              {{ badge.text }}
            </ion-badge>
            <ion-button v-if="searchQuery" slot="end" fill="clear" class="open-folder-btn" @click.stop="openContainingFolder(file)">
              <ion-icon :icon="folderOpen" class="open-folder-icon" slot="icon-only"></ion-icon>
            </ion-button>
          </ion-item>
        </ion-list>
      </template>

      <ion-alert :is-open="showRenameDialog" header="重命名"
        :inputs="renameAlertInputs"
        :buttons="[
          { text: '取消', role: 'cancel' },
          { text: '确定', handler: onRenameConfirm }
        ]"
        @didDismiss="showRenameDialog = false" />
      <ion-modal :is-open="showTagDialog" @didDismiss="showTagDialog = false">
        <ion-header>
          <ion-toolbar>
            <ion-title>标签管理</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="showTagDialog = false">完成</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content>
          <div class="tag-editor-content">
            <div v-if="editingFileTags.length > 0" class="existing-tags">
              <ion-chip v-for="tag in editingFileTags" :key="tag" color="primary" outline>
                {{ tag }}
                <ion-icon :icon="closeCircle" @click="handleRemoveTag(tag)"></ion-icon>
              </ion-chip>
            </div>
            <p v-else class="no-tags-hint">暂无标签</p>
            <div class="tag-input-row">
              <InputWithHistory
                v-model="newTagInput"
                :label="t('files.newTag')"
                placeholder="输入新标签名"
                :icon="pricetagOutline"
                history-key="files.newTag"
                @keyup-enter="handleAddNewTag()"
              />
              <ion-button fill="solid" color="primary" @click="handleAddNewTag()" :disabled="!newTagInput.trim()">
                添加
              </ion-button>
            </div>
          </div>
        </ion-content>
      </ion-modal>
      <ion-alert :is-open="showMoveDialog" header="移动到"
        :inputs="[{ name: 'target', type: 'text', placeholder: '目标路径', value: moveTargetPath }]"
        :buttons="[
          { text: '取消', role: 'cancel' },
          { text: '移动', handler: (d: any) => { moveTargetPath = d.target; handleMove(selectedFile!); } }
        ]"
        @didDismiss="showMoveDialog = false" />

    </ion-content>

    <ion-fab vertical="bottom" horizontal="end" slot="fixed">
      <ion-fab-button @click="handleUpload">
        <ion-icon :icon="add" />
      </ion-fab-button>
    </ion-fab>

    <input
      ref="fileInputRef"
      type="file"
      multiple
      style="display: none"
      @change="handleFileSelected"
    />

  </ion-page>
</template>
<script setup lang="ts">
// Files.vue 重构后只剩 thin script：调用 useFilesView() composable + 必要 imports。
// 原 1565 行 script 逻辑已全部抽到 ./useFilesView.ts。
//
// template 内直接使用的 state/handler 都在此解构到局部变量，
// Vue 3 <script setup> 自动暴露顶层 binding 给 template，所以 template 用法保持不变。

import {
  IonPage, IonHeader, IonToolbar, IonButtons, IonButton, IonTitle, IonContent,
  IonList, IonItem, IonIcon, IonLabel, IonBadge, IonSpinner, IonSearchbar, IonToggle,
  IonMenu, IonRefresher, IonRefresherContent, IonAlert, IonModal, IonInput,
  IonFab, IonFabButton, IonSegment, IonSegmentButton, IonListHeader, IonChip,
} from '@ionic/vue'
import {
  arrowBack, chevronForward, menuOutline, folder, folderOpen, lockClosed,
  cloudOffline, refresh, search, pricetagOutline, closeCircle, closeCircleOutline,
  filterOutline, swapVerticalOutline, alertCircle, close,
} from 'ionicons/icons'

import { useFilesView } from './useFilesView'
import { isImageFile, getFileIcon, getFileIconColor } from '@/composables/useFileList'
import { formatFileSize } from '@/api/encv'
import { formatDateTime } from '@/composables/useDateFormat'
import RelevanceBadge from '@/components/shared/RelevanceBadge.vue'
import InputWithHistory from '@/components/InputWithHistory.vue'

const {
  // i18n + composable re-exposed values
  t, vectorSearchStatus, thumbnailUrls, onThumbError,
  sortBy, sortDesc, mainSortLabel, setSortBy, showMainSort,
  // connection / loading state
  serverOnline, noPermission, loading, refreshing, connecting,
  // file list + sort state
  fileBadges, fileSubtitles, displayFiles,
  // path / navigation
  currentPath, pathSegments, navigateTo, goUp, highlightedPath, mainContentRef,
  openContainingFolder,
  // search state
  searchQuery, searchRecursive, searchResults, isSearching, searchMode,
  handleSearchInput, handleSearchClear, handleSearchToggle,
  // play error state
  playError, playErrorDetail, playErrorFile,
  clearPlayError, togglePlayErrorDetail,
  // plugin view state
  selectedPlugin, pluginTab, pluginFiles, pluginLoaded, showPluginFilters,
  sizeFilterMin, sizeFilterMax, timeFilterFrom, timeFilterTo,
  pluginSortBy, pluginSortDesc, pluginSortLabel, activeFilterCount, filteredPluginFiles,
  applySizePreset, applyTimePreset, clearAllPluginFilters,
  SIZE_PRESETS, TIME_PRESETS,
  // dialogs + selected
  showRenameDialog, showTagDialog, showMoveDialog,
  selectedFile, moveTargetPath,
  editingFileTags, newTagInput, renameAlertInputs,
  // menu / drawer
  plugins, tags, openSideDrawer, exitPluginMode, openPluginView, getPluginIcon, handleTagFilter,
  // file actions
  handleRefresh, retryConnection, handleRequestStorage,
  handleFileClick, handleLongPress, onRenameConfirm, handleMove,
  handleAddNewTag, handleRemoveTag,
  handleUpload, handleFileSelected, fileInputRef,
  // multi-mount 字段访问器
  mountDriverOf, mountPathOf, mountRootOf,
  // icons (template 用)
  add,
} = useFilesView()
</script>

