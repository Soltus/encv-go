<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('settings.title') }}</ion-title>
        <ion-buttons slot="end" v-if="dirty">
          <ion-button @click="handleResetConfig" color="medium">{{ t('settings.undo') }}</ion-button>
          <ion-button @click="handleSaveConfig" :disabled="configLoading">
            <ion-icon :icon="saveIcon" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.appearance') }}</ion-label>
          <ion-badge slot="end" color="medium" class="scope-badge">
            <ion-icon :icon="phonePortraitOutline" class="scope-badge-icon"></ion-icon>
            <span class="scope-text">{{ t('settings.localOnly') }}</span>
          </ion-badge>
        </ion-list-header>
        <ion-item button @click="goAppearance" detail>
          <ion-icon :icon="colorPaletteOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.appearance') }}</h3>
            <p>{{ t('settings.appearanceHelp') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.player') }}</ion-label>
          <ion-badge slot="end" color="medium" class="scope-badge">
            <ion-icon :icon="phonePortraitOutline" class="scope-badge-icon"></ion-icon>
            <span class="scope-text">{{ t('settings.localOnly') }}</span>
          </ion-badge>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="filmOutline" slot="start"></ion-icon>
          <ion-select
            :value="videoPlayerMode"
            @ionChange="handleVideoPlayerChange"
            :label="t('settings.videoPlayer')"
            label-placement="stacked"
            interface="action-sheet"
            mode="ios"
          >
            <ion-select-option value="artplayer">{{ t('settings.builtInArtplayer') }}</ion-select-option>
            <ion-select-option value="mpv-activity">MPV (Activity)</ion-select-option>
            <ion-select-option value="mpv-fragment" :disabled="true">MPV (Fragment) [实验]</ion-select-option>
            <ion-select-option value="mpv-compose" :disabled="true">MPV (Compose) [实验]</ion-select-option>
            <ion-select-option value="external">{{ t('settings.openExternal') }}</ion-select-option>
          </ion-select>
          <ion-badge v-if="isNative() && isMpvMode(videoPlayerMode) && mpvPluginStatus !== 'unknown' && mpvPluginStatus !== 'ready'" slot="end" :color="mpvPluginStatus === 'load_failed' || mpvPluginStatus === 'error' ? 'danger' : 'warning'">
            {{ t(mpvStatusI18nKey) }}
          </ion-badge>
          <ion-badge v-if="isNative() && isMpvMode(videoPlayerMode) && mpvPluginStatus === 'ready'" slot="end" color="success">✓</ion-badge>
        </ion-item>
        <ion-item>
          <ion-icon :icon="musicalNotesOutline" slot="start"></ion-icon>
          <ion-select
            :value="audioPlayerMode"
            @ionChange="handleAudioPlayerChange"
            :label="t('settings.audioPlayer')"
            label-placement="stacked"
            interface="action-sheet"
            mode="ios"
          >
            <ion-select-option value="mpv">{{ t('settings.builtInMpv') }}</ion-select-option>
            <ion-select-option value="external">{{ t('settings.openExternal') }}</ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-icon :icon="phonePortraitOutline" slot="start"></ion-icon>
          <ion-select
            :value="screenOrientation"
            @ionChange="handleScreenOrientationChange"
            :label="t('settings.screenOrientation')"
            label-placement="stacked"
            interface="action-sheet"
            mode="ios"
          >
            <ion-select-option value="auto">{{ t('settings.orientationAuto') }}</ion-select-option>
            <ion-select-option value="portrait">{{ t('settings.orientationPortrait') }}</ion-select-option>
            <ion-select-option value="landscape">{{ t('settings.orientationLandscape') }}</ion-select-option>
          </ion-select>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.connection') }}</ion-label>
        </ion-list-header>
        <ion-item button @click="goServer" detail>
          <ion-icon :icon="serverIcon" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.serverTitle') }}</h3>
            <p>
              <ion-badge :color="serverOnline ? 'success' : 'danger'">
                {{ serverOnline ? t('settings.online') : t('settings.offline') }}
              </ion-badge>
              <span v-if="serverOnline && backendPort" class="port-info">:{{ backendPort }}</span>
              <span v-if="!serverOnline && connectionError" class="connection-error-inline"> - {{ connectionError }}</span>
            </p>
            <!-- 🆕 2026-06-15：复用 desktop performPingCheck 的 instance_id 防劫持
                 展示当前 backend 进程唯一 ID（前 8 字符）+ version，
                 让用户/AI 能直接核对"我连的是不是同一个进程" -->
            <p v-if="serverOnline && backendInstanceId" class="instance-info">
              <code class="instance-id">{{ backendInstanceId.slice(0, 8) }}</code>
              <span v-if="backendVersion" class="version-info">v{{ backendVersion }}</span>
            </p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('settings.storage') }}</ion-label>
        </ion-list-header>
        <ion-item button @click="goCache" detail>
          <ion-icon :icon="databaseIcon" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.cacheAndIndex') }}</h3>
            <p>{{ indexStats?.isIndexing ? t('settings.indexing') : (indexStats && indexStats.totalFiles > 0 ? (indexStats.source === 'webdav' ? 'WebDAV ' + t('settings.indexReady') : t('settings.indexReady')) : t('settings.noIndexData')) }}</p>
          </ion-label>
        </ion-item>
        <ion-item button @click="goMounts" detail>
          <ion-icon :icon="serverIcon" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.mountsTitle') }}</h3>
            <p>{{ t('settings.mountsHelp') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <div v-if="configLoading && !configLoaded" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('settings.loadingConfig') }}</p>
      </div>

      <template v-else-if="configLoaded">
        <template v-for="section in schemaFields" :key="section.key">
          <!-- 特殊 section：database（独立二级页面） -->
          <ion-list v-if="section.key === 'database'">
            <ion-list-header>
              <ion-label>{{ section.sectionTitle ? tSectionTitle(section.sectionTitle) : tField(section.key) }}</ion-label>
              <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
                <ion-icon :icon="cloudOutline" class="scope-badge-icon"></ion-icon>
                <span class="scope-text">{{ t('settings.synced') }}</span>
              </ion-badge>
            </ion-list-header>
            <ion-item button @click="goDatabase" detail>
              <ion-icon :icon="hardwareChipOutline" slot="start"></ion-icon>
              <ion-label>
                <h3>{{ tField(section.key) }}</h3>
                <p>{{ dbInfo ? t('settings.engine') + ': ' + dbInfo.engine + ' · ' + dbInfo.taskCount + ' ' + t('settings.tasks') : t('settings.loading') }}</p>
              </ion-label>
            </ion-item>
          </ion-list>

          <!-- 过滤掉 server/admin/webdav/proxy/log 配置项，这些有独立页面 -->
        <template v-if="!['server', 'admin', 'webdav', 'proxy', 'log'].includes(section.key) && section.key !== 'database'">
          <ion-list v-if="section.key === 'plugin_settings'">
            <ion-list-header>
              <ion-label>{{ section.sectionTitle ? tSectionTitle(section.sectionTitle) : tField(section.key) }}</ion-label>
              <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
                <ion-icon :icon="cloudOutline" class="scope-badge-icon"></ion-icon>
                <span class="scope-text">{{ t('settings.synced') }}</span>
              </ion-badge>
            </ion-list-header>
            <ion-item button @click="goPlugins" detail>
              <ion-icon :icon="settingsOutline" slot="start"></ion-icon>
              <ion-label>
                <h3>{{ tField(section.key) }}</h3>
              </ion-label>
            </ion-item>
          </ion-list>

          <ion-list v-else-if="section.key === 'agent_settings'">
            <ion-list-header>
              <ion-label>{{ section.sectionTitle ? tSectionTitle(section.sectionTitle) : tField(section.key) }}</ion-label>
              <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
                <ion-icon :icon="cloudOutline" class="scope-badge-icon"></ion-icon>
                <span class="scope-text">{{ t('settings.synced') }}</span>
              </ion-badge>
            </ion-list-header>
            <ion-item button @click="goAgent" detail>
              <ion-icon :icon="sparklesOutline" slot="start"></ion-icon>
              <ion-label>
                <h3>{{ t('settings.agent') }}</h3>
                <p>{{ t('settings.agentSettingsHelp') }}</p>
              </ion-label>
            </ion-item>
          </ion-list>

          <ion-list v-else-if="section.type !== 'object' || !section.properties">
            <ion-list-header>
              <ion-label>{{ section.sectionTitle ? tSectionTitle(section.sectionTitle) : tField(section.key) }}</ion-label>
              <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
                <ion-icon :icon="cloudOutline" class="scope-badge-icon"></ion-icon>
                <span class="scope-text">{{ t('settings.synced') }}</span>
              </ion-badge>
            </ion-list-header>
            <ConfigFieldItem
              :field="section"
              :model-value="getValue([section.key])"
              :label="fieldLabel(section.key, section.required)"
              :placeholder="section.description || tField(section.key)"
              :icon="getFieldIcon(section.key, section.type)"
              @update:model-value="setValue([section.key], $event)"
              @input="handleInput([section.key], section, $event)"
              @browse="handleBrowsePath([section.key], section)"
              @reset="resetFieldToDefault([section.key], section)"
            />
          </ion-list>

          <ion-list v-else>
            <ion-list-header>
              <ion-label>{{ section.sectionTitle ? tSectionTitle(section.sectionTitle) : tField(section.key) }}</ion-label>
              <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
                <ion-icon :icon="cloudOutline" class="scope-badge-icon"></ion-icon>
                <span class="scope-text">{{ t('settings.synced') }}</span>
              </ion-badge>
            </ion-list-header>

            <template v-for="child in section.properties" :key="child.key">
              <template v-if="child.type === 'object' && child.properties && !child.isMap">
                <ion-item-divider>
                  <ion-label>{{ tField(child.key) }}</ion-label>
                </ion-item-divider>
                <template v-for="grandchild in child.properties" :key="grandchild.key">
                  <template v-if="isFieldVisible(grandchild)">
                    <ConfigFieldItem
                      :field="grandchild"
                      :model-value="getValue([section.key, child.key, grandchild.key])"
                      :label="fieldLabel(grandchild.key, grandchild.required)"
                      :placeholder="grandchild.description || tField(grandchild.key)"
                      :icon="getFieldIcon(grandchild.key, grandchild.type)"
                      @update:model-value="setValue([section.key, child.key, grandchild.key], $event)"
                      @input="handleInput([section.key, child.key, grandchild.key], grandchild, $event)"
                      @browse="handleBrowsePath([section.key, child.key, grandchild.key], grandchild)"
                      @reset="resetFieldToDefault([section.key, child.key, grandchild.key], grandchild)"
                    />
                  </template>
                </template>
              </template>

              <template v-else-if="child.isMap">
                <ion-item-divider>
                  <ion-label>{{ tField(child.key) }}</ion-label>
                </ion-item-divider>
                <template v-if="getMapEntries([section.key, child.key]).length > 0">
                  <ion-item v-for="[entryKey, entryVal] in getMapEntries([section.key, child.key])" :key="entryKey">
                    <ion-label>
                      <h3>{{ entryKey }}</h3>
                      <p v-if="entryVal && typeof entryVal === 'object'">
                        <template v-for="itemField in child.mapItemFields" :key="itemField.key">
                          {{ tField(itemField.key) }}: {{ (entryVal as Record<string, unknown>)[itemField.key] || '-' }}&nbsp;
                        </template>
                      </p>
                    </ion-label>
                  </ion-item>
                </template>
                <ion-item v-else>
                  <ion-label class="ion-text-wrap placeholder-text">
                    <p>{{ t('settings.noEntries') }}</p>
                  </ion-label>
                </ion-item>
              </template>

              <template v-else-if="isFieldVisible(child)">
                <ConfigFieldItem
                  :field="child"
                  :model-value="getValue([section.key, child.key])"
                  :label="fieldLabel(child.key, child.required)"
                  :placeholder="child.description || tField(child.key)"
                  :icon="getFieldIcon(child.key, child.type)"
                  @update:model-value="setValue([section.key, child.key], $event)"
                  @input="handleInput([section.key, child.key], child, $event)"
                  @browse="handleBrowsePath([section.key, child.key], child)"
                  @reset="resetFieldToDefault([section.key, child.key], child)"
                />
              </template>
            </template>

            <ion-item v-if="!section.properties || section.properties.length === 0">
              <ion-label class="ion-text-wrap placeholder-text">
                <p>{{ t('settings.noEntries') }}</p>
              </ion-label>
            </ion-item>
          </ion-list>
          </template>
        </template>
      </template>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.title') }}</ion-label>
        </ion-list-header>
        <ion-item button @click="goDevTools" detail>
          <ion-icon :icon="bugOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.title') }}</h3>
            <p>{{ t('devtools.devtoolsDesc') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label color="danger">{{ t('settings.dangerZone') }}</ion-label>
        </ion-list-header>
        <ion-item button @click="handleClearCache">
          <ion-icon :icon="trash" color="danger" slot="start"></ion-icon>
          <ion-label color="danger">{{ t('settings.clearCache') }}</ion-label>
        </ion-item>
        <ion-item button @click="handleResetSettings">
          <ion-icon :icon="refreshCircle" color="danger" slot="start"></ion-icon>
          <ion-label color="danger">{{ t('settings.resetSettings') }}</ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-item button @click="goAbout" detail>
          <ion-icon :icon="informationCircle" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.about') }}</h3>
            <p>ENCV-go v1.0.0</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-item button @click="openJsonEditor">
          <ion-icon :icon="documentText" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.editRawConfig') }}</h3>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-modal :is-open="showJsonEditor" @didDismiss="showJsonEditor = false">
        <ion-header>
          <ion-toolbar>
            <ion-title>{{ t('settings.editRawConfig') }}</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="showJsonEditor = false">{{ t('settings.cancel') }}</ion-button>
              <ion-button @click="handleSaveJson" :disabled="!!jsonError" color="primary">{{ t('settings.saveConfig') }}</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content class="json-editor-content">
          <div class="json-editor-layout">
            <div class="json-annotations" v-if="configAnnotations.length > 0">
              <div class="annotations-title">{{ t('settings.configAnnotations') }}</div>
              <div v-for="ann in configAnnotations" :key="ann.path" class="annotation-item">
                <span class="annotation-path">{{ ann.path }}</span>
                <span class="annotation-desc">{{ ann.description }}</span>
              </div>
            </div>
            <div class="json-textarea-wrapper">
              <textarea
                v-model="jsonText"
                class="json-textarea"
                spellcheck="false"
                @input="validateJson"
              ></textarea>
              <div v-if="jsonError" class="json-error">
                {{ t('settings.jsonError') }}: {{ jsonError }}
              </div>
            </div>
          </div>
        </ion-content>
      </ion-modal>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useModal } from "@encv/shared-components/composables/useModal";
import { useConfirmDialog } from "@encv/shared-components/composables/useConfirmDialog";
import {
  bugOutline,
  cloudOutline,
  colorPaletteOutline,
  server as databaseIcon,
  documentText,
  eyeOutline,
  filmOutline,
  folderOpen,
  gitNetworkOutline,
  globeOutline,
  hardwareChipOutline,
  imagesOutline,
  informationCircle,
  key,
  layersOutline,
  lockClosed,
  musicalNotesOutline,
  newspaperOutline,
  personOutline,
  phonePortraitOutline,
  readerOutline,
  refreshCircle,
  save as saveIcon,
  server as serverIcon,
  settingsOutline,
  shieldCheckmark,
  sparklesOutline,
  speedometerOutline,
  terminal,
  textOutline,
  toggleOutline,
  trash,
} from "ionicons/icons";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import type { DatabaseInfo, IndexStats } from "@encv/shared-components/api/encv";
import { fetchConfig, getDatabaseInfo, getIndexStats, updateConfig } from "@encv/shared-components/api/encv";
import ConfigFieldItem from "@/components/ConfigFieldItem.vue";
import FilePickerModal from "@encv/shared-components/components/FilePickerModal.vue";
import { useConfig } from "@encv/shared-components/composables/useConfig";
import { registerFileFeature, unregisterFileFeature } from "@encv/shared-components/composables/useFileFeatures";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useServerStatus } from "@encv/shared-components/composables/useServerStatus";
import { showToast } from "@encv/shared-components/composables/useToast";
import type { FieldDef } from "@encv/shared-components/config/schemaParser";
import { isMpvSubMode, PLAY_MODE } from "@encv/shared-components/constants/player";
import { createAlistEncryptFeature } from "@encv/shared-components/features/alist-encrypt/index";
import { ensurePluginLoaded, getPluginFullState, isNative, pickFolder } from "@/plugins/GoProcess";

