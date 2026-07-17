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

    <ion-content class="ion-padding">
      <div class="page-root">
      <div v-if="loading" class="state-box">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-box">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>
      <template v-else-if="org">
        <ion-card>
          <ion-card-header>
            <ion-card-title>{{ org.name }}</ion-card-title>
            <ion-card-subtitle>
              <ion-badge color="tertiary">{{ org.org_type }}</ion-badge>
              #{{ org.org_id }}
            </ion-card-subtitle>
          </ion-card-header>
          <ion-card-content>
            <ion-grid>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.memberCount") }}</div><div class="stat-val">{{ org.member_count }}</div></ion-col>
                <ion-col><div class="stat-label">{{ t("simverse.aliveCount") }}</div><div class="stat-val">{{ org.alive_count }}</div></ion-col>
              </ion-row>
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.avgLevel") }}</div><div class="stat-val">{{ org.avg_level.toFixed(1) }}</div></ion-col>
                <ion-col><div class="stat-label">{{ t("simverse.avgWealthTier") }}</div><div class="stat-val">{{ org.avg_wealth_tier.toFixed(1) }}</div></ion-col>
              </ion-row>
            </ion-grid>
          </ion-card-content>
        </ion-card>

        <ion-list :inset="true">
          <ion-item button detail @click="go(`/org/${org.org_id}/members`)">
            <ion-icon :icon="people" slot="start" color="primary" />
            <ion-label>{{ t("simverse.orgMembers") }}</ion-label>
          </ion-item>
          <ion-item button detail @click="go(`/org/${org.org_id}/territory`)">
            <ion-icon :icon="map" slot="start" color="success" />
            <ion-label>{{ t("simverse.orgTerritory") }}</ion-label>
          </ion-item>
        </ion-list>
      </template>
      </div>
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
  IonCardSubtitle,
  IonCardTitle,
  IonCol,
  IonContent,
  IonGrid,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonPage,
  IonRow,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, map, people, refreshOutline } from "ionicons/icons";
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
// useRouteTransition exposes onEnter/onLeave hooks for a parent <Transition> wrapper.
// App.vue currently uses plain ion-router-outlet, so this view self-animates
// .page-root via gsap.fromTo as a fallback (matches useRouteTransition defaults).
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
  // 详情路由入场动效（与 useRouteTransition 默认参数一致）
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
}
</style>
