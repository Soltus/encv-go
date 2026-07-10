<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.socialOverview") }}</ion-title>
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

      <template v-else-if="stats">
        <!-- 概览卡片 -->
        <div class="stat-grid">
          <div class="stat-card">
            <span class="stat-value">{{ stats.total_relations }}</span>
            <span class="stat-label">{{ t("simverse.socialTotal") }}</span>
          </div>
          <div class="stat-card">
            <span class="stat-value">{{ stats.sampled_npcs }}</span>
            <span class="stat-label">{{ t("simverse.socialSampled") }}</span>
          </div>
          <div class="stat-card">
            <span class="stat-value">{{ typeCount }}</span>
            <span class="stat-label">{{ t("simverse.socialByType") }}</span>
          </div>
        </div>

        <!-- 关系类型分布 -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.socialByType") }}</ion-label></ion-list-header>
          <ion-item v-for="(cnt, type) in stats.by_type" :key="type">
            <ion-label>{{ t("simverse.rel." + type) }}</ion-label>
            <ion-note slot="end">{{ cnt }}</ion-note>
            <ion-progress-bar
              :value="maxCount ? cnt / maxCount : 0"
              slot="end"
              class="rel-bar"
              :color="barColor(type)"
            />
          </ion-item>
        </ion-list>

        <!-- 区域分布 -->
        <ion-list v-if="regionEntries.length" :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.socialByRegion") }}</ion-label></ion-list-header>
          <ion-item v-for="[rid, cnt] in regionEntries" :key="'r' + rid" button detail @click="goRegion(rid)">
            <ion-label>{{ t("simverse.regions") }} #{{ rid }}</ion-label>
            <ion-note slot="end">{{ cnt }}</ion-note>
          </ion-item>
        </ion-list>

        <!-- 组织分布 -->
        <ion-list v-if="orgEntries.length" :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.socialByOrg") }}</ion-label></ion-list-header>
          <ion-item v-for="[oid, cnt] in orgEntries" :key="'o' + oid" button detail @click="goOrg(oid)">
            <ion-label>{{ t("simverse.orgs") }} #{{ oid }}</ion-label>
            <ion-note slot="end">{{ cnt }}</ion-note>
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
  IonProgressBar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseSocialStats, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadSocialStats } = useSimverse();

const loading = ref(false);
const error = ref("");
const stats = ref<SimverseSocialStats | null>(null);

const typeCount = computed(() => Object.keys(stats.value?.by_type || {}).length);
const maxCount = computed(() => {
  const vals = Object.values(stats.value?.by_type || {});
  return vals.length ? Math.max(...vals) : 0;
});
const regionEntries = computed(() => Object.entries(stats.value?.by_region || {}).sort((a, b) => b[1] - a[1]));
const orgEntries = computed(() => Object.entries(stats.value?.by_org || {}).sort((a, b) => b[1] - a[1]));

function barColor(type: string): string {
  if (type === "enemy" || type === "rival") return "danger";
  if (type === "friend" || type === "lover" || type === "spouse") return "success";
  if (type === "parent" || type === "child" || type === "sibling" || type === "master" || type === "apprentice") return "tertiary";
  return "primary";
}

function goRegion(id: string) {
  router.push(`/region/${id}`);
}
function goOrg(id: string) {
  router.push(`/org/${id}`);
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    stats.value = await loadSocialStats();
    if (!stats.value) error.value = t("simverse.socialNoRelations");
  } catch (e: any) {
    error.value = e.message || "Failed to load social stats";
  } finally {
    loading.value = false;
  }
}

onMounted(reload);
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
.stat-grid {
  display: flex;
  gap: 10px;
  padding: 16px;
}
.stat-card {
  flex: 1;
  background: var(--ion-color-light, #f4f5f8);
  border-radius: 12px;
  padding: 14px 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}
.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--ion-color-primary);
}
.stat-label {
  font-size: 12px;
  color: var(--ion-color-medium);
}
.rel-bar {
  width: 80px;
  margin-left: 12px;
}
</style>
