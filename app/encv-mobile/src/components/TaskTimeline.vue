<template>
  <div class="detail-section">
    <div class="section-title">{{ t('tasks.timeline') }}</div>
    <div class="timeline">
      <UnifiedTimelineCard
        v-for="entry in unifiedEntries"
        :key="entry.id"
        :entry="entry"
        :highlight="entry.isHighlight === true"
        v-model:expanded="expandedMap[entry.id]"
      >
        <!-- 自定义 detail slot：卡片化展开（输出路径 / 开始 / 完成 / 耗时） -->
        <template #detail="{ entry: e }">
          <div v-if="e.expandDetail?.outputPath" class="timeline-detail-card timeline-detail-card--path">
            <div class="timeline-detail-label">{{ t('tasks.outputFile') }}</div>
            <div class="timeline-detail-value timeline-detail-value--mono">{{ e.expandDetail.outputPath }}</div>
          </div>
          <div v-if="e.expandDetail?.startedAt" class="timeline-detail-card">
            <div class="timeline-detail-label">{{ t('tasks.startedAt') }}</div>
            <div class="timeline-detail-value">{{ e.expandDetail.startedAt }}</div>
          </div>
          <div v-if="e.expandDetail?.completedAt" class="timeline-detail-card">
            <div class="timeline-detail-label">{{ t('tasks.completedAt') }}</div>
            <div class="timeline-detail-value">{{ e.expandDetail.completedAt }}</div>
          </div>
          <div
            v-if="e.expandDetail?.duration"
            class="timeline-detail-card"
            :class="{ 'timeline-detail-card--highlight': e.isHighlight }"
          >
            <div class="timeline-detail-label">{{ t('tasks.duration') }}</div>
            <div class="timeline-detail-value">{{ e.expandDetail.duration }}</div>
          </div>
          <div
            v-if="e.expandDetail?.error"
            class="timeline-detail-card timeline-detail-card--error"
          >
            <div class="timeline-detail-label">{{ t('tasks.error') }}</div>
            <div class="timeline-detail-value timeline-detail-value--mono">{{ e.expandDetail.error }}</div>
          </div>
        </template>
      </UnifiedTimelineCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import { useI18n } from '@/composables/useI18n'
import { formatDateTime, formatDuration } from '@/composables/useDateFormat'
import type { EncvTask } from '@/api/encv'
import UnifiedTimelineCard from '@/components/shared/UnifiedTimelineCard.vue'
import {
  Phase,
  type StepStatus,
  type UnifiedTimelineEntry,
} from '@/lib/workflow/types'

const props = defineProps<{ task: EncvTask }>()
const { t } = useI18n()

// 展开状态映射：entry.id → 是否展开（受控模式）
const expandedMap = reactive<Record<string, boolean>>({})

// ==================== Phase 映射表 ====================

/** 裸 phase 字符串 → Phase 枚举（兼容后端推送的字符串值） */
const PHASE_MAP: Record<string, Phase> = {
  created: Phase.Created,
  analyzing: Phase.Analyzing,
  initializing: Phase.Initializing,
  preprocessing: Phase.Preprocessing,
  encrypting: Phase.Encrypting,
  decrypting: Phase.Decrypting,
  packing: Phase.Packing,
  verifying: Phase.Verifying,
  completed: Phase.Completed,
  // 兼容旧值
  done: Phase.Completed,
}

/** Phase 枚举 → i18n key（替代旧版 switch + 裸字符串） */
const PHASE_I18N_KEY: Record<Phase, string> = {
  [Phase.Created]: 'tasks.timelineCreated',
  [Phase.Analyzing]: 'tasks.phaseAnalyzing',
  [Phase.Initializing]: 'tasks.phaseInitializing',
  [Phase.Preprocessing]: 'tasks.phasePreprocessing',
  [Phase.Encrypting]: 'tasks.phaseEncrypting',
  [Phase.Decrypting]: 'tasks.phaseDecrypting',
  [Phase.Packing]: 'tasks.phasePacking',
  [Phase.Verifying]: 'tasks.phaseVerifying',
  [Phase.Completed]: 'tasks.phaseCompleted',
}

/** 根据 phase 字符串获取 i18n 标签 */
function getPhaseLabel(phase: string): string {
  const phaseEnum = PHASE_MAP[phase]
  if (!phaseEnum) return phase
  return t(PHASE_I18N_KEY[phaseEnum])
}