const router = useRouter();
const {
  isOnline: serverOnline,
  lastError: connectionError,
  checkStatus,
  backendPort,
  backendInstanceId,
  backendVersion,
} = useServerStatus();
const {
  schemaFields,
  loading: configLoading,
  dirty,
  restartNeeded,
  loadConfig,
  saveConfig,
  resetConfig,
  getFieldValue,
  setFieldValue,
  resetFieldToDefault,
} = useConfig();
const { t, tField, tSectionTitle } = useI18n();

const configLoaded = ref(false);
const indexStats = ref<IndexStats | null>(null);
const dbInfo = ref<DatabaseInfo | null>(null);

const videoPlayerMode = ref(localStorage.getItem("encv_player_video") || PLAY_MODE.ARTPLAYER);
const audioPlayerMode = ref(localStorage.getItem("encv_player_audio") || PLAY_MODE.MPV_PLUGIN);
const screenOrientation = ref(localStorage.getItem("encv_screen_orientation") || "auto");
const mpvPluginStatus = ref<string>("unknown");
const mpvPluginError = ref("");

const mpvStatusI18nKey = computed(() => {
  const keyMap: Record<string, string> = {
    not_installed: "settings.pluginNotInstalled",
    disabled: "settings.pluginDisabled",
    not_loaded: "settings.pluginNotLoaded",
    load_failed: "settings.pluginLoadFailed",
    error: "settings.pluginQueryFailed",
    framework_not_ready: "settings.pluginFrameworkNotReady",
  };
  return keyMap[mpvPluginStatus.value] || "settings.pluginQueryFailed";
});

