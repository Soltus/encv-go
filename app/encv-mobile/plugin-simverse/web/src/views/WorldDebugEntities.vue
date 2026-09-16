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
        <div class="px-3">
          <div class="ui-input">
            <ion-icon :icon="searchOutline" class="text-base-content/50" />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('simverse.search')"
              class="flex-1 bg-transparent outline-none text-sm"
            />
          </div>
        </div>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4">
        <div v-if="loading" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>
        <div v-else-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="() => reload(true)">{{ t("settings.check") }}</button>
        </div>

        <div v-else class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">
              <span>{{ t("simverse.total") }}: {{ total }}</span>
            </div>
            <div class="space-y-1">
              <div
                v-for="npc in filtered"
                :key="npc.id"
                class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                @click="goNPC(npc.id)"
              >
                <div class="flex-1 min-w-0">
                  <h3 class="text-sm font-semibold m-0 mb-1">{{ npc.name }}</h3>
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="ui-chip !text-xs !py-0.5" :class="profChipClass(npc.profession)">
                      {{ npc.profession }}
                    </span>
                    <span class="text-xs text-base-content/70">{{ npc.age }}{{ t("simverse.yearsOld") }}</span>
                    <span class="text-xs text-base-content/70">{{ t("simverse.regions") }} #{{ npc.region_id }}</span>
                    <span class="text-xs text-base-content/70">{{ t("simverse.orgDetail") }} #{{ npc.org_id }}</span>
                  </div>
                </div>
                <span class="text-xs text-base-content/70 font-medium">Lv.{{ npc.level }}</span>
              </div>
              <div v-if="!filtered.length" class="p-8 text-center text-sm text-base-content/50">
                {{ t("simverse.noData") }}
              </div>
            </div>
          </div>
        </div>

        <div v-if="hasMore" class="text-center py-4">
          <button type="button" class="ui-button !text-sm" @click="loadMore" :disabled="loading">
            <span v-if="loading">{{ t("settings.loading") }}...</span>
            <span v-else>{{ t("simverse.loadMore") }}</span>
          </button>
        </div>
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
import { alertCircleOutline, searchOutline } from "ionicons/icons";
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

function loadMore() {
  reload(false);
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
    priest: "!bg-tertiary/15 !text-tertiary !border-tertiary/30",
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
