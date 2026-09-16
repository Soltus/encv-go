import { computed, ref } from "vue";
import {
  activateTheme,
  ensureCss,
  fetchThemeManifest,
  unloadTheme,
  getThemePerf,
  type ThemeSource,
  type ThemePerfReport,
  type ThemePalette,
} from "@encv/shared-components/theme/themeLoader";
import { getThemeStorage } from "@encv/shared-components/theme/themeStorage";
import { DEFAULT_THEME, type ThemeManifest } from "@encv/shared-components/theme/themeLoader";

/**
 * 主题注册表 + 运行时闭环（encv-mobile 纯 CSS 路径）。
 *
 * 主题 = 运行时可加载的【文件夹资产包】（themes/<id>/theme.json + theme.css + 可选 theme.js /
 * assets/），由 themeLoader 注入 <link> 并切 [data-theme]。官方与第三方【同形态、同加载机制】，
 * 唯一区别是 builtIn（官方随包、预加载、不可卸载；第三方来自用户空间 / 集市，可卸载、LRU 回收）。
 *
 * 这里负责：注册可用主题、把选中主题派发给 themeLoader（注入 + 切换）、持久化偏好，
 * 以及【用户可安装 / 卸载 / 分发】主题（installTheme / uninstallTheme / Bazaar 集市），
 * 让「可加载 / 可卸载 / 可分发」对终端用户真正可用（不止是引擎内部能力）。
 * 加载 / 卸载 / 性能指标由 themeLoader 统一处理（见 theme/themeLoader.ts）。
 */

export interface UserThemeMeta {
  /** data-theme 属性值；encv 为默认（移除属性即回落 :root）。 */
  id: string;
  /** i18n 标签键（随包官方 / 集市内置主题用）。 */
  nameKey?: string;
  /** 运行时安装主题的直接显示名（无 nameKey 时显示，例如从 URL 安装的主题）。 */
  label?: string;
  /** 是否随包内置：仅分发标记（随包发布 + 外观页展示 + 不可卸载）。
   *  与第三方主题【加载机制、颜色力完全相同】，无任何特权。见 THEME_DEV.md §6.9/§6.10。 */
  builtIn?: boolean;
  /** 第三方主题的远程 CSS URL（集市 / 用户空间）。省略则按本地资源包解析（示例用）。 */
  url?: string;
  /** 是否携带 theme.js 运行时装饰钩子（仅本地资源包）。省略即无（避免对无 js 的主题请求 404）。 */
  js?: boolean;
  /** 清单声明的 theme.js 绝对 URL（从清单/URL 安装时使用，优先于 js 布尔的本地推导）。 */
  jsUrl?: string;
  /** 是否已下载到本地 store（续43：本地优先，云拉取也落地本地同一目录）。
   *  true ⇒ 激活时从本地 store 解析 css/js（离线可用），而非热链远程 URL。 */
  local?: boolean;
  /** 主题调色板声明（2026-07-17）：默认主色 / 背景色 + 风格化取色预设，供外观页取色器驱动。 */
  palette?: ThemePalette;
}

/** 集市（Bazaar）目录条目：用户可一键安装到用户空间的主题。 */
export interface BazaarEntry {
  id: string;
  nameKey: string;
  descKey?: string;
  /** 资源地址：省略则按本地资源包 /themes/<id>/theme.css 解析；给出则为远程 URL。 */
  url?: string;
  /** 主题调色板声明（集市主题同样声明，保证安装后即可定制）。 */
  palette?: ThemePalette;
}

/** 主题资产包基址（public/themes → 运行时 /themes/<id>/theme.css）。 */
const THEMES_BASE = "/themes";

/** 已安装（用户空间）主题的持久化键。 */
const INSTALLED_KEY = "encv-installed-themes";
/** 当前选中主题的持久化键。 */
const USER_THEME_KEY = "encv-user-theme";

