<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.pluginSettings') }}</ion-title>
        <ion-buttons slot="end" v-if="dirty">
          <ion-button @click="handleResetConfig" color="medium">{{ t('settings.undo') }}</ion-button>
          <ion-button @click="handleSaveConfig" :disabled="configLoading || suffixConflict.length > 0 || !!textExtsError">
            <ion-icon :icon="saveIcon" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="configLoading && !configLoaded" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('settings.loadingConfig') }}</p>
      </div>

      <template v-else-if="configLoaded && pluginSection">
        <ion-list>
          <ion-list-header>
            <ion-label>{{ pluginSection.sectionTitle ? tSectionTitle(pluginSection.sectionTitle) : tField(pluginSection.key) }}</ion-label>
            <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
              <ion-icon :icon="cloudOutline" class="scope-badge-icon"></ion-icon>
              <span class="scope-text">{{ t('settings.synced') }}</span>
            </ion-badge>
          </ion-list-header>

          <template v-for="child in pluginSection.properties" :key="child.key">
            <template v-if="child.type === 'object' && child.properties && !child.isMap">
              <ion-item-divider>
                <ion-label>{{ tField(child.key) }}</ion-label>
              </ion-item-divider>
              <template v-for="grandchild in child.properties" :key="grandchild.key">
                <template v-if="isFieldVisible(grandchild)">
                  <template v-if="child.key === 'text' && grandchild.key === 'custom_text_extensions'">
                    <ion-item>
                      <ion-icon :icon="textOutline" slot="start"></ion-icon>
                      <ion-input
                        :value="String(getValue([pluginSection.key, child.key, grandchild.key]) ?? '')"
                        :label="tField(grandchild.key)"
                        label-placement="stacked"
                        :placeholder="t('settings.customTextExtsHint')"
                        @ionInput="handleCustomTextExtsInput($event)"
                      ></ion-input>
                      <ion-icon :icon="cloudOutline" slot="end" class="sync-indicator"></ion-icon>
                    </ion-item>
                    <ion-item v-if="textExtsError" lines="none">
                      <ion-label class="ion-text-wrap error-text">
                        <p>{{ textExtsError }}</p>
                      </ion-label>
                    </ion-item>
                    <ion-item v-if="textExtsConflicts.length > 0" lines="none">
                      <ion-label class="ion-text-wrap conflict-text">
                        <p>{{ t('settings.textExtsConflictWarning', { extensions: textExtsConflicts.join(', ') }) }}</p>
                      </ion-label>
                    </ion-item>
                    <ion-item v-if="builtInTextExtsCount > 0" lines="none">
                      <ion-label class="ion-text-wrap hint-text">
                        <p>{{ t('settings.builtInTextExts', { count: String(builtInTextExtsCount) }) }}</p>
                      </ion-label>
                    </ion-item>
                  </template>
                  <ConfigFieldItem
                    v-else
                    :field="grandchild"
                    :model-value="getValue([pluginSection.key, child.key, grandchild.key])"
                    :label="fieldLabel(grandchild.key, grandchild.required)"
                    :placeholder="grandchild.description || tField(grandchild.key)"
                    :icon="getFieldIcon(grandchild.key, grandchild.type)"
                    @update:model-value="setValue([pluginSection.key, child.key, grandchild.key], $event)"
                    @input="handleInput([pluginSection.key, child.key, grandchild.key], grandchild, $event)"
                    @browse="handleBrowsePath([pluginSection.key, child.key, grandchild.key], grandchild)"
                    @reset="resetFieldToDefault([pluginSection.key, child.key, grandchild.key], grandchild)"
                  />
                </template>
              </template>
            </template>

            <template v-else-if="child.isMap">
              <ion-item-divider>
                <ion-label>{{ tField(child.key) }}</ion-label>
              </ion-item-divider>
              <template v-if="getMapEntries([pluginSection.key, child.key]).length > 0">
                <ion-item v-for="[entryKey, entryVal] in getMapEntries([pluginSection.key, child.key])" :key="entryKey">
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
                :model-value="getValue([pluginSection.key, child.key])"
                :label="fieldLabel(child.key, child.required)"
                :placeholder="child.description || tField(child.key)"
                :icon="getFieldIcon(child.key, child.type)"
                @update:model-value="setValue([pluginSection.key, child.key], $event)"
                @input="handleInput([pluginSection.key, child.key], child, $event)"
                @browse="handleBrowsePath([pluginSection.key, child.key], child)"
                @reset="resetFieldToDefault([pluginSection.key, child.key], child)"
              />
            </template>
          </template>

          <ion-item v-if="!pluginSection.properties || pluginSection.properties.length === 0">
            <ion-label class="ion-text-wrap placeholder-text">
              <p>{{ t('settings.noEntries') }}</p>
            </ion-label>
          </ion-item>
        </ion-list>

        <div v-if="suffixConflict.length > 0" class="suffix-conflict-warning" :class="{ 'api-unavailable': suffixConflict.includes(UNAVAILABLE) }">
          <ion-icon :icon="warningOutline"></ion-icon>
          <span v-if="suffixConflict.includes(UNAVAILABLE)">{{ t('settings.suffixCheckUnavailable') }}</span>
          <span v-else>{{ t('settings.suffixConflictWarning', { suffix: String(getFieldValue(['plugin_settings', 'alist_encrypt', 'suffix']) ?? ''), plugins: suffixConflict.join(', ') }) }}</span>
        </div>

        <ion-list v-if="isNative()">
        </ion-list>
      </template>

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
import { fetchConfig, fetchTextPreviewExts, invalidateTextExtsCache, updateConfig } from "@encv/shared-components/api/encv";
import FilePickerModal from "@encv/shared-components/components/FilePickerModal.vue";
import { useConfig } from "@encv/shared-components/composables/useConfig";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { usePluginExtensions } from "@encv/shared-components/composables/usePluginExtensions";
import { useServerStatus } from "@encv/shared-components/composables/useServerStatus";
import { showToast } from "@encv/shared-components/composables/useToast";
import type { FieldDef } from "@encv/shared-components/config/schemaParser";
import { isNative } from "@encv/shared-components/plugins/GoProcess";
import { modalController } from "@ionic/vue";
import {
  colorPaletteOutline,
  documentText,
  eyeOutline,
  filmOutline,
  folderOpen,
  imagesOutline,
  layersOutline,
  lockClosed,
  musicalNotesOutline,
  newspaperOutline,
  readerOutline,
  settingsOutline,
  shieldCheckmark,
  speedometerOutline,
  textOutline,
  toggleOutline,
} from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";

