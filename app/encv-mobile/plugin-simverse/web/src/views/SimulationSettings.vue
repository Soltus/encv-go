<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings" />
        </ion-buttons>
        <ion-title>{{ t("simverse.simulationSettings") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding">
      <div v-if="error" class="state-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
      </div>

      <ion-card>
        <ion-card-header>
          <ion-card-title>{{ t("simverse.perfTier") }}</ion-card-title>
        </ion-card-header>
        <ion-card-content>
          <ion-segment :value="currentTier" @ionChange="onTierChange">
            <ion-segment-button value="background">
              <ion-label>{{ t("simverse.tierBackground") }}</ion-label>
            </ion-segment-button>
            <ion-segment-button value="foreground">
              <ion-label>{{ t("simverse.tierForeground") }}</ion-label>
            </ion-segment-button>
            <ion-segment-button value="fg_idle">
              <ion-label>{{ t("simverse.tierIdle") }}</ion-label>
            </ion-segment-button>
          </ion-segment>
        </ion-card-content>
      </ion-card>

      <ion-list v-if="config" :inset="true">
        <ion-list-header><ion-label>{{ t("simverse.configDetails") }}</ion-label></ion-list-header>
        <ion-item>
          <ion-label>{{ t("simverse.perfTier") }}</ion-label>
          <ion-note slot="end">{{ config.tier_name }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.eventRate") }}</ion-label>
          <ion-note slot="end">{{ config.event_rate_mul }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.cacheSize") }}</ion-label>
          <ion-note slot="end">{{ config.cache_size }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.subSim") }}</ion-label>
          <ion-note slot="end">{{ config.sub_sim_active ? '✅' : '—' }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.subSimDepth") }}</ion-label>
          <ion-note slot="end">{{ config.sub_sim_depth }}</ion-note>
        </ion-item>
      </ion-list>
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
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonSegment,
  IonSegmentButton,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { type PerfTier, type SimverseWorldConfig, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { currentTier, worldConfig, setPerformanceTier, loadWorldConfig } = useSimverse();

const error = ref("");
const config = ref<SimverseWorldConfig | null>(null);

async function onTierChange(ev: any) {
  try {
    await setPerformanceTier(ev.detail.value as PerfTier);
    config.value = worldConfig.value;
  } catch (e: any) {
    error.value = e.message || "Set tier failed";
  }
}

async function reload() {
  try {
    await loadWorldConfig();
    config.value = worldConfig.value;
  } catch (e: any) {
    error.value = e.message || "Failed to load config";
  }
}
onMounted(reload);
</script>

<style scoped lang="scss">
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 12px;

  p {
    color: var(--color-error);
    margin: 0;
  }
}
</style>
