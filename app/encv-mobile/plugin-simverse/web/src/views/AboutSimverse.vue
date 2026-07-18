<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings" />
        </ion-buttons>
        <ion-title>{{ t("simverse.about") }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4 space-y-4">
        <div class="about-hero">
          <div class="about-logo">🌌</div>
          <h2>{{ t("simverse.about.title") }}</h2>
          <p class="about-desc">{{ t("simverse.about.desc") }}</p>
        </div>

        <div class="ui-card">
          <div class="p-3">
            <div class="ui-header mb-2">{{ t("simverse.about.info") }}</div>
            <div class="space-y-1">
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.about.version") }}</span>
                <span class="text-xs text-base-content/70 font-mono">v0.1.0</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.about.backend") }}</span>
                <span class="text-xs text-base-content/70 font-mono">Go · FractalWorld</span>
              </div>
              <div class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors">
                <span class="text-sm font-medium">{{ t("simverse.about.frontend") }}</span>
                <span class="text-xs text-base-content/70 font-mono">Vue 3 · Ionic · Vite</span>
              </div>
            </div>
          </div>
        </div>
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
  IonPage,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";

const { t } = useI18n();
</script>

<style scoped lang="scss">
.about-hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 32px 16px 16px;

  h2 {
    margin: 0 0 8px;
    font-size: 20px;
  }
}

.about-logo {
  font-size: 56px;
  margin-bottom: 8px;
}

.about-desc {
  color: var(--color-base-content);
  opacity: 0.7;
  font-size: 13px;
  max-width: 320px;
  line-height: 1.6;
}
</style>
