<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.cacheAndIndex') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content class="cache-detail-content">
      <!-- 索引状态总览 -->
      <div class="section-card index-overview">
        <div class="overview-header">
          <h4 class="card-title">
            <ion-icon :icon="statsChartOutline" class="title-icon primary"></ion-icon>
            {{ t('settings.indexStatus') }}
          </h4>
          <div v-if="stats?.isIndexing" class="indexing-badge">
            <ion-spinner name="crescent" class="mini-spinner"></ion-spinner>
            <span>{{ t('settings.indexing') }}</span>
          </div>
          <div v-else-if="stats" class="idle-badge">{{ t('settings.idle') }}</div>
        </div>

        <div class="overview-grid">
          <div class="stat-item">
            <ion-icon :icon="documentTextOutline" class="stat-icon"></ion-icon>
            <div class="stat-value">{{ stats?.totalFiles ?? '-' }}</div>
            <div class="stat-label">{{ t('settings.totalFiles') }}</div>
          </div>
          <div class="stat-item">
            <ion-icon :icon="folderOpenOutline" class="stat-icon"></ion-icon>
            <div class="stat-value">{{ stats?.totalDirs ?? '-' }}</div>
            <div class="stat-label">{{ t('settings.totalDirs') }}</div>
          </div>
          <div class="stat-item">
            <ion-icon :icon="serverOutline" class="stat-icon"></ion-icon>
            <div class="stat-value">{{ stats ? formatFileSize(stats.totalSize) : '-' }}</div>
            <div class="stat-label">{{ t('settings.totalSize') }}</div>
          </div>
          <div class="stat-item" v-if="stats?.containers">
            <ion-icon :icon="lockClosed" class="stat-icon warning"></ion-icon>
            <div class="stat-value warning-text">{{ stats.containers }}</div>
            <div class="stat-label">{{ t('settings.encryptedContainers') }}</div>
          </div>
        </div>

        <div v-if="stats" class="meta-row">
          <span class="meta-item">
            <ion-icon :icon="timeOutline" class="meta-icon"></ion-icon>
            {{ stats.indexedAt || t('settings.never') }}
          </span>
          <span class="meta-item" v-if="stats.lastBuildMs">
            <ion-icon :icon="timerOutline" class="meta-icon"></ion-icon>
            {{ stats.lastBuildMs }} ms
          </span>
          <span class="meta-item" v-if="stats.source">
            <ion-icon :icon="cloudOutline" class="meta-icon"></ion-icon>
            {{ stats.source === 'webdav' ? 'WebDAV' : t('settings.mobileIndex') }}
          </span>
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="action-grid">
        <button
          class="action-btn action-btn--primary"
          :disabled="stats?.isIndexing"
          @click="handleRebuild"
        >
          <ion-icon :icon="refreshCircleOutline"></ion-icon>
          <span>{{ stats?.isIndexing ? t('settings.indexing') : t('settings.rebuildIndex') }}</span>
        </button>
        <button
          class="action-btn action-btn--danger"
          :disabled="stats?.isIndexing"
          @click="handleClearIndex"
        >
          <ion-icon :icon="trashOutline"></ion-icon>
          <span>{{ t('settings.clearIndex') }}</span>
        </button>
      </div>

      <!-- 🆕 2026-07-02 全文索引（FTS5）入口 -->
      <ion-item button @click="goFullTextIndex" detail lines="full" class="fulltext-entry">
        <ion-icon :icon="searchOutline" slot="start" color="tertiary"></ion-icon>
        <ion-label>
          <h3>{{ t('settings.fullTextIndex') || '全文索引' }}</h3>
          <p>{{ t('settings.fullTextIndexDesc') || 'FTS5 + bm25 + CJK bigram 全文搜索引擎' }}</p>
        </ion-label>
      </ion-item>

      <!-- 缓存详情 -->
      <ion-list lines="full">
        <ion-list-header>
          <ion-label>{{ t('settings.searchCache') }}</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-icon :icon="searchOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.cacheEntries') }}</h3>
            <p>{{ searchCacheSize }}</p>
          </ion-label>
          <ion-button fill="clear" size="small" color="danger" @click="handleClearSearchCache">
            {{ t('files.clear') }}
          </ion-button>
        </ion-item>

        <ion-list-header>
          <ion-label>{{ t('settings.thumbCache') }}</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-icon :icon="imageOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.thumbCacheEntries') }}</h3>
            <p>{{ thumbCacheSize }} / {{ THUMB_CACHE_MAX }}</p>
          </ion-label>
          <ion-button fill="clear" size="small" color="danger" @click="handleClearThumbCache">
            {{ t('files.clear') }}
          </ion-button>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { formatFileSize } from "@/api/encv_files";
import {
  cloudOutline,
  documentTextOutline,
  folderOpenOutline,
  imageOutline,
  lockClosed,
  refreshCircleOutline,
  searchOutline,
  serverOutline,
  statsChartOutline,
  timeOutline,
  timerOutline,
  trashOutline,
} from "ionicons/icons";

