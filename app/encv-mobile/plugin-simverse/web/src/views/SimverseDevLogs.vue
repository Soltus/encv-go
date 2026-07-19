<template>
  <DevLogsViewer
    :title="t('devlogs.title')"
    :tabs="logTabs"
    :items="currentLogs"
    :show-tag-filter="false"
    @tab-change="onTabChange"
    @clear="onClear"
    @copy="onCopy"
  >
    <template #status="{ tab }">
      <div v-if="tab === 'frontend'" class="log-status-bar">
        <span class="status-dot online"></span>
        <span>{{ t('devlogs.total', { total: String(frontendLogs.length), filtered: String(frontendLogs.length) }) }}</span>
      </div>
      <div v-else class="log-status-bar">
        <span :class="['status-dot', backendOnline ? 'online' : 'offline']"></span>
        <span>{{ backendOnline ? t('devlogs.connected') : t('devlogs.disconnected') }}</span>
      </div>
    </template>
    <template #log-item="{ item }">
      <span class="log-source-icon" :title="getSourceIconTitle(item)">{{ getSourceIcon(item) }}</span>
      <span class="log-time">[{{ item.timestamp }}]</span>
      <ion-badge :color="getBadgeColor(item.level)" class="level-badge">{{ item.level.toUpperCase() }}</ion-badge>
      <span class="log-msg">{{ item.message }}</span>
    </template>
  </DevLogsViewer>
</template>

<script setup lang="ts">
import DevLogsViewer from "@encv/shared-components/components/DevLogsViewer.vue";
import type { LogEntry } from "@encv/shared-components/composables/useFrontendLogs";
import { useFrontendLogs } from "@encv/shared-components/composables/useFrontendLogs";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { IonBadge } from "@ionic/vue";
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { logs: frontendLogs, clearLogs: clearFrontendLogs } = useFrontendLogs();
const { loadChronicleWorld, isConnected, init } = useSimverse();

const backendOnline = isConnected;

const activeTab = ref<"frontend" | "backend">("frontend");
const logTabs = [
  { value: "frontend", label: t("devlogs.frontend") },
  { value: "backend", label: t("devlogs.backend") },
];

const backendLogs = ref<LogEntry[]>([]);
let backendLogId = 0;
let pollInterval: number | null = null;
let lastEventId = "";

const currentLogs = computed<readonly LogEntry[]>(() =>
  activeTab.value === "frontend" ? frontendLogs.value : backendLogs.value
);

function chronicleLevelToLogLevel(level: string): string {
  if (level === "critical" || level === "catastrophe") return "error";
  if (level === "major" || level === "minor") return "warn";
  if (level === "trivial") return "debug";
  return "info";
}

/**
 * 后端日志来源 = SimVerse chronicle world 事件（插件 WebView 内访问的是
 * 模拟引擎，不是主应用 Go backend）。轮询 loadChronicleWorld 并去重拼接，
 * 沿用原 SimverseDevLogs 的语义，仅把渲染交给共享 DevLogsViewer。
 */
async function loadBackendLogs() {
  try {
    const data = await loadChronicleWorld(0, 50);
    const events = (data?.items as Array<Record<string, any>>) || [];
    const newLogs: LogEntry[] = [];

    for (const evt of events) {
      const evtId = String(evt.id ?? "");
      if (evtId && evtId === lastEventId) break;

      const level = chronicleLevelToLogLevel(evt.level || evt.imp_name || "info");
      const tags = ["simverse", "chronicle", evt.type || "event", evt.level || ""];

      newLogs.push({
        id: ++backendLogId,
        timestamp: new Date().toLocaleTimeString(),
        level,
        message: `[Tick ${evt.tick}] ${evt.type_cn || evt.type}: ${(evt as any).data_tag || "(world event)"}`,
        source: "simverse.chronicle",
        tags,
      });
    }

    if (newLogs.length > 0) {
      backendLogs.value = [...newLogs.reverse(), ...backendLogs.value].slice(-1000);
      if (events.length > 0) {
        lastEventId = String(events[0].id ?? "");
      }
    }
  } catch {
    // 静默失败，避免污染日志
  }
}

function onTabChange(tab: string) {
  activeTab.value = tab as "frontend" | "backend";
}

function onClear() {
  if (activeTab.value === "frontend") {
    clearFrontendLogs();
  } else {
    backendLogs.value = [];
    lastEventId = "";
  }
}

function onCopy(_items: readonly LogEntry[]) {
  // DevLogsViewer 已内部复制并 toast，无需额外处理
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

function getSourceIcon(log: LogEntry): string {
  if (log.source?.includes("chronicle")) return "📜";
  if (log.source?.includes("world")) return "🌍";
  if (log.source?.includes("npc")) return "👤";
  if (log.level === "error") return "❌";
  if (log.level === "warn") return "⚠️";
  if (log.level === "debug") return "🔧";
  return "ℹ️";
}

function getSourceIconTitle(log: LogEntry): string {
  return log.source || log.level;
}

onMounted(() => {
  // 确保 simverse 引擎会话/WS 已建立（idempotent：多次调用只初始化一次）。
  // DevLogs 可能作为独立 tab 打开（未经过主 SimVerse 视图），此时若不初始化，
  // loadChronicleWorld 会因引擎未就绪而拿不到 chronicle 事件 → 后端日志为空。
  init().catch(() => {});
  loadBackendLogs();
  pollInterval = window.setInterval(loadBackendLogs, 5000);
});

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval);
});
</script>

<style scoped lang="scss">
.log-status-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;

  &.online {
    background: var(--color-success);
    box-shadow: 0 0 6px var(--color-success);
  }

  &.offline {
    background: var(--color-base-content);
    opacity: 0.5;
  }
}

.log-source-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  font-size: 12px;
  flex-shrink: 0;
  line-height: 1;
  user-select: none;
}

.log-time {
  color: var(--color-base-content);
  opacity: 0.7;
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
