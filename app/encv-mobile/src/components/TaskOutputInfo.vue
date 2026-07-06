<template>
  <div class="detail-section" v-if="task.status === 'completed'">
    <div class="section-title completed-section-title">
      <ion-icon :icon="checkmarkCircle" color="success"></ion-icon>
      {{ t('tasks.phaseCompleted') }}
    </div>
    <p class="completed-duration" v-if="durationStr">{{ t('tasks.duration') }}: {{ durationStr }}</p>

    <div v-if="outputInfo" class="output-block">
      <div class="output-header">
        <ion-icon :icon="documentTextOutline" color="primary"></ion-icon>
        <span class="output-label">{{ t('tasks.outputFile') }}</span>
      </div>
      <div class="output-name" :title="outputInfo.name">{{ outputInfo.name }}</div>
      <div class="output-meta">
        <span>{{ outputInfo.sizeLabel }}</span>
        <span class="output-sep">·</span>
        <span>{{ outputInfo.dirLabel }}</span>
      </div>
      <div class="output-actions">
        <ion-button
          v-if="canPreviewOutput"
          size="small"
          color="primary"
          @click="handleOpenOutput"
        >
          <ion-icon :icon="playCircleOutline" slot="start"></ion-icon>
          {{ t('tasks.openOutput') }}
        </ion-button>
        <ion-button
          size="small"
          color="medium"
          fill="outline"
          @click="handleLocateOutput"
        >
          <ion-icon :icon="folderOpenOutline" slot="start"></ion-icon>
          {{ t('tasks.locateInFiles') }}
        </ion-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  checkmarkCircle,
  documentTextOutline,
  folderOpenOutline,
  playCircleOutline,
} from "ionicons/icons";

import type { EncvTask } from "@/api/encv";
import { formatDuration } from "@/composables/useDateFormat";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import { computed } from "vue";

const props = defineProps<{ task: EncvTask }>();
const emit = defineEmits<{
  (e: "open", outputPath: string): void;
  (e: "locate", outputPath: string): void;
}>();
const { t } = useI18n();

const PREVIEWABLE_VIDEO = new Set(["mp4", "webm", "mov", "m4v", "mkv"]);

const durationStr = computed(() => {
  if (!props.task.createdAt) return "";
  const created = new Date(props.task.createdAt).getTime();
  if (Number.isNaN(created)) return "";
  if (props.task.completedAt) {
    const completed = new Date(props.task.completedAt).getTime();
    if (Number.isNaN(completed)) return "";
    return formatDuration(completed - created);
  }
  return "";
});

const outputInfo = computed(() => {
  const op = props.task.outputPath;
  if (!op) return null;
  const name = op.split("/").pop() || op;
  return {
    fullPath: op,
    name,
    dirLabel: dirOf(op),
    sizeLabel: "",
  };
});

const canPreviewOutput = computed(() => {
  if (!outputInfo.value) return false;
  const ext = outputInfo.value.name.split(".").pop()?.toLowerCase() || "";
  return PREVIEWABLE_VIDEO.has(ext);
});

function dirOf(p: string): string {
  const idx = p.lastIndexOf("/");
  if (idx < 0) return "/";
  return p.slice(0, idx) || "/";
}

function handleOpenOutput() {
  if (!outputInfo.value) return;
  const ext = outputInfo.value.name.split(".").pop()?.toLowerCase() || "";
  if (PREVIEWABLE_VIDEO.has(ext)) {
    emit("open", outputInfo.value.fullPath);
  } else {
    showToast({ message: t("tasks.previewUnsupportedExt") + ": " + ext, duration: 2000, color: "medium" });
  }
}

function handleLocateOutput() {
  if (!outputInfo.value) return;
  emit("locate", outputInfo.value.fullPath);
}
</script>

<style scoped>
.detail-section {
  margin-bottom: 20px;
}

.section-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--ion-text-color);
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.completed-section-title { color: var(--ion-color-success); }

.completed-duration {
  font-size: 13px;
  color: var(--ion-color-medium);
  margin-top: 4px;
}

.output-block {
  margin-top: 12px;
  padding: 12px;
  background: rgba(var(--ion-color-primary-rgb), 0.04);
  border: 1px solid rgba(var(--ion-color-primary-rgb), 0.15);
  border-radius: 8px;
}
.output-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
}
.output-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--ion-color-primary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.output-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--ion-text-color);
  word-break: break-all;
  margin-bottom: 4px;
}
.output-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--ion-color-medium);
  margin-bottom: 10px;
}
.output-sep { opacity: 0.5; }
.output-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.output-actions ion-button {
  --padding-start: 12px;
  --padding-end: 12px;
  height: 36px;
  font-size: 13px;
  font-weight: 500;
}
</style>
