<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/chronicles" />
        </ion-buttons>
        <ion-title>{{ t("simverse.causalChain") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading" class="state-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>

      <template v-else-if="event">
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.events") }}</ion-label></ion-list-header>
          <ion-item>
            <ion-label>{{ t("simverse.events") }}</ion-label>
            <ion-note slot="end">{{ event.type_cn }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.perf.tier") }}</ion-label>
            <ion-note slot="end">{{ event.level_cn }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.tick") }}</ion-label>
            <ion-note slot="end">{{ event.tick }}</ion-note>
          </ion-item>
          <ion-item v-if="event.entity_id">
            <ion-label>#{{ event.entity_id }}</ion-label>
          </ion-item>
        </ion-list>

        <ion-list v-if="event && event.causes?.length" :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.causal.cause") }} ({{ event.causes.length }})</ion-label></ion-list-header>
          <ion-item v-for="cause in event.causes" :key="cause.id" button @click="loadAndShowEvent(cause.id)">
            <ion-label class="ion-text-wrap">
              <h3>{{ cause.type_cn }}</h3>
              <p>tick {{ cause.tick }} · {{ cause.level_cn }}</p>
            </ion-label>
            <ion-icon :icon="chevronForward" slot="end" />
          </ion-item>
        </ion-list>
        <ion-list v-else :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.causal.cause") }}</ion-label></ion-list-header>
          <ion-item class="empty-item"><ion-label class="ion-text-center">{{ t("simverse.causal.noEvent") }}</ion-label></ion-item>
        </ion-list>

        <ion-list v-if="event && event.effects?.length" :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.causal.effect") }} ({{ event.effects.length }})</ion-label></ion-list-header>
          <ion-item v-for="eff in event.effects" :key="eff.id" button @click="loadAndShowEvent(eff.id)">
            <ion-label class="ion-text-wrap">
              <h3>{{ eff.type_cn }}</h3>
              <p>tick {{ eff.tick }} · {{ eff.level_cn }}</p>
            </ion-label>
            <ion-icon :icon="chevronForward" slot="end" />
          </ion-item>
        </ion-list>
        <ion-list v-else :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.causal.effect") }}</ion-label></ion-list-header>
          <ion-item class="empty-item"><ion-label class="ion-text-center">{{ t("simverse.causal.noEvent") }}</ion-label></ion-item>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonList, IonListHeader, IonLabel,
  IonItem, IonNote, IonSpinner,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline, chevronForward } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseChronicleEvent } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const { loadChronicleEvent } = useSimverse();

const loading = ref(false);
const error = ref("");
const event = ref<SimverseChronicleEvent | null>(null);

async function reload() {
  const id = Number(route.params.id);
  if (!id) return;
  loading.value = true;
  error.value = "";
  try {
    event.value = await loadChronicleEvent(id);
    if (!event.value) error.value = t("simverse.noData");
  } catch (e: any) {
    error.value = e.message || "Failed to load event";
  } finally {
    loading.value = false;
  }
}
async function loadAndShowEvent(id: number) {
  const detail = await loadChronicleEvent(id);
  if (detail) event.value = detail;
}
onMounted(reload);
watch(() => route.params.id, reload);
</script>

<style scoped>
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.state-container p { color: var(--ion-color-danger); margin: 0; }
.empty-item {
  --padding-start: 0;
  --inner-padding-end: 0;
  justify-content: center;
  color: var(--ion-color-medium);
  padding: 20px 0;
}
</style>
