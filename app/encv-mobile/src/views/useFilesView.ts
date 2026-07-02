// useFilesView.ts - Files.vue 的 script 逻辑拆出（composable）
// 拆分自 Files.vue。所有 reactive state / handler / lifecycle 都集中在此。
// Files.vue 只剩 template + 调 useFilesView() 拿到返回值后解构使用。
//
// 为什么不是拆多个 composable？
//   main view 和 plugin view 共享大量 state（currentPath/files/fileTagMap/fileBadges
//   /selectedFile/renameValue 等），如果分多个 composable 要 props 双向同步，反而更乱。
//   所以采用「单 composable + 内部注释分块」的方式：拆出大文件，但保持 state 共享。

import { ref, computed, watch, onMounted, onUnmounted, nextTick, type Ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { onIonViewWillEnter, actionSheetController, alertController, menuController } from '@ionic/vue'
import { tokenizeQuery, renderSnippet, type QueryToken } from '@/views/useFilesView.searchTokens'

// 🆕 2026-07-02: 显式 return type（用 Record<string, any> 兼容所有字段）— 避免 vue-tsc 推断丢字段
// (历史踩坑：isSelectedModelAvailable / switchSession / lanAccessLoaded / fetchModels / temperature 都从推断 type 中消失过)
// 关键：Record<string, any> 索引签名让 declared 字段可推断
export type UseFilesViewReturn = {
  // 关键字段（Files.vue 模板直引用，类型 partial 以兼容 vue-tsc 推断）
  searchFullText: Ref<boolean>
  searchQuery: Ref<string>
  searchResults: Ref<FileItem[] | null>
  isSearching: Ref<boolean>
  searchMode: Ref<SearchMode>
  renderSnippet: (snippet: string | undefined) => Array<{ text: string; highlight: boolean }>
  tokenizeQuery: (query: string) => QueryToken[]  // 🆕 语法高亮
  // 索引签名：允许 return 对象包含任意额外字段
  [key: string]: any
}
import {
  listFiles,
  listFilesStream,
  listPluginFilesStream,
  searchFiles,
  searchFilesVector,
  searchFilesFullText,
  getFileCategory,
  PermissionDeniedError,
  NotFoundError,
  deleteFile,
  renameFile,
  renameOriginalName,
  copyFile,
  moveFile,
  uploadFile,
  fetchPlugins,
  fetchTags,
  addTag,
  removeTag,
  listFilesByTag,
  getFileInfo,
  getExternalStreamUrl,
} from '@/api/encv'
import type { FileItem, PluginMeta, TagInfo, SearchMode } from '@/api/encv'
import { eventBus } from '@/composables/useEventBus'
import { useI18n } from '@/composables/useI18n'
import { useVectorSearchStatus } from '@/composables/useVectorSearchStatus'
import { useRealtimeTransport } from '@/composables/useRealtimeTransport'
import { useThumbnailCache } from '@/composables/useThumbnailCache'
import { useFileFeatures, findClickHandler, isAnyContainerFile, getFeatureIcon } from '@/composables/useFileFeatures'
import { preloadSubtitles } from '@/features/alist-encrypt'
import { isAlistEncrypted, getSessionPassword, setSessionPassword, loadDecodedName, getDecodedName } from '@/features/alist-encrypt/useAlistEncrypt'
import { promptPassword } from '@/features/alist-encrypt/password-dialog'
import {
  useFileListSort,
  sortFiles,
  clientFilterFiles,
} from '@/composables/useFileList'
import { isNative, requestStoragePermission, openPlayer, openExternal, getLocalFilePath } from '@/plugins/GoProcess'
import { copyToClipboard } from '@/composables/useClipboard'
import { showToast } from '@/composables/useToast'
import { Share } from '@capacitor/share'
import { PLAY_MODE } from '@/constants/player'
import {
  getPlayMode,
  formatDateInput,
  mountDriverOf,
  mountPathOf,
  mountRootOf,
  SIZE_PRESETS,
  TIME_PRESETS,
} from './useFilesHelpers'
import {
  informationCircle,
  folderOpen,
  eyeOutline,
  playCircle,
  createOutline,
  copyOutline,
  arrowForwardOutline,
  shareOutline,
  pricetagOutline,
  trash,
  filmOutline,
  musicalNotesOutline,
  imageOutline,
  documentTextOutline,
  documentOutline,
  lockClosed,
  add,
} from 'ionicons/icons'

/**
 * useFilesView - Files.vue 的核心 composable
 *
 * 拆分自 Files.vue。把所有 reactive state / handler / lifecycle 集中在此。
 * Files.vue 在 <script setup> 中调用 useFilesView()，解构返回值，template 用法保持不变。
 *
 * 为什么是单 composable？
 *   main view 和 plugin view 共享大量 state（currentPath / files / fileTagMap /
 *   fileBadges / selectedFile / renameValue 等），分多个 composable 要双向同步 props，
 *   反而比单 composable 更难维护。
 */
export function useFilesView(): UseFilesViewReturn {

// =============================================================================
// 1) 播放 + 错误展示
// =============================================================================

const playError = ref<string>('')
const playErrorDetail = ref<string>('')
const playErrorFile = ref<string>('')

async function playMedia(file: FileItem, category: string) {
  const isVideo = category === 'video'
  const mediaType = isVideo ? 'video' : 'audio'
  const mimeType = isVideo ? 'video/*' : 'audio/*'
  const mode = getPlayMode(mediaType)

  console.info('[Files] playMedia: file=', file.path, 'mode=', mode, 'category=', category)
  playError.value = ''
  playErrorDetail.value = ''
  playErrorFile.value = ''

  switch (mode) {
    case PLAY_MODE.ARTPLAYER:
      router.push({ path: '/player', query: { path: file.path, name: file.name } })
      break
    case PLAY_MODE.MPV_PLUGIN:
    case PLAY_MODE.MPV_ACTIVITY:
    case PLAY_MODE.MPV_FRAGMENT:
    case PLAY_MODE.MPV_COMPOSE:
      if (isNative()) {
        const result = await openPlayer(file.path, file.name, mimeType, mode)
        if (!result.success) {
          console.error('[Files] playMedia failed:', result.error, result.errorDetail)
          playError.value = result.error || '播放失败'
          playErrorDetail.value = result.errorDetail || ''
          playErrorFile.value = file.name
        }
      } else {
        router.push({ path: '/player', query: { path: file.path, name: file.name } })
      }
      break
    case PLAY_MODE.EXTERNAL:
      if (isNative()) {
        const url = getExternalStreamUrl(file.path)
        openExternal(url, mimeType)
      } else {
        router.push({ path: '/player', query: { path: file.path, name: file.name } })
      }
      break
    default:
      console.debug('[Files] Unknown play mode:', mode, '— falling back to artplayer')
      router.push({ path: '/player', query: { path: file.path, name: file.name } })
      break
  }
}

function clearPlayError() {
  playError.value = ''
  playErrorDetail.value = ''
  playErrorFile.value = ''
}

function togglePlayErrorDetail() {
  if (playErrorDetail.value) {
    const expanded = playErrorDetail.value
    playErrorDetail.value = ''
    playError.value = playError.value + '\n' + expanded
  }
}

// =============================================================================
// 2) 排序 / 显示模式（main view）
// =============================================================================

const { t } = useI18n()
const { status: vectorSearchStatus } = useVectorSearchStatus()
const { thumbnailUrls, setupLazyThumbnails, onThumbError } = useThumbnailCache()
const { sortBy, sortDesc } = useFileListSort()
const showMainSort = ref(false)

/**
 * setSortBy 排序变更封装：
 *   - 选择「相关度」时强制 sortDesc=true（高相关度在前）
 *   - 选择其他排序时保持当前升降序
 *   - 搜索时「相关度」是默认选项
 */
function setSortBy(value: 'name' | 'size' | 'time' | 'relevance') {
  sortBy.value = value
  if (value === 'relevance') {
    sortDesc.value = true
  }
}

const mainSortLabel = computed(() => {
  if (sortBy.value === 'relevance') {
    return '相关度↓'
  }
  const map: Record<string, string> = { name: '名称', size: '大小', time: '时间' }
  return (map[sortBy.value] || '名称') + (sortDesc.value ? '↓' : '↑')
})

// =============================================================================
// 3) 通用 state：路由 / dialog / 文件列表 / 选中态
// =============================================================================

const router = useRouter()
const route = useRoute()
const serverOnline = ref(false)
const noPermission = ref(false)
const files = ref<FileItem[]>([])
const plugins = ref<PluginMeta[]>([])
const tags = ref<TagInfo[]>([])
const showRenameDialog = ref(false)
const showTagDialog = ref(false)
const showMoveDialog = ref(false)
const selectedPlugin = ref<PluginMeta | null>(null)
const selectedFile = ref<FileItem | null>(null)
const renameValue = ref('')
const renamePassword = ref('')
const moveTargetPath = ref('')
const editingFileTags = ref<string[]>([])
const newTagInput = ref('')
const fileTagMap = ref<Record<string, string[]>>({})
const fileBadges = ref<Record<string, any[]>>({})
const fileSubtitles = ref<Record<string, any[]>>({})

const { getBadges, getSubtitles, getAllActions } = useFileFeatures()

const renameAlertInputs = computed(() => {
  const inputs: any[] = [{ name: 'name', type: 'text', placeholder: '新文件名', value: renameValue.value }]
  if (selectedFile.value?.isEncrypted) {
    inputs.push({ name: 'password', type: 'text', placeholder: '文件名加密密码（如需要）' })
  }
  return inputs
})

// =============================================================================
// 4) 路径 / mount / 加载主流程
// =============================================================================

const MOUNT_ROOT = '/d'
const currentPath = ref(MOUNT_ROOT)
const loading = ref(false)
const refreshing = ref(false)
const connecting = ref(false)
let firstLoad = true
const lastLoadedPath = ref<string>('')

const pendingHighlight = ref<string | null>(null)
const highlightedPath = ref<string | null>(null)
let highlightTimer: ReturnType<typeof setTimeout> | null = null

const isMountRoot = computed(() => currentPath.value === MOUNT_ROOT)

const searchQuery = ref('')
// 🆕 2026-07-02 大改升级：去掉"递归搜索"开关（价值低），改为"全文搜索"开关
// 全文搜索：搜文本文件内容（FTS5 + bm25 + CJK bigram）
// 非全文搜索：搜文件/目录名（原 name match 行为）
const searchFullText = ref(false)
const searchResults = ref<FileItem[] | null>(null)
const isSearching = ref(false)
const searchMode = ref<SearchMode>('none')
const lastFullResults = ref<FileItem[]>([])
const lastScrollTop = ref(0)
const mainContentRef = ref<any>(null)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchGeneration = 0

async function restoreScrollTop() {
  await nextTick()
  requestAnimationFrame(() => {
    if (mainContentRef.value && mainContentRef.value.$el && lastScrollTop.value > 0) {
      const scrollEl = mainContentRef.value.$el
      if (scrollEl && scrollEl.scrollTop !== undefined) {
        scrollEl.scrollTop = lastScrollTop.value
      }
    }
  })
}

const searchCache = new Map<string, { timestamp: number; results: FileItem[] }>()
const CACHE_TTL = 30000

const MAX_RETRIES = isNative() ? 15 : 3
const RETRY_DELAY = 1000

const pathSegments = computed(() => {
  if (!currentPath.value || currentPath.value === '/d') return []
  if (currentPath.value === MOUNT_ROOT) return []
  const parts = currentPath.value.split('/').filter(Boolean)
  return parts.map((name, index) => {
    let displayName = name
    if (index === 0 && name === 'd') {
      displayName = t('files.mountRoot') || '挂载点'
    } else if (index === 1) {
      const m = files.value.find(
        (f) => f.isDirectory && mountDriverOf(f) != null && f.name === name,
      )
      if (m) displayName = m.name
    }
    return {
      name: displayName,
      path: '/' + parts.slice(0, index + 1).join('/'),
    }
  })
})

const displayFiles = computed(() => {
  const raw = searchResults.value !== null ? searchResults.value : sortedFiles.value
  const tagMap = fileTagMap.value
  return raw.map(f => ({ ...f, _tags: tagMap[f.path] || [] }))
})

// tokenizeQuery / renderSnippet 已拆到 useFilesView.searchTokens.ts（可单测）

const sortedFiles = computed(() => {
  return sortFiles(files.value, sortBy.value, sortDesc.value)
})

let loadGeneration = 0
let isStreamLoading = false

async function loadFiles() {
  console.info('[Files] Loading files (stream), path:', currentPath.value)
  const gen = ++loadGeneration
  isStreamLoading = true
  pendingFileChanges.clear()
  const isPathChange = currentPath.value !== lastLoadedPath.value
  const isInitialLoad = files.value.length === 0 && firstLoad === true
  if (isPathChange || isInitialLoad) {
    loading.value = true
  } else {
    refreshing.value = true
  }
  files.value = []
  firstLoad = false
  connecting.value = false
  noPermission.value = false

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    if (gen !== loadGeneration) return

    try {
      const result = await listFilesStream(currentPath.value, (file) => {
        if (gen !== loadGeneration) return
        files.value.push(file)
        if (files.value.length === 1 && loading.value) {
          loading.value = false
          console.info('[Files] First item arrived, UI unlocked')
        }
      })

      serverOnline.value = true
      noPermission.value = false
      loading.value = false
      refreshing.value = false
      connecting.value = false
      console.info('[Files] Stream complete, total:', result.files.length, 'files')

      lastLoadedPath.value = currentPath.value
      loadFileTagsForCurrentDir()

      if (pendingHighlight.value) {
        const name = pendingHighlight.value
        pendingHighlight.value = null
        nextTick(() => highlightFile(name))
      }
      if (pendingFileChanges.size > 0) {
        void applyFileChanges()
      }
      return
    } catch (error) {
      if (error instanceof PermissionDeniedError) {
        serverOnline.value = true
        noPermission.value = true
        loading.value = false
        refreshing.value = false
        connecting.value = false
        if (pendingFileChanges.size > 0) { void applyFileChanges() }
        return
      }
      if (error instanceof NotFoundError) {
        serverOnline.value = true
        loading.value = false
        refreshing.value = false
        connecting.value = false
        if (currentPath.value !== '/d') {
          showToast({ message: t('files.pathNotFound'), duration: 2000, color: 'warning' })
          goUp()
        }
        if (pendingFileChanges.size > 0) { void applyFileChanges() }
        return
      }
      if (attempt < MAX_RETRIES) {
        connecting.value = true
        await new Promise(r => setTimeout(r, RETRY_DELAY))
      }
    } finally {
      if (gen === loadGeneration) {
        isStreamLoading = false
      }
    }
  }

  if (gen !== loadGeneration) return
  serverOnline.value = false
  loading.value = false
  refreshing.value = false
  connecting.value = false
}

async function handleRefresh(event: CustomEvent) {
  if (selectedPlugin.value) {
    pluginFiles.value = []
    pluginLoaded.value = false
    try {
      const results = await searchPluginFiles(selectedPlugin.value)
      pluginFiles.value = results
    } catch (e) {
      console.debug('[Files] Plugin refresh failed:', e)
    } finally {
      pluginLoaded.value = true
    }
  } else {
    try {
      files.value = await listFiles(currentPath.value)
      serverOnline.value = true
      noPermission.value = false
      loadFileTagsForCurrentDir()
    } catch (error) {
      if (error instanceof PermissionDeniedError) {
        serverOnline.value = true
        noPermission.value = true
      }
      if (error instanceof NotFoundError) {
        serverOnline.value = true
        if (currentPath.value !== '/d') {
          goUp()
        }
      }
    }
  }
  ;(event.target as any)?.complete?.()
}

async function retryConnection() {
  await loadFiles()
}

async function handleRequestStorage() {
  console.info('[Files] Requesting storage permission')
  await requestStoragePermission()
  setTimeout(() => loadFiles(), 1500)
}

function navigateTo(path: string) {
  currentPath.value = path
  searchQuery.value = ''
  searchResults.value = null
  loadFiles()
}

function highlightFile(name: string) {
  if (!name) return
  const target = files.value.find((f) => f.name === name)
  if (!target) {
    console.info('[Files] highlightFile: target not found in current dir:', name)
    return
  }
  highlightedPath.value = target.path
  nextTick(() => {
    const el = document.querySelector<HTMLElement>(
      `ion-item[data-highlight-path="${CSS.escape(target.path)}"]`,
    )
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
      console.info('[Files] highlightFile: scrolled to', target.path)
    } else {
      console.info('[Files] highlightFile: element not found for path:', target.path)
    }
  })
  if (highlightTimer) clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    highlightedPath.value = null
    highlightTimer = null
  }, 2000)
}

