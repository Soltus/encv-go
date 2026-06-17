<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.pluginTests') }}</ion-title>
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

      <!-- ========== 🆕 2026-06-11 内联错误卡（替代 Toast） ========== -->
      <div v-if="inlineError" class="inline-error-card" role="alert">
        <div class="inline-error-header">
          <ion-icon :icon="closeCircleOutline" color="danger" class="inline-error-icon"></ion-icon>
          <div class="inline-error-title-block">
            <div class="inline-error-title">{{ inlineError.title }}</div>
            <div class="inline-error-source">
              源头: {{ inlineError.source }} · {{ formatInlineErrorTime(inlineError.at) }}
            </div>
          </div>
          <button class="inline-error-close" @click="clearInlineError" aria-label="关闭">×</button>
        </div>
        <pre class="inline-error-message">{{ inlineError.message }}</pre>
        <div v-if="inlineError.hint" class="inline-error-hint">
          <strong>💡 排查:</strong> {{ inlineError.hint }}
        </div>
        <div class="inline-error-actions">
          <button
            v-if="inlineError.source === 'mockGenerate'"
            class="inline-error-retry"
            @click="handleGenerateMock"
            :disabled="isGenerating"
          >重试生成 Mock</button>
          <button
            v-if="inlineError.source === 'loadPlugins'"
            class="inline-error-retry"
            @click="handleLoadPlugins"
            :disabled="isLoadingPlugins"
          >重试加载插件</button>
          <button
            v-if="inlineError.source === 'mockReset'"
            class="inline-error-retry"
            @click="handleResetMock"
            :disabled="isResetting"
          >重试重置</button>
        </div>
      </div>

      <!-- ========== Mock 数据管理区 ========== -->
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
         <div v-if="(mockStats.skipped ?? 0) > 0" class="stat-row"><span>skipped (ffmpeg 缺 encoder)</span><span class="stat-value stat-value--warn">{{ mockStats.skipped }}</span></div>
          <div class="stat-row"><span>{{ t('devtools.totalSize') }}</span><span class="stat-value">{{ humanSize(mockStats.totalSize) }}</span></div>
        </div>

        <div v-if="generateProgressText" class="progress-text">{{ generateProgressText }}</div>

        <!-- ========== 🆕 2026-06-12 饱和调试：完整 ffmpeg 流程日志卡 ========== -->
        <!--
          设计目的：
            1. 即使后端 cgo 阻塞导致 SSE 流中断，前端也能展示「最后收到的 spec_diag」（在哪停止）
            2. 失败时一行红色高亮 + 完整 stderr 可点开
            3. 一键复制全部日志（带时间戳）—— 用户贴给开发者排查
            4. 流程每一步带：序号 / 状态 / relativePath / encoder / ffmpegArgs / exitCode / stderr
            5. 静态字节文件（JPEG/PNG/PDF/TXT/CSV）也展示（ffmpegArgs=[] 表明无 ffmpeg 调用）
        -->
        <div v-if="mockGenLog.length > 0" class="mock-gen-log-card">
          <div class="mock-gen-log-header">
            <div class="mock-gen-log-title">
              <ion-icon :icon="terminalOutline" color="primary"></ion-icon>
              <span>FFMPEG 流程日志</span>
              <span class="mock-gen-log-count">{{ mockGenLog.length }} / {{ mockGenLogTotal }}</span>
            </div>
            <button
              class="mock-gen-log-copy"
              :class="{ 'mock-gen-log-copy--copied': mockGenLogCopied }"
              @click="copyMockGenLog"
              :aria-label="mockGenLogCopied ? '已复制' : '复制全部日志'"
            >
              <ion-icon :icon="mockGenLogCopied ? checkmarkCircleOutline : copyOutline" slot="icon-only"></ion-icon>
              <span>{{ mockGenLogCopied ? '已复制' : '复制全部' }}</span>
            </button>
          </div>
          <div class="mock-gen-log-summary" v-if="mockGenLogSummary">
            <ion-icon :icon="mockGenLogSummary.failed > 0 ? warningOutline : checkmarkCircleOutline"
                      :color="mockGenLogSummary.failed > 0 ? 'warning' : 'success'"></ion-icon>
            <span>{{ mockGenLogSummary.text }}</span>
            <span v-if="mockGenLogSummary.disconnected" class="mock-gen-log-disconnect">
              ⚠ 后端连接已断开 — 下面 {{ mockGenLog.length }} 行是「处理到这步」
            </span>
          </div>
          <ol class="mock-gen-log-list">
            <li
              v-for="entry in mockGenLog"
              :key="entry.key"
              class="mock-gen-log-entry"
              :class="{
                'mock-gen-log-entry--failed': entry.status === 'failed',
                'mock-gen-log-entry--success': entry.status === 'ok',
                'mock-gen-log-entry--expanded': entry.expanded,
              }"
            >
              <div class="mock-gen-log-row" @click="toggleMockGenLogEntry(entry.key)">
                <span class="mock-gen-log-status">
                  <ion-icon
                    :icon="entry.status === 'failed' ? closeCircleOutline : entry.status === 'ok' ? checkmarkCircleOutline : ellipsisHorizontalOutline"
                    :color="entry.status === 'failed' ? 'danger' : entry.status === 'ok' ? 'success' : 'medium'"
                  ></ion-icon>
                </span>
                <!-- 🆕 2026-06-12：runner 标识（mediacodec=⚡硬件 / ffmpeg=⚙软件 / static=📄静态） -->
                <span class="mock-gen-log-runner" :class="`mock-gen-log-runner--${entry.runner}`">
                  {{ entry.runner === 'mediacodec' ? '⚡' : entry.runner === 'static' ? '📄' : '⚙' }}
                </span>
                <span class="mock-gen-log-idx">[{{ entry.index }}/{{ entry.total }}]</span>
                <span class="mock-gen-log-path">{{ entry.relativePath }}</span>
                <span class="mock-gen-log-encoder">{{ entry.encoder }}</span>
                <span v-if="entry.status === 'failed'" class="mock-gen-log-exitcode">exit={{ entry.exitCode }}</span>
                <ion-icon :icon="entry.expanded ? chevronUpOutline : chevronDownOutline" color="medium"></ion-icon>
              </div>
              <pre v-if="entry.expanded" class="mock-gen-log-detail">
