<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>{{ t("simverse.settings.title") }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content class="settings-content">
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t("simverse.settings.physics") }}</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-label>{{ t("simverse.settings.gravity") }}</ion-label>
          <ion-range :value="gravity" min="0" max="100" step="1" @ionChange="onGravityChange" />
          <ion-note slot="end">{{ gravity }}%</ion-note>
        </ion-item>

        <ion-item>
          <ion-label>{{ t("simverse.settings.fps") }}</ion-label>
          <ion-select :value="fpsLimit" @ionChange="onFpsChange">
            <ion-select-option value="30">30 FPS</ion-select-option>
            <ion-select-option value="60">60 FPS</ion-select-option>
            <ion-select-option value="120">120 FPS</ion-select-option>
            <ion-select-option value="0">无限制</ion-select-option>
          </ion-select>
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t("simverse.settings.graphics") }}</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-label>{{ t("simverse.settings.debugMode") }}</ion-label>
          <ion-toggle :checked="debugMode" @ionChange="debugMode = !debugMode" />
        </ion-item>

        <ion-item>
          <ion-label>{{ t("simverse.settings.showFps") }}</ion-label>
          <ion-toggle :checked="showFps" @ionChange="showFps = !showFps" />
        </ion-item>
      </ion-list>

      <ion-list>
        <ion-list-header>
          <ion-label>关于</ion-label>
        </ion-list-header>

        <ion-item>
          <ion-label>版本</ion-label>
          <ion-note slot="end">v0.1.0</ion-note>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();

const gravity = ref(50);
const fpsLimit = ref("60");
const debugMode = ref(false);
const showFps = ref(true);

function onGravityChange(e: any) {
  gravity.value = e.detail.value;
}

function onFpsChange(e: any) {
  fpsLimit.value = e.detail.value;
}
</script>

<style scoped>
.settings-content {
  --background: var(--ion-color-step-50);
}

ion-list {
  margin-bottom: 20px;
}
</style>
