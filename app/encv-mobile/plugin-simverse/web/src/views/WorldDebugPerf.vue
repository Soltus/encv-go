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

    <ion-content class="ion-padding">
      <div v-if="loading" class="state-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>

      <div v-else-if="error" class="state-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>

      <template v-else-if="metrics">
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.perf.tier") }}</ion-label></ion-list-header>
          <ion-item>
            <ion-label>{{ t("simverse.perf.tier") }}</ion-label>
            <ion-note slot="end">{{ metrics.tier }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.perf.tps") }}</ion-label>
            <ion-note slot="end">{{ metrics.ticks_per_sec }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.perf.samples") }}</ion-label>
            <ion-note slot="end">{{ metrics.samples }}</ion-note>
          </ion-item>
        </ion-list>

        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.perf.avgTick") }} (ns)</ion-label></ion-list-header>
          <ion-item>
            <ion-label>{{ t("simverse.perf.avgTick") }}</ion-label>
            <ion-note slot="end">{{ metrics.avg_tick_ns.toFixed(1) }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.perf.minTick") }}</ion-label>
            <ion-note slot="end">{{ metrics.min_tick_ns }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.perf.maxTick") }}</ion-label>
            <ion-note slot="end">{{ metrics.max_tick_ns }}</ion-note>
          </ion-item>
        </ion-list>

        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.memory") }}</ion-label></ion-list-header>
          <ion-item>
            <ion-label>{{ t("simverse.perf.npcMb") }}</ion-label>
            <ion-note slot="end">{{ metrics.npc_mb.toFixed(1) }} MB</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.perf.totalMb") }}</ion-label>
            <ion-note slot="end">{{ metrics.total_mb.toFixed(1) }} MB</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.population") }}</ion-label>
            <ion-note slot="end">{{ metrics.npc_count }}</ion-note>
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
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
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
.state-container {
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
