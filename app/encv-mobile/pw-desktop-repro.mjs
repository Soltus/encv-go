// P1 桌面端形态：真实浏览器复现/验证脚本（Chromium 走 /usr/bin/chromium）
// 用法：
//   node pw-desktop-repro.mjs                       # 默认 1440x900 截 test-visual/desktop-before.png
//   W=390 H=844 OUT=test-visual/mobile.png node pw-desktop-repro.mjs
// 输出：截图 + DOM 探针（formFactor / tab-bar 位置与尺寸 / 内容宽度）
import { chromium } from "@playwright/test";

const url = process.env.URL || "http://localhost:16666/";
const out = process.env.OUT || "test-visual/desktop-before.png";
const width = Number(process.env.W || 1440);
const height = Number(process.env.H || 900);

const browser = await chromium.launch({
  executablePath: "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage"],
});
const page = await browser.newPage({ viewport: { width, height } });
const errors = [];
page.on("console", m => {
  if (m.type() === "error") errors.push(m.text().slice(0, 200));
});

await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60000 });
await page.waitForTimeout(4000);
await page.screenshot({ path: out, fullPage: false });

const info = await page.evaluate(() => {
  // Capacitor 运行时真相（web 平台下 Capacitor 是否可用、插件是否可用）
  const C = window.Capacitor;
  const capacitor = C
    ? {
        platform: typeof C.getPlatform === "function" ? C.getPlatform() : null,
        isNativePlatform: typeof C.isNativePlatform === "function" ? C.isNativePlatform() : null,
        pluginGoProcess: typeof C.isPluginAvailable === "function" ? C.isPluginAvailable("GoProcess") : null,
        pluginApiProxy: typeof C.isPluginAvailable === "function" ? C.isPluginAvailable("ApiProxy") : null,
      }
    : null;
  const bar = document.querySelector("ion-tab-bar");
  const r = bar ? bar.getBoundingClientRect() : null;
  const outlet = document.querySelector("ion-router-outlet");
  const o = outlet ? outlet.getBoundingClientRect() : null;
  return {
    capacitor,
    formFactor: document.documentElement.dataset.formFactor ?? null,
    hasTabBar: !!bar,
    tabBar: r ? { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) } : null,
    rail: (() => {
      const el = document.querySelector(".desktop-rail");
      if (!el) return null;
      const rr = el.getBoundingClientRect();
      const active = el.querySelector(".rail-item.router-link-active .rail-label");
      return {
        x: Math.round(rr.x),
        y: Math.round(rr.y),
        w: Math.round(rr.width),
        h: Math.round(rr.height),
        items: el.querySelectorAll(".rail-item").length,
        activeLabel: active ? active.textContent : null,
      };
    })(),
    outlet: o ? { x: Math.round(o.x), y: Math.round(o.y), w: Math.round(o.width), h: Math.round(o.height) } : null,
    innerWidth: window.innerWidth,
    visibleText: (document.body.innerText || "").slice(0, 120).replace(/\s+/g, " "),
  };
});

console.log(JSON.stringify({ url, viewport: { width, height }, shot: out, ...info, errors: errors.slice(0, 5) }, null, 2));
await browser.close();
