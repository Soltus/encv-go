#!/usr/bin/env node
/**
 * app-dev MCP server (stdio JSON-RPC) — project-level dev gates for encv-mobile.
 *
 * Wraps the repo's long-running / flaky-terminal dev commands as MCP *tools* so
 * the agent can run them without `execute_command` (which is flaky in this
 * sandbox). Exposes:
 *
 *   app_check_all   node app/scripts/check-all.mjs  (full gate; --no-tests skips unit tests)
 *   app_typecheck   pnpm exec vue-tsc --noEmit --incremental  (per project)
 *   app_i18n        python3 scripts/i18n-tool.py <sub> --app encv-mobile
 *   app_format       pnpm exec biome format --write <files>  (cosmetic; avoids flaky terminal)
 *
 * Paths are derived from this file's location so the server is portable inside
 * the repo:
 *   __dirname  = /workspace/scripts
 *   REPO_ROOT  = /workspace
 *   APP_ROOT   = /workspace/app
 *
 * Long commands (check_all) get a generous internal timeout; the caller (agent)
 * blocks until completion and receives captured stdout/stderr + exit code, plus
 * (for check_all) the generated app/check-report.md summary.
 */

import { spawn } from "node:child_process";
import { readFileSync, watch } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { dirname, resolve, basename } from "node:path";
import * as skillManager from "./skill-manager.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(__dirname, ".."); // /workspace
const APP_ROOT = resolve(REPO_ROOT, "app"); // /workspace/app

const MAX_OUT = 60_000; // cap returned text; note truncation to the caller

/**
 * Run a command, returning { stdout, stderr, code, timedOut }.
 * @param {string} cmd
 * @param {string[]} args
 * @param {string} cwd
 * @param {number} timeoutMs
 */
function run(cmd, args, cwd, timeoutMs) {
  return new Promise((resolve) => {
    const child = spawn(cmd, args, { cwd, env: process.env });
    let stdout = "";
    let stderr = "";
    let done = false;
    const finish = (code, signal, timedOut = false) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      resolve({ stdout, stderr, code: code ?? (signal ? 1 : 0), timedOut });
    };
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      finish(null, "timeout", true);
    }, timeoutMs);
    child.stdout.on("data", (d) => {
      stdout += d;
      if (stdout.length > MAX_OUT * 4) stdout = stdout.slice(-MAX_OUT * 4);
    });
    child.stderr.on("data", (d) => {
      stderr += d;
      if (stderr.length > MAX_OUT * 4) stderr = stderr.slice(-MAX_OUT * 4);
    });
    child.on("error", (e) => {
      stderr += `\n[spawn error] ${e.message}`;
      finish(1, null);
    });
    child.on("close", (code, signal) => finish(code, signal));
  });
}

function trim(s) {
  if (s.length <= MAX_OUT) return s;
  return `…(truncated ${s.length - MAX_OUT} chars)…\n` + s.slice(-MAX_OUT);
}

// --- safety gate for app_exec (HMR) -------------------------------------
// The destructive-command rules live in app-dev-guard.mjs. This server watches
// that file and re-imports it on change, so editing the rules takes effect
// immediately — no MCP server restart, no new conversation (HMR-style reload).
const GUARD_MODULE = resolve(__dirname, "app-dev-guard.mjs");
let guardRef = null; // { APP_EXEC_DENY, guardAppExec }; null => fail-closed

async function loadGuard() {
  try {
    const mod = await import(pathToFileURL(GUARD_MODULE).href + `?t=${Date.now()}`);
    guardRef = mod;
    process.stderr.write(`[app-dev] guard reloaded: ${mod.APP_EXEC_DENY.length} rules\n`);
  } catch (e) {
    process.stderr.write(`[app-dev] guard load FAILED (kept previous): ${e.message}\n`);
  }
}

await loadGuard();

