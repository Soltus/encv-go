<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('devlogs.title') }}</ion-title>
      </ion-toolbar>
      <ion-toolbar class="tab-toolbar">
        <ion-segment :value="activeTab" @ionChange="onTabChange">
          <ion-segment-button value="frontend" @click="onTabClick('frontend')">
            {{ t('devlogs.frontend') }}
          </ion-segment-button>
          <ion-segment-button value="backend" @click="onTabClick('backend')">
            {{ t('devlogs.backend') }}
          </ion-segment-button>
        </ion-segment>
      </ion-toolbar>
      <div class="toolbar-row">
        <div class="filter-dropdowns">
          <FilterDropdown
            :options="levelDropdownOptions"
            v-model="selectedLevelsArray"
            :label="t('devlogs.level')"
            :multi-select="true"
            :show-actions="true"
            :select-all-text="t('devlogs.selectAll')"
            :clear-all-text="t('devlogs.clearAll')"
            @change="onLevelsChange"
          />
          <FilterDropdown
            :options="tagDropdownOptions"
            v-model="selectedTags"
            :label="t('devlogs.source')"
            :multi-select="true"
            :searchable="true"
            :search-placeholder="t('devlogs.searchTag')"
            :empty-text="t('devlogs.noTags')"
            :show-actions="true"
            :select-all-text="t('devlogs.selectAll')"
            :clear-all-text="t('devlogs.clearAll')"
            :empty-means-all="true"
            @change="onTagsChange"
          />
        </div>
        <div class="toolbar-actions">
          <ion-button
            fill="clear"
            size="small"
            :color="autoScrollEnabled ? 'primary' : 'medium'"
            :title="autoScrollEnabled ? t('devlogs.autoScrollOn') : t('devlogs.autoScrollOff')"
            @click="toggleAutoScroll"
          >
            <ion-icon
              :icon="autoScrollEnabled ? pauseOutline : playOutline"
              slot="icon-only"
            ></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" @click="handleCopy">
            <ion-icon :icon="copyOutline" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" color="danger" @click="handleClear">
            <ion-icon :icon="trashOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </div>
      </div>
      <div class="search-row">
        <ion-searchbar
          v-model="searchText"
          :placeholder="t('devlogs.searchPlaceholder')"
          class="log-searchbar"
          mode="ios"
          :debounce="150"
        ></ion-searchbar>
      </div>
    </ion-header>

    <ion-content ref="contentRef" class="log-content">
      <div v-if="activeTab === 'frontend'" class="log-list">
        <div v-if="filteredFrontend.length === 0" class="empty-logs">
          <p>{{ t('devlogs.noLogs') }}</p>
        </div>
        <VirtualLogList v-if="filteredFrontend.length > 0" :key="'frontend'" :items="filteredFrontend" :scroll-el="scrollEl" @select="onLogSelect">
          <template #default="slotProps">
            <span v-if="slotProps?.item" class="log-source-icon" :title="getSourceIconTitle(slotProps.item)">
              {{ getSourceIcon(slotProps.item) }}
            </span>
            <span v-if="slotProps?.item" class="log-time">[{{ slotProps.item.timestamp }}]</span>
            <ion-badge v-if="slotProps?.item" :color="getBadgeColor(slotProps.item.level)" class="level-badge">{{ slotProps.item.level.toUpperCase() }}</ion-badge>
            <span v-if="slotProps?.item" class="log-msg" v-html="highlightMatch(slotProps.item.message, searchText)"></span>
          </template>
        </VirtualLogList>
      </div>

      <div v-else class="log-list">
        <div class="devlog-status-card-wrap">
          <div class="simple-server-status">
            <ion-icon :icon="serverOnline ? wifiOutline : cloudOfflineOutline" :color="serverOnline ? 'success' : 'medium'" />
            <span>{{ serverOnline ? t('devlogs.connected') : t('devlogs.disconnected') }}</span>
          </div>
        </div>
        <div v-if="backendFilteredItems.length === 0" class="empty-logs">
          <p>{{ t('devlogs.noLogs') }}</p>
        </div>
        <VirtualLogList v-if="backendFilteredItems.length > 0" :key="'backend'" :items="backendFilteredItems" :scroll-el="scrollEl" @select="onLogSelect">
          <template #default="slotProps">
            <span v-if="slotProps?.item" class="log-source-icon" :title="getSourceIconTitle(slotProps.item)">
              {{ getSourceIcon(slotProps.item) }}
            </span>
            <span v-if="slotProps?.item" class="log-time">[{{ slotProps.item.timestamp }}]</span>
            <ion-badge v-if="slotProps?.item" :color="getBadgeColor(slotProps.item.level)" class="level-badge">{{ slotProps.item.level.toUpperCase() }}</ion-badge>
            <span v-if="slotProps?.item" class="log-msg" v-html="highlightMatch(slotProps.item.message, searchText)"></span>
          </template>
        </VirtualLogList>
      </div>
    </ion-content>

    <div class="scroll-buttons">
      <transition name="fade">
        <button
          v-if="showScrollToTop"
          type="button"
          class="scrollToTopBtn"
          :title="t('devlogs.scrollToTop')"
          :aria-label="t('devlogs.scrollToTop')"
          @click="onJumpToTop"
        >
          <ion-icon :icon="arrowUpOutline" class="scrollToTopIcon" />
        </button>
      </transition>
      <transition name="fade">
        <button
          v-if="!autoScrollEnabled"
          type="button"
          class="scrollToBottomBtn"
          :title="t('devlogs.scrollToBottom')"
          :aria-label="t('devlogs.scrollToBottom')"
          @click="onJumpToBottom"
        >
          <ion-icon :icon="arrowDownOutline" class="scrollToBottomIcon" />
        </button>
      </transition>
    </div>

    <ion-footer class="status-bar">
      <ion-toolbar>
        <div class="status-inner">
          <span class="status-text">{{ t('devlogs.total', { total: String(totalCurrent), filtered: String(filteredCurrent) }) }}</span>
          <span class="status-text auto-scroll-status" :class="{ paused: !autoScrollEnabled }">
            {{ autoScrollEnabled ? t('devlogs.autoScrollOn') : t('devlogs.autoScrollOff') }}
          </span>
        </div>
      </ion-toolbar>
    </ion-footer>

    <div v-if="selectedLog" class="log-detail-overlay" @click.self="closeLogDetail">
      <div class="log-detail-modal" role="dialog" aria-modal="true">
        <div class="log-detail-header">
          <h3 class="log-detail-title">{{ t('devlogs.logDetail') }}</h3>
          <button type="button" class="log-detail-close" :aria-label="t('devlogs.logDetailClose')" @click="closeLogDetail">
            <ion-icon :icon="closeOutline" />
          </button>
        </div>
        <div class="log-detail-body">
          <div class="log-detail-row log-detail-meta-row">
            <div class="log-detail-meta-item">
              <span class="log-detail-label">{{ t('devlogs.logDetailTimestamp') }}</span>
              <span class="log-detail-value log-time-detail">{{ selectedLog.timestamp }}</span>
            </div>
            <div class="log-detail-meta-item">
              <span class="log-detail-label">{{ t('devlogs.logDetailLevel') }}</span>
              <ion-badge :color="getBadgeColor(selectedLog.level)" class="level-badge">
                {{ selectedLog.level.toUpperCase() }}
              </ion-badge>
            </div>
          </div>
          <div v-if="selectedLog.source" class="log-detail-row">
            <span class="log-detail-label">{{ t('devlogs.logDetailSource') }}</span>
            <span class="log-detail-value log-source-detail">{{ selectedLog.source }}</span>
          </div>
          <div v-if="selectedLog.tags && selectedLog.tags.length > 0" class="log-detail-row">
            <span class="log-detail-label">{{ t('devlogs.logDetailTags') }}</span>
            <div class="log-detail-tags">
              <span
                v-for="tag in selectedLog.tags"
                :key="tag"
                class="log-tag-chip"
                @click="onTagClick(tag)"
              >{{ tag }}</span>
            </div>
          </div>
          <div class="log-detail-row log-detail-message-row">
            <span class="log-detail-label">{{ t('devlogs.logDetailMessage') }}</span>
            <pre class="log-detail-message">{{ selectedLog.message }}</pre>
          </div>
          <div v-if="selectedLog.level === 'error'" class="log-detail-row log-detail-stack-row">
            <span class="log-detail-label">{{ t('devlogs.logDetailStack') }}</span>
            <pre v-if="selectedLog.stack" class="log-detail-stack">{{ selectedLog.stack }}</pre>
            <pre v-else class="log-detail-stack log-detail-stack-empty">(no stack trace available)</pre>
          </div>
        </div>
        <div class="log-detail-footer">
          <ion-button fill="outline" size="small" @click="copyLogDetail">
            <ion-icon :icon="copyOutline" slot="start" />
            {{ t('devlogs.logDetailCopy') }}
          </ion-button>
          <ion-button fill="clear" size="small" @click="closeLogDetail">
            {{ t('devlogs.logDetailClose') }}
          </ion-button>
        </div>
      </div>
    </div>
  </ion-page>