function isMpvMode(mode: string): boolean {
  return isMpvSubMode(mode) || mode === "mpv-plugin" || mode === "mpv";
}

async function handleVideoPlayerChange(event: CustomEvent) {
  const value = event.detail.value;
  videoPlayerMode.value = value;
  localStorage.setItem("encv_player_video", value);
  if (isMpvMode(value)) await refreshMpvPluginStatus();
}

async function handleAudioPlayerChange(event: CustomEvent) {
  const value = event.detail.value;
  audioPlayerMode.value = value;
  localStorage.setItem("encv_player_audio", value);
  if (isMpvMode(value)) await refreshMpvPluginStatus();
}

function handleScreenOrientationChange(event: CustomEvent) {
  const value = event.detail.value;
  screenOrientation.value = value;
  localStorage.setItem("encv_screen_orientation", value);
  applyScreenOrientation(value);
}

async function applyScreenOrientation(orientation: string) {
  if (!isNative()) return;
  try {
    const { ScreenOrientation } = await import("@capacitor/screen-orientation");
    if (orientation === "portrait") {
      await ScreenOrientation.lock({ orientation: "portrait" });
    } else if (orientation === "landscape") {
      await ScreenOrientation.lock({ orientation: "landscape" });
    } else {
      await ScreenOrientation.unlock();
    }
  } catch (e) {
    console.debug("Failed to apply screen orientation:", e);
  }
}

