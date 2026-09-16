import { computed, ref, watch } from "vue";
import { motion } from "../motion/internal";
import { getMotionProfile } from "../motion/guard";
import { activeThemeId, getThemePalette } from "./useUserThemes";

const THEME_KEY = "encv-theme-preference";
const COLOR_KEY = "encv-theme-color";
const BG_COLOR_KEY = "encv-bg-color";
const P3_KEY = "encv-p3-mode";
const VIVID_KEY = "encv-vivid-mode";
const THEME_CUSTOM_KEY = "encv-theme-custom";

/** per-theme 自定义覆盖：key=主题 id，存主色 / 背景色覆盖（背景色 null 表示「用主题默认」）。 */
export type ThemeCustom = Record<string, { primary?: string; bg?: string | null }>;

const _isDarkForced = ref<boolean | null>(null);
const currentColor = ref("#4f8cff");
const currentBgColor = ref<string | null>(null);
const p3Mode = ref<"off" | "on" | "auto">("auto");
const vividMode = ref<"off" | "on">("off");
// 色彩浓度 / 对比度拆分为两个独立滑块（2026-07-17 优化），各自 50..200，默认 100。
const vividSaturation = ref(100);
const vividContrast = ref(100);

/** per-theme 自定义覆盖（2026-07-17）：用户按主题改主色 / 背景色，切换主题时各自重放。 */
const themeCustom = ref<ThemeCustom>(loadThemeCustom());
function loadThemeCustom(): ThemeCustom {
  if (typeof localStorage === "undefined") return {};
  try {
    const raw = localStorage.getItem(THEME_CUSTOM_KEY);
    return raw ? (JSON.parse(raw) as ThemeCustom) : {};
  } catch {
    return {};
  }
}
function saveThemeCustom(): void {
  if (typeof localStorage === "undefined") return;
  localStorage.setItem(THEME_CUSTOM_KEY, JSON.stringify(themeCustom.value));
}
/** 当前激活主题的调色板声明（同步来源），驱动外观页取色器。 */
const activeThemePalette = computed(() => getThemePalette(activeThemeId.value) ?? {});

export interface ThemePreset {
  name: string;
  value: string;
  colorRgb: string;
}

export interface BgPreset {
  name: string;
  value: string | null;
  description: string;
  category: "light" | "eye" | "dark" | "gradient";
  textColor: string;
  gradientColors?: [string, ...string[]];
}

export const THEME_PRESETS: ThemePreset[] = [
  { name: "Blue", value: "#4f8cff", colorRgb: "79, 140, 255" },
  { name: "Purple", value: "#8b5cf6", colorRgb: "139, 92, 246" },
  { name: "Green", value: "#22c55e", colorRgb: "34, 197, 94" },
  { name: "Orange", value: "#f97316", colorRgb: "249, 115, 22" },
  { name: "Red", value: "#ef4444", colorRgb: "239, 68, 68" },
  { name: "Pink", value: "#ec4899", colorRgb: "236, 72, 153" },
  { name: "Teal", value: "#14b8a6", colorRgb: "20, 184, 166" },
];

