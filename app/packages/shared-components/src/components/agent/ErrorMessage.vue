<!--
  ErrorMessage - 每条消息独立的错误块
  紧跟在出错的 user/assistant 消息下方显示
  支持 onRetry 回调（点击重试按钮时触发）
-->
<template>
  <div class="errorMessage">
    <MessageAuthor :icon="icon" :label="label" :variant="'error'" />
    <div class="errorMessageBody">
      <pre class="errorText">{{ text }}</pre>
      <button v-if="onRetry" type="button" class="errorRetryBtn" @click="onRetry">
        <ion-icon :icon="refreshIcon" />
        <span>重试</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { alertCircleOutline, refreshOutline } from "ionicons/icons";

defineProps<{
  text: string;
  onRetry?: () => void;
}>();

const _icon = alertCircleOutline;
const _refreshIcon = refreshOutline;
const _label = "出错";
</script>

<style scoped>
.errorMessage {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 0;
  max-width: 100%;
}

.errorMessageBody {
  padding-left: 30px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.errorText {
  margin: 0;
  padding: 8px 12px;
  background: var(--error-bg, rgba(239, 68, 68, 0.08));
  border: 1px solid var(--error-border, rgba(239, 68, 68, 0.25));
  border-radius: 6px;
  color: var(--ion-color-danger-shade, #c1272d);
  font-size: 12.5px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  white-space: pre-wrap;
  word-break: break-word;
}

.errorRetryBtn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  border: 1px solid var(--error-border, rgba(239, 68, 68, 0.25));
  border-radius: 6px;
  background: transparent;
  color: var(--ion-color-danger, #ef4444);
  font-size: 11.5px;
  cursor: pointer;
  font-family: inherit;
}
.errorRetryBtn:hover {
  background: var(--error-bg, rgba(239, 68, 68, 0.08));
}
</style>
