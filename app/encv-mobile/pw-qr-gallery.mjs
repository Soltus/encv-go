// P2b：扫码界面「从相册选择图片」真实浏览器回归锁
//
// 契约：
//   - 扫码面板提供「从相册选择」入口（隐藏 file input，accept="image/*"）；
//   - 选一张含配对码二维码的图片 → 前端解码 → 走**同一条** connectWithText 连通路径；
//   - 解码失败 / 非配对码必须**可见**（不静默）。
//
// 夹具：向后端申请**真实票据**，用 qrcode 渲染成 PNG（不依赖手工截图）。
// 用法：URL=http://127.0.0.1:16666/tabs/settings/peers node pw-qr-gallery.mjs
import { mkdirSync, writeFileSync } from "node:fs";
import { chromium } from "@playwright/test";
import QRCode from "qrcode";

const pageUrl = process.env.URL || "http://127.0.0.1:16666/tabs/settings/peers";
const base = pageUrl.replace(/\/tabs\/settings.*$/, "");
const results = [];
const check = (name, ok, extra = "") => {
  results.push({ name, ok });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${extra ? "  — " + extra : ""}`);
};

const read = sel => page.evaluate(s => (document.querySelector(s)?.textContent || "").trim(), sel);

let page;

// ── 夹具：真实票据 + 真实 PNG ──
async function makeFixture(text, name) {
  const p = `/tmp/qr-fixtures/${name}`;
  mkdirSync("/tmp/qr-fixtures", { recursive: true });
  writeFileSync(p, await QRCode.toBuffer(text, { width: 320, margin: 2 }));
  return p;
}

const tkRes = await fetch(`${base}/api/peerlink/ticket`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ hub: "http://127.0.0.1:2025/api/peerlink" }),
});
const tk = await tkRes.json();
if (!tk.pairingId) {
  console.log("无法申请真实票据:", JSON.stringify(tk).slice(0, 200));
  process.exit(1);
}
const realPairing = JSON.stringify({
  v: 2,
  hub: tk.hub,
  pairingId: tk.pairingId,
  psk: tk.psk,
  exp: 119,
});
const realPng = await makeFixture(realPairing, "pairing.png");
const junkPng = await makeFixture("this-is-not-a-pairing-code", "junk.png");
console.log("fixture ticket:", tk.pairingId.slice(0, 8) + "…");

const browser = await chromium.launch({
  executablePath: "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage"],
});
page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.goto(pageUrl, { waitUntil: "domcontentloaded", timeout: 60000 });
await page.waitForTimeout(4000);

// ① 入口存在
const fileInput = page.locator('[data-testid="qr-file-input"]');
check("扫码面板提供「从相册选择」入口", (await fileInput.count()) > 0);

// ② 选**真实票据**图 → 解码 → 连通
await fileInput.setInputFiles(realPng);
await page.waitForTimeout(3000);
const okText = await read('[data-testid="scan-ok"]');
const errText = await read('[data-testid="scan-error"]');
const edgeText = await read('[data-testid="edge-status"]');
check("选图后解码并连通（scan-ok 或已连上会合点）", !!okText || edgeText.includes("已连上会合点"),
  `ok=${okText.slice(0, 40)} edge=${edgeText.slice(0, 40)}`);
check("连通路径无错误提示", !errText, errText.slice(0, 80));

// ③ 非配对码图 → 必须可见失败（负向对照）
const page2 = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page = page2;
await page2.goto(pageUrl, { waitUntil: "domcontentloaded", timeout: 60000 });
await page2.waitForTimeout(3000);
await page2.locator('[data-testid="qr-file-input"]').setInputFiles(junkPng);
await page2.waitForTimeout(2500);
const negErr = await read('[data-testid="scan-error"]');
const negOk = await read('[data-testid="scan-ok"]');
check("非配对码图片给出可见失败（负向对照）", !!negErr && !negOk, negErr.slice(0, 80));

const failed = results.filter(r => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} PASS`);
await browser.close();
process.exit(failed ? 1 : 0);
