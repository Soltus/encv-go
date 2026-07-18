<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.chronicles") }}</ion-title>
        <ion-buttons slot="end">
          <span class="live-pill"><span class="live-dot" />{{ t("simverse.live") }}</span>
          <ion-button @click="loadData" :disabled="loading">
            <ion-icon :icon="refreshIcon" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="page-root p-4 space-y-4">
      <div v-if="loading && !loaded" class="state-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>

      <template v-else-if="loaded">
        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">
              <span>{{ t("simverse.chronicles") }}</span>
              <span class="text-xs text-base-content/70 font-normal">{{ worldData?.count || 0 }}</span>
            </div>
            <div class="space-y-1">
              <div
                v-for="evt in worldData?.items || []"
                :key="evt.id"
                class="chronicle-item flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                :class="`imp-${evt.importance}`"
                @click="openEventDetail(evt)"
              >
                <div class="level-avatar flex-shrink-0" :class="`level-${evt.level}`">
                  <span class="avatar-text">{{ levelIcon(evt.level) }}</span>
                </div>
                <div class="flex-1 min-w-0">
                  <h3 class="text-sm font-medium m-0">{{ evt.type_cn }}</h3>
                  <div class="flex items-center gap-2 mt-1">
                    <span class="ui-chip !text-xs !py-0.5" :class="impChipClass(evt.importance)">
                      {{ evt.imp_cn }}
                    </span>
                    <span class="text-xs text-base-content/70">tick {{ evt.tick }}</span>
                  </div>
                </div>
                <ion-icon :icon="chevronForward" class="text-base-content/40" />
              </div>
              <div v-if="!worldData?.items?.length" class="p-8 text-center text-sm text-base-content/50">
                {{ t("simverse.noData") }}
              </div>
            </div>
          </div>
        </div>
      </template>

      <ion-modal :is-open="showEventModal" @will-dismiss="showEventModal = false">
        <ion-header>
          <ion-toolbar>
            <ion-buttons slot="start">
              <ion-button @click="showEventModal = false">{{ t("simverse.back") }}</ion-button>
            </ion-buttons>
            <ion-title>{{ t("simverse.chronicles") }}</ion-title>
          </ion-toolbar>
        </ion-header>
        <ion-content class="p-4 space-y-4">
          <div v-if="selectedEvent" class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.eventDetail") }}</div>
              <div class="space-y-1">
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.events") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ selectedEvent.type_cn }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.tier") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ selectedEvent.level_cn }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.causal.cause") }}</span>
                  <span class="ui-chip !text-xs !py-0.5" :class="impChipClass(selectedEvent.importance)">
                    {{ selectedEvent.imp_cn }}
                  </span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.tick") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ selectedEvent.tick }}</span>
                </div>
                <div v-if="selectedEvent.entity_id" class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">Entity ID</span>
                  <span class="text-xs text-base-content/70 font-mono">#{{ selectedEvent.entity_id }}</span>
                </div>
              </div>
            </div>
          </div>

          <div v-if="selectedEvent && selectedEvent.causes?.length" class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.causal.cause") }} ({{ selectedEvent.causes.length }})</div>
              <div class="space-y-1">
                <div
                  v-for="cause in selectedEvent.causes"
                  :key="cause.id"
                  class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  @click="loadAndShowEvent(cause.id)"
                >
                  <div class="flex-1 min-w-0">
                    <h3 class="text-sm font-medium m-0">{{ cause.type_cn }}</h3>
                    <p class="text-xs text-base-content/70 m-0 mt-0.5">tick {{ cause.tick }} · {{ cause.level_cn }}</p>
                  </div>
                  <ion-icon :icon="chevronForward" class="text-base-content/40" />
                </div>
              </div>
            </div>
          </div>

          <div v-if="selectedEvent && selectedEvent.effects?.length" class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.causal.effect") }} ({{ selectedEvent.effects.length }})</div>
              <div class="space-y-1">
                <div
                  v-for="eff in selectedEvent.effects"
                  :key="eff.id"
                  class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  @click="loadAndShowEvent(eff.id)"
                >
                  <div class="flex-1 min-w-0">
                    <h3 class="text-sm font-medium m-0">{{ eff.type_cn }}</h3>
                    <p class="text-xs text-base-content/70 m-0 mt-0.5">tick {{ eff.tick }} · {{ eff.level_cn }}</p>
                  </div>
                  <ion-icon :icon="chevronForward" class="text-base-content/40" />
                </div>
              </div>
            </div>
          </div>
        </ion-content>
      </ion-modal>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonModal,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { chevronForward as chevronForwardIcon, refresh } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useGsap } from "@/composables/useGsap";
