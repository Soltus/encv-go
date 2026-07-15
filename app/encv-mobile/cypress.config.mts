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
 * ⚠️ 关键：viteConfig 必须是函数（不读项目根 vite.config.ts）
 *   - 项目 vite.config.ts 第一个 plugin 是 devStartGuard()，拦截 !PM2_HOME 启动
 *   - cypress 内部 Vite 加载项目 vite.config.ts 时 devStartGuard 会抛错
 *   - 用 viteConfig 函数返回最小配置，绕过项目 vite.config.ts
 *
 * 注意：
 *   - Cypress 15.17.0 + Electron 37.6.0（已 bundled，无需下载）
 *   - defaultBrowser: 'electron' 全局生效，无需每次 --browser electron
 *   - includeShadowDom: true 启用 .shadow() 跨 shadow DOM 查询
 *   - baseUrl: Vite dev server（cypress 启动前需手动起，或用 devServer 自动起）
 *
 * 2026-07-15：本配置以 ESM 形式加载（.mts），不使用 require / __dirname。
 *   - 纯 ESM 依赖：pixelmatch@7 + pngjs@7（均为 ESM-only），用于视觉回归截图对比。
 *   - 视觉回归用法：spec 内 `cy.compareSnapshot('name', threshold?)`，
 *     首次运行写 baseline，之后每次比对并在不匹配时产出 diff 图。
 */
import { defineConfig } from 'cypress'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import { fileURLToPath, dirname } from 'node:url'
import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs'
import pixelmatch from 'pixelmatch'
import { PNG } from 'pngjs'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// 视觉回归产物目录（已 gitignore）
const VISUAL_DIR = path.resolve(__dirname, 'cypress', 'visual')

const IS_FULL = process.env.ENCV_TEST_FULL === '1'

// 快速组件测试（默认跑）— 简单展示组件 + 核心交互
const FAST_INCLUDE = [
  'cypress/component/_smoke.cy.ts',
  'cypress/component/shared/*.cy.ts',
  'cypress/component/useSearchInput.cy.ts',
  'cypress/component/user-reported-bugs.cy.ts',
  'cypress/component/TaskActionButtons.cy.ts',
  'cypress/component/TaskBasicInfo.cy.ts',
  'cypress/component/TaskOutputInfo.cy.ts',
  'cypress/component/TaskDebugPanel.cy.ts',
  'cypress/component/visual-appearance.cy.ts',
]

// 慢速集成测试（ENCV_TEST_FULL=1 才跑）— 大规模数据 + WS + 虚拟列表 + file watcher
const SLOW_INCLUDE = [
  'cypress/component/Files-file-change.cy.ts',
  'cypress/component/Tasks.cy.ts',
  'cypress/component/Tasks-1000-scale.cy.ts',
  'cypress/component/Tasks-ws-batch.cy.ts',
  'cypress/component/Tasks-search-filter.cy.ts',
  'cypress/component/Tasks-group-card-summary.cy.ts',
  'cypress/component/TaskTimeline.cy.ts',
  'cypress/component/TaskVirtualList.cy.ts',
  'cypress/component/GroupDetail.cy.ts',
  'cypress/component/GroupDetail-search-filter.cy.ts',
]

const COMPONENT_SPECS = process.env.ENCV_TEST_VISUAL === '1'
  ? ['cypress/component/visual-appearance.cy.ts']
  : IS_FULL
    ? [...FAST_INCLUDE, ...SLOW_INCLUDE]
    : FAST_INCLUDE

// 视觉回归 task：对比当前截图与 baseline
//   - 首次（baseline 不存在）：直接写入 baseline，返回 { matched: true, firstRun: true }
//   - 之后：pixelmatch 比对，mismatch > threshold 时写 diff 图并返回 matched:false
function registerVisualTask(on: Cypress.PluginEvents) {
  on('task', {
    compareSnapshot(
      opts: { name: string; threshold?: number },
    ): { matched: boolean; firstRun?: boolean; mismatchPixels?: number; mismatchPercent?: number; diffPath?: string } {
      const { name, threshold = 0.1 } = opts
      const candidatePath = path.resolve(__dirname, 'cypress', 'screenshots', `${name}.png`)
      const baselinePath = path.resolve(VISUAL_DIR, 'baseline', `${name}.png`)
      const diffPath = path.resolve(VISUAL_DIR, 'diff', `${name}.png`)

      if (!existsSync(candidatePath)) {
        throw new Error(`[compareSnapshot] 缺少候选截图: ${candidatePath}（请先 cy.screenshot 写入该路径）`)
      }

      if (!existsSync(baselinePath)) {
        mkdirSync(path.dirname(baselinePath), { recursive: true })
        writeFileSync(baselinePath, readFileSync(candidatePath))
        return { matched: true, firstRun: true }
      }

      const imgA = PNG.sync.read(readFileSync(baselinePath))
      const imgB = PNG.sync.read(readFileSync(candidatePath))
      if (imgA.width !== imgB.width || imgA.height !== imgB.height) {
        throw new Error(
          `[compareSnapshot] 尺寸不一致 baseline ${imgA.width}x${imgA.height} vs candidate ${imgB.width}x${imgB.height}`,
        )
      }
      const { width, height } = imgA
      const diff = new PNG({ width, height })
      const mismatchPixels = pixelmatch(imgA.data, imgB.data, diff.data, width, height, {
        threshold,
      })
      const total = width * height
      const mismatchPercent = (mismatchPixels / total) * 100
      const matched = mismatchPixels / total <= threshold

      if (!matched) {
        mkdirSync(path.dirname(diffPath), { recursive: true })
        writeFileSync(diffPath, PNG.sync.write(diff))
      }

      return {
        matched,
        mismatchPixels,
        mismatchPercent: Number(mismatchPercent.toFixed(2)),
        diffPath: matched ? undefined : diffPath,
      }
    },
  })
}

export default defineConfig({
  // 全局默认用 Electron（避免 Chrome 下载）
  defaultBrowser: 'electron',

  // ============ Component Testing ============
  component: {
    devServer: {
      framework: 'vue',
      bundler: 'vite',
      // 关键：用内联函数返回最小 vite 配置，完全不走项目 vite.config.ts
      //   - 避开 devStartGuard 拦截（项目 vite.config.ts 第一个 plugin）
      //   - 避开 dynamicHmrHostPlugin（不需要）
      //   - 避开 frontendDepsManifestPlugin（不需要）
      //   - 只需 @vitejs/plugin-vue（解析 .vue 文件）+ @ alias（spec import @/...）
      viteConfig: async () => ({
        plugins: [vue()],
        resolve: {
          alias: {
            '@': path.resolve(__dirname, 'src'),
          },
        },
        // cypress 内部 Vite 不需要 hmr/port
        server: { hmr: false },
        // i18n 严格模式：缺 key 直接抛错，避免测试用 class 选择器漏测 i18n
        define: {
          'import.meta.env.VITE_I18N_STRICT': JSON.stringify('true'),
        },
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
    baseUrl: process.env.CYPRESS_BASE_URL || 'http://localhost:5173',
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
  setupNodeEvents(on, config) {
    // 视觉回归 task（ESM：pixelmatch@7 + pngjs@7）
    registerVisualTask(on)

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
