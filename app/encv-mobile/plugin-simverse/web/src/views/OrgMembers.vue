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
      <div class="p-4 space-y-3">
        <div v-if="loading && members.length === 0" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>
        <div v-else-if="error && members.length === 0" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="() => reload(true)">{{ t("settings.check") }}</button>
        </div>

        <template v-else>
          <div class="ui-header justify-between">
            <span>{{ t("simverse.total") }}: {{ total }}</span>
          </div>

          <div class="space-y-2 list-container">
            <div
              v-for="m in filtered"
              :key="m.id"
              class="ui-card list-item cursor-pointer hover:scale-[0.98] active:scale-[0.97] transition-transform"
              @click="goNPC(m.id)"
            >
              <div class="p-3 flex items-center gap-3">
                <div class="ui-bubble !p-0 !w-10 !h-10 flex items-center justify-center text-xl flex-shrink-0">
                  {{ m.name?.[0] || '?' }}
                </div>
                <div class="flex-1 min-w-0">
                  <h3 class="text-base font-semibold m-0 mb-1 truncate">{{ m.name }}</h3>
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="ui-chip !text-xs !py-0.5" :class="profChipClass(m.profession)">
                      {{ m.profession }}
                    </span>
                    <span class="text-xs text-base-content/60">
                      {{ m.age }}{{ t("simverse.yearsOld") }} · {{ t("simverse.regions") }} #{{ m.region_id }}
                    </span>
                  </div>
                </div>
                <div class="flex-shrink-0 text-xs font-mono text-base-content/70">
                  Lv.{{ m.level }}
                </div>
              </div>
            </div>
          </div>
        </template>

        <ion-infinite-scroll v-if="!loading && !error && hasMore" @ionInfinite="loadMore" threshold="100px">
          <ion-infinite-scroll-content :loading-text="t('settings.loading')" />
        </ion-infinite-scroll>
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
  IonInfiniteScroll,
  IonInfiniteScrollContent,
  IonPage,
  IonSearchbar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline } from "ionicons/icons";
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

function profChipClass(p: string): string {
  const map: Record<string, string> = {
    farmer: "!bg-success/15 !text-success !border-success/30",
    warrior: "!bg-error/15 !text-error !border-error/30",
    mage: "!bg-primary/15 !text-primary !border-primary/30",
    merchant: "!bg-warning/15 !text-warning !border-warning/30",
    priest: "!bg-info/15 !text-info !border-info/30",
    rogue: "!bg-base-content/15 !text-base-content/70 !border-base-content/20",
  };
  return map[p.toLowerCase()] || "!bg-base-content/15 !text-base-content/70 !border-base-content/20";
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
</style>
