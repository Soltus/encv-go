import { describe, it, expect } from "vitest";
import * as sass from "sass-embedded";
import fs from "node:fs";
import path from "node:path";
import { USER_THEMES } from "@encv/shared-components/composables/useUserThemes";

// 解析 surface.scss 入口：vitest 下 import.meta.url 不一定是 file:// 方案，
// 故用 process.cwd() + 候选路径探测，避免 "URL must be of scheme file"。
function resolveEntry(): string {
  const candidates = [
    path.resolve(process.cwd(), "..", "packages", "shared-components", "src", "theme", "surface.scss"),
    path.resolve(process.cwd(), "packages", "shared-components", "src", "theme", "surface.scss"),
    path.resolve(__dirname, "..", "..", "..", "..", "packages", "shared-components", "src", "theme", "surface.scss"),
  ];
  for (const c of candidates) {
    if (fs.existsSync(c)) return c;
  }
  return candidates[0]!;
}

const entry = resolveEntry();

const css = sass.compile(entry, {
  loadPaths: [path.dirname(entry)],
  quietDeps: true,
  silenceDeprecations: ["import", "global-builtin", "color-functions"],
}).css;

describe("surface.scss — SCSS 生成 + 主题无关契约（续35：官方==第三方）", () => {
  it("没有任何主题在编译期被烘焙（官方==第三方，均走运行时 color-mix 派生）", () => {
    // 续35：删除 _theme-palette.scss 后，编译产物不得出现 [data-theme=X]{ --tint-* / --color-*-hover }
    // 声明——所有 tint/hover 都由 :root 的 color-mix(var(--color-*)) 运行时派生，官方与第三方零特权差异。
    expect(css).not.toMatch(/\[data-theme=[^\]]*\]\s*\{[^}]*--tint-primary-bg\s*:/);
    expect(css).not.toMatch(/\[data-theme=[^\]]*\]\s*\{[^}]*--color-primary-hover\s*:/);
  });

  it("hover/active 由 :root 运行时 color-mix(var(--color-*)) 统一派生（官方==第三方均可用）", () => {
    // 续35：hover/active 不再按主题编译，而是在 :root 用 color-mix(var(--color-*)) 派生。
    // 因 var(--color-*) 按当前主题解析，单个 :root 定义对官方与第三方主题同样生效
    // （同时修复了 sunset/mint 等第三方主题按钮 hover 失效的既有 bug）。
    expect(css).toMatch(/--color-primary-hover:\s*color-mix\([^;]*var\(--color-primary\)[^;]*\)/);
    expect(css).toMatch(/--color-primary-active:\s*color-mix\([^;]*var\(--color-primary\)[^;]*\)/);
    expect(css).toMatch(/--color-success-hover:\s*color-mix\([^;]*var\(--color-success\)[^;]*\)/);
    expect(css).not.toMatch(/\[data-theme=[^\]]*\]\s*\{[^}]*--color-primary-hover\s*:/);
  });

  it("交互令牌全局可用（不随 data-theme 失效）", () => {
    expect(css).toMatch(/--elevation-chip-hover:/);
    expect(css).toMatch(/--lift-hover:/);
    expect(css).toMatch(/--press-scale:/);
    expect(css).toMatch(/--ring-focus:/);
  });

  it("表面类消费 --tint-* 令牌并保留运行时 var() 回退（任意主题换肤生效）", () => {
    expect(css).toMatch(/\.ui-chip\s*\{[^}]*--tint-primary-bg,\s*color-mix\(/);
    expect(css).toContain(".ui-badge--success");
    expect(css).toContain(".ui-badge--warning");
    expect(css).toContain(".ui-badge--error");
    expect(css).toContain(".ui-chip--neutral");
  });

  it("基态与变体均走单一 map 驱动（无 @if 100% 魔法分支 / pill-base 不再重复声明 gap）", () => {
    // badge 自带 gap 单次，不应出现 pill-base 的 gap:var(--gap-chip) 再被覆盖
    expect(css).toMatch(/\.ui-badge\s*\{[^}]*gap:\s*0\.25rem/);
    expect(css).not.toMatch(/\.ui-badge\s*\{[^}]*gap:\s*var\(--gap-chip\)[^}]*gap:\s*0\.25rem/);
  });

  it("暗色官方主题（ocean/midnight/encv-dark）同样走运行时派生，无编译期 --tint-* 声明", () => {
    for (const id of ["ocean", "midnight", "encv-dark"]) {
      expect(css).not.toMatch(new RegExp(`\\[data-theme=["']?${id}["']?\\]\\s*\\{[^}]*--tint-`));
    }
  });

  it("官方(builtIn)主题 = 注册表 + 纯 CSS 数据资产，与第三方同待遇（无编译期特权）", () => {
    const builtIn = USER_THEMES.filter(t => t.builtIn).map(t => t.id);
    // 6 个官方主题：encv / encv-dark（紫）+ 续34 新增 rose / ocean / forest / midnight
    expect(builtIn).toEqual(expect.arrayContaining(["encv", "encv-dark", "rose", "ocean", "forest", "midnight"]));
    expect(builtIn.length).toBeGreaterThanOrEqual(6);
    // 关键契约（续35）：官方主题不得拥有任何编译期特权——编译产物中不得出现
    // [data-theme=id]{ --tint-* / --color-*-hover } 声明。它们与第三方主题一样，
    // 仅靠 palette.css 的 [data-theme] 基色块（纯 CSS 数据）+ 运行时派生。
    for (const id of builtIn) {
      expect(css).not.toMatch(new RegExp(`\\[data-theme=["']?${id}["']?\\]\\s*\\{[^}]*--tint-primary-bg\\s*:`));
      expect(css).not.toMatch(new RegExp(`\\[data-theme=["']?${id}["']?\\]\\s*\\{[^}]*--color-primary-hover\\s*:`));
    }
  });

  it("用户主题 API 可用（blend / surface mixin 存在且能编译）", () => {
    // _user-theme 仅含 mixin/function，不应向项目 CSS 注入额外规则
    expect(css).not.toMatch(/ut\.surface/);
  });
});