<span class="lbl">ffmpeg args:</span>
{{ entry.ffmpegArgs.length > 0 ? entry.ffmpegArgs.join(' ') : '(静态字节 - 无 ffmpeg 调用)' }}
<span class="lbl">exit code:</span> {{ entry.exitCode }}
<span class="lbl">stderr:</span>
{{ entry.stderr || '(empty)' }}
<span class="lbl">at:</span> {{ entry.at }}
<!-- 🆕 修复 B1 + B2 (2026-06-17)：context 块（5 字段），失败时核心调试信息 -->
<span v-if="entry.srcSize !== undefined || entry.dstSize !== undefined || entry.workerTmpDir || entry.workerError || entry.contextInfo" class="lbl">context:</span>
<span v-if="entry.workerError">worker error: {{ entry.workerError }}
</span>
<span v-if="entry.workerTmpDir">worker tmp_dir: {{ entry.workerTmpDir }}
</span>
<span v-if="entry.srcSize !== undefined || entry.dstSize !== undefined">file sizes: src={{ entry.srcSize ?? 0 }} bytes, dst={{ entry.dstSize ?? 0 }} bytes
</span>
<span v-if="entry.contextInfo">{{ entry.contextInfo }}
</span></pre>
            </li>
          </ol>
        </div>
      </ion-list>

      <!-- ========== 工作流引擎运行器 ========== -->
      <ion-list>
        <ion-list-header>
          <ion-label>WORKFLOW ENGINE</ion-label>
        </ion-list-header>
        <p class="section-hint">加载插件后自动生成测试工作流定义，支持 DAG 编排、矩阵展开、条件执行</p>

        <ion-item button @click="handleLoadPlugins" :disabled="isLoadingPlugins">
          <ion-icon :icon="syncOutline" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.loadPlugins') }}</h3>
            <p>
              <span v-if="plugins.length > 0">{{ plugins.length }} {{ t('devtools.pluginsLoaded') }}</span>
              <span v-else>{{ t('devtools.notLoaded') }}</span>
            </p>
          </ion-label>
          <ion-spinner v-if="isLoadingPlugins" slot="end" name="dots"></ion-spinner>
        </ion-item>

        <ion-item>
          <ion-label>
            <h3>测试用例</h3>
            <p>{{ dynamicTestCases.length }} 个用例（{{ pluginCount }} 插件 × 动态笛卡尔积）</p>
          </ion-label>
        </ion-item>

        <!-- 运行控制 -->
        <ion-item
          button
          detail
          @click="handleRunWorkflow"
          :disabled="isRunning || dynamicTestCases.length === 0 || !mockGenerated"
        >
          <ion-icon :icon="playCircleOutline" slot="start" :color="mockGenerated ? 'success' : 'medium'"></ion-icon>
          <ion-label>
            <h3>Run Workflow（DAG 引擎）</h3>
            <p>
              <span v-if="!mockGenerated" style="color: var(--ion-color-danger)">⚠ 请先生成 Mock 数据</span>
              <span v-else>矩阵展开 → 依赖调度 → WS 回调驱动状态转移</span>
            </p>
          </ion-label>
        </ion-item>

        <ion-item v-if="isRunning && currentRun" button detail @click="handleCancel">
          <ion-icon :icon="closeCircleOutline" slot="start" color="danger"></ion-icon>
          <ion-label>
            <h3>Cancel Workflow</h3>
            <p>取消当前运行中的所有 Job / Step</p>
          </ion-label>
        </ion-item>

        <!-- 实时进度 -->
        <div v-if="progress.total > 0" class="progress-card">
          <ion-progress-bar :value="progress.completed / progress.total"></ion-progress-bar>
          <div class="progress-stats">
            <span>{{ progress.completed }} / {{ progress.total }}</span>
            <span class="passed">{{ progress.passed }} ✓</span>
            <span class="failed">{{ progress.failed }} ✗</span>
            <span v-if="progress.pending > 0" class="pending">{{ progress.pending }} ◌</span>
          </div>
        </div>
      </ion-list>

      <!-- ========== 测试报告 ========== -->

      <template v-if="currentRun">
        <!-- 报告头部 -->
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

        <!-- Pipeline 视图 -->
        <template v-if="viewMode === 'pipeline'">
          <JobPipelineCard
            v-for="job in currentRun.jobs"
            :key="job.id"
            :job="job"
            :step-names="stepNameMap"
            :display-name="getJobDisplayName(job.jobDefId)"
          />
        </template>

        <!-- Tree 视图 -->
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

      <!-- 历史运行 -->
      <ion-list v-if="runs.length > 1 && !currentRun">
        <ion-list-header><ion-label>PAST RUNS</ion-label></ion-list-header>
        <ion-item
          v-for="run in runs.slice(1, 11)"
          :key="run.id"
          button
          detail
          @click="currentRun = run"
        >
          <ion-label>
            <h3>{{ run.id.slice(4, 16) }}...</h3>
            <p>{{ run.status }} · {{ run.jobs.length }} jobs · {{ formatTime(run.createdAt) }}</p>
          </ion-label>
          <StepMiniBadge :status="run.status === 'running' ? 'queued' : run.status" :show-name="false" slot="end" />
        </ion-item>
      </ion-list>

    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'

// 🆕 2026-06-12 崩溃根因修复：后端 crash 完全静默 → 监听 MainActivity 推送的 window CustomEvent
//   链路：EncvGoService.sendBroadcast (Android system broadcast)
//       → MainActivity 的 android.content.BroadcastReceiver (Java 类, 不是 Capacitor plugin)
//       → bridge.webView.evaluateJavascript
//       → window.dispatchEvent(new CustomEvent('encv:backend-status', {detail:{port,running,error,...}}))
//   真实事件名是 'encv:backend-status' (MainActivity.kt:166 写死)
//   前端用 WebView 原生 window.addEventListener 订阅，**不需要任何 Capacitor plugin**。
function onBackendStatus(ev: Event) {
  const detail = (ev as CustomEvent<{ port: number; running: boolean; error?: string; source?: string }>).detail
  if (!detail) return
  const running = detail.running === true
  const error = detail.error
  if (running || !error) return  // 只在 running=false + error 有值时显示
  const raw = (error || '').toString()
  // 🆕 2026-06-12 Phase 4：go_hang（cgo ffmpeg_run 阻塞，Kotlin 端 mtime 探活触发）
  //   与 go_exit 区分：hang 是 Kotlin 主动 kill+restart；exit 是进程真退出
  const isHang = raw.startsWith('go_hang')
  const source = raw.startsWith('go_exit') || isHang || raw.startsWith('timeout') ? 'mockGenerate'
    : raw.startsWith('no_binary') ? 'loadPlugins'
    : 'loadPlugins'
  const title = isHang
    ? '后端无响应（hang 8s+），已自动重启'
    : '后端服务已退出'
  inlineError.value = {
    source,
    title,
    message: raw,
    detail: detail.port ? `port=${detail.port}` : '',
    at: Date.now(),
  }
}
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonButtons, IonBackButton,
  IonContent, IonList, IonListHeader, IonItem, IonLabel, IonIcon,
  IonSpinner, IonProgressBar,
} from '@ionic/vue'
import {
  addCircleOutline, trashOutline, syncOutline, playCircleOutline, closeCircleOutline,
  checkmarkCircleOutline, warningOutline, copyOutline, terminalOutline,
  chevronUpOutline, chevronDownOutline, ellipsisHorizontalOutline,
} from 'ionicons/icons'
import { useI18n } from '@/composables/useI18n'
import { showToast } from '@/composables/useToast'
import {
  fetchPlugins,
  type PluginMeta,
} from '@/api/encv'
import { generateMockFilesViaBackend, resetMockFilesViaBackend } from '@/api/mockGenerator'
import { extToRelativePath } from '@/lib/mockDataGenerator'
import { MOCK_GENERATE_ROOT } from '@/lib/mockConstants'
import { formatContainerVersion } from '@/constants/containerVersion'
import { useWorkflowEngine } from '@/composables/useWorkflowEngine'
import type { WorkflowDefinition, WorkflowRun, JobRun, StepRun, StepDefinition } from '@/lib/workflow/types'
import TestReportHeader from '@/components/automation/TestReportHeader.vue'
import StepMiniBadge from '@/components/automation/StepMiniBadge.vue'
import JobPipelineCard from '@/components/automation/JobPipelineCard.vue'
import TreeView from '@/components/automation/TreeView.vue'
import StepDetailPanel from '@/components/automation/StepDetailPanel.vue'

const { t } = useI18n()

// ---- Mock 数据 ----
// 🆕 2026-06-15 声明式：mockRoot = AUTOMATION_MOUNT_PATH + '/'（常量，不再 split/slice）
//   之前 .slice(0, 5) 隐式推导：DEFAULT_AUTOMATION_SOURCE 改前缀 → UI 静默选错 → 403
//   现在改 mount path = 改 src/lib/mockConstants.ts + 后端 mount.go，两个源，不会漏
// 保留 computed() 是因为下方有 `mockRoot.value` 引用，零行为变化
const mockRoot = computed(() => MOCK_GENERATE_ROOT)
const isGenerating = ref(false)
const isResetting = ref(false)
const mockStats = ref<{ count: number; totalSize: number; skipped?: number } | null>(null)
const generateProgressText = ref('')
const mockGenerated = ref(false)

