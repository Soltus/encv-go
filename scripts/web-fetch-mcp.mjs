#!/usr/bin/env node
/**
 * web-fetch MCP server (stdio JSON-RPC) — an advanced replacement for the
 * built-in web_fetch tool. Features:
 *   - retry: exponential backoff + Retry-After respect (network errors, 429/5xx)
 *   - sniffing: trust the server-declared content-type but VERIFY via magic
 *     bytes / HTML structure (servers often lie about content-type)
 *   - SPA detection: identify React/Vue/Next/Nuxt shells that a static fetch
 *     cannot render; optional headless-browser render (puppeteer/playwright,
 *     only if installed)
 *   - proxy: HTTP/HTTPS proxy via CONNECT tunnel, zero external dependencies
 *
 * Tool: web_fetch(url, method?, headers?, body?, timeoutMs?, retries?,
 *                 proxy?, render?, followRedirects?)
 *
 * The fetch core is exported and the server only auto-starts when run as the
 * main module, so it is unit-testable:
 *   node -e "import('/workspace/scripts/web-fetch-mcp.mjs').then(m=>...)"
 */

import { spawn } from "node:child_process";
import * as zlib from "node:zlib";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { request as httpRequest } from "node:http";
import { request as httpsRequest } from "node:https";
import { connect as netConnect } from "node:net";
import { connect as tlsConnect } from "node:tls";

const __dirname = dirname(fileURLToPath(import.meta.url));
const MAX_OUT = 200_000;

// ---------- small helpers ----------
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const backoffMs = (attempt) => Math.min(1000 * 2 ** attempt, 15000);
const isRetryableStatus = (s) => s === 429 || (s >= 500 && s <= 504);
function parseRetryAfter(v) {
  if (!v) return null;
  const sec = parseInt(v, 10);
  if (!isNaN(sec)) return sec * 1000;
  const d = Date.parse(v);
  if (!isNaN(d)) return Math.max(0, d - Date.now());
  return null;
}

// ---------- content sniffing ----------
export function sniffContentType(buf, declared) {
  const head = buf.slice(0, 512);
  if (head[0] === 0x1f && head[1] === 0x8b) return "application/gzip";
  if (head[0] === 0x50 && head[1] === 0x4b && (head[2] === 0x03 || head[2] === 0x05))
    return "application/zip";
  if (head[0] === 0x25 && head[1] === 0x50 && head[2] === 0x44) return "application/pdf";
  if (head[0] === 0xff && head[1] === 0xd8) return "image/jpeg";
  if (head.slice(0, 8).toString("ascii") === "\x89PNG\r\n\x1a\n") return "image/png";
  const text = buf.slice(0, 1024).toString("utf8").replace(/^﻿/, "");
  if (/^\s*</.test(text)) {
    if (/<!doctype\s+html/i.test(text) || /<html[\s>]/i.test(text)) return "text/html";
    if (/<\?xml/.test(text)) return "application/xml";
  } else {
    const t = text.trim();
    if (t[0] === "{" || t[0] === "[") return "application/json";
  }
  return declared || "application/octet-stream";
}

