<template>
  <div class="encrypt-body">
    <TaskFormFields
      :state="state"
      condition="encrypt"
      source-placeholder="/path/to/file"
      :on-update-source-path="props.onUpdateSourcePath"
      :on-update-target-path="props.onUpdateTargetPath"
      :on-update-primary-override="props.onUpdatePrimaryOverride"
      :on-update-secondary-password="props.onUpdateSecondaryPassword"
      :on-update-extra-value="props.onUpdateExtraValue"
    >
      <!-- 容器版本选择（仅插件声明 SupportVersionSelect 时显示） -->
      <template #after-source>
        <div v-if="state.taskOptions?.supportVersionSelect && versionOpts.length > 0" class="version-section">
          <ContainerVersionSelector
            :model-value="state.version"
            @update:model-value="(v: number) => props.onUpdateVersion?.(v)"
            :versions="versionOpts"
          />
        </div>
      </template>

      <!-- 加密强度选择（AES-128 / AES-256）—— 仅 ECv4 容器支持 -->
      <template #after-password>
        <div v-if="isV4Container" class="form-section">
          <div class="section-label">{{ t('tasks.cipherMode') }}</div>
          <ion-radio-group
            :value="state.cipherMode"
            @ionChange="(e: any) => props.onUpdateCipherMode?.(e.detail.value)"
            class="cipher-radio-group"
          >
            <RadioItem
              :value="0"
              :selected="state.cipherMode"
              class="extra-field-item cipher-item"
              @select="(v) => props.onUpdateCipherMode?.(v as number)"
            >
              <div class="cipher-title">
                <span>{{ t('tasks.cipherMode128') }}</span>
                <ion-badge color="primary" class="recommended-badge">{{ t('tasks.cipherModeRecommended') }}</ion-badge>
              </div>
              <ion-note class="cipher-desc">{{ t('tasks.cipherMode128Help') }}</ion-note>
            </RadioItem>
            <RadioItem
              :value="1"
              :selected="state.cipherMode"
              class="extra-field-item cipher-item"
              @select="(v) => props.onUpdateCipherMode?.(v as number)"
            >
              <div class="cipher-title">
                <span>{{ t('tasks.cipherMode256') }}</span>
              </div>
              <ion-note class="cipher-desc">{{ t('tasks.cipherMode256Help') }}</ion-note>
            </RadioItem>
          </ion-radio-group>
        </div>

        <!-- 压缩选择（无 / zstd）—— 仅 ECv4 容器支持 -->
        <div v-if="isV4Container" class="form-section">
          <div class="section-label">{{ t('tasks.compressionMode') }}</div>
          <ion-radio-group
            :value="state.compressionMode"
            @ionChange="(e: any) => props.onUpdateCompressionMode?.(e.detail.value)"
            class="cipher-radio-group"
          >
            <RadioItem
              value="none"
              :selected="state.compressionMode"
              class="extra-field-item cipher-item"
              @select="(v) => props.onUpdateCompressionMode?.(v as 'none' | 'zstd')"
            >
              <div class="cipher-title">
                <span>{{ t('tasks.compressionNone') }}</span>
              </div>
              <ion-note class="cipher-desc">{{ t('tasks.compressionNoneHelp') }}</ion-note>
            </RadioItem>
            <RadioItem
              value="zstd"
              :selected="state.compressionMode"
              class="extra-field-item cipher-item"
              @select="(v) => props.onUpdateCompressionMode?.(v as 'none' | 'zstd')"
            >
              <div class="cipher-title">
                <span>{{ t('tasks.compressionZstd') }}</span>
              </div>
              <ion-note class="cipher-desc">{{ t('tasks.compressionZstdHelp') }}</ion-note>
            </RadioItem>
          </ion-radio-group>
        </div>
      </template>
    </TaskFormFields>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { ContainerVersionInfo } from "@encv/shared-components/api/encv";
import ContainerVersionSelector from "@encv/shared-components/components/ContainerVersionSelector.vue";
import RadioItem from "@encv/shared-components/components/RadioItem.vue";
import TaskFormFields from "@encv/shared-components/components/TaskFormFields.vue";
import type { NewTaskState } from "@encv/shared-components/components/NewTaskState";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { isRecommendedVersion } from "@encv/shared-components/constants/containerVersion";

const props = withDefaults(
  defineProps<{
    state: NewTaskState;
    onUpdateSourcePath?: (v: string) => void;
    onUpdateTargetPath?: (v: string) => void;
    onUpdateVersion?: (v: number) => void;
    onUpdatePrimaryOverride?: (v: string) => void;
    onUpdateSecondaryPassword?: (v: string) => void;
    onUpdateCipherMode?: (v: number) => void;
    onUpdateCompressionMode?: (v: "none" | "zstd") => void;
    onUpdateExtraValue?: (payload: { key: string; value: string }) => void;
  }>(),
  {
    onUpdateSourcePath: undefined,
    onUpdateTargetPath: undefined,
    onUpdateVersion: undefined,
    onUpdatePrimaryOverride: undefined,
    onUpdateSecondaryPassword: undefined,
    onUpdateCipherMode: undefined,
    onUpdateCompressionMode: undefined,
    onUpdateExtraValue: undefined,
  }
);

const { t } = useI18n();

const versionOpts = computed<ContainerVersionInfo[]>(() => (Array.isArray(props.state.versionOptions) ? props.state.versionOptions : []));

// v4 容器：Header 含 CipherMode 字段（offset 2040-2042），且支持 zstd seekable 压缩
// v2/v3 容器：不显示 cipher mode / compression 控件（这两个特性 v4 独有）
const isV4Container = computed(() => isRecommendedVersion(Number(props.state.version)));
</script>

<style scoped>
.version-section {
  margin: 10px 0;
}

.section-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--ion-color-medium-shade);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin: 6px 0 4px;
}

.cipher-radio-group {
  width: 100%;
}

.cipher-item {
  --padding-start: 4px;
  --inner-padding-end: 4px;
}

.cipher-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.cipher-desc {
  display: block;
  color: var(--ion-color-medium);
  font-size: 0.8rem;
  margin-top: 2px;
}

.recommended-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
  --padding-top: 2px;
  --padding-bottom: 2px;
}
</style>
