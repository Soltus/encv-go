<template>
  <div
    :class="[
      'utc',
      `utc--${entry.status}`,
      {
        'utc--current': entry.isCurrent,
        'utc--highlight': highlight,
      },
    ]"
  >
    <!-- 左侧时间线轴 -->
    <div class="utc__axis">
      <div class="utc__dot" />
      <div class="utc__line" />
    </div>

    <!-- 右侧卡片 -->
    <div class="utc__card">
      <!-- 头部行 -->
      <div
        class="utc__header"
        :class="{ 'utc__header--clickable': entry.hasExpandableDetail }"
        @click="toggleExpand"
      >
        <div class="utc__header-left">
          <slot name="icon" :entry="entry">
            <PhaseIcon :phase="entry.phase" />
          </slot>
          <span class="utc__label">{{ entry.label }}</span>
          <slot name="meta" :entry="entry">
            <span v-if="entry.meta" class="utc__meta">{{ entry.meta }}</span>
          </slot>
        </div>
        <div class="utc__header-right">
          <span
            v-if="entry.duration"
            class="utc__duration"
            :class="{ 'utc__duration--highlight': highlight }"
          >{{ entry.duration }}</span>
          <span v-if="entry.time" class="utc__time">{{ entry.time }}</span>
          <ion-icon
            v-if="entry.hasExpandableDetail"
            :icon="isExpanded ? chevronUp : chevronDown"
            class="utc__chevron"
          />
        </div>
      </div>

      <!-- 进度条 + 速率 + ETA -->
      <div
        v-if="entry.progress != null || entry.speed || entry.eta"
        class="utc__metrics"
      >
        <div v-if="entry.progress != null" class="utc__progress-wrap">
          <div class="utc__progress-bar">
            <div
              class="utc__progress-fill"
              :style="{ width: `${entry.progress}%` }"
            />
          </div>
          <span class="utc__progress-text">{{ entry.progress }}%</span>
        </div>
        <span v-if="entry.speed" class="utc__metric">⚡ {{ entry.speed }}</span>
        <span v-if="entry.eta" class="utc__metric">⏳ {{ entry.eta }}</span>
      </div>

      <!-- 错误提示（始终显示，作为快速预览） -->
      <div v-if="entry.expandDetail?.error" class="utc__error-hint">
        <ion-icon :icon="alertCircleOutline" />
        <span>{{ entry.expandDetail.error }}</span>
      </div>

      <!-- 展开详情（卡片化） -->
      <div
        v-if="isExpanded && entry.hasExpandableDetail"
        class="utc__detail"
      >
        <slot name="detail" :entry="entry">
          <!-- 默认 detail 渲染：startedAt / completedAt / duration / outputPath / error / extra -->
          <div v-if="entry.expandDetail?.startedAt" class="utc__detail-card">
            <div class="utc__detail-label">开始时间</div>
            <div class="utc__detail-value">{{ entry.expandDetail.startedAt }}</div>
          </div>
          <div v-if="entry.expandDetail?.completedAt" class="utc__detail-card">
            <div class="utc__detail-label">完成时间</div>
            <div class="utc__detail-value">{{ entry.expandDetail.completedAt }}</div>
          </div>
          <div v-if="entry.expandDetail?.duration" class="utc__detail-card">
            <div class="utc__detail-label">耗时</div>
            <div class="utc__detail-value">{{ entry.expandDetail.duration }}</div>
          </div>
          <div v-if="entry.expandDetail?.outputPath" class="utc__detail-card">
            <div class="utc__detail-label">输出路径</div>
            <div class="utc__detail-value utc__detail-value--mono">{{ entry.expandDetail.outputPath }}</div>
          </div>
          <div
            v-if="entry.expandDetail?.error"
            class="utc__detail-card utc__detail-card--error"
          >
            <div class="utc__detail-label">错误信息</div>
            <div class="utc__detail-value utc__detail-value--mono">{{ entry.expandDetail.error }}</div>
          </div>
          <template v-if="entry.expandDetail?.extra">
            <div
              v-for="(value, key) in entry.expandDetail.extra"
              :key="key"
              class="utc__detail-card"
            >
              <div class="utc__detail-label">{{ key }}</div>
              <div class="utc__detail-value">{{ value }}</div>
            </div>
          </template>
        </slot>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { IonIcon } from '@ionic/vue'
import { chevronUp, chevronDown, alertCircleOutline } from 'ionicons/icons'
import PhaseIcon from './PhaseIcon.vue'
import type { UnifiedTimelineEntry } from '@/lib/workflow/types'

