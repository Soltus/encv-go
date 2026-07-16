// 表面材质（surface material）契约回归测试 —— 模糊令牌篇。
//
// 历史 bug（复现优先：本测试在修复前为 RED）：
//   高斯模糊长期是「全局开关」（`--encv-bg-blur`），实测它**不全局**——
//   codemogger_grep 证实只有 App.vue + HomePage header 读它，约 27 处组件
//   硬编码 `backdrop-filter: blur(8/12/20px)` 根本不读它。模糊应改为
//   **主题材质的一部分**（`--material-blur`），所有磨砂面统一读它，且由主题控制。
//
// 修复后：`applyBgBlur` / `setBgBlur` 写 canonical 的 `--material-blur`
// （`--encv-bg-blur` 仅保留为兼容别名，待散落硬编码站点迁移后移除）。
import { vi } from "vitest";
import { beforeEach, describe, expect, it } from "vitest";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, resolve } from "node:path";

// 回归锁：扫描两套源码树，确认没有任何磨砂面还硬编码 `backdrop-filter: blur(<px>)`
// 字面量（全部应改为 `blur(var(--material-blur, <px>))`）。剥离 `/* */` 与 `//` 注释，
// 避免 ServerStatusCard.vue 等文件里的说明性注释触发误报。
const SCAN_EXT = new Set([".vue", ".css"]);
const SKIP_DIRS = new Set(["node_modules", ".codemogger", "dist", "build", ".output", "coverage"]);

type BareHit = { file: string; line: number; text: string };
function walk(dir: string, hits: BareHit[]): void {
  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    return;
  }
  for (const name of entries) {
    if (SKIP_DIRS.has(name)) continue;
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      walk(full, hits);
      continue;
    }
    if (!SCAN_EXT.has(name.slice(name.lastIndexOf(".")))) continue;
    const stripped = readFileSync(full, "utf8")
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .replace(/\/\/[^\n]*/g, "");
    stripped.split("\n").forEach((ln, i) => {
      if (/(?:-webkit-)?backdrop-filter\s*:\s*blur\(\s*\d/.test(ln)) {
        hits.push({ file: full, line: i + 1, text: ln.trim() });
      }
    });
  }
}

const engine = vi.hoisted(() => {
  const applyVars = (target: any, vars?: Record<string, unknown>) => {
    if (target && target.style && vars) {
      for (const [k, v] of Object.entries(vars)) {
        if (k.startsWith("--")) target.style.setProperty(k, String(v));
      }
    }
  };
  return {
    registerPlugins: vi.fn(),
    set: vi.fn(),
    to: vi.fn((target: any, vars?: Record<string, unknown>) => {
      applyVars(target, vars);
      return { kill: vi.fn() };
    }),
    from: vi.fn((_t: unknown, _v?: unknown) => ({ kill: vi.fn() })),
    fromTo: vi.fn((_t: unknown, _f?: unknown, _v?: unknown) => ({ kill: vi.fn() })),
    context: vi.fn(() => ({ revert: vi.fn() })),
    quickTo: vi.fn(() => () => {}),
    delayedCall: vi.fn(() => ({ kill: vi.fn() })),
    createScrollTrigger: vi.fn(() => ({ kill: vi.fn(), progress: 0 })),
    flipGetState: vi.fn(),
    flipFrom: vi.fn(),
  };
});

vi.mock("@encv/shared-components/motion/internal", () => ({ motion: engine, noopMotion: engine }));

import { useTheme } from "@encv/shared-components/composables/useTheme";

describe("表面材质：背景模糊不再是独立全局设置（2026-07-17 优化）", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("useTheme 不再导出 setBgBlur / bgBlur（模糊改由主题材质 --material-blur 接管，设置项已删除）", () => {
    const api = useTheme();
    expect((api as Record<string, unknown>).setBgBlur).toBeUndefined();
    expect((api as Record<string, unknown>).bgBlur).toBeUndefined();
  });

  it("--material-blur 在共享主题变量中有默认定义（移除设置项后磨砂面仍有合理默认）", () => {
    const file = resolve(process.cwd(), "../packages/shared-components/src/theme/variables.css");
    expect(existsSync(file), `变量文件不存在: ${file}`).toBe(true);
    const css = readFileSync(file, "utf8");
    expect(css).toMatch(/--material-blur\s*:\s*\d+px/);
  });
});