/** 把 phase 字符串转为 Phase 枚举（未知值降级为 Created） */
function toPhase(phase: string | undefined | null): Phase {
  if (!phase) return Phase.Created
  return PHASE_MAP[phase] ?? Phase.Created
}

// ==================== 内部构建类型 ====================

/**
 * 内部条目（带原始耗时毫秒数，用于计算最长耗时高亮）
 * 不暴露给 UnifiedTimelineCard，仅用于 computed 内部排序
 */
interface InternalTimelineEntry extends UnifiedTimelineEntry {
  _durationMs?: number
}

// ==================== 耗时计算 ====================

/** 计算两个 ISO 时间字符串之间的毫秒数（无效返回 0） */
function calcDurationMs(startedAt?: string, completedAt?: string): number {
  if (!startedAt || !completedAt) return 0
  const start = new Date(startedAt).getTime()
  const end = new Date(completedAt).getTime()
  if (isNaN(start) || isNaN(end) || end < start) return 0
  return end - start
}

// ==================== 时间线构建 ====================

// 🆕 2026-06-18 Task 18：crypto params 摘要（显示在 "created" 条目的 meta slot）
// 返回 "AES-256 · zstd" / "AES-128" / "zstd" / ""（旧任务无 crypto 字段时返回空串）
function getCryptoSummary(): string {
  const task = props.task
  const parts: string[] = []
  if (task.cipherMode !== undefined && task.cipherMode !== null) {
    parts.push(task.cipherMode === 1 ? 'AES-256' : 'AES-128')
  }
  if (task.compressionMode === 'zstd') {
    parts.push('Zstd')
  } else if (task.compressionMode === 'none') {
    parts.push('none')
  }
  return parts.join(' · ')
}

