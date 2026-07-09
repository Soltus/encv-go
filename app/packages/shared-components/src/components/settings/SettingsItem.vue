<template>
  <ion-item :button="button" :detail="detail" :lines="lines" @click="onClick">
    <ion-icon v-if="icon" :icon="icon" slot="start"></ion-icon>
    <ion-label v-if="title || $slots.default">
      <h3 v-if="title">{{ title }}</h3>
      <p v-if="description">{{ description }}</p>
      <slot />
    </ion-label>
    <slot name="end" />
  </ion-item>
</template>

<script setup lang="ts">
import { IonItem, IonIcon, IonLabel } from "@ionic/vue";

interface Props {
  icon?: any;
  title?: string;
  description?: string;
  button?: boolean;
  detail?: boolean;
  lines?: "full" | "inset" | "none";
}
const props = withDefaults(defineProps<Props>(), {
  button: false,
  detail: false,
  lines: "inset",
});

const emit = defineEmits<(e: "click") => void>();

function onClick() {
  if (props.button) {
    emit("click");
  }
}
</script>

<style scoped>
ion-item {
  --padding-start: 16px;
  --inner-padding-end: 12px;
}
h3 {
  font-size: 16px;
  font-weight: 500;
  margin: 0 0 2px 0;
}
p {
  font-size: 13px;
  color: var(--ion-color-medium, #6b7280);
  margin: 0;
}
</style>
