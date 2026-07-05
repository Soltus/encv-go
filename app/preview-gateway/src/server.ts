/**
 * preview-gateway
 * ===============
 *
 * Single-port reverse proxy for sandbox preview.
 *
 *   外网/浏览器 → :16000 (OpenPreview) → :16666 (gateway) → 4 upstream
 *   本地 dev    → :16666 (gateway) → 4 upstream
 *
 * D1 (用户决策："好记"): 监听 :16666
 *   - 避开 :16000 (agent-tool-host 占用)
 *   - 避开 :5173 (vite 老端口，由 preview-proxy 旧 default 占用)
 *   - 避开 :8100 (vite 新端口，本 spec 改的)
 *
 * 路由策略（2026-06-09 黑名单机制大改）
 * ─────────────────────────────────────────
 * 历史：白名单（UPSTREAMS 数组逐条添加），添加多 + 漏网之鱼（如 /stream 漏配）。
 * 现在：黑名单。默认 upstream = encv-go (:2025) 接收**所有**后端请求，新加端点自动
 * 命中；只有命中 VITE_DENY 的路径才走 Vite。
 *
 * 路由表：
 *   /                  → encv-mobile (Vite, :8100)  ← SPA 根
 *   /player            → encv-mobile (Vite, :8100)  ← ArtPlayerView SPA
 *   /tabs/*            → encv-mobile (Vite, :8100)  ← Tabs SPA
 *   /@vite/...         → encv-mobile (Vite, :8100)  ← Vite dev artifacts
 *   /@fs/, /@id/, ...  → encv-mobile (Vite, :8100)  ← Vite dev artifacts
 *   /src/, /node_modules/, /assets/, /public/, /favicon.ico, /sw.js, /manifest → Vite
 *   /openlist-ui/*     → plugin-openlist-web (:5174)   ← 特殊 upstream（绝对路径子资源走 cookie/referer 兜底）
 *   /openlist/*        → OpenList 真实 fork (:5244)    ← 特殊 upstream（pathRewrite 剥前缀）
 *   /__gateway/*       → gateway inline（健康检查 + banner）
 *   **其他一切**        → encv-go (Go, :2025)            ← 默认：后端 API + stream + ws + ...
 *
 * WebSocket:
 *   Upgrade on /              → ws://:8100 (main app HMR)
 *   Upgrade on /openlist-ui/  → ws://:5174 (plugin HMR)
 *   Upgrade on 其他          → ws://:2025 (encv-go WS endpoints)
 */

import http from 'node:http'
import https from 'node:https'
import { URL } from 'node:url'
import httpProxy from 'http-proxy'
import { WebSocketServer, type WebSocket } from 'ws'
import type { IncomingMessage, ClientRequest, ServerResponse } from 'node:http'
import type { Duplex } from 'node:stream'
import { ChildrenManager, type ChildSpec, type ChildStatus } from './children.js'
import { resolvePaths } from './paths.js'
import { ensureMockData } from './preflight.js'
import {
  SPECIAL_UPSTREAMS,
  VITE_DENY,
  ENCV_GO_UPSTREAM,
  VITE_UPSTREAM,
  pickUpstream,
  type Upstream,
  type ViteDenyRule,
} from './routing.js'

// =============================================================================
// 黑名单路由表（denylist mechanism，2026-06-09 改造）
// =============================================================================
//
// 解决「白名单时代添加路由多 + 漏网之鱼」问题：
//   - 旧：UPSTREAMS 数组逐条添加，新加后端端点必须改 gateway 代码。
//   - 新：默认 upstream = encv-go (后端)，未来 encv-go 加任何新端点都自动命中。
//          只有命中 VITE_DENY 的路径才走 Vite。
//
// 三类 upstream（实际定义见 src/routing.ts，便于单测）：
//   1) SPECIAL_UPSTREAMS — 需要 pathRewrite 的特殊路由（plugin SPA / OpenList direct）
//   2) VITE_DENY         — 黑名单：命中走 Vite（SPA HTML + Vite dev artifacts + 静态资源）
//   3) ENCV_GO_UPSTREAM  — 兜底：encv-go（处理所有后端端点）
//
// 添加新后端端点：什么都不用做。encv-go 启起来就能访问。
// 添加新前端路由：在 routing.ts 加一行 VITE_DENY 即可。

const PORT = Number(process.env.PORT ?? 16666)
const HOST = process.env.HOST ?? '0.0.0.0'
const LOG_PREFIX = '[gateway]'

