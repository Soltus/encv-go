<template>
  <ion-page>
    <ion-tabs>
      <ion-router-outlet />

      <ion-tab-bar slot="bottom">
        <ion-tab-button tab="home" href="/tabs/home">
          <ion-icon :icon="homeOutline" />
          <ion-label>{{ t("simverse.tabs.home") }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="npcs" href="/tabs/npcs">
          <ion-icon :icon="peopleOutline" />
          <ion-label>{{ t("simverse.tabs.npcs") }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="chronicles" href="/tabs/chronicles">
          <ion-icon :icon="newspaperOutline" />
          <ion-label>{{ t("simverse.tabs.chronicles") }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="devlogs" href="/tabs/devlogs">
          <ion-icon :icon="documentTextOutline" />
          <ion-label>{{ t("simverse.tabs.devlogs") }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="settings" href="/tabs/settings">
          <ion-icon :icon="settingsOutline" />
          <ion-label>{{ t("simverse.tabs.settings") }}</ion-label>
        </ion-tab-button>
      </ion-tab-bar>
    </ion-tabs>
  </ion-page>
</template>

<script setup lang="ts">
import {
  home,
  people,
  newspaper,
  settings,
  documentText,
} from "ionicons/icons";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();

const homeOutline = home;
const peopleOutline = people;
const newspaperOutline = newspaper;
const documentTextOutline = documentText;
const settingsOutline = settings;
</script>
