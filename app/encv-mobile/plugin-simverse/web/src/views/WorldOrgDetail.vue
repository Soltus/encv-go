<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.orgDetail") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="page-root p-4 space-y-4">
      <div v-if="loading" class="state-box">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-box">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
      </div>
      <template v-else-if="org">
        <div class="ui-card">
          <div class="p-4">
            <h2 class="text-xl font-bold m-0 mb-1">{{ org.name }}</h2>
            <div class="flex items-center gap-2 flex-wrap">
              <span class="ui-chip !text-xs !py-0.5 !bg-tertiary/15 !text-tertiary !border-tertiary/30">
                {{ org.org_type }}
              </span>
              <span class="text-xs text-base-content/70 font-mono">#{{ org.org_id }}</span>
            </div>
          </div>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.stats") }}</div>
            <div class="grid grid-cols-2 gap-3">
              <div class="p-3 bg-base-200 rounded-lg text-center">
                <div class="stat-label">{{ t("simverse.memberCount") }}</div>
                <div class="stat-val">{{ org.member_count }}</div>
              </div>
              <div class="p-3 bg-base-200 rounded-lg text-center">
                <div class="stat-label">{{ t("simverse.aliveCount") }}</div>
                <div class="stat-val">{{ org.alive_count }}</div>
              </div>
              <div class="p-3 bg-base-200 rounded-lg text-center">
                <div class="stat-label">{{ t("simverse.avgLevel") }}</div>
                <div class="stat-val">{{ org.avg_level.toFixed(1) }}</div>
              </div>
              <div class="p-3 bg-base-200 rounded-lg text-center">
                <div class="stat-label">{{ t("simverse.avgWealthTier") }}</div>
                <div class="stat-val">{{ org.avg_wealth_tier.toFixed(1) }}</div>
              </div>
            </div>
          </div>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.actions") }}</div>
            <div class="space-y-1">
              <div
                class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                @click="go(`/org/${org.org_id}/members`)"
              >
                <ion-icon :icon="peopleOutline" class="text-primary text-xl" />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium">{{ t("simverse.orgMembers") }}</div>
                </div>
                <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
              </div>
              <div
                class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                @click="go(`/org/${org.org_id}/territory`)"
              >
                <ion-icon :icon="mapOutline" class="text-success text-xl" />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium">{{ t("simverse.orgTerritory") }}</div>
                </div>
                <ion-icon :icon="chevronForwardOutline" class="text-base-content/40" />
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
import { alertCircleOutline, chevronForwardOutline, mapOutline, peopleOutline, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useGsap } from "@/composables/useGsap";
import { useRouteTransition } from "@/composables/useRouteTransition";
import { type SimverseOrg, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadOrgDetail } = useSimverse();
const { gsap } = useGsap();
useRouteTransition();

const orgId = Number(route.params.id);
const loading = ref(false);
const error = ref("");
const org = ref<SimverseOrg | null>(null);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    org.value = await loadOrgDetail(orgId);
  } catch (e: any) {
    error.value = e.message || "Failed to load org";
  } finally {
    loading.value = false;
  }
}

function go(path: string) {
  router.push(path);
}

onMounted(() => {
  reload();
  const el = document.querySelector(".page-root");
  if (el) {
    gsap.fromTo(el, { opacity: 0, y: 24 }, { opacity: 1, y: 0, duration: 0.35, ease: "power2.out" });
  }
});
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
.stat-label {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
}
.stat-val {
  font-size: 22px;
  font-weight: 700;
  margin-top: 4px;
}
</style>
