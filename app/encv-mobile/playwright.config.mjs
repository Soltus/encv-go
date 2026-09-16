/**
 * Playwright 配置（视觉回归用，替代 Cypress——本沙箱 Cypress 的 bundled Electron
 * 无法以 Electron 模式启动，Playwright 的 Chromium 可正常 headless 运行）
 *
 * 思路（与 cypress component 一致）：用最小 Vite 配置起独立挂载壳 test-visual/，
 * 挂 AppearanceDetail，Playwright 截图后用 pixelmatch@7 + pngjs@7（ESM）比对。
 */
import { defineConfig, devices } from '@playwright/test'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { dirname } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  testDir: './test-visual',
  testMatch: '**/*.visual.ts',
  snapshotDir: './cypress/visual/baseline',
  timeout: 60_000,
  fullyParallel: false,
  reporter: [['list']],
  use: {
    baseURL: 'http://localhost:5199',
    viewport: { width: 390, height: 844 },
    launchOptions: {
      args: ['--no-sandbox', '--disable-dev-shm-usage'],
    },
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command:
      'npx vite --config test-visual/vite.config.ts --port 5199 --strictPort',
    port: 5199,
    reuseExistingServer: true,
    timeout: 120_000,
    env: {
      // 视觉壳不强校验 i18n key（空 messages），关闭严格模式避免噪音
      VITE_I18N_STRICT: 'false',
    },
  },
})
