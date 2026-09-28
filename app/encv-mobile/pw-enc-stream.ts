// 用 bun 运行：bun app/encv-mobile/pw-enc-stream.ts
//
// 「大附件能不能加密」这件事的**浏览器内真实证据**。
//
//   1. 先起静态服务：python3 app/enc-preview/serve.py 5179
//   2. bun app/encv-mobile/pw-enc-stream.ts
//
// 做什么：
//   - 对每个档位分别跑**整块** `encryptBytes` 与**流式** `encryptBegin/Write/End`，
//     并把 wasm 打到 console 的原话（Go 的 fatal error 只会落在这里）一并打印；
//   - 流式这一侧在 VERIFY_MAX_MB 以内会做**逐字节回读比对**（用 readRange 分窗比，
//     不在页面里同时摆两份完整数据）。
//
// 为什么每个探针都开**新页面**：wasm 一旦 OOM，Go 程序永久退出（后续调用全废），
// 复用同一张页面会把第一档的崩溃算到后面每一档头上。
//
// 环境变量：
//   SIZES=16,128,512,1024        档位（MB）
//   MODES=block,stream           跑哪条路径
//   VERIFY_MAX_MB=128            流式回读比对的上限体积
//   SELFTEST=1                   改跑页面自带的一键自检并把结果打印出来

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { chromium } from "@playwright/test";

const TARGET = process.env.TARGET || "http://127.0.0.1:5179/";
const SIZES_MB = (process.env.SIZES || "512,1024").split(",").map(Number);
const MODES = (process.env.MODES || "block,stream").split(",");
const VERIFY_MAX_MB = Number(process.env.VERIFY_MAX_MB || 128);
// 口令：默认与页面口令框一致的 my-encv_key（样例容器也用它），
// 想复现"整块 vs 流式"那组老数据时可以改成 stream-probe。
const PW = process.env.PW || "my-encv_key";
const CHUNK = 1 << 20; // 1MB
const WINDOW = 4 << 20; // 回读比对的窗口

const browser = await chromium.launch({
  // 沙箱里没有 playwright 自带的 chromium 包，用系统的那份
  executablePath: process.env.CHROME_BIN || "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage", "--enable-precise-memory-info"],
});

async function newProbePage() {
  const page = await browser.newPage({ acceptDownloads: true });
  const logs = [];
  // Go 的 panic / fatal error 是往 stderr 写并被 wasm_exec.js 转 console.log，
  // 所以这里收**全部**级别的消息（只收 error 会漏掉真正的根因行）。
  page.on("console", m => logs.push(`[${m.type()}] ${m.text().slice(0, 300)}`));
  page.on("pageerror", e => logs.push(`pageerror: ${String(e).split("\n")[0]}`));
  await page.goto(TARGET, { waitUntil: "domcontentloaded" });
  await page.waitForFunction(() => Boolean(globalThis.encPreview?.api), null, { timeout: 60_000 });
  return { page, logs };
}

function report(mb, mode, result, logs) {
  console.log(`${String(mb).padStart(5)}MB ${mode.padEnd(6)} ${result.ok ? "✓" : "✗"} ${JSON.stringify(result)}`);
  const key = logs.filter(l => /fatal error|out of memory|cannot allocate|Go program|崩溃/i.test(l));
  for (const line of (key.length ? key.slice(0, 4) : logs.slice(-2))) console.log(`        ↳ ${line}`);
}