import type { IndexStats } from "@/api/encv";
import { clearIndex, getIndexStats, rebuildIndex } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";
import { clearThumbCache, getThumbCacheSize, THUMB_CACHE_MAX } from "@/composables/useThumbnailCache";
import { showToast } from "@/composables/useToast";
import { alertController } from "@ionic/vue";
import { onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();
const { t } = useI18n();
const stats = ref<IndexStats | null>(null);
const searchCacheSize = ref(0);
const thumbCacheSize = ref(0);
let pollTimer: ReturnType<typeof setInterval> | null = null;

async function loadStats() {
  try {
    stats.value = await getIndexStats();
  } catch {
    stats.value = null;
  }
}

// 🆕 2026-07-02 跳转全文索引二级页（FTS5 详情）
function goFullTextIndex() {
  router.push("/tabs/settings/fulltext-index");
}

function updateSearchCacheSize() {
  try {
    let count = 0;
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key?.startsWith("search-cache:")) count++;
    }
    searchCacheSize.value = count;
  } catch {
    searchCacheSize.value = 0;
  }
}

async function handleRebuild() {
  try {
    await rebuildIndex();
    await loadStats();
    showToast({ message: t("settings.rebuildStarted"), duration: 1500, color: "success" });
  } catch {
    showToast({ message: t("settings.rebuildFailed"), duration: 2000, color: "danger" });
  }
}

async function handleClearIndex() {
  const alert = await alertController.create({
    header: t("settings.clearIndex"),
    message: t("settings.clearIndexConfirm"),
    buttons: [
      { text: t("files.cancelSelect"), role: "cancel" },
      {
        text: t("settings.clearIndex"),
        role: "destructive",
        handler: async () => {
          try {
            await clearIndex();
            await loadStats();
          } catch {
            showToast({ message: t("settings.clearFailed"), duration: 2000, color: "danger" });
          }
        },
      },
    ],
  });
  await alert.present();
}

function handleClearSearchCache() {
  const keysToRemove: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (key?.startsWith("search-cache:")) keysToRemove.push(key);
  }
  keysToRemove.forEach(key => localStorage.removeItem(key));
  searchCacheSize.value = 0;
  showToast({ message: t("settings.cleared"), duration: 1200, color: "medium" });
}

function updateThumbCacheSize() {
  thumbCacheSize.value = getThumbCacheSize();
}

async function handleClearThumbCache() {
  const alert = await alertController.create({
    header: t("settings.clearThumbCache"),
    message: t("settings.clearIndexConfirm"),
    buttons: [
      { text: t("files.cancelSelect"), role: "cancel" },
      {
        text: t("settings.clearThumbCache"),
        role: "destructive",
        handler: () => {
          clearThumbCache();
          thumbCacheSize.value = 0;
          showToast({ message: t("settings.cleared"), duration: 1200, color: "medium" });
        },
      },
    ],
  });
  await alert.present();
}

onMounted(() => {
  loadStats().catch(() => {});
  updateSearchCacheSize();
  updateThumbCacheSize();
  pollTimer = setInterval(() => {
    if (stats.value?.isIndexing) {
      loadStats().catch(() => {});
    }
  }, 2000);
});

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
});
</script>

<style scoped>
.cache-detail-content {
  --background: var(--ion-background-color);
}

.section-card {
  margin: 12px 16px;
  padding: 16px;
  border-radius: 14px;
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.65);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border-left: 3px solid var(--ion-color-primary);
}
body.dark .section-card {
  background: rgba(var(--ion-background-color-rgb, 30, 30, 30), 0.7);
}

.overview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}

.card-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 8px;
}
.title-icon { font-size: 17px; color: var(--ion-color-medium); }
.title-icon.primary { color: var(--ion-color-primary); }

.indexing-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--ion-color-warning);
  padding: 3px 10px;
  border-radius: 10px;
  background: rgba(var(--ion-color-warning-rgb), 0.1);
}
.mini-spinner {
  width: 14px;
  height: 14px;
}
.idle-badge {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--ion-color-success);
  padding: 3px 10px;
  border-radius: 10px;
  background: rgba(var(--ion-color-success-rgb), 0.08);
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  margin-bottom: 14px;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 10px 8px;
  border-radius: 10px;
  background: rgba(var(--ion-background-color-rgb), 0.5);
}
.stat-icon {
  font-size: 20px;
  color: var(--ion-color-primary);
  opacity: 0.7;
}
.stat-icon.warning { color: var(--ion-color-warning); opacity: 0.85; }
.stat-value {
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
  color: var(--ion-text-color);
}
.stat-value.warning-text { color: var(--ion-color-warning); }
.stat-label {
  font-size: 11px;
  color: var(--ion-color-medium);
  text-align: center;
}

.meta-row {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
  padding-top: 10px;
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.15);
}
.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11.5px;
  color: var(--ion-color-medium);
}
.meta-icon { font-size: 13px; }

.action-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin: 4px 16px 16px;
}

.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 12px 16px;
  border: none;
  border-radius: 12px;
  font-size: 13.5px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
  -webkit-tap-highlight-color: transparent;
  outline: none;
}
.action-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.action-btn ion-icon { font-size: 18px; }

.action-btn--primary {
  background: rgba(var(--ion-color-primary-rgb), 0.12);
  color: var(--ion-color-primary);
}
.action-btn--primary:not(:disabled):hover {
  background: rgba(var(--ion-color-primary-rgb), 0.2);
}
.action-btn--primary:not(:disabled):active {
  transform: scale(0.97);
}

.action-btn--danger {
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  color: var(--ion-color-danger);
}
.action-btn--danger:not(:disabled):active {
  transform: scale(0.97);
}

@media (max-width: 599px) {
  .overview-grid {
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
  }
  .stat-value { font-size: 17px; }
  .action-grid { gap: 8px; }
  .action-btn { padding: 10px 12px; font-size: 12.5px; }
}
</style>
