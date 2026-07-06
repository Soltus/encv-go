<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button :text="t('common.back')" />
        </ion-buttons>
        <ion-title>🌍 世界</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content class="world-content" :fullscreen="true">
      <div ref="worldContainer" class="world-container">
        <div class="world-placeholder">
          <div class="placeholder-icon">🎮</div>
          <p>SimVerse 世界视图</p>
          <p class="placeholder-hint">（物理引擎渲染区域）</p>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();
const worldContainer = ref<HTMLElement | null>(null);
</script>

<style scoped>
.world-content {
  --background: #1a1a2e;
}

.world-container {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: radial-gradient(circle at center, #16213e 0%, #0f0f23 100%);
}

.world-placeholder {
  text-align: center;
  color: rgba(255, 255, 255, 0.6);
}

.placeholder-icon {
  font-size: 64px;
  margin-bottom: 16px;
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { transform: scale(1); opacity: 0.8; }
  50% { transform: scale(1.1); opacity: 1; }
}

.world-placeholder p {
  margin: 4px 0;
  font-size: 14px;
}

.placeholder-hint {
  font-size: 12px;
  opacity: 0.6;
}
</style>
