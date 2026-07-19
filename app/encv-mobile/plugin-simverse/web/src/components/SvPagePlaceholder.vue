<template>
  <ion-page>
    <ion-header>
      <ion-toolbar color="transparent">
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/home" />
        </ion-buttons>
        <ion-title class="flex items-center gap-2">
          <ion-icon :icon="icon" />
          {{ title }}
        </ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content fullscreen class="ion-padding">
      <div
        class="flex flex-col items-center justify-center text-center py-10 px-5 text-base-content/60"
      >
        <div class="text-5xl mb-3 opacity-50">
          <ion-icon :icon="icon" />
        </div>
        <div class="text-sm">{{ t("simverse.comingSoon") }}</div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { IonBackButton, IonButtons, IonContent, IonHeader, IonIcon, IonPage, IonTitle, IonToolbar } from "@ionic/vue";
import {
  bag,
  bug,
  business,
  cash,
  construct,
  flag,
  gitNetwork,
  globe,
  informationCircle,
  laptop,
  location,
  map,
  newspaper,
  people,
  person,
  pricetag,
  save,
  settings,
  time,
} from "ionicons/icons";
import { computed } from "vue";

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

const title = computed(() => (props.titleKey ? t(props.titleKey as any) : "SimVerse"));
const icon = computed(() => iconMap[props.iconName || "globe"] || globe);
</script>
