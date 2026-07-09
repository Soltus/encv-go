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

    <ion-content class="ion-padding">
      <div class="about-hero">
        <div class="about-logo">🌌</div>
        <h2>{{ t("simverse.about.title") }}</h2>
        <p class="about-desc">{{ t("simverse.about.desc") }}</p>
      </div>

      <ion-list :inset="true">
        <ion-item>
          <ion-label>{{ t("simverse.about.version") }}</ion-label>
          <ion-note slot="end">v0.1.0</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.about.backend") }}</ion-label>
          <ion-note slot="end">Go · FractalWorld</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>{{ t("simverse.about.frontend") }}</ion-label>
          <ion-note slot="end">Vue 3 · Ionic · Vite</ion-note>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonContent, IonList, IonLabel, IonItem, IonNote,
} from "@ionic/vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();
</script>

<style scoped>
.about-hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 32px 16px 16px;
}
.about-logo {
  font-size: 56px;
  margin-bottom: 8px;
}
.about-hero h2 {
  margin: 0 0 8px;
  font-size: 20px;
}
.about-desc {
  color: var(--ion-color-medium);
  font-size: 13px;
  max-width: 320px;
  line-height: 1.6;
}
</style>