// 🆕 2026-06-12 饱和调试：流程日志（每个 spec 一行，含完整 ffmpeg 诊断）
//   - 即使后端 cgo 阻塞导致 SSE 中断，最后收到的 spec_diag 也会被记录
//   - 用户可点开看 stderr / 一键复制
//   - 失败行红色高亮 + 自动展开
interface MockGenLogEntry {
  key: string
  index: number
  total: number
  relativePath: string
  status: 'ok' | 'failed' | 'pending'
  encoder: string
  /**
   * 🆕 2026-06-12：runner 标识
   *   - "ffmpeg": 软件编（沙箱 / 真机兜底）
   *   - "mediacodec": 硬件编（Phase 3.3 实装，UI 显示 ⚡）
   *   - "static": 静态字节（PNG/JPEG/PDF/TXT 等，UI 显示 📄）
   */
  runner: 'ffmpeg' | 'mediacodec' | 'static' | string
  ffmpegArgs: string[]
  exitCode: number
  stderr: string
  at: string
  expanded: boolean
  _marked?: boolean // onProgress 标记过 ok 的不重复 mark
  // 🆕 修复 B1 + B2 (2026-06-17)：增强调试字段
  srcSize?: number
  dstSize?: number
  workerTmpDir?: string
  workerError?: string
  contextInfo?: string
}
const mockGenLog = ref<MockGenLogEntry[]>([])
const mockGenLogTotal = ref(0)
const mockGenLogCopied = ref(false)
const mockGenLogSummary = computed(() => {
  const failed = mockGenLog.value.filter((e) => e.status === 'failed').length
  const ok = mockGenLog.value.filter((e) => e.status === 'ok').length
  const pending = mockGenLog.value.filter((e) => e.status === 'pending').length
  const disconnected = mockGenLogTotal.value > 0 && mockGenLog.value.length < mockGenLogTotal.value && (failed + ok) < mockGenLogTotal.value
  let text = `${ok} ✓ / ${failed} ✗ / ${pending} ◌`
  if (disconnected) text = `${text}（流中断于 ${mockGenLog.value.length}/${mockGenLogTotal.value}）`
  return { failed, ok, pending, text, disconnected }
})

// 🆕 2026-06-11 修复：内联错误卡（替代 showToast，饱和调试原则：禁用 Toast）
// 历史：用户反馈「真机 mock 生成 ffmpeg 失败 / 后端崩溃 → 弹个 toast 就没了，根本看不到」
// 旧实现：`showToast({ message: '失败: xxx', duration: 2500 })` —— 2.5 秒后消失、且 1 次只 1 行
// 新实现：内联 card 持久显示，承载：title / message / stack / 关联 action（重试/查看后端日志）
interface InlineError {
  source: 'mockGenerate' | 'mockReset' | 'loadPlugins' | 'workflowStart' | 'workflow'
  title: string
  message: string
  detail?: string  // 来自后端的 detail / stack
  hint?: string    // 排查建议
  at: number       // Date.now()，用于显示「刚刚」/「N 分钟前」
}
const inlineError = ref<InlineError | null>(null)
function setInlineError(err: Omit<InlineError, 'at'>): void {
  inlineError.value = { ...err, at: Date.now() }
  // 同步 log 到 console 便于开发期排查
  // eslint-disable-next-line no-console
  console.error('[PluginTestsDetail] inline error', err)
}
function clearInlineError(): void {
  inlineError.value = null
}

// ---- 插件 & 用例 ----
const plugins = ref<PluginMeta[]>([])
const isLoadingPlugins = ref(false)
const dynamicTestCases = ref<any[]>([])
const pluginCount = computed(() => plugins.value.length)

// ---- 工作流引擎 ----
const engine = useWorkflowEngine()
const {
  definitions: wfDefs,
  runs,
  currentRun,
  isRunning,
  totalSteps,
  completedSteps,
  successSteps,
  failedSteps,
  startListening: wsStart,
  stopListening: wsStop,
} = engine

const viewMode = ref<'pipeline' | 'tree'>('pipeline')
const selectedStep = ref<StepRun | null>(null)
const _tickNow = ref(Date.now())
let tickHandle: ReturnType<typeof setInterval> | null = null

const platform = computed(() => {
  if (typeof navigator === 'undefined') return 'node'
  const ua = navigator.userAgent || ''
  if (/android/i.test(ua)) return 'android'
  if (/iphone|ipad|ipod/i.test(ua)) return 'ios'
  return 'web'
})

const reportDurationMs = computed(() => {
  if (!currentRun.value) return 0
  if (isRunning.value) return _tickNow.value - (currentRun.value.startedAt ? new Date(currentRun.value.startedAt).getTime() : Date.now())
  return currentRun.value.durationMs ?? 0
})

// 兼容旧接口名
const progress = computed(() => ({
  total: totalSteps.value,
  completed: completedSteps.value,
  passed: successSteps.value,
  failed: failedSteps.value,
  pending: Math.max(0, totalSteps.value - completedSteps.value),
}))

/** 从当前运行的 workflow definition 构建 step 名映射 */
const stepNameMap = computed(() => {
  const map = new Map<string, string>()
  const def = currentRun.value
    ? wfDefs.value.find((d: WorkflowDefinition) => d.id === currentRun.value!.workflowDefId)
    : null
  if (def) {
    for (const job of def.jobs) {
      for (const step of job.steps) {
        map.set(step.id, step.name)
      }
    }
  }
  // 如果没有 definition（历史运行），从 stepDefId 推断名称
  if (map.size === 0 && currentRun.value) {
    for (const job of currentRun.value.jobs) {
      for (const step of job.steps) {
        if (!map.has(step.stepDefId)) {
          map.set(step.stepDefId, step.stepDefId)
        }
      }
    }
  }
  return map
})

const jobDisplayNameMap = computed(() => {
  const map = new Map<string, string>()
  const def = currentRun.value
    ? wfDefs.value.find((d: WorkflowDefinition) => d.id === currentRun.value!.workflowDefId)
    : null
  if (def) {
    for (const job of def.jobs) map.set(job.id, job.name)
  }
  return map
})

function getJobDisplayName(jobDefId: string): string {
  return jobDisplayNameMap.value.get(jobDefId) ?? jobDefId
}

function findJobForStep(run: WorkflowRun, step: StepRun): JobRun | undefined {
  return run.jobs.find((j: JobRun) => j.steps.some((s: StepRun) => s.id === step.id))
}

const selectedStepJob = computed(() =>
  currentRun.value && selectedStep.value
    ? findJobForStep(currentRun.value, selectedStep.value)
    : null,
)

// ---- Handlers ----

