<template>
  <div class="character-page">
    <div class="event-page-header">
      <button
        class="btn btn-sm bg-primary/[0.18] border border-primary/30 text-white rounded-xl px-3.5 py-2 text-sm hover:bg-primary/[0.28] active:scale-95 transition-all duration-150"
        @click="$emit('close')"
      >← {{ t("simverse.back") }}</button>
      <span class="event-page-title">🧑 {{ t("simverse.character") }}</span>
    </div>
    <div class="character-body">
      <div class="stub-banner">⚠️ {{ t("simverse.bStubHint") }}</div>
      <div class="char-card">
        <div class="char-avatar">⚔️</div>
        <div class="char-name">冒险者 · Lv.{{ playerLevel }}</div>
        <div class="char-res">
          <span>💎 {{ playerDiamond }}</span>
          <span>🪙 {{ playerGold }}</span>
          <span>⚡ {{ playerStamina }}/120</span>
        </div>
        <div class="char-skills">
          <span v-for="s in playerSkills.slice(0, 4)" :key="s.id" class="char-skill">{{ s.icon }} {{ s.name }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";

defineProps<{
  playerLevel: number;
  playerDiamond: number;
  playerGold: number;
  playerStamina: number;
  playerSkills: { id: number; name: string; icon: string; level: number; rarity: string }[];
}>();

defineEmits<{
  (e: "close"): void;
}>();

const { t } = useI18n();
</script>

<style scoped lang="scss">
@use './simverse-world/character';
@use './simverse-world/intervene';
@use './simverse-world/common';
</style>
