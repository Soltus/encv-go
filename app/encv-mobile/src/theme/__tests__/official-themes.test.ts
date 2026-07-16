import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import fs from "node:fs";
import path from "node:path";
import {
  activateTheme,
  fetchThemeManifest,
  unloadTheme,
  getThemePerf,
  resetThemeLoaderForTest,
  syncIonicRgb,
  __stubJsModule,
  type ThemeSource,
} from "@encv/shared-components/theme/themeLoader";
import { useUserThemes, USER_THEMES } from "@encv/shared-components/composables/useUserThemes";

// 运行时主题资源包根目录（app/encv-mobile/public/themes）
const base = path.resolve(__dirname, "..", "..", "..", "public", "themes");
const ALL = ["encv", "encv-dark", "rose", "ocean", "forest", "midnight", "sunset", "mint", "kitty"] as const;
const OFFICIAL = ["encv", "encv-dark", "rose", "ocean", "forest", "midnight"] as const;
// 不应出现在编译期 palette.css 的官方主题（encv 默认基态除外，它作为 :root 回落有意保留）
const COMPILE_TIME_FORBIDDEN = ["encv-dark", "rose", "ocean", "forest", "midnight"] as const;

function read(id: string, file: string): string {
  const p = path.join(base, id, file);
  if (!fs.existsSync(p)) throw new Error(`主题资源包缺失：${p}`);
  return fs.readFileSync(p, "utf-8");
}

