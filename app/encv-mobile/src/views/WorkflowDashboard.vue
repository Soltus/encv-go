<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.automationTests') }}</ion-title>
        <ion-buttons slot="end">
          <!-- 视图切换 -->
          <button
            class="view-toggle"
            :class="{ 'view-toggle--active': viewMode === 'pipeline' }"
            @click="viewMode = 'pipeline'"
          >Pipeline</button>
          <span class="view-toggle-sep">|</span>
          <button
            class="view-toggle"
            :class="{ 'view-toggle--active': viewMode === 'tree' }"
            @click="viewMode = 'tree'"
          >Tree</button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>

      <!-- ========== Mock 数据管理区（保留原有功能）========== -->
      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('devtools.mockDataManager') }}</ion-label>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.mockDataManagerHint') }}</p>

        <ion-item>
          <ion-label>
            <h3>{{ t('devtools.mockRoot') }}</h3>
            <p><code class="mock-root-path">{{ mockRoot }}</code></p>
          </ion-label>
        </ion-item>

        <ion-item button @click="handleGenerateMock" :disabled="isGenerating">
          <ion-icon :icon="addCircleOutline" slot="start" color="primary"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.generateMock') }}</h3>
            <p>{{ t('devtools.generateMockDesc') }}</p>
          </ion-label>
          <ion-spinner v-if="isGenerating" slot="end" name="dots"></ion-spinner>
        </ion-item>

        <ion-item button @click="handleResetMock" :disabled="isResetting">
          <ion-icon :icon="trashOutline" slot="start" color="danger"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.resetMock') }}</h3>
            <p>{{ t('devtools.resetMockDesc') }}</p>
          </ion-label>
          <ion-spinner v-if="isResetting" slot="end" name="dots"></ion-spinner>
        </ion-item>

        <div v-if="mockStats" class="mock-stats-card">
          <div class="stat-row"><span>{{ t('devtools.fileCount') }}</span><span class="stat-value">{{ mockStats.count }}</span></div>
          <div class="stat-row"><span>{{ t('devtools.totalSize') }}</span><span class="stat-value">{{ humanSize(mockStats.totalSize) }}</span></div>
        </div>

        <div v-if="generateProgressText" class="progress-text">{{ generateProgressText }}</div>
      </ion-list>

      <!-- ========== 工作流选择 + 运行 ========== -->
      <ion-list>
        <ion-list-header>
          <ion-label>WORKFLOW ENGINE</ion-label>
        </ion-list-header>

        <!-- 工作流模板选择 -->
        <ion-item>
          <ion-select
            :value="selectedDefId"
            @ionChange="selectedDefId = $event.detail.value"
            interface="action-sheet"
            placeholder="Select workflow..."
          >
            <ion-select-option
              v-for="def in definitions"
              :key="def.id"
              :value="def.id"
            >
              {{ def.name }}{{ def.builtin ? ' (builtin)' : '' }}
            </ion-select-option>
          </ion-select>
        </ion-item>

        <ion-item button detail @click="handleRunWorkflow" :disabled="!selectedDefId || isRunning">
          <ion-icon :icon="playCircleOutline" slot="start" color="success"></ion-icon>
          <ion-label>
            <h3>Run Workflow</h3>
            <p>{{ selectedDef?.description ?? 'Execute the selected workflow' }}</p>
          </ion-label>
        </ion-item>

        <ion-item v-if="isRunning && currentRun" button @click="handleCancel" detail>
          <ion-icon :icon="closeCircleOutline" slot="start" color="danger"></ion-icon>
          <ion-label>
            <h3>Cancel Run</h3>
            <p>Stop the current workflow execution</p>
          </ion-label>
        </ion-item>
      </ion-list>

      <!-- ========== 运行报告（Pipeline 或 Tree）========== -->

      <template v-if="currentRun">
        <!-- 报告头部（复用 DOSSIER 风格） -->
        <TestReportHeader
          :run-id="currentRun.id"
          :opened-at="currentRun.createdAt"
          :duration-ms="reportDurationMs"
          :total="totalSteps"
          :passed="successSteps"
          :failed="failedSteps"
          :skipped="0"
          :pending="totalSteps - completedSteps"
          :platform="platform"
        />

        <!-- ===== Pipeline 视图 ===== -->
        <template v-if="viewMode === 'pipeline'">
          <JobPipelineCard
            v-for="job in currentRun.jobs"
            :key="job.id"
            :job="job"
            :step-names="stepNameMap"
            :display-name="getJobDisplayName(job.jobDefId)"
          />
        </template>

        <!-- ===== Tree 视图 ===== -->
        <template v-else>
          <TreeView
            :workflow-run="currentRun"
            :step-names="stepNameMap"
            :job-display-names="jobDisplayNameMap"
            @select-step="onSelectStep"
          />
          <StepDetailPanel
            v-if="selectedStep"
            :step-run="selectedStep"
            :job-run="selectedStepJob!"
          />
        </template>
      </template>

      <!-- 历史运行列表 -->
      <ion-list v-if="serviceRuns.length > 0 && !currentRun">
        <ion-list-header>
          <ion-label>PAST RUNS</ion-label>
        </ion-list-header>
        <ion-item
          v-for="record in serviceRuns.slice(0, 10)"
          :key="record.id"
          button
          detail
          @click="selectHistoryRun(record)"
        >
          <ion-label>
            <h3>{{ record.id.slice(0, 16) }}...</h3>
            <p>{{ record.workflowRun?.status ?? 'unknown' }} · {{ record.workflowRun?.jobs.length ?? 0 }} jobs · {{ formatTime(record.startedAt) }}</p>
          </ion-label>
          <StepMiniBadge :status="(record.workflowRun?.status === 'running' ? 'queued' : record.workflowRun?.status) ?? 'queued'" :show-name="false" slot="end" />
        </ion-item>
      </ion-list>

    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { addCircleOutline, closeCircleOutline, playCircleOutline, trashOutline } from "ionicons/icons";
