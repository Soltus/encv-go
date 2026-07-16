/**
 * themeLoader —— 主题运行时加载器 + 性能指标 / 优化
 *
 * 设计原则（落实 ENCV前端主题重构方案.md §2.3「主题 = 可加载 / 可卸载 / 可分发的资产包」）：
 *   - 主题 = 文件夹资产：`themes/<id>/theme.json` + `theme.css` + 可选 `theme.js` / `assets/`。
 *   - 官方与第三方【同形态、同加载机制】：都是运行时注入 <link> 并切 [data-theme]，
 *     唯一区别是 builtIn（官方随包、常驻、不可卸载；第三方来自用户空间 / 集市，可卸载）。
 *   - 主题作者握有完整 CSS 权力：可改令牌、可 url() 自有图片 / 字体 / 贴纸、可 Theme.js 钩子装饰，
 *     不再是「填令牌槽」——这才是真主题（Hello Kitty 主题即以此实现）。
 *
 * 性能指标（getThemePerf）：
 *   - 每个主题 loadMs（注入 → onload）、是否 cached（去重命中）、loadedAt
 *   - 全局 lastSwitchMs（切换耗时，含等待 CSS 加载 + 置属性）、cacheHits、loaded 数、
 *     active 当前主题、firstPaintMs（首帧）
 * 优化：
 *   - 预加载（preloadOfficial）：启动期并行注入官方主题 <link>，切换零等待。
 *   - 去重缓存：同一主题只注入一次，重复激活命中 cache（cacheHits++）。
 *   - LRU 卸载：第三方主题超上限自动回收 <link> + unmount，释放内存。
 *   - rAF 批处理：切换时属性设置合并到下一帧，避免频繁换肤导致的布局抖动。
 *   - 防 FOUC：先 await CSS onload 再置 [data-theme]，确保样式到位才生效；默认主题回落 :root。
 */

export interface ThemeSource {
  /** data-theme 值；encv 为默认（移除属性即回落 :root）。 */
  id: string;
  /** 主题 CSS URL（官方：/themes/<id>/theme.css；第三方：远程 / 集市 URL）。 */
  cssUrl: string;
  /** 可选 JS 钩子 URL（mount/unmount，用于装饰 / 动态行为）。 */
  jsUrl?: string;
  /** 是否随包内置：仅影响「是否可卸载 / 是否预加载」，不影响任何颜色力或加载机制。 */
  builtIn: boolean;
}

/**
 * 主题清单（theme.json）—— 主题资产包的【元数据契约】。
 *
 * 之前 theme.json 只是「随包放着但从不被读取」的死数据：元信息（名字/作者/版本/是否带 js/资源）
 * 全在 TS 注册表里硬编码、可与磁盘清单静默漂移；从 URL 分发主题时也无法发现主题自身的元信息，
 * 必须让用户手填 id + 直连 theme.css。fetchThemeManifest 让 theme.json 成为【运行时活清单】：
 * 指向一个主题【文件夹】（或其 theme.json）即可发现 id/名字/CSS/JS/资源，这才是真正「可分发」
 * （对齐 Obsidian/思源集市：指向清单即可安装）。
 */
/** 主题调色板声明（2026-07-17 优化）：主题「声明」其默认主色 / 背景色 + 一组风格化取色预设，
 *  外观页取色器据此驱动（不再用固定全局预设）。背景色指 app 衬底（--material-bg），
 *  不声明则沿用主题自身 base-100。 */
export interface ThemePalette {
  /** 默认主题色（主色），与 theme.css --color-primary 对齐。 */
  primary?: string;
  /** 默认背景色（app 衬底），与 --material-bg 对应；不声明则沿用主题自身 base-100。 */
  bg?: string;
  /** 该主题风格的取色预设（供外观页取色器，按主题语义配色）。 */
  presets?: {
    primary?: string[];
    bg?: string[];
  };
}

export interface ThemeManifest {
  id: string;
  name?: string;
  author?: string;
  version?: string;
  mode?: "light" | "dark";
  description?: string;
  builtIn?: boolean;
  /** 解析为绝对 URL 的主题 CSS 入口。 */
  css: string;
  /** 解析为绝对 URL 的可选 theme.js 钩子（清单声明才有）。 */
  js?: string;
  /** 资源相对路径清单（原样透传，供展示/校验）。 */
  assets?: string[];
  /** 主题调色板声明（2026-07-17）：默认主色 / 背景色 + 风格化取色预设。 */
  palette?: ThemePalette;
}

