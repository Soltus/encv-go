<template>
  <div class="detail-modal" @click.self="$emit('close')">
    <div class="detail-card">
      <div class="detail-header">
        <div class="detail-avatar" :style="focusBuild ? { background: `linear-gradient(135deg, ${ARCH_META[focusBuild.primary].colorCss}, var(--color-secondary))` } : {}">
          {{ selectedNpc.name?.[0] }}
        </div>
        <div class="detail-info">
          <div class="detail-name">{{ selectedNpc.name }}</div>
          <div class="detail-meta">
            {{ selectedNpc.species }} · {{ selectedNpc.gender }} · {{ selectedNpc.age }}{{ t("simverse.yearsOld") }}
          </div>
          <div v-if="focusBuild" class="detail-build">
            <span class="build-chip" :style="{ background: ARCH_META[focusBuild.primary].colorCss }">
              {{ ARCH_META[focusBuild.primary].emoji }} {{ ARCH_META[focusBuild.primary].name }}
            </span>
            <span class="build-synergy">★{{ focusBuild.synergy }}</span>
          </div>
        </div>
        <button class="detail-close" @click="$emit('close')">✕</button>
      </div>

      <div class="focus-actions">
        <button class="focus-action-btn" :class="{ active: inSquad }"
                :disabled="!inSquad && squadIds.length >= maxSquad" @click="$emit('toggle-squad')">
          {{ inSquad ? t("simverse.focus.removeFromSquad") : t("simverse.focus.addToSquad") }}
        </button>
        <span v-if="!inSquad && squadIds.length >= maxSquad" class="focus-action-hint">
          {{ t("simverse.focus.squadFull") }}
        </span>
      </div>

      <ion-segment :value="focusTab" @ionChange="onFocusTabChange" class="focus-tabs" scrollable>
        <ion-segment-button value="identity">
          <ion-label>{{ t("simverse.focus.identity") }}</ion-label>
        </ion-segment-button>
        <ion-segment-button value="timeline">
          <ion-label>{{ t("simverse.focus.timeline") }}</ion-label>
        </ion-segment-button>
        <ion-segment-button value="relations">
          <ion-label>{{ t("simverse.focus.relations") }}</ion-label>
        </ion-segment-button>
      </ion-segment>

      <div class="detail-body">
        <div v-if="focusTab === 'identity'" class="detail-grid">
          <div class="detail-item">
            <span class="item-label">{{ t("simverse.profession") }}</span>
            <span class="item-value">{{ selectedNpc.profession }}</span>
          </div>
          <div class="detail-item">
            <span class="item-label">{{ t("simverse.level") }}</span>
            <span class="item-value highlight">Lv.{{ selectedNpc.level }}</span>
          </div>
          <div class="detail-item">
            <span class="item-label">{{ t("simverse.health") }}</span>
            <span class="item-value success">{{ selectedNpc.health }} / {{ selectedNpc.max_health }}</span>
          </div>
          <div class="detail-item">
            <span class="item-label">{{ t("simverse.energy") }}</span>
            <span class="item-value warning">{{ selectedNpc.energy }} / {{ selectedNpc.max_energy }}</span>
          </div>
          <div class="detail-item">
            <span class="item-label">{{ t("simverse.wealthTier") }}</span>
            <span class="item-value">{{ selectedNpc.wealth_tier }}</span>
          </div>
          <div class="detail-item">
            <span class="item-label">{{ t("simverse.socialTier") }}</span>
            <span class="item-value">{{ selectedNpc.social_tier }}</span>
          </div>
          <div class="detail-item">
            <span class="item-label">{{ t("simverse.lifeStage") }}</span>
            <span class="item-value">{{ selectedNpc.life_stage }}</span>
          </div>
          <div class="detail-item">
            <span class="item-label">{{ t("simverse.alive") }}</span>
            <span class="item-value" :class="{ alive: selectedNpc.is_alive, dead: !selectedNpc.is_alive }">
              {{ selectedNpc.is_alive ? t("common.on") : t("common.off") }}
            </span>
          </div>
        </div>

        <div v-else-if="focusTab === 'timeline'" class="focus-panel">
          <div v-if="focusLoading" class="empty-state">{{ t("simverse.loading") }}</div>
          <template v-else>
            <div v-for="ev in npcChronicle" :key="ev.id" class="chrono-row" :class="'imp-' + ev.importance">
              <div class="chrono-tick">Tick {{ ev.tick }}</div>
              <div class="chrono-title">{{ ev.type_cn }}</div>
              <div class="chrono-meta">{{ ev.imp_cn }} · {{ ev.level_cn }}</div>
            </div>
            <div v-if="npcChronicle.length === 0" class="empty-state">{{ t("simverse.focus.chronicleEmpty") }}</div>
          </template>
        </div>

        <div v-else-if="focusTab === 'relations'" class="focus-panel">
          <div v-if="focusLoading" class="empty-state">{{ t("simverse.loading") }}</div>
          <template v-else>
            <div v-for="rel in npcRelations" :key="rel.target_id" class="rel-row" @click="$emit('select-npc', rel.target)">
              <div class="rel-avatar">{{ rel.target?.name?.[0] }}</div>
              <div class="rel-info">
                <div class="rel-name">{{ rel.target?.name }}</div>
                <div class="rel-type">{{ t("simverse.rel." + rel.rel_type) }}</div>
              </div>
              <div class="rel-affinity">
                <span class="rel-affinity-val">{{ rel.affinity }}</span>
                <span class="rel-affinity-label">{{ t("simverse.focus.affinity") }}</span>
              </div>
            </div>
            <div v-if="npcRelations.length === 0" class="empty-state">{{ t("simverse.focus.relEmpty") }}</div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { IonLabel, IonSegment, IonSegmentButton } from "@ionic/vue";
import { computed } from "vue";
import { type SimverseChronicleEvent, type SimverseNPC, type SimverseRelation } from "@/composables/useSimverse";
import { ARCH_META, deriveBuildFromNPC } from "@/game/builds";

const props = defineProps<{
  selectedNpc: SimverseNPC;
  focusTab: "identity" | "timeline" | "relations";
  npcChronicle: SimverseChronicleEvent[];
  npcRelations: SimverseRelation[];
  focusLoading: boolean;
  squadIds: number[];
  maxSquad: number;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "toggle-squad"): void;
  (e: "focus-tab-change", tab: "identity" | "timeline" | "relations"): void;
  (e: "select-npc", npc: SimverseNPC): void;
}>();

const { t } = useI18n();

const inSquad = computed(() => props.squadIds.includes(props.selectedNpc.id));
const focusBuild = computed(() => deriveBuildFromNPC(props.selectedNpc));

function onFocusTabChange(ev: CustomEvent) {
  emit("focus-tab-change", ev.detail.value as "identity" | "timeline" | "relations");
}
</script>

<style scoped lang="scss">
@use './simverse-world/focus';
@use './simverse-world/common';
</style>
