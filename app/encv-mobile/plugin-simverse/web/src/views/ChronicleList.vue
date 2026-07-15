<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button :default-href="backHref" />
        </ion-buttons>
        <ion-title>{{ t("simverse.chronicles") }}</ion-title>
        <ion-buttons slot="end">
          <span class="live-pill"><span class="live-dot" />{{ t("simverse.live") }}</span>
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
      <ion-toolbar>
        <ion-segment :value="filterLevel" @ionChange="onLevelChange">
          <ion-segment-button value="all">
            {{ t("simverse.allEvents") }}
          </ion-segment-button>
          <ion-segment-button value="2">
            {{ t("simverse.important") }}
          </ion-segment-button>
          <ion-segment-button value="4">
            {{ t("simverse.major") }}
          </ion-segment-button>
        </ion-segment>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading" class="loading-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>

      <div v-else-if="error" class="error-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>

      <template v-else>
        <div class="era-header">
          <div class="era-badge">Era {{ era }}</div>
          <div class="event-count">{{ totalEvents }} {{ t("simverse.events") }}</div>
        </div>

        <div class="timeline">
          <div
            v-for="(event, idx) in filteredEvents"
            :key="event.id"
            class="timeline-item"
            :class="`importance-${event.importance}`"
          >
            <div class="timeline-dot">
              <span class="dot-inner"></span>
            </div>
            <div class="timeline-content" @click="goToDetail(event.id)">
              <div class="event-header">
                <ion-badge :color="getImportanceColor(event.importance)" size="small">
                  {{ event.imp_cn }}
                </ion-badge>
                <span class="event-tick">Tick {{ event.tick }}</span>
              </div>
              <h3 class="event-type">{{ event.type_cn }}</h3>
              <p v-if="event.causes && event.causes.length > 0" class="event-causes">
                因: {{ event.causes.slice(0, 2).map(c => c.type_cn).join(", ") }}
              </p>
            </div>
            <div v-if="idx < filteredEvents.length - 1" class="timeline-line"></div>
          </div>
        </div>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonBadge,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonSegment,
  IonSegmentButton,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useLiveRefresh } from "@/composables/useLiveRefresh";
import { type SimverseChronicleEvent, useSimverse } from "@/composables/useSimverse";

const props = defineProps<{
  npcId?: number;
  backHref?: string;
}>();

const { t } = useI18n();
const router = useRouter();
const { loadChronicleWorld, loadChronicleNPC, chronicleSignal } = useSimverse();

const loading = ref(false);
const error = ref("");
const events = ref<SimverseChronicleEvent[]>([]);
const totalEvents = ref(0);
const era = ref(0);
const filterLevel = ref("all");

const filteredEvents = computed(() => {
  if (filterLevel.value === "all") return events.value;
  const minImp = parseInt(filterLevel.value, 10);
  return events.value.filter(e => e.importance >= minImp);
});

async function loadEvents(silent = false) {
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    let data;
    if (props.npcId) {
      data = await loadChronicleNPC(props.npcId, 100);
    } else {
      data = await loadChronicleWorld(1, 100);
    }
    if (data) {
      events.value = data.items || [];
      totalEvents.value = data.count || 0;
      era.value = (data as any).era || 0;
    }
  } catch (e: any) {
    if (silent) console.warn("[simverse] chronicle refresh failed:", e);
    else error.value = e.message || "Failed to load chronicles";
  } finally {
    if (!silent) loading.value = false;
  }
}

function reload() {
  loadEvents();
}

function onLevelChange(e: any) {
  filterLevel.value = e.detail.value;
}

function goToDetail(id: number) {
  router.push(`/chronicle/${id}`);
}

function getImportanceColor(imp: number): string {
  if (imp >= 5) return "danger";
  if (imp >= 3) return "warning";
  if (imp >= 2) return "primary";
  return "medium";
}

onMounted(() => {
  loadEvents();
});

// P7 持续演化：编年史随时间线实时刷新（WS 推送优先，未连接时 8s 兜底轮询）
useLiveRefresh(() => loadEvents(true), { signal: chronicleSignal, pollMs: 8000 });
</script>

<style scoped>
.loading-container,
.error-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.error-container p {
  color: var(--ion-color-danger);
  margin: 0;
}
.era-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px 8px;
}
.era-badge {
  font-size: 14px;
  font-weight: 600;
  color: var(--ion-color-primary);
  background: rgba(79, 140, 255, 0.1);
  padding: 4px 12px;
  border-radius: 12px;
}
.event-count {
  font-size: 13px;
  color: var(--ion-color-medium, #6b7280);
}
.timeline {
  position: relative;
  padding: 8px 0 24px 20px;
}
.timeline-item {
  position: relative;
  padding-left: 24px;
  padding-bottom: 20px;
}
.timeline-dot {
  position: absolute;
  left: 0;
  top: 4px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--ion-background-color, var(--color-white));
  border: 2px solid var(--ion-color-medium, #6b7280);
  z-index: 1;
}
.timeline-item.importance-4 .timeline-dot,
.timeline-item.importance-5 .timeline-dot {
  border-color: var(--ion-color-danger, #ef4444);
}
.timeline-item.importance-3 .timeline-dot {
  border-color: var(--ion-color-warning, #f59e0b);
}
.timeline-item.importance-2 .timeline-dot {
  border-color: var(--ion-color-primary, #4f8cff);
}
.dot-inner {
  display: block;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--ion-color-medium, #6b7280);
  margin: 2px auto;
}
.timeline-item.importance-4 .dot-inner,
.timeline-item.importance-5 .dot-inner {
  background: var(--ion-color-danger, #ef4444);
}
.timeline-item.importance-3 .dot-inner {
  background: var(--ion-color-warning, #f59e0b);
}
.timeline-item.importance-2 .dot-inner {
  background: var(--ion-color-primary, #4f8cff);
}
.timeline-line {
  position: absolute;
  left: 5px;
  top: 16px;
  width: 2px;
  height: calc(100% + 4px);
  background: var(--ion-color-light, #e5e7eb);
}
.live-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  font-weight: 600;
  color: var(--ion-color-success, #22c55e);
  padding: 2px 8px;
  border-radius: 10px;
  background: rgba(34, 197, 94, 0.12);
  margin-right: 4px;
}
.live-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--ion-color-success, #22c55e);
  animation: live-pulse 1.6s ease-in-out infinite;
}
@keyframes live-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
.timeline-content {
  cursor: pointer;
}
.event-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.event-tick {
  font-size: 11px;
  color: var(--ion-color-medium, #6b7280);
  font-family: monospace;
}
.event-type {
  font-size: 14px;
  font-weight: 500;
  margin: 0 0 4px 0;
}
.event-causes {
  font-size: 12px;
  color: var(--ion-color-medium, #6b7280);
  margin: 0;
}
</style>
