import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Plugin } from 'vite'

/**
 * 2026-06-17：构建时生成 frontend-deps.json manifest
 *
 * 数据源：package.json (dependencies + devDependencies)
 * 产物：src/generated/frontend-deps.json (纳入 git 跟踪)
 *
 * 关于 importance 分类：
 *   - core: 必备框架
 *   - light: 业务可选
 *   - transitive: 不直接 import (前端 npm 通常不暴露此项)
 *
 * 为什么用 LLM 知识硬编码 importance 规则（不读代码）：
 *   - 这是 LLM 驱动的 manifest 生成，importance 体现"是否本项目代码直接 import"
 *   - 团队 commit 时可手改 importance 字段（git diff 可见）
 *   - 写完跑一次后，下游 useLibraries composable 只读不重写
 */
const CORE_LIBS = new Set([
  'vue',
  'vue-router',
  '@ionic/vue',
  '@ionic/vue-router',
  'ionicons',
  '@capacitor/core',
  '@capacitor/android',
  '@capacitor/cli',
  'vite',
  '@vitejs/plugin-vue',
  'vue-tsc',
  'typescript',
  'vitest',
])

export function frontendDepsManifestPlugin(): Plugin {
  const __dirname = dirname(fileURLToPath(import.meta.url))
  const root = resolve(__dirname, '..')
  return {
    name: 'frontend-deps-manifest',
    enforce: 'pre',
    buildStart() {
      regenerate(root)
    },
    configureServer(server) {
      // dev 启动时也跑一次，HMR 即可生效
      regenerate(root)
    },
    handleHotUpdate({ file }) {
      if (file.endsWith('package.json')) {
        regenerate(root)
      }
    },
  }
}

function regenerate(root: string) {
  const pkgPath = resolve(root, 'package.json')
  if (!existsSync(pkgPath)) return
  let pkg: any
  try {
    pkg = JSON.parse(readFileSync(pkgPath, 'utf-8'))
  } catch (e) {
    console.warn('[frontend-deps-manifest] failed to parse package.json:', e)
    return
  }

  const out: {
    schema_version: number
    generated_at: string
    source_file: string
    items: Array<{
      name: string
      version: string
      version_range: string
      source: string
      kind: string
      importance: string
      description: string
    }>
  } = {
    schema_version: 1,
    generated_at: new Date().toISOString(),
    source_file: 'package.json',
    items: [],
  }

  function pushAll(section: 'dependencies' | 'devDependencies', kind: string) {
    const obj = pkg[section] || {}
    for (const [name, range] of Object.entries(obj)) {
      out.items.push({
        name,
        version: String(range).replace(/^[\^~]/, ''),
        version_range: String(range),
        source: 'package.json',
        kind,
        importance: CORE_LIBS.has(name) ? 'core' : 'light',
        description: '',
      })
    }
  }

  pushAll('dependencies', 'dependency')
  pushAll('devDependencies', 'devDependency')

  const outPath = resolve(root, 'src/generated/frontend-deps.json')
  try {
    if (!existsSync(dirname(outPath))) {
      mkdirSync(dirname(outPath), { recursive: true })
    }
    writeFileSync(outPath, JSON.stringify(out, null, 2) + '\n', 'utf-8')
  } catch (e) {
    console.warn('[frontend-deps-manifest] failed to write manifest:', e)
  }
}