function toSource(meta: UserThemeMeta): ThemeSource {
  // 本地优先：已拉取到后端本地目录的主题 → 永远从同源 /themes/<id>/ 解析（不热链远程 CDN）。
  const cssUrl = meta.local ? `${THEMES_BASE}/${meta.id}/theme.css` : (meta.url ?? `${THEMES_BASE}/${meta.id}/theme.css`);
  // jsUrl 优先级：本地优先路径下 js:true → 同源 /themes/<id>/theme.js；
  // 否则清单/URL 安装显式声明的 jsUrl > 本地资源包 js:true 推导（避免对无钩子主题 404）。
  const jsUrl = meta.local
    ? meta.js
      ? `${THEMES_BASE}/${meta.id}/theme.js`
      : undefined
    : (meta.jsUrl ?? (!meta.url && meta.js ? `${THEMES_BASE}/${meta.id}/theme.js` : undefined));
  return { id: meta.id, cssUrl, jsUrl, builtIn: Boolean(meta.builtIn) };
}

/** 随包内置主题（官方 6 + 示例第三方 3）。新增官方主题 = 三处同步（与第三方完全一致）：
 *   1) public/themes/<id>/theme.json + theme.css（纯 CSS 数据资产，可含 url() 资源 / theme.js 钩子）
 *   2) 此处登记 { id, nameKey, builtIn: true }
 *   3) i18n settings.ts 加 settings.theme<Id> 名
 *   ❌ 不再 @import 进 theme-core.css（那会让主题被编译期烤进产物，丧失「可加载/卸载/分发」）。 */
/** 各主题的默认调色板（2026-07-17）：与各自 theme.css 的 --color-primary / base-100 对齐，
 *  并给出一组「风格化」取色预设供外观页取色器驱动（不再用固定全局预设）。
 *  此 TS 声明是取色器的【同步来源】；同源 palette 也写入各 theme.json 做忠实镜像（契约锁见测试）。 */
