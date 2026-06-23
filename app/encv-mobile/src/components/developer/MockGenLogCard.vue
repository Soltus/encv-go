<template>
  <div v-if="log.length > 0" class="mock-gen-log-card">
    <!-- 头部：标题 + 计数 + 复制按钮 -->
    <div class="mock-gen-log-header">
      <div class="mock-gen-log-title">
        <ion-icon :icon="terminalOutline" color="primary"></ion-icon>
        <span>FFMPEG 流程日志</span>
        <span class="mock-gen-log-count">{{ log.length }} / {{ summary?.total ?? log.length }}</span>
      </div>
      <button
        class="mock-gen-log-copy"
        :class="{ 'mock-gen-log-copy--copied': copied }"
        @click="emit('copy')"
        :aria-label="copied ? '已复制' : '复制全部日志'"
      >
        <ion-icon :icon="copied ? checkmarkCircleOutline : copyOutline" slot="icon-only"></ion-icon>
        <span>{{ copied ? '已复制' : '复制全部' }}</span>
      </button>
    </div>

    <!-- 汇总信息：ok / failed / skipped / disconnected -->
    <div class="mock-gen-log-summary" v-if="summary">
      <ion-icon
        :icon="summary.failed > 0 ? warningOutline : checkmarkCircleOutline"
        :color="summary.failed > 0 ? 'warning' : 'success'"
      ></ion-icon>
      <span>{{ summaryText }}</span>
      <span v-if="summary.disconnected" class="mock-gen-log-disconnect">
        <ion-icon :icon="warningOutline" class="mock-gen-log-disconnect-icon"></ion-icon>
        <span>后端连接已断开 — 下面 {{ log.length }} 行是「处理到这步」</span>
      </span>
    </div>

    <!-- 日志列表：使用 UnifiedTimelineCard 渲染每条 entry -->
    <div class="mock-gen-log-list">
      <UnifiedTimelineCard
        v-for="entry in log"
        :key="entry.key"
        :entry="toUnifiedTimelineEntry(entry)"
        :expanded="entry.expanded"
        @toggle="onToggle(entry.key, $event)"
      >
        <!-- 自定义 icon slot：runner 图标（flash=mediacodec / cog=ffmpeg / document=static） -->
        <template #icon>
          <span class="mock-gen-log-runner" :class="`mock-gen-log-runner--${entry.runner}`">
            <ion-icon :icon="runnerIcon(entry.runner)" />
          </span>
        </template>

        <!-- 自定义 meta slot：[index/total] · encoder · exit code -->
        <template #meta>
          <span class="mock-gen-log-idx">[{{ entry.index }}/{{ entry.total }}]</span>
          <span class="mock-gen-log-encoder">{{ entry.encoder }}</span>
          <span v-if="entry.status === 'failed'" class="mock-gen-log-exitcode">
            exit={{ entry.exitCode }}
          </span>
        </template>

        <!-- 自定义 detail slot：ffmpeg args / stderr / context 卡片化 -->
        <template #detail>
          <div v-if="entry.ffmpegArgs.length > 0" class="mock-gen-log-detail-card">
            <div class="mock-gen-log-detail-label">FFMPEG Args</div>
            <pre class="mock-gen-log-detail-value">{{ entry.ffmpegArgs.join(' ') }}</pre>
          </div>
          <div v-else class="mock-gen-log-detail-card">
            <div class="mock-gen-log-detail-label">FFMPEG Args</div>
            <pre class="mock-gen-log-detail-value">(静态字节 - 无 ffmpeg 调用)</pre>
          </div>

          <div v-if="entry.stderr" class="mock-gen-log-detail-card mock-gen-log-detail-card--stderr">
            <div class="mock-gen-log-detail-label">STDERR</div>
            <pre class="mock-gen-log-detail-value">{{ entry.stderr }}</pre>
          </div>

          <div v-if="entry.workerTmpDir" class="mock-gen-log-detail-card">
            <div class="mock-gen-log-detail-label">Worker Tmp Dir</div>
            <div class="mock-gen-log-detail-value">{{ entry.workerTmpDir }}</div>
          </div>

          <div v-if="entry.workerError" class="mock-gen-log-detail-card mock-gen-log-detail-card--error">
            <div class="mock-gen-log-detail-label">Worker Error</div>
            <div class="mock-gen-log-detail-value">{{ entry.workerError }}</div>
          </div>

          <div v-if="entry.contextInfo" class="mock-gen-log-detail-card">
            <div class="mock-gen-log-detail-label">Context</div>
            <pre class="mock-gen-log-detail-value">{{ entry.contextInfo }}</pre>
          </div>

          <div
            v-if="entry.srcSize !== undefined || entry.dstSize !== undefined"
            class="mock-gen-log-detail-card"
          >
            <div class="mock-gen-log-detail-label">File Sizes</div>
            <div class="mock-gen-log-detail-value">
              src={{ entry.srcSize ?? 0 }} bytes, dst={{ entry.dstSize ?? 0 }} bytes
            </div>
          </div>
        </template>
      </UnifiedTimelineCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IonIcon } from '@ionic/vue'
