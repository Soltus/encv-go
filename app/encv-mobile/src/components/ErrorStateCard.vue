<template>
  <div class="error-state-card" :class="`error-state-card--${errorType}`">
    <!-- 头部：图标 + 标题 -->
    <div class="error-header">
      <div class="error-icon-wrap" :class="`error-icon-wrap--${errorType}`">
        <ion-icon :icon="iconForType" class="error-icon" />
      </div>
      <h2 class="error-title">{{ title }}</h2>
      <p v-if="subtitle" class="error-subtitle">{{ subtitle }}</p>
    </div>

    <!-- 主要操作按钮 -->
    <div class="error-actions">
      <ion-button
        v-if="canRetry"
        expand="block"
        color="primary"
        @click="$emit('retry')"
      >
        <ion-icon :icon="refresh" slot="start" />
        {{ retryText }}
      </ion-button>
      <ion-button
        v-if="canCopyDebug"
        expand="block"
        fill="outline"
        color="medium"
        @click="copyDebugInfo"
      >
        <ion-icon :icon="copyOutline" slot="start" />
        {{ copyText }}
      </ion-button>
    </div>

    <!-- 错误详情卡片（可折叠） -->
    <div v-if="details && details.length > 0" class="error-details ui-card">
      <button
        type="button"
        class="details-toggle"
        :aria-expanded="detailsExpanded"
        @click="detailsExpanded = !detailsExpanded"
      >
        <ion-icon
          :icon="detailsExpanded ? chevronDown : chevronForward"
          class="toggle-icon"
        />
        <span>{{ detailsExpanded ? '收起调试信息' : '查看调试信息' }}</span>
        <ion-badge color="medium" class="details-count">{{ details.length }}</ion-badge>
      </button>

      <div v-if="detailsExpanded" class="details-list">
        <div
          v-for="(item, idx) in details"
          :key="idx"
          class="detail-row"
        >
          <span class="detail-label">{{ item.label }}</span>
          <div class="detail-value-wrap">
            <code
              :class="['detail-value', item.copyable ? 'detail-value--copyable' : '']"
              :title="item.copyable ? '点击复制' : ''"
              @click="item.copyable && copyValue(item.value)"
            >{{ item.value }}</code>
            <ion-icon
              v-if="item.copyable"
              :icon="copyOutline"
              class="copy-icon"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- 底部建议（按错误类型） -->
    <div v-if="suggestion" class="error-suggestion">
      <ion-icon :icon="bulbOutline" class="suggestion-icon" />
      <p>{{ suggestion }}</p>
    </div>

    <!-- Toast 提示（短时显示在右下）-->
    <div v-if="toastVisible" class="copy-toast">
      <ion-icon :icon="checkmarkCircle" />
      <span>{{ toastText }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  alertCircleOutline,
  bulbOutline,
  checkmarkCircle,
  chevronDown,
  chevronForward,
  cloudOfflineOutline,
  copyOutline,
  documentOutline,
  helpCircleOutline,
  lockClosedOutline,
  refresh,
} from "ionicons/icons";
import { computed, ref } from "vue";

export type ErrorType = "network_error" | "gateway_error" | "format_error" | "init_failed" | "playback_failed" | "auth_error" | "unknown";

export interface ErrorDetailItem {
  label: string;
  value: string;
  copyable?: boolean;
}

interface Props {
  errorType: ErrorType;
  title: string;
  subtitle?: string;
  details?: ErrorDetailItem[];
  /** 显示重试按钮 */
  canRetry?: boolean;
  /** 显示"复制调试信息"按钮 */
  canCopyDebug?: boolean;
  /** 重试按钮文案 */
  retryText?: string;
  /** 复制按钮文案 */
  copyText?: string;
}

const props = withDefaults(defineProps<Props>(), {
  subtitle: undefined,
  details: () => [],
  canRetry: true,
  canCopyDebug: true,
  retryText: "重试",
  copyText: "复制调试信息",
});

