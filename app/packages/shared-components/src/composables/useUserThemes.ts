import { computed, ref } from "vue";

/**
 * Phase 5 · 用户主题运行时闭环（encv-mobile 纯 CSS 路径）。
 *
 * 主题 = 一组 `[data-theme="<id>"]` 的纯 CSS 调色板覆盖（见 theme/user-themes.css）。
 * 这里只负责：注册可用主题、把选中主题写到 documentElement 的 data-theme 属性、
 * 持久化偏好、模块加载即水合。
 *
 * 与插件 web 路径的区别：插件 web 走 daisyUI @plugin 编译期注册，运行时只能切
 * 换「已注册名」；主应用走纯 CSS，故可零构建地热切换任意已登记的 data-theme。
 */

export interface UserThemeMeta {
  /** data-theme 属性值；encv 为默认（移除属性即回落 :root）。 */
  id: string;
  /** i18n 标签键。 */
  nameKey: string;
  /** 是否为内置主题（encv / encv-dark）。 */
  builtIn?: boolean;
}

const USER_THEME_KEY = "encv-user-theme";

/** 可用主题注册表：内置 + 示例用户主题。新增主题在此登记 + 在 user-themes.css 加块。 */
export const USER_THEMES: UserThemeMeta[] = [
  { id: "encv", nameKey: "settings.themeBuiltIn", builtIn: true },
  { id: "encv-dark", nameKey: "settings.themeBuiltInDark", builtIn: true },
  { id: "sunset", nameKey: "settings.userThemeSunset" },
  { id: "mint", nameKey: "settings.userThemeMint" },
];

const DEFAULT_THEME = "encv";

const activeThemeId = ref<string>(DEFAULT_THEME);

function applyToDom(id: string) {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  // 默认主题：移除属性，回落 :root（encv 调色板），避免与暗色 bgColor 逻辑叠加。
  if (id === DEFAULT_THEME) {
    root.removeAttribute("data-theme");
  } else {
    root.setAttribute("data-theme", id);
  }
}

function readStored(): string | null {
  if (typeof localStorage === "undefined") return null;
  const v = localStorage.getItem(USER_THEME_KEY);
  if (v && USER_THEMES.some(t => t.id === v)) return v;
  return null;
}

// 模块加载即水合：恢复上次选中的用户主题。
activeThemeId.value = readStored() ?? DEFAULT_THEME;
applyToDom(activeThemeId.value);

export function useUserThemes() {
  const activeTheme = computed(() => activeThemeId.value);

  function applyTheme(id: string) {
    if (!USER_THEMES.some(t => t.id === id)) return;
    activeThemeId.value = id;
    applyToDom(id);
    if (typeof localStorage === "undefined") return;
    if (id === DEFAULT_THEME) localStorage.removeItem(USER_THEME_KEY);
    else localStorage.setItem(USER_THEME_KEY, id);
  }

  function isActive(id: string): boolean {
    return activeThemeId.value === id;
  }

  return { themes: USER_THEMES, activeTheme, applyTheme, isActive };
}