function openContainingFolder(file: FileItem) {
  if (!file || !file.path) {
    // 搜索结果可能没有完整 path，防御性处理
    searchQuery.value = ''
    searchResults.value = null
    return
  }
  const parts = file.path.split('/').filter(Boolean)
  let parentDir: string
  if (parts.length >= 2 && parts[0] === 'd') {
    parentDir = '/' + parts.slice(0, 2).join('/')
  } else {
    parentDir = file.path.substring(0, file.path.lastIndexOf('/')) || MOUNT_ROOT
  }
  searchQuery.value = ''
  searchResults.value = null
  navigateTo(parentDir)
}

function goUp() {
  if (isMountRoot.value) return
  if (!currentPath.value) {
    currentPath.value = MOUNT_ROOT
    searchQuery.value = ''
    searchResults.value = null
    loadFiles()
    return
  }
  const parts = currentPath.value.split('/').filter(Boolean)
  if (parts.length === 2 && parts[0] === 'd') {
    currentPath.value = MOUNT_ROOT
  } else {
    parts.pop()
    currentPath.value = parts.length === 0 ? MOUNT_ROOT : '/' + parts.join('/')
  }
  searchQuery.value = ''
  searchResults.value = null
  loadFiles()
}

async function handleFileClick(file: FileItem) {
  const clickResult = await findClickHandler(file)
  if (clickResult?.handled) {
    const cached = getSessionPassword(file.path)
    let password: string | undefined | null = cached
    if (!password) {
      password = await promptPassword(file.name)
      if (!password) return
    }
    setSessionPassword(file.path, password)
    if (isAlistEncrypted(file)) {
      await loadDecodedName(file, password)
    }
    const displayName = isAlistEncrypted(file)
      ? (getDecodedName(file.path) || file.name)
      : file.name
    router.push({ path: '/player', query: { path: file.path, name: displayName, alistPath: file.path, alistPassword: password } })
    return
  }

  if (file.isDirectory) {
    // 修复：搜索结果（递归）的文件夹可能在任意位置，必须用 file.path 而不是 currentPath + '/' + file.name。
    // 原写法对非当前目录的搜索结果文件夹会导航到错误路径导致 404 / render crash。
    const targetPath = file.path || (currentPath.value === '/d' ? '/d' : currentPath.value) + '/' + file.name
    if (!file.path) {
      console.warn('[Files] Folder click missing path, falling back to currentPath + name', file)
    }
    navigateTo(targetPath)
    return
  }

  if (isAlistEncrypted(file)) {
    const password = await promptPassword(file.name)
    if (!password) return
    router.push({ path: '/player', query: { path: file.path, name: file.name, alistPath: file.path, alistPassword: password } })
    return
  }

  if (file.isEncrypted) {
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: 'true' },
    })
    return
  }

  const category = getFileCategory(file.name)
  console.info('[Files] Click:', file.name, 'category:', category)
  if (category === 'video' || category === 'audio') {
    playMedia(file, category)
  } else {
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: 'false' },
    })
  }
}

