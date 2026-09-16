<template>
  <div class="task-form-fields">
    <!-- 源文件 -->
    <div class="form-section">
      <InputWithHistory
        :model-value="state.sourcePath"
        :label="t('tasks.sourcePath')"
        :placeholder="sourcePlaceholder"
        :icon="documentText"
        :history-key="`task.${condition}.sourcePath`"
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
        :history-key="`task.${condition}.targetPath`"
        browsable
        @update:model-value="(v: string) => props.onUpdateTargetPath?.(v)"
        @browse="handleBrowseTarget"
      />
    </div>

    <!-- 加密/解密独有区块可插入到源/目标之后（如容器版本选择） -->
    <slot name="after-source" />

    <!-- 密码字段（仅 PasswordGlobal 策略显示） -->
    <div v-if="!state.taskOptions || state.taskOptions.passwordStrategy === 'global'" class="form-section password-section">
      <InputWithHistory
        :model-value="state.primaryOverride"
        :label="t('tasks.passwordOverride')"
        :placeholder="t('tasks.passwordOverrideHelp')"
        :icon="lockClosed"
        input-type="password"
        :history-key="`task.${condition}.primaryOverride`"
        @update:model-value="(v: string) => props.onUpdatePrimaryOverride?.(v)"
      />
      <InputWithHistory
        :model-value="state.secondaryPassword"
        :label="t('tasks.secondaryPassword')"
        :placeholder="t('tasks.secondaryPasswordHelp')"
        :icon="lockClosed"
        input-type="password"
        :history-key="`task.${condition}.secondaryPassword`"
        @update:model-value="(v: string) => props.onUpdateSecondaryPassword?.(v)"
      />
    </div>

    <!-- 加密/解密独有区块可插入到密码段之后（如 cipher / compression 选择） -->
    <slot name="after-password" />

    <!-- extraFields（按 condition 过滤） -->
    <template v-for="field in extraFields" :key="field.key">
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
        :history-key="`task.${condition}.extra.${field.key}`"
        @update:model-value="(v: string) => props.onUpdateExtraValue?.({ key: field.key, value: v })"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { documentText, folderOpen, lockClosed } from "ionicons/icons";
import { computed } from "vue";
import type { TaskField } from "@encv/shared-components/api/encv";
import InputWithHistory from "@encv/shared-components/components/InputWithHistory.vue";
import type { NewTaskState } from "@encv/shared-components/components/NewTaskState";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useFilePickerBrowse } from "@encv/shared-components/composables/useFilePickerBrowse";

const props = withDefaults(
  defineProps<{
    state: NewTaskState;
    condition: "encrypt" | "decrypt";
    sourcePlaceholder?: string;
    onUpdateSourcePath?: (v: string) => void;
    onUpdateTargetPath?: (v: string) => void;
    onUpdatePrimaryOverride?: (v: string) => void;
    onUpdateSecondaryPassword?: (v: string) => void;
    onUpdateExtraValue?: (payload: { key: string; value: string }) => void;
  }>(),
  {
    sourcePlaceholder: "/path/to/file",
    onUpdateSourcePath: undefined,
    onUpdateTargetPath: undefined,
    onUpdatePrimaryOverride: undefined,
    onUpdateSecondaryPassword: undefined,
    onUpdateExtraValue: undefined,
  }
);

const { t } = useI18n();
const { browseFile, browseFolder } = useFilePickerBrowse();

const extraFields = computed<TaskField[]>(() => {
  const arr = Array.isArray(props.state.filteredExtraFields) ? props.state.filteredExtraFields : [];
  return arr.filter(f => !f.condition || f.condition === props.condition);
});

function getExtra(key: string): string {
  const ev = props.state?.extraValues;
  if (!ev || typeof ev !== "object") return "";
  return ev[key] || "";
}

async function handleBrowseSource() {
  const path = await browseFile();
  if (path) props.onUpdateSourcePath?.(path);
}

async function handleBrowseTarget() {
  const path = await browseFolder();
  if (path) props.onUpdateTargetPath?.(path);
}
</script>

<style scoped>
.form-section {
  margin-bottom: 12px;
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
</style>
