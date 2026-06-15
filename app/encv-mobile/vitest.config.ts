import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'path'

// vitest 4.x: use test.alias (not resolve.alias) for tsconfig paths to be
// resolved correctly via vite's resolver. resolve.alias is silently ignored
// in some vitest 4 + vite 8 combinations, leading to "Cannot find package
// '@/...'" errors even though vue-tsc compiles fine.
const SRC_DIR = path.resolve(__dirname, './src')
const TDESIGN_STUB = path.resolve(__dirname, './src/engines/__tests__/__mocks__/tdesign-chat.mjs')

export default defineConfig({
  plugins: [vue()],
  // 强制 vitest 把 encv-mobile 视为项目根，避免 pnpm monorepo 自动发现
  // 把 /workspace 当 root 后找不到本目录的 vitest.config.ts。
  root: __dirname,
  resolve: {
    alias: {
      '@': SRC_DIR,
      // TDesign chat 在 pnpm 严格模式下，@tdesign-vue-next/chat 的 module 字段
      // 走不通 Node.js 原生 resolver。测试环境用 stub 替代（保留类型信息）。
      '@tdesign-vue-next/chat': TDESIGN_STUB,
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: [
      '__tests__/**/*.test.ts',
      'src/__tests__/**/*.test.ts',
      'src/composables/**/*.test.ts',
      'src/engines/**/*.test.ts',
      'src/views/**/*.test.ts',
      // 🆕 2026-06-10：src/api 下的单测（getApiBaseUrl 等纯 API 测试）
      'src/api/**/*.test.ts',
      // 🆕 2026-06-10：src/lib 下的单测（mockDataGenerator、dev-start-guard 等纯函数模块）
      'src/lib/**/*.test.ts',
      // 🆕 2026-06-15：src/utils 下的单测（RingBuffer / IncrementalFilter 等纯数据
      // 结构 + 压力测试。1M push 实测约 1-2s，能在 60s 超时内跑完）
      'src/utils/**/*.test.ts',
    ],
    // RingBuffer 10M 压测需要放宽默认 5s 超时
    testTimeout: 60_000,
  },
})
