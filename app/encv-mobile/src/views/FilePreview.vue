<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button @click="router.back()">
            <ion-icon :icon="arrowBack" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
        <ion-title>{{ fileName }}</ion-title>
        <ion-buttons slot="end" v-if="previewType === 'text'">
          <ion-button @click="toggleWrap" fill="clear">
            <ion-icon :icon="textWrap ? returnDownBackOutline : returnDownForwardOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="preview-content">
      <div v-if="loading" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('filePreview.loading') }}</p>
      </div>

      <div v-else-if="error" class="error-state">
        <ion-icon :icon="alertCircle" class="error-icon"></ion-icon>
        <h3>{{ t('filePreview.loadError') }}</h3>
        <p>{{ error }}</p>
        <ion-button @click="loadFile">{{ t('filePreview.retry') }}</ion-button>
      </div>

      <template v-else>
        <div v-if="isEncryptedPreview" class="file-info-card">
          <div class="info-grid">
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.name') || 'Name' }}</span>
              <span class="info-value">{{ fileName }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.size') || 'Size' }}</span>
              <span class="info-value">{{ formatFileSize(fileSize) }}</span>
            </div>
            <div class="info-row" v-if="fileModified">
              <span class="info-label">{{ t('fileInfo.modified') || 'Modified' }}</span>
              <span class="info-value">{{ fileModified }}</span>
            </div>
            <div class="info-row" v-if="fileMimeType">
              <span class="info-label">MIME</span>
              <span class="info-value code-text">{{ fileMimeType }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('fileInfo.category') || 'Category' }}</span>
              <ion-badge color="medium">{{ fileCategory }}</ion-badge>
            </div>
            <div class="info-row">
              <span class="info-label">{{ t('files.encrypted') }}</span>
              <ion-badge color="warning">Yes</ion-badge>
            </div>
          </div>
        </div>

        <div v-if="previewType === 'image'" class="preview-wrapper image-preview">
          <img :src="streamUrl" class="preview-image" />
        </div>

        <div v-else-if="previewType === 'pdf'" class="preview-wrapper pdf-preview">
          <iframe :src="pdfPreviewUrl" class="preview-iframe"></iframe>
        </div>

        <div v-else-if="previewType === 'text'" class="text-content-wrapper">
          <div v-if="textLoading" class="text-loading">
            <ion-spinner name="crescent"></ion-spinner>
            <p>Loading text...</p>
          </div>
          <div v-else-if="textError" class="text-error">
            <p>{{ textError }}</p>
            <ion-button size="small" @click="loadTextContent">Retry</ion-button>
            <ion-button v-if="textErrorDetail" size="small" fill="outline" color="medium" @click="showTextErrorDetail = !showTextErrorDetail">
              {{ showTextErrorDetail ? 'Hide' : 'Show' }} Details
            </ion-button>
            <pre v-if="showTextErrorDetail && textErrorDetail" class="error-detail">{{ textErrorDetail }}</pre>
          </div>
          <pre v-else class="text-content" :class="{ 'no-wrap': !textWrap }">{{ textContent }}</pre>
        </div>

        <div v-else-if="previewType === 'container'" class="preview-wrapper container-info">
          <div class="container-card">
            <h4 class="card-title">
              <ion-icon :icon="lockClosed" class="title-icon"></ion-icon>
              ENCV Container
            </h4>
            <div v-if="containerInfo?.error" class="container-error">
              <ion-icon :icon="alertCircle" class="error-icon"></ion-icon>
              <p>{{ containerInfo.error }}</p>
            </div>
            <div class="info-grid" v-if="containerInfo && !containerInfo.error">
              <div class="info-row">
                <span class="info-label">{{ t('fileInfo.version') }}</span>
                <span class="info-value">{{ formatContainerVersion(containerInfo.version) || '?' }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">{{ t('fileInfo.containerId') }}</span>
                <span class="info-value code-text">{{ containerInfo.container_id ?? '-' }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">{{ t('fileInfo.containerType') }}</span>
                <span class="info-value">{{ containerInfo.container_type ?? '-' }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">{{ t('fileInfo.seekable') }}</span>
                <ion-badge :color="containerInfo.is_seekable ? 'success' : 'medium'">
                  {{ containerInfo.is_seekable ? 'Yes' : 'No' }}
                </ion-badge>
              </div>
              <div class="info-row" v-if="containerInfo.original_duration">
                <span class="info-label">{{ t('fileInfo.duration') }}</span>
                <span class="info-value">{{ formatDuration(containerInfo.original_duration) }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">{{ t('fileInfo.segmentCount') }}</span>
                <span class="info-value">{{ containerInfo.segment_count ?? 0 }}</span>
              </div>
            </div>
          </div>

          <div class="manifest-section">
            <div class="manifest-header" @click="showManifest = !showManifest">
              <span>{{ t('fileInfo.manifest') }}</span>
              <ion-icon :icon="showManifest ? chevronDown : chevronForward"></ion-icon>
            </div>
            <pre v-if="showManifest && containerInfo" class="manifest-json"><code>{{ manifestJson }}</code></pre>
          </div>
        </div>

        <div v-else class="preview-wrapper unsupported-preview">
          <ion-icon :icon="helpCircleOutline" class="unsupported-icon"></ion-icon>
          <h3>{{ fileName }}</h3>
          <div class="unsupported-meta">
            <span class="meta-item"><ion-icon :icon="documentTextOutline"></ion-icon> {{ filePath }}</span>
            <span class="meta-item"><ion-icon :icon="informationCircle"></ion-icon> {{ formatFileSize(fileSize) }}</span>
          </div>
          <p class="unsupported-msg">{{ t('filePreview.unsupported') }}</p>
        </div>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  alertCircle,
  arrowBack,
  chevronDown,
  chevronForward,
  documentTextOutline,
  helpCircleOutline,
  informationCircle,
  lockClosed,
  returnDownBackOutline,
  returnDownForwardOutline,
} from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

import {
  fetchTextPreviewExts,
  getApiBaseUrl,
  getFileCategory,
  getFileExtension,
  getFilePreviewUrl,
  getFileStreamUrl,
  proxySafeEncode,
} from "@encv/shared-components/api/encv";
import { formatFileSize } from "@encv/shared-components/api/encv_files";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { formatContainerVersion } from "@encv/shared-components/constants/containerVersion";
import { isNative, openPlayer } from "@/plugins/GoProcess";

type PreviewType = "image" | "pdf" | "text" | "container" | "unsupported";

interface ContainerInfo {
  version?: number;
  container_id?: string;
  container_type?: string;
  is_seekable?: boolean;
  original_duration?: number;
  segment_count?: number;
  segments?: unknown[];
  error?: string;
}

const { t } = useI18n();
const router = useRouter();
const route = useRoute();

const filePath = ref("");
const fileName = ref("");
const fileSize = ref(0);
const loading = ref(true);
const error = ref("");
const previewType = ref<PreviewType>("unsupported");
const streamUrl = ref("");
const pdfPreviewUrl = ref("");
const showManifest = ref(false);
const containerInfo = ref<ContainerInfo | null>(null);
const manifestJson = ref("");
const fileModified = ref("");
const fileMimeType = ref("");
const fileCategory = ref("");

const textContent = ref("");
const textLoading = ref(false);
const textError = ref("");
const textErrorDetail = ref<string>("");
const showTextErrorDetail = ref(false);
const textWrap = ref(true);

const isEncryptedPreview = computed(() => route.query.isEncrypted === "true");

function toggleWrap() {
  textWrap.value = !textWrap.value;
}

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

async function determinePreviewType(name: string, isEncrypted?: boolean): Promise<PreviewType> {
  if (isEncrypted) return "container";

  const category = getFileCategory(name);
  const ext = getFileExtension(name);

  if (category === "image") return "image";
  if (ext === "pdf") return "pdf";

  const textExts = await fetchTextPreviewExts();
  if (textExts.has(ext)) return "text";
  if (category === "document" || category === "other") return "text";
  return "unsupported";
}

async function loadTextContent() {
  const path = filePath.value;
  if (!path) return;

  textLoading.value = true;
  textError.value = "";
  textContent.value = "";

  try {
    const baseUrl = getApiBaseUrl();
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 30000);

    const resp = await fetch(`${baseUrl}/decrypt?file=${proxySafeEncode(path)}`, {
      signal: controller.signal,
    });
    clearTimeout(timeoutId);

    if (!resp.ok) {
      const body = await resp.text().catch(() => "");
      let detail = "";
      try {
        const j = JSON.parse(body);
        if (j.debug) detail = JSON.stringify(j.debug, null, 2);
      } catch {
        /* not json */
      }
      textErrorDetail.value = detail;
      throw new Error(`HTTP ${resp.status}: ${resp.statusText}${body ? " | " + body : ""}`);
    }

    textContent.value = await resp.text();
  } catch (e: any) {
    if (e.name === "AbortError") {
      textError.value = "Request timed out after 30 seconds";
    } else {
      textError.value = e?.message || String(e);
    }
  } finally {
    textLoading.value = false;
  }
}

async function loadFile() {
  const path = (route.query.path as string) || "";
  const name = (route.query.name as string) || "";
  if (!path) {
    error.value = t("filePreview.noPath");
    loading.value = false;
    return;
  }
  filePath.value = path;
  fileName.value = name || path.split("/").pop() || path;
  loading.value = true;
  error.value = "";
  showManifest.value = false;
  containerInfo.value = null;

  const isEncrypted = route.query.isEncrypted === "true";

  if (isEncrypted) {
    try {
      const baseUrl = getApiBaseUrl();
      const resp = await fetch(`${baseUrl}/api/file/info?path=${proxySafeEncode(path)}`);
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const info = await resp.json();

      fileSize.value = info.size || 0;
      fileModified.value = info.modified || "";
      fileMimeType.value = info.mime_type || "";
      fileCategory.value = info.category || "";

      if (info.is_encv_container && info.container) {
        const containerType = info.container.container_type;
        containerInfo.value = info.container;
        try {
          const str = JSON.stringify(info.container.manifest || info.container, null, 2);
          manifestJson.value = /^[\x20-\x7E\t\n\r]*$/.test(str) ? str : "(contains non-printable characters)";
        } catch {
          manifestJson.value = "(invalid)";
        }

        switch (containerType) {
          case "image":
            previewType.value = "image";
            streamUrl.value = getFileStreamUrl(path);
            break;
          case "video":
          case "audio":
            if (isNative()) {
              const mimeType = containerType === "video" ? "video/*" : "audio/*";
              openPlayer(path, fileName.value, mimeType);
            } else {
              router.push({ path: "/player", query: { path, name: fileName.value } });
            }
            loading.value = false;
            return;
          case "document":
          case "text": {
            const ext = getFileExtension(fileName.value);
            if (ext === "pdf") {
              previewType.value = "pdf";
              pdfPreviewUrl.value = getFilePreviewUrl("pdf.html", path);
            } else {
              previewType.value = "text";
            }
            break;
          }
          default:
            previewType.value = "container";
        }
      } else {
        previewType.value = "unsupported";
      }
    } catch (e: any) {
      console.error("Failed to load encrypted file:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
      error.value = e?.message || String(e);
    } finally {
      loading.value = false;
    }
    if (previewType.value === "text") {
      loadTextContent();
    }
    return;
  }

  previewType.value = await determinePreviewType(fileName.value, isEncrypted);

  try {
    if (previewType.value === "image") {
      console.info("Loading stream preview:", fileName.value);
      streamUrl.value = getFileStreamUrl(path);
    } else if (previewType.value === "pdf") {
      console.info("Loading PDF preview:", fileName.value);
      pdfPreviewUrl.value = getFilePreviewUrl("pdf.html", path);
    } else if (previewType.value === "text") {
      console.info("Loading text preview:", fileName.value);
    } else {
      console.info("Unsupported file type:", fileName.value);
      fileSize.value = 0;
    }
  } catch (e: any) {
    console.error("Failed to load file:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
    error.value = e?.message || String(e);
  } finally {
    loading.value = false;
  }

  if (previewType.value === "text") {
    loadTextContent();
  }
}

onMounted(() => loadFile());
</script>

<style scoped>
.preview-content {
  --background: var(--ion-background-color);
}

.preview-wrapper {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.image-preview {
  align-items: center;
  justify-content: center;
  padding: 8px;
}

.preview-image {
  width: 100%;
  max-height: 100%;
  object-fit: contain;
  border-radius: 4px;
}

.pdf-preview {
  flex: 1;
}

.preview-iframe {
  width: 100%;
  height: 100%;
  border: none;
  flex: 1;
}

.text-content-wrapper {
  height: 100%;
  display: flex;
  flex-direction: column;
  position: relative;
}

.text-content {
  flex: 1;
  margin: 0;
  padding: 16px;
  overflow: auto;
  font-family: 'JetBrains Mono', 'Fira Code', 'SF Mono', Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #e6edf3;
  white-space: pre-wrap;
  word-break: break-word;
  tab-size: 4;
  -webkit-overflow-scrolling: touch;
}

.text-content.no-wrap {
  white-space: pre;
  word-break: normal;
}

.text-loading,
.text-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: #888;
  gap: 12px;
  text-align: center;
}

.text-error p {
  color: #e74c3c;
  font-size: 13px;
  margin: 0;
}

.error-detail {
  width: 100%;
  max-height: 200px;
  overflow: auto;
  background: rgba(0, 0, 0, 0.3);
  border-radius: 6px;
  padding: 10px;
  margin-top: 8px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  line-height: 1.5;
  color: #f0c674;
  white-space: pre-wrap;
  word-break: break-all;
}

.container-info {
  padding: 16px;
  gap: 12px;
  overflow-y: auto;
}

.file-info-card {
  background: rgba(255, 255, 255, 0.04);
  border-radius: 8px;
  padding: 14px;
  margin: 12px 16px 0;
  border-left: 3px solid var(--ion-color-primary);
}

.container-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  background: rgba(231, 76, 60, 0.1);
  border-radius: 6px;
  color: #e74c3c;
  margin-bottom: 12px;
}
.container-error .error-icon { font-size: 20px; }
.container-error p { margin: 0; font-size: 13px; }

.container-card {
  background: rgba(255, 255, 255, 0.04);
  border-radius: 8px;
  padding: 14px;
  border-left: 3px solid var(--ion-color-primary);
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
.title-icon { color: var(--ion-color-primary); }

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
}

.info-value {
  color: var(--ion-text-color);
  font-weight: 400;
}

.code-text {
  font-family: monospace;
  font-size: 12px;
  word-break: break-all;
}

.manifest-section {
  background: rgba(255, 255, 255, 0.04);
  border-radius: 8px;
  overflow: hidden;
}

.manifest-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-text-secondary);
  user-select: none;
}
.manifest-header:hover { color: var(--ion-text-color); }

.manifest-json {
  margin: 0;
  padding: 10px 14px;
  max-height: 300px;
  overflow: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  line-height: 1.5;
  color: #888;
  white-space: pre-wrap;
  word-break: break-all;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.unsupported-preview {
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #888;
  text-align: center;
  padding: 24px;
}

.unsupported-icon {
  font-size: 64px;
  opacity: 0.4;
  color: #888;
}

.unsupported-preview h3 {
  margin: 0;
  color: #cdd6f4;
  font-size: 16px;
  word-break: break-all;
}

.unsupported-meta {
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: #888;
  flex-wrap: wrap;
  justify-content: center;
}

.unsupported-msg {
  color: #666;
  font-size: 13px;
  margin: 8px 0 0;
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
</style>