const { isOnline: serverOnline } = useServerStatus();
const {
  schemaFields,
  loading: configLoading,
  dirty,
  loadConfig,
  saveConfig,
  resetConfig,
  getFieldValue,
  setFieldValue,
  resetFieldToDefault,
} = useConfig();
const { getConflictingPlugins, load: loadExtensions, UNAVAILABLE, data: pluginExtData } = usePluginExtensions();
const { t, tField, tSectionTitle } = useI18n();

const configLoaded = ref(false);
const suffixConflict = ref<string[]>([]);
const textExtsError = ref("");
const textExtsConflicts = ref<string[]>([]);
const builtInTextExtsCount = ref(0);

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

async function _openJsonEditor() {
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

function _validateJson() {
  try {
    JSON.parse(jsonText.value);
    jsonError.value = "";
  } catch (e) {
    jsonError.value = e instanceof Error ? e.message : String(e);
  }
}

async function _handleSaveJson() {
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

const _pluginSection = computed<FieldDef | undefined>(() => {
  return schemaFields.value.find(s => s.key === "plugin_settings");
});

function getValue(path: string[]): unknown {
  return getFieldValue(path);
}

function setValue(path: string[], value: unknown) {
  setFieldValue(path, value);
}

function _getMapEntries(path: string[]): [string, Record<string, unknown>][] {
  const val = getFieldValue(path);
  if (!val || typeof val !== "object") return [];
  return Object.entries(val as Record<string, unknown>) as [string, Record<string, unknown>][];
}

function _handleInput(path: string[], field: FieldDef, event: CustomEvent) {
  const val = (event.target as HTMLInputElement).value;
  if (field.type === "integer") {
    setFieldValue(path, val ? Number(val) : 0);
  } else {
    setFieldValue(path, val);
  }

  if (path.length === 3 && path[0] === "plugin_settings" && path[1] === "alist_encrypt" && path[2] === "suffix") {
    checkSuffixConflict(val);
  }
}

function checkSuffixConflict(suffix: string) {
  if (!suffix || suffix === ".") {
    suffixConflict.value = [];
    return;
  }
  const conflicts = getConflictingPlugins(suffix);
  suffixConflict.value = conflicts;
}

const TEXT_EXT_PATTERN = /^[a-z0-9](?:[a-z0-9\-.]*[a-z0-9])?(?:\s*,\s*[a-z0-9](?:[a-z0-9\-.]*[a-z0-9])?)*$/;

function parseAndValidateTextExts(raw: string): { valid: boolean; error: string; extensions: string[] } {
  const trimmed = raw.trim();
  if (!trimmed) return { valid: true, error: "", extensions: [] };

  if (!TEXT_EXT_PATTERN.test(trimmed)) {
    return { valid: false, error: t("settings.textExtsFormatError"), extensions: [] };
  }

  const extensions = trimmed
    .split(",")
    .map(s => s.trim().toLowerCase())
    .filter(s => s.length > 0);
  const dupes = extensions.filter((ext, i) => extensions.indexOf(ext) !== i);
  if (dupes.length > 0) {
    return { valid: false, error: t("settings.textExtsDuplicateError", { ext: dupes[0] }), extensions: [] };
  }

  return { valid: true, error: "", extensions };
}

function checkTextExtsConflicts(extensions: string[]): string[] {
  if (!extensions.length || !pluginExtData.value) return [];
  const allExts = Object.keys(pluginExtData.value.extensions);
  const conflicts = extensions.filter(ext => allExts.includes("." + ext.toLowerCase()));
  return conflicts;
}

async function _handleCustomTextExtsInput(event: CustomEvent) {
  const raw = (event.target as HTMLInputElement).value || "";
  setValue(["plugin_settings", "text", "custom_text_extensions"], raw);

  const result = parseAndValidateTextExts(raw);
  textExtsError.value = result.error;

  if (result.valid && result.extensions.length > 0) {
    textExtsConflicts.value = checkTextExtsConflicts(result.extensions);
  } else {
    textExtsConflicts.value = [];
  }
}

async function _handleBrowsePath(path: string[], field: FieldDef) {
  const isFolder = field.key !== "file";
  const currentVal = String(getFieldValue(path) || "/");
  const modal = await modalController.create({
    component: FilePickerModal,
    componentProps: {
      mode: isFolder ? "folder" : "file",
      initialPath: currentVal,
    },
  });
  await modal.present();
  const { data, role } = await modal.onDidDismiss();
  if (role === "select" && data) {
    setFieldValue(path, data.path);
  }
}

function _fieldLabel(key: string, _required?: boolean): string {
  return tField(key);
}

const fieldIconMap: Record<string, string> = {
  plugin_cache_dir: folderOpen,
  ext: documentText,
  container_chunk_size_mb: filmOutline,
  light_container_main_chunk_enabled: layersOutline,
  track_extensions: eyeOutline,
  keep_mkv_for_mkv_source: filmOutline,
  verify_after_pack: shieldCheckmark,
  skip_merge_for_split_mkv: filmOutline,
  allow_no_reencode: speedometerOutline,
  default_stream_preset: colorPaletteOutline,
  video: filmOutline,
  audio: musicalNotesOutline,
  image: imagesOutline,
  wps: readerOutline,
  pdf: newspaperOutline,
  text: textOutline,
  disable_signature_verification: shieldCheckmark,
};

function _getFieldIcon(fieldKey: string, fieldType: string): string {
  if (fieldIconMap[fieldKey]) return fieldIconMap[fieldKey];
  if (fieldType === "boolean") return toggleOutline;
  if (fieldType === "integer") return speedometerOutline;
  if (fieldKey.includes("password")) return lockClosed;
  return settingsOutline;
}

function _isFieldVisible(field: FieldDef): boolean {
  if (!field.platform || field.platform === "both") return true;
  if (field.platform === "mobile") return isNative();
  if (field.platform === "desktop") return !isNative();
  return true;
}

async function _handleSaveConfig() {
  try {
    const textExtsVal = String(getValue(["plugin_settings", "text", "custom_text_extensions"]) ?? "");
    if (textExtsVal) {
      const parsed = parseAndValidateTextExts(textExtsVal);
      if (!parsed.valid) return;
      const cfg = await fetchConfig();
      if (!cfg.preview) cfg.preview = {};
      (cfg.preview as Record<string, unknown>).text_extensions = parsed.extensions;
      await updateConfig(cfg);
      invalidateTextExtsCache();
    }
    await saveConfig();
    showToast({ message: t("settings.configSaved"), duration: 1500, color: "success" });
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: t("settings.configSaveFailed") + ": " + detail, duration: 3000, color: "danger" });
  }
}

