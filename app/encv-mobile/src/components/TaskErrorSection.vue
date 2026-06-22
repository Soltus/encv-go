<script setup lang="ts">
/**
 * 🆕 2026-06-22 Q2B：完整错误详情区
 *
 * 取代旧的"只显示 task.error 字符串"。新功能：
 * - 错误分类 chip（useErrorAnalyzer 12 类）
 * - 修复建议列表
 * - phase 时间链（折叠展开）
 * - 结构化 errorDetail JSON（折叠展开）
 * - 复制错误按钮
 */
import { computed, ref } from 'vue'
import { useI18n } from '@/composables/useI18n'
import { analyzeError, type ErrorAnalysis } from '@/composables/useErrorAnalyzer'

interface Props {
  task: {
    id: string
    error?: string
    errorDetail?: string
  }
}

const props = defineProps<Props>()
const { t } = useI18n()

// 🆕 Q2B：优先解析 errorDetail JSON（后端 classifyError 输出）
// 兜底：解析 error 字符串
const errorAnalysis = computed<ErrorAnalysis>(() => {
  if (props.task.errorDetail) {
    try {
      const detail = JSON.parse(props.task.errorDetail)
      // 后端 JSON → ErrorAnalysis
      return analyzeError(detail.raw ?? props.task.error ?? '', {
        phase: detail.phase,
      })
    } catch {
      // JSON 解析失败（可能不是后端 JSON）→ 退回到原始字符串
      return analyzeError(props.task.errorDetail, {})
    }
  }
  if (props.task.error) {
    return analyzeError(props.task.error, {})
  }
  return {
    category: 'unknown',
    phase: 'unknown',
    summary: t('tasks.errorNoDetail') ?? 'No error detail available',
    technicalExplanation: '',
    chain: [],
    fixes: [],
  }
})

// 分类显示标签（按 ErrorCategory 走 i18n）
const categoryLabel = computed(() => {
  const cat = errorAnalysis.value.category
  const key = `tasks.error.category.${cat}` as const
  // 兜底：使用 summary 字段
  return t(key) !== key ? t(key) : errorAnalysis.value.summary
})

// phase 标签
const phaseLabel = computed(() => {
  const phase = errorAnalysis.value.phase
  return t(`tasks.error.phase.${phase}`) ?? phase
})

// 复制按钮
const copySuccess = ref(false)
async function copyError() {
  const text = JSON.stringify({
    taskId: props.task.id,
    error: props.task.error,
    errorDetail: props.task.errorDetail,
    analysis: errorAnalysis.value,
  }, null, 2)
  try {
    await navigator.clipboard.writeText(text)
    copySuccess.value = true
    setTimeout(() => (copySuccess.value = false), 2000)
  } catch (e) {
    // 兜底：旧浏览器或 Capacitor WebView clipboard 不可用
    console.warn('Failed to copy to clipboard', e)
  }
}

// 折叠展开
const showRawDetail = ref(false)
const showChain = ref(false)
</script>

