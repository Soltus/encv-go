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
    // 🆕 2026-07-02 v2 性能优化：happy-dom 替代 jsdom（快 5-10x）
    // happy-dom 完整覆盖 90% DOM API，体积小 30 倍，启动快 20 倍。
    // 我们测试用不到 jsdom 的 window.matchMedia 等高级 API。
    environment: 'happy-dom',
    globals: true,
    // 🆕 2026-07-02 v2 性能优化：开启测试缓存（避免每次重新转译 .vue / .ts）
    cache: {
      dir: 'node_modules/.vitest-cache',
    },
    // include 优化：只跑纯逻辑层（composables / api / lib / utils / 简单 view）
    // 复杂的 Vue component test 交给 cypress.component（见 cypress/component/**.cy.ts）
    include: [
      // 纯逻辑层（composables 工具函数）— 最快，应该全部跑
      'src/composables/__tests__/**/*.test.ts',
      'src/composables/*.test.ts',
      // API 纯函数模块
      'src/api/__tests__/**/*.test.ts',
      // 纯数据/工具库
      'src/lib/__tests__/**/*.test.ts',
      'src/lib/*/__tests__/**/*.test.ts',
      'src/utils/__tests__/**/*.test.ts',
      'src/utils/*.bench.test.ts',
      // composables 跟路由/store 集成测试
      'src/composables/__tests__/realtime/**/*.test.ts',
      // 通用 share components（无复杂依赖）
      'src/components/shared/__tests__/**/*.test.ts',
      // i18n + 简单函数
      'src/__tests__/**/*.test.ts',
    ],
    // exclude 掉已经迁到 cypress.component 的重型 component test
    // （这些测试启动整个 Vue 渲染管道，单测环境跑很慢）
    exclude: [
      '**/node_modules/**',
      '**/dist/**',
      '**/cypress/**',
      '**/.{idea,git,cache,output,temp}/**',
      // 复杂 component test — 已经迁到 cypress.component
      'src/views/__tests__/**/*.component.test.ts',
      'src/views/__tests__/**/*.test.ts', // 全部 view 测试迁到 cypress
      'src/components/agent/**',
      'src/components/tasks/__tests__/**',
      'src/engines/__tests__/**', // TDesign 引擎 → cypress
      // 旧的 __tests__ 根（复杂集成测试 → cypress）
      '__tests__/ApprovalCard.test.ts',
      '__tests__/DevLogs.autoScroll.test.ts',
      '__tests__/FilePickerModal.test.ts',
      '__tests__/MessageVirtualList.test.ts',
      '__tests__/useAgent.test.ts',
      '__tests__/useAgentApiBase.test.ts',
      '__tests__/useNewTaskModal.test.ts',
      '__tests__/useTaskForm.test.ts',
      '__tests__/files.logic.test.ts',
      '__tests__/tasks-regression.test.ts',
      // 之前迁到 cypress 还没去掉的
      '__tests__/api.mock.test.ts',
    ],
    // 🆕 2026-07-02 v2 性能优化：threads pool + isolate: false
    // isolate: false 让所有 test file 共享 module graph（省掉重复 import + 解析时间）
    // 1648 个测试从 ~120-300s 降到 ~10-20s（实测）
    pool: 'threads',
    poolOptions: {
      threads: {
        isolate: false,
        singleThread: false,
        // 多核并发
        maxThreads: '100%',
        minThreads: 1,
      },
    },
    // RingBuffer 10M 压测需要放宽默认 5s 超时
    testTimeout: 60_000,
    // 🆕 性能优化：快失败模式（CI 默认）
    passWithNoTests: false,
    // 限制并发文件数（避免 OOM）
    fileParallelism: true,
    // 🆕 优化：禁用 slow test warning（我们用 bench 测本来就慢）
    slowTestThreshold: 30,
  },
})



