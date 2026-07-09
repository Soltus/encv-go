import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import { vueComponentCheckPlugin } from '../../../../app/packages/shared-components/src/vite-plugins/vue-component-check'
import { i18nOptimizePlugin } from '../../../../app/packages/shared-components/src/vite-plugins/i18n-optimize'

function injectBaseHref(href: string): Plugin {
  const basePrefix = href.replace(/\/$/, '')
  return {
    name: 'inject-base-href',
    transformIndexHtml: {
      order: 'post',
      handler(html) {
        let result = html
        result = result.replace(
          '<!--VITE-BASE-HREF-PLACEHOLDER-->',
          `<base href="${href}" />`,
        )
        result = result.replace(
          /<script\s+type="module"\s+src="[^"]*\/@vite\/client"[^>]*><\/script>/g,
          '<!-- @vite/client removed (hmr disabled in sandbox dev) -->',
        )
        return result
      },
    },
  }
}

function dynamicHmrHostPlugin(): Plugin {
  const envHmrHost = process.env.HMR_HOST
  const envHmrProtocol = process.env.HMR_PROTOCOL as 'ws' | 'wss' | undefined
  const envHmrClientPort = process.env.HMR_CLIENT_PORT

  let detectedHost: string | null = null
  let detectedProtocol: 'ws' | 'wss' = 'ws'
  let hostSource: 'env' | 'detected' | 'pending' = 'pending'

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
            `[dynamic-hmr-host] UPDATE host ${prevHost}->${detectedHost} proto ${prevProto}->${detectedProtocol} url=${req.url}`,
          )
        }
        next()
      })

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
            .replace(/__HMR_HOSTNAME__/g, '""')
            .replace(/__HMR_PROTOCOL__/g, '""')
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
  base: process.env.VITE_BASE || './',
  plugins: [vueComponentCheckPlugin(), vue(), i18nOptimizePlugin(), injectBaseHref(process.env.VITE_BASE || '/mpv-ui/'), dynamicHmrHostPlugin()],
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
    emptyOutDir: true,
  },
  server: {
    port: 5175,
    host: '0.0.0.0',
    strictPort: false,
    allowedHosts: true,
    hmr: false,
    fs: {
      allow: [
        path.resolve(__dirname),
        path.resolve(__dirname, '..', '..', '..'),
        path.resolve(__dirname, '..', '..', '..', 'node_modules'),
        path.resolve('/workspace/app/encv-mobile'),
        path.resolve('/workspace/app/encv-mobile/node_modules'),
        path.resolve('/workspace'),
      ],
    },
  },
})
