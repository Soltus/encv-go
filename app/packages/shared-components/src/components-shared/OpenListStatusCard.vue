<template>
  <div class="openlist-status-card" :class="cardClass">
    <div class="card-header">
      <div class="card-title-row">
        <ion-icon :icon="serverOutline" class="card-icon"></ion-icon>
        <span class="card-title">OpenList</span>
        <span :class="['status-badge', 'badge-' + badgeColor]">{{ statusLabel }}</span>
      </div>
    </div>

    <!-- running: 详情网格 -->
    <div v-if="state === 'running'" class="card-body card-body-success">
      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">PID</span>
          <span class="info-value">{{ runtime.pid }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Port</span>
          <span class="info-value">{{ runtime.port }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">Data</span>
          <span class="info-value">{{ formattedDataSize }}</span>
        </div>
        <div class="info-item">
          <span class="info-label">心跳</span>
          <span class="info-value heartbeat-fresh">
            {{ heartbeatLabel }}
          </span>
        </div>
      </div>
    </div>

    <!-- stopped -->
    <div v-else-if="state === 'stopped'" class="card-body card-body-medium">
      <p class="status-line">OpenList 未运行</p>
    </div>

    <!-- port_conflict -->
    <div v-else-if="state === 'port_conflict'" class="card-body card-body-warning">
      <p class="status-line status-line-warning">端口 {{ runtime.port || 5244 }} 被占用</p>
    </div>

    <!-- not_installed -->
    <div v-else class="card-body card-body-medium">
      <p class="status-line">OpenList 扩展未安装</p>
    </div>

    <div v-if="runtime.lastError" class="card-error">{{ runtime.lastError }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import type { OpenListRuntime } from "./index";

const props = defineProps<{
  runtime: OpenListRuntime;
  refreshIntervalMs?: number;
}>();

const nowMs = ref(Date.now());
let timer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  timer = setInterval(() => {
    nowMs.value = Date.now();
  }, 1000);
});

onUnmounted(() => {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
});

const state = computed(() => {
  if (!props.runtime.isInstalled) return "not_installed";
  if (props.runtime.lastError?.toLowerCase().includes("port")) return "port_conflict";
  return props.runtime.running ? "running" : "stopped";
});

const _formattedDataSize = computed(() => {
  const b = props.runtime.dataSizeBytes || 0;
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`;
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`;
});

const _heartbeatLabel = computed(() => {
  if (!props.runtime.lastUpdateTs) return "-";
  const deltaSec = Math.max(0, Math.floor((nowMs.value - props.runtime.lastUpdateTs) / 1000));
  if (deltaSec <= 5) return "正常";
  return `${deltaSec}s 前`;
});

const _cardClass = computed(() => `state-${state.value}`);

const _badgeColor = computed(() => {
  if (state.value === "running") return "success";
  if (state.value === "port_conflict") return "danger";
  return "medium";
});

const _statusLabel = computed(() => {
  if (state.value === "running") return "运行中";
  if (state.value === "port_conflict") return "端口冲突";
  if (state.value === "not_installed") return "未安装";
  return "已停止";
});
</script>

<style scoped>
.openlist-status-card {
  margin: 12px 12px 0;
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--ion-background-color, #ffffff);
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
  border-left-width: 3px;
}
.openlist-status-card.state-running { border-left-color: var(--ion-color-success); }
.openlist-status-card.state-conflict,
.openlist-status-card.state-port_conflict { border-left-color: var(--ion-color-warning); }
.openlist-status-card.state-stopped,
.openlist-status-card.state-not_installed { border-left-color: var(--ion-color-medium); }

.card-header { margin-bottom: 8px; }
.card-title-row { display: flex; align-items: center; gap: 6px; }
.card-icon { font-size: 16px; color: var(--ion-color-primary); flex-shrink: 0; }
.card-title { font-size: 14px; font-weight: 600; flex: 1 1 auto; }
.status-badge { font-size: 11px; flex-shrink: 0; padding: 2px 8px; border-radius: 10px; font-weight: 600; }
.badge-success { background: var(--ion-color-success, #2dd36f); color: #fff; }
.badge-danger { background: var(--ion-color-danger, #eb445a); color: #fff; }
.badge-medium { background: var(--ion-color-medium, #92949c); color: #fff; }

.card-body { display: flex; flex-direction: column; gap: 8px; padding-top: 4px; }
.status-line { margin: 0; font-size: 13px; font-weight: 500; }
.status-line-warning { color: #e65100; }
.card-body-warning { background: #fff8e1; border-radius: 6px; padding: 8px; }
.card-body-medium { color: var(--ion-color-medium); }

.info-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 6px 14px; }
.info-item { display: flex; flex-direction: column; gap: 1px; }
.info-label { font-size: 11px; color: var(--ion-color-medium); }
.info-value { font-size: 13px; font-weight: 500; font-family: monospace; }
.heartbeat-fresh { color: var(--ion-color-success); }
.card-error { margin-top: 6px; font-size: 11px; color: var(--ion-color-danger); word-break: break-word; }
</style>