export const BG_PRESETS: BgPreset[] = [
  { name: "bg.white", value: "#ffffff", description: "bg.whiteDesc", category: "light", textColor: "#1a1a1a" },
  { name: "bg.snow", value: "#f8fafc", description: "bg.snowDesc", category: "light", textColor: "#1e293b" },
  { name: "bg.pearl", value: "#fef3ed", description: "bg.pearlDesc", category: "light", textColor: "#431407" },
  { name: "bg.mist", value: "#f0f4f8", description: "bg.mistDesc", category: "light", textColor: "#334155" },
  { name: "bg.cloud", value: "#eef2ff", description: "bg.cloudDesc", category: "light", textColor: "#1e3a5f" },
  { name: "bg.ivory", value: "#fffff0", description: "bg.ivoryDesc", category: "light", textColor: "#3d3626" },

  { name: "bg.sepia", value: "#f4ecd8", description: "bg.sepiaDesc", category: "eye", textColor: "#5b4636" },
  { name: "bg.sage", value: "#dce4d0", description: "bg.sageDesc", category: "eye", textColor: "#3a4a30" },
  { name: "bg.lavender", value: "#e6e0f0", description: "bg.lavenderDesc", category: "eye", textColor: "#3a2f4a" },
  { name: "bg.cream", value: "#f5efe0", description: "bg.creamDesc", category: "eye", textColor: "#4a3f2a" },
  { name: "bg.dustyRose", value: "#f5e0dc", description: "bg.dustyRoseDesc", category: "eye", textColor: "#5c3333" },
  { name: "bg.frost", value: "#e8f0f8", description: "bg.frostDesc", category: "eye", textColor: "#2a3f52" },

  { name: "bg.lightBlack", value: "#1a1a1a", description: "bg.lightBlackDesc", category: "dark", textColor: "#e0e0e0" },
  { name: "bg.darkBlack", value: "#000000", description: "bg.darkBlackDesc", category: "dark", textColor: "#ffffff" },
  { name: "bg.midnight", value: "#0a0e1a", description: "bg.midnightDesc", category: "dark", textColor: "#d0d8e8" },
  { name: "bg.charcoal", value: "#2a2a2e", description: "bg.charcoalDesc", category: "dark", textColor: "#d8d8d8" },
  { name: "bg.ink", value: "#171923", description: "bg.inkDesc", category: "dark", textColor: "#c8cad8" },
  { name: "bg.obsidian", value: "#121218", description: "bg.obsidianDesc", category: "dark", textColor: "#b8b8c8" },

  {
    name: "bg.deepForest",
    value: null,
    description: "bg.deepForestDesc",
    category: "gradient",
    textColor: "#e8f0e0",
    gradientColors: ["#134e5e", "#71b280"],
  },
  {
    name: "bg.oceanDusk",
    value: null,
    description: "bg.oceanDuskDesc",
    category: "gradient",
    textColor: "#ffffff",
    gradientColors: ["#2c3e50", "#4ca1af"],
  },
  {
    name: "bg.warmHoney",
    value: null,
    description: "bg.warmHoneyDesc",
    category: "gradient",
    textColor: "#4a3520",
    gradientColors: ["#f6d365", "#fda085"],
  },
  {
    name: "bg.creamGray",
    value: null,
    description: "bg.creamGrayDesc",
    category: "gradient",
    textColor: "#3a3a3a",
    gradientColors: ["#fdfcfb", "#e2d1c3", "#c9d6df"],
  },
  {
    name: "bg.sunsetGlow",
    value: null,
    description: "bg.sunsetGlowDesc",
    category: "gradient",
    textColor: "#ffffff",
    gradientColors: ["#fa709a", "#fee140"],
  },
  {
    name: "bg.auroraNight",
    value: null,
    description: "bg.auroraNightDesc",
    category: "gradient",
    textColor: "#e0e8ff",
    gradientColors: ["#0c1445", "#1a237e", "#283593"],
  },
];

function hexToRgb(hex: string): string {
  const clean = hex.replace("#", "");
  if (clean.length !== 6) return "79, 140, 255";
  const r = parseInt(clean.substring(0, 2), 16);
  const g = parseInt(clean.substring(2, 4), 16);
  const b = parseInt(clean.substring(4, 6), 16);
  return `${r}, ${g}, ${b}`;
}

function getContrastColor(hex: string): string {
  const rgb = hexToRgb(hex);
  const [r, g, b] = rgb.split(",").map(Number);
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  return luminance > 0.5 ? "#000000" : "#ffffff";
}

function isColorDark(hex: string): boolean {
  const rgb = hexToRgb(hex);
  const [r, g, b] = rgb.split(",").map(Number);
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  return luminance <= 0.5;
}

