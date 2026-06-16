/**
 * 2026-06-17：useLibraries composable
 *
 * 数据源合并：
 *   1. /api/libraries (Go 后端，runtime/debug.ReadBuildInfo 拿 Go deps)
 *   2. 静态 src/generated/frontend-deps.json (vite plugin build 时生成)
 *   3. getAndroidDeps() native bridge (Android assets/android-deps.json)
 *
 * 状态计算（useLibraries 自身）:
 *   - active: items 数组里有 + version 非空 + source 非空
 *   - broken: items 里有但 version 或 source 为空
 *   - historical: items 里有但 source 全部为空（manifest 残留但 source 不可达）
 *
 * 重要性 (importance):
 *   - core / light / transitive — 直接从 manifest 读
 *
 * 描述 fallback:
 *   - description 显式有 → 直接显示
 *   - description 为空 → 尝试 npm / GitHub / Maven Central API 解析
 *   - 全部失败 → "无描述" 占位符
 *   - 结果缓存到 localStorage["encv_lib_desc_cache_v1"]，TTL 7 天
 */
import { ref, computed } from 'vue'
import frontendDeps from '@/generated/frontend-deps.json'
import { getAndroidDeps, isNative } from '@/plugins/GoProcess'

export type LibSource =
  | 'package.json'
  | 'libs.versions.toml'
  | 'build.gradle.kts'
  | 'go.mod'
  | 'runtime.Version()'
  | 'unknown'

export type LibStatus = 'active' | 'broken' | 'historical'
export type LibImportance = 'core' | 'light' | 'transitive'

export interface LibraryItem {
  name: string
  version: string
  versionRange?: string
  source: LibSource
  kind: 'dependency' | 'devDependency' | 'transitive' | 'plugin' | 'test' | 'androidTest' | 'runtime'
  importance: LibImportance
  status: LibStatus
  description: string
  descriptionFallback?: string
  descriptionStatus: 'explicit' | 'fetched' | 'placeholder' | 'fetching'
}

const backendItems = ref<LibraryItem[]>([])
const androidItems = ref<LibraryItem[]>([])
const loading = ref<boolean>(false)
const loaded = ref<boolean>(false)
const error = ref<string | null>(null)

// 描述缓存：name -> { desc, fetchedAt, ttl }
interface CacheEntry {
  desc: string
  status: 'fetched' | 'placeholder'
  fetchedAt: number
}
const DESC_CACHE_KEY = 'encv_lib_desc_cache_v1'
const DESC_CACHE_TTL_MS = 7 * 24 * 60 * 60 * 1000

function loadDescCache(): Record<string, CacheEntry> {
  try {
    const raw = localStorage.getItem(DESC_CACHE_KEY)
    if (!raw) return {}
    const obj = JSON.parse(raw)
    return typeof obj === 'object' && obj ? obj : {}
  } catch {
    return {}
  }
}

function saveDescCache(cache: Record<string, CacheEntry>) {
  try {
    localStorage.setItem(DESC_CACHE_KEY, JSON.stringify(cache))
  } catch {
    // localStorage 容量满 — 静默忽略，下次再试
  }
}

function readCachedDesc(name: string): CacheEntry | null {
  const cache = loadDescCache()
  const entry = cache[name]
  if (!entry) return null
  if (Date.now() - entry.fetchedAt > DESC_CACHE_TTL_MS) return null
  return entry
}

function writeCachedDesc(name: string, entry: CacheEntry) {
  const cache = loadDescCache()
  cache[name] = entry
  saveDescCache(cache)
}

