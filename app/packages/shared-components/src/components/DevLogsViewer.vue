<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ title || t('devlogs.title') }}</ion-title>
        <slot name="toolbar-end" />
      </ion-toolbar>
      <ion-toolbar v-if="tabs && tabs.length > 1" class="tab-toolbar">
        <ion-segment :value="activeTab" @ionChange="onTabChange">
          <ion-segment-button
            v-for="tab in tabs"
            :key="tab.value"
            :value="tab.value"
            @click="onTabClick(tab.value)"
          >
            {{ tab.label }}
          </ion-segment-button>
        </ion-segment>
      </ion-toolbar>
      <div class="toolbar-row">
        <div class="filter-dropdowns">
          <FilterDropdown
            :options="levelOptions"
            v-model="selectedLevelsArray"
            :label="t('devlogs.level')"
            :multi-select="true"
            :show-actions="true"
            :select-all-text="t('devlogs.selectAll')"
            :clear-all-text="t('devlogs.clearAll')"
            @change="onLevelsChange"
          />
          <FilterDropdown
            v-if="showTagFilter"
            :options="tagOptions"
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
          <slot name="actions" />
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
      <div v-for="tab in tabs" :key="tab.value" class="log-list" v-show="activeTab === tab.value">
        <slot name="status" :tab="tab.value" />
        <div v-if="getFilteredItems(tab.value).length === 0" class="empty-logs">
          <p>{{ t('devlogs.noLogs') }}</p>
        </div>
        <VirtualLogList
          v-if="getFilteredItems(tab.value).length > 0"
          :key="tab.value"
          :items="getFilteredItems(tab.value)"
          :scroll-el="scrollEl"
          @select="onLogSelect"
        >
          <template #default="slotProps">
            <slot name="log-item" :item="slotProps.item" :index="slotProps.index" :tab="tab.value">
              <span v-if="slotProps?.item" class="log-time">[{{ slotProps.item.timestamp }}]</span>
              <ion-badge
                v-if="slotProps?.item"
                :color="getBadgeColor(slotProps.item.level)"
                class="level-badge"
              >{{ slotProps.item.level.toUpperCase() }}</ion-badge>
              <span v-if="slotProps?.item" class="log-msg">{{ slotProps.item.message }}</span>
            </slot>
          </template>
        </VirtualLogList>
      </div>

      <div class="scroll-buttons">
        <transition name="fade">
          <button
            v-if="showScrollToTop"
            type="button"
            class="scroll-btn scroll-top-btn"
            :title="t('devlogs.scrollToTop')"
            @click="scrollToTop"
          >
            <ion-icon :icon="arrowUpOutline" />
          </button>
        </transition>
        <transition name="fade">
          <button
            v-if="showScrollToBottom"
            type="button"
            class="scroll-btn scroll-bottom-btn"
            :title="t('devlogs.scrollToBottom')"
            @click="scrollToBottom"
          >
            <ion-icon :icon="arrowDownOutline" />
          </button>
        </transition>
      </div>
    </ion-content>

    <div v-if="selectedLog" class="log-detail-overlay" @click.self="closeLogDetail">
      <div class="log-detail-modal">
        <div class="log-detail-header">
          <h3>{{ t('devlogs.logDetail') }}</h3>
          <ion-button fill="clear" size="small" @click="closeLogDetail">
            <ion-icon :icon="closeOutline" slot="icon-only" />
          </ion-button>
        </div>
        <div class="log-detail-body">
          <slot name="log-detail" :log="selectedLog">
            <div v-if="selectedLog" class="log-detail-row">
              <span class="log-detail-label">{{ t('devlogs.logDetailTimestamp') }}</span>
              <span class="log-detail-value">{{ selectedLog.timestamp }}</span>
            </div>
            <div v-if="selectedLog" class="log-detail-row">
              <span class="log-detail-label">{{ t('devlogs.logDetailLevel') }}</span>
              <ion-badge :color="getBadgeColor(selectedLog.level)">
                {{ selectedLog.level.toUpperCase() }}
              </ion-badge>
            </div>
            <div v-if="selectedLog" class="log-detail-row log-detail-message-row">
              <span class="log-detail-label">{{ t('devlogs.logDetailMessage') }}</span>
              <pre class="log-detail-message">{{ selectedLog.message }}</pre>
            </div>
          </slot>
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

