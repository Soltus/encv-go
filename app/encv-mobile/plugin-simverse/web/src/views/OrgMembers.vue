<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/orgs" />
        </ion-buttons>
        <ion-title>{{ t("simverse.orgMembers") }}</ion-title>
      </ion-toolbar>
      <ion-toolbar>
        <ion-searchbar
          v-model="searchQuery"
          :placeholder="t('simverse.search')"
          mode="ios"
          :debounce="200"
        />
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

      <ion-list v-else :inset="true">
        <ion-list-header>
          <ion-label>{{ t("simverse.total") }}: {{ total }}</ion-label>
        </ion-list-header>
        <ion-item
          v-for="m in filtered"
          :key="m.id"
          button
          detail
          @click="goNPC(m.id)"
        >
          <ion-label>
            <h3>{{ m.name }}</h3>
            <p>
              <ion-badge :color="profColor(m.profession)" size="small">{{ m.profession }}</ion-badge>
              <span class="meta">{{ m.age }}{{ t("simverse.yearsOld") }} · {{ t("simverse.regions") }} #{{ m.region_id }}</span>
            </p>
          </ion-label>
          <ion-note slot="end" color="medium">Lv.{{ m.level }}</ion-note>
        </ion-item>
      </ion-list>

      <ion-infinite-scroll v-if="!loading && !error && hasMore" @ionInfinite="loadMore" threshold="100px">
        <ion-infinite-scroll-content :loading-text="t('settings.loading')" />
      </ion-infinite-scroll>
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
  IonInfiniteScroll,
  IonInfiniteScrollContent,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonSearchbar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { type SimverseOrgMember, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadOrgMembers } = useSimverse();

const orgId = Number(route.params.id);
const loading = ref(false);
const error = ref("");
const searchQuery = ref("");
const members = ref<SimverseOrgMember[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 50;
const hasMore = ref(true);

const filtered = computed(() => {
  if (!searchQuery.value) return members.value;
  const q = searchQuery.value.toLowerCase();
  return members.value.filter(m => m.name.toLowerCase().includes(q) || m.profession.toLowerCase().includes(q));
});

async function reload(isRefresh = false) {
  if (isRefresh) {
    page.value = 1;
    hasMore.value = true;
    members.value = [];
  }
  if (!hasMore.value) return;
  loading.value = true;
  error.value = "";
  try {
    const data = await loadOrgMembers(orgId, page.value, pageSize);
    if (data) {
      members.value = isRefresh ? data.items : [...members.value, ...data.items];
      total.value = data.total;
      hasMore.value = members.value.length < data.total;
      page.value++;
    }
  } catch (e: any) {
    error.value = e.message || "Failed to load members";
  } finally {
    loading.value = false;
  }
}

function loadMore(ev: any) {
  reload().finally(() => ev.target.complete());
}

function goNPC(id: number) {
  router.push(`/npc/${id}`);
}

function profColor(p: string): string {
  const map: Record<string, string> = {
    farmer: "success",
    warrior: "danger",
    mage: "primary",
    merchant: "warning",
    priest: "tertiary",
    rogue: "medium",
  };
  return map[p.toLowerCase()] || "medium";
}

onMounted(() => reload(true));
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
