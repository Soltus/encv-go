<template>
  <div class="tasks-tab">
    <div v-if="runTasks.length === 0" class="empty-state">
      <ion-icon :icon="listOutline" class="empty-icon" color="medium"></ion-icon>
      <h3>{{ t('tasks.groupDetail.emptyTasks') }}</h3>
      <p>{{ t('tasks.groupDetail.emptyTasksDesc') }}</p>
    </div>
    <ion-list v-else class="tasks-tab__list">
      <!-- 🆕 2026-06-22 Q4：ion-item-sliding 包裹（Q5A 滑动操作） -->
      <ion-item-sliding v-for="tk in runTasks" :key="tk.id">
        <ion-item-options side="start">
          <ion-item-option
            v-if="canCancel(tk)"
            color="warning"
            @click="onCancel(tk)"
          >
            <ion-icon slot="icon-only" :icon="stopCircleOutline"></ion-icon>
            {{ t('tasks.cancel') }}
          </ion-item-option>
        </ion-item-options>

        <ion-item
          class="tasks-tab__item"
          :button="!multiSelectMode"
          :detail="!multiSelectMode"
          :color="selectedIds.has(tk.id) ? 'primary' : ''"
          @click="onItemClick(tk)"
        >
          <!-- 🆕 Q4：多选模式时显示 checkbox -->
          <ion-checkbox
            v-if="multiSelectMode"
            slot="start"
            :checked="selectedIds.has(tk.id)"
            @ion-change="emit('toggle-select', tk.id)"
          />
          <ion-icon
            v-else
            :icon="getTaskIcon(tk)"
            :color="getStatusColor(tk.status)"
            slot="start"
          ></ion-icon>

          <ion-label>
            <h2>
              {{ getTaskName(tk) }}
              <ion-badge v-if="isEncrypted(tk)" color="warning" class="encrypted-badge">
                <ion-icon :icon="lockClosed" size="small"></ion-icon>
                {{ t('tasks.encrypted') }}
              </ion-badge>
            </h2>
            <p class="tasks-tab__meta">
              <ion-badge :color="getStatusColor(tk.status)" class="tl-status-badge">
                {{ getStatusLabel(tk.status) }}
              </ion-badge>
              <!-- 🆕 Q8B：typeMap 查表 -->
              <span class="tasks-tab__type">{{ getTaskTypeLabel(tk.type, t) }}</span>
              <ion-badge v-if="tk.pluginName" color="primary" class="tasks-tab__plugin">{{ tk.pluginName }}</ion-badge>
            </p>
            <p class="tasks-tab__time">
              <span>{{ formatDateTime(tk.createdAt) }}</span>
              <span v-if="getTaskDuration(tk)">· {{ getTaskDuration(tk) }}</span>
              <span v-if="tk.performanceSummary?.avgThroughput">· {{ tk.performanceSummary.avgThroughput.toFixed(1) }} MB/s</span>
            </p>
            <p v-if="tk.error" class="tasks-tab__error">
              <ion-icon :icon="alertCircle" size="small"></ion-icon>
              {{ tk.error }}
            </p>
          </ion-label>
        </ion-item>

        <ion-item-options side="end">
          <ion-item-option
            color="primary"
            @click="onRetry(tk)"
            v-if="canRetry(tk)"
          >
            <ion-icon slot="icon-only" :icon="refreshOutline"></ion-icon>
          </ion-item-option>
          <ion-item-option
            color="danger"
            @click="onDelete(tk)"
          >
            <ion-icon slot="icon-only" :icon="trashOutline"></ion-icon>
          </ion-item-option>
        </ion-item-options>
      </ion-item-sliding>
    </ion-list>
  </div>
</template>

<script setup lang="ts">
import { toastController } from "@ionic/vue";
import { alertCircle, listOutline, lockClosed, refreshOutline, stopCircleOutline, trashOutline } from "ionicons/icons";

import type { EncvTask } from "@/api/encv";
import { cancelTask, deleteTask, retryTask } from "@/api/encv";
import { formatDateTime } from "@/composables/useDateFormat";
import { useI18n } from "@/composables/useI18n";
import { getTaskTypeIcon, getTaskTypeLabel } from "@/lib/taskTypeLabel";

const { t } = useI18n();

