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
        <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
      </div>

      <div v-else-if="npc" class="p-4 space-y-4">
        <!-- 头部卡片 -->
        <div class="ui-card">
          <div class="p-4 flex items-start gap-4">
            <div class="ui-bubble w-16 h-16 flex items-center justify-center text-3xl flex-shrink-0">
              {{ avatarEmoji }}
            </div>
            <div class="flex-1 min-w-0">
              <h2 class="text-xl font-bold m-0 mb-2">{{ npc.name }}</h2>
              <div class="flex items-center gap-2 flex-wrap mb-2">
                <span class="ui-chip !text-xs" :class="profChipClass(npc.profession)">
                  {{ npc.profession }}
                </span>
                <span class="text-sm font-semibold text-base-content/80">Lv.{{ npc.level }}</span>
                <span :class="npc.is_alive ? 'text-success' : 'text-base-content/50'" class="flex items-center gap-1 text-sm">
                  <ion-icon :icon="npc.is_alive ? heartOutline : skullOutline" :class="npc.is_alive ? 'text-success' : 'text-base-content/50'" />
                  {{ npc.is_alive ? t("simverse.alive") : t("simverse.deceased") }}
                </span>
              </div>
              <p class="text-sm text-base-content/70 m-0">
                {{ npc.age }}{{ t("simverse.yearsOld") }} ·
                {{ npc.species }} ·
                {{ npc.life_stage }}
                <span v-if="npc.current_behavior_cn">· {{ npc.current_behavior_cn }}</span>
              </p>
            </div>
          </div>
        </div>

        <!-- 快捷入口 -->
        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.detail.quickAccess") }}</div>
            <div class="grid grid-cols-2 gap-2">
              <button type="button" class="ui-button ui-button--ghost justify-start" @click="goSub('inventory')">
                <ion-icon :icon="bagOutline" class="mr-2 text-primary" />
                {{ t("simverse.detail.viewInventory") }}
              </button>
              <button type="button" class="ui-button ui-button--ghost justify-start" @click="goSub('timeline')">
                <ion-icon :icon="timeOutline" class="mr-2 text-tertiary" />
                {{ t("simverse.detail.viewTimeline") }}
              </button>
              <button type="button" class="ui-button ui-button--ghost justify-start" @click="goSub('relations')">
                <ion-icon :icon="gitNetworkOutline" class="mr-2 text-success" />
                {{ t("simverse.detail.viewRelations") }}
              </button>
              <button type="button" class="ui-button ui-button--ghost justify-start" @click="goSub('behavior')">
                <ion-icon :icon="pulseOutline" class="mr-2 text-warning" />
                {{ t("simverse.detail.viewBehavior") }}
              </button>
            </div>
          </div>
        </div>

        <!-- 核心属性 -->
        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.stats") }}</div>
            <div class="space-y-2">
              <div class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.health") }}</span>
                <span class="text-sm font-mono font-medium">{{ Math.round(npc.health) }}/{{ npc.max_health }}</span>
              </div>
              <div class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.energy") }}</span>
                <span class="text-sm font-mono font-medium">{{ Math.round(npc.energy) }}/{{ npc.max_energy }}</span>
              </div>
              <div v-if="npc.mana !== undefined" class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.mana") }}</span>
                <span class="text-sm font-mono font-medium">{{ Math.round(npc.mana) }}/{{ npc.max_mana }}</span>
              </div>
              <div class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.mood") }}</span>
                <span class="text-sm font-medium">{{ npc.mood ?? '—' }}</span>
              </div>
              <div class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.satisfaction") }}</span>
                <span class="text-sm font-medium">{{ npc.satisfaction ?? '—' }}</span>
              </div>
              <div class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.experience") }}</span>
                <span class="text-sm font-mono font-medium">{{ npc.experience ?? '—' }}</span>
              </div>
              <div class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.wealthTier") }}</span>
                <span class="text-sm font-medium">{{ npc.wealth_tier }}</span>
              </div>
              <div class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.socialTier") }}</span>
                <span class="text-sm font-medium">{{ npc.social_tier }}</span>
              </div>
              <div class="flex items-center justify-between py-2 border-b border-base-200">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.children") }}</span>
                <span class="text-sm font-mono font-medium">{{ npc.num_children ?? '—' }}</span>
              </div>
              <div class="flex items-center justify-between py-2">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.marriages") }}</span>
                <span class="text-sm font-mono font-medium">{{ npc.num_marriages ?? '—' }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 归属 -->
        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.detail.personality") }}</div>
            <div class="space-y-2">
              <button
                v-if="npc.org_id"
                type="button"
                class="ui-button ui-button--ghost w-full justify-between"
                @click="goOrg(npc.org_id)"
              >
                <span>{{ t("simverse.detail.orgId") }}</span>
                <span class="text-base-content/70 font-mono">#{{ npc.org_id }}</span>
              </button>
              <button
                v-if="npc.region_id"
                type="button"
                class="ui-button ui-button--ghost w-full justify-between"
                @click="goRegion(npc.region_id)"
              >
                <span>{{ t("simverse.detail.regionId") }}</span>
                <span class="text-base-content/70 font-mono">#{{ npc.region_id }}</span>
              </button>
              <div v-if="npc.home_region_id" class="flex items-center justify-between py-2">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.homeRegion") }}</span>
                <span class="text-sm font-mono text-base-content/70">#{{ npc.home_region_id }}</span>
              </div>
              <div v-if="npc.born_at" class="flex items-center justify-between py-2">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.bornAt") }}</span>
                <span class="text-sm font-mono text-base-content/70">tick {{ npc.born_at }}</span>
              </div>
              <div v-if="npc.died_at" class="flex items-center justify-between py-2">
                <span class="text-sm text-base-content/80">{{ t("simverse.detail.diedAt") }}</span>
                <span class="text-sm font-mono text-base-content/70">tick {{ npc.died_at }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 技能 -->
        <div v-if="skillEntries.length" class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.skills") }}</div>
            <div class="space-y-2">
              <div
                v-for="s in skillEntries"
                :key="s.key"
                class="flex items-center justify-between py-2 border-b border-base-200 last:border-b-0 last:pb-0"
              >
                <span class="text-sm text-base-content/80">{{ s.key }}</span>
                <span class="text-sm font-mono font-medium">{{ s.value }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 流派 / Build (P14) -->
        <div v-if="build" class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.build") }}</div>
            <div class="space-y-3">
              <div class="flex items-center gap-3 flex-wrap">
                <span class="ui-chip !text-sm !font-semibold" :class="archChipClass(build.primary)">
                  {{ archLabel(build.primary) }}
                </span>
                <span class="build-stars flex items-center gap-1" :aria-label="t('simverse.build.synergy')">
                  <ion-icon
                    v-for="i in 5"
                    :key="i"
                    :icon="i <= build.synergy ? star : starOutline"
                    :class="i <= build.synergy ? 'text-warning' : 'text-base-content/30'"
                  />
                </span>
              </div>
              <div v-if="build.tags.length > 1" class="flex flex-wrap gap-2">
                <span
                  v-for="tag in build.tags.slice(1)"
                  :key="tag"
                  class="ui-chip ui-chip--mono !text-xs"
                >
                  {{ archLabel(tag) }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- 大五人格 -->
        <div v-if="bigFiveEntries.length" class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.detail.bigFive") }}</div>
            <div class="space-y-2">
              <div
                v-for="b in bigFiveEntries"
                :key="b.key"
                class="flex items-center justify-between py-2 border-b border-base-200 last:border-b-0 last:pb-0"
              >
                <span class="text-sm text-base-content/80">{{ b.key }}</span>
                <span class="text-sm font-mono font-medium">{{ b.value }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 价值观 / 兴趣 -->
        <div v-if="npc.top_values?.length || npc.top_interests?.length" class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.detail.values") }} / {{ t("simverse.detail.interests") }}</div>
            <div class="space-y-3">
              <div v-if="npc.top_values?.length" class="flex flex-wrap gap-2">
                <span
                  v-for="v in npc.top_values"
                  :key="v"
                  class="ui-chip !text-xs !bg-tertiary/15 !text-tertiary !border-tertiary/30"
                >
                  {{ v }}
                </span>
              </div>
              <div v-if="npc.top_interests?.length" class="flex flex-wrap gap-2">
                <span
                  v-for="i in npc.top_interests"
                  :key="i"
                  class="ui-chip !text-xs !bg-success/15 !text-success !border-success/30"
                >
                  {{ i }}
                </span>
              </div>
            </div>
          </div>
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
import {
  alertCircleOutline,
  bagOutline,
  gitNetworkOutline,
  pulseOutline,
  refreshOutline,
  star,
  starOutline,
  timeOutline,
  heartOutline,
  skullOutline,
} from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { type SimverseNPCDetail, useSimverse } from "@/composables/useSimverse";
import { type ArchetypeKey, deriveNPCBuild } from "@/game/builds";

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

const skillEntries = computed(() => (npc.value ? Object.entries(npc.value.skills || {}).map(([key, value]) => ({ key, value })) : []));
const bigFiveEntries = computed(() => (npc.value ? Object.entries(npc.value.big_five || {}).map(([key, value]) => ({ key, value })) : []));

const build = computed(() => (npc.value ? deriveNPCBuild(npc.value) : null));

const ARCH_COLOR: Record<ArchetypeKey, string> = {
  warrior: "danger",
  guardian: "warning",
  scholar: "primary",
  merchant: "success",
  artisan: "tertiary",
  healer: "success",
  leader: "secondary",
  hermit: "medium",
  rogue: "dark",
  artist: "tertiary",
};

function archLabel(key: ArchetypeKey): string {
  return t(`simverse.build.${key}`);
}

function archChipClass(key: ArchetypeKey): string {
  const color = ARCH_COLOR[key] || "medium";
  const map: Record<string, string> = {
    danger: "!bg-error/15 !text-error !border-error/30",
    warning: "!bg-warning/15 !text-warning !border-warning/30",
    primary: "!bg-primary/15 !text-primary !border-primary/30",
    success: "!bg-success/15 !text-success !border-success/30",
    tertiary: "!bg-tertiary/15 !text-tertiary !border-tertiary/30",
    secondary: "!bg-secondary/15 !text-secondary !border-secondary/30",
    medium: "!bg-base-content/15 !text-base-content/70 !border-base-content/20",
    dark: "!bg-base-content/15 !text-base-content/70 !border-base-content/20",
  };
  return map[color] || map.medium;
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
  return map[p?.toLowerCase()] || "!bg-base-content/15 !text-base-content/70 !border-base-content/20";
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

<style scoped lang="scss">
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.state-container p {
  color: var(--color-error);
  margin: 0;
}
.build-stars {
  font-size: 14px;
}
</style>
