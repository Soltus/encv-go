<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.entityBrowser") }}</ion-title>
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
          v-for="npc in filtered"
          :key="npc.id"
          button
          detail
          @click="goNPC(npc.id)"
        >
          <ion-label>
            <h3>{{ npc.name }}</h3>
            <p>
              <ion-badge :color="profColor(npc.profession)" size="small">{{ npc.profession }}</ion-badge>
              <span class="meta">{{ npc.age }}{{ t("simverse.yearsOld") }} · {{ t("simverse.regions") }} #{{ npc.region_id }} · {{ t("simverse.orgDetail") }} #{{ npc.org_id }}</span>
            </p>
          </ion-label>
          <ion-note slot="end" color="medium">Lv.{{ npc.level }}</ion-note>
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
import { useRouter } from "vue-router";
import { type SimverseNPC, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadNPCList } = useSimverse();

const loading = ref(false);
const error = ref("");
const searchQuery = ref("");
const npcs = ref<SimverseNPC[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 50;
const hasMore = ref(true);

const filtered = computed(() => {
  if (!searchQuery.value) return npcs.value;
  const q = searchQuery.value.toLowerCase();
  return npcs.value.filter(n => n.name.toLowerCase().includes(q) || n.profession.toLowerCase().includes(q));
});

async function reload(isRefresh = false) {
  if (isRefresh) {
    page.value = 1;
    hasMore.value = true;
    npcs.value = [];
  }
  if (!hasMore.value) return;
  loading.value = true;
  error.value = "";
  try {
    const data = await loadNPCList(page.value, pageSize);
    npcs.value = isRefresh ? data.items : [...npcs.value, ...data.items];
    total.value = data.total;
    hasMore.value = npcs.value.length < data.total;
    page.value++;
  } catch (e: any) {
    error.value = e.message || "Failed to load entities";
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