import { useRouteTransition } from "@/composables/useRouteTransition";
import { useLiveRefresh } from "../composables/useLiveRefresh";
import { type SimverseChronicleEvent, type SimverseChronicleWorldResponse, useSimverse } from "../composables/useSimverse";

const { t } = useI18n();
const { loadChronicleWorld, loadChronicleEvent, chronicleSignal } = useSimverse();
const { gsap } = useGsap();
useRouteTransition();

const loading = ref(false);
const loaded = ref(false);
const worldData = ref<SimverseChronicleWorldResponse | null>(null);
const refreshIcon = refresh;
const chevronForward = chevronForwardIcon;
const showEventModal = ref(false);
const selectedEvent = ref<SimverseChronicleEvent | null>(null);

function levelIcon(level: string): string {
  const map: Record<string, string> = {
    Personal: "个",
    Family: "家",
    Organization: "组",
    Regional: "区",
    World: "世",
  };
  return map[level] || "?";
}

function impChipClass(imp: number): string {
  const classes = [
    "!bg-base-content/15 !text-base-content/70 !border-base-content/20",
    "!bg-tertiary/15 !text-tertiary !border-tertiary/30",
    "!bg-primary/15 !text-primary !border-primary/30",
    "!bg-success/15 !text-success !border-success/30",
    "!bg-warning/15 !text-warning !border-warning/30",
    "!bg-error/15 !text-error !border-error/30",
  ];
  return classes[imp] || classes[0];
}

async function loadData() {
  loading.value = true;
  try {
    const data = await loadChronicleWorld(2, 50);
    worldData.value = data;
    loaded.value = true;
  } catch (e) {
    console.warn("Failed to load chronicle:", e);
  } finally {
    loading.value = false;
  }
}
async function openEventDetail(evt: SimverseChronicleEvent) {
  const detail = await loadChronicleEvent(evt.id);
  if (detail) {
    selectedEvent.value = detail;
    showEventModal.value = true;
  }
}
async function loadAndShowEvent(id: number) {
  const detail = await loadChronicleEvent(id);
  if (detail) selectedEvent.value = detail;
}

onMounted(() => {
  loadData();
  const el = document.querySelector(".page-root");
  if (el) {
    gsap.fromTo(el, { opacity: 0, y: 24 }, { opacity: 1, y: 0, duration: 0.35, ease: "power2.out" });
  }
});

useLiveRefresh(loadData, { signal: chronicleSignal, pollMs: 8000 });
</script>

<style scoped lang="scss">
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px;
  gap: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
}
.level-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 600;
  color: #fff;
  background: var(--color-base-content);
}
.level-avatar.level-Personal { background: var(--color-accent); }
.level-avatar.level-Family { background: var(--color-success); }
.level-avatar.level-Organization { background: var(--color-primary); }
.level-avatar.level-Regional { background: var(--color-warning); }
.level-avatar.level-World { background: var(--color-error); }
.avatar-text { font-size: 12px; }
.live-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  font-weight: 600;
  color: var(--color-success);
  padding: 2px 8px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--color-success) 12%, transparent);
  margin-right: 4px;
}
.live-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-success);
  animation: live-pulse 1.6s ease-in-out infinite;
}
@keyframes live-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
</style>