<template>
  <div class="error-section">
    <div class="error-header">
      <div class="error-chips">
        <ion-chip class="error-chip-category" :color="errorAnalysis.category === 'unknown' ? 'medium' : 'danger'">
          <ion-icon :icon="errorAnalysis.category === 'unknown' ? 'help-circle-outline' : 'alert-circle'" />
          <ion-label>{{ categoryLabel }}</ion-label>
        </ion-chip>
        <ion-chip class="error-chip-phase">
          <ion-icon icon="git-branch-outline" />
          <ion-label>{{ phaseLabel }}</ion-label>
        </ion-chip>
      </div>
      <ion-button fill="clear" size="small" @click="copyError">
        <ion-icon :icon="copySuccess ? 'checkmark-circle' : 'copy-outline'" slot="start" />
        {{ copySuccess ? t('tasks.errorCopied') : t('tasks.errorCopy') }}
      </ion-button>
    </div>

    <p v-if="errorAnalysis.technicalExplanation" class="error-explanation">
      {{ errorAnalysis.technicalExplanation }}
    </p>

    <!-- 修复建议 -->
    <div v-if="errorAnalysis.fixes.length > 0" class="error-fixes">
      <h4 class="error-fixes-title">
        <ion-icon icon="construct-outline" />
        {{ t('tasks.errorFixesTitle') }}
      </h4>
      <ol class="error-fixes-list">
        <li v-for="(fix, idx) in errorAnalysis.fixes" :key="idx" class="error-fix-item">
          <strong>{{ fix.title }}</strong>
          <span v-if="fix.detail" class="error-fix-detail">{{ fix.detail }}</span>
        </li>
      </ol>
    </div>

    <!-- phase 时间链（折叠） -->
    <div v-if="errorAnalysis.chain.length > 0" class="error-chain">
      <button class="error-expand-toggle" @click="showChain = !showChain">
        <ion-icon :icon="showChain ? 'chevron-down' : 'chevron-forward'" />
        <span>{{ t('tasks.errorChainTitle') }} ({{ errorAnalysis.chain.length }})</span>
      </button>
      <ol v-if="showChain" class="error-chain-list">
        <li v-for="(step, idx) in errorAnalysis.chain" :key="idx" class="error-chain-step">
          <span class="error-chain-label">{{ step.title }}</span>
          <span v-if="step.detail" class="error-chain-detail">{{ step.detail }}</span>
        </li>
      </ol>
    </div>

    <!-- 原始 errorDetail JSON（折叠） -->
    <div v-if="task.errorDetail" class="error-raw">
      <button class="error-expand-toggle" @click="showRawDetail = !showRawDetail">
        <ion-icon :icon="showRawDetail ? 'chevron-down' : 'chevron-forward'" />
        <span>{{ t('tasks.errorRawTitle') }}</span>
      </button>
      <pre v-if="showRawDetail" class="error-raw-content">{{ task.errorDetail }}</pre>
    </div>
  </div>
</template>

<style scoped>
.error-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
  background: var(--ion-color-danger-tint, #fde7e9);
  border-radius: 8px;
  border-left: 4px solid var(--ion-color-danger, #eb445a);
}

.error-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.error-chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.error-chip-category,
.error-chip-phase {
  margin: 0;
  height: 28px;
  font-size: 12px;
}

.error-explanation {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--ion-color-dark, #222);
}

.error-fixes-title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 4px 0;
  font-size: 14px;
  font-weight: 600;
}

.error-fixes-list {
  margin: 0;
  padding-left: 24px;
  font-size: 13px;
  line-height: 1.6;
}

.error-fix-item {
  margin-bottom: 6px;
}

.error-fix-detail {
  display: block;
  margin-top: 2px;
  color: var(--ion-color-medium-shade, #666);
  font-size: 12px;
}

.error-chain,
.error-raw {
  border-top: 1px solid var(--ion-color-danger-shade, #c93545);
  padding-top: 8px;
}

.error-expand-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  background: transparent;
  border: none;
  padding: 0;
  font-size: 13px;
  font-weight: 500;
  color: var(--ion-color-primary, #3880ff);
  cursor: pointer;
}

.error-chain-list {
  margin: 6px 0 0 0;
  padding-left: 24px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ion-color-dark-shade, #333);
}

.error-chain-step {
  margin-bottom: 4px;
}

.error-chain-label {
  font-weight: 500;
}

.error-chain-detail {
  display: block;
  margin-top: 2px;
  color: var(--ion-color-medium-shade, #666);
  font-size: 11px;
  font-family: monospace;
}

.error-raw-content {
  margin: 6px 0 0 0;
  padding: 8px;
  background: rgba(0, 0, 0, 0.05);
  border-radius: 4px;
  font-size: 11px;
  line-height: 1.4;
  font-family: monospace;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow-y: auto;
}
</style>
