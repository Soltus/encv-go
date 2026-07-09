<script setup lang="ts">
/**
 * 🆕 2026-06-22 v2：极简错误详情
 *
 * 设计：
 * - 没 error/errorDetail 整个组件不渲染（v-if）
 * - 默认只显示一行：标题（user-facing summary）+ 错误分类 chip
 * - "详情" 按钮点击展开：技术说明 + 修复建议 + 原始错误
 * - 不再用刺眼红色背景 / border-left 装饰
 */

import { analyzeError, type ErrorAnalysis } from "@/composables/useErrorAnalyzer";
import { useI18n } from "@/composables/useI18n";
import { computed, ref } from "vue";

interface Props {
  task: {
    id: string;
    error?: string;
    errorDetail?: string;
  };
}

const props = defineProps<Props>();
const { t } = useI18n();

// 🆕 v2：整个组件在没错误时不渲染（避免 UI 永远显示 chip）
const hasError = computed(() => Boolean(props.task.error || props.task.errorDetail));

const errorAnalysis = computed<ErrorAnalysis | null>(() => {
  if (!hasError.value) return null;
  if (props.task.errorDetail) {
    try {
      const detail = JSON.parse(props.task.errorDetail);
      return analyzeError(detail.raw ?? props.task.error ?? "", {
        phase: detail.phase,
      });
    } catch {
      return analyzeError(props.task.errorDetail, {});
    }
  }
  if (props.task.error) {
    return analyzeError(props.task.error, {});
  }
  return null;
});

const categoryLabel = computed(() => {
  const cat = errorAnalysis.value?.category ?? "unknown";
  const key = `tasks.error.category.${cat}` as const;
  return t(key) !== key ? t(key) : (errorAnalysis.value?.summary ?? cat);
});

const copySuccess = ref(false);
const expanded = ref(false);
async function copyError() {
  if (!errorAnalysis.value) return;
  const text = JSON.stringify(
    {
      taskId: props.task.id,
      error: props.task.error,
      errorDetail: props.task.errorDetail,
    },
    null,
    2
  );
  try {
    await navigator.clipboard.writeText(text);
    copySuccess.value = true;
    setTimeout(() => (copySuccess.value = false), 2000);
  } catch (e) {
    console.warn("Failed to copy to clipboard", e);
  }
}
</script>

<template>
  <div v-if="hasError && errorAnalysis" class="error-section">
    <div class="error-summary">
      <ion-chip class="error-chip" :color="errorAnalysis.category === 'unknown' ? 'medium' : 'danger'">
        <ion-icon :icon="errorAnalysis.category === 'unknown' ? 'help-circle-outline' : 'alert-circle-outline'" />
        <ion-label>{{ categoryLabel }}</ion-label>
      </ion-chip>
      <ion-button v-if="errorAnalysis.fixes.length > 0 || errorAnalysis.technicalExplanation" fill="clear" size="small" @click="expanded = !expanded">
        <ion-icon :icon="expanded ? 'chevron-up' : 'chevron-down'" slot="end" />
        {{ expanded ? t('tasks.errorHideDetail') : t('tasks.errorShowDetail') }}
      </ion-button>
    </div>

    <div v-if="expanded" class="error-detail">
      <p v-if="errorAnalysis.technicalExplanation" class="error-explanation">
        {{ errorAnalysis.technicalExplanation }}
      </p>

      <ol v-if="errorAnalysis.fixes.length > 0" class="error-fixes">
        <li v-for="(fix, idx) in errorAnalysis.fixes" :key="idx" class="error-fix">
          <strong>{{ fix.title }}</strong>
          <span v-if="fix.detail">{{ fix.detail }}</span>
        </li>
      </ol>

      <details v-if="task.errorDetail" class="error-raw">
        <summary>{{ t('tasks.errorRawTitle') }}</summary>
        <pre>{{ task.errorDetail }}</pre>
      </details>

      <ion-button fill="clear" size="small" @click="copyError">
        <ion-icon :icon="copySuccess ? 'checkmark-circle-outline' : 'copy-outline'" slot="start" />
        {{ copySuccess ? t('tasks.errorCopied') : t('tasks.errorCopy') }}
      </ion-button>
    </div>
  </div>
</template>

<style scoped>
.error-section {
  margin-top: 8px;
}

.error-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.error-chip {
  margin: 0;
  height: 24px;
  font-size: 12px;
}

.error-detail {
  margin-top: 8px;
  padding: 8px 12px;
  background: var(--ion-color-light, #f4f5f8);
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.error-explanation {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--ion-color-dark, #222);
}

.error-fixes {
  margin: 0;
  padding-left: 20px;
  font-size: 13px;
  line-height: 1.6;
}

.error-fix {
  margin-bottom: 4px;
}

.error-fix span {
  display: block;
  margin-top: 2px;
  color: var(--ion-color-medium-shade, #666);
  font-size: 12px;
}

.error-raw {
  font-size: 12px;
}

.error-raw summary {
  cursor: pointer;
  color: var(--ion-color-primary, #3880ff);
  user-select: none;
}

.error-raw pre {
  margin: 4px 0 0 0;
  padding: 6px;
  background: rgba(0, 0, 0, 0.05);
  border-radius: 4px;
  font-size: 11px;
  line-height: 1.4;
  font-family: monospace;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 160px;
  overflow-y: auto;
}
</style>