describe("表面材质：背景并入材质令牌 --material-bg / --material-bg-p3", () => {
  beforeEach(() => {
    const root = document.documentElement;
    root.style.removeProperty("--material-bg");
    root.style.removeProperty("--material-bg-p3");
    root.style.removeProperty("--ion-background-color");
    root.style.removeProperty("--ion-background-color-rgb");
    document.body.style.backgroundColor = "";
    document.body.style.backgroundImage = "";
    localStorage.clear();
  });

  it("setBgColor 写 canonical --material-bg（纯色不写 -p3 孪生），绘制面读 --material-bg-active", () => {
    const { setBgColor } = useTheme();
    setBgColor("#1a1a1a");
    expect(document.documentElement.style.getPropertyValue("--material-bg")).toBe("#1a1a1a");
    expect(document.documentElement.style.getPropertyValue("--material-bg-p3")).toBe("");
    // 关键：body 与 --ion-background-color 都读中转变量 --material-bg-active，
    // 这样 vivid.css 的 @media (color-gamut: p3) 才能覆盖它、让 P3 宽色域背景真正生效。
    expect(document.body.style.backgroundColor).toBe("var(--material-bg-active)");
    expect(document.documentElement.style.getPropertyValue("--ion-background-color")).toBe("var(--material-bg-active)");
  });

  it("setBgGradient 写 --material-bg（srgb）与 --material-bg-p3（display-p3 孪生，关掉 §6.17 缺口）", () => {
    const { setBgGradient } = useTheme();
    setBgGradient(["#134e5e", "#71b280"]);
    const bg = document.documentElement.style.getPropertyValue("--material-bg");
    const bgP3 = document.documentElement.style.getPropertyValue("--material-bg-p3");
    expect(bg).toContain("linear-gradient");
    expect(bg).toContain("#134e5e");
    expect(bgP3).toContain("linear-gradient");
    expect(bgP3).toContain("display-p3");
    expect(bgP3).not.toContain("#134e5e"); // srgb 停色已被转成 display-p3
    expect(document.body.style.backgroundImage).toBe("var(--material-bg-active)");
  });

  it("setBgColor(null) 复位：移除 --material-bg / -p3，回落桥接默认（不残留坏值）", () => {
    const { setBgColor } = useTheme();
    setBgColor("#1a1a1a");
    setBgColor(null);
    expect(document.documentElement.style.getPropertyValue("--material-bg")).toBe("");
    expect(document.documentElement.style.getPropertyValue("--material-bg-p3")).toBe("");
    expect(document.documentElement.style.getPropertyValue("--ion-background-color")).toBe("");
    expect(document.body.style.backgroundColor).toBe("");
  });
});

describe("表面材质：全仓无裸 backdrop-filter 字面量（回归锁）", () => {
  it("所有磨砂面都读 var(--material-blur)，无硬编码 blur(<px>)", () => {
    // vitest 在 encv-mobile 下运行；共享组件包在 ../packages 下。
    const root = process.cwd();
    const encvSrc = resolve(root, "src");
    const sharedSrc = resolve(root, "../packages/shared-components/src");
    // 健全检查：路径必须真实存在，否则扫描会「空过」造成假绿。
    expect(existsSync(encvSrc), `扫码目录不存在: ${encvSrc}`).toBe(true);
    expect(existsSync(sharedSrc), `扫码目录不存在: ${sharedSrc}`).toBe(true);
    const hits: BareHit[] = [];
    walk(encvSrc, hits);
    walk(sharedSrc, hits);
    expect(
      hits,
      "发现未接入 --material-blur 的磨砂面（应改为 blur(var(--material-blur, <px>))）：\n" +
        hits.map(h => `${h.file}:${h.line}  ${h.text}`).join("\n")
    ).toEqual([]);
  });
});