// --- skill manager (CRUD + multi-path + monitoring) ------------------------
await skillManager.loadRegistry();
skillManager.startWatching();
// Watch the DIRECTORY (not the file inode): editors / write_to_file do atomic
// save (write temp + rename), which unlinks the original inode and silently
// stops an inotify watch on the FILE. Watching the dir + filtering by basename
// survives atomic saves. (This was the HMR bug: guard edits didn't reload.)
const GUARD_DIR = dirname(GUARD_MODULE);
const GUARD_BASENAME = basename(GUARD_MODULE);
let guardWatchTimer = null;
const onGuardChange = () => {
  clearTimeout(guardWatchTimer);
  guardWatchTimer = setTimeout(loadGuard, 80); // debounce double-fire
};
try {
  watch(GUARD_DIR, (event, filename) => {
    if (filename === GUARD_BASENAME) onGuardChange();
  });
  process.stderr.write(`[app-dev] HMR watching ${GUARD_DIR}/${GUARD_BASENAME}\n`);
} catch (e) {
  process.stderr.write(`[app-dev] HMR watch FAILED: ${e.message}\n`);
}

function textResult(text, isError = false) {
  return { content: [{ type: "text", text: text || "(empty)" }], isError };
}

// --- argument normalization: alias resolution + unknown-arg feedback --------
// 传错参数名（如把 command 写成 cmd 之外未被识别的拼写）会被静默当成 undefined，
// 工具退化为错误行为且无任何报错。这里统一：① 参数别名 ② 未知参数名直接报错。
function lev(a, b) {
  const m = a.length, n = b.length;
  const dp = Array.from({ length: m + 1 }, (_, i) => [i, ...Array(n).fill(0)]);
  for (let j = 0; j <= n; j++) dp[0][j] = j;
  for (let i = 1; i <= m; i++)
    for (let j = 1; j <= n; j++)
      dp[i][j] = a[i - 1] === b[j - 1]
        ? dp[i - 1][j - 1]
        : 1 + Math.min(dp[i - 1][j - 1], dp[i - 1][j], dp[i][j - 1]);
  return dp[m][n];
}
function suggest(candidates, word) {
  let best = null, bestD = Infinity;
  for (const c of candidates) {
    const d = lev(word, c);
    if (d < bestD) { bestD = d; best = c; }
  }
  return bestD <= Math.max(2, Math.floor(word.length / 2)) ? best : null;
}
function checkArgs(name, args, toolDefs, aliasMap, passThrough) {
  const tool = toolDefs.find((t) => t.name === name);
  const aliases = aliasMap[name] || {};
  const props = (tool && tool.inputSchema && tool.inputSchema.properties) || {};
  const canonical = Object.keys(props);
  const accepted = new Set([...canonical, ...Object.keys(aliases), ...Object.values(aliases)]);
  for (const [alias, canon] of Object.entries(aliases)) {
    if (args[canon] === undefined && args[alias] !== undefined) args[canon] = args[alias];
  }
  const unknown = Object.keys(args).filter((k) => !accepted.has(k));
  if (unknown.length && !(passThrough && passThrough.has(name))) {
    const valid = [...new Set([...canonical, ...Object.keys(aliases)])].sort();
    const tip = unknown
      .map((k) => {
        const g = suggest(canonical, k);
        return g ? `'${k}' (did you mean '${g}'?)` : `'${k}'`;
      })
      .join(", ");
    return {
      error:
        `Unknown parameter(s): ${tip}.\n` +
        `Valid parameters for '${name}': ${valid.join(", ")}.\n` +
        `(Unknown parameters are rejected so typos don't silently no-op.)`,
    };
  }
  return { args, error: null };
}

