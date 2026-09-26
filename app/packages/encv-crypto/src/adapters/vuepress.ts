/**
 * adapters/vuepress.ts —— VuePress / Vite 站点的适配层。
 *
 * 只有「怎么把 wasm 读进来」是 VuePress 特有的；加解密路径与 Obsidian / 思源完全一致。
 *
 * 用法（推荐把 wasm 产物拷到 public 目录，或由 vite 插件复制到 base 路径）：
 *
 *   // .vuepress/encryptEnhance.ts（客户端增强）
 *   import EncvWorker from "@encv/crypto/worker?worker";
 *   import { createVuePressCrypto } from "@encv/crypto/adapters/vuepress";
 *
 *   export default defineClientAppEnhance(async () => {
 *     const encv = await createVuePressCrypto({
 *       base: "/encv-crypto/",
 *       createWorker: () => new EncvWorker(),
 *     });
 *     const cipher = await encv.encryptTextToBase64("# 机密段落", password);
 *   });
 */

import { createCryptoClient, fetchBinary, fetchText, joinUrl, type CryptoClient } from "../client";

export interface VuePressCryptoOptions {
  /** wasm 产物所在 URL 前缀，默认 "/encv-crypto/"。 */
  base?: string;
  /** 传入则由 Worker 承载加解密；不传在主线程跑（构建/SSR 更安全）。 */
  createWorker?: () => Worker;
  timeoutMs?: number;
}

export function createVuePressCrypto(options: VuePressCryptoOptions = {}): Promise<CryptoClient> {
  const base = options.base ?? "/encv-crypto/";
  return createCryptoClient({
    loadWasm: () => fetchBinary(joinUrl(base, "encv.wasm")),
    loadExecSource: () => fetchText(joinUrl(base, "wasm_exec.js")),
    createWorker: options.createWorker,
    timeoutMs: options.timeoutMs,
  });
}
