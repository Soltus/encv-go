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
        <ion-searchbar
          v-model="searchQuery"
          :placeholder="t('simverse.search')"
          mode="ios"
          :debounce="200"
        />
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading" class="loading-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>

      <div v-else-if="error" class="error-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>

      <ion-list v-else :inset="true" class="list-container">
        <ion-list-header>
          <ion-label>{{ t("simverse.total") }}: {{ total }}</ion-label>
        </ion-list-header>
        <ion-item
          v-for="npc in filteredNPCs"
          :key="npc.id"
          class="list-item"
          button
          detail
          @click="goToDetail(npc.id)"
        >
          <div slot="start" class="npc-avatar">
            {{ getAvatarEmoji(npc) }}
          </div>
          <ion-label>
            <h3>{{ npc.name }}</h3>
            <p>
              <ion-badge :color="getProfessionColor(npc.profession)" size="small">
                {{ npc.profession }}
              </ion-badge>
              <span class="npc-meta">
                {{ npc.age }}{{ t("simverse.yearsOld") }} · {{ npc.species }}
              </span>
            </p>
            <p class="npc-secondary">
              <span :class="{ 'alive': npc.is_alive, 'dead': !npc.is_alive }">
                {{ npc.is_alive ? '❤' : '💀' }}
                {{ npc.is_alive ? t("simverse.alive") : t("simverse.deceased") }}
              </span>
              <span v-if="npc.health !== undefined" class="hp-bar">
                HP: {{ Math.round(npc.health) }}/{{ npc.max_health }}
              </span>
            </p>
          </ion-label>
          <ion-note slot="end" color="medium">Lv.{{ npc.level }}</ion-note>
        </ion-item>
      </ion-list>

      <ion-infinite-scroll
        v-if="!loading && !error && hasMore"
        @ionInfinite="loadMore"
        threshold="100px"
      >
        <ion-infinite-scroll-content
          :loading-text="t('settings.loading')"
        />
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

function getProfessionColor(profession: string): string {
  const map: Record<string, string> = {
    farmer: "success",
    warrior: "danger",
    mage: "primary",
    merchant: "warning",
    priest: "tertiary",
    rogue: "medium",
  };
  return map[profession.toLowerCase()] || "medium";
}

onMounted(() => {
  loadNPCs(true);
  // 列表项入场动效：滚动到视口时渐入
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
.loading-container,
.error-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.error-container p {
  color: var(--color-error);
  margin: 0;
}
.npc-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--color-base-200);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
}
h3 {
  font-size: 15px;
  font-weight: 600;
  margin: 0 0 4px 0;
}
p {
  font-size: 12px;
  color: var(--color-base-content);
  margin: 0 0 4px 0;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.npc-meta {
  font-size: 12px;
}
.npc-secondary {
  font-size: 11px;
  gap: 12px;
}
.alive {
  color: var(--color-success);
}
.dead {
  color: var(--color-base-content);
  opacity: 0.6;
}
.hp-bar {
  font-family: monospace;
}
</style>