export function detectSPA(html) {
  const markers = [];
  if (/<div[^>]+id=["'](root|app|__next|app-root)["']/.test(html)) markers.push("root-div");
  if (/react(\.production)?\.min\.js|<script[^>]+react/.test(html)) markers.push("react");
  if (/vue(\.runtime)?(\.prod)?\.js|<script[^>]+vue/.test(html)) markers.push("vue");
  if (/_next\/static|__NEXT_DATA__|next-router-state-tree/.test(html)) markers.push("next");
  if (/_nuxt\/|__NUXT__|window.__NUXT__/.test(html)) markers.push("nuxt");
  const body = html.match(/<body[^>]*>([\s\S]*)<\/body>/i);
  const bodyText = body
    ? body[1]
        .replace(/<script[\s\S]*?<\/script>/gi, "")
        .replace(/<style[\s\S]*?<\/style>/gi, "")
        .replace(/<[^>]+>/g, " ")
        .replace(/\s+/g, " ")
        .trim()
    : "";
  const hasModuleScripts = /\bscript[^>]+\btype\s*=\s*["']?module["']?/i.test(html);
  const isSPA =
    (markers.length > 0 || hasModuleScripts) && /<div[^>]+id=/i.test(html);
  const framework = markers.includes("next")
    ? "Next.js"
    : markers.includes("nuxt")
    ? "Nuxt"
    : markers.includes("react")
    ? "React"
    : markers.includes("vue")
    ? "Vue"
    : isSPA
    ? "unknown-SPA"
    : "";
  return { isSPA, framework, hasModuleScripts, bodyTextLen: bodyText.length, markers };
}

function htmlToText(html) {
  const title = (html.match(/<title[^>]*>([\s\S]*?)<\/title>/i) || [])[1] || "";
  let s = html
    .replace(/<script[\s\S]*?<\/script>/gi, " ")
    .replace(/<style[\s\S]*?<\/style>/gi, " ")
    .replace(/<[^>]+>/g, " ");
  s = s
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&apos;/g, "'")
    .replace(/\s+/g, " ")
    .trim();
  return (title ? `# ${title}\n\n` : "") + s;
}

// ---------- stream / proxy plumbing ----------
function readStream(stream) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    stream.on("data", (c) => chunks.push(c));
    stream.on("end", () => resolve(Buffer.concat(chunks)));
    stream.on("error", reject);
  });
}

// Returns a `createConnection(opts, cb)` for https.request that tunnels through
// an HTTP proxy via CONNECT, then upgrades to TLS. Zero deps.
function makeProxyConnection(proxyUrl) {
  const pu = new URL(proxyUrl);
  const proxyHost = pu.hostname;
  const proxyPort = parseInt(pu.port) || 80;
  return (connOpts, cb) => {
    const host = connOpts.host || connOpts.hostname;
    const port = connOpts.port || 443;
    const sock = netConnect(proxyPort, proxyHost, () => {
      sock.write(`CONNECT ${host}:${port} HTTP/1.1\r\nHost: ${host}:${port}\r\n\r\n`);
    });
    let buf = "";
    sock.on("data", (chunk) => {
      buf += chunk.toString("binary");
      const i = buf.indexOf("\r\n\r\n");
      if (i < 0) return;
      sock.removeAllListeners("data");
      const statusLine = buf.slice(0, i).split("\r\n")[0];
      const code = parseInt((statusLine.split(" ")[1] || "0"), 10);
      if (code >= 200 && code < 300) {
        const leftover = Buffer.from(buf.slice(i + 4), "binary");
        const tls = tlsConnect({ socket: sock, servername: host });
        tls.unshift(leftover);
        tls.on("error", (e) => cb(e));
        tls.on("connect", () => cb(null, tls));
      } else {
        cb(new Error("proxy CONNECT failed: " + statusLine));
      }
    });
    sock.on("error", (e) => cb(e));
  };
}

// ---------- request implementations ----------
async function nativeRequest(url, { method = "GET", headers = {}, body, timeoutMs }) {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(url, {
      method,
      headers,
      body,
      signal: ctrl.signal,
      redirect: "follow",
    });
    const buf = Buffer.from(await res.arrayBuffer());
    const h = {};
    res.headers.forEach((v, k) => (h[k] = v));
    return { status: res.status, headers: h, body: buf };
  } finally {
    clearTimeout(t);
  }
}