/**
 * 通用时间线卡片组件
 *
 * 作为任务时间线（TaskTimeline）和 FFMPEG 日志条目（MockGenLogCard）的共用骨架。
 * 调用方负责从 StepRun / MockGenLogEntry 等领域模型转换为 UnifiedTimelineEntry。
 *
 * 展开状态支持两种模式：
 * - 受控模式：父组件传 v-model:expanded，由父组件管理状态
 * - 非受控模式：父组件不传 expanded，组件内部管理（可通过 defaultExpanded 设置初始值）
 */
const props = withDefaults(defineProps<{
  entry: UnifiedTimelineEntry
  /** 受控展开状态（可选，未传时组件内部自管理） */
  expanded?: boolean
  /** 非受控模式下的默认展开状态 */
  defaultExpanded?: boolean
  /** 是否高亮（如最长耗时） */
  highlight?: boolean
}>(), {
  // 显式设为 undefined，覆盖 Vue 3 对可选 boolean prop 默认 false 的类型转换
  // 这样 expanded 未传时为 undefined，组件进入非受控模式
  expanded: undefined,
  defaultExpanded: false,
  highlight: false,
})

const emit = defineEmits<{
  (e: 'update:expanded', value: boolean): void
  (e: 'toggle', value: boolean): void
}>()

// 内部展开状态（仅非受控模式使用）
const internalExpanded = ref(props.defaultExpanded)

// 实际展开状态：受控模式优先用 prop，非受控模式用内部状态
const isExpanded = computed(() =>
  props.expanded !== undefined ? props.expanded : internalExpanded.value,
)

function toggleExpand() {
  if (!props.entry.hasExpandableDetail) return
  const newValue = !isExpanded.value
  // 非受控模式下更新内部状态
  if (props.expanded === undefined) {
    internalExpanded.value = newValue
  }
  emit('update:expanded', newValue)
  emit('toggle', newValue)
}
</script>

<style scoped>
.utc {
  display: flex;
  align-items: stretch;
  gap: 0;
  position: relative;
}

/* ==================== 左侧时间线轴 ==================== */
.utc__axis {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex-shrink: 0;
  width: 16px;
  padding-top: 14px;
}

.utc__dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--ion-color-step-300, #b2b2b2);
  border: 2px solid var(--ion-color-step-300, #b2b2b2);
  flex-shrink: 0;
  z-index: 1;
}

