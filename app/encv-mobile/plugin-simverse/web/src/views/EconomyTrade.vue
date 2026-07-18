<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/economy" />
        </ion-buttons>
        <ion-title>{{ t("simverse.trade") }}</ion-title>
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
        <div v-else-if="!shocks || !shocks.items.length" class="state-box">
          <ion-icon :icon="swapHorizontal" size="large" class="text-base-content/40" />
          <p class="text-base-content/70">{{ t("simverse.noEconomy") }}</p>
        </div>

        <template v-else>
          <div class="flex items-center justify-between">
            <span class="ui-chip ui-chip--mono">{{ t("simverse.shock") }}: {{ shocks.count }}</span>
          </div>

          <div class="ui-card">
            <div class="p-3 space-y-1">
              <div
                v-for="(s, i) in shocks.items"
                :key="i"
                class="flex items-start gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors"
              >
                <ion-icon :icon="trendingUp" :class="s.change >= 0 ? 'text-success' : 'text-error'" class="text-xl mt-0.5" />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium">{{ s.message }}</div>
                  <div class="text-xs text-base-content/70 mt-1">
                    {{ t("simverse.regions") }} #{{ s.region_id }} · {{ s.resource }} · {{ t("simverse.price") }}: {{ s.price }}
                  </div>
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
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline, swapHorizontal, trendingUp } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useLiveRefresh } from "@/composables/useLiveRefresh";
import { type SimverseEconomyShocksResponse, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadEconomyShocks, economySignal } = useSimverse();

const loading = ref(false);
const error = ref("");
const shocks = ref<SimverseEconomyShocksResponse | null>(null);

async function reload(silent?: boolean | Event) {
  const isSilent = silent === true;
  if (!isSilent) {
    loading.value = true;
    error.value = "";
  }
  try {
    shocks.value = await loadEconomyShocks();
  } catch (e: any) {
    if (isSilent) console.warn("[simverse] shocks refresh failed:", e);
    else error.value = e.message || "Failed to load shocks";
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