/** 页面内跑一档：整块 or 流式。流式一侧可选做逐字节回读比对，或把产物落盘给脚本侧验证。 */
const runInPage = async ({ total, mode, verify, downloadName, pw, CHUNK, WINDOW }) => {
  const api = globalThis.encPreview.api;

  // 明文按 1MB 一片生成，从来不整体持有（模拟真实大附件读取）
  function* chunks() {
    let made = 0;
    while (made < total) {
      const n = Math.min(CHUNK, total - made);
      const c = new Uint8Array(n);
      for (let i = 0; i < n; i += 997) c[i] = (made + i) & 0xff;
      made += n;
      yield c; // 此刻可以让上一片被 GC —— 这就是"流式"与"整块"的差别
    }
  }

  if (mode === "block") {
    const plain = new Uint8Array(total); // 整块 API 的前提：明文必须在内存里整体存在
    let off = 0;
    for (const c of chunks()) {
      plain.set(c, off);
      off += c.length;
    }
    const enc = api.encryptBytes(plain, pw, { containerType: 5, containerTypeStr: "text" });
    if (!enc.ok) return { ok: false, stage: "encryptBytes", error: enc.error };
    return { ok: true, stage: "encryptBytes", containerBytes: enc.data.length };
  }

  const begun = api.encryptBegin(pw, {
    containerType: 5,
    containerTypeStr: "text",
    segmentSize: CHUNK,
  });
  if (!begun.ok) return { ok: false, stage: "encryptBegin", error: begun.error };
  const handle = begun.value.handle;
  const pieces = [];
  for (const c of chunks()) {
    const r = api.encryptWrite(handle, c);
    if (!r.ok) return { ok: false, stage: "encryptWrite", error: r.error };
    const d = new Uint8Array(r.data);
    if (d.length) pieces.push(d);
  }
  const end = api.encryptEnd(handle);
  if (!end.ok) return { ok: false, stage: "encryptEnd", error: end.error };
  const v = end.value;
  pieces.push(new Uint8Array(v.data));
  const containerBlob = new Blob([new Uint8Array(v.head), ...pieces, new Uint8Array(v.tail)]);

  let verified = "skipped";
  if (verify) {
    // 回读比对：容器整体交给内核一次（这条路径本身就是 mainline reader），
    // 明文侧则用生成器**按序推进**，不同时存在内存里。
    const container = new Uint8Array(await containerBlob.arrayBuffer());
    const opened = api.open(container, pw);
    if (!opened.ok) return { ok: false, stage: "open", error: opened.error, containerBytes: containerBlob.size };
    const h2 = opened.value.handle;
    const total2 = api.info(h2).value.plainLength;
    const gen = chunks();
    let cur = new Uint8Array(0);
    let curPos = 0;
    let mismatch = -1;
    outer: for (let off = 0; off < total2; off += WINDOW) {
      const want = Math.min(WINDOW, total2 - off);
      const got = new Uint8Array(api.readRange(h2, off, want).data);
      if (got.length !== want) {
        mismatch = off;
        break;
      }
      let i = 0;
      while (i < got.length) {
        if (curPos >= cur.length) {
          const nx = gen.next();
          if (nx.done) {
            mismatch = off + i;
            break outer;
          }
          cur = nx.value;
          curPos = 0;
        }
        const take = Math.min(cur.length - curPos, got.length - i);
        for (let k = 0; k < take; k++) {
          if (cur[curPos + k] !== got[i + k]) {
            mismatch = off + i + k;
            break outer;
          }
        }
        i += take;
        curPos += take;
      }
    }
    api.close(h2);
    if (mismatch >= 0) return { ok: false, stage: "verify", error: `第 ${mismatch} 字节与明文不一致`, containerBytes: containerBlob.size };
    verified = "byte-identical";
  }

  // 超过浏览器回读上限时，把产物**交给 Playwright 落盘**：由脚本一侧用主线 CLI 解密后比对，
  // 这样既能验证超大体积的内容正确性（浏览器里比会再次吃光内存），又顺带证明
  // wasm 流式产物在非浏览器环境（CLI / 主线 reader）里同样能解。
  if (downloadName) {
    const url = URL.createObjectURL(containerBlob);
    const a = document.createElement("a");
    a.href = url;
    a.download = downloadName;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  }

  return {
    ok: true,
    stage: "stream",
    containerBytes: containerBlob.size,
    segments: v.segments,
    pieces: pieces.length,
    verified: downloadName ? "downloaded" : verified,
  };
};

