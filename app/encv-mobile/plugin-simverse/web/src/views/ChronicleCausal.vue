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

        <template v-else-if="event">
          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.events") }}</div>
              <div class="space-y-1">
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.events") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ event.type_cn }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.perf.tier") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ event.level_cn }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.tick") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ event.tick }}</span>
                </div>
                <div v-if="event.entity_id" class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">关联实体</span>
                  <span class="text-xs text-base-content/70 font-mono">#{{ event.entity_id }}</span>
                </div>
              </div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.causal.cause") }} ({{ event.causes?.length || 0 }})</div>
              <div v-if="event.causes?.length" class="space-y-1">
                <div
                  v-for="cause in event.causes"
                  :key="cause.id"
                  class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  @click="loadAndShowEvent(cause.id)"
                >
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium truncate">{{ cause.type_cn }}</div>
                    <div class="text-xs text-base-content/70">tick {{ cause.tick }} · {{ cause.level_cn }}</div>
                  </div>
                  <ion-icon :icon="chevronForward" class="text-base-content/40 ml-2" />
                </div>
              </div>
              <div v-else class="p-4 text-center text-sm text-base-content/50">
                {{ t("simverse.causal.noEvent") }}
              </div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.causal.effect") }} ({{ event.effects?.length || 0 }})</div>
              <div v-if="event.effects?.length" class="space-y-1">
                <div
                  v-for="eff in event.effects"
                  :key="eff.id"
                  class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  @click="loadAndShowEvent(eff.id)"
                >
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium truncate">{{ eff.type_cn }}</div>
                    <div class="text-xs text-base-content/70">tick {{ eff.tick }} · {{ eff.level_cn }}</div>
                  </div>
                  <ion-icon :icon="chevronForward" class="text-base-content/40 ml-2" />
                </div>
              </div>
              <div v-else class="p-4 text-center text-sm text-base-content/50">
                {{ t("simverse.causal.noEvent") }}
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
import { alertCircleOutline, chevronForward, refreshOutline } from "ionicons/icons";
import { onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { type SimverseChronicleEvent, useSimverse } from "@/composables/useSimverse";

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

<style scoped lang="scss">
.state-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.state-box p { color: var(--color-error); margin: 0; }
</style>