export const isDark = computed(() => {
  if (_isDarkForced.value !== null) return _isDarkForced.value;
  if (currentBgColor.value) return isColorDark(currentBgColor.value);
  return false;
});

function syncDarkClass() {
  if (isDark.value) {
    document.body.classList.add("dark");
  } else {
    document.body.classList.remove("dark");
  }
}

function applyBgColor(bgColor: string | null, gradientColors?: [string, ...string[]]) {
  currentBgColor.value = bgColor;
  const root = document.documentElement;
  const angle = 135;
  if (gradientColors && gradientColors.length >= 2) {
    const stops = gradientColors.join(", ");
    const srgbGradient = `linear-gradient(${angle}deg, ${stops})`;
    // 材质契约（THEME_DEV.md §6.18 step 3/4）：背景/渐变是主题材质的一部分。
    // canonical 令牌 --material-bg 持有渐变；--material-bg-p3 孪生（停色转 display-p3）
    // 关掉 §6.17 的 P3 背景缺口，仅真 P3 屏 + .encv-p3 时由 vivid.css 切换。
    // --ion-background-color / body 统一读中转变量 --material-bg-active（vivid.css 定义，
    // P3 下换 -p3 孪生），故所有磨砂/卡片背景跟随同一材质、且 P3 宽色域真正生效。
    const p3Stops = gradientColors.map(c => hexToP3Token(c) ?? c).join(", ");
    const p3Gradient = `linear-gradient(${angle}deg, ${p3Stops})`;
    root.style.setProperty("--material-bg", srgbGradient);
    root.style.setProperty("--material-bg-p3", p3Gradient);
    root.style.setProperty("--ion-background-color", "var(--material-bg-active)");
    root.style.setProperty("--ion-background-color-rgb", hexToRgb(gradientColors[0]));
    root.style.setProperty("--encv-bg-text-color", "#ffffff");
    root.style.setProperty("--encv-bg-gradient", srgbGradient);
    root.style.setProperty("--encv-bg-gradient-angle", `${angle}deg`);
    document.body.style.backgroundImage = "var(--material-bg-active)";
    document.body.style.backgroundSize = "cover";
    document.body.style.backgroundAttachment = "fixed";
  } else if (bgColor) {
    root.style.setProperty("--material-bg", bgColor);
    root.style.removeProperty("--material-bg-p3");
    root.style.removeProperty("--encv-bg-gradient");
    root.style.removeProperty("--encv-bg-gradient-angle");
    root.style.setProperty("--ion-background-color", "var(--material-bg-active)");
    root.style.setProperty("--ion-background-color-rgb", hexToRgb(bgColor));
    root.style.setProperty("--encv-bg-text-color", getContrastColor(bgColor));
    document.body.style.backgroundColor = "var(--material-bg-active)";
    document.body.style.backgroundImage = "";
  } else {
    root.style.removeProperty("--material-bg");
    root.style.removeProperty("--material-bg-p3");
    root.style.removeProperty("--ion-background-color");
    root.style.removeProperty("--ion-background-color-rgb");
    root.style.removeProperty("--encv-bg-text-color");
    root.style.removeProperty("--encv-bg-gradient");
    root.style.removeProperty("--encv-bg-gradient-angle");
    document.body.style.backgroundColor = "";
    document.body.style.backgroundImage = "";
  }
  syncDarkClass();
}

export function setBgColor(color: string | null) {
  if (!color) {
    localStorage.removeItem(BG_COLOR_KEY);
    applyBgColor(null);
    return;
  }
  localStorage.setItem(BG_COLOR_KEY, color);
  applyBgColor(color);
}

export function setBgGradient(colors: [string, ...string[]]) {
  const key = colors.join("|");
  localStorage.setItem(BG_COLOR_KEY, `gradient:${key}`);
  applyBgColor(null, colors);
}