// =============================================================================
// 5) 搜索
// =============================================================================

function handleSearchInput() {
  const query = searchQuery.value.trim()
  if (!query) {
    searchGeneration++
    searchResults.value = null
    isSearching.value = false
    return
  }
  searchTimer = setTimeout(() => performSearch(), 300)
}

function handleSearchClear() {
  searchGeneration++
  searchQuery.value = ''
  searchResults.value = null
  isSearching.value = false
}

function handleSearchToggle() {
  if (searchQuery.value.trim()) {
    performSearch()
  }
}

/**
 * 🆕 2026-07-02 插入操作符到搜索框（在光标位置）。
 *
 * 用法：用户点击语法高亮下方的 [＆] [｜] [￢] 按钮 → 在 searchQuery 末尾（或光标处）插入对应英文关键字。
 *
 * 设计要点：
 *   - 插入的是英文（AND/OR/NOT/quote/regex:），因为后端 FTS5 只认这些
 *   - UI 层会把它们渲染成符号（&/｜/￢），不影响实际查询
 *   - 触发 onIonInput 让 tokenize 重新解析 + 触发 performSearch
 */
function insertOperator(op: string) {
  const cur = searchQuery.value || ''
  // 简化：插入到末尾（ion-searchbar 没有暴露光标位置 API）
  // 自动补空格：phrase 引号特殊处理
  let insertion = op
  if (op === '""') {
    // 光标放在两个引号之间
    insertion = '""'
  }
  searchQuery.value = cur + insertion
  handleSearchInput()
}

