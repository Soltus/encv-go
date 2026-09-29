/**
 * ENCV 容器预览页的静态服务（bun + TypeScript）。
 *
 * 用 bun 跑：bun app/encv-preview/serve.ts 5179
 *
 * 刻意**只做静态托管**：这一页的全部加解密都在浏览器里的 wasm 内核完成，
 * 不请求任何后端接口（没有 /stream、没有 /api、没有健康检查）。
 * 任何形式的后端代理都会掩盖「是否真的纯前端」这件事。
 *
 * 比"裸静态托管"多做的三件事：
 *
 * 1. **目录 → index.html**：`/` 与 `/samples/` 都要能落到 index.html，
 *    否则浏览器打开根路径就是 404，页面一个脚本都跑不起来。
 * 2. **HTTP Range**：流式打开（api.openStream）要按区间取字节，
 *    服务端不认 Range 就只能整个下载，那"大容器不必整体进内存"这条
 *    就在网络这一环上漏掉了。
 * 3. **整文件响应支持 gzip**：wasm 内核实测 8.15MB，gzip 后 2.18MB。
 *
 * ⚠️ Range 与压缩**互斥**：Range 的 start/end 是**压缩前**的字节偏移，
 * 把 gzip 套到 206 上，客户端拿到的是压缩流的区间，解出来的字节全错。
 * 所以只对 200 压缩，206 一律原样发送。
 */

import { existsSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

const ROOT = import.meta.dir;
const port = Number(process.argv[2] ?? 5179);

const MIME: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".wasm": "application/wasm",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".ico": "image/x-icon",
  ".txt": "text/plain; charset=utf-8",
  ".md": "text/markdown; charset=utf-8",
  ".pdf": "application/pdf",
  ".mp4": "video/mp4",
  ".mp3": "audio/mpeg",
};

// 体积实测（2026-09-30）：container 内核 8.15MB → gzip 2.18MB；crypto 内核 3.25MB → 0.91MB。
const COMPRESSIBLE = new Set([".wasm", ".js", ".mjs", ".css", ".html", ".json", ".svg", ".txt", ".md"]);
const COMPRESS_MIN = 1024;

/** 压缩结果缓存：key = mtime + size，避免每个请求都把 8MB 重压一遍。 */
const gzipCache = new Map<string, { key: string; payload: Uint8Array | null }>();

function extname(p: string): string {
  const i = p.lastIndexOf(".");
  return i < 0 ? "" : p.slice(i).toLowerCase();
}

function gzipFor(abs: string): Uint8Array | null {
  if (!COMPRESSIBLE.has(extname(abs))) return null;

  let st: ReturnType<typeof statSync>;
  try {
    st = statSync(abs);
  } catch {
    return null;
  }
  if (st.size < COMPRESS_MIN) return null;

  const key = `${st.mtimeMs}:${st.size}`;
  const hit = gzipCache.get(abs);
  if (hit && hit.key === key) return hit.payload;

  const raw = new Uint8Array(readFileSync(abs));
  const gz = Bun.gzipSync(raw, { level: 9 });
  // 压完反而更大就别压（例如本身就是压缩格式的 png / zip 类容器）
  const payload = gz.length < raw.length ? gz : null;

  gzipCache.set(abs, { key, payload });
  return payload;
}

/** URL 路径 → 磁盘绝对路径；越出 ROOT 一律拒绝（防 ../ 穿越）。 */
function resolveInsideRoot(urlPath: string): string | null {
  let decoded: string;
  try {
    decoded = decodeURIComponent(urlPath.split("?")[0].split("#")[0]);
  } catch {
    return null;
  }
  if (decoded.includes("\0")) return null;

  const rel = decoded.replace(/^\/+/, "");
  const abs = path.resolve(ROOT, rel || ".");
  if (abs !== ROOT && !abs.startsWith(ROOT + path.sep)) return null;
  return abs;
}

/** 解析 "bytes=start-end"；非法返回 null（调用方按"无 Range"处理），越界返回 "unsatisfiable"。 */
function parseRange(header: string, size: number): { start: number; end: number } | null | "unsatisfiable" {
  const spec = header
    .replace(/^bytes=/, "")
    .split(",")[0]
    .trim();
  const [a, b] = spec.split("-");
  if (a === "" && b === "") return null;

  let start: number;
  let end: number;
  if (a === "") {
    // bytes=-N：最后 N 字节
    const n = Number(b);
    if (!Number.isFinite(n) || n <= 0) return null;
    start = Math.max(0, size - n);
    end = size - 1;
  } else {
    start = Number(a);
    end = b === "" ? size - 1 : Math.min(Number(b), size - 1);
    if (!Number.isFinite(start) || !Number.isFinite(end)) return null;
  }
  if (start > end || start >= size) return "unsatisfiable";
  return { start, end };
}

const server = Bun.serve({
  port,
  async fetch(req) {
    if (req.method !== "GET" && req.method !== "HEAD") {
      return new Response("method not allowed", { status: 405 });
    }

    let abs = resolveInsideRoot(new URL(req.url).pathname);
    if (!abs) return new Response("bad path", { status: 400 });

    // 目录 → index.html（`/` 与 `/samples/` 都走这条）
    if (existsSync(abs) && statSync(abs).isDirectory()) {
      abs = path.join(abs, "index.html");
    }

    const file = Bun.file(abs);
    if (!(await file.exists())) return new Response("not found", { status: 404 });

    const size = file.size;
    const headers = new Headers({
      "Content-Type": MIME[extname(abs)] ?? "application/octet-stream",
      "Accept-Ranges": "bytes",
      "Cache-Control": "no-cache",
    });

    // HEAD 也要带 Content-Length：页面自检用 HEAD 判断样例/资源在不在
    if (req.method === "HEAD") {
      headers.set("Content-Length", String(size));
      return new Response(null, { status: 200, headers });
    }

    const rangeHeader = req.headers.get("Range");
    if (rangeHeader) {
      const r = parseRange(rangeHeader, size);
      if (r === "unsatisfiable") {
        headers.set("Content-Range", `bytes */${size}`);
        headers.set("Content-Length", "0");
        return new Response(null, { status: 416, headers });
      }
      if (r) {
        // 206 不压缩（见文件头注释）
        const buf = new Uint8Array(await file.slice(r.start, r.end + 1).arrayBuffer());
        headers.set("Content-Length", String(buf.length));
        headers.set("Content-Range", `bytes ${r.start}-${r.end}/${size}`);
        return new Response(buf, { status: 206, headers });
      }
      // Range 头格式不认识 → 按 RFC 当作没有 Range，返回整个文件
    }

    const wantsGzip = (req.headers.get("Accept-Encoding") ?? "").toLowerCase().includes("gzip");
    const gz = wantsGzip ? gzipFor(abs) : null;
    if (gz) {
      headers.set("Content-Encoding", "gzip");
      headers.set("Vary", "Accept-Encoding");
      headers.set("Content-Length", String(gz.length));
      return new Response(gz, { status: 200, headers });
    }

    headers.set("Content-Length", String(size));
    return new Response(await file.arrayBuffer(), { status: 200, headers });
  },
});

console.log(
  `encv-preview serving ${ROOT} on http://127.0.0.1:${server.port} （纯静态 · 目录 → index.html · Range · 整文件 gzip；不代理任何后端）`
);
