<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.worldEconomy") }}</ion-title>
        <ion-buttons slot="end">
          <span class="live-pill"><span class="live-dot" />{{ t("simverse.live") }}</span>
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
      <template v-else-if="prices">
        <ion-segment :value="String(selectedRegion)" @ionChange="onRegionChange">
          <ion-segment-button v-for="r in regionOptions" :key="r" :value="String(r)">
            <ion-label>#{{ r }}</ion-label>
          </ion-segment-button>
        </ion-segment>

        <ion-card>
          <ion-card-header>
            <ion-card-title>{{ t("simverse.prices") }} · #{{ prices.region_id }}</ion-card-title>
          </ion-card-header>
          <ion-card-content>
            <ion-grid>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.tradeVolume") }}</div><div class="stat-val">{{ prices.trade_volume }}</div></ion-col>
                <ion-col><div class="stat-label">{{ t("simverse.shock") }}</div><div class="stat-val">{{ shocks?.count ?? 0 }}</div></ion-col>
              </ion-row>
            </ion-grid>
          </ion-card-content>
        </ion-card>

        <ion-list-header><ion-label>{{ t("simverse.prices") }}</ion-label></ion-list-header>
        <ion-list :inset="true">
          <ion-item v-for="res in priceRows" :key="res.key">
            <ion-label>{{ res.key }}</ion-label>
            <ion-note slot="end" color="medium">
              {{ t("simverse.price") }} {{ res.price }} · {{ t("simverse.supply") }} {{ res.supply }} · {{ t("simverse.demand") }} {{ res.demand }}
            </ion-note>
          </ion-item>
        </ion-list>

        <ion-list-header v-if="shocks && shocks.items.length"><ion-label>{{ t("simverse.shock") }}</ion-label></ion-list-header>
        <ion-list v-if="shocks && shocks.items.length" :inset="true">
          <ion-item v-for="(sh, i) in shocks.items" :key="i">
            <ion-label>
              <h3>{{ sh.resource }}</h3>
              <p>{{ sh.message }}</p>
            </ion-label>
            <ion-note slot="end" :color="sh.change >= 0 ? 'success' : 'danger'">
              {{ sh.change >= 0 ? "+" : "" }}{{ sh.change.toFixed(1) }}%
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
  IonButton, IonIcon, IonContent, IonCard, IonCardHeader, IonCardTitle,
  IonCardContent, IonGrid, IonRow, IonCol, IonList, IonListHeader, IonLabel,
  IonItem, IonNote, IonSpinner, IonSegment, IonSegmentButton,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  useSimverse,
  type SimverseEconomyPrices,
  type SimverseEconomyShocksResponse,
} from "@/composables/useSimverse";
import { useLiveRefresh } from "@/composables/useLiveRefresh";

const { t } = useI18n();
const { loadEconomyPrices, loadEconomyShocks, recordQuestAction, economySignal } = useSimverse();

const regionOptions = [1, 2, 3, 4, 5];
const selectedRegion = ref(1);
const loading = ref(false);
const error = ref("");
const prices = ref<SimverseEconomyPrices | null>(null);
const shocks = ref<SimverseEconomyShocksResponse | null>(null);

const priceRows = computed(() => {
  if (!prices.value) return [];
  const out: { key: string; price: number; supply: number; demand: number }[] = [];
  for (const k of Object.keys(prices.value.prices)) {
    out.push({
      key: k,
      price: prices.value.prices[k],
      supply: prices.value.supply[k] ?? 0,
      demand: prices.value.demand[k] ?? 0,
    });
  }
  return out;
});

async function reload(silent = false) {
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    prices.value = await loadEconomyPrices(selectedRegion.value);
    shocks.value = await loadEconomyShocks();
    if (!silent) recordQuestAction("view_economy");
  } catch (e: any) {
    if (silent) console.warn("[simverse] world economy refresh failed:", e);
    else error.value = e.message || "Failed to load economy";
  } finally {
    if (!silent) loading.value = false;
  }
}

function onRegionChange(ev: any) {
  selectedRegion.value = Number(ev.detail.value);
  reload();
}

onMounted(reload);

// P7 持续演化：世界经济随世界演化实时刷新（WS 推送优先，未连接时 8s 兜底轮询）
useLiveRefresh(() => reload(true), { signal: economySignal, pollMs: 8000 });
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
.live-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  font-weight: 600;
  color: var(--ion-color-success, #22c55e);
  padding: 2px 8px;
  border-radius: 10px;
  background: rgba(34, 197, 94, 0.12);
  margin-right: 4px;
}
.live-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--ion-color-success, #22c55e);
  animation: live-pulse 1.6s ease-in-out infinite;
}
@keyframes live-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
</style>
