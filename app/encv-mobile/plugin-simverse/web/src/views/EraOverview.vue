<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.eraOverview") }}</ion-title>
        <ion-buttons slot="end">
          <span class="live-pill"><span class="live-dot" />{{ t("simverse.live") }}</span>
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
          <button type="button" class="ui-button" @click="() => reload()">{{ t("settings.check") }}</button>
        </div>
        <template v-else-if="era">
          <div class="ui-card">
            <div class="p-4">
              <h2 class="text-lg font-bold m-0 mb-3">{{ t("simverse.currentEra") }} · {{ era.era }}</h2>
              <div class="grid grid-cols-2 gap-4">
                <div class="text-center">
                  <div class="stat-label">{{ t("simverse.worldTick") }}</div>
                  <div class="stat-val">{{ era.world_tick }}</div>
                </div>
                <div class="text-center">
                  <div class="stat-label">{{ t("simverse.eventCount") }}</div>
                  <div class="stat-val">{{ era.event_count }}</div>
                </div>
              </div>
            </div>
          </div>

          <div class="ui-header">{{ t("simverse.events") }}</div>
          <div class="space-y-2">
            <div
              v-for="ev in era.events"
              :key="ev.id"
              class="ui-card"
            >
              <div class="p-3 flex items-start gap-3">
                <div class="ui-bubble !p-0 !w-10 !h-10 flex items-center justify-center flex-shrink-0 !bg-warning/15 !border-warning/30">
                  <ion-icon :icon="flag" class="text-warning" />
                </div>
                <div class="flex-1 min-w-0">
                  <h3 class="text-base font-semibold m-0 mb-1">{{ ev.type }}</h3>
                  <p class="text-xs text-base-content/60 m-0">
                    {{ t("simverse.tick") }}: {{ ev.tick }} · {{ t("simverse.importance") }}: {{ ev.importance }}
                  </p>
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
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, flag, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useLiveRefresh } from "@/composables/useLiveRefresh";
import { type SimverseEra, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadEra, chronicleSignal } = useSimverse();

const loading = ref(false);
const error = ref("");
const era = ref<SimverseEra | null>(null);

async function reload(silent = false) {
  if (!silent) {
    loading.value = true;
    error.value = "";
  }
  try {
    era.value = await loadEra(50);
  } catch (e: any) {
    if (silent) console.warn("[simverse] era refresh failed:", e);
    else error.value = e.message || "Failed to load era";
  } finally {
    if (!silent) loading.value = false;
  }
}

onMounted(reload);

useLiveRefresh(() => reload(true), { signal: chronicleSignal, pollMs: 8000 });
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
.stat-label {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
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
  color: var(--color-success);
  padding: 2px 8px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--color-success) 12%, transparent);
  margin-right: 4px;
}
.live-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-success);
  animation: live-pulse 1.6s ease-in-out infinite;
}
@keyframes live-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(0.7); }
}
</style>