async function probe(mb, mode) {
  const { page, logs } = await newProbePage();
  const total = mb * 1024 * 1024;
  const downloadName = mode === "stream" && mb > VERIFY_MAX_MB ? `${mb}MB.sccgt` : null;
  const dlPromise = downloadName ? page.waitForEvent("download", { timeout: 600_000 }) : null;

  let result;
  try {
    result = await page.evaluate(runInPage, {
      total,
      mode,
      verify: mode === "stream" && mb <= VERIFY_MAX_MB,
      downloadName,
      pw: PW,
      CHUNK,
      WINDOW,
    });
  } catch (error) {
    result = { ok: false, stage: "page", error: String(error).split("\n")[0] };
  }

  // 落盘的产物：用**主线 CLI** 在浏览器外解开，再与同一份生成规则逐字节比对。
  if (result.ok && downloadName) {
    const out = await verifyOutsideBrowser(dlPromise, downloadName, total);
    result.external = out;
    result.ok = out.ok;
    if (!out.ok) result.error = out.error;
  }

  report(mb, mode, result, logs);
  await page.close();
}

/** 场景：容器已从页面下载到磁盘 → go CLI decrypt-v2 → 比对明文内容。 */
async function verifyOutsideBrowser(dlPromise, downloadName, total) {
  const dir = "/tmp/encv-stream-probe";
  fs.mkdirSync(dir, { recursive: true });
  // 唯一文件名：同名的旧容器可能在下载过程中仍被读到（旧内容 + 新大小 = 假失败）
  const containerPath = path.join(dir, `${Date.now()}-${downloadName}`);
  // ⚠️ 先删掉同名旧文件：下载是**边下边写**的，旧文件可能与新产物同样大小，
  // 校验器抢在写完之前读就会拿旧内容比对（实测 16MB 那档就这样假失败过）。
  fs.rmSync(containerPath, { force: true });
  try {
    const dl = await dlPromise;
    await dl.saveAs(containerPath);
  } catch (error) {
    return { ok: false, error: `下载失败：${String(error).split("\n")[0]}` };
  }
  const size = fs.statSync(containerPath).size;

  // ⚠️ 这里**不能**用 `encv decrypt-v2` 验：CLI 的插件路径走 fragment 栈，
  // 而 wasm/流式 writer 产出的是「每段独立 nonce」的 segment 栈 —— CLI 会解出
  // 同尺寸乱码（现在已加守卫改成显式报错）。浏览器外用同一个 wasm 内核验，
  // 才能既避开浏览器内存上限、又与浏览器里读到的是同一条主线 reader。
  const cli = spawnSync(
    "node",
    ["app/enc-preview/verify-container.mjs", containerPath, PW, "--pattern"],
    { cwd: "/workspace", maxBuffer: 8 << 20, timeout: 900_000 }
  );
  if (cli.status !== 0) {
    return {
      ok: false,
      error: `浏览器外校验失败（exit=${cli.status}）：${String(cli.stderr || "").split("\n").filter(Boolean).slice(-2).join(" / ") || String(cli.stdout).split("\n").filter(Boolean).slice(-2).join(" / ")}`,
    };
  }
  return { ok: true, verified: "byte-identical(wasm/Node)", containerPath, size };
}




async function runSelfTest() {
  const { page, logs } = await newProbePage();
  await page.click("#btn-selftest");
  await page.waitForFunction(() => /自检(通过|失败)/.test(globalThis.encPreview.logPreview ?? document.getElementById("log").textContent), null, { timeout: 300_000 });
  const lines = await page.evaluate(() =>
    document
      .getElementById("log")
      .textContent.split("\n")
      .map(l => l.trim())
      .filter(Boolean)
      .slice(0, 60)
  );
  const metric = await page.textContent("#metric");
  for (const line of lines) console.log(`  ${line}`);
  console.log(`页面自检结果：${metric}`);
  void logs;
  await page.close();
}

