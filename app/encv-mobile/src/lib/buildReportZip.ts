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

import JSZip from 'jszip'
import type { UnifiedRunRecord, JobRun, StepRun } from './workflow/types'
import type { EncvTask } from '@/api/encv'
import type { TFunction } from '@/composables/useI18n'

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
    locale?: string
    deviceInfo?: {
      platform?: string
      model?: string
      osVersion?: string
      appVersion?: string
    }
  },
): Promise<Blob> {
  const zip = new JSZip()

  // 1. report.json — AI 解析主入口
  zip.file('report.json', buildReportJson(run, runTasks))

  // 2. summary.md — 人类可读概览
  zip.file('summary.md', buildSummaryMd(run, runTasks, t))

  // 3. cases.md — case 索引
  const casesByStatus = groupCasesByStatus(run, runTasks)
  zip.file('cases.md', buildCasesIndexMd(casesByStatus, t))

  // 4. metadata.json — 报告元数据
  zip.file('metadata.json', buildMetadataJson(run, options))

  // 5. timeline.txt — 任务时间线（按时间戳排序）
  zip.file('timeline.txt', buildTimelineTxt(run))

  // 6. cases/*.md — 单 case 详情（cases 目录）
  const casesFolder = zip.folder('cases')
  if (casesFolder) {
    const cases = flattenCases(run, runTasks)
    cases.forEach((c, idx) => {
      const filename = `${String(idx + 1).padStart(3, '0')}-${c.status}-${sanitizeFilename(c.name)}.md`
      casesFolder.file(filename, buildCaseDetailMd(c, idx + 1, t))
    })
  }

  return await zip.generateAsync({ type: 'blob', compression: 'DEFLATE', compressionOptions: { level: 6 } })
}

// ==================== Data Structures ====================

interface CaseDetail {
  /** step.id（workflowRun.steps[].id） */
  id: string
  /** 序号 1-based */
  index: number
  /** 显示名（plugin-name + taskType） */
  name: string
  /** 状态（success/failure/skipped） */
  status: 'success' | 'failure' | 'skipped'
  /** 插件名 */
  pluginName: string
  /** 任务类型（encrypt/decrypt） */
  taskType: 'encrypt' | 'decrypt'
  /** 源路径 */
  sourcePath: string
  /** 目标路径 */
  targetPath?: string
  /** 开始时间 ISO */
  startedAt?: string
  /** 完成时间 ISO */
  completedAt?: string
  /** 耗时 ms */
  durationMs?: number
  /** 实时进度（0-100） */
  progress?: number
  /** 实时阶段 */
  phase?: string
  /** 实时速率 */
  speed?: string
  /** 实时剩余时间 */
  eta?: string
  /** 错误信息 */
  error?: string
  /** step 原始数据 */
  step: StepRun
  /** 关联的 job 原始数据 */
  job: JobRun
}

interface CasesByStatus {
  success: CaseDetail[]
  failure: CaseDetail[]
  skipped: CaseDetail[]
}

// ==================== Helpers ====================

/** 把 UnifiedRunRecord + runTasks 扁平化成 CaseDetail[] */
function flattenCases(run: UnifiedRunRecord, runTasks: EncvTask[]): CaseDetail[] {
  const wRun = run.workflowRun
  if (!wRun) return []

  // 实时 EncvTask 按 id 索引
  const liveById = new Map<string, EncvTask>()
  for (const t of runTasks) {
    liveById.set(t.id, t)
  }

  const cases: CaseDetail[] = []
  let counter = 0
  for (const job of wRun.jobs) {
    for (const step of job.steps) {
      counter += 1
      const action = inferAction(step)
      const status = step.status === 'success' ? 'success'
        : step.status === 'skipped' ? 'skipped'
        : 'failure'

      // 关联实时数据：step.taskId 是 EncvTask.id
      const live = step.taskId ? liveById.get(step.taskId) : undefined

      cases.push({
        id: step.id,
        index: counter,
        name: action ? `${action.pluginName}-${action.taskType}` : step.id,
        status,
        pluginName: action?.pluginName ?? 'unknown',
        taskType: action?.taskType ?? 'encrypt',
        sourcePath: action?.params?.sourcePath ?? '',
        targetPath: action?.params?.targetPath ?? action?.params?.sourcePath,
        startedAt: step.startedAt,
        completedAt: step.completedAt,
        durationMs: step.durationMs,
        progress: live?.progress ?? step.progress,
        phase: live?.phase ?? step.phase,
        speed: live?.speed,
        eta: live?.eta,
        error: step.error ?? live?.error,
        step,
        job,
      })
    }
  }
  return cases
}

