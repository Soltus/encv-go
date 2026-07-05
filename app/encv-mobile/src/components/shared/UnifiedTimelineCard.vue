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
    <!-- 左侧时间线轴：状态 icon bubble + 连续连接线 -->
    <div class="utc__axis">
      <div :class="['utc__icon-bubble', `utc__icon-bubble--${entry.status}`]">
        <ion-spinner
          v-if="isRunningLike"
          name="dots"
          class="utc__icon utc__icon--spin"
        />
        <ion-icon
          v-else
          :icon="statusIcon"
          class="utc__icon"
        />
      </div>
      <div class="utc__line" />
    </div>

    <!-- 右侧卡片（左侧 4px 状态色边） -->
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
        <span v-if="entry.speed" class="utc__metric">
          <ion-icon :icon="flashOutline" class="utc__metric-icon" />
          {{ entry.speed }}
        </span>
        <span v-if="entry.eta" class="utc__metric">
          <ion-icon :icon="hourglassOutline" class="utc__metric-icon" />
          {{ entry.eta }}
        </span>
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
import { ban, checkmarkCircle, closeCircle, ellipseOutline, hourglass, syncOutline, timerOutline } from "ionicons/icons";
import { computed, ref } from "vue";
import type { StepStatus, UnifiedTimelineEntry } from "@/lib/workflow/types";

/**
 * 🆕 v4 2026-06-18 M4：行业标准时间线视觉
 *
 * 设计原则（用户原话）：
 *   - 垂直（不是水平 — 水平展示不了）
 *   - 不能有中断间隔（条目间连续连接，无 margin 隔开）
 *   - 整体铺满宽度（不是传统时间线左侧只占 16-32px 浪费）
 *   - 左侧 4px 边框色高亮（不是顶部 2px 色条）
 *   - 状态图标（不是纯色圆点）：
 *     · success   → 绿色对勾 checkmarkCircle（静态）
 *     · running   → 黄色转圈 ion-spinner dots（持续旋转）
 *     · failure   → 红色叉号 closeCircle
 *     · cancelled → 灰色禁止 ban
 *     · pending   → 灰色空心圆 ellipseOutline
 *     · queued    → 灰色排队 syncOutline
 *     · timed_out → 黄色时钟 hourglass
 *     · skipped   → 灰色 removeCircle
 *     · default   → 灰色 playCircle
 *
 * 视觉对齐目标：GitHub Actions workflow runs / Linear activity / Datadog logs / Vercel
 * 部署时间线。
 */
const props = withDefaults(
  defineProps<{
    entry: UnifiedTimelineEntry;
    /** 受控展开状态（可选，未传时组件内部自管理） */
    expanded?: boolean;
    /** 非受控模式下的默认展开状态 */
    defaultExpanded?: boolean;
    /** 是否高亮（如最长耗时） */
    highlight?: boolean;
  }>(),
  {
    // 显式设为 undefined，覆盖 Vue 3 对可选 boolean prop 默认 false 的类型转换
    // 这样 expanded 未传时为 undefined，组件进入非受控模式
    expanded: undefined,
    defaultExpanded: false,
    highlight: false,
  }
);

const emit = defineEmits<{
  (e: "update:expanded", value: boolean): void;
  (e: "toggle", value: boolean): void;
}>();

// 内部展开状态（仅非受控模式使用）
const internalExpanded = ref(props.defaultExpanded);

// 实际展开状态：受控模式优先用 prop，非受控模式用内部状态
const isExpanded = computed(() => (props.expanded !== undefined ? props.expanded : internalExpanded.value));

function _toggleExpand() {
  if (!props.entry.hasExpandableDetail) return;
  const newValue = !isExpanded.value;
  // 非受控模式下更新内部状态
  if (props.expanded === undefined) {
    internalExpanded.value = newValue;
  }
  emit("update:expanded", newValue);
  emit("toggle", newValue);
}

// ==================== 🆕 v4 M4：状态图标 + 颜色映射 ====================

/** 是否「running-like」（用旋转 spinner 替代静态 icon） */
const _isRunningLike = computed(() => {
  const s: StepStatus = props.entry.status;
  return s === "running" || s === "cancelling";
});

/**
 * 状态 → ionicon 名称
 * 注意：running 走 ion-spinner 路径（isRunningLike），不进这里
 */
const _statusIcon = computed(() => {
  const s: StepStatus = props.entry.status;
  switch (s) {
    case "success":
      return checkmarkCircle;
    case "failure":
      return closeCircle;
    case "cancelled":
    case "skipped":
      return ban;
    case "pending":
      return ellipseOutline;
    case "queued":
    case "submitted":
      return syncOutline;
    case "timed_out":
      return hourglass;
    default:
      return timerOutline;
  }
});
</script>