// 🆕 Q4：接收多选参数
const props = withDefaults(
  defineProps<{
    runTasks: EncvTask[];
    multiSelectMode?: boolean;
    selectedIds?: Set<string>;
    searchQuery?: string;
  }>(),
  {
    multiSelectMode: false,
    selectedIds: () => new Set<string>(),
    searchQuery: "",
  }
);

const emit = defineEmits<{
  (e: "select-task", task: EncvTask): void;
  (e: "toggle-select", taskId: string): void;
  (e: "open-performance"): void;
}>();

function onItemClick(tk: EncvTask) {
  if (props.multiSelectMode) {
    emit("toggle-select", tk.id);
  } else {
    emit("select-task", tk);
  }
}

function getTaskName(task: EncvTask): string {
  if (task.targetPath) return task.targetPath.split("/").pop() || task.targetPath;
  if (task.sourcePath) return task.sourcePath.split("/").pop() || task.sourcePath;
  return task.id.slice(0, 8);
}

function getTaskIcon(task: EncvTask): string {
  return getTaskTypeIcon(task.type);
}

function isEncrypted(task: EncvTask): boolean {
  return task.type === "encrypt" || (task.targetPath?.endsWith(".encv") ?? false);
}

function getStatusLabel(status: EncvTask["status"]): string {
  return t(`tasks.status.${status}`);
}

function getStatusColor(status: EncvTask["status"]): string {
  switch (status) {
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "running":
    case "cancelling":
      return "warning";
    case "cancelled":
      return "medium";
    case "queued":
      return "primary";
    default:
      return "medium";
  }
}

function getTaskDuration(task: EncvTask): string {
  if (!task.completedAt) return "";
  const ms = new Date(task.completedAt).getTime() - new Date(task.createdAt).getTime();
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`;
}

function canCancel(tk: EncvTask): boolean {
  return tk.status === "running" || tk.status === "queued";
}
function canRetry(tk: EncvTask): boolean {
  return tk.status === "failed" || tk.status === "cancelled";
}

async function onCancel(tk: EncvTask) {
  try {
    await cancelTask(tk.id);
    const toast = await toastController.create({ message: t("tasks.cancelSuccess"), duration: 1500, color: "success" });
    await toast.present();
  } catch (err: any) {
    const toast = await toastController.create({ message: err.message ?? t("common.failed"), duration: 2000, color: "danger" });
    await toast.present();
  }
}

async function onRetry(tk: EncvTask) {
  try {
    await retryTask(tk.id);
    const toast = await toastController.create({ message: t("tasks.retrySuccess"), duration: 1500, color: "success" });
    await toast.present();
  } catch (err: any) {
    const toast = await toastController.create({ message: err.message ?? t("common.failed"), duration: 2000, color: "danger" });
    await toast.present();
  }
}

async function onDelete(tk: EncvTask) {
  try {
    await deleteTask(tk.id);
    const toast = await toastController.create({ message: t("tasks.deleteSuccess"), duration: 1500, color: "success" });
    await toast.present();
  } catch (err: any) {
    const toast = await toastController.create({ message: err.message ?? t("common.failed"), duration: 2000, color: "danger" });
    await toast.present();
  }
}
</script>

<style scoped>
.tasks-tab__list {
  padding: 12px;
  background: transparent;
}
.tasks-tab__item {
  --padding-start: 8px;
  --padding-end: 8px;
  --min-height: 64px;
  margin-bottom: 6px;
}
.tasks-tab__meta {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 4px;
  font-size: 12px;
}
.tasks-tab__plugin {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
}
.tasks-tab__type {
  font-size: 11px;
  color: var(--ion-color-medium);
}
.tasks-tab__time {
  font-size: 11px;
  color: var(--ion-color-medium-shade);
  margin-top: 4px;
  font-family: monospace;
}
.tasks-tab__error {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--ion-color-danger-shade);
  margin-top: 4px;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.encrypted-badge {
  font-size: 10px;
  margin-left: 4px;
  --padding-start: 4px;
  --padding-end: 4px;
}
.tl-status-badge {
  font-size: 10px;
}
.empty-state {
  text-align: center;
  padding: 48px 16px;
}
.empty-icon {
  font-size: 56px;
  margin-bottom: 8px;
  display: block;
  margin-left: auto;
  margin-right: auto;
}
</style>
