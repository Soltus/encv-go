<template>
  <DevLogsViewer
    :title="t('mpv.devlogs.title')"
    :tabs="logTabs"
    :items="currentLogs"
    :show-tag-filter="false"
    @clear="onClear"
    @copy="onCopy"
  >
    <template #status="{ tab }">
      <div v-if="tab === 'frontend'" class="log-status-bar">
        <span class="status-dot online"></span>
        <span>前端日志 {{ frontendLogs.length }} 条</span>
      </div>
      <div v-else class="log-status-bar">
        <span :class="['status-dot', playerOnline ? 'online' : 'offline']"></span>
        <span>播放器日志 {{ playerLogs.length }} 条</span>
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
import { logBuffer, MpvNative } from "@/plugins/mpv-native";

const { t } = useI18n();

const logTabs = [
  { value: "frontend", label: t("mpv.devlogs.frontend") },
  { value: "player", label: t("mpv.devlogs.player") },
];

const frontendLogs = ref<any[]>([]);
const playerLogs = ref<any[]>([]);
const playerOnline = ref(false);

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

  pollPlayerStatus();
  pollTimer = setInterval(pollPlayerStatus, 3000);
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

function pollPlayerStatus() {
  if (!window.MpvNative) {
    playerOnline.value = false;
    return;
  }
  try {
    const status = MpvNative.getStatus();
    playerOnline.value = status.playing || status.paused;
  } catch {
    playerOnline.value = false;
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
