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
//   HYBRID_SHOT  截图路径（默认 /tmp/hybrid-player.png）
//
// 断言（失败即 exit 1）—— 全部是"可观测的真实状态"，不是"没抛异常"：
//   1) /stream 响应 206（解密流真的按需供给）
//   2) artplayer 触发 video:playing（事件层）
//   3) <video> 真实解码中：readyState>=2 且 paused=false
//   4) currentTime 真的在前进（2 秒内前进 > 0.1s）—— 卡在第一帧也会 PASS 的假绿在这里被抓出
//   5) 拖到 1.0s 之后仍能解码播放（真机最常命中的路径；后端 Range 静默失效就是在这里炸的）
//   6) 页面无「播放失败」错误卡片 / 无未捕获异常
// （Range 请求头只观测不断言：小样本会被整段缓冲，拖动不再发 Range；
//   Range 正确性由 emu-backend-check.sh 的 seek 断言 + Go 单测锁住）
import { chromium } from "playwright";

const API = process.env.HYBRID_API ?? "http://127.0.0.1:12025";
const BASE = process.env.HYBRID_BASE ?? "http://127.0.0.1:18081";
const FILE = process.env.HYBRID_PATH ?? "/d/primary/out/sample.4pm.sccgv";
const NAME = process.env.HYBRID_NAME ?? "sample.mp4";
const SHOT = process.env.HYBRID_SHOT ?? "/tmp/hybrid-player.png";
const TIMEOUT = Number(process.env.HYBRID_TIMEOUT ?? 40000);

let saw206 = false;
let sawPlaying = false;
let sawError = false;
let sawSeekRange = false; // 播放器真的发过 start>0 的 Range（拖动/续传）
const pageErrors: string[] = [];
const streamCodes: string[] = [];
const streamRanges: string[] = [];

