/**
 * Cypress 配置（Vue 3 + Vite 组件测试）
 * 
 * 设计参考：app/encv-mobile/cypress.config.ts
 * 
 * 核心特性：
 *   - 使用 bundled Electron 浏览器（避免 Chrome 下载超时）
 *   - Component Testing：挂载真组件，测试模板逻辑
 *   - 使用 viteConfig 函数绕过项目 vite.config.ts
 *   - 支持 SIMVERSE_TEST_FULL 环境变量控制全量测试
 * 
 * 用法：
 *   bash scripts/cypress-component.sh SimverseHome      # 单 spec 模糊匹配
 *   SIMVERSE_TEST_FULL=1 bash scripts/cypress-component.sh  # 全量测试（CI 专用）
 */
import { defineConfig } from 'cypress'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const IS_FULL = process.env.SIMVERSE_TEST_FULL === '1'

// 快速组件测试（默认跑）
const FAST_INCLUDE = [
  'cypress/component/_smoke.cy.ts',
  'cypress/component/SimverseHome.cy.ts',
]

// 慢速集成测试（SIMVERSE_TEST_FULL=1 才跑）
const SLOW_INCLUDE = [
  // 待添加：大规模数据测试、WS 事件测试等
]

const COMPONENT_SPECS = IS_FULL
  ? [...FAST_INCLUDE, ...SLOW_INCLUDE]
  : FAST_INCLUDE

export default defineConfig({
  // 全局默认用 Electron
  defaultBrowser: 'electron',

  // ============ Component Testing ============
  component: {
    devServer: {
      framework: 'vue',
      bundler: 'vite',
      viteConfig: async () => ({
        plugins: [vue()],
        resolve: {
          alias: {
            '@self': path.resolve(__dirname, 'src'),
            '@shared': path.resolve(__dirname, '../packages/shared-components/src'),
            '@': path.resolve(__dirname, 'src'),
          },
        },
        server: { hmr: false },
      }),
    },
    indexHtmlFile: 'cypress/support/component-index.html',
    specPattern: COMPONENT_SPECS,
    supportFile: 'cypress/support/component.ts',
    includeShadowDom: true,
    video: false,
    screenshotOnRunFailure: true,
  },

  // ============ E2E Testing ============
  e2e: {
    baseUrl: 'http://localhost:8200',
    specPattern: 'cypress/e2e/**/*.cy.ts',
    supportFile: 'cypress/support/e2e.ts',
    includeShadowDom: true,
    video: false,
    screenshotOnRunFailure: true,
  },

  // ============ 全局配置 ============
  viewportWidth: 1280,
  viewportHeight: 720,
  
  setupNodeEvents(on, config) {
    // Electron 沙箱参数
    on('before:browser:launch', (browser, launchOptions) => {
      if (browser.name === 'electron') {
        launchOptions.args = launchOptions.args || []
        launchOptions.args.push('--no-sandbox')
        launchOptions.args.push('--disable-dev-shm-usage')
      }
      return launchOptions
    })
    return config
  },
})
