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
        <template v-else-if="prices">
          <div class="flex items-center justify-between">
            <span class="ui-chip ui-chip--mono">{{ t("simverse.regions") }} #{{ prices.region_id }}</span>
            <span class="text-sm text-base-content/70">{{ t("simverse.tradeVolume") }}: {{ prices.trade_volume }}</span>
          </div>

          <div class="ui-card">
            <div class="p-3 space-y-1">
              <div
                v-for="r in resourceKeys"
                :key="r"
                class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors"
              >
                <span class="text-sm font-medium">{{ r }}</span>
                <div class="flex items-center gap-2">
                  <span class="text-xs text-base-content/70">
                    {{ t("simverse.supply") }}:{{ prices.supply[r] }} / {{ t("simverse.demand") }}:{{ prices.demand[r] }}
                  </span>
                  <span class="text-sm font-mono font-medium text-warning">{{ prices.prices[r] }}</span>
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

async function reload(silent?: boolean | Event) {
  const isSilent = silent === true;
  if (!isSilent) {
    loading.value = true;
    error.value = "";
  }
  try {
    prices.value = await loadEconomyPrices(region.value);
    if (!isSilent) recordQuestAction("view_economy");
  } catch (e: any) {
    if (isSilent) console.warn("[simverse] prices refresh failed:", e);
    else error.value = e.message || "Failed to load prices";
  } finally {
    if (!isSilent) loading.value = false;
  }
}

onMounted(reload);

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
