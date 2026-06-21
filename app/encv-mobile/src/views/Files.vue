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

    <ion-content id="main-content">
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
          <ion-spinner name="crescent"></ion-spinner>
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

        <div v-if="!selectedPlugin && !searchQuery" class="main-sort-bar">
          <ion-item button detail @click="showMainSort = !showMainSort">
            <ion-icon :icon="swapVerticalOutline" slot="start"></ion-icon>
            <ion-label>排序</ion-label>
            <ion-badge slot="end" color="medium">{{ mainSortLabel }}</ion-badge>
          </ion-item>
          <ion-list v-if="showMainSort" :inset="true">
            <ion-item>
              <ion-label position="stacked">排序方式</ion-label>
              <div class="filter-chips">
                <ion-chip v-for="s in ['name', 'size', 'time'] as const" :key="s"
                  :button="true" :outline="sortBy !== s" :color="sortBy === s ? 'primary' : undefined"
                  @click.stop="sortBy = s">
                  {{ s === 'name' ? '名称' : s === 'size' ? '大小' : '时间' }}
                </ion-chip>
              </div>
              <div class="filter-chips" style="margin-top:4px">
                <ion-chip :button="true" :outline="!!sortDesc" :color="!sortDesc ? 'primary' : undefined" @click.stop="sortDesc = false">升序 ↑</ion-chip>
                <ion-chip :button="true" :outline="!sortDesc" :color="!!sortDesc ? 'primary' : undefined" @click.stop="sortDesc = true">降序 ↓</ion-chip>
              </div>
            </ion-item>
          </ion-list>
        </div>

        <ion-list>
          <ion-item
            v-for="file in displayFiles"
            :key="file.path"
            @click="handleFileClick(file)"
            v-longpress="() => handleLongPress(file)"
            :data-highlight-path="file.path"
            :class="{ 'file-highlight': highlightedPath === file.path }"
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
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { onIonViewWillEnter } from '@ionic/vue'
import {
  IonPage,
  IonHeader,
  IonToolbar,
  IonTitle,
  IonButtons,
  IonButton,
  IonContent,
  IonRefresher,
  IonRefresherContent,
  IonList,
  IonItem,
  IonIcon,
  IonLabel,
  IonBadge,
  IonSpinner,
  IonSearchbar,
  IonToggle,
  actionSheetController,
  alertController,
  menuController,
  IonAlert,
  IonMenu,
  IonSegment,
  IonSegmentButton,
  IonListHeader,
  IonChip,
  IonModal,
  IonInput,
  IonFab,
  IonFabButton,
} from '@ionic/vue'
import {
  arrowBack,
  chevronForward,
  folder,
  folderOpen,
  eyeOutline,
  playCircle,
  lockClosed,
  cloudOffline,
  refresh,
  trash,
  search,
  informationCircle,
  menuOutline,
  pricetagOutline,
  createOutline,
  copyOutline,
  arrowForwardOutline,
  shareOutline,
  closeCircle,
  closeCircleOutline,
  filterOutline,
  swapVerticalOutline,
  alertCircle,
  close,
  filmOutline,
  add,
  musicalNotesOutline,
  imageOutline,
  documentTextOutline,
  documentOutline,
} from 'ionicons/icons'
import {
  listFiles,
  listFilesStream,
  listPluginFilesStream,
  searchFiles,
  formatFileSize,
  getFileCategory,
  PermissionDeniedError,
  NotFoundError,
  deleteFile,
  renameFile,
  renameOriginalName,
  copyFile,
  moveFile,
  uploadFile,
  fetchPlugins,
  fetchTags,
  addTag,
  removeTag,
  listFilesByTag,
  getFileInfo,
} from '@/api/encv'
import type { FileItem, PluginMeta, TagInfo } from '@/api/encv'
import { eventBus } from '@/composables/useEventBus'
import { useI18n } from '@/composables/useI18n'
import { formatDateTime } from '@/composables/useDateFormat'
import { useRealtimeTransport } from '@/composables/useRealtimeTransport'
import { useThumbnailCache } from '@/composables/useThumbnailCache'
import { useFileFeatures, findClickHandler, isAnyContainerFile, getFeatureIcon } from '@/composables/useFileFeatures'
import { preloadSubtitles } from '@/features/alist-encrypt'
import { isAlistEncrypted, getSessionPassword, setSessionPassword, loadDecodedName, getDecodedName } from '@/features/alist-encrypt/useAlistEncrypt'
import { promptPassword } from '@/features/alist-encrypt/password-dialog'
import {
  isImageFile,
  getFileIcon,
  getFileIconColor,
  useFileListSort,
  sortFiles,
} from '@/composables/useFileList'
import { vLongpress } from '@/directives/longpress'
import { isNative, requestStoragePermission, openPlayer, openExternal, getLocalFilePath } from '@/plugins/GoProcess'
import { getExternalStreamUrl } from '@/api/encv'
import { copyToClipboard } from '@/composables/useClipboard'
import { showToast } from '@/composables/useToast'
import { Share } from '@capacitor/share'
import { PLAY_MODE, type PlayMode, VIDEO_DEFAULT, AUDIO_DEFAULT } from '@/constants/player'
import InputWithHistory from '@/components/InputWithHistory.vue'

const ALL_VALID_MODES: PlayMode[] = [
  PLAY_MODE.ARTPLAYER,
  PLAY_MODE.MPV_PLUGIN,
  PLAY_MODE.MPV_ACTIVITY,
  PLAY_MODE.MPV_FRAGMENT,
  PLAY_MODE.MPV_COMPOSE,
  PLAY_MODE.EXTERNAL,
]

function isValidPlayMode(value: string): value is PlayMode {
  return (ALL_VALID_MODES as readonly string[]).includes(value)
}

function getPlayMode(mediaType: 'video' | 'audio'): PlayMode {
  const key = mediaType === 'video' ? 'encv_player_video' : 'encv_player_audio'
  const stored = localStorage.getItem(key)
  if (stored && isValidPlayMode(stored)) return stored
  return mediaType === 'video' ? VIDEO_DEFAULT : AUDIO_DEFAULT
}

const playError = ref<string>('')
const playErrorDetail = ref<string>('')
const playErrorFile = ref<string>('')

async function playMedia(file: FileItem, category: string) {
  const isVideo = category === 'video'
  const mediaType = isVideo ? 'video' : 'audio'
  const mimeType = isVideo ? 'video/*' : 'audio/*'
  const mode = getPlayMode(mediaType)

  console.info('[Files] playMedia: file=', file.path, 'mode=', mode, 'category=', category)
  playError.value = ''
  playErrorDetail.value = ''
  playErrorFile.value = ''

  switch (mode) {
    case PLAY_MODE.ARTPLAYER:
      router.push({ path: '/player', query: { path: file.path, name: file.name } })
      break
    case PLAY_MODE.MPV_PLUGIN:
    case PLAY_MODE.MPV_ACTIVITY:
    case PLAY_MODE.MPV_FRAGMENT:
    case PLAY_MODE.MPV_COMPOSE:
      if (isNative()) {
        const result = await openPlayer(file.path, file.name, mimeType, mode)
        if (!result.success) {
          console.error('[Files] playMedia failed:', result.error, result.errorDetail)
          playError.value = result.error || '播放失败'
          playErrorDetail.value = result.errorDetail || ''
          playErrorFile.value = file.name
        }
      } else {
        router.push({ path: '/player', query: { path: file.path, name: file.name } })
      }
      break
    case PLAY_MODE.EXTERNAL:
      if (isNative()) {
        const url = getExternalStreamUrl(file.path)
        openExternal(url, mimeType)
      } else {
        router.push({ path: '/player', query: { path: file.path, name: file.name } })
      }
      break
    default:
      console.debug('[Files] Unknown play mode:', mode, '— falling back to artplayer')
      router.push({ path: '/player', query: { path: file.path, name: file.name } })
      break
  }
}

