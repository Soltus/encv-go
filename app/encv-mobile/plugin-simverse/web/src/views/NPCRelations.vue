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

        <!-- 关系网卡牌连线 (P14) -->
        <div class="rel-graph-card">
          <div class="graph-title">{{ t("simverse.relGraph") }} · {{ t("simverse.socialAffinity") }}</div>
          <svg class="rel-graph" viewBox="0 0 320 320" width="100%">
            <line
              v-for="n in graphNodes"
              :key="'l' + n.id"
              :x1="cx" :y1="cy" :x2="n.x" :y2="n.y"
              :stroke="relStroke(n.relType)"
              :stroke-width="lineWidth(n.affinity)"
              :stroke-opacity="lineOpacity(n.affinity)"
            />
            <g class="self-node">
              <circle :cx="cx" :cy="cy" r="30" fill="var(--ion-color-primary)" />
              <text :x="cx" :y="cy + 5" text-anchor="middle" fill="#fff" font-size="14" font-weight="700">{{ initial(data.name) }}</text>
              <text :x="cx" :y="cy + 48" text-anchor="middle" font-size="11" fill="var(--ion-color-medium)">{{ data.name }}</text>
            </g>
            <g
              v-for="n in graphNodes"
              :key="n.id"
              class="rel-node"
              @click="goTarget(n.id)"
            >
              <circle
                :cx="n.x" :cy="n.y" r="22"
                :fill="relStroke(n.relType)" fill-opacity="0.16"
                :stroke="relStroke(n.relType)" stroke-width="2"
              />
              <text :x="n.x" :y="n.y + 5" text-anchor="middle" font-size="13" font-weight="700" :fill="relStroke(n.relType)">{{ initial(n.name) }}</text>
              <text :x="n.x" :y="n.y + 40" text-anchor="middle" font-size="10" :fill="relStroke(n.relType)">{{ n.affinity > 0 ? "+" : "" }}{{ n.affinity }}</text>
            </g>
          </svg>
          <p class="graph-hint">{{ t("simverse.relGraphHint") }}</p>
        </div>

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
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonAvatar,
  IonBackButton,
  IonButton,
  IonButtons,
  IonChip,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, refreshOutline } from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { type SimverseRelationListResponse, useSimverse } from "@/composables/useSimverse";

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

// P14 关系网卡牌连线：SVG 坐标与连线样式
const GRAPH_MAX = 9;
const cx = 160;
const cy = 150;

const graphNodes = computed(() => {
  if (!data.value) return [];
  const rels = [...data.value.relations].sort((a, b) => Math.abs(b.affinity) - Math.abs(a.affinity)).slice(0, GRAPH_MAX);
  const N = rels.length;
  const R = 110;
  return rels.map((r, i) => {
    const ang = ((-90 + (i * 360) / Math.max(1, N)) * Math.PI) / 180;
    return {
      id: r.target_id,
      name: r.target.name,
      relType: r.rel_type,
      affinity: r.affinity,
      x: cx + R * Math.cos(ang),
      y: cy + R * Math.sin(ang),
    };
  });
});

function relStroke(type: string): string {
  return `var(--ion-color-${relColor(type)})`;
}
function initial(name: string): string {
  return String(name || "?").charAt(0);
}
function lineWidth(affinity: number): number {
  return Math.max(1, Math.min(5, Math.abs(affinity) / 10));
}
function lineOpacity(affinity: number): number {
  return 0.25 + Math.min(0.6, Math.abs(affinity) / 50);
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
.rel-graph-card {
  margin: 12px 16px;
  padding: 12px;
  border-radius: 14px;
  background: var(--ion-color-light, #f3f4f6);
}
.graph-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-color-medium);
  margin-bottom: 4px;
}
.rel-graph {
  display: block;
  width: 100%;
  max-height: 340px;
}
.rel-node {
  cursor: pointer;
}
.rel-node:active circle {
  fill-opacity: 0.32;
}
.graph-hint {
  text-align: center;
  font-size: 11px;
  color: var(--ion-color-medium);
  margin: 4px 0 0;
}
</style>