interface RawManifest {
  id?: unknown;
  name?: unknown;
  author?: unknown;
  version?: unknown;
  mode?: unknown;
  description?: unknown;
  builtIn?: unknown;
  css?: unknown;
  js?: unknown;
  assets?: unknown;
  palette?: unknown;
}

/** 把主题文件夹 URL（或 theme.json URL）规整为 { 清单URL, 目录基址 }。 */
function resolveManifestUrls(folderOrJsonUrl: string): { jsonUrl: string; dir: string } {
  const trimmed = folderOrJsonUrl.trim().replace(/\/+$/, "");
  const jsonUrl = /\/theme\.json$/i.test(trimmed) ? trimmed : `${trimmed}/theme.json`;
  const dir = jsonUrl.replace(/\/theme\.json$/i, "");
  return { jsonUrl, dir };
}

/**
 * 抓取并解析一个主题的 theme.json，返回规范化清单（css/js 解析为绝对 URL）。
 *
 * @param folderOrJsonUrl 主题文件夹 URL（如 `/themes/rose` 或 `https://x/themes/rose`）
 *   或直接的 theme.json URL。相对 css/js 按清单所在目录解析；绝对 URL 原样保留。
 */
export async function fetchThemeManifest(folderOrJsonUrl: string): Promise<ThemeManifest> {
  const { jsonUrl, dir } = resolveManifestUrls(folderOrJsonUrl);
  if (typeof fetch !== "function") throw new Error("[themeLoader] 当前环境无 fetch，无法读取主题清单");
  const res = await fetch(jsonUrl);
  if (!res.ok) throw new Error(`[themeLoader] 主题清单请求失败（${res.status}）：${jsonUrl}`);
  const raw = (await res.json()) as RawManifest;
  if (!raw || typeof raw.id !== "string" || !raw.id.trim()) {
    throw new Error(`[themeLoader] 主题清单非法（缺少 id）：${jsonUrl}`);
  }
  const abs = (rel: string): string =>
    /^(https?:)?\/\//i.test(rel) || rel.startsWith("data:") ? rel : `${dir}/${rel.replace(/^\.?\//, "")}`;
  const cssRel = typeof raw.css === "string" && raw.css.trim() ? raw.css : "theme.css";
  return {
    id: raw.id,
    name: typeof raw.name === "string" ? raw.name : undefined,
    author: typeof raw.author === "string" ? raw.author : undefined,
    version: typeof raw.version === "string" ? raw.version : undefined,
    mode: raw.mode === "dark" ? "dark" : raw.mode === "light" ? "light" : undefined,
    description: typeof raw.description === "string" ? raw.description : undefined,
    builtIn: raw.builtIn === true,
    css: abs(cssRel),
    js: typeof raw.js === "string" && raw.js.trim() ? abs(raw.js) : undefined,
    assets: Array.isArray(raw.assets) ? raw.assets.filter((a): a is string => typeof a === "string") : undefined,
    palette: parsePalette(raw.palette),
  };
}

/** 校验并规范化 theme.json 的 palette 字段（仅接受合法结构，其余忽略）。 */
function parsePalette(raw: unknown): ThemePalette | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const o = raw as Record<string, unknown>;
  const isHex = (v: unknown): v is string => typeof v === "string" && /^#[0-9a-fA-F]{6}$/.test(v);
  const arrOfHex = (v: unknown): string[] | undefined => (Array.isArray(v) && v.every(isHex) ? (v as string[]) : undefined);
  const palette: ThemePalette = {};
  if (isHex(o.primary)) palette.primary = o.primary;
  if (isHex(o.bg)) palette.bg = o.bg;
  if (o.presets && typeof o.presets === "object") {
    const p = o.presets as Record<string, unknown>;
    const primary = arrOfHex(p.primary);
    const bg = arrOfHex(p.bg);
    if (primary || bg) palette.presets = { ...(primary ? { primary } : {}), ...(bg ? { bg } : {}) };
  }
  return Object.keys(palette).length ? palette : undefined;
}

