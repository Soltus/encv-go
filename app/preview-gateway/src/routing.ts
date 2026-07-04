/**
 * preview-gateway 路由匹配核心逻辑（黑名单机制）
 * ==============================================
 *
 * 抽离到独立模块以便单测。server.ts 引用此模块的 pickUpstream。
 *
 * 设计原则（2026-06-09 黑名单改造）：
 *   - 默认 upstream = encv-go（一切后端端点）
 *   - 特殊 upstream（带 pathRewrite）优先
 *   - 黑名单 VITE_DENY 命中 → Vite
 *
 * 优势：未来 encv-go 新增后端端点**无需修改** gateway 配置。漏配问题彻底解决。
 */

export interface Upstream {
  /** URL prefix on the gateway（只用于 SPECIAL_UPSTREAMS，encv-go/VITE_UPSTREAM 无需） */
  match?: string
  /** HTTP target URL (no trailing slash) */
  target: string
  /** WebSocket target URL */
  wsTarget: string
  /** Human-readable name for logging */
  name: string
  /** Hint shown in 502 error */
  hint: string
  /** 是否 health 必检（false = 按需上游） */
  required?: boolean
  /** Path rewrite function（特殊 upstream 专属） */
  pathRewrite?: (path: string) => string
}

export interface ViteDenyRule {
  match: string
  mode: 'exact' | 'prefix'
  why: string
}

/** 特殊 upstream：带 pathRewrite 的少数路由 */
export const SPECIAL_UPSTREAMS: Upstream[] = [
  {
    match: '/openlist-ui',
    target: 'http://127.0.0.1:5174',
    wsTarget: 'ws://127.0.0.1:5174',
    name: 'plugin-openlist-web',
    hint: 'Check pm2 status for plugin-openlist-vite',
    required: false,
  },
  {
    match: '/openlist',
    target: 'http://127.0.0.1:5244',
    wsTarget: 'ws://127.0.0.1:5244',
    name: 'openlist-direct',
    hint: 'Check pm2 status for openlist (:5244)',
    required: false,
    pathRewrite: (p) => p.replace(/^\/openlist(?=\/|$)/, '') || '/',
  },
]

/** 黑名单：命中走 Vite */
export const VITE_DENY: ViteDenyRule[] = [
  // ① SPA HTML 路由（encv-mobile Vue Router）
  { match: '/',       mode: 'exact',  why: '根路径：Vite serve index.html，SPA 入口' },
  { match: '/player', mode: 'prefix', why: 'ArtPlayerView SPA（router/index.ts:12）' },
  { match: '/tabs',   mode: 'prefix', why: 'Tabs SPA 全部子路由（home/files/tasks/settings/devlogs/...）' },

  // ② Vite dev artifacts
  { match: '/@vite/',         mode: 'prefix', why: 'Vite HMR client + module graph' },
  { match: '/@fs/',           mode: 'prefix', why: 'Vite fs allowlist' },
  { match: '/@id/',           mode: 'prefix', why: 'Vite virtual module id' },
  { match: '/@react-refresh', mode: 'exact',  why: 'React HMR 桥' },
  { match: '/@client',        mode: 'exact',  why: 'Vite 内部 client-side HMR' },
  { match: '/src/',           mode: 'prefix', why: 'Vite 源码模块' },
  { match: '/node_modules/',  mode: 'prefix', why: 'Vite 优化后的 deps' },

  // ③ 静态资源
  { match: '/assets/',     mode: 'prefix', why: 'Vite build assets' },
  { match: '/public/',     mode: 'prefix', why: 'Vite public 目录' },
  { match: '/favicon.ico', mode: 'exact',  why: 'favicon' },
  { match: '/sw.js',       mode: 'exact',  why: 'Service Worker' },
  { match: '/manifest',    mode: 'prefix', why: 'PWA manifest' },
]

/** 默认 upstream：encv-go。处理所有后端 API/stream/WS/... */
export const ENCV_GO_UPSTREAM: Upstream = {
  target: 'http://127.0.0.1:2025',
  wsTarget: 'ws://127.0.0.1:2025',
  name: 'encv-go',
  hint: 'Check pm2 status for start-preview (encv-go :2025)',
}

