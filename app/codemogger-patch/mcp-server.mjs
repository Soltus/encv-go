#!/usr/bin/env node
/**
 * codemogger MCP server (stdio JSON-RPC).
 *
 * Wraps ./codemogger-shim so CodeBuddy can query the codemogger code-graph
 * (references / context / search / impact / leaks) as MCP *tools* — no terminal
 * round-trip, no `execute_command` flakiness.
 *
 * Config (env, all optional):
 *   CODEMOGGER_ROOT   codebase root to index (default /workspace/app/encv-mobile)
 *   CODEMOGGER_SHIM   path to the shim (default: ./codemogger-shim next to this file)
 *   CODEMOGGER_AUTOINDEX   "1" to let the shim auto-reindex before every read
 *                          query (default off — avoids MCP-request timeouts;
 *                          call the `codemogger_index` tool explicitly instead)
 *
 * The shim derives the codebase dir from `--db <root>/.codemogger/index.db`,
 * so every invocation passes that flag to keep reads pointed at the right index.
 */

import { spawn } from "node:child_process";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));

const ROOT = process.env.CODEMOGGER_ROOT || "/workspace/app/encv-mobile";
const SHIM = process.env.CODEMOGGER_SHIM || join(__dirname, "codemogger-shim");
const DB = join(ROOT, ".codemogger", "index.db");
const AUTOINDEX = process.env.CODEMOGGER_AUTOINDEX === "1" ? "0" : "1"; // shim reads NO_AUTOINDEX

/** Run the shim with the given args, returning { stdout, stderr, code }.
 *  opts.db=false skips pinning --db (used by multi-root index/search/grep, which
 *  read roots from .codemogger.json instead). */
function runShim(args, opts = {}, timeoutMs = 120_000) {
  return new Promise((resolve) => {
    // Ensure the codemogger index dir exists. codemogger's SQLite opens the db
    // with a shared WAL coordination path and fails with
    //   I/O error (statfs shared WAL coordination path): entity not found
    // if the parent <root>/.codemogger dir is missing. Creating it up-front
    // keeps the MCP self-sufficient (no manual `mkdir` / .keep workaround).
    if (opts.db !== false) {
      try {
        mkdirSync(dirname(DB), { recursive: true });
      } catch (e) {
        /* non-fatal: let the shim surface a clearer error if it truly can't write */
      }
    }
    const fullArgs = opts.db === false ? [...args] : [...args, "--db", DB];
    const child = spawn("bash", [SHIM, ...fullArgs], {
      // extraEnv 用于把单次调用的作用域变量（如按目录抓 grep 的
      // CODEMOGGER_GREP_ROOT）注入 shim，避免为了「限定目录」而回退到裸终端。
      env: { ...process.env, CODEMOGGER_NO_AUTOINDEX: AUTOINDEX, ...(opts.extraEnv || {}) },
    });
    let stdout = "";
    let stderr = "";
    let done = false;
    const finish = (code, signal) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      resolve({ stdout, stderr, code: code ?? (signal ? 1 : 0), signal });
    };
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      finish(null, "timeout");
    }, timeoutMs);
    child.stdout.on("data", (d) => (stdout += d));
    child.stderr.on("data", (d) => (stderr += d));
    child.on("error", (e) => {
      stderr += `\n[spawn error] ${e.message}`;
      finish(1, null);
    });
    child.on("close", (code, signal) => finish(code, signal));
  });
}

function textResult(text, isError = false) {
  return { content: [{ type: "text", text: text || "(empty)" }], isError };
}

// --- structured (JSON) parsing for grep/search -----------------------------
// codemogger's shim emits text (grep uses `grep -rniF -n` -> `path:line:content`;
// search adds `---- [rootname · w=w] ----` labels per root). The shim has no
// native --format, so we parse the text here into a scriptable array so
// migration lists can be filtered (e.g. drop built `android/.../assets/*.css`).
function parseHits(text) {
  const hits = [];
  let root = undefined;
  for (const raw of String(text).split("\n")) {
    const line = raw.replace(/\r$/, "");
    if (!line.trim()) continue;
    // multi-root / concept label header:  ---- [rootname · w=1.5] ----  or  ---- term: foo ----
    const label = line.match(/^----\s*\[(.+?)\s*·\s*w=([\d.]+)\]\s*----$/);
    if (label) { root = label[1].trim(); continue; }
    if (/^----\s*term:\s*.+?\s*----$/.test(line)) { continue; }
    // section header (== ... ==) — never a hit line
    if (/^==.*==$/.test(line)) { continue; }
    // hit line: <path>:<line>:<content>  (paths have no ':' on linux; content may)
    const m = line.match(/^(.+?):(\d+):(.*)$/);
    if (m) {
      const hit = { file: m[1], line: Number(m[2]), match: m[3] };
      if (root) hit.root = root;
      hits.push(hit);
    }
  }
  return hits;
}

