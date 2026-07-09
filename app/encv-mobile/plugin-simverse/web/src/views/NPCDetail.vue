<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/npcs" />
        </ion-buttons>
        <ion-title>{{ t("simverse.npcDetail") }}</ion-title>
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

      <div v-else-if="npc" class="detail">
        <!-- 头部卡片 -->
        <div class="hero">
          <div class="hero-avatar">{{ avatarEmoji }}</div>
          <div class="hero-info">
            <h2>{{ npc.name }}</h2>
            <p>
              <ion-badge :color="professionColor(npc.profession)" size="small">
                {{ npc.profession }}
              </ion-badge>
              <span class="hero-meta">Lv.{{ npc.level }}</span>
              <span :class="npc.is_alive ? 'alive' : 'dead'">
                {{ npc.is_alive ? '❤' : '💀' }}
                {{ npc.is_alive ? t("simverse.alive") : t("simverse.deceased") }}
              </span>
            </p>
            <p class="hero-sub">
              {{ npc.age }}{{ t("simverse.yearsOld") }} ·
              {{ npc.species }} ·
              {{ npc.life_stage }}
              <span v-if="npc.current_behavior_cn">· {{ npc.current_behavior_cn }}</span>
            </p>
          </div>
        </div>

        <!-- 快捷入口 -->
        <ion-list :inset="true">
          <ion-item button detail @click="goSub('inventory')">
            <ion-icon :icon="bagOutline" slot="start" color="primary" />
            <ion-label>{{ t("simverse.detail.viewInventory") }}</ion-label>
          </ion-item>
          <ion-item button detail @click="goSub('timeline')">
            <ion-icon :icon="timeOutline" slot="start" color="tertiary" />
            <ion-label>{{ t("simverse.detail.viewTimeline") }}</ion-label>
          </ion-item>
          <ion-item button detail @click="goSub('relations')">
            <ion-icon :icon="gitNetworkOutline" slot="start" color="success" />
            <ion-label>{{ t("simverse.detail.viewRelations") }}</ion-label>
          </ion-item>
          <ion-item button detail @click="goSub('behavior')">
            <ion-icon :icon="pulseOutline" slot="start" color="warning" />
            <ion-label>{{ t("simverse.detail.viewBehavior") }}</ion-label>
          </ion-item>
        </ion-list>

        <!-- 核心属性 -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.stats") }}</ion-label></ion-list-header>
          <ion-item>
            <ion-label>{{ t("simverse.health") }}</ion-label>
            <ion-note slot="end">{{ Math.round(npc.health) }}/{{ npc.max_health }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.energy") }}</ion-label>
            <ion-note slot="end">{{ Math.round(npc.energy) }}/{{ npc.max_energy }}</ion-note>
          </ion-item>
          <ion-item v-if="npc.mana !== undefined">
            <ion-label>{{ t("simverse.detail.mana") }}</ion-label>
            <ion-note slot="end">{{ Math.round(npc.mana) }}/{{ npc.max_mana }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.detail.mood") }}</ion-label>
            <ion-note slot="end">{{ npc.mood ?? '—' }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.detail.satisfaction") }}</ion-label>
            <ion-note slot="end">{{ npc.satisfaction ?? '—' }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.detail.experience") }}</ion-label>
            <ion-note slot="end">{{ npc.experience ?? '—' }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.wealthTier") }}</ion-label>
            <ion-note slot="end">{{ npc.wealth_tier }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.socialTier") }}</ion-label>
            <ion-note slot="end">{{ npc.social_tier }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.detail.children") }}</ion-label>
            <ion-note slot="end">{{ npc.num_children ?? '—' }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.detail.marriages") }}</ion-label>
            <ion-note slot="end">{{ npc.num_marriages ?? '—' }}</ion-note>
          </ion-item>
        </ion-list>

        <!-- 归属 -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.detail.personality") }}</ion-label></ion-list-header>
          <ion-item button detail v-if="npc.org_id" @click="goOrg(npc.org_id)">
            <ion-label>{{ t("simverse.detail.orgId") }}</ion-label>
            <ion-note slot="end">#{{ npc.org_id }}</ion-note>
          </ion-item>
          <ion-item button detail v-if="npc.region_id" @click="goRegion(npc.region_id)">
            <ion-label>{{ t("simverse.detail.regionId") }}</ion-label>
            <ion-note slot="end">#{{ npc.region_id }}</ion-note>
          </ion-item>
          <ion-item v-if="npc.home_region_id">
            <ion-label>{{ t("simverse.detail.homeRegion") }}</ion-label>
            <ion-note slot="end">#{{ npc.home_region_id }}</ion-note>
          </ion-item>
          <ion-item v-if="npc.born_at">
            <ion-label>{{ t("simverse.detail.bornAt") }}</ion-label>
            <ion-note slot="end">tick {{ npc.born_at }}</ion-note>
          </ion-item>
          <ion-item v-if="npc.died_at">
            <ion-label>{{ t("simverse.detail.diedAt") }}</ion-label>
            <ion-note slot="end">tick {{ npc.died_at }}</ion-note>
          </ion-item>
        </ion-list>

        <!-- 技能 -->
        <ion-list v-if="skillEntries.length" :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.skills") }}</ion-label></ion-list-header>
          <ion-item v-for="s in skillEntries" :key="s.key">
            <ion-label>{{ s.key }}</ion-label>
            <ion-note slot="end">{{ s.value }}</ion-note>
          </ion-item>
        </ion-list>

        <!-- 大五人格 -->
        <ion-list v-if="bigFiveEntries.length" :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.detail.bigFive") }}</ion-label></ion-list-header>
          <ion-item v-for="b in bigFiveEntries" :key="b.key">
            <ion-label>{{ b.key }}</ion-label>
            <ion-note slot="end">{{ b.value }}</ion-note>
          </ion-item>
        </ion-list>

        <!-- 价值观 / 兴趣 -->
        <ion-list v-if="npc.top_values?.length || npc.top_interests?.length" :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.detail.values") }} / {{ t("simverse.detail.interests") }}</ion-label></ion-list-header>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <div v-if="npc.top_values?.length" class="chip-row">
                <ion-chip v-for="v in npc.top_values" :key="v" color="tertiary" outline>{{ v }}</ion-chip>
              </div>
              <div v-if="npc.top_interests?.length" class="chip-row">
                <ion-chip v-for="i in npc.top_interests" :key="i" color="success" outline>{{ i }}</ion-chip>
              </div>
            </ion-label>
          </ion-item>
        </ion-list>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonList, IonListHeader, IonLabel,
  IonItem, IonBadge, IonNote, IonSpinner, IonChip,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline, bagOutline, timeOutline, gitNetworkOutline, pulseOutline } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseNPCDetail } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadNPCDetail, recordQuestAction } = useSimverse();

const loading = ref(false);
const error = ref("");
const npc = ref<SimverseNPCDetail | null>(null);

const avatarEmoji = computed(() => {
  const avatars = ["🧙", "⚔️", "🛡️", "🧑‍🌾", "👨‍🍳", "🗡️", "🏹", "📜", "💎", "🪓"];
  const id = Number(route.params.id) || 0;
  return avatars[id % avatars.length];
});

const skillEntries = computed(() =>
  npc.value ? Object.entries(npc.value.skills || {}).map(([key, value]) => ({ key, value })) : []
);
const bigFiveEntries = computed(() =>
  npc.value ? Object.entries(npc.value.big_five || {}).map(([key, value]) => ({ key, value })) : []
);

function professionColor(profession: string): string {
  const map: Record<string, string> = {
    farmer: "success", warrior: "danger", mage: "primary",
    merchant: "warning", priest: "tertiary", rogue: "medium",
  };
  return map[profession?.toLowerCase()] || "medium";
}

function goSub(which: "inventory" | "timeline" | "relations" | "behavior") {
  router.push(`/npc/${route.params.id}/${which}`);
}
function goOrg(id: number) {
  router.push(`/org/${id}`);
}
function goRegion(id: number) {
  router.push(`/region/${id}`);
}

async function reload() {
  const id = Number(route.params.id);
  if (!id) return;
  loading.value = true;
  error.value = "";
  try {
    npc.value = await loadNPCDetail(id);
    recordQuestAction("view_npc");
  } catch (e: any) {
    error.value = e.message || "Failed to load NPC";
  } finally {
    loading.value = false;
  }
}

onMounted(reload);
watch(() => route.params.id, reload);
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
.hero {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 16px 8px;
}
.hero-avatar {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: var(--ion-color-light, #f3f4f6);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 30px;
  flex-shrink: 0;
}
.hero-info h2 {
  margin: 0 0 4px;
  font-size: 18px;
}
.hero-info p {
  margin: 0 0 4px;
  font-size: 12px;
  color: var(--ion-color-medium);
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.hero-meta {
  font-weight: 600;
}
.hero-sub {
  font-size: 12px;
}
.alive { color: var(--ion-color-success); }
.dead { color: var(--ion-color-medium); }
.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px 0;
}
</style>
