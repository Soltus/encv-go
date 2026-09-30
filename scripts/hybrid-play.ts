// hybrid-play.ts —— 混合通路端到端验证（真机级"加密视频预览"回归闸门）
//
// 架构（为什么这么搭，见 .codebuddy/memory/2026-10-01.md §10）：
//   模拟器内跑真实 Go 后端（Android 文件/权限/mount 语义 = 真机）
//     ↓ adb forward tcp:12025 → tcp:2025
//   宿主机 Chromium 跑真实前端 dist（替代模拟器里会崩的 WebView：
//   crashpad minidump + SIGTRAP，属模拟器环境限制，非 app 代码问题）
//
// 用法（由 scripts/hybrid-e2e.sh 编排；也可手动）：
//   cd app/encv-mobile && bun ../../scripts/hybrid-play.ts
// 环境变量：
//   HYBRID_API   后端地址（默认 http://127.0.0.1:12025，经 adb forward）
//   HYBRID_BASE  前端静态地址（默认 http://127.0.0.1:18081）
//   HYBRID_PATH  容器虚拟路径（默认 /d/primary/out/sample.4pm.sccgv）
//   HYBRID_NAME  显示名（默认 sample.mp4）
// 断言（失败即 exit 1）：
//   1) /stream 响应 206
//   2) artplayer video:loadedmetadata 且 video:playing
//   3) 页面无「播放失败」错误卡片
import { chromium } from "playwright";

const API = process.env.HYBRID_API ?? "http://127.0.0.1:12025";
const BASE = process.env.HYBRID_BASE ?? "http://127.0.0.1:18081";
const FILE = process.env.HYBRID_PATH ?? "/d/primary/out/sample.4pm.sccgv";
const NAME = process.env.HYBRID_NAME ?? "sample.mp4";

let saw206 = false;
let sawPlaying = false;
let sawError = false;

const browser = await chromium.launch({
  executablePath: process.env.HYBRID_CHROME ?? "/usr/bin/chromium",
  args: ["--no-sandbox", "--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage({ viewport: { width: 420, height: 900 } });
page.on("response", (r) => {
  if (r.url().includes("/stream")) {
    console.log(`[stream] ${r.status()} ${r.url()}`);
    if (r.status() === 206) saw206 = true;
  }
});
page.on("console", (m) => {
  const t = m.text();
  if (t.includes("video:playing")) sawPlaying = true;
  if (t.includes("Artplayer error event")) sawError = true;
});
page.on("pageerror", (e) => {
  sawError = true;
  console.log(`[pageerror] ${e.message}`);
});

// 真机上 API base 来自 localStorage（useApiBaseProbe 写入）；这里显式指向转发端口
await page.goto(BASE + "/", { waitUntil: "domcontentloaded", timeout: 30000 });
await page.evaluate((api) => localStorage.setItem("encv-server-url", api), API);

await page.goto(
  `${BASE}/player?path=${encodeURIComponent(FILE)}&name=${encodeURIComponent(NAME)}`,
  { waitUntil: "domcontentloaded", timeout: 30000 }
);
await page.waitForTimeout(15000);

const body = await page.locator("body").innerText().catch(() => "");
if (/播放失败/.test(body)) sawError = true;
await page.screenshot({ path: process.env.HYBRID_SHOT ?? "/tmp/hybrid-player.png", fullPage: true });
await browser.close();

console.log(`\n=== 断言 ===`);
console.log(`/stream 206        : ${saw206 ? "PASS" : "FAIL"}`);
console.log(`artplayer playing  : ${sawPlaying ? "PASS" : "FAIL"}`);
console.log(`无错误卡片/事件     : ${!sawError ? "PASS" : "FAIL"}`);
if (!saw206 || !sawPlaying || sawError) {
  console.log("\n❌ 端到端验证失败");
  process.exit(1);
}
console.log("\n✅ 端到端验证通过：加密视频解密流 → artplayer 播放成功");
