// P4 执行端审批弹窗验证（真实浏览器渲染；挂起数据用 route stub）
//
// 断言：
//   1. 本端出现挂起的远程调用时，**任何页面**都会弹出审批卡（全局挂载）
//   2. 展示来源设备、工具名、破坏性提示、超时倒计时
//   3. 点「信任此设备」→ POST /api/peerlink/agent/approve 带 decision=trust_device
//   4. 决策后挂起清空 → 弹窗消失（不会重复弹同一个）
import { chromium } from "@playwright/test";

const ORIGIN = process.env.ORIGIN || "http://127.0.0.1:8125";
const PAGE = `${ORIGIN}/tabs/files`;
const SHOT = process.env.OUT || "test-visual/remote-approval.png";

const browser = await chromium.launch({ executablePath: "/usr/bin/chromium", args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

const CALL_ID = "call-1";
let pendingItems = [
  {
    callId: CALL_ID,
    peerId: "peer-x",
    peerName: "Desktop",
    tool: "encrypt_video",
    destructive: true,
    createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 80_000).toISOString(),
  },
];

await page.route("**/api/peerlink/agent/pending", route =>
  route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: pendingItems }) }),
);

const approveCalls = [];
await page.route("**/api/peerlink/agent/approve", route => {
  const body = JSON.parse(route.request().postData() || "{}");
  approveCalls.push(body);
  // 决策成功后挂起清空
  pendingItems = [];
  return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ok: true }) });
});

await page.goto(PAGE, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(1500);
await page.waitForSelector("[data-testid=remote-approval]", { timeout: 15000 });

const out = await page.evaluate(() => {
  const el = document.querySelector("[data-testid=remote-approval]");
  return {
    peer: el.querySelector("[data-testid=ra-peer]")?.textContent?.trim() || "",
    tool: el.querySelector("[data-testid=ra-tool]")?.textContent?.trim() || "",
    countdown: el.querySelector("[data-testid=ra-countdown]")?.textContent?.trim() || "",
    destructive: !!el.querySelector("[data-testid=ra-destructive]"),
    buttons: Array.from(el.querySelectorAll("button")).map(b => b.textContent.trim()),
  };
});
console.log(JSON.stringify(out, null, 2));

if (out.peer !== "Desktop") throw new Error(`来源设备展示错误: ${out.peer}`);
if (out.tool !== "encrypt_video") throw new Error(`工具名展示错误: ${out.tool}`);
if (!out.destructive) throw new Error("破坏性工具未显示警示");
if (!/\d+s/.test(out.countdown)) throw new Error(`倒计时缺失: ${out.countdown}`);
if (out.buttons.length !== 3) throw new Error(`决策按钮数量应为 3: ${JSON.stringify(out.buttons)}`);

await page.screenshot({ path: SHOT, fullPage: false });

// 点「信任此设备」
await page.click("[data-testid=ra-trust]");
await page.waitForTimeout(2600); // 等下一轮轮询回来（2s）

if (approveCalls.length !== 1) throw new Error(`approve 调用次数异常: ${JSON.stringify(approveCalls)}`);
if (approveCalls[0].decision !== "trust_device" || approveCalls[0].callId !== CALL_ID) {
  throw new Error(`approve 负载错误: ${JSON.stringify(approveCalls[0])}`);
}
// 决策后不应再弹同一个
const stillOpen = await page.$("[data-testid=remote-approval]");
if (stillOpen) throw new Error("决策后弹窗未消失（可能重复弹同一个请求）");

console.log("approveCalls:", JSON.stringify(approveCalls));
await browser.close();
console.log("REMOTE_APPROVAL_OK");
