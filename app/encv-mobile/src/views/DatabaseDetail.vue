<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.database') }}</ion-title>
        <ion-buttons slot="end" v-if="dirty">
          <ion-button @click="handleResetConfig" color="medium">{{ t('settings.undo') }}</ion-button>
          <ion-button @click="handleSaveConfig" :disabled="configLoading">
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

      <template v-else-if="configLoaded && databaseSection">
        <!-- 当前运行状态 -->
        <ion-list>
          <ion-list-header>
            <ion-label>当前运行状态</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label>运行引擎</ion-label>
            <ion-note slot="end">
              <ion-badge :color="engineBadgeColor">
                {{ dbInfo?.engine || '—' }}
              </ion-badge>
            </ion-note>
          </ion-item>
          <ion-item>
            <ion-label>并发度</ion-label>
            <ion-note slot="end">{{ dbInfo?.concurrency || '—' }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>任务总数</ion-label>
            <ion-note slot="end">{{ dbInfo?.taskCount ?? '—' }}</ion-note>
          </ion-item>
          <ion-item v-if="dbInfo?.fallbackReason" class="engine-mismatch-item">
            <ion-label class="ion-text-wrap">
              <p class="mismatch-warning">
                <ion-icon :icon="warningOutline" class="warn-icon"></ion-icon>
                {{ dbInfo.fallbackReason }}
              </p>
            </ion-label>
          </ion-item>
          <ion-item v-else-if="configEngine !== dbInfo?.engine && dbInfo" class="engine-mismatch-item">
            <ion-label class="ion-text-wrap">
              <p class="mismatch-warning">
                <ion-icon :icon="warningOutline" class="warn-icon"></ion-icon>
                配置值「{{ configEngine }}」与当前运行引擎「{{ dbInfo.engine }}」不一致，
                重启应用后生效。
              </p>
            </ion-label>
          </ion-item>
        </ion-list>

        <!-- schema 驱动的配置字段 -->
        <ion-list>
          <ion-list-header>
            <ion-label>引擎配置</ion-label>
            <ion-badge slot="end" color="warning" class="scope-badge">
              <span class="scope-text">需重启</span>
            </ion-badge>
          </ion-list-header>

          <template v-for="child in databaseSection.properties" :key="child.key">
            <template v-if="child.key === 'engine'">
              <div class="config-field config-field-card" :class="{ 'field-modified': isEngineModified }">
                <div class="field-label-row">
                  <ion-icon :icon="getFieldIcon(child.key, child.type)" class="field-icon"></ion-icon>
                  <span class="field-label-text">
                    {{ fieldLabel(child.key, child.required) }}
                    <span v-if="child.required" class="required-mark">*</span>
                  </span>
                  <ion-button v-if="isEngineModified" fill="clear" size="small" class="reset-btn" @click="resetField(child.key, child)">
                    <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
                  </ion-button>
                </div>
                <p v-if="child.description" class="field-description-text">{{ child.description }}</p>
                <div class="preset-cards">
                  <div
                    v-for="opt in engineOptions"
                    :key="opt.value"
                    class="preset-card"
                    :class="{
                      'preset-card-active': configEngine === opt.value,
                      'preset-card-disabled': opt.disabled
                    }"
                    @click="!opt.disabled && handleFieldChange(child.key, opt.value)"
                  >
                    <div class="preset-card-title">{{ opt.label }}</div>
                    <div v-if="opt.disabled" class="preset-card-disabled-text">{{ opt.disabledReason || '暂不支持' }}</div>
                    <div v-else-if="opt.description" class="preset-card-desc">{{ opt.description }}</div>
                  </div>
                </div>
                <div v-if="hasEngineDefault" class="default-hint-row">
                  {{ t('settings.default') }}: {{ engineDefaultLabel }}
                </div>
              </div>
            </template>
            <template v-else-if="isFieldVisible(child)">
              <ConfigFieldItem
                :field="child"
                :model-value="getFieldValue(['database', child.key])"
                :label="fieldLabel(child.key, child.required)"
                :placeholder="child.description || tField(child.key)"
                :icon="getFieldIcon(child.key, child.type)"
                @update:model-value="handleFieldChange(child.key, $event)"
                @input="handleInput(child.key, child, $event)"
                @browse="handleBrowsePath(child.key, child)"
                @reset="resetField(child.key, child)"
              />
            </template>
          </template>
        </ion-list>

        <!-- 导入 / 导出 -->
        <ion-list>
          <ion-list-header>
            <ion-label>导入 / 导出</ion-label>
          </ion-list-header>
          <ion-item button @click="handleExportDatabase" :disabled="dbLoading">
            <ion-icon :icon="downloadOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>导出数据库</h3>
              <p>下载为 JSON 文件，可用于备份或迁移</p>
            </ion-label>
            <ion-spinner v-if="dbLoading" slot="end" name="crescent"></ion-spinner>
          </ion-item>
          <ion-item button @click="triggerImportFile" :disabled="dbLoading">
            <ion-icon :icon="cloudUploadOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>导入数据库</h3>
              <p>从 JSON 文件导入，将清空现有数据</p>
            </ion-label>
          </ion-item>
          <input
            ref="importFileInput"
            type="file"
            accept=".json"
            style="display: none"
            @change="handleImportFileSelected"
          />
        </ion-list>

        <!-- 本地备份 -->
        <ion-list>
          <ion-list-header>
            <ion-label>本地备份</ion-label>
          </ion-list-header>
          <ion-item button @click="handleBackupDatabase" :disabled="dbLoading">
            <ion-icon :icon="saveOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>立即备份</h3>
              <p>备份到本地 .encv-backups 目录</p>
            </ion-label>
          </ion-item>
        </ion-list>

        <!-- 跨引擎迁移说明 -->
        <ion-list>
          <ion-list-header>
            <ion-label color="danger">跨引擎迁移</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <p class="ion-text-wrap" style="font-size: 0.85em; color: var(--ion-color-medium);">
                支持 SQLite ↔ Turso ↔ LibSQL 之间的数据迁移。
                迁移步骤：先从旧引擎导出 JSON → 切换到新引擎 → 导入 JSON。
              </p>
            </ion-label>
          </ion-item>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  IonBackButton,
  IonBadge,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { cloudUploadOutline, downloadOutline, refreshOutline, save as saveIcon, saveOutline, warningOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { backupDatabase, exportDatabase, getDatabaseInfo, importDatabase } from "@/api/encv";
import ConfigFieldItem from "@/components/ConfigFieldItem.vue";
import { useConfig } from "@/composables/useConfig";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import type { FieldDef } from "@/config/schemaParser";

const { t } = useI18n();
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

const configLoaded = computed(() => !configLoading.value && schemaFields.value.length > 0);

const databaseSection = computed<FieldDef | undefined>(() => {
  return schemaFields.value.find(s => s.key === "database");
});

const configEngine = computed(() => getFieldValue(["database", "engine"]) as string);

const dbInfo = ref<any>(null);
const dbLoading = ref(false);
const importFileInput = ref<HTMLInputElement | null>(null);

const engineBadgeColor = computed(() => {
  const eng = dbInfo.value?.engine;
  if (eng === "turso" || eng === "libsql") return "success";
  if (eng === "sqlite") return "primary";
  return "medium";
});

const engineField = computed<FieldDef | undefined>(() => {
  return databaseSection.value?.properties?.find(p => p.key === "engine");
});

const engineOptions = computed(() => {
  const opts = engineField.value?.selectOptions ?? [];
  const engines = dbInfo.value?.availableEngines;
  return opts.map(opt => {
    let disabled = false;
    let disabledReason = "";
    if (engines) {
      const info = engines.find((e: any) => e.name === opt.value);
      if (info) {
        disabled = !info.available;
        disabledReason = info.reason || "";
      }
    }
    return {
      ...opt,
      disabled,
      disabledReason,
    };
  });
});

const isEngineModified = computed(() => {
  const current = configEngine.value;
  const def = engineField.value?.default;
  if (current === def) return false;
  if (current == null && (def === "" || def === 0 || def === false)) return false;
  return String(current) !== String(def);
});

const hasEngineDefault = computed(() => {
  return engineField.value?.default !== undefined;
});

const engineDefaultLabel = computed(() => {
  const def = engineField.value?.default;
  if (def === undefined) return "-";
  const opt = engineOptions.value.find(o => o.value === String(def));
  return opt ? opt.label : String(def);
});

onMounted(async () => {
  try {
    await loadConfig();
  } catch (e) {
    // config 加载失败在 Settings 主页面已经提示过了
  }
  loadDatabaseInfo().catch(() => {});
});

async function loadDatabaseInfo() {
  try {
    dbInfo.value = await getDatabaseInfo();
  } catch (e) {
    console.warn("[DatabaseDetail] loadDatabaseInfo failed:", e);
  }
}

async function handleSaveConfig() {
  try {
    await saveConfig();
    showToast({ message: t("settings.saveSuccess"), color: "success" });
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    showToast({ message: t("settings.saveFailed") + ": " + msg, color: "danger" });
  }
}

function handleResetConfig() {
  resetConfig();
}

function handleFieldChange(key: string, value: unknown) {
  setFieldValue(["database", key], value);
}

function handleInput(key: string, _field: FieldDef, value: unknown) {
  setFieldValue(["database", key], value);
}

function handleBrowsePath(key: string, _field: FieldDef) {
  // 移动端路径选择暂不实现
  console.warn("[DatabaseDetail] browse path not implemented for:", key);
}

function resetField(key: string, field: FieldDef) {
  resetFieldToDefault(["database", key], field);
}

function isFieldVisible(field: FieldDef): boolean {
  // 根据引擎类型显示/隐藏相关字段
  const engine = getFieldValue(["database", "engine"]) as string;
  if (field.key === "path") {
    return engine === "sqlite" || engine === "turso" || engine === "libsql";
  }
  if (field.key === "turso_sync_url" || field.key === "turso_auth_token") {
    return engine === "turso";
  }
  return true;
}

function fieldLabel(key: string, _required?: boolean): string {
  // 直接用字段 key 作为 label（schema 驱动）
  return key;
  // TODO: 接入 i18n
}

function tField(key: string): string {
  return key;
}

function getFieldIcon(_key: string, _type: string): string {
  // 简单映射，不需要复杂图标
  return "";
}

async function handleExportDatabase() {
  try {
    dbLoading.value = true;
    await exportDatabase();
    showToast({ message: "数据库导出成功", color: "success" });
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    showToast({ message: "导出失败: " + msg, color: "danger" });
  } finally {
    dbLoading.value = false;
  }
}

function triggerImportFile() {
  importFileInput.value?.click();
}

async function handleImportFileSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;

  const confirmed = confirm("导入将清空所有现有数据，确定要继续吗？此操作不可撤销！");
  if (!confirmed) {
    input.value = "";
    return;
  }

  try {
    dbLoading.value = true;
    const result = await importDatabase(file);
    showToast({ message: `导入成功：${result.imported.tasks} 个任务`, color: "success" });
    await loadDatabaseInfo();
    window.dispatchEvent(new CustomEvent("tasks:reload"));
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    showToast({ message: "导入失败: " + msg, color: "danger" });
  } finally {
    dbLoading.value = false;
    input.value = "";
  }
}

