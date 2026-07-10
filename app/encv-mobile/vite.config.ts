import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import fs from 'node:fs'

// =============================================================================
// ⚠️ 防御机制：禁止直接 vite 启动（必须通过 PM2 → preview-gateway）
// =============================================================================
//
// 本项目架构：preview-gateway(:16666) spawn vite(:8100) 作为子进程。
// 唯一合法启动链路：
//   pm2 start ecosystem.config.cjs
//     → preview-gateway (spawn vite with SPAWN_VITE=1 env)
//       → vite 正常启动
//
// 非法启动方式一律被 dev-start-guard 拦截并抛错：
//   - 直接 vite / npm run dev / pnpm exec vite
//   - CI=true 绕过（2026-06-15 已收紧，CI 不应跑 vite dev）
//   - PPA_SPAWNED=1 包装（2026-06-15 已收紧，仅 PM2 进程树合法）
//   - nohup / bash -c 包装
//
// 守卫实现：./src/lib/dev-start-guard.ts
// 单测：      src/composables/__tests__/dev-start-guard.test.ts
// 文件必须在 src/ 下 — vite 5/6/7/8 默认不 transform src/ 外的 .ts，
// scripts/ 下的 .ts 会被 vite 当 external → 守卫拿不到 devStartGuard 函数
import { devStartGuard } from '../packages/shared-components/src/lib/dev-start-guard'
import { frontendDepsManifestPlugin } from './vite-plugins/frontend-deps-manifest'
import { i18nOptimizePlugin } from '../packages/shared-components/src/vite-plugins/i18n-optimize'
import { vueComponentCheckPlugin } from '../packages/shared-components/src/vite-plugins/vue-component-check'
import { fileSizeLimitPlugin } from '../packages/shared-components/src/vite-plugins/file-size-limit'
import Components from 'unplugin-vue-components/vite'

// =============================================================================
// ENCV Mobile Vite Config
// =============================================================================
// D9 决策（spec/unify-sandbox-preview-port §3.1）: vite 是纯净 SPA dev server，
// 不做任何反向代理。统一由 preview-gateway (:16666) 接管跨上游转发。
//
// 历史胶水（已撤销）:
//   - `cors: { origin: '*' }` —— 用于绕过 agent-tool-host 的 Origin 改写。
//     现在 :16666 网关用 changeOrigin: false 透传 Origin 头，Vite 默认
//     cors=true 会 reflect origin，与浏览器 Origin 匹配，CORS 天然通过。
//   - `server.proxy: { '/api', '/p', '/openlist/', '/play' }` —— 跨上游转发。
//     全部迁移到 preview-gateway :16666 的 UPSTREAMS 列表。
//   - `openlistUiProxy` plugin —— /openlist-ui 在主 app 内嵌时用的辅助中间件。
//     实际路由现在由 preview-gateway :16666/openlist-ui → plugin-openlist-web :5174
//     独立处理（plugin-openlist-web vite 自己用 VITE_BASE=/openlist-ui/ 处理前缀）。
//
// D14 决策（spec/unify-sandbox-preview-port）：沙箱 dev HMR 修复
//   - vite 默认 server.hmr.host = server.host = '0.0.0.0'，
//     浏览器无法连接（沙箱 dev 浏览器跑在外部 trae.cn 域名）
//   - 用 dynamicHmrHostPlugin 在 enforce:'pre' 阶段拦截 @vite/client，
//     替换 __HMR_HOSTNAME__ / __HMR_PORT__ / __HMR_PROTOCOL__ / __HMR_BASE__
//     为 auto-detected 外部 host + 网关端口 16666
// =============================================================================

