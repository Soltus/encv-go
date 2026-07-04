<template>
  <div class="encrypt-body">
    <!-- 源文件 -->
    <div class="form-section">
      <InputWithHistory
        :model-value="state.sourcePath"
        :label="t('tasks.sourcePath')"
        placeholder="/path/to/file"
        :icon="documentText"
        history-key="task.encrypt.sourcePath"
        browsable
        @update:model-value="(v: string) => props.onUpdateSourcePath?.(v)"
        @browse="handleBrowseSource"
      />

      <!-- 目标路径 -->
      <InputWithHistory
        :model-value="state.targetPath"
        :label="t('tasks.targetPath')"
        :placeholder="t('tasks.targetPathPlaceholder')"
        :icon="folderOpen"
        history-key="task.encrypt.targetPath"
        browsable
        @update:model-value="(v: string) => props.onUpdateTargetPath?.(v)"
        @browse="handleBrowseTarget"
      />
    </div>

    <!-- 容器版本选择（仅插件声明 SupportVersionSelect 时显示） -->
    <div v-if="state.taskOptions?.supportVersionSelect && versionOpts.length > 0" class="version-section">
      <ContainerVersionSelector
        :model-value="state.version"
        @update:model-value="(v: number) => props.onUpdateVersion?.(v)"
        :versions="versionOpts"
      />
    </div>

    <!-- 密码字段（仅 PasswordGlobal 策略显示） -->
    <div v-if="!state.taskOptions || state.taskOptions.passwordStrategy === 'global'" class="form-section password-section">
      <InputWithHistory
        :model-value="state.primaryOverride"
        :label="t('tasks.passwordOverride')"
        :placeholder="t('tasks.passwordOverrideHelp')"
        :icon="lockClosed"
        input-type="password"
        history-key="task.encrypt.primaryOverride"
        @update:model-value="(v: string) => props.onUpdatePrimaryOverride?.(v)"
      />
      <InputWithHistory
        :model-value="state.secondaryPassword"
        :label="t('tasks.secondaryPassword')"
        :placeholder="t('tasks.secondaryPasswordHelp')"
        :icon="lockClosed"
        input-type="password"
        history-key="task.encrypt.secondaryPassword"
        @update:model-value="(v: string) => props.onUpdateSecondaryPassword?.(v)"
      />
    </div>

    <!-- 加密强度选择（AES-128 / AES-256）—— 仅 ECv4 容器支持
         ECv2/ECv3 容器的 Header 没有 CipherMode 字段（ECv4 才有，offset 2040-2042），
         故 cipher mode 控件只在 isRecommendedVersion() 为 true 时显示。
         整行点击切换：使用 RadioItem 包装器（统一所有 radio 选择项的 UX） -->
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

    <!-- 压缩选择（无 / zstd）—— 仅 ECv4 容器支持
         ECv2/ECv3 容器没有 zstd seekable 支持，compression 控件只在 isRecommendedVersion() 为 true 时显示。
         整行点击切换：使用 RadioItem 包装器 -->
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

    <!-- 加密模式专属 extraFields（condition === 'encrypt' 或无 condition） -->
    <template v-for="field in encryptExtraFields" :key="field.key">
      <ion-item
        v-if="field.type === 'bool'"
        lines="none"
        class="extra-field-item"
      >
        <ion-label>{{ t(field.label) }}</ion-label>
        <ion-toggle
          slot="end"
          :checked="getExtra(field.key) === 'true'"
          @ionChange="(e: any) => { const v = e.detail.checked ? 'true' : 'false'; props.onUpdateExtraValue?.({ key: field.key, value: v }) }"
          class="extra-field-toggle"
        />
        <ion-note v-if="field.help" slot="helper">{{ t(field.help) }}</ion-note>
      </ion-item>

      <ion-item
        v-else-if="field.type === 'select'"
        lines="none"
        class="extra-field-item"
      >
        <ion-select
          :model-value="getExtra(field.key)"
          @ionChange="(e: any) => props.onUpdateExtraValue?.({ key: field.key, value: e.detail.value })"
          :label="t(field.label)"
          interface="action-sheet"
          placement="bottom"
          class="extra-field-select"
        >
          <ion-select-option
            v-for="opt in (field.options || [])"
            :key="opt"
            :value="opt"
          >
            {{ field.optionLabels?.[opt] ?? opt }}
          </ion-select-option>
        </ion-select>
        <ion-note v-if="field.help" slot="helper">{{ t(field.help) }}</ion-note>
      </ion-item>

      <InputWithHistory
        v-else
        :model-value="getExtra(field.key)"
        :label="t(field.label)"
        :placeholder="t(field.help || '')"
        :icon="field.type === 'password' ? lockClosed : documentText"
        :input-type="field.type === 'password' ? 'password' : 'text'"
        :history-key="`task.encrypt.extra.${field.key}`"
        @update:model-value="(v: string) => props.onUpdateExtraValue?.({ key: field.key, value: v })"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { IonBadge, IonItem, IonLabel, IonNote, IonRadioGroup, IonSelect, IonSelectOption, IonToggle, modalController } from "@ionic/vue";
import { documentText, folderOpen, lockClosed } from "ionicons/icons";
import { computed } from "vue";
import type { ContainerVersionInfo, TaskField } from "@encv/shared-components/api/encv";
import ContainerVersionSelector from "@encv/shared-components/components/ContainerVersionSelector.vue";
import FilePickerModal from "@encv/shared-components/components/FilePickerModal.vue";
import InputWithHistory from "@encv/shared-components/components/InputWithHistory.vue";
import type { NewTaskState } from "@encv/shared-components/components/NewTaskState";
import RadioItem from "@encv/shared-components/components/RadioItem.vue";
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

const encryptExtraFields = computed<TaskField[]>(() => {
  const arr = Array.isArray(props.state.filteredExtraFields) ? props.state.filteredExtraFields : [];
  return arr.filter(f => !f.condition || f.condition === "encrypt");
});

function getExtra(key: string): string {
  const ev = props.state?.extraValues;
  if (!ev || typeof ev !== "object") return "";
  return ev[key] || "";
}

async function handleBrowseSource() {
  const modal = await modalController.create({
    component: FilePickerModal,
    componentProps: { mode: "file" as const },
  });
  await modal.present();
  const { data, role } = await modal.onDidDismiss();
  if (role === "select" && data) {
    props.onUpdateSourcePath?.(data.path);
  }
}

async function handleBrowseTarget() {
  const modal = await modalController.create({
    component: FilePickerModal,
    componentProps: { mode: "folder" as const },
  });
  await modal.present();
  const { data, role } = await modal.onDidDismiss();
  if (role === "select" && data) {
    props.onUpdateTargetPath?.(data.path);
  }
}
</script>

<style scoped>
.form-section {
  margin-bottom: 12px;
}

.version-section {
  margin: 10px 0;
}

.password-section {
  margin-top: 8px;
}

.extra-field-item {
  --background: transparent;
  --padding-start: 0;
  --padding-end: 0;
  --inner-padding-end: 0;
  margin-top: 4px;
  color: var(--ion-text-color);
}

.extra-field-toggle {
  --padding-start: 0;
}

.extra-field-item ion-note[slot=helper] {
  color: var(--ion-text-color, inherit);
  opacity: 0.6;
  font-size: 0.8rem;
}

.extra-field-select {
  width: 100%;
  --padding-start: 0;
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
