<template>
  <div class="local-openlist-card" :class="cardClass">
    <div class="card-header">
      <div class="card-title-row">
        <ion-icon :icon="serverIcon" class="card-icon"></ion-icon>
        <span class="card-title">{{ t('remote.localOpenListTitle') }}</span>
        <span :class="['status-badge', 'badge-' + badgeColor]">{{ statusLabel }}</span>
      </div>
    </div>

    <!-- not_installed: gray card -->
    <div v-if="state === 'not_installed'" class="card-body">
      <p class="status-line">{{ t('remote.localOpenListNotInstalled') }}</p>
      <p class="status-desc">前往扩展管理页面安装 OpenList 扩展</p>
      <ion-button size="small" fill="solid" color="primary" @click="goToExtensions">
        <ion-icon :icon="extensionPuzzleOutline" slot="start"></ion-icon>
        前往扩展管理
      </ion-button>
    </div>

    <!-- running: green card (keep existing pattern) -->
    <div v-else-if="state === 'running'" class="card-body card-body-success">
      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">{{ t('remote.localOpenListPid') }}</span>
          <span class="info-value">{{ pid }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">{{ t('remote.localOpenListPort') }}</span>
          <span class="info-value">{{ port }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">{{ t('remote.localOpenListDataSize') }}</span>
          <span class="info-value">{{ formattedDataSize }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">{{ t('remote.localOpenListHeartbeat') }}</span>
          <span class="info-value" :class="{ 'heartbeat-fresh': isHeartbeatFresh, 'heartbeat-stale': !isHeartbeatFresh }">
            {{ heartbeatLabel }}
          </span>
        </div>
      </div>
      <ion-button size="small" fill="solid" color="primary" class="open-webui-btn" @click="openWebUi">
        <ion-icon :icon="openIcon" slot="start"></ion-icon>
        {{ t('remote.localOpenListOpenWebUi') }}
      </ion-button>
    </div>

    <!-- port_conflict: orange card -->
    <div v-else-if="state === 'port_conflict'" class="card-body">
      <p class="status-line status-line-warning">{{ t('remote.localOpenListPortConflict', { port: String(port || 5244) }) }}</p>
      <p class="status-desc">请修改 OpenList 端口或关闭占用该端口的程序</p>
      <ion-button size="small" fill="solid" color="warning" @click="goToSettings">
        <ion-icon :icon="settingsIcon" slot="start"></ion-icon>
        修改端口
      </ion-button>
    </div>

    <!-- crash_loop: red card -->
    <div v-else-if="state === 'crash_loop'" class="card-body">
      <p class="status-line status-line-error">OpenList 反复崩溃</p>
      <p class="status-desc">过去 10 秒内 OpenList 进程反复重启 {{ crashCount }} 次，请查看诊断日志</p>
      <ion-button size="small" fill="solid" color="danger" @click="goToDevLogs">
        <ion-icon :icon="bugOutline" slot="start"></ion-icon>
        查看诊断日志
      </ion-button>
    </div>

    <!-- stopped / else -->
    <div v-else class="card-body card-body-medium">
      <p class="status-line">{{ t('remote.localOpenListStopped') }}</p>
    </div>

    <div v-if="error" class="card-error">{{ t('remote.localOpenListLoadFailed') }}: {{ error }}</div>
  </div>
</template>

<script setup lang="ts">
import { formatFileSize } from "@encv/shared-components/api/encv";
import { eventBus } from "@encv/shared-components/composables/useEventBus";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useOpenListBridge } from "@encv/shared-components/composables/useOpenListBridge";
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";

const HEARTBEAT_FRESH_MS = 5000;
const CRASH_LOOP_WINDOW_MS = 10_000;
const CRASH_LOOP_THRESHOLD = 3;

const { t } = useI18n();
const router = useRouter();

useOpenListBridge();

const state = ref<string>("not_installed");
const pid = ref(0);
const port = ref(5244);
const dataDirSize = ref(0);
const lastHeartbeat = ref(0);
const error = ref("");
const crashCount = ref(0);

let nowTickTimer: ReturnType<typeof setInterval> | null = null;
const nowMs = ref(Date.now());

// ------ crash-loop detection ------
interface TransitionEntry {
  timestamp: number;
  running: boolean;
}
const transitionHistory: TransitionEntry[] = [];

function recordTransition(running: boolean) {
  const now = Date.now();
  transitionHistory.push({ timestamp: now, running });

  // prune entries older than CRASH_LOOP_WINDOW_MS
  const cutoff = now - CRASH_LOOP_WINDOW_MS;
  while (transitionHistory.length > 0 && transitionHistory[0].timestamp < cutoff) {
    transitionHistory.shift();
  }

  // count stopped (running=false) transitions in the window
  const stoppedCount = transitionHistory.filter(e => !e.running).length;

  if (stoppedCount >= CRASH_LOOP_THRESHOLD) {
    state.value = "crash_loop";
    crashCount.value = stoppedCount;
    console.error("[SAT-DBG][OpenList][StatusCard] crash_loop detected:", stoppedCount, "stopped transitions in last 10s");
  }
}

// ------ eventBus handlers ------
function onOpenListStatus(data: {
  running: boolean;
  port: number;
  pid: number;
  dataSizeBytes: number;
  isInstalled: boolean;
  lastError: string;
  lastUpdateTs: number;
}) {
  console.error("[SAT-DBG][OpenList][StatusCard] openlist:status event:", data);
  lastHeartbeat.value = Date.now();
  if (!data.isInstalled) {
    state.value = "not_installed";
    return;
  }
  if (data.lastError?.toLowerCase().includes("port")) {
    state.value = "port_conflict";
    return;
  }
  if (data.running) {
    state.value = "running";
    port.value = data.port;
    pid.value = data.pid;
    dataDirSize.value = data.dataSizeBytes;
    recordTransition(true);
  } else {
    if (state.value !== "crash_loop") {
      state.value = "stopped";
    }
    recordTransition(false);
  }
}

function onOpenListError(data: { type: string; message: string; code?: number }) {
  console.error("[SAT-DBG][OpenList][StatusCard] openlist:error event:", data);

  if (data.type === "port_conflict") {
    state.value = "port_conflict";
    error.value = "";
  } else {
    error.value = data.message;
  }
}

// ------ navigation ------
function _goToExtensions() {
  router.push("/tabs/extensions");
}

function _goToSettings() {
  router.push("/tabs/settings");
}

function _goToDevLogs() {
  router.push("/tabs/devlogs");
}

function _openWebUi() {
  window.open(`http://127.0.0.1:${port.value || 5244}/#/login`, "_system");
}

// ------ computed ------
const _cardClass = computed(() => {
  if (state.value === "running") return "state-running";
  if (state.value === "port_conflict") return "state-conflict";
  if (state.value === "crash_loop") return "state-crash-loop";
  return "state-idle";
});

const _badgeColor = computed(() => {
  if (state.value === "running") return "success";
  if (state.value === "port_conflict") return "danger";
  if (state.value === "crash_loop") return "danger";
  return "medium";
});

const _statusLabel = computed(() => {
  if (state.value === "running") return t("remote.localOpenListRunning");
  if (state.value === "port_conflict") return t("remote.localOpenListPortConflict", { port: String(port.value || 5244) });
  if (state.value === "crash_loop") return "反复崩溃";
  if (state.value === "not_installed") return t("remote.localOpenListNotInstalled");
  return t("remote.localOpenListStopped");
});

const _formattedDataSize = computed(() => formatFileSize(dataDirSize.value));

const _isHeartbeatFresh = computed(() => {
  if (!lastHeartbeat.value) return false;
  return nowMs.value - lastHeartbeat.value <= HEARTBEAT_FRESH_MS;
});

const _heartbeatLabel = computed(() => {
  if (!lastHeartbeat.value) return "-";
  const deltaSec = Math.max(0, Math.floor((nowMs.value - lastHeartbeat.value) / 1000));
  if (deltaSec <= 5) return t("remote.localOpenListHeartbeatFresh");
  return t("remote.localOpenListHeartbeatStale", { seconds: String(deltaSec) });
});

// ------ lifecycle ------
onMounted(() => {
  eventBus.on("openlist:status", onOpenListStatus);
  eventBus.on("openlist:error", onOpenListError);
  nowTickTimer = setInterval(() => {
    nowMs.value = Date.now();
  }, 1000);
});

onUnmounted(() => {
  eventBus.off("openlist:status", onOpenListStatus);
  eventBus.off("openlist:error", onOpenListError);
  if (nowTickTimer) {
    clearInterval(nowTickTimer);
    nowTickTimer = null;
  }
});
</script>

<style scoped>
.local-openlist-card {
  margin: 12px 12px 0;
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--ion-background-color, #ffffff);
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
  border-left-width: 3px;
  transition: border-color 0.2s;
}

body.dark .local-openlist-card {
  border-color: #2a2a2c;
  background: #1f1f21;
}

.local-openlist-card.state-running {
  border-left-color: var(--ion-color-success);
}

.local-openlist-card.state-conflict {
  border-left-color: var(--ion-color-warning);
  background: #fff8e1;
}

body.dark .local-openlist-card.state-conflict {
  background: #2a2412;
}

.local-openlist-card.state-crash-loop {
  border-left-color: #c62828;
  background: #ffebee;
}

body.dark .local-openlist-card.state-crash-loop {
  background: #2a1218;
}

.local-openlist-card.state-idle {
  border-left-color: var(--ion-color-medium);
}

.card-header {
  margin-bottom: 8px;
}

.card-title-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.card-icon {
  font-size: 16px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
}

.card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--ion-text-color, #000);
  flex: 1 1 auto;
}

.status-badge {
  font-size: 11px;
  flex-shrink: 0;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 600;
}

.badge-success {
  background: var(--ion-color-success, #2dd36f);
  color: #fff;
}

.badge-danger {
  background: var(--ion-color-danger, #eb445a);
  color: #fff;
}

.badge-medium {
  background: var(--ion-color-medium, #92949c);
  color: #fff;
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 4px;
}

.status-line {
  margin: 0;
  font-size: 13px;
  color: var(--ion-text-color, #000);
  font-weight: 500;
}

.status-desc {
  margin: 0;
  font-size: 12px;
  color: var(--ion-color-medium);
}

.status-line-warning {
  color: #e65100;
}

body.dark .status-line-warning {
  color: #ffb74d;
}

.status-line-error {
  color: #c62828;
}

body.dark .status-line-error {
  color: #ef9a9a;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 6px 14px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.info-label {
  font-size: 11px;
  color: var(--ion-color-medium);
}

.info-value {
  font-size: 13px;
  color: var(--ion-text-color, #000);
  font-weight: 500;
  font-family: monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.heartbeat-fresh {
  color: var(--ion-color-success);
}

.heartbeat-stale {
  color: var(--ion-color-warning);
}

.open-webui-btn {
  align-self: flex-start;
}

.card-error {
  margin-top: 6px;
  font-size: 11px;
  color: var(--ion-color-danger);
  word-break: break-word;
}
</style>