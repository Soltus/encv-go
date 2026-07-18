<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.intervention") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4 space-y-4">
        <div v-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-3">{{ t("simverse.intervention.control") }}</div>
            <div class="flex items-center justify-between mb-4">
              <span class="ui-chip" :class="isRunning ? '!bg-success/15 !text-success' : '!bg-base-300 !text-base-content/70'">
                {{ isRunning ? t("simverse.intervention.running") : t("simverse.intervention.stopped") }}
              </span>
              <span class="text-xs text-base-content/70 font-mono">tick {{ currentTick }}</span>
            </div>

            <div class="grid grid-cols-2 gap-2">
              <button
                v-if="!isRunning"
                type="button"
                class="ui-button !bg-success !text-success-content"
                @click="doControl('start')"
              >
                {{ t("simverse.intervention.start") }}
              </button>
              <button
                v-if="isRunning"
                type="button"
                class="ui-button !bg-warning !text-warning-content"
                @click="doControl('pause')"
              >
                {{ t("simverse.intervention.pause") }}
              </button>
              <button
                v-if="!isRunning"
                type="button"
                class="ui-button"
                @click="doControl('resume')"
              >
                {{ t("simverse.intervention.resume") }}
              </button>
              <button
                type="button"
                class="ui-button !bg-base-200 !text-base-content"
                @click="doControl('step')"
              >
                {{ t("simverse.intervention.step") }}
              </button>
              <button
                v-if="isRunning"
                type="button"
                class="ui-button !bg-error/10 !text-error col-span-2"
                @click="doControl('stop')"
              >
                {{ t("simverse.intervention.stop") }}
              </button>
            </div>
          </div>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.intervention.tier") }}</div>
            <ion-segment :value="currentTier" @ionChange="onTierChange">
              <ion-segment-button value="background">
                <ion-label>{{ t("simverse.tierBackground") }}</ion-label>
              </ion-segment-button>
              <ion-segment-button value="foreground">
                <ion-label>{{ t("simverse.tierForeground") }}</ion-label>
              </ion-segment-button>
              <ion-segment-button value="fg_idle">
                <ion-label>{{ t("simverse.tierIdle") }}</ion-label>
              </ion-segment-button>
            </ion-segment>
          </div>
        </div>
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
  IonLabel,
  IonPage,
  IonSegment,
  IonSegmentButton,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { type PerfTier, useSimverse, type WorldAction } from "@/composables/useSimverse";

const { t } = useI18n();
const { isRunning, currentTick, currentTier, controlWorld, setPerformanceTier, loadWorldState, loadWorldConfig } = useSimverse();

const error = ref("");

async function doControl(action: WorldAction) {
  try {
    await controlWorld(action);
  } catch (e: any) {
    error.value = e.message || "Control failed";
  }
}

async function onTierChange(ev: any) {
  const tier = ev.detail.value as PerfTier;
  try {
    await setPerformanceTier(tier);
  } catch (e: any) {
    error.value = e.message || "Set tier failed";
  }
}

async function reload() {
  try {
    await loadWorldState();
    await loadWorldConfig();
  } catch (e: any) {
    error.value = e.message || "Failed to load world state";
  }
}

onMounted(reload);
</script>

<style scoped lang="scss">
.state-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 12px;

  p {
    color: var(--color-error);
    margin: 0;
  }
}
</style>
