<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/server"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.httpServer') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content :fullscreen="true" class="ion-padding">
      <div class="settings-container">
        <ion-list v-if="childFields.length > 0" :inset="true">
          <ion-list-header><ion-label>{{ t('settings.httpServerSettings') }}</ion-label></ion-list-header>
          <template v-for="field in childFields" :key="field.key">
            <ConfigFieldItem
              :field="field"
              :model-value="getFieldValue(['server', field.key])"
              :label="fieldLabel(field.key, field.required)"
              :placeholder="field.description || tField(field.key)"
              :icon="getFieldIcon(field.key, field.type)"
              @update:model-value="setValue(['server', field.key], $event)"
              @input="handleInput(['server', field.key], field, $event)"
              @browse="handleBrowsePath(['server', field.key], field)"
            />
          </template>
        </ion-list>
        <ion-button expand="block" color="primary" :disabled="!dirty || loading" @click="handleSave">
          {{ t('settings.saveConfig') }}
        </ion-button>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import FilePickerModal from "@encv/shared-components/components/FilePickerModal.vue";
import { useConfig } from "@encv/shared-components/composables/useConfig";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";
import type { FieldDef } from "@encv/shared-components/config/schemaParser";
import { parseSchema } from "@encv/shared-components/config/schemaParser";
import { modalController } from "@ionic/vue";
import { cloudOutline, folderOpen, settingsOutline, speedometerOutline } from "ionicons/icons";
import { computed } from "vue";

const { t } = useI18n();
const { getFieldValue, setFieldValue, dirty, loading, saveConfig } = useConfig();

const SECTION_KEY = "server";

const sectionDef = computed(() => parseSchema().find(s => s.key === SECTION_KEY));
const _childFields = computed(() => sectionDef.value?.properties ?? []);

function tField(key: string): string {
  return t(`settings.${key}`);
}

function _fieldLabel(key: string, required?: boolean): string {
  return tField(key) + (required ? " *" : "");
}

const fieldIconMap: Record<string, string> = {
  port: speedometerOutline,
  dir: folderOpen,
};

function _getFieldIcon(fieldKey: string, fieldType: string): string {
  if (fieldIconMap[fieldKey]) return fieldIconMap[fieldKey];
  if (fieldType === "boolean") return settingsOutline;
  if (fieldType === "integer") return speedometerOutline;
  if (fieldKey.includes("password")) return cloudOutline;
  return settingsOutline;
}

function _setValue(path: string[], value: unknown) {
  setFieldValue(path, value);
}

function _handleInput(path: string[], _field: FieldDef, event: CustomEvent) {
  const val = (event.target as HTMLInputElement).value;
  if (_field.type === "integer") {
    setFieldValue(path, val ? Number(val) : 0);
  } else {
    setFieldValue(path, val);
  }
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
</script>
