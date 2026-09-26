/**
 * worker.ts —— Worker 入口：在后台线程加载 WASM 并执行加解密。
 *
 * 设计要点：
 *  1. 这里不 import 任何资源（不用 ?url / ?worker），wasm 与 wasm_exec.js 由主线程
 *     通过 init 消息送进来。这样同一份 worker 源码能被 Vite / esbuild / rollup 任意打包器处理，
 *     VuePress、Obsidian、思源三种插件形态都不用各写一份 worker。
 *  2. 所有加解密都在 WASM 里完成，worker 只是调度层。
 *
 * 用法（Vite）：
 *   import EncvWorker from "@encv/crypto/worker?worker";
 *   const crypto = await createCryptoClient({ ..., createWorker: () => new EncvWorker() });
 */

import { loadEncvWasm, type EncvWasmApi } from "./core";
import { PROTOCOL_VERSION, type WorkerRequest, type WorkerResponse } from "./protocol";

/** 只声明本 worker 用到的作用域能力，避免同时引入 DOM 与 WebWorker 两套 lib 造成类型冲突。 */
interface WorkerScope {
  onmessage: ((event: MessageEvent<WorkerRequest>) => void) | null;
  postMessage(message: WorkerResponse, transfer?: Transferable[]): void;
}

const ctx = self as unknown as WorkerScope;

let apiPromise: Promise<EncvWasmApi> | null = null;

ctx.onmessage = (event: MessageEvent<WorkerRequest>) => {
  void handle(event.data);
};

async function handle(req: WorkerRequest): Promise<void> {
  try {
    switch (req.type) {
      case "init": {
        if (req.protocolVersion !== PROTOCOL_VERSION) {
          reply({ id: req.id, ok: false, error: `协议版本不一致：SDK=${req.protocolVersion} worker=${PROTOCOL_VERSION}` });
          return;
        }
        apiPromise = loadEncvWasm({ wasm: req.wasm, execSource: req.execSource });
        await apiPromise;
        reply({ id: req.id, ok: true });
        return;
      }
      default: {
        if (!apiPromise) {
          reply({ id: req.id, ok: false, error: "worker 尚未 init" });
          return;
        }
        const api = await apiPromise;
        switch (req.type) {
          case "encrypt":
            replyBytes(req.id, api.encrypt(new Uint8Array(req.data), req.password, req.options));
            return;
          case "encryptWithEntropy":
            replyBytes(
              req.id,
              api.encryptWithEntropy(new Uint8Array(req.data), req.password, req.options, new Uint8Array(req.salt), new Uint8Array(req.iv))
            );
            return;
          case "decrypt":
            replyBytes(req.id, api.decrypt(new Uint8Array(req.data), req.password));
            return;
          case "encryptText":
            replyBytes(req.id, api.encryptText(req.text, req.password, req.options));
            return;
          case "decryptText":
            reply({ id: req.id, ok: true, text: api.decryptText(new Uint8Array(req.data), req.password) });
            return;
          case "verifyPassword":
            reply({
              id: req.id,
              ok: true,
              value: api.verifyPassword(new Uint8Array(req.data), req.password),
            });
            return;
          case "deriveKey":
            replyBytes(req.id, api.deriveKey(req.password, new Uint8Array(req.salt), req.keyLen));
            return;
          case "parseHeader":
            reply({ id: req.id, ok: true, value: api.parseHeader(new Uint8Array(req.data)) });
            return;
          case "constants":
            reply({ id: req.id, ok: true, value: api.constants() });
            return;
        }
      }
    }
  } catch (error) {
    reply({ id: req.id, ok: false, error: error instanceof Error ? error.message : String(error) });
  }
}

function replyBytes(id: number, bytes: Uint8Array): void {
  // 拷一份独立 buffer 再转移：Go wasm 的内存会被后续调用复用，不能直接交出视图。
  const buffer = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(buffer).set(bytes);
  reply({ id, ok: true, bytes: buffer }, [buffer]);
}

function reply(message: WorkerResponse, transfer?: Transferable[]): void {
  ctx.postMessage(message, transfer);
}