function clearPlayError() {
  playError.value = ''
  playErrorDetail.value = ''
  playErrorFile.value = ''
}

function togglePlayErrorDetail() {
  if (playErrorDetail.value) {
    const expanded = playErrorDetail.value
    playErrorDetail.value = ''
    playError.value = playError.value + '\n' + expanded
  }
}

const { t } = useI18n()
const { thumbnailUrls, setupLazyThumbnails, onThumbError } = useThumbnailCache()
const { sortBy, sortDesc } = useFileListSort()
const showMainSort = ref(false)

const mainSortLabel = computed(() => {
  const map: Record<string, string> = { name: '名称', size: '大小', time: '时间' }
  return (map[sortBy.value] || '名称') + (sortDesc.value ? '↓' : '↑')
})
const router = useRouter()
const route = useRoute()
const serverOnline = ref(false)
const noPermission = ref(false)
const files = ref<FileItem[]>([])
const plugins = ref<PluginMeta[]>([])
const tags = ref<TagInfo[]>([])
const showRenameDialog = ref(false)
const showTagDialog = ref(false)
const showMoveDialog = ref(false)
const selectedPlugin = ref<PluginMeta | null>(null)
const selectedFile = ref<FileItem | null>(null)
const renameValue = ref('')
const renamePassword = ref('')
const moveTargetPath = ref('')
const editingFileTags = ref<string[]>([])
const newTagInput = ref('')
const fileTagMap = ref<Record<string, string[]>>({})
const fileBadges = ref<Record<string, any[]>>({})
const fileSubtitles = ref<Record<string, any[]>>({})

const { getBadges, getSubtitles, getAllActions } = useFileFeatures()

const renameAlertInputs = computed(() => {
  const inputs: any[] = [{ name: 'name', type: 'text', placeholder: '新文件名', value: renameValue.value }]
  if (selectedFile.value?.isEncrypted) {
    inputs.push({ name: 'password', type: 'text', placeholder: '文件名加密密码（如需要）' })
  }
  return inputs
})

// 🆕 2026-06-15 multi-mount 适配：currentPath 起始改为 '/d'（mount 虚拟根）
//   旧版：'/' 直接是 serving dir（一个根目录）
//   新版：'/' 不再合法；根是 '/d'，下面是 mount 列表（primary / automation / sandbox）
//   后端 /api/files?path=/d → mount registry 解析为 mount 列表
//   后续 /api/files?path=/d/<name>/... → 走对应 mount driver
const MOUNT_ROOT = '/d'
const currentPath = ref(MOUNT_ROOT)
const loading = ref(false)
const refreshing = ref(false)
const connecting = ref(false)
let firstLoad = true
// 🆕 v4 Bug1 修复（v2）：记录上次成功加载的 path，只有真正切换路径才清空列表
//   之前用 files.value[0]?.path !== currentPath.value 判断是错的：
//   files[0] 是当前目录下的子文件，path 永远不等于 currentPath，
//   导致 file:change 自动刷新也走 path-change 分支 → 清空列表 → 闪 loading。
const lastLoadedPath = ref<string>('')

// 🆕 v3 2026-06-18 Task 8：route.query 驱动的文件高亮
//   - TaskDetailModal.locateOutput 跳转 /tabs/files?path=<dir>&highlight=<name>
//   - onIonViewWillEnter 读取 query，导航到目录后等待 loadFiles 完成，再滚动 + 高亮
//   - highlightedPath 保存当前要高亮的 file.path（用于 ion-item class 绑定）
const pendingHighlight = ref<string | null>(null)
const highlightedPath = ref<string | null>(null)
let highlightTimer: ReturnType<typeof setTimeout> | null = null

// 🆕 2026-06-15 multi-mount 适配：mount 根 /d 由后端 handleListFilesGin 返 mount 列表
//   - 后端响应里 mount 伪 item 含 mount_driver / mount_path / mount_root 字段
//   - frontend Files.vue 通过 mountDriverOf(file) / mountPathOf(file) / mountRootOf(file) 访问
//   - 这里不再维护本地 mounts ref（避免与文件列表数据源不一致）
const isMountRoot = computed(() => currentPath.value === MOUNT_ROOT)

const searchQuery = ref('')
const searchRecursive = ref(false)
const searchResults = ref<FileItem[] | null>(null)
const isSearching = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchGeneration = 0

const searchCache = new Map<string, { timestamp: number; results: FileItem[] }>()
const CACHE_TTL = 30000

const MAX_RETRIES = isNative() ? 15 : 3
const RETRY_DELAY = 1000

const pathSegments = computed(() => {
  if (currentPath.value === '/d') return [] // mount 根：面包屑在 ion-toolbar v-if 控制不显示
  // 🆕 2026-06-15 multi-mount 适配：面包屑第一段固定是 'd'（mount 根名）
  //   旧版：path='/' → 空 parts → []
  //   新版：path='/d/<name>/<sub>' → 显示 [d, <name>, <sub>]
  //   第一段点 → 跳到 /d（mount 根）
  if (currentPath.value === MOUNT_ROOT) return []
  const parts = currentPath.value.split('/').filter(Boolean)
  return parts.map((name, index) => {
    let displayName = name
    if (index === 0 && name === 'd') {
      displayName = t('files.mountRoot') || '挂载点' // 第一段显示成"挂载点"
    } else if (index === 1) {
      // mount name 段：从 files.value 里找 mount 伪 item（mount_driver 字段存在），
      // 找不到就原样用路径段（容错）
      const m = files.value.find(
        (f) =>
          f.isDirectory &&
          mountDriverOf(f) != null &&
          f.name === name,
      )
      if (m) displayName = m.name
    }
    return {
      name: displayName,
      path: '/' + parts.slice(0, index + 1).join('/'),
    }
  })
})

const displayFiles = computed(() => {
  const raw = searchResults.value !== null ? searchResults.value : sortedFiles.value
  const tagMap = fileTagMap.value
  return raw.map(f => ({ ...f, _tags: tagMap[f.path] || [] }))
})

const sortedFiles = computed(() => {
  return sortFiles(files.value, sortBy.value, sortDesc.value)
})

let loadGeneration = 0
/** stream 进行中标志：避免 file:change 在 stream 未完成时触发清空重载 */
let isStreamLoading = false

