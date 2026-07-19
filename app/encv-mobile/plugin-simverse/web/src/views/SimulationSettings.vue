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

    <ion-content>
      <div class="p-4 space-y-4">
        <div v-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.perfTier") }}</div>
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
          </div>
        </div>

        <div v-if="config" class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.configDetails") }}</div>
            <div class="space-y-1">
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.perfTier") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ config.tier_name }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.eventRate") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ config.event_rate_mul }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.cacheSize") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ config.cache_size }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.subSim") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ config.sub_sim_active ? '✓' : '—' }}</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.subSimDepth") }}</span>
                <span class="text-xs text-base-content/70 font-mono">{{ config.sub_sim_depth }}</span>
              </div>
            </div>
          </div>
        </div>
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
  IonLabel,
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
.state-box {
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