// 参数别名表（覆盖本服务全部工具）。skill add/update 把整个 args 透传给
// skill-manager（其参数集会演进），故设为 passThrough 不校验未知参数。
const ARG_ALIASES = {
  app_check_all:      { skipTests: "noTests", "no-tests": "noTests" },
  app_typecheck:      { proj: "project", dir: "project", target: "project" },
  app_i18n:           { cmd: "command", sub: "command", k: "key", t: "threshold", target: "app" },
  app_format:         { file: "files", path: "files", paths: "files" },
  app_exec:           { cmd: "command", dir: "cwd", workingDir: "cwd", timeout: "timeoutMs" },
  app_skill_list:     { f: "filter", q: "filter" },
  app_skill_get:      { n: "name", skill: "name" },
  app_skill_add:      { m: "method", source: "src", repo: "url", n: "name", target: "targetPath" },
  app_skill_update:   { m: "method", source: "src", repo: "url", n: "name", target: "targetPath" },
  app_skill_remove:   { n: "name", skill: "name" },
  app_skill_path_add:    { p: "path", dir: "path" },
  app_skill_path_remove: { p: "path", dir: "path" },
};
const PASS_THROUGH = new Set(["app_skill_add", "app_skill_update"]);

const TOOLS = [
  {
    name: "app_check_all",
    description:
      "Run the full pnpm-workspace gate: biome CI, encv-mobile + shared + plugin typechecks, i18n lint --all, unit tests, vite build. Writes app/check-report.md. Use noTests:true to skip unit tests (faster).",
    inputSchema: {
      type: "object",
      properties: {
        noTests: {
          type: "boolean",
          default: false,
          description: "pass --no-tests to skip the vitest suite",
        },
      },
    },
  },
  {
    name: "app_typecheck",
    description:
      "vue-tsc --noEmit --incremental for one project. Default encv-mobile. Other valid projects: shared-components, plugin-openlist/web, plugin-mpv-player/web, plugin-simverse/web.",
    inputSchema: {
      type: "object",
      properties: {
        project: {
          type: "string",
          default: "encv-mobile",
          description: "project dir under /workspace/app (or packages/shared-components)",
        },
      },
    },
  },
  {
    name: "app_i18n",
    description:
      "Run scripts/i18n-tool.py for encv-mobile. command: scan | lint | var-check | dup | stats | gen-types | db-init | find-key | en-check.",
    inputSchema: {
      type: "object",
      properties: {
        command: {
          type: "string",
          enum: ["scan", "lint", "var-check", "dup", "stats", "gen-types", "db-init", "find-key", "en-check"],
        },
        key: { type: "string", description: "search key, only for find-key" },
        threshold: { type: "number", description: "dup similarity threshold (default 0.85)" },
        app: { type: "string", default: "encv-mobile", description: "target app name" },
      },
      required: ["command"],
    },
  },
  {
    name: "app_format",
    description:
      "Run `biome format --write` on given files (paths relative to /workspace/app). Use to fix biome CI 'format File content differs' errors without the flaky execute_command terminal. Defaults to the K28 files if no paths given.",
    inputSchema: {
      type: "object",
      properties: {
        files: {
          type: "array",
          items: { type: "string" },
          description: "file paths relative to /workspace/app, e.g. ['packages/shared-components/src/lib/taskCollection.ts']",
        },
      },
    },
  },
  {
    name: "app_exec",
    description:
      "Run an arbitrary shell command via `bash -c` inside the repo (cwd must stay under /workspace, defaults to /workspace). Stable alternative to the flaky built-in terminal: runs in this persistent MCP process, captures stdout/stderr, enforces a timeout. Use for commands that hang/crash the normal terminal (e.g. gradle, long builds, docker). WARNING: executes arbitrary commands — use with care. A safety gate blocks destructive ops: batch/recursive deletes (rm -rf/rmdir/shred), dangerous git (reset --hard, clean -f, checkout -- ., push --force, branch -D), privilege escalation (sudo/su), curl|sh / wget|sh. Process kills (kill/pkill/killall/fuser) are ALLOWED when targeting same-identity or non-critical processes (e.g. clearing a stale local dev server / occupied port); they are only blocked when they would hit an environment-connection process (ssh tunnel, code-server, the dev MCP, session daemon, self/ancestors, init) or an unverified other-identity live process.",
    inputSchema: {
      type: "object",
      properties: {
        command: {
          type: "string",
          description: "full shell command, e.g. 'bash scripts/build-android.sh' or 'ls -la app'",
        },
        cwd: {
          type: "string",
          description: "absolute working dir; must be under /workspace. Default /workspace.",
        },
        timeoutMs: {
          type: "number",
          default: 120000,
          description: "kill the command after this many ms (hard cap 600000).",
        },
      },
      required: ["command"],
    },
  },
  {
    name: "app_guard_reload",
    description:
      "Manually re-load the app_exec safety-gate rules from scripts/app-dev-guard.mjs (HMR). Use to confirm the running MCP server picked up edits, or to force a reload after changing the guard file. Returns the active rule count.",
    inputSchema: { type: "object", properties: {} },
  },
  {
    name: "app_skill_list",
    description:
      "List all registered project skills (name, path, source, hash). Optional `filter` matches name/displayName/description. Skills live under multiple base paths and are auto-discovered + watched.",
    inputSchema: {
      type: "object",
      properties: { filter: { type: "string", description: "case-insensitive substring filter" } },
    },
  },
  {
    name: "app_skill_get",
    description: "Get one skill's full metadata by name (path, source, sourceType, hash, description).",
    inputSchema: {
      type: "object",
      properties: { name: { type: "string" } },
      required: ["name"],
    },
  },
  {
    name: "app_skill_add",
    description:
      "Add a skill. method=import copies a local dir/file into a skill path; method=npx runs `npm install <package>` then copies the first SKILL.md found; method=git does `git clone --depth 1 <url>` (optional subdir/ref) then copies skill(s). Optional name (else derived), targetPath (else first writable base path).",
    inputSchema: {
      type: "object",
      properties: {
        method: { type: "string", enum: ["import", "npx", "git"] },
        src: { type: "string", description: "import: local path (file or dir) of the skill" },
        package: { type: "string", description: "npx: npm package name to install" },
        bin: { type: "string", description: "npx: optional installer command args to run after install" },
        url: { type: "string", description: "git: repo URL to clone" },
        subdir: { type: "string", description: "git: subpath within repo holding the skill(s)" },
        ref: { type: "string", description: "git: branch/tag to clone (default default branch)" },
        name: { type: "string", description: "override skill name (else derived from source/dir)" },
        targetPath: { type: "string", description: "base skill path key (repo-relative), else first writable" },
      },
      required: ["method"],
    },
  },
  {
    name: "app_skill_update",
    description:
      "Update a skill. With `method` re-fetches via import/npx/git into the same path. Without `method` just refreshes the stored hash from disk (after manual edits).",
    inputSchema: {
      type: "object",
      properties: {
        name: { type: "string" },
        method: { type: "string", enum: ["import", "npx", "git"] },
        src: { type: "string" },
        package: { type: "string" },
        url: { type: "string" },
        subdir: { type: "string" },
        ref: { type: "string" },
        targetPath: { type: "string" },
      },
      required: ["name"],
    },
  },
  {
    name: "app_skill_remove",
    description: "Remove a skill: delete its directory and drop it from the registry.",
    inputSchema: {
      type: "object",
      properties: { name: { type: "string" } },
      required: ["name"],
    },
  },
  {
    name: "app_skill_paths",
    description: "List the configured skill base paths (editable list; skills are discovered under each).",
    inputSchema: { type: "object", properties: {} },
  },
  {
    name: "app_skill_path_add",
    description: "Add a skill base path (repo-relative, e.g. .codebuddy/skills). Enables discovery + watching there.",
    inputSchema: {
      type: "object",
      properties: { path: { type: "string" } },
      required: ["path"],
    },
  },
  {
    name: "app_skill_path_remove",
    description: "Remove a skill base path from the tracked list (does not delete skills already there).",
    inputSchema: {
      type: "object",
      properties: { path: { type: "string" } },
      required: ["path"],
    },
  },
  {
    name: "app_skill_rescan",
    description: "Force a re-scan of all skill paths: re-discover skills, recompute hashes, resync the registry. Returns the discovered skill count.",
    inputSchema: { type: "object", properties: {} },
  },
];

