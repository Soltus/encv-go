<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>
          <span class="home-title">{{ t('mpv.home.title') }}</span>
          <span v-if="!isDevPreview" class="version-mini">v{{ version }}</span>
        </ion-title>
        <ion-buttons slot="end">
          <ion-button @click="goToDevLogs" :title="t('mpv.devlogs.title')">
            <ion-icon :icon="documentTextOutline" slot="icon-only" />
          </ion-button>
          <ion-button @click="goToSettings" :title="t('mpv.settings.title')">
            <ion-icon :icon="settingsOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="section">
        <ion-card class="entry-card" button @click="openRecent">
          <ion-card-content class="entry-content">
            <div class="entry-icon recent-icon">
              <ion-icon :icon="timeOutline" />
            </div>
            <div class="entry-text">
              <h3>{{ t('mpv.home.recent') }}</h3>
              <p>最近播放的媒体文件</p>
            </div>
            <ion-icon :icon="chevronForwardOutline" class="entry-arrow" />
          </ion-card-content>
        </ion-card>

        <ion-card class="entry-card" button @click="openLibrary">
          <ion-card-content class="entry-content">
            <div class="entry-icon library-icon">
              <ion-icon :icon="libraryOutline" />
            </div>
            <div class="entry-text">
              <h3>{{ t('mpv.home.library') }}</h3>
              <p>浏览媒体库</p>
            </div>
            <ion-icon :icon="chevronForwardOutline" class="entry-arrow" />
          </ion-card-content>
        </ion-card>

        <ion-card class="entry-card" button @click="openPlaylist">
          <ion-card-content class="entry-content">
            <div class="entry-icon playlist-icon">
              <ion-icon :icon="listOutline" />
            </div>
            <div class="entry-text">
              <h3>{{ t('mpv.home.playlist') }}</h3>
              <p>管理播放列表</p>
            </div>
            <ion-icon :icon="chevronForwardOutline" class="entry-arrow" />
          </ion-card-content>
        </ion-card>
      </div>

      <div v-if="isDevPreview" class="dev-preview-notice">
        <div class="dev-preview-notice-row">
          <span class="notice-icon">🛠</span>
          <span class="notice-title">沙箱 Preview 模式</span>
        </div>
        <div class="dev-preview-notice-text">
          MPV 播放器在沙箱下不可用，UI 仅用于前端开发预览。
          <br />
          所有播放控制功能在真机上生效。
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { chevronForwardOutline, documentTextOutline, libraryOutline, listOutline, settingsOutline, timeOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { logBuffer, MpvNative } from "@/plugins/mpv-native";

const { t } = useI18n();
const router = useRouter();

const version = ref("unknown");
const isDevPreview = ref(false);

onMounted(() => {
  isDevPreview.value = !window.MpvNative;
  version.value = MpvNative.getVersion();

  logBuffer.info("MPV Player Web UI 已启动");
  if (isDevPreview.value) {
    logBuffer.info("当前为沙箱 dev preview 模式，播放器功能不可用");
  }
});

function openRecent() {
  logBuffer.info("打开最近播放");
}

function openLibrary() {
  logBuffer.info("打开媒体库");
}

function openPlaylist() {
  logBuffer.info("打开播放列表");
}

function goToDevLogs() {
  router.push("/devlogs");
}

function goToSettings() {
  router.push("/settings");
}
</script>

<style scoped>
.section {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.entry-card {
  margin: 0;
  border-radius: 12px;
  overflow: hidden;
}

.entry-content {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px !important;
}

.entry-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  color: var(--color-white);
  flex-shrink: 0;
}

.recent-icon {
  background: linear-gradient(135deg, #4f8cff 0%, #6366f1 100%);
}

.library-icon {
  background: linear-gradient(135deg, #2dd36f 0%, #10b981 100%);
}

.playlist-icon {
  background: linear-gradient(135deg, #ffc409 0%, #f97316 100%);
}

.entry-text {
  flex: 1;
  min-width: 0;
}

.entry-text h3 {
  margin: 0 0 4px 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--ion-text-color);
}

.entry-text p {
  margin: 0;
  font-size: 13px;
  color: var(--ion-color-medium);
}

.entry-arrow {
  color: var(--ion-color-medium);
  font-size: 20px;
  flex-shrink: 0;
}

.home-title {
  font-weight: 600;
}

.version-mini {
  margin-left: 8px;
  font-size: 11px;
  color: var(--ion-color-medium);
  font-weight: normal;
}

.dev-preview-notice {
  margin: 12px;
  padding: 12px 14px;
  border-radius: 10px;
  background: linear-gradient(135deg,
    rgba(245, 158, 11, 0.12) 0%,
    rgba(239, 68, 68, 0.10) 100%);
  border: 1px solid rgba(245, 158, 11, 0.4);
  border-left: 4px solid #f59e0b;
}

.dev-preview-notice-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.notice-icon {
  font-size: 18px;
}

.notice-title {
  font-size: 13px;
  font-weight: 700;
  color: #f59e0b;
  letter-spacing: 0.5px;
}

.dev-preview-notice-text {
  font-size: 12px;
  line-height: 1.6;
  color: var(--ion-text-color);
}
</style>