</template>

<script setup lang="ts">
import type { DropdownOption } from "@encv/shared-components/components/shared/FilterDropdown.vue";
import FilterDropdown from "@encv/shared-components/components/shared/FilterDropdown.vue";
import VirtualLogList from "@encv/shared-components/components/VirtualLogList.vue";
import { copyToClipboard } from "@encv/shared-components/composables/useClipboard";
import type { LogEntry } from "@encv/shared-components/composables/useFrontendLogs";
import { useFrontendLogs } from "@encv/shared-components/composables/useFrontendLogs";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";
import { useIonContentScroll } from "@encv/shared-components/composables/useIonContentScroll";
import { type IonContent, IonIcon } from "@ionic/vue";
import { useConfirmDialog } from "@encv/shared-components/composables/useConfirmDialog";
import {
  arrowDownOutline,
  arrowUpOutline,
  closeOutline,
  cloudOfflineOutline,
  copyOutline,
  pauseOutline,
  playOutline,
  trashOutline,
  wifiOutline,
} from "ionicons/icons";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { logs: frontendLogs, clearLogs: clearFrontendLogs } = useFrontendLogs();
const { loadChronicleWorld, loadWorldState: fetchWorldState, isConnected } = useSimverse();

const serverOnline = isConnected;

const activeTab = ref<"frontend" | "backend">("frontend");
const searchText = ref("");
const autoScrollEnabled = ref(true);
const contentRef = ref<InstanceType<typeof IonContent> | null>(null);
// K42：统一走 useIonContentScroll 取 ion-content 滚动元素（含重试 / ResizeObserver 兜底）
const { scrollEl, initScrollElWithRetry } = useIonContentScroll(contentRef);
const selectedLog = ref<LogEntry | null>(null);
const showScrollToTop = ref(false);

