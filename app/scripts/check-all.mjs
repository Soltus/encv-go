#!/usr/bin/env node
/**
 * check-all.mjs — pnpm 工作区全量检查 + 报告
 *
 * 覆盖（与 migration-task-system.md §6 门禁对齐，并补全 plugin-* 项目）：
 *   1. Biome CI（根 workspace，lint 全部包，含 plugin-*）
 *   2. encv-mobile 类型检查（含 pretypecheck 的 i18n scan + var-check）
 *   3. shared-components 独立类型检查
 *   4. plugin-openlist/web 类型检查（vue-tsc --noEmit）
 *   5. plugin-mpv-player/web 类型检查（vue-tsc --noEmit）
 *   6. plugin-simverse/web 类型检查（vue-tsc --noEmit）
 *   7. i18n 全局 lint --all（仓库根 scripts/i18n-tool.py）
 *   8. encv-mobile 单元测试（vitest run，可用 --no-tests 跳过）
 *
 * 用法：
 *   pnpm check:all            # 跑全部
 *   pnpm check:all --no-tests # 跳过单元测试（快）
 *   node scripts/check-all.mjs --no-tests
 *
 * 退出码：任一检查失败 → 1；全部通过 → 0。
 * 报告同时打印到 stdout 并写入 <app-root>/check-report.md。
 */

import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { mkdirSync, writeFileSync, rmSync, existsSync, readdirSync } from "node:fs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const APP_ROOT = resolve(__dirname, ".."); // /workspace/app
const REPO_ROOT = resolve(APP_ROOT, ".."); // /workspace
const I18N_TOOL = resolve(REPO_ROOT, "scripts/i18n-tool.py");

const NO_TESTS = process.argv.includes("--no-tests");

/** 单个检查套件定义 */
const SUITES = [
  {
    name: "Biome CI (workspace)",
    cwd: APP_ROOT,
    cmd: "pnpm",
    args: ["biome:ci"],
    timeoutMs: 180_000,
  },
  {
    name: "encv-mobile typecheck",
    cwd: resolve(APP_ROOT, "encv-mobile"),
    cmd: "pnpm",
    // 直接调 vue-tsc --incremental：跳过 pnpm typecheck 的 pretypecheck(i18n) 钩子
    // （i18n 已由独立套件覆盖），并用增量缓存让热重跑 <5s。
    args: ["exec", "vue-tsc", "--noEmit", "--incremental"],
    timeoutMs: 300_000,
  },
  {
    name: "plugin-openlist/web typecheck",
    cwd: resolve(APP_ROOT, "encv-mobile/plugin-openlist/web"),
    cmd: "pnpm",
    args: ["exec", "vue-tsc", "--noEmit", "--incremental"],
    timeoutMs: 300_000,
  },
  {
    name: "plugin-mpv-player/web typecheck",
    cwd: resolve(APP_ROOT, "encv-mobile/plugin-mpv-player/web"),
    cmd: "pnpm",
    args: ["exec", "vue-tsc", "--noEmit", "--incremental"],
    timeoutMs: 300_000,
  },
  {
    name: "plugin-simverse/web typecheck",
    cwd: resolve(APP_ROOT, "encv-mobile/plugin-simverse/web"),
    cmd: "pnpm",
    args: ["exec", "vue-tsc", "--noEmit", "--incremental"],
    timeoutMs: 300_000,
  },
  {
    name: "shared-components typecheck",
    cwd: resolve(APP_ROOT, "packages/shared-components"),
    cmd: "pnpm",
    args: ["exec", "vue-tsc", "--noEmit", "-p", "tsconfig.json", "--incremental"],
    timeoutMs: 300_000,
  },
  {
    name: "i18n lint --all",
    cwd: REPO_ROOT,
    cmd: "python3",
    args: [I18N_TOOL, "lint", "--all"],
    timeoutMs: 180_000,
  },
  {
    name: "encv-mobile unit tests",
    cwd: resolve(APP_ROOT, "encv-mobile"),
    cmd: "pnpm",
    args: ["test:unit"],
    timeoutMs: 600_000,
    skip: NO_TESTS,
  },
];

const COLORS = {
  reset: "\x1b[0m",
  green: "\x1b[32m",
  red: "\x1b[31m",
  yellow: "\x1b[33m",
  bold: "\x1b[1m",
  dim: "\x1b[2m",
};
const c = (color, s) => `${COLORS[color]}${s}${COLORS.reset}`;