async function dispatch(name, args = {}) {
  if (name === "app_check_all") {
    const a = ["scripts/check-all.mjs"];
    if (args.noTests) a.push("--no-tests");
    const r = await run("node", a, APP_ROOT, 900_000);
    let report = "";
    try {
      const p = resolve(APP_ROOT, "check-report.md");
      report = `\n\n=== app/check-report.md ===\n${readFileSync(p, "utf8").slice(-MAX_OUT)}`;
    } catch {
      /* report not written */
    }
    const head = `exit=${r.code}${r.timedOut ? " (TIMED OUT)" : ""}\n--- stdout ---\n${trim(r.stdout)}\n--- stderr ---\n${trim(r.stderr)}`;
    return textResult(head + report, r.code !== 0);
  }

  if (name === "app_typecheck") {
    const proj = args.project || "encv-mobile";
    const dir = proj === "shared-components" ? resolve(APP_ROOT, "packages/shared-components") : resolve(APP_ROOT, proj);
    const r = await run("pnpm", ["exec", "vue-tsc", "--noEmit", "--incremental"], dir, 300_000);
    return textResult(`exit=${r.code}${r.timedOut ? " (TIMED OUT)" : ""}\n${trim(r.stdout)}\n${trim(r.stderr)}`, r.code !== 0);
  }

  if (name === "app_i18n") {
    const sub = args.command;
    const a = ["scripts/i18n-tool.py", sub, "--app", args.app || "encv-mobile"];
    if (sub === "find-key" && args.key) a.push(args.key);
    if (sub === "dup" && typeof args.threshold === "number") a.push("--threshold", String(args.threshold));
    const r = await run("python3", a, REPO_ROOT, 300_000);
    return textResult(`exit=${r.code}${r.timedOut ? " (TIMED OUT)" : ""}\n${trim(r.stdout)}\n${trim(r.stderr)}`, r.code !== 0);
  }

  if (name === "app_format") {
    const files = (args.files && args.files.length)
      ? args.files
      : [
          "packages/shared-components/src/lib/taskCollection.ts",
          "packages/shared-components/src/lib/taskCollection.test.ts",
          "packages/shared-components/src/stores/taskStore.ts",
          "packages/shared-components/src/stores/runTasksStore.ts",
        ];
    const r = await run("pnpm", ["exec", "biome", "format", "--write", ...files], APP_ROOT, 120_000);
    return textResult(`exit=${r.code}${r.timedOut ? " (TIMED OUT)" : ""}\n--- stdout ---\n${trim(r.stdout)}\n--- stderr ---\n${trim(r.stderr)}`, r.code !== 0);
  }

  if (name === "app_exec") {
    const raw = String(args.command || "").trim();
    if (!raw) return textResult("command is required", true);
    if (!guardRef) return textResult("⛔ 安全门禁模块未就绪，拒绝执行以防误放行。", true);
    const blocked = await guardRef.guardAppExec(raw);
    if (blocked) return textResult(`⛔ 命令被安全门禁拦截（${blocked}），如需执行请改用更安全的等价命令。`, true);
    const cwd =
      args.cwd && String(args.cwd).startsWith("/workspace")
        ? String(args.cwd)
        : REPO_ROOT;
    const timeoutMs = Math.min(Number(args.timeoutMs) || 120000, 600_000);
    const r = await run("bash", ["-c", raw], cwd, timeoutMs);
    return textResult(
      `exit=${r.code}${r.timedOut ? " (TIMED OUT)" : ""}\n--- stdout ---\n${trim(r.stdout)}\n--- stderr ---\n${trim(r.stderr)}`,
      r.code !== 0
    );
  }

  if (name === "app_guard_reload") {
    await loadGuard();
    const n = guardRef ? guardRef.APP_EXEC_DENY.length : 0;
    return textResult(`guard reloaded: ${n} rules active`);
  }

  // --- skill management -----------------------------------------------------
  if (name === "app_skill_list") {
    const list = skillManager.listSkills(args.filter);
    return textResult(JSON.stringify(list, null, 2));
  }
  if (name === "app_skill_get") {
    const s = skillManager.getSkill(args.name);
    return s ? textResult(JSON.stringify(s, null, 2)) : textResult(`skill not found: ${args.name}`, true);
  }
  if (name === "app_skill_add") {
    const r = await skillManager.addSkill(args);
    if (!r.ok) return textResult(`add failed: ${r.error}`, true);
    await skillManager.scanAndSync();
    return textResult(`skill added: ${JSON.stringify(r)}`);
  }
  if (name === "app_skill_update") {
    const r = await skillManager.updateSkill(args.name, args);
    if (!r.ok) return textResult(`update failed: ${r.error}`, true);
    return textResult(`skill updated: ${JSON.stringify(r)}`);
  }
  if (name === "app_skill_remove") {
    const r = await skillManager.removeSkill(args.name);
    if (!r.ok) return textResult(`remove failed: ${r.error}`, true);
    return textResult(`skill removed: ${JSON.stringify(r)}`);
  }
  if (name === "app_skill_paths") {
    return textResult(JSON.stringify(skillManager.listSkillPaths(), null, 2));
  }
  if (name === "app_skill_path_add") {
    const r = skillManager.addSkillPath(args.path);
    if (!r.ok) return textResult(`add path failed: ${r.error}`, true);
    return textResult(`skill path added: ${JSON.stringify(r)}`);
  }
  if (name === "app_skill_path_remove") {
    const r = skillManager.removeSkillPath(args.path);
    if (!r.ok) return textResult(`remove path failed: ${r.error}`, true);
    return textResult(`skill path removed: ${JSON.stringify(r)}`);
  }
  if (name === "app_skill_rescan") {
    const synced = await skillManager.scanAndSync();
    return textResult(`rescanned: ${Object.keys(synced).length} skill(s)`);
  }

  return textResult(`Unknown tool: ${name}`, true);
}