// 🆕 2026-06-15 multi-mount 适配：根目录 /d 由后端 handleListFilesGin 返 mount 列表
//   旧前端逻辑：currentPath='/' → 后端列 serving dir 子目录
//   新前端逻辑：currentPath='/d' → 后端 mountListAsFiles 返 mount 列表 → Files.vue 当目录展示
//   mount 伪 item 包含 mount_driver / mount_path / mount_root 字段
//   进 mount 后 currentPath='/d/<name>' → listFiles 走 mount.Resolve → 列 mount root 下的文件
async function loadFiles() {
  // mount 根走 listFiles 走 stream 同样路径 /api/files/stream?path=/d → 后端 mountListAsFiles
  //   让前端只需维护一个 loadFiles 路径
  console.info('[Files] Loading files (stream), path:', currentPath.value)
  const gen = ++loadGeneration
  isStreamLoading = true
  // 🆕 v6 2026-06-22：stream 启动时清空累积的 file:change（stream 会拉全量，增量无意义）
  pendingFileChanges.clear()
  // 🆕 v4 Bug1 修复：自动更新（file:change）下不闪全屏 loading
  //   - 首次加载（isInitialLoad）/ 路径切换 → 全屏 loading + 清空
  //   - 自动 reload（isRefresh）→ 顶部 spinner + 清空（不闪全屏 loading，但避免 stream 追加导致重复）
  const isPathChange = currentPath.value !== lastLoadedPath.value
  const isInitialLoad = files.value.length === 0 && firstLoad === true
  if (isPathChange || isInitialLoad) {
    loading.value = true
  } else {
    refreshing.value = true  // 顶部小 indicator（不闪全屏 loading）
  }
  files.value = []  // 🆕 始终清空：避免 stream push 追加到老数据后面导致重复
  firstLoad = false
  connecting.value = false
  noPermission.value = false

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    if (gen !== loadGeneration) return

    try {
      const result = await listFilesStream(currentPath.value, (file) => {
        if (gen !== loadGeneration) return
        files.value.push(file)
        if (files.value.length === 1 && loading.value) {
          loading.value = false
          console.info('[Files] First item arrived, UI unlocked')
        }
      })

      serverOnline.value = true
      noPermission.value = false
      loading.value = false
      refreshing.value = false
      connecting.value = false
      console.info('[Files] Stream complete, total:', result.files.length, 'files')

      lastLoadedPath.value = currentPath.value
      loadFileTagsForCurrentDir()

      // 🆕 v3 2026-06-18 Task 8：loadFiles 完成后处理 pendingHighlight
      if (pendingHighlight.value) {
        const name = pendingHighlight.value
        pendingHighlight.value = null
        // nextTick 等 ion-item 渲染完再滚动 + 高亮
        nextTick(() => highlightFile(name))
      }
      // 🆕 v6 2026-06-22：stream 完成后 flush 累积的 file:change 增量更新（不全量 reload）
      if (pendingFileChanges.size > 0) {
        void applyFileChanges()
      }
      return
    } catch (error) {
      if (error instanceof PermissionDeniedError) {
        serverOnline.value = true
        noPermission.value = true
        loading.value = false
        refreshing.value = false
        connecting.value = false
        if (pendingFileChanges.size > 0) { void applyFileChanges() }
        return
      }
      if (error instanceof NotFoundError) {
        serverOnline.value = true
        loading.value = false
        refreshing.value = false
        connecting.value = false
        if (currentPath.value !== '/d') {
          showToast({ message: t('files.pathNotFound'), duration: 2000, color: 'warning' })
          goUp()
        }
        if (pendingFileChanges.size > 0) { void applyFileChanges() }
        return
      }
      if (attempt < MAX_RETRIES) {
        connecting.value = true
        await new Promise(r => setTimeout(r, RETRY_DELAY))
      }
    } finally {
      // 只有当前 generation 的 stream 才清理标志，避免老 callback 误清
      if (gen === loadGeneration) {
        isStreamLoading = false
      }
    }
  }

  if (gen !== loadGeneration) return
  serverOnline.value = false
  loading.value = false
  refreshing.value = false
  connecting.value = false
}

async function handleRefresh(event: CustomEvent) {
  if (selectedPlugin.value) {
    pluginFiles.value = []
    pluginLoaded.value = false
    try {
      const results = await searchPluginFiles(selectedPlugin.value)
      pluginFiles.value = results
    } catch (e) {
      console.debug('[Files] Plugin refresh failed:', e)
    } finally {
      pluginLoaded.value = true
    }
  } else {
    try {
      files.value = await listFiles(currentPath.value)
      serverOnline.value = true
      noPermission.value = false
      loadFileTagsForCurrentDir()
    } catch (error) {
      if (error instanceof PermissionDeniedError) {
        serverOnline.value = true
        noPermission.value = true
      }
      if (error instanceof NotFoundError) {
        serverOnline.value = true
        if (currentPath.value !== '/d') {
          goUp()
        }
      }
    }
  }
  ;(event.target as any)?.complete?.()
}

async function retryConnection() {
  await loadFiles()
}

async function handleRequestStorage() {
  console.info('[Files] Requesting storage permission')
  await requestStoragePermission()
  setTimeout(() => loadFiles(), 1500)
}

function navigateTo(path: string) {
  currentPath.value = path
  searchQuery.value = ''
  searchResults.value = null
  loadFiles()
}

// 🆕 v3 2026-06-18 Task 8：高亮指定文件（route.query.highlight 驱动）
//   - 在 files.value 中按 name 找匹配项（basename 匹配，避免路径前缀差异）
//   - scrollIntoView 滚动到目标 ion-item
//   - 临时加 file-highlight class（2 秒后移除）
function highlightFile(name: string) {
  if (!name) return
  // 在已加载的 files 中找 basename 匹配
  const target = files.value.find((f) => f.name === name)
  if (!target) {
    console.info('[Files] highlightFile: target not found in current dir:', name)
    return
  }
  highlightedPath.value = target.path
  // 滚动到目标 ion-item（用 data-file-path 属性定位）
  nextTick(() => {
    const el = document.querySelector<HTMLElement>(
      `ion-item[data-highlight-path="${CSS.escape(target.path)}"]`,
    )
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
      console.info('[Files] highlightFile: scrolled to', target.path)
    } else {
      console.info('[Files] highlightFile: element not found for path:', target.path)
    }
  })
  // 2 秒后移除高亮
  if (highlightTimer) clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    highlightedPath.value = null
    highlightTimer = null
  }, 2000)
}

function openContainingFolder(file: FileItem) {
  // 🆕 2026-06-15 multi-mount 适配：父目录 = /d/<mount_name>
  //   旧版：file.path='/foo/bar.txt' → parent='/foo'
  //   新版：file.path='/d/primary/foo/bar.txt' → parent='/d/primary'（不能进 /d/foo）
  const parts = file.path.split('/').filter(Boolean)
  let parentDir: string
  if (parts.length >= 2 && parts[0] === 'd') {
    // /d/<name>/... → 父 = /d/<name>
    parentDir = '/' + parts.slice(0, 2).join('/')
  } else {
    parentDir = file.path.substring(0, file.path.lastIndexOf('/')) || MOUNT_ROOT
  }
  searchQuery.value = ''
  searchResults.value = null
  navigateTo(parentDir)
}

function goUp() {
  // 🆕 2026-06-15 multi-mount 适配：
  //   旧版：'/' 不能再上；其他路径弹掉最后一段
  //   新版：'/d' 不能再上；'/d/<name>' 上到 '/d'；'/<more>' 弹最后一段
  if (isMountRoot.value) return
  const parts = currentPath.value.split('/').filter(Boolean)
  if (parts.length === 2 && parts[0] === 'd') {
    // 当前在 /d/<name> 顶层 → 退到 /d
    currentPath.value = MOUNT_ROOT
  } else {
    parts.pop()
    currentPath.value = parts.length === 0 ? MOUNT_ROOT : '/' + parts.join('/')
  }
  searchQuery.value = ''
  searchResults.value = null
  loadFiles()
}