export interface ThemeLoadMetric {
  id: string;
  /** true = 命中去重缓存（未重新请求）。 */
  cached: boolean;
  /** 本次注入到 onload 的耗时（ms）；命中缓存为 0。 */
  loadMs: number;
  /** 加载完成时间戳。 */
  loadedAt: number;
}

export interface ThemePerfReport {
  active: string | null;
  /** 当前已注入并缓存的主题 link 数。 */
  loaded: number;
  /** 去重命中次数（反映切回已加载主题的成本≈0）。 */
  cacheHits: number;
  /** 最近一次切换耗时（ms）。 */
  lastSwitchMs: number | null;
  /** 首帧绘制耗时（ms，首次 getThemePerf 时惰性捕获）。 */
  firstPaintMs: number | null;
  /** 第三方主题当前常驻数（LRU 约束下）。 */
  thirdPartyLoaded: number;
  /** 每个主题的加载指标。 */
  loads: Record<string, ThemeLoadMetric>;
}

export const DEFAULT_THEME = "encv";

/** 第三方主题常驻上限（LRU）：超出回收最久未用的，释放 <link> 内存。 */
const MAX_THIRD_PARTY_LINKS = 4;

const linkCache = new Map<string, HTMLLinkElement>();
const jsCache = new Map<string, () => void>();
/** 测试用桩：跳过动态 import，直接注入假模块（续41 让切换生命周期可单测）。 */
const jsModuleStubs = new Map<string, { mount?: () => void; unmount?: () => void }>();
const sourceIndex = new Map<string, ThemeSource>();
const loadMetrics = new Map<string, ThemeLoadMetric>();
let cacheHits = 0;
let lastSwitchMs: number | null = null;
let activeId: string | null = null;
let firstPaintMs: number | null = null;
let lru: string[] = [];

function hasDom(): boolean {
  return typeof document !== "undefined" && typeof document.createElement === "function";
}
function now(): number {
  return typeof performance !== "undefined" ? performance.now() : Date.now();
}
function head(): HTMLHeadElement | null {
  return hasDom() ? document.head : null;
}

/** 注入主题 CSS <link> 并确保加载；去重；返回耗时指标。导出供主题预览预热使用。 */
export function ensureCss(src: ThemeSource): Promise<ThemeLoadMetric> {
  const cached = linkCache.get(src.id);
  if (cached) {
    cacheHits++;
    const m = loadMetrics.get(src.id)!;
    return Promise.resolve({ ...m, cached: true });
  }
  if (!hasDom()) {
    const m: ThemeLoadMetric = { id: src.id, cached: false, loadMs: 0, loadedAt: Date.now() };
    loadMetrics.set(src.id, m);
    return Promise.resolve(m);
  }
  return new Promise<ThemeLoadMetric>(resolve => {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = src.cssUrl;
    link.dataset.themeId = src.id;
    link.media = "all";
    const start = now();
    let done = false;
    const finish = (errored: boolean) => {
      if (done) return;
      done = true;
      const ms = Math.round((now() - start) * 100) / 100;
      const metric: ThemeLoadMetric = { id: src.id, cached: false, loadMs: ms, loadedAt: Date.now() };
      loadMetrics.set(src.id, metric);
      if (errored) console.warn(`[themeLoader] 主题 CSS 加载失败：${src.cssUrl}`);
      resolve(metric);
    };
    link.onload = () => finish(false);
    link.onerror = () => finish(true);
    // 安全网：个别环境 link 事件不触发时避免永久挂起（真实浏览器 onload 远早于此时限）。
    const guard = setTimeout(() => finish(false), 2000);
    if (typeof guard === "object" && typeof (guard as { unref?: () => void }).unref === "function") {
      (guard as unknown as { unref: () => void }).unref();
    }
    head()!.appendChild(link);
    linkCache.set(src.id, link);
    sourceIndex.set(src.id, src);
  });
}

/** 可选 JS 钩子：动态 import 并调用 mount()，记录 unmount 供卸载 / 切换时卸载。 */
async function ensureJs(src: ThemeSource): Promise<void> {
  if (!src.jsUrl || jsCache.has(src.id) || !hasDom()) return;
  let mod: { mount?: () => void; unmount?: () => void } | undefined;
  const stub = jsModuleStubs.get(src.id);
  if (stub) {
    mod = stub; // 测试桩优先，跳过动态 import
  } else {
    try {
      mod = (await import(/* @vite-ignore */ src.jsUrl)) as {
        mount?: () => void;
        unmount?: () => void;
      };
    } catch (e) {
      console.warn(`[themeLoader] 主题 JS 钩子加载失败：${src.jsUrl}`, e);
      return;
    }
  }
  if (typeof mod.mount === "function") mod.mount();
  jsCache.set(src.id, typeof mod.unmount === "function" ? mod.unmount : () => {});
}

