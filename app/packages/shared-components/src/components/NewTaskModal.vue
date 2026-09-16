<template>
  <ion-page>
    <ion-header class="modal-header">
      <ion-toolbar>
        <ion-title>{{ t('tasks.newTask') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="handleClose" fill="clear" size="small" color="medium">
            {{ t('tasks.close') }}
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding">
      <!-- 任务类型选择 -->
      <div class="form-section">
        <div class="field-group">
          <ion-select
            :model-value="taskType"
            @ionChange="(e: any) => { props.onUpdateTaskType?.(e.detail.value) }"
            interface="action-sheet"
            :label="t('tasks.taskType')"
            label-placement="stacked"
            class="task-type-select"
          >
            <ion-select-option value="encrypt">{{ t('tasks.encrypt') }}</ion-select-option>
            <ion-select-option value="decrypt">{{ t('tasks.decrypt') }}</ion-select-option>
          </ion-select>
        </div>
      </div>

      <!-- 模式分支: EncryptBody / DecryptBody
           源/目标文件输入（含浏览按钮）由 EncryptBody/DecryptBody 内部 InputWithHistory 渲染。
           原顶部"浏览"行已删除，避免出现两个源文件输入导致重复。 -->
      <div v-if="isPredicting" class="plugin-section predicting">
        <ion-spinner name="crescent" class="predict-spinner"></ion-spinner>
        <span class="predict-text">{{ t('tasks.phaseAnalyzing') }}</span>
      </div>

      <div v-else-if="cands.length > 1" class="plugin-section multi-plugin">
        <div class="section-label">{{ t('tasks.selectPlugin') }}</div>
        <ion-select
          :model-value="selectedIdx"
          @ionChange="(e: any) => { props.onSelectPlugin?.(e.detail.value) }"
          interface="action-sheet"
          placement="bottom"
          class="plugin-select"
        >
          <ion-select-option v-for="(c, idx) in cands" :key="idx" :value="idx">
            {{ c.name }}
            <span class="match-type-badge" :class="'mt-' + c.matchType">{{ c.matchType }}</span>
          </ion-select-option>
        </ion-select>
      </div>

      <div v-else-if="cands.length === 1 && pluginName" class="plugin-section single-plugin">
        <ion-icon :icon="checkmarkCircle" color="success" class="plugin-check"></ion-icon>
        <div class="plugin-info">
          <span class="plugin-name">{{ pluginName }}</span>
          <span class="plugin-match-type">{{ getMatchTypeLabel(cands[0].matchType) }}</span>
        </div>
      </div>

      <!-- 密码策略提示 -->
      <div v-if="!isPredicting && taskOpts" class="plugin-hint" :class="{ 'strategy-independent': taskOpts.passwordStrategy === 'independent' }">
        {{ taskOpts.passwordStrategy === 'independent' ? t('tasks.usesIndependentPassword') : t('tasks.usesGlobalPassword') }}
      </div>

      <!-- 模式分支: EncryptBody / DecryptBody -->
      <EncryptBody
        v-if="taskType === 'encrypt'"
        :state="effectiveState"
        :on-update-source-path="props.onUpdateSourcePath"
        :on-update-target-path="props.onUpdateTargetPath"
        :on-update-version="props.onUpdateVersion"
        :on-update-primary-override="props.onUpdatePrimaryOverride"
        :on-update-secondary-password="props.onUpdateSecondaryPassword"
        :on-update-cipher-mode="props.onUpdateCipherMode"
        :on-update-compression-mode="props.onUpdateCompressionMode"
        :on-update-extra-value="props.onUpdateExtraValue"
      />
      <DecryptBody
        v-else
        :state="effectiveState"
        :on-update-source-path="props.onUpdateSourcePath"
        :on-update-target-path="props.onUpdateTargetPath"
        :on-update-primary-override="props.onUpdatePrimaryOverride"
        :on-update-secondary-password="props.onUpdateSecondaryPassword"
        :on-update-extra-value="props.onUpdateExtraValue"
      />

      <!-- 提交按钮 -->
      <ion-button
        expand="block"
        class="submit-btn"
        :disabled="!src || isPredicting"
        @click="() => { props.onSubmit?.() }"
      >
        <ion-icon :icon="lockClosed" slot="start"></ion-icon>
        {{ t('tasks.createTask') }}
      </ion-button>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { modalController } from "@ionic/vue";
import { checkmarkCircle, lockClosed } from "ionicons/icons";
import { computed, reactive } from "vue";
import type { ContainerVersionInfo, PluginCandidate, TaskField, TaskOptions } from "@encv/shared-components/api/encv";
import DecryptBody from "@encv/shared-components/components/DecryptBody.vue";
import EncryptBody from "@encv/shared-components/components/EncryptBody.vue";
import type { NewTaskState } from "@encv/shared-components/components/NewTaskState";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    state?: NewTaskState;
    taskType: string;
    sourcePath: string;
    targetPath: string;
    candidates: PluginCandidate[];
    predictedPlugin: string | null;
    taskOptions: TaskOptions | null;
    primaryOverride: string;
    secondaryPassword: string;
    version: number;
    versionOptions: ContainerVersionInfo[];
    extraValues: Record<string, string>;
    filteredExtraFields: TaskField[];
    selectedPluginIndex: number;
    cipherMode: number;
    compressionMode: "none" | "zstd";
    onUpdateTaskType?: (v: string) => void;
    onUpdateSourcePath?: (v: string) => void;
    onUpdateTargetPath?: (v: string) => void;
    onUpdateVersion?: (v: number) => void;
    onUpdatePrimaryOverride?: (v: string) => void;
    onUpdateSecondaryPassword?: (v: string) => void;
    onUpdateCipherMode?: (v: number) => void;
    onUpdateCompressionMode?: (v: "none" | "zstd") => void;
    onUpdateExtraValue?: (payload: { key: string; value: string }) => void;
    onSelectPlugin?: (index: number) => void;
    onSubmit?: () => void;
  }>(),
  {
    state: undefined,
    onUpdateTaskType: undefined,
    onUpdateSourcePath: undefined,
    onUpdateTargetPath: undefined,
    onUpdateVersion: undefined,
    onUpdatePrimaryOverride: undefined,
    onUpdateSecondaryPassword: undefined,
    onUpdateCipherMode: undefined,
    onUpdateCompressionMode: undefined,
    onUpdateExtraValue: undefined,
    onSelectPlugin: undefined,
    onSubmit: undefined,
  }
);

