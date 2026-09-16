<template>
  <div class="decrypt-body">
    <TaskFormFields
      :state="state"
      condition="decrypt"
      source-placeholder="/path/to/container.enc"
      :on-update-source-path="props.onUpdateSourcePath"
      :on-update-target-path="props.onUpdateTargetPath"
      :on-update-primary-override="props.onUpdatePrimaryOverride"
      :on-update-secondary-password="props.onUpdateSecondaryPassword"
      :on-update-extra-value="props.onUpdateExtraValue"
    />
  </div>
</template>

<script setup lang="ts">
import TaskFormFields from "@encv/shared-components/components/TaskFormFields.vue";
import type { NewTaskState } from "@encv/shared-components/components/NewTaskState";

const props = withDefaults(
  defineProps<{
    state: NewTaskState;
    onUpdateSourcePath?: (v: string) => void;
    onUpdateTargetPath?: (v: string) => void;
    onUpdatePrimaryOverride?: (v: string) => void;
    onUpdateSecondaryPassword?: (v: string) => void;
    onUpdateExtraValue?: (payload: { key: string; value: string }) => void;
  }>(),
  {
    onUpdateSourcePath: undefined,
    onUpdateTargetPath: undefined,
    onUpdatePrimaryOverride: undefined,
    onUpdateSecondaryPassword: undefined,
    onUpdateExtraValue: undefined,
  }
);
</script>

<style scoped>
.decrypt-body {
  display: block;
}
</style>