async function handleFileClick(file: FileItem) {
  const clickResult = await findClickHandler(file)
  if (clickResult?.handled) {
    const cached = getSessionPassword(file.path)
    let password: string | undefined | null = cached
    if (!password) {
      password = await promptPassword(file.name)
      if (!password) return
    }
    setSessionPassword(file.path, password)
    if (isAlistEncrypted(file)) {
      await loadDecodedName(file, password)
    }
    const displayName = isAlistEncrypted(file)
      ? (getDecodedName(file.path) || file.name)
      : file.name
    router.push({ path: '/player', query: { path: file.path, name: displayName, alistPath: file.path, alistPassword: password } })
    return
  }

  if (file.isDirectory) {
    // 🆕 2026-06-15 multi-mount 适配：根目录已是 /d
    //   旧版：'/' + name = '/<name>'
    //   新版：'/d' + name = '/d/<name>'
    const base = currentPath.value === '/d' ? '/d' : currentPath.value
    const newPath = base + '/' + file.name
    navigateTo(newPath)
    return
  }

  if (isAlistEncrypted(file)) {
    const password = await promptPassword(file.name)
    if (!password) return
    router.push({ path: '/player', query: { path: file.path, name: file.name, alistPath: file.path, alistPassword: password } })
    return
  }

  if (file.isEncrypted) {
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: 'true' },
    })
    return
  }

  const category = getFileCategory(file.name)
  console.info('[Files] Click:', file.name, 'category:', category)
  if (category === 'video' || category === 'audio') {
    playMedia(file, category)
  } else {
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: 'false' },
    })
  }
}

function handleSearchInput() {
  const query = searchQuery.value.trim()
  if (!query) {
    searchGeneration++
    searchResults.value = null
    isSearching.value = false
    return
  }
  searchTimer = setTimeout(() => performSearch(), searchRecursive.value ? 600 : 300)
}

function handleSearchClear() {
  searchGeneration++
  searchQuery.value = ''
  searchResults.value = null
  isSearching.value = false
}

function handleSearchToggle() {
  if (searchQuery.value.trim()) {
    performSearch()
  }
}

async function performSearch() {
  const query = searchQuery.value.trim()
  if (!query) return
  if (selectedPlugin.value) return

  if (isSearching.value) {
    searchGeneration++
  }
  const gen = ++searchGeneration

  const cacheKey = `${currentPath.value}:${query}:${searchRecursive.value}`
  const cached = searchCache.get(cacheKey)
  if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
    searchResults.value = cached.results
    return
  }

  isSearching.value = true
  try {
    const results = await searchFiles(currentPath.value, query, searchRecursive.value)
    if (gen !== searchGeneration) return
    searchResults.value = results
    searchCache.set(cacheKey, { timestamp: Date.now(), results })
  } catch {
    if (gen !== searchGeneration) return
    searchResults.value = []
  }
  isSearching.value = false
}

async function handleLongPress(file: FileItem) {
  const category = file.isDirectory ? 'directory' : getFileCategory(file.name)

  const buttons: any[] = []

  // ===== Section 1: 查看 / 打开 =====
  buttons.push({
    text: t('files.info'),
    icon: informationCircle,
    cssClass: 'action-section-view',
    handler: () => {
      router.push({ path: '/tabs/file-info', query: { path: file.path, name: file.name } })
    },
  })

  if (file.isDirectory) {
    buttons.push({
      text: t('files.open'),
      icon: folderOpen,
      cssClass: 'action-section-view',
      handler: () => {
        // 🆕 2026-06-15 multi-mount 适配：根目录 /d + name
        const base = currentPath.value === '/d' ? '/d' : currentPath.value
        const newPath = base + '/' + file.name
        navigateTo(newPath)
      },
    })
  } else if (file.isEncrypted) {
    buttons.push({
      text: t('files.preview'),
      icon: eyeOutline,
      cssClass: 'action-section-view',
      handler: () => {
        router.push({
          path: '/tabs/preview',
          query: { path: file.path, name: file.name, isEncrypted: 'true' },
        })
      },
    })
  } else {
    const isMedia = category === 'video' || category === 'audio'

    const featureActions = await getAllActions(file)
    for (const fa of featureActions) {
      buttons.push({
        text: fa.text(),
        icon: fa.icon,
        cssClass: 'action-section-view',
        ...(fa.color ? { role: undefined, cssClass: `action-section-view action-color-${fa.color}` } : {}),
        handler: () => {
          fa.handler(file)
        },
      })
    }

    buttons.push({
      text: isMedia ? t('files.play') : t('files.preview'),
      icon: isMedia ? playCircle : eyeOutline,
      cssClass: 'action-section-view',
      handler: () => {
        if (isMedia) {
          playMedia(file, category)
        } else {
          router.push({
            path: '/tabs/preview',
            query: { path: file.path, name: file.name, isEncrypted: 'false' },
          })
        }
      },
    })
  }

  // ===== Section 3: 文件管理 =====
  buttons.push({
    text: '重命名',
    icon: createOutline,
    cssClass: 'action-section-manage',
    handler: () => {
      selectedFile.value = file
      renameValue.value = file.name
      renamePassword.value = ''
      showRenameDialog.value = true
    },
  })
  buttons.push({
    text: '复制',
    icon: copyOutline,
    cssClass: 'action-section-manage',
    handler: () => {
      handleCopy(file)
    },
  })
  buttons.push({
    text: '移动',
    icon: arrowForwardOutline,
    cssClass: 'action-section-manage',
    handler: () => {
      selectedFile.value = file
      moveTargetPath.value = currentPath.value
      showMoveDialog.value = true
    },
  })
  buttons.push({
    text: '分享',
    icon: shareOutline,
    cssClass: 'action-section-manage',
    handler: () => {
      handleShare(file)
    },
  })
  buttons.push({
    text: '标签管理',
    icon: pricetagOutline,
    cssClass: 'action-section-manage',
    handler: async () => {
      selectedFile.value = file
      newTagInput.value = ''
      editingFileTags.value = []
      showTagDialog.value = true
      try {
        const allTags = await fetchTags()
        editingFileTags.value = allTags
          .filter(t => t.count > 0)
          .map(t => t.name)
          .slice(0, 10)
      } catch {}
    },
  })

  // ===== Section 4: 危险操作 =====
  // 🆕 2026-06-10 修复：文件夹长按菜单缺少删除操作
  // 历史 bug：line 1041 `if (!file.isDirectory)` 阻止文件夹删除
  // 修复：去掉 !file.isDirectory 条件 — 文件 / 文件夹都能删除
  buttons.push({
    text: t('files.delete'),
    icon: trash,
    role: 'destructive',
    cssClass: 'action-section-danger',
    handler: () => {
      handleDeleteFile(file)
    },
  })

  buttons.push({
    text: t('files.cancelSelect'),
    role: 'cancel',
  })

  const actionSheet = await actionSheetController.create({
    header: file.name,
    buttons,
    cssClass: 'file-action-sheet',
  })
  await actionSheet.present()
}

