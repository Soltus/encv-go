import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const resolve = (p: string) => path.resolve(__dirname, p)

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@self': resolve('src'),
      '@': resolve('../encv-mobile/src'),
      '@shared': resolve('../packages/shared-components/src'),
    },
  },
  server: {
    port: 8200,
    host: '0.0.0.0',
    strictPort: true,
    cors: true,
    allowedHosts: true,
  },
  build: {
    outDir: 'dist',
  },
})