const fallbackState = reactive<NewTaskState>({
  taskType: "encrypt",
  sourcePath: "",
  targetPath: "",
  candidates: [],
  predictedPlugin: null,
  taskOptions: null,
  primaryOverride: "",
  secondaryPassword: "",
  version: 4,
  versionOptions: [],
  extraValues: {},
  filteredExtraFields: [],
  selectedPluginIndex: 0,
  cipherMode: 0,
  compressionMode: "none",
});

const effectiveState = computed<NewTaskState>(() => {
  if (props.state) return props.state;
  return fallbackState;
});

const src = computed(() => effectiveState.value.sourcePath ?? props.sourcePath ?? "");
const taskType = computed(() => effectiveState.value.taskType ?? props.taskType ?? "encrypt");
const cands = computed<PluginCandidate[]>(() => {
  const arr = effectiveState.value.candidates ?? props.candidates;
  return Array.isArray(arr) ? arr : [];
});
const pluginName = computed(() => effectiveState.value.predictedPlugin ?? props.predictedPlugin ?? "");
const selectedIdx = computed(() =>
  typeof effectiveState.value.selectedPluginIndex === "number"
    ? effectiveState.value.selectedPluginIndex
    : typeof props.selectedPluginIndex === "number"
      ? props.selectedPluginIndex
      : 0
);
const taskOpts = computed(() => effectiveState.value.taskOptions ?? props.taskOptions ?? null);

const isPredicting = computed(() => {
  return src.value.length > 0 && cands.value.length === 0 && !pluginName.value;
});

function getMatchTypeLabel(matchType: string): string {
  switch (matchType) {
    case "mime":
      return "MIME";
    case "extension":
      return "Extension";
    case "general":
      return "General";
    case "container":
      return "Container";
    default:
      return matchType;
  }
}

async function handleClose() {
  await modalController.dismiss();
}
</script>

<style scoped>
.modal-header ion-toolbar {
  --padding-start: 8px;
  --padding-end: 4px;
}

.form-section {
  margin-bottom: 12px;
}

.field-group {
  position: relative;
  margin-bottom: 8px;
}

/* 插件区域 */
.plugin-section {
  margin: 10px 0;
  padding: 10px 14px;
  border-radius: 10px;
  background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.5);
  backdrop-filter: blur(var(--material-blur, 8px));
  -webkit-backdrop-filter: blur(var(--material-blur, 8px));
}

body.dark .plugin-section {
  background: rgba(30, 30, 30, 0.55);
}

.plugin-section.predicting {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px;
  justify-content: center;
}

.predict-spinner {
  width: 20px;
  height: 20px;
  --color: var(--ion-color-primary);
}

.predict-text {
  font-size: 13px;
  color: var(--ion-color-medium);
}

.plugin-section.multi-plugin {
  padding: 10px 14px;
}

.section-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--ion-color-medium-shade);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 6px;
}

.plugin-select {
  width: 100%;
  --padding-start: 0;
  max-width: none;
}

.plugin-section.single-plugin {
  display: flex;
  align-items: center;
  gap: 10px;
}

.plugin-check {
  font-size: 20px;
  flex-shrink: 0;
}

.plugin-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.plugin-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--ion-text-color);
}

.plugin-match-type {
  font-size: 11px;
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.1);
  padding: 1px 8px;
  border-radius: 10px;
  align-self: flex-start;
}

.match-type-badge {
  font-size: 10px;
  opacity: 0.7;
  margin-left: 6px;
}

.mt-mime { color: #3880ff; }
.mt-extension { color: #2dd36f; }
.mt-general { color: #ffc409; }
.mt-container { color: #eb445a; }

/* 密码策略提示 */
.plugin-hint {
  padding: 8px 14px;
  font-size: 12px;
  color: var(--ion-color-medium);
  background: rgba(var(--ion-color-primary-rgb), 0.06);
  border-radius: 8px;
  margin: 8px 0;
  border-left: 3px solid var(--ion-color-medium);
}

.plugin-hint.strategy-independent {
  border-left-color: var(--ion-color-primary);
  color: var(--ion-color-primary);
  font-weight: 500;
}

/* 提交按钮 */
.submit-btn {
  margin-top: 20px;
  --border-radius: 10px;
  height: 48px;
  font-weight: 600;
  letter-spacing: 0.3px;
}

.submit-btn:disabled {
  opacity: 0.5;
}
</style>