async function handleCopy(file: FileItem) {
  const ext = file.name.includes('.') ? '.' + file.name.split('.').pop() : ''
  const baseName = ext ? file.name.slice(0, -ext.length) : file.name
  const destName = `${baseName}_copy${ext}`
  // 🆕 2026-06-15 multi-mount 适配：根目录 /d 而不是 /
  const destPath = currentPath.value === '/d' ? `/d/${destName}` : `${currentPath.value}/${destName}`
  try {
    await copyFile(file.path, destPath)
    await loadFiles()
  } catch (e) { showToast({ message: `复制失败: ${e}` }) }
}

function onRenameConfirm(d: any) {
  renameValue.value = d.name ?? renameValue.value
  renamePassword.value = d.password ?? ''
  if (selectedFile.value) handleRename(selectedFile.value)
}

async function handleRename(file: FileItem) {
  if (!renameValue.value.trim() || renameValue.value === file.name) return
  try {
    if (file.isEncrypted) {
      const result = await renameOriginalName(file.path, renameValue.value.trim(), renamePassword.value.trim() || undefined)
      if (result.success) {
        showToast({ message: '原始文件名已更新' })
      }
    } else {
      await renameFile(file.path, renameValue.value.trim())
    }
    showRenameDialog.value = false
    renamePassword.value = ''
    await loadFiles()
  } catch (e) { showToast({ message: `重命名失败: ${e}` }) }
}

async function handleMove(file: FileItem) {
  if (!moveTargetPath.value || moveTargetPath.value === file.path) return
  try {
    const destPath = moveTargetPath.value.endsWith('/') ? `${moveTargetPath.value}${file.name}` : `${moveTargetPath.value}/${file.name}`
    await moveFile(file.path, destPath)
    showMoveDialog.value = false
    await loadFiles()
  } catch (e) { showToast({ message: `移动失败: ${e}` }) }
}

async function handleShare(file: FileItem) {
  if (isNative()) {
    try {
      const localPath = await getLocalFilePath(file.path)
      if (localPath) {
        await Share.share({ title: file.name, url: 'file://' + localPath })
      } else {
        showToast({ message: '仅支持本地文件分享', duration: 2500, color: 'warning' })
      }
    } catch (e) { showToast({ message: '分享失败或已取消' }) }
  } else {
    copyToClipboard(getExternalStreamUrl(file.path)).then(ok => showToast({ message: ok ? '链接已复制到剪贴板' : '复制失败', color: ok ? 'success' : 'danger' }))
  }
}

const fileInputRef = ref<HTMLInputElement>()

function handleUpload() {
  fileInputRef.value?.click()
}

async function handleFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files?.length) return

  const files = Array.from(input.files)
  let successCount = 0
  let failCount = 0

  for (const file of files) {
    try {
      showToast({ message: `正在上传: ${file.name}...`, color: 'primary', duration: 2000 })
      await uploadFile(currentPath.value, file)
      successCount++
    } catch (e) {
      console.error('[Files] upload failed:', file.name, e instanceof Error ? `${e.name}: ${e.message}` : String(e))
      failCount++
    }
  }

  if (successCount > 0) {
    showToast({
      message: `成功上传 ${successCount} 个文件${failCount > 0 ? `，${failCount} 个失败` : ''}`,
      color: failCount > 0 ? 'warning' : 'success',
      duration: 3000,
    })
    await loadFiles()
  }

  input.value = ''
}

async function handleAddNewTag() {
  if (!selectedFile.value || !newTagInput.value.trim()) return
  const tag = newTagInput.value.trim()
  if (editingFileTags.value.includes(tag)) {
    newTagInput.value = ''
    return
  }
  try {
    await addTag(selectedFile.value.path, tag)
    editingFileTags.value.push(tag)
    newTagInput.value = ''
  } catch (e) { showToast({ message: '添加标签失败' }) }
}

async function handleRemoveTag(tag: string) {
  if (!selectedFile.value) return
  try {
    await removeTag(selectedFile.value.path, tag)
    editingFileTags.value = editingFileTags.value.filter(t => t !== tag)
  } catch (e) { showToast({ message: '移除标签失败' }) }
}

async function loadFileTagsForCurrentDir() {
  try {
    const allTags = await fetchTags()
    const map: Record<string, string[]> = {}
    for (const tag of allTags) {
      if (tag.count > 0) {
        for (const f of files.value) {
          if (!map[f.path]) map[f.path] = []
          map[f.path].push(tag.name)
        }
      }
    }
    fileTagMap.value = map
  } catch {}

  const badgesMap: Record<string, any[]> = {}
  const subtitlesMap: Record<string, any[]> = {}
  for (const f of files.value) {
    const badges = await getBadges(f)
    if (badges.length > 0) badgesMap[f.path] = badges
    const subs = await getSubtitles(f)
    if (subs.length > 0) subtitlesMap[f.path] = subs
  }
  fileBadges.value = badgesMap
  fileSubtitles.value = subtitlesMap

  preloadSubtitles(files.value)
  setupLazyThumbnails()
}

async function handleDeleteFile(file: FileItem) {
  // 🆕 2026-06-10 修复 #1：删除安全防御
  //   1) 根目录客户端拦截
  //   2) 文件夹二次确认（防误删整目录）
  //   3) 文件夹删除前先 list 一次，让用户看到"包含 N 个文件 + M 个子目录"
  //   4) 详细错误 toast（把后端 error 透传给用户）
  if (file.path === '/' || file.path === '') {
    showToast({ message: '不能删除根目录', duration: 2000, color: 'danger' })
    return
  }

  if (file.isDirectory) {
    // 🆕 删除文件夹前先 list 一次，让用户在确认前看到包含多少内容
    let detail = '此操作不可撤销'
    try {
      const list = await listFiles(file.path)
      const filesInDir = list.filter((f: FileItem) => !f.isDirectory).length
      const subDirs = list.filter((f: FileItem) => f.isDirectory).length
      detail = `包含 ${filesInDir} 个文件 + ${subDirs} 个子目录，此操作不可撤销。`
    } catch (e) {
      // list 失败：仍允许删除（用户可能想强制删一个无法 list 的目录）
      console.warn('[Files] list directory failed before delete:', file.path, e)
    }
    const dirAlert = await alertController.create({
      header: t('files.delete'),
      subHeader: `📁 ${file.name}`,
      message: `确认删除文件夹 "${file.name}" 及其所有内容？\n\n${detail}`,
      buttons: [
        { text: t('files.cancelSelect'), role: 'cancel' },
        { text: t('files.delete'), role: 'destructive', handler: () => doDelete(file) },
      ],
    })
    await dirAlert.present()
  } else {
    // 文件删除用普通 alert
    const alert = await alertController.create({
      header: t('files.delete'),
      message: t('files.deleteConfirm', { name: file.name }),
      buttons: [
        { text: t('files.cancelSelect'), role: 'cancel' },
        { text: t('files.delete'), role: 'destructive', handler: () => doDelete(file) },
      ],
    })
    await alert.present()
  }
}

