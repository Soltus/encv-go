<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/about"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.engineDetail') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('engine.runtimeStatus') }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="videocamOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>FFmpeg</h3>
            <p>
              <ion-badge :color="engineStatus?.ffmpeg_available ? 'success' : 'danger'">
                {{ engineStatus?.ffmpeg_available ? t('engine.available') : t('engine.unavailable') }}
              </ion-badge>
              <span v-if="!engineStatus?.ffmpeg_available && engineStatus?.ffmpeg_detail" class="engine-detail-text">
                {{ engineStatus.ffmpeg_detail }}
              </span>
            </p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-icon :icon="searchOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>FFprobe</h3>
            <p>
              <ion-badge :color="engineStatus?.ffprobe_available ? 'success' : 'danger'">
                {{ engineStatus?.ffprobe_available ? t('engine.available') : t('engine.unavailable') }}
              </ion-badge>
              <span v-if="!engineStatus?.ffprobe_available && engineStatus?.ffprobe_detail" class="engine-detail-text">
                {{ engineStatus.ffprobe_detail }}
              </span>
            </p>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-button fill="outline" size="small" @click="handleRefresh">
            <ion-icon :icon="refreshIcon" slot="start"></ion-icon>
            {{ t('engine.refresh') }}
          </ion-button>
        </ion-item>
      </ion-list>

      <div v-if="buildInfoLoading" class="build-info-loading">
        <ion-spinner name="crescent"></ion-spinner>
      </div>
      <div v-else-if="buildInfoError" class="build-info-error">
        <ion-icon :icon="warningOutline" color="warning"></ion-icon>
        <div class="build-info-error-content">
          <span class="error-title">{{ t('engine.loadFailed') }}</span>
          <span v-if="buildInfoErrorMessage" class="error-detail">{{ buildInfoErrorMessage }}</span>
        </div>
      </div>

      <template v-if="buildInfo">
        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('engine.buildInfo') }}</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-icon :icon="videocamOutline" slot="start"></ion-icon>
            <ion-label>
              <div class="lib-title-row">
                <h3 class="lib-name">FFmpeg</h3>
                <ion-badge color="medium" class="lib-badge version-badge">{{ buildInfo.ffmpeg_version }}</ion-badge>
                <ion-badge color="danger" class="lib-badge license-badge">{{ buildInfo.ffmpeg_license }}</ion-badge>
              </div>
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-icon :icon="constructOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('engine.ndkVersion') }}</h3>
              <p class="mono-text">{{ buildInfo.ndk_version }}</p>
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-icon :icon="phonePortraitOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('engine.apiLevel') }}</h3>
              <p>{{ buildInfo.api_level }}</p>
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-icon :icon="settingsOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('engine.abi') }}</h3>
              <p>{{ buildInfo.abi }}</p>
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-icon :icon="linkOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('engine.linking') }}</h3>
              <p>{{ t('engine.staticLinking') }}</p>
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-icon :icon="calendarOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('engine.buildDate') }}</h3>
              <p>{{ formatDate(buildInfo.build_date) }}</p>
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-icon :icon="codeSlashOutline" slot="start"></ion-icon>
            <ion-label class="ion-text-wrap">
              <h3>{{ t('engine.cflags') }}</h3>
              <p class="mono-text cflags-text">{{ buildInfo.cflags }}</p>
            </ion-label>
          </ion-item>
        </ion-list>

        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('engine.components') }}</ion-label>
          </ion-list-header>

          <ion-accordion-group>
            <ion-accordion value="decoders">
              <ion-item slot="header" class="lib-header-item">
                <ion-icon :icon="downloadOutline" slot="start" class="lib-icon"></ion-icon>
                <ion-label>
                  <div class="lib-title-row">
                    <h3 class="lib-name">{{ t('engine.decoders') }}</h3>
                    <ion-badge color="success" class="lib-badge count-badge">{{ buildInfo.enabled_decoders?.length ?? 0 }}</ion-badge>
                  </div>
                </ion-label>
              </ion-item>
              <div class="accordion-content" slot="content">
                <div class="tag-list">
                  <span v-for="d in buildInfo.enabled_decoders" :key="d" class="tech-tag decoder-tag">{{ d }}</span>
                </div>
              </div>
            </ion-accordion>

            <ion-accordion value="encoders">
              <ion-item slot="header" class="lib-header-item">
                <ion-icon :icon="cloudUploadOutline" slot="start" class="lib-icon"></ion-icon>
                <ion-label>
                  <div class="lib-title-row">
                    <h3 class="lib-name">{{ t('engine.encoders') }}</h3>
                    <ion-badge color="primary" class="lib-badge count-badge">{{ buildInfo.enabled_encoders?.length ?? 0 }}</ion-badge>
                  </div>
                </ion-label>
              </ion-item>
              <div class="accordion-content" slot="content">
                <div class="tag-list">
                  <span v-for="e in buildInfo.enabled_encoders" :key="e" class="tech-tag encoder-tag">{{ e }}</span>
                </div>
              </div>
            </ion-accordion>

            <ion-accordion value="muxers">
              <ion-item slot="header" class="lib-header-item">
                <ion-icon :icon="filmOutline" slot="start" class="lib-icon"></ion-icon>
                <ion-label>
                  <div class="lib-title-row">
                    <h3 class="lib-name">{{ t('engine.muxers') }}</h3>
                    <ion-badge color="warning" class="lib-badge count-badge">{{ buildInfo.enabled_muxers?.length ?? 0 }}</ion-badge>
                  </div>
                </ion-label>
              </ion-item>
              <div class="accordion-content" slot="content">
                <div class="tag-list">
                  <span v-for="m in buildInfo.enabled_muxers" :key="m" class="tech-tag muxer-tag">{{ m }}</span>
                </div>
              </div>
            </ion-accordion>

            <ion-accordion value="demuxers">
              <ion-item slot="header" class="lib-header-item">
                <ion-icon :icon="documentOutline" slot="start" class="lib-icon"></ion-icon>
                <ion-label>
                  <div class="lib-title-row">
                    <h3 class="lib-name">{{ t('engine.demuxers') }}</h3>
                    <ion-badge color="tertiary" class="lib-badge count-badge">{{ buildInfo.enabled_demuxers?.length ?? 0 }}</ion-badge>
                  </div>
                </ion-label>
              </ion-item>
              <div class="accordion-content" slot="content">
                <div class="tag-list">
                  <span v-for="d in buildInfo.enabled_demuxers" :key="d" class="tech-tag demuxer-tag">{{ d }}</span>
                </div>
              </div>
            </ion-accordion>

            <ion-accordion value="parsers">
              <ion-item slot="header" class="lib-header-item">
                <ion-icon :icon="codeWorkingOutline" slot="start" class="lib-icon"></ion-icon>
                <ion-label>
                  <div class="lib-title-row">
                    <h3 class="lib-name">{{ t('engine.parsers') }}</h3>
                    <ion-badge color="medium" class="lib-badge count-badge">{{ buildInfo.enabled_parsers?.length ?? 0 }}</ion-badge>
                  </div>
                </ion-label>
              </ion-item>
              <div class="accordion-content" slot="content">
                <div class="tag-list">
                  <span v-for="p in buildInfo.enabled_parsers" :key="p" class="tech-tag parser-tag">{{ p }}</span>
                </div>
              </div>
            </ion-accordion>

            <ion-accordion value="protocols">
              <ion-item slot="header" class="lib-header-item">
                <ion-icon :icon="globeOutline" slot="start" class="lib-icon"></ion-icon>
                <ion-label>
                  <div class="lib-title-row">
                    <h3 class="lib-name">{{ t('engine.protocols') }}</h3>
                    <ion-badge color="danger" class="lib-badge count-badge">{{ buildInfo.enabled_protocols?.length ?? 0 }}</ion-badge>
                  </div>
                </ion-label>
              </ion-item>
              <div class="accordion-content" slot="content">
                <div class="tag-list">
                  <span v-for="p in buildInfo.enabled_protocols" :key="p" class="tech-tag protocol-tag">{{ p }}</span>
                </div>
              </div>
            </ion-accordion>

            <ion-accordion v-if="buildInfo.enabled_filters && buildInfo.enabled_filters.length > 0" value="filters">
              <ion-item slot="header" class="lib-header-item">
                <ion-icon :icon="filterOutline" slot="start" class="lib-icon"></ion-icon>
                <ion-label>
                  <div class="lib-title-row">
                    <h3 class="lib-name">{{ t('engine.filters') }}</h3>
                    <ion-badge color="success" class="lib-badge count-badge">{{ buildInfo.enabled_filters.length }}</ion-badge>
                  </div>
                </ion-label>
              </ion-item>
              <div class="accordion-content" slot="content">
                <div class="tag-list">
                  <span v-for="f in buildInfo.enabled_filters" :key="f" class="tech-tag filter-tag">{{ f }}</span>
                </div>
              </div>
            </ion-accordion>

            <ion-accordion value="static-libs">
              <ion-item slot="header" class="lib-header-item">
                <ion-icon :icon="cubeOutline" slot="start" class="lib-icon"></ion-icon>
                <ion-label>
                  <div class="lib-title-row">
                    <h3 class="lib-name">{{ t('engine.staticLibs') }}</h3>
                    <ion-badge color="medium" class="lib-badge count-badge">{{ buildInfo.static_libs?.length ?? 0 }}</ion-badge>
                  </div>
                </ion-label>
              </ion-item>
              <div class="accordion-content" slot="content">
                <div class="tag-list">
                  <span v-for="lib in buildInfo.static_libs" :key="lib" class="tech-tag lib-tag">{{ lib }}</span>
                </div>
              </div>
            </ion-accordion>
          </ion-accordion-group>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  calendarOutline,
  cloudUploadOutline,
  codeSlashOutline,
  codeWorkingOutline,
  constructOutline,
  cubeOutline,
  documentOutline,
  downloadOutline,
  filmOutline,
  filterOutline,
  globeOutline,
  linkOutline,
  phonePortraitOutline,
  refresh as refreshIcon,
  searchOutline,
  settingsOutline,
  videocamOutline,
  warningOutline,
} from "ionicons/icons";
import { onMounted, ref } from "vue";
import { type BuildInfo, type FFmpegStatus, fetchBuildInfo, fetchFFmpegStatus } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";

