/**
 * adapters/obsidian.ts —— Obsidian 插件的适配层。
 *
 * Obsidian 插件跑在 Electron 里，插件目录下的文件不能随便 fetch，
 * 所以这里把「读文件」交给调用方注入（通常是 app.vault.adapter），
 * 加解密仍然走同一份 WASM。
 *
 * 用法：
 *
 *   import { createObsidianCrypto } from "@encv/crypto/adapters/obsidian";
 *
 *   const encv = await createObsidianCrypto({
 *     dir: `${this.manifest.dir}/wasm`,
 *     readBinary: (p) => this.app.vault.adapter.readBinary(p),
 *     readText: (p) => this.app.vault.adapter.read(p),
 *   });
 *   const cipher = await encv.encryptText(noteContent, password);
 *
 * 默认不使用 Worker：Obsidian 插件由 esbuild 打成单文件，
 * 拆 worker 需要额外打包配置；笔记量级的主线程开销（~10ms）可以接受。
 * 确实需要后台线程时，自己打包 worker 并通过 createWorker 注入即可。
 */

import { createCryptoClient, type CryptoClient } from "../client";

export interface ObsidianCryptoOptions {
  /** 插件内 wasm 目录（含 encv.wasm 与 wasm_exec.js）。 */
  dir: string;
  /** 读二进制文件，例如 vault.adapter.readBinary。 */
  readBinary: (path: string) => Promise<ArrayBuffer>;
  /** 读文本文件，例如 vault.adapter.read。 */
  readText: (path: string) => Promise<string>;
  /** 需要后台线程时注入（可选）。 */
  createWorker?: () => Worker;
  timeoutMs?: number;
}

export function createObsidianCrypto(options: ObsidianCryptoOptions): Promise<CryptoClient> {
  const wasmPath = `${options.dir.replace(/\/$/, "")}/encv.wasm`;
  const execPath = `${options.dir.replace(/\/$/, "")}/wasm_exec.js`;
  return createCryptoClient({
    loadWasm: () => options.readBinary(wasmPath),
    loadExecSource: () => options.readText(execPath),
    createWorker: options.createWorker,
    timeoutMs: options.timeoutMs,
  });
}
