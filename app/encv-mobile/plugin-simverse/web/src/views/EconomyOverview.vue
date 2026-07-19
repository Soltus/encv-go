<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.economyOverview") }}</ion-title>
        <ion-buttons slot="end">
          <span class="badge badge-success badge-sm gap-1 live-pill"><span class="live-dot" />{{ t("simverse.live") }}</span>
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
        <template v-else>
          <div v-if="prices" class="ui-card bar">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.prices") }} · #{{ prices.region_id }}</div>
              <div class="grid grid-cols-2 gap-3">
                <div class="bg-base-200 rounded-lg p-3">
                  <div class="text-xs text-base-content/70 mb-1">{{ t("simverse.tradeVolume") }}</div>
                  <div class="text-lg font-semibold font-mono">{{ prices.trade_volume }}</div>
                </div>
                <div class="bg-base-200 rounded-lg p-3">
                  <div class="text-xs text-base-content/70 mb-1">{{ t("simverse.shock") }}</div>
                  <div class="text-lg font-semibold font-mono">{{ shocks?.count ?? 0 }}</div>
                </div>
              </div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3 space-y-1">
              <div
                class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                @click="go('/economy/prices')"
              >
                <div class="flex items-center gap-3">
                  <ion-icon :icon="pricetag" class="text-warning text-xl" />
                  <span class="text-sm font-medium">{{ t("simverse.prices") }}</span>
                </div>
                <ion-icon :icon="chevronForward" class="text-base-content/40" />
              </div>
              <div
                class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                @click="go('/economy/trade')"
              >
                <div class="flex items-center gap-3">
                  <ion-icon :icon="swapHorizontal" class="text-primary text-xl" />
                  <span class="text-sm font-medium">{{ t("simverse.trade") }}</span>
                </div>
                <ion-icon :icon="chevronForward" class="text-base-content/40" />
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
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, chevronForward, pricetag, refreshOutline, swapHorizontal } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useLiveRefresh } from "@/composables/useLiveRefresh";
import { useGsap } from "@/composables/useGsap";
import { type SimverseEconomyPrices, type SimverseEconomyShocksResponse, useSimverse } from "@/composables/useSimverse";

const { gsap } = useGsap();

const { t } = useI18n();
const router = useRouter();
const { loadEconomyPrices, loadEconomyShocks, recordQuestAction, economySignal } = useSimverse();

const loading = ref(false);
const error = ref("");
const prices = ref<SimverseEconomyPrices | null>(null);
const shocks = ref<SimverseEconomyShocksResponse | null>(null);

async function reload(silent?: boolean | Event) {
  const isSilent = silent === true;
  if (!isSilent) {
    loading.value = true;
    error.value = "";
  }
  try {
    prices.value = await loadEconomyPrices(1);
    shocks.value = await loadEconomyShocks();
    if (!isSilent) recordQuestAction("view_economy");
  } catch (e: any) {
    if (isSilent) console.warn("[simverse] economy refresh failed:", e);
    else error.value = e.message || "Failed to load economy";
  } finally {
    if (!isSilent) loading.value = false;
  }
}

function go(path: string) {
  router.push(path);
}

onMounted(() => {
  reload();
  gsap.from(".bar", { scaleX: 0, transformOrigin: "left", stagger: 0.05, duration: 0.6, ease: "power2.out" });
});

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
  margin-right: 4px;
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
