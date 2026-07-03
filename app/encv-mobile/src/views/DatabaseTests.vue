<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools/automation-hub"></ion-back-button>
        </ion-buttons>
        <ion-title>数据库自动化测试</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <p class="section-hint">
        对当前数据库引擎运行完整测试：CRUD、批量写入、查询过滤、并发压测、导出导入一致性。
        测试数据使用独立前缀，不会影响真实任务数据。
      </p>

      <ion-list>
        <ion-list-header>
          <ion-label>当前引擎</ion-label>
        </ion-list-header>
        <ion-item>
          <ion-label>
            <h3>{{ dbInfo?.engine || '—' }}</h3>
            <p>任务总数：{{ dbInfo?.taskCount ?? '—' }}</p>
          </ion-label>
          <ion-badge slot="end" :color="dbInfo?.engine === 'sqlite' ? 'primary' : 'success'">
            {{ dbInfo?.engine || '—' }}
          </ion-badge>
        </ion-item>
      </ion-list>

      <div v-for="(group, cat) in groupedScenarios" :key="cat" class="scenario-group">
        <div class="group-header">
          <ion-icon :icon="cat === '引擎特殊能力' ? starOutline : listOutline"
            :color="cat === '引擎特殊能力' ? 'warning' : 'primary'"
          ></ion-icon>
          <span class="group-title">{{ cat }}</span>
          <span class="group-count">{{ group.length }} 项</span>
        </div>

        <ion-list>
          <ion-item
            v-for="scenario in group"
            :key="scenario.id"
            class="scenario-item"
            :class="{
              'scenario-passed': scenario.status === 'passed',
              'scenario-failed': scenario.status === 'failed',
              'scenario-running': scenario.status === 'running',
              'scenario-capability': cat === '引擎特殊能力',
            }"
          >
            <div class="scenario-content">
              <div class="scenario-header">
                <span class="scenario-name">{{ scenario.name }}</span>
                <ion-badge
                  v-if="scenario.status === 'passed'"
                  color="success"
                  class="scenario-badge"
                >通过</ion-badge>
                <ion-badge
                  v-else-if="scenario.status === 'failed'"
                  color="danger"
                  class="scenario-badge"
                >失败</ion-badge>
                <ion-badge
                  v-else-if="scenario.status === 'running'"
                  color="primary"
                  class="scenario-badge"
                >进行中</ion-badge>
                <ion-badge
                  v-else
                  color="medium"
                  class="scenario-badge"
                >待执行</ion-badge>
              </div>
              <p class="scenario-desc">{{ scenario.description }}</p>

              <div v-if="scenario.status === 'running'" class="scenario-progress">
                <ion-progress-bar type="indeterminate"></ion-progress-bar>
              </div>

              <div v-if="scenario.durationMs != null" class="scenario-meta">
                <span>耗时：{{ (scenario.durationMs / 1000).toFixed(2) }}s</span>
              </div>

              <div v-if="scenario.metrics" class="metrics-card">
                <div v-for="(value, key) in scenario.metrics" :key="key" class="metric-row">
                  <span class="metric-key">{{ key }}</span>
                  <span class="metric-value">{{ formatMetricValue(value) }}</span>
                </div>
              </div>

              <div v-if="scenario.error" class="error-card">
                <ion-icon :icon="closeCircleOutline" color="danger" class="error-icon"></ion-icon>
                <span class="error-text">{{ scenario.error }}</span>
              </div>
            </div>
          </ion-item>
        </ion-list>
      </div>

      <div class="action-section">
        <ion-button
          expand="block"
          :disabled="isRunning"
          @click="handleRunTests"
        >
          <ion-icon :icon="playCircleOutline" slot="start"></ion-icon>
          {{ isRunning ? '测试进行中...' : '开始测试' }}
        </ion-button>

        <div v-if="isRunning" class="current-phase">
          <ion-spinner name="dots"></ion-spinner>
          <span>{{ currentPhase || '准备中...' }}</span>
        </div>
      </div>

      <div v-if="summary" class="summary-card" :class="summary.failed > 0 ? 'summary-failed' : 'summary-passed'">
        <div class="summary-title">
          <ion-icon :icon="summary.failed > 0 ? closeCircleOutline : checkmarkCircleOutline"
            :color="summary.failed > 0 ? 'danger' : 'success'"
          ></ion-icon>
          <span>{{ summary.failed > 0 ? '测试未通过' : '全部通过' }}</span>
        </div>
        <div class="summary-stats">
          <div class="summary-stat">
            <span class="stat-label">总场景</span>
            <span class="stat-value">{{ summary.total }}</span>
          </div>
          <div class="summary-stat">
            <span class="stat-label">通过</span>
            <span class="stat-value stat-passed">{{ summary.passed }}</span>
          </div>
          <div class="summary-stat">
            <span class="stat-label">失败</span>
            <span class="stat-value stat-failed">{{ summary.failed }}</span>
          </div>
          <div class="summary-stat">
            <span class="stat-label">总耗时</span>
            <span class="stat-value">{{ (summary.totalMs / 1000).toFixed(2) }}s</span>
          </div>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  IonBackButton,
  IonBadge,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonPage,
  IonProgressBar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { checkmarkCircleOutline, closeCircleOutline, listOutline, playCircleOutline, starOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { getDatabaseInfo, runDatabaseTests, type DBTestProgress } from "@/api/encv_perf";
import { showToast } from "@/composables/useToast";

interface ScenarioItem {
  id: string;
  name: string;
  description: string;
  status: "pending" | "running" | "passed" | "failed";
  category?: string;
  durationMs?: number;
  metrics?: Record<string, any>;
  error?: string;
}

const dbInfo = ref<any>(null);
const isRunning = ref(false);
const currentPhase = ref("");

const scenarios = ref<ScenarioItem[]>([
  { id: "crud", name: "CRUD 基础操作", description: "创建、读取、更新、删除任务", status: "pending", category: "基础" },
  { id: "batch_write", name: "批量写入性能", description: "批量创建 1000 条任务，测写入吞吐", status: "pending", category: "基础" },
  { id: "query_filter", name: "查询过滤", description: "按类型、状态、触发器、runId 等多维度过滤", status: "pending", category: "基础" },
  { id: "concurrency", name: "并发压测", description: "5 协程并发写入，测事务隔离性", status: "pending", category: "基础" },
  { id: "export_import", name: "导出导入一致性", description: "导出 JSON → 删除 → 导入 → 验证数据一致", status: "pending", category: "基础" },
  { id: "large_table_query", name: "大表查询性能", description: "5000 条数据下的单条件/多条件/分页查询，测索引优化能力", status: "pending", category: "引擎特殊能力" },
  { id: "concurrent_rw", name: "并发读写分离", description: "3 写 + 5 读同时跑 5s，测 MVCC / 读写不阻塞能力", status: "pending", category: "引擎特殊能力" },
  { id: "transaction", name: "ACID 事务验证", description: "导入导出一致性 / 回滚 / 更新原子性，测事务 ACID 完整性", status: "pending", category: "引擎特殊能力" },
]);

const summary = ref<{
  total: number;
  passed: number;
  failed: number;
  totalMs: number;
} | null>(null);

const failedCount = computed(() => scenarios.value.filter(s => s.status === "failed").length);

const groupedScenarios = computed(() => {
  const groups: Record<string, ScenarioItem[]> = {};
  for (const s of scenarios.value) {
    const cat = s.category || "其他";
    if (!groups[cat]) groups[cat] = [];
    groups[cat].push(s);
  }
  return groups;
});

function formatMetricValue(v: any): string {
  if (typeof v === "number") {
    if (v > 1000 && Number.isInteger(v)) return v.toLocaleString();
    if (v < 100) return v.toFixed(2);
    return Math.round(v).toString();
  }
  if (typeof v === "boolean") return v ? "是" : "否";
  if (Array.isArray(v)) return v.join(", ");
  return String(v);
}

onMounted(async () => {
  try {
    dbInfo.value = await getDatabaseInfo();
  } catch (e) {
    console.warn("[DatabaseTests] load db info failed:", e);
  }
});

function resetScenarios() {
  for (const s of scenarios.value) {
    s.status = "pending";
    s.durationMs = undefined;
    s.metrics = undefined;
    s.error = undefined;
  }
  summary.value = null;
}

function handleProgress(p: DBTestProgress) {
  currentPhase.value = p.message;

  if (p.phase === "started") {
    const scenario = scenarios.value.find(s => s.id === p.scenario);
    if (scenario) {
      scenario.status = "running";
    }
  } else if (p.phase === "passed" || p.phase === "failed") {
    const scenario = scenarios.value.find(s => s.id === p.scenario);
    if (scenario) {
      scenario.status = p.phase as "passed" | "failed";
      scenario.durationMs = p.durationMs;
      scenario.metrics = p.metrics;
      scenario.error = p.error;
    }
  } else if (p.phase === "completed") {
    const passed = (p.metrics?.passed as string[] | undefined)?.length ?? 0;
    const failed = (p.metrics?.failed as string[] | undefined)?.length ?? 0;
    const total = (p.metrics?.total as number) ?? 0;
    summary.value = {
      total,
      passed,
      failed,
      totalMs: p.durationMs ?? 0,
    };
  }
}

async function handleRunTests() {
  if (isRunning.value) return;

  resetScenarios();
  isRunning.value = true;
  currentPhase.value = "准备中...";

  try {
    await runDatabaseTests(undefined, handleProgress);
    showToast({ message: "测试完成", color: failedCount.value > 0 ? "warning" : "success" });
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    showToast({ message: "测试失败: " + msg, color: "danger" });
  } finally {
    isRunning.value = false;
    currentPhase.value = "";
  }
}
</script>

<style scoped>
.section-hint {
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  margin: 12px 16px 8px;
  line-height: 1.5;
}

.scenario-item {
  --inner-padding-end: 0;
}

.scenario-group {
  margin-top: 12px;
}

.group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  font-size: 14px;
  font-weight: 600;
  color: var(--encv-text-secondary, #666);
}

.group-title {
  flex: 1;
}

.group-count {
  font-size: 12px;
  font-weight: normal;
  color: var(--encv-text-tertiary, #999);
}

.scenario-capability {
  --background: var(--ion-color-warning-rgb, 255, 193, 7);
  --background-opacity: 0.05;
}

.scenario-content {
  width: 100%;
  padding: 12px 0;
}

.scenario-header {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.scenario-name {
  font-size: 15px;
  font-weight: 600;
}

.scenario-badge {
  font-size: 0.65em;
  --padding-start: 6px;
  --padding-end: 6px;
}

.scenario-desc {
  font-size: 12px;
  color: var(--ion-color-medium);
  margin: 4px 0 0;
}

.scenario-progress {
  margin-top: 8px;
}

.scenario-meta {
  font-size: 11px;
  color: var(--ion-color-medium);
  margin-top: 6px;
}

.metrics-card {
  margin-top: 8px;
  padding: 8px 10px;
  background: var(--ion-color-light, #f4f5f8);
  border-radius: 6px;
}

body.dark .metrics-card {
  background: #2a2a2c;
}

.metric-row {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  padding: 2px 0;
}

.metric-key {
  color: var(--ion-color-medium);
}

.metric-value {
  font-weight: 500;
}

.error-card {
  margin-top: 8px;
  padding: 8px 10px;
  background: var(--ion-color-danger-50, #fef0f0);
  border-radius: 6px;
  display: flex;
  align-items: flex-start;
  gap: 6px;
}

body.dark .error-card {
  background: rgba(239, 68, 68, 0.15);
}

.error-icon {
  font-size: 14px;
  flex-shrink: 0;
  margin-top: 1px;
}

.error-text {
  font-size: 11px;
  color: var(--ion-color-danger);
  line-height: 1.4;
  word-break: break-all;
}

.scenario-passed {
  --background: var(--ion-color-success-50, rgba(34, 197, 94, 0.08));
}

.scenario-failed {
  --background: var(--ion-color-danger-50, rgba(239, 68, 68, 0.08));
}

.action-section {
  padding: 16px;
}

.current-phase {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 12px;
  font-size: 13px;
  color: var(--ion-color-medium);
}

.summary-card {
  margin: 0 16px 24px;
  padding: 16px;
  border-radius: 12px;
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
}

body.dark .summary-card {
  border-color: #2a2a2c;
}

.summary-passed {
  background: var(--ion-color-success-50, rgba(34, 197, 94, 0.08));
  border-color: var(--ion-color-success-200, #86efac);
}

.summary-failed {
  background: var(--ion-color-danger-50, rgba(239, 68, 68, 0.08));
  border-color: var(--ion-color-danger-200, #fca5a5);
}

.summary-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 12px;
}

.summary-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
}

.summary-stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.stat-label {
  font-size: 11px;
  color: var(--ion-color-medium);
}

.stat-value {
  font-size: 18px;
  font-weight: 600;
}

.stat-passed {
  color: var(--ion-color-success);
}

.stat-failed {
  color: var(--ion-color-danger);
}
</style>