async function performSearch() {
  const query = searchQuery.value.trim()
  if (!query) return
  if (selectedPlugin.value) return

  if (mainContentRef.value && mainContentRef.value.$el) {
    const scrollEl = mainContentRef.value.$el
    if (scrollEl && scrollEl.scrollTop !== undefined) {
      lastScrollTop.value = scrollEl.scrollTop
    }
  }

  const clientHits = clientFilterFiles(lastFullResults.value, query)
  if (clientHits.length > 0) {
    searchResults.value = clientHits
    restoreScrollTop()
  }

  if (isSearching.value) {
    searchGeneration++
  }
  const gen = ++searchGeneration

  const cacheKey = `${currentPath.value}:${query}:fulltext=${searchFullText.value}`
  const cached = searchCache.get(cacheKey)
  if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
    if (gen !== searchGeneration) return
    searchResults.value = cached.results
    lastFullResults.value = cached.results
    restoreScrollTop()
    return
  }

  isSearching.value = clientHits.length === 0
  try {
    let results: FileItem[] = []
    let mode: SearchMode = 'none'
    if (searchFullText.value) {
      // 🆕 2026-07-02 用户反馈：所有用户输入（含 AND/OR/NOT）全部当普通文本搜索
      // 解决方案：把整段 query 包成 phrase "..."，FTS5 phrase 模式会把所有内容当字面量
      //   - 用户输入 "在线 AND 高清" → FTS5 搜 "在线 AND 高清" 整体（phrase match）
      //   - FTS5 bigram 在 phrase 内部仍生效（"在"+"线" "高"+"清" 都能命中）
      //   - AND/OR/NOT 不再被解析为操作符
      // 例外：如果用户用了 phrase 语法 "..." 自己的，我们不再嵌套（避免 ""..." 语法错误）
      //  - 检测：query 已经含 "..." 则不再包
      let ftsQuery = query
      if (!query.includes('"') && !query.toLowerCase().startsWith('regex:')) {
        // 转义内部 "（防止用户输入里含未闭合 quote）
        ftsQuery = `"${query.replace(/"/g, '\\"')}"`
      }
      const ftResult = await searchFilesFullText(ftsQuery, 200, currentPath.value)
      results = ftResult.results
      mode = 'strict' // 全文搜索视为严格匹配
    } else {
      try {
        const vecResult = await searchFilesVector(currentPath.value, query, true, 200)
        results = vecResult.results
        mode = vecResult.search_mode
      } catch {
        results = await searchFiles(currentPath.value, query, false)
        mode = 'none'
      }
    }

    if (gen !== searchGeneration) return
    searchResults.value = results
    lastFullResults.value = results
    searchMode.value = mode
    if (results.length > 0 && sortBy.value !== 'relevance') {
      sortBy.value = 'relevance'
      sortDesc.value = true
    }
    searchCache.set(cacheKey, { timestamp: Date.now(), results })
    restoreScrollTop()
  } catch {
    if (gen !== searchGeneration) return
    searchResults.value = []
    searchMode.value = 'none'
  }
  isSearching.value = false
}

// =============================================================================
// 6) 长按菜单（action sheet）
// =============================================================================