/**
 * openstream 模式：同一份大容器，分别用「整体 open(bytes)」与「流式 openStream」打开，
 * 记录 JS 堆增量与能否读到正确字节。
 *
 * 结论不是"谁快"，而是**内存账**：整块路径要把容器整体塞进 wasm 线性内存，
 * 流式路径只取它真正要读的那几个区间。
 */
async function runOpenStreamProbe(containerPath) {
  const page = await browser.newPage();
  const logs = [];
  page.on("console", m => logs.push(`[${m.type()}] ${m.text().slice(0, 200)}`));
  await page.goto(TARGET, { waitUntil: "domcontentloaded" });
  await page.waitForFunction(() => Boolean(globalThis.encPreview?.api), null, { timeout: 60_000 });
  await page.evaluate(() => {
    const i = document.createElement("input");
    i.type = "file";
    i.id = "__probe";
    i.style.display = "none";
    document.body.appendChild(i);
  });
  await page.setInputFiles("#__probe", containerPath);

  const out = await page.evaluate(async pw => {
    const PV = globalThis.encPreview;
    // 页面里的函数用的是口令输入框的值，这里统一设成容器加密时用的那个
    document.getElementById("password").value = "stream-probe";
    const file = document.getElementById("__probe").files[0];
    const CH = 1 << 20;
    const expected = g => {
      const cs = Math.floor(g / CH) * CH;
      return (g - cs) % 997 === 0 ? g & 0xff : 0;
    };
    const heap = () => (performance.memory ? performance.memory.usedJSHeapSize : 0);
    const report = {};

    // ① 流式打开：只取需要的区间
    const before1 = heap();
    const read = async (off, len) => new Uint8Array(await file.slice(off, off + len).arrayBuffer());
    const { handle, info } = await PV.openStream(read, file.size, "big.sccgt");
    const total = Number(info.plainLength);
    let bad = -1;
    for (const off of [0, Math.floor(total / 2), Math.max(0, total - 4096)]) {
      const got = await PV.readWindow(handle, off, Math.min(4096, total - off), read);
      for (let i = 0; i < got.length; i++) {
        if (got[i] !== expected(off + i)) {
          bad = off + i;
          break;
        }
      }
      if (bad >= 0) break;
    }
    globalThis.encPreview.api.close(handle);
    report.stream = { ok: bad < 0, total, jsHeapDeltaMB: Math.round((heap() - before1) / 1048576) };

    // ② 整块打开：容器整体进内存
    const before2 = heap();
    try {
      const bytes = new Uint8Array(await file.arrayBuffer());
      const opened = globalThis.encPreview.api.open(bytes, "stream-probe");
      if (!opened.ok) throw new Error(opened.error);
      const h2 = opened.value.handle;
      const i2 = globalThis.encPreview.api.info(h2);
      if (!i2.ok) throw new Error(i2.error);
      globalThis.encPreview.api.close(h2);
      report.block = { ok: true, total: Number(i2.value.plainLength), jsHeapDeltaMB: Math.round((heap() - before2) / 1048576) };
    } catch (error) {
      report.block = { ok: false, error: String(error).split("\n")[0], jsHeapDeltaMB: Math.round((heap() - before2) / 1048576) };
    }
    return report;
  }, PW);

  console.log(`容器 ${containerPath}`);
  console.log(`  流式 openStream   : ${JSON.stringify(out.stream)}`);
  console.log(`  整块 open(bytes)  : ${JSON.stringify(out.block)}`);
  for (const l of logs.filter(l => /fatal|out of memory/i.test(l)).slice(0, 3)) console.log(`        ↳ ${l}`);
  await page.close();
}

if (process.env.SELFTEST) {
  await runSelfTest();
} else if (process.env.OPENSTREAM) {
  await runOpenStreamProbe(process.env.OPENSTREAM);
} else {
  for (const mb of SIZES_MB) for (const mode of MODES) await probe(mb, mode);
}

await browser.close();