// --- argument normalization: alias resolution + unknown-arg feedback --------
// 痛点：此前传错参数名（如把 query 写成 pattern）会被静默当成 undefined，工具
// 退化为错误行为且无任何报错。这里统一：① 参数别名（pattern/q/text → query 等）
// ② 未知参数名直接报错，避免「传错名却跑通、结果不对」还查半天。
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
// toolDefs: TOOLS 数组；aliasMap: { 工具名: { 别名: 规范名 } }；passThrough: Set<工具名>
// 就地把别名归一到规范名；未知参数返回 error（passThrough 工具不校验未知参数）。
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

// 参数别名表（覆盖本服务全部工具）。新增工具时在此补一行即可。
const ARG_ALIASES = {
  codemogger_references: { symbol: "target", file: "target", module: "target", path: "target", name: "target" },
  codemogger_context:    { file: "target", path: "target", symbol: "target", name: "target" },
  codemogger_search:     { q: "query", pattern: "query", text: "query", term: "query", path: "dir", root: "dir" },
  codemogger_grep:       { q: "query", pattern: "query", text: "query", term: "query", path: "dir", root: "dir" },
  codemogger_impact:     { symbol: "target", module: "target", file: "target", path: "target" },
  codemogger_leaks:      { directory: "dir", path: "dir", root: "dir" },
  codemogger_index:      { directory: "dir", path: "dir", root: "dir" },
  codemogger_css_source: { file: "file", path: "file", css: "file", f: "file", line: "line", l: "line" },
};
const PASS_THROUGH = new Set();

const TOOLS = [
  {
    name: "codemogger_references",
    description:
      "Who imports a symbol/module/file. mode: symbol (default, alias-agnostic blast radius), module (exact alias specifier), file (what the file itself imports).",
    inputSchema: {
      type: "object",
      properties: {
        target: { type: "string", description: "symbol name, module alias, or file path" },
        mode: { type: "string", enum: ["symbol", "module", "file"], default: "symbol" },
        format: { type: "string", enum: ["text", "json"], default: "text" },
      },
      required: ["target"],
    },
  },
  {
    name: "codemogger_context",
    description:
      "Chunk outline of a symbol's whole file (hit chunk tagged <<<) or a file path (exact/suffix). expand: also print each chunk's snippet body.",
    inputSchema: {
      type: "object",
      properties: {
        target: { type: "string", description: "symbol name or file path" },
        expand: { type: "boolean", default: false },
      },
      required: ["target"],
    },
  },
  {
    name: "codemogger_search",
    description:
      "Multi-root hybrid full-text search driven by workspace .codemogger.json: codemogger FTS over each declared root (results labeled + ordered by root weight) PLUS a per-root grep complement over the configurable general-knowledge file set (docs + config + source codemogger's FTS can't parse). Backend/config roots get FIRST-CLASS labeled, weight-ordered grep blocks (not a flat wasted grep). Concept-aware (expands known concepts to precise terms). Use for architectural questions across the whole workspace (e.g. 'where is the SPA served', 'servingDir', 'themesDir', 'docker', 'config', across encv-mobile / internal / openlist / combolite). Optional `dir` scopes the grep complement to a subtree (sets CODEMOGGER_GREP_ROOT) — covers the 'grep a specific folder' case so you never need the terminal.",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "keyword(s) to search" },
        dir: { type: "string", description: "optional absolute dir to scope the search's grep complement to (sets CODEMOGGER_GREP_ROOT)" },
        format: { type: "string", enum: ["text", "json"], default: "text", description: "output format. 'json' returns a parseable array of {file,line,match,root?} so migration lists can be scripted/filtered (e.g. drop built assets). search tags each hit with its root." },
      },
      required: ["query"],
    },
  },
  {
    name: "codemogger_grep",
    description:
      "Direct grep over the configurable general-knowledge file set in .codemogger.json `grep.include` — by default Go/.md/.json/.yaml/.kt/.ts/.vue/Dockerfile/toml (docs + config + source codemogger's FTS can't index). Surfaces backend, config and doc facts that the structured index misses. Optional `dir` scopes the tree to a single folder (sets CODEMOGGER_GREP_ROOT) — this is the MCP-native replacement for terminal `grep -r <path>` so you never fall back to execute_command. Supports `|` as OR for multiple literals (e.g. `foo|bar`).",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "literal string to grep for" },
        dir: { type: "string", description: "optional absolute dir to scope the grep to (sets CODEMOGGER_GREP_ROOT)" },
        format: { type: "string", enum: ["text", "json"], default: "text", description: "output format. 'json' returns a parseable array of {file,line,match} so migration lists can be scripted/filtered (e.g. drop built assets). grep has no per-root labels, so no `root` field." },
      },
      required: ["query"],
    },
  },
  {
    name: "codemogger_impact",
    description:
      "Alias-agnostic blast radius of a module/symbol — queries BOTH '@/x' and '@encv/shared-components/x' and merges importer lists. Run BEFORE moving/decoupling a module.",
    inputSchema: {
      type: "object",
      properties: { target: { type: "string", description: "module alias or bare symbol" } },
      required: ["target"],
    },
  },
  {
    name: "codemogger_leaks",
    description:
      "List every shared->app reverse '@/' import under a dir (default: CODEMOGGER_ROOT). Verifies 'shared layer must NOT depend on app layer' after a decoupling pass.",
    inputSchema: {
      type: "object",
      properties: { dir: { type: "string", description: "directory to scan (default root)" } },
    },
  },
  {
    name: "codemogger_index",
    description: "Multi-root (re)index driven by workspace .codemogger.json: indexes every declared root into its own <root>/.codemogger/index.db. Run after code changes so search/references see fresh data across all roots.",
    inputSchema: {
      type: "object",
      properties: { dir: { type: "string", description: "directory to index (default CODEMOGGER_ROOT)" } },
    },
  },
  {
    name: "codemogger_list",
    description: "List indexed codebases known to codemogger.",
    inputSchema: { type: "object", properties: {} },
  },
  {
    name: "codemogger_css_source",
    description:
      "Trace a compiled CSS *product* back to its SCSS source via the *.css.map source map (Sass/Vite-emitted). Answers 'which .scss partial + line produced CSS line N?', even for rules generated by @mixin/@function/@each (which never appear literally in SCSS). This is the safety net that lets SCSS advanced capabilities be used freely. Modes: with `line` → provenance of one CSS line (file:line:col + snippet); without `line` → per-source summary (how many generated lines came from each .scss source, with sample mappings).",
    inputSchema: {
      type: "object",
      properties: {
        file: { type: "string", description: "path to the compiled .css product (must have a sibling .css.map)" },
        line: { type: "number", description: "optional 1-based CSS line to trace (omitted → summary across all lines)" },
        format: { type: "string", enum: ["text", "json"], default: "text", description: "output format" },
        top: { type: "number", description: "for summary mode, how many top sources to show (default 8)" },
      },
      required: ["file"],
    },
  },
];