async function handleGenerateMock() {
  if (isGenerating.value) return
  isGenerating.value = true
  generateProgressText.value = ''
  mockStats.value = null
  // 🆕 2026-06-12 饱和调试：清空 + 准备流程日志
  mockGenLog.value = []
  mockGenLogTotal.value = 0
  mockGenLogCopied.value = false
  let lastCount = 0
  let lastSize = 0
  // 🆕 2026-06-11 v4：跟踪被跳过的文件（real device 上 ffmpeg 没编 mp3/flac encoder 常见）
  const skippedFiles: { relativePath: string; reason: string; exitCode: number; stderr: string }[] = []
  try {
    const result = await generateMockFilesViaBackend({
      root: mockRoot.value,
      type: 'all',
      // 🆕 v4：30s 硬超时。后端 hang 时主动 abort → catch 块 → inline error UI
      // 历史 bug：real device 偶发 cgo dlopen 阻塞 → gin SSE 不响应 → spinner 永远转 → 体感"崩溃"
      //
      // 🆕 2026-06-11 Phase 2：真机走 subprocess ffmpeg-worker（cgo 在 worker 内部），
      // 父进程 ctx cancel 时 Go exec.CommandContext 默认 SIGKILL worker 进程（Go 1.20+），
      // 因此 30s 硬超时更多是兜底，理论上不会再触发。
      // 但**仍保留**：如果 worker 启动慢 / SIGKILL 在 cgo OS thread 卡住内核调度，
      // 前端 abort 至少断 SSE 让用户看到错误（不再 spinner 永远）。
      timeoutMs: 30000,
      onSpecDiag: (diag) => {
        // 🆕 2026-06-12 饱和调试：每个 spec 处理前先记一行
        //   哪怕 progress 事件因 cgo 阻塞没收到，至少能看到「处理到这步」
        //   关键：用 relativePath + index 找已有 row（spec_plan 时已 push pending），
        //         替换为完整诊断版（status / stderr / exitCode）
        //   真机 cgo 阻塞时只有 plan 行（pending），诊断版（ok/failed）永远到不了 → 前端仍能看到 pending 行
        if (diag.relativePath === '__starting__') {
          // starting 事件：更新 total 即可
          mockGenLogTotal.value = diag.total
          return
        }
        // 找同 relativePath 已有 row（plan 阶段 push 过）
        const existing = mockGenLog.value.findIndex((e) => e.relativePath === diag.relativePath && e.index === diag.index)
        const entry: MockGenLogEntry = {
          key: `${diag.index}-${diag.relativePath}-${diag.status}`,
          index: diag.index,
          total: diag.total,
          relativePath: diag.relativePath,
          status: diag.status,
          encoder: diag.encoder,
          runner: diag.runner, // 🆕 2026-06-12
          ffmpegArgs: diag.ffmpegArgs,
          exitCode: diag.exitCode,
          stderr: diag.stderr,
          at: new Date().toISOString(),
          expanded: diag.status === 'failed', // 失败自动展开
          // 🆕 修复 B1 + B2 (2026-06-17)：保留 5 字段到 entry（UI 渲染 context 块用）
          srcSize: diag.srcSize,
          dstSize: diag.dstSize,
          workerTmpDir: diag.workerTmpDir,
          workerError: diag.workerError,
          contextInfo: diag.contextInfo,
        }
        if (existing >= 0) {
          mockGenLog.value[existing] = entry
        } else {
          mockGenLog.value.push(entry)
        }
        generateProgressText.value = `[${diag.index}/${diag.total}] ${diag.relativePath} (${diag.status})`
      },
      onSpecPlan: (diag) => {
        // 🆕 2026-06-12 饱和调试：handler 入口发的"待跑"列表（pending 状态）
        //   真机 cgo 阻塞时只有这些行能到达 → 前端能定位"卡在哪个 spec"
        if (diag.relativePath === '__starting__') {
          mockGenLogTotal.value = diag.total
          return
        }
        // 找同 relativePath 已有 row，避免重复 push
        const existing = mockGenLog.value.findIndex((e) => e.relativePath === diag.relativePath && e.index === diag.index)
        const entry: MockGenLogEntry = {
          key: `${diag.index}-${diag.relativePath}-plan`,
          index: diag.index,
          total: diag.total,
          relativePath: diag.relativePath,
          status: 'pending',
          encoder: diag.encoder,
          runner: diag.runner, // 🆕 2026-06-12
          ffmpegArgs: diag.ffmpegArgs,
          exitCode: 0,
          stderr: '',
          at: new Date().toISOString(),
          expanded: false,
        }
        if (existing >= 0) {
          // 保留已有行（plan 后已被 diag 替换过），不动
        } else {
          mockGenLog.value.push(entry)
        }
        mockGenLogTotal.value = diag.total
      },
      onProgress: (p) => {
        lastCount++
        lastSize += p.size
        generateProgressText.value = `(${lastCount}) ${p.relativePath}`
        // 🆕 2026-06-12：把对应 spec_diag 行标记为 ok
        const e = mockGenLog.value.find((e) => e.relativePath === p.relativePath && e.status === 'ok' && e.exitCode === 0 && !e._marked)
        if (e) {
          e._marked = true
        }
      },
      onSpecFailed: (fail) => {
        // 🆕 2026-06-12 饱和调试：spec 失败带完整 ffmpeg 诊断
        skippedFiles.push({ relativePath: fail.relativePath, reason: fail.reason, exitCode: fail.exitCode, stderr: fail.stderr })
        // 找对应的 spec_diag 行，更新状态 + 附加 stderr
        const e = mockGenLog.value.find((e) => e.relativePath === fail.relativePath && e.status !== 'failed')
        if (e) {
          e.status = 'failed'
          e.exitCode = fail.exitCode
          e.stderr = fail.stderr || e.stderr
          e.expanded = true // 自动展开失败行
        }
        generateProgressText.value = `⚠️ 失败 ${fail.relativePath} (exit=${fail.exitCode})`
        console.warn('[mock-gen] spec failed', fail)
      },
      onSkipped: (info) => {
        skippedFiles.push({ relativePath: info.relativePath, reason: info.reason, exitCode: -1, stderr: '' })
        generateProgressText.value = `⚠️ 跳过 ${info.relativePath}（${info.reason}）`
        console.warn('[mock-gen] skipped', info)
      },
    })
    mockStats.value = { count: result.count || lastCount, totalSize: result.totalSize || lastSize, skipped: result.skipped ?? skippedFiles.length }
    mockGenerated.value = true
    // 🆕 v4：如果有 skipped 文件，inline error card 显示（warning 风格而非 error）
    if (result.skipped > 0 || skippedFiles.length > 0) {
      const reasonList = skippedFiles.map((s) => {
        const tail = s.stderr ? `\n     stderr: ${s.stderr.split('\n')[0]}` : ''
        return `  - ${s.relativePath} (exit=${s.exitCode}): ${s.reason}${tail}`
      }).join('\n')
      setInlineError({
        source: 'mockGenerate',
        title: `Mock 生成完成（${result.count} 成功 / ${result.skipped} 跳过）`,
        message: `以下 ${result.skipped} 个文件因 ffmpeg build 限制被跳过（real device 常见：mp3/flac encoder 未编入 libffmpeg.so）：\n${reasonList}`,
        hint: '此为 warning，不是 fatal error。mp4/mkv 仍可用。继续跑自动化测试可只勾选支持格式。下方「FFMPEG 流程日志」可点开看完整 stderr / 复制。',
      })
    } else {
      // 🆕 2026-06-12：success 时不弹 toast（饱和调试原则），让流程日志卡展示全部 ✓
      // 历史：toast 2.5s 一闪就消失，用户看不到「生成了 9 个文件全 ok」的确认
    }
  } catch (e) {
    // 🆕 2026-06-11 修复：用 inline error card 替代 toast
    // 历史：真机 mock 生成 ffmpeg 失败 + 后端崩溃 → toast 2.5s 一闪就消失，用户看不到根因
    const errMsg = e instanceof Error ? e.message : String(e)
    const classified = classifyMockError(errMsg)
    // 🆕 2026-06-12：把"已收到但流中断"的 diag 也带过去，前端可显示「在 N/M 处停止」
    const lastDiag = mockGenLog.value[mockGenLog.value.length - 1]
    const stopHint = lastDiag
      ? `\n\n📍 最后收到 spec_diag：[${lastDiag.index}/${lastDiag.total}] ${lastDiag.relativePath}\n   ffmpeg 调了：${lastDiag.ffmpegArgs.join(' ') || '(无)'}\n   exit code：${lastDiag.exitCode}\n   stderr：${lastDiag.stderr || '(empty)'}`
      : ''
    setInlineError({
      source: 'mockGenerate',
      title: classified.title,
      message: errMsg + stopHint,
      hint: classified.hint,
    })
    // 不弹 toast（饱和调试原则：禁用 Toast），错误卡已持久显示
  } finally {
    isGenerating.value = false
    generateProgressText.value = ''
  }
}

// ---- 🆕 2026-06-12 饱和调试：流程日志卡操作 ----

function toggleMockGenLogEntry(key: string) {
  const e = mockGenLog.value.find((e) => e.key === key)
  if (e) e.expanded = !e.expanded
}

function copyMockGenLog() {
  if (mockGenLog.value.length === 0) return
  const lines: string[] = []
  lines.push(`# ENCV Mock 生成流程日志`)
  lines.push(`# at: ${new Date().toISOString()}`)
  lines.push(`# total: ${mockGenLogTotal.value}`)
  lines.push(`# entries: ${mockGenLog.value.length}`)
  lines.push(`# root: ${mockRoot.value}`)
  lines.push(``)
  for (const e of mockGenLog.value) {
    const status = e.status === 'ok' ? '✓' : e.status === 'failed' ? '✗' : '◌'
    lines.push(`[${status}] [${e.index}/${e.total}] ${e.relativePath}`)
    lines.push(`    runner: ${e.runner}  (mediacodec=硬件⚡ / ffmpeg=软件⚙ / static=静态📄)`)
    lines.push(`    encoder: ${e.encoder}`)
    lines.push(`    ffmpeg args: ${e.ffmpegArgs.length > 0 ? e.ffmpegArgs.join(' ') : '(静态字节 - 无 ffmpeg 调用)'}`)
    lines.push(`    exit code: ${e.exitCode}`)
    lines.push(`    at: ${e.at}`)
    // 🆕 修复 B1 + B2 (2026-06-17)：复制时附带 context 信息（便于问题反馈时粘贴到 issue）
    if (e.workerTmpDir) lines.push(`    worker tmp_dir: ${e.workerTmpDir}`)
    if (e.workerError) lines.push(`    worker error: ${e.workerError}`)
    if (e.srcSize !== undefined || e.dstSize !== undefined) {
      lines.push(`    file sizes: src=${e.srcSize ?? 0} bytes, dst=${e.dstSize ?? 0} bytes`)
    }
    if (e.contextInfo) lines.push(`    context: ${e.contextInfo}`)
    if (e.stderr) {
      lines.push(`    stderr:`)
      for (const ln of e.stderr.split('\n')) lines.push(`      ${ln}`)
    }
    lines.push(``)
  }
  const text = lines.join('\n')
  navigator.clipboard?.writeText(text).then(() => {
    mockGenLogCopied.value = true
    setTimeout(() => { mockGenLogCopied.value = false }, 2000)
  }).catch((e) => {
    console.error('[mock-gen] copy failed', e)
    // fallback: 弹 prompt 让用户手动复制
    window.prompt('复制以下日志', text)
  })
}