import { computed, onMounted, onUnmounted, ref } from "vue";
import { generateMockFilesViaBackend, resetMockFilesViaBackend } from "@encv/shared-components/api/mockGenerator";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";
import { useWorkflowStore } from "@encv/shared-components/composables/useWorkflowStore";
import { useWorkflowTaskService } from "@encv/shared-components/composables/useWorkflowTaskService";
import { MOCK_GENERATE_ROOT } from "@encv/shared-components/lib/mockConstants";
import type { JobRun, StepRun, UnifiedRunRecord, WorkflowDefinition, WorkflowRun } from "@encv/shared-components/lib/workflow/types";

const { t } = useI18n();

// ---- Legacy: Mock 数据 / 自动化测试 ----
// 🆕 2026-06-15 声明式：mockRoot = AUTOMATION_MOUNT_PATH + '/'（常量，不再 split/slice）
//   之前 .slice(0, 5) 隐式推导：DEFAULT_AUTOMATION_SOURCE 改前缀 → UI 静默选错 → 403
//   现在改 mount path = 改 src/lib/mockConstants.ts + 后端 mount.go，两个源，不会漏
const mockRoot = MOCK_GENERATE_ROOT;
const isGenerating = ref(false);
const isResetting = ref(false);
const mockStats = ref<{ count: number; totalSize: number } | null>(null);
const generateProgressText = ref("");

// ---- Workflow Engine ----
// 🆕 Task 7：useWorkflowEngine 已退役，拆分为：
//   - useWorkflowStore：definitions CRUD + 内置模板注册
//   - useWorkflowTaskService：运行执行 + WS 事件 + 持久化
const store = useWorkflowStore();
const { definitions, getDefinition, registerBuiltinTemplates } = store;
const {
  currentRun,
  isRunning,
  totalSteps,
  completedSteps,
  successSteps,
  failedSteps,
  runs: serviceRuns,
  submitRun,
  cancelRun,
} = useWorkflowTaskService();

const selectedDefId = ref<string>("");
const viewMode = ref<"pipeline" | "tree">("pipeline");
const selectedStep = ref<StepRun | null>(null);
const _tickNow = ref(Date.now());
let tickHandle: ReturnType<typeof setInterval> | null = null;

const platform = computed(() => {
  if (typeof navigator === "undefined") return "node";
  const ua = navigator.userAgent || "";
  if (/android/i.test(ua)) return "android";
  if (/iphone|ipad|ipod/i.test(ua)) return "ios";
  return "web";
});