// 描述 fallback 解析（npm → GitHub → Maven Central）
async function fetchFallbackDescription(name: string, version: string): Promise<string | null> {
  // 1. npm (适用于 frontend libs)
  if (!name.includes(':') && !name.startsWith('@')) {
    try {
      const r = await fetch(`https://registry.npmjs.org/${encodeURIComponent(name)}/${version}`)
      if (r.ok) {
        const data = await r.json()
        if (data.description) return data.description as string
      }
    } catch {
      // npm 失败时继续尝试 GitHub
    }
    // 2. GitHub via npm homepage
    try {
      const r = await fetch(`https://registry.npmjs.org/${encodeURIComponent(name)}/${version}`)
      if (r.ok) {
        const data = await r.json()
        const repo = (data.repository && (data.repository.url || data.repository)) as string | undefined
        if (repo) {
          const m = String(repo).match(/github\.com[/:]([\w.-]+)\/([\w.-]+?)(?:\.git)?$/i)
          if (m) {
            const gr = await fetch(`https://api.github.com/repos/${m[1]}/${m[2]}`)
            if (gr.ok) {
              const gd = await gr.json()
              if (gd.description) return gd.description as string
            }
          }
        }
      }
    } catch {
      // ignore
    }
  }

  // 3. Maven Central（Android deps: group:artifact:version）
  if (name.includes(':')) {
    const [group, artifact] = name.split(':')
    if (group && artifact) {
      try {
        const r = await fetch(
          `https://search.maven.org/solrsearch/select?q=g:${encodeURIComponent(group)}+AND+a:${encodeURIComponent(artifact)}&rows=1&wt=json`,
        )
        if (r.ok) {
          const data = await r.json()
          if (data.response && data.response.docs && data.response.docs[0]) {
            const desc = data.response.docs[0].descr || data.response.docs[0].description
            if (desc) return String(desc)
          }
        }
      } catch {
        // ignore
      }
    }
  }

  return null
}

// 计算 status 字段
// 保留 _name 参数是预留给未来加入 cross-source 验证（manifest 之间版本不一致时标 broken）
function computeStatus(_name: string, version: string, source: string): LibStatus {
  if (!version || version === '(unknown)' || version === 'managed-by-bom') {
    // managed-by-bom 也是合法 version（被 BOM 管），不算 broken
    if (version === 'managed-by-bom') return 'active'
    return 'broken'
  }
  if (!source || source === 'unknown') return 'historical'
  return 'active'
}

function normalizeSource(s: string | undefined): LibSource {
  if (!s) return 'unknown'
  if (s === 'package.json') return 'package.json'
  if (s === 'libs.versions.toml') return 'libs.versions.toml'
  if (s === 'build.gradle.kts') return 'build.gradle.kts'
  if (s === 'go.mod') return 'go.mod'
  if (s === 'runtime.Version()') return 'runtime.Version()'
  return 'unknown'
}

function buildFromFrontend(): LibraryItem[] {
  const items: LibraryItem[] = []
  for (const raw of (frontendDeps as any).items as Array<any>) {
    const source = normalizeSource(raw.source)
    const importance = (raw.importance as LibImportance) || 'light'
    const item: LibraryItem = {
      name: String(raw.name),
      version: String(raw.version || ''),
      versionRange: raw.version_range,
      source,
      kind: (raw.kind as LibraryItem['kind']) || 'dependency',
      importance,
      status: computeStatus(raw.name, raw.version, source),
      description: String(raw.description || ''),
      descriptionStatus: raw.description ? 'explicit' : 'placeholder',
    }
    items.push(item)
  }
  return items
}

function buildFromBackend(raws: Array<any>): LibraryItem[] {
  return raws.map((r) => {
    const source = normalizeSource(r.source)
    const importance = (r.importance as LibImportance) || 'light'
    return {
      name: String(r.name),
      version: String(r.version || ''),
      versionRange: r.version_range,
      source,
      kind: (r.kind as LibraryItem['kind']) || 'dependency',
      importance,
      status: computeStatus(r.name, r.version, source),
      description: String(r.description || ''),
      descriptionStatus: r.description ? 'explicit' : 'placeholder',
    } as LibraryItem
  })
}

