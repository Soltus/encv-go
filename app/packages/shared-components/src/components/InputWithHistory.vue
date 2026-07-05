<template>
  <div class="input-with-history" :class="{ 'history-open': showHistory && entries.length > 0, 'has-error': !!errorText }">
    <ion-item class="input-row" :class="{ 'field-modified': isCustomized, 'ion-invalid': !!errorText, 'ion-touched': !!errorText }">
      <ion-icon v-if="icon" :icon="icon" slot="start"></ion-icon>
      <ion-input
        :model-value="modelValue"
        @ionInput="handleInput"
        @ionBlur="handleBlur"
        @ionFocus="handleFocus"
        @keyup.enter="handleEnter"
        :label="label"
        label-placement="stacked"
        :placeholder="placeholder"
        :type="resolvedType"
        :clear-input="clearInput"
        :error-text="errorText"
        :disabled="disabled"
      ></ion-input>
      <ion-button v-if="inputType === 'password'" slot="end" fill="clear" class="aux-btn" @click="togglePassword">
        <ion-icon :icon="showPassword ? eyeOffOutline : eyeOutline" slot="icon-only"></ion-icon>
      </ion-button>
      <ion-button v-else-if="historyKey" slot="end" fill="clear" class="aux-btn" @click="showHistory = !showHistory">
        <ion-icon :icon="showHistory ? chevronUpOutline : chevronDownOutline" slot="icon-only"></ion-icon>
      </ion-button>
      <ion-button v-if="browsable" slot="end" fill="clear" class="aux-btn" @click="$emit('browse')">
        <ion-icon :icon="folderOpen" slot="icon-only"></ion-icon>
      </ion-button>
      <ion-button v-if="isCustomized" slot="end" fill="clear" size="small" class="reset-btn" @click="$emit('reset')">
        <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
      </ion-button>
    </ion-item>
    <div v-if="showHistory && entries.length > 0" class="history-panel">
      <div class="history-header">
        <span class="history-title">{{ t('inputHistory.recent') }}</span>
        <ion-button fill="clear" size="small" @click="handleClear">{{ t('inputHistory.clear') }}</ion-button>
      </div>
      <div class="history-list">
        <div
          v-for="(entry, idx) in entries"
          :key="`${entry.timestamp}-${idx}`"
          class="history-item"
          @click="handleSelect(entry.value)"
        >
          <ion-icon :icon="timeOutline" class="history-icon"></ion-icon>
          <span class="history-value">{{ entry.value }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { clearHistory, getHistory, recordHistory } from "@encv/shared-components/composables/useInputHistory";
import { computed, ref } from "vue";

const props = defineProps<{
  modelValue: string;
  label: string;
  placeholder?: string;
  icon?: string | { name: string; ios: string; md: string };
  inputType?: "text" | "password" | "email" | "number";
  historyKey?: string;
  browsable?: boolean;
  clearInput?: boolean;
  isCustomized?: boolean;
  errorText?: string;
  disabled?: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  browse: [];
  reset: [];
  "commit-history": [value: string];
  "keyup-enter": [];
  blur: [];
}>();

const { t } = useI18n();
const showPassword = ref(false);
const showHistory = ref(false);

const entries = computed(() => (props.historyKey ? getHistory(props.historyKey) : []));

const _resolvedType = computed(() => {
  if (props.inputType !== "password") return props.inputType || "text";
  return showPassword.value ? "text" : "password";
});

function _handleInput(e: CustomEvent) {
  // 防御：ionInput 事件必须是 CustomEvent 携带 detail.value
  // 但代码路径中可能存在非 CustomEvent 派发（如测试代码 dispatchEvent(new Event('ionInput'))），
  // 这种情况下 e.detail 是 undefined，原代码会抛 "Cannot read properties of undefined"
  const detail: any = (e as any)?.detail;
  const raw = typeof detail?.value === "string" || typeof detail?.value === "number" ? detail.value : "";
  emit("update:modelValue", raw);
}

function _handleFocus() {
  if (props.historyKey && entries.value.length > 0) {
    showHistory.value = true;
  }
}

function _handleBlur() {
  if (props.historyKey) {
    recordHistory(props.historyKey, props.modelValue);
  }
  setTimeout(() => {
    showHistory.value = false;
  }, 150);
  // 关键：暴露 blur 事件给父组件——父组件可借此自动保存
  // 场景：用户修改 API Key 后不按 Enter 就离开 input → blur 时自动加密保存
  emit("blur");
}

function _handleSelect(value: string) {
  emit("update:modelValue", value);
  emit("commit-history", value);
  recordHistory(props.historyKey!, value);
  showHistory.value = false;
}

function _handleClear() {
  if (props.historyKey) clearHistory(props.historyKey);
  showHistory.value = false;
}

function _togglePassword() {
  showPassword.value = !showPassword.value;
}

function _handleEnter() {
  emit("keyup-enter");
}
</script>

<style scoped>
.input-with-history {
  position: relative;
}

.input-row {
  --min-height: 56px;
}

.input-row.field-modified {
  border-left: 3px solid var(--ion-color-primary);
  --padding-start: 13px;
}

.aux-btn {
  --padding-start: 6px;
  --padding-end: 6px;
  min-width: 40px;
  min-height: 40px;
  margin: 0;
}

.reset-btn {
  --padding-start: 4px;
  --padding-end: 4px;
  min-width: 28px;
  min-height: 28px;
  margin: 0;
}

.reset-btn ion-icon {
  font-size: 16px;
}

.history-panel {
  background: var(--ion-color-light, #f4f5f8);
  border-radius: 8px;
  margin: 0 12px 8px 12px;
  overflow: hidden;
  animation: slide-down 0.2s ease-out;
}

body.dark .history-panel {
  background: var(--ion-background-color, #1f1f21);
  border: 1px solid #2a2a2c;
}

.history-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  font-size: 12px;
  color: var(--ion-color-medium);
  border-bottom: 1px solid var(--ion-color-light-shade, #e0e0e0);
}

body.dark .history-header {
  border-bottom-color: #2a2a2c;
}

.history-title {
  font-weight: 600;
}

.history-list {
  max-height: 200px;
  overflow-y: auto;
}

.history-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  cursor: pointer;
  border-bottom: 1px solid var(--ion-color-light-shade, #e0e0e0);
  transition: background 0.15s;
}

.history-item:last-child {
  border-bottom: none;
}

.history-item:hover,
.history-item:active {
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}

body.dark .history-item {
  border-bottom-color: #2a2a2c;
}

.history-icon {
  font-size: 14px;
  color: var(--ion-color-medium);
  flex-shrink: 0;
}

.history-value {
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: monospace;
}

@keyframes slide-down {
  from {
    opacity: 0;
    transform: translateY(-8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