function goAppearance() {
  router.push("/tabs/settings/appearance");
}

function goDevTools() {
  router.push("/tabs/settings/devtools");
}

const showJsonEditor = ref(false);
const jsonText = ref("");
const jsonError = ref("");
const configAnnotations = ref<{ path: string; description: string }[]>([]);

function extractAnnotations(schema: any, prefix: string = ""): { path: string; description: string }[] {
  const result: { path: string; description: string }[] = [];
  if (!schema || typeof schema !== "object") return result;
  if (schema.properties) {
    for (const [key, val] of Object.entries(schema.properties)) {
      const prop = val as any;
      const fullPath = prefix ? `${prefix}.${key}` : key;
      if (prop.description) {
        result.push({ path: fullPath, description: prop.description });
      }
      if (prop.properties) {
        result.push(...extractAnnotations(prop, fullPath));
      }
    }
  }
  if (schema.$defs) {
    for (const [key, val] of Object.entries(schema.$defs)) {
      result.push(...extractAnnotations(val, `$defs.${key}`));
    }
  }
  return result;
}

async function openJsonEditor() {
  try {
    const cfg = await fetchConfig();
    jsonText.value = JSON.stringify(cfg, null, 2);
    jsonError.value = "";
    try {
      const schemaResp = await fetch("/api/config/schema");
      if (schemaResp.ok) {
        const schema = await schemaResp.json();
        configAnnotations.value = extractAnnotations(schema);
      }
    } catch {}
    showJsonEditor.value = true;
  } catch {
    showToast({ message: t("settings.configSaveFailed"), duration: 2000, color: "danger" });
  }
}

