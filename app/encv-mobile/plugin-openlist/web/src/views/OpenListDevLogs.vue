<template>
  <DevLogsViewer
    :title="t('openlist.devlogs.title')"
    :tabs="logTabs"
    :items="currentLogs"
    :show-tag-filter="false"
    @clear="onClear"
    @copy="onCopy"
  >
    <template #status="{ tab }">
      <div v-if="tab === 'frontend'" class="log-status-bar">
        <span class="status-dot online"></span>
        <span>{{ t('openlist.devlogs.frontendCount', { count: frontendLogs.length }) }}</span>
      </div>
      <div v-else class="log-status-bar">
        <span :class="['status-dot', backendOnline ? 'online' : 'offline']"></span>
        <span>{{ t('openlist.devlogs.backendCount', { count: backendLogs.length }) }}</span>
      </div>
    </template>
    <template #log-item="{ item, tab }">
      <span class="log-time">[{{ formatTime(item.timestamp) }}]</span>
      <ion-badge :color="getBadgeColor(item.level)" class="level-badge">
        {{ item.level.toUpperCase() }}
      </ion-badge>
      <span class="log-msg">{{ item.message }}</span>
    </template>
  </DevLogsViewer>
</template>

<script setup lang="ts">
import DevLogsViewer from "@encv/shared-components/components/DevLogsViewer.vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { computed, onMounted, onUnmounted, ref } from "vue";
import type { OpenListLog } from "@/components-shared";
import { logBuffer, OpenListNative } from "@/plugins/openlist-native";

const { t } = useI18n();

const logTabs = [
  { value: "frontend", label: t("openlist.devlogs.frontend") },
  { value: "backend", label: t("openlist.devlogs.backend") },
];

const frontendLogs = ref<OpenListLog[]>([]);
const backendLogs = ref<OpenListLog[]>([]);
const backendOnline = ref(false);

const currentLogs = computed(() => {
  return frontendLogs.value as any[];
});

let unsubscribeLog: (() => void) | null = null;
let pollTimer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  unsubscribeLog = logBuffer.subscribe(all => {
    frontendLogs.value = [...all].reverse();
  });
  frontendLogs.value = [...logBuffer.getAll()].reverse();

  pollBackendLogs();
  pollTimer = setInterval(pollBackendLogs, 3000);
});

onUnmounted(() => {
  if (unsubscribeLog) unsubscribeLog();
  if (pollTimer) clearInterval(pollTimer);
});

function formatTime(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleTimeString("zh-CN", { hour12: false });
}

function getBadgeColor(level: string): string {
  switch (level) {
    case "error":
      return "danger";
    case "warn":
      return "warning";
    case "info":
      return "primary";
    case "debug":
      return "medium";
    default:
      return "medium";
  }
}

function pollBackendLogs() {
  if (!window.OpenListNative) {
    backendOnline.value = false;
    return;
  }
  try {
    const status = OpenListNative.getStatus();
    backendOnline.value = status.running;
  } catch {
    backendOnline.value = false;
  }
}

function onClear() {
  logBuffer.clear();
}

function onCopy(items: readonly any[]) {
  console.log("copy logs:", items.length);
}
</script>

<style scoped>
.log-status-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  font-size: 12px;
  color: var(--ion-color-medium);
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.status-dot.online {
  background: var(--ion-color-success);
  box-shadow: 0 0 6px var(--ion-color-success);
}
.status-dot.offline {
  background: var(--ion-color-medium);
}
.log-time {
  color: var(--ion-color-medium, #6b7280);
  font-size: 11px;
  flex-shrink: 0;
}
.level-badge {
  font-size: 10px;
  padding: 1px 6px;
  flex-shrink: 0;
}
.log-msg {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
