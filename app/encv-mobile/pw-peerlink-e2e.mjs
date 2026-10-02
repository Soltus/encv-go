// P2b 端到端验证脚本（真实浏览器 + 真实后端）
//
// 流程：
//   1. 打开 /tabs/settings/peers（Capacitor web 生产产物，经类网关源站托管）
//   2. 点「生成配对码」→ 真实后端下发票据 → 页面渲染二维码
//   3. 用 Node 模拟"安卓端扫码"：HMAC proof → POST /api/peerlink/pair
//   4. 页面轮询到已配对 → 显示 SAS 6 位 → 与本地独立计算的 SAS 比对（防 Hub 篡改）
//   5. 点「一致，信任该设备」→ 断言进入已信任态
import crypto from "node:crypto";
import { chromium } from "@playwright/test";

const ORIGIN = process.env.ORIGIN || "http://127.0.0.1:8124";
const PAGE = process.env.PAGE_URL || `${ORIGIN}/tabs/settings/peers`;
const SHOT = process.env.OUT || "test-visual/peerlink-e2e.png";

function proofHex(pskHex, pairingId, deviceId) {
  return crypto.createHmac("sha256", Buffer.from(pskHex, "hex")).update(`${pairingId}|${deviceId}`).digest("hex");
}

function sasFromPsk(pskHex) {
  const h = crypto.createHmac("sha256", Buffer.from(pskHex, "hex")).update("encv-peerlink-sas").digest();
  const n = (h[0] << 24) | (h[1] << 16) | (h[2] << 8) | h[3];
  return String(((n >>> 0) % 1000000)).padStart(6, "0");
}

const browser = await chromium.launch({ executablePath: "/usr/bin/chromium", args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

let ticket = null;
page.on("response", async res => {
  if (res.url().includes("/api/peerlink/ticket") && res.request().method() === "POST") {
    try {
      ticket = await res.json();
    } catch {}
  }
});

await page.goto(PAGE, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(2500);

// ① 生成配对码
await page.getByText("生成配对码").first().click();
await page.waitForTimeout(1500);

if (!ticket) throw new Error("未捕获到 ticket 响应");
console.log("ticket:", { pairingId: ticket.pairingId, hub: ticket.hub, expiresIn: ticket.expiresIn });

// ② 二维码是否真的渲染出来（canvas 有**不透明**的深浅像素，且深浅都有 = 真的画了模块）
await page.screenshot({ path: SHOT.replace(".png", "-waiting.png") });
const qr = await page.evaluate(() => {
  const c = document.querySelector("canvas.qrCanvas");
  if (!c) return { canvas: false };
  const ctx = c.getContext("2d");
  const d = ctx.getImageData(0, 0, c.width, c.height).data;
  let dark = 0, light = 0;
  const total = c.width * c.height;
  for (let i = 0; i < d.length; i += 4) {
    if (d[i + 3] === 0) continue; // 完全透明 = 没画
    if (d[i] < 128) dark++; else light++;
  }
  return { canvas: true, w: c.width, h: c.height, dark, light, total };
});
console.log("qr:", qr);
if (!qr.canvas || qr.dark === 0 || qr.light === 0) throw new Error(`二维码未真正渲染: ${JSON.stringify(qr)}`);

// ③ 模拟安卓端扫码配对
const deviceId = "e2e-android-0001";
const proof = proofHex(ticket.psk, ticket.pairingId, deviceId);
const pairRes = await fetch(`${ORIGIN}/api/peerlink/pair`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ pairingId: ticket.pairingId, deviceId, name: "Pixel 9", platform: "android", proof }),
});
const pairJson = await pairRes.json();
console.log("pair:", { status: pairRes.status, peerId: pairJson.peerId });
if (pairRes.status !== 200) throw new Error(`配对失败: ${pairRes.status} ${JSON.stringify(pairJson)}`);

// ④ 页面应显示 SAS，且与本地独立计算一致
await page.waitForSelector(".sasCode", { timeout: 15000 });
const shown = (await page.textContent(".sasCode")).trim();
const expected = sasFromPsk(ticket.psk);
console.log("sas:", { shown, expected, match: shown === expected });
if (shown !== expected) throw new Error(`SAS 不一致: 页面=${shown} 本地=${expected}`);

// ⑤ 点击信任
await page.getByText("一致，信任该设备").first().click();
await page.waitForTimeout(600);
const confirmed = await page.textContent(".confirmedText").catch(() => null);
console.log("confirmed:", confirmed);
if (!confirmed) throw new Error("未进入已信任态");

await page.screenshot({ path: SHOT });
console.log("E2E_OK");
await browser.close();