function validateJson() {
  try {
    JSON.parse(jsonText.value);
    jsonError.value = "";
  } catch (e) {
    jsonError.value = e instanceof Error ? e.message : String(e);
  }
}

async function handleSaveJson() {
  try {
    const parsed = JSON.parse(jsonText.value);
    await updateConfig(parsed);
    showJsonEditor.value = false;
    showToast({ message: t("settings.configSaved"), duration: 1500, color: "success" });
    await loadConfig();
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: t("settings.configSaveFailed") + ": " + detail, duration: 3000, color: "danger" });
  }
}

function goServer() {
  router.push("/tabs/settings/server");
}

function goAbout() {
  router.push("/tabs/settings/about");
}

function goCache() {
  router.push("/tabs/settings/cache");
}

function goDatabase() {
  router.push("/tabs/settings/database");
}

function goMounts() {
  router.push("/tabs/settings/mounts");
}

function goPlugins() {
  router.push("/tabs/settings/plugins");
}

function goAgent() {
  router.push("/tabs/settings/agent");
}

function getValue(path: string[]): unknown {
  return getFieldValue(path);
}

function setValue(path: string[], value: unknown) {
  setFieldValue(path, value);
}

function getMapEntries(path: string[]): [string, Record<string, unknown>][] {
  const val = getFieldValue(path);
  if (!val || typeof val !== "object") return [];
  return Object.entries(val as Record<string, unknown>) as [string, Record<string, unknown>][];
}

