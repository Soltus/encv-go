/**
 * adapters/siyuan.ts —— 思源笔记插件的适配层。
 *
 * 思源插件的前端资源通过 `/plugins/<name>/...` 提供，可以直接 fetch，
 * 因此这里只是把 base 路径算对；加解密仍走同一份 WASM。
 *
 * 用法：
 *
 *   import { createSiYuanCrypto } from "@encv/crypto/adapters/siyuan";
 *
 *   const encv = await createSiYuanCrypto({ pluginName: "encv-crypto" });
 *   const cipher = await encv.encryptText(blockMarkdown, password);
 */

import { createCryptoClient, fetchBinary, fetchText, joinUrl, type CryptoClient } from "../client";

export interface SiYuanCryptoOptions {
  /** 插件名（manifest.json 的 name），用于拼出 /plugins/<name>/ 前缀。 */
  pluginName: string;
  /** 覆盖默认前缀（思源版本差异或自定义静态目录时使用）。 */
  base?: string;
  /** 需要后台线程时注入（可选）。 */
  createWorker?: () => Worker;
  timeoutMs?: number;
}

export function createSiYuanCrypto(options: SiYuanCryptoOptions): Promise<CryptoClient> {
  const base = options.base ?? `/plugins/${options.pluginName}/`;
  return createCryptoClient({
    loadWasm: () => fetchBinary(joinUrl(base, "encv.wasm")),
    loadExecSource: () => fetchText(joinUrl(base, "wasm_exec.js")),
    createWorker: options.createWorker,
    timeoutMs: options.timeoutMs,
  });
}
