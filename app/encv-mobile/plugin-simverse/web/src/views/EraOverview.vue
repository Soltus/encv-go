<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.eraOverview") }}</ion-title>
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
import { onMounted, ref } from "vue";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonCard, IonCardHeader, IonCardTitle,
  IonCardContent, IonGrid, IonRow, IonCol, IonList, IonListHeader, IonLabel,
  IonItem, IonSpinner,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline, flag } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseEra } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadEra } = useSimverse();

const loading = ref(false);
const error = ref("");
const era = ref<SimverseEra | null>(null);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    era.value = await loadEra(50);
  } catch (e: any) {
    error.value = e.message || "Failed to load era";
  } finally {
    loading.value = false;
  }
}

onMounted(reload);
</script>

<style scoped>
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
  color: var(--ion-color-medium, #6b7280);
}
.stat-val {
  font-size: 22px;
  font-weight: 700;
}
</style>
