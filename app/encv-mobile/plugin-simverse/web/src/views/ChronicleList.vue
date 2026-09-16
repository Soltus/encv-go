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
      <div class="p-4 space-y-4">
        <div v-if="loading" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>

        <div v-else-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
        </div>

        <template v-else>
          <div class="flex items-center justify-between">
            <span class="ui-chip ui-chip--mono">Era {{ era }}</span>
            <span class="text-sm text-base-content/70">{{ totalEvents }} {{ t("simverse.events") }}</span>
          </div>

          <div class="timeline timeline-container">
            <div
              v-for="(event, idx) in filteredEvents"
              :key="event.id"
              class="timeline-item"
              :class="`importance-${event.importance}`"
            >
              <div class="timeline-dot">
                <span class="dot-inner"></span>
              </div>
              <div class="ui-card timeline-content" @click="goToDetail(event.id)">
                <div class="p-3">
                  <div class="flex items-center justify-between mb-2">
                    <span class="ui-chip" :class="getImportanceChipClass(event.importance)">
                      {{ event.imp_cn }}
                    </span>
                    <span class="text-xs text-base-content/70 font-mono">Tick {{ event.tick }}</span>
                  </div>
                  <h3 class="text-sm font-medium mb-1">{{ event.type_cn }}</h3>
                  <p v-if="event.causes && event.causes.length > 0" class="text-xs text-base-content/70">
                    因: {{ event.causes.slice(0, 2).map(c => c.type_cn).join(", ") }}
                  </p>
                </div>
              </div>
              <div v-if="idx < filteredEvents.length - 1" class="timeline-line"></div>
            </div>
          </div>
        </template>
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
import { useGsap } from "@/composables/useGsap";
import { useLiveRefresh } from "@/composables/useLiveRefresh";
import { type SimverseChronicleEvent, useSimverse } from "@/composables/useSimverse";

const props = defineProps<{
  npcId?: number;
  backHref?: string;
}>();

const { t } = useI18n();
const router = useRouter();
const { loadChronicleWorld, loadChronicleNPC, chronicleSignal } = useSimverse();
const { gsap, ScrollTrigger } = useGsap();

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

function getImportanceChipClass(imp: number): string {
  if (imp >= 5) return "!bg-error/10 !text-error";
  if (imp >= 3) return "!bg-warning/10 !text-warning";
  if (imp >= 2) return "!bg-primary/10 !text-primary";
  return "!bg-base-300 !text-base-content/70";
}

onMounted(() => {
  loadEvents();
  gsap.from(".timeline-item", {
    opacity: 0,
    x: -30,
    duration: 0.5,
    stagger: 0.1,
    ease: "power2.out",
    scrollTrigger: {
      trigger: ".timeline-container",
      start: "top 80%",
      end: "bottom 20%",
      scrub: 1,
    },
  });
  void ScrollTrigger;
});

useLiveRefresh(() => loadEvents(true), { signal: chronicleSignal, pollMs: 8000 });
</script>

<style scoped lang="scss">
.state-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;

  p {
    color: var(--color-error);
    margin: 0;
  }
}

.timeline {
  position: relative;
  padding: 8px 0 8px 8px;
}

.timeline-item {
  position: relative;
  padding-left: 20px;
  padding-bottom: 16px;
}

.timeline-dot {
  position: absolute;
  left: 0;
  top: 16px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--color-base-100);
  border: 2px solid var(--color-base-content);
  z-index: 1;
}

.timeline-item.importance-4 .timeline-dot,
.timeline-item.importance-5 .timeline-dot {
  border-color: var(--color-error);
}

.timeline-item.importance-3 .timeline-dot {
  border-color: var(--color-warning);
}

.timeline-item.importance-2 .timeline-dot {
  border-color: var(--color-primary);
}

.dot-inner {
  display: block;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--color-base-content);
  margin: 2px auto;
}

.timeline-item.importance-4 .dot-inner,
.timeline-item.importance-5 .dot-inner {
  background: var(--color-error);
}

.timeline-item.importance-3 .dot-inner {
  background: var(--color-warning);
}

.timeline-item.importance-2 .dot-inner {
  background: var(--color-primary);
}

.timeline-line {
  position: absolute;
  left: 5px;
  top: 28px;
  width: 2px;
  height: calc(100% + 4px);
  background: var(--color-base-300);
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

.timeline-content {
  cursor: pointer;
}
</style>
