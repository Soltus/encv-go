import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import { vueComponentCheckPlugin } from '../../../../app/packages/shared-components/src/vite-plugins/vue-component-check'
import { i18nOptimizePlugin } from '../../../../app/packages/shared-components/src/vite-plugins/i18n-optimize'

// ⚠️ 沙箱 dev 必须只用 `path.resolve(__dirname, ...)`，**禁止**用
//   `import { fileURLToPath } from 'node:url' + new URL(...)` 模式！
// 根因：vite 8 (rolldown) 在 bundle vite.config.ts 时，对 `node:url` 的命名导入
//       处理有 bug —— `fileURLToPath` 符号没绑进 namespace，runtime 抛
//       `ReferenceError: fileURLToPath is not defined at vite.config.ts:119:12`。
//       主 app 一直用 `path.resolve(__dirname, ...)` 是因为它 plain CJS-style，
//       bundler 不需要解析 `node:url` 命名导入，所以能跑。
//       plugin-openlist 改成同名风格后，两个 vite 行为一致，bug 也消失。

/**
 * plugin-openlist/web Vite 配置
 *
 * 关键配置：`base: './'` (生产) | `/openlist-ui/` (沙箱 dev, VITE_BASE env)
 *   生产模式：WebView 加载 `file:///android_asset/openlist/index.html`
 *   资源路径必须用相对路径 `./assets/...`（Vite 默认 `/assets/...` 会在 file:// 下 404）
 *
 * 沙箱 dev 启动 OpenList 后端（真实 Hi-Sillot fork）：
 *   Terminal 1: bash scripts/dev-openlist.sh
 *   → 启动 http://127.0.0.1:5244/，前端 dist 来自 app/openlist/Hi-Sillot-OpenList/public/dist/
 *
 * 沙箱 dev 启动本 Vite（plugin 管理 UI，端口 5174）：
 *   Terminal 2: bash scripts/dev-openlist-web.sh
 *   → OpenListWebView 内的 iframe 直访 http://127.0.0.1:5244/#/login
 *
 * Production（Android WebView）：
 *   - WebView 加载 file:///android_asset/openlist/index.html（plugin-openlist/src/main/assets/openlist/）
 *   - iframe 内部直访 http://127.0.0.1:5244/（与本机 OpenList 进程同设备）
 *
 * 撤销 /openlist-spa/ subpath 路由改造：OpenList 应在原始环境 / 跑，
 * iframe / fetch 均直访 :5244，无需 Vite proxy。
 * 但保留 `__openlist-health` 中间件：Node 端探测 5244，绕过浏览器 CORS，
 * 让 OpenListWebView 的状态机有可靠的 health 探测通道。
 */

/**
 * 自定义中间件：显式健康检查端点
 * 解决 fetch('http://127.0.0.1:5244/...', { mode: 'cors' }) 在 502 时被浏览器 CORS 拦截，
 * 导致 res.status 变成 0（opaque），state 误判为 loading 的问题。
 *
 * 直接在 Node 端用 fetch 探测 5244，回 JSON 给浏览器，带 CORS 头 → 永远可读。
 * 同源访问（plugin-openlist vite :5174 fetch 自己 /__openlist-health）也工作。
 */
/**
 * 把 <base href="..."> 注入到 <head> 最早位置 + 删除 Vite 自动注入的 @vite/client
 *
 * 解决两个问题：
 * ① Vite dev 把 <script src="/@vite/client"> 注入到 <head> 顶部，
 *    早于手写 <base>，导致 base href 不生效。
 * ② vite 8 (rolldown) 即使设了 `server.hmr: false`，**仍然**会注入
 *    <script src="/@vite/client"> —— 因为它内部走的是 htmlRewritePlugin
 *    而 hmr:false 只关 HMR 的 WS server，**不阻止** client 脚本注入。
 *    沙箱 dev agent-tool-host :16000 不支持 WS upgrade，所以必须物理删除
 *    这个 script，否则浏览器立刻报 `WebSocket closed without opened`。
 *
 * 实现思路：
 *   1. 在 index.html 写 <!--VITE-BASE-HREF-PLACEHOLDER--> 占位符
 *      (放在 <head> 第一个子元素位置 — Vite 不会移位)
 *   2. plugin 在 transformIndexHtml 钩子 (order: 'post') 把占位符替换成 <base> 标签
 *   3. plugin 在同一个钩子里**直接删除** <script type="module" src=".../@vite/client">
 *
 * 之所以不用 'pre' 钩子直接 prepend <base>：Vite 8 的 order: 'pre' 在某些
 * 内置 plugin 之后才执行（如 htmlRewritePlugin），导致 @vite/client 仍抢先注入。
 */