async function requestViaProxy(targetUrl, proxyUrl, { method = "GET", headers = {}, body, timeoutMs, redirectsLeft = 5 }) {
  const tu = new URL(targetUrl);
  const isHttps = tu.protocol === "https:";
  const targetPort = tu.port || (isHttps ? 443 : 80);
  const finalHeaders = { ...headers, host: tu.hostname + ":" + targetPort };
  if (body && !finalHeaders["content-length"] && !finalHeaders["Content-Length"]) {
    finalHeaders["content-length"] = Buffer.byteLength(body);
  }
  const pu = new URL(proxyUrl);
  if (pu.username) {
    const auth = Buffer.from(
      `${decodeURIComponent(pu.username)}:${decodeURIComponent(pu.password)}`
    ).toString("base64");
    finalHeaders["proxy-authorization"] = "Basic " + auth;
  }
  const doReq = isHttps ? httpsRequest : httpRequest;
  const options = isHttps
    ? {
        method,
        hostname: tu.hostname,
        port: targetPort,
        path: tu.pathname + tu.search,
        headers: finalHeaders,
        createConnection: makeProxyConnection(proxyUrl),
      }
    : {
        method,
        hostname: pu.hostname,
        port: parseInt(pu.port) || 80,
        path: targetUrl,
        headers: finalHeaders,
      };
  const res = await new Promise((resolve, reject) => {
    const r = doReq(options, resolve);
    r.on("error", reject);
    r.setTimeout(timeoutMs, () => r.destroy(new Error("timeout")));
    if (body) r.write(body);
    r.end();
  });
  let buf = await readStream(res);
  const enc = (res.headers["content-encoding"] || "").toLowerCase();
  if (enc.includes("gzip")) buf = zlib.gunzipSync(buf);
  else if (enc.includes("deflate")) buf = zlib.inflateSync(buf);
  const status = res.statusCode;
  const loc = res.headers["location"];
  if (status >= 300 && status < 400 && loc && redirectsLeft > 0) {
    const next = new URL(loc, targetUrl).href;
    return requestViaProxy(next, proxyUrl, {
      method,
      headers,
      body,
      timeoutMs,
      redirectsLeft: redirectsLeft - 1,
    });
  }
  return { status, headers: res.headers, body: buf };
}

export async function fetchWithRetry(url, { method = "GET", headers = {}, body, timeoutMs = 30000, retries = 3, proxy = null }) {
  let attempt = 0;
  while (true) {
    const isLast = attempt >= retries;
    let res = null;
    let err = null;
    try {
      res = proxy
        ? await requestViaProxy(url, proxy, { method, headers, body, timeoutMs })
        : await nativeRequest(url, { method, headers, body, timeoutMs });
    } catch (e) {
      err = e;
    }
    if (err) {
      if (isLast) throw err;
      await sleep(backoffMs(attempt));
      attempt++;
      continue;
    }
    if (!isLast && isRetryableStatus(res.status)) {
      const ra = parseRetryAfter(res.headers["retry-after"]);
      await sleep(ra ?? backoffMs(attempt));
      attempt++;
      continue;
    }
    res._attempts = attempt + 1;
    return res;
  }
}

export async function renderSPA(url, timeoutMs) {
  for (const mod of ["puppeteer", "playwright"]) {
    try {
      const m = await import(mod);
      const p = m.default || m;
      const browser = await p.launch({ headless: "new", args: ["--no-sandbox"] });
      const page = await browser.newPage();
      await page.goto(url, { waitUntil: "networkidle0", timeout: timeoutMs });
      const txt = await page.evaluate(() => document.body.innerText);
      await browser.close();
      return txt;
    } catch {
      /* try next */
    }
  }
  return null;
}

// ---------- MCP server ----------
const TOOLS = [
  {
    name: "web_fetch",
    description:
      "Advanced web fetch (replacement for built-in web_fetch). Retries with exponential backoff + Retry-After, sniffs the real content-type via magic bytes (ignores lying servers), detects SPA shells (React/Vue/Next/Nuxt) and can render them via a headless browser if installed, and supports HTTP/HTTPS proxy via CONNECT tunnel (zero deps). Returns status, declared vs sniffed content-type, SPA info, and the (truncated) content.",
    inputSchema: {
      type: "object",
      properties: {
        url: { type: "string", description: "absolute http(s) URL" },
        method: { type: "string", default: "GET", description: "GET/POST/..." },
        headers: { type: "object", description: "extra request headers" },
        body: { type: "string", description: "request body (for POST/PUT)" },
        timeoutMs: { type: "number", default: 30000, description: "per-attempt timeout (hard cap 120000)" },
        retries: { type: "number", default: 3, description: "retry count on network error / 429 / 5xx" },
        proxy: { type: "string", description: "optional HTTP/HTTPS proxy URL, e.g. http://127.0.0.1:7890" },
        render: { type: "boolean", default: false, description: "if true and target is a SPA, render via headless browser (puppeteer/playwright) when available" },
        followRedirects: { type: "boolean", default: true, description: "follow 3xx redirects" },
      },
      required: ["url"],
    },
  },
];

