// P1 Task 1.2.2 桌面布局第二阶段：真实浏览器复现/验证脚本（Chromium 走 /usr/bin/chromium）
// 用法：
//   URL=http://127.0.0.1:8100/tabs/files OUT=test-visual/desktop-layout-before.png node pw-desktop-layout.mjs
//   W=1920 H=1080 OUT=test-visual/desktop-layout-before-1920.png node pw-desktop-layout.mjs
// 输出：截图 + DOM 探针（rail 宽 / 内容区宽 / ion-content 直接子元素中最宽者的宽度）
import { chromium } from "@playwright/test";

const url = process.env.URL || "http://127.0.0.1:8100/tabs/files";
const out = process.env.OUT || "test-visual/desktop-layout-before.png";
const width = Number(process.env.W || 1440);
const height = Number(process.env.H || 900);

const browser = await chromium.launch({
  executablePath: "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage"],
});
const page = await browser.newPage({ viewport: { width, height } });
// dev 态默认打 :16666（沙箱网关），这里显式指向真实后端，否则页面停在"服务器离线"兜底页
const apiBase = process.env.API_BASE || "http://127.0.0.1:2025";
await page.addInitScript(base => {
  try {
    localStorage.setItem("encv-server-url", base);
  } catch {
    /* 忽略：隐私模式下 localStorage 不可用 */
  }
}, apiBase);
const errors = [];
page.on("console", m => {
  if (m.type() === "error") errors.push(m.text().slice(0, 200));
});

await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60000 });
await page.waitForTimeout(5000);
await page.screenshot({ path: out, fullPage: false });

const info = await page.evaluate(() => {
  const rect = el => {
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const rail = document.querySelector(".desktop-rail");
  const shell = document.querySelector(".desktop-content");
  // ⚠️ App.vue 根上还有一个 ion-router-outlet（1440 宽），必须限定在桌面壳内量
  const outlet = shell ? shell.querySelector("ion-router-outlet") : null;
  const ionContent = shell ? shell.querySelector("ion-content") : null;
  // ion-content 的内容是 light DOM（slot 内容仍在文档树里），直接子元素可量
  const kids = ionContent ? [...ionContent.children] : [];
  const measured = kids
    .map(el => ({
      tag: el.tagName.toLowerCase(),
      cls: (el.className || "").toString().split(" ")[0] || "",
      ...rect(el),
    }))
    .filter(m => m.w && m.w > 0);
  const widest = measured.reduce((a, b) => (b.w > (a?.w ?? 0) ? b : a), null);
  return {
    formFactor: document.documentElement.dataset.formFactor ?? null,
    rail: rect(rail),
    shell: rect(shell),
    outlet: rect(outlet),
    ionContent: rect(ionContent),
    pageChildren: measured.slice(0, 8),
    widestChild: widest,
    innerWidth: window.innerWidth,
    visibleText: (document.body.innerText || "").slice(0, 160).replace(/\s+/g, " "),
  };
});

console.log(JSON.stringify({ url, viewport: { width, height }, shot: out, ...info, errors: errors.slice(0, 5) }, null, 2));
await browser.close();
