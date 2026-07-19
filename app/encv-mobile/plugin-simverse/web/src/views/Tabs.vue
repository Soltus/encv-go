<template>
  <ion-page>
    <ion-tabs>
      <ion-router-outlet />

      <ion-tab-bar slot="bottom" class="bg-base-100 border-t border-base-300">
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
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { documentText, home, newspaper, people, settings } from "ionicons/icons";

const { t } = useI18n();

const homeOutline = home;
const peopleOutline = people;
const newspaperOutline = newspaper;
const documentTextOutline = documentText;
const settingsOutline = settings;
</script>

<style scoped lang="scss">
// Ionic tab-bar theming via daisyUI tokens.
// Ionic uses --background on ion-tab-bar and --color / --color-selected on
// ion-tab-button, so we feed daisyUI CSS variables into those custom
// properties to keep runtime light/dark theme switching intact.
ion-tab-bar {
  --background: var(--color-base-100);

  ion-tab-button {
    --color: color-mix(in srgb, var(--color-base-content) 60%, transparent);
    --color-selected: var(--color-primary);
  }
}
</style>