/** 黑名单命中时使用的 upstream：encv-mobile Vite */
export const VITE_UPSTREAM: Upstream = {
  target: 'http://127.0.0.1:8100',
  wsTarget: 'ws://127.0.0.1:8100',
  name: 'encv-mobile-vite',
  hint: 'Check pm2 status for start-preview (encv-mobile vite :8100)',
}

/**
 * 前缀匹配：path 等于 prefix 或 path 以 prefix + '/' 开头。
 * '/stream' 命中 '/stream' 和 '/stream/...'，但不命中 '/streamer'。
 * '/@vite/' 命中 '/@vite/' 和 '/@vite/client'，但不命中 '/@vite-other'。
 *
 * 正确处理两种 prefix 形态：
 *   - prefix 不带尾斜杠（如 '/stream'）：要求 path === prefix 或 path === prefix + '/...'
 *   - prefix 带尾斜杠（如 '/@vite/'）：要求 path === prefix 或 path === prefix + '...'
 */
export function matchesPrefix(path: string, prefix: string): boolean {
  // 规范化：剥掉 prefix 尾部的 /（如果存在）
  const norm = prefix.endsWith('/') ? prefix.slice(0, -1) : prefix
  if (path === norm) return true
  // 严格：path 必须以 norm + '/' 开头（防止 /stream 匹配 /streamer）
  if (path.startsWith(norm + '/')) return true
  return false
}

/** VITE_DENY 规则匹配 */
export function matchesViteDeny(path: string, rule: ViteDenyRule): boolean {
  if (rule.mode === 'exact') return path === rule.match
  // prefix 模式：分两种 prefix 形态
  //   1) prefix 带尾 '/'（如 '/@vite/'）：rule.match 已是分隔符，path 只需以 rule.match 开头
  //      '/@vite/' 命中 '/@vite/'、'/@vite/client'，但不命中 '/@vite-other'（'/@vite' 不在 '/@vite-other' 之前的边界）
  //   2) prefix 不带尾 '/'（如 '/manifest'）：需要 path === match、或 path 以 match + '/' / match + '.' 开头
  //      '/manifest' 命中 '/manifest'、'/manifest.json'、'/manifest/foo'，但不命中 '/manifest-foo'
  if (path === rule.match) return true
  if (!path.startsWith(rule.match)) return false
  if (rule.match.endsWith('/')) {
    // 已带分隔符，path 只需以 rule.match 开头（确保不是 /@vite-other 这种）
    // 即: rule.match.length < path.length
    return path.length > rule.match.length
  }
  // 不带分隔符：next char 必须是 / 或 .（防止 /manifest 误匹配 /manifest-foo）
  const next = path[rule.match.length]
  return next === '/' || next === '.'
}

/**
 * 黑名单机制下的路由匹配。
 *
 * 匹配顺序（first match wins）：
 *   ① SPECIAL_UPSTREAMS — 特殊 upstream（带 pathRewrite）
 *   ② Cookie / Referer 兜底 — plugin SPA 子资源
 *   ③ VITE_DENY 黑名单 — SPA / Vite artifacts / 静态资源 → Vite
 *   ④ ENCV_GO_UPSTREAM — 默认（一切后端端点）
 */
export function pickUpstream(
  url: string | undefined,
  referer: string | undefined,
  cookie: string | undefined,
): Upstream {
  // req.url 包含 query string。'/stream?path=xxx' 不应误判为「不是 /stream」。
  const pathOnly = url?.split('?', 1)[0] ?? '/'

  // ① 特殊 upstream 优先
  for (const up of SPECIAL_UPSTREAMS) {
    if (!up.match) continue
    if (matchesPrefix(pathOnly, up.match)) return up
  }

  // ② Cookie / Referer 兜底：plugin SPA 子资源
  if (cookie && /(?:^|;\s*)__plugin_spa=1/.test(cookie)) {
    return SPECIAL_UPSTREAMS.find((u) => u.match === '/openlist-ui') ?? VITE_UPSTREAM
  }
  if (referer && /\/openlist-ui\//.test(referer)) {
    return SPECIAL_UPSTREAMS.find((u) => u.match === '/openlist-ui') ?? VITE_UPSTREAM
  }

  // ③ 黑名单：命中 VITE_DENY 的路径走 Vite
  for (const rule of VITE_DENY) {
    if (matchesViteDeny(pathOnly, rule)) return VITE_UPSTREAM
  }

  // ④ 默认：encv-go
  return ENCV_GO_UPSTREAM
}
