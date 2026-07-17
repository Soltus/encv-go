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

    <ion-content class="ion-padding">
      <div v-if="loading" class="state-box">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>
      <div v-else-if="error" class="state-box">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>
      <template v-else-if="summary">
        <ion-card>
          <ion-card-content>
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
          </ion-card-content>
        </ion-card>

        <ion-list :inset="true">
          <ion-item-group v-for="grp in grouped" :key="grp.key">
            <ion-item-divider>
              <ion-label>{{ grp.label }}</ion-label>
            </ion-item-divider>
            <ion-card v-for="q in grp.quests" :key="q.id" class="quest-card">
              <ion-card-content>
                <div class="quest-head">
                  <span class="quest-icon">{{ q.icon }}</span>
                  <div class="quest-title">
                    <h3>{{ q.title }}</h3>
                    <p>{{ q.desc }}</p>
                  </div>
                </div>
                <ion-progress-bar :value="progressRatio(q)" color="primary" />
                <div class="quest-meta">
                  <span>{{ t("simverse.questProgress") }} {{ q.progress }} / {{ q.goal }}</span>
                </div>
                <div class="quest-reward">
                  <ion-badge color="tertiary">{{ q.reward.icon }} {{ q.reward.diamond }} {{ t("simverse.questDiamond") }}</ion-badge>
                  <ion-badge color="warning">{{ q.reward.gold }} {{ t("simverse.questGold") }}</ion-badge>
                  <ion-badge color="success">{{ q.reward.exp }} {{ t("simverse.questExp") }}</ion-badge>
                </div>
                <ion-button
                  v-if="q.status === 'active'"
                  expand="block"
                  size="small"
                  class="claim-btn"
                  :disabled="q.progress < q.goal || claiming"
                  @click="claim(q)"
                >
                  {{ q.progress >= q.goal ? t("simverse.questClaim") : t("simverse.questActiveCount") }}
                </ion-button>
                <ion-badge v-else-if="q.status === 'claimed'" color="medium" class="status-badge">
                  {{ t("simverse.questClaimed") }}
                </ion-badge>
                <ion-badge v-else color="light" class="status-badge">{{ q.status }}</ion-badge>
              </ion-card-content>
            </ion-card>
          </ion-item-group>
        </ion-list>
      </template>
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
  IonCard,
  IonCardContent,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonItemDivider,
  IonItemGroup,
  IonLabel,
  IonList,
  IonPage,
  IonProgressBar,
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

.quest-card {
  margin: 8px 0;
}

.quest-head {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.quest-icon {
  font-size: 24px;
}

.quest-title {
  h3 {
    margin: 0;
    font-size: 15px;
  }

  p {
    margin: 2px 0 0;
    font-size: 12px;
    color: var(--color-base-content);
    opacity: 0.7;
  }
}

.quest-meta {
  font-size: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
  margin: 8px 0 4px;
}

.quest-reward {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 8px;
}

.claim-btn {
  margin-top: 10px;
}

.status-badge {
  display: block;
  margin-top: 10px;
  text-align: center;
}
</style>
