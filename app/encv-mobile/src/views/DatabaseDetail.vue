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
        </ion-list>

        <!-- 可用引擎列表 -->
        <ion-list>
          <ion-list-header>
            <ion-label>数据库引擎</ion-label>
            <ion-badge slot="end" color="warning" class="scope-badge">
              <span class="scope-text">需重启</span>
            </ion-badge>
          </ion-list-header>

          <ion-item
            v-for="engine in availableEngines"
            :key="engine.name"
            class="engine-item"
            :class="{ 'engine-item-base': engine.is_base }"
          >
            <div class="engine-item-content">
              <div class="engine-header">
                <div class="engine-title-row">
                  <span class="engine-title">{{ engine.label }}</span>
                  <ion-badge v-if="engine.is_base" class="base-badge" color="primary">底座</ion-badge>
                  <ion-badge v-else-if="engine.enabled" class="status-badge" color="success">已启用</ion-badge>
                  <ion-badge v-else-if="engine.available" class="status-badge" color="medium">未启用</ion-badge>
                  <ion-badge v-else class="status-badge" color="danger">不可用</ion-badge>
                </div>
                <p v-if="engine.description" class="engine-desc">{{ engine.description }}</p>
              </div>

              <!-- 能力标签 -->
              <div v-if="engine.capabilities?.length" class="capability-tags">
                <span
                  v-for="cap in engine.capabilities"
                  :key="cap"
                  class="cap-tag"
                >{{ cap }}</span>
              </div>

              <!-- 开关 / 状态提示 -->
              <div class="engine-action-row">
                <template v-if="engine.is_base">
                  <span class="base-hint">默认底座，始终启用，不可切换</span>
                </template>
                <template v-else-if="!engine.available">
                  <span class="unavailable-hint">{{ engine.reason || '暂不支持' }}</span>
                </template>
                <template v-else>
                  <ion-toggle
                    :checked="isEngineEnabled(engine.name)"
                    @ion-change="toggleEngine(engine.name, $event)"
                    :disabled="!engine.available"
                  ></ion-toggle>
                </template>
              </div>
            </div>
          </ion-item>
        </ion-list>

        <!-- 其他配置字段（path / turso 相关） -->
        <ion-list>
          <ion-list-header>
            <ion-label>引擎配置</ion-label>
          </ion-list-header>

          <template v-for="child in databaseSection.properties" :key="child.key">
            <template v-if="child.key !== 'engine' && child.key !== 'enable_engines' && isFieldVisible(child)">
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
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { alertController } from "@ionic/vue";
import { cloudUploadOutline, downloadOutline, saveOutline as saveIcon, saveOutline, warningOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { backupDatabase, exportDatabase, getDatabaseInfo, importDatabase } from "@/api/encv";
import ConfigFieldItem from "@/components/ConfigFieldItem.vue";
import { useConfig } from "@/composables/useConfig";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import type { FieldDef } from "@/config/schemaParser";
import { restartBackend } from "@/plugins/GoProcess";

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

const dbInfo = ref<any>(null);
const dbLoading = ref(false);
const importFileInput = ref<HTMLInputElement | null>(null);

const engineBadgeColor = computed(() => {
  const eng = dbInfo.value?.engine;
  if (eng === "turso" || eng === "libsql") return "success";
  if (eng === "sqlite") return "primary";
  if (eng === "objectbox") return "tertiary";
  return "medium";
});

const availableEngines = computed(() => {
  return dbInfo.value?.availableEngines || [];
});

function isEngineEnabled(name: string): boolean {
  const enableMap = getFieldValue(["database", "enable_engines"]) as Record<string, boolean> | undefined;
  if (name === "sqlite") return true;
  return enableMap?.[name] ?? false;
}

function toggleEngine(name: string, event: any) {
  const checked = event.detail.checked;
  const current = (getFieldValue(["database", "enable_engines"]) as Record<string, boolean> | undefined) || {};
  const next = { ...current, [name]: checked };
  setFieldValue(["database", "enable_engines"], next);
}

onMounted(async () => {
  try {
    await loadConfig();
  } catch (_e) {
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
    const before = getFieldValue(["database", "enable_engines"]);
    const beforeEngine = getFieldValue(["database", "engine"]);
    await saveConfig();
    showToast({ message: t("settings.saveSuccess"), color: "success" });

    const after = getFieldValue(["database", "enable_engines"]);
    const afterEngine = getFieldValue(["database", "engine"]);
    const enginesChanged = JSON.stringify(before) !== JSON.stringify(after);
    const engineChanged = beforeEngine !== afterEngine;

    if (enginesChanged || engineChanged) {
      await askRestart();
    }
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    showToast({ message: t("settings.saveFailed") + ": " + msg, color: "danger" });
  }
}

async function askRestart() {
  const alert = await alertController.create({
    header: "需要重启生效",
    message: "数据库引擎配置已修改，需要重启后端才能生效。是否立即重启？",
    buttons: [
      {
        text: "稍后再说",
        role: "cancel",
      },
      {
        text: "立即重启",
        handler: async () => {
          showToast({ message: "正在重启后端...", color: "primary" });
          try {
            await restartBackend();
            showToast({ message: "后端重启成功", color: "success" });
            loadDatabaseInfo().catch(() => {});
          } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            showToast({ message: "重启失败: " + msg, color: "danger" });
          }
        },
      },
    ],
  });
  await alert.present();
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

.engine-item {
  --inner-padding-end: 0;
}

.engine-item-content {
  width: 100%;
  padding: 12px 0;
}

.engine-header {
  margin-bottom: 8px;
}

.engine-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.engine-title {
  font-size: 15px;
  font-weight: 600;
}

.base-badge {
  font-size: 0.65em;
  --padding-start: 6px;
  --padding-end: 6px;
}

.status-badge {
  font-size: 0.65em;
  --padding-start: 6px;
  --padding-end: 6px;
}

.engine-desc {
  font-size: 12px;
  color: var(--ion-color-medium);
  margin: 4px 0 0;
  line-height: 1.4;
}

.capability-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin: 8px 0;
}

.cap-tag {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--ion-color-light, #f4f5f8);
  color: var(--ion-color-medium, #92949c);
  white-space: nowrap;
}

body.dark .cap-tag {
  background: #2a2a2c;
  color: #92949c;
}

.engine-action-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  margin-top: 8px;
}

.base-hint {
  font-size: 12px;
  color: var(--ion-color-medium);
}

.unavailable-hint {
  font-size: 12px;
  color: var(--ion-color-danger);
}

.engine-item-base {
  --background: var(--ion-color-primary-50, rgba(79, 140, 255, 0.08));
}
</style>
