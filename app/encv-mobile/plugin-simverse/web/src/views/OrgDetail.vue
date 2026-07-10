<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/orgs" />
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
              <ion-row>
                <ion-col><div class="stat-label">{{ t("simverse.avgCareerStage") }}</div><div class="stat-val">{{ org.avg_career_stage.toFixed(1) }}</div></ion-col>
              </ion-row>
            </ion-grid>
          </ion-card-content>
        </ion-card>

        <ion-list-header><ion-label>{{ t("simverse.regionDistribution") }}</ion-label></ion-list-header>
        <div class="chips">
          <ion-chip v-for="(cnt, rid) in org.region_distribution" :key="rid" @click="goRegion(Number(rid))">
            <ion-label>{{ t("simverse.regions") }} #{{ rid }} · {{ cnt }}</ion-label>
          </ion-chip>
        </div>

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
  IonChip,
  IonCol,
  IonContent,
  IonGrid,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonPage,
  IonRow,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, map, people, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { type SimverseOrg, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadOrgDetail } = useSimverse();

const loading = ref(false);
const error = ref("");
const org = ref<SimverseOrg | null>(null);
const orgId = Number(route.params.id);

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
function goRegion(id: number) {
  router.push(`/region/${id}`);
}

onMounted(reload);
</script>

<style scoped>
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
  color: var(--ion-color-medium, #6b7280);
}
.stat-val {
  font-size: 22px;
  font-weight: 700;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 0 8px 16px;
}
</style>