async function doDelete(file: FileItem) {
  try {
    await deleteFile(file.path)
    await loadFiles()
    showToast({ message: `已删除 ${file.name}`, duration: 1500, color: 'success' })
  } catch (e) {
    // 🆕 把后端 error message 完整透传给用户（不只说"删除失败"）
    const msg = e instanceof Error ? e.message : String(e)
    console.error('[Files] deleteFile failed:', file.path, msg)
    showToast({ message: `${t('files.deleteFailed')}: ${msg}`, duration: 3500, color: 'danger' })
  }
}

// 🆕 v6 2026-06-22：file:change 增量更新（真正的文件变更，不全量 reload）
//   - 后端 payload: { path, action: 'create'|'delete'|'modify' }
//   - 防抖 300ms 内合并多次事件（latest action per path wins）
//   - delete: 直接从 files.value 移除（无 API 调用）
//   - create/modify: 仅当文件父目录 === currentPath 时，调 getFileInfo 拿单文件 metadata 增删改
//   - stream 进行中时延后应用（pendingFileChanges），避免与 stream push 冲突
let fileChangeDebounceTimer: number | null = null
/** 防抖窗口内累积的事件：path → latest action */
const pendingFileChanges = new Map<string, 'create' | 'delete' | 'modify'>()

function onFileChange(payload: { path: string; action: 'create' | 'delete' | 'modify' }) {
  // searchCache 仍需清空（搜索结果可能过期）
  searchCache.clear()
  pendingFileChanges.set(payload.path, payload.action)
  if (fileChangeDebounceTimer !== null) {
    clearTimeout(fileChangeDebounceTimer)
  }
  fileChangeDebounceTimer = window.setTimeout(() => {
    fileChangeDebounceTimer = null
    // stream 进行中：延后到 stream 完成后再 apply（loadFiles 末尾会 flush）
    if (isStreamLoading) {
      console.info('[Files] file:change deferred, stream loading', pendingFileChanges.size, 'changes')
      return
    }
    void applyFileChanges()
  }, 300)
}

/**
 * 应用累积的 file:change 增量更新到 files.value
 * - delete: 直接 splice 移除
 * - create/modify: 仅处理父目录 === currentPath 的文件，调 getFileInfo 拿 metadata
 *   - 已存在 → 替换（modify）
 *   - 不存在 → push（create）
 *   - 404 → 移除（文件在 fetch 前已被删）
 */
async function applyFileChanges() {
  if (pendingFileChanges.size === 0) return
  // 取出并清空（避免 apply 期间新事件丢失——新事件会重新进 map 并启新 timer）
  const changes = new Map(pendingFileChanges)
  pendingFileChanges.clear()

  // 1) 先处理 delete（纯客户端，无 API）
  for (const [path, action] of changes) {
    if (action === 'delete') {
      const idx = files.value.findIndex((f) => f.path === path)
      if (idx >= 0) {
        files.value.splice(idx, 1)
        console.info('[Files] incremental delete:', path)
      }
    }
  }

  // 2) 处理 create/modify：只 fetch 父目录 === currentPath 的文件
  const visiblePaths: string[] = []
  for (const [path, action] of changes) {
    if (action === 'delete') continue
    // 判断父目录是否 === currentPath（文件在当前视图才需要更新）
    const parent = path.substring(0, path.lastIndexOf('/')) || '/d'
    if (parent !== currentPath.value) continue
    visiblePaths.push(path)
  }
  if (visiblePaths.length === 0) return

  // 并发 fetch 所有可见文件的 metadata
  const results = await Promise.allSettled(visiblePaths.map((p) => getFileInfo(p)))
  for (let i = 0; i < results.length; i++) {
    const r = results[i]
    const path = visiblePaths[i]
    if (r.status === 'fulfilled') {
      const fileItem = r.value
      const idx = files.value.findIndex((f) => f.path === path)
      if (idx >= 0) {
        // modify：替换原条目（保留位置，更新 metadata）
        files.value[idx] = fileItem
        console.info('[Files] incremental modify:', path)
      } else {
        // create：追加
        files.value.push(fileItem)
        console.info('[Files] incremental create:', path)
      }
    } else {
      // fetch 失败（404 等）：从列表移除（文件已不存在）
      const idx = files.value.findIndex((f) => f.path === path)
      if (idx >= 0) {
        files.value.splice(idx, 1)
        console.info('[Files] incremental remove (fetch failed):', path)
      }
    }
  }
}

async function loadPlugins() {
  try { plugins.value = await fetchPlugins() } catch {}
}
async function loadTags() {
  try { tags.value = await fetchTags() } catch {}
}

function openPluginView(plugin: PluginMeta) {
  files.value = []
  loading.value = true
  pluginLoaded.value = false
  selectedPlugin.value = plugin
  menuController.close()
}

async function exitPluginMode() {
  selectedPlugin.value = null
  await menuController.close()
  files.value = []
  loading.value = true
  await loadFiles()
}

async function openSideDrawer() {
  await menuController.open('plugin-menu')
}

function getPluginIcon(plugin: PluginMeta): any {
  const featureIcon = getFeatureIcon(plugin.name)
  if (featureIcon) return featureIcon
  const icons: Record<string, string> = { video: filmOutline, audio: musicalNotesOutline, image: imageOutline, pdf: documentTextOutline, text: documentOutline, wps: documentOutline }
  return icons[plugin.name] || lockClosed
}

// 🆕 2026-06-15 multi-mount 适配：mount 伪 item 字段访问器
//   强类型访问 (file as any) 在 template 里太丑，集中到 helper 里
function mountDriverOf(file: FileItem): string | null {
  const f = file as FileItem & { mount_driver?: string }
  return f.mount_driver ?? null
}
function mountPathOf(file: FileItem): string {
  const f = file as FileItem & { mount_path?: string }
  return f.mount_path ?? ''
}
function mountRootOf(file: FileItem): string {
  const f = file as FileItem & { mount_root?: string }
  return f.mount_root ?? ''
}

async function searchPluginFiles(
  plugin: PluginMeta,
  onItem?: (file: FileItem) => void
): Promise<FileItem[]> {
  if (!plugin.supportedExtensions || plugin.supportedExtensions.length === 0) return []
  const result = await listPluginFilesStream(
    currentPath.value,
    plugin.supportedExtensions,
    (file) => { onItem?.(file) }
  )
  return result.files
}

async function handleTagFilter(tagName: string) {
  menuController.close()
  files.value = []
  loading.value = true
  selectedPlugin.value = null
  try {
    files.value = await listFilesByTag(tagName, currentPath.value)
    loadFileTagsForCurrentDir()
  } catch (e) { showToast({ message: `筛选失败: ${e}` }) }
  finally { loading.value = false }
}

const pluginTab = ref<'source' | 'container'>('source')
const pluginFiles = ref<FileItem[]>([])
const pluginLoaded = ref(false)
let pluginLoadGeneration = 0

const sizeFilterMin = ref<number | null>(null)
const sizeFilterMax = ref<number | null>(null)
const timeFilterFrom = ref<string | null>(null)
const timeFilterTo = ref<string | null>(null)
const showPluginFilters = ref(false)

const pluginSortBy = ref<'name' | 'size' | 'time'>('name')
const pluginSortDesc = ref(false)

const pluginSortLabel = computed(() => {
  const map: Record<string, string> = { name: '名称', size: '大小', time: '时间' }
  return (map[pluginSortBy.value] || '名称') + (pluginSortDesc.value ? '↓' : '↑')
})