export const USER_THEMES: UserThemeMeta[] = [
  {
    id: "encv",
    nameKey: "settings.themeBuiltIn",
    builtIn: true,
    palette: {
      primary: "#8b5cf6",
      bg: "#ffffff",
      presets: {
        primary: ["#8b5cf6", "#06b6d4", "#ec4899", "#6366f1", "#0ea5e9", "#a855f7"],
        bg: ["#ffffff", "#f3f4f6", "#faf5ff", "#eef2ff", "#f5f3ff"],
      },
    },
  },
  {
    id: "encv-dark",
    nameKey: "settings.themeBuiltInDark",
    builtIn: true,
    palette: {
      primary: "#9c6df7",
      bg: "#0f172a",
      presets: {
        primary: ["#9c6df7", "#1fbdd8", "#ee5aa3", "#818cf8", "#38bdf8", "#c084fc"],
        bg: ["#0f172a", "#1e293b", "#0b1120", "#171923", "#121218"],
      },
    },
  },
  {
    id: "rose",
    nameKey: "settings.themeRose",
    builtIn: true,
    palette: {
      primary: "#e11d48",
      bg: "#fffafb",
      presets: {
        primary: ["#e11d48", "#0ea5e9", "#db2777", "#f43f5e", "#be123c", "#fb7185"],
        bg: ["#fffafb", "#ffeef2", "#fff1f2", "#fce7ef", "#fff0f5"],
      },
    },
  },
  {
    id: "ocean",
    nameKey: "settings.themeOcean",
    builtIn: true,
    palette: {
      primary: "#22d3ee",
      bg: "#0b1120",
      presets: {
        primary: ["#22d3ee", "#2dd4bf", "#818cf8", "#38bdf8", "#0ea5e9", "#34d399"],
        bg: ["#0b1120", "#111c33", "#0a0e14", "#0f172a", "#101a2e"],
      },
    },
  },
  {
    id: "forest",
    nameKey: "settings.themeForest",
    builtIn: true,
    palette: {
      primary: "#16a34a",
      bg: "#f3faf5",
      presets: {
        primary: ["#16a34a", "#0d9488", "#65a30d", "#22c55e", "#10b981", "#84cc16"],
        bg: ["#f3faf5", "#dcfce7", "#ecfdf5", "#f0fdf4", "#effaf1"],
      },
    },
  },
  {
    id: "midnight",
    nameKey: "settings.themeMidnight",
    builtIn: true,
    palette: {
      primary: "#818cf8",
      bg: "#0a0a14",
      presets: {
        primary: ["#818cf8", "#a78bfa", "#c084fc", "#6366f1", "#8b5cf6", "#a855f7"],
        bg: ["#0a0a14", "#13131f", "#0a0e14", "#121218", "#1a1a2e"],
      },
    },
  },
  // 示例第三方主题：与官方【同形态、同加载机制】，仅 builtIn:false（可卸载、LRU 回收）。
  {
    id: "sunset",
    nameKey: "settings.userThemeSunset",
    palette: {
      primary: "#f97316",
      bg: "#fffaf5",
      presets: {
        primary: ["#f97316", "#fb923c", "#f43f5e", "#f59e0b", "#ea580c", "#fb7185"],
        bg: ["#fffaf5", "#fdeede", "#fff7ed", "#ffedd5", "#fff1e6"],
      },
    },
  },
  {
    id: "mint",
    nameKey: "settings.userThemeMint",
    palette: {
      primary: "#10b981",
      bg: "#f3faf7",
      presets: {
        primary: ["#10b981", "#06b6d4", "#14b8a6", "#22c55e", "#0ea5e9", "#2dd4bf"],
        bg: ["#f3faf7", "#d9f2ea", "#ecfdf5", "#effaf7", "#e6f7f1"],
      },
    },
  },
  // 全能力示例（真主题证明）：自带 assets/ 图片 + @font-face 字体 + theme.js 装饰钩子。
  // 这就是「Hello Kitty 主题怎么实现」的可运行答案——不只是换色。
  {
    id: "kitty",
    nameKey: "settings.userThemeKitty",
    js: true,
    palette: {
      primary: "#ff5fa2",
      bg: "#fff5fa",
      presets: {
        primary: ["#ff5fa2", "#ffb3d1", "#ff8fab", "#fb7185", "#f472b6", "#f9a8d4"],
        bg: ["#fff5fa", "#ffe6f0", "#fff0f6", "#ffeef5", "#fff1f7"],
      },
    },
  },
];

/** 集市（Bazaar）示例目录：用户可一键安装到用户空间的本地资源包（安装前不出现在主网格）。 */
export const BAZAAR: BazaarEntry[] = [
  {
    id: "neon",
    nameKey: "settings.bazaarNeon",
    descKey: "settings.bazaarNeonDesc",
    palette: {
      primary: "#39ff14",
      bg: "#0a0e14",
      presets: {
        primary: ["#39ff14", "#00e5ff", "#ff2bd6", "#38bdf8", "#a3e635", "#22d3ee"],
        bg: ["#0a0e14", "#121823", "#0b1120", "#0d1117", "#0a0e14"],
      },
    },
  },
  {
    id: "paper",
    nameKey: "settings.bazaarPaper",
    descKey: "settings.bazaarPaperDesc",
    palette: {
      primary: "#a0785a",
      bg: "#f7f1e6",
      presets: {
        primary: ["#a0785a", "#7d9b76", "#c08552", "#b08968", "#8a7a66", "#9c7a52"],
        bg: ["#f7f1e6", "#efe6d6", "#faf6ee", "#f3ebdc", "#f9f3ea"],
      },
    },
  },
];

const SOURCES = new Map<string, ThemeSource>(USER_THEMES.map(m => [m.id, toSource(m)] as const));
const DEFAULT_SOURCE = SOURCES.get(DEFAULT_THEME)!;

export const activeThemeId = ref<string>(DEFAULT_THEME);
const installedThemes = ref<UserThemeMeta[]>([]);
const themePerf = ref<ThemePerfReport>(getThemePerf());

