<template>
  <ion-page class="sv-page">
    <ion-header>
      <ion-toolbar color="transparent">
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/home" />
        </ion-buttons>
        <ion-title class="sv-card-title">
          <ion-icon :icon="icon" />
          {{ title }}
        </ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content fullscreen class="ion-padding">
      <div class="sv-empty">
        <div class="sv-empty-icon">
          <ion-icon :icon="icon" />
        </div>
        <div class="sv-empty-text">{{ t("simverse.comingSoon") }}</div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { computed } from "vue";
import {
  IonPage,
  IonHeader,
  IonToolbar,
  IonButtons,
  IonBackButton,
  IonTitle,
  IonContent,
  IonIcon,
} from "@ionic/vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  globe,
  people,
  business,
  map,
  newspaper,
  cash,
  settings,
  bug,
  person,
  flag,
  location,
  time,
  bag,
  pricetag,
  gitNetwork,
  save,
  informationCircle,
  construct,
  laptop,
} from "ionicons/icons";

const props = defineProps<{
  titleKey?: string;
  iconName?: string;
}>();

const { t } = useI18n();

const iconMap: Record<string, any> = {
  globe,
  people,
  business,
  map,
  newspaper,
  cash,
  settings,
  bug,
  person,
  flag,
  location,
  time,
  bag,
  pricetag,
  gitNetwork,
  save,
  informationCircle,
  construct,
  laptop,
};

const title = computed(() =>
  props.titleKey ? t(props.titleKey as any) : "SimVerse"
);
const icon = computed(() => iconMap[props.iconName || "globe"] || globe);
</script>