async function handleLongPress(file: FileItem) {
  const category = file.isDirectory ? 'directory' : getFileCategory(file.name)
  const buttons: any[] = []

  buttons.push({
    text: t('files.info'),
    icon: informationCircle,
    cssClass: 'action-section-view',
    handler: () => {
      router.push({ path: '/tabs/file-info', query: { path: file.path, name: file.name } })
    },
  })

  if (file.isDirectory) {
    buttons.push({
      text: t('files.open'),
      icon: folderOpen,
      cssClass: 'action-section-view',
      handler: () => {
        const base = currentPath.value === '/d' ? '/d' : currentPath.value
        const newPath = base + '/' + file.name
        navigateTo(newPath)
      },
    })
  } else if (file.isEncrypted) {
    buttons.push({
      text: t('files.preview'),
      icon: eyeOutline,
      cssClass: 'action-section-view',
      handler: () => {
        router.push({
          path: '/tabs/preview',
          query: { path: file.path, name: file.name, isEncrypted: 'true' },
        })
      },
    })
  } else {
    const isMedia = category === 'video' || category === 'audio'

    const featureActions = await getAllActions(file)
    for (const fa of featureActions) {
      buttons.push({
        text: fa.text(),
        icon: fa.icon,
        cssClass: 'action-section-view',
        ...(fa.color ? { role: undefined, cssClass: `action-section-view action-color-${fa.color}` } : {}),
        handler: () => {
          fa.handler(file)
        },
      })
    }

    buttons.push({
      text: isMedia ? t('files.play') : t('files.preview'),
      icon: isMedia ? playCircle : eyeOutline,
      cssClass: 'action-section-view',
      handler: () => {
        if (isMedia) {
          playMedia(file, category)
        } else {
          router.push({
            path: '/tabs/preview',
            query: { path: file.path, name: file.name, isEncrypted: 'false' },
          })
        }
      },
    })
  }

  buttons.push({
    text: '重命名',
    icon: createOutline,
    cssClass: 'action-section-manage',
    handler: () => {
      selectedFile.value = file
      renameValue.value = file.name
      renamePassword.value = ''
      showRenameDialog.value = true
    },
  })
  buttons.push({
    text: '复制',
    icon: copyOutline,
    cssClass: 'action-section-manage',
    handler: () => {
      handleCopy(file)
    },
  })
  buttons.push({
    text: '移动',
    icon: arrowForwardOutline,
    cssClass: 'action-section-manage',
    handler: () => {
      selectedFile.value = file
      moveTargetPath.value = currentPath.value
      showMoveDialog.value = true
    },
  })
  buttons.push({
    text: '分享',
    icon: shareOutline,
    cssClass: 'action-section-manage',
    handler: () => {
      handleShare(file)
    },
  })
  buttons.push({
    text: '标签管理',
    icon: pricetagOutline,
    cssClass: 'action-section-manage',
    handler: async () => {
      selectedFile.value = file
      newTagInput.value = ''
      editingFileTags.value = []
      showTagDialog.value = true
      try {
        const allTags = await fetchTags()
        editingFileTags.value = allTags
          .filter(t => t.count > 0)
          .map(t => t.name)
          .slice(0, 10)
      } catch {}
    },
  })

  buttons.push({
    text: t('files.delete'),
    icon: trash,
    role: 'destructive',
    cssClass: 'action-section-danger',
    handler: () => {
      handleDeleteFile(file)
    },
  })

  buttons.push({
    text: t('files.cancelSelect'),
    role: 'cancel',
  })

  const actionSheet = await actionSheetController.create({
    header: file.name,
    buttons,
    cssClass: 'file-action-sheet',
  })
  await actionSheet.present()
}

// =============================================================================
// 7) 文件操作（copy / rename / move / share / delete / tag）
// =============================================================================

async function handleCopy(file: FileItem) {
  const baseName = file.name.replace(/\.[^.]+$/, '')
  const ext = file.name.includes('.') ? '.' + file.name.split('.').pop() : ''
  const destName = `${baseName}_copy${ext}`
  const destPath = currentPath.value === '/d' ? `/d/${destName}` : `${currentPath.value}/${destName}`
  try {
    await copyFile(file.path, destPath)
    showToast({ message: t('tasks.copy') + ' ' + t('tasks.taskCreated'), duration: 1500, color: 'success' })
  } catch (err: any) {
    showToast({ message: err.message || 'Copy failed', duration: 2000, color: 'danger' })
  }
}

function onRenameConfirm(d: any) {
  renameValue.value = d.name ?? renameValue.value
  renamePassword.value = d.password ?? ''
  if (selectedFile.value) handleRename(selectedFile.value)
}

async function handleRename(file: FileItem) {
  if (!renameValue.value.trim() || renameValue.value === file.name) return
  try {
    if (file.isEncrypted) {
      const result = await renameOriginalName(file.path, renameValue.value.trim(), renamePassword.value.trim() || undefined)
      if (result.success) {
        showToast({ message: '原始文件名已更新', duration: 1500, color: 'success' })
      }
    } else {
      await renameFile(file.path, renameValue.value.trim())
      showToast({ message: t('tasks.rename') + ' ' + t('tasks.taskCreated'), duration: 1500, color: 'success' })
    }
    showRenameDialog.value = false
    renamePassword.value = ''
    if (file.isEncrypted) {
      await loadFiles()
    }
  } catch (err: any) {
    showToast({ message: err.message || 'Rename failed', duration: 2000, color: 'danger' })
  }
}

async function handleMove(file: FileItem) {
  if (!moveTargetPath.value || moveTargetPath.value === file.path) return
  const destPath = moveTargetPath.value.endsWith('/')
    ? `${moveTargetPath.value}${file.name}`
    : `${moveTargetPath.value}/${file.name}`
  try {
    await moveFile(file.path, destPath)
    showMoveDialog.value = false
    showToast({ message: t('tasks.move') + ' ' + t('tasks.taskCreated'), duration: 1500, color: 'success' })
  } catch (err: any) {
    showToast({ message: err.message || 'Move failed', duration: 2000, color: 'danger' })
  }
}

async function handleShare(file: FileItem) {
  if (isNative()) {
    try {
      const localPath = await getLocalFilePath(file.path)
      if (localPath) {
        await Share.share({ title: file.name, url: 'file://' + localPath })
      } else {
        showToast({ message: '仅支持本地文件分享', duration: 2500, color: 'warning' })
      }
    } catch (e) { showToast({ message: '分享失败或已取消' }) }
  } else {
    copyToClipboard(getExternalStreamUrl(file.path)).then(ok => showToast({ message: ok ? '链接已复制到剪贴板' : '复制失败', color: ok ? 'success' : 'danger' }))
  }
}

const fileInputRef = ref<HTMLInputElement>()