// classifyMockError 把后端 throw 出来的错误分类，给出精确的排查 hint。
// 后端错误源（cmd/ffmpeg-worker/ + internal/server/mock_generator.go）：
//   - "[ENGINE_LOAD_FAILED] ..."        cgo dlopen libffmpeg.so 失败（路径/架构错）
//   - "[ENGINE_SYMBOL_MISSING] ..."     libffmpeg.so 缺 ffmpeg_run symbol（没编进 ffmpeg_run_main）
//   - "[ENGINE_EXIT_ERROR] ..."         ffmpeg_run 内部 exit_code != 0
//   - "ffmpeg worker exit 124"          worker 软超时（worker main_android.go 自爆）
//   - "ffmpeg worker reported: ..."     worker 报告的 stderr/stdout 错误
//   - "start worker: ..."               worker binary 启动失败（binary 不在 / 权限不够）
//   - "ffmpeg-worker binary not found"  locateWorker() 找不到 worker binary
//   - "context canceled" / "timeout"    前端 30s AbortController 触发
//   - "502/503/504/connection refused"  后端崩溃 / mock_generator panic
//   - "root ... is not in allowlist"    mockRoot 路径不在后端白名单
//
// 2026-06-11 Phase 2: 增加 worker 错误分类。
function classifyMockError(errMsg: string): { title: string; hint: string } {
  // 1. ffmpeg worker 启动失败（worker binary 找不到 / 不能 exec）
  if (/ffmpeg-worker binary not found/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：ffmpeg-worker 未找到',
      hint: '真机 Kotlin 端未把 libffmpeg-worker.so 打到 jniLibs/arm64-v8a/，或 ENCV_FFMPEG_WORKER 未设置。\n\n排查：\n  1) 确认 jniLibs/arm64-v8a/libffmpeg-worker.so 存在（应跟 libencv-go.so / libffmpeg.so 一起）\n  2) adb logcat | grep EncvGoService 看启动时 ENCV_FFMPEG_WORKER 实际值\n  3) 沙箱可绕过：用 ExecRunner 直接调系统 ffmpeg（确认 worker 模式正常后）',
    }
  }
  if (/start worker:/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：worker 启动失败',
      hint: 'ffmpeg-worker binary 找到了但启动失败（权限/架构/链接器错误）。\n\n排查：\n  1) adb shell ls -l $ENCV_FFMPEG_WORKER 看是否可执行\n  2) adb shell chmod +x $ENCV_FFMPEG_WORKER  必要时手动加执行位\n  3) adb logcat | grep ffmpeg-worker 看 stderr 详细错误',
    }
  }
  // 2. cgo dlopen libffmpeg.so 失败
  if (/\[ENGINE_LOAD_FAILED\]/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：cgo 加载 libffmpeg.so 失败',
      hint: 'worker 内部 cgo dlopen libffmpeg.so 失败（路径错/架构错/missing lib）。\n\n排查：\n  1) adb shell ls $ENCV_LIB_DIR/libffmpeg.so 是否存在\n  2) adb shell file $ENCV_LIB_DIR/libffmpeg.so 确认是 ARM aarch64\n  3) 重新 build ffmpeg：bash app/encv-mobile/scripts/build-ffmpeg-android.sh',
    }
  }
  // 3. libffmpeg.so 缺 ffmpeg_run symbol（build 时没编 ffmpeg_run_main.c）
  if (/\[ENGINE_SYMBOL_MISSING\]/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：libffmpeg.so 缺 ffmpeg_run symbol',
      hint: 'libffmpeg.so 存在但没编 ffmpeg_run_main() 入口。\n\n排查：\n  1) 重新 build ffmpeg：bash app/encv-mobile/scripts/build-ffmpeg-android.sh\n  2) 确认 build 脚本中 --enable-ffmpeg_run_main 之类选项\n  3) ffmpeg 4.x 之前 ffmpeg_run 是 main()；5.x 之后需 --extra-cflags="-DFFMPEG_RUN_MAIN=1"',
    }
  }
  // 4. ffmpeg 内部 exit_code != 0（mp4 转码失败 / 文件权限 / encoder 不支持）
  if (/\[ENGINE_EXIT_ERROR\]/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：ffmpeg_run 内部错误',
      hint: 'ffmpeg 执行失败（exit code != 0）。常见原因：\n  - input file 不可读\n  - encoder 不支持（真机可能没编 libx264/libmp3lame/flac）\n  - output path 无写权限\n\n排查：adb logcat | grep ffmpeg-worker 看完整 stderr',
    }
  }
  // 5. worker 软超时
  if (/exit code? 124|ffmpeg worker exit 124|soft timeout/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：ffmpeg 单次执行超时',
      hint: 'worker 内部 timeoutMs 软超时触发（self-exit 124）。cgo ffmpeg_run 阻塞超过 ctx deadline。\n\n排查：\n  1) 检查 input file 是否太大/有问题\n  2) 增加 mock_generator.go 的 ctx timeout（默认 30s）\n  3) worker 自身 SIGKILL 不需要软超时兜底，前端 abort 即可',
    }
  }
  // 6. 通用 worker reported 错误（兜底）
  if (/ffmpeg worker reported/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：ffmpeg worker 报告错误',
      hint: 'worker 进程返回了错误但分类没匹配上。检查 adb logcat | grep ffmpeg-worker 完整 stderr。\n\n错误格式：[ENGINE_*] 开头的错误码对应该分类。',
    }
  }
  // 7. 前端 abort / context canceled
  if (/context canceled|abort|timeout/i.test(errMsg)) {
    return {
      title: 'Mock 生成超时（30s）',
      hint: '超过 30s 未完成。可能原因：\n  - 父进程 ctx cancel 触发 worker SIGKILL（最常见，Phase 2 之后）\n  - 父进程 mockGenMu 串行化导致排队（10 并发时）\n  - 后端 cgo OS thread 死锁（极少）\n\n排查：adb logcat | grep encv-go | grep mock',
    }
  }
  // 8. 后端崩溃
  if (/502|503|504|connection.*refused|network.*error/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：后端无响应',
      hint: '后端进程可能已崩溃（502/网络拒绝）。\n\n排查：\n  1) adb logcat | grep encv-go | tail -200\n  2) 真机：开发者选项里重启 backend service\n  3) 沙箱：pm2 logs encv-go 看 panic stack',
    }
  }
  // 9. mockRoot 路径不在白名单（老 allowlist 错误，保留兼容）
  if (/not in allowlist/i.test(errMsg)) {
    return {
      title: 'Mock 生成失败：路径不在白名单',
      hint: '后端 servingDir 校验拒绝。mockRoot 必须是白名单前缀（如 /storage/emulated/0/encv-automation）。\n\n排查：检查 settings.user.json 的 mockRoot 配置。',
    }
  }
  // 🆕 2026-06-15 multi-mount：mount 路径解析失败（最常见）
  //   后端响应：{error: "resolve \"/d/automation/...\"...available mounts: [primary→/d/primary, ...]"}
  if (/invalid mount path|resolve.*no mount matches|available mounts/i.test(errMsg)) {
    // 提取 available_mounts 字段
    const availMatch = errMsg.match(/available mounts:\s*\[([^\]]*)\]/)
    const availList = availMatch?.[1]?.trim() || '(unknown — 见后端日志)'
    return {
      title: 'Mock 生成失败：mockRoot 不是有效 mount 路径',
      hint:
        `后端 mount registry 找不到 mockRoot。\n\n` +
        `当前可用 mount：[${availList}]\n\n` +
        `常见 bug：mockRoot 派生用了字符串切片（fragile）\n` +
        `  - 应使用 MOCK_GENERATE_ROOT 声明式常量（src/lib/mockConstants.ts）\n` +
        `  - 错误示例：mockRoot = "/d/automation/01-plain-media/video/"（取多了）\n` +
        `  - 正确示例：mockRoot = "/d/automation/"（mount 根）\n\n` +
        `排查：\n` +
        `  1) WorkflowDashboard.vue L201 / PluginTestsDetail.vue L373 → 改 N=3\n` +
        `  2) 后端 slog：grep "Mock generate rejected" /workspace/encv.log`,
    }
  }
  // 10. 兜底
  return {
    title: 'Mock 数据生成失败',
    hint: '检查 mockRoot 路径权限 / 后端 SSE 响应 / 后端 mock_generator.go 日志（pm2 logs encv-go 或 adb logcat）',
  }
}

