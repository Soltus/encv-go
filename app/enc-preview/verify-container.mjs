/**
 * verify-container.mjs —— 在 **Node** 里用同一份 wasm 内核校验容器（不开浏览器）。
 *
 * 用处：浏览器产出的容器（尤其是大附件）要有一条不依赖浏览器内存的验真路径。
 * 这里加载的是与浏览器/主应用同源的 `encv-container.wasm`，所以能验证的仍然是
 * 主线 `internal/v2/reader` 这条读取路径 —— 与浏览器里的 open/readRange 完全一致。
 *
 * 用法：
 *   node app/enc-preview/verify-container.mjs <容器路径> <口令>
 *   node app/enc-preview/verify-container.mjs <容器路径> <口令> --pattern  # 按 pw-enc-stream.ts 的生成规则逐字节比对
 *
 * 与 `encv decrypt-v2` 的关系：CLI 插件路径（fragment 栈）现在也能解 wasm/流式产物了
 * （per-fragment nonce，见 README「已知边界」），体积大时**优先用 CLI** ——
 * 本脚本把容器整体读进 wasm 线性内存，实测上限约 512MB（1GB 会在 info() 那步被打死，
 * 脚本会明说，不假装通过）。需要"不开浏览器"的小体积快速验真时用它。
 */

import fs from "node:fs";
import { webcrypto } from "node:crypto";

const [containerPath, password, ...rest] = process.argv.slice(2);
if (!containerPath || !password) {
  console.error("用法：node verify-container.mjs <容器路径> <口令> [--pattern]");
  process.exit(2);
}
const usePattern = rest.includes("--pattern");

const BUILD_DIR = new URL("./wasm/", import.meta.url);
const WASM = new URL("encv-container.wasm", BUILD_DIR);
const GLUE = new URL("wasm_exec.js", BUILD_DIR);
for (const [label, p] of [
  ["容器", containerPath],
  ["wasm", WASM],
  ["胶水", GLUE],
]) {
  if (!fs.existsSync(p)) {
    console.error(`✗ 找不到${label}：${p instanceof URL ? p.pathname : p}（先跑 make wasm）`);
    process.exit(2);
  }
}

// Go 的浏览器胶水依赖这几个全局对象；Node 里手动补上即可复用同一份 wasm_exec.js。
globalThis.fs = fs;
globalThis.process = process;
globalThis.crypto ??= webcrypto;
globalThis.performance ??= performance;

await import(GLUE.href);

const go = new globalThis.Go();
const instance = (await WebAssembly.instantiate(fs.readFileSync(WASM), go.importObject)).instance;
go.run(instance); // 不 await：内核挂好 globalThis 后由下面的轮询接管

for (let i = 0; i < 500 && !globalThis.encvContainer; i++) await new Promise(r => setTimeout(r, 10));
const api = globalThis.encvContainer;
if (!api) {
  console.error("✗ wasm 内核未挂载 globalThis.encvContainer");
  process.exit(1);
}

const bytes = new Uint8Array(fs.readFileSync(containerPath));
const opened = api.open(bytes, password);
if (!opened.ok) {
  console.error(`✗ 打开失败：${opened.error}`);
  process.exit(1);
}
const handle = opened.value.handle;
const infoRes = api.info(handle);
if (!infoRes || !infoRes.ok) {
  // 大容器里最常遇到的是这一步直接把 wasm 打死：容器 + 明文都要在
  // Go 的线性内存里，Node/浏览器的上限都绕不过去（实测 ~512MB 可过，1GB 不行）。
  console.error(`✗ 取容器信息失败：${infoRes?.error ?? "内核无响应（多半是容器太大、wasm 内存不够）"}`);
  process.exit(1);
}
const info = infoRes.value;
const total = Number(info.plainLength);
console.log(
  `容器 ${bytes.length} 字节 → 明文 ${total} 字节 · ${info.segments} 段 · 类型 ${info.containerType} · 分层密钥 ${
    info.hasWrappedDEK ? "✓" : "无"
  }`
);

// pw-enc-stream.ts 里 chunks() 的规则：每片（1MB 对齐）内部，下标每 997 写一个
// (全局偏移 & 0xff)，其余为 0。注意判定用的是**片内下标**，不是全局偏移。
const CHUNK = 1 << 20;
function expectedByte(g) {
  const chunkStart = Math.floor(g / CHUNK) * CHUNK;
  return (g - chunkStart) % 997 === 0 ? g & 0xff : 0;
}

let bad = -1;
if (usePattern) {
  const WINDOW = 4 << 20;
  for (let off = 0; off < total && bad < 0; off += WINDOW) {
    const got = new Uint8Array(api.readRange(handle, off, Math.min(WINDOW, total - off)).data);
    for (let i = 0; i < got.length; i++) {
      if (got[i] !== expectedByte(off + i)) {
        bad = off + i;
        break;
      }
    }
  }
}

api.close(handle);
if (usePattern) {
  if (bad >= 0) {
    console.error(`✗ 第 ${bad} 字节是 ${expectedByte(bad)} 规则之外的取值`);
    process.exit(1);
  }
  console.log(`✓ 逐字节比对通过（${total} 字节，与 pw-enc-stream.ts 的生成规则一致）`);
} else {
  console.log("（未指定 --pattern，跳过逐字节比对）");
}