function applyColor(color: string) {
  const root = document.documentElement;
  const rgb = hexToRgb(color);
  const contrast = getContrastColor(color);
  const prev = currentColor.value;

  // 单一语义源：写入 daisyUI 的 --color-primary，shade/tint 由 bridge.css
  // 经 color-mix(var(--color-primary) ...) 自动派生（Chromium 111+ ✅）。
  // 这样换主题/自定义主色时，Ionic 与 daisyUI 组件共用同一派生链，
  // useTheme 不再手搓 Ionic 的 shade/tint 数学（ACL 解耦：业务只认 --color-*）。
  // 派生令牌（Ionic -rgb / contrast / P3 孪生）始终即时写入，保证正确性（测试也依赖终态）。
  root.style.setProperty("--color-primary", color);
  // Ionic web component 仍需 -rgb / contrast（bridge.css 对自定义色不负责，见其注释）：
  root.style.setProperty("--ion-color-primary", color);
  root.style.setProperty("--ion-color-primary-rgb", rgb);
  root.style.setProperty("--ion-color-primary-contrast", contrast);
  root.style.setProperty("--ion-color-primary-contrast-rgb", hexToRgb(contrast));

  // 臻彩 P3：写出 display-p3 宽色域令牌（CSS 仅支持 P3 屏 + .encv-p3 时使用，见 vivid.css）。
  // 不再写死内置 7 色 —— 任意有效 hex（含自定义取色 / 远程主题色）一律自动归一化派生，
  // 使「臻彩」开关对全色域全自动生效；非法色移除令牌回退 srgb，不报错。
  const p3Token = hexToP3Token(color);
  if (p3Token) {
    root.style.setProperty("--color-primary-p3", p3Token);
    root.style.setProperty("--color-primary-srgb", color);
  } else {
    root.style.removeProperty("--color-primary-p3");
    root.style.removeProperty("--color-primary-srgb");
  }

  currentColor.value = color;
  localStorage.setItem(COLOR_KEY, color);

  // gsap 平滑过渡「视觉主色」（从 prev 到 color）：框架层不耦合，仅在此 composable 内；
  // reduced-motion / 无 gsap 时直接落终态（上面已写入）。onUpdate 在动画中逐帧覆盖
  // --color-primary（终态即 color，故测试/无动效环境也能拿到正确终值）。
  const profile = getMotionProfile();
  if (profile.enabled && typeof motion?.to === "function" && prev && prev !== color) {
    // hexToRgb 返回 "r, g, b" 字符串；这里用数值对象驱动 gsap 逐帧过渡（终态 = color）。
    const parse = (hex: string): { r: number; g: number; b: number } => {
      const c = hex.replace("#", "");
      return {
        r: parseInt(c.substring(0, 2), 16) || 0,
        g: parseInt(c.substring(2, 4), 16) || 0,
        b: parseInt(c.substring(4, 6), 16) || 0,
      };
    };
    const a = parse(prev);
    const b = parse(color);
    const proxy = { r: a.r, g: a.g, b: a.b };
    motion.to(proxy, {
      r: b.r,
      g: b.g,
      b: b.b,
      duration: 0.3,
      ease: "power2.out",
      overwrite: "auto",
      onUpdate() {
        root.style.setProperty("--color-primary", `rgb(${Math.round(proxy.r)}, ${Math.round(proxy.g)}, ${Math.round(proxy.b)})`);
      },
    });
  }
}

/** 复位主色到「主题自身」声明（移除内联覆盖，让 [data-theme] 级联的 --color-primary 生效）。
 *  仅移除自定义色相关令牌；--ion-color-*-rgb 由 themeLoader.syncIonicRgb 在切主题时已刷新为正确值，保留。 */
