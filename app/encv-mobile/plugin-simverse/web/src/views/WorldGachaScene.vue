<template>
  <div ref="gachaModalRef" class="gacha-modal-overlay" @click.self="$emit('close')">
    <div class="gacha-modal-content">
      <button class="gacha-modal-close" @click="$emit('close')">✕</button>
      <div class="gacha-banner-large">
        <div class="banner-bg"></div>
        <div class="banner-content">
          <div class="banner-icon-large">🎴</div>
          <div class="banner-title-large">{{ t("simverse.gachaTitle") }}</div>
          <div class="banner-desc-large">{{ t("simverse.gachaDesc") }}</div>
        </div>
        <div class="sparkle-layer">
          <span v-for="i in 12" :key="i" class="sparkle" :style="getSparkleStyle(i)">✦</span>
        </div>
      </div>

      <div class="gacha-pool-info">
        <div class="pool-rate">
          <span class="rate-label">SSR</span>
          <span class="rate-val">1%</span>
        </div>
        <div class="pool-rate">
          <span class="rate-label">SR</span>
          <span class="rate-val">8%</span>
        </div>
        <div class="pool-rate">
          <span class="rate-label">R</span>
          <span class="rate-val">30%</span>
        </div>
        <div class="pool-rate">
          <span class="rate-label">N</span>
          <span class="rate-val">61%</span>
        </div>
      </div>

      <div class="gacha-actions-large">
        <button class="gacha-big-btn single" :disabled="isGachaAnimating" @click="$emit('gacha', 1)">
          <span class="btn-icon-large">🎴</span>
          <span class="btn-name">{{ t("simverse.singlePull") }}</span>
          <span class="btn-cost">100 💎</span>
        </button>
        <button class="gacha-big-btn ten" :disabled="isGachaAnimating" @click="$emit('gacha', 10)">
          <span class="btn-icon-large">🎴×10</span>
          <span class="btn-name">{{ t("simverse.tenPull") }}</span>
          <span class="btn-cost">900 💎</span>
          <span class="btn-badge">{{ t("simverse.guaranteedRare") }}</span>
        </button>
      </div>

      <div v-if="gachaHistory.length > 0" class="gacha-history">
        <div class="history-title">最近召唤</div>
        <div class="history-list">
          <div v-for="(item, i) in gachaHistory.slice(0, 6)" :key="i"
               class="history-item"
               :class="item.rarity">
            <span class="hist-icon">{{ item.icon }}</span>
            <span class="hist-name">{{ item.name }}</span>
            <span class="hist-rarity">{{ item.rarity }}</span>
          </div>
        </div>
      </div>

      <div v-if="isGachaAnimating" ref="gachaFlashRef" class="gacha-animation-overlay">
        <div class="gacha-cards-container" :class="{ reveal: gachaRevealed }">
          <div v-for="(item, i) in gachaAnimResults" :key="i"
               class="gacha-card-anim"
               :class="[item.rarity, { revealed: gachaRevealed }]"
               :style="{ animationDelay: (i * 0.1) + 's' }">
            <div class="card-inner">
              <div class="card-front">
                <span class="card-back-icon">✦</span>
              </div>
              <div class="card-back">
                <span class="gacha-item-icon">{{ item.icon }}</span>
                <span class="gacha-item-name">{{ item.name }}</span>
                <span class="gacha-item-rarity" :class="item.rarity">{{ item.rarity }}</span>
              </div>
            </div>
          </div>
        </div>
        <div v-if="gachaRevealed" class="gacha-skip-btn" @click="$emit('finish-gacha')">
          点击继续
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { ref } from "vue";

defineProps<{
  isGachaAnimating: boolean;
  gachaRevealed: boolean;
  gachaAnimResults: { name: string; icon: string; rarity: string }[];
  gachaHistory: { name: string; icon: string; rarity: string }[];
}>();

defineEmits<{
  (e: "close"): void;
  (e: "gacha", count: number): void;
  (e: "finish-gacha"): void;
}>();

const { t } = useI18n();

const gachaModalRef = ref<HTMLElement | null>(null);
const gachaFlashRef = ref<HTMLElement | null>(null);

function getSparkleStyle(index: number) {
  const angle = (index / 12) * 360;
  const delay = index * 0.1;
  return {
    transform: `rotate(${angle}deg) translateY(-60px)`,
    animationDelay: `${delay}s`,
  };
}
</script>

<style scoped lang="scss">
@use './simverse-world/gacha';
@use './simverse-world/common';
</style>