// --- minimal MCP stdio JSON-RPC server -----------------------------------
let buf = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => {
  buf += chunk;
  let nl;
  while ((nl = buf.indexOf("\n")) >= 0) {
    const line = buf.slice(0, nl).trim();
    buf = buf.slice(nl + 1);
    if (line) handleLine(line);
  }
});
process.stdin.on("end", () => {
  if (buf.trim()) handleLine(buf.trim());
});

function send(obj) {
  process.stdout.write(JSON.stringify(obj) + "\n");
}

function handleLine(line) {
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return;
  }
  const { id, method } = msg;
  if (method === "initialize") {
    send({
      jsonrpc: "2.0",
      id,
      result: {
        protocolVersion: msg.params?.protocolVersion || "2024-11-05",
        capabilities: { tools: {} },
        serverInfo: { name: "app-dev", version: "0.1.0" },
      },
    });
  } else if (method === "notifications/initialized") {
    // no response
  } else if (method === "ping") {
    send({ jsonrpc: "2.0", id, result: {} });
  } else if (method === "tools/list") {
    send({ jsonrpc: "2.0", id, result: { tools: TOOLS } });
  } else if (method === "tools/call") {
    const name = msg.params?.name;
    const rawArgs = msg.params?.arguments || {};
    const norm = checkArgs(name, rawArgs, TOOLS, ARG_ALIASES, PASS_THROUGH);
    if (norm.error) {
      send({ jsonrpc: "2.0", id, result: textResult(norm.error, true) });
      return;
    }
    dispatch(name, norm.args)
      .then((res) => send({ jsonrpc: "2.0", id, result: res }))
      .catch((e) => send({ jsonrpc: "2.0", id, result: textResult(String(e), true) }));
  } else if (id !== undefined) {
    send({ jsonrpc: "2.0", id, error: { code: -32601, message: `Method not found: ${method}` } });
  }
}
