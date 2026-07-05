// useFilesView.ts - Files.vue 的 script 逻辑拆出（composable）
// 拆分自 Files.vue。所有 reactive state / handler / lifecycle 都集中在此。
// Files.vue 只剩 template + 调 useFilesView() 拿到返回值后解构使用。
//
// 为什么不是拆多个 composable？
//   main view 和 plugin view 共享大量 state（currentPath/files/fileTagMap/fileBadges
//   /selectedFile/renameValue 等），如果分多个 composable 要 props 双向同步，反而更乱。
//   所以采用「单 composable + 内部注释分块」的方式：拆出大文件，但保持 state 共享。

// 🆕 2026-07-02: contenteditable 搜索框 + span units（替换原 ion-searchbar + 外部 overlay）
import { useSearchInput } from "@encv/shared-components/composables/useSearchInput";
import { type QueryToken, renderSnippet, tokenizeQuery } from "@encv/shared-components/views/useFilesView.searchTokens";
import { actionSheetController, alertController, menuController, onIonViewWillEnter } from "@ionic/vue";
import { computed, nextTick, onMounted, onUnmounted, type Ref, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

// 🆕 2026-07-02: 显式 return type（用 Record<string, any> 兼容所有字段）— 避免 vue-tsc 推断丢字段
// (历史踩坑：isSelectedModelAvailable / switchSession / lanAccessLoaded / fetchModels / temperature 都从推断 type 中消失过)
// 关键：Record<string, any> 索引签名让 declared 字段可推断
export type UseFilesViewReturn = {
  // 关键字段（Files.vue 模板直引用，类型 partial 以兼容 vue-tsc 推断）
  searchFullText: Ref<boolean>;
  searchQuery: Ref<string>;
  searchResults: Ref<FileItem[] | null>;
  isSearching: Ref<boolean>;
  searchMode: Ref<SearchMode>;
  renderSnippet: (snippet: string | undefined) => Array<{ text: string; highlight: boolean }>;
  tokenizeQuery: (query: string) => QueryToken[]; // 🆕 语法高亮
  // 索引签名：允许 return 对象包含任意额外字段
  [key: string]: any;
};

import { Share } from "@capacitor/share";
import type { FileItem, IndexStats, PluginMeta, SearchMode, TagInfo } from "@encv/shared-components/api/encv";
import {
  addTag,
  copyFile,
  deleteFile,
  fetchPlugins,
  fetchTags,
  getExternalStreamUrl,
  getFileCategory,
  getFileInfo,
  getFullTextIndexStats,
  // 🆕 2026-07-03：搜索空结果时主动查询索引状态，便于诊断（FTS 是否可用、索引了多少文件）
  getIndexStats,
  listFiles,
  listFilesByTag,
  listFilesStream,
  listPluginFilesStream,
  moveFile,
  NotFoundError,
  PermissionDeniedError,
  removeTag,
  renameFile,
  renameOriginalName,
  searchFiles,
  searchFilesFullText,
  searchFilesVector,
  uploadFile,
} from "@encv/shared-components/api/encv";
import type { FullTextIndexStats } from "@encv/shared-components/api/encv_search";
import { copyToClipboard } from "@encv/shared-components/composables/useClipboard";
import { eventBus } from "@encv/shared-components/composables/useEventBus";
import { findClickHandler, getFeatureIcon, isAnyContainerFile, useFileFeatures } from "@encv/shared-components/composables/useFileFeatures";
import { clientFilterFiles, sortFiles, useFileListSort } from "@encv/shared-components/composables/useFileList";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { useRealtimeTransport } from "@encv/shared-components/composables/useRealtimeTransport";
import { useThumbnailCache } from "@encv/shared-components/composables/useThumbnailCache";
import { showToast } from "@encv/shared-components/composables/useToast";
import { useVectorSearchStatus } from "@encv/shared-components/composables/useVectorSearchStatus";
import { PLAY_MODE } from "@encv/shared-components/constants/player";
import { preloadSubtitles } from "@encv/shared-components/features/alist-encrypt";
import { promptPassword } from "@encv/shared-components/features/alist-encrypt/password-dialog";
import {
  getDecodedName,
  getSessionPassword,
  isAlistEncrypted,
  loadDecodedName,
  setSessionPassword,
} from "@encv/shared-components/features/alist-encrypt/useAlistEncrypt";
import { getLocalFilePath, isNative, openExternal, openPlayer, requestStoragePermission } from "@encv/shared-components/plugins/GoProcess";
import {
  add,
  arrowForwardOutline,
  copyOutline,
  createOutline,
  documentOutline,
  documentTextOutline,
  eyeOutline,
  filmOutline,
  folderOpen,
  imageOutline,
  informationCircle,
  lockClosed,
  musicalNotesOutline,
  playCircle,
  pricetagOutline,
  shareOutline,
  trash,
} from "ionicons/icons";
import { formatDateInput, getPlayMode, mountDriverOf, mountPathOf, mountRootOf, SIZE_PRESETS, TIME_PRESETS } from "./useFilesHelpers";

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

  const playError = ref<string>("");
  const playErrorDetail = ref<string>("");
  const playErrorFile = ref<string>("");

  async function playMedia(file: FileItem, category: string) {
    const isVideo = category === "video";
    const mediaType = isVideo ? "video" : "audio";
    const mimeType = isVideo ? "video/*" : "audio/*";
    const mode = getPlayMode(mediaType);

    console.info("[Files] playMedia: file=", file.path, "mode=", mode, "category=", category);
    playError.value = "";
    playErrorDetail.value = "";
    playErrorFile.value = "";

    switch (mode) {
      case PLAY_MODE.ARTPLAYER:
        router.push({ path: "/player", query: { path: file.path, name: file.name } });
        break;
      case PLAY_MODE.MPV_PLUGIN:
      case PLAY_MODE.MPV_ACTIVITY:
      case PLAY_MODE.MPV_FRAGMENT:
      case PLAY_MODE.MPV_COMPOSE:
        if (isNative()) {
          const result = await openPlayer(file.path, file.name, mimeType, mode);
          if (!result.success) {
            console.error("[Files] playMedia failed:", result.error, result.errorDetail);
            playError.value = result.error || "播放失败";
            playErrorDetail.value = result.errorDetail || "";
            playErrorFile.value = file.name;
          }
        } else {
          router.push({ path: "/player", query: { path: file.path, name: file.name } });
        }
        break;
      case PLAY_MODE.EXTERNAL:
        if (isNative()) {
          const url = getExternalStreamUrl(file.path);
          openExternal(url, mimeType);
        } else {
          router.push({ path: "/player", query: { path: file.path, name: file.name } });
        }
        break;
      default:
        console.debug("[Files] Unknown play mode:", mode, "— falling back to artplayer");
        router.push({ path: "/player", query: { path: file.path, name: file.name } });
        break;
    }
  }

  function clearPlayError() {
    playError.value = "";
    playErrorDetail.value = "";
    playErrorFile.value = "";
  }

  function togglePlayErrorDetail() {
    if (playErrorDetail.value) {
      const expanded = playErrorDetail.value;
      playErrorDetail.value = "";
      playError.value = playError.value + "\n" + expanded;
    }
  }

  // =============================================================================
  // 2) 排序 / 显示模式（main view）
  // =============================================================================

  const { t } = useI18n();
  const { status: vectorSearchStatus } = useVectorSearchStatus();
  const { thumbnailUrls, setupLazyThumbnails, onThumbError } = useThumbnailCache();
  const { sortBy, sortDesc } = useFileListSort();
  const showMainSort = ref(false);

  /**
   * setSortBy 排序变更封装：
   *   - 选择「相关度」时强制 sortDesc=true（高相关度在前）
   *   - 选择其他排序时保持当前升降序
   *   - 搜索时「相关度」是默认选项
   */
  function setSortBy(value: "name" | "size" | "time" | "relevance") {
    sortBy.value = value;
    if (value === "relevance") {
      sortDesc.value = true;
    }
  }

  const mainSortLabel = computed(() => {
    if (sortBy.value === "relevance") {
      return "相关度↓";
    }
    const map: Record<string, string> = { name: "名称", size: "大小", time: "时间" };
    return (map[sortBy.value] || "名称") + (sortDesc.value ? "↓" : "↑");
  });

  // =============================================================================
  // 3) 通用 state：路由 / dialog / 文件列表 / 选中态
  // =============================================================================

  const router = useRouter();
  const route = useRoute();
  const serverOnline = ref(false);
  const noPermission = ref(false);
  const files = ref<FileItem[]>([]);
  const plugins = ref<PluginMeta[]>([]);
  const tags = ref<TagInfo[]>([]);
  const showRenameDialog = ref(false);
  const showTagDialog = ref(false);
  const showMoveDialog = ref(false);
  const selectedPlugin = ref<PluginMeta | null>(null);
  const selectedFile = ref<FileItem | null>(null);
  const renameValue = ref("");
  const renamePassword = ref("");
  const moveTargetPath = ref("");
  const editingFileTags = ref<string[]>([]);
  const newTagInput = ref("");
  const fileTagMap = ref<Record<string, string[]>>({});
  const fileBadges = ref<Record<string, any[]>>({});
  const fileSubtitles = ref<Record<string, any[]>>({});

  const { getBadges, getSubtitles, getAllActions } = useFileFeatures();

  const renameAlertInputs = computed(() => {
    const inputs: any[] = [{ name: "name", type: "text", placeholder: "新文件名", value: renameValue.value }];
    if (selectedFile.value?.isEncrypted) {
      inputs.push({ name: "password", type: "text", placeholder: "文件名加密密码（如需要）" });
    }
    return inputs;
  });

  // =============================================================================
  // 4) 路径 / mount / 加载主流程
  // =============================================================================

  const MOUNT_ROOT = "/d";
  const currentPath = ref(MOUNT_ROOT);
  const loading = ref(false);
  const refreshing = ref(false);
  const connecting = ref(false);
  let firstLoad = true;
  const lastLoadedPath = ref<string>("");

  const pendingHighlight = ref<string | null>(null);
  const highlightedPath = ref<string | null>(null);
  let highlightTimer: ReturnType<typeof setTimeout> | null = null;

  const isMountRoot = computed(() => currentPath.value === MOUNT_ROOT);

  // =============================================================================
  // 5) 搜索（2026-07-02 重写：contenteditable + span units + 默认关闭 FTS + 降级）
  // =============================================================================
  //
  // 核心改进（用户强反馈）：
  //   - 输入框改用 <div contenteditable>，span units（text/op）在 input 内部高亮
  //   - 插入按钮插入 symbol span 到光标位置（不是字符串 " AND "）
  //   - FTS 搜索默认关闭（普通搜索走 searchFilesVector）
  //   - FTS 失败时静默降级 + banner 提示，不破坏现有结果
  //   - FTS 增强模式：merge + 去重 + 全文 badge（不替换普通结果）

  const { queryInputRef, queryValue, onQueryInput, onQueryKeydown, insertSymbol, clearInput } = useSearchInput({
    onChange: handleSearchInput,
  });

  // 兼容旧代码：searchQuery 仍指向 queryValue（Files.vue 其它处也读 searchQuery.value）
  const searchQuery = queryValue;

  // 🆕 2026-07-02：用户反馈"默认关闭 + merge" → searchFullText 默认 false
  const searchFullText = ref(false);
  const searchResults = ref<FileItem[] | null>(null);
  const isSearching = ref(false);
  const searchMode = ref<SearchMode>("none");
  const lastFullResults = ref<FileItem[]>([]);
  const lastScrollTop = ref(0);
  const mainContentRef = ref<any>(null);
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  let searchGeneration = 0;

  // 🆕 A6：FTS 失败 banner 提示（不抛错、不清空结果）
  const fulltextBanner = ref<{ type: "unavailable" | "error"; message: string } | null>(null);

  // 🆕 2026-07-03：搜索诊断信息（搜索结果为空时主动查询，UI 显示给用户辅助诊断）
  //   背景：安卓真机 FTS5 可能因 SQLite 编译选项异常，但前端搜索空状态只有 1 行文案，
  //   无法告诉用户"为什么没结果"。用户进不去 /settings/fulltext-index 页面（classList bug）
  //   也无法在那查看 FTS 状态。所以搜索界面直接显示诊断信息。
  interface SearchDiagnostics {
    ftsAvailable: boolean;
    ftsError?: string;
    fts5Enabled?: boolean;
    ftsIndexSize?: number; // FTS 索引的文件数（如果是 0 说明索引为空）
    ftsTokenizer?: string;
    totalFiles?: number; // 普通索引的文件数（与 ftsIndexSize 对比可判断 FTS 是否漏索引）
    isIndexing?: boolean;
    vectorSearch: string; // 'unknown' | 'available' | 'degraded' | 'unavailable'
    lastQuery?: string;
    lastMode?: SearchMode;
    lastNormalCount?: number;
    lastFtsCount?: number;
    searchedAt?: string;
  }
  const searchDiagnostics = ref<SearchDiagnostics | null>(null);

  /**
   * 🆕 2026-07-03：主动查询 FTS + 索引状态，用于搜索空结果时显示诊断卡片。
   *
   * 并发拉取两个接口：
   *   - GET /api/files/search-fulltext/stats  → FTS5 可用性 + 索引文件数 + 分词器
   *   - GET /api/index/stats                  → 普通索引文件数 + 是否正在索引
   *
   * 任一接口失败不抛错（降级为 undefined，UI 显示 '-'）。
   */
  async function refreshSearchDiagnostics(context?: {
    query?: string;
    mode?: SearchMode;
    normalCount?: number;
    ftsCount?: number;
  }): Promise<void> {
    // 分别 try/catch，避免一个失败影响另一个
    let ftsStats: { available: boolean; stats?: FullTextIndexStats; error?: string };
    try {
      ftsStats = await getFullTextIndexStats();
    } catch (e) {
      ftsStats = { available: false, error: e instanceof Error ? e.message : String(e) };
    }

    let indexStats: IndexStats | null = null;
    try {
      indexStats = await getIndexStats();
    } catch {
      // 降级为 null（UI 显示 '-'）
    }

    try {
      searchDiagnostics.value = {
        ftsAvailable: ftsStats.available,
        ftsError: ftsStats.error,
        fts5Enabled: ftsStats.stats?.fts5Enabled,
        ftsIndexSize: ftsStats.stats?.totalFiles,
        ftsTokenizer: ftsStats.stats?.tokenizer,
        totalFiles: indexStats?.totalFiles,
        isIndexing: indexStats?.isIndexing,
        vectorSearch: vectorSearchStatus.value,
        lastQuery: context?.query,
        lastMode: context?.mode,
        lastNormalCount: context?.normalCount,
        lastFtsCount: context?.ftsCount,
        searchedAt: new Date().toLocaleTimeString("zh-CN", { hour12: false }),
      };
      console.info("[Search] diagnostics refreshed", {
        ftsAvailable: ftsStats.available,
        ftsIndexSize: ftsStats.stats?.totalFiles,
        totalFiles: indexStats?.totalFiles,
        vectorSearch: vectorSearchStatus.value,
      });
    } catch (e) {
      console.warn("[Search] refresh diagnostics failed", e);
    }
  }

  async function restoreScrollTop() {
    await nextTick();
    requestAnimationFrame(() => {
      if (mainContentRef.value?.$el && lastScrollTop.value > 0) {
        const scrollEl = mainContentRef.value.$el;
        if (scrollEl && scrollEl.scrollTop !== undefined) {
          scrollEl.scrollTop = lastScrollTop.value;
        }
      }
    });
  }

  const searchCache = new Map<string, { timestamp: number; results: FileItem[] }>();
  const CACHE_TTL = 30000;

  const MAX_RETRIES = isNative() ? 15 : 3;
  const RETRY_DELAY = 1000;

  const pathSegments = computed(() => {
    if (!currentPath.value || currentPath.value === "/d") return [];
    if (currentPath.value === MOUNT_ROOT) return [];
    const parts = currentPath.value.split("/").filter(Boolean);
    return parts.map((name, index) => {
      let displayName = name;
      if (index === 0 && name === "d") {
        displayName = t("files.mountRoot") || "挂载点";
      } else if (index === 1) {
        const m = files.value.find(f => f.isDirectory && mountDriverOf(f) != null && f.name === name);
        if (m) displayName = m.name;
      }
      return {
        name: displayName,
        path: "/" + parts.slice(0, index + 1).join("/"),
      };
    });
  });

  const displayFiles = computed(() => {
    const raw = searchResults.value !== null ? searchResults.value : sortedFiles.value;
    const tagMap = fileTagMap.value;
    return raw.map(f => ({ ...f, _tags: tagMap[f.path] || [] }));
  });

  // tokenizeQuery / renderSnippet 已拆到 useFilesView.searchTokens.ts（可单测）

  const sortedFiles = computed(() => {
    return sortFiles(files.value, sortBy.value, sortDesc.value);
  });

  let loadGeneration = 0;
  let isStreamLoading = false;

  async function loadFiles() {
    console.info("[Files] Loading files (stream), path:", currentPath.value);
    const gen = ++loadGeneration;
    isStreamLoading = true;
    pendingFileChanges.clear();
    const isPathChange = currentPath.value !== lastLoadedPath.value;
    const isInitialLoad = files.value.length === 0 && firstLoad === true;
    if (isPathChange || isInitialLoad) {
      loading.value = true;
    } else {
      refreshing.value = true;
    }
    files.value = [];
    firstLoad = false;
    connecting.value = false;
    noPermission.value = false;

    for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
      if (gen !== loadGeneration) return;

      try {
        const result = await listFilesStream(currentPath.value, file => {
          if (gen !== loadGeneration) return;
          files.value.push(file);
          if (files.value.length === 1 && loading.value) {
            loading.value = false;
            console.info("[Files] First item arrived, UI unlocked");
          }
        });

        serverOnline.value = true;
        noPermission.value = false;
        loading.value = false;
        refreshing.value = false;
        connecting.value = false;
        console.info("[Files] Stream complete, total:", result.files.length, "files");

        lastLoadedPath.value = currentPath.value;
        loadFileTagsForCurrentDir();

        if (pendingHighlight.value) {
          const name = pendingHighlight.value;
          pendingHighlight.value = null;
          nextTick(() => highlightFile(name));
        }
        if (pendingFileChanges.size > 0) {
          void applyFileChanges();
        }
        return;
      } catch (error) {
        if (error instanceof PermissionDeniedError) {
          serverOnline.value = true;
          noPermission.value = true;
          loading.value = false;
          refreshing.value = false;
          connecting.value = false;
          if (pendingFileChanges.size > 0) {
            void applyFileChanges();
          }
          return;
        }
        if (error instanceof NotFoundError) {
          serverOnline.value = true;
          loading.value = false;
          refreshing.value = false;
          connecting.value = false;
          if (currentPath.value !== "/d") {
            showToast({ message: t("files.pathNotFound"), duration: 2000, color: "warning" });
            goUp();
          }
          if (pendingFileChanges.size > 0) {
            void applyFileChanges();
          }
          return;
        }
        if (attempt < MAX_RETRIES) {
          connecting.value = true;
          await new Promise(r => setTimeout(r, RETRY_DELAY));
        }
      } finally {
        if (gen === loadGeneration) {
          isStreamLoading = false;
        }
      }
    }

    if (gen !== loadGeneration) return;
    serverOnline.value = false;
    loading.value = false;
    refreshing.value = false;
    connecting.value = false;
  }

  async function handleRefresh(event: CustomEvent) {
    if (selectedPlugin.value) {
      pluginFiles.value = [];
      pluginLoaded.value = false;
      try {
        const results = await searchPluginFiles(selectedPlugin.value);
        pluginFiles.value = results;
      } catch (e) {
        console.debug("[Files] Plugin refresh failed:", e);
      } finally {
        pluginLoaded.value = true;
      }
    } else {
      try {
        files.value = await listFiles(currentPath.value);
        serverOnline.value = true;
        noPermission.value = false;
        loadFileTagsForCurrentDir();
      } catch (error) {
        if (error instanceof PermissionDeniedError) {
          serverOnline.value = true;
          noPermission.value = true;
        }
        if (error instanceof NotFoundError) {
          serverOnline.value = true;
          if (currentPath.value !== "/d") {
            goUp();
          }
        }
      }
    }
    (event.target as any)?.complete?.();
  }

  async function retryConnection() {
    await loadFiles();
  }

  async function handleRequestStorage() {
    console.info("[Files] Requesting storage permission");
    await requestStoragePermission();
    setTimeout(() => loadFiles(), 1500);
  }

  function navigateTo(path: string) {
    currentPath.value = path;
    searchQuery.value = "";
    searchResults.value = null;
    loadFiles();
  }

  function highlightFile(name: string) {
    if (!name) return;
    const target = files.value.find(f => f.name === name);
    if (!target) {
      console.info("[Files] highlightFile: target not found in current dir:", name);
      return;
    }
    highlightedPath.value = target.path;
    nextTick(() => {
      const el = document.querySelector<HTMLElement>(`ion-item[data-highlight-path="${CSS.escape(target.path)}"]`);
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "center" });
        console.info("[Files] highlightFile: scrolled to", target.path);
      } else {
        console.info("[Files] highlightFile: element not found for path:", target.path);
      }
    });
    if (highlightTimer) clearTimeout(highlightTimer);
    highlightTimer = setTimeout(() => {
      highlightedPath.value = null;
      highlightTimer = null;
    }, 2000);
  }

  function openContainingFolder(file: FileItem) {
    if (!file?.path) {
      // 搜索结果可能没有完整 path，防御性处理
      searchQuery.value = "";
      searchResults.value = null;
      return;
    }
    const parts = file.path.split("/").filter(Boolean);
    let parentDir: string;
    if (parts.length >= 2 && parts[0] === "d") {
      parentDir = "/" + parts.slice(0, 2).join("/");
    } else {
      parentDir = file.path.substring(0, file.path.lastIndexOf("/")) || MOUNT_ROOT;
    }
    searchQuery.value = "";
    searchResults.value = null;
    navigateTo(parentDir);
  }

  function goUp() {
    if (isMountRoot.value) return;
    if (!currentPath.value) {
      currentPath.value = MOUNT_ROOT;
      searchQuery.value = "";
      searchResults.value = null;
      loadFiles();
      return;
    }
    const parts = currentPath.value.split("/").filter(Boolean);
    if (parts.length === 2 && parts[0] === "d") {
      currentPath.value = MOUNT_ROOT;
    } else {
      parts.pop();
      currentPath.value = parts.length === 0 ? MOUNT_ROOT : "/" + parts.join("/");
    }
    searchQuery.value = "";
    searchResults.value = null;
    loadFiles();
  }

  async function handleFileClick(file: FileItem) {
    const clickResult = await findClickHandler(file);
    if (clickResult?.handled) {
      const cached = getSessionPassword(file.path);
      let password: string | undefined | null = cached;
      if (!password) {
        password = await promptPassword(file.name);
        if (!password) return;
      }
      setSessionPassword(file.path, password);
      if (isAlistEncrypted(file)) {
        await loadDecodedName(file, password);
      }
      const displayName = isAlistEncrypted(file) ? getDecodedName(file.path) || file.name : file.name;
      router.push({ path: "/player", query: { path: file.path, name: displayName, alistPath: file.path, alistPassword: password } });
      return;
    }

    if (file.isDirectory) {
      // 修复：搜索结果（递归）的文件夹可能在任意位置，必须用 file.path 而不是 currentPath + '/' + file.name。
      // 原写法对非当前目录的搜索结果文件夹会导航到错误路径导致 404 / render crash。
      const targetPath = file.path || (currentPath.value === "/d" ? "/d" : currentPath.value) + "/" + file.name;
      if (!file.path) {
        console.warn("[Files] Folder click missing path, falling back to currentPath + name", file);
      }
      navigateTo(targetPath);
      return;
    }

    if (isAlistEncrypted(file)) {
      const password = await promptPassword(file.name);
      if (!password) return;
      router.push({ path: "/player", query: { path: file.path, name: file.name, alistPath: file.path, alistPassword: password } });
      return;
    }

    if (file.isEncrypted) {
      router.push({
        path: "/tabs/preview",
        query: { path: file.path, name: file.name, isEncrypted: "true" },
      });
      return;
    }

    const category = getFileCategory(file.name);
    console.info("[Files] Click:", file.name, "category:", category);
    if (category === "video" || category === "audio") {
      playMedia(file, category);
    } else {
      router.push({
        path: "/tabs/preview",
        query: { path: file.path, name: file.name, isEncrypted: "false" },
      });
    }
  }

  // =============================================================================
  // 5) 搜索
  // =============================================================================

  function handleSearchInput() {
    const query = searchQuery.value.trim();
    if (!query) {
      searchGeneration++;
      searchResults.value = null;
      isSearching.value = false;
      fulltextBanner.value = null; // 清空 banner
      return;
    }
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => performSearch(), 300);
  }

  function handleSearchClear() {
    searchGeneration++;
    clearInput();
    searchResults.value = null;
    isSearching.value = false;
    fulltextBanner.value = null;
  }

  function handleSearchToggle() {
    if (searchQuery.value.trim()) {
      performSearch();
    }
  }

  /**
   * 🆕 A6：用户手动关闭 FTS 降级 banner（不重试 FTS，仅 UI 提示关闭）
   */
  function dismissFulltextBanner() {
    fulltextBanner.value = null;
  }

  /**
   * 🆕 2026-07-03：搜索空结果诊断卡片 - 重试按钮
   *   清空搜索结果缓存 + 重新执行搜索（避免缓存命中导致一直空）
   */
  function retrySearch() {
    // 清空该 query 的所有缓存（不同 fulltext 模式分别有缓存 key）
    for (const key of Array.from(searchCache.keys())) {
      if (key.includes(`:${searchQuery.value.trim()}:`)) {
        searchCache.delete(key);
      }
    }
    console.info("[Search] retry triggered by user", { query: searchQuery.value });
    performSearch();
  }

  /**
   * 🆕 2026-07-03：搜索空结果诊断卡片 - 跳转全文索引页
   *   直接 router.push（绕过三级导航 classList bug，直接打开独立路由）
   *   路径：/tabs/settings/fulltext-index（router/index.ts 已注册）
   */
  function goFullTextIndexFromSearch() {
    console.info("[Search] navigate to fulltext-index page from empty state");
    router.push("/tabs/settings/fulltext-index");
  }

  /**
   * 🆕 2026-07-02 重写：插入 symbol span 到 contenteditable div 的光标位置。
   *
   * 之前：插入 " AND " 字符串（用户强烈反对：要求插入的是符号 ＆，不是英文 AND）
   * 现在：调 useSearchInput.insertSymbol，插入 `<span data-kind="op" data-op="AND">＆</span>`
   *
   * 触发：useSearchInput.insertSymbol 自动 syncFromDiv → searchQuery.value 更新 → 触发 performSearch
   */
  function insertOperator(op: string) {
    // op 是 opKey（'AND' | 'OR' | 'NOT' | '__phrase_open__' | '__regex_prefix__'）
    insertSymbol(op);
  }

  /**
   * 🆕 2026-07-03 优化2：把 FTS5 布尔查询转换为向量搜索关键词。
   *
   * 应用场景（用户原话）：
   *   "即使搜索有逻辑符 FTS 搜索没有匹配结果，结果也不应当为空。
   *    同样遵守增量合并原则，以及匹配过少智能贪婪，转换逻辑符进行普通向量搜索。"
   *
   * 转换规则（与后端 internal/fts/query.go 语法对齐）：
   *   - 在线 AND 高清       → "在线 高清"      （AND 视为空格，两个词都保留）
   *   - 在线 OR 播放        → "在线 播放"       （OR 视为空格，两个词都保留，反正向量搜有相关度排序）
   *   - 在线 NOT 视频       → "在线"           （NOT 后的词丢弃）
   *   - "exact phrase" 高清 → "exact phrase 高清" （去引号，phrase 当作单个关键词）
   *   - regex:^photo.*      → "photo"          （去 regex: 前缀和 /.../ 边界，提取词项）
   *   - 在线 AND (高清 OR 视频) → "在线 高清 视频"（去括号，扁平化）
   *
   * 边界处理：
   *   - 用户原始 query 没有布尔语法 → 返回原 query（fallback 不会触发，但函数本身安全）
   *   - cleanedQuery 为空（如 "NOT 视频"）→ 返回空字符串，调用方需判空
   *   - 转义双引号 \" 也按双引号处理
   */
  function convertBooleanQueryToVectorKeywords(query: string): string {
    let s = query;
    // 1. 去 regex: 前缀（regex:^foo 或 regex:/^foo/），保留词项
    s = s.replace(/\bregex:\S+/g, " ");
    // 2. 去 /pattern/ 边界斜杠，保留 pattern 内的词
    s = s.replace(/\/([^/\s]+)\//g, " $1 ");
    // 3. 去所有双引号（含转义 \"），phrase 内容当作普通词
    s = s.replace(/\\"/g, " ").replace(/"/g, " ");
    // 4. 去括号
    s = s.replace(/[()]/g, " ");
    // 5. 切词，丢 AND/OR，丢 NOT 后的下一个词
    const tokens = s.split(/\s+/).filter(t => t.length > 0);
    const keywords: string[] = [];
    let skipNext = false;
    for (const tok of tokens) {
      if (tok === "NOT") {
        skipNext = true;
        continue;
      }
      if (tok === "AND" || tok === "OR") {
        continue;
      }
      if (skipNext) {
        skipNext = false;
        continue;
      }
      keywords.push(tok);
    }
    return keywords.join(" ");
  }

  async function performSearch() {
    const query = searchQuery.value.trim();
    if (!query) return;
    if (selectedPlugin.value) return;

    if (mainContentRef.value?.$el) {
      const scrollEl = mainContentRef.value.$el;
      if (scrollEl && scrollEl.scrollTop !== undefined) {
        lastScrollTop.value = scrollEl.scrollTop;
      }
    }

    // 🐛 2026-07-02 修复：插入逻辑符（AND/OR/NOT/phrase/regex）后搜索无结果
    //   根因：普通搜索（searchFilesVector）不支持布尔语法，只有 FTS 全文搜索支持。
    //   修复：检测到 query 包含布尔操作符时，自动启用全文搜索。
    const hasBooleanSyntax = /\b(AND|OR|NOT)\b|^regex:|"/.test(query);
    const useFullText = searchFullText.value || hasBooleanSyntax;

    const clientHits = clientFilterFiles(lastFullResults.value, query);
    if (clientHits.length > 0) {
      searchResults.value = clientHits;
      restoreScrollTop();
    }

    if (isSearching.value) {
      searchGeneration++;
    }
    const gen = ++searchGeneration;

    const cacheKey = `${currentPath.value}:${query}:fulltext=${useFullText}`;
    const cached = searchCache.get(cacheKey);
    if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
      if (gen !== searchGeneration) return;
      searchResults.value = cached.results;
      lastFullResults.value = cached.results;
      restoreScrollTop();
      return;
    }

    isSearching.value = clientHits.length === 0;
    // 🆕 2026-07-03：搜索流程打日志（hijackConsole 已 hook，会自动镜像到 DevLogs frontend tab）
    //   背景：用户反馈"devlogs 也没有日志输出"——根因是搜索代码完全没打 console，不是 DevLogs 坏了
    console.info("[Search] performSearch start", {
      query,
      useFullText,
      hasBooleanSyntax,
      path: currentPath.value,
      vectorSearch: vectorSearchStatus.value,
    });
    try {
      // A4 设计：普通搜索（name + vector）始终跑，FTS 是增量增强
      let normalResults: FileItem[] = [];
      let mode: SearchMode = "none";
      try {
        const vecResult = await searchFilesVector(currentPath.value, query, true, 200);
        normalResults = vecResult.results;
        mode = vecResult.search_mode;
      } catch (e) {
        console.warn("[Search] searchFilesVector failed, fallback to searchFiles", e);
        normalResults = await searchFiles(currentPath.value, query, true);
        mode = "none";
      }

      if (gen !== searchGeneration) return;

      // A4：FTS 关闭时只显示普通结果（但如果 query 有布尔语法，自动走 FTS）
      if (!useFullText) {
        searchResults.value = normalResults;
        lastFullResults.value = normalResults;
        searchMode.value = mode;
        fulltextBanner.value = null;
        console.info("[Search] normal mode done", { count: normalResults.length, mode });
      } else {
        // A4：FTS 开启 → merge + 去重（普通结果 ∪ FTS 结果，按 path 去重）
        // A6：FTS 失败时静默降级 + banner 提示（不破坏普通结果）
        let ftsResults: FileItem[] = [];
        let ftsAvailable = true;
        let ftsErrorMessage = "";
        let ftsDbEngine: "sqlite" | "libsql" | "none" = "none";
        try {
          // 包成 phrase 防 AND/OR/NOT 被误解析（仅当用户没手动写布尔语法时）
          let ftsQuery = query;
          if (!hasBooleanSyntax) {
            if (!query.includes('"') && !query.toLowerCase().startsWith("regex:")) {
              ftsQuery = `"${query.replace(/"/g, '\\"')}"`;
            }
          }
          const ftResult = await searchFilesFullText(ftsQuery, 200, currentPath.value);
          ftsResults = ftResult.results;
          ftsDbEngine = ftResult.dbEngine;
          if (ftResult.dbEngine === "none") {
            ftsAvailable = false;
            ftsErrorMessage = "全文索引未初始化";
          }
          console.info("[Search] FTS query done", {
            ftsQuery,
            ftsCount: ftsResults.length,
            dbEngine: ftResult.dbEngine,
            indexSize: ftResult.indexSize,
          });
        } catch (e) {
          ftsAvailable = false;
          ftsErrorMessage = e instanceof Error ? e.message : String(e);
          console.warn("[Search] FTS query failed", {
            error: ftsErrorMessage,
            query,
          });
        }

        if (gen !== searchGeneration) return;

        // 去重：normal + fts，按 path
        const seen = new Set<string>();
        const merged: FileItem[] = [];
        for (const f of normalResults) {
          if (!seen.has(f.path)) {
            seen.add(f.path);
            merged.push(f);
          }
        }
        for (const f of ftsResults) {
          if (!seen.has(f.path)) {
            seen.add(f.path);
            merged.push({ ...f, _fulltextHit: true } as FileItem);
          }
        }

        searchResults.value = merged;
        lastFullResults.value = merged;
        searchMode.value = mode;

        // 🆕 2026-07-03 优化2：FTS + 普通 都为 0 时，转换布尔查询为关键词重试向量搜索
        //   用户原话：「即使搜索有逻辑符 FTS 搜索没有匹配结果，结果也不应当为空」
        //             「合并结果为0才触发贪婪」「都要做，反正有相关度排序」
        //
        // 触发条件：merged.length === 0 且 query 含布尔语法
        //   - 把 "在线 AND 高清" → "在线 高清"（NOT 后的词丢弃，regex 前缀去掉，引号去掉）
        //   - 用 cleanedQuery 重跑向量搜索
        //   - 命中：标记 searchMode='greedy' 让 UI 显示橙色"语义近似"banner
        //   - 仍 0：标记 searchMode='greedy' 让用户看到"已尝试贪婪匹配"
        //
        // 为什么不直接走后端 greedy 模式？
        //   - 后端 greedy 是关键词 0 命中时 bigram 放宽，但传入的 query 含 AND/OR/NOT 大写词
        //     会被后端当普通关键词处理 → 命中文件名含 "AND" 字符的文件 → 干扰结果
        //   - 必须先在前端清洗 query 才能正确触发后端向量搜索
        if (merged.length === 0 && hasBooleanSyntax) {
          const cleanedQuery = convertBooleanQueryToVectorKeywords(query);
          console.info("[Search] FTS+vector empty, retry with cleaned keywords", {
            originalQuery: query,
            cleanedQuery,
          });
          if (cleanedQuery && cleanedQuery !== query) {
            try {
              const retryVec = await searchFilesVector(currentPath.value, cleanedQuery, true, 200);
              if (gen !== searchGeneration) return;
              if (retryVec.results.length > 0) {
                // 命中：用清洗后关键词的向量结果（语义近似，非精确布尔匹配）
                searchResults.value = retryVec.results;
                lastFullResults.value = retryVec.results;
                searchMode.value = "greedy"; // 触发橙色 banner
                console.info("[Search] retry vector found results via cleaned keywords", {
                  count: retryVec.results.length,
                  backendMode: retryVec.search_mode,
                });
              } else {
                // 仍 0：标记 greedy 让 UI 显示"已尝试贪婪匹配但无结果"
                searchMode.value = "greedy";
                console.info("[Search] retry vector still empty, set greedy mode for UI");
              }
            } catch (e) {
              console.warn("[Search] retry vector failed", e);
              searchMode.value = "greedy";
            }
          } else {
            // cleanedQuery 为空（如 "NOT 视频"）或等于原 query：仍标 greedy 让用户知道已尝试
            searchMode.value = "greedy";
          }
        }

        // A6 banner：FTS 不可用时静默降级 + 提示原因
        if (!ftsAvailable) {
          fulltextBanner.value = {
            type: ftsErrorMessage.includes("not initialized") ? "unavailable" : "error",
            message: `全文搜索不可用：${ftsErrorMessage}，已降级到普通搜索`,
          };
        } else {
          fulltextBanner.value = null;
        }
        console.info("[Search] FTS merge done", {
          normalCount: normalResults.length,
          ftsCount: ftsResults.length,
          mergedCount: merged.length,
          ftsAvailable,
          ftsDbEngine,
        });
      }

      if (searchResults.value.length > 0 && sortBy.value !== "relevance") {
        sortBy.value = "relevance";
        sortDesc.value = true;
      }
      searchCache.set(cacheKey, { timestamp: Date.now(), results: searchResults.value });
      restoreScrollTop();

      // 🆕 2026-07-03：搜索结果为空时主动查询诊断信息（FTS 状态 + 索引统计）
      //   UI 会显示诊断卡片，让用户知道"为什么没结果"（FTS 不可用？索引为空？正在索引？）
      if (searchResults.value.length === 0) {
        console.info("[Search] empty results, fetching diagnostics for UI", {
          query,
          useFullText,
          mode: searchMode.value,
        });
        await refreshSearchDiagnostics({
          query,
          mode: searchMode.value,
          normalCount: normalResults.length,
          ftsCount: useFullText ? (searchResults.value.length as number) : undefined,
        });
      }
    } catch (e) {
      console.warn("[Search] performSearch failed", { query, error: e });
      if (gen !== searchGeneration) return;
      searchResults.value = [];
      searchMode.value = "none";
      fulltextBanner.value = null;
      // 🆕 2026-07-03：搜索失败时也查询诊断信息，便于用户排查
      await refreshSearchDiagnostics({ query, mode: "none" }).catch(() => {});
    }
    isSearching.value = false;
  }

  // =============================================================================
  // 6) 长按菜单（action sheet）
  // =============================================================================

  async function handleLongPress(file: FileItem) {
    const category = file.isDirectory ? "directory" : getFileCategory(file.name);
    const buttons: any[] = [];

    buttons.push({
      text: t("files.info"),
      icon: informationCircle,
      cssClass: "action-section-view",
      handler: () => {
        router.push({ path: "/tabs/file-info", query: { path: file.path, name: file.name } });
      },
    });

    if (file.isDirectory) {
      buttons.push({
        text: t("files.open"),
        icon: folderOpen,
        cssClass: "action-section-view",
        handler: () => {
          const base = currentPath.value === "/d" ? "/d" : currentPath.value;
          const newPath = base + "/" + file.name;
          navigateTo(newPath);
        },
      });
    } else if (file.isEncrypted) {
      buttons.push({
        text: t("files.preview"),
        icon: eyeOutline,
        cssClass: "action-section-view",
        handler: () => {
          router.push({
            path: "/tabs/preview",
            query: { path: file.path, name: file.name, isEncrypted: "true" },
          });
        },
      });
    } else {
      const isMedia = category === "video" || category === "audio";

      const featureActions = await getAllActions(file);
      for (const fa of featureActions) {
        buttons.push({
          text: fa.text(),
          icon: fa.icon,
          cssClass: "action-section-view",
          ...(fa.color ? { role: undefined, cssClass: `action-section-view action-color-${fa.color}` } : {}),
          handler: () => {
            fa.handler(file);
          },
        });
      }

      buttons.push({
        text: isMedia ? t("files.play") : t("files.preview"),
        icon: isMedia ? playCircle : eyeOutline,
        cssClass: "action-section-view",
        handler: () => {
          if (isMedia) {
            playMedia(file, category);
          } else {
            router.push({
              path: "/tabs/preview",
              query: { path: file.path, name: file.name, isEncrypted: "false" },
            });
          }
        },
      });
    }

    buttons.push({
      text: "重命名",
      icon: createOutline,
      cssClass: "action-section-manage",
      handler: () => {
        selectedFile.value = file;
        renameValue.value = file.name;
        renamePassword.value = "";
        showRenameDialog.value = true;
      },
    });
    buttons.push({
      text: "复制",
      icon: copyOutline,
      cssClass: "action-section-manage",
      handler: () => {
        handleCopy(file);
      },
    });
    buttons.push({
      text: "移动",
      icon: arrowForwardOutline,
      cssClass: "action-section-manage",
      handler: () => {
        selectedFile.value = file;
        moveTargetPath.value = currentPath.value;
        showMoveDialog.value = true;
      },
    });
    buttons.push({
      text: "分享",
      icon: shareOutline,
      cssClass: "action-section-manage",
      handler: () => {
        handleShare(file);
      },
    });
    buttons.push({
      text: "标签管理",
      icon: pricetagOutline,
      cssClass: "action-section-manage",
      handler: async () => {
        selectedFile.value = file;
        newTagInput.value = "";
        editingFileTags.value = [];
        showTagDialog.value = true;
        try {
          const allTags = await fetchTags();
          editingFileTags.value = allTags
            .filter(t => t.count > 0)
            .map(t => t.name)
            .slice(0, 10);
        } catch {}
      },
    });

    buttons.push({
      text: t("files.delete"),
      icon: trash,
      role: "destructive",
      cssClass: "action-section-danger",
      handler: () => {
        handleDeleteFile(file);
      },
    });

    buttons.push({
      text: t("files.cancelSelect"),
      role: "cancel",
    });

    const actionSheet = await actionSheetController.create({
      header: file.name,
      buttons,
      cssClass: "file-action-sheet",
    });
    await actionSheet.present();
  }

  // =============================================================================
  // 7) 文件操作（copy / rename / move / share / delete / tag）
  // =============================================================================

  async function handleCopy(file: FileItem) {
    const baseName = file.name.replace(/\.[^.]+$/, "");
    const ext = file.name.includes(".") ? "." + file.name.split(".").pop() : "";
    const destName = `${baseName}_copy${ext}`;
    const destPath = currentPath.value === "/d" ? `/d/${destName}` : `${currentPath.value}/${destName}`;
    try {
      await copyFile(file.path, destPath);
      showToast({ message: t("tasks.copy") + " " + t("tasks.taskCreated"), duration: 1500, color: "success" });
    } catch (err: any) {
      showToast({ message: err.message || "Copy failed", duration: 2000, color: "danger" });
    }
  }

  function onRenameConfirm(d: any) {
    renameValue.value = d.name ?? renameValue.value;
    renamePassword.value = d.password ?? "";
    if (selectedFile.value) handleRename(selectedFile.value);
  }

  async function handleRename(file: FileItem) {
    if (!renameValue.value.trim() || renameValue.value === file.name) return;
    try {
      if (file.isEncrypted) {
        const result = await renameOriginalName(file.path, renameValue.value.trim(), renamePassword.value.trim() || undefined);
        if (result.success) {
          showToast({ message: "原始文件名已更新", duration: 1500, color: "success" });
        }
      } else {
        await renameFile(file.path, renameValue.value.trim());
        showToast({ message: t("tasks.rename") + " " + t("tasks.taskCreated"), duration: 1500, color: "success" });
      }
      showRenameDialog.value = false;
      renamePassword.value = "";
      if (file.isEncrypted) {
        await loadFiles();
      }
    } catch (err: any) {
      showToast({ message: err.message || "Rename failed", duration: 2000, color: "danger" });
    }
  }

  async function handleMove(file: FileItem) {
    if (!moveTargetPath.value || moveTargetPath.value === file.path) return;
    const destPath = moveTargetPath.value.endsWith("/") ? `${moveTargetPath.value}${file.name}` : `${moveTargetPath.value}/${file.name}`;
    try {
      await moveFile(file.path, destPath);
      showMoveDialog.value = false;
      showToast({ message: t("tasks.move") + " " + t("tasks.taskCreated"), duration: 1500, color: "success" });
    } catch (err: any) {
      showToast({ message: err.message || "Move failed", duration: 2000, color: "danger" });
    }
  }

  async function handleShare(file: FileItem) {
    if (isNative()) {
      try {
        const localPath = await getLocalFilePath(file.path);
        if (localPath) {
          await Share.share({ title: file.name, url: "file://" + localPath });
        } else {
          showToast({ message: "仅支持本地文件分享", duration: 2500, color: "warning" });
        }
      } catch (_e) {
        showToast({ message: "分享失败或已取消" });
      }
    } else {
      copyToClipboard(getExternalStreamUrl(file.path)).then(ok =>
        showToast({ message: ok ? "链接已复制到剪贴板" : "复制失败", color: ok ? "success" : "danger" })
      );
    }
  }

  const fileInputRef = ref<HTMLInputElement>();

  function handleUpload() {
    fileInputRef.value?.click();
  }

  async function handleFileSelected(event: Event) {
    const input = event.target as HTMLInputElement;
    if (!input.files?.length) return;

    const filesToUpload = Array.from(input.files);
    let successCount = 0;
    let failCount = 0;

    for (const file of filesToUpload) {
      try {
        showToast({ message: `正在上传: ${file.name}...`, color: "primary", duration: 2000 });
        await uploadFile(currentPath.value, file);
        successCount++;
      } catch (e) {
        console.error("[Files] upload failed:", file.name, e instanceof Error ? `${e.name}: ${e.message}` : String(e));
        failCount++;
      }
    }

    if (successCount > 0) {
      showToast({
        message: `成功上传 ${successCount} 个文件${failCount > 0 ? `，${failCount} 个失败` : ""}`,
        color: failCount > 0 ? "warning" : "success",
        duration: 3000,
      });
      await loadFiles();
    }

    input.value = "";
  }

  async function handleAddNewTag() {
    if (!selectedFile.value || !newTagInput.value.trim()) return;
    const tag = newTagInput.value.trim();
    if (editingFileTags.value.includes(tag)) {
      newTagInput.value = "";
      return;
    }
    try {
      await addTag(selectedFile.value.path, tag);
      editingFileTags.value.push(tag);
      newTagInput.value = "";
    } catch (_e) {
      showToast({ message: "添加标签失败" });
    }
  }

  async function handleRemoveTag(tag: string) {
    if (!selectedFile.value) return;
    try {
      await removeTag(selectedFile.value.path, tag);
      editingFileTags.value = editingFileTags.value.filter(t => t !== tag);
    } catch (_e) {
      showToast({ message: "移除标签失败" });
    }
  }

  async function loadFileTagsForCurrentDir() {
    try {
      const allTags = await fetchTags();
      const map: Record<string, string[]> = {};
      for (const tag of allTags) {
        if (tag.count > 0) {
          for (const f of files.value) {
            if (!map[f.path]) map[f.path] = [];
            map[f.path].push(tag.name);
          }
        }
      }
      fileTagMap.value = map;
    } catch {}

    const badgesMap: Record<string, any[]> = {};
    const subtitlesMap: Record<string, any[]> = {};
    for (const f of files.value) {
      const badges = await getBadges(f);
      if (badges.length > 0) badgesMap[f.path] = badges;
      const subs = await getSubtitles(f);
      if (subs.length > 0) subtitlesMap[f.path] = subs;
    }
    fileBadges.value = badgesMap;
    fileSubtitles.value = subtitlesMap;

    preloadSubtitles(files.value);
    setupLazyThumbnails();
  }

  async function handleDeleteFile(file: FileItem) {
    if (file.path === "/" || file.path === "") {
      showToast({ message: "不能删除根目录", duration: 2000, color: "danger" });
      return;
    }

    if (file.isDirectory) {
      let detail = "此操作不可撤销";
      try {
        const list = await listFiles(file.path);
        const filesInDir = list.filter((f: FileItem) => !f.isDirectory).length;
        const subDirs = list.filter((f: FileItem) => f.isDirectory).length;
        detail = `包含 ${filesInDir} 个文件 + ${subDirs} 个子目录，此操作不可撤销。`;
      } catch (e) {
        console.warn("[Files] list directory failed before delete:", file.path, e);
      }
      const dirAlert = await alertController.create({
        header: t("files.delete"),
        subHeader: `📁 ${file.name}`,
        message: `确认删除文件夹 "${file.name}" 及其所有内容？\n\n${detail}`,
        buttons: [
          { text: t("files.cancelSelect"), role: "cancel" },
          { text: t("files.delete"), role: "destructive", handler: () => doDelete(file) },
        ],
      });
      await dirAlert.present();
    } else {
      const alert = await alertController.create({
        header: t("files.delete"),
        message: t("files.deleteConfirm", { name: file.name }),
        buttons: [
          { text: t("files.cancelSelect"), role: "cancel" },
          { text: t("files.delete"), role: "destructive", handler: () => doDelete(file) },
        ],
      });
      await alert.present();
    }
  }

  async function doDelete(file: FileItem) {
    try {
      await deleteFile(file.path);
      showToast({ message: t("tasks.delete") + " " + t("tasks.taskCreated"), duration: 1500, color: "success" });
    } catch (err: any) {
      showToast({ message: err.message || "Delete failed", duration: 2000, color: "danger" });
    }
  }

  // =============================================================================
  // 8) file:change 增量更新
  // =============================================================================

  let fileChangeDebounceTimer: number | null = null;
  const pendingFileChanges = new Map<string, "create" | "delete" | "modify">();

  function onFileChange(payload: { path: string; action: "create" | "delete" | "modify" }) {
    searchCache.clear();
    pendingFileChanges.set(payload.path, payload.action);
    if (fileChangeDebounceTimer !== null) {
      clearTimeout(fileChangeDebounceTimer);
    }
    fileChangeDebounceTimer = window.setTimeout(() => {
      fileChangeDebounceTimer = null;
      if (isStreamLoading) {
        console.info("[Files] file:change deferred, stream loading", pendingFileChanges.size, "changes");
        return;
      }
      void applyFileChanges();
    }, 300);
  }

  async function applyFileChanges() {
    if (pendingFileChanges.size === 0) return;
    const changes = new Map(pendingFileChanges);
    pendingFileChanges.clear();

    for (const [path, action] of changes) {
      if (action === "delete") {
        const idx = files.value.findIndex(f => f.path === path);
        if (idx >= 0) {
          files.value.splice(idx, 1);
          console.info("[Files] incremental delete:", path);
        }
      }
    }

    const visiblePaths: string[] = [];
    for (const [path, action] of changes) {
      if (action === "delete") continue;
      const parent = path.substring(0, path.lastIndexOf("/")) || "/d";
      if (parent !== currentPath.value) continue;
      visiblePaths.push(path);
    }
    if (visiblePaths.length === 0) return;

    const results = await Promise.allSettled(visiblePaths.map(p => getFileInfo(p)));
    for (let i = 0; i < results.length; i++) {
      const r = results[i];
      const path = visiblePaths[i];
      if (r.status === "fulfilled") {
        const fileItem = r.value;
        const idx = files.value.findIndex(f => f.path === path);
        if (idx >= 0) {
          files.value[idx] = fileItem;
          console.info("[Files] incremental modify:", path);
        } else {
          files.value.push(fileItem);
          console.info("[Files] incremental create:", path);
        }
      } else {
        const idx = files.value.findIndex(f => f.path === path);
        if (idx >= 0) {
          files.value.splice(idx, 1);
          console.info("[Files] incremental remove (fetch failed):", path);
        }
      }
    }
  }

  // =============================================================================
  // 9) 侧边栏 / Plugins / Tags
  // =============================================================================

  async function loadPlugins() {
    try {
      plugins.value = await fetchPlugins();
    } catch {}
  }
  async function loadTags() {
    try {
      tags.value = await fetchTags();
    } catch {}
  }

  function openPluginView(plugin: PluginMeta) {
    files.value = [];
    loading.value = true;
    pluginLoaded.value = false;
    selectedPlugin.value = plugin;
    menuController.close();
  }

  async function exitPluginMode() {
    selectedPlugin.value = null;
    await menuController.close();
    files.value = [];
    loading.value = true;
    await loadFiles();
  }

  async function openSideDrawer() {
    await menuController.open("plugin-menu");
  }

  function getPluginIcon(plugin: PluginMeta): any {
    const featureIcon = getFeatureIcon(plugin.name);
    if (featureIcon) return featureIcon;
    const icons: Record<string, string> = {
      video: filmOutline,
      audio: musicalNotesOutline,
      image: imageOutline,
      pdf: documentTextOutline,
      text: documentOutline,
      wps: documentOutline,
    };
    return icons[plugin.name] || lockClosed;
  }

  async function searchPluginFiles(plugin: PluginMeta, onItem?: (file: FileItem) => void): Promise<FileItem[]> {
    if (!plugin.supportedExtensions || plugin.supportedExtensions.length === 0) return [];
    const result = await listPluginFilesStream(currentPath.value, plugin.supportedExtensions, file => {
      onItem?.(file);
    });
    return result.files;
  }

  async function handleTagFilter(tagName: string) {
    menuController.close();
    files.value = [];
    loading.value = true;
    selectedPlugin.value = null;
    try {
      files.value = await listFilesByTag(tagName, currentPath.value);
      loadFileTagsForCurrentDir();
    } catch (e) {
      showToast({ message: `筛选失败: ${e}` });
    } finally {
      loading.value = false;
    }
  }

  // =============================================================================
  // 10) Plugin view state（筛选 / 排序 / 切换）
  // =============================================================================

  const pluginTab = ref<"source" | "container">("source");
  const pluginFiles = ref<FileItem[]>([]);
  const pluginLoaded = ref(false);
  let pluginLoadGeneration = 0;

  const sizeFilterMin = ref<number | null>(null);
  const sizeFilterMax = ref<number | null>(null);
  const timeFilterFrom = ref<string | null>(null);
  const timeFilterTo = ref<string | null>(null);
  const showPluginFilters = ref(false);

  const pluginSortBy = ref<"name" | "size" | "time">("name");
  const pluginSortDesc = ref(false);

  const pluginSortLabel = computed(() => {
    const map: Record<string, string> = { name: "名称", size: "大小", time: "时间" };
    return (map[pluginSortBy.value] || "名称") + (pluginSortDesc.value ? "↓" : "↑");
  });

  const activeFilterCount = computed(() => {
    let c = 0;
    if (sizeFilterMin.value !== null) c++;
    if (sizeFilterMax.value !== null) c++;
    if (timeFilterFrom.value !== null) c++;
    if (timeFilterTo.value !== null) c++;
    return c;
  });

  function applySizePreset(preset: (typeof SIZE_PRESETS)[number]) {
    sizeFilterMin.value = "min" in preset ? ((preset as { min?: number }).min ?? null) : null;
    sizeFilterMax.value = "max" in preset ? ((preset as { max?: number }).max ?? null) : null;
  }
  function applyTimePreset(preset: (typeof TIME_PRESETS)[number]) {
    const now = new Date();
    const from = new Date(now);
    from.setDate(from.getDate() - preset.days);
    from.setHours(0, 0, 0, 0);
    timeFilterFrom.value = formatDateInput(from);
    if (preset.days === 0) {
      timeFilterTo.value = formatDateInput(now);
    } else {
      timeFilterTo.value = null;
    }
  }

  function clearAllPluginFilters() {
    sizeFilterMin.value = null;
    sizeFilterMax.value = null;
    timeFilterFrom.value = null;
    timeFilterTo.value = null;
    pluginSortBy.value = "name";
    pluginSortDesc.value = false;
  }

  const filteredPluginFiles = computed(() => {
    if (!selectedPlugin.value) return [];
    let list: FileItem[];
    if (pluginTab.value === "container") {
      list = pluginFiles.value.filter(f => isAnyContainerFile(f));
    } else {
      list = pluginFiles.value.filter(f => !isAnyContainerFile(f));
    }
    const query = searchQuery.value.trim().toLowerCase();
    if (query) {
      list = list.filter(f => f.name.toLowerCase().includes(query));
    }
    if (sizeFilterMin.value !== null) {
      list = list.filter(f => (f.size || 0) >= sizeFilterMin.value!);
    }
    if (sizeFilterMax.value !== null) {
      list = list.filter(f => (f.size || 0) <= sizeFilterMax.value!);
    }
    if (timeFilterFrom.value !== null) {
      const from = new Date(timeFilterFrom.value).getTime();
      list = list.filter(f => (f.modified ? new Date(f.modified).getTime() : 0) >= from);
    }
    if (timeFilterTo.value !== null) {
      const to = new Date(timeFilterTo.value).getTime();
      list = list.filter(f => (f.modified ? new Date(f.modified).getTime() : 0) <= to);
    }
    list.sort((a, b) => {
      if (a.isDirectory && !b.isDirectory) return -1;
      if (!a.isDirectory && b.isDirectory) return 1;
      let cmp = 0;
      switch (pluginSortBy.value) {
        case "name":
          cmp = a.name.localeCompare(b.name);
          break;
        case "size":
          cmp = (a.size || 0) - (b.size || 0);
          break;
        case "time":
          cmp = (Number(a.modified) || 0) - (Number(b.modified) || 0);
          break;
      }
      return pluginSortDesc.value ? -cmp : cmp;
    });
    const tagMap = fileTagMap.value;
    return list.map(f => ({ ...f, _tags: tagMap[f.path] || [] }));
  });

  watch(selectedPlugin, async plugin => {
    if (plugin) {
      const gen = ++pluginLoadGeneration;
      pluginTab.value = "source";
      pluginLoaded.value = false;
      pluginFiles.value = [];
      console.info("[Files] Loading plugin files (stream):", plugin.name);
      try {
        const results = await searchPluginFiles(plugin, file => {
          if (gen !== pluginLoadGeneration) return;
          pluginFiles.value.push(file);
          if (pluginFiles.value.length === 1 && !pluginLoaded.value) {
            console.info("[Files] First plugin item arrived, UI unlocked");
          }
        });
        if (gen !== pluginLoadGeneration) return;
        pluginFiles.value = results;
      } catch (e) {
        console.debug("[Files] Plugin stream load failed:", e);
      }
      if (gen === pluginLoadGeneration) {
        pluginLoaded.value = true;
        setupLazyThumbnails();
      }
    }
  });

  // =============================================================================
  // 11) Lifecycle (onMounted / onIonViewWillEnter / onUnmounted)
  // =============================================================================

  function onBackendReady(data: { port?: number; running?: boolean }) {
    if (data.running || data.port) {
      loadFiles();
    }
  }

  function onBackendReadyWindow(event: Event) {
    const detail = (event as CustomEvent).detail || {};
    onBackendReady(detail);
  }

  onMounted(() => {
    loadFiles();
    loadPlugins();
    loadTags();
    eventBus.on("file:change", onFileChange);
    window.addEventListener("encv:backend-ready", onBackendReadyWindow as EventListener);
    useRealtimeTransport().setFileChangeGate(true, () => {
      if (fileChangeDebounceTimer !== null) {
        clearTimeout(fileChangeDebounceTimer);
        fileChangeDebounceTimer = null;
      }
      loadFiles();
    });
    if (import.meta.env.DEV) {
      import("@encv/shared-components/composables/useTestBackdoor").then(({ useTestBackdoor }) => {
        import("@encv/shared-components/composables/useNewTaskModal").then(({ useNewTaskModal: createNewTaskModal }) => {
          const { openNewTask } = createNewTaskModal();
          useTestBackdoor(files, {
            onLongPress: handleLongPress,
            onClick: handleFileClick,
            navigateTo: navigateTo,
            openNewTask: (sourcePath?: string, taskType?: "encrypt" | "decrypt") => {
              return openNewTask(sourcePath, taskType);
            },
            __debugOnFileChange: onFileChange,
            __debugGetPendingChanges: () => pendingFileChanges.size,
            __debugIsStreamLoading: () => isStreamLoading,
          });
        });
      });
    }
  });

  onIonViewWillEnter(() => {
    const qPath = typeof route.query.path === "string" ? route.query.path : "";
    const qHighlight = typeof route.query.highlight === "string" ? route.query.highlight : "";

    if (qPath && qHighlight) {
      if (qPath !== currentPath.value) {
        pendingHighlight.value = qHighlight;
        currentPath.value = qPath;
        searchQuery.value = "";
        searchResults.value = null;
        loadFiles();
      } else {
        highlightFile(qHighlight);
      }
      router.replace({ path: route.path, query: {} });
      return;
    }

    if (files.value.length === 0 && !loading.value && !connecting.value) {
      loadFiles();
    }
  });

  onUnmounted(() => {
    eventBus.off("file:change", onFileChange);
    window.removeEventListener("encv:backend-ready", onBackendReadyWindow as EventListener);
    if (searchTimer) clearTimeout(searchTimer);
    if (highlightTimer) {
      clearTimeout(highlightTimer);
      highlightTimer = null;
    }
    if (fileChangeDebounceTimer !== null) {
      clearTimeout(fileChangeDebounceTimer);
      fileChangeDebounceTimer = null;
    }
  });

  watch(
    () => route.path,
    newPath => {
      const isFilesActive = newPath.startsWith("/tabs/files");
      useRealtimeTransport().setFileChangeGate(isFilesActive, () => {
        if (fileChangeDebounceTimer !== null) {
          clearTimeout(fileChangeDebounceTimer);
          fileChangeDebounceTimer = null;
        }
        loadFiles();
      });
    },
    { immediate: true }
  );

  // =============================================================================
  // 12) Return：Files.vue 模板需要的所有 state + handlers
  // =============================================================================

  // 断言 return 类型为 UseFilesViewReturn（关键字段 required，非 declared 字段靠 index signature）
  // 避免 vue-tsc 推断时丢字段
  return {
    // refs (state)
    playError,
    playErrorDetail,
    playErrorFile,
    t,
    vectorSearchStatus,
    thumbnailUrls,
    onThumbError,
    sortBy,
    sortDesc,
    showMainSort,
    mainSortLabel,
    serverOnline,
    noPermission,
    files,
    plugins,
    tags,
    showRenameDialog,
    showTagDialog,
    showMoveDialog,
    selectedPlugin,
    selectedFile,
    renameValue,
    renamePassword,
    moveTargetPath,
    editingFileTags,
    newTagInput,
    fileTagMap,
    fileBadges,
    fileSubtitles,
    renameAlertInputs,
    currentPath,
    loading,
    refreshing,
    connecting,
    pendingHighlight,
    highlightedPath,
    isMountRoot,
    searchQuery,
    searchFullText,
    searchResults,
    isSearching,
    searchMode,
    fulltextBanner, // 🆕 A6: FTS 降级 banner
    searchDiagnostics, // 🆕 2026-07-03: 搜索空结果时的诊断信息（FTS 状态 + 索引统计）
    refreshSearchDiagnostics, // 🆕 2026-07-03: 手动刷新诊断信息（重试按钮用）
    retrySearch, // 🆕 2026-07-03: 重试搜索（清缓存 + 重新 performSearch）
    goFullTextIndexFromSearch, // 🆕 2026-07-03: 跳转全文索引页（绕过三级导航 classList bug）
    queryInputRef, // 🆕 A3: 绑到 <div contenteditable> 的 ref
    onQueryInput, // 🆕 A3: @input 处理器
    onQueryKeydown, // 🆕 A3: @keydown 处理器
    renderSnippet,
    tokenizeQuery,
    mainContentRef,
    pathSegments,
    displayFiles,
    fileInputRef,
    pluginTab,
    pluginFiles,
    pluginLoaded,
    sizeFilterMin,
    sizeFilterMax,
    timeFilterFrom,
    timeFilterTo,
    showPluginFilters,
    pluginSortBy,
    pluginSortDesc,
    pluginSortLabel,
    activeFilterCount,
    filteredPluginFiles,
    SIZE_PRESETS,
    TIME_PRESETS,
    // helpers (template 直接调用)
    mountDriverOf,
    mountPathOf,
    mountRootOf,
    // icons (template 用)
    add,
    // functions (handlers)
    playMedia,
    clearPlayError,
    togglePlayErrorDetail,
    setSortBy,
    handleRefresh,
    retryConnection,
    handleRequestStorage,
    navigateTo,
    goUp,
    handleFileClick,
    highlightFile,
    openContainingFolder,
    handleSearchInput,
    handleSearchClear,
    handleSearchToggle,
    insertOperator,
    dismissFulltextBanner, // 🆕 A6: 用户手动关掉 banner
    handleLongPress,
    handleCopy,
    onRenameConfirm,
    handleRename,
    handleMove,
    handleShare,
    handleUpload,
    handleFileSelected,
    handleAddNewTag,
    handleRemoveTag,
    handleDeleteFile,
    doDelete,
    loadPlugins,
    loadTags,
    openPluginView,
    exitPluginMode,
    openSideDrawer,
    getPluginIcon,
    handleTagFilter,
    applySizePreset,
    applyTimePreset,
    clearAllPluginFilters,
  } as unknown as UseFilesViewReturn;
}
