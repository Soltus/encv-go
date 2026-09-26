/**
 * parity.test.mjs —— 「Go 原生 ↔ 前端 WASM」的对等契约测试。
 *
 * 读的是 Go 侧生成的同一份黄金向量：
 *   internal/v2/crypto/simple/testdata/vectors.json
 * （由 `go run ./cmd/encv-crypto-vectors` 生成，`bash scripts/build-wasm.sh` 会自动跑）
 *
 * 只要有一侧（Go 加密层 / wasm 构建 / SDK 调用方式）发生漂移，这里立刻红。
 *
 * 运行：
 *   bash scripts/build-wasm.sh          # 先在仓库根目录构建 wasm 与向量
 *   cd app/packages/encv-crypto && node --test test/parity.test.mjs
 */

import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";
import { before, test } from "node:test";

const here = dirname(fileURLToPath(import.meta.url));
const pkgRoot = resolve(here, "..");
const wasmPath = resolve(pkgRoot, "wasm/encv.wasm");
const execPath = resolve(pkgRoot, "wasm/wasm_exec.js");
// 仓库根：app/packages/encv-crypto → app/packages → app → 仓库根
const repoRoot = resolve(pkgRoot, "../../..");
const vectorsPath = resolve(
  repoRoot,
  "internal/v2/crypto/simple/testdata/vectors.json"
);

const hasArtifacts = existsSync(wasmPath) && existsSync(execPath) && existsSync(vectorsPath);
if (!hasArtifacts) {
  console.warn("⚠ 跳过对等测试：缺少 wasm 产物或黄金向量。先在仓库根目录跑：bash scripts/build-wasm.sh");
}

const toHex = (bytes) => Buffer.from(bytes).toString("hex");
const fromHex = (hex) => new Uint8Array(Buffer.from(hex, "hex"));

/** 启动 wasm（等价于浏览器里 SDK 做的事情，只是换了 Node 的宿主胶水）。 */
async function bootWasm() {
  vm.runInThisContext(readFileSync(execPath, "utf8"), { filename: execPath });
  const GoCtor = globalThis.Go;
  assert.equal(typeof GoCtor, "function", "wasm_exec.js 未定义 globalThis.Go");

  const go = new GoCtor();
  const result = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
  // 不 await：Go 的 main 注册完导出后会永久阻塞，等待它会把测试挂住。
  const done = go.run(result.instance);
  done.catch((err) => console.error("wasm 程序异常退出：", err));

  for (let i = 0; i < 200 && !globalThis.encvCrypto; i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  const api = globalThis.encvCrypto;
  assert.ok(api, "wasm 未挂载 globalThis.encvCrypto（main 提前退出？）");
  return api;
}

/**
 * wasm 导出函数一律返回「结果对象」{ ok, data | text | value | error } 而不是抛异常，
 * 这里做与 src/core.ts 相同的解包，顺便验证 SDK 的解包语义与 wasm 契约一致。
 */
function unwrap(res) {
  if (!res || res.ok !== true) {
    throw new Error(res?.error ?? "encv wasm: unknown error");
  }
  if (res.data !== undefined) return res.data;
  if (res.text !== undefined) return res.text;
  return res.value;
}

let api;
let vectors;

before(async () => {
  if (!hasArtifacts) return;
  api = await bootWasm();
  vectors = JSON.parse(readFileSync(vectorsPath, "utf8"));
});

test("wasm 常量与黄金向量元信息一致", { skip: !hasArtifacts }, () => {
  const c = unwrap(api.constants());
  assert.equal(c.magic, vectors.magic ?? "ENCS");
  assert.equal(c.formatVersion, vectors.formatVersion);
  assert.equal(c.headerSize, vectors.headerSize);
  assert.equal(c.defaultIterations, vectors.iterations);
  assert.equal(c.algorithm, "aes-256-ctr");
});

test("密钥派生与 Go 侧 crypto.GenerateKey 完全对等", { skip: !hasArtifacts }, () => {
  assert.ok(vectors.derivations.length > 0, "向量里没有 derivation 用例");
  for (const d of vectors.derivations) {
    const out = unwrap(api.deriveKey(d.password, fromHex(d.saltHex), d.keyLen));
    assert.equal(toHex(out), d.keyHex, `派生不一致：${d.name}`);
  }
});

test("信封字节与 Go 侧逐字节一致（用固定 salt/iv 复现）", { skip: !hasArtifacts }, () => {
  assert.ok(vectors.cases.length > 0, "向量里没有 envelope 用例");
  for (const c of vectors.cases) {
    const blob = unwrap(
      api.encryptWithEntropy(
        fromHex(c.plaintextHex),
        c.password,
        { cipherMode: c.cipherMode },
        fromHex(c.saltHex),
        fromHex(c.ivHex)
      )
    );
    assert.equal(toHex(blob), c.containerHex, `信封字节不一致：${c.name}`);
  }
});

test("解密黄金信封能还原明文", { skip: !hasArtifacts }, () => {
  for (const c of vectors.cases) {
    const plain = unwrap(api.decrypt(fromHex(c.containerHex), c.password));
    assert.equal(toHex(plain), c.plaintextHex, `明文不一致：${c.name}`);
  }
});

test("随机盐/IV 加解密往返（含中文与 emoji）", { skip: !hasArtifacts }, () => {
  const text = "# 秘密\n内容：中文 ✔ emoji 🔐";
  const blob = unwrap(api.encryptText(text, "密码 with 中文"));
  assert.ok(blob.byteLength > 47, "信封长度异常");
  const back = unwrap(api.decryptText(blob, "密码 with 中文"));
  assert.equal(back, text);
});

test("密码错误必须报错而不是返回乱码", { skip: !hasArtifacts }, () => {
  const blob = unwrap(api.encryptText("secret", "correct horse"));
  const res = api.decryptText(blob, "wrong horse");
  assert.equal(res.ok, false);
  assert.match(String(res.error), /password/i);
  assert.equal(api.verifyPassword(blob, "correct horse"), true);
  assert.equal(api.verifyPassword(blob, "wrong horse"), false);
});

test("AES-128 与 AES-256 两种模式都能往返", { skip: !hasArtifacts }, () => {
  for (const cipherMode of [0, 1]) {
    const blob = unwrap(api.encryptText("mode test", "pw", { cipherMode }));
    const header = unwrap(api.parseHeader(blob));
    assert.equal(header.cipherMode, cipherMode);
    assert.equal(unwrap(api.decryptText(blob, "pw")), "mode test");
  }
});

test("坏参数只让本次调用失败，不能打死整个 wasm 实例", { skip: !hasArtifacts }, () => {
  const res = api.decryptText({ not: "a Uint8Array" }, "pw");
  assert.equal(res.ok, false, "错误类型的参数应当返回错误结果而不是 panic");
  // 关键断言：wasm 实例还活着，后续调用必须照常工作。
  const blob = unwrap(api.encryptText("still alive", "pw"));
  assert.equal(unwrap(api.decryptText(blob, "pw")), "still alive");
});
