<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons v-if="showBackButton" slot="start">
          <ion-back-button :default-href="backHref" />
        </ion-buttons>
        <ion-title>{{ title }}</ion-title>
        <ion-buttons slot="end" v-if="dirty || loading">
          <ion-button v-if="dirty" @click="onReset" color="medium">{{ resetText }}</ion-button>
          <ion-button @click="onSave" :disabled="loading">
            <ion-spinner v-if="loading" slot="icon-only" name="crescent" />
            <ion-icon v-else :icon="saveIcon" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <slot />
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton, IonButton, IonIcon, IonContent, IonSpinner } from "@ionic/vue";
import { save as saveIcon } from "ionicons/icons";
import { useI18n } from "../../composables/useI18n";

interface Props {
  title: string;
  dirty?: boolean;
  loading?: boolean;
  showBackButton?: boolean;
  backHref?: string;
  resetText?: string;
}
const props = withDefaults(defineProps<Props>(), {
  dirty: false,
  loading: false,
  showBackButton: false,
  backHref: "",
});

const { t } = useI18n();
const resetText = props.resetText || t("settings.undo");

const emit = defineEmits<(e: "save") => void | (e: "reset") => void>();

function onSave() {
  emit("save");
}
function onReset() {
  emit("reset");
}
</script>

<style scoped>
</style>