function groupCasesByStatus(run: UnifiedRunRecord, runTasks: EncvTask[]): CasesByStatus {
  const grouped: CasesByStatus = { success: [], failure: [], skipped: [] }
  for (const c of flattenCases(run, runTasks)) {
    grouped[c.status].push(c)
  }
  return grouped
}

/** 从 step 推断 ActionSpec（pluginName / taskType / params） */
function inferAction(step: StepRun): {
  pluginName: string
  taskType: 'encrypt' | 'decrypt'
  params?: { sourcePath?: string; targetPath?: string }
} | null {
  const matrixVars = (step as any).matrixVars
  if (matrixVars && typeof matrixVars === 'object') {
    return {
      pluginName: String(matrixVars.pluginName ?? matrixVars.plugin ?? 'unknown'),
      taskType: (matrixVars.taskType === 'decrypt' ? 'decrypt' : 'encrypt'),
      params: {
        sourcePath: matrixVars.sourcePath,
        targetPath: matrixVars.targetPath,
      },
    }
  }
  return null
}

/** 文件名安全化（去除 / \ : * ? " < > | 空格转 -） */
function sanitizeFilename(name: string): string {
  return name
    .replace(/[/\\:*?"<>|]/g, '-')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .toLowerCase()
    .slice(0, 80)
}

/** 格式化 ISO 时间到本地 YYYY-MM-DD HH:mm:ss */
function formatLocalTime(iso?: string): string {
  if (!iso) return 'N/A'
  try {
    const d = new Date(iso)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  } catch {
    return iso
  }
}

/** 格式化 ms → "1m 23s" / "500ms" */
function formatDuration(ms?: number): string {
  if (ms == null) return 'N/A'
  if (ms < 1000) return `${ms}ms`
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}.${Math.floor((ms % 1000) / 100)}s`
  const m = Math.floor(s / 60)
  const rs = s % 60
  return `${m}m${rs}s`
}

// ==================== File Generators ====================

/** 1. report.json — AI 解析主入口 */
function buildReportJson(run: UnifiedRunRecord, runTasks: EncvTask[]): string {
  const wRun = run.workflowRun
  const cases = flattenCases(run, runTasks)

  // 按 plugin 聚合
  const pluginStats: Record<string, { total: number; passed: number; failed: number; skipped: number }> = {}
  for (const c of cases) {
    if (!pluginStats[c.pluginName]) {
      pluginStats[c.pluginName] = { total: 0, passed: 0, failed: 0, skipped: 0 }
    }
    const s = pluginStats[c.pluginName]
    s.total += 1
    if (c.status === 'success') s.passed += 1
    else if (c.status === 'failure') s.failed += 1
    else s.skipped += 1
  }

  // 失败 case 详情（AI 优先关注的）
  const failures = cases
    .filter((c) => c.status === 'failure')
    .map((c) => ({
      caseId: c.id,
      caseFile: `cases/${String(c.index).padStart(3, '0')}-failure-${sanitizeFilename(c.name)}.md`,
      plugin: c.pluginName,
      taskType: c.taskType,
      sourcePath: c.sourcePath,
      targetPath: c.targetPath,
      error: c.error,
      durationMs: c.durationMs,
      startedAt: c.startedAt,
      completedAt: c.completedAt,
    }))

  return JSON.stringify({
    schema: 'encv-report/v1',
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
      percent: run.totalCases > 0
        ? Math.round(((run.passed + run.skipped) / run.totalCases) * 100)
        : 0,
    },
    plugins: Object.entries(pluginStats).map(([name, s]) => ({ name, ...s })),
    failures,
    cases: cases.map((c) => ({
      caseId: c.id,
      caseFile: `cases/${String(c.index).padStart(3, '0')}-${c.status}-${sanitizeFilename(c.name)}.md`,
      status: c.status,
      plugin: c.pluginName,
      taskType: c.taskType,
      sourcePath: c.sourcePath,
      durationMs: c.durationMs,
      progress: c.progress,
      phase: c.phase,
    })),
  }, null, 2)
}

/** 2. summary.md — 人类可读概览 */
function buildSummaryMd(run: UnifiedRunRecord, runTasks: EncvTask[], t: TFunction): string {
  const wRun = run.workflowRun
  const cases = flattenCases(run, runTasks)
  const triggeredByLabel = wRun?.triggeredBy
    ? t(`tasks.triggeredBy_${wRun.triggeredBy}`)
    : 'N/A'

  // 按 plugin 聚合表格
  const pluginStats: Record<string, { total: number; passed: number; failed: number }> = {}
  for (const c of cases) {
    if (!pluginStats[c.pluginName]) {
      pluginStats[c.pluginName] = { total: 0, passed: 0, failed: 0 }
    }
    const s = pluginStats[c.pluginName]
    s.total += 1
    if (c.status === 'success') s.passed += 1
    else if (c.status === 'failure') s.failed += 1
  }

  const lines: string[] = []
  lines.push(`# ${t('tasks.reportTitle')}`)
  lines.push('')
  lines.push(`## ${t('tasks.reportOverview')}`)
  lines.push('')
  lines.push(`- ${t('tasks.reportRunId')}: \`${run.id}\``)
  lines.push(`- ${t('tasks.reportTriggeredBy')}: ${triggeredByLabel}`)
  lines.push(`- ${t('tasks.reportStartedAt')}: ${formatLocalTime(run.startedAt)}`)
  lines.push(`- ${t('tasks.reportCompletedAt')}: ${formatLocalTime(run.completedAt)}`)
  lines.push(`- ${t('tasks.reportDuration')}: ${formatDuration(wRun?.durationMs)}`)
  lines.push(`- ${t('tasks.reportStatus')}: \`${wRun?.status ?? 'N/A'}\``)
  lines.push('')
  lines.push(`## ${t('tasks.reportSummary')}`)
  lines.push('')
  lines.push(`- ${t('tasks.reportTotal')}: **${run.totalCases}**`)
  lines.push(`- ${t('tasks.reportPassed')}: **${run.passed}** ✅`)
  lines.push(`- ${t('tasks.reportFailed')}: **${run.failed}** ❌`)
  lines.push(`- ${t('tasks.reportSkipped')}: **${run.skipped}** ⏭️`)
  lines.push('')
  lines.push(`## ${t('tasks.reportPerPlugin')}`)
  lines.push('')
  lines.push(`| ${t('tasks.reportPlugin')} | ${t('tasks.reportTotal')} | ${t('tasks.reportPassed')} | ${t('tasks.reportFailed')} |`)
  lines.push(`| --- | ---: | ---: | ---: |`)
  for (const [name, s] of Object.entries(pluginStats)) {
    lines.push(`| ${name} | ${s.total} | ${s.passed} | ${s.failed} |`)
  }
  lines.push('')

  // 失败 case 摘要
  const failures = cases.filter((c) => c.status === 'failure')
  if (failures.length > 0) {
    lines.push(`## ${t('tasks.reportFailedCases')} (${failures.length})`)
    lines.push('')
    for (const c of failures) {
      const filename = `cases/${String(c.index).padStart(3, '0')}-failure-${sanitizeFilename(c.name)}.md`
      lines.push(`- [${String(c.index).padStart(3, '0')}] **${c.pluginName}** · ${c.taskType} · \`${truncatePath(c.sourcePath)}\` — [${t('tasks.reportViewDetails')}](${filename})`)
      if (c.error) {
        lines.push(`  - \`${truncateError(c.error)}\``)
      }
    }
    lines.push('')
  }

  lines.push('---')
  lines.push('')
  lines.push(`${t('tasks.reportGeneratedAt')}: ${formatLocalTime(new Date().toISOString())}`)
  lines.push('')

  return lines.join('\n')
}

/** 3. cases.md — case 索引（按状态分组） */
function buildCasesIndexMd(grouped: CasesByStatus, t: TFunction): string {
  const lines: string[] = []
  lines.push(`# ${t('tasks.reportCasesIndexTitle')}`)
  lines.push('')
  lines.push(`${t('tasks.reportCasesIndexDesc')}`)
  lines.push('')

  for (const [statusKey, statusLabel, emoji] of [
    ['success', t('tasks.reportPassed'), '✅'],
    ['failure', t('tasks.reportFailed'), '❌'],
    ['skipped', t('tasks.reportSkipped'), '⏭️'],
  ] as const) {
    const list = grouped[statusKey as keyof CasesByStatus]
    if (list.length === 0) continue
    lines.push(`## ${emoji} ${statusLabel} (${list.length})`)
    lines.push('')
    for (const c of list) {
      const filename = `cases/${String(c.index).padStart(3, '0')}-${c.status}-${sanitizeFilename(c.name)}.md`
      lines.push(`- [${String(c.index).padStart(3, '0')}] **${c.pluginName}** · ${c.taskType} · \`${truncatePath(c.sourcePath)}\` · ${formatDuration(c.durationMs)} → [${t('tasks.reportViewDetails')}](${filename})`)
    }
    lines.push('')
  }

  return lines.join('\n')
}

/** 4. metadata.json — 报告元数据 */
function buildMetadataJson(
  run: UnifiedRunRecord,
  options?: { locale?: string; deviceInfo?: { platform?: string; model?: string; osVersion?: string; appVersion?: string } },
): string {
  return JSON.stringify({
    schema: 'encv-report-metadata/v1',
    generatedAt: new Date().toISOString(),
    generator: {
      name: 'encv-mobile',
      version: options?.deviceInfo?.appVersion ?? '1.0.0',
    },
    run: {
      id: run.id,
      caseCount: run.totalCases,
    },
    device: {
      platform: options?.deviceInfo?.platform ?? 'unknown',
      model: options?.deviceInfo?.model ?? 'unknown',
      osVersion: options?.deviceInfo?.osVersion ?? 'unknown',
    },
    app: {
      locale: options?.locale ?? 'zh-CN',
    },
  }, null, 2)
}

/** 5. timeline.txt — 任务时间线（NDJSON-like，按时间戳排序） */
function buildTimelineTxt(run: UnifiedRunRecord): string {
  const lines: string[] = []
  const wRun = run.workflowRun
  if (!wRun) {
    return `# Timeline\n# No workflowRun data\n`
  }

  lines.push(`# ENCV Run Timeline (NDJSON-like, sorted by timestamp)`)
  lines.push(`# runId=${run.id}`)
  lines.push('')

  // 收集所有事件
  const events: Array<{ ts: string; line: string }> = []

  // run 事件
  events.push({ ts: wRun.createdAt, line: `RUN_CREATED id=${wRun.id} triggeredBy=${wRun.triggeredBy}` })
  if (wRun.startedAt) {
    events.push({ ts: wRun.startedAt, line: `RUN_STARTED id=${wRun.id}` })
  }

  // step 事件
  for (const job of wRun.jobs) {
    for (const step of job.steps) {
      if (step.startedAt) {
        events.push({ ts: step.startedAt, line: `TASK_STARTED id=${step.id} stepDefId=${step.stepDefId} status=running` })
      }
      if (step.completedAt) {
        events.push({ ts: step.completedAt, line: `TASK_COMPLETED id=${step.id} status=${step.status} durationMs=${step.durationMs ?? 'N/A'}` })
      }
    }
  }

  // run 完成
  if (wRun.completedAt) {
    events.push({ ts: wRun.completedAt, line: `RUN_COMPLETED id=${wRun.id} status=${wRun.status} total=${run.totalCases} passed=${run.passed} failed=${run.failed}` })
  }

  // 按时间戳排序
  events.sort((a, b) => a.ts.localeCompare(b.ts))

  for (const e of events) {
    lines.push(`${e.ts} ${e.line}`)
  }

  return lines.join('\n') + '\n'
}

/** 6. cases/*.md — 单 case 详情 */
function buildCaseDetailMd(c: CaseDetail, caseNumber: number, t: TFunction): string {
  const lines: string[] = []
  const statusEmoji = c.status === 'success' ? '✅' : c.status === 'failure' ? '❌' : '⏭️'
  const statusLabel = c.status === 'success'
    ? t('tasks.reportStatusPassed')
    : c.status === 'failure'
      ? t('tasks.reportStatusFailed')
      : t('tasks.reportStatusSkipped')

  lines.push(`# Case #${String(caseNumber).padStart(3, '0')}: ${c.pluginName} ${c.taskType}`)
  lines.push('')
  lines.push(`**${t('tasks.reportCaseStatus')}**: ${statusEmoji} ${statusLabel}`)
  lines.push('')
  lines.push(`## ${t('tasks.reportCaseBasicInfo')}`)
  lines.push('')
  lines.push(`- ${t('tasks.reportCaseId')}: \`${c.id}\``)
  lines.push(`- ${t('tasks.reportCasePlugin')}: \`${c.pluginName}\``)
  lines.push(`- ${t('tasks.reportCaseType')}: \`${c.taskType}\``)
  lines.push(`- ${t('tasks.reportCaseSource')}: \`${c.sourcePath}\``)
  if (c.targetPath) {
    lines.push(`- ${t('tasks.reportCaseTarget')}: \`${c.targetPath}\``)
  }
  lines.push(`- ${t('tasks.reportCaseStarted')}: ${formatLocalTime(c.startedAt)}`)
  lines.push(`- ${t('tasks.reportCaseCompleted')}: ${formatLocalTime(c.completedAt)}`)
  lines.push(`- ${t('tasks.reportCaseDuration')}: ${formatDuration(c.durationMs)}`)
  lines.push('')

  // 实时进度信息（如果有）
  if (c.progress != null || c.phase || c.speed || c.eta) {
    lines.push(`## ${t('tasks.reportCaseLiveProgress')}`)
    lines.push('')
    if (c.phase) lines.push(`- ${t('tasks.reportCasePhase')}: \`${c.phase}\``)
    if (c.progress != null) lines.push(`- ${t('tasks.reportCaseProgress')}: **${c.progress}%**`)
    if (c.speed) lines.push(`- ${t('tasks.reportCaseSpeed')}: \`${c.speed}\``)
    if (c.eta) lines.push(`- ${t('tasks.reportCaseEta')}: \`${c.eta}\``)
    lines.push('')
  }

  // 错误信息
  if (c.error) {
    lines.push(`## ${t('tasks.reportCaseError')}`)
    lines.push('')
    lines.push('```')
    lines.push(c.error)
    lines.push('```')
    lines.push('')
  }

  // step 原始数据（AI 友好的机器可读附录）
  lines.push(`## ${t('tasks.reportCaseStepSnapshot')}`)
  lines.push('')
  lines.push('```json')
  lines.push(JSON.stringify({
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
  }, null, 2))
  lines.push('```')
  lines.push('')

  lines.push('---')
  lines.push('')
  lines.push(`[${t('tasks.reportCaseBackToIndex')}](../cases.md)`)

  return lines.join('\n')
}

/** 截断路径（避免长路径破坏 md 排版） */
function truncatePath(p: string, max = 60): string {
  if (!p) return ''
  if (p.length <= max) return p
  return `...${p.slice(-(max - 3))}`
}

/** 截断错误（单行） */
function truncateError(e: string, max = 120): string {
  if (!e) return ''
  const firstLine = e.split('\n')[0]
  if (firstLine.length <= max) return firstLine
  return `${firstLine.slice(0, max - 3)}...`
}