describe("主题资源包（续37：官方 == 第三方，运行时可加载资产包）", () => {
  it("每个主题都是 themes/<id>/{theme.json,theme.css} 文件夹资产", () => {
    for (const id of ALL) {
      const json = JSON.parse(read(id, "theme.json"));
      const css = read(id, "theme.css");
      expect(json.id, `${id}.theme.json id 匹配`).toBe(id);
      expect(css, `${id} 含 [data-theme="${id}"] 令牌数据`).toMatch(new RegExp(`\\[data-theme=["']?${id}["']?\\]\\s*\\{`));
    }
  });

  it("官方主题 builtIn=true、第三方 builtIn=false（唯一区别=预装）", () => {
    for (const id of OFFICIAL) expect(JSON.parse(read(id, "theme.json")).builtIn).toBe(true);
    for (const id of ["sunset", "mint"] as const) expect(JSON.parse(read(id, "theme.json")).builtIn).toBe(false);
  });

  it("官方主题定义独立设计语言（形状/密度/立体感互不相同），不只是换色", () => {
    const cssMap = Object.fromEntries(OFFICIAL.map(t => [t, read(t, "theme.css")]));
    for (const id of OFFICIAL) {
      expect(cssMap[id], `${id} 应定义 --radius-box`).toMatch(/--radius-box\s*:/);
      expect(cssMap[id], `${id} 应定义 --density 或 --elevation-*（证明非换色）`).toMatch(/--density\s*:|--elevation-/);
    }
    const radii = OFFICIAL.map(t => cssMap[t]!.match(/--radius-box\s*:\s*([^;]+);/)?.[1]?.trim());
    expect(new Set(radii).size, "圆角应存在多种不同值（非统一形状只换色）").toBeGreaterThan(1);
    const densities = OFFICIAL.map(t => cssMap[t]!.match(/--density\s*:\s*([^;]+);/)?.[1]?.trim());
    expect(new Set(densities).size, "密度应存在多种不同值").toBeGreaterThan(1);
  });

  it("主题不含编译期特权令牌（与第三方零差异，防止特权回潮）", () => {
    for (const id of ALL) {
      const css = read(id, "theme.css");
      expect(css).not.toMatch(/--color-primary-hover\s*:/);
      expect(css).not.toMatch(/--tint-primary-bg\s*:/);
    }
  });

  it("资源包能力：rose 用 url() 自带装饰资源（Hello Kitty 式贴图/背景即此机制）", () => {
    const css = read("rose", "theme.css");
    expect(css).toMatch(/url\(/);
  });

  it("kitty 全能力示例：行使 assets/ 图片 + @font-face 字体 + 真实选择器装饰（真主题证明）", () => {
    const json = JSON.parse(read("kitty", "theme.json"));
    expect(json.builtIn, "kitty 为第三方（builtIn=false）").toBe(false);
    // 清单声明三类资产
    expect(json.js, "声明 theme.js 钩子").toBe("theme.js");
    expect(Array.isArray(json.assets) && json.assets.length > 0, "声明 assets/ 资源").toBe(true);

    const css = read("kitty", "theme.css");
    expect(css, "携带 @font-face 自有字体族").toMatch(/@font-face\s*\{/);
    expect(css, "用 url(./assets/...) 引用自带图片资源").toMatch(/url\(["']?\.\/assets\//);
    expect(css, "用真实选择器 ::before 贴装饰（非仅换色）").toMatch(/\.ui-panel::before/);

    // assets/ 图片文件真实存在（随资源包分发，非编译进产物）
    expect(fs.existsSync(path.join(base, "kitty", "assets", "bg.svg"))).toBe(true);
    expect(fs.existsSync(path.join(base, "kitty", "assets", "sticker.svg"))).toBe(true);

    // theme.js 运行时装饰钩子存在且导出 mount/unmount
    const js = read("kitty", "theme.js");
    expect(js, "导出 mount()").toMatch(/export\s+function\s+mount/);
    expect(js, "导出 unmount()").toMatch(/export\s+function\s+unmount/);
  });

  it("palette.css 仅留 encv 默认基态，不再编译期烤入其它官方主题（零编译特权）", () => {
    const palette = fs.readFileSync(
      path.resolve(__dirname, "..", "..", "..", "..", "packages", "shared-components", "src", "theme", "palette.css"),
      "utf-8"
    );
    // 默认基态 [data-theme="encv"] 有意保留（:root 回落，零加载成本）
    expect(palette).toMatch(/\[data-theme=["']?encv["']?\]/);
    // 其余官方主题必须迁出（不再编译进产物）
    for (const id of COMPILE_TIME_FORBIDDEN) {
      expect(palette, `${id} 不应出现在编译期 palette.css`).not.toMatch(new RegExp(`\\[data-theme=["']?${id}["']?\\]`));
    }
  });
});

describe("themeLoader（续37：运行时注入 + 性能指标 + 优化）", () => {
  beforeEach(() => resetThemeLoaderForTest());

  function fireLoad(id: string) {
    const link = document.head.querySelector(`link[data-theme-id="${id}"]`) as HTMLLinkElement | null;
    expect(link, `${id} 应已注入 <link>`).toBeTruthy();
    link!.dispatchEvent(new Event("load"));
  }

  it("activateTheme 注入 <link> 并切到 [data-theme]，记录切换耗时", async () => {
    const src: ThemeSource = { id: "rose", cssUrl: "/themes/rose/theme.css", builtIn: true };
    const p = activateTheme(src);
    fireLoad("rose");
    await p;
    expect(document.documentElement.getAttribute("data-theme")).toBe("rose");
    expect(getThemePerf().active).toBe("rose");
    expect(getThemePerf().lastSwitchMs).not.toBeNull();
  });

  it("默认主题回落 :root（移除 data-theme 属性，零加载成本）", async () => {
    const src: ThemeSource = { id: "encv", cssUrl: "/themes/encv/theme.css", builtIn: true };
    const p = activateTheme(src);
    fireLoad("encv");
    await p;
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });

  it("去重缓存：重复激活命中缓存，cacheHits 递增且只注入一个 <link>", async () => {
    const src: ThemeSource = { id: "ocean", cssUrl: "/themes/ocean/theme.css", builtIn: true };
    const p1 = activateTheme(src);
    fireLoad("ocean");
    await p1;
    const before = getThemePerf().cacheHits;
    const p2 = activateTheme(src); // 缓存命中，无新 link、无 onload 依赖
    await p2;
    expect(getThemePerf().cacheHits).toBe(before + 1);
    expect(document.head.querySelectorAll('link[data-theme-id="ocean"]').length).toBe(1);
  });

  it("卸载：第三方主题可卸载（移除 <link>），官方 builtIn 不可卸载", async () => {
    const third: ThemeSource = { id: "mint", cssUrl: "/themes/mint/theme.css", builtIn: false };
    const p1 = activateTheme(third);
    fireLoad("mint");
    await p1;
    expect(document.head.querySelector('link[data-theme-id="mint"]')).toBeTruthy();
    unloadTheme("mint");
    expect(document.head.querySelector('link[data-theme-id="mint"]')).toBeNull();

    const official: ThemeSource = { id: "forest", cssUrl: "/themes/forest/theme.css", builtIn: true };
    const p2 = activateTheme(official);
    fireLoad("forest");
    await p2;
    unloadTheme("forest"); // 官方忽略
    expect(document.head.querySelector('link[data-theme-id="forest"]')).toBeTruthy();
  });

  it("预加载官方主题后，切回已加载主题零成本（cacheHits 反映）", async () => {
    const builtIns: ThemeSource[] = OFFICIAL.map(id => ({
      id,
      cssUrl: `/themes/${id}/theme.css`,
      builtIn: true,
    }));
    // 模拟 main.ts 启动期预加载
    for (const s of builtIns) {
      const p = activateTheme(s);
      fireLoad(s.id);
      await p;
    }
    const before = getThemePerf().cacheHits;
    // 用户切回已预加载的官方主题
    const p = activateTheme(builtIns[2]!);
    await p;
    expect(getThemePerf().cacheHits).toBe(before + 1);
  });
});

describe("useUserThemes（续38：用户可安装 / 卸载 / 分发 + 集市）", () => {
  beforeEach(() => {
    resetThemeLoaderForTest();
    const u = useUserThemes();
    for (const t of [...u.installedList.value]) u.uninstallTheme(t.id);
    if (typeof localStorage !== "undefined") localStorage.clear();
  });

  it("集市（Bazaar）提供可安装条目，安装后进入 allThemes 并持久化", () => {
    const { allThemes, installFromBazaar, isInstalled, bazaar } = useUserThemes();
    expect(bazaar.length, "应有示例集市条目").toBeGreaterThan(0);
    const entry = bazaar[0]!;
    expect(isInstalled(entry.id), "安装前不在用户空间").toBe(false);
    installFromBazaar(entry);
    expect(isInstalled(entry.id), "安装后应在用户空间").toBe(true);
    expect(
      allThemes.value.some(t => t.id === entry.id),
      "应出现在主题网格"
    ).toBe(true);
    const stored = JSON.parse(localStorage.getItem("encv-installed-themes") || "[]");
    expect(
      stored.some((t: { id: string }) => t.id === entry.id),
      "应持久化到 localStorage"
    ).toBe(true);
  });

  it("卸载第三方主题：移除注册 + 删持久化 + loader 卸载 <link>", () => {
    const { installFromBazaar, uninstallTheme, isInstalled, bazaar } = useUserThemes();
    const entry = bazaar[0]!;
    installFromBazaar(entry);
    expect(isInstalled(entry.id)).toBe(true);
    // 安装即预热 <link>
    expect(document.head.querySelector(`link[data-theme-id="${entry.id}"]`), "预热应注入 link").toBeTruthy();
    uninstallTheme(entry.id);
    expect(isInstalled(entry.id), "卸载后不在用户空间").toBe(false);
    expect(document.head.querySelector(`link[data-theme-id="${entry.id}"]`), "卸载应移除 link").toBeNull();
    const stored = JSON.parse(localStorage.getItem("encv-installed-themes") || "[]");
    expect(
      stored.some((t: { id: string }) => t.id === entry.id),
      "卸载应删持久化"
    ).toBe(false);
  });

  it("从 URL 安装主题（可分发）：远程 theme.css 注册为第三方并预热", () => {
    const { installTheme, allThemes } = useUserThemes();
    installTheme({ id: "url-demo", label: "Demo", url: "https://x.test/demo.css", builtIn: false });
    expect(
      allThemes.value.some(t => t.id === "url-demo"),
      "URL 主题应进入网格"
    ).toBe(true);
    const link = document.head.querySelector('link[data-theme-id="url-demo"]') as HTMLLinkElement | null;
    expect(link, "远程 link 应注入").toBeTruthy();
    expect(link!.getAttribute("href")).toBe("https://x.test/demo.css");
  });

  it("官方主题 builtIn 为真，uninstall 静默忽略（不可卸载）", () => {
    const { isBuiltIn, uninstallTheme } = useUserThemes();
    expect(isBuiltIn("rose")).toBe(true);
    expect(() => uninstallTheme("rose")).not.toThrow();
    expect(isBuiltIn("rose"), "官方仍不可卸载").toBe(true);
  });
});

describe("themeLoader.syncIonicRgb（续40：Ionic 半透明层跟随任意主题，不止实体色）", () => {
  const root = typeof document !== "undefined" ? document.documentElement : null;

  afterEach(() => {
    // 清理内联令牌，避免跨用例污染
    if (!root) return;
    for (const p of [
      "--color-primary",
      "--color-primary-content",
      "--color-base-100",
      "--color-base-content",
      "--ion-color-primary-rgb",
      "--ion-color-primary-contrast-rgb",
      "--ion-background-color-rgb",
      "--ion-text-color-rgb",
    ]) {
      root.style.removeProperty(p);
    }
  });

  it("把当前 --color-primary 派生为 --ion-color-primary-rgb（主题无关，任意主题生效）", () => {
    if (!root) return;
    // jsdom 不解析 <link> 主题 CSS，故直接以内联令牌模拟「rose 主色 #e11d48」已随 [data-theme] 落位
    root.style.setProperty("--color-primary", "#e11d48");
    root.style.setProperty("--color-primary-content", "#ffffff");
    root.style.setProperty("--color-base-100", "#fff1f3");
    root.style.setProperty("--color-base-content", "#4c0519");
    syncIonicRgb();
    expect(root.style.getPropertyValue("--ion-color-primary-rgb").trim()).toBe("225, 29, 72");
    expect(root.style.getPropertyValue("--ion-color-primary-contrast-rgb").trim()).toBe("255, 255, 255");
    expect(root.style.getPropertyValue("--ion-background-color-rgb").trim()).toBe("255, 241, 243");
    expect(root.style.getPropertyValue("--ion-text-color-rgb").trim()).toBe("76, 5, 25");
  });

  it("派生覆盖 bridge.css 硬编码的 encv 紫：切到远程主题也不会残留紫色半透明层", () => {
    if (!root) return;
    root.style.setProperty("--color-primary", "#39ff14"); // neon 绿
    root.style.setProperty("--color-primary-content", "#04140a");
    root.style.setProperty("--color-base-100", "#0a0e14");
    root.style.setProperty("--color-base-content", "#d7f5e6");
    // 模拟 bridge.css 对 encv 的硬编码兜底值仍存在
    root.style.setProperty("--ion-color-primary-rgb", "139, 92, 246");
    syncIonicRgb();
    expect(root.style.getPropertyValue("--ion-color-primary-rgb").trim()).toBe("57, 255, 20");
  });
});

describe("themeLoader 切换生命周期（续41：切换主题卸载上一主题 JS 装饰，不残留/不丢挂载）", () => {
  const body = typeof document !== "undefined" ? document.body : null;
  const DECOR = "kitty-theme-decor";

  const stubKitty = (onMount?: () => void) => {
    __stubJsModule("kitty", {
      mount() {
        onMount?.();
        if (body && !body.querySelector(`#${DECOR}`)) {
          const d = body.appendChild(document.createElement("div"));
          d.id = DECOR;
        }
      },
      unmount() {
        body?.querySelector(`#${DECOR}`)?.remove();
      },
    });
  };
  const src = (id: string, jsUrl?: string): ThemeSource => ({
    id,
    cssUrl: `/themes/${id}/theme.css`,
    jsUrl,
    builtIn: false,
  });
  const hasDecor = () => (body ? body.querySelector(`#${DECOR}`) : null);

  beforeEach(() => {
    resetThemeLoaderForTest();
    body?.querySelectorAll(`#${DECOR}`).forEach(el => el.remove());
    stubKitty();
  });
  afterEach(() => {
    resetThemeLoaderForTest();
    body?.querySelectorAll(`#${DECOR}`).forEach(el => el.remove());
  });

  it("切到 kitty 挂载装饰；切到无 JS 的 rose 后装饰被卸载（不残留）", async () => {
    if (!body) return;
    await activateTheme(src("kitty", "/themes/kitty/theme.js"));
    expect(hasDecor(), "切到 kitty 后装饰应存在").not.toBeNull();
    await activateTheme(src("rose"));
    expect(hasDecor(), "切走后 kitty 装饰应被卸载").toBeNull();
  });

  it("离开再切回 kitty 会重新挂载（jsCache 已清，不丢挂载能力）", async () => {
    if (!body) return;
    await activateTheme(src("kitty", "/themes/kitty/theme.js"));
    await activateTheme(src("rose"));
    expect(hasDecor()).toBeNull();
    await activateTheme(src("kitty", "/themes/kitty/theme.js"));
    expect(hasDecor(), "切回 kitty 应重新挂载装饰").not.toBeNull();
  });

  it("重复激活同一主题只挂载一次（幂等，不重复挂 / 不误卸载）", async () => {
    if (!body) return;
    let mounts = 0;
    stubKitty(() => {
      mounts++;
    });
    await activateTheme(src("kitty", "/themes/kitty/theme.js"));
    await activateTheme(src("kitty", "/themes/kitty/theme.js"));
    expect(mounts, "同一主题重复激活只挂载一次").toBe(1);
    expect(hasDecor(), "装饰应仍在").not.toBeNull();
  });
});

describe("theme.json 活清单（续42：清单驱动加载/分发，非死数据）", () => {
  it("本地主题清单的 builtIn/js 与 TS 注册表一致（防两处元信息静默漂移）", () => {
    for (const meta of USER_THEMES) {
      const json = JSON.parse(read(meta.id, "theme.json"));
      expect(Boolean(json.builtIn), `${meta.id} builtIn 清单↔注册表一致`).toBe(Boolean(meta.builtIn));
      // 注册表 js:true ⇔ 清单声明 theme.js（kitty 等携带钩子的主题）
      const registryHasJs = Boolean(meta.js);
      const manifestHasJs = typeof json.js === "string" && json.js.length > 0;
      expect(manifestHasJs, `${meta.id} js 钩子声明清单↔注册表一致`).toBe(registryHasJs);
    }
  });
});

describe("fetchThemeManifest（续42：抓取并解析 theme.json，css/js 解析为绝对 URL）", () => {
  afterEach(() => vi.restoreAllMocks());

  function mockFetch(jsonUrl: string, body: unknown, ok = true, status = 200) {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        expect(url, "应请求清单地址").toBe(jsonUrl);
        return {
          ok,
          status,
          json: async () => body,
        } as unknown as Response;
      })
    );
  }

  it("从文件夹 URL 读取清单，相对 css/js 按目录解析为绝对 URL", async () => {
    mockFetch("https://x.test/themes/cool/theme.json", {
      id: "cool",
      name: "Cool",
      css: "theme.css",
      js: "theme.js",
      assets: ["assets/a.svg"],
    });
    const m = await fetchThemeManifest("https://x.test/themes/cool");
    expect(m.id).toBe("cool");
    expect(m.name).toBe("Cool");
    expect(m.css).toBe("https://x.test/themes/cool/theme.css");
    expect(m.js).toBe("https://x.test/themes/cool/theme.js");
    expect(m.assets).toEqual(["assets/a.svg"]);
  });

  it("直接传 theme.json URL 也可（css 缺省回退 theme.css）", async () => {
    mockFetch("https://x.test/t/theme.json", { id: "t" });
    const m = await fetchThemeManifest("https://x.test/t/theme.json");
    expect(m.css).toBe("https://x.test/t/theme.css");
    expect(m.js).toBeUndefined();
  });

  it("清单声明绝对 css URL 时原样保留（跨域资源）", async () => {
    mockFetch("https://x.test/t/theme.json", { id: "t", css: "https://cdn.test/x.css" });
    const m = await fetchThemeManifest("https://x.test/t");
    expect(m.css).toBe("https://cdn.test/x.css");
  });

  it("清单缺 id → 抛错；HTTP 失败 → 抛错", async () => {
    mockFetch("https://x.test/bad/theme.json", { name: "no id" });
    await expect(fetchThemeManifest("https://x.test/bad")).rejects.toThrow(/id/);
    mockFetch("https://x.test/404/theme.json", {}, false, 404);
    await expect(fetchThemeManifest("https://x.test/404")).rejects.toThrow(/404/);
  });
});

describe("installThemeFromUrl（续42：按清单分发安装，自动发现元信息）", () => {
  beforeEach(() => {
    resetThemeLoaderForTest();
    const u = useUserThemes();
    for (const t of [...u.installedList.value]) u.uninstallTheme(t.id);
    if (typeof localStorage !== "undefined") localStorage.clear();
  });
  afterEach(() => vi.restoreAllMocks());

  it("读取远程 theme.json，据清单派生 id/名字，拉取后在同源本地 /themes/<id>/ 注入 <link>", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ id: "remote-cool", name: "Remote Cool", css: "theme.css" }),
      })) as unknown as typeof fetch
    );
    const { installThemeFromUrl, allThemes, isInstalled } = useUserThemes();
    await installThemeFromUrl("https://x.test/themes/remote-cool");
    expect(isInstalled("remote-cool"), "清单 id 安装进用户空间").toBe(true);
    const meta = allThemes.value.find(t => t.id === "remote-cool");
    expect(meta?.label, "标签取清单 name").toBe("Remote Cool");
    const link = document.head.querySelector('link[data-theme-id="remote-cool"]') as HTMLLinkElement | null;
    // 本地优先：注入同源 /themes/<id>/，而非热链远程 CDN。
    expect(link?.getAttribute("href"), "本地优先：同源 /themes/<id>/ 注入").toBe("/themes/remote-cool/theme.css");
  });
});