async function dispatch(name, args = {}) {
  switch (name) {
    case "codemogger_references": {
      const a = ["references", args.target];
      if (args.mode === "module") a.push("--module");
      if (args.mode === "file") a.push("--file");
      if (args.format === "json") a.push("--format", "json");
      const r = await runShim(a);
      return textResult(r.stdout || r.stderr, r.code !== 0);
    }
    case "codemogger_context": {
      const a = ["context", args.target];
      if (args.expand) a.push("--expand");
      const r = await runShim(a);
      return textResult(r.stdout || r.stderr, r.code !== 0);
    }
  case "codemogger_search": {
    const opts = { db: false };
    // dir 作用域：限定 grep 补集到子树，覆盖「按目录搜」的终端回退场景。
    if (args.dir) opts.extraEnv = { CODEMOGGER_GREP_ROOT: String(args.dir) };
    const r = await runShim(["search", args.query], opts);
    if (args.format === "json" && r.code === 0) {
      return textResult(JSON.stringify(parseHits(r.stdout), null, 2), false);
    }
    return textResult(r.stdout || r.stderr, r.code !== 0);
  }
  case "codemogger_grep": {
    const opts = { db: false };
    // dir 作用域：等价于终端 `grep -r <path>`，覆盖「按目录抓 grep」的终端回退场景。
    if (args.dir) opts.extraEnv = { CODEMOGGER_GREP_ROOT: String(args.dir) };
    const r = await runShim(["grep", args.query], opts);
    if (args.format === "json" && r.code === 0) {
      return textResult(JSON.stringify(parseHits(r.stdout), null, 2), false);
    }
    return textResult(r.stdout || r.stderr, r.code !== 0);
  }
  case "codemogger_index": {
    const r = await runShim(args.dir ? ["index", args.dir] : ["index"], { db: false });
    return textResult(r.stdout || r.stderr, r.code !== 0);
  }
  case "codemogger_impact": {
    const r = await runShim(["impact", args.target]);
    return textResult(r.stdout || r.stderr, r.code !== 0);
  }
  case "codemogger_leaks": {
    const r = await runShim(["leaks", args.dir || ROOT]);
    return textResult(r.stdout || r.stderr, r.code !== 0);
  }
  case "codemogger_list": {
    // `list` 不接受 --db（它列出已索引的 codebase），故显式不 pin db，
    // 否则 --db 会被透传给底层的 `codemogger list`，行为与预期不符。
    const r = await runShim(["list"], { db: false });
    return textResult(r.stdout || r.stderr, r.code !== 0);
  }
  case "codemogger_css_source": {
    return dispatch_css_source(args);
  }
    default:
      return textResult(`Unknown tool: ${name}`, true);
  }
}

async function dispatch_css_source(args) {
  const file = args.file;
  if (!file) return textResult("codemogger_css_source: 'file' is required", true);
  const cli = ["css-source", file];
  if (typeof args.line === "number") cli.push("--line", String(args.line));
  if (args.format === "json") cli.push("--json");
  if (typeof args.top === "number") cli.push("--top", String(args.top));
  const r = await runShim(cli, { db: false });
  return textResult(r.stdout || r.stderr, r.code !== 0);
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
        serverInfo: { name: "codemogger", version: "0.1.0" },
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