const HEALTH_TIMEOUT_MS = 3000

// =============================================================================
// Logging
// =============================================================================

function log(...args: unknown[]): void {
  console.log(LOG_PREFIX, ...args)
}

function logUpstream(req: IncomingMessage, up: Upstream, status: 'OK' | 'FAIL', err?: unknown): void {
  const ip = req.socket.remoteAddress ?? '?'
  const ua = (req.headers['user-agent'] ?? '').slice(0, 50)
  if (err !== undefined) {
    log(`${ip} ${status} ${up.name} ${req.method} ${req.url} (${(err as Error).message ?? err}) ua="${ua}"`)
  } else {
    log(`${ip} ${status} ${up.name} ${req.method} ${req.url} ua="${ua}"`)
  }
}

// =============================================================================
// Route matching（黑名单机制，核心逻辑在 src/routing.ts）
// =============================================================================
//
// pickUpstream / matchesPrefix / matchesViteDeny 已抽出到 src/routing.ts 便于单测。
// 此处仅 re-export 供 server.ts 内部使用。

// =============================================================================
// HTTP proxy (per-upstream instance so error handlers are isolated)
// =============================================================================

function createProxyFor(up: Upstream): httpProxy {
  const proxy = httpProxy.createProxyServer({
    target: up.target,
    ws: false,           // ws handled separately via 'upgrade' event
    changeOrigin: false,  // CRITICAL: do NOT rewrite Origin/Host — see spec §3.3
    xfwd: true,           // add X-Forwarded-* headers (helps Vite detect proxy)
    preserveHeaderKeyCase: true,
    proxyTimeout: 120_000,   // agent chat 多轮 LLM 调用可能需要 60s+
    timeout: 120_000,         // 同上（非流式端点如 /api/models 仍秒回，不影响）
  })

  // ⚠️ 沙箱 dev critical: override http-proxy's xfwd behavior for X-Forwarded-Proto.
  // http-proxy's built-in xfwd: true uses `req.socket.encrypted` to decide
  // X-Forwarded-Proto. If Trae proxy terminates TLS and forwards HTTP to us,
  // req.socket.encrypted=false → xfwd writes "http" → vite's @vite/client
  // injects `ws://...` → browser on https:// page gets SecurityError
  // "An insecure WebSocket connection may not be initiated from a page loaded over HTTPS".
  //
  // We override by reading `req.protocol` (Node IncomingMessage property) which
  // already honors the X-Forwarded-Proto header set by Trae proxy. If Trae didn't
  // set it, we fall back to 'https' for Trae-sandbox domains (match *.trae.cn or
  // has the well-known preview-gw prefix), otherwise 'http'.
  proxy.on('proxyReq', (proxyReq, req) => {
    // ⚠️ 沙箱 dev critical: 重写 Origin 头。
    // Trae 代理 → 沙箱 :16666 → :2025 backend，浏览器 Origin 是 trae.cn 域名，
    // :2025 的 CORS 白名单不含 trae.cn → 403 Forbidden → 前端 fetch /api/* 失败。
    // 解法：把 Origin 改成 :16666 (白名单内) 让 :2025 放行。
    // 副作用风险：0 — backend 只看 Origin 判断跨域，不依赖 Origin 做其他逻辑
    // (token 鉴权走 Authorization header / cookie，不走 Origin)。
    proxyReq.setHeader('Origin', 'http://localhost:16666')

    const host = String(req.headers.host || '')
    const xfpRaw = req.headers['x-forwarded-proto']
    let xfpFirstStr: string = ''
    if (Array.isArray(xfpRaw) && xfpRaw.length > 0 && xfpRaw[0] !== undefined) {
      xfpFirstStr = xfpRaw[0] as string
    } else if (typeof xfpRaw === 'string') {
      xfpFirstStr = xfpRaw
    }
    const xfpFromIncoming = xfpFirstStr.toLowerCase().split(',')[0]?.trim() ?? ''
    let xfp = xfpFromIncoming
    if (!xfp) {
      // Heuristic: Trae sandbox external domains are HTTPS. The Trae proxy
      // terminates TLS at its edge; the connection from Trae to us is plain
      // HTTP, so req.protocol would say 'http' — but the user-facing URL is
      // HTTPS. Trust the host pattern.
      if (/trae\.cn$/i.test(host) || /agent-sandbox/i.test(host) || /^run-agent-/i.test(host)) {
        xfp = 'https'
      } else {
        const sock: any = req.socket
        if (sock?.encrypted) {
          xfp = 'https'
        } else {
          xfp = 'http'
        }
      }
    }
    if (xfp) {
      proxyReq.setHeader('X-Forwarded-Proto', xfp)
    }
  })

  // ⚠️ 沙箱 dev critical: when user visits /openlist-ui/ (plugin SPA entry),
  // inject Set-Cookie: __plugin_spa=1 so subsequent subresource requests
  // (Vite's absolute-root imports: /src/App.vue, /@fs/..., /node_modules/...)
  // can be routed to :5174 even when Referer is empty (Trae IDE default
  // referrer-policy strips it). This is the linchpin that makes
  // /openlist-ui/ not stay blank.
  if (up.match === '/openlist-ui') {
    proxy.on('proxyRes', (proxyRes, _req) => {
      // Only inject for HTML responses (the SPA entry document)
      const ct = proxyRes.headers['content-type']
      if (ct && /text\/html/i.test(String(ct))) {
        const existing = proxyRes.headers['set-cookie']
        const cookieLine = '__plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600'
        if (existing) {
          proxyRes.headers['set-cookie'] = Array.isArray(existing)
            ? [...existing, cookieLine]
            : [String(existing), cookieLine]
        } else {
          proxyRes.headers['set-cookie'] = [cookieLine]
        }
      }
    })
  }

  proxy.on('error', (err, req, resOrSocket) => {
    // Error can happen for both HTTP and WS requests
    if ('writeHead' in resOrSocket && typeof (resOrSocket as ServerResponse).writeHead === 'function') {
      const res = resOrSocket as ServerResponse
      // Only write 502 if headers not yet sent
      if (!res.headersSent) {
        res.writeHead(502, { 'Content-Type': 'application/json; charset=utf-8' })
        const body = {
          error: 'upstream_unavailable',
          upstream: up.name,
          target: up.target,
          path: req.url,
          hint: up.hint,
          detail: (err as Error).message ?? String(err),
        }
        res.end(JSON.stringify(body, null, 2))
      }
      logUpstream(req, up, 'FAIL', err)
    } else {
      // WebSocket or raw socket — destroy
      const sock = resOrSocket as Duplex
      sock.destroy()
      logUpstream(req, up, 'FAIL', err)
    }
  })

  return proxy
}

