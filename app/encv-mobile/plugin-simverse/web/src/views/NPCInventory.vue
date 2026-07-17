<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/npcs" />
        </ion-buttons>
        <ion-title>{{ t("simverse.npcInventory") }}</ion-title>
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

      <template v-else-if="npc">
        <ion-list :inset="true">
          <ion-list-header>
            <ion-label>{{ t("simverse.npcInventory") }}</ion-label>
          </ion-list-header>
          <ion-item v-for="item in inventoryEntries" :key="item.key">
            <ion-label>{{ item.key }}</ion-label>
            <ion-note slot="end">×{{ item.value }}</ion-note>
          </ion-item>
          <ion-item v-if="!inventoryEntries.length" class="empty-item">
            <ion-label class="ion-text-center">{{ t("simverse.detail.noInventory") }}</ion-label>
          </ion-item>
        </ion-list>

        <ion-list v-if="bankEntries.length" :inset="true">
          <ion-list-header>
            <ion-label>{{ t("simverse.gold") }} / {{ t("simverse.diamond") }}</ion-label>
          </ion-list-header>
          <ion-item v-for="item in bankEntries" :key="item.key">
            <ion-label>{{ item.key }}</ion-label>
            <ion-note slot="end">{{ item.value }}</ion-note>
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
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { type SimverseNPCDetail, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const { loadNPCDetail } = useSimverse();

const loading = ref(false);
const error = ref("");
const npc = ref<SimverseNPCDetail | null>(null);

const inventoryEntries = computed(() =>
  npc.value ? Object.entries(npc.value.inventory || {}).map(([key, value]) => ({ key, value })) : []
);
const bankEntries = computed(() => (npc.value ? Object.entries(npc.value.bank || {}).map(([key, value]) => ({ key, value })) : []));

async function reload() {
  const id = Number(route.params.id);
  if (!id) return;
  loading.value = true;
  error.value = "";
  try {
    npc.value = await loadNPCDetail(id);
  } catch (e: any) {
    error.value = e.message || "Failed to load NPC";
  } finally {
    loading.value = false;
  }
}

onMounted(reload);
watch(() => route.params.id, reload);
</script>

<style scoped lang="scss">
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.state-container p {
  color: var(--color-error);
  margin: 0;
}
.empty-item {
  --padding-start: 0;
  --inner-padding-end: 0;
  justify-content: center;
  color: var(--color-base-content);
  opacity: 0.7;
  padding: 24px 0;
}
</style>