const SIZE_PRESETS = [
  { label: '< 1MB', max: 1024 * 1024 },
  { label: '1MB - 10MB', min: 1024 * 1024, max: 10 * 1024 * 1024 },
  { label: '10MB - 100MB', min: 10 * 1024 * 1024, max: 100 * 1024 * 1024 },
  { label: '> 100MB', min: 100 * 1024 * 1024 },
] as const
const TIME_PRESETS = [
  { label: '今天', days: 0 },
  { label: '近 3 天', days: 3 },
  { label: '近 7 天', days: 7 },
  { label: '近 30 天', days: 30 },
] as const

const activeFilterCount = computed(() => {
  let c = 0
  if (sizeFilterMin.value !== null) c++
  if (sizeFilterMax.value !== null) c++
  if (timeFilterFrom.value !== null) c++
  if (timeFilterTo.value !== null) c++
  return c
})

function applySizePreset(preset: typeof SIZE_PRESETS[number]) {
  sizeFilterMin.value = 'min' in preset ? (preset as { min?: number }).min ?? null : null
  sizeFilterMax.value = 'max' in preset ? (preset as { max?: number }).max ?? null : null
}
function applyTimePreset(preset: typeof TIME_PRESETS[number]) {
  const now = new Date()
  const from = new Date(now)
  from.setDate(from.getDate() - preset.days)
  from.setHours(0, 0, 0, 0)
  timeFilterFrom.value = formatDateInput(from)
  if (preset.days === 0) {
    timeFilterTo.value = formatDateInput(now)
  } else {
    timeFilterTo.value = null
  }
}

function formatDateInput(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}
function clearAllPluginFilters() {
  sizeFilterMin.value = null
  sizeFilterMax.value = null
  timeFilterFrom.value = null
  timeFilterTo.value = null
  pluginSortBy.value = 'name'
  pluginSortDesc.value = false
}
const filteredPluginFiles = computed(() => {
  if (!selectedPlugin.value) return []
  let list: FileItem[]
  if (pluginTab.value === 'container') {
    list = pluginFiles.value.filter(f => isAnyContainerFile(f))
  } else {
    list = pluginFiles.value.filter(f => !isAnyContainerFile(f))
  }
  const query = searchQuery.value.trim().toLowerCase()
  if (query) {
    list = list.filter(f => f.name.toLowerCase().includes(query))
  }
  if (sizeFilterMin.value !== null) {
    list = list.filter(f => (f.size || 0) >= sizeFilterMin.value!)
  }
  if (sizeFilterMax.value !== null) {
    list = list.filter(f => (f.size || 0) <= sizeFilterMax.value!)
  }
  if (timeFilterFrom.value !== null) {
    const from = new Date(timeFilterFrom.value).getTime()
    list = list.filter(f => (f.modified ? new Date(f.modified).getTime() : 0) >= from)
  }
  if (timeFilterTo.value !== null) {
    const to = new Date(timeFilterTo.value).getTime()
    list = list.filter(f => (f.modified ? new Date(f.modified).getTime() : 0) <= to)
  }
  list.sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1
    if (!a.isDirectory && b.isDirectory) return 1
    let cmp = 0
    switch (pluginSortBy.value) {
      case 'name': cmp = a.name.localeCompare(b.name); break
      case 'size': cmp = (a.size || 0) - (b.size || 0); break
      case 'time': cmp = (Number(a.modified) || 0) - (Number(b.modified) || 0); break
    }
    return pluginSortDesc.value ? -cmp : cmp
  })
  const tagMap = fileTagMap.value
  return list.map(f => ({ ...f, _tags: tagMap[f.path] || [] }))
})

watch(selectedPlugin, async (plugin) => {
  if (plugin) {
    const gen = ++pluginLoadGeneration
    pluginTab.value = 'source'
    pluginLoaded.value = false
    pluginFiles.value = []
    console.info('[Files] Loading plugin files (stream):', plugin.name)
    try {
      const results = await searchPluginFiles(plugin, (file) => {
        if (gen !== pluginLoadGeneration) return
        pluginFiles.value.push(file)
        if (pluginFiles.value.length === 1 && !pluginLoaded.value) {
          console.info('[Files] First plugin item arrived, UI unlocked')
        }
      })
      if (gen !== pluginLoadGeneration) return
      pluginFiles.value = results
    } catch (e) {
      console.debug('[Files] Plugin stream load failed:', e)
    }
    if (gen === pluginLoadGeneration) {
      pluginLoaded.value = true
      setupLazyThumbnails()
    }
  }
})

function onBackendReady(data: { port?: number; running?: boolean }) {
  if (data.running || data.port) {
    loadFiles()
  }
}

onMounted(() => {
  loadFiles()
  loadPlugins()
  loadTags()
  eventBus.on('file:change', onFileChange)
  window.addEventListener('encv:backend-ready', onBackendReadyWindow as EventListener)
  // 🆕 v4 M1：注册 file:change tab active gate（默认 active=true，Files 切到非可见时设 false）
  useRealtimeTransport().setFileChangeGate(true, () => {
    // 切回 Files tab 时兜底刷一次（防抖 timer 内可能还有 pending 事件 → 一起 flush）
    if (fileChangeDebounceTimer !== null) {
      clearTimeout(fileChangeDebounceTimer)
      fileChangeDebounceTimer = null
    }
    loadFiles()
  })
  if (import.meta.env.DEV) {
    import('@/composables/useTestBackdoor').then(({ useTestBackdoor }) => {
      import('@/composables/useNewTaskModal').then(({ useNewTaskModal: createNewTaskModal }) => {
        const { openNewTask } = createNewTaskModal()
        useTestBackdoor(files, {
          onLongPress: handleLongPress,
          onClick: handleFileClick,
          navigateTo: navigateTo,
          openNewTask: (sourcePath?: string, taskType?: 'encrypt' | 'decrypt') => {
            return openNewTask(sourcePath, taskType)
          },
        })
      })
    })
  }
})

onIonViewWillEnter(() => {
  // 🆕 v3 2026-06-18 Task 8：读取 route.query.path / route.query.highlight
  //   - TaskDetailModal.locateOutput 跳转 /tabs/files?path=<dir>&highlight=<name>
  //   - 若 path 与 currentPath 不同 → 导航到目标目录，loadFiles 完成后高亮
  //   - 若 path 与 currentPath 相同 → 直接高亮（不重新 loadFiles）
  //   - 若无 query → 旧行为（仅在 files 为空时 reload）
  const qPath = typeof route.query.path === 'string' ? route.query.path : ''
  const qHighlight = typeof route.query.highlight === 'string' ? route.query.highlight : ''

  if (qPath && qHighlight) {
    if (qPath !== currentPath.value) {
      // 切换目录 + 延迟高亮（loadFiles 完成后触发）
      pendingHighlight.value = qHighlight
      currentPath.value = qPath
      searchQuery.value = ''
      searchResults.value = null
      loadFiles()
    } else {
      // 同目录直接高亮
      highlightFile(qHighlight)
    }
    // 消费完 query 后清掉，避免下次进入 tab 又触发
    router.replace({ path: route.path, query: {} })
    return
  }

  if (files.value.length === 0 && !loading.value && !connecting.value) {
    loadFiles()
  }
})

