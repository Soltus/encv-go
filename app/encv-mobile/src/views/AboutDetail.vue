<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.about') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- App 通用信息 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.about') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="informationCircle" slot="start"></ion-icon>
          <ion-label>
            <h3>ENCV-go</h3>
            <p>{{ appVersion }}</p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-icon :icon="codeSlash" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.engine') }}</h3>
            <p>ENCV-go Daemon</p>
          </ion-label>
        </ion-item>
        <ion-item button @click="openGitHub">
          <ion-icon :icon="logoGithub" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.github') }}</h3>
            <p>{{ t('settings.sourceCode') }}</p>
          </ion-label>
          <ion-icon :icon="openOutline" slot="end"></ion-icon>
        </ion-item>
      </ion-list>

      <!-- 原生引擎 (FFmpeg 来自 /api/build-info) -->
      <div v-if="buildInfoLoading" class="build-info-loading">
        <ion-spinner name="crescent"></ion-spinner>
      </div>
      <div v-else-if="buildInfoError" class="build-info-error">
        <ion-icon :icon="warningOutline" color="warning"></ion-icon>
        <span>{{ t('about.failedToLoad') }}</span>
      </div>
      <ion-list v-else>
        <ion-list-header>
          <ion-label>{{ t('about.nativeEngine') }}</ion-label>
        </ion-list-header>
        <ion-item v-if="buildInfo" button detail @click="goFfmpegEngine">
          <ion-icon :icon="videocamOutline" slot="start" class="lib-icon ffmpeg-icon"></ion-icon>
          <ion-label>
            <div class="lib-title-row">
              <h3 class="lib-name">FFmpeg</h3>
              <ion-badge color="medium" class="lib-badge version-badge">{{ buildInfo.ffmpeg_version }}</ion-badge>
              <ion-badge color="danger" class="lib-badge license-badge">{{ buildInfo.ffmpeg_license }}</ion-badge>
            </div>
            <p class="lib-desc">{{ t('about.ffmpegDesc') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- 🆕 2026-06-17：库展示重构 - 数据源 manifest + 状态/重要性双重标记 -->

      <!-- Android 库 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('about.androidLibs') }}</ion-label>
        </ion-list-header>
        <div v-if="libsLoading" class="build-info-loading">
          <ion-spinner name="crescent"></ion-spinner>
        </div>
        <div v-else-if="androidItems.length === 0" class="build-info-error">
          <ion-icon :icon="warningOutline" color="medium"></ion-icon>
          <span>{{ t('about.libsEmpty') }}</span>
        </div>
        <template v-else>
          <LibraryRow v-for="item in androidItems" :key="`android-${item.name}`" :item="item" @vue:mounted="onLibMounted(item)" />
        </template>
      </ion-list>

      <!-- 前端库 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('about.frontendLibs') }}</ion-label>
        </ion-list-header>
        <div v-if="frontendItems.length === 0" class="build-info-error">
          <ion-icon :icon="warningOutline" color="medium"></ion-icon>
          <span>{{ t('about.libsEmpty') }}</span>
        </div>
        <template v-else>
          <LibraryRow v-for="item in frontendItems" :key="`frontend-${item.name}`" :item="item" @vue:mounted="onLibMounted(item)" />
        </template>
      </ion-list>

      <!-- 后端库 -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('about.backendLibs') }}</ion-label>
        </ion-list-header>
        <div v-if="libsLoading && backendItems.length === 0" class="build-info-loading">
          <ion-spinner name="crescent"></ion-spinner>
        </div>
        <div v-else-if="backendItems.length === 0 && libsError" class="build-info-error">
          <ion-icon :icon="warningOutline" color="warning"></ion-icon>
          <span>{{ t('about.libsFailed') }}: {{ libsError }}</span>
        </div>
        <div v-else-if="backendItems.length === 0" class="build-info-error">
          <ion-icon :icon="warningOutline" color="medium"></ion-icon>
          <span>{{ t('about.libsEmpty') }}</span>
        </div>
        <template v-else>
          <LibraryRow v-for="item in backendItems" :key="`backend-${item.name}`" :item="item" @vue:mounted="onLibMounted(item)" />
        </template>
      </ion-list>

    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { codeSlash, informationCircle, logoGithub, openOutline, videocamOutline, warningOutline } from "ionicons/icons";
import { onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { type BuildInfo, fetchBuildInfo } from "@/api/encv";
import LibraryRow from "@/components/LibraryRow.vue";
import { useI18n } from "@/composables/useI18n";
import { type LibraryItem, useLibraries } from "@/composables/useLibraries";

const { t } = useI18n();
const router = useRouter();

const buildInfo = ref<BuildInfo | null>(null);
const buildInfoLoading = ref(true);
const buildInfoError = ref(false);
const appVersion = ref("v1.0.0");

const {
  androidItems,
  frontendItems,
  backendItems,
  loading: libsLoading,
  error: libsError,
  load: loadLibraries,
  resolveDescription,
} = useLibraries();

onMounted(async () => {
  // 加载 buildInfo
  try {
    const info = await fetchBuildInfo();
    buildInfo.value = info;
    if (info.app_version) {
      appVersion.value = info.app_version;
    }
  } catch {
    buildInfoError.value = true;
  } finally {
    buildInfoLoading.value = false;
  }
  // 加载库列表
  loadLibraries();
});

/**
 * 触发 description 解析
 * 监听所有 items 的变化,逐个处理无 description 的项
 */
async function onLibMounted(item: LibraryItem) {
  if (item.description) return; // 已有显式描述
  if (item.descriptionStatus === "fetched" || item.descriptionStatus === "placeholder") return; // 已尝试过
  // 异步解析（fire-and-forget）
  resolveDescription(item).catch(() => {
    item.descriptionStatus = "placeholder";
  });
}

watch([androidItems, frontendItems, backendItems], () => {
  for (const list of [androidItems.value, frontendItems.value, backendItems.value]) {
    for (const item of list) {
      if (!item.description && item.descriptionStatus === "placeholder") {
        onLibMounted(item);
      }
    }
  }
});

function openGitHub() {
  window.open("https://github.com/Soltus/encv-go", "_blank");
}

function goFfmpegEngine() {
  // 三级页：FFmpeg 引擎详情（runtime status + build info + 7 类 components）
  router.push("/tabs/settings/about/engine");
}
</script>

<style scoped>
.lib-icon {
  font-size: 24px;
  margin-right: 8px;
}

.ffmpeg-icon {
  color: var(--ion-color-primary);
}

.lib-title-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.lib-name {
  margin: 0;
  font-weight: 600;
}

.lib-desc {
  margin: 2px 0 0;
  font-size: 12px;
  opacity: 0.6;
}

.lib-badge {
  font-size: 10px;
  font-weight: 600;
  --padding-start: 6px;
  --padding-end: 6px;
  --padding-top: 2px;
  --padding-bottom: 2px;
}

.license-badge {
  --background: rgba(var(--ion-color-danger-rgb), 0.12);
  --color: var(--ion-color-danger);
}

.version-badge {
  --background: rgba(var(--ion-color-medium-rgb), 0.12);
  --color: var(--ion-color-medium-shade);
}

.build-info-loading {
  display: flex;
  justify-content: center;
  padding: 24px;
}

.build-info-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  font-size: 13px;
  opacity: 0.6;
}
</style>
