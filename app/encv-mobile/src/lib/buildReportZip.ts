/**
 * buildReportZip — 把 WorkflowRun 数据打包成 AI 友好 zip
 *
 * 设计目标（spec v5-bug3，2026-06-18）：
 * - 拆掉 PluginTestsDetail 独立页面，状态融入任务聚合展示
 * - 改为导出 zip，5 顶层文件 + cases/ 子目录，AI 友好结构
 * - 支持任意需要"前端打包 zip"的场景复用（zstd 插件输出、配置备份等）
 *
 * zip 结构：
 *   encvreport-{runId8}-{date}.zip
 *   ├── report.json     # 机器可读主入口（AI 解析）
 *   ├── summary.md      # 人类可读概览
 *   ├── cases.md        # case 索引（按状态分组，链接到 cases/*.md）
 *   ├── metadata.json   # 报告元数据（生成环境、设备、locale）
 *   ├── timeline.txt    # 任务时间线（按时间戳排序的 NDJSON-like）
 *   └── cases/          # 单 case 详情
 *       ├── 001-passed-xxx.md
 *       ├── 002-failed-yyy.md
 *       └── ...
 *
 * 数据源：
 * - `run` (UnifiedRunRecord) — localStorage 持久化的运行记录，含 workflowRun 快照
 * - `runTasks` (EncvTask[]) — useTasksList 实时任务列表（progress/phase/speed/eta 更新）
 *
 * 为什么不用 Go 后端 zip：
 *   - workflow run 数据在前端 localStorage（`encv_workflow_tasks_v1`）
 *   - 现有"导出日志"用 Go `archive/zip` 是因为 log 在后端（[LogSettingsDetail.vue:163-181]）
 *   - run data 上传给后端做 zip 不合理（数据上传 + 网络往返 + 后端无对应 endpoint）
 *   - 前端 JSZip 3.10.1 是最简可行方案
 */

import JSZip from "jszip";
import type { CalibrationResult, EncvTask, PerformanceSummary } from "@/api/encv";
import type { TFunction } from "@/composables/useI18n";
import type { JobRun, StepRun, UnifiedRunRecord } from "./workflow/types";

// ==================== Public API ====================

/**
 * 构建报告 zip 并返回 Blob
 * @param run UnifiedRunRecord 持久化运行记录
 * @param runTasks EncvTask[] 实时任务列表（group card 展示的子任务）
 * @param t i18n TFunction（用于本地化 case md 字段）
 * @param options.locale 当前语言（写入 metadata.json）
 * @param options.deviceInfo 设备信息（写入 metadata.json，可选）
 */
export async function buildReportZip(
  run: UnifiedRunRecord,
  runTasks: EncvTask[],
  t: TFunction,
  options?: {
    locale?: string;
    deviceInfo?: {
      platform?: string;
      model?: string;
      osVersion?: string;
      appVersion?: string;
    };
    /** 🆕 2026-06-22 硬件校准结果（写入 report.json 顶层 + performance.md） */
    calibration?: CalibrationResult | null;
  }
): Promise<Blob> {
  const zip = new JSZip();

  // 1. report.json — AI 解析主入口
  zip.file("report.json", buildReportJson(run, runTasks, options));

  // 2. summary.md — 人类可读概览
  zip.file("summary.md", buildSummaryMd(run, runTasks, t, options));

  // 3. cases.md — case 索引
  const casesByStatus = groupCasesByStatus(run, runTasks);
  zip.file("cases.md", buildCasesIndexMd(casesByStatus, t));

  // 4. metadata.json — 报告元数据
  zip.file("metadata.json", buildMetadataJson(run, options));

  // 5. timeline.txt — 任务时间线（按时间戳排序）
  zip.file("timeline.txt", buildTimelineTxt(run));

  // 6. performance.md — 🆕 性能报告（硬件校准 + plugin 聚合 + phase 分布）
  zip.file("performance.md", buildPerformanceMd(run, runTasks, t, options));

  // 7. cases/*.md — 单 case 详情（cases 目录）
  const casesFolder = zip.folder("cases");
  if (casesFolder) {
    const cases = flattenCases(run, runTasks);
    cases.forEach((c, idx) => {
      const filename = `${String(idx + 1).padStart(3, "0")}-${c.status}-${sanitizeFilename(c.name)}.md`;
      casesFolder.file(filename, buildCaseDetailMd(c, idx + 1, t));
    });
  }

  return await zip.generateAsync({ type: "blob", compression: "DEFLATE", compressionOptions: { level: 6 } });
}

// ==================== Data Structures ====================

