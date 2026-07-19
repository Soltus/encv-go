<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.behavior") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4 space-y-4">
        <div v-if="loading" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>

        <div v-else-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
        </div>

        <template v-else-if="stats">
          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.configDetails") }}</div>
              <div class="space-y-1">
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.population") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ stats.total_npcs }}</span>
                </div>
                <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                  <span class="text-sm font-medium">{{ t("simverse.alive") }}</span>
                  <span class="text-xs text-base-content/70 font-mono">{{ stats.alive_npcs }}</span>
                </div>
              </div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.behavior") }}</div>
              <div class="space-y-2">
                <div
                  v-for="b in behaviorDist"
                  :key="b.key"
                  class="p-3 rounded-lg hover:bg-base-200 transition-colors"
                >
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-sm font-medium">{{ b.label }}</span>
                    <span class="text-xs text-base-content/70 font-mono">{{ b.value }} · {{ b.pct }}%</span>
                  </div>
                  <div class="h-1.5 bg-base-300 rounded-full overflow-hidden">
                    <div
                      class="h-full bg-primary rounded-full transition-all"
                      :style="{ width: b.pct + '%' }"
                    ></div>
                  </div>
                </div>
                <div v-if="!behaviorDist.length" class="p-4 text-center text-sm text-base-content/50">
                  {{ t("simverse.noData") }}
                </div>
              </div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">
                <span>{{ t("simverse.npcs") }}</span>
                <span class="text-xs text-base-content/70 font-normal">{{ behaviorList.total }}</span>
              </div>
              <div class="space-y-1">
                <div
                  v-for="npc in behaviorList.items"
                  :key="npc.npc_id"
                  class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  @click="goNpc(npc.npc_id)"
                >
                  <div class="ui-bubble flex-shrink-0">
                    <span class="text-lg">{{ avatarEmoji(npc.npc_id) }}</span>
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium">{{ npc.npc_name }}</div>
                    <div class="flex items-center gap-2 mt-0.5">
                      <span class="ui-chip !text-xs !py-0.5" :class="behaviorChipClass(npc.current_behavior)">
                        {{ npc.current_behavior_cn || npc.current_behavior }}
                      </span>
                      <span class="text-xs text-base-content/70">Lv.{{ npc.level }} · {{ npc.profession }}</span>
                    </div>
                    <div class="flex items-center gap-3 mt-1">
                      <span class="text-xs text-base-content/70">😊 {{ npc.mood }}</span>
                      <span class="text-xs text-base-content/70">⚡ {{ npc.energy }}</span>
                    </div>
                  </div>
                  <ion-icon :icon="chevronForward" class="text-base-content/40" />
                </div>
                <div v-if="!behaviorList.items.length" class="p-4 text-center text-sm text-base-content/50">
                  {{ t("simverse.noData") }}
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
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
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, chevronForward, refreshOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseBehaviorState, type SimverseBehaviorStats, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadBehaviorStats, loadBehaviorList } = useSimverse();

const loading = ref(false);
const error = ref("");
const stats = ref<SimverseBehaviorStats | null>(null);
const behaviorList = ref<{ total: number; items: SimverseBehaviorState[] }>({ total: 0, items: [] });

const behaviorLabels: Record<string, string> = {
  idle: "闲置",
  work: "工作",
  rest: "休息",
  eat: "进食",
  sleep: "睡眠",
  socialize: "社交",
  explore: "探索",
  trade: "交易",
};

const behaviorDist = computed(() => {
  if (!stats.value) return [];
  const dist = stats.value.behavior_dist || {};
  const total = stats.value.total_npcs || 1;
  return Object.entries(dist)
    .map(([key, value]) => ({
      key,
      label: behaviorLabels[key] || key,
      value,
      pct: Math.round((value / total) * 100),
    }))
    .sort((a, b) => b.value - a.value);
});

function behaviorChipClass(behavior: string): string {
  const map: Record<string, string> = {
    work: "!bg-primary/15 !text-primary",
    rest: "!bg-base-300 !text-base-content/70",
    eat: "!bg-warning/15 !text-warning",
    sleep: "!bg-tertiary/15 !text-tertiary",
    socialize: "!bg-success/15 !text-success",
    explore: "!bg-secondary/15 !text-secondary",
    trade: "!bg-error/15 !text-error",
  };
  return map[behavior?.toLowerCase()] || "!bg-base-300 !text-base-content/70";
}

function avatarEmoji(id: number): string {
  const avatars = ["🧙", "⚔️", "🛡️", "🧑‍🌾", "👨‍🍳", "🗡️", "🏹", "📜", "💎", "🪓"];
  return avatars[id % avatars.length];
}

function goNpc(id: number) {
  router.push(`/world/npc/${id}`);
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const [s, list] = await Promise.all([loadBehaviorStats(), loadBehaviorList(1, 50)]);
    stats.value = s;
    if (list) behaviorList.value = { total: list.total, items: list.items };
    if (!s && !list) error.value = t("simverse.noData");
  } catch (e: any) {
    error.value = e.message || "Failed to load behavior data";
  } finally {
    loading.value = false;
  }
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

  p {
    color: var(--color-error);
    margin: 0;
  }
}
</style>
