<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.regions") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4 space-y-3">
        <div v-if="loading" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>
        <div v-else-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
        </div>
        <div v-else-if="!regions.length" class="state-box">
          <ion-icon :icon="map" size="large" color="medium" />
          <p>{{ t("simverse.noRegions") }}</p>
        </div>

        <template v-else>
          <div class="ui-header justify-between">
            <span>{{ t("simverse.total") }}: {{ total }}</span>
          </div>

          <div class="space-y-2">
            <div
              v-for="r in regions"
              :key="r.region_id"
              class="ui-card cursor-pointer hover:scale-[0.98] active:scale-[0.97] transition-transform"
              @click="goDetail(r.region_id)"
            >
              <div class="p-3 flex items-center gap-3">
                <div class="ui-bubble !p-0 !w-10 !h-10 flex items-center justify-center flex-shrink-0 !bg-success/15 !border-success/30">
                  <ion-icon :icon="location" class="text-success" />
                </div>
                <div class="flex-1 min-w-0">
                  <h3 class="text-base font-semibold m-0 mb-1">{{ t("simverse.regions") }} #{{ r.region_id }}</h3>
                  <div class="flex items-center gap-3 text-xs text-base-content/60">
                    <span>{{ t("simverse.population") }}: {{ r.population }}</span>
                    <span>{{ t("simverse.npc") }}s: {{ r.npc_count }}</span>
                  </div>
                </div>
                <div class="flex-shrink-0 text-xs font-mono text-base-content/70">
                  Lv.{{ r.avg_level.toFixed(1) }}
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
import { alertCircleOutline, location, map, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseRegion, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadRegionList } = useSimverse();

const loading = ref(false);
const error = ref("");
const regions = ref<SimverseRegion[]>([]);
const total = ref(0);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const data = await loadRegionList();
    if (data) {
      regions.value = data.items;
      total.value = data.count;
    }
  } catch (e: any) {
    error.value = e.message || "Failed to load regions";
  } finally {
    loading.value = false;
  }
}

function goDetail(id: number) {
  router.push(`/region/${id}`);
}

onMounted(reload);
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
</style>