<script setup lang="ts" generic="T extends { id: number; level: string; message: string; timestamp: string; tags?: string[] }">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonButton, IonIcon,
  IonSegment, IonSegmentButton, IonSearchbar, IonContent, IonBadge, IonSpinner,
} from "@ionic/vue";
import { playOutline, pauseOutline, copyOutline, trashOutline, arrowUpOutline, arrowDownOutline, closeOutline } from "ionicons/icons";
import { useI18n } from "../composables/useI18n";
import { copyToClipboard } from "../composables/useClipboard";
import { showToast } from "../composables/useToast";
import VirtualLogList from "./VirtualLogList.vue";
import FilterDropdown from "./shared/FilterDropdown.vue";
import type { DropdownOption } from "./shared/FilterDropdown.vue";

interface LogTab {
  value: string;
  label: string;
}

interface Props {
  title?: string;
  tabs?: LogTab[];
  items?: readonly T[];
  levelOptions?: DropdownOption[];
  tagOptions?: DropdownOption[];
  showTagFilter?: boolean;
}
const props = withDefaults(defineProps<Props>(), {
  tabs: () => [
    { value: "frontend", label: "Frontend" },
    { value: "backend", label: "Backend" },
  ],
  items: () => [],
  levelOptions: () => [
    { value: "debug", label: "DEBUG" },
    { value: "info", label: "INFO" },
    { value: "warn", label: "WARN" },
    { value: "error", label: "ERROR" },
  ],
  tagOptions: () => [],
  showTagFilter: true,
});

const emit = defineEmits<{
  (e: "tab-change", tab: string): void
  (e: "clear"): void
  (e: "copy", items: readonly T[]): void
  (e: "select", item: T): void
}>();

const { t } = useI18n();

const activeTab = ref(props.tabs[0]?.value || "frontend");
const searchText = ref("");
const autoScrollEnabled = ref(true);
const contentRef = ref<InstanceType<typeof IonContent> | null>(null);
const scrollEl = ref<HTMLElement | null>(null);
const selectedLog = ref<T | null>(null);
const showScrollToTop = ref(false);
const showScrollToBottom = ref(false);

const selectedLevels = ref<Set<string>>(new Set(["debug", "info", "warn", "error"]));
const selectedTags = ref<string[]>([]);

const selectedLevelsArray = computed(() => Array.from(selectedLevels.value));

const filteredItems = computed(() => {
  let items = props.items;
  if (selectedLevels.value.size > 0) {
    items = items.filter((l) => selectedLevels.value.has(l.level));
  }
  if (selectedTags.value.length > 0) {
    items = items.filter((l) => l.tags?.some((t: string) => selectedTags.value.includes(t)));
  }
  if (searchText.value) {
    const q = searchText.value.toLowerCase();
    items = items.filter((l) => l.message.toLowerCase().includes(q));
  }
  return items;
});

function getFilteredItems(_tab: string): readonly T[] {
  return filteredItems.value;
}

function onTabChange(e: any) {
  activeTab.value = e.detail.value;
  emit("tab-change", e.detail.value);
}
function onTabClick(tab: string) {
  activeTab.value = tab;
  emit("tab-change", tab);
}

function onLevelsChange(values: string[]) {
  selectedLevels.value = new Set(values);
}
function onTagsChange(values: string[]) {
  selectedTags.value = values;
}

function onLogSelect(item: T) {
  selectedLog.value = item;
  emit("select", item);
}
function closeLogDetail() {
  selectedLog.value = null;
}

function getBadgeColor(level: string): string {
  switch (level) {
    case "error": return "danger";
    case "warn": return "warning";
    case "info": return "primary";
    case "debug": return "medium";
    default: return "medium";
  }
}

function toggleAutoScroll() {
  autoScrollEnabled.value = !autoScrollEnabled.value;
  if (autoScrollEnabled.value) {
    scrollToBottom();
  }
}

async function scrollToBottom() {
  if (!scrollEl.value) return;
  scrollEl.value.scrollTop = scrollEl.value.scrollHeight;
}
async function scrollToTop() {
  if (!scrollEl.value) return;
  scrollEl.value.scrollTop = 0;
}