async function handleResetMock() {
  if (isResetting.value) return
  isResetting.value = true
  try {
    const r = await resetMockFilesViaBackend(mockRoot.value)
    mockStats.value = null
    mockGenerated.value = false
    showToast({ message: `Reset: ${r.removed} files`, color: 'success', duration: 1500 })
  } catch (e) {
    // 🆕 2026-06-11 修复：inline error card
    setInlineError({
      source: 'mockReset',
      title: 'Mock 数据重置失败',
      message: e instanceof Error ? e.message : String(e),
      hint: '检查 5 个 mock 目录权限（01-plain-media / 02-alist-encrypt / 03-encv-containers / 04-boundary-test / 02-test-output）',
    })
  } finally {
    isResetting.value = false
  }
}

async function handleLoadPlugins() {
  isLoadingPlugins.value = true
  try {
    plugins.value = await fetchPlugins()
    // 自动构建动态测试用例 + 工作流定义
    buildDynamicWorkflow()
    showToast({ message: `${plugins.value.length} plugins, ${dynamicTestCases.value.length} cases`, color: 'success', duration: 1500 })
  } catch (e) {
    // 🆕 2026-06-11 修复：inline error card
    setInlineError({
      source: 'loadPlugins',
      title: '加载插件失败',
      message: e instanceof Error ? e.message : String(e),
      hint: '检查后端 /api/plugins 是否 200；plugin 元数据是否含 supportedExtensions/taskOptions',
    })
  } finally {
    isLoadingPlugins.value = false
  }
}

/**
 * 把 ext 映射到 mock 目录分类
 *   mp4/mkv/avi/mov → 'video'
 *   mp3/flac/ogg/m4a/wav → 'audio'
 *   png/jpg/jpeg/gif/webp → 'image'
 *   pdf → 'pdf'
 *   doc/docx/xls/xlsx/ppt/pptx → 'wps'
 *   txt/md → 'text'
 *   encv → 'alist-encrypted'
 */
function categoryForExt(ext: string): string {
  const e = ext.toLowerCase().replace(/^\./, '')
  if (['mp4', 'mkv', 'avi', 'mov', 'webm', 'flv', 'wmv'].includes(e)) return 'video'
  if (['mp3', 'flac', 'ogg', 'm4a', 'wav', 'aac', 'opus'].includes(e)) return 'audio'
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'tiff'].includes(e)) return 'image'
  if (['pdf'].includes(e)) return 'pdf'
  if (['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx'].includes(e)) return 'wps'
  if (['txt', 'md', 'rtf', 'log'].includes(e)) return 'text'
  if (['encv', 'ae'].includes(e)) return 'alist-encrypted'
  return 'misc'
}

/**
 * 根据已加载的插件，动态构建 WorkflowDefinition。
 * 这是核心：把旧的「硬编码笛卡尔积」升级为 DAG 工作流定义，
 * 然后通过引擎的 runWorkflow() 执行。
 *
 * 🆕 2026-06-10 重构 v3：拆 2 个 job + 唯一子目录
 *   - job1: `encrypt-all` (parallel max:5) — 所有 encrypt step
 *     targetPath = `${mockRoot}02-test-output/${safeId}/`
 *   - job2: `decrypt-all` (parallel max:5, needs: ['encrypt-all']) — 解密 encrypt 的产物
 *     sourcePath = `${mockRoot}02-test-output/${safeId}/sample.${sourceExt}.${plugin.containerExtension}`
 *   - safeId = plugin_v{version}_{ext}_{k1=v1_k2=v2...}（特殊字符替换为 _）
 *   - 同一 safeId 唯一 → 同一组 encrypt+decrypt 在同一子目录，产物不冲突
 */
