/**
 * useWebDavWorkflowAdapter — WebDAV 8 module 协调器（基于 useWorkflowTaskService）
 *
 * Task 9 of unify-workflow-task-service spec：
 *  - 退役 useWebDavAutomationTests.ts，将其协调逻辑迁移为 useWorkflowTaskService 的 workflowDefinition 模板
 *  - 持久化 key 从 encv_webdav_automation_results_v2 迁移到 encv_workflow_tasks_v1（旧数据不迁移）
 *  - 保持 UI 行为不变：返回与原 useWebDavAutomationTests 相同的接口
 *
 * 设计要点：
 *  - WebDAV 测试是纯 HTTP fetch（PROPFIND / GET / OPTIONS ...），不是 EncvTask，无法走 submitRun → submitAction 链路
 *  - 因此本 adapter 仍用 useWebDavTestRunner.runCase() 执行测试
 *  - 但持久化 / 历史管理统一走 useWorkflowTaskService.runs（UnifiedRunRecord 格式，含 WorkflowRun 快照）
 *  - 每个 module run 构造一个 WorkflowRun 快照（1 job = module，steps = cases），写入 service.runs
 *  - WebDAV workflowDefinition 模板作为 metadata 注册（id = webdav-automation-tests），用于从 unified runs 中过滤 WebDAV 记录
 *
 * 与 useWorkflowTaskService 关系：
 *  - 共享 service.runs ref + encv_workflow_tasks_v1 localStorage key
 *  - 不调用 submitRun（WebDAV 测试不走 EncvTask createTask API）
 *  - 直接操作 service.runs.value + 手动持久化（service 没有公开 addRecord 方法）
 *  - clearHistory 只清 WebDAV 记录（按 workflowDefId 过滤），不影响插件测试记录
 */

import { type ComputedRef, computed, type Ref, ref, watch } from "vue";
import { generateMockFilesViaBackend } from "@encv/shared-components/api/mockGenerator";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useWebDavManifest } from "@encv/shared-components/composables/useWebDavManifest";
import { getModuleById, WEBDAV_TEST_MODULES } from "@encv/shared-components/composables/useWebDavTestModules";
import { useWebDavTestRunner } from "@encv/shared-components/composables/useWebDavTestRunner";
import { useWorkflowTaskService } from "@encv/shared-components/composables/useWorkflowTaskService";
import { MOCK_GENERATE_ROOT } from "@encv/shared-components/lib/mockConstants";
import type { StepStatus, UnifiedRunRecord, WorkflowDefinition, WorkflowRun, WorkflowStatus } from "@encv/shared-components/lib/workflow/types";
import type { TestCaseResult, TestCaseStatus, TestRun, WebDavTestContext } from "@encv/shared-components/types/webdav-test";

// ============= 类型定义（与原 useWebDavAutomationTests 保持一致）=============

export type ModuleRunStatus = "idle" | "running" | "done" | "cancelling" | "cancelled" | "error";

export interface ModuleRunState {
  status: ModuleRunStatus;
  startedAt?: string;
  completedAt?: string;
  results: TestCaseResult[];
  error?: string;
}

export interface UseWebDavAutomationTestsReturn {
  modules: typeof WEBDAV_TEST_MODULES;
  moduleStates: Record<string, Ref<ModuleRunState>>;
  historyRuns: Ref<TestRun[]>;
  isAnyRunning: ComputedRef<boolean>;
  manifestComposable: ReturnType<typeof useWebDavManifest>;
  /** 跑单个 module（所有 case 串行） */
  runModule: (moduleId: string) => Promise<void>;
  /** 跑所有 module（按顺序） */
  runAll: () => Promise<void>;
  /** 取消正在运行的 module */
  cancelModule: (moduleId: string) => void;
  /** 清空历史 */
  clearHistory: () => void;
  /** 清空单个 module 的当前结果 */
  resetModule: (moduleId: string) => void;
}

// ============= WebDAV workflow 模板（metadata 注册）=============
//
// 8 module → 8 jobs，顺序依赖（module 2 needs module 1，...）
// 每个 job 的 steps 对应 module 内的 cases（action 为 http_request 占位，实际执行走 runCase）
// 此模板不直接被 submitRun 执行，仅作为 UnifiedRunRecord.workflowRun.workflowDefId 的标识

const WEBDAV_WORKFLOW_DEF_ID = "webdav-automation-tests";