const selectedLevels = ref<Set<string>>(new Set(["debug", "info", "warn", "error"]));
const selectedTags = ref<string[]>([]);
const tagDropdownOptions = ref<DropdownOption[]>([]);

const levelDropdownOptions: DropdownOption[] = [
  { value: "debug", label: "DEBUG" },
  { value: "info", label: "INFO" },
  { value: "warn", label: "WARN" },
  { value: "error", label: "ERROR" },
];

const selectedLevelsArray = computed(() => Array.from(selectedLevels.value));

const backendLogs = ref<LogEntry[]>([]);
let backendLogId = 0;
let pollInterval: number | null = null;
let lastEventId = "";

const filteredFrontend = computed(() => {
  let items = frontendLogs.value;
  if (selectedLevels.value.size > 0) {
    items = items.filter(l => selectedLevels.value.has(l.level));
  }
  if (selectedTags.value.length > 0) {
    items = items.filter(l => l.tags?.some(t => selectedTags.value.includes(t)));
  }
  if (searchText.value) {
    const q = searchText.value.toLowerCase();
    items = items.filter(l => l.message.toLowerCase().includes(q));
  }
  return items;
});

const backendFilteredItems = computed(() => {
  let items = backendLogs.value;
  if (selectedLevels.value.size > 0) {
    items = items.filter(l => selectedLevels.value.has(l.level));
  }
  if (selectedTags.value.length > 0) {
    items = items.filter(l => l.tags?.some(t => selectedTags.value.includes(t)));
  }
  if (searchText.value) {
    const q = searchText.value.toLowerCase();
    items = items.filter(l => l.message.toLowerCase().includes(q));
  }
  return items;
});

const totalCurrent = computed(() => (activeTab.value === "frontend" ? frontendLogs.value.length : backendLogs.value.length));

const filteredCurrent = computed(() =>
  activeTab.value === "frontend" ? filteredFrontend.value.length : backendFilteredItems.value.length
);

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
  if (log.source?.includes("error")) return "❌";
  if (log.level === "error") return "❌";
  if (log.level === "warn") return "⚠️";
  if (log.level === "debug") return "🔧";
  return "ℹ️";
}

function getSourceIconTitle(log: LogEntry): string {
  return log.source || log.level;
}

