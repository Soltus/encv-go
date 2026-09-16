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
      <div class="p-4 space-y-4">
        <div v-if="loading" class="state-container">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>

        <div v-else-if="error" class="state-container">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
        </div>

        <template v-else-if="data">
          <!-- Hero -->
          <div class="ui-card">
            <div class="p-4">
              <h2 class="text-xl font-bold m-0 mb-1">{{ data.name }}</h2>
              <p class="text-sm text-base-content/60 m-0">
                #{{ data.npc_id }} · {{ t("simverse.socialTotal") }}: {{ data.count }}
              </p>
            </div>
          </div>

          <!-- 关系计数概览 -->
          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.socialCounts") }}</div>
              <div class="flex flex-wrap gap-2">
                <span
                  v-for="(cnt, type) in sortedCounts"
                  :key="type"
                  class="ui-chip !text-xs"
                  :class="relChipClass(type)"
                >
                  {{ t("simverse.rel." + type) }} {{ cnt }}
                </span>
              </div>
            </div>
          </div>

          <!-- 关系网卡牌连线 (P14) -->
          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.relGraph") }} · {{ t("simverse.socialAffinity") }}</div>
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
                  <circle :cx="cx" :cy="cy" r="30" fill="var(--color-primary)" />
                  <text :x="cx" :y="cy + 5" text-anchor="middle" fill="#fff" font-size="14" font-weight="700">{{ initial(data.name) }}</text>
                  <text :x="cx" :y="cy + 48" text-anchor="middle" font-size="11" fill="var(--color-base-content)">{{ data.name }}</text>
                </g>
                <g
                  v-for="n in graphNodes"
                  :key="n.id"
                  class="rel-node cursor-pointer"
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
              <p class="text-center text-xs text-base-content/60 mt-2 mb-0">{{ t("simverse.relGraphHint") }}</p>
            </div>
          </div>

          <!-- 关系列表 -->
          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.npcRelations") }}</div>
              <div v-if="data.relations.length" class="space-y-2">
                <button
                  v-for="rel in data.relations"
                  :key="rel.target_id"
                  type="button"
                  class="ui-button ui-button--ghost w-full justify-between !px-3 !py-2"
                  @click="goTarget(rel.target_id)"
                >
                  <div class="flex items-center gap-3">
                    <div class="ui-bubble w-9 h-9 flex items-center justify-center text-sm font-semibold flex-shrink-0" :class="relBubbleClass(rel.rel_type)">
                      {{ rel.target.name.charAt(0) }}
                    </div>
                    <div class="text-left">
                      <div class="text-sm font-medium text-base-content">{{ rel.target.name }}</div>
                      <div class="text-xs text-base-content/60">
                        {{ t("simverse.rel." + rel.rel_type) }} · #{{ rel.target_id }}
                      </div>
                    </div>
                  </div>
                  <span
                    class="text-sm font-mono font-medium"
                    :class="rel.affinity >= 0 ? 'text-success' : 'text-error'"
                  >
                    {{ rel.affinity > 0 ? "+" : "" }}{{ rel.affinity }}
                  </span>
                </button>
              </div>
              <div v-else class="text-center py-6 text-base-content/60 text-sm">
                {{ t("simverse.socialNoRelations") }}
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

function relChipClass(type: string): string {
  const color = relColor(type);
  const map: Record<string, string> = {
    danger: "!bg-error/15 !text-error !border-error/30",
    success: "!bg-success/15 !text-success !border-success/30",
    tertiary: "!bg-tertiary/15 !text-tertiary !border-tertiary/30",
    medium: "!bg-base-content/15 !text-base-content/70 !border-base-content/20",
  };
  return map[color] || map.medium;
}

function relBubbleClass(type: string): string {
  const color = relColor(type);
  const map: Record<string, string> = {
    danger: "!bg-error/15 !text-error",
    success: "!bg-success/15 !text-success",
    tertiary: "!bg-tertiary/15 !text-tertiary",
    medium: "!bg-base-content/15 !text-base-content/70",
  };
  return map[color] || map.medium;
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
  const ionicToDaisy: Record<string, string> = {
    danger: "--color-error",
    success: "--color-success",
    primary: "--color-primary",
    warning: "--color-warning",
    tertiary: "--color-accent",
    medium: "--color-base-content",
  };
  return `var(${ionicToDaisy[relColor(type)] || "--color-base-content"})`;
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
.rel-graph {
  display: block;
  width: 100%;
  max-height: 340px;
}
.rel-node:active circle {
  fill-opacity: 0.32;
}
</style>