const reportDurationMs = computed(() => {
  if (!currentRun.value) return 0;
  if (isRunning.value) return _tickNow.value - (currentRun.value.startedAt ? new Date(currentRun.value.startedAt).getTime() : Date.now());
  return currentRun.value.durationMs ?? 0;
});

const selectedDef = computed(() => definitions.value.find(d => d.id === selectedDefId.value));

/** 构建 stepDefId → step name 的映射 */
const stepNameMap = computed(() => {
  const map = new Map<string, string>();
  const def = (selectedDef.value ?? currentRun.value) ? definitions.value.find(d => d.id === currentRun.value!.workflowDefId) : null;
  if (def) {
    for (const job of def.jobs) {
      for (const step of job.steps) {
        map.set(step.id, step.name);
      }
    }
  }
  return map;
});

/** 构建 jobDefId → display name 的映射 */
const jobDisplayNameMap = computed(() => {
  const map = new Map<string, string>();
  const def = currentRun.value ? definitions.value.find(d => d.id === currentRun.value!.workflowDefId) : null;
  if (def) {
    for (const job of def.jobs) {
      map.set(job.id, job.name);
    }
  }
  return map;
});

function getJobDisplayName(jobDefId: string): string {
  return jobDisplayNameMap.value.get(jobDefId) ?? jobDefId;
}

function findJobForStep(run: WorkflowRun, step: StepRun): JobRun | undefined {
  return run.jobs.find((j: JobRun) => j.steps.some((s: StepRun) => s.id === step.id));
}

// ---- Handlers ----

async function handleGenerateMock() {
  if (isGenerating.value) return;
  isGenerating.value = true;
  generateProgressText.value = "";
  mockStats.value = null;
  let lastCount = 0;
  let lastSize = 0;
  try {
    const result = await generateMockFilesViaBackend({
      root: mockRoot,
      type: "all",
      onProgress: p => {
        lastCount++;
        lastSize += p.size;
        generateProgressText.value = `(${lastCount}) ${p.relativePath}`;
      },
    });
    mockStats.value = { count: result.count || lastCount, totalSize: result.totalSize || lastSize };
    showToast({ message: `${t("devtools.generateMock")}: ${mockStats.value.count}`, color: "success", duration: 1500 });
  } catch (e) {
    showToast({ message: `${t("devtools.generateMock")} failed: ${e instanceof Error ? e.message : e}`, color: "danger", duration: 2500 });
  } finally {
    isGenerating.value = false;
    generateProgressText.value = "";
  }
}

async function handleResetMock() {
  if (isResetting.value) return;
  isResetting.value = true;
  try {
    const r = await resetMockFilesViaBackend(mockRoot);
    mockStats.value = null;
    showToast({ message: `Reset: ${r.removed} files`, color: "success", duration: 1500 });
  } catch (e) {
    showToast({ message: `Reset failed: ${e instanceof Error ? e.message : e}`, color: "danger", duration: 2500 });
  } finally {
    isResetting.value = false;
  }
}

async function handleRunWorkflow() {
  if (!selectedDefId.value || isRunning.value) return;
  const def = getDefinition(selectedDefId.value);
  if (!def) return;
  try {
    await submitRun({ workflow: def, triggeredBy: "automation" });
    showToast({ message: `Workflow started`, color: "success", duration: 1500 });
  } catch (e) {
    showToast({ message: `${e instanceof Error ? e.message : e}`, color: "danger", duration: 2500 });
  }
}

async function handleCancel() {
  if (currentRun.value) {
    await cancelRun(currentRun.value.id);
  }
  showToast({ message: "Workflow cancelled", color: "warning", duration: 1500 });
}

function selectHistoryRun(record: UnifiedRunRecord) {
  // 从 UnifiedRunRecord.workflowRun 快照恢复到 currentRun（UI 回放历史运行）
  if (record.workflowRun) {
    currentRun.value = record.workflowRun;
  }
}

function onSelectStep(step: StepRun) {
  selectedStep.value = step;
}

const selectedStepJob = computed(() =>
  currentRun.value && selectedStep.value ? findJobForStep(currentRun.value, selectedStep.value) : null
);

// ---- Lifecycle ----