const proxies = new Map<string, httpProxy>()
// 黑名单机制：proxy pool 由 ENCV_GO_UPSTREAM（默认）+ VITE_UPSTREAM（黑名单命中）+ SPECIAL_UPSTREAMS（特殊）组成
proxies.set(ENCV_GO_UPSTREAM.name, createProxyFor(ENCV_GO_UPSTREAM))
proxies.set(VITE_UPSTREAM.name, createProxyFor(VITE_UPSTREAM))
for (const up of SPECIAL_UPSTREAMS) proxies.set(up.name, createProxyFor(up))

// =============================================================================
// HTTP request handler
// =============================================================================

const server = http.createServer((req, res) => {
  // Health endpoint — handled inline, not proxied
  if (req.url === '/__gateway/health') {
    return handleHealth(req, res)
  }

  // Gateway-internal banner endpoint (handy for sanity check)
  if (req.url === '/__gateway') {
    res.writeHead(200, { 'Content-Type': 'application/json; charset=utf-8' })
    res.end(JSON.stringify({
      name: 'preview-gateway',
      version: '1.0.0',
      see: '/__gateway/health',
    }, null, 2))
    return
  }

  const up = pickUpstream(req.url, req.headers.referer, req.headers.cookie)
  const proxy = proxies.get(up.name)
  if (!proxy) {
    res.writeHead(500, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({ error: 'no_proxy_for_upstream', upstream: up.name }))
    return
  }

  // Apply pathRewrite (if any) before forwarding
  const originalUrl = req.url
  if (up.pathRewrite) {
    req.url = up.pathRewrite(req.url ?? '/')
  }

  // No body parsing — just forward the stream
  proxy.web(req, res, { target: up.target }, (err) => {
    // Restore req.url in case the connection is reused (defensive)
    if (up.pathRewrite) req.url = originalUrl
    // err already handled by proxy.on('error') listener; this callback
    // is only for sync errors during dispatch.
    if (!res.headersSent) {
      res.writeHead(500, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({
        error: 'gateway_dispatch_error',
        detail: (err as Error).message ?? String(err),
      }))
    }
  })
})