interface CaseDetail {
  /** step.id（workflowRun.steps[].id） */
  id: string;
  /** 序号 1-based */
  index: number;
  /** 显示名（plugin-name + taskType） */
  name: string;
  /** 状态（success/failure/skipped） */
  status: "success" | "failure" | "skipped";
  /** 插件名 */
  pluginName: string;
  /** 任务类型（encrypt/decrypt） */
  taskType: "encrypt" | "decrypt";
  /** 源路径 */
  sourcePath: string;
  /** 目标路径 */
  targetPath?: string;
  /** 开始时间 ISO */
  startedAt?: string;
  /** 完成时间 ISO */
  completedAt?: string;
  /** 耗时 ms */
  durationMs?: number;
  /** 实时进度（0-100） */
  progress?: number;
  /** 实时阶段 */
  phase?: string;
  /** 实时速率 */
  speed?: string;
  /** 实时剩余时间 */
  eta?: string;
  /** 错误信息 */
  error?: string;
  /** 🆕 性能摘要（来自 live task.performanceSummary，仅 completed 状态有值） */
  performance?: PerformanceSummary;
  /** step 原始数据 */
  step: StepRun;
  /** 关联的 job 原始数据 */
  job: JobRun;
}

interface CasesByStatus {
  success: CaseDetail[];
  failure: CaseDetail[];
  skipped: CaseDetail[];
}

// ==================== Helpers ====================

/** 把 UnifiedRunRecord + runTasks 扁平化成 CaseDetail[] */
function flattenCases(run: UnifiedRunRecord, runTasks: EncvTask[]): CaseDetail[] {
  const wRun = run.workflowRun;
  if (!wRun) return [];

  // 实时 EncvTask 按 id 索引
  const liveById = new Map<string, EncvTask>();
  for (const t of runTasks) {
    liveById.set(t.id, t);
  }

  const cases: CaseDetail[] = [];
  let counter = 0;
  for (const job of wRun.jobs) {
    for (const step of job.steps) {
      counter += 1;
      // 关联实时数据：step.taskId 是 EncvTask.id
      const live = step.taskId ? liveById.get(step.taskId) : undefined;
      const action = inferAction(step, live);
      const status = step.status === "success" ? "success" : step.status === "skipped" ? "skipped" : "failure";

      cases.push({
        id: step.id,
        index: counter,
        name: action ? `${action.pluginName}-${action.taskType}` : step.id,
        status,
        pluginName: action?.pluginName ?? "unknown",
        taskType: action?.taskType ?? "encrypt",
        sourcePath: action?.params?.sourcePath ?? "",
        targetPath: action?.params?.targetPath ?? action?.params?.sourcePath,
        startedAt: step.startedAt,
        completedAt: step.completedAt,
        durationMs: step.durationMs,
        progress: live?.progress ?? step.progress,
        phase: live?.phase ?? step.phase,
        speed: live?.speed,
        eta: live?.eta,
        error: step.error ?? live?.error,
        performance: live?.performanceSummary,
        step,
        job,
      });
    }
  }
  return cases;
}

function groupCasesByStatus(run: UnifiedRunRecord, runTasks: EncvTask[]): CasesByStatus {
  const grouped: CasesByStatus = { success: [], failure: [], skipped: [] };
  for (const c of flattenCases(run, runTasks)) {
    grouped[c.status].push(c);
  }
  return grouped;
}

/** 从 step 推断 ActionSpec（pluginName / taskType / params）
 *
 * 按优先级尝试多种 fallback：
 *   1. step.matrixVars（matrix 策略，最完整）
 *   2. step.stepDef.action（parallel/sequential 策略的 action 定义）
 *   3. step.stepDefId 正则匹配（从 ID 中提取 plugin 和 type）
 *   4. step.taskId + live task 数据（从 EncvTask.pluginName/type 反推）
 */
