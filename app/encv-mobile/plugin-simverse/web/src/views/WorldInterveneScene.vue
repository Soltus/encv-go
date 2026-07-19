<template>
  <div class="intervene-page">
    <div class="event-page-header">
      <button
        class="btn btn-sm bg-primary/[0.18] border border-primary/30 text-white rounded-xl px-3.5 py-2 text-sm hover:bg-primary/[0.28] active:scale-95 transition-all duration-150"
        @click="$emit('close')"
      >← {{ t("simverse.back") }}</button>
      <span class="event-page-title">🎛️ {{ t("simverse.intervene") }}</span>
    </div>
    <div class="intervene-body">
      <section class="card bg-white/[0.04] border border-primary/[0.18] rounded-2xl p-4">
        <div class="card-title text-[13px] font-bold text-white/85 mb-3">{{ t("simverse.timeControl") }}</div>
        <div class="ctrl-row">
          <button
            class="btn btn-sm primary bg-success/20 border border-success/[0.45] text-white rounded-xl px-4 py-2.5 text-sm hover:bg-primary/[0.28] active:scale-95 transition-all duration-150"
            @click="$emit('toggle-running')"
          >
            {{ worldState?.running ? "⏸ " + t("simverse.pause") : "▶ " + t("simverse.resume") }}
          </button>
          <button
            class="btn btn-sm bg-primary/[0.16] border border-primary/30 text-white rounded-xl px-4 py-2.5 text-sm hover:bg-primary/[0.28] active:scale-95 transition-all duration-150"
            @click="$emit('step-once')"
          >⏭ {{ t("simverse.step") }}</button>
        </div>
        <div class="ctrl-row">
          <span class="ctrl-hint">{{ t("simverse.fastForward") }}</span>
          <input class="ff-input" type="number" min="1" max="200" :value="ffSteps" @input="$emit('update:ffSteps', Number(($event.target as HTMLInputElement).value))" />
          <span class="ctrl-hint">{{ t("simverse.steps") }}</span>
          <button
            class="btn btn-sm bg-primary/[0.16] border border-primary/30 text-white rounded-xl px-4 py-2.5 text-sm hover:bg-primary/[0.28] active:scale-95 transition-all duration-150"
            @click="$emit('fast-forward')"
          >⏩</button>
        </div>
      </section>
      <section class="card bg-white/[0.04] border border-primary/[0.18] rounded-2xl p-4">
        <div class="card-title text-[13px] font-bold text-white/85 mb-3">{{ t("simverse.worldSnapshot") }}</div>
        <div class="ctrl-row">
          <button
            class="btn btn-sm bg-primary/[0.16] border border-primary/30 text-white rounded-xl px-4 py-2.5 text-sm hover:bg-primary/[0.28] active:scale-95 transition-all duration-150"
            @click="$emit('save')"
          >💾 {{ t("simverse.saveNow") }}</button>
          <button
            class="btn btn-sm bg-primary/[0.16] border border-primary/30 text-white rounded-xl px-4 py-2.5 text-sm hover:bg-primary/[0.28] active:scale-95 transition-all duration-150"
            @click="$emit('load')"
          >📂 {{ t("simverse.loadSave") }}</button>
        </div>
        <div v-if="saveMsg" class="ctrl-msg">{{ saveMsg }}</div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";

defineProps<{
  worldState: { running: boolean } | null;
  ffSteps: number;
  saveMsg: string;
}>();

defineEmits<{
  (e: "close"): void;
  (e: "toggle-running"): void;
  (e: "step-once"): void;
  (e: "update:ffSteps", value: number): void;
  (e: "fast-forward"): void;
  (e: "save"): void;
  (e: "load"): void;
}>();

const { t } = useI18n();
</script>

<style scoped lang="scss">
@use './simverse-world/intervene';
@use './simverse-world/common';
</style>
