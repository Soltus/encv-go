/**
 * @encv/crypto —— ENCV 加密层的前端封装。
 *
 * 一句话：加解密在 Go 编译的 WASM 里做（复用 internal/v2/crypto），
 * 这里只提供「加载 + 调度 + 适配宿主」的能力。任何算法改动请改 Go 侧。
 */

export {
  createCryptoClient,
  createCryptoClientFromUrls,
  fetchBinary,
  fetchText,
  joinUrl,
} from "./client";
export type { CryptoClient, CryptoClientOptions, CryptoRuntimeMode } from "./client";

export { loadEncvWasm, loadGoRuntime } from "./core";
export type { EncvWasmApi } from "./core";

export { fromBase64, toBase64, utf8Decode, utf8Encode } from "./base64";

export { PROTOCOL_VERSION } from "./protocol";
export type {
  CipherMode,
  EncryptOptions,
  EnvelopeConstants,
  EnvelopeHeader,
} from "./protocol";
