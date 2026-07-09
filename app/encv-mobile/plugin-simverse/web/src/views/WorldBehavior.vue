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
      <div v-if="loading" class="state-container">
        <ion-spinner name="crescent" />
        <p>{{ t("settings.loading") }}</p>
      </div>

      <div v-else-if="error" class="state-container">
        <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
        <p>{{ error }}</p>
        <ion-button @click="reload">{{ t("settings.check") }}</ion-button>
      </div>

      <template v-else-if="stats">
        <!-- 概览 -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.configDetails") }}</ion-label></ion-list-header>
          <ion-item>
            <ion-label>{{ t("simverse.population") }}</ion-label>
            <ion-note slot="end">{{ stats.total_npcs }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t("simverse.alive") }}</ion-label>
            <ion-note slot="end">{{ stats.alive_npcs }}</ion-note>
          </ion-item>
        </ion-list>

        <!-- 行为分布 -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.behavior") }}</ion-label></ion-list-header>
          <ion-item v-for="b in behaviorDist" :key="b.key">
            <ion-label class="dist-label">
              <span>{{ b.label }}</span>
              <div class="dist-bar">
                <div class="dist-fill" :style="{ width: b.pct + '%' }" />
              </div>
            </ion-label>
            <ion-note slot="end">{{ b.value }} · {{ b.pct }}%</ion-note>
          </ion-item>
          <ion-item v-if="!behaviorDist.length" class="empty-item">
            <ion-label class="ion-text-center">{{ t("simverse.noData") }}</ion-label>
          </ion-item>
        </ion-list>

        <!-- NPC 行为列表 -->
        <ion-list :inset="true">
          <ion-list-header>
            <ion-label>{{ t("simverse.npcs") }}</ion-label>
            <ion-note slot="end">{{ behaviorList.total }}</ion-note>
          </ion-list-header>
          <ion-item
            v-for="npc in behaviorList.items"
            :key="npc.npc_id"
            button
            detail
            @click="goNpc(npc.npc_id)"
          >
            <div slot="start" class="npc-avatar">{{ avatarEmoji(npc.npc_id) }}</div>
            <ion-label class="ion-text-wrap">
              <h3>{{ npc.npc_name }}</h3>
              <p>
                <ion-badge :color="behaviorColor(npc.current_behavior)" size="small">
                  {{ npc.current_behavior_cn || npc.current_behavior }}
                </ion-badge>
                <span class="npc-meta">Lv.{{ npc.level }} · {{ npc.profession }}</span>
              </p>
              <p class="npc-sub">
                <span>😊 {{ npc.mood }}</span>
                <span>⚡ {{ npc.energy }}</span>
              </p>
            </ion-label>
          </ion-item>
          <ion-item v-if="!behaviorList.items.length" class="empty-item">
            <ion-label class="ion-text-center">{{ t("simverse.noData") }}</ion-label>
          </ion-item>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonList, IonListHeader, IonLabel,
  IonItem, IonNote, IonBadge, IonSpinner,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseBehaviorStats, type SimverseBehaviorState } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadBehaviorStats, loadBehaviorList } = useSimverse();

const loading = ref(false);
const error = ref("");
const stats = ref<SimverseBehaviorStats | null>(null);
const behaviorList = ref<{ total: number; items: SimverseBehaviorState[] }>({ total: 0, items: [] });

const behaviorLabels: Record<string, string> = {
  idle: "闲置", work: "工作", rest: "休息", eat: "进食",
  sleep: "睡眠", socialize: "社交", explore: "探索", trade: "交易",
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

function behaviorColor(behavior: string): string {
  const map: Record<string, string> = {
    work: "primary", rest: "medium", eat: "warning",
    sleep: "tertiary", socialize: "success", explore: "secondary", trade: "danger",
  };
  return map[behavior?.toLowerCase()] || "medium";
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
    const [s, list] = await Promise.all([
      loadBehaviorStats(),
      loadBehaviorList(1, 50),
    ]);
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

<style scoped>
.state-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
.state-container p { color: var(--ion-color-danger); margin: 0; }
.dist-label {
  width: 100%;
}
.dist-bar {
  height: 6px;
  border-radius: 3px;
  background: var(--ion-color-light, #e5e7eb);
  overflow: hidden;
  margin-top: 6px;
}
.dist-fill {
  height: 100%;
  background: var(--ion-color-primary);
  transition: width 0.3s ease;
}
.npc-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--ion-color-light, #f3f4f6);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}
.npc-meta {
  font-size: 12px;
  margin-left: 8px;
  color: var(--ion-color-medium);
}
.npc-sub {
  font-size: 12px;
  display: flex;
  gap: 12px;
  color: var(--ion-color-medium);
}
.empty-item {
  --padding-start: 0;
  --inner-padding-end: 0;
  justify-content: center;
  color: var(--ion-color-medium);
  padding: 24px 0;
}
</style>
