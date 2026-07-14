#!/usr/bin/env node
/**
 * check-all.mjs — pnpm 工作区全量检查 + 报告
 *
 * 覆盖（补全 plugin-* 项目）：
 *   1. Biome CI（根 workspace，lint 全部包，含 plugin-*）
 *   2. encv-mobile 类型检查（vue-tsc --noEmit）
 *   3. shared-components 独立类型检查（vue-tsc --noEmit）
 *   4. plugin-openlist/web 类型检查（vue-tsc --noEmit）
 *   5. plugin-mpv-player/web 类型检查（vue-tsc --noEmit）
 *   6. plugin-simverse/web 类型检查（vue-tsc --noEmit）
 *   7. i18n 全局 lint --all（仓库根 scripts/i18n-tool.py）
 *   8. encv-mobile 单元测试（vitest run，可用 --no-tests 跳过）
 *   9. encv-mobile 构建校验（vite build，纯 web 打包；typecheck 已由套件 2 覆盖）
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
    // 用 JSON reporter：biome 文本报告器会把"计数里有、但渲染缺失"的 error
    // 吞掉（本次就遇到 Found 1 error 却无任何 ✖ 诊断）。JSON 结构化输出
    // 必然包含 severity:error 的诊断，从根上解决 check:all 抓不到真正错误的问题。
    args: ["exec", "biome", "ci", "--reporter=json", "."],
    json: true,
    timeoutMs: 180_000,
  },
  {
    name: "encv-mobile typecheck",
    cwd: resolve(APP_ROOT, "encv-mobile"),
    cmd: "pnpm",
    // 直接调 vue-tsc --noEmit：跳过 pnpm typecheck 的 pretypecheck(i18n) 钩子
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
    args: ["exec", "vue-tsc", "--noEmit", "--incremental"],
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
  {
    name: "encv-mobile build (vite)",
    cwd: resolve(APP_ROOT, "encv-mobile"),
    cmd: "pnpm",
    // 纯 web 打包校验：typecheck 已由独立套件覆盖，这里只跑 vite build，
    // 用于捕获"类型通过但构建期才报错"的回归（动态 import / 资源缺失 / 插件配置等）。
    args: ["exec", "vite", "build"],
    timeoutMs: 600_000,
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

/**
 * 从捕获输出里抽取"强错误特征行"，作为无结构化输出时的回退。
 * 覆盖 tsc(error TS)、vitest(FAIL)、模块找不到、异常等常见失败信号。
 */
function extractErrorLines(text, max = 80) {
  const lines = String(text || "").split("\n");
  const STRONG = /(✖|error\s+TS\d+|cannot\s+find|module\s+not\s+found|failed\s+to\s+(load|resolve)|SyntaxError|TypeError|ReferenceError|deserialize|DEPRECATED|deprecated|✗|\bFAIL\b|not valid|did you mean|Usage:|Error:)/i;
  const out = [];
  for (const ln of lines) {
    const t = ln.trim();
    if (t && STRONG.test(t)) out.push(t);
    if (out.length >= max) break;
  }
  return out;
}

/**
 * 收集某个套件的"真正错误"，用于在终端与报告中直接呈现，
 * 而不必翻开 check-logs 里成百上千行的原始输出。
 *
 * 优先走结构化输出（biome 的 JSON reporter）：biome 文本报告器会把
 * "计数里有、但渲染缺失"的 error 吞掉，JSON 则必然包含 severity:error
 * 的诊断。无结构化输出时回退到文本特征行抓取。
 */
function collectErrors(suite, r) {
  if (suite.json) {
    // biome 的 --json 会把完整 JSON 输出为单行，但前后可能混入非 JSON 文本
    // （如 "The --json option is unstable..." 警告、"✖ Some errors..." 总结行），
    // 直接 JSON.parse(整个 stdout) 会失败。这里先整体尝试，再逐行找 JSON 行。
    const data = parseBiomeJson(r.stdout);
    if (data) {
      const diags = Array.isArray(data.diagnostics) ? data.diagnostics : [];
      const errs = diags
        .filter((d) => d && d.severity === "error")
        .map((d) => {
          const loc = d.location || {};
          // biome JSON: location.path 是字符串，行列在 location.start.{line,column}
          const start = loc.start || (loc.span && loc.span.start) || {};
          const hasLine = start.line != null && start.line > 0;
          const where = loc.path
            ? `${loc.path}${hasLine ? `:${start.line}:${start.column != null ? start.column : 0}` : ""}`
            : "(配置/全局)";
          return `${where}  ${d.category || ""}  ${d.message || ""}`.replace(/\s+/g, " ").trim();
        });
      if (errs.length) return errs;
    }
  }
  return extractErrorLines(`${r.stdout || ""}\n${r.stderr || ""}`);
}