function highlightMatch(text: string, query: string): string {
  if (!query) return escapeHtml(text);
  const escaped = escapeHtml(text);
  const q = escapeHtml(query);
  const regex = new RegExp(`(${q.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "gi");
  return escaped.replace(regex, '<mark class="search-highlight">$1</mark>');
}

function escapeHtml(text: string): string {
  const div = document.createElement("div");
  div.textContent = text;
  return div.innerHTML;
}

function onTabChange(e: any) {
  activeTab.value = e.detail.value;
  nextTick(() => scrollToBottom());
}

function onTabClick(tab: string) {
  activeTab.value = tab as "frontend" | "backend";
}

function onLevelsChange(values: string[]) {
  selectedLevels.value = new Set(values);
}

function onTagsChange() {}

function toggleAutoScroll() {
  autoScrollEnabled.value = !autoScrollEnabled.value;
  if (autoScrollEnabled.value) {
    scrollToBottom();
  }
}

async function scrollToBottom() {
  if (!contentRef.value) return;
  try {
    await (contentRef.value as any).scrollToBottom(0);
  } catch {}
}

function onJumpToTop() {
  (contentRef.value as any)?.scrollToTop(0);
}

function onJumpToBottom() {
  autoScrollEnabled.value = true;
  scrollToBottom();
}

function onLogSelect(log: LogEntry) {
  selectedLog.value = log;
}

function closeLogDetail() {
  selectedLog.value = null;
}

function onTagClick(tag: string) {
  if (!selectedTags.value.includes(tag)) {
    selectedTags.value = [...selectedTags.value, tag];
  }
}

async function copyLogDetail() {
  if (!selectedLog.value) return;
  const text = `[${selectedLog.value.timestamp}] ${selectedLog.value.level.toUpperCase()}\n${selectedLog.value.message}${selectedLog.value.stack ? "\n" + selectedLog.value.stack : ""}`;
  const ok = await copyToClipboard(text);
  showToast(ok ? { message: t("devlogs.logDetailCopied") } : { message: t("devlogs.copyFailed"), color: "danger" });
}

async function handleCopy() {
  const items = activeTab.value === "frontend" ? filteredFrontend.value : backendFilteredItems.value;
  const text = items.map(l => `[${l.timestamp}] ${l.level.toUpperCase()} ${l.message}`).join("\n");
  const ok = await copyToClipboard(text);
  showToast(ok ? { message: t("devlogs.copied", { count: String(items.length) }) } : { message: t("devlogs.copyFailed"), color: "danger" });
}

async function handleClear() {
  if (
    await useConfirmDialog().confirm({
      header: t("devlogs.clear"),
      message: t("devlogs.clearConfirm"),
      confirmText: t("devlogs.clear"),
      danger: true,
    })
  ) {
    if (activeTab.value === "frontend") {
      clearFrontendLogs();
    } else {
      backendLogs.value = [];
      lastEventId = "";
    }
  }
}

function chronicleLevelToLogLevel(level: string): string {
  if (level === "critical" || level === "catastrophe") return "error";
  if (level === "major" || level === "minor") return "warn";
  if (level === "trivial") return "debug";
  return "info";
}

async function loadBackendLogs() {
  try {
    const data = await loadChronicleWorld(0, 50);
    const events = data?.items || [];
    const newLogs: LogEntry[] = [];

    for (const evt of events) {
      const evtId = String(evt.id || "");
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
        lastEventId = String(events[0].id || "");
      }
      if (autoScrollEnabled.value && activeTab.value === "backend") {
        nextTick(() => scrollToBottom());
      }
      updateTagOptions();
    }
  } catch (e) {
    // 静默失败，避免污染日志
  }
}

function updateTagOptions() {
  const tagSet = new Set<string>();
  for (const log of backendLogs.value) {
    if (log.tags) {
      for (const t of log.tags) tagSet.add(t);
    }
  }
  tagDropdownOptions.value = Array.from(tagSet).map(t => ({ value: t, label: t }));
}

// scroll 元素就绪后挂载滚动监听（K42：元素经 useIonContentScroll 异步解析）
watch(
  scrollEl,
  el => {
    if (el) el.addEventListener("scroll", onScroll);
  },
  { immediate: true }
);

function onScroll() {
  if (!scrollEl.value) return;
  const { scrollTop, scrollHeight, clientHeight } = scrollEl.value;
  showScrollToTop.value = scrollTop > 200;

  if (autoScrollEnabled.value) {
    const atBottom = scrollHeight - scrollTop - clientHeight < 50;
    if (!atBottom) {
      autoScrollEnabled.value = false;
    }
  }
}

onMounted(async () => {
  initScrollElWithRetry();
  await loadBackendLogs();
  pollInterval = window.setInterval(() => {
    if (activeTab.value === "backend") {
      loadBackendLogs();
    }
  }, 5000);
  scrollToBottom();
});

onBeforeUnmount(() => {
  if (pollInterval) clearInterval(pollInterval);
  if (scrollEl.value) {
    scrollEl.value.removeEventListener("scroll", onScroll);
  }
});

watch(activeTab, () => {
  nextTick(() => {
    if (autoScrollEnabled.value) scrollToBottom();
  });
});
</script>

<style scoped>
.tab-toolbar {
  --padding-start: 12px;
  --padding-end: 12px;
  --min-height: 44px;
}

.toolbar-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 12px 8px;
  gap: 8px;
  min-height: 40px;
}

.filter-dropdowns {
  display: flex;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.toolbar-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.search-row {
  padding: 0 8px 8px;
}

.log-searchbar {
  padding-top: 0;
  padding-bottom: 0;
}

.log-content {
  --background: var(--ion-background-color);
}

.log-list {
  height: 100%;
}

.empty-logs {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 200px;
  color: var(--ion-color-medium);
  font-size: 14px;
}

.log-source-icon {
  flex-shrink: 0;
  font-size: 12px;
  width: 16px;
  text-align: center;
}

.log-time {
  color: var(--ion-color-medium);
  font-size: 11px;
  flex-shrink: 0;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
}

.level-badge {
  --padding-start: 4px;
  --padding-end: 4px;
  --padding-top: 0;
  --padding-bottom: 0;
  font-size: 9px;
  flex-shrink: 0;
}

.log-msg {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
  line-height: 1.4;
}

:deep(.search-highlight) {
  background: rgba(245, 158, 11, 0.3);
  border-radius: 2px;
  padding: 0 2px;
}

.scroll-buttons {
  position: fixed;
  right: 16px;
  bottom: 80px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  z-index: 100;
}

.scrollToTopBtn,
.scrollToBottomBtn {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  border: none;
  background: var(--ion-color-primary);
  color: white;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
  transition: all 0.2s;
}

.scrollToTopBtn:active,
.scrollToBottomBtn:active {
  transform: scale(0.95);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.status-bar {
  --background: var(--ion-toolbar-background);
  border-top: 1px solid var(--ion-color-step-150);
}

.status-inner {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 16px;
  font-size: 12px;
  color: var(--ion-color-medium);
}

.auto-scroll-status {
  font-weight: 500;
  color: var(--ion-color-primary);
}

.auto-scroll-status.paused {
  color: var(--ion-color-warning);
}

.devlog-status-card-wrap {
  padding: 8px 12px 4px;
}

.log-detail-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.log-detail-modal {
  width: 100%;
  max-width: 500px;
  max-height: 80vh;
  background: var(--ion-background-color);
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.log-detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  border-bottom: 1px solid var(--ion-color-step-100);
  flex-shrink: 0;
}

.log-detail-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.log-detail-close {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: none;
  background: var(--ion-color-step-100);
  color: var(--ion-color-medium);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

.log-detail-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}

.log-detail-row {
  margin-bottom: 12px;
}

.log-detail-meta-row {
  display: flex;
  gap: 16px;
}

.log-detail-meta-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.log-detail-label {
  font-size: 11px;
  color: var(--ion-color-medium);
  font-weight: 500;
}

.log-detail-value {
  font-size: 13px;
  color: var(--ion-text-color);
}

.log-time-detail {
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
}

.log-source-detail {
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
  color: var(--ion-color-primary);
}

.log-detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.log-tag-chip {
  padding: 2px 8px;
  background: var(--ion-color-step-100);
  border-radius: 10px;
  font-size: 11px;
  cursor: pointer;
  color: var(--ion-color-medium);
  transition: all 0.2s;
}

.log-tag-chip:hover {
  background: var(--ion-color-step-200);
  color: var(--ion-text-color);
}

.log-detail-message {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
  white-space: pre-wrap;
  word-break: break-all;
  background: var(--ion-color-step-50);
  padding: 12px;
  border-radius: 8px;
}

.log-detail-stack {
  margin: 0;
  font-size: 11px;
  line-height: 1.5;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
  white-space: pre-wrap;
  word-break: break-all;
  background: rgba(var(--ion-color-danger-rgb), 0.05);
  padding: 12px;
  border-radius: 8px;
  color: var(--ion-color-danger);
}

.log-detail-stack-empty {
  color: var(--ion-color-medium);
  font-style: italic;
  background: var(--ion-color-step-50);
}

.log-detail-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--ion-color-step-100);
  flex-shrink: 0;
}
</style>