// 去除 ANSI 颜色转义序列，避免子进程（biome/tsc/vitest）的彩色输出
// 被原样写进 check-report.md 造成"乱码"。
const ANSI_RE = /\x1b\[[0-9;]*m/g;
const stripAnsi = (s) => (s || "").replace(ANSI_RE, "");

// 每个套件的完整输出写到独立日志文件，报告中仅用相对路径引用。
// 这样既不截断丢失上下文（之前只截最后 40 行），也不会把冗长输出塞进报告本身。
const LOGS_DIR = resolve(APP_ROOT, "check-logs");
const slugify = (s) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");

/**
 * 运行前清理上一轮的 check-logs 目录，并在终端输出清理情况。
 * 逐条删除，单条失败（占用/权限等）不中断整体，最后汇总报告。
 */
function cleanLogsDir() {
  console.log(`${c("bold", "CLEAN")}  ${c("dim", LOGS_DIR)}`);
  if (!existsSync(LOGS_DIR)) {
    console.log(c("dim", "  目录不存在，无需清理"));
    mkdirSync(LOGS_DIR, { recursive: true });
    console.log("");
    return;
  }
  let entries = [];
  try {
    entries = readdirSync(LOGS_DIR);
  } catch (e) {
    console.log(c("red", `  读取目录失败：${e.message}（将尝试强制重建）`));
    try {
      rmSync(LOGS_DIR, { recursive: true, force: true });
    } catch (e2) {
      console.log(c("red", `  强制删除也失败：${e2.message}`));
    }
    mkdirSync(LOGS_DIR, { recursive: true });
    console.log("");
    return;
  }
  let removed = 0;
  const failed = [];
  for (const name of entries) {
    const p = resolve(LOGS_DIR, name);
    try {
      // recursive+force：既能删文件也能删可能残留的子目录；force 忽略不存在
      rmSync(p, { recursive: true, force: true });
      removed++;
    } catch (e) {
      failed.push(`${name} (${e.code || e.message})`);
    }
  }
  console.log(c("dim", `  已清理 ${removed} 个旧日志条目`));
  if (failed.length) {
    console.log(c("yellow", `  ⚠ 跳过 ${failed.length} 个（占用/异常）：`));
    for (const f of failed) console.log(c("yellow", `    - ${f}`));
  }
  console.log("");
}

/** 运行单个命令，返回 { status, durationMs, stdout, stderr } */
function runSuite(suite) {
  return new Promise((resolvePromise) => {
    const start = Date.now();
    const child = spawn(suite.cmd, suite.args, {
      cwd: suite.cwd,
      // 强制子进程输出纯文本（关闭颜色），否则 biome 等会把 ANSI 颜色码
      // 写进 check-report.md，导致报告乱码。
      env: { ...process.env, NO_COLOR: "1", FORCE_COLOR: "0" },
      shell: false,
    });

    let out = "";
    let err = "";
    const onData = (buf, sink) => sink + buf.toString();
    child.stdout?.on("data", (b) => (out = onData(b, out)));
    child.stderr?.on("data", (b) => (err = onData(b, err)));

    let killed = false;
    const timer = setTimeout(() => {
      killed = true;
      try {
        process.kill(-child.pid, "SIGKILL");
      } catch {
        child.kill("SIGKILL");
      }
    }, suite.timeoutMs);

    child.on("error", (e) => {
      clearTimeout(timer);
      resolvePromise({ status: "ERROR", durationMs: Date.now() - start, stdout: out, stderr: `${err}\n${e.message}` });
    });

    child.on("close", (code) => {
      clearTimeout(timer);
      const durationMs = Date.now() - start;
      let status;
      if (killed) status = "TIMEOUT";
      else if (code === 0) status = "PASS";
      else status = "FAIL";
      resolvePromise({ status, durationMs, stdout: out, stderr: err });
    });
  });
}

function fmtDuration(ms) {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

/** 运行单个套件：打印进度、写完整日志，返回结果对象 */
async function runOne(suite) {
  if (suite.skip) {
    console.log(`${c("yellow", "SKIP")}  ${suite.name}`);
    return { ...suite, status: "SKIP", durationMs: 0, stdout: "", stderr: "", slug: slugify(suite.name) };
  }
  process.stdout.write(`${c("bold", "RUN")}   ${suite.name} ... `);
  const r = await runSuite(suite);
  const dur = fmtDuration(r.durationMs);
  const tag =
    r.status === "PASS"
      ? c("green", "PASS")
      : r.status === "TIMEOUT"
        ? c("yellow", "TIMEOUT")
        : c("red", r.status);
  console.log(`${tag}  ${c("dim", dur)}`);
  if (r.status !== "PASS") {
    const tail = (r.stderr || r.stdout || "").trim().split("\n").slice(-25).join("\n");
    console.log(c("dim", "────────── 输出尾部 ──────────"));
    console.log(tail);
    console.log(c("dim", "──────────────────────────────"));
  }
  const slug = slugify(suite.name);
  const logFile = resolve(LOGS_DIR, `${slug}.log`);
  writeFileSync(logFile, stripAnsi(`${r.stdout || ""}\n${r.stderr || ""}`), "utf8");
  return { ...suite, ...r, slug };
}

/** 带并发上限地运行所有套件，结果按传入顺序返回（保证报告表格顺序稳定） */
async function runAllConcurrent(suites, limit) {
  const results = new Array(suites.length);
  let cursor = 0;
  async function worker() {
    while (cursor < suites.length) {
      const i = cursor++;
      results[i] = await runOne(suites[i]);
    }
  }
  const n = Math.max(1, Math.min(limit, suites.length));
  await Promise.all(Array.from({ length: n }, worker));
  return results;
}

async function main() {
  console.log(c("bold", "══════════ pnpm 工作区全量检查 ══════════"));
  console.log(c("dim", `app-root: ${APP_ROOT}`));
  console.log(c("dim", `i18n-tool: ${I18N_TOOL}`));
  console.log("");

  // 第一步：清理上一轮的 check-logs（避免旧日志与新结果混在一起）
  cleanLogsDir();

  // 并发执行所有套件（并发上限 4，避免 5+ 个 vue-tsc 同时跑撑爆内存）；
  // 结果按 SUITES 定义顺序收集，报告表格顺序稳定。
  const results = await runAllConcurrent(SUITES, 4);

  // 汇总
  const pass = results.filter((r) => r.status === "PASS").length;
  const fail = results.filter((r) => r.status === "FAIL" || r.status === "TIMEOUT" || r.status === "ERROR").length;
  const skip = results.filter((r) => r.status === "SKIP").length;
  const total = results.length;

  console.log("");
  console.log(c("bold", "══════════ 汇总 ══════════"));
  console.log(`${c("green", "PASS")}    ${pass}`);
  console.log(`${c("red", "FAIL")}    ${fail}`);
  if (skip) console.log(`${c("yellow", "SKIP")}    ${skip}`);
  console.log(`total   ${total}`);

  // 写报告文件（markdown）
  const lines = [];
  lines.push(`# pnpm 工作区检查报告`);
  lines.push("");
  lines.push(`- 生成时间：${new Date().toISOString()}`);
  lines.push(`- app-root：\`${APP_ROOT}\``);
  lines.push("");
  lines.push(`| 检查 | 状态 | 耗时 |`);
  lines.push(`| --- | --- | --- |`);
  for (const r of results) {
    lines.push(`| ${r.name} | ${r.status} | ${fmtDuration(r.durationMs)} |`);
  }
  lines.push("");
  lines.push(`**结果**：${pass} 通过 / ${fail} 失败 / ${skip} 跳过 / 共 ${total}`);
  if (fail > 0) {
    lines.push("");
    lines.push(`## 失败详情`);
    lines.push("");
    lines.push(`各套件完整输出已写入 \`check-logs/\` 目录，下方以相对路径引用：`);
    for (const r of results.filter((x) => x.status !== "PASS" && x.status !== "SKIP")) {
      lines.push("");
      lines.push(`### ${r.name} — ${r.status}`);
      lines.push(`- 完整输出：[\`check-logs/${r.slug}.log\`](./check-logs/${r.slug}.log)`);
    }
  }
  writeFileSync(resolve(APP_ROOT, "check-report.md"), lines.join("\n"), "utf8");
  console.log(c("dim", `\n报告已写入 ${resolve(APP_ROOT, "check-report.md")}`));

  process.exit(fail > 0 ? 1 : 0);
}

main();