/**
 * 从可能混入非 JSON 文本的 biome --json 输出里稳健地取出 JSON 对象。
 * 先整体 parse；失败则逐行找以 { 开头、} 结尾且能 parse 的行（biome 输出为单行）。
 */
function parseBiomeJson(stdout) {
  // 关键：即便设了 NO_COLOR/FORCE_COLOR=0，biome 仍会在 JSON 前输出 ANSI 复位码
  // (\x1b[0m)，导致 JSON.parse 与逐行 startsWith("{") 全部失败。必须先剥离 ANSI。
  const s = stripAnsi(String(stdout || ""));
  try {
    return JSON.parse(s);
  } catch {
    // 继续逐行找
  }
  for (const line of s.split("\n")) {
    const t = line.trim();
    if (t.startsWith("{") && t.endsWith("}")) {
      try {
        return JSON.parse(t);
      } catch {
        // 试下一行
      }
    }
  }
  return null;
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
  const errors = r.status !== "PASS" ? collectErrors(suite, r) : [];
  if (r.status !== "PASS") {
    if (suite.json) {
      // JSON 套件：终端只呈现提炼出的错误，不刷原始 JSON（避免上千行噪音）
      console.log(c("dim", "────────── 错误（结构化提取） ──────────"));
      if (errors.length) errors.forEach((e) => console.log(c("red", e)));
      else {
        // 结构化提取落空（如命令自身用法错误）：回退显示原始输出前若干行，
        // 绝不把"命令挂了"这类问题掩盖成"(无错误)"。
        console.log(c("yellow", "⚠ 未能从结构化输出提取错误，原始输出前 40 行："));
        const raw = `${r.stdout || ""}\n${r.stderr || ""}`.trim().split("\n").slice(0, 40);
        raw.forEach((l) => console.log(c("dim", l)));
      }
      console.log(c("dim", "──────────────────────────────────────────"));
    } else {
      // 完整输出原始结果（不截断），全量日志另存于 check-logs/<slug>.log
      const out = (r.stderr || r.stdout || "").trim();
      console.log(c("dim", "────────── 输出（完整，未截断） ──────────"));
      console.log(out);
      console.log(c("dim", "──────────────────────────────────────────"));
    }
  }
  const slug = slugify(suite.name);
  const logFile = resolve(LOGS_DIR, `${slug}.log`);
  writeFileSync(logFile, stripAnsi(`${r.stdout || ""}\n${r.stderr || ""}`), "utf8");
  return { ...suite, ...r, slug, errors };
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
  console.log(c("bold", "═══════════ pnpm 工作区全量检查 ═══════════"));
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
  console.log(c("bold", "═══════════ 汇总 ═══════════"));
  console.log(`${c("green", "PASS")}    ${pass}`);
  console.log(`${c("red", "FAIL")}    ${fail}`);
  if (skip) console.log(`${c("yellow", "SKIP")}    ${skip}`);
  console.log(`total   ${total}`);

  // 写报告文件（markdown）
  const lines = [];
  lines.push(`# pnpm 工作区检查报告`);
  lines.push("");
  // 用本地时区格式化（toISOString() 是 UTC，会与实际跑检查的时间差 8h，造成"时间不准"的错觉）
  const _now = new Date();
  const _pad = (n) => String(n).padStart(2, "0");
  const _tz = -_now.getTimezoneOffset(); // 分钟，东正西负
  const _tzSign = _tz >= 0 ? "+" : "-";
  const _tzH = _pad(Math.floor(Math.abs(_tz) / 60));
  const _tzM = _pad(Math.abs(_tz) % 60);
  const localTs = `${_now.getFullYear()}-${_pad(_now.getMonth() + 1)}-${_pad(_now.getDate())} ` +
    `${_pad(_now.getHours())}:${_pad(_now.getMinutes())}:${_pad(_now.getSeconds())} ` +
    `(UTC${_tzSign}${_tzH}:${_tzM})`;
  lines.push(`- 生成时间：${localTs}`);
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
      const errs = r.errors || [];
      if (errs.length) {
        lines.push("");
        lines.push("<details><summary>错误摘要</summary>");
        lines.push("");
        lines.push("```");
        errs.forEach((e) => lines.push(e));
        lines.push("```");
        lines.push("</details>");
      } else {
        lines.push("");
        lines.push("> ⚠ 未能从输出中提取结构化错误，请直接查看上方日志文件。若日志是 biome 用法错误，说明 `--reporter` 在该版本不被 `ci` 支持，需改用 `biome check --reporter=json`。");
      }
    }
  }
  writeFileSync(resolve(APP_ROOT, "check-report.md"), lines.join("\n"), "utf8");
  console.log(c("dim", `\n报告已写入 ${resolve(APP_ROOT, "check-report.md")}`));

  process.exit(fail > 0 ? 1 : 0);
}

main();
