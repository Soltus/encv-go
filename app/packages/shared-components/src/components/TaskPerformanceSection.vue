<template>
  <div class="detail-section" v-if="summary || metrics">
    <div class="section-title-row">
      <span class="section-title">{{ t('tasks.performance.title') }}</span>
      <ion-badge v-if="displayGrade" :color="gradeColor">
        {{ t(`tasks.performance.grade.${displayGrade}`) }} {{ displayGradeScore }}
      </ion-badge>
    </div>

    <!-- 摘要指标 -->
    <div class="metrics-grid" v-if="displayMetrics">
      <div class="metric-item">
        <span class="metric-label">{{ t('tasks.performance.sourceSize') }}</span>
        <span class="metric-value">{{ formatBytes(displayMetrics.sourceSize) }}</span>
      </div>
      <div class="metric-item">
        <span class="metric-label">{{ t('tasks.performance.outputSize') }}</span>
        <span class="metric-value">{{ formatBytes(displayMetrics.outputSize) }}</span>
      </div>
      <div class="metric-item" v-if="displayMetrics.sizeRatio">
        <span class="metric-label">{{ t('tasks.performance.sizeRatio') }}</span>
        <span class="metric-value">{{ displayMetrics.sizeRatio.toFixed(3) }}</span>
      </div>
      <div class="metric-item">
        <span class="metric-label">{{ t('tasks.performance.avgThroughput') }}</span>
        <span class="metric-value">{{ displayMetrics.avgThroughput.toFixed(1) }} MB/s</span>
      </div>
      <div class="metric-item" v-if="displayMetrics.peakThroughput > 0">
        <span class="metric-label">{{ t('tasks.performance.peakThroughput') }}</span>
        <span class="metric-value">{{ displayMetrics.peakThroughput.toFixed(1) }} MB/s</span>
      </div>
      <div class="metric-item" v-if="displayMetrics.p50Throughput > 0">
        <span class="metric-label">P50</span>
        <span class="metric-value">{{ displayMetrics.p50Throughput.toFixed(1) }} MB/s</span>
      </div>
      <div class="metric-item" v-if="displayMetrics.p99Throughput > 0">
        <span class="metric-label">P99</span>
        <span class="metric-value">{{ displayMetrics.p99Throughput.toFixed(1) }} MB/s</span>
      </div>
      <div class="metric-item">
        <span class="metric-label">{{ t('tasks.performance.totalDuration') }}</span>
        <span class="metric-value">{{ displayMetrics.totalDurationMs }} ms</span>
      </div>
    </div>

    <!-- Phase 耗时时间线 -->
    <div class="phase-timings" v-if="phaseTimings.length > 0">
      <div class="phase-timing-header">{{ t('tasks.performance.phaseTimings') }}</div>
      <div v-for="pt in phaseTimings" :key="pt.phase" class="phase-timing-item">
        <div class="phase-info">
          <span class="phase-name">{{ pt.phase }}</span>
          <span class="phase-duration">{{ pt.durationMs }}ms</span>
          <span class="phase-throughput" v-if="(pt.throughputMBps ?? 0) > 0">
            {{ (pt.throughputMBps ?? 0).toFixed(1) }} MB/s
          </span>
        </div>
        <div class="phase-bar-container">
          <div class="phase-bar" :style="{ width: phaseBarWidth(pt.durationMs) + '%' }"></div>
        </div>
      </div>
    </div>

    <!-- 硬件校准信息 -->
    <div class="calibration-info" v-if="displayMetrics && displayMetrics.cpuScore > 0">
      <span class="metric-label">{{ t('tasks.performance.calibration') }}:</span>
      <span class="metric-value">
        CPUScore {{ displayMetrics.cpuScore.toFixed(2) }} ({{ displayMetrics.cpuLabel }})
      </span>
    </div>

    <!-- 展开完整指标按钮 -->
    <ion-button
      v-if="summary && !metrics"
      fill="clear"
      size="small"
      @click="loadFullMetrics"
    >
      {{ loading ? t('common.loading') : t('tasks.performance.viewFullMetrics') }}
    </ion-button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import {
  type EncvTask,
  getTaskPerformance,
  type PerformanceMetrics,
  type PerformanceSummary,
  type PhaseTiming,
} from "@encv/shared-components/api/encv";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { formatBytes } from "@encv/shared-components/lib/format";