const WEBDAV_WORKFLOW_DEF: WorkflowDefinition = {
  id: WEBDAV_WORKFLOW_DEF_ID,
  name: "WebDAV 自动化测试",
  description: "8 module × 46 test case（auth/basic_ops/protocol/concurrency/large_payload/edge/attack/encrypted_container_preview）",
  createdAt: "2026-06-18T00:00:00.000Z",
  updatedAt: "2026-06-18T00:00:00.000Z",
  trigger: "manual",
  builtin: true,
  jobs: WEBDAV_TEST_MODULES.map((m, i) => ({
    id: m.id,
    name: m.nameI18n,
    // 顺序依赖：module 2 needs module 1，module 3 needs module 2，...
    needs: i > 0 ? [WEBDAV_TEST_MODULES[i - 1].id] : undefined,
    strategy: { type: "sequential" as const },
    steps: m.cases.map(c => ({
      id: c.id,
      name: c.nameI18n,
      // action 为 http_request 占位（实际执行走 useWebDavTestRunner.runCase，不走 submitAction）
      action: {
        type: "http_request" as const,
        method: c.method,
        url: "",
      },
    })),
  })),
};

// ============= 辅助函数 =============

/** TestCaseStatus → StepStatus 映射 */
function testCaseStatusToStepStatus(s: TestCaseStatus): StepStatus {
  switch (s) {
    case "success":
      return "success";
    case "failure":
      return "failure";
    case "skipped":
      return "skipped";
    case "timed_out":
      return "timed_out";
    case "running":
      return "running";
    case "pending":
      return "pending";
  }
}

/** ModuleRunStatus → WorkflowStatus 映射（用于 WorkflowRun 快照） */
function moduleStatusToWorkflowStatus(s: ModuleRunStatus): WorkflowStatus {
  switch (s) {
    case "idle":
      return "pending";
    case "running":
      return "running";
    case "cancelling":
      return "running";
    case "done":
      return "success";
    case "cancelled":
      return "cancelled";
    case "error":
      return "failure";
  }
}

/** 从 module run 构造 WorkflowRun 快照（1 job = module，steps = cases） */
function buildWorkflowRunSnapshot(runId: string, moduleId: string, state: ModuleRunState, results: TestCaseResult[]): WorkflowRun {
  const now = new Date().toISOString();
  return {
    id: runId,
    workflowDefId: WEBDAV_WORKFLOW_DEF_ID,
    status: moduleStatusToWorkflowStatus(state.status),
    triggeredBy: "user",
    createdAt: state.startedAt ?? now,
    startedAt: state.startedAt,
    completedAt: state.completedAt,
    durationMs:
      state.startedAt && state.completedAt ? new Date(state.completedAt).getTime() - new Date(state.startedAt).getTime() : undefined,
    jobs: [
      {
        id: `${runId}_job_${moduleId}`,
        jobDefId: moduleId,
        status: moduleStatusToWorkflowStatus(state.status),
        conclusion:
          state.status === "done"
            ? "success"
            : state.status === "cancelled"
              ? "cancelled"
              : state.status === "error"
                ? "failure"
                : undefined,
        startedAt: state.startedAt,
        completedAt: state.completedAt,
        durationMs:
          state.startedAt && state.completedAt ? new Date(state.completedAt).getTime() - new Date(state.startedAt).getTime() : undefined,
        steps: results.map(r => ({
          id: r.id,
          stepDefId: r.id,
          status: testCaseStatusToStepStatus(r.status),
          startedAt: state.startedAt,
          completedAt: state.completedAt,
          durationMs: r.durationMs,
          error: r.error,
        })),
      },
    ],
  };
}

/** 从 UnifiedRunRecord 转回 TestRun（UI 兼容） */
function recordToTestRun(record: UnifiedRunRecord): TestRun {
  const workflowRun = record.workflowRun;
  const job = workflowRun?.jobs[0];
  const moduleId = job?.jobDefId ?? "unknown";
  const results: TestCaseResult[] = (job?.steps ?? []).map(s => ({
    id: s.id,
    name: s.id, // name 在原实现中由 translateName 填充，历史记录用 id 兜底
    module: moduleId,
    status: (s.status === "success"
      ? "success"
      : s.status === "skipped"
        ? "skipped"
        : s.status === "timed_out"
          ? "timed_out"
          : s.status === "failure"
            ? "failure"
            : "pending") as TestCaseStatus,
    durationMs: s.durationMs ?? 0,
    error: s.error,
  }));
  return {
    id: record.id,
    startedAt: record.startedAt,
    completedAt: record.completedAt,
    module: moduleId,
    totalCases: record.totalCases,
    passed: record.passed,
    failed: record.failed,
    skipped: record.skipped,
    results,
  };
}