import {
  terminalOutline,
  copyOutline,
  checkmarkCircleOutline,
  warningOutline,
  flashOutline,
  settingsOutline,
  documentTextOutline,
} from 'ionicons/icons'
import UnifiedTimelineCard from '@/components/shared/UnifiedTimelineCard.vue'
import { Phase, type UnifiedTimelineEntry, type StepStatus } from '@/lib/workflow/types'
import type { MockGenLogEntry, MockGenLogSummary } from '@/composables/useMockGenLog'
// v3 2026-06-18：FFMPEG 日志时间格式化（避免 ISO 字符串撑满宽度导致溢出）
import { formatDateTime } from '@/composables/useDateFormat'

/**
 * MockGenLogCard — FFMPEG 流程日志卡（Task 13 SubTask 13.2/13.3）
 *
 * 使用 UnifiedTimelineCard 作为骨架，把 MockGenLogEntry 转换为 UnifiedTimelineEntry。
 * 转换规则：
 *   - id: entry.key
 *   - phase: Phase.Completed（FFMPEG 日志无 phase 概念，用 Completed 作为默认）
 *   - label: entry.relativePath
 *   - status: 'ok' → 'success' / 'failed' → 'failure' / 'pending' → 'running'
 *   - time: entry.at
 *   - hasExpandableDetail: true（所有条目都可展开看 ffmpeg args / stderr）
 *   - icon slot: runner ion-icon（flash=mediacodec / settings=ffmpeg / document=static）
 *   - meta slot: [index/total] · encoder · exit code
 *   - detail slot: ffmpeg args / stderr / context 卡片化
 */
const props = defineProps<{
  /** 日志条目列表 */
  log: MockGenLogEntry[]
  /** 汇总信息（null 时不渲染汇总行） */
  summary: MockGenLogSummary | null
  /** 是否已复制（控制复制按钮状态） */
  copied?: boolean
}>()

const emit = defineEmits<{
  (e: 'toggle', key: string): void
  (e: 'copy'): void
}>()

/** runner 标识 → ion-icon */
function runnerIcon(runner: string) {
  if (runner === 'mediacodec') return flashOutline
  if (runner === 'static') return documentTextOutline
  return settingsOutline // ffmpeg / default
}

/** MockGenLogEntry.status → StepStatus 映射 */
const STATUS_MAP: Record<MockGenLogEntry['status'], StepStatus> = {
  ok: 'success',
  failed: 'failure',
  pending: 'running',
}

/**
 * MockGenLogEntry → UnifiedTimelineEntry 转换器
 *
 * FFMPEG 日志无 phase 概念，统一用 Phase.Completed 作为默认值
 * （UnifiedTimelineCard 的 PhaseIcon 会渲染 checkmarkCircleOutline，
 *   但本组件用 #icon slot 覆盖为 runner ion-icon，phase 仅影响状态色边框）
 */
function toUnifiedTimelineEntry(entry: MockGenLogEntry): UnifiedTimelineEntry {
  return {
    id: entry.key,
    phase: Phase.Completed,
    label: entry.relativePath,
    // v3 2026-06-18：格式化 ISO 时间戳为可读形式（避免 24 字符 ISO 字符串撑满宽度）
    time: formatDateTime(entry.at),
    status: STATUS_MAP[entry.status],
    hasExpandableDetail: true,
    expandDetail: {
      // error 字段：失败时把 stderr 作为快速预览（UnifiedTimelineCard 会渲染 error-hint）
      ...(entry.status === 'failed' && entry.stderr
        ? { error: entry.stderr.split('\n')[0] }
        : {}),
    },
  }
}

/** 汇总文本（保持与原 PluginTestsDetail.vue 一致的格式） */
const summaryText = computed(() => {
  if (!props.summary) return ''
  const { ok, failed, skipped } = props.summary
  let text = `${ok} ✓ / ${failed} ✗ / ${skipped} ◌`
  if (props.summary.disconnected) {
    text = `${text}（流中断于 ${props.log.length}/${props.summary.total}）`
  }
  return text
})