function resetColorToTheme(): void {
  const root = document.documentElement;
  root.style.removeProperty("--color-primary");
  root.style.removeProperty("--ion-color-primary");
  root.style.removeProperty("--color-primary-p3");
  root.style.removeProperty("--color-primary-srgb");
  currentColor.value = getThemePalette(activeThemeId.value)?.primary ?? currentColor.value;
}

/** 按当前激活主题的 per-theme 覆盖重放主色 / 背景（切换主题时由 watch 触发，initTheme 末也调用）。 */
function reapplyActiveTheme(): void {
  const ov = themeCustom.value[activeThemeId.value];
  if (ov?.primary) applyColor(ov.primary);
  else resetColorToTheme();

  const bg = ov?.bg;
  if (bg === undefined) applyBgColor(null);
  else if (typeof bg === "string" && bg.startsWith("gradient:")) {
    const colors = bg
      .substring(9)
      .split("|")
      .filter(c => /^#[0-9a-fA-F]{6}$/.test(c)) as [string, ...string[]];
    if (colors.length >= 2) applyBgColor(null, colors);
    else applyBgColor(null);
  } else if (bg === null) applyBgColor(null);
  else applyBgColor(bg);
}

function applyP3Mode(mode: "off" | "on" | "auto") {
  p3Mode.value = mode;
  const root = document.documentElement;
  // 真实 P3 仅由 .encv-p3 根类 + @media (color-gamut: p3) 控制（见 vivid.css）：
  // 'off' 移除类；'on'/'auto' 加类。@media 保证真实 P3 仅在有 P3 屏幕时生效，
  // 非 P3 屏下 P3 模式仅强化 vivid 饱和（见上方滤镜），不再写无效的 color-gamut 属性。
  if (mode === "off") root.classList.remove("encv-p3");
  else root.classList.add("encv-p3");
  localStorage.setItem(P3_KEY, mode);
}

/** 滑块值(50..200) → 归一化 vivid 量(0..1)：
 *  50→0（近关）、100→0.34（默认即可见，修复旧版「默认=恒等滤镜」的 bug）、200→1（最浓）。
 *  色彩浓度 / 对比度各自独立走同一映射，互不影响（2026-07-17 拆分优化）。 */
function vividAmountFromValue(value: number): number {
  const clamped = Math.max(50, Math.min(200, value));
  return Math.round(((clamped - 50) / 150) * 100) / 100;
}

/** 任意有效 hex 色 → display-p3 宽色域令牌（0..1 归一化直接塞入更宽基色 = 更艳）。
 *  返回 null 表示非法色（调用方移除令牌回退 srgb，不报错）。
 *  覆盖 3 位 / 6 位 hex；让「臻彩」开关对内置主题色、自定义取色、远程主题色
 *  全自动生效，无需预置坐标表。 */
function hexToP3Token(hex: string): string | null {
  const clean = hex.replace("#", "").toLowerCase();
  let r: number, g: number, b: number;
  if (clean.length === 3) {
    r = parseInt(clean[0] + clean[0], 16);
    g = parseInt(clean[1] + clean[1], 16);
    b = parseInt(clean[2] + clean[2], 16);
  } else if (clean.length === 6) {
    r = parseInt(clean.substring(0, 2), 16);
    g = parseInt(clean.substring(2, 4), 16);
    b = parseInt(clean.substring(4, 6), 16);
  } else {
    return null;
  }
  if ([r, g, b].some(n => Number.isNaN(n))) return null;
  const f = (n: number) => (n / 255).toFixed(4);
  return `color(display-p3 ${f(r)} ${f(g)} ${f(b)})`;
}

function applyVividMode(mode: "off" | "on") {
  vividMode.value = mode;
  const root = document.documentElement;
  // 加/去 .encv-vivid 根类：guard.ts / tokens.css 的动效强度 1.3 boost 依赖此（此前从未添加）。
  if (mode === "on") root.classList.add("encv-vivid");
  else root.classList.remove("encv-vivid");
  syncVividFilter();
  localStorage.setItem(VIVID_KEY, mode);
}

function applyVividSaturation(value: number) {
  vividSaturation.value = Math.max(50, Math.min(200, value));
  syncVividFilter();
  localStorage.setItem(`${VIVID_KEY}-saturation`, String(vividSaturation.value));
}

function applyVividContrast(value: number) {
  vividContrast.value = Math.max(50, Math.min(200, value));
  syncVividFilter();
  localStorage.setItem(`${VIVID_KEY}-contrast`, String(vividContrast.value));
}

function syncVividFilter() {
  const root = document.documentElement;
  const off = vividMode.value === "off";
  const satTarget = off ? 0 : vividAmountFromValue(vividSaturation.value);
  const conTarget = off ? 0 : vividAmountFromValue(vividContrast.value);
  const profile = getMotionProfile();
  // gsap 平滑过渡两个独立量（色彩浓度 / 对比度，MotionEngine ACL，可直接换 anime.js）；
  // reduced-motion 下直接落终态。
  if (profile.enabled && typeof motion?.to === "function") {
    motion.to(root, {
      "--encv-vivid-sat": satTarget,
      "--encv-vivid-contrast": conTarget,
      duration: 0.32,
      ease: "power2.out",
      overwrite: "auto",
    });
  } else {
    root.style.setProperty("--encv-vivid-sat", String(satTarget));
    root.style.setProperty("--encv-vivid-contrast", String(conTarget));
  }
}

/** 设置「当前激活主题」的主色覆盖（per-theme 持久化）。 */
export function setThemePrimary(color: string): void {
  const id = activeThemeId.value;
  if (!themeCustom.value[id]) themeCustom.value[id] = {};
  themeCustom.value[id].primary = color;
  saveThemeCustom();
  applyColor(color);
}

/** 设置「当前激活主题」的背景色覆盖（per-theme 持久化）；null = 用主题默认。 */
export function setThemeBg(color: string | null): void {
  const id = activeThemeId.value;
  if (!themeCustom.value[id]) themeCustom.value[id] = {};
  themeCustom.value[id].bg = color;
  saveThemeCustom();
  applyBgColor(color);
}

/** 复位「当前激活主题」的主色到主题声明。 */
export function resetThemePrimary(): void {
  const id = activeThemeId.value;
  if (themeCustom.value[id]) {
    delete themeCustom.value[id].primary;
    saveThemeCustom();
  }
  resetColorToTheme();
}

/** 复位「当前激活主题」的背景色到主题声明。 */
export function resetThemeBg(): void {
  const id = activeThemeId.value;
  if (themeCustom.value[id]) {
    delete themeCustom.value[id].bg;
    saveThemeCustom();
  }
  applyBgColor(null);
}

// 切换主题时重放该主题的 per-theme 覆盖（或回落主题默认）。框架层解耦：
// 仅监听 useUserThemes 暴露的 activeThemeId，不反向依赖。
watch(activeThemeId, () => reapplyActiveTheme());

function supportsP3(): boolean {
  if (typeof window === "undefined") return false;
  return window.matchMedia("(color-gamut: p3)").matches;
}

function initTheme() {
  const stored = localStorage.getItem(THEME_KEY);
  if (stored !== null) {
    _isDarkForced.value = stored === "dark";
  } else {
    _isDarkForced.value = null;
    const prefersDark = window.matchMedia("(prefers-color-scheme: dark)");
    prefersDark.addEventListener("change", e => {
      if (_isDarkForced.value === null && !currentBgColor.value) {
        _isDarkForced.value = e.matches;
        syncDarkClass();
      }
    });
  }

  // 迁移：旧全局主题色 / 背景色 → 当前激活主题的 per-theme 覆盖（若尚无覆盖则升级为覆盖，保留用户选择）。
  // 之后统一由 reapplyActiveTheme() 重放（无覆盖则回落主题自身声明，符合「不再固定全局」的意图）。
  const id = activeThemeId.value;
  const ov = (themeCustom.value[id] ??= {});
  const legacyColor = localStorage.getItem(COLOR_KEY);
  if (legacyColor && /^#[0-9a-fA-F]{6}$/.test(legacyColor) && ov.primary === undefined) {
    ov.primary = legacyColor;
    saveThemeCustom();
  }
  const legacyBg = localStorage.getItem(BG_COLOR_KEY);
  if (legacyBg && ov.bg === undefined) {
    if (legacyBg.startsWith("gradient:")) {
      const colorsStr = legacyBg.substring(9);
      const colors = colorsStr.split("|").filter(c => /^#[0-9a-fA-F]{6}$/.test(c));
      if (colors.length >= 2) ov.bg = `gradient:${colors.join("|")}`;
    } else if (/^#[0-9a-fA-F]{6}$/.test(legacyBg)) {
      ov.bg = legacyBg;
    }
    if (ov.bg !== undefined) saveThemeCustom();
  }
  syncDarkClass();

  // P3 在删除「自动/始终开启/关闭」选项组后变为「自动」：始终按 auto 行为（.encv-p3 常驻，
  // 真实 P3 屏由 @media (color-gamut:p3) 决定，srgb 屏仅强化 vivid 饱和）。不再读旧 storedP3，
  // 旧「off」偏好随之失效（符合删除冗余选项组的意图）。
  applyP3Mode("auto");

  const storedVivid = localStorage.getItem(VIVID_KEY) as "off" | "on" | null;
  if (storedVivid && ["off", "on"].includes(storedVivid)) {
    vividMode.value = storedVivid;
  }
  const storedSat = localStorage.getItem(`${VIVID_KEY}-saturation`);
  if (storedSat !== null) {
    const v = parseInt(storedSat, 10);
    if (!Number.isNaN(v)) vividSaturation.value = v;
  }
  const storedCon = localStorage.getItem(`${VIVID_KEY}-contrast`);
  if (storedCon !== null) {
    const v = parseInt(storedCon, 10);
    if (!Number.isNaN(v)) vividContrast.value = v;
  }
  // 复应用 vivid 根类 + 滤镜：此前只置 vividMode.value 却没调用 applyVividMode，
  // 导致刷新后 .encv-vivid 类丢失 → 滤镜不挂（实测「臻彩显示」刷新即失效）。
  applyVividMode(vividMode.value);
  // 按当前激活主题的 per-theme 覆盖（或主题默认）重放主色 / 背景。
  reapplyActiveTheme();
}

function setThemeColor(color: string) {
  setThemePrimary(color);
}

/** 语义化别名（与 setThemeColor 等价），指向 daisyUI 语义主色令牌。 */
export const setPrimaryColor = setThemeColor;

export function setP3Mode(mode: "off" | "on" | "auto") {
  applyP3Mode(mode);
}

export function setVividMode(mode: "off" | "on") {
  applyVividMode(mode);
}

export function setVividSaturation(value: number) {
  applyVividSaturation(value);
}

export function setVividContrast(value: number) {
  applyVividContrast(value);
}

const isP3Supported = ref(false);

function detectP3Support() {
  isP3Supported.value = supportsP3();
}

export function useTheme() {
  return {
    isDark,
    currentColor,
    currentBgColor,
    p3Mode,
    vividMode,
    vividSaturation,
    vividContrast,
    isP3Supported,
    activeThemePalette,
    themeCustom,
    initTheme,
    detectP3Support,
    setThemeColor,
    setPrimaryColor,
    setThemePrimary,
    setThemeBg,
    resetThemePrimary,
    resetThemeBg,
    setBgColor,
    setBgGradient,
    setP3Mode,
    setVividMode,
    setVividSaturation,
    setVividContrast,
    THEME_PRESETS,
    BG_PRESETS,
  };
}
