<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/economy" />
        </ion-buttons>
        <ion-title>{{ t("simverse.prices") }}</ion-title>
      </ion-toolbar>
      <ion-toolbar>
        <ion-searchbar
          v-model="regionInput"
          :placeholder="t('simverse.regions') + ' #'"
          mode="ios"
          type="number"
          :debounce="300"
          @ionInput="onRegionChange"
        />
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
      <template v-else-if="prices">
        <ion-list-header>
          <ion-label>{{ t("simverse.regions") }} #{{ prices.region_id }} · {{ t("simverse.tradeVolume") }}: {{ prices.trade_volume }}</ion-label>
        </ion-list-header>
        <ion-list :inset="true">
          <ion-item v-for="r in resourceKeys" :key="r">
            <ion-label>{{ r }}</ion-label>
            <ion-note slot="end" color="warning">{{ prices.prices[r] }}</ion-note>
            <ion-note slot="end" color="medium" class="mini">
              {{ t("simverse.supply") }}:{{ prices.supply[r] }} / {{ t("simverse.demand") }}:{{ prices.demand[r] }}
            </ion-note>
          </ion-item>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonList, IonListHeader, IonLabel,
  IonItem, IonNote, IonSpinner, IonSearchbar,
} from "@ionic/vue";
import { alertCircleOutline } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseEconomyPrices } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadEconomyPrices, recordQuestAction } = useSimverse();

const loading = ref(false);
const error = ref("");
const regionInput = ref("1");
const region = ref(1);
const prices = ref<SimverseEconomyPrices | null>(null);

const resourceKeys = computed(() => (prices.value ? Object.keys(prices.value.prices) : []));

function onRegionChange() {
  const n = parseInt(regionInput.value, 10);
  if (!isNaN(n) && n >= 0) {
    region.value = n;
    reload();
  }
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    prices.value = await loadEconomyPrices(region.value);
    recordQuestAction("view_economy");
  } catch (e: any) {
    error.value = e.message || "Failed to load prices";
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
.mini {
  font-size: 11px;
  margin-left: 10px;
}
</style>