// =============================================================================
// WebSocket upgrade handler (HMR critical — see spec §3.4)
// =============================================================================

server.on('upgrade', (req, socket, head) => {
  const up = pickUpstream(req.url, req.headers.referer, req.headers.cookie)
  const proxy = proxies.get(up.name)
  if (!proxy) {
    socket.write('HTTP/1.1 500 Internal Server Error\r\n\r\n')
    socket.destroy()
    return
  }
  // Apply pathRewrite for WS too (HMR ws path: /openlist-ui/?token=... → /?token=...)
  if (up.pathRewrite) {
    req.url = up.pathRewrite(req.url ?? '/')
  }
  // http-proxy's `ws()` method handles the upgrade transparently
  proxy.ws(req, socket, head, { target: up.wsTarget }, (err) => {
    if (err) {
      logUpstream(req, up, 'FAIL', err)
      try {
        socket.write('HTTP/1.1 502 Bad Gateway\r\n\r\n')
        socket.destroy()
      } catch {
        // socket already destroyed
      }
    }
  })
})

// =============================================================================
// Health endpoint — concurrent ping of all upstreams (spec §3.5)
// =============================================================================

interface UpstreamHealth {
  url: string
  alive: boolean
  latency_ms: number
  error?: string
}

/** 全局子进程管理器（main() 启动时赋值；health 端点读取） */
let childrenManager: ChildrenManager | null = null

async function pingUpstream(up: Upstream): Promise<UpstreamHealth> {
  const start = Date.now()
  const url = new URL(up.target)
  const opts: https.RequestOptions = {
    hostname: url.hostname,
    port: url.port,
    path: '/',
    method: 'HEAD',
    timeout: HEALTH_TIMEOUT_MS,
    headers: { 'User-Agent': 'preview-gateway/health' },
  }
  return new Promise((resolve) => {
    const lib = url.protocol === 'https:' ? https : http
    const req = lib.request(opts, (res) => {
      res.resume() // drain
      resolve({
        url: up.target,
        alive: res.statusCode !== undefined && res.statusCode < 500,
        latency_ms: Date.now() - start,
      })
    })
    req.on('timeout', () => {
      req.destroy(new Error('timeout'))
    })
    req.on('error', (err) => {
      resolve({
        url: up.target,
        alive: false,
        latency_ms: Date.now() - start,
        error: (err as Error).message ?? String(err),
      })
    })
    req.end()
  })
}

async function handleHealth(_req: IncomingMessage, res: ServerResponse): Promise<void> {
  // 黑名单机制下需 health 检查的所有 upstream：
  //   - ENCV_GO_UPSTREAM (默认)
  //   - VITE_UPSTREAM (Vite)
  //   - SPECIAL_UPSTREAMS (plugin-vite, openlist direct)
  const all = [ENCV_GO_UPSTREAM, VITE_UPSTREAM, ...SPECIAL_UPSTREAMS]
  // Deduplicate by name
  const unique = new Map<string, Upstream>()
  for (const up of all) unique.set(up.name, up)
  const upstreamList = Array.from(unique.values())

  // 必检 vs 按需：只有 `required !== false` 的 upstream 计入 ok 计算
  // （plugin-openlist-web / openlist 默认 SPAWN_* off 时按需 down）
  const requiredUpstreams = upstreamList.filter((u) => u.required !== false)
  const optionalUpstreams = upstreamList.filter((u) => u.required === false)

  const checks = await Promise.all(
    upstreamList.map(async (up) => [up.name, await pingUpstream(up)] as const),
  )
  const upstreams: Record<string, UpstreamHealth> = {}
  for (const [name, h] of checks) upstreams[name] = h
  const requiredAlive = requiredUpstreams.every((u) => upstreams[u.name]?.alive === true)

  // 子进程状态：只有方案 C 启用了 ChildrenManager 时才有数据
  const children = childrenManager?.getStatuses() ?? []
  const childrenAlive = children.every((c) => c.ready)

  // ok 定义：所有 required upstream alive + 所有 spawned children ready
  // optional upstream 不参与（按需 down 是预期）
  const ok = requiredAlive && childrenAlive

  res.writeHead(ok ? 200 : 503, { 'Content-Type': 'application/json; charset=utf-8' })
  res.end(
    JSON.stringify(
      {
        ok,
        upstreams,
        children,
        // 冗余字段：方便用户快速看出"哪些是按需"
        optionalDown: optionalUpstreams
          .filter((u) => upstreams[u.name]?.alive !== true)
          .map((u) => ({ name: u.name, url: u.target, hint: u.hint })),
      },
      null,
      2,
    ),
  )
}

