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
        ⚠ 后端连接已断开 — 下面 {{ log.length }} 行是「处理到这步」
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
        <!-- 自定义 icon slot：runner 图标（⚡mediacodec / ⚙ffmpeg / 📄static） -->
        <template #icon>
          <span class="mock-gen-log-runner" :class="`mock-gen-log-runner--${entry.runner}`">
            {{ runnerEmoji(entry.runner) }}
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
} from 'ionicons/icons'
import UnifiedTimelineCard from '@/components/shared/UnifiedTimelineCard.vue'
import { Phase, type UnifiedTimelineEntry, type StepStatus } from '@/lib/workflow/types'
import type { MockGenLogEntry, MockGenLogSummary } from '@/composables/useMockGenLog'

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
 *   - icon slot: runner emoji（⚡mediacodec / ⚙ffmpeg / 📄static）
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

/** runner 标识 → emoji 字符 */
function runnerEmoji(runner: string): string {
  if (runner === 'mediacodec') return '⚡'
  if (runner === 'static') return '📄'
  return '⚙'
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
 *   但本组件用 #icon slot 覆盖为 runner emoji，phase 仅影响状态色边框）
 */
function toUnifiedTimelineEntry(entry: MockGenLogEntry): UnifiedTimelineEntry {
  return {
    id: entry.key,
    phase: Phase.Completed,
    label: entry.relativePath,
    time: entry.at,
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
/* ========== FFMPEG 流程日志卡（迁移自 PluginTestsDetail.vue 1390-1506 行） ========== */
.mock-gen-log-card {
  margin: 8px 16px 12px;
  padding: 12px 14px;
  background: linear-gradient(180deg, #0F1419 0%, #0A0E12 100%);
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  color: #E0E0E0;
}

.mock-gen-log-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
.mock-gen-log-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: #F4EFE6;
}
.mock-gen-log-title ion-icon {
  font-size: 14px;
}
.mock-gen-log-count {
  color: #6B7280;
  font-size: 11px;
  margin-left: 4px;
}
.mock-gen-log-copy {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: rgba(255, 255, 255, 0.05);
  color: #E0E0E0;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 4px;
  padding: 3px 8px;
  font-size: 11px;
  font-family: inherit;
  cursor: pointer;
  transition: all 0.15s;
}
.mock-gen-log-copy:hover {
  background: rgba(255, 255, 255, 0.1);
}
.mock-gen-log-copy ion-icon {
  font-size: 12px;
}
.mock-gen-log-copy--copied {
  background: rgba(34, 197, 94, 0.15);
  color: #4ADE80;
  border-color: rgba(34, 197, 94, 0.3);
}

.mock-gen-log-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 0;
  font-size: 12px;
  color: #9CA3AF;
  flex-wrap: wrap;
}
.mock-gen-log-summary ion-icon {
  font-size: 14px;
}
.mock-gen-log-disconnect {
  color: #F59E0B;
  font-weight: 600;
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
  border-radius: 4px;
  font-size: 12px;
  font-weight: bold;
  flex-shrink: 0;
}
.mock-gen-log-runner--ffmpeg {
  background: rgba(139, 92, 246, 0.15);
  color: #8B5CF6;
}
.mock-gen-log-runner--mediacodec {
  background: rgba(34, 197, 94, 0.15);
  color: #22C55E;
}
.mock-gen-log-runner--static {
  background: rgba(100, 116, 139, 0.15);
  color: #64748B;
}

/* ========== meta slot 内的元素 ========== */
.mock-gen-log-idx {
  color: #6B7280;
  font-weight: 600;
  font-size: 11px;
}
.mock-gen-log-encoder {
  color: #8B5CF6;
  font-size: 10.5px;
}
.mock-gen-log-exitcode {
  color: #FCA5A5;
  font-weight: 600;
  font-size: 10.5px;
}

/* ========== detail slot 内的卡片 ========== */
.mock-gen-log-detail-card {
  background: rgba(0, 0, 0, 0.3);
  border-radius: 4px;
  padding: 6px 8px;
  min-width: 0;
}
.mock-gen-log-detail-card--stderr {
  grid-column: 1 / -1;
}
.mock-gen-log-detail-card--error {
  background: rgba(220, 38, 38, 0.12);
  grid-column: 1 / -1;
}
.mock-gen-log-detail-label {
  font-size: 10px;
  color: #58A6FF;
  font-weight: 600;
  margin-bottom: 2px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.mock-gen-log-detail-value {
  font-size: 11px;
  color: #C9D1D9;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  max-height: 200px;
  overflow-y: auto;
}

/* ========== 暗黑模式适配（body.dark class） ========== */
:global(body.dark) .mock-gen-log-card {
  background: linear-gradient(180deg, #1A1F24 0%, #131719 100%);
  border-color: rgba(255, 255, 255, 0.1);
}
:global(body.dark) .mock-gen-log-title {
  color: #F4EFE6;
}
:global(body.dark) .mock-gen-log-detail-card {
  background: rgba(255, 255, 255, 0.04);
}
:global(body.dark) .mock-gen-log-detail-value {
  color: rgba(255, 255, 255, 0.88);
}
</style>
