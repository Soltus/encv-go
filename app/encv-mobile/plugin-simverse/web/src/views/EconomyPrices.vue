<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/economy" />
        </ion-buttons>
        <ion-title>{{ t("simverse.prices") }}</ion-title>
        <ion-buttons slot="end">
          <span class="badge badge-success badge-sm gap-1 live-pill"><span class="live-dot" />{{ t("simverse.live") }}</span>
        </ion-buttons>
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
  IonSearchbar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { useLiveRefresh } from "@/composables/useLiveRefresh";
import { type SimverseEconomyPrices, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadEconomyPrices, recordQuestAction, economySignal } = useSimverse();

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

async function reload(silent = false) {
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    prices.value = await loadEconomyPrices(region.value);
    if (!silent) recordQuestAction("view_economy");
  } catch (e: any) {
    if (silent) console.warn("[simverse] prices refresh failed:", e);
    else error.value = e.message || "Failed to load prices";
  } finally {
    if (!silent) loading.value = false;
  }
}

onMounted(reload);

// P7 持续演化：物价随世界演化实时刷新（WS 推送优先，未连接时 8s 兜底轮询）
useLiveRefresh(() => reload(true), { signal: economySignal, pollMs: 8000 });
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

.mini {
  font-size: 11px;
  margin-left: 10px;
}

.live-pill {
  font-weight: 600;
  color: var(--color-success);
  background: color-mix(in srgb, var(--color-success) 12%, transparent);
}

.live-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-success);
  box-shadow: 0 0 6px var(--color-success);
  animation: live-pulse 1.6s ease-in-out infinite;
}

@keyframes live-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
</style>