function refreshPerf(): void {
  themePerf.value = getThemePerf();
}

function registerSource(meta: UserThemeMeta): ThemeSource {
  const src = toSource(meta);
  SOURCES.set(meta.id, src);
  return src;
}

/** 取某主题的调色板声明（2026-07-17）：内置注册表优先，其次已安装主题。取色器据此驱动。 */
export function getThemePalette(id: string): ThemePalette | undefined {
  const reg = USER_THEMES.find(t => t.id === id);
  if (reg?.palette) return reg.palette;
  const inst = installedThemes.value.find(t => t.id === id);
  return inst?.palette;
}

function loadInstalled(): UserThemeMeta[] {
  if (typeof localStorage === "undefined") return [];
  try {
    const raw = localStorage.getItem(INSTALLED_KEY);
    if (!raw) return [];
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? (arr.filter(x => x?.id) as UserThemeMeta[]) : [];
  } catch {
    return [];
  }
}

function saveInstalled(): void {
  if (typeof localStorage === "undefined") return;
  localStorage.setItem(INSTALLED_KEY, JSON.stringify(installedThemes.value));
}

/** 安装一个主题到用户空间：登记源 + 预热 <link> + 持久化。返回其源（供立即预览/应用）。 */
export function installTheme(meta: UserThemeMeta): ThemeSource {
  const clean: UserThemeMeta = { ...meta, builtIn: false };
  if (!installedThemes.value.some(t => t.id === clean.id)) {
    installedThemes.value = [...installedThemes.value, clean];
    saveInstalled();
  }
  const src = registerSource(clean);
  void ensureCss(src); // 预热，安装即可在网格中预览 / 一键应用
  refreshPerf();
  return src;
}

/** 从集市一键安装。 */
export function installFromBazaar(entry: BazaarEntry): ThemeSource {
  return installTheme({ id: entry.id, nameKey: entry.nameKey, url: entry.url, builtIn: false, palette: entry.palette });
}

/**
 * 从一个主题【文件夹 URL】（或其 theme.json URL）安装 —— 真正的「可分发」路径。
 *
 * 抓取并解析主题自带的 theme.json 清单，据此发现 id / 名字 / CSS / JS，无需用户手填 id 或直连
 * theme.css（对齐 Obsidian / 思源集市：指向清单即可安装）。随后通过 ThemeStorage 端口让【后端】
 * 把远程主题拉取到【本地同一目录】themes/<id>/（本地优先、不热链 CDN），前端激活时从同源
 * /themes/<id>/ 加载。框架不存字节、不感知 Go —— 只依赖注入的端口。
 */
export async function installThemeFromUrl(folderOrJsonUrl: string): Promise<ThemeSource> {
  const manifest = await fetchThemeManifest(folderOrJsonUrl);
  await getThemeStorage().pullToLocal({ id: manifest.id, sourceUrl: folderOrJsonUrl, manifest });
  return installTheme({
    id: manifest.id,
    label: manifest.name ?? manifest.id,
    builtIn: false,
    local: true, // 激活时从同源 /themes/<id>/ 解析，而非热链远程 CDN
    js: Boolean(manifest.js),
    palette: manifest.palette,
  });
}

/**
 * 裸 .css 直链安装（无清单回退）：同样通过端口拉取到后端【本地同一目录】，
 * 本地优先从同源 /themes/<id>/theme.css 加载。id 由文件名推导（无清单元信息）。
 */
export async function installThemeFromCssLink(cssUrl: string): Promise<ThemeSource> {
  const seg =
    cssUrl
      .split("/")
      .pop()
      ?.replace(/\.css$/i, "") ?? "theme";
  const id = `url-${seg}`;
  const manifest: ThemeManifest = { id, css: cssUrl };
  await getThemeStorage().pullToLocal({ id, sourceUrl: cssUrl, manifest, cssOnly: true });
  return installTheme({ id, label: seg, builtIn: false, local: true });
}

