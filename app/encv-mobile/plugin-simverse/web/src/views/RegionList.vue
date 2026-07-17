<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.regions") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading" class="state-box">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-box">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>
      <div v-else-if="!regions.length" class="state-box">
        <ion-icon :icon="map" size="large" color="medium" />
        <p>{{ t("simverse.noRegions") }}</p>
      </div>

      <ion-list v-else :inset="true">
        <ion-list-header>
          <ion-label>{{ t("simverse.total") }}: {{ total }}</ion-label>
        </ion-list-header>
        <ion-item
          v-for="r in regions"
          :key="r.region_id"
          button
          detail
          @click="goDetail(r.region_id)"
        >
          <ion-icon :icon="location" slot="start" color="success" />
          <ion-label>
            <h3>{{ t("simverse.regions") }} #{{ r.region_id }}</h3>
            <p>
              <span class="meta">{{ t("simverse.population") }}: {{ r.population }}</span>
              <span class="meta">{{ t("simverse.npc") }}s: {{ r.npc_count }}</span>
            </p>
          </ion-label>
          <ion-note slot="end" color="medium">Lv.{{ r.avg_level.toFixed(1) }}</ion-note>
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
import { alertCircleOutline, location, map, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseRegion, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadRegionList } = useSimverse();

const loading = ref(false);
const error = ref("");
const regions = ref<SimverseRegion[]>([]);
const total = ref(0);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const data = await loadRegionList();
    if (data) {
      regions.value = data.items;
      total.value = data.count;
    }
  } catch (e: any) {
    error.value = e.message || "Failed to load regions";
  } finally {
    loading.value = false;
  }
}

function goDetail(id: number) {
  router.push(`/region/${id}`);
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
}

.meta {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
  margin-right: 8px;
}
</style>