function buildDynamicWorkflow(): void {
  if (plugins.value.length === 0) {
    dynamicTestCases.value = []
    return
  }

  const encryptSteps: StepDefinition[] = []
  const decryptSteps: StepDefinition[] = []

  for (const plugin of plugins.value) {
    const opts = plugin.taskOptions
    if (!opts) continue

    // 🆕 2026-06-15 修 #4：按 plugin.supportedExtensions[0] + mock spec 实际路径派生 sourcePath
    // （不再硬编码 sample.{ext}，跟 mock 真相对齐：mp3→music.mp3 / mkv→comedy.mkv / jpg→photo.jpg）
    const supportedExts = plugin.supportedExtensions ?? []
    if (supportedExts.length === 0) continue
    const sourceExt = supportedExts[0]
    const specRelPath = extToRelativePath(sourceExt)
    const sourcePath = specRelPath
      ? `${mockRoot.value}${specRelPath}`
      : `${mockRoot.value}01-plain-media/${categoryForExt(sourceExt)}/sample.${sourceExt}`

    const versions: number[] = opts.supportVersionSelect && opts.supportedVersions
      ? opts.supportedVersions
      : [opts.defaultVersion]

    // ====== 修 #5：遍历 plugin.taskOptions.ExtraFields ======
    const selectFields: { field: any; values: string[] }[] = []
    const boolFields: { field: any }[] = []
    for (const f of opts.extraFields ?? []) {
      if (f.type === 'select' && Array.isArray(f.options) && f.options.length > 1) {
        selectFields.push({ field: f, values: f.options })
      } else if (f.type === 'bool') {
        boolFields.push({ field: f })
      }
    }

    for (const version of versions) {
      // encrypt / decrypt 各自的 ExtraFields
      const encryptSelectFields = selectFields.filter(
        (sf) => !sf.field.condition || sf.field.condition === 'encrypt',
      )
      const encryptBoolFields = boolFields.filter(
        (bf) => !bf.field.condition || bf.field.condition === 'encrypt',
      )
      const decryptSelectFields = selectFields.filter(
        (sf) => !sf.field.condition || sf.field.condition === 'decrypt',
      )
      const decryptBoolFields = boolFields.filter(
        (bf) => !bf.field.condition || bf.field.condition === 'decrypt',
      )

      // encrypt 笛卡尔积展开
      const encryptSelectCombos = cartesianExpand(
        encryptSelectFields.map((sf) => sf.values),
      )
      const encryptBoolCombos: boolean[][] = []
      if (encryptBoolFields.length === 0) {
        encryptBoolCombos.push([])
      } else {
        const n = encryptBoolFields.length
        for (let mask = 0; mask < 1 << n; mask++) {
          encryptBoolCombos.push(Array.from({ length: n }, (_, i) => Boolean(mask & (1 << i))))
        }
      }

      // decrypt 笛卡尔积展开
      const decryptSelectCombos = cartesianExpand(
        decryptSelectFields.map((sf) => sf.values),
      )
      const decryptBoolCombos: boolean[][] = []
      if (decryptBoolFields.length === 0) {
        decryptBoolCombos.push([])
      } else {
        const n = decryptBoolFields.length
        for (let mask = 0; mask < 1 << n; mask++) {
          decryptBoolCombos.push(Array.from({ length: n }, (_, i) => Boolean(mask & (1 << i))))
        }
      }

      // 🆕 安全 ID 工具：把 plugin + version + ext + extraFields 转成文件系统安全的子目录名
      const makeSafeId = (extraFields: Record<string, string>): string => {
        const sortedKeys = Object.keys(extraFields).sort()
        const parts: string[] = [plugin.name, formatContainerVersion(version), sourceExt]
        for (const k of sortedKeys) {
          parts.push(`${k}-${extraFields[k]}`)
        }
        return parts.join('_').replace(/[^\w.-]/g, '_').replace(/_+/g, '_')
      }

      // ============== Encrypt 步骤生成 ==============
      for (const selectCombo of encryptSelectCombos) {
        for (const boolCombo of encryptBoolCombos) {
          const extraFields: Record<string, string> = {}
          encryptSelectFields.forEach((sf, i) => {
            const val = selectCombo[i]
            if (val !== undefined) extraFields[sf.field.key] = val
          })
          encryptBoolFields.forEach((bf, i) => {
            extraFields[bf.field.key] = boolCombo[i] ? 'true' : 'false'
          })

          const safeId = makeSafeId(extraFields)
          // 🆕 修 #5：每个 safeId 唯一子目录 → 多次加密不会互相覆盖
          const targetPath = `${mockRoot.value}02-test-output/${safeId}/`

          const stepId = `enc_${safeId}`
          const nameParts: string[] = [plugin.name, 'ENCRYPT', formatContainerVersion(version), sourceExt]
          for (const sf of encryptSelectFields) {
            const v = extraFields[sf.field.key]
            if (v) {
              const label = sf.field.optionLabels?.[v] ?? v
              nameParts.push(`${sf.field.key}=${label}`)
            }
          }
          for (const bf of encryptBoolFields) {
            const v = extraFields[bf.field.key]
            if (v) nameParts.push(`${bf.field.key}=${v}`)
          }

          encryptSteps.push({
            id: stepId,
            name: nameParts.join(' · '),
            action: {
              type: 'encv_task',
              taskType: 'encrypt',
              pluginName: plugin.name,
              params: {
                sourcePath,
                targetPath,
                password: 'automation-test-pwd',
                version,
                extraFields: Object.keys(extraFields).length > 0 ? extraFields : undefined,
              },
            },
          })

          dynamicTestCases.value.push({
            id: stepId,
            phase: 'encrypt',
            pluginName: plugin.name,
            taskType: 'encrypt',
            version,
            sourcePath,
            sourceExt,
            targetPath,
            safeId,
            extraFields: { ...extraFields },
          })
        }
      }

      // ============== Decrypt 步骤生成（依赖 encrypt-all 完成后） ==============
      for (const selectCombo of decryptSelectCombos) {
        for (const boolCombo of decryptBoolCombos) {
          const extraFields: Record<string, string> = {}
          decryptSelectFields.forEach((sf, i) => {
            const val = selectCombo[i]
            if (val !== undefined) extraFields[sf.field.key] = val
          })
          decryptBoolFields.forEach((bf, i) => {
            extraFields[bf.field.key] = boolCombo[i] ? 'true' : 'false'
          })

          // 复用 encrypt 阶段产物的 safeId（decrypt 只读，不写入）
          // decrypt 自己的 extraFields 不影响产物路径 → 沿用 plugin+version+ext 作为子目录
          // 但 decrypt 的 extraFields 会影响解密参数（如果 plugin decrypt 需要）
          // 安全做法：decrypt 用 {plugin+version+ext} 作为目录基础，+ dec- 前缀
          const baseSafeId = makeSafeId({})
          const safeId = `dec_${baseSafeId}` + (Object.keys(extraFields).length > 0
            ? '_' + Object.keys(extraFields).sort().map(k => `${k}-${extraFields[k]}`).join('_').replace(/[^\w.-]/g, '_')
            : '')

          // 🆕 修 #5：解密读 encrypt 阶段写出的产物
          // encrypt 步骤把 spec.relativePath basename 加密成 basename + containerExt
          // 加密后文件名由 plugin 内部决定 → 后端 outputExt = ext + containerExt
          // 之前硬编码 sample.${sourceExt}.${containerExt} → 对 mp3/mkv/jpg 等 mock 实际名不一致 → "文件不存在"
          // ⚠️ plugin.containerExtension 是后端从 plugin.GetContainerExtension() 返回的权威值
          //    不允许任何硬编码 fallback（用户原则：任何硬编码都错）
          if (!plugin.containerExtension) {
            throw new Error(`Plugin ${plugin.name} 缺少 containerExtension（后端 plugin.GetContainerExtension() 返回空）`)
          }
          const containerExt = plugin.containerExtension
          const sourceBasename = sourcePath.split('/').pop() ?? `sample.${sourceExt}`
          const encryptedFileName = `${sourceBasename}.${containerExt}`
          const sourcePathForDecrypt = `${mockRoot.value}02-test-output/${baseSafeId}/${encryptedFileName}`

          const stepId = `dec_${safeId.replace(/^dec_/, '')}`
          const nameParts: string[] = [plugin.name, 'DECRYPT', formatContainerVersion(version), sourceExt]
          for (const sf of decryptSelectFields) {
            const v = extraFields[sf.field.key]
            if (v) {
              const label = sf.field.optionLabels?.[v] ?? v
              nameParts.push(`${sf.field.key}=${label}`)
            }
          }
          for (const bf of decryptBoolFields) {
            const v = extraFields[bf.field.key]
            if (v) nameParts.push(`${bf.field.key}=${v}`)
          }

          decryptSteps.push({
            id: stepId,
            name: nameParts.join(' · '),
            action: {
              type: 'encv_task',
              taskType: 'decrypt',
              pluginName: plugin.name,
              params: {
                sourcePath: sourcePathForDecrypt,
                // decrypt 写到原 01-plain-media 旁边（用 .dec.{ext} 后缀避免覆盖原文件）
                targetPath: `${mockRoot.value}02-test-output/${safeId}/`,
                password: 'automation-test-pwd',
                version,
                extraFields: Object.keys(extraFields).length > 0 ? extraFields : undefined,
              },
            },
          })

          dynamicTestCases.value.push({
            id: stepId,
            phase: 'decrypt',
            pluginName: plugin.name,
            taskType: 'decrypt',
            version,
            sourcePath: sourcePathForDecrypt,
            sourceExt,
            targetPath: `${mockRoot.value}02-test-output/${safeId}/`,
            safeId,
            extraFields: { ...extraFields },
          })
        }
      }
    }
  }

  // 构建或更新工作流定义
  const existingIdx = wfDefs.value.findIndex((d) => d.id === 'dynamic-auto-test')
  const wfDef: WorkflowDefinition = {
    id: 'dynamic-auto-test',
    name: '自动化测试套件（动态）',
    description: `${plugins.value.length} 插件 × 源扩展名 × 版本 × 加密选项笛卡尔积
（encrypt-all 全部并行 → decrypt-all 等 encrypt 完成后并行）`,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    trigger: 'manual',
    env: { PASSWORD: 'automation-test-pwd' },
    jobs: [
      // 🆕 DAG 拆 2 个 job
      {
        id: 'encrypt-all',
        name: '🔒 Encrypt All (parallel)',
        strategy: { type: 'parallel', max: 5 },
        steps: encryptSteps,
      },
      {
        id: 'decrypt-all',
        name: '🔓 Decrypt All (parallel, after encrypt-all)',
        needs: ['encrypt-all'],  // 🆕 关键：DAG 依赖
        strategy: { type: 'parallel', max: 5 },
        steps: decryptSteps,
      },
    ],
  }

  if (existingIdx >= 0) {
    engine.updateDefinition('dynamic-auto-test', wfDef)
  } else {
    engine.createDefinition(wfDef)
  }
}

/** 笛卡尔积展开：输入 [[1,2],[a,b,c]] → 输出 [[1,a],[1,b],[1,c],[2,a],[2,b],[2,c]] */
function cartesianExpand(arrays: string[][]): string[][] {
  if (arrays.length === 0) return [[]]
  if (arrays.some((a) => a.length === 0)) return [[]]
  return arrays.reduce<string[][]>(
    (acc, curr) => acc.flatMap((a) => curr.map((c) => [...a, c])),
    [[]],
  )
}

async function handleRunWorkflow() {
  if (isRunning.value || dynamicTestCases.value.length === 0) return
  if (!mockGenerated.value) {
    showToast({ message: '请先生成 Mock 数据！', color: 'warning', duration: 2000 })
    return
  }

  try {
    await engine.runWorkflow('dynamic-auto-test', 'automation')
    showToast({
      message: `Workflow started: ${dynamicTestCases.value.length} steps`,
      color: 'success',
      duration: 1500,
    })
  } catch (e) {
    // 🆕 2026-06-11 修复：inline error card
    setInlineError({
      source: 'workflowStart',
      title: '启动工作流失败',
      message: e instanceof Error ? e.message : String(e),
      hint: '检查后端 task 队列是否满 / 是否已有运行中的 workflow / mock 数据是否生成',
    })
  }
}