function injectBaseHref(href: string): Plugin {
  const basePrefix = href.replace(/\/$/, '')
  return {
    name: 'inject-base-href',
    transformIndexHtml: {
      order: 'post',  // 必须 'post' —— 这样 Vite 已注入 @vite/client 后我们才能改其 src
      handler(html) {
        let result = html
        // 1. 替换占位符为 <base> 标签（必须在最前）
        result = result.replace(
          '<!--VITE-BASE-HREF-PLACEHOLDER-->',
          `<base href="${href}" />`,
        )
        // 2. 物理删除 @vite/client 脚本 —— vite 8 hmr:false 不可靠
        //    匹配两种形式（带 / 带不带 basePrefix）：
        //      <script type="module" src="/@vite/client"></script>
        //      <script type="module" src="/openlist-ui/@vite/client"></script>
        result = result.replace(
          /<script\s+type="module"\s+src="[^"]*\/@vite\/client"[^>]*><\/script>/g,
          '<!-- @vite/client removed (hmr disabled in sandbox dev) -->',
        )
        return result
      },
    },
  }
}

function openlistHealthPlugin(): Plugin {
  return {
    name: 'openlist-health',
    configureServer(server) {
      // 挂到 /openlist-ui/__openlist-health —— 与 preview-gateway 透传的完整路径一致。
      // （如果 pathRewrite 曾经剥前缀，URL 会是 /__openlist-health；现在保留前缀，
      //   vite 收到的就是 /openlist-ui/__openlist-health，必须挂这里才能匹配。）
      server.middlewares.use('/openlist-ui/__openlist-health', async (req, res) => {
        const start = Date.now()
        res.setHeader('Content-Type', 'application/json; charset=utf-8')
        res.setHeader('Access-Control-Allow-Origin', '*')
        res.setHeader('Cache-Control', 'no-store')

        const target = 'http://127.0.0.1:5244/api/public/settings'
        const ac = new AbortController()
        const timer = setTimeout(() => ac.abort(), 3000)
        try {
          const r = await fetch(target, { signal: ac.signal })
          clearTimeout(timer)
          const elapsed = Date.now() - start
          res.statusCode = 200
          res.end(JSON.stringify({
            alive: true,
            upstreamStatus: r.status,
            latency: elapsed,
            target,
            ts: Date.now(),
          }))
        } catch (e: any) {
          clearTimeout(timer)
          const elapsed = Date.now() - start
          res.statusCode = 200
          res.end(JSON.stringify({
            alive: false,
            error: e?.name === 'AbortError' ? 'timeout' : (e?.message || String(e)),
            code: e?.cause?.code || e?.code,
            latency: elapsed,
            target,
            ts: Date.now(),
          }))
        }
      })
    },
  }
}

/**
 * 沙箱 dev 动态 HMR host 修复
 *
 * 根因：vite 默认 server.hmr.host = server.host = '0.0.0.0'，
 * 但浏览器无法连接 0.0.0.0 / localhost:5174（沙箱 dev 浏览器跑在外部 trae.cn 域名，
 * localhost 指向浏览器自己的机器，非沙箱服务器）。
 *
 * 修复：在 enforce:'pre' 阶段拦截 @vite/client 模块，替换 __HMR_HOSTNAME__ /
 *       __HMR_PORT__ / __HMR_PROTOCOL__ / __HMR_BASE__ 占位符为
 *       从首次 HTTP 请求的 Host 头检测出的外部域名 + 网关端口 16666。
 *
 * 工作流程：
 *   1. 浏览器 GET http://<external-host>:16666/openlist-ui/
 *   2. 网关剥前缀 → :5174 vite 收到 GET /
 *   3. vite 中间件 (configureServer) 检测 req.headers.host = '<external-host>:16666'
 *      → 存到 detectedHost / detectedProtocol
 *   4. vite 响应 HTML (含 <script src="/openlist-ui/@vite/client">)
 *   5. 浏览器 GET /openlist-ui/@vite/client → 网关剥前缀 → :5174 vite
 *   6. vite transform @vite/client → 本插件 enforce:'pre' 替换占位符
 *   7. 浏览器收到 client.mjs，HMR client 连接 ws://<external-host>:16666/?token=...
 *   8. 网关 WS upgrade → 路由到 :5174，HMR 成功
 *
 * 兜底：如果 HMR_HOST env 已设置，用 env 值；否则 auto-detect。
 */