<style scoped>
.utc {
  display: flex;
  align-items: stretch;
  gap: 0;
  position: relative;
  /* 🆕 v4 M4：去掉行间 margin，让条目连续铺满（无中断间隔） */
  margin: 0;
  padding: 0;
}

/* ==================== 🆕 v4 M4：左侧时间线轴 ==================== */
/* 整条竖线贯穿所有 entry，由每个 entry 自己的 .utc__line 片段拼接 */
.utc__axis {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex-shrink: 0;
  width: 28px;  /* 🆕 加宽到 28px 容纳 icon bubble + 1px 居中连接线 */
  padding: 8px 0 0;
  position: relative;
}

/* 状态 icon bubble（替代旧版纯色圆点） */
.utc__icon-bubble {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  z-index: 2;
  background: var(--tl-state-created);
  color: #fff;
  box-shadow: 0 0 0 3px var(--tl-card-bg-gradient-start, #FAFBFC);
  transition: transform 0.2s ease;
}

.utc__icon {
  font-size: 14px;
  line-height: 1;
  color: #fff;
}
.utc__icon--spin {
  width: 14px;
  height: 14px;
}

/* 各状态颜色 — 与 design token 对齐 */
.utc__icon-bubble--success { background: var(--tl-state-completed); }
.utc__icon-bubble--failure { background: var(--tl-state-failed); }
.utc__icon-bubble--running {
  background: var(--tl-state-analyzing);
  /* 黄色 / 蓝色光晕脉冲（提示活跃） */
  animation: utc-running-pulse 1.4s ease-in-out infinite;
}
.utc__icon-bubble--cancelling {
  background: var(--tl-state-failed);
  animation: utc-running-pulse 1.4s ease-in-out infinite;
}
.utc__icon-bubble--cancelled { background: var(--tl-state-cancelled); }
.utc__icon-bubble--skipped { background: var(--tl-state-cancelled); }
.utc__icon-bubble--timed_out { background: var(--tl-state-preprocessing); }
.utc__icon-bubble--pending,
.utc__icon-bubble--queued,
.utc__icon-bubble--submitted {
  background: var(--tl-state-created);
}

@keyframes utc-running-pulse {
  0%, 100% { box-shadow: 0 0 0 3px var(--tl-card-bg-gradient-start, #FAFBFC),
                        0 0 0 5px rgba(var(--tl-state-analyzing-rgb), 0); }
  50% { box-shadow: 0 0 0 3px var(--tl-card-bg-gradient-start, #FAFBFC),
                0 0 0 8px rgba(var(--tl-state-analyzing-rgb), 0.25); }
}

/* 连接线：贯穿整个 entry，延伸到下一 entry（视觉上连续） */
.utc__line {
  flex: 1;
  width: 2px;
  background: var(--tl-card-border-strong);
  margin-top: 2px;
  min-height: 0;  /* 允许收缩，最后一个由 :last-child 隐藏 */
  /* 让连续线视觉上不被 icon 切断 */
  position: relative;
  z-index: 1;
}

/* 最后一个条目隐藏连接线 */
.utc:last-child .utc__line {
  display: none;
}

/* ==================== 🆕 v4 M4：右侧卡片（左侧 4px 状态色边） ====================
 * 关键改动：
 *   1. 顶部 2px 渐变条 → 左侧 4px 实色边（贴轴，铺满高度）
 *   2. 去掉 margin-bottom: 8px（消除「中断间隔」）
 *   3. 圆角收紧到左上 0（与 icon 气泡视觉衔接）
 */
.utc__card {
  flex: 1;
  min-width: 0;
  background: linear-gradient(
    180deg,
    var(--tl-card-bg-gradient-start),
    var(--tl-card-bg-gradient-end)
  );
  border-radius: 0 var(--tl-card-radius) var(--tl-card-radius) 0;
  border: 1px solid var(--tl-card-border);
  border-left: none;
  padding: 10px 12px 10px 14px;
  margin: 0 0 0 4px;  /* 🆕 左 4px gap（让 icon bubble 留呼吸空间），右下 0（与下条连接线对齐） */
  box-shadow: var(--tl-shadow-card);
  transition: box-shadow 0.2s ease;
  position: relative;
  overflow: hidden;
}

/* 左侧 4px 状态色边（顶部 2px → 改左侧 4px 贴轴） */
.utc__card::before {
  content: '';
  position: absolute;
  left: -1px;  /* 抵消 card 左边框 1px */
  top: -1px;
  bottom: -1px;
  width: 4px;
  background: var(--tl-state-created);
}

/* ==================== 状态色（左侧 4px 边 + icon bubble） ==================== */
.utc--success .utc__card::before { background: var(--tl-state-completed); }
.utc--success .utc__icon-bubble { background: var(--tl-state-completed); }

.utc--failure .utc__card::before { background: var(--tl-state-failed); }
.utc--failure .utc__icon-bubble { background: var(--tl-state-failed); }

.utc--running .utc__card::before {
  background: linear-gradient(
    180deg,
    var(--tl-state-analyzing),
    var(--tl-state-encrypting)
  );
}
.utc--running .utc__icon-bubble { background: var(--tl-state-analyzing); }

.utc--cancelling .utc__card::before { background: var(--tl-state-failed); }
.utc--cancelling .utc__icon-bubble { background: var(--tl-state-failed); }

.utc--cancelled .utc__card::before,
.utc--skipped .utc__card::before { background: var(--tl-state-cancelled); }
.utc--cancelled .utc__icon-bubble,
.utc--skipped .utc__icon-bubble { background: var(--tl-state-cancelled); }

.utc--timed_out .utc__card::before { background: var(--tl-state-preprocessing); }
.utc--timed_out .utc__icon-bubble { background: var(--tl-state-preprocessing); }

.utc--queued .utc__card::before,
.utc--submitted .utc__card::before,
.utc--pending .utc__card::before { background: var(--tl-state-created); }

/* ==================== current 状态高亮 ==================== */
.utc--current .utc__card {
  box-shadow: 0 0 0 2px rgba(var(--tl-state-analyzing-rgb), 0.25),
    var(--tl-shadow-card);
}

/* ==================== highlight 状态（最长耗时等） ==================== */
.utc--highlight .utc__card {
  background: linear-gradient(
    180deg,
    var(--tl-card-bg-gradient-start),
    rgba(var(--tl-state-preprocessing-rgb), 0.06)
  );
}

.utc__duration--highlight {
  font-weight: 700;
  color: var(--tl-state-preprocessing);
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
  color: var(--tl-card-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.utc__meta {
  font-size: 11px;
  color: var(--tl-card-text-secondary);
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
  color: var(--tl-card-text-secondary);
  font-family: var(--tl-card-font-mono);
  white-space: nowrap;
}

.utc__time {
  font-size: 11px;
  color: var(--tl-card-text-tertiary);
  white-space: nowrap;
  /* v3 2026-06-18：限制时间最大宽度，避免长 ISO 字符串撑满宽度导致溢出 */
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.utc__chevron {
  font-size: 14px;
  color: var(--tl-card-text-tertiary);
  flex-shrink: 0;
  transition: transform 0.2s ease;
}

/* ==================== 进度条 + 速率 + ETA（与 TestReportHeader 统一：4px 高，2px 圆角） ==================== */
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
  height: var(--tl-progress-height);
  background: var(--tl-progress-bg);
  border-radius: var(--tl-progress-radius);
  overflow: hidden;
}

.utc__progress-fill {
  height: 100%;
  background: var(--tl-progress-fill);
  border-radius: var(--tl-progress-radius);
  transition: width 0.3s ease;
}

.utc__progress-text {
  font-size: 10px;
  font-weight: 600;
  color: var(--tl-card-text-secondary);
  font-family: var(--tl-card-font-mono);
  white-space: nowrap;
}

.utc__metric {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 10px;
  color: var(--tl-card-text-secondary);
  white-space: nowrap;
}

.utc__metric-icon {
  font-size: 11px;
  flex-shrink: 0;
}

/* ==================== 错误提示 ==================== */
.utc__error-hint {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
  padding: 4px 8px;
  background: rgba(var(--tl-state-failed-rgb), 0.08);
  border-radius: var(--tl-card-radius-sm);
  font-size: 11px;
  color: var(--tl-state-failed);
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

/* ==================== 展开详情（左侧 2px 边线 + padding，非嵌套卡片） ==================== */
.utc__detail {
  margin-top: 10px;
  padding: var(--tl-detail-padding);
  border-left: var(--tl-detail-border-left);
  margin-left: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 8px;
  animation: utc-detail-enter 0.2s ease;
}

.utc__detail-card {
  min-width: 0;
}

.utc__detail-card--error {
  grid-column: 1 / -1;
}

.utc__detail-label {
  font-size: 10px;
  color: var(--tl-card-text-tertiary);
  margin-bottom: 2px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.utc__detail-value {
  font-size: 12px;
  color: var(--tl-card-text-primary);
  font-weight: 500;
  word-break: break-all;
}

.utc__detail-value--mono {
  font-family: var(--tl-card-font-mono);
  font-size: 11px;
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
</style>
