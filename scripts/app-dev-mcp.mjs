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
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

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

function textResult(text, isError = false) {
  return { content: [{ type: "text", text: text || "(empty)" }], isError };
}

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
    const args = msg.params?.arguments || {};
    dispatch(name, args)
      .then((res) => send({ jsonrpc: "2.0", id, result: res }))
      .catch((e) => send({ jsonrpc: "2.0", id, result: textResult(String(e), true) }));
  } else if (id !== undefined) {
    send({ jsonrpc: "2.0", id, error: { code: -32601, message: `Method not found: ${method}` } });
  }
}
