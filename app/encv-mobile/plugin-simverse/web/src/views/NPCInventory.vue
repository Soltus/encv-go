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

        <template v-else-if="npc">
          <div class="ui-card">
            <div class="p-4">
              <div class="ui-header mb-3 flex items-center gap-2">
                <ion-icon :icon="bagOutline" class="text-primary" />
                {{ t("simverse.npcInventory") }}
              </div>
              <div v-if="inventoryEntries.length" class="space-y-1">
                <div
                  v-for="item in inventoryEntries"
                  :key="item.key"
                  class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors"
                >
                  <div class="flex items-center gap-3">
                    <div class="ui-bubble w-8 h-8 flex items-center justify-center text-sm flex-shrink-0 !bg-base-200">
                      <ion-icon :icon="cubeOutline" class="text-base-content/70" />
                    </div>
                    <span class="text-sm font-medium">{{ item.key }}</span>
                  </div>
                  <span class="ui-chip ui-chip--mono !text-xs">
                    ×{{ item.value }}
                  </span>
                </div>
              </div>
              <div v-else class="empty-state">
                <ion-icon :icon="bagHandleOutline" class="text-base-content/30 text-3xl mb-2" />
                {{ t("simverse.detail.noInventory") }}
              </div>
            </div>
          </div>

          <div v-if="bankEntries.length" class="ui-card">
            <div class="p-4">
              <div class="ui-header mb-3 flex items-center gap-2">
                <ion-icon :icon="walletOutline" class="text-warning" />
                {{ t("simverse.gold") }} / {{ t("simverse.diamond") }}
              </div>
              <div class="space-y-1">
                <div
                  v-for="item in bankEntries"
                  :key="item.key"
                  class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors"
                >
                  <div class="flex items-center gap-3">
                    <div class="ui-bubble w-8 h-8 flex items-center justify-center text-sm flex-shrink-0 !bg-warning/10">
                      <ion-icon :icon="item.key.toLowerCase().includes('diamond') ? diamondOutline : cashOutline" :class="item.key.toLowerCase().includes('diamond') ? 'text-tertiary' : 'text-warning'" />
                    </div>
                    <span class="text-sm font-medium">{{ item.key }}</span>
                  </div>
                  <span class="text-sm font-mono font-semibold text-base-content/80">
                    {{ item.value }}
                  </span>
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
import {
  alertCircleOutline,
  refreshOutline,
  bagOutline,
  bagHandleOutline,
  cubeOutline,
  walletOutline,
  cashOutline,
  diamondOutline,
} from "ionicons/icons";
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
.state-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.state-box p {
  color: var(--color-error);
  margin: 0;
}
.empty-state {
  text-align: center;
  color: var(--color-base-content);
  opacity: 0.7;
  padding: 24px 0;
  font-size: 14px;
  display: flex;
  flex-direction: column;
  align-items: center;
}
</style>
