// P2b 配对二维码：**dev + 网关（:16666）**路径的真实浏览器回归锁
//
// 为什么必须锁这条路径（2026-10-03 真 bug）：
//   配对面板是运行时 `import("qrcode")`，dev 态由 vite **按需**预打包；
//   若预打包失败（实测：陈旧 pnpm store 路径 ENOENT ⇒ 重优化崩），
//   `/node_modules/.vite/deps/qrcode.js` 不存在 ⇒ 动态 import 404 ⇒
//   UI 报"二维码依赖未安装"（**误导**，依赖其实装着的）。
//   生产构建把 qrcode 打进 chunk，所以这条路径**只有 dev+网关才暴露**。
//
// 用法（需 preview-gateway :16666 已起）：
//   URL=http://127.0.0.1:16666/tabs/settings/peers node pw-pairing-qr.mjs
import { chromium } from "@playwright/test";

const url = process.env.URL || "http://127.0.0.1:16666/tabs/settings/peers";
const results = [];
const check = (name, ok, extra = "") => {
  results.push({ name, ok });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${extra ? "  — " + extra : ""}`);
};

const browser = await chromium.launch({
  executablePath: "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage"],
});
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60000 });
await page.waitForTimeout(4000);

await page.getByText("生成配对码", { exact: false }).first().click();
await page.waitForTimeout(4000);

const info = await page.evaluate(() => {
  const c = document.querySelector("canvas");
  let pixels = null;
  if (c) {
    const d = c.getContext("2d").getImageData(0, 0, c.width, c.height).data;
    let dark = 0;
    let light = 0;
    for (let i = 0; i < d.length; i += 4) {
      if (d[i + 3] === 0) continue;
      if (d[i] < 128) dark++;
      else light++;
    }
    pixels = { w: c.width, h: c.height, dark, light };
  }
  const diag = document.querySelector(".qrDiag");
  return {
    canvas: pixels,
    diag: diag ? diag.textContent : null,
    waiting: (document.body.innerText || "").includes("等待安卓端扫码"),
  };
});

check("二维码画布已渲染（220×220）", !!info.canvas && info.canvas.w === 220 && info.canvas.h === 220,
  JSON.stringify(info.canvas));
check("画布有真实深浅模块（非全透明）", !!info.canvas && info.canvas.dark > 100 && info.canvas.light > 100,
  `dark=${info.canvas?.dark} light=${info.canvas?.light}`);
check("无 qrDiag 报错（渲染未降级）", !info.diag, info.diag ?? "");
check("进入等待扫码态", info.waiting);

const failed = results.filter(r => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} PASS`);
await browser.close();
process.exit(failed ? 1 : 0);
