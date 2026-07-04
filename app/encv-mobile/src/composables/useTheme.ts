import { computed, ref } from "vue";

const THEME_KEY = "encv-theme-preference";
const COLOR_KEY = "encv-theme-color";
const BG_COLOR_KEY = "encv-bg-color";
const BG_BLUR_KEY = "encv-bg-blur";
const P3_KEY = "encv-p3-mode";
const VIVID_KEY = "encv-vivid-mode";

const _isDarkForced = ref<boolean | null>(null);
const currentColor = ref("#4f8cff");
const currentBgColor = ref<string | null>(null);
const bgBlur = ref(12);
const p3Mode = ref<"off" | "on" | "auto">("auto");
const vividMode = ref<"off" | "on">("off");
const vividIntensity = ref(100);

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
  if (gradientColors && gradientColors.length >= 2) {
    const angle = 135;
    const stops = gradientColors.join(", ");
    root.style.setProperty("--ion-background-color", `linear-gradient(${angle}deg, ${stops})`);
    root.style.setProperty("--ion-background-color-rgb", hexToRgb(gradientColors[0]));
    root.style.setProperty("--encv-bg-text-color", "#ffffff");
    root.style.setProperty("--encv-bg-gradient", `linear-gradient(${angle}deg, ${stops})`);
    root.style.setProperty("--encv-bg-gradient-angle", `${angle}deg`);
    document.body.style.backgroundImage = `linear-gradient(${angle}deg, ${stops})`;
    document.body.style.backgroundSize = "cover";
    document.body.style.backgroundAttachment = "fixed";
  } else if (bgColor) {
    root.style.removeProperty("--encv-bg-gradient");
    root.style.removeProperty("--encv-bg-gradient-angle");
    root.style.setProperty("--ion-background-color", bgColor);
    root.style.setProperty("--ion-background-color-rgb", hexToRgb(bgColor));
    root.style.setProperty("--encv-bg-text-color", getContrastColor(bgColor));
    document.body.style.backgroundColor = bgColor;
    document.body.style.backgroundImage = "";
  } else {
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
  currentColor.value = color;
  const root = document.documentElement;
  const rgb = hexToRgb(color);
  const contrast = getContrastColor(color);

  const lighter = (hex: string, percent: number): string => {
    const clean = hex.replace("#", "");
    let r = parseInt(clean.substring(0, 2), 16);
    let g = parseInt(clean.substring(2, 4), 16);
    let b = parseInt(clean.substring(4, 6), 16);
    r = Math.min(255, Math.round(r + ((255 - r) * percent) / 100));
    g = Math.min(255, Math.round(g + ((255 - g) * percent) / 100));
    b = Math.min(255, Math.round(b + ((255 - b) * percent) / 100));
    return `#${r.toString(16).padStart(2, "0")}${g.toString(16).padStart(2, "0")}${b.toString(16).padStart(2, "0")}`;
  };

  const darker = (hex: string, percent: number): string => {
    const clean = hex.replace("#", "");
    let r = parseInt(clean.substring(0, 2), 16);
    let g = parseInt(clean.substring(2, 4), 16);
    let b = parseInt(clean.substring(4, 6), 16);
    r = Math.max(0, Math.round(r * (1 - percent / 100)));
    g = Math.max(0, Math.round(g * (1 - percent / 100)));
    b = Math.max(0, Math.round(b * (1 - percent / 100)));
    return `#${r.toString(16).padStart(2, "0")}${g.toString(16).padStart(2, "0")}${b.toString(16).padStart(2, "0")}`;
  };

  root.style.setProperty("--ion-color-primary", color);
  root.style.setProperty("--ion-color-primary-rgb", rgb);
  root.style.setProperty("--ion-color-primary-contrast", contrast);
  root.style.setProperty("--ion-color-primary-contrast-rgb", hexToRgb(contrast));
  root.style.setProperty("--ion-color-primary-shade", darker(color, 10));
  root.style.setProperty("--ion-color-primary-tint", lighter(color, 10));

  localStorage.setItem(COLOR_KEY, color);
}

