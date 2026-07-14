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

/** Run the shim with the given args, returning { stdout, stderr, code }. */
function runShim(args, timeoutMs = 120_000) {
  return new Promise((resolve) => {
    // Ensure the codemogger index dir exists. codemogger's SQLite opens the db
    // with a shared WAL coordination path and fails with
    //   I/O error (statfs shared WAL coordination path): entity not found
    // if the parent <root>/.codemogger dir is missing. Creating it up-front
    // keeps the MCP self-sufficient (no manual `mkdir` / .keep workaround).
    try {
      mkdirSync(dirname(DB), { recursive: true });
    } catch (e) {
      /* non-fatal: let the shim surface a clearer error if it truly can't write */
    }
    const fullArgs = [...args, "--db", DB];
    const child = spawn("bash", [SHIM, ...fullArgs], {
      env: { ...process.env, CODEMOGGER_NO_AUTOINDEX: AUTOINDEX },
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
    description: "Full-text search over code bodies (name/signature/body weighted FTS).",
    inputSchema: {
      type: "object",
      properties: { query: { type: "string", description: "keyword(s) to search" } },
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
    description: "Re-index the codebase (or a given dir) so subsequent reads see fresh data.",
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
      const r = await runShim(["search", args.query]);
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
    case "codemogger_index": {
      const r = await runShim(["index", args.dir || ROOT]);
      return textResult(r.stdout || r.stderr, r.code !== 0);
    }
    case "codemogger_list": {
      const r = await runShim(["list"]);
      return textResult(r.stdout || r.stderr, r.code !== 0);
    }
    default:
      return textResult(`Unknown tool: ${name}`, true);
  }
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
    const args = msg.params?.arguments || {};
    dispatch(name, args)
      .then((res) => send({ jsonrpc: "2.0", id, result: res }))
      .catch((e) => send({ jsonrpc: "2.0", id, result: textResult(String(e), true) }));
  } else if (id !== undefined) {
    send({ jsonrpc: "2.0", id, error: { code: -32601, message: `Method not found: ${method}` } });
  }
}
