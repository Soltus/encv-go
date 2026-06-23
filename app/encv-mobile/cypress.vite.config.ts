/**
 * Cypress Component Testing 专用 Vite 配置
 *
 * 区别于主 vite.config.ts：
 *   - 不含 devStartGuard（cypress 是测试场景，不是 dev 启动）
 *   - 不含 frontendDepsManifestPlugin（cypress 不需要前端依赖清单）
 *   - 不含 dynamicHmrHostPlugin（cypress 跑在 cypress 自己的 vite，无 HMR host 修复需求）
 *   - server.hmr=false（避免 Electron 触发 WS 噪音）
 *
 * ⚠️ Vue template 编译器说明：
 *   - cypress 15 + Vite 默认用 vue.runtime.esm-bundler.js（runtime-only，不含 template compiler）
 *   - defineComponent({ template: '...' }) 静默失败（无编译错误，渲染为 <!---->）
 *   - 修法：spec 文件 import 时直接走 'vue/dist/vue.esm-bundler.js'
 *     见 cypress/support/component.ts 统一 re-export
 *   - 不在 vite alias 里设（cypress devServer 不一定读用户 vite config 的 alias）
 *
 * 保留：
 *   - @vitejs/plugin-vue（必须，让 cypress 能编译 .vue SFC）
 *   - '@' alias（cypress 解析项目路径用，**用绝对路径**避免 cypress 加载 vite config 时的 cwd 不确定）
 *
 * ⚠️ cypress 用 require.resolve 加载此 config（走 CJS），不能用 import.meta.url
 *   - 改用 process.cwd() 兜底，但 cypress 调用时 cwd 是项目根
 *   - 加 sanity check：path 必须存在，不存在就报错
 */
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import fs from 'node:fs'

// 多种方式找项目根（兼容 cypress require.resolve 加载场景）
function findProjectRoot(): string {
  // 1) process.cwd()（cypress 默认）
  const cwd = process.cwd()
  if (fs.existsSync(path.join(cwd, 'src', 'components', 'tasks', 'TaskVirtualList.vue'))) {
    return cwd
  }
  // 2) 从 __dirname 推导（cypress 配置可能在 cypress.config.ts 同目录）
  let dir = __dirname
  for (let i = 0; i < 5; i++) {
    if (fs.existsSync(path.join(dir, 'src', 'components', 'tasks', 'TaskVirtualList.vue'))) {
      return dir
    }
    dir = path.dirname(dir)
  }
  throw new Error(`[cypress.vite.config] 无法定位项目根（src/components/tasks/TaskVirtualList.vue），cwd=${cwd} __dirname=${__dirname}`)
}

const projectRoot = findProjectRoot()

export default defineConfig({
  plugins: [vue()],
  server: {
    hmr: false,
  },
  resolve: {
    alias: {
      '@': path.resolve(projectRoot, 'src'),
    },
  },
})
