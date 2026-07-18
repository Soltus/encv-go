<template>
  <div class="event-page">
    <div class="event-page-header">
      <button
        class="btn btn-sm bg-primary/[0.18] border border-primary/30 text-white rounded-xl px-3.5 py-2 text-sm hover:bg-primary/[0.28] active:scale-95 transition-all duration-150"
        @click="$emit('close')"
      >← {{ t("simverse.back") }}</button>
      <span class="event-page-title">📜 {{ t("simverse.chronicles") }}</span>
    </div>
    <div class="event-feed">
      <div v-for="ev in recentEvents" :key="ev.id" class="event-row" :class="'imp-' + ev.importance">
        <div class="event-row-tick">Tick {{ ev.tick }}</div>
        <div class="event-row-title">{{ ev.type_cn }}</div>
        <div class="event-row-meta">{{ ev.imp_cn }} · {{ ev.level_cn }}</div>
      </div>
      <div v-if="recentEvents.length === 0" class="empty-state">{{ t("simverse.noData") }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { type SimverseChronicleEvent } from "@/composables/useSimverse";

defineProps<{
  recentEvents: SimverseChronicleEvent[];
}>();

defineEmits<{
  (e: "close"): void;
}>();

const { t } = useI18n();
</script>

<style scoped lang="scss">
@use './simverse-world/event';
@use './simverse-world/common';
</style>
