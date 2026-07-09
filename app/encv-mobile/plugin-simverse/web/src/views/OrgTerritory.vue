<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/orgs" />
        </ion-buttons>
        <ion-title>{{ t("simverse.orgTerritory") }}</ion-title>
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
      <template v-else-if="data">
        <ion-card>
          <ion-card-header>
            <ion-card-title>{{ data.name }}</ion-card-title>
            <ion-card-subtitle>#{{ data.org_id }}</ion-card-subtitle>
          </ion-card-header>
        </ion-card>

        <ion-list-header v-if="data.territory.length">
          <ion-label>{{ t("simverse.regionDistribution") }}</ion-label>
        </ion-list-header>
        <ion-list v-if="data.territory.length" :inset="true">
          <ion-item
            v-for="terr in data.territory"
            :key="terr.region_id"
            button
            detail
            @click="goRegion(terr.region_id)"
          >
            <ion-icon :icon="location" slot="start" color="success" />
            <ion-label>{{ t("simverse.regions") }} #{{ terr.region_id }}</ion-label>
            <ion-note slot="end" color="medium">{{ terr.members }}</ion-note>
          </ion-item>
        </ion-list>
        <div v-else class="state-box">
          <p>{{ t("simverse.noRegions") }}</p>
        </div>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonCard, IonCardHeader, IonCardTitle,
  IonCardSubtitle, IonList, IonListHeader, IonLabel, IonItem, IonNote, IonSpinner,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline, location } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseOrgTerritoryResponse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadOrgTerritory } = useSimverse();

const orgId = Number(route.params.id);
const loading = ref(false);
const error = ref("");
const data = ref<SimverseOrgTerritoryResponse | null>(null);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    data.value = await loadOrgTerritory(orgId);
  } catch (e: any) {
    error.value = e.message || "Failed to load territory";
  } finally {
    loading.value = false;
  }
}

function goRegion(id: number) {
  router.push(`/region/${id}`);
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
</style>