function handleUpload() {
  fileInputRef.value?.click()
}

async function handleFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files?.length) return

  const filesToUpload = Array.from(input.files)
  let successCount = 0
  let failCount = 0

  for (const file of filesToUpload) {
    try {
      showToast({ message: `正在上传: ${file.name}...`, color: 'primary', duration: 2000 })
      await uploadFile(currentPath.value, file)
      successCount++
    } catch (e) {
      console.error('[Files] upload failed:', file.name, e instanceof Error ? `${e.name}: ${e.message}` : String(e))
      failCount++
    }
  }

  if (successCount > 0) {
    showToast({
      message: `成功上传 ${successCount} 个文件${failCount > 0 ? `，${failCount} 个失败` : ''}`,
      color: failCount > 0 ? 'warning' : 'success',
      duration: 3000,
    })
    await loadFiles()
  }

  input.value = ''
}

async function handleAddNewTag() {
  if (!selectedFile.value || !newTagInput.value.trim()) return
  const tag = newTagInput.value.trim()
  if (editingFileTags.value.includes(tag)) {
    newTagInput.value = ''
    return
  }
  try {
    await addTag(selectedFile.value.path, tag)
    editingFileTags.value.push(tag)
    newTagInput.value = ''
  } catch (e) { showToast({ message: '添加标签失败' }) }
}

async function handleRemoveTag(tag: string) {
  if (!selectedFile.value) return
  try {
    await removeTag(selectedFile.value.path, tag)
    editingFileTags.value = editingFileTags.value.filter(t => t !== tag)
  } catch (e) { showToast({ message: '移除标签失败' }) }
}

async function loadFileTagsForCurrentDir() {
  try {
    const allTags = await fetchTags()
    const map: Record<string, string[]> = {}
    for (const tag of allTags) {
      if (tag.count > 0) {
        for (const f of files.value) {
          if (!map[f.path]) map[f.path] = []
          map[f.path].push(tag.name)
        }
      }
    }
    fileTagMap.value = map
  } catch {}

  const badgesMap: Record<string, any[]> = {}
  const subtitlesMap: Record<string, any[]> = {}
  for (const f of files.value) {
    const badges = await getBadges(f)
    if (badges.length > 0) badgesMap[f.path] = badges
    const subs = await getSubtitles(f)
    if (subs.length > 0) subtitlesMap[f.path] = subs
  }
  fileBadges.value = badgesMap
  fileSubtitles.value = subtitlesMap

  preloadSubtitles(files.value)
  setupLazyThumbnails()
}

async function handleDeleteFile(file: FileItem) {
  if (file.path === '/' || file.path === '') {
    showToast({ message: '不能删除根目录', duration: 2000, color: 'danger' })
    return
  }

  if (file.isDirectory) {
    let detail = '此操作不可撤销'
    try {
      const list = await listFiles(file.path)
      const filesInDir = list.filter((f: FileItem) => !f.isDirectory).length
      const subDirs = list.filter((f: FileItem) => f.isDirectory).length
      detail = `包含 ${filesInDir} 个文件 + ${subDirs} 个子目录，此操作不可撤销。`
    } catch (e) {
      console.warn('[Files] list directory failed before delete:', file.path, e)
    }
    const dirAlert = await alertController.create({
      header: t('files.delete'),
      subHeader: `📁 ${file.name}`,
      message: `确认删除文件夹 "${file.name}" 及其所有内容？\n\n${detail}`,
      buttons: [
        { text: t('files.cancelSelect'), role: 'cancel' },
        { text: t('files.delete'), role: 'destructive', handler: () => doDelete(file) },
      ],
    })
    await dirAlert.present()
  } else {
    const alert = await alertController.create({
      header: t('files.delete'),
      message: t('files.deleteConfirm', { name: file.name }),
      buttons: [
        { text: t('files.cancelSelect'), role: 'cancel' },
        { text: t('files.delete'), role: 'destructive', handler: () => doDelete(file) },
      ],
    })
    await alert.present()
  }
}

async function doDelete(file: FileItem) {
  try {
    await deleteFile(file.path)
    showToast({ message: t('tasks.delete') + ' ' + t('tasks.taskCreated'), duration: 1500, color: 'success' })
  } catch (err: any) {
    showToast({ message: err.message || 'Delete failed', duration: 2000, color: 'danger' })
  }
}

// =============================================================================
// 8) file:change 增量更新
// =============================================================================

let fileChangeDebounceTimer: number | null = null
const pendingFileChanges = new Map<string, 'create' | 'delete' | 'modify'>()

function onFileChange(payload: { path: string; action: 'create' | 'delete' | 'modify' }) {
  searchCache.clear()
  pendingFileChanges.set(payload.path, payload.action)
  if (fileChangeDebounceTimer !== null) {
    clearTimeout(fileChangeDebounceTimer)
  }
  fileChangeDebounceTimer = window.setTimeout(() => {
    fileChangeDebounceTimer = null
    if (isStreamLoading) {
      console.info('[Files] file:change deferred, stream loading', pendingFileChanges.size, 'changes')
      return
    }
    void applyFileChanges()
  }, 300)
}

async function applyFileChanges() {
  if (pendingFileChanges.size === 0) return
  const changes = new Map(pendingFileChanges)
  pendingFileChanges.clear()

  for (const [path, action] of changes) {
    if (action === 'delete') {
      const idx = files.value.findIndex((f) => f.path === path)
      if (idx >= 0) {
        files.value.splice(idx, 1)
        console.info('[Files] incremental delete:', path)
      }
    }
  }

  const visiblePaths: string[] = []
  for (const [path, action] of changes) {
    if (action === 'delete') continue
    const parent = path.substring(0, path.lastIndexOf('/')) || '/d'
    if (parent !== currentPath.value) continue
    visiblePaths.push(path)
  }
  if (visiblePaths.length === 0) return

  const results = await Promise.allSettled(visiblePaths.map((p) => getFileInfo(p)))
  for (let i = 0; i < results.length; i++) {
    const r = results[i]
    const path = visiblePaths[i]
    if (r.status === 'fulfilled') {
      const fileItem = r.value
      const idx = files.value.findIndex((f) => f.path === path)
      if (idx >= 0) {
        files.value[idx] = fileItem
        console.info('[Files] incremental modify:', path)
      } else {
        files.value.push(fileItem)
        console.info('[Files] incremental create:', path)
      }
    } else {
      const idx = files.value.findIndex((f) => f.path === path)
      if (idx >= 0) {
        files.value.splice(idx, 1)
        console.info('[Files] incremental remove (fetch failed):', path)
      }
    }
  }
}