// =============================================================================
// Child process orchestration (方案 C：网关合一)
// =============================================================================

/**
 * 根据 env 决定要启哪些子进程。env 默认值遵循"沙箱 dev 必启 + 其它按需"原则。
 *   SPAWN_GO=1          (default 1) — air → encv-go on :2025
 *   SPAWN_VITE=1        (default 1) — encv-mobile Vite on :8100
 *   SPAWN_PLUGIN_VITE=0 (default 0) — plugin-openlist-web Vite on :5174
 *   SPAWN_OPENLIST=0    (default 0) — OpenList Go fork on :5244
 *
 * 任何 SPAWN_X 显式设 0 即关闭该子进程 — gateway 仅转发，不管理。
 */
function buildChildSpecs(paths: ReturnType<typeof resolvePaths>): ChildSpec[] {
  const specs: ChildSpec[] = []

  // 1) encv-go (air) — mobile overlay 触发的关键
  if (process.env.SPAWN_GO !== '0') {
    specs.push({
      name: 'encv-go',
      cmd: paths.airBin,
      args: [],
      // env 注入铁律：ENCV_DEV_PREVIEW=1 / ENCV_MOBILE=1 必须传递
      // （go run 路径下 .air-run.sh 兜底已删除，pm2 ecosystem.config.cjs env
      //  → preview-gateway process.env → air → go run → encv binary 一路继承）
      // 2026-06-10 改造：删 ENCV_MOCK_ROOT（mobile overlay 直接决定 servingDir，不需要这个 env 中转）
      env: {
        ...process.env,
        ENCV_DEV_PREVIEW: process.env.ENCV_DEV_PREVIEW ?? '1',
        ENCV_MOBILE: process.env.ENCV_MOBILE ?? '1',
      },
      cwd: paths.repoRoot,
      readyUrl: 'http://127.0.0.1:2025/api/config',
      // 沙箱首次冷编实测 3-5 分钟（go mod 全量 + 全量 build + 沙箱 CPU 慢）
      // 给 10 分钟兜底，避免 pm2 死循环重启
      readyTimeoutMs: 600_000,
    })
  }

  // 2) encv-mobile Vite — 主 app
  if (process.env.SPAWN_VITE !== '0') {
    specs.push({
      name: 'encv-mobile-vite',
      cmd: paths.nodeBin,
      args: [paths.viteJsMain, '--host', '0.0.0.0', '--port', '8100', '--strictPort'],
      env: { ...process.env, PATH: process.env.PATH ?? '' },
      cwd: paths.mobileDir,
      readyUrl: 'http://127.0.0.1:8100/',
      readyTimeoutMs: 30_000,
    })
  }

  // 3) plugin-openlist-web Vite — 默认不启（按需）
  if (process.env.SPAWN_PLUGIN_VITE === '1') {
    specs.push({
      name: 'plugin-openlist-vite',
      cmd: paths.nodeBin,
      args: [paths.viteJsPlugin, '--host', '0.0.0.0', '--port', '5174', '--strictPort'],
      env: {
        ...process.env,
        PATH: process.env.PATH ?? '',
        // 沙箱 dev 必须设 VITE_BASE=/openlist-ui/（D11 修复，详见 plugin README）
        VITE_BASE: '/openlist-ui/',
      },
      cwd: paths.pluginWebDir,
      readyUrl: 'http://127.0.0.1:5174/',
      readyTimeoutMs: 30_000,
    })
  }

  // 4) OpenList 真实 fork — 默认不启（重 + 慢 + 多数沙箱场景不需要）
  if (process.env.SPAWN_OPENLIST === '1') {
    specs.push({
      name: 'openlist',
      cmd: 'bash',
      args: [paths.openlistScript, '--port', '5244'],
      env: {
        ...process.env,
        PATH: process.env.PATH ?? '',
        OPENLIST_DATA: '/tmp/openlist-data',
      },
      cwd: paths.mobileDir,
      readyUrl: 'http://127.0.0.1:5244/',
      readyTimeoutMs: 60_000,
    })
  }

  // 5) simverse-frontend Vite — SimVerse 独立前端
  if (process.env.SPAWN_SIMVERSE_VITE !== '0') {
    specs.push({
      name: 'simverse-frontend',
      cmd: paths.nodeBin,
      args: [paths.viteJsMain, '--host', '0.0.0.0', '--port', '8200', '--strictPort'],
      env: { ...process.env, PATH: process.env.PATH ?? '' },
      cwd: paths.simverseFrontendDir,
      readyUrl: 'http://127.0.0.1:8200/',
      readyTimeoutMs: 30_000,
    })
  }

  return specs
}

