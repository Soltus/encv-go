<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.eraOverview") }}</ion-title>
        <ion-buttons slot="end">
          <span class="live-pill"><span class="live-dot" />{{ t("simverse.live") }}</span>
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
      <template v-else-if="era">
        <ion-card>
          <ion-card-header>
            <ion-card-title>{{ t("simverse.currentEra") }} · {{ era.era }}</ion-card-title>
          </ion-card-header>
          <ion-card-content>
            <ion-grid>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.worldTick") }}</div><div class="stat-val">{{ era.world_tick }}</div></ion-col>
                <ion-col><div class="stat-label">{{ t("simverse.eventCount") }}</div><div class="stat-val">{{ era.event_count }}</div></ion-col>
              </ion-row>
            </ion-grid>
          </ion-card-content>
        </ion-card>

        <ion-list-header><ion-label>{{ t("simverse.events") }}</ion-label></ion-list-header>
        <ion-list :inset="true">
          <ion-item v-for="ev in era.events" :key="ev.id">
            <ion-icon :icon="flag" slot="start" color="warning" />
            <ion-label>
              <h3>{{ ev.type }}</h3>
              <p>{{ t("simverse.tick") }}: {{ ev.tick }} · {{ t("simverse.importance") }}: {{ ev.importance }}</p>
            </ion-label>
          </ion-item>
        </ion-list>
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
import { alertCircleOutline, flag, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useLiveRefresh } from "@/composables/useLiveRefresh";
import { type SimverseEra, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadEra, chronicleSignal } = useSimverse();

const loading = ref(false);
const error = ref("");
const era = ref<SimverseEra | null>(null);

async function reload(silent = false) {
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    era.value = await loadEra(50);
  } catch (e: any) {
    if (silent) console.warn("[simverse] era refresh failed:", e);
    else error.value = e.message || "Failed to load era";
  } finally {
    if (!silent) loading.value = false;
  }
}

onMounted(reload);

// P7 持续演化：时代/编年史随世界演化实时刷新（WS 推送优先，未连接时 8s 兜底轮询）
useLiveRefresh(() => reload(true), { signal: chronicleSignal, pollMs: 8000 });
</script>

<style scoped lang="scss">
.state-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.stat-label {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
}
.stat-val {
  font-size: 22px;
  font-weight: 700;
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
