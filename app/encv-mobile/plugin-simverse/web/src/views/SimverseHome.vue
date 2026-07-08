<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button @click="exitToMainApp">
            <ion-icon slot="icon-only" :icon="arrowBackOutline" />
          </ion-button>
        </ion-buttons>
        <ion-title>{{ t("simverse.home.title") }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content :fullscreen="true" class="home-content">
      <div class="hero-section">
        <div class="hero-icon">🌍</div>
        <h1 class="hero-title">{{ t("simverse.home.title") }}</h1>
        <p class="hero-subtitle">{{ t("simverse.home.subtitle") }}</p>
      </div>

      <div class="action-cards">
        <ion-card class="action-card enter-world" button @click="goToWorld">
          <ion-card-content>
            <div class="card-icon">🎮</div>
            <ion-card-title>{{ t("simverse.home.enterWorld") }}</ion-card-title>
            <ion-card-subtitle>进入横屏模拟世界</ion-card-subtitle>
          </ion-card-content>
        </ion-card>

        <ion-card class="action-card" button @click="goToChronicle">
          <ion-card-content>
            <div class="card-icon">📜</div>
            <ion-card-title>{{ t("simverse.home.chronicle") }}</ion-card-title>
            <ion-card-subtitle>查看世界历史事件</ion-card-subtitle>
          </ion-card-content>
        </ion-card>
      </div>

      <div class="stats-grid">
        <ion-card class="stat-card">
          <ion-card-content>
            <div class="stat-value">{{ fps }}</div>
            <div class="stat-label">FPS</div>
          </ion-card-content>
        </ion-card>
        <ion-card class="stat-card">
          <ion-card-content>
            <div class="stat-value">{{ entityCount }}</div>
            <div class="stat-label">实体</div>
          </ion-card-content>
        </ion-card>
        <ion-card class="stat-card">
          <ion-card-content>
            <div class="stat-value">{{ worldAge }}</div>
            <div class="stat-label">世界年龄</div>
          </ion-card-content>
        </ion-card>
      </div>

      <div class="exit-section">
        <ion-button expand="block" fill="outline" color="medium" @click="exitToMainApp">
          <ion-icon slot="start" :icon="exitOutline" />
          返回主应用
        </ion-button>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { closeWorld, isNativePluginMode, unlockScreenOrientation } from "@/plugins/SimVerse";
import { arrowBack, exit } from "ionicons/icons";

const { t } = useI18n();
const router = useRouter();

const fps = ref(60);
const entityCount = ref(128);
const worldAge = ref("3h 24m");

const arrowBackOutline = arrowBack;
const exitOutline = exit;

function goToWorld() {
  router.push("/world");
}

function goToChronicle() {
  router.push("/chronicle/1");
}

async function exitToMainApp() {
  try {
    if (isNativePluginMode()) {
      await unlockScreenOrientation();
      await closeWorld();
    } else {
      window.close();
    }
  } catch (e) {
    console.warn("[SimverseHome] Exit failed:", e);
  }
}
</script>

<style scoped>
.home-content {
  --background: linear-gradient(180deg, rgba(139, 92, 246, 0.08) 0%, transparent 30%);
}

.hero-section {
  text-align: center;
  padding: 40px 20px 20px;
}

.hero-icon {
  font-size: 72px;
  margin-bottom: 16px;
  animation: float 3s ease-in-out infinite;
}

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-10px); }
}

.hero-title {
  font-size: 28px;
  font-weight: 700;
  margin: 0 0 8px;
  background: linear-gradient(135deg, #8b5cf6, #06b6d4);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.hero-subtitle {
  font-size: 14px;
  color: var(--ion-color-medium);
  margin: 0;
}

.action-cards {
  padding: 0 16px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-bottom: 20px;
}

.action-card {
  margin: 0;
  transition: transform 0.2s ease;
}

.action-card:active {
  transform: scale(0.97);
}

.card-icon {
  font-size: 36px;
  margin-bottom: 8px;
}

.action-card ion-card-title {
  font-size: 16px;
  font-weight: 600;
}

.action-card ion-card-subtitle {
  font-size: 12px;
}

.stats-grid {
  padding: 0 16px;
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}

.stat-card {
  margin: 0;
  text-align: center;
}

.stat-card ion-card-content {
  padding: 16px 8px;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--ion-color-primary);
  margin-bottom: 4px;
}

.action-card.enter-world {
  border: 2px solid rgba(139, 92, 246, 0.3);
}

.exit-section {
  padding: 24px 16px 40px;
}

.stat-label {
  font-size: 11px;
  color: var(--ion-color-medium);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
</style>
