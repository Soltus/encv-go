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
      <div class="page-root">
      <div v-if="loading && !loaded" class="state-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>

      <template v-else-if="loaded">
        <ion-list>
          <ion-list-header>
            <ion-label>{{ t("simverse.chronicles") }}</ion-label>
            <ion-note slot="end">{{ worldData?.count || 0 }}</ion-note>
          </ion-list-header>
          <ion-item
            v-for="evt in worldData?.items || []"
            :key="evt.id"
            class="chronicle-item"
            :class="`imp-${evt.importance}`"
            button
            @click="openEventDetail(evt)"
          >
            <ion-avatar slot="start" class="level-avatar" :class="`level-${evt.level}`">
              <span class="avatar-text">{{ levelIcon(evt.level) }}</span>
            </ion-avatar>
            <ion-label class="ion-text-wrap">
              <h3>{{ evt.type_cn }}</h3>
              <p>
                <ion-badge :color="impBadgeColor(evt.importance)" class="imp-badge">
                  {{ evt.imp_cn }}
                </ion-badge>
                <span class="tick-info">tick {{ evt.tick }}</span>
              </p>
            </ion-label>
            <ion-icon :icon="chevronForward" slot="end" />
          </ion-item>
          <ion-item v-if="!worldData?.items?.length" class="empty-item">
            <ion-label class="ion-text-center">{{ t("simverse.noData") }}</ion-label>
          </ion-item>
        </ion-list>
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
        <ion-content>
          <ion-list v-if="selectedEvent">
            <ion-item>
              <ion-label>{{ t("simverse.events") }}</ion-label>
              <ion-note slot="end">{{ selectedEvent.type_cn }}</ion-note>
            </ion-item>
            <ion-item>
              <ion-label>{{ t("simverse.perf.tier") }}</ion-label>
              <ion-note slot="end">{{ selectedEvent.level_cn }}</ion-note>
            </ion-item>
            <ion-item>
              <ion-label>{{ t("simverse.causal.cause") }}</ion-label>
              <ion-note slot="end">
                <ion-badge :color="impBadgeColor(selectedEvent.importance)">{{ selectedEvent.imp_cn }}</ion-badge>
              </ion-note>
            </ion-item>
            <ion-item>
              <ion-label>{{ t("simverse.tick") }}</ion-label>
              <ion-note slot="end">{{ selectedEvent.tick }}</ion-note>
            </ion-item>
            <ion-item v-if="selectedEvent.entity_id">
              <ion-label>#{{ selectedEvent.entity_id }}</ion-label>
            </ion-item>
          </ion-list>

          <ion-list v-if="selectedEvent && selectedEvent.causes?.length">
            <ion-list-header><ion-label>{{ t("simverse.causal.cause") }} ({{ selectedEvent.causes.length }})</ion-label></ion-list-header>
            <ion-item v-for="cause in selectedEvent.causes" :key="cause.id" button @click="loadAndShowEvent(cause.id)">
              <ion-label class="ion-text-wrap">
                <h3>{{ cause.type_cn }}</h3>
                <p>tick {{ cause.tick }} · {{ cause.level_cn }}</p>
              </ion-label>
              <ion-icon :icon="chevronForward" slot="end" />
            </ion-item>
          </ion-list>

          <ion-list v-if="selectedEvent && selectedEvent.effects?.length">
            <ion-list-header><ion-label>{{ t("simverse.causal.effect") }} ({{ selectedEvent.effects.length }})</ion-label></ion-list-header>
            <ion-item v-for="eff in selectedEvent.effects" :key="eff.id" button @click="loadAndShowEvent(eff.id)">
              <ion-label class="ion-text-wrap">
                <h3>{{ eff.type_cn }}</h3>
                <p>tick {{ eff.tick }} · {{ eff.level_cn }}</p>
              </ion-label>
              <ion-icon :icon="chevronForward" slot="end" />
            </ion-item>
          </ion-list>
        </ion-content>
      </ion-modal>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonAvatar,
  IonBackButton,
  IonBadge,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonNote,
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
// useRouteTransition exposes onEnter/onLeave hooks for a parent <Transition> wrapper.
// App.vue currently uses plain ion-router-outlet, so this view self-animates
// .page-root via gsap.fromTo as a fallback (matches useRouteTransition defaults).
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
function impBadgeColor(imp: number): string {
  const colors = ["medium", "tertiary", "primary", "success", "warning", "danger"];
  return colors[imp] || "medium";
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
  // 详情路由入场动效（与 useRouteTransition 默认参数一致）
  const el = document.querySelector(".page-root");
  if (el) {
    gsap.fromTo(el, { opacity: 0, y: 24 }, { opacity: 1, y: 0, duration: 0.35, ease: "power2.out" });
  }
});

// P7 持续演化：世界编年史随时间线实时刷新（WS 推送优先，未连接时 8s 兜底轮询）
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
.chronicle-item { --padding-start: 12px; --inner-padding-end: 8px; }
.imp-badge { margin-right: 8px; }
.tick-info { color: var(--color-base-content); opacity: 0.7; font-size: 12px; }
.empty-item {
  --padding-start: 0;
  --inner-padding-end: 0;
  justify-content: center;
  color: var(--color-base-content);
  opacity: 0.7;
  padding: 30px 0;
}
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
