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
      <template v-else>
        <ion-card v-if="prices" class="bar">
          <ion-card-header>
            <ion-card-title>{{ t("simverse.prices") }} · #{{ prices.region_id }}</ion-card-title>
          </ion-card-header>
          <ion-card-content>
            <div class="stats stats-vertical sm:stats-horizontal shadow w-full">
              <div class="stat">
                <div class="stat-title">{{ t("simverse.tradeVolume") }}</div>
                <div class="stat-value">{{ prices.trade_volume }}</div>
              </div>
              <div class="stat">
                <div class="stat-title">{{ t("simverse.shock") }}</div>
                <div class="stat-value">{{ shocks?.count ?? 0 }}</div>
              </div>
            </div>
          </ion-card-content>
        </ion-card>

        <ion-list :inset="true">
          <ion-item button detail class="rank-item" @click="go('/economy/prices')">
            <ion-icon :icon="pricetag" slot="start" color="warning" />
            <ion-label>{{ t("simverse.prices") }}</ion-label>
          </ion-item>
          <ion-item button detail class="rank-item" @click="go('/economy/trade')">
            <ion-icon :icon="swapHorizontal" slot="start" color="primary" />
            <ion-label>{{ t("simverse.trade") }}</ion-label>
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
  IonPage,
  IonRow,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, pricetag, refreshOutline, swapHorizontal } from "ionicons/icons";
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

async function reload(silent = false) {
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    prices.value = await loadEconomyPrices(1);
    shocks.value = await loadEconomyShocks();
    if (!silent) recordQuestAction("view_economy");
  } catch (e: any) {
    if (silent) console.warn("[simverse] economy refresh failed:", e);
    else error.value = e.message || "Failed to load economy";
  } finally {
    if (!silent) loading.value = false;
  }
}

function go(path: string) {
  router.push(path);
}

onMounted(() => {
  reload();
  gsap.from(".bar", { scaleX: 0, transformOrigin: "left", stagger: 0.05, duration: 0.6, ease: "power2.out" });
  gsap.from(".rank-item", { y: 20, opacity: 0, stagger: 0.08, duration: 0.4 });
});

// P7 持续演化：经济行情随世界演化实时刷新（WS 推送优先，未连接时 8s 兜底轮询）
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