function _handleResetConfig() {
  resetConfig();
}

onMounted(async () => {
  if (serverOnline.value) {
    try {
      await loadConfig();
      configLoaded.value = true;
    } catch (e) {
      // loadConfig 现在会抛错（不再静默回退到 schema 默认值，
      // 避免"后端挂了"伪装成"已加载、配置全空"的状态机混乱）。
      // PluginSettings 没有像 AgentSettingsDetail 那样专门的错误态 UI，
      // 这里用 toast 提示 + 静默回退到 configLoaded=true 显示空字段，
      // 与 Settings.vue 的降级策略保持一致。
      console.error("[PluginSettings] loadConfig failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
      configLoaded.value = true;
    }
    try {
      await loadExtensions();
    } catch {}
    try {
      const exts = await fetchTextPreviewExts();
      builtInTextExtsCount.value = exts.size;
    } catch {}
  }
});

watch(serverOnline, async online => {
  if (online && !configLoaded.value) {
    try {
      await loadConfig();
      configLoaded.value = true;
    } catch (e) {
      console.error("[PluginSettings] watch loadConfig failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
      configLoaded.value = true;
    }
  }
});
</script>

<style scoped>
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
.browse-btn {
  --padding-start: 8px;
  --padding-end: 8px;
  min-width: 44px;
  min-height: 44px;
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
}
.scope-badge-icon {
  font-size: 12px;
}
.scope-synced {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  --color: var(--ion-color-primary);
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
.sync-indicator {
  font-size: 14px;
  color: var(--ion-color-primary);
  opacity: 0.4;
  margin-left: 4px;
}
.config-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
  margin-left: 6px;
}
.badge-server { background: #3880ff; color: white; }
.badge-mobile { background: #8c61ff; color: white; }
.badge-v4 { background: #2dd36f; color: white; }
.preset-cards {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.preset-card {
  flex: 1;
  padding: 12px;
  border: 2px solid #e0e0e0;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
}
.preset-card-active {
  border-color: #3880ff;
  background: rgba(56, 128, 255, 0.08);
}
.preset-card-title {
  font-weight: 600;
  font-size: 14px;
}
.preset-card-desc {
  font-size: 12px;
  color: #666;
  margin-top: 4px;
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
  border-bottom: 1px solid var(--ion-color-light);
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
  border-bottom: 1px solid rgba(var(--ion-color-medium-rgb), 0.15);
}
.annotation-item:last-child {
  border-bottom: none;
}
.annotation-path {
  font-size: 12px;
  font-weight: 600;
  color: var(--ion-color-primary);
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
  background: rgba(var(--ion-color-danger-rgb), 0.1);
  color: var(--ion-color-danger);
  font-size: 12px;
  font-family: monospace;
}
.suffix-conflict-warning {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 16px;
  padding: 10px 14px;
  background: rgba(255, 152, 0, 0.1);
  border-radius: 8px;
  border-left: 3px solid #e65100;
  color: #e65100;
  font-size: 13px;
}
.suffix-conflict-warning.api-unavailable {
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  border-left-color: var(--ion-color-danger);
  color: var(--ion-color-danger);
}
.suffix-conflict-warning ion-icon {
  font-size: 20px;
  flex-shrink: 0;
}
.error-text p {
  font-size: 13px;
  color: var(--ion-color-danger);
  margin: 0;
}
.conflict-text p {
  font-size: 13px;
  color: #e65100;
  margin: 0;
}
.hint-text p {
  font-size: 13px;
  color: var(--ion-text-secondary);
  margin: 0;
}
</style>