function handleInput(path: string[], field: FieldDef, event: CustomEvent) {
  const val = (event.target as HTMLInputElement).value;
  if (path.length >= 2 && path[0] === "webdav" && path[1] === "root" && val) {
    const err = validateWebdavRoute(val);
    if (err) {
      showToast({ message: err, duration: 3000, color: "danger" });
      return;
    }
  }
  if (field.type === "integer") {
    setFieldValue(path, val ? Number(val) : 0);
  } else {
    setFieldValue(path, val);
  }
}

function validateWebdavRoute(val: string): string | null {
  const t = val.trim();
  if (!t) return null;
  if (t === "/" || t === "//") return 'WebDAV 路由不能为 "/"，这会导致服务崩溃';
  if (!t.startsWith("/")) return 'WebDAV 路由必须以 "/" 开头';
  return null;
}

async function handleBrowsePath(path: string[], field: FieldDef) {
  if (isNative()) {
    const result = await pickFolder();
    if (result.path) {
      setFieldValue(path, result.path);
    }
    return;
  }
  const isFolder = field.key !== "file";
  const currentVal = String(getFieldValue(path) || "/");
  const { openModal } = useModal();
  const { data, role } = await openModal<{ path: string }>({
    component: FilePickerModal,
    componentProps: {
      mode: isFolder ? "folder" : "file",
      initialPath: currentVal,
    },
  });
  if (role === "select" && data) {
    setFieldValue(path, data.path);
  }
}

function fieldLabel(key: string, _required?: boolean): string {
  return tField(key);
}

const fieldIconMap: Record<string, string> = {
  password: key,
  recover: refreshCircle,
  output_path: folderOpen,
  plugin_settings: settingsOutline,
  server: cloudOutline,
  admin: shieldCheckmark,
  webdav: globeOutline,
  proxy: gitNetworkOutline,
  log: terminal,
  port: speedometerOutline,
  dir: folderOpen,
  username: personOutline,
  root: documentText,
  level: speedometerOutline,
  file: documentText,
  console: terminal,
  host: serverIcon,
  description: textOutline,
  sites: globeOutline,
  disable_signature_verification: shieldCheckmark,
  ext: documentText,
  container_chunk_size_mb: filmOutline,
  light_container_main_chunk_enabled: layersOutline,
  track_extensions: eyeOutline,
  keep_mkv_for_mkv_source: filmOutline,
  verify_after_pack: shieldCheckmark,
  plugin_cache_dir: folderOpen,
  skip_merge_for_split_mkv: filmOutline,
  allow_no_reencode: speedometerOutline,
  default_stream_preset: colorPaletteOutline,
  video: filmOutline,
  audio: musicalNotesOutline,
  image: imagesOutline,
  wps: readerOutline,
  pdf: newspaperOutline,
  text: textOutline,
};

function getFieldIcon(fieldKey: string, fieldType: string): string {
  if (fieldIconMap[fieldKey]) return fieldIconMap[fieldKey];
  if (fieldType === "boolean") return toggleOutline;
  if (fieldType === "integer") return speedometerOutline;
  if (fieldKey.includes("password")) return lockClosed;
  return settingsOutline;
}