// =============================================================================
// Startup (async main)
// =============================================================================

/**
 * 主启动流程。串行：解析路径 → preflight → 子进程就绪 → server.listen。
 * 任何一步失败 throw，pm2 会看到 exit 1 → 整套重启。
 */
async function main(): Promise<void> {
  // ── Step 1: 路径解析（fail-fast）──
  const paths = resolvePaths()

  // ── Step 2：preflight 建空 mobileDataDir ──
  // 2026-06-14 修复：传 mobileDataDir（= /storage/emulated/0），不是 mobileDir（= app/encv-mobile）
  // 之前传错导致 service-guard 一直 BLOCK：os.Stat('/workspace/app/encv-mobile') ≠ '/storage/emulated/0'，
  // mobile 真机/预览标准路径不匹配。
  // ensureMockData 只建空目录，mock 数据由用户主动调后端 /api/mock/generate 生成。
  await ensureMockData(paths.mobileDataDir)

  // ── Step 3: 启动子进程 ──
  childrenManager = new ChildrenManager()
  const specs = buildChildSpecs(paths)
  if (specs.length === 0) {
    log('no children to spawn (all SPAWN_* off) — gateway serves as pure proxy')
  } else {
    await childrenManager.startAll(specs)
  }

  // ── Step 4: 防御守卫（黑名单机制已无 UPSTREAMS 白名单）──
  //   历史教训：2026-06-07 曾因 UPSTREAMS 漏配 /ws 导致 WebSocket 卡死
  //   现已统一走 ENCV_GO_UPSTREAM 默认 upstream，无需手动维护。
  //   此处保留 hook 以便未来加新防御性检查。

  // ── Step 5: 启动 HTTP server ──
  server.listen(PORT, HOST, () => {
    log(`listening on http://${HOST}:${PORT} (D1: 好记，16666)`)
    log(`children spawned: ${specs.length} (${specs.map((s) => s.name).join(', ') || 'none'})`)
    log(`routing strategy: DENYLIST (default = encv-go :2025; VITE_DENY → encv-mobile Vite :8100)`)
    log(`default upstream: ${ENCV_GO_UPSTREAM.name} → ${ENCV_GO_UPSTREAM.target}`)
    log(`vite denylist (${VITE_DENY.length} rules):`)
    for (const rule of VITE_DENY) {
      const prefix = rule.mode === 'exact' ? '   = ' : '  ⊆ '
      log(`   ${prefix}${rule.match.padEnd(22)} (${rule.why})`)
    }
    log(`special upstreams:`)
    for (const up of SPECIAL_UPSTREAMS) {
      log(`   ⊆ ${(up.match ?? '').padEnd(22)} → ${up.target}  (${up.name})${up.pathRewrite ? ' [pathRewrite]' : ''}`)
    }
    log(`health:  http://${HOST}:${PORT}/__gateway/health`)
    log(`external: :16000 (OpenPreview) → :${PORT} (this gateway) after agent-browser navigate triggers auto-register`)
  })
}

// Graceful shutdown (pm2 sends SIGINT)
let shuttingDown = false
async function shutdown(signal: string): Promise<void> {
  if (shuttingDown) return
  shuttingDown = true
  log(`received ${signal}, shutting down...`)
  // 先停子进程（kill 顺序：SIGTERM → 5s → SIGKILL），再关 server
  if (childrenManager) {
    await childrenManager.stopAll()
  }
  server.close(() => {
    log(`server closed, exit 0`)
    process.exit(0)
  })
  setTimeout(() => {
    log(`shutdown timeout, force exit`)
    process.exit(1)
  }, 8_000).unref()
}

for (const sig of ['SIGINT', 'SIGTERM'] as const) {
  process.on(sig, () => {
    void shutdown(sig)
  })
}

// 启动入口（顶层 await 在 ESM 下可用）
main().catch((err) => {
  log(`FATAL: main() failed: ${(err as Error).message}`)
  log((err as Error).stack ?? '')
  process.exit(1)
})