/** 卸载用户空间主题：移除注册 + 删持久化 + 后端本地目录清理 + loader 卸载 <link>/unmount；
 *  若正激活则回落默认。后端清理经 ThemeStorage 端口（默认同源无操作，Go 适配器真实删盘）。 */
export function uninstallTheme(id: string): void {
  if (!installedThemes.value.some(t => t.id === id)) return;
  installedThemes.value = installedThemes.value.filter(t => t.id !== id);
  saveInstalled();
  void getThemeStorage().removeLocal(id);
  unloadTheme(id);
  if (activeThemeId.value === id) {
    activeThemeId.value = DEFAULT_THEME;
    void activateTheme(DEFAULT_SOURCE).then(refreshPerf);
  }
  refreshPerf();
}

function readStored(): string | null {
  if (typeof localStorage === "undefined") return null;
  const v = localStorage.getItem(USER_THEME_KEY);
  if (v && SOURCES.has(v)) return v;
  return null;
}

/** 启动期调用（main.ts）：水合已安装主题 + 预热全部主题 link（切换/预览零等待）+ 激活偏好。 */
export async function initUserThemes(): Promise<void> {
  const installed = loadInstalled();
  for (const m of installed) registerSource(m);
  installedThemes.value = installed;

  // 预热：随包主题 + 示例第三方 + 已安装主题，全部注入 <link>，切换/预览零等待。
  const all = [...USER_THEMES, ...installed];
  for (const m of all) {
    const src = SOURCES.get(m.id) ?? registerSource(m);
    void ensureCss(src);
  }

  const stored = readStored() ?? DEFAULT_THEME;
  activeThemeId.value = stored;
  await activateTheme(SOURCES.get(stored) ?? DEFAULT_SOURCE);
  refreshPerf();
}

/** 预热单个主题 CSS（供外观页可视预览：确保该主题 <link> 已注入再渲染预览）。 */
export function ensureThemeLoaded(id: string): void {
  const src = SOURCES.get(id);
  if (src) void ensureCss(src);
}

export function useUserThemes() {
  const allThemes = computed(() => [...USER_THEMES, ...installedThemes.value]);
  const builtInThemes = computed(() => USER_THEMES.filter(t => t.builtIn));
  const userThemes = computed(() => USER_THEMES.filter(t => !t.builtIn));
  const installedList = computed(() => installedThemes.value);
  const activeTheme = computed(() => activeThemeId.value);

  function isActive(id: string): boolean {
    return activeThemeId.value === id;
  }
  function isBuiltIn(id: string): boolean {
    return Boolean(USER_THEMES.find(t => t.id === id)?.builtIn);
  }
  function isInstalled(id: string): boolean {
    return installedThemes.value.some(t => t.id === id);
  }
  function themeLabel(meta: UserThemeMeta): string {
    return meta.label ?? (meta.nameKey ? meta.nameKey : meta.id);
  }

  function applyTheme(id: string) {
    const src = SOURCES.get(id);
    if (!src) return;
    activeThemeId.value = id;
    // 异步注入 + 切换：先 await CSS onload 再置属性，避免 FOUC；完成后刷新性能指标。
    void activateTheme(src).then(refreshPerf);
    if (typeof localStorage === "undefined") return;
    if (id === DEFAULT_THEME) localStorage.removeItem(USER_THEME_KEY);
    else localStorage.setItem(USER_THEME_KEY, id);
  }

  return {
    themes: allThemes,
    allThemes,
    builtInThemes,
    userThemes,
    installedList,
    activeTheme,
    themePerf,
    bazaar: BAZAAR,
    applyTheme,
    isActive,
    isBuiltIn,
    isInstalled,
    themeLabel,
    installTheme,
    installFromBazaar,
    installThemeFromUrl,
    installThemeFromCssLink,
    uninstallTheme,
    ensureThemeLoaded,
  };
}
