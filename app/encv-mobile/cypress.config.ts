/**
 * Cypress 配置（2026-06-23 全量替换 jsdom 组件测试）
 *
 * 设计：
 *   - 沙箱网络受限（下载 Chrome 超时），全部用 bundled Electron 浏览器
 *   - Component Testing（Vue + Vite）：挂载真组件，测试模板逻辑
 *     - 真实 ion-content shadow DOM（jsdom 不支持）
 *     - 真实 Worker postMessage 链路
 *     - 真实 ion-infinite-scroll 滚动行为
 *   - E2E Testing（Vite dev server + 真路由）：测试整页交互
 *     - 真实路由 + ion-tabs + keep-alive
 *     - 真实 WS 事件（task:created/task:progress/task:completed）
 *     - 真实后端 API 集成
 *
 * 与 jsdom 区别：
 *   - 不需要 mock Ionic 组件（Electron 有完整 web components）
 *   - 不需要 mock ion-content shadow DOM（Electron 原生支持）
 *   - 不需要 mock Worker（Electron 有完整 Web Worker 支持）
 *   - 不需要 mock IndexedDB（Electron 真实支持）
 *
 * 注意：
 *   - Cypress 15.17.0 + Electron 37.6.0（已 bundled，无需下载）
 *   - defaultBrowser: 'electron' 全局生效，无需每次 --browser electron
 *   - includeShadowDom: true 启用 .shadow() 跨 shadow DOM 查询
 *   - baseUrl: Vite dev server（cypress 启动前需手动起，或用 devServer 自动起）
 */
import { defineConfig } from 'cypress'

export default defineConfig({
  // 全局默认用 Electron（避免 Chrome 下载）
  defaultBrowser: 'electron',

  // ============ Component Testing ============
  component: {
    devServer: {
      framework: 'vue',
      bundler: 'vite',
    },
    indexHtmlFile: 'cypress/support/component-index.html',
    specPattern: 'cypress/component/**/*.cy.ts',
    supportFile: 'cypress/support/component.ts',
    includeShadowDom: true,
    video: false,
    screenshotOnRunFailure: true,
  },

  // ============ E2E Testing ============
  e2e: {
    baseUrl: 'http://localhost:5173',
    specPattern: 'cypress/e2e/**/*.cy.ts',
    supportFile: 'cypress/support/e2e.ts',
    includeShadowDom: true,
    video: false,
    screenshotOnRunFailure: true,
    setupNodeEvents(on, config) {
      config.env.apiBase = process.env.CYPRESS_API_BASE || 'http://localhost:2025'
      return config
    },
  },

  // ============ 全局配置 ============
  viewportWidth: 1280,
  viewportHeight: 720,
  // Electron 沙箱参数（--no-sandbox 必需，root 权限下）
  // 通过 setupNodeEvents 注入
  setupNodeEvents(on, config) {
    on('before:browser:launch', (browser, launchOptions) => {
      if (browser.name === 'electron') {
        // Electron 沙箱
        launchOptions.args = launchOptions.args || []
        launchOptions.args.push('--no-sandbox')
        launchOptions.args.push('--disable-dev-shm-usage')
        // 中文 locale（项目用 i18n zh-CN）
        launchOptions.args.push('--lang=zh-CN')
      }
      return launchOptions
    })
    return config
  },
})