const browser = await chromium.launch({
  executablePath: process.env.HYBRID_CHROME ?? "/usr/bin/chromium",
  args: ["--no-sandbox", "--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage({ viewport: { width: 420, height: 900 } });
page.on("response", (r) => {
  if (r.url().includes("/stream")) {
    console.log(`[stream] ${r.status()} ${r.url()}`);
    streamCodes.push(String(r.status()));
    if (r.status() === 206) saw206 = true;
    const rg = r.request().headers()["range"] ?? "";
    if (rg) {
      streamRanges.push(rg);
      const m = /^bytes=(\d+)-/.exec(rg);
      if (m && Number(m[1]) > 0) sawSeekRange = true; // 非 0 起点 = 真拖动/续传
    }
  }
});
page.on("console", (m) => {
  const t = m.text();
  if (t.includes("video:playing")) sawPlaying = true;
  if (t.includes("Artplayer error event")) sawError = true;
});
page.on("pageerror", (e) => {
  sawError = true;
  pageErrors.push(e.message);
  console.log(`[pageerror] ${e.message}`);
});

// 真机上 API base 来自 localStorage（useApiBaseProbe 写入）；这里显式指向转发端口
await page.goto(BASE + "/", { waitUntil: "domcontentloaded", timeout: 30000 });
await page.evaluate((api) => localStorage.setItem("encv-server-url", api), API);
await page.goto(
  `${BASE}/player?path=${encodeURIComponent(FILE)}&name=${encodeURIComponent(NAME)}`,
  { waitUntil: "domcontentloaded", timeout: 30000 }
);

// 等 <video> 出现（最多 TIMEOUT），不再无脑 sleep
let videoState = { readyState: 0, paused: true, currentTime: 0, duration: 0, w: 0, h: 0 };
try {
  await page.waitForFunction(
    () => {
      const v = document.querySelector("video") as HTMLVideoElement | null;
      return !!v && v.readyState >= 2;
    },
    { timeout: TIMEOUT }
  );
  videoState = await page.evaluate(() => {
    const v = document.querySelector("video") as HTMLVideoElement;
    return {
      readyState: v.readyState,
      paused: v.paused,
      currentTime: v.currentTime,
      duration: v.duration,
      w: v.videoWidth,
      h: v.videoHeight,
    };
  });
} catch {
  console.log(`⚠️  ${TIMEOUT}ms 内 <video> 未达到 readyState>=2`);
}

// 关键：currentTime 必须真的前进（防"加载成功但卡死第一帧"的假绿）
const t1 = await page.evaluate(() => (document.querySelector("video") as HTMLVideoElement | null)?.currentTime ?? 0);
await page.waitForTimeout(2000);
const t2 = await page.evaluate(() => (document.querySelector("video") as HTMLVideoElement | null)?.currentTime ?? 0);
const advanced = t2 - t1 > 0.1;

// 拖动（seek）：真机上最常命中的路径。历史上这里踩过真 bug ——
// 后端对小文件的 HTTP Range 静默失效（响应头写 bytes N-M/size，实体体却返回文件头），
// 播放器一旦拖动就拿到错位字节 → 解码失败。故必须显式验证"拖动后还能解码播放"。
const seekTo = 1.0;
await page.evaluate((to) => {
  const v = document.querySelector("video") as HTMLVideoElement | null;
  if (v) v.currentTime = to;
}, seekTo);
await page.waitForTimeout(2500);
const afterSeek = await page.evaluate(() => {
  const v = document.querySelector("video") as HTMLVideoElement | null;
  return v ? { t: v.currentTime, paused: v.paused, readyState: v.readyState, err: v.error?.message ?? "" } : null;
});
// 拖过去之后必须继续播（或已正常播完），且不能停在拖动点之前
const seekOk = !!afterSeek && afterSeek.readyState >= 2 && afterSeek.t > seekTo + 0.05 && !afterSeek.err;

const body = await page.locator("body").innerText().catch(() => "");
if (/播放失败/.test(body)) sawError = true;
await page.screenshot({ path: SHOT, fullPage: true });
await browser.close();

const decoding = videoState.readyState >= 2 && !videoState.paused;
console.log("\n=== 观测 ===");
console.log(`/stream 状态码     : ${streamCodes.join(",") || "(无)"}`);
console.log(`video             : readyState=${videoState.readyState} paused=${videoState.paused} ` +
  `${videoState.w}x${videoState.h} duration=${videoState.duration.toFixed(2)}`);
console.log(`currentTime       : ${t1.toFixed(2)} → ${t2.toFixed(2)}（前进 ${(t2 - t1).toFixed(2)}s）`);
console.log(`拖动到 ${seekTo}s    : ${afterSeek ? `currentTime=${afterSeek.t.toFixed(2)} paused=${afterSeek.paused} readyState=${afterSeek.readyState}${afterSeek.err ? " err=" + afterSeek.err : ""}` : "无 <video>"}`);
// ⚠️ 仅观测、不作断言：小样本（几十 KB）会被浏览器整段缓冲，拖动不再触发新的 Range 请求，
//    观测到 "bytes=0-" 属预期。后端侧的 Range 正确性由 emu-backend-check.sh 的 seek 断言锁住
//    （以及 Go 单测 content_range_cached_test.go）。
console.log(`Range 请求头（观测）: ${streamRanges.join(" , ") || "(无)"}${sawSeekRange ? "" : "  ← 小样本整段缓冲，属预期"}`);
if (pageErrors.length) console.log(`未捕获异常         : ${pageErrors.join(" | ")}`);

console.log("\n=== 断言 ===");
console.log(`/stream 206                : ${saw206 ? "PASS" : "FAIL"}`);
console.log(`artplayer playing 事件      : ${sawPlaying ? "PASS" : "FAIL"}`);
console.log(`<video> 真实解码中          : ${decoding ? "PASS" : "FAIL"}`);
console.log(`currentTime 前进            : ${advanced ? "PASS" : "FAIL"}`);
console.log(`拖动后仍能解码播放          : ${seekOk ? "PASS" : "FAIL"}`);
console.log(`无错误卡片/未捕获异常        : ${!sawError ? "PASS" : "FAIL"}`);
if (!saw206 || !sawPlaying || !decoding || !advanced || !seekOk || sawError) {
  console.log("\n❌ 端到端验证失败（截图 " + SHOT + "）");
  process.exit(1);
}
console.log("\n✅ 端到端验证通过：加密视频解密流 → 浏览器真实解码播放");