/** UnifiedTimelineCard 的 toggle 事件转发为 toggle(key) */
function onToggle(key: string, _value: boolean): void {
  emit('toggle', key)
}
</script>

<style scoped>
/* ========== FFMPEG 流程日志卡（用 design token 双主题适配） ========== */
.mock-gen-log-card {
  margin: 8px 16px 12px;
  padding: 12px 14px;
  background: linear-gradient(180deg, var(--tl-card-bg-gradient-start) 0%, var(--tl-card-bg-gradient-end) 100%);
  border-radius: var(--tl-card-radius);
  border: 1px solid var(--tl-card-border);
  font-family: var(--tl-card-font-mono);
  color: var(--tl-card-text-primary);
}

.mock-gen-log-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--tl-card-border);
}
.mock-gen-log-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--tl-card-text-primary);
}
.mock-gen-log-title ion-icon {
  font-size: 14px;
}
.mock-gen-log-count {
  color: var(--tl-card-text-tertiary);
  font-size: 11px;
  margin-left: 4px;
}
.mock-gen-log-copy {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--tl-card-border);
  color: var(--tl-card-text-primary);
  border: 1px solid var(--tl-card-border-strong);
  border-radius: var(--tl-card-radius-sm);
  padding: 3px 8px;
  font-size: 11px;
  font-family: inherit;
  cursor: pointer;
  transition: all 0.15s;
}
.mock-gen-log-copy:hover {
  background: var(--tl-card-border-strong);
}
.mock-gen-log-copy ion-icon {
  font-size: 12px;
}
.mock-gen-log-copy--copied {
  background: rgba(var(--tl-state-completed-rgb), 0.15);
  color: var(--tl-state-completed);
  border-color: rgba(var(--tl-state-completed-rgb), 0.3);
}

.mock-gen-log-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 0;
  font-size: 12px;
  color: var(--tl-card-text-secondary);
  flex-wrap: wrap;
}
.mock-gen-log-summary ion-icon {
  font-size: 14px;
}
.mock-gen-log-disconnect {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--tl-state-preprocessing);
  font-weight: 600;
}
.mock-gen-log-disconnect-icon {
  font-size: 13px;
}

.mock-gen-log-list {
  margin-top: 4px;
}

/* ========== runner 标识（icon slot 内） ========== */
.mock-gen-log-runner {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: var(--tl-card-radius-sm);
  font-size: 12px;
  font-weight: bold;
  flex-shrink: 0;
}
.mock-gen-log-runner ion-icon {
  font-size: 13px;
}
.mock-gen-log-runner--ffmpeg {
  background: rgba(var(--tl-state-packing-rgb), 0.15);
  color: var(--tl-state-packing);
}
.mock-gen-log-runner--mediacodec {
  background: rgba(var(--tl-state-completed-rgb), 0.15);
  color: var(--tl-state-completed);
}
.mock-gen-log-runner--static {
  background: rgba(var(--tl-state-created-rgb), 0.15);
  color: var(--tl-state-created);
}

/* ========== meta slot 内的元素 ========== */
.mock-gen-log-idx {
  color: var(--tl-card-text-tertiary);
  font-weight: 600;
  font-size: 11px;
}
.mock-gen-log-encoder {
  color: var(--tl-state-packing);
  font-size: 10.5px;
}
.mock-gen-log-exitcode {
  color: var(--tl-state-failed);
  font-weight: 600;
  font-size: 10.5px;
}

/* ========== detail slot 内的卡片 ========== */
.mock-gen-log-detail-card {
  background: var(--tl-card-border);
  border-radius: var(--tl-card-radius-sm);
  padding: 6px 8px;
  min-width: 0;
}
.mock-gen-log-detail-card--stderr {
  grid-column: 1 / -1;
}
.mock-gen-log-detail-card--error {
  background: rgba(var(--tl-state-failed-rgb), 0.12);
  grid-column: 1 / -1;
}
.mock-gen-log-detail-label {
  font-size: 10px;
  color: var(--tl-state-analyzing);
  font-weight: 600;
  margin-bottom: 2px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.mock-gen-log-detail-value {
  font-size: 11px;
  color: var(--tl-card-text-primary);
  font-family: var(--tl-card-font-mono);
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  max-height: 200px;
  overflow-y: auto;
}
</style>