function isFieldVisible(field: FieldDef): boolean {
  if (field.key === "console") return false;
  if (!field.platform || field.platform === "both") return true;
  if (field.platform === "mobile") return isNative();
  if (field.platform === "desktop") return !isNative();
  return true;
}

async function handleClearCache() {
  if (
    await useConfirmDialog().confirm({
      header: t("settings.clearCache"),
      message: t("settings.clearCacheConfirm"),
      confirmText: t("settings.clear"),
      danger: true,
    })
  ) {
    const themePref = localStorage.getItem("encv-theme-preference");
    const serverPref = localStorage.getItem("encv-server-url");
    const webdavPref = localStorage.getItem("encv-webdav-configs");
    const localePref = localStorage.getItem("encv-locale");
    localStorage.clear();
    if (themePref) localStorage.setItem("encv-theme-preference", themePref);
    if (serverPref) localStorage.setItem("encv-server-url", serverPref);
    if (webdavPref) localStorage.setItem("encv-webdav-configs", webdavPref);
    if (localePref) localStorage.setItem("encv-locale", localePref);
    showToast({
      message: t("settings.cacheCleared"),
      duration: 1500,
      color: "success",
    });
  }
}

async function handleResetSettings() {
  if (
    await useConfirmDialog().confirm({
      header: t("settings.resetSettings"),
      message: t("settings.resetConfirm"),
      confirmText: t("settings.reset"),
      danger: true,
    })
  ) {
    localStorage.clear();
    showToast({
      message: t("settings.settingsReset"),
      duration: 1500,
      color: "success",
    });
  }
}

async function handleSaveConfig() {
  try {
    await saveConfig();
    if (restartNeeded.value) {
      showToast({
        message: t("settings.configSavedRestartNeeded"),
        duration: 4000,
        color: "warning",
      });
    } else {
      showToast({
        message: t("settings.configSaved"),
        duration: 1500,
        color: "success",
      });
    }
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({
      message: t("settings.configSaveFailed") + ": " + detail,
      duration: 3000,
      color: "danger",
    });
  }
}

function handleResetConfig() {
  resetConfig();
}

let alistFeatureRegistered = false;

function syncAlistEncryptFeature() {
  const enabled = getFieldValue(["plugin_settings", "alist_encrypt", "enabled"]) as boolean | undefined;
  if (enabled === alistFeatureRegistered) return;
  alistFeatureRegistered = !!enabled;
  if (enabled === true) {
    registerFileFeature(createAlistEncryptFeature());
  } else {
    unregisterFileFeature("alist-encrypt");
  }
}

onMounted(() => {
  checkStatus().catch(() => {});
  if (serverOnline.value) {
    loadConfig()
      .then(() => {
        configLoaded.value = true;
      })
      .catch(() => {
        configLoaded.value = true;
      });
    getIndexStats()
      .then(s => {
        indexStats.value = s;
      })
      .catch(() => {});
    loadDatabaseInfo().catch(() => {});
    if (isNative()) {
      refreshMpvPluginStatus().catch(() => {});
    }
    syncAlistEncryptFeature();
  }
  window.addEventListener("plugin-state-changed", refreshMpvPluginStatus);
});

onUnmounted(() => {
  window.removeEventListener("plugin-state-changed", refreshMpvPluginStatus);
});

// ========== 数据库管理 ==========

async function loadDatabaseInfo() {
  try {
    dbInfo.value = await getDatabaseInfo();
  } catch (e) {
    console.warn("[Settings] loadDatabaseInfo failed:", e);
  }
}