function handleCancel() {
  engine.cancelCurrentRun()
  showToast({ message: 'Workflow cancelled', color: 'warning', duration: 1500 })
}

function onSelectStep(step: StepRun) {
  selectedStep.value = step
}

function formatTime(iso: string): string {
  try { return new Date(iso).toLocaleTimeString() } catch { return iso }
}

function formatInlineErrorTime(at: number): string {
  // 把 Date.now() 渲染成「刚刚 / N 分钟前 / HH:MM:SS」
  const secAgo = Math.floor((Date.now() - at) / 1000)
  if (secAgo < 5) return '刚刚'
  if (secAgo < 60) return `${secAgo} 秒前`
  if (secAgo < 3600) return `${Math.floor(secAgo / 60)} 分钟前`
  return new Date(at).toLocaleTimeString()
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

onMounted(() => {
  tickHandle = setInterval(() => { _tickNow.value = Date.now() }, 1000)
  wsStart()
  // 🆕 2026-06-12：监听 MainActivity 推送的 CustomEvent，显示 lastError
  window.addEventListener('encv:backend-status', onBackendStatus)
})

onUnmounted(() => {
  if (tickHandle) clearInterval(tickHandle)
  wsStop()
  window.removeEventListener('encv:backend-status', onBackendStatus)
})
</script>

<style scoped>
.section-hint { font-size: 12px; color: var(--ion-color-medium-shade); padding: 8px 16px 4px; margin: 0; }
.mock-root-path { font-family: monospace; font-size: 12px; background: var(--ion-color-light-shade); padding: 2px 6px; border-radius: 4px; }

.mock-stats-card { margin: 8px 16px; padding: 12px 16px; background: var(--ion-color-light); border-radius: 8px; }
.stat-row { display: flex; justify-content: space-between; align-items: center; padding: 4px 0; font-size: 14px; }
.stat-value { font-weight: 600; font-family: monospace; }
.stat-value--warn { color: #B8860B; }
.progress-text { font-size: 12px; color: var(--ion-color-medium); padding: 4px 16px; font-family: monospace; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.progress-card { margin: 8px 16px; padding: 12px 16px; background: var(--ion-color-light); border-radius: 8px; }
.progress-stats { display: flex; justify-content: space-between; margin-top: 6px; font-size: 13px; }
.progress-stats .passed { color: var(--ion-color-success); }
.progress-stats .failed { color: var(--ion-color-danger); }
.progress-stats .pending { color: #B8860B; }

/* ========== 🆕 2026-06-12 饱和调试：FFMPEG 流程日志卡 ========== */
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
.mock-gen-log-title { display: flex; align-items: center; gap: 6px; font-size: 12px; font-weight: 600; color: #F4EFE6; }
.mock-gen-log-title ion-icon { font-size: 14px; }
.mock-gen-log-count { color: #6B7280; font-size: 11px; margin-left: 4px; }
.mock-gen-log-copy {
  display: inline-flex; align-items: center; gap: 4px;
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
.mock-gen-log-copy:hover { background: rgba(255, 255, 255, 0.1); }
.mock-gen-log-copy ion-icon { font-size: 12px; }
.mock-gen-log-copy--copied { background: rgba(34, 197, 94, 0.15); color: #4ADE80; border-color: rgba(34, 197, 94, 0.3); }

.mock-gen-log-summary {
  display: flex; align-items: center; gap: 6px;
  padding: 6px 0;
  font-size: 12px;
  color: #9CA3AF;
  flex-wrap: wrap;
}
.mock-gen-log-summary ion-icon { font-size: 14px; }
.mock-gen-log-disconnect { color: #F59E0B; font-weight: 600; }

.mock-gen-log-list { list-style: none; margin: 0; padding: 0; }
.mock-gen-log-entry {
  border-left: 2px solid transparent;
  padding: 4px 0 4px 8px;
  margin: 1px 0;
  transition: background 0.15s;
}
.mock-gen-log-entry--success { border-left-color: rgba(34, 197, 94, 0.4); }
.mock-gen-log-entry--failed {
  border-left-color: var(--ion-color-danger);
  background: rgba(220, 38, 38, 0.06);
}
.mock-gen-log-row {
  display: flex; align-items: center; gap: 6px;
  font-size: 11.5px;
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 3px;
  user-select: none;
}
.mock-gen-log-row:hover { background: rgba(255, 255, 255, 0.04); }
.mock-gen-log-status { display: flex; align-items: center; }
.mock-gen-log-status ion-icon { font-size: 13px; }
.mock-gen-log-idx { color: #6B7280; font-weight: 600; }
.mock-gen-log-path { color: #E0E0E0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mock-gen-log-encoder { color: #8B5CF6; font-size: 10.5px; }

/* 🆕 2026-06-12：runner 标识（mediacodec=硬件 / ffmpeg=软件 / static=静态） */
.mock-gen-log-runner {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: bold;
  margin-right: 4px;
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
.mock-gen-log-exitcode { color: #FCA5A5; font-weight: 600; font-size: 10.5px; }
.mock-gen-log-row ion-icon:last-child { font-size: 12px; color: #6B7280; }

.mock-gen-log-detail {
  margin: 4px 0 0;
  padding: 8px 10px;
  background: rgba(0, 0, 0, 0.3);
  border-radius: 4px;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  color: #C9D1D9;
  max-height: 240px;
  overflow-y: auto;
}
.mock-gen-log-detail .lbl { color: #58A6FF; font-weight: 600; display: block; margin-top: 4px; }
.mock-gen-log-detail .lbl:first-child { margin-top: 0; }

.view-toggle { font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; font-size: 11px; background: none; border: none; color: #6B5D4C; cursor: pointer; padding: 2px 6px; border-radius: 3px; }
.view-toggle--active { background: #1A1A1A; color: #F4EFE6; }
.view-toggle-sep { color: #C9BBA1; }

/* 🆕 2026-06-11 内联错误卡（饱和调试原则：禁用 Toast，错误必须持久可见） */
.inline-error-card {
  margin: 12px 16px;
  padding: 14px 16px;
  background: linear-gradient(180deg, rgba(220, 38, 38, 0.08) 0%, rgba(220, 38, 38, 0.04) 100%);
  border-left: 4px solid var(--ion-color-danger);
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(220, 38, 38, 0.15);
}
.inline-error-header { display: flex; align-items: flex-start; gap: 10px; }
.inline-error-icon { font-size: 24px; flex-shrink: 0; margin-top: 2px; }
.inline-error-title-block { flex: 1; min-width: 0; }
.inline-error-title { font-size: 15px; font-weight: 600; color: var(--ion-color-danger-shade); line-height: 1.3; }
.inline-error-source { font-size: 11px; color: var(--ion-color-medium); margin-top: 2px; font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; }
.inline-error-close { background: none; border: none; font-size: 24px; line-height: 1; color: var(--ion-color-medium); cursor: pointer; padding: 0 4px; flex-shrink: 0; }
.inline-error-close:hover { color: var(--ion-color-danger); }
.inline-error-message {
  margin: 10px 0 0 34px;
  padding: 10px 12px;
  background: rgba(0, 0, 0, 0.04);
  border-radius: 4px;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ion-color-dark);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 200px;
  overflow-y: auto;
}
.inline-error-hint {
  margin: 10px 0 0 34px;
  padding: 8px 10px;
  background: rgba(59, 130, 246, 0.08);
  border-left: 3px solid var(--ion-color-primary);
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ion-color-dark);
}
.inline-error-actions { margin: 12px 0 0 34px; display: flex; gap: 8px; }
.inline-error-retry {
  padding: 6px 14px;
  background: var(--ion-color-danger);
  color: white;
  border: none;
  border-radius: 4px;
  font-size: 13px;
  cursor: pointer;
  font-weight: 500;
}
.inline-error-retry:hover { background: var(--ion-color-danger-shade); }
.inline-error-retry:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