// =============================================================================
// 9) 侧边栏 / Plugins / Tags
// =============================================================================

async function loadPlugins() {
  try { plugins.value = await fetchPlugins() } catch {}
}
async function loadTags() {
  try { tags.value = await fetchTags() } catch {}
}

function openPluginView(plugin: PluginMeta) {
  files.value = []
  loading.value = true
  pluginLoaded.value = false
  selectedPlugin.value = plugin
  menuController.close()
}

async function exitPluginMode() {
  selectedPlugin.value = null
  await menuController.close()
  files.value = []
  loading.value = true
  await loadFiles()
}

async function openSideDrawer() {
  await menuController.open('plugin-menu')
}

function getPluginIcon(plugin: PluginMeta): any {
  const featureIcon = getFeatureIcon(plugin.name)
  if (featureIcon) return featureIcon
  const icons: Record<string, string> = { video: filmOutline, audio: musicalNotesOutline, image: imageOutline, pdf: documentTextOutline, text: documentOutline, wps: documentOutline }
  return icons[plugin.name] || lockClosed
}

async function searchPluginFiles(
  plugin: PluginMeta,
  onItem?: (file: FileItem) => void
): Promise<FileItem[]> {
  if (!plugin.supportedExtensions || plugin.supportedExtensions.length === 0) return []
  const result = await listPluginFilesStream(
    currentPath.value,
    plugin.supportedExtensions,
    (file) => { onItem?.(file) }
  )
  return result.files
}

async function handleTagFilter(tagName: string) {
  menuController.close()
  files.value = []
  loading.value = true
  selectedPlugin.value = null
  try {
    files.value = await listFilesByTag(tagName, currentPath.value)
    loadFileTagsForCurrentDir()
  } catch (e) { showToast({ message: `筛选失败: ${e}` }) }
  finally { loading.value = false }
}

// =============================================================================
// 10) Plugin view state（筛选 / 排序 / 切换）
// =============================================================================

const pluginTab = ref<'source' | 'container'>('source')
const pluginFiles = ref<FileItem[]>([])
const pluginLoaded = ref(false)
let pluginLoadGeneration = 0

const sizeFilterMin = ref<number | null>(null)
const sizeFilterMax = ref<number | null>(null)
const timeFilterFrom = ref<string | null>(null)
const timeFilterTo = ref<string | null>(null)
const showPluginFilters = ref(false)

const pluginSortBy = ref<'name' | 'size' | 'time'>('name')
const pluginSortDesc = ref(false)

const pluginSortLabel = computed(() => {
  const map: Record<string, string> = { name: '名称', size: '大小', time: '时间' }
  return (map[pluginSortBy.value] || '名称') + (pluginSortDesc.value ? '↓' : '↑')
})

const activeFilterCount = computed(() => {
  let c = 0
  if (sizeFilterMin.value !== null) c++
  if (sizeFilterMax.value !== null) c++
  if (timeFilterFrom.value !== null) c++
  if (timeFilterTo.value !== null) c++
  return c
})

function applySizePreset(preset: typeof SIZE_PRESETS[number]) {
  sizeFilterMin.value = 'min' in preset ? (preset as { min?: number }).min ?? null : null
  sizeFilterMax.value = 'max' in preset ? (preset as { max?: number }).max ?? null : null
}
function applyTimePreset(preset: typeof TIME_PRESETS[number]) {
  const now = new Date()
  const from = new Date(now)
  from.setDate(from.getDate() - preset.days)
  from.setHours(0, 0, 0, 0)
  timeFilterFrom.value = formatDateInput(from)
  if (preset.days === 0) {
    timeFilterTo.value = formatDateInput(now)
  } else {
    timeFilterTo.value = null
  }
}

function clearAllPluginFilters() {
  sizeFilterMin.value = null
  sizeFilterMax.value = null
  timeFilterFrom.value = null
  timeFilterTo.value = null
  pluginSortBy.value = 'name'
  pluginSortDesc.value = false
}

const filteredPluginFiles = computed(() => {
  if (!selectedPlugin.value) return []
  let list: FileItem[]
  if (pluginTab.value === 'container') {
    list = pluginFiles.value.filter(f => isAnyContainerFile(f))
  } else {
    list = pluginFiles.value.filter(f => !isAnyContainerFile(f))
  }
  const query = searchQuery.value.trim().toLowerCase()
  if (query) {
    list = list.filter(f => f.name.toLowerCase().includes(query))
  }
  if (sizeFilterMin.value !== null) {
    list = list.filter(f => (f.size || 0) >= sizeFilterMin.value!)
  }
  if (sizeFilterMax.value !== null) {
    list = list.filter(f => (f.size || 0) <= sizeFilterMax.value!)
  }
  if (timeFilterFrom.value !== null) {
    const from = new Date(timeFilterFrom.value).getTime()
    list = list.filter(f => (f.modified ? new Date(f.modified).getTime() : 0) >= from)
  }
  if (timeFilterTo.value !== null) {
    const to = new Date(timeFilterTo.value).getTime()
    list = list.filter(f => (f.modified ? new Date(f.modified).getTime() : 0) <= to)
  }
  list.sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1
    if (!a.isDirectory && b.isDirectory) return 1
    let cmp = 0
    switch (pluginSortBy.value) {
      case 'name': cmp = a.name.localeCompare(b.name); break
      case 'size': cmp = (a.size || 0) - (b.size || 0); break
      case 'time': cmp = (Number(a.modified) || 0) - (Number(b.modified) || 0); break
    }
    return pluginSortDesc.value ? -cmp : cmp
  })
  const tagMap = fileTagMap.value
  return list.map(f => ({ ...f, _tags: tagMap[f.path] || [] }))
})

