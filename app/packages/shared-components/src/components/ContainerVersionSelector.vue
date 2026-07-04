<template>
  <div class="version-selector">
    <ion-radio-group :value="modelValue" @ionChange="handleChange">
      <RadioItem
        v-for="ver in versions"
        :key="ver.version"
        :value="ver.version"
        :selected="modelValue"
        :disabled="ver.status === 'deprecated'"
        :class="['version-item', `version-status-${ver.status}`]"
        @select="(v) => emit('update:modelValue', v as number)"
      >
        <span class="version-label">{{ ver.label }}</span>
        <ion-badge
          v-if="ver.status === 'recommended'"
          color="success"
          slot="end"
          class="status-badge"
        >{{ t('containerVersion.recommended') }}</ion-badge>
        <ion-badge
          v-else-if="ver.status === 'deprecated'"
          color="medium"
          slot="end"
          class="status-badge"
        >{{ t('containerVersion.deprecated') }}</ion-badge>
      </RadioItem>
    </ion-radio-group>
  </div>
</template>

<script setup lang="ts">
import { IonBadge, IonRadioGroup } from "@ionic/vue";
import { computed } from "vue";
import type { ContainerVersionInfo } from "@encv/shared-components/api/encv";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { CONTAINER_VERSIONS, DEFAULT_CONTAINER_VERSION } from "@encv/shared-components/constants/containerVersion";
import RadioItem from "./RadioItem.vue";

const props = withDefaults(
  defineProps<{
    modelValue: number;
    versions?: ContainerVersionInfo[];
  }>(),
  {
    modelValue: DEFAULT_CONTAINER_VERSION,
  }
);

const emit = defineEmits<(e: "update:modelValue", value: number) => void>();

const { t } = useI18n();

// 🆕 2026-06-11 v2 cleanup：版本列表从 constants/containerVersion.ts 统一派生
// 命名规则：ECv = ENCV Container，大写 EC，小写 v，避免与项目内 v2 架构命名混淆。
// 注：ECV2 已在 SupportedVersions 中移除，不再可选。
const versions = computed<ContainerVersionInfo[]>(() => props.versions ?? [...CONTAINER_VERSIONS]);

function handleChange(event: CustomEvent) {
  emit("update:modelValue", event.detail.value as number);
}
</script>

<style scoped>
.version-selector {
  width: 100%;
}

.version-item {
  --padding-start: 8px;
  --inner-padding-end: 12px;
  cursor: pointer;
}

.version-item.item-disabled {
  cursor: not-allowed;
  opacity: 0.55;
  pointer-events: auto;
}

.version-item.version-status-deprecated {
  --color: var(--ion-color-medium);
  opacity: 0.7;
}

.version-item.version-status-recommended {
  --background: rgba(var(--ion-color-success-rgb), 0.04);
}

.version-label {
  font-weight: 500;
}

.status-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
  --padding-top: 3px;
  --padding-bottom: 3px;
}
</style>