function inferAction(
  step: StepRun,
  liveTask?: EncvTask
): {
  pluginName: string;
  taskType: "encrypt" | "decrypt";
  params?: { sourcePath?: string; targetPath?: string };
} | null {
  // 1. matrixVars（matrix 策略）
  const matrixVars = (step as any).matrixVars;
  if (matrixVars && typeof matrixVars === "object") {
    return {
      pluginName: String(matrixVars.pluginName ?? matrixVars.plugin ?? "unknown"),
      taskType: matrixVars.taskType === "decrypt" ? "decrypt" : "encrypt",
      params: {
        sourcePath: matrixVars.sourcePath,
        targetPath: matrixVars.targetPath,
      },
    };
  }

  // 2. stepDef.action（parallel / sequential 策略）
  const stepDef = (step as any).stepDef;
  if (stepDef?.action && typeof stepDef.action === "object") {
    const action = stepDef.action;
    return {
      pluginName: String(action.pluginName ?? action.plugin ?? "unknown"),
      taskType: action.taskType === "decrypt" ? "decrypt" : "encrypt",
      params: {
        sourcePath: action.sourcePath ?? action.path,
        targetPath: action.targetPath,
      },
    };
  }

  // 3. stepDefId 正则匹配（enc_{plugin}_{type}_... 格式）
  const stepDefId = step.stepDefId ?? "";
  const m = stepDefId.match(/^enc_(\w+?)_(\w+?)_/);
  if (m) {
    return {
      pluginName: m[1],
      taskType: m[2] === "decrypt" ? "decrypt" : "encrypt",
      params: {},
    };
  }

  // 4. 从 live EncvTask 反推
  if (liveTask) {
    return {
      pluginName: liveTask.pluginName ?? "unknown",
      taskType: liveTask.type === "decrypt" ? "decrypt" : "encrypt",
      params: {
        sourcePath: liveTask.sourcePath,
        targetPath: liveTask.targetPath,
      },
    };
  }

  return null;
}

