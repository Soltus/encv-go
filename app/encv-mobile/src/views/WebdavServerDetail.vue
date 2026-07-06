<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/server"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.webdavServer') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content :fullscreen="true" class="ion-padding">
      <div class="settings-container">
        <ion-list v-if="childFields.length > 0" :inset="true">
          <ion-list-header><ion-label>{{ t('settings.webdavServerSettings') }}</ion-label></ion-list-header>
          <template v-for="field in childFields" :key="field.key">
            <ConfigFieldItem
              :field="field"
              :model-value="getFieldValue(['webdav', field.key])"
              :label="fieldLabel(field.key, field.required)"
              :placeholder="field.description || tField(field.key)"
              :icon="getFieldIcon(field.key, field.type)"
              @update:model-value="setValue(['webdav', field.key], $event)"
              @input="handleInput(['webdav'], field, $event)"
              @browse="handleBrowsePath(['webdav', field.key], field)"
            />
          </template>
        </ion-list>

        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t('settings.webdavTest') }}</ion-label></ion-list-header>
          <ion-item>
            <ion-icon :icon="globeOutline" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('settings.webdavTestConnection') }}</h3>
              <p>{{ t('settings.webdavTestConnectionDesc') }}</p>
            </ion-label>
            <ion-button slot="end" fill="outline" size="small" @click="handleTestWebdav" :disabled="webdavTesting">
              <ion-spinner v-if="webdavTesting" slot="icon-only" name="crescent"></ion-spinner>
              <span v-else>{{ t('settings.test') }}</span>
            </ion-button>
          </ion-item>
        </ion-list>

        <ion-button expand="block" color="primary" :disabled="!dirty || loading" @click="handleSave">
          {{ t('settings.saveConfig') }}
        </ion-button>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { testLocalWebDAV } from "@/api/encv";
import FilePickerModal from "@/components/FilePickerModal.vue";
import { useConfig } from "@/composables/useConfig";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import type { FieldDef } from "@/config/schemaParser";
import { parseSchema } from "@/config/schemaParser";
import { modalController } from "@ionic/vue";
import { documentText, folderOpen, globeOutline, lockClosed, personOutline, settingsOutline } from "ionicons/icons";
import { computed, ref } from "vue";

const { t } = useI18n();
const { getFieldValue, setFieldValue, dirty, loading, saveConfig } = useConfig();

const SECTION_KEY = "webdav";

const sectionDef = computed(() => parseSchema().find(s => s.key === SECTION_KEY));
const _childFields = computed(() => sectionDef.value?.properties ?? []);
const webdavTesting = ref(false);

function tField(key: string): string {
  return t(`settings.${key}`);
}

function _fieldLabel(key: string, required?: boolean): string {
  return tField(key) + (required ? " *" : "");
}

const fieldIconMap: Record<string, string> = {
  root: documentText,
  dir: folderOpen,
  username: personOutline,
  password: lockClosed,
};

function _getFieldIcon(fieldKey: string, fieldType: string): string {
  if (fieldIconMap[fieldKey]) return fieldIconMap[fieldKey];
  if (fieldType === "boolean") return settingsOutline;
  if (fieldType === "integer") return globeOutline;
  if (fieldKey.includes("password")) return lockClosed;
  return globeOutline;
}

function _setValue(path: string[], value: unknown) {
  setFieldValue(path, value);
}

function _handleInput(path: string[], _field: FieldDef, event: CustomEvent) {
  const val = (event.target as HTMLInputElement).value;
  if (path.length >= 2 && path[1] === "root" && val) {
    const err = validateWebdavRoute(val);
    if (err) {
      showToast({ message: err, duration: 3000, color: "danger" });
      return;
    }
  }
  if (_field.type === "integer") {
    setFieldValue(path, val ? Number(val) : 0);
  } else {
    setFieldValue(path, val);
  }
}

function validateWebdavRoute(val: string): string | null {
  if (!val.startsWith("/")) return t("settings.webdavRootMustStartSlash");
  const invalidChars = /[^\w\-./]/;
  if (invalidChars.test(val)) return t("settings.webdavRootInvalidChars");
  return null;
}

async function _handleBrowsePath(path: string[], field: FieldDef) {
  const isFolder = field.key !== "file";
  const currentVal = String(getFieldValue(path) || "/");
  const modal = await modalController.create({
    component: FilePickerModal,
    componentProps: {
      initialPath: currentVal,
      mode: isFolder ? "folder" : "file",
      title: `Select ${isFolder ? "Directory" : "File"}`,
    },
  });
  await modal.present();
  const result = await modal.onDidDismiss<string>();
  if (result.data && result.data !== currentVal) {
    setFieldValue(path, result.data);
  }
}

async function _handleSave() {
  try {
    await saveConfig();
    showToast({ message: t("settings.configSaved"), duration: 1500, color: "success" });
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: t("settings.configSaveFailed") + ": " + detail, duration: 3000, color: "danger" });
  }
}

async function _handleTestWebdav() {
  webdavTesting.value = true;
  try {
    const result = await testLocalWebDAV();
    if (!result.available) {
      showToast({
        message: t("settings.webdavTestFailed") + ": " + (result.error || "WebDAV not enabled"),
        duration: 4000,
        color: "danger",
      });
    } else {
      showToast({
        message: t("settings.webdavTestSuccess") + ` (${result.url})`,
        duration: 3000,
        color: "success",
      });
    }
  } catch (e) {
    showToast({
      message: t("settings.webdavTestFailed") + ": " + (e instanceof Error ? e.message : String(e)),
      duration: 4000,
      color: "danger",
    });
  } finally {
    webdavTesting.value = false;
  }
}
</script>
