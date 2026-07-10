<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/regions" />
        </ion-buttons>
        <ion-title>{{ t("simverse.regionDetail") }}</ion-title>
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
      <template v-else-if="region">
        <ion-card>
          <ion-card-header>
            <ion-card-title>{{ t("simverse.regions") }} #{{ region.region_id }}</ion-card-title>
          </ion-card-header>
          <ion-card-content>
            <ion-grid>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.npcCount") }}</div><div class="stat-val">{{ region.npc_count }}</div></ion-col>
                <ion-col><div class="stat-label">{{ t("simverse.aliveCount") }}</div><div class="stat-val">{{ region.alive_count }}</div></ion-col>
              </ion-row>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.population") }}</div><div class="stat-val">{{ region.population }}</div></ion-col>
                <ion-col><div class="stat-label">{{ t("simverse.avgLevel") }}</div><div class="stat-val">{{ region.avg_level.toFixed(1) }}</div></ion-col>
              </ion-row>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.avgWealthTier") }}</div><div class="stat-val">{{ region.avg_wealth_tier.toFixed(1) }}</div></ion-col>
              </ion-row>
            </ion-grid>
          </ion-card-content>
        </ion-card>

        <ion-list-header><ion-label>{{ t("simverse.events") }}</ion-label></ion-list-header>
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
import { useRoute, useRouter } from "vue-router";
import { type SimverseEraEvent, type SimverseRegion, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadRegionDetail } = useSimverse();

const regionId = Number(route.params.id);
const loading = ref(false);
const error = ref("");
const region = ref<SimverseRegion | null>(null);
const events = ref<SimverseEraEvent[]>([]);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const data = await loadRegionDetail(regionId);
    if (data) {
      region.value = data.region;
      events.value = data.events;
    }
  } catch (e: any) {
    error.value = e.message || "Failed to load region";
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
  padding: 40px 20px;
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
