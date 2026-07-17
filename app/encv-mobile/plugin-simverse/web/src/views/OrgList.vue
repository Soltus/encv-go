<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.orgs") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading" class="state-box">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-box">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>
      <div v-else-if="!orgs.length" class="state-box">
        <ion-icon :icon="business" size="large" color="medium" />
        <p>{{ t("simverse.noOrgs") }}</p>
      </div>

      <ion-list v-else :inset="true">
        <ion-list-header>
          <ion-label>{{ t("simverse.total") }}: {{ total }}</ion-label>
        </ion-list-header>
        <ion-item
          v-for="org in orgs"
          :key="org.org_id"
          button
          detail
          @click="goDetail(org.org_id)"
        >
          <ion-icon :icon="gitNetwork" slot="start" color="primary" />
          <ion-label>
            <h3>{{ org.name }}</h3>
            <p>
              <ion-badge color="tertiary" size="small">{{ org.org_type }}</ion-badge>
              <span class="meta">{{ t("simverse.memberCount") }}: {{ org.member_count }}</span>
            </p>
          </ion-label>
          <ion-note slot="end" color="medium">Lv.{{ org.avg_level.toFixed(1) }}</ion-note>
        </ion-item>
      </ion-list>
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
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, business, gitNetwork, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseOrg, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadOrgList } = useSimverse();

const loading = ref(false);
const error = ref("");
const orgs = ref<SimverseOrg[]>([]);
const total = ref(0);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const data = await loadOrgList();
    if (data) {
      orgs.value = data.items;
      total.value = data.count;
    }
  } catch (e: any) {
    error.value = e.message || "Failed to load orgs";
  } finally {
    loading.value = false;
  }
}

function goDetail(id: number) {
  router.push(`/org/${id}`);
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

.meta {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
  margin-left: 8px;
}
</style>
