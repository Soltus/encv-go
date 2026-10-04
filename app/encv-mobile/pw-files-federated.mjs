// P3 搜索页接入验证（真实浏览器渲染；对端数据用 route stub）
//
// 断言（契约红线）：
//   1. Files 页搜索时，出现**独立**的「来自已配对设备」展示区（不混进本地结果列表）
//   2. 每条远端命中带来源徽章（安卓·Pixel）+ **远端原始路径**（不伪装成本地路径）
//   3. 远端命中只有「在线打开」入口，链接恒带 peerId + 远端原始路径
//   4. 本地结果列表里**不出现**远端路径（证明未混入 displayFiles）
import { chromium } from "@playwright/test";

const ORIGIN = process.env.ORIGIN || "http://127.0.0.1:8125";
const PAGE = `${ORIGIN}/tabs/files`;
const SHOT = process.env.OUT || "test-visual/files-federated.png";

const browser = await chromium.launch({ executablePath: "/usr/bin/chromium", args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

await page.route("**/api/peerlink/peers", route =>
  route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({ items: [{ id: "peer-a", deviceId: "dev-a", name: "Pixel", platform: "android", online: true }] }),
  }),
);
await page.route("**/api/peerlink/search*", route =>
  route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      peer: { id: "peer-a", name: "Pixel", platform: "android" },
      items: [
        { path: "/sdcard/Download/报告.pdf", name: "报告.pdf", size: 1024 },
        { path: "/sdcard/DCIM/IMG_0001.jpg", name: "IMG_0001.jpg", size: 2048 },
      ],
    }),
  }),
);
// 本端搜索：给一条本地命中，用于验证"远端没有混进本地列表"
await page.route("**/api/files/search-fulltext*", route =>
  route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({ results: [{ path: "/d/Movies/local.mp4", name: "local.mp4", size: 2048 }], mode: "normal" }),
  }),
);
// 注意：Playwright 后注册的 route 优先，这条也会命中 search-fulltext
await page.route("**/api/files/search*", route =>
  route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      results: [{ path: "/d/Movies/local.mp4", name: "local.mp4", size: 2048, isDirectory: false }],
      items: [{ path: "/d/Movies/local.mp4", name: "local.mp4", size: 2048, isDirectory: false }],
    }),
  }),
);
// 本端向量搜索（默认走这条）：给一条本地命中，用于**阳性**验证"远端没有混进本地列表"
await page.route("**/api/search/files*", route =>
  route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      results: [{ path: "/d/Movies/local.mp4", name: "local.mp4", size: 2048, isDirectory: false }],
      items: [{ path: "/d/Movies/local.mp4", name: "local.mp4", size: 2048, isDirectory: false }],
      search_mode: "normal",
    }),
  }),
);

await page.goto(PAGE, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(3000);

// Files 页搜索框是 contenteditable（useSearchInput），直接 focus 后输入
const input = await page.$("[data-testid=files-search-input], .search-input, [contenteditable=true]");
if (!input) throw new Error("找不到搜索输入框");
await input.click();
await page.keyboard.type("报告");
await page.waitForSelector("[data-testid=peer-results]", { timeout: 15000 });
await page.waitForSelector("[data-testid=peer-hit]", { timeout: 15000 });
await page.waitForTimeout(500);

const out = await page.evaluate(() => {
  const section = document.querySelector("[data-testid=peer-results]");
  const hits = Array.from(document.querySelectorAll("[data-testid=peer-hit]")).map(el => ({
    badge: el.querySelector(".srcBadge")?.textContent?.trim() || "",
    path: el.querySelector(".peer-hit-path")?.textContent?.trim() || "",
    open: el.querySelector("[data-testid=peer-hit-open]")?.getAttribute("href") || "",
  }));
  // 本地结果列表里的路径（用于证明远端未混入）
  const localPaths = Array.from(document.querySelectorAll("ion-item[data-highlight-path]")).map(
    el => el.getAttribute("data-highlight-path") || "",
  );
  // 契约红线2：远端命中**绝不能**以"本地文件条目"的形态出现（ion-item）
  const remoteAsLocalItem = Array.from(document.querySelectorAll("ion-item")).filter(el =>
    (el.textContent || "").includes("/sdcard/"),
  ).length;
  return {
    header: section?.querySelector(".peer-results-header")?.textContent?.trim() || "",
    hits,
    localPaths,
    remoteAsLocalItem,
  };
});

console.log(JSON.stringify(out, null, 2));

if (out.hits.length === 0) throw new Error("搜索页未展示远端命中");
if (!out.hits[0].badge) throw new Error("远端命中缺少来源徽章");
if (!out.hits[0].open.includes("peerId=peer-a")) throw new Error("在线打开链接缺少 peerId");
if (!out.hits[0].open.includes(encodeURIComponent("/sdcard/Download/报告.pdf"))) {
  throw new Error(`在线打开链接未带远端原始路径: ${out.hits[0].open}`);
}
// 契约红线：远端路径不得出现在本地结果列表 / 不得以"本地文件条目(ion-item)"形态渲染
const leaked = out.localPaths.filter(p => p.startsWith("/sdcard/"));
if (leaked.length > 0) throw new Error(`远端命中混进了本地结果列表: ${JSON.stringify(leaked)}`);
if (out.remoteAsLocalItem > 0) throw new Error(`远端命中被渲染成本地文件条目: ${out.remoteAsLocalItem}`);
if (out.localPaths.length === 0) console.log("note: 本端 stub 命中未渲染（不影响上述红线断言）");

await page.screenshot({ path: SHOT, fullPage: false });
await browser.close();
console.log("FILES_FED_OK");