function textResult(text, isError = false) {
  return { content: [{ type: "text", text: text || "(empty)" }], isError };
}

// --- argument normalization: alias resolution + unknown-arg feedback --------
// 传错参数名（如把 url 写成 uri/link）会被静默当成 undefined，工具退化为错误行为
// 且无任何报错。这里统一：① 参数别名 ② 未知参数名直接报错。
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

// 参数别名表（覆盖本服务全部工具）。
const ARG_ALIASES = {
  web_fetch: {
    uri: "url", link: "url",
    verb: "method",
    header: "headers",
    data: "body",
    timeout: "timeoutMs",
    retry: "retries",
    proxyUrl: "proxy",
    headless: "render",
    follow: "followRedirects",
  },
};
const PASS_THROUGH = new Set();

async function dispatch(name, args = {}) {
  if (name === "web_fetch") {
    const url = String(args.url || "").trim();
    if (!url) return textResult("url is required", true);
    let parsed;
    try {
      parsed = new URL(url);
    } catch {
      return textResult("invalid url", true);
    }
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:")
      return textResult("only http(s) urls supported", true);

    const proxy =
      args.proxy ||
      process.env.WEB_FETCH_PROXY ||
      process.env.HTTPS_PROXY ||
      process.env.https_proxy ||
      null;
    const timeoutMs = Math.min(Number(args.timeoutMs) || 30000, 120000);
    const retries = Number.isFinite(args.retries) ? args.retries : 3;
    const method = String(args.method || "GET").toUpperCase();
    const headers = args.headers && typeof args.headers === "object" ? args.headers : {};
    const body = typeof args.body === "string" ? args.body : undefined;

    let res;
    try {
      res = await fetchWithRetry(url, { method, headers, body, timeoutMs, retries, proxy });
    } catch (e) {
      return textResult(`fetch failed after ${retries + 1} attempt(s): ${e.message}`, true);
    }

    const declared = (res.headers["content-type"] || "").split(";")[0].trim();
    const sniffed = sniffContentType(res.body, declared);
    let text = res.body.toString("utf8");
    let spaInfo = "";
    let renderedNote = "";
    if (sniffed === "text/html") {
      const spa = detectSPA(text);
      spaInfo = `\nSPA: ${spa.isSPA ? "YES" : "no"}${
        spa.framework ? " (" + spa.framework + ")" : ""
      } (moduleScripts=${spa.hasModuleScripts}, bodyTextLen=${spa.bodyTextLen})`;
      if (args.render && spa.isSPA) {
        const rendered = await renderSPA(url, timeoutMs);
        if (rendered) {
          text = rendered;
          renderedNote = "\n[rendered via headless browser]";
        } else {
          renderedNote = "\n[render requested but no headless browser (puppeteer/playwright) installed]";
        }
      }
      if (!renderedNote.includes("headless")) text = htmlToText(text);
    }
    const truncated =
      text.length > MAX_OUT
        ? text.slice(0, MAX_OUT) + `\n…(truncated ${text.length - MAX_OUT} chars)…`
        : text;
    const report =
      `url: ${url}\n` +
      `finalStatus: ${res.status}\n` +
      `declaredType: ${declared || "(none)"}\n` +
      `sniffedType: ${sniffed}\n` +
      `attempts: ${res._attempts || 1}${spaInfo}${renderedNote}\n\n` +
      `=== content ===\n${truncated}`;
    return textResult(report, res.status >= 400);
  }
  return textResult(`Unknown tool: ${name}`, true);
}

function startServer() {
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
          serverInfo: { name: "web-fetch", version: "0.1.0" },
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
}

// Only auto-start the server when run as the main module, so the fetch core can
// be unit-tested via dynamic import without blocking on stdin.
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