onMounted(() => {
  tickHandle = setInterval(() => {
    _tickNow.value = Date.now();
  }, 1000);
  // useWorkflowTaskService 内部通过 useTaskEventBridge 自动订阅 WS 4 件套事件
  registerBuiltinTemplates(BUILTIN_TEMPLATES);
  // 默认选中第一个模板
  if (definitions.value.length > 0 && !selectedDefId.value) {
    selectedDefId.value = definitions.value[0].id;
  }
});
onUnmounted(() => {
  if (tickHandle) clearInterval(tickHandle);
  // useWorkflowTaskService 内部通过 useTaskEventBridge 自动取消订阅
});

// ---- Utils ----

function humanSize(bytes: number): string {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleTimeString();
  } catch {
    return iso;
  }
}

// ==================== 内置模板定义 ====================

const BUILTIN_TEMPLATES: WorkflowDefinition[] = [
  {
    id: "builtin-auto-test",
    name: "自动化测试套件",
    description: "生成 Mock → 矩阵加密测试 → 解密验证",
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    trigger: "manual",
    env: { PASSWORD: "automation-test-pwd" },
    builtin: true,
    jobs: [
      {
        id: "setup-mock",
        name: "生成 Mock 数据",
        steps: [
          {
            id: "gen-mock",
            name: "Generate Mock Files",
            action: { type: "encv_task", taskType: "encrypt", pluginName: "mock-generator", params: {} },
          },
        ],
      },
      {
        id: "test-encrypt",
        name: "加密测试矩阵",
        needs: ["setup-mock"],
        strategy: {
          type: "matrix",
          axes: {
            plugin: ["video-v4", "audio-v4"],
            cipher: ["0", "1"],
            compression: ["none", "zstd"],
          },
        },
        steps: [
          {
            id: "enc-step",
            name: "Encrypt (${{plugin}}, c${{cipher}}, ${{compression}})",
            action: {
              type: "encv_task",
              taskType: "encrypt",
              pluginName: "${{plugin}}",
              params: {
                sourcePath: "/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4",
                password: "${{PASSWORD}}",
                version: 4,
                cipherMode: Number("${{cipher}}"),
                compressionMode: "${{compression}}" as any,
              },
            },
          },
        ],
      },
      {
        id: "test-decrypt",
        name: "解密验证",
        needs: ["test-encrypt"],
        steps: [
          {
            id: "dec-step",
            name: "Decrypt Verification",
            action: {
              type: "encv_task",
              taskType: "decrypt",
              pluginName: "${{plugin}}",
              params: {
                sourcePath: "/storage/emulated/0/encv-automation/02-encrypted/video/sample.mp4.${containerExt}",
                password: "${{PASSWORD}}",
              },
            },
            if: { op: "success" }, // 仅在加密成功后执行
          },
        ],
      },
    ],
  },
  {
    id: "builtin-batch-transcode",
    name: "批量转码",
    description: "并行转码多个文件",
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    trigger: "manual",
    builtin: true,
    jobs: [
      {
        id: "transcode-all",
        name: "批量转码",
        strategy: { type: "parallel", max: 3 },
        steps: [
          {
            id: "tc-1",
            name: "Transcode File 1",
            action: { type: "encv_task", taskType: "encrypt", pluginName: "ffmpeg", params: {} },
          },
        ],
      },
    ],
  },
  {
    id: "builtin-custom",
    name: "自定义流水线",
    description: "空白模板，自由编排",
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    trigger: "manual",
    builtin: true,
    jobs: [],
  },
];
</script>

<style scoped>
.section-hint {
  font-size: 12px;
  color: var(--ion-color-medium-shade);
  padding: 8px 16px 4px;
  margin: 0;
}
.mock-root-path {
  font-family: monospace;
  font-size: 12px;
  background: var(--ion-color-light-shade);
  padding: 2px 6px;
  border-radius: 4px;
}
.mock-stats-card {
  margin: 8px 16px;
  padding: 12px 16px;
  background: var(--ion-color-light);
  border-radius: 8px;
}
.stat-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  font-size: 14px;
}
.stat-value { font-weight: 600; font-family: monospace; }
.progress-text {
  font-size: 12px;
  color: var(--ion-color-medium);
  padding: 4px 16px;
  font-family: monospace;
}

/* View toggle */
.view-toggle {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  background: none;
  border: none;
  color: #6B5D4C;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 3px;
}
.view-toggle--active {
  background: #1A1A1A;
  color: #F4EFE6;
}
.view-toggle-sep { color: #C9BBA1; }
</style>
