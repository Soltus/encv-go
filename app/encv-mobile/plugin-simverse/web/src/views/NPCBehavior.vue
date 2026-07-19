<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/npcs" />
        </ion-buttons>
        <ion-title>{{ t("simverse.detail.viewBehavior") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding">
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
          <!-- 当前行为快照 -->
          <div v-if="behavior" class="ui-card">
            <div class="p-4">
              <div class="ui-header mb-4">{{ t("simverse.behavior") }}</div>
              <div class="behavior-now mb-4">
                <span class="ui-chip !text-sm !font-semibold !bg-warning/15 !text-warning !border-warning/30">
                  <ion-icon :icon="pulseOutline" class="mr-1" />
                  {{ behavior.current_behavior_cn || behavior.current_behavior }}
                </span>
              </div>
              <div class="grid grid-cols-2 gap-4">
                <div class="stat-item">
                  <div class="stat-label text-xs text-base-content/60 mb-1">{{ t("simverse.detail.mood") }}</div>
                  <div class="stat-val text-xl font-bold">{{ behavior.mood }}</div>
                </div>
                <div class="stat-item">
                  <div class="stat-label text-xs text-base-content/60 mb-1">{{ t("simverse.detail.energy") }}</div>
                  <div class="stat-val text-xl font-bold">{{ behavior.energy }}</div>
                </div>
                <div class="stat-item">
                  <div class="stat-label text-xs text-base-content/60 mb-1">{{ t("simverse.behaviorStart") }}</div>
                  <div class="stat-val text-xl font-bold font-mono">{{ behavior.behavior_start_tick }}</div>
                </div>
                <div class="stat-item">
                  <div class="stat-label text-xs text-base-content/60 mb-1">{{ t("simverse.behaviorDuration") }}</div>
                  <div class="stat-val text-xl font-bold font-mono">{{ behavior.behavior_duration }}</div>
                </div>
              </div>
            </div>
          </div>

          <!-- 行为时间线（该 NPC 的编年史事件流） -->
          <div class="ui-card">
            <div class="p-4">
              <div class="ui-header mb-3">{{ t("simverse.chronicleTimeline") }}</div>
              <div v-if="events.length" class="space-y-3">
                <div
                  v-for="ev in events"
                  :key="ev.id"
                  class="flex items-start gap-3 py-2 border-b border-base-200 last:border-b-0 last:pb-0"
                >
                  <div class="ui-bubble w-8 h-8 flex items-center justify-center text-xs flex-shrink-0 !bg-primary/10">
                    <ion-icon :icon="timeOutline" class="text-primary" />
                  </div>
                  <div class="flex-1 min-w-0">
                    <h3 class="text-sm font-medium m-0 mb-1">{{ ev.type }}</h3>
                    <p class="text-xs text-base-content/60 m-0">
                      Tick {{ ev.tick }} · {{ t("simverse.importance") }} {{ ev.importance }}
                    </p>
                  </div>
                </div>
              </div>
              <div v-else class="state-box py-10">
                <p class="text-base-content/60 m-0">{{ t("simverse.causal.noEvent") }}</p>
              </div>
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
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline, pulseOutline, timeOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { type SimverseBehaviorState, type SimverseChronicleEvent, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const { loadBehaviorList, loadChronicleNPC } = useSimverse();

const npcId = Number(route.params.id);
const loading = ref(false);
const error = ref("");
const behavior = ref<SimverseBehaviorState | null>(null);
const events = ref<SimverseChronicleEvent[]>([]);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const [list, chron] = await Promise.all([loadBehaviorList(1, 500), loadChronicleNPC(npcId, 30)]);
    if (list) {
      behavior.value = list.items.find(b => b.npc_id === npcId) || null;
    }
    if (chron) {
      events.value = chron.items;
    }
  } catch (e: any) {
    error.value = e.message || "Failed to load behavior";
  } finally {
    loading.value = false;
  }
}

onMounted(reload);
</script>

<style scoped lang="scss">
.state-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 16px;
}
.stat-item {
  text-align: left;
}
</style>