/**
 * 沙箱 dev 动态 HMR host 修复（主 app 版本）
 *
 * 与 plugin-openlist/web 的同名插件逻辑一致，但端口默认 16666 (preview-gateway)。
 * HMR config 来源优先级：env (HMR_HOST / HMR_PROTOCOL / HMR_CLIENT_PORT) >
 *                       auto-detect (首次请求 Host 头) > fallback 'localhost'。
 *
 * 工作流程：
 *   1. 浏览器 GET http://<external-host>:16666/ → 网关 → :8100 vite
 *   2. vite 中间件检测 req.headers.host = '<external-host>:16666' → 存 detected
 *   3. vite 响应 HTML (含 <script src="/@vite/client">)
 *   4. 浏览器 GET /@vite/client → 网关 → :8100 vite
 *   5. transform @vite/client → 本插件 enforce:'pre' 替换占位符
 *   6. 浏览器收到 client.mjs，HMR client 连接 ws://<external-host>:16666/?token=...
 *   7. 网关 WS upgrade → 路由到 :8100，HMR 成功
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
   *   1. X-Forwarded-Proto 头（preview-gateway 透传，HTTPS→:8100 时设为 'https'）
   *   2. Referer 头（页面层协议）
   *   3. req.socket.encrypted（直连 HTTPS，但 :8100 一般是 HTTP，兜底）
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
    if (envHmrHost) {
      hostSource = 'env'
      return { host: envHmrHost, protocol: envHmrProtocol || 'ws' }
    }
    if (reqHost) {
      hostSource = 'detected'
      return { host: reqHost.split(':')[0], protocol: proto || 'ws' }
    }
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
      //   - 用 transform 钩子有缺陷：vite 模块图会缓存 transformed code，
      //     我的 mutable 状态（detectedHost/detectedProtocol）变了，但 cache hit
      //     不会重跑 transform → 旧 host 一直保留
      //   - 用中间件直接响应：每次请求都基于最新状态生成
      server.middlewares.use(async (req, res, next) => {
        const url = req.url || ''
        // 匹配 /@vite/client 但不匹配 /@vite/client/env.mjs（vite 内部依赖）
        if (!/^\/@vite\/client(\?|$)/.test(url)) {
          return next()
        }

        try {
          // 读 vite client 源文件
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
          // 尝试读取 vite 内部生成的 ws token
          const wsToken = (server.config as any).webSocketToken

          // ⚠️ 必须替换全部 vite 占位符，否则浏览器解析 JS 报 SyntaxError
          //   vite 8 (rolldown) 的 client.mjs 包含的占位符：
          //     __MODE__, __BASE__, __SERVER_HOST__,
          //     __HMR_PROTOCOL__, __HMR_HOSTNAME__, __HMR_PORT__,
          //     __HMR_DIRECT_TARGET__, __HMR_BASE__, __HMR_TIMEOUT__,
          //     __HMR_ENABLE_OVERLAY__, __HMR_CONFIG_NAME__,
          //     __WS_TOKEN__, __SERVER_FORWARD_CONSOLE__, __BUNDLED_DEV__,
          //     __PURE__
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

export default defineConfig({
  plugins: [
    devStartGuard(),  // ⚠️ 防御：禁止直接 vite 启动，必须通过 PM2 → preview-gateway
    frontendDepsManifestPlugin(),  // 🆕 2026-06-17：读 package.json 生成 frontend-deps.json manifest
    fileSizeLimitPlugin({ failOnError: true }),  // 🆕 工作区文件行数门禁（useAgent.ts 已拆分，主 app 一并强制）
    i18nOptimizePlugin(),  // 🆕 i18n HMR 热重载 + 构建优化
    vueComponentCheckPlugin({
      dev: process.env.NODE_ENV !== 'production',
      failOnError: false,
      exclude: [/node_modules/, /prototypes/],
      globalComponents: [
        'ChatThinking',
        'ChatMarkdown',
        'ChatList',
        'ChatItem',
        'MarkdownStream',
      ],
    }),
    ...(process.env.NODE_ENV === 'production' ? [Components({
      dirs: ['src/components', '../packages/shared-components/src/components'],
      extensions: ['vue'],
      deep: true,
      dts: 'src/components.d.ts',
    })] : []),
    vue(),
    dynamicHmrHostPlugin(),
    // @/ alias 多路径 fallback：优先本地 src，其次 shared-components
    {
      name: 'encv-alias-fallback',
      resolveId(source, importer, options) {
        if (source.startsWith('@/')) {
          const relativePath = source.slice(2)
          // 去掉 ?worker / ?raw / ?url 等查询参数
          const cleanPath = relativePath.split('?')[0]
          const query = relativePath.includes('?') ? '?' + relativePath.split('?')[1] : ''
          
          const dirs = [
            path.resolve(__dirname, 'src'),
            path.resolve(__dirname, '../packages/shared-components/src'),
          ]
          for (const dir of dirs) {
            const fullPath = path.join(dir, cleanPath)
            const candidates = [
              fullPath,
              fullPath + '.ts',
              fullPath + '.vue',
              fullPath + '.js',
              fullPath + '.tsx',
              fullPath + '.jsx',
              fullPath + '.mjs',
              fullPath + '.cjs',
              path.join(fullPath, 'index.ts'),
              path.join(fullPath, 'index.js'),
              path.join(fullPath, 'index.mjs'),
            ]
            for (const tryPath of candidates) {
              if (fs.existsSync(tryPath) && fs.statSync(tryPath).isFile()) {
                return tryPath + query
              }
            }
          }
          // fallback：返回本地 src 路径，让 vite 自己处理错误
          return path.join(dirs[0], cleanPath) + query
        }
        return null
      },
    },
    // ────────────────────────────────────────────────────────────────────────
    // ⚠️ CRITICAL: 沙箱 dev 必须删除 Vite 自动注入的 @vite/client 脚本！
    //
    // vite 8 (rolldown) 即使设了 `server.hmr = false`，**仍然**会注入
    // <script type="module" src="/@vite/client"> —— hmr:false 只关 HMR 的 WS
    // server，**不阻止** htmlRewritePlugin 注入 client 脚本。
    //
    // 沙箱 dev 链路: trae 域名 → agent-tool-host(:16000) → preview-gateway(:16666) → vite(:8100)
    // agent-tool-host 不支持 WebSocket 升级 → 浏览器拿 wss://...:16666/?token=...
    // 主动连接永远立刻被关 → vite 8 client.mjs:892 抛红字 `[vite] failed to connect to websocket`
    //
    // 修复: 在 transformIndexHtml (order: 'post') 物理删除 @vite/client 的 <script> 标签。
    {
      name: 'remove-vite-client-sandbox-dev',
      transformIndexHtml: {
        order: 'post',
        handler(html: string) {
          return html.replace(
            /<script\s+type="module"\s+src="[^"]*\/@vite\/client"[^>]*><\/script>/g,
            '<!-- @vite/client removed (hmr disabled in sandbox dev) -->',
          )
        },
      },
    } as Plugin,
  ],
  server: {
    // 统一入口改 :8100（由 preview-gateway :16666 接管对外暴露）
    port: 8100,
    // 监听所有接口（沙箱 IPv6/IPv4 兼容）
    host: '0.0.0.0',
    // ⚠️ 沙箱 dev 必须允许任意 Host 头：
    //   - vite 5+ 默认 server.allowedHosts 锁 localhost / 127.0.0.1
    //   - 外部 trae.cn 域名会被 vite 拒绝（403 "Blocked request"）
    //   - preview-gateway 透传 Host 头（changeOrigin: false），
    //     vite 看到的是原始外部域名
    //   - 设 true 允许所有 Host
    allowedHosts: true,
    // Vite 默认 cors=true 会 reflect Origin —— 配合 preview-gateway changeOrigin:false，
    // 链路 :16666 → :8100 看到的 Origin=Host 匹配，CORS 天然通过
    hmr: false,
  },
  resolve: {
    alias: {
      '@encv/shared-components': path.resolve(__dirname, '../packages/shared-components/src'),
      '@encv/shared-components/': path.resolve(__dirname, '../packages/shared-components/src') + '/',
    },
  },
  build: {
    rollupOptions: {
      // ⚠️ 防御：显式声明入口 HTML，防止 Vite 自动扫描 plugin-openlist 等子目录的 index.html
      // 导致去 src/views/ 找 OpenListWebView.vue 等不存在的文件（子项目有自己的 vite 配置和 @ alias）
      input: {
        main: path.resolve(__dirname, 'index.html'),
        'debug-decrypt': path.resolve(__dirname, 'public/debug-decrypt.html'),
      },
      output: {
        // Vite 8 (rolldown) requires manualChunks to be a function, not an object.
        manualChunks(id: string) {
          if (id.includes('node_modules')) {
            if (id.includes('artplayer')) return 'vendor-artplayer'
            if (id.includes('@tdesign') || id.includes('tdesign')) return 'vendor-tdesign'
            if (id.includes('markstream') || id.includes('markdown-it')) return 'vendor-markdown'
            if (id.includes('vue-virtual-scroller')) return 'vendor-virtual'
            const coreLibs = ['vue', 'vue-router', '@ionic/vue', '@ionic/vue-router', 'ionicons']
            for (const lib of coreLibs) {
              if (id.includes(lib)) return 'vendor-core'
            }
            return 'vendor'
          }
          if (id.includes('/i18n/') || id.includes('i18n/common') || id.includes('i18n/settings') || id.includes('i18n/tasks')) {
            return 'i18n'
          }
          if (id.includes('useI18n')) {
            return 'i18n'
          }
        },
      },
    },
  },
})
