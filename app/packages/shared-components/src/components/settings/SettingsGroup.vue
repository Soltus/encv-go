<template>
  <ion-list :inset="inset">
    <ion-list-header v-if="title || $slots.header">
      <ion-label v-if="title">{{ title }}</ion-label>
      <slot name="header" />
      <ion-badge v-if="scopeLabel" slot="end" :color="scopeColor" class="scope-badge">
        <ion-icon v-if="scopeIcon" :icon="scopeIcon" class="scope-badge-icon"></ion-icon>
        <span v-if="scopeLabel" class="scope-text">{{ scopeLabel }}</span>
      </ion-badge>
    </ion-list-header>
    <slot />
  </ion-list>
</template>

<script setup lang="ts">
import { IonList, IonListHeader, IonLabel, IonBadge, IonIcon } from "@ionic/vue";

interface Props {
  title?: string;
  inset?: boolean;
  scopeLabel?: string;
  scopeColor?: string;
  scopeIcon?: any;
}
withDefaults(defineProps<Props>(), {
  inset: false,
  scopeColor: "medium",
});
</script>

<style scoped>
.scope-badge {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 500;
  padding: 2px 8px;
}
.scope-badge-icon {
  font-size: 13px;
}
.scope-text {
  line-height: 1;
}
</style>