const { t } = useI18n();

const engineStatus = ref<FFmpegStatus | null>(null);
const buildInfo = ref<BuildInfo | null>(null);
const buildInfoLoading = ref(true);
const buildInfoError = ref(false);
const buildInfoErrorMessage = ref(""); // ✅ 新增：保存详细错误信息

function formatDate(dateStr: string): string {
  if (!dateStr) return "";
  try {
    const d = new Date(dateStr);
    return d.toLocaleDateString() + " " + d.toLocaleTimeString();
  } catch {
    return dateStr;
  }
}

async function handleRefresh() {
  try {
    engineStatus.value = await fetchFFmpegStatus();
  } catch {}
}

onMounted(async () => {
  try {
    engineStatus.value = await fetchFFmpegStatus();
  } catch {}
  try {
    buildInfo.value = await fetchBuildInfo();
  } catch (e) {
    buildInfoError.value = true;
    buildInfoErrorMessage.value = (e as Error)?.message || String(e) || "Unknown error";
    console.error("[FfmpegEngineDetail] build info load error:", e); // ✅ 控制台也打出来
  } finally {
    buildInfoLoading.value = false;
  }
});
</script>

<style scoped>
.engine-detail-text {
  color: var(--ion-color-danger);
  font-size: 11px;
  margin-left: 4px;
  word-break: break-all;
}

