<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button @click="router.back()">
            <ion-icon :icon="arrowBack" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
        <ion-title>{{ t('files.info') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content class="file-info-content">
      <div v-if="loading" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('filePreview.loading') }}</p>
      </div>

      <div v-else-if="error" class="error-state">
        <ion-icon :icon="alertCircle" class="error-icon"></ion-icon>
        <h3>{{ t('filePreview.loadError') }}</h3>
        <p>{{ error }}</p>
        <ion-button @click="loadInfo">{{ t('filePreview.retry') }}</ion-button>
      </div>

      <div v-else-if="info" class="info-scroll">
        <!-- 缩略图预览 -->
        <div v-if="isImageOrVideo" class="section-card thumbnail-section">
          <div class="thumbnail-wrapper" @click="handlePreviewClick">
            <img
              v-if="thumbnailUrl"
              :src="thumbnailUrl"
              class="thumbnail-image"
              loading="lazy"
              @error="thumbnailError = true"
            />
            <div v-else class="thumbnail-placeholder">
              <ion-icon :icon="isImageFile ? imageOutline : filmOutline" class="placeholder-icon"></ion-icon>
              <span class="placeholder-text">{{ isImageFile ? t('fileInfo.imagePreview') : t('fileInfo.videoPreview') }}</span>
            </div>
          </div>
          <div v-if="info.is_encv_container && containerData?.original_duration" class="duration-badge">
            {{ formatDuration(containerData.original_duration) }}
          </div>
        </div>

        <!-- 文件基本信息 -->
        <div class="section-card">
          <h4 class="card-title">
            <ion-icon :icon="documentTextOutline" class="title-icon"></ion-icon>
            {{ t('files.info') }}
          </h4>
          <div class="info-grid">
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.name') || 'Name' }}</span>
              <span class="info-value">{{ info.name }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.path') || 'Path' }}</span>
              <span class="info-value code-text">{{ info.path }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.size') || 'Size' }}</span>
              <span class="info-value">{{ formatFileSize(info.size) }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.modified') || 'Modified' }}</span>
              <span class="info-value">{{ formatTime(info.modified) }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">MIME</span>
              <span class="info-value code-text">{{ info.mime_type }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.category') || 'Category' }}</span>
              <ion-badge color="medium">{{ info.category }}</ion-badge>
            </div>
            <div class="info-row" v-if="info.is_encrypted">
              <span class="info-label">{{ t('files.encrypted') }}</span>
              <ion-badge color="warning">Yes</ion-badge>
            </div>
          </div>
        </div>

        <!-- 插件聚合索引 -->
        <div v-if="pluginPrediction" class="section-card plugin-index-card">
          <h4 class="card-title">
            <ion-icon :icon="settingsOutline" class="title-icon primary"></ion-icon>
            {{ t('fileInfo.pluginIndex') }}
          </h4>
          <div v-if="pluginPrediction.pluginName" class="plugin-matched">
            <div class="matched-plugin-header">
              <ion-badge color="primary">{{ pluginPrediction.pluginName }}</ion-badge>
              <span class="match-type-badge">{{ matchTypeLabel(pluginPrediction.candidates?.[0]?.matchType) }}</span>
            </div>
            <p class="plugin-desc">{{ pluginMatchDesc }}</p>
          </div>
          <div v-else class="no-plugin-match">
            <ion-icon :icon="helpCircleOutline" class="no-match-icon"></ion-icon>
            <span>{{ t('fileInfo.noPluginMatch') }}</span>
          </div>
          <div v-if="pluginPrediction.candidates?.length > 1" class="candidate-list">
            <div class="candidate-item" v-for="c in pluginPrediction.candidates" :key="c.name">
              <span class="candidate-name">{{ c.name }}</span>
              <ion-badge :color="c.name === pluginPrediction.pluginName ? 'primary' : 'medium'" size="small">
                {{ c.priority }}
              </ion-badge>
            </div>
          </div>
        </div>

        <!-- Alist-Encrypt -->
        <div v-if="isAlistEnc" class="section-card alist-enc-card">
          <h4 class="card-title">
            <ion-icon :icon="lockClosed" class="title-icon danger"></ion-icon>
            Alist-Encrypt
          </h4>
          <div class="info-grid">
            <div class="info-row">
              <span class="info-label">{{ t('files.encrypted') || '加密状态' }}</span>
              <ion-badge color="danger">{{ t('files.yes') || '是' }}</ion-badge>
            </div>
            <div v-if="decodedName" class="info-row alist-decoded-row">
              <span class="info-label">{{ t('fileInfo.originalName') || '原始文件名' }}</span>
              <span class="info-value alist-decoded-value">{{ decodedName }}</span>
            </div>
          </div>
        </div>

        <!-- ENCV Container -->
        <div v-if="info.is_encv_container && containerData" class="section-card container-card">
          <h4 class="card-title">
            <ion-icon :icon="lockClosed" class="title-icon primary"></ion-icon>
            ENCV Container
          </h4>
          <div class="info-grid">
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.version') }}</span>
              <span class="info-value">{{ formatContainerVersion(containerData.version) || '?' }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.containerId') }}</span>
              <span class="info-value code-text">{{ containerData.container_id ? containerData.container_id : '(auto)' }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.containerType') }}</span>
              <ion-badge color="primary">{{ containerData.container_type ?? '-' }}</ion-badge>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.seekable') }}</span>
              <ion-badge :color="containerData.is_seekable ? 'success' : 'medium'">
                {{ containerData.is_seekable ? 'Yes' : 'No' }}
              </ion-badge>
            </div>
            <div class="info-row" v-if="containerData.original_duration != null">
              <span class="info-label">{{ t('fileInfo.duration') }}</span>
              <span class="info-value">{{ formatDuration(containerData.original_duration) }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.segmentCount') }}</span>
              <span class="info-value">{{ containerData.segment_count ?? 0 }}</span>
            </div>
          </div>
        </div>

        <!-- Manifest -->
        <div v-if="info.is_encv_container && containerData" class="section-card manifest-card">
          <div class="manifest-header" @click="showManifest = !showManifest">
            <h4 class="card-title inline-title">
              <ion-icon :icon="listOutline" class="title-icon"></ion-icon>
              {{ t('fileInfo.manifest') }}
            </h4>
            <ion-icon :icon="showManifest ? chevronDown : chevronForward"></ion-icon>
          </div>
          <pre v-if="showManifest" class="manifest-json"><code>{{ manifestJson }}</code></pre>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { formatContainerVersion } from "@/constants/containerVersion";
import { formatFileSize } from "@/api/encv_files";
import {
  alertCircle,
  arrowBack,
  chevronDown,
  chevronForward,
  documentTextOutline,
  filmOutline,
  helpCircleOutline,
  imageOutline,
  listOutline,
  lockClosed,
  settingsOutline,
} from "ionicons/icons";

import type { FileItem, PredictPluginResponse } from "@/api/encv";
import { getApiBaseUrl, getExternalStreamUrl, predictPlugin, proxySafeEncode } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";
import { getDecodedName, isAlistEncrypted, loadDecodedName } from "@/features/alist-encrypt/useAlistEncrypt";
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

const { t } = useI18n();
const router = useRouter();
const route = useRoute();

interface ContainerData {
  version: number;
  container_id: string;
  container_type: string;
  is_seekable: boolean;
  original_duration?: number | null;
  segment_count?: number | null;
  segments?: unknown[];
  manifest_size?: number;
  header?: Record<string, unknown>;
  manifest?: unknown;
}

interface FileInfo {
  name: string;
  path: string;
  size: number;
  modified: string;
  mime_type: string;
  category: string;
  is_directory: boolean;
  is_encrypted: boolean;
  is_encv_container: boolean;
  container?: ContainerData;
}

const loading = ref(true);
const error = ref("");
const info = ref<FileInfo | null>(null);
const containerData = ref<ContainerData | null>(null);
const showManifest = ref(false);
const manifestJson = ref("");
const decodedName = ref<string | null>(null);
const isAlistEnc = ref(false);
const thumbnailError = ref(false);
const pluginPrediction = ref<PredictPluginResponse | null>(null);

const thumbnailUrl = computed(() => {
  if (!info.value || thumbnailError.value) return "";
  const path = info.value.path;
  if (!path) return "";
  return getExternalStreamUrl(path);
});

const isImageFile = computed(() => {
  const mime = info.value?.mime_type || "";
  return mime.startsWith("image/");
});

const isVideoFile = computed(() => {
  const mime = info.value?.mime_type || "";
  return mime.startsWith("video/");
});

const isImageOrVideo = computed(() => isImageFile.value || isVideoFile.value);

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

function formatTime(isoStr: string): string {
  try {
    return new Date(isoStr).toLocaleString();
  } catch {
    return isoStr;
  }
}

function matchTypeLabel(type?: string): string {
  const map: Record<string, string> = {
    mime: "MIME",
    extension: "扩展名",
    general: "通用",
    container: "容器",
  };
  return type ? map[type] || type : "-";
}

const pluginMatchDesc = computed(() => {
  const p = pluginPrediction.value;
  if (!p?.pluginName) return "";
  const candidates = p.candidates || [];
  if (candidates.length <= 1) return t("fileInfo.pluginExactMatch") || "精确匹配";
  return `${t("fileInfo.pluginCandidateCount") || "候选"} ${candidates.length}`;
});

function handlePreviewClick() {
  router.push({ path: "/tabs/files", query: { action: "preview", path: info.value?.path } });
}

async function loadInfo() {
  const path = (route.query.path as string) || "";
  if (!path) {
    error.value = t("filePreview.noPath");
    loading.value = false;
    return;
  }

  loading.value = true;
  error.value = "";
  showManifest.value = false;
  thumbnailError.value = false;

  try {
    const baseUrl = getApiBaseUrl();
    const resp = await fetch(`${baseUrl}/api/file/info?path=${proxySafeEncode(path)}`);
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const data = (await resp.json()) as FileInfo;
    info.value = data;
    containerData.value = data.container || null;

    if (data.container?.manifest) {
      try {
        const str = JSON.stringify(data.container.manifest, null, 2);
        manifestJson.value = /^[\x20-\x7E\t\n\r]*$/.test(str) ? str : "(contains non-printable characters)";
      } catch {
        manifestJson.value = "(invalid)";
      }
    } else {
      manifestJson.value = "(none)";
    }

    const fileItem: FileItem = {
      name: data.name,
      path: data.path,
      isDirectory: data.is_directory,
      isEncrypted: data.is_encrypted,
      size: data.size,
    };
    isAlistEnc.value = isAlistEncrypted(fileItem);
    if (isAlistEnc.value) {
      loadDecodedName(fileItem)
        .then(name => {
          decodedName.value = name || getDecodedName(data.path) || null;
        })
        .catch(() => {});
    } else {
      decodedName.value = null;
    }

    try {
      const taskType = data.is_encv_container ? ("decrypt" as const) : ("encrypt" as const);
      pluginPrediction.value = await predictPlugin(path, taskType);
    } catch {
      pluginPrediction.value = null;
    }
  } catch (e: any) {
    console.error("[FileInfo] failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
    error.value = e?.message || String(e);
  } finally {
    loading.value = false;
  }
}

onMounted(() => loadInfo());
</script>

<style scoped>
.file-info-content {
  --background: var(--ion-background-color);
}

.info-scroll {
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-card {
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.6);
  border-radius: 10px;
  padding: 16px;
  border-left: 3px solid var(--ion-color-medium);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}
.section-card.container-card { border-left-color: var(--ion-color-primary); }
.section-card.alist-enc-card { border-left-color: var(--ion-color-danger); }
.section-card.manifest-card { border-left-color: var(--ion-color-tertiary); }
.section-card.plugin-index-card { border-left-color: var(--ion-color-warning); }

.thumbnail-section {
  padding: 0;
  overflow: hidden;
  position: relative;
}

.thumbnail-wrapper {
  width: 100%;
  aspect-ratio: 16 / 9;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  overflow: hidden;
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.15);
}

.thumbnail-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.thumbnail-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--ion-color-medium);
}
.placeholder-icon { font-size: 40px; opacity: 0.5; }
.placeholder-text { font-size: 13px; }

.duration-badge {
  position: absolute;
  bottom: 8px;
  right: 8px;
  background: rgba(0, 0, 0, 0.7);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
  backdrop-filter: blur(4px);
}

.card-title {
  margin: 0 0 12px;
  font-size: 15px;
  font-weight: 600;
  color: var(--ion-text-color);
  display: flex;
  align-items: center;
  gap: 8px;
}
.inline-title { margin: 0; }
.title-icon { color: var(--ion-color-medium); }
.title-icon.primary { color: var(--ion-color-primary); }
.title-icon.danger { color: var(--ion-color-danger); }

.info-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.info-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
}

.info-label {
  color: var(--ion-text-secondary);
  font-weight: 500;
  min-width: 80px;
  flex-shrink: 0;
}

.info-value {
  color: var(--ion-text-color);
  font-weight: 400;
  text-align: right;
  word-break: break-all;
}

.code-text {
  font-family: monospace;
  font-size: 11px;
}

/* 插件聚合索引 */
.plugin-matched {
  margin-bottom: 10px;
}
.matched-plugin-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.match-type-badge {
  font-size: 10px;
  color: var(--ion-color-medium);
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  padding: 1px 6px;
  border-radius: 4px;
}
.plugin-desc {
  font-size: 12px;
  color: var(--ion-text-secondary);
  margin: 0;
  line-height: 1.4;
}
.no-plugin-match {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--ion-color-medium);
}
.no-match-icon { font-size: 18px; opacity: 0.5; }
.candidate-list {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.candidate-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
}
.candidate-name {
  color: var(--ion-text-secondary);
}

.manifest-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  cursor: pointer;
  user-select: none;
  -webkit-tap-highlight-color: transparent;
  margin-bottom: 0;
}
.manifest-header:hover .card-title { opacity: 0.8; }

.manifest-json {
  margin: 10px 0 0;
  padding: 10px 12px;
  max-height: 350px;
  overflow: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  line-height: 1.5;
  color: #888;
  white-space: pre-wrap;
  word-break: break-all;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 6px;
}

.loading-container, .error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 60%;
  color: #888;
  text-align: center;
  gap: 12px;
}
.error-icon { font-size: 48px; opacity: 0.5; color: #e74c3c; }

.alist-decoded-row {
  margin-top: 4px;
  padding-top: 8px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}
.alist-decoded-value {
  color: var(--ion-color-primary);
  font-weight: 600;
  font-size: 13px;
}
</style>