defineEmits<(e: "retry") => void>();

const detailsExpanded = ref(false);
const toastVisible = ref(false);
const toastText = ref("");

/** 按错误类型映射 ICON + 主题色 + 建议文案 */
const config = computed(() => {
  switch (props.errorType) {
    case "network_error":
      return {
        icon: cloudOfflineOutline,
        suggestion:
          "网络无法到达服务器。请检查：\n• 手机/浏览器与 ENCV 后端的网络连通性\n• 切换 WiFi / 移动数据\n• 若使用 Trae 沙箱预览，可能受限于 401 鉴权",
      };
    case "gateway_error":
      return {
        icon: lockClosedOutline,
        suggestion: "网关层拦截（常见 Trae 沙箱 401）。预览模式可能在当前网络不可用。",
      };
    case "format_error":
      return {
        icon: documentOutline,
        suggestion: "媒体格式不受支持。推荐使用 MP4 / WebM 容器 + H.264 视频 + AAC 音频。",
      };
    case "init_failed":
      return {
        icon: alertCircleOutline,
        suggestion: "播放器初始化失败。可尝试重试，或刷新页面重新进入。",
      };
    case "auth_error":
      return {
        icon: lockClosedOutline,
        suggestion: "鉴权失败。请检查文件是否加密、密码是否正确。",
      };
    case "playback_failed":
      return {
        icon: alertCircleOutline,
        suggestion: "播放过程中出错。可尝试：\n• 重新进入页面\n• 检查容器是否损坏\n• 查看 DevLogs 中 [ArtPlayer] 详细日志",
      };
    default:
      return {
        icon: helpCircleOutline,
        suggestion: '发生未知错误。点击"查看调试信息"获取详情，或粘贴给开发者。',
      };
  }
});

const iconForType = computed(() => config.value.icon);
const suggestion = computed(() => config.value.suggestion);

/** 拼接所有 details 为单行 JSON，用于一键复制调试信息 */
function buildDebugInfo(): string {
  return JSON.stringify(
    {
      errorType: props.errorType,
      title: props.title,
      subtitle: props.subtitle,
      details: props.details,
      timestamp: new Date().toISOString(),
      userAgent: typeof navigator !== "undefined" ? navigator.userAgent : "(no navigator)",
    },
    null,
    2
  );
}

async function copyDebugInfo() {
  const text = buildDebugInfo();
  const ok = await copyToClipboard(text);
  showToast(ok ? "✓ 调试信息已复制" : "⚠ 复制失败，请手动选择");
}

async function copyValue(value: string) {
  const ok = await copyToClipboard(value);
  showToast(ok ? "✓ 已复制" : "⚠ 复制失败");
}

async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // 降级到 textarea + execCommand
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

let toastTimer: ReturnType<typeof setTimeout> | null = null;
function showToast(text: string) {
  toastText.value = text;
  toastVisible.value = true;
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    toastVisible.value = false;
  }, 1800);
}
</script>

<style scoped>
.error-state-card {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  width: 100%;
  max-width: 480px;
  margin: 0 auto;
  padding: 8px 0;
}

/* 头部 */
.error-header {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  margin-bottom: 24px;
}

.error-icon-wrap {
  width: 88px;
  height: 88px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 16px;
  background: color-mix(in srgb, var(--color-error) 12%, transparent);
  position: relative;
}

.error-icon-wrap::after {
  content: '';
  position: absolute;
  inset: -8px;
  border-radius: 50%;
  border: 2px solid color-mix(in srgb, var(--color-error) 18%, transparent);
  animation: pulse 2.4s ease-out infinite;
}

.error-icon-wrap--gateway_error,
.error-icon-wrap--auth_error {
  background: color-mix(in srgb, var(--color-warning) 12%, transparent);
}
.error-icon-wrap--gateway_error::after,
.error-icon-wrap--auth_error::after {
  border-color: color-mix(in srgb, var(--color-warning) 18%, transparent);
}

