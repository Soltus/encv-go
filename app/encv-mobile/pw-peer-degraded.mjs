// P5 / Task 5.5 降级矩阵**持久性** UI 验证（真实浏览器）
//
// 断言：
//  1. 对端离线 + 本端未连互联服务 ⇒ 页面顶部出现**常驻条幅**（不是 Toast）
//  2. 条幅内容可对上降级矩阵（peer_offline / hub_not_paired），且带对端名
//  3. **持久化**：等待 12s（跨过一轮 10s 轮询）后条幅仍在（Toast 早就消失了）
//  4. 后端返回"全部正常"后条幅自动消失
import { chromium } from "@playwright/test";

const ORIGIN = process.env.ORIGIN || "http://127.0.0.1:8125";
const PAGE = `${ORIGIN}/tabs/files`;
const SHOT = process.env.OUT || "test-visual/peer-degraded.png";

const browser = await chromium.launch({ executablePath: "/usr/bin/chromium", args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

let state = {
  peers: [{ id: "peer-a", name: "Pixel", platform: "android", online: false }],
  edge: { running: false, connected: false },
};

await page.route("**/api/peerlink/peers", route =>
  route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: state.peers }) }),
);
await page.route("**/api/peerlink/edge/status", route =>
  route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(state.edge) }),
);

await page.goto(PAGE, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(1200);
await page.waitForSelector("[data-testid=peer-degraded]", { timeout: 15000 });

const read = () =>
  page.evaluate(() => {
    const bar = document.querySelector("[data-testid=peer-degraded]");
    if (!bar) return null;
    return Array.from(bar.querySelectorAll(".pdRow")).map(r => ({
      kind: r.getAttribute("data-testid"),
      text: r.querySelector(".pdText")?.textContent?.trim() || "",
    }));
  });

const first = await read();
console.log("notices:", JSON.stringify(first, null, 2));

if (!first || first.length === 0) throw new Error("降级条幅未出现");
if (!first.some(n => n.kind === "pd-peer_offline")) throw new Error("缺少对端离线降级项");
if (!first.some(n => n.kind === "pd-hub_not_paired")) throw new Error("缺少本端未连互联服务降级项");
if (!first.some(n => n.text.includes("Pixel"))) throw new Error("降级项未带对端名");

await page.screenshot({ path: SHOT, fullPage: false });

// 持久化：跨过一轮 10s 轮询后仍在（Toast 早就没了）
await page.waitForTimeout(12000);
const still = await read();
if (!still || still.length === 0) throw new Error("条幅不是持久性的（12s 后消失）");
console.log("still visible after 12s:", JSON.stringify(still.map(n => n.kind)));

// 状态恢复 → 自动消失
state = { peers: [{ id: "peer-a", name: "Pixel", platform: "android", online: true }], edge: { running: true, connected: true } };
await page.waitForTimeout(11000);
const after = await read();
if (after) throw new Error(`状态恢复后条幅未消失: ${JSON.stringify(after)}`);

await browser.close();
console.log("PEER_DEGRADED_OK");
