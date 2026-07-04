<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.composePrototypes') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <p class="section-hint">{{ t('devtools.composePrototypesHint') }}</p>

      <div class="prototype-cards">
        <div
          v-for="proto in prototypes"
          :key="proto.id"
          class="prototype-card"
          @click="handlePrototypeClick(proto)"
        >
          <div class="proto-header">
            <div class="proto-icon-wrap" :style="{ background: proto.accentColor }">
              <ion-icon :icon="iconMap[proto.icon]" class="proto-icon"></ion-icon>
            </div>
            <div class="proto-title-area">
              <h3 class="proto-title">{{ proto.name }}</h3>
              <p class="proto-route">{{ proto.route }}</p>
            </div>
            <ion-icon :icon="chevronForward" class="proto-arrow"></ion-icon>
          </div>
          <div class="proto-compose-path">
            <span class="path-label">Compose</span>
            <code class="path-value">{{ proto.composePath }}</code>
          </div>
          <p class="proto-desc">{{ proto.description }}</p>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { IonBackButton, IonButtons, IonContent, IonHeader, IonIcon, IonPage, IonTitle, IonToolbar } from "@ionic/vue";
import { chevronForward, colorPaletteOutline, musicalNotesOutline, playCircleOutline, settingsOutline } from "ionicons/icons";
import { useRouter } from "vue-router";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { getAllPrototypes } from "./prototypes/registry";
import "./prototypes/prototype-cards.css";

const { t } = useI18n();
const router = useRouter();

const prototypes = getAllPrototypes();

const iconMap: Record<string, string> = {
  "play-circle": playCircleOutline,
  settings: settingsOutline,
  "musical-notes": musicalNotesOutline,
  "color-palette": colorPaletteOutline,
};

function handlePrototypeClick(proto: (typeof prototypes)[0]) {
  router.push(`/tabs/settings/devtools/prototype/${proto.id}`);
}
</script>

<style scoped>
.section-hint {
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  margin: 12px 16px 8px;
  line-height: 1.5;
}
</style>
