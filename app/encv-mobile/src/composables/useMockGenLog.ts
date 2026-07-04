/**
 * useMockGenLog — FFMPEG 流程日志状态 + 5 个 SSE 回调 + 交互函数
 *
 * 从 PluginTestsDetail.vue 抽取（Task 13 SubTask 13.1）。
 *
 * 设计目的：
 *   1. 即使后端 cgo 阻塞导致 SSE 流中断，前端也能展示「最后收到的 spec_diag」（在哪停止）
 *   2. 失败时一行红色高亮 + 完整 stderr 可点开
 *   3. 一键复制全部日志（带时间戳）—— 用户贴给开发者排查
 *   4. 流程每一步带：序号 / 状态 / relativePath / encoder / ffmpegArgs / exitCode / stderr
 *   5. 静态字节文件（JPEG/PNG/PDF/TXT/CSV）也展示（ffmpegArgs=[] 表明无 ffmpeg 调用）
 *
 * 行为一致性：5 个 SSE 回调（onSpecDiag / onSpecPlan / onProgress / onSpecFailed / onSkipped）
 * 完整迁移自 PluginTestsDetail.vue 608-707 行，保持状态转移 / 自动展开 / _marked 标记等逻辑不变。
 */
import { computed, ref } from "vue";
import type { MockProgress, MockSpecDiag, MockSpecFailed } from "@/api/mockGenerator";

/**
 * 单条 FFMPEG 流程日志条目
 *
 * 字段说明：
 *   - key: 唯一 key（index-relativePath-status），用于 v-for / toggle
 *   - index / total: 1-based 序号 / 总数
 *   - relativePath: 相对路径（如 "01-plain-media/video/sample.mp4"）
 *   - status: 'ok' | 'failed' | 'pending'（pending = spec_plan 阶段，未收到 spec_diag）
 *   - encoder: encoder 提示（"h264+aac (-c copy)" / "libmp3lame" / "JPEG (static)"）
 *   - runner: 'ffmpeg' | 'mediacodec' | 'static'（mediacodec=硬件⚡ / ffmpeg=软件⚙ / static=静态📄）
 *   - ffmpegArgs: 完整 ffmpeg 命令行（空数组 = 静态字节，无 ffmpeg 调用）
 *   - exitCode: ffmpeg 退出码（0=成功, 1=编码失败, 124=ctx timeout, -1=spawn/前置失败）
 *   - stderr: ffmpeg stderr 全文
 *   - at: ISO 时间戳
 *   - expanded: 是否展开详情
 *   - _marked: onProgress 标记过 ok 的不重复 mark（内部字段）
 *   - srcSize / dstSize / workerTmpDir / workerError / contextInfo: 增强调试字段（B1+B2 修复）
 */
export interface MockGenLogEntry {
  key: string;
  index: number;
  total: number;
  relativePath: string;
  status: "ok" | "failed" | "pending";
  encoder: string;
  /**
   * runner 标识
   *   - "ffmpeg": 软件编（沙箱 / 真机兜底）
   *   - "mediacodec": 硬件编（Phase 3.3 实装，UI 显示 ⚡）
   *   - "static": 静态字节（PNG/JPEG/PDF/TXT 等，UI 显示 📄）
   */
  runner: "ffmpeg" | "mediacodec" | "static" | string;
  ffmpegArgs: string[];
  exitCode: number;
  stderr: string;
  at: string;
  expanded: boolean;
  _marked?: boolean;
  srcSize?: number;
  dstSize?: number;
  workerTmpDir?: string;
  workerError?: string;
  contextInfo?: string;
}

/**
 * 流程日志汇总信息
 *
 *   - total: 总数（mockGenLogTotal，由 starting 事件设置）
 *   - ok: 成功数（status='ok'）
 *   - failed: 失败数（status='failed'）
 *   - skipped: 跳过数（status='pending'，未收到 spec_diag 的行）
 *   - disconnected: 流是否中断（total > 0 且 entries < total 且 (ok+failed) < total）
 */
export interface MockGenLogSummary {
  total: number;
  ok: number;
  failed: number;
  skipped: number;
  disconnected: boolean;
}

/**
 * 被跳过的文件记录（onSpecFailed / onSkipped 收集，供组件用于 mockStats / inline error）
 */
export interface SkippedFile {
  relativePath: string;
  reason: string;
  exitCode: number;
  stderr: string;
}

export interface UseMockGenLogOptions {
  /**
   * 获取 mock root 路径（用于 copyMockGenLog 输出 header）
   * 不传时 copy 文本不含 root 行
   */
  getRoot?: () => string;
}

