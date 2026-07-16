/**
 * 外观：per-theme 主色 / 背景色定制（2026-07-17 优化）
 *
 * 复现优先：本测试覆盖「不再用固定全局预设 + 按主题分别保存覆盖」的核心逻辑。
 * 为避免 happy-dom 下 themeLoader 的真实网络拉取（activateTheme 会 fetch theme.css），
 * 直接驱动 useUserThemes 暴露的 activeThemeId（与 AppearanceDetail 点击卡片同一条状态链），
 * 由 useTheme 的 watch 触发 reapplyActiveTheme 重放 per-theme 覆盖。
 *
 * 注：组内断言以「响应式真值（currentColor / themeCustom）+ 持久化」为准，
 * 因为 gsap 会在 0.3s 内把 --color-primary 从 hex 过渡为 rgb（终态 = 同色），
 * 终态 DOM 表现由 Playwright 真实浏览器测试（appearance-customize.visual.ts）守门。
 */
import { beforeEach, describe, expect, it } from "vitest";
import { nextTick } from "vue";
import { resolve } from "node:path";
import { existsSync, readFileSync } from "node:fs";
import { useTheme } from "@encv/shared-components/composables/useTheme";
import { useUserThemes, activeThemeId, getThemePalette, USER_THEMES } from "@encv/shared-components/composables/useUserThemes";

const CUSTOM_KEY = "encv-theme-custom";

describe("外观：per-theme 主题色 / 背景色定制", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.style.cssText = "";
    // 复位到默认主题，并清空任何残留覆盖，保证用例间隔离
    activeThemeId.value = "encv";
    const t = useTheme();
    (t.themeCustom as { value: Record<string, unknown> }).value = {};
  });

  it("setThemePrimary 写入响应式主色并持久化到「当前主题」覆盖", () => {
    const t = useTheme();
    activeThemeId.value = "encv";
    t.setThemePrimary("#06b6d4");

    expect(t.currentColor.value).toBe("#06b6d4");
    const stored = JSON.parse(localStorage.getItem(CUSTOM_KEY) || "{}");
    expect(stored["encv"]?.primary).toBe("#06b6d4");
  });

  it("切换主题后 per-theme 覆盖互相隔离，并随主题重放", async () => {
    const t = useTheme();

    activeThemeId.value = "encv";
    t.setThemePrimary("#06b6d4"); // 覆盖 encv
    activeThemeId.value = "ocean";
    await nextTick();
    t.setThemePrimary("#22d3ee"); // 覆盖 ocean（与 encv 互不影响）
    await nextTick();

    const stored = JSON.parse(localStorage.getItem(CUSTOM_KEY) || "{}");
    expect(stored["encv"]?.primary).toBe("#06b6d4");
    expect(stored["ocean"]?.primary).toBe("#22d3ee");

    // 切回 encv → 重放其覆盖（currentColor 回落 #06b6d4，非 ocean 的色）
    activeThemeId.value = "encv";
    await nextTick();
    expect(t.currentColor.value).toBe("#06b6d4");

    // 切到 ocean → 重放其覆盖
    activeThemeId.value = "ocean";
    await nextTick();
    expect(t.currentColor.value).toBe("#22d3ee");
  });

  it("resetThemePrimary 移除覆盖并回落该主题声明的默认主色", () => {
    const t = useTheme();
    activeThemeId.value = "forest";
    const forestDefault = getThemePalette("forest")?.primary;
    t.setThemePrimary("#65a30d");
    expect(t.currentColor.value).toBe("#65a30d");

    t.resetThemePrimary();
    // 回落主题声明的默认主色（非固定全局色）
    expect(t.currentColor.value).toBe(forestDefault);
    const stored = JSON.parse(localStorage.getItem(CUSTOM_KEY) || "{}");
    expect(stored["forest"]?.primary).toBeUndefined();
  });

  it("setThemeBg 写入 --material-bg 并按主题持久化覆盖", () => {
    const t = useTheme();
    activeThemeId.value = "sunset";
    t.setThemeBg("#123456");

    expect(document.documentElement.style.getPropertyValue("--material-bg")).toBe("#123456");
    const stored = JSON.parse(localStorage.getItem(CUSTOM_KEY) || "{}");
    expect(stored["sunset"]?.bg).toBe("#123456");
  });

  it("resetThemeBg 移除背景覆盖（--material-bg 被清除，回落主题材质）", () => {
    const t = useTheme();
    activeThemeId.value = "mint";
    t.setThemeBg("#0f766e");
    expect(document.documentElement.style.getPropertyValue("--material-bg")).toBe("#0f766e");

    t.resetThemeBg();
    expect(document.documentElement.style.getPropertyValue("--material-bg")).toBe("");
    const stored = JSON.parse(localStorage.getItem(CUSTOM_KEY) || "{}");
    expect(stored["mint"]?.bg).toBeUndefined();
  });

  it("getThemePalette 返回各主题声明的预设（取色器驱动源，不再固定全局）", () => {
    for (const id of USER_THEMES.map(t => t.id)) {
      const p = getThemePalette(id);
      expect(p, `主题 ${id} 应有调色板声明`).toBeDefined();
      expect(p?.primary, `${id} 应声明默认主色`).toMatch(/^#[0-9a-fA-F]{6}$/);
      expect(Array.isArray(p?.presets?.primary), `${id} 应有主色预设数组`).toBe(true);
    }
  });

  it("契约锁：USER_THEMES 声明与各 theme.json 的 palette 一致（忠实镜像）", () => {
    for (const theme of USER_THEMES) {
      const p = theme.palette;
      expect(p, `USER_THEMES[${theme.id}] 应有 palette`).toBeDefined();
      expect(p?.primary, `${theme.id}.palette.primary 缺失`).toBeDefined();
      expect(Array.isArray(p?.presets?.primary), `${theme.id} 应声明主色预设`).toBe(true);

      // 镜像校验：public/themes/<id>/theme.json 的 palette 必须与 TS 声明一致
      const jsonPath = resolve(process.cwd(), `public/themes/${theme.id}/theme.json`);
      expect(existsSync(jsonPath), `theme.json 缺失: ${jsonPath}`).toBe(true);
      const json = JSON.parse(readFileSync(jsonPath, "utf8"));
      expect(json.palette?.primary, `${theme.id} theme.json palette.primary 应与 TS 一致`).toBe(p?.primary);
      expect(JSON.stringify(json.palette?.presets?.primary), `${theme.id} theme.json 主色预设应与 TS 一致`).toBe(
        JSON.stringify(p?.presets?.primary)
      );
    }
  });
});
