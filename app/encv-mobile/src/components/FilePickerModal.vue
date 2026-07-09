<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button v-if="currentPath !== '/'" @click="goUp">
            <ion-icon :icon="arrowBack" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
        <ion-title>{{ mode === 'folder' ? t('files.selectFolder') : t('files.selectFile') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button v-if="mode === 'folder'" @click="showNewFolderInput">
            <ion-icon :icon="add" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button @click="cancel">{{ t('files.cancelSelect') }}</ion-button>
        </ion-buttons>
      </ion-toolbar>
      <ion-toolbar v-if="currentPath !== '/'">
        <div class="breadcrumb-scroll">
          <div class="breadcrumb">
            <span class="breadcrumb-item" @click="navigateTo('/')">{{ t('files.root') }}</span>
            <span v-for="(segment, index) in pathSegments" :key="index" class="breadcrumb-segment">
              <ion-icon :icon="chevronForward" class="breadcrumb-sep"></ion-icon>
              <span class="breadcrumb-item" @click="navigateTo(segment.path)">{{ segment.name }}</span>
            </span>
          </div>
        </div>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('files.loading') }}</p>
      </div>

      <div v-else-if="noPermission" class="empty-state">
        <ion-icon :icon="lockClosed" class="empty-icon"></ion-icon>
        <h3>{{ t('files.noPermission') }}</h3>
        <p>{{ t('files.noPermissionDesc') }}</p>
      </div>

      <div v-else-if="displayFiles.length === 0" class="empty-state">
        <ion-icon :icon="folderOpen" class="empty-icon"></ion-icon>
        <h3>{{ t('files.emptyDir') }}</h3>
        <p>{{ t('files.emptyDirDesc') }}</p>
      </div>

      <div v-if="showNewFolder" class="overlay-backdrop" @click="cancelNewFolder"></div>

      <div v-if="showNewFolder" class="new-folder-input-overlay">
        <ion-input v-model="newFolderName" :placeholder="t('files.newFolderName')" enterkeyhint="done"
                   @keydown.enter="confirmNewFolder"></ion-input>
        <ion-button size="small" @click="confirmNewFolder">{{ t('common.confirm') }}</ion-button>
        <ion-button size="small" fill="clear" @click="cancelNewFolder">{{ t('common.cancel') }}</ion-button>
      </div>

      <ion-list>
        <ion-item
          v-for="file in displayFiles"
          :key="file.path"
          @click="handleFileClick(file)"
          detail
        >
          <ion-icon
            :icon="getFileIcon(file)"
            :color="getFileIconColor(file)"
            slot="start"
          ></ion-icon>
          <ion-label>
            <h2>{{ file.name }}</h2>
            <p v-if="!file.isDirectory && file.size">{{ formatFileSize(file.size) }}</p>
            <p v-else-if="file.isDirectory">{{ t('files.directory') }}</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <div v-if="mode === 'folder' && !loading" class="folder-picker-bar">
        <span class="current-path-label">{{ currentPath }}</span>
        <ion-button size="small" @click="selectCurrentFolder">
          {{ t('files.selectThisFolder') }}
        </ion-button>
      </div>

      <div v-if="mode === 'file' && !loading && displayFiles.length > 0" class="picker-hint">
        {{ t('files.tapToSelect') }}
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { formatFileSize } from "@/api/encv_files";
import type { FileItem } from "@/api/encv";
import { createDirectory, getFileCategory, listFiles, PermissionDeniedError } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";
import { alertController, modalController } from "@ionic/vue";
import {
  add,
  arrowBack,
  chevronForward,
  document,
  documentText,
  folder,
  folderOpen,
  image,
  lockClosed,
  musicalNotes,
  videocam,
} from "ionicons/icons";
import { computed, onMounted, ref } from "vue";

const props = withDefaults(
  defineProps<{
    mode?: "file" | "folder";
    initialPath?: string;
  }>(),
  {
    mode: "file",
    initialPath: "/",
  }
);

const { t } = useI18n();
const files = ref<FileItem[]>([]);
const currentPath = ref(props.initialPath || "/");
const loading = ref(false);
const noPermission = ref(false);
const showNewFolder = ref(false);
const newFolderName = ref("");

const pathSegments = computed(() => {
  if (currentPath.value === "/") return [];
  const parts = currentPath.value.split("/").filter(Boolean);
  return parts.map((name, index) => ({
    name,
    path: "/" + parts.slice(0, index + 1).join("/"),
  }));
});

const sortedFiles = computed(() => {
  return [...files.value].sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1;
    if (!a.isDirectory && b.isDirectory) return 1;
    return a.name.localeCompare(b.name);
  });
});

