<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings" />
        </ion-buttons>
        <ion-title>{{ t("simverse.performanceSettings") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
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

          <div v-if="metrics" class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.perf.tier") }}</div>
              <div class="space-y-1">
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.tps") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.ticks_per_sec }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.avgTick") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.avg_tick_ns.toFixed(1) }} ns</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.totalMb") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.total_mb.toFixed(1) }} MB</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.population") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.npc_count }}</span>
                </div>
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
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonLabel,
  IonPage,
  IonSegment,
  IonSegmentButton,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { type PerfTier, type SimversePerfMetrics, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { currentTier, loadPerfMetrics, setPerformanceTier } = useSimverse();

const loading = ref(false);
const error = ref("");
const metrics = ref<SimversePerfMetrics | null>(null);

async function onTierChange(ev: any) {
  try {
    await setPerformanceTier(ev.detail.value as PerfTier);
  } catch (e: any) {
    error.value = e.message || "Set tier failed";
  }
}
async function reload() {
  loading.value = true;
  error.value = "";
  try {
    metrics.value = await loadPerfMetrics();
  } catch (e: any) {
    error.value = e.message || "Failed to load perf metrics";
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
  padding: 60px 20px;
  gap: 16px;

  p {
    color: var(--color-error);
    margin: 0;
  }
}
</style>