function applyBgBlur(blur: number) {
  bgBlur.value = Math.max(0, Math.min(40, blur));
  const root = document.documentElement;
  root.style.setProperty("--encv-bg-blur", `${bgBlur.value}px`);
  localStorage.setItem(BG_BLUR_KEY, String(bgBlur.value));
}

function applyP3Mode(mode: "off" | "on" | "auto") {
  p3Mode.value = mode;
  const root = document.documentElement;
  if (mode === "on") {
    root.style.setProperty("--encv-color-gamut", "display-p3");
    root.classList.add("encv-force-p3");
  } else if (mode === "off") {
    root.style.setProperty("--encv-color-gamut", "srgb");
    root.classList.remove("encv-force-p3");
  } else {
    root.style.removeProperty("--encv-color-gamut");
    root.classList.remove("encv-force-p3");
  }
  localStorage.setItem(P3_KEY, mode);
}

function applyVividMode(mode: "off" | "on") {
  vividMode.value = mode;
  syncVividFilter();
  localStorage.setItem(VIVID_KEY, mode);
}

function applyVividIntensity(value: number) {
  vividIntensity.value = Math.max(50, Math.min(200, value));
  syncVividFilter();
  localStorage.setItem(`${VIVID_KEY}-intensity`, String(vividIntensity.value));
}

function syncVividFilter() {
  const root = document.documentElement;
  if (vividMode.value === "off") {
    root.style.removeProperty("--encv-vivid-filter");
  } else {
    const s = vividIntensity.value / 100;
    const contrast = 1 + (s - 1) * 0.25;
    const saturate = 1 + (s - 1) * 0.5;
    root.style.setProperty("--encv-vivid-filter", `contrast(${contrast.toFixed(2)}) saturate(${saturate.toFixed(2)})`);
  }
}

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

  const storedColor = localStorage.getItem(COLOR_KEY);
  if (storedColor && /^#[0-9a-fA-F]{6}$/.test(storedColor)) {
    applyColor(storedColor);
  }

  const storedBg = localStorage.getItem(BG_COLOR_KEY);
  if (storedBg) {
    if (storedBg.startsWith("gradient:")) {
      const colorsStr = storedBg.substring(9);
      const colors = colorsStr.split("|").filter(c => /^#[0-9a-fA-F]{6}$/.test(c)) as [string, ...string[]];
      if (colors.length >= 2) applyBgColor(null, colors);
    } else if (/^#[0-9a-fA-F]{6}$/.test(storedBg)) {
      applyBgColor(storedBg);
    }
  }
  syncDarkClass();

  const storedBlur = localStorage.getItem(BG_BLUR_KEY);
  if (storedBlur !== null) {
    const v = parseInt(storedBlur, 10);
    if (!isNaN(v)) applyBgBlur(v);
  }

  const storedP3 = localStorage.getItem(P3_KEY) as "off" | "on" | "auto" | null;
  if (storedP3 && ["off", "on", "auto"].includes(storedP3)) {
    applyP3Mode(storedP3);
  }

  const storedVivid = localStorage.getItem(VIVID_KEY) as "off" | "on" | null;
  if (storedVivid && ["off", "on"].includes(storedVivid)) {
    vividMode.value = storedVivid;
  }
  const storedIntensity = localStorage.getItem(`${VIVID_KEY}-intensity`);
  if (storedIntensity !== null) {
    const v = parseInt(storedIntensity, 10);
    if (!isNaN(v)) vividIntensity.value = v;
  }
  syncVividFilter();
}

function setThemeColor(color: string) {
  applyColor(color);
}

export function setBgBlur(blur: number) {
  applyBgBlur(blur);
}

export function setP3Mode(mode: "off" | "on" | "auto") {
  applyP3Mode(mode);
}

export function setVividMode(mode: "off" | "on") {
  applyVividMode(mode);
}

export function setVividIntensity(value: number) {
  applyVividIntensity(value);
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
    bgBlur,
    p3Mode,
    vividMode,
    vividIntensity,
    isP3Supported,
    initTheme,
    detectP3Support,
    setThemeColor,
    setBgColor,
    setBgGradient,
    setBgBlur,
    setP3Mode,
    setVividMode,
    setVividIntensity,
    THEME_PRESETS,
    BG_PRESETS,
  };
}
