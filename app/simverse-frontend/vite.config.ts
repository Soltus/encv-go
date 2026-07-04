import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      // 关键：simverse-frontend 自身的源码用 @self/
      '@self': path.resolve(__dirname, 'src'),
      // 从 encv-mobile 引用的源码中的 @/ 指向 encv-mobile/src
      '@': path.resolve(__dirname, '../encv-mobile/src'),
      // 共享组件包
      '@shared': path.resolve(__dirname, '../packages/shared-components/src'),
    },
  },
  server: {
    port: 8200,
    host: '0.0.0.0',
    allowedHosts: true,
  },
  build: {
    outDir: 'dist',
  },
})
