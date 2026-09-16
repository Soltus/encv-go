<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.perfMon") }}</ion-title>
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

        <template v-else-if="metrics">
          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.perf.tier") }}</div>
              <div class="space-y-1">
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.tier") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.tier }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.tps") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.ticks_per_sec }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.samples") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.samples }}</span>
                </div>
              </div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.perf.avgTick") }} (ns)</div>
              <div class="space-y-1">
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.avgTick") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.avg_tick_ns.toFixed(1) }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.minTick") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.min_tick_ns }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.maxTick") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.max_tick_ns }}</span>
                </div>
              </div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.memory") }}</div>
              <div class="space-y-1">
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.npcMb") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ metrics.npc_mb.toFixed(1) }} MB</span>
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
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { type SimversePerfMetrics, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadPerfMetrics } = useSimverse();

const loading = ref(false);
const error = ref("");
const metrics = ref<SimversePerfMetrics | null>(null);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    metrics.value = await loadPerfMetrics();
    if (!metrics.value) error.value = t("simverse.noData");
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
