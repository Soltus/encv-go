<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('home.title') }}</ion-title>
      </ion-toolbar>
    </ion-header>
    <ion-content class="home-content">
      <div class="home-welcome">
        <h2>ENCV</h2>
        <p>{{ t('home.subtitle') }}</p>
      </div>

      <div class="home-cards">
        <div class="home-card player-card" @click="handleOpenPlayer">
          <ion-icon :icon="playCircle" class="card-icon player-icon"></ion-icon>
          <div class="card-info">
            <h3>{{ t('home.player') }}</h3>
            <p>{{ t('home.playerDesc') }}</p>
          </div>
        </div>

        <div class="home-card" @click="handleOpenFiles">
          <ion-icon :icon="folder" class="card-icon files-icon"></ion-icon>
          <div class="card-info">
            <h3>{{ t('home.files') }}</h3>
            <p>{{ t('home.filesDesc') }}</p>
          </div>
        </div>

        <div class="home-card" @click="handleOpenTasks">
          <ion-icon :icon="lockClosed" class="card-icon tasks-icon"></ion-icon>
          <div class="card-info">
            <h3>{{ t('home.tasks') }}</h3>
            <p>{{ t('home.tasksDesc') }}</p>
          </div>
        </div>

        <div class="home-card" @click="handleOpenRemote">
          <ion-icon :icon="globe" class="card-icon remote-icon"></ion-icon>
          <div class="card-info">
            <h3>{{ t('home.remote') }}</h3>
            <p>{{ t('home.remoteDesc') }}</p>
          </div>
        </div>

        <div class="home-card extensions-card" @click="handleOpenExtensions">
          <ion-icon :icon="layersOutline" class="card-icon extensions-icon"></ion-icon>
          <div class="card-info">
            <h3>{{ t('home.extensions') }}</h3>
            <p>{{ t('home.extensionsDesc') }}</p>
          </div>
        </div>

        <div class="home-card simverse-card" @click="handleOpenSimverse">
          <ion-icon :icon="planet" class="card-icon simverse-icon"></ion-icon>
          <div class="card-info">
            <h3>SimVerse 世界</h3>
            <p>进入横屏模拟世界</p>
          </div>
        </div>
      </div>

      <!-- 浮动 AI 入口（Phase 7.6） -->
      <AgentEntry />
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@/composables/useI18n";
import { isNative } from "@/plugins/GoProcess";
import { openWorld } from "@/plugins/SimVerse";
import { onIonViewWillEnter } from "@ionic/vue";
import { useRouter } from "vue-router";

const { t } = useI18n();
const router = useRouter();

function _handleOpenPlayer() {
  router.push("/player");
}

function _handleOpenFiles() {
  router.push("/tabs/files");
}

function _handleOpenTasks() {
  router.push("/tabs/tasks");
}

function _handleOpenRemote() {
  router.push("/tabs/remote");
}

function _handleOpenExtensions() {
  router.push("/tabs/extensions");
}

function _handleOpenSimverse() {
  if (isNative()) {
    openWorld("default", "SimVerse");
  } else {
    router.push("/simverse/world");
  }
}

onIonViewWillEnter(() => {});
</script>

<style scoped>
.home-content {
  --background: var(--ion-background-color);
}

.home-welcome {
  text-align: center;
  padding: 32px 20px 16px;
}

.home-welcome h2 {
  font-size: 28px;
  font-weight: 700;
  margin: 0 0 4px;
  color: var(--ion-text-color);
}

.home-welcome p {
  font-size: 14px;
  color: var(--encv-text-secondary);
  margin: 0;
}

.home-cards {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  padding: 8px 16px 24px;
}

.home-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px 12px;
  border-radius: 16px;
  background: rgba(var(--ion-background-color-rgb), 0.55);
  backdrop-filter: blur(var(--encv-bg-blur, 8px));
  -webkit-backdrop-filter: blur(var(--encv-bg-blur, 8px));
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
  min-height: 140px;
}

.home-card:active {
  transform: scale(0.97);
}

.player-card {
  grid-column: 1 / -1;
  flex-direction: row;
  justify-content: flex-start;
  gap: 16px;
  padding: 24px 20px;
  min-height: 100px;
  background: linear-gradient(135deg, rgba(var(--ion-color-primary-rgb), 0.12), rgba(var(--ion-color-primary-rgb), 0.04));
  border: 1px solid rgba(var(--ion-color-primary-rgb), 0.2);
}

.card-icon {
  font-size: 36px;
  margin-bottom: 8px;
}

.player-card .card-icon {
  font-size: 44px;
  margin-bottom: 0;
  color: var(--ion-color-primary);
}

.files-icon {
  color: var(--ion-color-primary);
}

.tasks-icon {
  color: var(--ion-color-warning);
}

.remote-icon {
  color: var(--ion-color-success);
}

.extensions-icon {
  color: #8b5cf6;
}

.simverse-card {
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.12), rgba(139, 92, 246, 0.08));
  border: 1px solid rgba(20, 184, 166, 0.2);
}

.simverse-icon {
  color: #14b8a6;
}

.card-info {
  text-align: center;
}

.player-card .card-info {
  text-align: left;
}

.card-info h3 {
  font-size: 15px;
  font-weight: 600;
  margin: 0 0 4px;
  color: var(--ion-text-color);
}

.card-info p {
  font-size: 12px;
  color: var(--encv-text-secondary);
  margin: 0;
}
</style>
