<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/settings" />
        </ion-buttons>
        <ion-title>{{ t("simverse.saveManagement") }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading" class="loading-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>

      <template v-else>
        <ion-list :inset="true">
          <ion-list-header>
            <ion-label>{{ t("simverse.currentSave") }}</ion-label>
          </ion-list-header>

          <ion-item v-if="!saveInfo?.has_save" lines="none">
            <ion-icon :icon="saveOutline" slot="start" color="medium" />
            <ion-label class="ion-text-wrap">
              <h3>{{ t("simverse.noSave") }}</h3>
              <p>{{ t("simverse.noSaveDesc") }}</p>
            </ion-label>
          </ion-item>

          <template v-else>
            <ion-item lines="none">
              <ion-icon :icon="saveOutline" slot="start" color="primary" />
              <ion-label>
                <h3>{{ t("simverse.savedAt") }}: {{ formatDate(saveInfo.saved_at) }}</h3>
                <p>Tick {{ saveInfo.tick }} · {{ saveInfo.npc_count }} {{ t("simverse.npcs") }}</p>
              </ion-label>
              <ion-note slot="end">{{ formatSize(saveInfo.size_bytes) }}</ion-note>
            </ion-item>
          </template>
        </ion-list>

        <ion-list :inset="true">
          <ion-list-header>
            <ion-label>{{ t("simverse.actions") }}</ion-label>
          </ion-list-header>

          <ion-item button @click="doSave" :disabled="saving">
            <ion-icon :icon="cloudUploadOutline" slot="start" color="primary" />
            <ion-label>
              <h3>{{ t("simverse.saveNow") }}</h3>
              <p v-if="saving">{{ t("settings.checking") }}...</p>
            </ion-label>
            <ion-spinner v-if="saving" slot="end" name="crescent" size="small" />
          </ion-item>

          <ion-item button @click="doLoad" :disabled="!saveInfo?.has_save || loading">
            <ion-icon :icon="cloudDownloadOutline" slot="start" color="success" />
            <ion-label>
              <h3>{{ t("simverse.loadSave") }}</h3>
              <p>{{ t("simverse.loadSaveDesc") }}</p>
            </ion-label>
          </ion-item>

          <ion-item button @click="confirmDelete" :disabled="!saveInfo?.has_save" class="danger-item">
            <ion-icon :icon="trashOutline" slot="start" color="danger" />
            <ion-label>
              <h3 class="danger-text">{{ t("simverse.deleteSave") }}</h3>
              <p>{{ t("simverse.deleteSaveDesc") }}</p>
            </ion-label>
          </ion-item>
        </ion-list>

        <ion-list :inset="true" v-if="storage">
          <ion-list-header>
            <ion-label>{{ t("simverse.storage") }}</ion-label>
          </ion-list-header>
          <ion-item lines="none">
            <ion-label>
              <h3>{{ formatSize(storage.used_bytes) }} / {{ formatSize(storage.total_bytes) }}</h3>
              <p>{{ t("simverse.available") }}: {{ formatSize(storage.available_bytes) }}</p>
            </ion-label>
          </ion-item>
          <ion-item lines="none">
            <ion-progress-bar :value="storage.used_bytes / storage.total_bytes" color="primary" />
          </ion-item>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
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
  IonProgressBar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { useConfirmDialog } from "@encv/shared-components/composables/useConfirmDialog";
import { cloudDownloadOutline, cloudUploadOutline, saveOutline, trashOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { type SimverseSaveInfo, type SimverseStorageStatus, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadSaveInfo, saveWorld, loadWorld, loadStorageStatus } = useSimverse();

const loading = ref(false);
const saving = ref(false);
const saveInfo = ref<SimverseSaveInfo | null>(null);
const storage = ref<SimverseStorageStatus | null>(null);

async function loadData() {
  loading.value = true;
  try {
    const [info, st] = await Promise.all([loadSaveInfo(), loadStorageStatus().catch(() => null)]);
    saveInfo.value = info;
    storage.value = st;
  } finally {
    loading.value = false;
  }
}

async function doSave() {
  saving.value = true;
  try {
    await saveWorld();
    await loadData();
    await useConfirmDialog().showAlert({
      header: t("simverse.saveSuccess"),
      message: t("simverse.saveSuccessDesc"),
      okText: "OK",
    });
  } catch (e: any) {
    await useConfirmDialog().showAlert({
      header: t("errors.error"),
      message: e.message || "Save failed",
      okText: "OK",
    });
  } finally {
    saving.value = false;
  }
}

async function doLoad() {
  if (
    await useConfirmDialog().confirm({
      header: t("simverse.confirmLoad"),
      message: t("simverse.confirmLoadDesc"),
      confirmText: t("simverse.load"),
    })
  ) {
    try {
      await loadWorld();
      await loadData();
      await useConfirmDialog().showAlert({
        header: t("simverse.loadSuccess"),
        message: t("simverse.loadSuccessDesc"),
        okText: "OK",
      });
    } catch (e: any) {
      await useConfirmDialog().showAlert({
        header: t("errors.error"),
        message: e.message || "Load failed",
        okText: "OK",
      });
    }
  }
}

async function confirmDelete() {
  if (
    await useConfirmDialog().confirm({
      header: t("simverse.confirmDelete"),
      message: t("simverse.confirmDeleteDesc"),
      confirmText: t("simverse.delete"),
      danger: true,
    })
  ) {
    // TODO: delete save API
    await useConfirmDialog().showAlert({
      header: t("simverse.notImplemented"),
      message: t("simverse.notImplementedDesc"),
      okText: "OK",
    });
  }
}

function formatDate(s?: string): string {
  if (!s) return "-";
  try {
    return new Date(s).toLocaleString();
  } catch {
    return s;
  }
}

function formatSize(bytes?: number): string {
  if (bytes == null) return "-";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

onMounted(() => {
  loadData();
});
</script>

<style scoped lang="scss">
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}

.danger-item h3 {
  color: var(--color-error);
}
</style>