onUnmounted(() => {
  eventBus.off('file:change', onFileChange)
  window.removeEventListener('encv:backend-ready', onBackendReadyWindow as EventListener)
  if (searchTimer) clearTimeout(searchTimer)
  // 🆕 v3 2026-06-18 Task 8：清理高亮定时器
  if (highlightTimer) {
    clearTimeout(highlightTimer)
    highlightTimer = null
  }
  // 🆕 v4 M1：清 file:change 防抖 timer
  if (fileChangeDebounceTimer !== null) {
    clearTimeout(fileChangeDebounceTimer)
    fileChangeDebounceTimer = null
  }
})

function onBackendReadyWindow(event: Event) {
  const detail = (event as CustomEvent).detail || {}
  onBackendReady(detail)
}

// 🆕 v4 M1：根据 route path 变化同步 file:change gate
//   - 当前 path 是 /tabs/files/* → gate=true（Files tab 可见，接收 file:change）
//   - 其他 → gate=false（Tasks/Settings tab 可见时丢 file:change，避免后台狂刷）
watch(
  () => route.path,
  (newPath) => {
    const isFilesActive = newPath.startsWith('/tabs/files')
    useRealtimeTransport().setFileChangeGate(isFilesActive, () => {
      // 切回时兜底（onMounted 注册的 onActive 也会触发；这里再覆盖一次保险）
      if (fileChangeDebounceTimer !== null) {
        clearTimeout(fileChangeDebounceTimer)
        fileChangeDebounceTimer = null
      }
      loadFiles()
    })
  },
  { immediate: true },
)
</script>

<style scoped>
/* 🆕 v3 2026-06-18 Task 8：route.query.highlight 驱动的文件高亮 */
/*   - 2 秒临时高亮（highlightFile 函数控制 highlightedPath）
/*   - 用 ion-color-primary 浅底 + 左侧 3px 边条，视觉锚点清晰 */
.file-highlight {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  box-shadow: inset 3px 0 0 var(--ion-color-primary);
  transition: background 0.2s ease, box-shadow 0.2s ease;
}

/* 播放错误展示区域 */
.play-error-banner {
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  border-left: 3px solid var(--ion-color-danger);
  border-radius: 6px;
  margin: 8px 12px;
  padding: 10px 12px;
}

/* 🆕 v4 Bug1 修复：自动更新顶栏细 indicator（不阻塞老数据渲染） */
.files-refresh-bar {
  position: sticky;
  top: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  background: var(--ion-color-primary-tint);
  color: var(--ion-color-primary-contrast);
  font-size: 11px;
  font-weight: 500;
  border-bottom: 1px solid var(--ion-color-primary-shade);
  animation: files-refresh-bar-enter 0.15s ease-out;
}
.files-refresh-bar__spinner {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
}
.files-refresh-bar__label {
  white-space: nowrap;
}
@keyframes files-refresh-bar-enter {
  from { transform: translateY(-100%); opacity: 0; }
  to { transform: translateY(0); opacity: 1; }
}

.play-error-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.play-error-file {
  font-weight: 500;
  color: var(--ion-color-danger);
  font-size: 14px;
}

.play-error-message {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin-top: 4px;
  margin-bottom: 0;
}

.play-error-detail-row {
  margin-top: 6px;
}

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

.breadcrumb-scroll {
  --background: transparent;
}

.breadcrumb {
  display: flex;
  align-items: center;
  padding: 0 16px;
  white-space: nowrap;
}

.breadcrumb-item {
  cursor: pointer;
  color: var(--ion-color-primary);
  font-size: 14px;
}

.breadcrumb-item:hover {
  text-decoration: underline;
}

.breadcrumb-sep {
  font-size: 14px;
  margin: 0 4px;
  color: var(--encv-text-secondary);
}

.breadcrumb-segment {
  display: flex;
  align-items: center;
}

.recursive-toggle {
  margin-right: 8px;
  font-size: 12px;
}

.search-path {
  font-size: 11px;
  color: var(--encv-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.open-folder-btn {
  --padding-start: 8px;
  --padding-end: 8px;
  min-width: 44px;
  min-height: 44px;
  margin: 0;
}

.open-folder-icon {
  font-size: 20px;
  color: var(--ion-color-primary);
}

.tag-editor-content {
  padding: 16px;
}
.existing-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.no-tags-hint {
  color: var(--ion-text-secondary);
  font-size: 14px;
  margin-bottom: 16px;
}
.tag-input-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.tag-input-row .input-with-history {
  flex: 1;
}
.tag-input-row .input-with-history ion-input {
  --padding-start: 12px;
}
.file-tag-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}
.file-thumbnail-slot {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.file-thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 8px;
}
.thumb-fallback {
  opacity: 0.4;
}
ion-item {
  contain: layout style;
}
.file-tag-chips {
  contain: content;
}
.filter-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.filter-chips ion-chip {
  font-size: 12px;
  --padding-start: 8px;
  --padding-end: 8px;
  cursor: pointer;
}
.plugin-header {
  padding: 0 4px;
}
.plugin-header-top {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 4px 4px;
}
.plugin-back-btn {
  --padding-start: 4px;
  --padding-end: 4px;
  --padding-top: 4px;
  --padding-bottom: 4px;
  min-width: 32px;
  height: 32px;
  flex-shrink: 0;
}
.plugin-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--ion-color-dark);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 1;
  min-width: 0;
}
.plugin-segment {
  flex-shrink: 0;
  --segment-height: 28px;
  margin: 0;
}
.plugin-segment ion-segment-button {
  --padding-start: 10px;
  --padding-end: 10px;
  font-size: 12px;
  min-height: 28px;
  line-height: 1;
}
.filter-toggle-item {
  --padding-start: 12px;
  --padding-end: 12px;
  --min-height: 40px;
}
.main-sort-bar {
  padding: 0 4px;
}

.real-name {
  color: var(--ion-color-danger);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 🆕 2026-06-15 multi-mount 适配：mount 伪 item 样式（driver badge + mount_path + root_path） */
.mount-driver-badge {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 10px;
  text-transform: lowercase;
  margin-right: 6px;
  vertical-align: middle;
}
.mount-path-inline {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  padding: 1px 4px;
  border-radius: 3px;
}
.mount-root-inline {
  font-size: 10px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  margin-left: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

/* ===== 长按菜单分区样式 (action-sheet overlay，需 :global) ===== */
:global(.file-action-sheet .action-button) {
  padding: 12px 16px;
  font-size: 15px;
}

:global(.file-action-sheet .action-section-view) {
  --color: var(--ion-color-primary);
}
:global(.file-action-sheet .action-section-view .action-button-icon) {
  color: var(--ion-color-primary) !important;
}

:global(.file-action-sheet .action-section-crypto) {
  --color: var(--ion-color-warning);
}
:global(.file-action-sheet .action-section-crypto .action-button-icon) {
  color: #e6a000 !important;
}

:global(.file-action-sheet .action-section-manage) {
  --color: var(--ion-color-medium);
}
:global(.file-action-sheet .action-section-manage .action-button-icon) {
  color: var(--ion-color-medium-shade) !important;
}

:global(.file-action-sheet .action-section-danger) {
  --color: var(--ion-color-danger);
}
:global(.file-action-sheet .action-section-danger .action-button-icon) {
  color: var(--ion-color-danger) !important;
}</style>