function dynamicHmrHostPlugin(): Plugin {
  const envHmrHost = process.env.HMR_HOST
  const envHmrProtocol = process.env.HMR_PROTOCOL as 'ws' | 'wss' | undefined
  const envHmrClientPort = process.env.HMR_CLIENT_PORT

  let detectedHost: string | null = null
  let detectedProtocol: 'ws' | 'wss' = 'ws'
  let hostSource: 'env' | 'detected' | 'pending' = 'pending'

  /**
   * 多源探测 HTTPS：
   *   1. X-Forwarded-Proto 头（preview-gateway 透传，HTTPS→:5174 时设为 'https'）
   *   2. Referer 头（页面层协议）
   *   3. req.socket.encrypted（直连 HTTPS，但 :5174 一般是 HTTP，兜底）
   *   4. Host 端口（:443 → https，其余 → http）
   * 优先级递减。任一命中即返回。
   */
  function detectProtocolFromReq(req: any): 'ws' | 'wss' {
    const xfp = String(req.headers?.['x-forwarded-proto'] || '').toLowerCase().split(',')[0].trim()
    if (xfp === 'https') return 'wss'
    if (xfp === 'http') return 'ws'

    const ref = String(req.headers?.referer || req.headers?.referrer || '')
    if (ref.startsWith('https://')) return 'wss'
    if (ref.startsWith('http://')) return 'ws'

    if (req.socket?.encrypted || req.connection?.encrypted) return 'wss'

    const host = String(req.headers?.host || '')
    if (host.endsWith(':443')) return 'wss'

    return 'ws'
  }

  function resolveHost(
    reqHost?: string,
    proto?: 'ws' | 'wss',
  ): { host: string; protocol: 'ws' | 'wss' } {
    // 优先用 env
    if (envHmrHost) {
      hostSource = 'env'
      return { host: envHmrHost, protocol: envHmrProtocol || 'ws' }
    }
    // 其次 auto-detect
    if (reqHost) {
      hostSource = 'detected'
      return { host: reqHost.split(':')[0], protocol: proto || 'ws' }
    }
    // 最后 fallback
    return { host: 'localhost', protocol: 'ws' }
  }

  return {
    name: 'dynamic-hmr-host',
    enforce: 'pre',
    configureServer(server) {
      // ① 中间件：每次请求都用最新 host/protocol 覆盖
      //   - 不再过滤"本地 host"——localhost 才是沙箱本地调试最常用的访问方式
      //   - 历史 BUG：之前用 isLocalHost 过滤，导致用户从 trae 域名切到 localhost 后，
      //     detectedHost 锁在 trae 域名，@vite/client 注入错误 host，浏览器 HMR 死
      //   - 同一时刻只会有一个访问者（沙箱 preview 单人调试），所以"最新请求赢"是安全的
      server.middlewares.use((req, res, next) => {
        if (!req.headers.host) return next()

        const rawHost = req.headers.host.split(':')[0]
        const proto = detectProtocolFromReq(req)

        const prevHost = detectedHost
        const prevProto = detectedProtocol
        detectedHost = rawHost
        detectedProtocol = proto
        hostSource = 'detected'
        if (prevHost !== detectedHost || prevProto !== detectedProtocol) {
          console.log(
            `[dynamic-hmr-host] UPDATE host ${prevHost}->${detectedHost} proto ${prevProto}->${detectedProtocol} url=${req.url} xfp=${req.headers['x-forwarded-proto'] || ''} ref=${String(req.headers.referer || '').substring(0, 60)}`,
          )
        }
        next()
      })

      // ② ⚠️ 关键：直接接管 /@vite/client 请求的响应
      //   - 用 transform 钩子有缺陷：vite 模块图缓存 mutable 状态
      //   - 用中间件直接响应：每次请求都基于最新状态生成
      server.middlewares.use(async (req, res, next) => {
        const url = req.url || ''
        if (!/^\/@vite\/client(\?|$)/.test(url)) {
          return next()
        }

        try {
          const viteClientPath = require.resolve('vite/dist/client/client.mjs', {
            paths: [server.config.root],
          })
          const fs = await import('node:fs/promises')
          let code = await fs.readFile(viteClientPath, 'utf-8')

          const { host, protocol } = resolveHost(detectedHost ?? undefined, detectedProtocol)
          const port = envHmrClientPort ? Number(envHmrClientPort) : 16666
          const base = '/'
          const devBase = server.config.base || '/'
          const mode = server.config.mode || 'development'
          const serverHostStr = `${host}:${port}${devBase}`
          const directTarget = `${host}:${port}${devBase}`
          const hmrTimeout = 30000
          const hmrEnableOverlay = true
          const hmrConfigName = 'vite.config.ts'
          const wsToken = (server.config as any).webSocketToken

          // ⚠️ 必须替换全部 vite 8 (rolldown) client.mjs 占位符
          code = code
            .replace(/__MODE__/g, JSON.stringify(mode))
            .replace(/__BASE__/g, JSON.stringify(devBase))
            .replace(/__SERVER_HOST__/g, JSON.stringify(serverHostStr))
            .replace(/__HMR_PORT__/g, JSON.stringify(port))
            .replace(/__HMR_DIRECT_TARGET__/g, JSON.stringify(directTarget))
            .replace(/__HMR_BASE__/g, JSON.stringify(base))
            .replace(/__HMR_TIMEOUT__/g, JSON.stringify(hmrTimeout))
            .replace(/__HMR_ENABLE_OVERLAY__/g, JSON.stringify(hmrEnableOverlay))
            .replace(/__HMR_CONFIG_NAME__/g, JSON.stringify(hmrConfigName))
            .replace(/__WS_TOKEN__/g, JSON.stringify(wsToken ?? ''))
            .replace(/__SERVER_FORWARD_CONSOLE__/g, 'false')
            .replace(/__BUNDLED_DEV__/g, 'false')
            // ⚠️ CRITICAL: HMR hostname 必须注入空字符串，让 vite 8 fallback 到
            //   `__HMR_HOSTNAME__ || importMetaUrl.hostname` 的 importMetaUrl 分支。
            //   沙箱 trae 域名 hash 部分（run-agent-{32hex}...）每次 sandbox 重启都变，
            //   我们检测到的 host 在 sandbox 重启后立刻作废，注入新值也无意义。
            //   改用 importMetaUrl = import.meta.url 的 host（当前页面的 origin），
            //   浏览器会永远用正确的当前 sandbox 域名连 HMR WS。
            .replace(/__HMR_HOSTNAME__/g, '""')
            // 协议同理：注入空字符串 → vite 8 fallback 到 importMetaUrl.protocol
            //   importMetaUrl.protocol === "https:" ? "wss" : "ws"
            //   用户访问 https://... → "wss"，http://... → "ws"，永远正确
            .replace(/__HMR_PROTOCOL__/g, '""')
            // ⚠️ __PURE__ 是 rollup tree-shaking 注解（/* @__PURE__ */），
            //   在 vite 8 client.mjs 中只以这种合法 JS 注释形式出现。
            //   浏览器能正常解析 /* @__PURE__ */，无需替换。
            //   切勿替换为 /*#__PURE__*/ —— 会与 @ 前缀拼成 /* @/*#__PURE__*/ */ 这种非法嵌套注释。
            // ⚠️ client.mjs 第 1 行有 `import "@vite/env"` —— vite 的虚拟模块
            //   标识符，正常 transform 流程会被 resolve 成 /@vite/env 真实 URL。
            //   我们 middleware 直返绕过了 transform 流程，必须手动把裸标识符
            //   替换为绝对 URL，否则浏览器报：
            //     Uncaught TypeError: Failed to resolve module specifier "@vite/env"
            .replace(/import\s+["']@vite\/env["'];?/g, 'import "/@vite/env";')

          res.setHeader('Content-Type', 'text/javascript')
          res.setHeader('Cache-Control', 'no-cache')
          res.statusCode = 200
          res.end(code)
        } catch (e) {
          console.error('[dynamic-hmr-host] @vite/client serve failed:', e)
          next()
        }
      })
    },
  }
}

/**
 * 沙箱 dev / 真机 prod 区分
 *  - sandbox dev (VITE_BASE=/openlist-ui/): HTML base = /openlist-ui/
 *      原因：dev_preview_proxy 在 :2025 把 /openlist-ui/* 反代到本 vite :5174
 *      vite 收到 /openlist-ui/src/main.ts，base 匹配，serve web/src/main.ts
 *      资源路径是绝对 /openlist-ui/assets/...，浏览器解析为同源请求（:2025）
 *  - production (默认 './'): HTML base = ./
 *      原因：Android WebView 加载 file:///android_asset/openlist/index.html
 *      资源路径必须相对 ./assets/...（绝对路径在 file:// 协议下 404）
 */

export default defineConfig({
  // ⚠️ 沙箱 dev 用绝对 base（VITE_BASE），生产用相对 './'
  // 沙箱 dev：HTML 内 <base href="/openlist-ui/">，vite 处理 /openlist-ui/* 前缀
  // 生产：HTML 内 <base href="./">，Android WebView file:// 协议下加载相对资源
  base: process.env.VITE_BASE || './',
  plugins: [vueComponentCheckPlugin(), vue(), i18nOptimizePlugin(), openlistHealthPlugin(), injectBaseHref(process.env.VITE_BASE || '/openlist-ui/'), dynamicHmrHostPlugin()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
      '@encv/shared-components': path.resolve(__dirname, '../../../../app/packages/shared-components/src'),
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    sourcemap: false,
    // 显式清空 dist（保证 CI 干净构建）
    emptyOutDir: true,
    // 生成包含 <base href="./"> 的 index.html（file:// 加载必需）
    // Vite 默认会处理，这里只是注释强调
  },
  server: {
    port: 5174,
    host: '0.0.0.0',
    strictPort: false,
    // ⚠️ 沙箱 dev 必须允许任意 Host 头：
    //   - vite 5+ 默认 server.allowedHosts 锁 localhost / 127.0.0.1
    //   - 外部 trae.cn 域名（如 run-agent-...trae.cn）会被 vite 拒绝（403）
    //   - preview-gateway 透传 Host 头（changeOrigin: false），
    //     所以 vite 看到的是原始外部域名，不是 localhost
    //   - 设 true 允许所有 Host，匹配 preview-gateway 反代场景
    allowedHosts: true,
    // ⚠️ CRITICAL: 沙箱 dev 必须禁用 HMR！ (详见主 app vite.config.ts 同名注释)
    // 链路: trae 域名 → agent-tool-host(:16000) → preview-gateway(:16666) → vite(:5174)
    // agent-tool-host 不支持 WebSocket 升级 → vite 8 client.mjs:892 永远抛 `WebSocket closed without opened`
    // 修复: hmr:false 让 vite 不再注入 /@vite/client, console 不再有 WS 错误
    // 代价: 用户改代码需 Ctrl+R 硬刷 —— 沙箱 dev 可接受
    hmr: false,
    // ⚠️ 沙箱 dev 必须扩展 server.fs.allow：
    //   - vite 默认 fs.allow 只允许项目根目录 + 其祖先
    //   - main.ts 内 import "/@fs/workspace/app/encv-mobile/node_modules/..." 引用的是
    //     encv-mobile 主 app 的 node_modules（plugin-openlist/web 自己没装 @ionic/vue）
    //   - 不扩 allow 时 vite 返回 403/404/SPA fallback → 浏览器收到 text/html →
    //     ES module loader 拒绝执行 → main.ts 中断 → 空白
    fs: {
      allow: [
        path.resolve(__dirname),
        path.resolve(__dirname, '..', '..', '..'),  // encv-mobile root（包含 monorepo node_modules）
        path.resolve(__dirname, '..', '..', '..', 'node_modules'),
        path.resolve('/workspace/app/encv-mobile'),
        path.resolve('/workspace/app/encv-mobile/node_modules'),
        path.resolve('/workspace'),
      ],
    },
  },
})
