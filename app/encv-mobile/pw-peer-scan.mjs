// P2b Task 2.6 端到端验证脚本（真实浏览器 + 真实 Go 后端）
//
// 为什么这么搭：扫码端（安卓）的链路是
//   UI（粘贴/扫码）→ POST /api/peerlink/edge/pair → 本端 Go 进程作为 Edge 出网连 Hub
// 沙箱没有相机，所以**走"粘贴配对码"分支**（与扫码共用同一条 connectWithText 路径），
// 后端、票据、配对、WebSocket 长连接全部是真的，只有"取图像"这步被替换成粘贴。
//
// 前置（宿主机）：
//   1) go build -o /tmp/encvd-scan ./cmd/encv
//   2) ENCV_CONFIG_PATH=/tmp/peerlink-scan.json /tmp/encvd-scan start   （端口自选，从日志解析）
//   3) NODE_ENV=production vite build                                   （dist = Capacitor webDir）
//   4) python3 /tmp/gwlike-scan.py <dist> http://127.0.0.1:<port> 8126  （类网关源站）
//   5) ORIGIN=http://127.0.0.1:8126 HUB=http://127.0.0.1:<port> node pw-peer-scan.mjs
//
// 断言：
//   ① web（无相机）：提示"改用粘贴配对码"可见
//   ② 点「开始扫码」→ 错误**可见**（不静默失败）
//   ③ 真实票据 + 真实 /edge/pair → 成功文案
//   ④ /edge/status → running + connected（Go 侧长连接真的建起来了）
//   ⑤ Hub 侧 /peers 看到该 Edge 在线（不是只有本端自己说成功）
import { chromium } from "@playwright/test";

const ORIGIN = process.env.ORIGIN || "http://127.0.0.1:8126";
const HUB = process.env.HUB || "http://127.0.0.1:1999"; // 后端进程自己的地址（Edge 出网目标）
const PAGE = process.env.PAGE_URL || `${ORIGIN}/tabs/settings/peers`;
const SHOT = process.env.OUT || "test-visual/peer-scan.png";

const fail = [];
function check(name, cond, detail) {
  console.log(`${cond ? "PASS" : "FAIL"} ${name}${detail ? ` — ${detail}` : ""}`);
  if (!cond) fail.push(name);
}

// ① 真实票据（Hub = 本进程，loopback 允许；与桌面端二维码内容同构）
const ticketRes = await fetch(`${HUB}/api/peerlink/ticket`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ hub: HUB }),
});
const ticket = await ticketRes.json();
console.log("ticket:", { pairingId: ticket.pairingId, hub: ticket.hub, expiresIn: ticket.expiresIn });
if (ticketRes.status !== 200 || !ticket.pairingId) throw new Error(`票据失败: ${ticketRes.status}`);
const pairingCode = JSON.stringify({ v: 2, hub: ticket.hub, pairingId: ticket.pairingId, psk: ticket.psk, exp: 119 });

const browser = await chromium.launch({ executablePath: "/usr/bin/chromium", args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
const errors = [];
page.on("pageerror", e => errors.push(String(e)));

await page.goto(PAGE, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(2500);

// ① 无相机提示可见
const noCamera = await page.locator("[data-testid=scan-no-camera]").first();
check("① web 无相机时提示改用粘贴", await noCamera.isVisible().catch(() => false), (await noCamera.textContent().catch(() => ""))?.trim().slice(0, 40));

// ② 点「开始扫码」→ 错误必须可见（禁止静默失败）
await page.locator("[data-testid=scan-start]").first().click();
await page.waitForTimeout(800);
const scanErr = (await page.locator("[data-testid=scan-error]").first().textContent().catch(() => "")) || "";
check("② 扫码失败可见（非静默）", scanErr.trim().length > 0, scanErr.trim().slice(0, 60));

// ②b 负向对照：非法配对码必须**可见地**报错（证明 ③ 的成功不是"永远绿"）
await page.locator("[data-testid=paste-input]").first().fill('{"nope":1}');
await page.locator("[data-testid=paste-submit]").first().click();
await page.waitForTimeout(600);
const badText = ((await page.locator("[data-testid=scan-error]").first().textContent().catch(() => "")) || "").trim();
check("②b 非法配对码可见报错（负向对照）", /无法识别|缺少必需字段|Unrecognized|missing/i.test(badText), badText.slice(0, 40));

// ③ 粘贴真实配对码 → 连接
await page.locator("[data-testid=paste-input]").first().fill(pairingCode);
await page.locator("[data-testid=paste-submit]").first().click();
await page.waitForTimeout(2500);
const okText = ((await page.locator("[data-testid=scan-ok]").first().textContent().catch(() => "")) || "").trim();
const errText = ((await page.locator("[data-testid=scan-error]").first().textContent().catch(() => "")) || "").trim();
check("③ 粘贴配对码 → 连接成功", okText.length > 0, `ok="${okText.slice(0, 60)}" err="${errText.slice(0, 80)}"`);

// ④ Edge 状态：running + connected（真 WebSocket）
let status = null;
for (let i = 0; i < 10; i++) {
  const r = await fetch(`${ORIGIN}/api/peerlink/edge/status`, { headers: { "X-Peerlink-Operator": "1" } });
  status = await r.json();
  if (status.running && status.connected) break;
  await new Promise(r2 => setTimeout(r2, 500));
}
check("④ /edge/status = running + connected", !!(status?.running && status?.connected), JSON.stringify(status));

const edgeLabel = ((await page.locator("[data-testid=edge-status]").first().textContent().catch(() => "")) || "").trim();
check("④b 页面互联状态显示已连上", /已连上|Connected/.test(edgeLabel), edgeLabel.slice(0, 60));

// ⑤ Hub 侧看到该 peer 在线（自证：不是只有本端说成功）
const peersRes = await fetch(`${ORIGIN}/api/peerlink/peers`, { headers: { "X-Peerlink-Operator": "1" } });
const peers = await peersRes.json();
const online = (peers.items || []).filter(p => p.online);
check("⑤ Hub 侧 peers 有在线 peer", peersRes.status === 200 && online.length > 0, JSON.stringify((peers.items || []).map(p => ({ id: String(p.id).slice(0, 8), online: p.online }))));

// ⑥ 红线：psk 绝不落盘（localStorage / sessionStorage 都不得出现）
const storageDump = await page.evaluate(() => JSON.stringify(localStorage) + JSON.stringify(sessionStorage));
check("⑦ psk 不落盘（存储里搜不到票据密钥）", !storageDump.includes(ticket.psk) && !storageDump.includes(ticket.pairingId), `len=${storageDump.length}`);

check("⑥ 无 JS 运行时错误", errors.length === 0, errors.slice(0, 2).join(" | "));

await page.screenshot({ path: SHOT });
await browser.close();

console.log(fail.length === 0 ? "PEER_SCAN_E2E_OK" : `PEER_SCAN_E2E_FAIL: ${fail.join(", ")}`);
process.exit(fail.length === 0 ? 0 : 1);