// ============= 主 composable =============

export function useWebDavAutomationTests(): UseWebDavAutomationTestsReturn {
  const { t } = useI18n();
  const service = useWorkflowTaskService();
  const manifestComposable = useWebDavManifest();
  const { runCase } = useWebDavTestRunner();

  // 8 module 独立状态
  const moduleStates: Record<string, Ref<ModuleRunState>> = {};
  for (const m of WEBDAV_TEST_MODULES) {
    moduleStates[m.id] = ref<ModuleRunState>({
      status: "idle",
      results: [],
    });
  }

  // 历史（从 service.runs 派生，过滤 WebDAV 记录）
  const historyRuns = ref<TestRun[]>(loadWebDavHistory());

  // 监听 service.runs 变化，同步 historyRuns
  watch(
    () => service.runs.value,
    () => {
      historyRuns.value = loadWebDavHistory();
    },
    { deep: true }
  );

  const isAnyRunning: ComputedRef<boolean> = computed(() => {
    return Object.values(moduleStates).some(s => s.value.status === "running" || s.value.status === "cancelling");
  });

  // 当前 run 的 abort controller（per module）
  const abortControllers: Record<string, AbortController | null> = {};

  // ============= 持久化（共享 encv_workflow_tasks_v1）=============

  /** 从 service.runs 过滤 WebDAV 记录，转回 TestRun[] */
  function loadWebDavHistory(): TestRun[] {
    return service.runs.value
      .filter(r => r.workflowRun?.workflowDefId === WEBDAV_WORKFLOW_DEF_ID)
      .map(recordToTestRun)
      .sort((a, b) => b.startedAt.localeCompare(a.startedAt));
  }

  /** 将 WebDAV module run 持久化到 service.runs + localStorage */
  function persistWebDavRun(runId: string, moduleId: string, state: ModuleRunState, results: TestCaseResult[]): void {
    if (results.length === 0) return;
    const workflowRun = buildWorkflowRunSnapshot(runId, moduleId, state, results);
    const passed = results.filter(r => r.status === "success").length;
    const failed = results.filter(r => r.status === "failure" || r.status === "timed_out").length;
    const skipped = results.filter(r => r.status === "skipped").length;
    const record: UnifiedRunRecord = {
      id: runId,
      startedAt: state.startedAt ?? new Date().toISOString(),
      completedAt: state.completedAt,
      totalCases: results.length,
      passed,
      failed,
      skipped,
      results: results.map(r => ({
        caseId: r.id,
        status: r.status === "success" ? ("success" as const) : r.status === "skipped" ? ("skipped" as const) : ("failure" as const),
        error: r.error,
        duration: `${r.durationMs}ms`,
      })),
      workflowRun,
    };
    // 追加到 service.runs（共享 encv_workflow_tasks_v1）
    const existing = service.runs.value.filter(r => r.id !== runId);
    service.runs.value = [record, ...existing];
    // 手动持久化（service 没有公开 addRecord 方法）
    saveRunsToStorage(service.runs.value);
  }

  /** 直接写 localStorage（与 service 共享同一 key） */
  function saveRunsToStorage(runs: UnifiedRunRecord[]): void {
    try {
      const sorted = [...runs].sort((a, b) => b.startedAt.localeCompare(a.startedAt)).slice(0, 50);
      localStorage.setItem("encv_workflow_tasks_v1", JSON.stringify(sorted));
    } catch (e) {
      console.warn("[useWebDavWorkflowAdapter] persist failed:", e);
    }
  }

  // ============= mock media 兜底（同原实现）=============

  const ensureMockMediaCache = { lastRun: 0, inFlight: false };
  async function ensureMockMedia(): Promise<void> {
    if (Date.now() - ensureMockMediaCache.lastRun < 30_000) return;
    if (ensureMockMediaCache.inFlight) return;
    ensureMockMediaCache.inFlight = true;
    try {
      console.log("[useWebDavWorkflowAdapter] ensureMockMedia: generateMockFilesViaBackend", MOCK_GENERATE_ROOT);
      const result = await generateMockFilesViaBackend({ root: MOCK_GENERATE_ROOT });
      console.log("[useWebDavWorkflowAdapter] ensureMockMedia: done", result);
      ensureMockMediaCache.lastRun = Date.now();
    } catch (e) {
      console.warn("[useWebDavWorkflowAdapter] ensureMockMedia failed (non-fatal):", e);
    } finally {
      ensureMockMediaCache.inFlight = false;
    }
  }

  // ============= 执行核心 =============

  async function runModule(moduleId: string): Promise<void> {
    const module = getModuleById(moduleId);
    if (!module) {
      console.error(`[useWebDavWorkflowAdapter] unknown module: ${moduleId}`);
      return;
    }
    const state = moduleStates[moduleId];
    if (state.value.status === "running" || state.value.status === "cancelling") return;

    // 1. 确保 manifest 已就绪
    if (!manifestComposable.isReady.value) {
      await manifestComposable.refresh();
    }
    if (!manifestComposable.isReady.value || !manifestComposable.activeMount.value) {
      state.value = {
        status: "error",
        results: [],
        error: "manifest not ready: backend webdav may not be enabled",
      };
      return;
    }

    // 1.5 复用插件测试的 mock 生成（确保 plain media 存在）
    await ensureMockMedia();

    const ctx = buildContext(manifestComposable);
    const controller = new AbortController();
    abortControllers[moduleId] = controller;
    ctx.abortSignal = controller.signal;
    ctx.triggerAbort = () => controller.abort();

    const runId = `run_${Date.now()}_${moduleId}`;
    state.value = {
      status: "running",
      startedAt: new Date().toISOString(),
      results: [],
    };

    const translateName = (id: string) => t(`devtools.webdav.cases.${id}.name`);
    const results: TestCaseResult[] = [];

    try {
      for (const desc of module.cases) {
        if (controller.signal.aborted) {
          // 用户取消：剩余 case 标记 skipped
          for (const skipped of module.cases.filter(c => !results.find(r => r.id === c.id))) {
            results.push({
              id: skipped.id,
              name: translateName(skipped.id),
              module: skipped.module,
              status: "skipped",
              durationMs: 0,
            });
          }
          break;
        }
        const result = await runCase(desc, ctx, {
          abortSignal: controller.signal,
          translateName,
        });
        results.push(result);
        state.value = { ...state.value, results: [...results] };
      }

      state.value = {
        ...state.value,
        status: controller.signal.aborted ? "cancelled" : "done",
        completedAt: new Date().toISOString(),
        results,
      };
    } catch (e) {
      state.value = {
        ...state.value,
        status: "error",
        completedAt: new Date().toISOString(),
        error: e instanceof Error ? e.message : String(e),
        results,
      };
    } finally {
      abortControllers[moduleId] = null;
      persistWebDavRun(runId, moduleId, state.value, results);
    }
  }

  async function runAll(): Promise<void> {
    for (const m of WEBDAV_TEST_MODULES) {
      await runModule(m.id);
    }
  }

  function cancelModule(moduleId: string): void {
    const ctrl = abortControllers[moduleId];
    if (ctrl) {
      ctrl.abort();
      const state = moduleStates[moduleId];
      if (state.value.status === "running") {
        state.value = { ...state.value, status: "cancelling" };
      }
    }
  }

  function clearHistory(): void {
    // 只清 WebDAV 记录，保留其他 workflow 记录
    const remaining = service.runs.value.filter(r => r.workflowRun?.workflowDefId !== WEBDAV_WORKFLOW_DEF_ID);
    service.runs.value = remaining;
    saveRunsToStorage(remaining);
    historyRuns.value = [];
  }

  function resetModule(moduleId: string): void {
    moduleStates[moduleId].value = { status: "idle", results: [] };
  }

  return {
    modules: WEBDAV_TEST_MODULES,
    moduleStates,
    historyRuns,
    isAnyRunning,
    manifestComposable,
    runModule,
    runAll,
    cancelModule,
    clearHistory,
    resetModule,
  };
}

// ============= 内部辅助 =============

function buildContext(manifestComposable: ReturnType<typeof useWebDavManifest>): WebDavTestContext {
  if (!manifestComposable.manifest.value || !manifestComposable.activeMount.value) {
    throw new Error("manifest not ready");
  }
  return {
    manifest: manifestComposable.manifest.value,
    serverBaseUrl: manifestComposable.serverBaseUrl.value,
    webdavPath: manifestComposable.webdavPath.value || manifestComposable.activeMount.value.webdav_path,
    auth: manifestComposable.auth.value,
    activeMount: manifestComposable.activeMount.value,
    shared: {},
  };
}

// 导出 workflow 定义模板（供外部注册 / 调试使用）
export { WEBDAV_WORKFLOW_DEF, WEBDAV_WORKFLOW_DEF_ID };
