<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/npcs" />
        </ion-buttons>
        <ion-title>{{ t("simverse.npcRelations") }}</ion-title>
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

      <template v-else-if="data">
        <div class="hero">
          <h2>{{ data.name }}</h2>
          <p class="muted">#{{ data.npc_id }} · {{ t("simverse.socialTotal") }}: {{ data.count }}</p>
        </div>

        <!-- 关系计数概览 -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.socialCounts") }}</ion-label></ion-list-header>
          <div class="chip-row">
            <ion-chip
              v-for="(cnt, type) in sortedCounts"
              :key="type"
              :color="relColor(type)"
              outline
            >
              {{ t("simverse.rel." + type) }} {{ cnt }}
            </ion-chip>
          </div>
        </ion-list>

        <!-- 关系列表 -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>{{ t("simverse.npcRelations") }}</ion-label></ion-list-header>
          <ion-item
            v-for="rel in data.relations"
            :key="rel.target_id"
            button
            detail
            @click="goTarget(rel.target_id)"
          >
            <ion-avatar slot="start" class="rel-avatar">{{ rel.target.name.charAt(0) }}</ion-avatar>
            <ion-label>
              <h3>{{ rel.target.name }}</h3>
              <p>{{ t("simverse.rel." + rel.rel_type) }} · #{{ rel.target_id }}</p>
            </ion-label>
            <ion-note
              slot="end"
              :color="rel.affinity >= 0 ? 'success' : 'danger'"
            >
              {{ rel.affinity > 0 ? "+" : "" }}{{ rel.affinity }}
            </ion-note>
          </ion-item>
        </ion-list>

        <p v-if="!data.relations.length" class="empty-note">{{ t("simverse.socialNoRelations") }}</p>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonButton, IonIcon, IonContent, IonList, IonListHeader, IonLabel,
  IonItem, IonNote, IonSpinner, IonChip, IonAvatar,
} from "@ionic/vue";
import { refreshOutline, alertCircleOutline } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useSimverse, type SimverseRelationListResponse } from "@/composables/useSimverse";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const { loadNPCRelations } = useSimverse();

const loading = ref(false);
const error = ref("");
const data = ref<SimverseRelationListResponse | null>(null);

const sortedCounts = computed(() => {
  if (!data.value) return {};
  const entries = Object.entries(data.value.counts).sort((a, b) => b[1] - a[1]);
  const out: Record<string, number> = {};
  for (const [k, v] of entries) out[k] = v;
  return out;
});

function relColor(type: string): string {
  switch (type) {
    case "enemy":
    case "rival":
      return "danger";
    case "friend":
    case "lover":
    case "spouse":
      return "success";
    case "parent":
    case "child":
    case "sibling":
    case "master":
    case "apprentice":
      return "tertiary";
    default:
      return "medium";
  }
}

function goTarget(id: number) {
  router.push(`/npc/${id}/relations`);
}

async function reload() {
  const id = Number(route.params.id);
  if (!id) return;
  loading.value = true;
  error.value = "";
  try {
    data.value = await loadNPCRelations(id);
    if (!data.value) error.value = t("simverse.socialNoRelations");
  } catch (e: any) {
    error.value = e.message || "Failed to load relations";
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
  padding: 16px 20px 4px;
}
.hero h2 {
  margin: 0;
  font-size: 22px;
}
.muted {
  color: var(--ion-color-medium);
  margin: 4px 0 0;
}
.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 6px 0;
}
.rel-avatar {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--ion-color-primary);
  color: #fff;
  font-weight: 600;
  border-radius: 50%;
}
.empty-note {
  text-align: center;
  color: var(--ion-color-medium);
  padding: 24px;
}
</style>