async function handleBackupDatabase() {
  try {
    dbLoading.value = true;
    const result = await backupDatabase();
    showToast({ message: `备份成功：${result.name}`, color: "success" });
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    showToast({ message: "备份失败: " + msg, color: "danger" });
  } finally {
    dbLoading.value = false;
  }
}
</script>

<style scoped>
.engine-mismatch-item {
  --background: var(--ion-color-warning-50, #fff8e1);
}

.mismatch-warning {
  font-size: 0.85em;
  color: var(--ion-color-warning);
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
}

.warn-icon {
  font-size: 1.1em;
  flex-shrink: 0;
}

.scope-badge {
  font-size: 0.7em;
}

.scope-text {
  font-weight: 500;
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px;
  gap: 12px;
  color: var(--ion-color-medium);
}

.config-field-card {
  display: block;
  padding: 12px 16px;
  border-bottom: 1px solid var(--ion-color-light-shade, #e0e0e0);
  background: transparent;
}

body.dark .config-field-card {
  border-bottom-color: #2a2a2c;
}

.config-field-card.field-modified {
  border-left: 3px solid var(--ion-color-primary);
  padding-left: 13px;
}

.field-icon {
  font-size: 18px;
  color: var(--ion-color-medium);
  flex-shrink: 0;
}

.field-label-row {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.field-label-text {
  flex: 1 1 auto;
  min-width: 0;
}

.field-description-text {
  font-size: 12px;
  color: var(--ion-color-medium);
  white-space: normal;
  margin: 2px 0 0;
}

.default-hint-row {
  font-size: 11px;
  color: var(--ion-color-medium);
  margin-top: 6px;
}

.required-mark {
  color: var(--ion-color-danger);
  margin-left: 2px;
}

.reset-btn {
  --padding-start: 4px;
  --padding-end: 4px;
  min-width: 28px;
  min-height: 28px;
  margin: 0;
}

.reset-btn ion-icon {
  font-size: 16px;
}

.preset-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(80px, 1fr));
  gap: 8px;
  margin-top: 10px;
  width: 100%;
}

.preset-card {
  padding: 10px 8px;
  border: 2px solid var(--ion-color-light-shade, #e0e0e0);
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: center;
  background: var(--ion-background-color, transparent);
}

.preset-card-active {
  border-color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}

.preset-card-disabled {
  opacity: 0.4;
  cursor: not-allowed;
  filter: grayscale(0.5);
}

.preset-card-disabled-text {
  font-size: 10px;
  color: var(--ion-color-medium);
  margin-top: 2px;
}

.preset-card-title {
  font-size: 13px;
  font-weight: 500;
}

.preset-card-desc {
  font-size: 10px;
  color: var(--ion-color-medium);
  margin-top: 2px;
}
</style>