.error-icon-wrap--format_error {
  background: color-mix(in srgb, var(--color-base-content) 18%, var(--color-base-100));
}
.error-icon-wrap--format_error::after {
  border-color: color-mix(in srgb, var(--color-base-content) 18%, var(--color-base-100));
}

.error-icon {
  font-size: 48px;
  color: var(--color-error);
}

.error-icon-wrap--gateway_error .error-icon,
.error-icon-wrap--auth_error .error-icon {
  color: var(--color-warning);
}

.error-icon-wrap--format_error .error-icon {
  color: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 85%, var(--color-black));
}

.error-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--ion-text-color);
  margin: 0 0 4px;
  line-height: 1.4;
}

.error-subtitle {
  font-size: 13px;
  color: var(--encv-text-secondary, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)));
  margin: 0;
  line-height: 1.5;
}

/* 操作按钮 */
.error-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 24px;
}

.error-actions ion-button {
  --border-radius: 12px;
  font-weight: 500;
}

/* 详情列表 */
/* 表面（背景/圆角/描边）已上提到全局 .ui-card（默认对齐原外观，零回退）。
   本 scoped 仅保留 overflow:hidden（功能裁剪，非主题表面）。 */
.error-details {
  overflow: hidden;
}

.details-toggle {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  background: transparent;
  border: none;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  color: var(--ion-text-color);
  transition: background-color 0.15s ease;
}

.details-toggle:hover {
  background: rgba(0, 0, 0, 0.04);
}

.toggle-icon {
  font-size: 16px;
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
}

.details-count {
  margin-left: auto;
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 6px;
}

.details-list {
  padding: 4px 0 8px;
  max-height: 320px;
  overflow-y: auto;
}

.detail-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 14px;
  border-top: 1px solid var(--color-base-200);
}

.detail-label {
  flex: 0 0 80px;
  font-size: 11px;
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
  padding-top: 2px;
}

.detail-value-wrap {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  align-items: flex-start;
  gap: 6px;
}

.detail-value {
  flex: 1 1 auto;
  min-width: 0;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ion-text-color);
  background: var(--ion-background-color, var(--color-white));
  padding: 2px 6px;
  border-radius: 4px;
  word-break: break-all;
  display: block;
  border: 1px solid var(--color-base-200);
}

.detail-value--copyable {
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.detail-value--copyable:hover {
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-primary) 25%, transparent);
}

.copy-icon {
  font-size: 14px;
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
  margin-top: 4px;
  flex-shrink: 0;
}

/* 底部建议 */
.error-suggestion {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-top: 20px;
  padding: 12px 14px;
  background: color-mix(in srgb, var(--color-primary) 6%, transparent);
  border-radius: 10px;
  border-left: 3px solid var(--color-primary);
}

.suggestion-icon {
  font-size: 20px;
  color: var(--color-primary);
  flex-shrink: 0;
  margin-top: 2px;
}

.error-suggestion p {
  font-size: 12px;
  line-height: 1.6;
  color: var(--ion-text-color);
  margin: 0;
  white-space: pre-line;
  opacity: 0.85;
}

/* 复制 toast */
.copy-toast {
  position: fixed;
  bottom: 32px;
  left: 50%;
  transform: translateX(-50%);
  background: color-mix(in srgb, var(--color-base-content) 85%, transparent);
  color: var(--color-white);
  padding: 10px 18px;
  border-radius: 24px;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
  z-index: 9999;
  animation: toast-in 0.2s ease-out;
}

.copy-toast ion-icon {
  font-size: 18px;
  color: var(--color-success);
}

@keyframes pulse {
  0% { transform: scale(0.95); opacity: 0.6; }
  60% { transform: scale(1.1); opacity: 0; }
  100% { transform: scale(1.1); opacity: 0; }
}

@keyframes toast-in {
  from { transform: translate(-50%, 12px); opacity: 0; }
  to { transform: translate(-50%, 0); opacity: 1; }
}
</style>
