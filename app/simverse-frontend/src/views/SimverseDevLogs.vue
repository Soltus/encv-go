<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>{{ t("simverse.devlogs.title") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="clearLogs">
            <ion-icon :icon="trashOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>

      <ion-toolbar>
        <ion-searchbar
          :placeholder="t('simverse.devlogs.search')"
          v-model="searchText"
        />
      </ion-toolbar>

      <ion-segment :value="activeTab" @ionChange="onTabChange">
        <ion-segment-button value="frontend">
          <ion-label>{{ t("simverse.devlogs.frontend") }}</ion-label>
        </ion-segment-button>
        <ion-segment-button value="backend">
          <ion-label>{{ t("simverse.devlogs.backend") }}</ion-label>
        </ion-segment-button>
      </ion-segment>
    </ion-header>

    <ion-content class="devlogs-content">
      <div v-if="filteredLogs.length === 0" class="empty-logs">
        <ion-icon :icon="documentTextOutline" class="empty-icon" />
        <p>{{ t("simverse.devlogs.noLogs") }}</p>
      </div>

      <div v-else class="log-list">
        <div
          v-for="log in filteredLogs"
          :key="log.id"
          class="log-entry"
          :class="log.level"
        >
          <span class="log-time">[{{ log.timestamp }}]</span>
          <ion-badge :color="getBadgeColor(log.level)" class="level-badge">
            {{ log.level.toUpperCase() }}
          </ion-badge>
          <span class="log-msg">{{ log.message }}</span>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { trashOutline, documentTextOutline } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();

interface LogEntry {
  id: number;
  timestamp: string;
  level: "info" | "warn" | "error" | "debug";
  source: string;
  message: string;
}

const activeTab = ref<"frontend" | "backend">("frontend");
const searchText = ref("");

const frontendLogs = ref<LogEntry[]>([]);
const backendLogs = ref<LogEntry[]>([]);

let logIdCounter = 0;

function addLog(level: LogEntry["level"], source: string, message: string, target: "frontend" | "backend") {
  const log: LogEntry = {
    id: ++logIdCounter,
    timestamp: new Date().toLocaleTimeString(),
    level,
    source,
    message,
  };
  if (target === "frontend") {
    frontendLogs.value.push(log);
  } else {
    backendLogs.value.push(log);
  }
}

function clearLogs() {
  if (activeTab.value === "frontend") {
    frontendLogs.value = [];
  } else {
    backendLogs.value = [];
  }
}

function onTabChange(e: any) {
  activeTab.value = e.detail.value;
}

const filteredLogs = computed(() => {
  const logs = activeTab.value === "frontend" ? frontendLogs.value : backendLogs.value;
  if (!searchText.value) return logs;
  const q = searchText.value.toLowerCase();
  return logs.filter(l =>
    l.message.toLowerCase().includes(q) ||
    l.source.toLowerCase().includes(q) ||
    l.level.includes(q)
  );
});

function getBadgeColor(level: string): string {
  switch (level) {
    case "error": return "danger";
    case "warn": return "warning";
    case "info": return "primary";
    case "debug": return "medium";
    default: return "medium";
  }
}

onMounted(() => {
  addLog("info", "simverse", "Simverse 前端已启动", "frontend");
  addLog("info", "simverse", "物理引擎初始化完成", "frontend");
  addLog("debug", "renderer", "LeaferUI 渲染器就绪", "frontend");
  addLog("warn", "physics", "重力值偏高，建议调至 50% 以下", "frontend");
  addLog("info", "simverse", "世界加载中...", "backend");
  addLog("info", "simverse", "已生成 128 个实体", "backend");
  addLog("error", "simverse", "（示例）实体 42 碰撞检测异常", "backend");
});
</script>

<style scoped>
.devlogs-content {
  --background: var(--ion-background-color);
}

.empty-logs {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: var(--ion-color-medium);
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.empty-logs p {
  margin: 0;
  font-size: 14px;
}

.log-list {
  padding: 4px 0;
}

.log-entry {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 12px;
  line-height: 1.5;
  border-bottom: 1px solid var(--ion-color-step-50);
}

.log-entry.error {
  background: rgba(var(--ion-color-danger-rgb), 0.05);
}

.log-entry.warn {
  background: rgba(var(--ion-color-warning-rgb), 0.05);
}

.log-time {
  color: var(--ion-color-medium);
  white-space: nowrap;
  font-size: 11px;
}

.level-badge {
  --padding-start: 4px;
  --padding-end: 4px;
  --padding-top: 0;
  --padding-bottom: 0;
  font-size: 9px;
}

.log-msg {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
