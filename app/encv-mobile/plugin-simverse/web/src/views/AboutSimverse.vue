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
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonNote,
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
