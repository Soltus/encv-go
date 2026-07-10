import path from 'node:path'
import fs from 'node:fs'
import type { Plugin } from 'vite'

// =============================================================================
// @/ 多路径 fallback 解析插件（encv-mobile / vitest 共用）
// =============================================================================
//
// 背景：Module G 去重后，encv-mobile/src 下与 shared 重复的 composables
// （useToast / useClipboard / useDateFormat / useSearchInput / relativeTime /
// activeStatus 等）已被删除，统一通过 `@/composables/useX` 回退到 shared。
//
// 本插件让 Vite/vitest 的运行时模块解析具备与 tsconfig `@/*` 二级回退一致的
// 能力：优先本地 src，其次 shared-components/src。这样删除本地副本后，
// `@/composables/useX` 在 dev/build/测试 三种环境都能解析到 shared。
//
// ⚠️ 不设 enforce（保持 normal 阶段），与 vite.config.ts 的 inline 版一致：
// vitest.config.ts 刻意【不设置 '@' 别名】，所以 `@/...` 在 vite:resolve 阶段
// 无法被别名解析、会落到普通插件阶段，由本插件（normal 阶段）拦截并做
// 本地优先 / shared 次之的回退。若设 enforce:'pre' 反而在个别 vitest 版本里
// 不被 transform 期的 this.resolve() 咨询到，导致静默失效。
//
// ⚠️ 关键坑（2026-07-11 实测）：本插件被 vitest 打包进 vitest.config.ts 时，
// esbuild 会把插件内的 `__dirname` 解析为【config 目录】(/workspace/app/encv-mobile)
// 而非插件自身目录 (/workspace/app/encv-mobile/vite-plugins)。若用
// `path.resolve(__dirname, '../src')` 会得到 /workspace/app/src（高一级），
// 导致所有文件存在性检查失败、插件静默返回 null、import 解析失败。
// 因此 roots 必须由调用方显式传入（调用处 __dirname 正确），插件内不再依赖
// 自身的 __dirname。
//
// 注意：仅拦截 `@/` 开头的 source；`@encv/shared-components/...` 等由各自的
// alias 处理，互不干扰。
export function encvAliasFallback(options?: { roots?: string[] }): Plugin {
  // 调用方必须传入 roots（见上方 __dirname 坑说明）。保留一个明显错误的默认值
  // 以在遗漏传参时快速暴露问题，而非静默解析失败。
  const dirs = options?.roots ?? [
    path.resolve(process.cwd(), 'encv-mobile/src'),
    path.resolve(process.cwd(), 'packages/shared-components/src'),
  ]
  return {
    name: 'encv-alias-fallback',
    resolveId(source) {
      if (source.startsWith('@/')) {
        const relativePath = source.slice(2)
        // 去掉 ?worker / ?raw / ?url 等查询参数
        const cleanPath = relativePath.split('?')[0]
        const query = relativePath.includes('?') ? '?' + relativePath.split('?')[1] : ''

        // dirs 由调用方（vitest.config.ts）显式传入，已基于正确的 __dirname 解析
        let found: string | null = null
        for (const dir of dirs) {
          const fullPath = path.join(dir, cleanPath)
          const candidates = [
            fullPath,
            fullPath + '.ts',
            fullPath + '.vue',
            fullPath + '.js',
            fullPath + '.tsx',
            fullPath + '.jsx',
            fullPath + '.mjs',
            fullPath + '.cjs',
            path.join(fullPath, 'index.ts'),
            path.join(fullPath, 'index.js'),
            path.join(fullPath, 'index.mjs'),
          ]
          for (const tryPath of candidates) {
            if (fs.existsSync(tryPath) && fs.statSync(tryPath).isFile()) {
              found = tryPath + query
              break
            }
          }
          if (found) break
        }
        if (found) return found
        // fallback：返回本地 src 路径，让 vite 自己处理错误（与 vite.config.ts 行为一致）
        return path.join(dirs[0], cleanPath) + query
      }
      return null
    },
  }
}
