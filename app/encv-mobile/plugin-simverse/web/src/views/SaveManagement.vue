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
      <div class="p-4 space-y-4">
        <div v-if="loading" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>

        <template v-else>
          <div class="ui-card">
            <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.currentSave") }}</div>
              <div v-if="!saveInfo?.has_save" class="flex items-center gap-3 p-3">
                <ion-icon :icon="saveOutline" class="text-base-content/40 text-2xl" />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium">{{ t("simverse.noSave") }}</div>
                  <div class="text-xs text-base-content/70 mt-0.5">{{ t("simverse.noSaveDesc") }}</div>
                </div>
              </div>
              <template v-else>
                <div class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <ion-icon :icon="saveOutline" class="text-primary text-2xl" />
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium">{{ t("simverse.savedAt") }}: {{ formatDate(saveInfo.saved_at) }}</div>
                    <div class="text-xs text-base-content/70 mt-0.5">Tick {{ saveInfo.tick }} · {{ saveInfo.npc_count }} {{ t("simverse.npcs") }}</div>
                  </div>
                  <span class="text-xs text-base-content/70 font-mono">{{ formatSize(saveInfo.size_bytes) }}</span>
                </div>
              </template>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.actions") }}</div>
              <div class="space-y-1">
                <div
                  class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  @click="doSave"
                  :class="{ 'opacity-50 pointer-events-none': saving }"
                >
                  <ion-icon :icon="cloudUploadOutline" class="text-primary text-xl" />
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium">{{ t("simverse.saveNow") }}</div>
                    <div v-if="saving" class="text-xs text-base-content/70 mt-0.5">{{ t("settings.checking") }}...</div>
                  </div>
                  <ion-spinner v-if="saving" name="crescent" size="small" />
                </div>

                <div
                  class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  :class="{ 'opacity-50 pointer-events-none': !saveInfo?.has_save || loading }"
                  @click="doLoad"
                >
                  <ion-icon :icon="cloudDownloadOutline" class="text-success text-xl" />
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium">{{ t("simverse.loadSave") }}</div>
                    <div class="text-xs text-base-content/70 mt-0.5">{{ t("simverse.loadSaveDesc") }}</div>
                  </div>
                </div>

                <div
                  class="flex items-center gap-3 p-3 rounded-lg hover:bg-error/10 transition-colors cursor-pointer"
                  :class="{ 'opacity-50 pointer-events-none': !saveInfo?.has_save }"
                  @click="confirmDelete"
                >
                  <ion-icon :icon="trashOutline" class="text-error text-xl" />
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium text-error">{{ t("simverse.deleteSave") }}</div>
                    <div class="text-xs text-base-content/70 mt-0.5">{{ t("simverse.deleteSaveDesc") }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="storage" class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.storage") }}</div>
              <div class="space-y-2">
                <div class="flex items-center justify-between p-3">
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium">{{ formatSize(storage.used_bytes) }} / {{ formatSize(storage.total_bytes) }}</div>
                    <div class="text-xs text-base-content/70 mt-0.5">{{ t("simverse.available") }}: {{ formatSize(storage.available_bytes) }}</div>
                  </div>
                </div>
                <div class="h-2 bg-base-300 rounded-full overflow-hidden mx-3">
                  <div
                    class="h-full bg-primary transition-all"
                    :style="{ width: storageUsedPercent + '%' }"
                  ></div>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
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
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { useConfirmDialog } from "@encv/shared-components/composables/useConfirmDialog";
import { cloudDownloadOutline, cloudUploadOutline, saveOutline, trashOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { type SimverseSaveInfo, type SimverseStorageStatus, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadSaveInfo, saveWorld, loadWorld, loadStorageStatus } = useSimverse();

const loading = ref(false);
const saving = ref(false);
const saveInfo = ref<SimverseSaveInfo | null>(null);
const storage = ref<SimverseStorageStatus | null>(null);

const storageUsedPercent = computed(() => {
  if (!storage.value || !storage.value.total_bytes) return 0;
  return (storage.value.used_bytes / storage.value.total_bytes) * 100;
});

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
.state-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
</style>