async function refreshMpvPluginStatus() {
  try {
    const state = await getPluginFullState("com.encvgo.plugin.mpv");
    console.info("[Settings] MPV plugin raw state:", JSON.stringify(state));
    mpvPluginError.value = "";

    if (state.status === "ready") {
      mpvPluginStatus.value = "ready";
      return;
    }

    if (state.status === "not_loaded" || state.status === "not_installed") {
      console.info("[Settings] MPV plugin status=${state.status}, attempting to load...");
      const loaded = await ensurePluginLoaded("com.encvgo.plugin.mpv");
      if (loaded) {
        mpvPluginStatus.value = "ready";
        console.info("[Settings] MPV plugin loaded successfully");
      } else {
        mpvPluginStatus.value = "load_failed";
        mpvPluginError.value = "插件加载失败";
        console.debug("[Settings] MPV plugin load failed");
      }
    } else {
      mpvPluginStatus.value = state.status;
      console.debug("[Settings] MPV plugin status:", state.status);
    }
  } catch (e: any) {
    console.error("[Settings] refreshMpvPluginStatus failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
    mpvPluginStatus.value = "error";
    mpvPluginError.value = e.message || "查询失败";
  }
}

watch(serverOnline, online => {
  if (online) {
    if (!configLoaded.value) {
      loadConfig()
        .then(() => {
          configLoaded.value = true;
        })
        .catch(() => {
          configLoaded.value = true;
        });
    }
    getIndexStats()
      .then(s => {
        indexStats.value = s;
      })
      .catch(() => {});
  }
});

watch(
  () => getFieldValue(["plugin_settings", "alist_encrypt", "enabled"]),
  enabled => {
    if (enabled === true) {
      registerFileFeature(createAlistEncryptFeature());
    } else {
      unregisterFileFeature("alist-encrypt");
    }
  }
);
</script>

<style scoped>
.hint-text p {
  font-size: 13px;
  color: var(--ion-text-secondary);
  margin: 0;
}
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  color: var(--encv-text-secondary);
}
.placeholder-text {
  opacity: 0.5;
  font-style: italic;
}
.scope-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 8px;
  --padding-top: 3px;
  --padding-bottom: 3px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex-shrink: 0;
}
.scope-badge-icon {
  font-size: 12px;
}
.scope-synced {
  --background: color-mix(in srgb, var(--color-primary) 12%, transparent);
  --color: var(--color-primary);
}
@media (max-width: 599px) {
  .scope-badge {
    --padding-start: 5px;
    --padding-end: 5px;
    --padding-top: 2px;
    --padding-bottom: 2px;
  }
  .scope-badge .scope-text {
    display: none;
  }
}
.port-info {
  font-size: 12px;
  opacity: 0.7;
  margin-left: 4px;
}
/* 🆕 2026-06-15：backend 进程身份展示（performPingCheck 模式复用）
   让用户/AI 能直接核对"我连的是不是同一个进程" */
.instance-info {
  margin: 4px 0 0;
  display: flex;
  align-items: center;
  gap: 6px;
}
.instance-info .instance-id {
  font-family: var(--ion-font-family-monospace, monospace);
  font-size: 11px;
  background: var(--color-base-200);
  padding: 1px 5px;
  border-radius: 3px;
  color: var(--color-primary);
}
.instance-info .version-info {
  font-size: 11px;
  opacity: 0.7;
}
.connection-error-inline {
  color: var(--color-error);
  font-size: 12px;
}
.browse-btn {
  --padding-start: 8px;
  --padding-end: 8px;
  min-width: 44px;
  min-height: 44px;
}
.json-editor-content {
  --background: var(--ion-background-color);
}
.json-editor-layout {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.json-annotations {
  border-bottom: 1px solid var(--color-base-200);
  max-height: 40%;
  overflow-y: auto;
  padding: 12px 16px;
}
.annotations-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--ion-text-color);
  margin-bottom: 8px;
}
.annotation-item {
  display: flex;
  flex-direction: column;
  padding: 4px 0;
  border-bottom: 1px solid color-mix(in srgb, var(--color-base-content) 15%, var(--color-base-100));
}
.annotation-item:last-child {
  border-bottom: none;
}
.annotation-path {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-primary);
  font-family: monospace;
}
.annotation-desc {
  font-size: 12px;
  color: var(--encv-text-secondary);
  margin-top: 2px;
}
.json-textarea-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0;
  min-height: 0;
}
.json-textarea {
  flex: 1;
  width: 100%;
  min-height: 200px;
  padding: 12px 16px;
  font-family: 'Courier New', Courier, monospace;
  font-size: 13px;
  line-height: 1.5;
  background: var(--ion-background-color);
  color: var(--ion-text-color);
  border: none;
  outline: none;
  resize: none;
  box-sizing: border-box;
}
.json-error {
  padding: 8px 16px;
  background: color-mix(in srgb, var(--color-error) 10%, transparent);
  color: var(--color-error);
  font-size: 12px;
  font-family: monospace;
}
</style>
