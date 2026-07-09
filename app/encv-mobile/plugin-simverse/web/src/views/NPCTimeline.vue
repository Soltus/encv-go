<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/npcs" />
        </ion-buttons>
        <ion-title>{{ t("simverse.npcTimeline") }}</ion-title>
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
            <ion-label>{{ t("simverse.detail.lifeEvents") }}: {{ npc.life_events ?? 0 }}</ion-label>
          </ion-list-header>
        </ion-list>

        <ion-list :inset="true">
          <ion-list-header>
            <ion-label>{{ t("simverse.detail.memory") }}</ion-label>
          </ion-list-header>
          <ion-item
            v-for="mem in memories"
            :key="mem.id"
            class="mem-item"
          >
            <div slot="start" class="mem-dot" :class="`imp-${mem.importance}`" />
            <ion-label class="ion-text-wrap">
              <h3>{{ memTypeLabel(mem.type) }}</h3>
              <p>
                <span class="tick-info">tick {{ mem.created_at }}</span>
                <span class="strength">强度 {{ mem.strength }}</span>
              </p>
            </ion-label>
          </ion-item>
          <ion-item v-if="!memories.length" class="empty-item">
            <ion-label class="ion-text-center">{{ t("simverse.detail.noMemory") }}</ion-label>
          </ion-item>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonList, IonListHeader, IonLabel,
  IonItem, IonSpinner,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseNPCDetail, type SimverseMemory } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const { loadNPCDetail } = useSimverse();

const loading = ref(false);
const error = ref("");
const npc = ref<SimverseNPCDetail | null>(null);

const memories = computed<SimverseMemory[]>(() => npc.value?.short_term_mem || []);

function memTypeLabel(type: string): string {
  return type || t("simverse.detail.memory");
}

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

<style scoped>
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.state-container p {
  color: var(--ion-color-danger);
  margin: 0;
}
.mem-item {
  --padding-start: 12px;
}
.mem-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--ion-color-medium);
}
.mem-dot.imp-0 { background: var(--ion-color-medium); }
.mem-dot.imp-1 { background: var(--ion-color-tertiary); }
.mem-dot.imp-2 { background: var(--ion-color-primary); }
.mem-dot.imp-3 { background: var(--ion-color-success); }
.mem-dot.imp-4 { background: var(--ion-color-warning); }
.mem-dot.imp-5 { background: var(--ion-color-danger); }
.tick-info {
  color: var(--ion-color-medium);
  font-size: 12px;
  margin-right: 10px;
}
.strength {
  font-size: 12px;
  color: var(--ion-color-medium);
}
.empty-item {
  --padding-start: 0;
  --inner-padding-end: 0;
  justify-content: center;
  color: var(--ion-color-medium);
  padding: 24px 0;
}
</style>
