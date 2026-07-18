<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/orgs" />
        </ion-buttons>
        <ion-title>{{ t("simverse.orgTerritory") }}</ion-title>
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
        <template v-else-if="data">
          <div class="ui-card">
            <div class="p-4">
              <h2 class="text-lg font-bold m-0 mb-1">{{ data.name }}</h2>
              <p class="text-sm text-base-content/60 m-0">#{{ data.org_id }}</p>
            </div>
          </div>

          <template v-if="data.territory.length">
            <div class="ui-header">{{ t("simverse.regionDistribution") }}</div>
            <div class="space-y-2">
              <div
                v-for="terr in data.territory"
                :key="terr.region_id"
                class="ui-card cursor-pointer hover:scale-[0.98] active:scale-[0.97] transition-transform"
                @click="goRegion(terr.region_id)"
              >
                <div class="p-3 flex items-center gap-3">
                  <div class="ui-bubble !p-0 !w-10 !h-10 flex items-center justify-center flex-shrink-0 !bg-success/15 !border-success/30">
                    <ion-icon :icon="location" class="text-success" />
                  </div>
                  <div class="flex-1 min-w-0">
                    <h3 class="text-base font-semibold m-0">{{ t("simverse.regions") }} #{{ terr.region_id }}</h3>
                  </div>
                  <div class="flex-shrink-0 text-sm font-mono text-base-content/70">
                    {{ terr.members }}
                  </div>
                </div>
              </div>
            </div>
          </template>
          <div v-else class="state-box">
            <p>{{ t("simverse.noRegions") }}</p>
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
import { alertCircleOutline, location, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { type SimverseOrgTerritoryResponse, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadOrgTerritory } = useSimverse();

const orgId = Number(route.params.id);
const loading = ref(false);
const error = ref("");
const data = ref<SimverseOrgTerritoryResponse | null>(null);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    data.value = await loadOrgTerritory(orgId);
  } catch (e: any) {
    error.value = e.message || "Failed to load territory";
  } finally {
    loading.value = false;
  }
}

function goRegion(id: number) {
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
