/**
 * 视觉回归挂载壳的最小 Vite 配置（绕过项目根 vite.config.ts 的 devStartGuard 等 plugin）
 */
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { dirname } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))

export default {
  root: __dirname,
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '..', 'src'),
      '@encv/shared-components': path.resolve(__dirname, '..', '..', 'packages', 'shared-components', 'src'),
      '@encv/shared-components/': path.resolve(__dirname, '..', '..', 'packages', 'shared-components', 'src') + '/',
    },
  },
  server: { hmr: false },
}
