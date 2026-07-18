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
      <div class="p-4 space-y-4">
        <div v-if="loading" class="state-container">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>

        <div v-else-if="error" class="state-container">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
        </div>

        <template v-else-if="npc">
          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header">
                {{ t("simverse.detail.lifeEvents") }}: {{ npc.life_events ?? 0 }}
              </div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-3">{{ t("simverse.detail.memory") }}</div>
              <div v-if="memories.length" class="space-y-3">
                <div
                  v-for="mem in memories"
                  :key="mem.id"
                  class="flex items-start gap-3 py-2"
                >
                  <div class="mem-dot mt-1.5 flex-shrink-0" :class="`imp-${mem.importance}`" />
                  <div class="flex-1 min-w-0">
                    <h3 class="text-sm font-medium m-0 mb-1">{{ memTypeLabel(mem.type) }}</h3>
                    <p class="text-xs text-base-content/60 m-0 flex items-center gap-3">
                      <span class="tick-info">
                        <ion-icon :icon="timeOutline" class="inline-block mr-1 text-xs" />
                        tick {{ mem.created_at }}
                      </span>
                      <span class="strength">
                        <ion-icon :icon="flashOutline" class="inline-block mr-1 text-xs" />
                        强度 {{ mem.strength }}
                      </span>
                    </p>
                  </div>
                </div>
              </div>
              <div v-else class="empty-item text-center py-6 text-base-content/60 text-sm">
                {{ t("simverse.detail.noMemory") }}
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
import { alertCircleOutline, refreshOutline, timeOutline, flashOutline } from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { type SimverseMemory, type SimverseNPCDetail, useSimverse } from "@/composables/useSimverse";

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
.mem-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--color-base-content);
}
.mem-dot.imp-0 { background: var(--color-base-content); opacity: 0.5; }
.mem-dot.imp-1 { background: var(--color-accent); }
.mem-dot.imp-2 { background: var(--color-primary); }
.mem-dot.imp-3 { background: var(--color-success); }
.mem-dot.imp-4 { background: var(--color-warning); }
.mem-dot.imp-5 { background: var(--color-error); }
</style>