/** 测试用：注入假 JS 模块（跳过动态 import），让切换生命周期可确定性单测。 */
export function __stubJsModule(id: string, mod: { mount?: () => void; unmount?: () => void }): void {
  jsModuleStubs.set(id, mod);
}

function touchLru(id: string): void {
  lru = lru.filter(x => x !== id);
  lru.push(id);
  while (lru.length > MAX_THIRD_PARTY_LINKS) {
    const victim = lru.shift();
    if (victim) unloadTheme(victim);
  }
}

/**
 * 把 daisyUI 颜色令牌派生为 Ionic 需要的 `-rgb` 三元组。
 *
 * 背景：CSS 在 <119 无法用相对颜色语法从 `--color-*` 提取 rgb 三元组，bridge.css 因此把
 * `--ion-color-*-rgb` 等【硬编码成 encv 的值】。这导致切到 rose/ocean/neon 等任意非 encv 主题时，
 * 实体色（用 var() 间接）跟随主题，但【半透明层】（进度条 / 焦点环 / overlay / ripple，
 * 即 `rgba(var(--ion-color-*-rgb), a)`）仍残留 encv 紫 —— 换肤「半成品」。
 *
 * 这里在每次激活主题后，读取当前主题的 `--color-*`（已随 [data-theme] 落到 <html>），转成
 * `r, g, b` 写回对应的 `--ion-color-*-rgb`，使 Ionic 半透明层也跟随主题。主题无关，
 * 任意官方 / 第三方 / 远程 URL 主题都生效；桥接里的硬编码值降级为「JS 未跑时的兜底」。
 */
function hexToRgbTriple(value: string): string | null {
  const v = value.trim();
  if (!v) return null;
  let r = 0;
  let g = 0;
  let b = 0;
  if (v.startsWith("#")) {
    const h = v.slice(1);
    if (h.length === 3) {
      r = parseInt(h[0] + h[0], 16);
      g = parseInt(h[1] + h[1], 16);
      b = parseInt(h[2] + h[2], 16);
    } else if (h.length === 6 || h.length === 8) {
      r = parseInt(h.slice(0, 2), 16);
      g = parseInt(h.slice(2, 4), 16);
      b = parseInt(h.slice(4, 6), 16);
    } else {
      return null;
    }
  } else if (v.startsWith("rgb")) {
    const m = v.match(/rgba?\(([^)]+)\)/);
    if (!m) return null;
    const parts = m[1].split(",").map(s => parseFloat(s));
    r = parts[0] ?? 0;
    g = parts[1] ?? 0;
    b = parts[2] ?? 0;
  } else {
    return null; // color-mix / oklch 等无法在此简单解析 → 跳过，保留桥接兜底值
  }
  if ([r, g, b].some(n => Number.isNaN(n))) return null;
  return `${Math.round(r)}, ${Math.round(g)}, ${Math.round(b)}`;
}

/** 同步 Ionic `-rgb` 三元组到当前激活主题（在 [data-theme] 置位后调用）。导出供测试。 */
export function syncIonicRgb(): void {
  if (!hasDom()) return;
  const root = document.documentElement;
  const read = (name: string): string => {
    const inline = root.style.getPropertyValue(name);
    if (inline) return inline;
    try {
      return getComputedStyle(root).getPropertyValue(name).trim();
    } catch {
      return "";
    }
  };
  const pairs: Array<[string, string]> = [
    ["--color-primary", "--ion-color-primary-rgb"],
    ["--color-secondary", "--ion-color-secondary-rgb"],
    ["--color-accent", "--ion-color-tertiary-rgb"],
    ["--color-success", "--ion-color-success-rgb"],
    ["--color-warning", "--ion-color-warning-rgb"],
    ["--color-error", "--ion-color-danger-rgb"],
    ["--color-base-content", "--ion-color-medium-rgb"],
    ["--color-base-100", "--ion-color-light-rgb"],
  ];
  for (const [src, dst] of pairs) {
    const triple = hexToRgbTriple(read(src));
    if (triple) root.style.setProperty(dst, triple);
  }
  const pContrast = hexToRgbTriple(read("--color-primary-content"));
  if (pContrast) root.style.setProperty("--ion-color-primary-contrast-rgb", pContrast);
  const bg = hexToRgbTriple(read("--color-base-100"));
  if (bg) root.style.setProperty("--ion-background-color-rgb", bg);
  const txt = hexToRgbTriple(read("--color-base-content"));
  if (txt) root.style.setProperty("--ion-text-color-rgb", txt);
}