async function load(): Promise<void> {
  if (loading.value || loaded.value) return
  loading.value = true
  error.value = null
  try {
    // Android deps (native only)
    if (isNative()) {
      const android = await getAndroidDeps()
      if (android && Array.isArray(android.items)) {
        androidItems.value = buildFromBackend(android.items as any)
      }
    }

    // Backend (Go) deps — 通过 /api/libraries
    try {
      const params = new URLSearchParams()
      if (androidItems.value.length > 0) {
        params.set(
          'android_manifest',
          JSON.stringify({
            schema_version: 1,
            items: androidItems.value.map((a) => ({
              name: a.name,
              version: a.version,
              version_range: a.versionRange,
              source: a.source,
              kind: a.kind,
              importance: a.importance,
              description: a.description,
            })),
          }),
        )
      }
      const url = `/api/libraries${params.toString() ? '?' + params.toString() : ''}`
      const r = await fetch(url)
      if (r.ok) {
        const data = await r.json()
        if (data && Array.isArray(data.items)) {
          backendItems.value = buildFromBackend(data.items as any)
        }
      } else {
        error.value = `GET /api/libraries ${r.status}`
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }

    loaded.value = true
  } finally {
    loading.value = false
  }
}

export function useLibraries() {
  // 前端 deps（静态 import）
  const frontendItems = computed<LibraryItem[]>(() => buildFromFrontend())

  // 合并 backend + android
  // Android items 也来自 backend（被推送过去），但 web 模式 / 推送失败时单独显示
  const allItems = computed<LibraryItem[]>(() => {
    const merged = new Map<string, LibraryItem>()
    // 顺序：frontend → android → backend（后到的覆盖前面的 source 字段）
    for (const it of frontendItems.value) merged.set(it.name, it)
    for (const it of androidItems.value) merged.set(it.name, it)
    for (const it of backendItems.value) merged.set(it.name, it)
    return Array.from(merged.values())
  })

  const itemsByCategory = computed<{
    android: LibraryItem[]
    frontend: LibraryItem[]
    backend: LibraryItem[]
  }>(() => {
    const android: LibraryItem[] = []
    const frontend: LibraryItem[] = []
    const backend: LibraryItem[] = []
    for (const it of allItems.value) {
      if (
        it.source === 'libs.versions.toml' ||
        it.source === 'build.gradle.kts' ||
        it.name.startsWith('androidx.') ||
        it.name.startsWith('com.google.') ||
        it.name.startsWith('io.github.') ||
        it.name.startsWith('com.android.') ||
        it.name.startsWith('org.jetbrains.') ||
        it.name.startsWith('com.squareup.') ||
        it.name.startsWith('com.tencent.') ||
        it.name.startsWith('io.insert-koin')
      ) {
        android.push(it)
      } else if (it.source === 'package.json' || it.source === 'go.mod' || it.source === 'runtime.Version()') {
        if (it.source === 'package.json') {
          frontend.push(it)
        } else {
          backend.push(it)
        }
      } else if (it.kind === 'plugin' || it.kind === 'androidTest') {
        android.push(it)
      } else if (it.name.includes('/')) {
        // Go path (e.g. github.com/gin-gonic/gin) — 后端库
        backend.push(it)
      } else {
        frontend.push(it)
      }
    }
    // 排序：core 优先，然后按名字
    function sortFn(a: LibraryItem, b: LibraryItem): number {
      const ai = a.importance === 'core' ? 0 : a.importance === 'light' ? 1 : 2
      const bi = b.importance === 'core' ? 0 : b.importance === 'light' ? 1 : 2
      if (ai !== bi) return ai - bi
      return a.name.localeCompare(b.name)
    }
    return {
      android: android.sort(sortFn),
      frontend: frontend.sort(sortFn),
      backend: backend.sort(sortFn),
    }
  })

  /**
   * 解析 description：先查缓存，命中即用；未命中走 fallback API
   */
  async function resolveDescription(item: LibraryItem): Promise<string> {
    if (item.description) return item.description

    // 缓存命中
    const cached = readCachedDesc(item.name)
    if (cached) {
      return cached.desc
    }

    // 标记为 fetching
    item.descriptionStatus = 'fetching'

    // 走 fallback
    const desc = await fetchFallbackDescription(item.name, item.version)
    if (desc) {
      writeCachedDesc(item.name, { desc, status: 'fetched', fetchedAt: Date.now() })
      item.descriptionFallback = desc
      item.descriptionStatus = 'fetched'
      return desc
    } else {
      writeCachedDesc(item.name, { desc: '', status: 'placeholder', fetchedAt: Date.now() })
      item.descriptionStatus = 'placeholder'
      return ''
    }
  }

  return {
    androidItems: itemsByCategory.value.android,
    frontendItems: itemsByCategory.value.frontend,
    backendItems: itemsByCategory.value.backend,
    allItems: allItems.value,
    loading: computed(() => loading.value),
    loaded: computed(() => loaded.value),
    error: computed(() => error.value),
    load,
    resolveDescription,
  }
}
