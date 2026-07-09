<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/economy" />
        </ion-buttons>
        <ion-title>{{ t("simverse.trade") }}</ion-title>
        <ion-buttons slot="end">
          <span class="live-pill"><span class="live-dot" />{{ t("simverse.live") }}</span>
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
      <div v-else-if="!shocks || !shocks.items.length" class="state-box">
        <ion-icon :icon="swapHorizontal" size="large" color="medium" />
        <p>{{ t("simverse.noEconomy") }}</p>
      </div>

      <ion-list v-else :inset="true">
        <ion-list-header>
          <ion-label>{{ t("simverse.shock") }}: {{ shocks.count }}</ion-label>
        </ion-list-header>
        <ion-item v-for="(s, i) in shocks.items" :key="i">
          <ion-icon :icon="trendingUp" slot="start" :color="s.change >= 0 ? 'success' : 'danger'" />
          <ion-label>
            <h3>{{ s.message }}</h3>
            <p>{{ t("simverse.regions") }} #{{ s.region_id }} · {{ s.resource }} · {{ t("simverse.price") }}: {{ s.price }}</p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonList, IonListHeader, IonLabel,
  IonItem, IonSpinner,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline, swapHorizontal, trendingUp } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseEconomyShocksResponse } from "@/composables/useSimverse";
import { useLiveRefresh } from "@/composables/useLiveRefresh";

const { t } = useI18n();
const { loadEconomyShocks, economySignal } = useSimverse();

const loading = ref(false);
const error = ref("");
const shocks = ref<SimverseEconomyShocksResponse | null>(null);

async function reload(silent = false) {
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    shocks.value = await loadEconomyShocks();
  } catch (e: any) {
    if (silent) console.warn("[simverse] shocks refresh failed:", e);
    else error.value = e.message || "Failed to load shocks";
  } finally {
    if (!silent) loading.value = false;
  }
}

onMounted(reload);

// P7 持续演化：价格冲击流随世界演化实时刷新（WS 推送优先，未连接时 8s 兜底轮询）
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
