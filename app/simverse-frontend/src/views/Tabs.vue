<template>
  <ion-page>
    <ion-tabs>
      <ion-router-outlet />

      <ion-tab-bar slot="bottom">
        <ion-tab-button tab="home" href="/tabs/home">
          <ion-icon :icon="homeOutline" />
          <ion-label>{{ t("tabs.home") }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="world" href="/tabs/world">
          <ion-icon :icon="globeOutline" />
          <ion-label>{{ t("tabs.world") }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="settings" href="/tabs/settings">
          <ion-icon :icon="settingsOutline" />
          <ion-label>{{ t("tabs.settings") }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="devlogs" href="/tabs/devlogs">
          <ion-icon :icon="bugOutline" />
          <ion-label>{{ t("tabs.devlogs") }}</ion-label>
        </ion-tab-button>
      </ion-tab-bar>
    </ion-tabs>
  </ion-page>
</template>

<script setup lang="ts">
import { home, globe, settings, bug } from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();

const homeOutline = home;
const globeOutline = globe;
const settingsOutline = settings;
const bugOutline = bug;
</script>