/** 文件名安全化（去除 / \ : * ? " < > | 空格转 -） */
function sanitizeFilename(name: string): string {
  return name
    .replace(/[/\\:*?"<>|]/g, "-")
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .toLowerCase()
    .slice(0, 80);
}

/** 格式化 ISO 时间到本地 YYYY-MM-DD HH:mm:ss */
function formatLocalTime(iso?: string): string {
  if (!iso) return "N/A";
  try {
    const d = new Date(iso);
    const pad = (n: number) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  } catch {
    return iso;
  }
}

/** 格式化 ms → "1m 23s" / "500ms" */
function formatDuration(ms?: number): string {
  if (ms == null) return "N/A";
  if (ms < 1000) return `${ms}ms`;
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}.${Math.floor((ms % 1000) / 100)}s`;
  const m = Math.floor(s / 60);
  const rs = s % 60;
  return `${m}m${rs}s`;
}

// ==================== File Generators ====================

/** 1. report.json — AI 解析主入口 */
function buildReportJson(run: UnifiedRunRecord, runTasks: EncvTask[], options?: { calibration?: CalibrationResult | null }): string {
  const wRun = run.workflowRun;
  const cases = flattenCases(run, runTasks);

  // 按 plugin 聚合
  const pluginStats: Record<string, { total: number; passed: number; failed: number; skipped: number }> = {};
  for (const c of cases) {
    if (!pluginStats[c.pluginName]) {
      pluginStats[c.pluginName] = { total: 0, passed: 0, failed: 0, skipped: 0 };
    }
    const s = pluginStats[c.pluginName];
    s.total += 1;
    if (c.status === "success") s.passed += 1;
    else if (c.status === "failure") s.failed += 1;
    else s.skipped += 1;
  }

  // 失败 case 详情（AI 优先关注的）
  const failures = cases
    .filter(c => c.status === "failure")
    .map(c => ({
      caseId: c.id,
      caseFile: `cases/${String(c.index).padStart(3, "0")}-failure-${sanitizeFilename(c.name)}.md`,
      plugin: c.pluginName,
      taskType: c.taskType,
      sourcePath: c.sourcePath,
      targetPath: c.targetPath,
      error: c.error,
      durationMs: c.durationMs,
      startedAt: c.startedAt,
      completedAt: c.completedAt,
    }));

  // 🆕 性能聚合（按 pluginName 分组）
  const perfByPlugin: Record<
    string,
    {
      count: number;
      avgThroughput: number;
      avgGradeScore: number;
      gradeDistribution: { excellent: number; good: number; warn: number };
    }
  > = {};
  for (const c of cases) {
    if (!c.performance) continue;
    const key = c.pluginName;
    if (!perfByPlugin[key]) {
      perfByPlugin[key] = {
        count: 0,
        avgThroughput: 0,
        avgGradeScore: 0,
        gradeDistribution: { excellent: 0, good: 0, warn: 0 },
      };
    }
    const p = perfByPlugin[key];
    p.count += 1;
    p.avgThroughput += c.performance.avgThroughput;
    p.avgGradeScore += c.performance.gradeScore;
    if (c.performance.grade === "excellent") p.gradeDistribution.excellent += 1;
    else if (c.performance.grade === "good") p.gradeDistribution.good += 1;
    else p.gradeDistribution.warn += 1;
  }
  // 计算平均值
  for (const p of Object.values(perfByPlugin)) {
    if (p.count > 0) {
      p.avgThroughput = Math.round((p.avgThroughput / p.count) * 10) / 10;
      p.avgGradeScore = Math.round(p.avgGradeScore / p.count);
    }
  }

  const report: Record<string, unknown> = {
    schema: "encv-report/v1",
    run: {
      id: run.id,
      workflowDefId: wRun?.workflowDefId,
      status: wRun?.status,
      triggeredBy: wRun?.triggeredBy,
      createdAt: wRun?.createdAt,
      startedAt: run.startedAt,
      completedAt: run.completedAt,
      durationMs: wRun?.durationMs,
    },
    summary: {
      total: run.totalCases,
      passed: run.passed,
      failed: run.failed,
      skipped: run.skipped,
      percent: run.totalCases > 0 ? Math.round(((run.passed + run.skipped) / run.totalCases) * 100) : 0,
    },
    plugins: Object.entries(pluginStats).map(([name, s]) => ({ name, ...s })),
    failures,
    cases: cases.map(c => {
      const caseEntry: Record<string, unknown> = {
        caseId: c.id,
        caseFile: `cases/${String(c.index).padStart(3, "0")}-${c.status}-${sanitizeFilename(c.name)}.md`,
        status: c.status,
        plugin: c.pluginName,
        taskType: c.taskType,
        sourcePath: c.sourcePath,
        durationMs: c.durationMs,
        progress: c.progress,
        phase: c.phase,
      };
      // 🆕 可选 performance 字段（向后兼容 v1）
      if (c.performance) {
        caseEntry.performance = {
          avgThroughput: c.performance.avgThroughput,
          grade: c.performance.grade,
          gradeScore: c.performance.gradeScore,
          totalDurationMs: c.performance.totalDurationMs,
          sourceSize: c.performance.sourceSize,
          outputSize: c.performance.outputSize,
        };
      }
      return caseEntry;
    }),
  };

  // 🆕 顶层 performance 聚合（如果有任何 case 含性能数据）
  if (Object.keys(perfByPlugin).length > 0) {
    report.performance = Object.entries(perfByPlugin).map(([name, p]) => ({
      plugin: name,
      ...p,
    }));
  }

  // 🆕 顶层 calibration（如果调用方提供）
  if (options?.calibration) {
    report.calibration = {
      cpuScore: options.calibration.cpuScore,
      aesThroughput: options.calibration.aesThroughput,
      cpuLabel: options.calibration.cpuLabel,
      calibratedAt: options.calibration.calibratedAt,
      goVersion: options.calibration.goVersion,
      os: options.calibration.os,
      arch: options.calibration.arch,
      numCpu: options.calibration.numCpu,
    };
  }

  return JSON.stringify(report, null, 2);
}

/** 2. summary.md — 人类可读概览 */
function buildSummaryMd(
  run: UnifiedRunRecord,
  runTasks: EncvTask[],
  t: TFunction,
  options?: { calibration?: CalibrationResult | null }
): string {
  const wRun = run.workflowRun;
  const cases = flattenCases(run, runTasks);
  const triggeredByLabel = wRun?.triggeredBy ? t(`tasks.triggeredBy_${wRun.triggeredBy}`) : "N/A";

  // 按 plugin 聚合表格
  const pluginStats: Record<string, { total: number; passed: number; failed: number }> = {};
  for (const c of cases) {
    if (!pluginStats[c.pluginName]) {
      pluginStats[c.pluginName] = { total: 0, passed: 0, failed: 0 };
    }
    const s = pluginStats[c.pluginName];
    s.total += 1;
    if (c.status === "success") s.passed += 1;
    else if (c.status === "failure") s.failed += 1;
  }

  const lines: string[] = [];
  lines.push(`# ${t("tasks.reportTitle")}`);
  lines.push("");
  lines.push(`## ${t("tasks.reportOverview")}`);
  lines.push("");
  lines.push(`- ${t("tasks.reportRunId")}: \`${run.id}\``);
  lines.push(`- ${t("tasks.reportTriggeredBy")}: ${triggeredByLabel}`);
  lines.push(`- ${t("tasks.reportStartedAt")}: ${formatLocalTime(run.startedAt)}`);
  lines.push(`- ${t("tasks.reportCompletedAt")}: ${formatLocalTime(run.completedAt)}`);
  lines.push(`- ${t("tasks.reportDuration")}: ${formatDuration(wRun?.durationMs)}`);
  lines.push(`- ${t("tasks.reportStatus")}: \`${wRun?.status ?? "N/A"}\``);
  lines.push("");
  lines.push(`## ${t("tasks.reportSummary")}`);
  lines.push("");
  lines.push(`- ${t("tasks.reportTotal")}: **${run.totalCases}**`);
  lines.push(`- ${t("tasks.reportPassed")}: **${run.passed}** ✅`);
  lines.push(`- ${t("tasks.reportFailed")}: **${run.failed}** ❌`);
  lines.push(`- ${t("tasks.reportSkipped")}: **${run.skipped}** ⏭️`);
  lines.push("");
  lines.push(`## ${t("tasks.reportPerPlugin")}`);
  lines.push("");
  lines.push(`| ${t("tasks.reportPlugin")} | ${t("tasks.reportTotal")} | ${t("tasks.reportPassed")} | ${t("tasks.reportFailed")} |`);
  lines.push(`| --- | ---: | ---: | ---: |`);
  for (const [name, s] of Object.entries(pluginStats)) {
    lines.push(`| ${name} | ${s.total} | ${s.passed} | ${s.failed} |`);
  }
  lines.push("");

  // 🆕 性能聚合表格（如果有性能数据）
  const perfByPlugin = aggregatePerformanceByPlugin(cases);
  if (Object.keys(perfByPlugin).length > 0) {
    lines.push(`## ${t("tasks.performance.reportAggregation")}`);
    lines.push("");
    lines.push(
      `| ${t("tasks.reportPlugin")} | ${t("tasks.performance.caseCount")} | ${t("tasks.performance.avgThroughput")} | ${t("tasks.performance.grade")} |`
    );
    lines.push(`| --- | ---: | ---: | :--- |`);
    for (const [name, p] of Object.entries(perfByPlugin)) {
      const gradeStr = formatGradeDistribution(p.gradeDistribution, t);
      lines.push(`| ${name} | ${p.count} | ${p.avgThroughput.toFixed(1)} MB/s | ${gradeStr} |`);
    }
    lines.push("");
    if (options?.calibration) {
      lines.push(
        `- ${t("tasks.performance.calibrationTitle")}: CPUScore ${options.calibration.cpuScore.toFixed(2)} (${options.calibration.cpuLabel})`
      );
      lines.push(`- ${t("tasks.performance.calibratedAt")}: ${formatLocalTime(options.calibration.calibratedAt)}`);
      lines.push("");
    }
    lines.push(`> ${t("tasks.performance.reportSeeDetail")}: [performance.md](performance.md)`);
    lines.push("");
  }

  // 失败 case 摘要
  const failures = cases.filter(c => c.status === "failure");
  if (failures.length > 0) {
    lines.push(`## ${t("tasks.reportFailedCases")} (${failures.length})`);
    lines.push("");
    for (const c of failures) {
      const filename = `cases/${String(c.index).padStart(3, "0")}-failure-${sanitizeFilename(c.name)}.md`;
      lines.push(
        `- [${String(c.index).padStart(3, "0")}] **${c.pluginName}** · ${c.taskType} · \`${truncatePath(c.sourcePath)}\` — [${t("tasks.reportViewDetails")}](${filename})`
      );
      if (c.error) {
        lines.push(`  - \`${truncateError(c.error)}\``);
      }
    }
    lines.push("");
  }

  lines.push("---");
  lines.push("");
  lines.push(`${t("tasks.reportGeneratedAt")}: ${formatLocalTime(new Date().toISOString())}`);
  lines.push("");

  return lines.join("\n");
}

/** 3. cases.md — case 索引（按状态分组） */
function buildCasesIndexMd(grouped: CasesByStatus, t: TFunction): string {
  const lines: string[] = [];
  lines.push(`# ${t("tasks.reportCasesIndexTitle")}`);
  lines.push("");
  lines.push(`${t("tasks.reportCasesIndexDesc")}`);
  lines.push("");

  for (const [statusKey, statusLabel, emoji] of [
    ["success", t("tasks.reportPassed"), "✅"],
    ["failure", t("tasks.reportFailed"), "❌"],
    ["skipped", t("tasks.reportSkipped"), "⏭️"],
  ] as const) {
    const list = grouped[statusKey as keyof CasesByStatus];
    if (list.length === 0) continue;
    lines.push(`## ${emoji} ${statusLabel} (${list.length})`);
    lines.push("");
    for (const c of list) {
      const filename = `cases/${String(c.index).padStart(3, "0")}-${c.status}-${sanitizeFilename(c.name)}.md`;
      lines.push(
        `- [${String(c.index).padStart(3, "0")}] **${c.pluginName}** · ${c.taskType} · \`${truncatePath(c.sourcePath)}\` · ${formatDuration(c.durationMs)} → [${t("tasks.reportViewDetails")}](${filename})`
      );
    }
    lines.push("");
  }

  return lines.join("\n");
}

/** 4. metadata.json — 报告元数据 */
function buildMetadataJson(
  run: UnifiedRunRecord,
  options?: { locale?: string; deviceInfo?: { platform?: string; model?: string; osVersion?: string; appVersion?: string } }
): string {
  return JSON.stringify(
    {
      schema: "encv-report-metadata/v1",
      generatedAt: new Date().toISOString(),
      generator: {
        name: "encv-mobile",
        version: options?.deviceInfo?.appVersion ?? "1.0.0",
      },
      run: {
        id: run.id,
        caseCount: run.totalCases,
      },
      device: {
        platform: options?.deviceInfo?.platform ?? "unknown",
        model: options?.deviceInfo?.model ?? "unknown",
        osVersion: options?.deviceInfo?.osVersion ?? "unknown",
      },
      app: {
        locale: options?.locale ?? "zh-CN",
      },
    },
    null,
    2
  );
}

/** 5. timeline.txt — 任务时间线（NDJSON-like，按时间戳排序） */
function buildTimelineTxt(run: UnifiedRunRecord): string {
  const lines: string[] = [];
  const wRun = run.workflowRun;
  if (!wRun) {
    return `# Timeline\n# No workflowRun data\n`;
  }

  lines.push(`# ENCV Run Timeline (NDJSON-like, sorted by timestamp)`);
  lines.push(`# runId=${run.id}`);
  lines.push("");

  // 收集所有事件
  const events: Array<{ ts: string; line: string }> = [];

  // run 事件
  events.push({ ts: wRun.createdAt, line: `RUN_CREATED id=${wRun.id} triggeredBy=${wRun.triggeredBy}` });
  if (wRun.startedAt) {
    events.push({ ts: wRun.startedAt, line: `RUN_STARTED id=${wRun.id}` });
  }

  // step 事件
  for (const job of wRun.jobs) {
    for (const step of job.steps) {
      if (step.startedAt) {
        events.push({ ts: step.startedAt, line: `TASK_STARTED id=${step.id} stepDefId=${step.stepDefId} status=running` });
      }
      if (step.completedAt) {
        events.push({
          ts: step.completedAt,
          line: `TASK_COMPLETED id=${step.id} status=${step.status} durationMs=${step.durationMs ?? "N/A"}`,
        });
      }
    }
  }

  // run 完成
  if (wRun.completedAt) {
    events.push({
      ts: wRun.completedAt,
      line: `RUN_COMPLETED id=${wRun.id} status=${wRun.status} total=${run.totalCases} passed=${run.passed} failed=${run.failed}`,
    });
  }

  // 按时间戳排序
  events.sort((a, b) => a.ts.localeCompare(b.ts));

  for (const e of events) {
    lines.push(`${e.ts} ${e.line}`);
  }

  return lines.join("\n") + "\n";
}

/** 🆕 6.5. performance.md — 性能报告（硬件校准 + plugin 聚合 + phase 分布） */
function buildPerformanceMd(
  run: UnifiedRunRecord,
  runTasks: EncvTask[],
  t: TFunction,
  options?: { calibration?: CalibrationResult | null }
): string {
  const cases = flattenCases(run, runTasks);
  const perfByPlugin = aggregatePerformanceByPlugin(cases);
  const lines: string[] = [];

  lines.push(`# ${t("tasks.performance.reportTitle")}`);
  lines.push("");

  // 硬件校准信息
  if (options?.calibration) {
    const cal = options.calibration;
    lines.push(`## ${t("tasks.performance.calibrationTitle")}`);
    lines.push("");
    lines.push(`- CPUScore: **${cal.cpuScore.toFixed(2)}** (${cal.cpuLabel})`);
    lines.push(`- AES-CTR ${t("tasks.performance.throughput")}: ${cal.aesThroughput.toFixed(0)} MB/s`);
    lines.push(`- ${t("tasks.performance.calibratedAt")}: ${formatLocalTime(cal.calibratedAt)}`);
    lines.push(`- ${t("tasks.performance.platform")}: ${cal.os}/${cal.arch} (${cal.numCpu} CPU)`);
    lines.push(`- Go: ${cal.goVersion}`);
    lines.push("");
  }

  // Plugin 性能详情
  if (Object.keys(perfByPlugin).length === 0) {
    lines.push(`> ${t("tasks.performance.noData")}`);
    lines.push("");
    return lines.join("\n");
  }

  lines.push(`## ${t("tasks.performance.pluginAggregation")}`);
  lines.push("");
  lines.push(
    `| ${t("tasks.reportPlugin")} | ${t("tasks.performance.caseCount")} | ${t("tasks.performance.avgThroughput")} | ${t("tasks.performance.grade")} |`
  );
  lines.push(`| --- | ---: | ---: | :--- |`);
  for (const [name, p] of Object.entries(perfByPlugin)) {
    const gradeStr = formatGradeDistribution(p.gradeDistribution, t);
    lines.push(`| ${name} | ${p.count} | ${p.avgThroughput.toFixed(1)} MB/s | ${gradeStr} |`);
  }
  lines.push("");

  // 每个 plugin 的 case 明细
  lines.push(`## ${t("tasks.performance.caseDetails")}`);
  lines.push("");
  for (const pluginName of Object.keys(perfByPlugin)) {
    lines.push(`### ${pluginName}`);
    lines.push("");
    lines.push(
      `| # | ${t("tasks.performance.avgThroughput")} | ${t("tasks.performance.grade")} | ${t("tasks.performance.totalDuration")} | ${t("tasks.performance.sourceSize")} | ${t("tasks.performance.outputSize")} |`
    );
    lines.push(`| ---: | ---: | :--- | ---: | ---: | ---: |`);
    const pluginCases = cases.filter(c => c.pluginName === pluginName && c.performance);
    for (const c of pluginCases) {
      const p = c.performance!;
      const gradeLabel = gradeLabelLocalized(p.grade, t);
      lines.push(
        `| ${String(c.index).padStart(3, "0")} | ${p.avgThroughput.toFixed(1)} MB/s | ${gradeLabel} (${p.gradeScore.toFixed(0)}) | ${formatDuration(p.totalDurationMs)} | ${formatBytes(p.sourceSize)} | ${formatBytes(p.outputSize)} |`
      );
    }
    lines.push("");
  }

  lines.push("---");
  lines.push("");
  lines.push(`[${t("tasks.reportCaseBackToIndex")}](summary.md)`);

  return lines.join("\n");
}

/** 🆕 按 pluginName 聚合性能数据 */
function aggregatePerformanceByPlugin(cases: CaseDetail[]): Record<
  string,
  {
    count: number;
    avgThroughput: number;
    avgGradeScore: number;
    gradeDistribution: { excellent: number; good: number; warn: number };
  }
> {
  const result: Record<
    string,
    {
      count: number;
      avgThroughput: number;
      avgGradeScore: number;
      gradeDistribution: { excellent: number; good: number; warn: number };
    }
  > = {};
  for (const c of cases) {
    if (!c.performance) continue;
    const key = c.pluginName;
    if (!result[key]) {
      result[key] = {
        count: 0,
        avgThroughput: 0,
        avgGradeScore: 0,
        gradeDistribution: { excellent: 0, good: 0, warn: 0 },
      };
    }
    const p = result[key];
    p.count += 1;
    p.avgThroughput += c.performance.avgThroughput;
    p.avgGradeScore += c.performance.gradeScore;
    if (c.performance.grade === "excellent") p.gradeDistribution.excellent += 1;
    else if (c.performance.grade === "good") p.gradeDistribution.good += 1;
    else p.gradeDistribution.warn += 1;
  }
  for (const p of Object.values(result)) {
    if (p.count > 0) {
      p.avgThroughput = p.avgThroughput / p.count;
      p.avgGradeScore = p.avgGradeScore / p.count;
    }
  }
  return result;
}

/** 🆕 格式化评级分布为字符串 */
function formatGradeDistribution(dist: { excellent: number; good: number; warn: number }, t: TFunction): string {
  const parts: string[] = [];
  if (dist.excellent > 0) parts.push(`${dist.excellent} ${t("tasks.performance.grade.excellent")}`);
  if (dist.good > 0) parts.push(`${dist.good} ${t("tasks.performance.grade.good")}`);
  if (dist.warn > 0) parts.push(`${dist.warn} ${t("tasks.performance.grade.warn")}`);
  return parts.length > 0 ? parts.join(" / ") : "-";
}

/** 🆕 评级本地化标签 */
function gradeLabelLocalized(grade: "excellent" | "good" | "warn", t: TFunction): string {
  if (grade === "excellent") return t("tasks.performance.grade.excellent");
  if (grade === "good") return t("tasks.performance.grade.good");
  return t("tasks.performance.grade.warn");
}

/** 🆕 格式化字节数为人类可读（KB/MB/GB） */
function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = bytes;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

/** 6. cases/*.md — 单 case 详情 */
function buildCaseDetailMd(c: CaseDetail, caseNumber: number, t: TFunction): string {
  const lines: string[] = [];
  const statusEmoji = c.status === "success" ? "✅" : c.status === "failure" ? "❌" : "⏭️";
  const statusLabel =
    c.status === "success"
      ? t("tasks.reportStatusPassed")
      : c.status === "failure"
        ? t("tasks.reportStatusFailed")
        : t("tasks.reportStatusSkipped");

  lines.push(`# Case #${String(caseNumber).padStart(3, "0")}: ${c.pluginName} ${c.taskType}`);
  lines.push("");
  lines.push(`**${t("tasks.reportCaseStatus")}**: ${statusEmoji} ${statusLabel}`);
  lines.push("");
  lines.push(`## ${t("tasks.reportCaseBasicInfo")}`);
  lines.push("");
  lines.push(`- ${t("tasks.reportCaseId")}: \`${c.id}\``);
  lines.push(`- ${t("tasks.reportCasePlugin")}: \`${c.pluginName}\``);
  lines.push(`- ${t("tasks.reportCaseType")}: \`${c.taskType}\``);
  lines.push(`- ${t("tasks.reportCaseSource")}: \`${c.sourcePath}\``);
  if (c.targetPath) {
    lines.push(`- ${t("tasks.reportCaseTarget")}: \`${c.targetPath}\``);
  }
  lines.push(`- ${t("tasks.reportCaseStarted")}: ${formatLocalTime(c.startedAt)}`);
  lines.push(`- ${t("tasks.reportCaseCompleted")}: ${formatLocalTime(c.completedAt)}`);
  lines.push(`- ${t("tasks.reportCaseDuration")}: ${formatDuration(c.durationMs)}`);
  lines.push("");

  // 实时进度信息（如果有）
  if (c.progress != null || c.phase || c.speed || c.eta) {
    lines.push(`## ${t("tasks.reportCaseLiveProgress")}`);
    lines.push("");
    if (c.phase) lines.push(`- ${t("tasks.reportCasePhase")}: \`${c.phase}\``);
    if (c.progress != null) lines.push(`- ${t("tasks.reportCaseProgress")}: **${c.progress}%**`);
    if (c.speed) lines.push(`- ${t("tasks.reportCaseSpeed")}: \`${c.speed}\``);
    if (c.eta) lines.push(`- ${t("tasks.reportCaseEta")}: \`${c.eta}\``);
    lines.push("");
  }

  // 🆕 性能指标区块（如果有 performanceSummary）
  if (c.performance) {
    const p = c.performance;
    lines.push(`## ${t("tasks.performance.caseMetricsTitle")}`);
    lines.push("");
    lines.push(`| ${t("tasks.performance.metric")} | ${t("tasks.performance.value")} |`);
    lines.push(`| :--- | :--- |`);
    lines.push(`| ${t("tasks.performance.avgThroughput")} | **${p.avgThroughput.toFixed(1)} MB/s** |`);
    lines.push(`| ${t("tasks.performance.grade")} | ${gradeLabelLocalized(p.grade, t)} (${p.gradeScore.toFixed(0)}/100) |`);
    lines.push(`| ${t("tasks.performance.totalDuration")} | ${formatDuration(p.totalDurationMs)} |`);
    lines.push(`| ${t("tasks.performance.sourceSize")} | ${formatBytes(p.sourceSize)} |`);
    lines.push(`| ${t("tasks.performance.outputSize")} | ${formatBytes(p.outputSize)} |`);
    if (p.sourceSize > 0 && p.outputSize > 0) {
      const ratio = ((p.outputSize / p.sourceSize) * 100).toFixed(1);
      lines.push(`| ${t("tasks.performance.sizeRatio")} | ${ratio}% |`);
    }
    lines.push("");
  }

  // 错误信息
  if (c.error) {
    lines.push(`## ${t("tasks.reportCaseError")}`);
    lines.push("");
    lines.push("```");
    lines.push(c.error);
    lines.push("```");
    lines.push("");
  }

  // step 原始数据（AI 友好的机器可读附录）
  lines.push(`## ${t("tasks.reportCaseStepSnapshot")}`);
  lines.push("");
  lines.push("```json");
  lines.push(
    JSON.stringify(
      {
        id: c.step.id,
        stepDefId: c.step.stepDefId,
        status: c.step.status,
        startedAt: c.step.startedAt,
        completedAt: c.step.completedAt,
        durationMs: c.step.durationMs,
        progress: c.step.progress,
        phase: c.step.phase,
        error: c.step.error,
        taskId: c.step.taskId,
        matrixVars: c.step.matrixVars,
      },
      null,
      2
    )
  );
  lines.push("```");
  lines.push("");

  lines.push("---");
  lines.push("");
  lines.push(`[${t("tasks.reportCaseBackToIndex")}](../cases.md)`);

  return lines.join("\n");
}

/** 截断路径（避免长路径破坏 md 排版） */
function truncatePath(p: string, max = 60): string {
  if (!p) return "";
  if (p.length <= max) return p;
  return `...${p.slice(-(max - 3))}`;
}

/** 截断错误（单行） */
function truncateError(e: string, max = 120): string {
  if (!e) return "";
  const firstLine = e.split("\n")[0];
  if (firstLine.length <= max) return firstLine;
  return `${firstLine.slice(0, max - 3)}...`;
}
