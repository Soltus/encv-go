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
      <div v-if="loading" class="state-box">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-box">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>
      <template v-else>
        <!-- 当前行为快照 -->
        <ion-card v-if="behavior">
          <ion-card-header>
            <ion-card-title>{{ t("simverse.behavior") }}</ion-card-title>
          </ion-card-header>
          <ion-card-content>
            <div class="behavior-now">
              <span class="behavior-badge">{{ behavior.current_behavior_cn || behavior.current_behavior }}</span>
            </div>
            <ion-grid>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.detail.mood") }}</div><div class="stat-val">{{ behavior.mood }}</div></ion-col>
                <ion-col><div class="stat-label">{{ t("simverse.detail.energy") }}</div><div class="stat-val">{{ behavior.energy }}</div></ion-col>
              </ion-row>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.behaviorStart") }}</div><div class="stat-val">{{ behavior.behavior_start_tick }}</div></ion-col>
                <ion-col><div class="stat-label">{{ t("simverse.behaviorDuration") }}</div><div class="stat-val">{{ behavior.behavior_duration }}</div></ion-col>
              </ion-row>
            </ion-grid>
          </ion-card-content>
        </ion-card>

        <!-- 行为时间线（该 NPC 的编年史事件流） -->
        <ion-list-header><ion-label>{{ t("simverse.chronicleTimeline") }}</ion-label></ion-list-header>
        <ion-list v-if="events.length" :inset="true">
          <ion-item v-for="ev in events" :key="ev.id">
            <ion-label>
              <h3>{{ ev.type }}</h3>
              <p>Tick {{ ev.tick }} · {{ t("simverse.importance") }} {{ ev.importance }}</p>
            </ion-label>
          </ion-item>
        </ion-list>
        <div v-else class="state-box">
          <p>{{ t("simverse.causal.noEvent") }}</p>
        </div>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonButton,
  IonButtons,
  IonCard,
  IonCardContent,
  IonCardHeader,
  IonCardTitle,
  IonCol,
  IonContent,
  IonGrid,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonPage,
  IonRow,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
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
.behavior-now {
  margin-bottom: 12px;
}
.behavior-badge {
  display: inline-block;
  padding: 6px 14px;
  border-radius: 16px;
  background: color-mix(in srgb, var(--color-warning) 18%, transparent);
  color: var(--color-warning);
  font-weight: 600;
  font-size: 14px;
}
.stat-label {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
}
.stat-val {
  font-size: 20px;
  font-weight: 700;
}
</style>
