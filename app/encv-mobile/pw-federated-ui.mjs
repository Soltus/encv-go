// P3 来源徽章 UI 验证（真实浏览器渲染；远端数据用 route stub，本端搜索走真后端）
//
// 断言：
//   - 结果区出现「本机」与「安卓·Pixel」两种来源徽章（远端命中必须可辨识来源）
//   - 远端路径原样展示（不伪装成本地路径）
//   - 对端状态 chip 显示"在线"
import { chromium } from "@playwright/test";

const ORIGIN = process.env.ORIGIN || "http://127.0.0.1:8125";
const PAGE = `${ORIGIN}/tabs/settings/peers`;
const SHOT = process.env.OUT || "test-visual/federated-ui.png";

const browser = await chromium.launch({ executablePath: "/usr/bin/chromium", args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

// 远端：模拟一个在线对端 + 它的本地索引结果
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
    body: JSON.stringify({ peer: { id: "peer-a", name: "Pixel", platform: "android" }, items: [{ path: "/sdcard/Download/报告.pdf", name: "报告.pdf", size: 1024 }] }),
  }),
);
// P3.4：远端读通道（在线打开）—— 返回字节 + 来源标注头
await page.route("**/api/peerlink/file*", route =>
  route.fulfill({
    status: 200,
    contentType: "application/octet-stream",
    // 与后端一致：非 ASCII 路径在响应头里是百分号编码（latin-1 语义）
    headers: {
      "X-Peer-Id": "peer-a",
      "X-Peer-Name": "Pixel",
      "X-Peer-Remote-Path": encodeURIComponent("/sdcard/Download/报告.pdf"),
    },
    body: "REMOTE-BYTES",
  }),
);
// 本端：走真实后端（经类网关源站代理到 encv-go）
await page.route("**/api/files/search-fulltext*", route =>
  route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({ results: [{ path: "/Movies/local.mp4", name: "local.mp4", size: 2048 }], items: [{ path: "/Movies/local.mp4", name: "local.mp4", size: 2048 }] }),
  }),
);

await page.goto(PAGE, { waitUntil: "domcontentloaded" });
await page.waitForTimeout(2500);

await page.fill("[data-testid=fed-input]", "报告");
await page.click("[data-testid=fed-run]");
await page.waitForSelector("[data-testid=fed-results]", { timeout: 10000 });

const out = await page.evaluate(() => {
  const items = Array.from(document.querySelectorAll(".fedItem")).map(el => ({
    badge: el.querySelector(".srcBadge .srcText")?.textContent?.trim() || "",
    badgeClass: el.querySelector(".srcBadge")?.className || "",
    path: el.querySelector(".fedPath")?.textContent?.trim() || "",
  }));
  const status = Array.from(document.querySelectorAll("[data-testid=fed-status] .fedStatusChip")).map(e => e.textContent.trim());
  const openLinks = Array.from(document.querySelectorAll("[data-testid=fed-open]")).map(a => ({
    text: a.textContent.trim(),
    href: a.getAttribute("href"),
  }));
  return { items, status, openLinks };
});

console.log(JSON.stringify(out, null, 2));

const hasLocal = out.items.some(i => i.badge === "本机");
const hasPeer = out.items.some(i => i.badge === "安卓·Pixel" && i.path === "/sdcard/Download/报告.pdf");
if (!hasLocal) throw new Error("缺少「本机」来源徽章");
if (!hasPeer) throw new Error("缺少「安卓·Pixel」来源徽章或远端路径被改写");
if (!out.status.some(s => s.includes("在线"))) throw new Error("缺少对端在线状态");

// P3.4：远端命中必须有「在线打开」，且链接带 peerId + **远端原始路径**
const open = out.openLinks[0];
if (!open) throw new Error("远端命中缺少「在线打开」入口");
if (!open.href.includes("peerId=peer-a")) throw new Error(`在线打开链接缺少 peerId: ${open.href}`);
if (!open.href.includes(encodeURIComponent("/sdcard/Download/报告.pdf"))) {
  throw new Error(`在线打开链接未带远端原始路径（可能被改写成本地路径）: ${open.href}`);
}
console.log("openLink:", open);

// 真取一次（走页面内 fetch，命中上面的 stub）：确认字节 + 来源标注头
const fileResp = await page.evaluate(async href => {
  const r = await fetch(href);
  return { status: r.status, peerId: r.headers.get("x-peer-id"), remotePath: r.headers.get("x-peer-remote-path"), body: await r.text() };
}, open.href);
console.log("fileResp:", fileResp);
if (fileResp.status !== 200 || fileResp.peerId !== "peer-a") {
  throw new Error(`远端读通道异常: ${JSON.stringify(fileResp)}`);
}
const decodedRemote = decodeURIComponent(fileResp.remotePath || "");
if (!decodedRemote.startsWith("/sdcard/")) {
  throw new Error(`远端读响应缺少原始远端路径标注: ${JSON.stringify(fileResp)}`);
}
console.log("decodedRemotePath:", decodedRemote);

await page.screenshot({ path: SHOT });
console.log("FED_UI_OK");
await browser.close();