const unifiedEntries = computed<UnifiedTimelineEntry[]>(() => {
  const entries: InternalTimelineEntry[] = []
  const steps = props.task.steps ?? []
  const isTerminal = ['completed', 'failed', 'cancelled'].includes(props.task.status)

  // 1. 始终推送 "created" 事件
  // 🆕 Task 18：meta 字段显示 crypto params 摘要（折叠态可见）
  const cryptoMeta = getCryptoSummary()
  entries.push({
    id: `created-${props.task.createdAt}`,
    phase: Phase.Created,
    label: t('tasks.timelineCreated'),
    time: formatDateTime(props.task.createdAt),
    status: 'success',
    isCurrent: false,
    hasExpandableDetail: false,
    meta: cryptoMeta || undefined,
  })

  // 2. 从 task.steps 派生（如果存在）
  if (steps.length > 0) {
    for (const step of steps) {
      const phaseEnum = toPhase(step.phase)
      const isCurrent = step.phase === props.task.phase && !step.completedAt && props.task.status === 'running'
      const completed = !!step.completedAt
      const durationMs = calcDurationMs(step.startedAt, step.completedAt)
      const durationStr = completed && durationMs > 0 ? formatDuration(durationMs) : undefined

      // 状态映射：current → running, completed → success, 否则 pending
      let status: StepStatus
      if (isCurrent) {
        status = 'running'
      } else if (completed) {
        status = 'success'
      } else {
        status = 'pending'
      }

      // 派生 progress / speed / eta（仅当前 step 从 task 级别字段继承）
      const progress = isCurrent ? props.task.progress : undefined
      const speed = isCurrent ? props.task.speed : undefined
      const eta = isCurrent ? props.task.eta : undefined

      // 时间显示：进行中显示"进行中..."；完成显示耗时；否则空
      const timeStr = isCurrent
        ? t('tasks.timelineInProgress')
        : completed
          ? durationStr ?? t('tasks.timelineDone')
          : ''

      // 展开详情
      const expandDetail: UnifiedTimelineEntry['expandDetail'] = {}
      let hasExpand = false
      if (step.startedAt) {
        expandDetail.startedAt = formatDateTime(step.startedAt)
        hasExpand = true
      }
      if (step.completedAt) {
        expandDetail.completedAt = formatDateTime(step.completedAt)
        hasExpand = true
      }
      if (durationStr) {
        expandDetail.duration = durationStr
        hasExpand = true
      }
      if (step.detail) {
        expandDetail.outputPath = step.detail
        hasExpand = true
      }

      entries.push({
        id: `${step.phase}-${step.startedAt}`,
        phase: phaseEnum,
        label: getPhaseLabel(step.phase),
        time: timeStr,
        duration: durationStr,
        progress,
        speed,
        eta,
        status,
        isCurrent,
        hasExpandableDetail: hasExpand,
        expandDetail: hasExpand ? expandDetail : undefined,
        _durationMs: durationMs,
      })
    }
  } else {
    // 3. fallback：task.steps 为空时从 phase 序列派生（保留旧版行为）
    const phases = ['analyzing', 'initializing', 'preprocessing', 'encrypting', 'decrypting', 'packing', 'verifying']
    const phaseOrder = phases.indexOf(props.task.phase ?? '')

    for (let i = 0; i < phases.length; i++) {
      const p = phases[i]
      const isCurrent = p === props.task.phase && props.task.status === 'running'
      const isPast = !isCurrent && (phaseOrder > i || isTerminal)

      let status: StepStatus
      if (isCurrent) {
        status = 'running'
      } else if (isPast) {
        status = 'success'
      } else {
        status = 'pending'
      }

      entries.push({
        id: `${p}-fallback-${i}`,
        phase: toPhase(p),
        label: getPhaseLabel(p),
        time: isCurrent ? t('tasks.timelineInProgress') : isPast ? t('tasks.timelineDone') : '',
        progress: isCurrent ? props.task.progress : undefined,
        speed: isCurrent ? props.task.speed : undefined,
        eta: isCurrent ? props.task.eta : undefined,
        status,
        isCurrent,
        hasExpandableDetail: false,
      })
    }
  }

  // 4. 完成态：追加 "completed" 事件
  if (props.task.status === 'completed') {
    entries.push({
      id: `completed-${props.task.completedAt ?? ''}`,
      phase: Phase.Completed,
      label: t('tasks.phaseCompleted'),
      time: props.task.completedAt ? formatDateTime(props.task.completedAt) : '',
      status: 'success',
      isCurrent: false,
      hasExpandableDetail: false,
    })
  }

  // 5. 失败 / 取消态：标记最后一个事件为 failure，并附加错误信息
  if (props.task.status === 'failed' || props.task.status === 'cancelled') {
    const last = entries[entries.length - 1]
    if (last) {
      last.status = 'failure'
      last.label = props.task.status === 'failed' ? t('tasks.failed') : t('tasks.cancelled')
      if (props.task.error) {
        last.hasExpandableDetail = true
        last.expandDetail = {
          ...(last.expandDetail ?? {}),
          error: props.task.error,
        }
      }
    }
  }

  // 6. 计算最长耗时 phase 并高亮
  let maxDurationMs = 0
  let maxEntryId: string | null = null
  for (const entry of entries) {
    if (entry._durationMs && entry._durationMs > maxDurationMs) {
      maxDurationMs = entry._durationMs
      maxEntryId = entry.id
    }
  }
  if (maxEntryId) {
    const maxEntry = entries.find((e) => e.id === maxEntryId)
    if (maxEntry) maxEntry.isHighlight = true
  }

  // 7. 剥离内部字段，返回 UnifiedTimelineEntry[]
  return entries.map((entry) => {
    const { _durationMs: _ignored, ...rest } = entry
    void _ignored
    return rest
  })
})
</script>

<style scoped>
.detail-section {
  margin-bottom: 20px;
}

.section-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--ion-text-color);
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.timeline {
  position: relative;
  padding-left: 4px;
}

/* ==================== 自定义 detail slot 卡片样式 ==================== */
/* 覆盖 UnifiedTimelineCard 默认 detail slot 的网格布局，使用更紧凑的卡片 */
.timeline-detail-card {
  background: var(--tl-card-border);
  border-radius: var(--tl-card-radius-sm);
  padding: 6px 8px;
  min-width: 0;
}

.timeline-detail-card--path {
  grid-column: 1 / -1;
}

.timeline-detail-card--highlight {
  background: rgba(var(--tl-state-preprocessing-rgb), 0.1);
  border: 1px solid rgba(var(--tl-state-preprocessing-rgb), 0.3);
}

.timeline-detail-card--error {
  background: rgba(var(--tl-state-failed-rgb), 0.08);
  grid-column: 1 / -1;
}

.timeline-detail-label {
  font-size: 10px;
  color: var(--tl-card-text-tertiary);
  margin-bottom: 2px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.timeline-detail-value {
  font-size: 12px;
  color: var(--tl-card-text-primary);
  font-weight: 500;
  word-break: break-all;
}

.timeline-detail-value--mono {
  font-family: var(--tl-card-font-mono);
  font-size: 11px;
}
</style>
