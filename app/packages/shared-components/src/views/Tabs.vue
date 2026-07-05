<template>
  <ion-page>
    <ion-tabs @ionTabsWillChange="onTabsWillChange" @ionTabsDidChange="onTabsDidChange">
      <ion-router-outlet></ion-router-outlet>
      <ion-tab-bar slot="bottom">
        <ion-tab-button tab="home" href="/tabs/home">
          <ion-icon :icon="home"></ion-icon>
          <ion-label>{{ t('tabs.home') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="files" href="/tabs/files">
          <ion-icon :icon="folder"></ion-icon>
          <ion-label>{{ t('tabs.files') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="tasks" href="/tabs/tasks">
          <ion-icon :icon="list"></ion-icon>
          <ion-label>{{ t('tabs.tasks') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="remote" href="/tabs/remote">
          <ion-icon :icon="globe"></ion-icon>
          <ion-label>{{ t('tabs.remote') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="settings" href="/tabs/settings">
          <ion-icon :icon="settings"></ion-icon>
          <ion-label>{{ t('tabs.settings') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="devlogs" href="/tabs/devlogs">
          <ion-icon :icon="bug"></ion-icon>
          <ion-label>DevLogs</ion-label>
        </ion-tab-button>
      </ion-tab-bar>
    </ion-tabs>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();

function _onTabsWillChange(event: CustomEvent) {
  void event?.detail?.tab;
}

function _onTabsDidChange(event: CustomEvent) {
  void event?.detail?.tab;
}
</script>
