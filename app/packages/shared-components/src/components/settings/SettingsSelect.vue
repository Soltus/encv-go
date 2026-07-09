<template>
  <ion-item :lines="lines">
    <ion-icon v-if="icon" :icon="icon" slot="start"></ion-icon>
    <ion-select
      :value="modelValue"
      @ionChange="onChange"
      :label="label"
      label-placement="stacked"
      :interface="interface"
      mode="ios"
    >
      <ion-select-option
        v-for="opt in options"
        :key="opt.value"
        :value="opt.value"
        :disabled="opt.disabled"
      >
        {{ opt.label }}
      </ion-select-option>
    </ion-select>
    <slot name="end" />
  </ion-item>
</template>

<script setup lang="ts">
import { IonItem, IonIcon, IonSelect, IonSelectOption } from "@ionic/vue";

interface SelectOption {
  value: string | number;
  label: string;
  disabled?: boolean;
}

interface Props {
  icon?: any;
  label: string;
  modelValue: string | number;
  options: SelectOption[];
  lines?: "full" | "inset" | "none";
  interface?: "action-sheet" | "alert" | "popover";
}
withDefaults(defineProps<Props>(), {
  lines: "inset",
  interface: "action-sheet",
});

const emit = defineEmits<(e: "update:modelValue", value: string | number) => void>();

function onChange(e: any) {
  emit("update:modelValue", e.detail.value);
}
</script>

<style scoped>
ion-item {
  --padding-start: 16px;
  --inner-padding-end: 12px;
}
</style>