interface DisplayMetrics {
  sourceSize: number;
  outputSize: number;
  sizeRatio: number;
  avgThroughput: number;
  peakThroughput: number;
  p50Throughput: number;
  p99Throughput: number;
  totalDurationMs: number;
  cpuScore: number;
  cpuLabel: string;
}

const props = defineProps<{ task: EncvTask }>();
const { t } = useI18n();

const metrics = ref<PerformanceMetrics | null>(null);
const loading = ref(false);

const summary = computed<PerformanceSummary | undefined>(() => props.task.performanceSummary);

// 显示用的指标：优先用完整 metrics，否则用 summary
const displayMetrics = computed<DisplayMetrics | null>(() => {
  if (metrics.value) {
    const m = metrics.value;
    return {
      sourceSize: m.sourceSize,
      outputSize: m.outputSize,
      sizeRatio: m.sizeRatio,
      avgThroughput: m.avgThroughput,
      peakThroughput: m.peakThroughput,
      p50Throughput: m.p50Throughput,
      p99Throughput: m.p99Throughput,
      totalDurationMs: m.totalDurationMs,
      cpuScore: m.cpuScore,
      cpuLabel: m.cpuLabel,
    };
  }
  if (summary.value) {
    const s = summary.value;
    return {
      sourceSize: s.sourceSize,
      outputSize: s.outputSize,
      sizeRatio: s.sourceSize > 0 ? s.outputSize / s.sourceSize : 0,
      avgThroughput: s.avgThroughput,
      peakThroughput: 0,
      p50Throughput: 0,
      p99Throughput: 0,
      totalDurationMs: s.totalDurationMs,
      cpuScore: 0,
      cpuLabel: "",
    };
  }
  return null;
});

const displayGrade = computed(() => {
  if (metrics.value) return metrics.value.grade;
  if (summary.value) return summary.value.grade;
  return "";
});

const displayGradeScore = computed(() => {
  if (metrics.value) return metrics.value.gradeScore.toFixed(0);
  if (summary.value) return summary.value.gradeScore.toFixed(0);
  return "";
});

const gradeColor = computed(() => {
  switch (displayGrade.value) {
    case "excellent":
      return "success";
    case "good":
      return "primary";
    case "warn":
      return "warning";
    default:
      return "medium";
  }
});

const phaseTimings = computed<PhaseTiming[]>(() => {
  return metrics.value?.phaseTimings || [];
});

const maxPhaseDuration = computed(() => {
  if (phaseTimings.value.length === 0) return 1;
  return Math.max(...phaseTimings.value.map(p => p.durationMs), 1);
});

function phaseBarWidth(durationMs: number): number {
  return (durationMs / maxPhaseDuration.value) * 100;
}

async function loadFullMetrics() {
  loading.value = true;
  try {
    metrics.value = await getTaskPerformance(props.task.id);
  } catch (err) {
    console.error("loadFullMetrics failed:", err);
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.detail-section {
  background: var(--ion-card-background, #fff);
  border-radius: 12px;
  padding: 16px;
  margin: 8px 0;
}
.section-title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--encv-text-primary, #000);
}
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px 16px;
  margin-bottom: 12px;
}
.metric-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.metric-label {
  font-size: 11px;
  color: var(--encv-text-secondary, #999);
}
.metric-value {
  font-size: 13px;
  font-weight: 500;
  color: var(--encv-text-primary, #000);
}
.phase-timings {
  margin-top: 12px;
}
.phase-timing-header {
  font-size: 12px;
  font-weight: 600;
  color: var(--encv-text-secondary, #999);
  margin-bottom: 8px;
}
.phase-timing-item {
  margin-bottom: 8px;
}
.phase-info {
  display: flex;
  gap: 8px;
  font-size: 12px;
  margin-bottom: 2px;
}
.phase-name {
  flex: 1;
  color: var(--encv-text-primary, #000);
}
.phase-duration {
  color: var(--encv-text-secondary, #999);
}
.phase-throughput {
  color: var(--ion-color-primary);
}
.phase-bar-container {
  height: 4px;
  background: var(--ion-color-light, #f4f5f8);
  border-radius: 2px;
  overflow: hidden;
}
.phase-bar {
  height: 100%;
  background: var(--ion-color-primary);
  border-radius: 2px;
  transition: width 0.3s ease;
}
.calibration-info {
  margin-top: 12px;
  padding-top: 8px;
  border-top: 1px solid var(--ion-color-light, #f4f5f8);
  font-size: 12px;
  display: flex;
  gap: 8px;
}
</style>