watch(selectedPlugin, async (plugin) => {
  if (plugin) {
    const gen = ++pluginLoadGeneration
    pluginTab.value = 'source'
    pluginLoaded.value = false
    pluginFiles.value = []
    console.info('[Files] Loading plugin files (stream):', plugin.name)
    try {
      const results = await searchPluginFiles(plugin, (file) => {
        if (gen !== pluginLoadGeneration) return
        pluginFiles.value.push(file)
        if (pluginFiles.value.length === 1 && !pluginLoaded.value) {
          console.info('[Files] First plugin item arrived, UI unlocked')
        }
      })
      if (gen !== pluginLoadGeneration) return
      pluginFiles.value = results
    } catch (e) {
      console.debug('[Files] Plugin stream load failed:', e)
    }
    if (gen === pluginLoadGeneration) {
      pluginLoaded.value = true
      setupLazyThumbnails()
    }
  }
})

// =============================================================================
// 11) Lifecycle (onMounted / onIonViewWillEnter / onUnmounted)
// =============================================================================

function onBackendReady(data: { port?: number; running?: boolean }) {
  if (data.running || data.port) {
    loadFiles()
  }
}

function onBackendReadyWindow(event: Event) {
  const detail = (event as CustomEvent).detail || {}
  onBackendReady(detail)
}

onMounted(() => {
  loadFiles()
  loadPlugins()
  loadTags()
  eventBus.on('file:change', onFileChange)
  window.addEventListener('encv:backend-ready', onBackendReadyWindow as EventListener)
  useRealtimeTransport().setFileChangeGate(true, () => {
    if (fileChangeDebounceTimer !== null) {
      clearTimeout(fileChangeDebounceTimer)
      fileChangeDebounceTimer = null
    }
    loadFiles()
  })
  if (import.meta.env.DEV) {
    import('@/composables/useTestBackdoor').then(({ useTestBackdoor }) => {
      import('@/composables/useNewTaskModal').then(({ useNewTaskModal: createNewTaskModal }) => {
        const { openNewTask } = createNewTaskModal()
        useTestBackdoor(files, {
          onLongPress: handleLongPress,
          onClick: handleFileClick,
          navigateTo: navigateTo,
          openNewTask: (sourcePath?: string, taskType?: 'encrypt' | 'decrypt') => {
            return openNewTask(sourcePath, taskType)
          },
          __debugOnFileChange: onFileChange,
          __debugGetPendingChanges: () => pendingFileChanges.size,
          __debugIsStreamLoading: () => isStreamLoading,
        })
      })
    })
  }
})

onIonViewWillEnter(() => {
  const qPath = typeof route.query.path === 'string' ? route.query.path : ''
  const qHighlight = typeof route.query.highlight === 'string' ? route.query.highlight : ''

  if (qPath && qHighlight) {
    if (qPath !== currentPath.value) {
      pendingHighlight.value = qHighlight
      currentPath.value = qPath
      searchQuery.value = ''
      searchResults.value = null
      loadFiles()
    } else {
      highlightFile(qHighlight)
    }
    router.replace({ path: route.path, query: {} })
    return
  }

  if (files.value.length === 0 && !loading.value && !connecting.value) {
    loadFiles()
  }
})

onUnmounted(() => {
  eventBus.off('file:change', onFileChange)
  window.removeEventListener('encv:backend-ready', onBackendReadyWindow as EventListener)
  if (searchTimer) clearTimeout(searchTimer)
  if (highlightTimer) {
    clearTimeout(highlightTimer)
    highlightTimer = null
  }
  if (fileChangeDebounceTimer !== null) {
    clearTimeout(fileChangeDebounceTimer)
    fileChangeDebounceTimer = null
  }
})

watch(
  () => route.path,
  (newPath) => {
    const isFilesActive = newPath.startsWith('/tabs/files')
    useRealtimeTransport().setFileChangeGate(isFilesActive, () => {
      if (fileChangeDebounceTimer !== null) {
        clearTimeout(fileChangeDebounceTimer)
        fileChangeDebounceTimer = null
      }
      loadFiles()
    })
  },
  { immediate: true },
)

// =============================================================================
// 12) Return：Files.vue 模板需要的所有 state + handlers
// =============================================================================

// 断言 return 类型为 UseFilesViewReturn（关键字段 required，非 declared 字段靠 index signature）
// 避免 vue-tsc 推断时丢字段
return {
  // refs (state)
  playError, playErrorDetail, playErrorFile,
  t,
  vectorSearchStatus,
  thumbnailUrls, onThumbError,
  sortBy, sortDesc,
  showMainSort,
  mainSortLabel,
  serverOnline, noPermission,
  files, plugins, tags,
  showRenameDialog, showTagDialog, showMoveDialog,
  selectedPlugin, selectedFile,
  renameValue, renamePassword, moveTargetPath,
  editingFileTags, newTagInput,
  fileTagMap, fileBadges, fileSubtitles,
  renameAlertInputs,
  currentPath,
  loading, refreshing, connecting,
  pendingHighlight, highlightedPath,
  isMountRoot,
  searchQuery, searchFullText, searchResults, isSearching,
  searchMode,
  renderSnippet,
  tokenizeQuery,
  mainContentRef,
  pathSegments, displayFiles,
  fileInputRef,
  pluginTab, pluginFiles, pluginLoaded,
  sizeFilterMin, sizeFilterMax, timeFilterFrom, timeFilterTo, showPluginFilters,
  pluginSortBy, pluginSortDesc,
  pluginSortLabel,
  activeFilterCount,
  filteredPluginFiles,
  SIZE_PRESETS, TIME_PRESETS,
  // helpers (template 直接调用)
  mountDriverOf, mountPathOf, mountRootOf,
  // icons (template 用)
  add,
  // functions (handlers)
  playMedia, clearPlayError, togglePlayErrorDetail,
  setSortBy,
  handleRefresh, retryConnection, handleRequestStorage,
  navigateTo, goUp, handleFileClick, highlightFile, openContainingFolder,
  handleSearchInput, handleSearchClear, handleSearchToggle,
  insertOperator,
  handleLongPress,
  handleCopy, onRenameConfirm, handleRename, handleMove, handleShare,
  handleUpload, handleFileSelected,
  handleAddNewTag, handleRemoveTag,
  handleDeleteFile, doDelete,
  loadPlugins, loadTags,
  openPluginView, exitPluginMode, openSideDrawer,
  getPluginIcon,
  handleTagFilter,
  applySizePreset, applyTimePreset, clearAllPluginFilters,
} as unknown as UseFilesViewReturn
}