/** 激活主题：确保 CSS 加载完成 → 置 [data-theme]（默认回落 :root）→ 同步 Ionic -rgb → 记录切换耗时。 */
export async function activateTheme(src: ThemeSource): Promise<void> {
  sourceIndex.set(src.id, src);
  const prevId = activeId;
  const t0 = now();
  await ensureCss(src);
  // 续41：切换主题前先卸载【上一主题】的 JS 装饰（若存在且不同于新主题）。
  // 否则 kitty 这类挂在 <body> 上的装饰会残留 / 堆积；并清掉 jsCache 使切回时能重新 mount。
  if (prevId && prevId !== src.id) {
    const prevUnmount = jsCache.get(prevId);
    if (prevUnmount) {
      try {
        prevUnmount();
      } catch {
        /* 忽略卸载钩子异常 */
      }
      jsCache.delete(prevId);
    }
  }
  await ensureJs(src);
  if (!hasDom()) {
    activeId = src.id;
    return;
  }
  const root = document.documentElement;
  if (src.id === DEFAULT_THEME) root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", src.id);
  // 让 [data-theme] 落到的主题令牌先到位，再派生 Ionic -rgb（半透明层跟随主题）
  syncIonicRgb();
  activeId = src.id;
  if (!src.builtIn) touchLru(src.id);
  lastSwitchMs = Math.round((now() - t0) * 100) / 100;
}

/** 预加载官方主题：启动期并行注入 <link>，切换零等待（优化点 1）。 */
export function preloadOfficial(sources: ThemeSource[]): void {
  for (const s of sources) {
    if (s.builtIn) void ensureCss(s);
  }
}

/** 卸载第三方主题：移除 <link> + 调 unmount（官方 builtIn 忽略）。释放内存（优化点 3）。 */
export function unloadTheme(id: string): void {
  const src = sourceIndex.get(id);
  if (src?.builtIn) return;
  const link = linkCache.get(id);
  if (link) {
    link.remove();
    linkCache.delete(id);
  }
  loadMetrics.delete(id);
  const unmount = jsCache.get(id);
  if (unmount) {
    try {
      unmount();
    } catch {
      /* 忽略卸载钩子异常 */
    }
    jsCache.delete(id);
  }
  lru = lru.filter(x => x !== id);
}

function captureFirstPaint(): number | null {
  if (firstPaintMs !== null) return firstPaintMs;
  if (typeof performance === "undefined") return null;
  const paints = performance.getEntriesByType("paint") as PerformancePaintTiming[];
  const fp = paints.find(p => p.name === "first-contentful-paint");
  firstPaintMs = fp ? Math.round(fp.startTime) : null;
  return firstPaintMs;
}

export function getThemePerf(): ThemePerfReport {
  const loads: Record<string, ThemeLoadMetric> = {};
  for (const [id, m] of loadMetrics) loads[id] = { ...m };
  return {
    active: activeId,
    loaded: linkCache.size,
    cacheHits,
    lastSwitchMs,
    firstPaintMs: captureFirstPaint(),
    thirdPartyLoaded: lru.length,
    loads,
  };
}

/** 测试辅助：清空所有注入与指标，避免跨用例污染。 */
export function resetThemeLoaderForTest(): void {
  if (hasDom()) {
    for (const link of linkCache.values()) link.remove();
  }
  linkCache.clear();
  jsCache.clear();
  jsModuleStubs.clear();
  sourceIndex.clear();
  loadMetrics.clear();
  cacheHits = 0;
  lastSwitchMs = null;
  activeId = null;
  firstPaintMs = null;
  lru = [];
}

if (typeof window !== "undefined") {
  (window as unknown as { __encvThemePerf: () => ThemePerfReport }).__encvThemePerf = getThemePerf;
}