.build-info-loading {
  display: flex;
  justify-content: center;
  padding: 24px;
}

.build-info-error {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 16px;
  background: rgba(255, 193, 7, 0.1);
  border-radius: 8px;
}

.build-info-error-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.error-title {
  font-weight: 500;
  color: var(--ion-color-warning);
}

.error-detail {
  font-size: 12px;
  color: var(--ion-color-medium);
  font-family: monospace;
  word-break: break-all;
}


.lib-header-item {
  --padding-start: 12px;
}

.lib-icon {
  font-size: 24px;
  margin-right: 8px;
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

.count-badge {
  --background: rgba(var(--ion-color-medium-rgb), 0.08);
  --color: var(--ion-color-medium);
}

.mono-text {
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', 'JetBrains Mono', monospace;
  font-size: 12px;
}

.cflags-text {
  font-size: 11px;
  line-height: 1.4;
  opacity: 0.8;
  word-break: break-all;
}

.accordion-content {
  padding: 8px 16px 16px;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.tech-tag {
  font-size: 11px;
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', 'JetBrains Mono', monospace;
  padding: 2px 8px;
  border-radius: 4px;
  white-space: nowrap;
}

.decoder-tag {
  background: rgba(var(--ion-color-success-rgb), 0.1);
  color: var(--ion-color-success-shade);
}

.encoder-tag {
  background: rgba(var(--ion-color-primary-rgb), 0.1);
  color: var(--ion-color-primary-shade);
}

.muxer-tag {
  background: rgba(var(--ion-color-warning-rgb), 0.1);
  color: var(--ion-color-warning-shade);
}

.demuxer-tag {
  background: rgba(var(--ion-color-tertiary-rgb), 0.1);
  color: var(--ion-color-tertiary-shade);
}

.parser-tag {
  background: rgba(var(--ion-color-medium-rgb), 0.1);
  color: var(--ion-color-medium-shade);
}

.protocol-tag {
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  color: var(--ion-color-danger-shade);
}

.filter-tag {
  background: rgba(var(--ion-color-success-rgb), 0.08);
  color: var(--ion-color-success-shade);
}

.lib-tag {
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  color: var(--ion-color-medium);
  font-size: 10px;
}
</style>