export function useMockGenLog(options?: UseMockGenLogOptions) {
  // ---- 状态 ----
  const mockGenLog = ref<MockGenLogEntry[]>([]);
  const mockGenLogTotal = ref(0);
  const mockGenLogCopied = ref(false);
  /** 进度文本（如 "[3/9] 01-plain-media/video/sample.mp4 (ok)"） */
  const progressText = ref("");
  /** 被跳过的文件列表（onSpecFailed / onSkipped 收集） */
  const skippedFiles = ref<SkippedFile[]>([]);
  /** onProgress 累计的文件数（用于 mockStats fallback） */
  const lastCount = ref(0);
  /** onProgress 累计的字节数（用于 mockStats fallback） */
  const lastSize = ref(0);

  // ---- 汇总 computed ----
  const mockGenLogSummary = computed<MockGenLogSummary | null>(() => {
    if (mockGenLog.value.length === 0 && mockGenLogTotal.value === 0) return null;
    const failed = mockGenLog.value.filter(e => e.status === "failed").length;
    const ok = mockGenLog.value.filter(e => e.status === "ok").length;
    const pending = mockGenLog.value.filter(e => e.status === "pending").length;
    const disconnected =
      mockGenLogTotal.value > 0 && mockGenLog.value.length < mockGenLogTotal.value && failed + ok < mockGenLogTotal.value;
    return {
      total: mockGenLogTotal.value || mockGenLog.value.length,
      ok,
      failed,
      skipped: pending,
      disconnected,
    };
  });

  // ---- 5 个 SSE 回调（迁移自 PluginTestsDetail.vue 608-707 行）----

  /**
   * onSpecDiag: 每个 spec 处理前先记一行
   *   哪怕 progress 事件因 cgo 阻塞没收到，至少能看到「处理到这步」
   *   关键：用 relativePath + index 找已有 row（spec_plan 时已 push pending），
   *         替换为完整诊断版（status / stderr / exitCode）
   *   真机 cgo 阻塞时只有 plan 行（pending），诊断版（ok/failed）永远到不了 → 前端仍能看到 pending 行
   */
  function onSpecDiag(diag: MockSpecDiag): void {
    if (diag.relativePath === "__starting__") {
      // starting 事件：更新 total 即可
      mockGenLogTotal.value = diag.total;
      return;
    }
    // 找同 relativePath 已有 row（plan 阶段 push 过）
    const existing = mockGenLog.value.findIndex(e => e.relativePath === diag.relativePath && e.index === diag.index);
    const entry: MockGenLogEntry = {
      key: `${diag.index}-${diag.relativePath}-${diag.status}`,
      index: diag.index,
      total: diag.total,
      relativePath: diag.relativePath,
      status: diag.status,
      encoder: diag.encoder,
      runner: diag.runner,
      ffmpegArgs: diag.ffmpegArgs,
      exitCode: diag.exitCode,
      stderr: diag.stderr,
      at: new Date().toISOString(),
      expanded: diag.status === "failed", // 失败自动展开
      srcSize: diag.srcSize,
      dstSize: diag.dstSize,
      workerTmpDir: diag.workerTmpDir,
      workerError: diag.workerError,
      contextInfo: diag.contextInfo,
    };
    if (existing >= 0) {
      mockGenLog.value[existing] = entry;
    } else {
      mockGenLog.value.push(entry);
    }
    progressText.value = `[${diag.index}/${diag.total}] ${diag.relativePath} (${diag.status})`;
  }

  /**
   * onSpecPlan: handler 入口发的"待跑"列表（pending 状态）
   *   真机 cgo 阻塞时只有这些行能到达 → 前端能定位"卡在哪个 spec"
   */
  function onSpecPlan(diag: MockSpecDiag): void {
    if (diag.relativePath === "__starting__") {
      mockGenLogTotal.value = diag.total;
      return;
    }
    // 找同 relativePath 已有 row，避免重复 push
    const existing = mockGenLog.value.findIndex(e => e.relativePath === diag.relativePath && e.index === diag.index);
    const entry: MockGenLogEntry = {
      key: `${diag.index}-${diag.relativePath}-plan`,
      index: diag.index,
      total: diag.total,
      relativePath: diag.relativePath,
      status: "pending",
      encoder: diag.encoder,
      runner: diag.runner,
      ffmpegArgs: diag.ffmpegArgs,
      exitCode: 0,
      stderr: "",
      at: new Date().toISOString(),
      expanded: false,
    };
    if (existing >= 0) {
      // 保留已有行（plan 后已被 diag 替换过），不动
    } else {
      mockGenLog.value.push(entry);
    }
    mockGenLogTotal.value = diag.total;
  }

  /**
   * onProgress: 累计 count/size + 把对应 spec_diag 行标记为 ok
   */
  function onProgress(p: MockProgress): void {
    lastCount.value++;
    lastSize.value += p.size;
    progressText.value = `(${lastCount.value}) ${p.relativePath}`;
    // 把对应 spec_diag 行标记为 ok
    const e = mockGenLog.value.find(e => e.relativePath === p.relativePath && e.status === "ok" && e.exitCode === 0 && !e._marked);
    if (e) {
      e._marked = true;
    }
  }

  /**
   * onSpecFailed: spec 失败带完整 ffmpeg 诊断
   *   收集到 skippedFiles + 找对应 spec_diag 行更新状态 + 自动展开
   */
  function onSpecFailed(fail: MockSpecFailed): void {
    skippedFiles.value.push({
      relativePath: fail.relativePath,
      reason: fail.reason,
      exitCode: fail.exitCode,
      stderr: fail.stderr,
    });
    // 找对应的 spec_diag 行，更新状态 + 附加 stderr
    const e = mockGenLog.value.find(e => e.relativePath === fail.relativePath && e.status !== "failed");
    if (e) {
      e.status = "failed";
      e.exitCode = fail.exitCode;
      e.stderr = fail.stderr || e.stderr;
      e.expanded = true; // 自动展开失败行
    }
    progressText.value = `⚠️ 失败 ${fail.relativePath} (exit=${fail.exitCode})`;
    // eslint-disable-next-line no-console
    console.warn("[mock-gen] spec failed", fail);
  }

  /**
   * onSkipped: 被跳过的文件（通常是 ffmpeg build 没编该 encoder）
   */
  function onSkipped(info: { relativePath: string; reason: string }): void {
    skippedFiles.value.push({
      relativePath: info.relativePath,
      reason: info.reason,
      exitCode: -1,
      stderr: "",
    });
    progressText.value = `⚠️ 跳过 ${info.relativePath}（${info.reason}）`;
    // eslint-disable-next-line no-console
    console.warn("[mock-gen] skipped", info);
  }

  // ---- 交互函数 ----

  /** 切换某条日志的展开状态 */
  function toggleMockGenLogEntry(key: string): void {
    const e = mockGenLog.value.find(e => e.key === key);
    if (e) e.expanded = !e.expanded;
  }

  /** 复制全部日志到剪贴板（带 header + 每条 entry 完整信息） */
  function copyMockGenLog(): void {
    if (mockGenLog.value.length === 0) return;
    const lines: string[] = [];
    lines.push(`# ENCV Mock 生成流程日志`);
    lines.push(`# at: ${new Date().toISOString()}`);
    lines.push(`# total: ${mockGenLogTotal.value}`);
    lines.push(`# entries: ${mockGenLog.value.length}`);
    if (options?.getRoot) lines.push(`# root: ${options.getRoot()}`);
    lines.push(``);
    for (const e of mockGenLog.value) {
      const status = e.status === "ok" ? "✓" : e.status === "failed" ? "✗" : "◌";
      lines.push(`[${status}] [${e.index}/${e.total}] ${e.relativePath}`);
      lines.push(`    runner: ${e.runner}  (mediacodec=硬件⚡ / ffmpeg=软件⚙ / static=静态📄)`);
      lines.push(`    encoder: ${e.encoder}`);
      lines.push(`    ffmpeg args: ${e.ffmpegArgs.length > 0 ? e.ffmpegArgs.join(" ") : "(静态字节 - 无 ffmpeg 调用)"}`);
      lines.push(`    exit code: ${e.exitCode}`);
      lines.push(`    at: ${e.at}`);
      if (e.workerTmpDir) lines.push(`    worker tmp_dir: ${e.workerTmpDir}`);
      if (e.workerError) lines.push(`    worker error: ${e.workerError}`);
      if (e.srcSize !== undefined || e.dstSize !== undefined) {
        lines.push(`    file sizes: src=${e.srcSize ?? 0} bytes, dst=${e.dstSize ?? 0} bytes`);
      }
      if (e.contextInfo) lines.push(`    context: ${e.contextInfo}`);
      if (e.stderr) {
        lines.push(`    stderr:`);
        for (const ln of e.stderr.split("\n")) lines.push(`      ${ln}`);
      }
      lines.push(``);
    }
    const text = lines.join("\n");
    navigator.clipboard
      ?.writeText(text)
      .then(() => {
        mockGenLogCopied.value = true;
        setTimeout(() => {
          mockGenLogCopied.value = false;
        }, 2000);
      })
      .catch(e => {
        // eslint-disable-next-line no-console
        console.error("[mock-gen] copy failed", e);
        // fallback: 弹 prompt 让用户手动复制
        window.prompt("复制以下日志", text);
      });
  }

  /** 重置所有状态（开始新一轮生成前调用） */
  function resetMockGenLog(): void {
    mockGenLog.value = [];
    mockGenLogTotal.value = 0;
    mockGenLogCopied.value = false;
    progressText.value = "";
    skippedFiles.value = [];
    lastCount.value = 0;
    lastSize.value = 0;
  }

  return {
    // 状态
    mockGenLog,
    mockGenLogTotal,
    mockGenLogCopied,
    progressText,
    skippedFiles,
    lastCount,
    lastSize,
    // computed
    mockGenLogSummary,
    // 交互
    toggleMockGenLogEntry,
    copyMockGenLog,
    resetMockGenLog,
    // SSE 回调
    onSpecDiag,
    onSpecPlan,
    onProgress,
    onSpecFailed,
    onSkipped,
  };
}
