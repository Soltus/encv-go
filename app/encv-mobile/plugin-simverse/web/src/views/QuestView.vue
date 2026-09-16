<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.quests") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
      <ion-toolbar>
        <ion-segment :value="filter" @ionChange="onFilter">
          <ion-segment-button value="all">
            <ion-label>{{ t("simverse.quests") }}</ion-label>
          </ion-segment-button>
          <ion-segment-button value="daily">
            <ion-label>{{ t("simverse.questDaily") }}</ion-label>
          </ion-segment-button>
          <ion-segment-button value="achieve">
            <ion-label>{{ t("simverse.questAchieve") }}</ion-label>
          </ion-segment-button>
        </ion-segment>
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
          <button type="button" class="ui-button" @click="() => reload()">{{ t("settings.check") }}</button>
        </div>
        <template v-else-if="summary">
          <div class="ui-card">
            <div class="p-4">
              <div class="stats stats-vertical sm:stats-horizontal shadow w-full">
                <div class="stat">
                  <div class="stat-value">{{ summary.active_count }}</div>
                  <div class="stat-title">{{ t("simverse.questActiveCount") }}</div>
                </div>
                <div class="stat">
                  <div class="stat-value">{{ summary.completable }}</div>
                  <div class="stat-title">{{ t("simverse.questCompletable") }}</div>
                </div>
                <div class="stat">
                  <div class="stat-value">{{ summary.claimed_count }}</div>
                  <div class="stat-title">{{ t("simverse.questClaimed") }}</div>
                </div>
              </div>
            </div>
          </div>

          <div class="space-y-4">
            <div v-for="grp in grouped" :key="grp.key" class="space-y-2">
              <div class="ui-header">{{ grp.label }}</div>
              <div class="space-y-2">
                <div v-for="q in grp.quests" :key="q.id" class="ui-card">
                  <div class="p-4 space-y-3">
                    <div class="quest-head">
                      <span class="quest-icon">{{ q.icon }}</span>
                      <div class="quest-title">
                        <h3 class="text-base font-semibold m-0">{{ q.title }}</h3>
                        <p class="text-sm text-base-content/70 m-0">{{ q.desc }}</p>
                      </div>
                    </div>
                    <div class="progress-bar">
                      <div class="progress-fill" :style="{ width: (progressRatio(q) * 100) + '%' }"></div>
                    </div>
                    <div class="quest-meta text-xs text-base-content/70">
                      {{ t("simverse.questProgress") }} {{ q.progress }} / {{ q.goal }}
                    </div>
                    <div class="quest-reward flex gap-2 flex-wrap">
                      <span class="ui-chip !text-xs !py-0.5 !bg-tertiary/15 !text-tertiary !border-tertiary/30">
                        {{ q.reward.icon }} {{ q.reward.diamond }} {{ t("simverse.questDiamond") }}
                      </span>
                      <span class="ui-chip !text-xs !py-0.5 !bg-warning/15 !text-warning !border-warning/30">
                        {{ q.reward.gold }} {{ t("simverse.questGold") }}
                      </span>
                      <span class="ui-chip !text-xs !py-0.5 !bg-success/15 !text-success !border-success/30">
                        {{ q.reward.exp }} {{ t("simverse.questExp") }}
                      </span>
                    </div>
                    <button
                      v-if="q.status === 'active'"
                      type="button"
                      class="ui-button w-full"
                      :disabled="q.progress < q.goal || claiming"
                      @click="claim(q)"
                    >
                      {{ q.progress >= q.goal ? t("simverse.questClaim") : t("simverse.questActiveCount") }}
                    </button>
                    <span
                      v-else-if="q.status === 'claimed'"
                      class="block text-center text-sm text-base-content/50 py-2"
                    >
                      {{ t("simverse.questClaimed") }}
                    </span>
                    <span
                      v-else
                      class="block text-center text-sm text-base-content/50 py-2"
                    >
                      {{ q.status }}
                    </span>
                  </div>
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
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonSegment,
  IonSegmentButton,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { type SimverseQuest, type SimverseQuestSummary, type SimverseQuestType, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const { loadQuestSummary, claimQuest } = useSimverse();

const loading = ref(false);
const error = ref("");
const claiming = ref(false);
const filter = ref<"all" | "daily" | "achieve">("all");
const summary = ref<SimverseQuestSummary | null>(null);

const TYPE_LABELS: Record<SimverseQuestType, string> = {
  daily: t("simverse.questDaily"),
  achieve: t("simverse.questAchieve"),
  story: t("simverse.questStory"),
  economy: t("simverse.questEconomy"),
};

const grouped = computed(() => {
  if (!summary.value) return [];
  const quests = summary.value.quests.slice().sort((a, b) => a.sort_order - b.sort_order);
  const types: SimverseQuestType[] = filter.value === "all" ? ["daily", "achieve", "story", "economy"] : [filter.value];
  return types.map(key => ({ key, label: TYPE_LABELS[key], quests: quests.filter(q => q.type === key) })).filter(g => g.quests.length > 0);
});

function progressRatio(q: SimverseQuest): number {
  return q.goal > 0 ? Math.min(1, q.progress / q.goal) : 0;
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    summary.value = await loadQuestSummary();
  } catch (e: any) {
    error.value = e.message || "Failed to load quests";
  } finally {
    loading.value = false;
  }
}

async function claim(q: SimverseQuest) {
  claiming.value = true;
  try {
    const res = await claimQuest(q.id);
    if (res?.success) {
      q.status = "claimed";
      await reload();
    }
  } finally {
    claiming.value = false;
  }
}

function onFilter(ev: any) {
  filter.value = ev.detail.value;
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

.quest-head {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.quest-icon {
  font-size: 24px;
}

.progress-bar {
  width: 100%;
  height: 6px;
  background: var(--color-base-200);
  border-radius: 3px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: var(--color-primary);
  border-radius: 3px;
  transition: width 0.3s ease;
}
</style>
