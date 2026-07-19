<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.npcs") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
      <ion-toolbar>
        <div class="px-2 w-full">
          <div class="ui-input">
            <ion-icon :icon="searchOutline" class="text-base-content/50 mr-2" />
            <input
              v-model="searchQuery"
              :placeholder="t('simverse.search')"
              class="flex-1 bg-transparent outline-none text-base-content"
            />
          </div>
        </div>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4 space-y-3">
        <div v-if="loading" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>

        <div v-else-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
        </div>

        <template v-else>
          <div class="ui-header justify-between">
            <span>{{ t("simverse.total") }}: {{ total }}</span>
          </div>

          <div class="space-y-2 list-container">
            <div
              v-for="npc in filteredNPCs"
              :key="npc.id"
              class="ui-card list-item cursor-pointer hover:scale-[0.98] active:scale-[0.97] transition-transform"
              @click="goToDetail(npc.id)"
            >
              <div class="p-3 flex items-center gap-3">
                <div class="ui-bubble !p-0 !w-10 !h-10 flex items-center justify-center text-xl flex-shrink-0">
                  {{ getAvatarEmoji(npc) }}
                </div>
                <div class="flex-1 min-w-0">
                  <h3 class="text-base font-semibold m-0 mb-1 truncate">{{ npc.name }}</h3>
                  <div class="flex items-center gap-2 flex-wrap mb-1">
                    <span class="ui-chip !text-xs !py-0.5" :class="profChipClass(npc.profession)">
                      {{ npc.profession }}
                    </span>
                    <span class="text-xs text-base-content/60">
                      {{ npc.age }}{{ t("simverse.yearsOld") }} · {{ npc.species }}
                    </span>
                  </div>
                  <div class="flex items-center gap-3 text-xs">
                    <span :class="{ 'text-success': npc.is_alive, 'text-base-content/50': !npc.is_alive }" class="flex items-center gap-1">
                      <ion-icon :icon="npc.is_alive ? heartOutline : skullOutline" :class="npc.is_alive ? 'text-success' : 'text-base-content/50'" />
                      {{ npc.is_alive ? t("simverse.alive") : t("simverse.deceased") }}
                    </span>
                    <span v-if="npc.health !== undefined" class="text-xs font-mono text-base-content/70">
                      HP: {{ Math.round(npc.health) }}/{{ npc.max_health }}
                    </span>
                  </div>
                </div>
                <div class="flex-shrink-0 text-xs font-mono text-base-content/70">
                  Lv.{{ npc.level }}
                </div>
              </div>
            </div>
          </div>
        </template>

        <ion-infinite-scroll
          v-if="!loading && !error && hasMore"
          @ionInfinite="loadMore"
          threshold="100px"
        >
          <ion-infinite-scroll-content
            :loading-text="t('settings.loading')"
          />
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
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline, searchOutline, heartOutline, skullOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useGsap } from "@/composables/useGsap";
import { type SimverseNPC, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadNPCList } = useSimverse();
const { gsap, ScrollTrigger } = useGsap();

const loading = ref(false);
const error = ref("");
const searchQuery = ref("");
const npcs = ref<SimverseNPC[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 50;
const hasMore = ref(true);

const filteredNPCs = computed(() => {
  if (!searchQuery.value) return npcs.value;
  const q = searchQuery.value.toLowerCase();
  return npcs.value.filter(
    n => n.name.toLowerCase().includes(q) || n.profession.toLowerCase().includes(q) || n.species.toLowerCase().includes(q)
  );
});

async function loadNPCs(isRefresh = false) {
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
    if (isRefresh) {
      npcs.value = data.items;
    } else {
      npcs.value = [...npcs.value, ...data.items];
    }
    total.value = data.total;
    hasMore.value = npcs.value.length < data.total;
    page.value++;
  } catch (e: any) {
    error.value = e.message || "Failed to load NPCs";
  } finally {
    loading.value = false;
  }
}

function reload() {
  loadNPCs(true);
}

function loadMore(ev: any) {
  loadNPCs().finally(() => {
    ev.target.complete();
  });
}

function goToDetail(id: number) {
  router.push(`/npc/${id}`);
}

function getAvatarEmoji(npc: SimverseNPC): string {
  const avatars = ["🧙", "⚔️", "🛡️", "🧑‍🌾", "👨‍🍳", "🗡️", "🏹", "📜", "💎", "🪓"];
  const idx = npc.id % avatars.length;
  return avatars[idx];
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

onMounted(() => {
  loadNPCs(true);
  gsap.from(".list-item", {
    opacity: 0,
    y: 20,
    duration: 0.4,
    stagger: 0.05,
    ease: "power2.out",
    scrollTrigger: {
      trigger: ".list-container",
      start: "top 80%",
    },
  });
  void ScrollTrigger;
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
</style>