const displayFiles = computed(() => {
  if (props.mode === "folder") {
    return sortedFiles.value.filter(f => f.isDirectory);
  }
  return sortedFiles.value;
});

function getFileIcon(file: FileItem) {
  if (file.isDirectory) return folder;
  if (file.isEncrypted) return lockClosed;
  const category = getFileCategory(file.name);
  switch (category) {
    case "video":
      return videocam;
    case "audio":
      return musicalNotes;
    case "image":
      return image;
    case "document":
      return document;
    default:
      return documentText;
  }
}

function getFileIconColor(file: FileItem) {
  if (file.isDirectory) return "primary";
  if (file.isEncrypted) return "warning";
  const category = getFileCategory(file.name);
  switch (category) {
    case "video":
      return "danger";
    case "audio":
      return "tertiary";
    case "image":
      return "success";
    default:
      return "medium";
  }
}

async function loadFiles() {
  loading.value = true;
  noPermission.value = false;
  try {
    files.value = await listFiles(currentPath.value);
    noPermission.value = false;
  } catch (error) {
    if (error instanceof PermissionDeniedError) {
      noPermission.value = true;
    }
    files.value = [];
  }
  loading.value = false;
}

function navigateTo(path: string) {
  currentPath.value = path;
  loadFiles();
}

function goUp() {
  if (currentPath.value === "/") return;
  const parts = currentPath.value.split("/").filter(Boolean);
  parts.pop();
  currentPath.value = parts.length === 0 ? "/" : "/" + parts.join("/");
  loadFiles();
}

function handleFileClick(file: FileItem) {
  if (file.isDirectory) {
    const newPath = currentPath.value === "/" ? "/" + file.name : currentPath.value + "/" + file.name;
    navigateTo(newPath);
    return;
  }
  if (props.mode === "file") {
    modalController.dismiss({ path: file.path, name: file.name }, "select");
  }
}

function selectCurrentFolder() {
  modalController.dismiss({ path: currentPath.value, name: currentPath.value.split("/").filter(Boolean).pop() || "/" }, "select");
}

function cancel() {
  modalController.dismiss(null, "cancel");
}

function showNewFolderInput() {
  showNewFolder.value = true;
  newFolderName.value = "";
}

async function confirmNewFolder() {
  const name = newFolderName.value.trim();
  if (!name) return;
  try {
    await createDirectory(currentPath.value, name);
    const newPath = currentPath.value === "/" ? `/${name}` : `${currentPath.value}/${name}`;
    showNewFolder.value = false;
    newFolderName.value = "";
    navigateTo(newPath);
  } catch (error) {
    const msg = error instanceof Error ? error.message : String(error);
    const alert = await alertController.create({
      header: t("files.createFolderFailed"),
      message: msg,
      buttons: [t("common.confirm")],
    });
    await alert.present();
  }
}

function cancelNewFolder() {
  showNewFolder.value = false;
  newFolderName.value = "";
}

onMounted(() => {
  loadFiles();
});
</script>

<style scoped>
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 50%;
  color: var(--encv-text-secondary);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 50%;
  padding: 24px;
  text-align: center;
  color: var(--encv-text-secondary);
}

.empty-icon {
  font-size: 64px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.breadcrumb-scroll {
  --background: transparent;
}

.breadcrumb {
  display: flex;
  align-items: center;
  padding: 0 16px;
  white-space: nowrap;
}

.breadcrumb-item {
  cursor: pointer;
  color: var(--ion-color-primary);
  font-size: 14px;
}

.breadcrumb-item:hover {
  text-decoration: underline;
}

.breadcrumb-sep {
  font-size: 14px;
  margin: 0 4px;
  color: var(--encv-text-secondary);
}

.breadcrumb-segment {
  display: flex;
  align-items: center;
}

.picker-hint {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 12px 16px;
  text-align: center;
  font-size: 14px;
  color: var(--ion-color-primary);
  background: var(--ion-background-color, #fff);
  border-top: 1px solid var(--ion-color-light, #f4f5f8);
  z-index: 10;
}

.folder-picker-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  background: var(--ion-background-color, #fff);
  border-top: 1px solid var(--ion-color-light, #f4f5f8);
  z-index: 10;
}

.current-path-label {
  font-size: 13px;
  color: var(--encv-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  margin-right: 12px;
}

.overlay-backdrop {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0, 0, 0, 0.3);
  z-index: 19;
}

.new-folder-input-overlay {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  background: var(--ion-background-color, #fff);
  border-top: 1px solid var(--ion-color-light, #f4f5f8);
  z-index: 20;
}

.new-folder-input-overlay ion-input {
  flex: 1;
}
</style>
