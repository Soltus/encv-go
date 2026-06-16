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
 * 关于 description/icon/license：
 *   - 硬编码到 LIB_META 表 (LLM 知识)；缺失时为空字符串 / unknown
 *   - 前端 useLibraries composable 走 npm/GitHub/Maven API fallback
 *   - 团队 commit 时可手改字段（git diff 可见）
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

/**
 * 库元信息表 (LLM 知识库)
 * 缺字段时 = 空 description / icon='help-circle' / license='unknown'
 */
const LIB_META: Record<
  string,
  { description: string; icon: string; license: string }
> = {
  // runtime deps
  '@ajuarezso/capacitor-high-refresh-rate': {
    description: 'Capacitor 插件：高刷新率屏幕支持',
    icon: 'speedometer',
    license: 'MIT',
  },
  '@capacitor/android': {
    description: 'Capacitor Android 平台 native 桥接',
    icon: 'logo-android',
    license: 'MIT',
  },
  '@capacitor/core': {
    description: 'Capacitor 核心运行时（Web ↔ Native 桥接）',
    icon: 'git-network',
    license: 'MIT',
  },
  '@capacitor/device': {
    description: 'Capacitor 设备信息插件（型号/OS/平台）',
    icon: 'phone-portrait',
    license: 'MIT',
  },
  '@capacitor/screen-orientation': {
    description: 'Capacitor 屏幕方向控制插件',
    icon: 'phone-portrait-outline',
    license: 'MIT',
  },
  '@capacitor/share': {
    description: 'Capacitor 系统分享面板插件',
    icon: 'share-social',
    license: 'MIT',
  },
  '@capacitor/status-bar': {
    description: 'Capacitor 状态栏样式控制插件',
    icon: 'phone-portrait',
    license: 'MIT',
  },
  '@ionic/vue': {
    description: 'Ionic Vue UI 组件库（移动端 Web）',
    icon: 'logo-ionic',
    license: 'MIT',
  },
  '@ionic/vue-router': {
    description: 'Ionic Vue 路由集成（适配 ion-router-outlet）',
    icon: 'git-network',
    license: 'MIT',
  },
  '@tanstack/vue-virtual': {
    description: 'TanStack Virtual — 大列表虚拟滚动',
    icon: 'list',
    license: 'MIT',
  },
  '@tdesign-vue-next/chat': {
    description: 'TDesign Vue 聊天组件（AI 消息渲染）',
    icon: 'chatbubbles',
    license: 'MIT',
  },
  artplayer: {
    description: 'ArtPlayer — 现代 HTML5 视频播放器',
    icon: 'play-circle',
    license: 'MIT',
  },
  ionicons: {
    description: 'Ionicons — Ionic 官方图标库',
    icon: 'image',
    license: 'MIT',
  },
  markstreamvue: {
    description: 'MarkStream — Markdown 流式渲染（AI 输出）',
    icon: 'document-text',
    license: 'MIT',
  },
  vconsole: {
    description: 'vConsole — 移动端 Web 调试控制台',
    icon: 'terminal',
    license: 'MIT',
  },
  vue: {
    description: 'Vue.js — 渐进式 JavaScript 框架',
    icon: 'logo-vue',
    license: 'MIT',
  },
  'vue-router': {
    description: 'Vue.js 官方路由管理器',
    icon: 'git-network',
    license: 'MIT',
  },
  'vue-virtual-scroller': {
    description: 'Vue Virtual Scroller — 虚拟滚动列表',
    icon: 'list',
    license: 'MIT',
  },
  // devDeps
  '@capacitor/cli': {
    description: 'Capacitor CLI — 平台 sync/build 命令行',
    icon: 'terminal',
    license: 'MIT',
  },
  '@vitejs/plugin-vue': {
    description: 'Vite Vue SFC 编译插件',
    icon: 'logo-vue',
    license: 'MIT',
  },
  '@vitest/coverage-v8': {
    description: 'Vitest V8 覆盖率报告',
    icon: 'analytics',
    license: 'MIT',
  },
  '@vue/test-utils': {
    description: 'Vue Test Utils — Vue 组件测试工具',
    icon: 'flask',
    license: 'MIT',
  },
  jsdom: {
    description: 'jsdom — 浏览器 DOM 的 Node 实现',
    icon: 'globe',
    license: 'MIT',
  },
  sirv: {
    description: 'sirv — 静态文件服务器（Vite preview 用）',
    icon: 'server',
    license: 'MIT',
  },
  typescript: {
    description: 'TypeScript — JavaScript 类型化超集',
    icon: 'code-slash',
    license: 'Apache-2.0',
  },
  vite: {
    description: 'Vite — 下一代前端构建工具',
    icon: 'flash',
    license: 'MIT',
  },
  vitest: {
    description: 'Vitest — Vite 原生单元测试框架',
    icon: 'flask',
    license: 'MIT',
  },
  'vue-tsc': {
    description: 'vue-tsc — Vue + TypeScript 类型检查 CLI',
    icon: 'checkmark-circle',
    license: 'MIT',
  },
}

const UNKNOWN_META = {
  description: '',
  icon: 'help-circle',
  license: 'unknown',
} as const

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
      icon: string
      license: string
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
      const meta = LIB_META[name] ?? UNKNOWN_META
      out.items.push({
        name,
        version: String(range).replace(/^[\^~]/, ''),
        version_range: String(range),
        source: 'package.json',
        kind,
        importance: CORE_LIBS.has(name) ? 'core' : 'light',
        description: meta.description,
        icon: meta.icon,
        license: meta.license,
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