.utc__line {
  flex: 1;
  width: 2px;
  background: var(--ion-color-step-200, #d3d3d3);
  margin-top: 2px;
  min-height: 8px;
}

/* 最后一个条目隐藏连接线（由父组件通过 :last-child 控制） */
.utc:last-child .utc__line {
  display: none;
}

/* ==================== 右侧卡片 ==================== */
.utc__card {
  flex: 1;
  min-width: 0;
  background: var(--ion-card-background, #ffffff);
  border-radius: 8px;
  padding: 12px;
  margin-left: 8px;
  margin-bottom: 8px;
  border-left: 4px solid var(--ion-color-step-300, #b2b2b2);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  transition: box-shadow 0.2s ease, border-color 0.2s ease;
}

/* ==================== 状态色左边框 ==================== */
.utc--success .utc__card {
  border-left-color: var(--ion-color-success, #2dd36f);
}
.utc--success .utc__dot {
  background: var(--ion-color-success, #2dd36f);
  border-color: var(--ion-color-success, #2dd36f);
}

.utc--failure .utc__card {
  border-left-color: var(--ion-color-danger, #eb445a);
}
.utc--failure .utc__dot {
  background: var(--ion-color-danger, #eb445a);
  border-color: var(--ion-color-danger, #eb445a);
}

.utc--running .utc__card {
  border-left-color: var(--ion-color-primary, #4f8cff);
}
.utc--running .utc__dot {
  background: var(--ion-color-primary, #4f8cff);
  border-color: var(--ion-color-primary, #4f8cff);
  animation: utc-pulse 1.5s ease-in-out infinite;
}

.utc--cancelled .utc__card,
.utc--skipped .utc__card {
  border-left-color: var(--ion-color-medium, #92949c);
}
.utc--cancelled .utc__dot,
.utc--skipped .utc__dot {
  background: var(--ion-color-medium, #92949c);
  border-color: var(--ion-color-medium, #92949c);
}

.utc--timed_out .utc__card {
  border-left-color: var(--ion-color-warning, #ffc409);
}
.utc--timed_out .utc__dot {
  background: var(--ion-color-warning, #ffc409);
  border-color: var(--ion-color-warning, #ffc409);
}

.utc--queued .utc__card,
.utc--submitted .utc__card,
.utc--pending .utc__card {
  border-left-color: var(--ion-color-step-300, #b2b2b2);
}

@keyframes utc-pulse {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(var(--ion-color-primary-rgb, 79, 140, 255), 0.4);
  }
  50% {
    box-shadow: 0 0 0 5px rgba(var(--ion-color-primary-rgb, 79, 140, 255), 0.1);
  }
}

/* ==================== current 状态高亮 ==================== */
.utc--current .utc__card {
  box-shadow: 0 0 0 2px rgba(var(--ion-color-primary-rgb, 79, 140, 255), 0.25),
    0 1px 3px rgba(0, 0, 0, 0.04);
}

/* ==================== highlight 状态（最长耗时等） ==================== */
.utc--highlight .utc__card {
  background: rgba(var(--ion-color-warning-rgb, 255, 196, 9), 0.06);
}

.utc__duration--highlight {
  font-weight: 700;
  color: var(--ion-color-warning, #ffc409);
}

/* ==================== 头部行 ==================== */
.utc__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  min-height: 24px;
}

.utc__header--clickable {
  cursor: pointer;
}

.utc__header-left {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  flex: 1;
}

.utc__label {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-text-color, #000);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.utc__meta {
  font-size: 11px;
  color: var(--ion-color-medium, #92949c);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 1;
}

.utc__header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.utc__duration {
  font-size: 11px;
  font-weight: 500;
  color: var(--ion-color-medium, #92949c);
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  white-space: nowrap;
}

.utc__time {
  font-size: 11px;
  color: var(--ion-color-medium, #92949c);
  white-space: nowrap;
}

.utc__chevron {
  font-size: 14px;
  color: var(--ion-color-medium, #92949c);
  flex-shrink: 0;
  transition: transform 0.2s ease;
}

/* ==================== 进度条 + 速率 + ETA ==================== */
.utc__metrics {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
  flex-wrap: wrap;
}

.utc__progress-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 100px;
}

.utc__progress-bar {
  flex: 1;
  height: 4px;
  background: var(--ion-color-step-200, #d3d3d3);
  border-radius: 2px;
  overflow: hidden;
}

.utc__progress-fill {
  height: 100%;
  background: linear-gradient(
    90deg,
    var(--ion-color-primary, #4f8cff),
    var(--ion-color-primary-shade, #3a78e0)
  );
  border-radius: 2px;
  transition: width 0.3s ease;
}

.utc__progress-text {
  font-size: 10px;
  font-weight: 600;
  color: var(--ion-color-medium, #92949c);
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  white-space: nowrap;
}

.utc__metric {
  font-size: 10px;
  color: var(--ion-color-medium, #92949c);
  white-space: nowrap;
}

/* ==================== 错误提示 ==================== */
.utc__error-hint {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
  padding: 4px 8px;
  background: rgba(var(--ion-color-danger-rgb, 235, 68, 90), 0.08);
  border-radius: 4px;
  font-size: 11px;
  color: var(--ion-color-danger, #eb445a);
}

.utc__error-hint ion-icon {
  font-size: 12px;
  flex-shrink: 0;
}

.utc__error-hint span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ==================== 展开详情（卡片化） ==================== */
.utc__detail {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 8px;
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--ion-color-step-100, #ececec);
}

.utc__detail-card {
  background: var(--ion-color-step-50, #f7f7f7);
  border-radius: 6px;
  padding: 6px 8px;
  min-width: 0;
}

.utc__detail-card--error {
  background: rgba(var(--ion-color-danger-rgb, 235, 68, 90), 0.08);
  grid-column: 1 / -1;
}

.utc__detail-label {
  font-size: 10px;
  color: var(--ion-color-medium, #92949c);
  margin-bottom: 2px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.utc__detail-value {
  font-size: 12px;
  color: var(--ion-text-color, #000);
  font-weight: 500;
  word-break: break-all;
}

.utc__detail-value--mono {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
}

/* ==================== 展开动画（CSS animation，避免 Vue <transition> 在 jsdom 中不触发 transitionend） ==================== */
.utc__detail {
  animation: utc-detail-enter 0.2s ease;
}

@keyframes utc-detail-enter {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* ==================== 暗黑模式适配（body.dark） ==================== */
:global(body.dark) .utc__card {
  background: rgba(255, 255, 255, 0.04);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
}

:global(body.dark) .utc--current .utc__card {
  box-shadow: 0 0 0 2px rgba(var(--ion-color-primary-rgb, 79, 140, 255), 0.35),
    0 1px 3px rgba(0, 0, 0, 0.2);
}

:global(body.dark) .utc--highlight .utc__card {
  background: rgba(var(--ion-color-warning-rgb, 255, 196, 9), 0.1);
}

:global(body.dark) .utc__label {
  color: rgba(255, 255, 255, 0.92);
}

:global(body.dark) .utc__detail-card {
  background: rgba(255, 255, 255, 0.06);
}

:global(body.dark) .utc__detail-value {
  color: rgba(255, 255, 255, 0.88);
}

:global(body.dark) .utc__detail {
  border-top-color: rgba(255, 255, 255, 0.08);
}

:global(body.dark) .utc__progress-bar {
  background: rgba(255, 255, 255, 0.12);
}
</style>
