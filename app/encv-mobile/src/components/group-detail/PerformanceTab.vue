<template>
  <div class="performance-tab">
    <!-- 硬件校准信息 -->
    <ion-card v-if="calibration">
      <ion-card-header>
        <ion-card-title>{{ t('tasks.performance.calibrationTitle') }}</ion-card-title>
      </ion-card-header>
      <ion-card-content>
        <div class="calib-grid">
          <div class="calib-item">
            <span class="calib-label">CPUScore</span>
            <span class="calib-value">{{ calibration.cpuScore.toFixed(2) }} ({{ calibration.cpuLabel }})</span>
          </div>
          <div class="calib-item">
            <span class="calib-label">AES-CTR</span>
            <span class="calib-value">{{ calibration.aesThroughput.toFixed(0) }} MB/s</span>
          </div>
          <div class="calib-item">
            <span class="calib-label">{{ t('tasks.performance.calibratedAt') }}</span>
            <span class="calib-value">{{ formatTime(calibration.calibratedAt) }}</span>
          </div>
          <div class="calib-item">
            <span class="calib-label">{{ t('tasks.performance.platform') }}</span>
            <span class="calib-value">{{ calibration.os }}/{{ calibration.arch }} ({{ calibration.numCpu }} CPU)</span>
          </div>
        </div>
      </ion-card-content>
    </ion-card>

    <!-- Plugin 聚合表格 -->
    <ion-card v-if="pluginAggregation.length > 0">
      <ion-card-header>
        <ion-card-title>{{ t('tasks.performance.pluginAggregation') }}</ion-card-title>
      </ion-card-header>
      <ion-card-content>
        <table class="perf-table">
          <thead>
            <tr>
              <th>Plugin</th>
              <th>{{ t('tasks.performance.caseCount') }}</th>
              <th>{{ t('tasks.performance.avgThroughput') }}</th>
              <th>{{ t('tasks.performance.grade') }}</th>
              <th>{{ t('tasks.performance.trend') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="agg in pluginAggregation" :key="agg.pluginName">
              <td>{{ agg.pluginName }}</td>
              <td>{{ agg.caseCount }}</td>
              <td>{{ agg.avgThroughput.toFixed(1) }} MB/s</td>
              <td>
                <ion-badge :color="gradeColor(agg.dominantGrade)">
                  {{ t(`tasks.performance.grade.${agg.dominantGrade}`) }}
                </ion-badge>
              </td>
              <td>
                <span v-if="agg.trendPctChange !== 0" :class="trendClass(agg.trendPctChange)">
                  {{ trendArrow(agg.trendPctChange) }} {{ Math.abs(agg.trendPctChange).toFixed(1) }}%
                </span>
                <span v-else class="trend-flat">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </ion-card-content>
    </ion-card>

    <!-- 空状态 -->
    <div v-else-if="!loading" class="empty-state">
      <ion-icon :icon="analyticsOutline" size="large" color="medium"></ion-icon>
      <p>{{ t('tasks.performance.noData') }}</p>
    </div>

    <ion-spinner v-if="loading" name="crescent" class="loading-spinner"></ion-spinner>
  </div>
</template>

<script setup lang="ts">
import { IonBadge, IonCard, IonCardContent, IonCardHeader, IonCardTitle, IonIcon, IonSpinner } from "@ionic/vue";
import { analyticsOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { type CalibrationResult, type EncvTask, getCalibration, getPerformanceHistory, type PerformanceMetrics } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";

const props = defineProps<{ runTasks: EncvTask[] }>();
const { t } = useI18n();

const loading = ref(false);
const calibration = ref<CalibrationResult | null>(null);
const historyMap = ref<Map<string, PerformanceMetrics[]>>(new Map());

interface PluginAggregation {
  pluginName: string;
  caseCount: number;
  avgThroughput: number;
  dominantGrade: "excellent" | "good" | "warn";
  trendPctChange: number;
}

const pluginAggregation = computed<PluginAggregation[]>(() => {
  const map = new Map<string, EncvTask[]>();
  for (const task of props.runTasks) {
    if (!task.performanceSummary) continue;
    const key = task.pluginName || "unknown";
    if (!map.has(key)) map.set(key, []);
    map.get(key)!.push(task);
  }

  const result: PluginAggregation[] = [];
  for (const [pluginName, tasks] of map) {
    const throughputs = tasks.map(t => t.performanceSummary?.avgThroughput || 0).filter(v => v > 0);
    const avgThroughput = throughputs.length > 0 ? throughputs.reduce((a, b) => a + b, 0) / throughputs.length : 0;

    const grades = tasks.map(t => t.performanceSummary?.grade || "good");
    const gradeCount: Record<string, number> = { excellent: 0, good: 0, warn: 0 };
    for (const g of grades) gradeCount[g] = (gradeCount[g] || 0) + 1;
    const dominantGrade = (Object.entries(gradeCount).sort((a, b) => b[1] - a[1])[0]?.[0] || "good") as "excellent" | "good" | "warn";

    // 趋势：与历史对比
    let trendPctChange = 0;
    const history = historyMap.value.get(pluginName);
    if (history && history.length >= 2) {
      const current = avgThroughput;
      const previous = history[1].avgThroughput; // history[0] 是最新
      if (previous > 0) {
        trendPctChange = ((current - previous) / previous) * 100;
      }
    }

    result.push({
      pluginName,
      caseCount: tasks.length,
      avgThroughput,
      dominantGrade,
      trendPctChange,
    });
  }

  return result.sort((a, b) => b.caseCount - a.caseCount);
});

function gradeColor(grade: string): string {
  switch (grade) {
    case "excellent":
      return "success";
    case "good":
      return "primary";
    case "warn":
      return "warning";
    default:
      return "medium";
  }
}

function trendArrow(pct: number): string {
  return pct > 0 ? "↗" : "↘";
}

function trendClass(pct: number): string {
  return pct > 0 ? "trend-up" : "trend-down";
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

async function loadData() {
  loading.value = true;
  try {
    calibration.value = await getCalibration();

    // 加载每个 plugin 的历史
    const pluginNames = new Set(props.runTasks.map(t => t.pluginName || "unknown"));
    for (const plugin of pluginNames) {
      try {
        const history = await getPerformanceHistory(plugin, "encrypt", 10);
        if (history.length > 0) {
          historyMap.value.set(plugin, history);
        }
      } catch (err) {
        console.error(`load history for ${plugin} failed:`, err);
      }
    }
  } catch (err) {
    console.error("load performance data failed:", err);
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  loadData();
});
</script>

<style scoped>
.performance-tab {
  padding: 8px;
}
.calib-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px 16px;
}
.calib-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.calib-label {
  font-size: 11px;
  color: var(--encv-text-secondary, #999);
}
.calib-value {
  font-size: 13px;
  font-weight: 500;
}
.perf-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.perf-table th {
  text-align: left;
  padding: 8px 4px;
  border-bottom: 1px solid var(--ion-color-light, #f4f5f8);
  color: var(--encv-text-secondary, #999);
  font-weight: 600;
}
.perf-table td {
  padding: 8px 4px;
  border-bottom: 1px solid var(--ion-color-light, #f4f5f8);
}
.trend-up {
  color: var(--ion-color-success);
}
.trend-down {
  color: var(--ion-color-danger);
}
.trend-flat {
  color: var(--encv-text-secondary, #999);
}
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 48px 16px;
  color: var(--encv-text-secondary, #999);
}
.loading-spinner {
  display: block;
  margin: 24px auto;
}
</style>
