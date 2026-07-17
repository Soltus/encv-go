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

    <ion-content class="ion-padding">
      <div v-if="error" class="state-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
      </div>

      <!-- 运行状态 -->
      <ion-card>
        <ion-card-header>
          <ion-card-title>{{ t("simverse.intervention.control") }}</ion-card-title>
        </ion-card-header>
        <ion-card-content>
          <div class="status-row">
            <ion-badge :color="isRunning ? 'success' : 'medium'" size="large">
              {{ isRunning ? t("simverse.intervention.running") : t("simverse.intervention.stopped") }}
            </ion-badge>
            <span class="tick">tick {{ currentTick }}</span>
          </div>

          <div class="btn-grid">
            <ion-button v-if="!isRunning" expand="block" color="success" @click="doControl('start')">
              {{ t("simverse.intervention.start") }}
            </ion-button>
            <ion-button v-if="isRunning" expand="block" color="warning" @click="doControl('pause')">
              {{ t("simverse.intervention.pause") }}
            </ion-button>
            <ion-button v-if="!isRunning" expand="block" @click="doControl('resume')">
              {{ t("simverse.intervention.resume") }}
            </ion-button>
            <ion-button expand="block" fill="outline" @click="doControl('step')">
              {{ t("simverse.intervention.step") }}
            </ion-button>
            <ion-button v-if="isRunning" expand="block" color="danger" fill="outline" @click="doControl('stop')">
              {{ t("simverse.intervention.stop") }}
            </ion-button>
          </div>
        </ion-card-content>
      </ion-card>

      <!-- 性能档位 -->
      <ion-card>
        <ion-card-header>
          <ion-card-title>{{ t("simverse.intervention.tier") }}</ion-card-title>
        </ion-card-header>
        <ion-card-content>
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
        </ion-card-content>
      </ion-card>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonBadge,
  IonButton,
  IonButtons,
  IonCard,
  IonCardContent,
  IonCardHeader,
  IonCardTitle,
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
.state-container {
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

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.tick {
  font-family: monospace;
  color: var(--color-base-content);
  opacity: 0.7;
}

.btn-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;

  ion-button[expand="block"] {
    margin: 0;
  }
}
</style>