function handleScroll() {
  if (!scrollEl.value) return;
  const el = scrollEl.value;
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
  const atTop = el.scrollTop < 50;
  showScrollToTop.value = !atTop;
  showScrollToBottom.value = !atBottom && !autoScrollEnabled.value;
}

async function ensureScrollEl() {
  if (!contentRef.value) return;
  try {
    const el = await (contentRef.value as any).getScrollElement();
    scrollEl.value = el;
    el.addEventListener("scroll", handleScroll);
  } catch (e) {
    console.warn("[DevLogsViewer] getScrollElement failed:", e);
  }
}

async function handleCopy() {
  const items = filteredItems.value;
  if (items.length === 0) return;
  const text = items.map((l) => `[${l.timestamp}] [${l.level.toUpperCase()}] ${l.message}`).join("\n");
  try {
    await copyToClipboard(text);
    showToast({ message: t("devlogs.copied", { count: String(items.length) }) });
    emit("copy", items);
  } catch {
    showToast({ message: t("devlogs.copyFailed"), color: "danger" });
  }
}

function copyLogDetail() {
  if (!selectedLog.value) return;
  const log = selectedLog.value;
  const text = `[${log.timestamp}] [${log.level.toUpperCase()}] ${log.message}`;
  copyToClipboard(text).then(() => {
    showToast({ message: t("devlogs.logDetailCopied") });
  });
}

function handleClear() {
  emit("clear");
}

watch(
  () => filteredItems.value.length,
  () => {
    if (autoScrollEnabled.value) {
      nextTick(() => scrollToBottom());
    }
  }
);

onMounted(() => {
  ensureScrollEl();
});

onBeforeUnmount(() => {
  if (scrollEl.value) {
    scrollEl.value.removeEventListener("scroll", handleScroll);
  }
});

defineExpose({
  scrollToBottom,
  scrollToTop,
  contentRef,
  scrollEl,
});
</script>

<style scoped>
.tab-toolbar {
  --padding-start: 12px;
  --padding-end: 12px;
  min-height: 44px;
}
.toolbar-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px 8px;
  gap: 8px;
}
.filter-dropdowns {
  display: flex;
  gap: 8px;
  flex: 1;
}
.toolbar-actions {
  display: flex;
  gap: 4px;
}
.search-row {
  padding: 0 12px 8px;
}
.log-searchbar {
  --padding-start: 0;
  --padding-end: 0;
}
.log-content {
  --background: var(--ion-background-color, #fff);
}
.log-list {
  position: relative;
  min-height: 100%;
}
.empty-logs {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: var(--ion-color-medium, #6b7280);
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
.scroll-buttons {
  position: fixed;
  right: 16px;
  bottom: 24px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  z-index: 100;
}
.scroll-btn {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  border: none;
  background: var(--ion-color-primary, #4f8cff);
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  font-size: 18px;
}
.scroll-btn:active {
  transform: scale(0.95);
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
.log-detail-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}
.log-detail-modal {
  background: var(--ion-background-color, #fff);
  border-radius: 12px;
  width: 100%;
  max-width: 600px;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.log-detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--ion-color-light, #f3f4f6);
}
.log-detail-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
.log-detail-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
}
.log-detail-row {
  display: flex;
  gap: 12px;
  margin-bottom: 12px;
  align-items: flex-start;
}
.log-detail-label {
  width: 80px;
  flex-shrink: 0;
  color: var(--ion-color-medium, #6b7280);
  font-size: 13px;
  padding-top: 2px;
}
.log-detail-value {
  flex: 1;
  font-size: 14px;
  word-break: break-all;
}
.log-detail-message-row {
  flex-direction: column;
  gap: 4px;
}
.log-detail-message {
  background: var(--ion-color-light, #f3f4f6);
  padding: 12px;
  border-radius: 8px;
  font-family: monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
.log-detail-footer {
  display: flex;
  gap: 8px;
  padding: 12px 20px;
  border-top: 1px solid var(--ion-color-light, #f3f4f6);
  justify-content: flex-end;
}
</style>
