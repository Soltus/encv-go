<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button :default-href="backHref"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ computedTitle }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <ion-list>
        <ion-list-header>
          <ion-label>{{ computedTitle }}</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-icon :icon="informationCircle" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ appName }}</h3>
            <p>{{ appVersion }}</p>
          </ion-label>
        </ion-item>
        <ion-item v-if="engineName">
          <ion-icon :icon="codeSlash" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.engine') }}</h3>
            <p>{{ engineName }}</p>
          </ion-label>
        </ion-item>
        <ion-item v-if="githubUrl" button @click="openGitHub">
          <ion-icon :icon="logoGithub" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.github') }}</h3>
            <p>{{ t('settings.sourceCode') }}</p>
          </ion-label>
          <ion-icon :icon="openOutline" slot="end"></ion-icon>
        </ion-item>
      </ion-list>

      <ion-list v-if="license || licenseUrl">
        <ion-list-header>
          <ion-label>{{ t('about.license') }}</ion-label>
        </ion-list-header>
        <ion-item v-if="licenseUrl" button @click="openLicense">
          <ion-icon :icon="documentTextOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ license }}</h3>
            <p>{{ t('about.thirdPartyLibs') }}</p>
          </ion-label>
          <ion-icon :icon="openOutline" slot="end"></ion-icon>
        </ion-item>
        <ion-item v-else>
          <ion-icon :icon="documentTextOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ license }}</h3>
          </ion-label>
        </ion-item>
      </ion-list>

      <slot name="extra"></slot>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  codeSlash,
  documentTextOutline,
  informationCircle,
  logoGithub,
  openOutline,
} from "ionicons/icons";

import { computed } from "vue";
import { useI18n } from "../composables/useI18n";

interface Props {
  title?: string;
  appName: string;
  appVersion?: string;
  engineName?: string;
  githubUrl?: string;
  license?: string;
  licenseUrl?: string;
  backHref?: string;
}

const props = defineProps<Props>();

const { t } = useI18n();

const computedTitle = computed(() => props.title || t('settings.about'));

function openGitHub() {
  if (props.githubUrl) {
    window.open(props.githubUrl, "_blank");
  }
}

function openLicense() {
  if (props.licenseUrl) {
    window.open(props.licenseUrl, "_blank");
  }
}
</script>

<style scoped>
</style>
