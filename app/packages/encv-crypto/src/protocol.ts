/**
 * protocol.ts —— 主线程 ↔ Worker 的消息协议（两端共用同一份定义）。
 *
 * 这一层只描述「消息形状」，不做任何密码学运算：
 * 所有加解密都在 WASM（Go 编译产物）里完成，TS 侧永远拿不到密钥派生过程。
 */

/** 协议版本。SDK 与 worker 必须一致，否则直接拒绝工作（避免半新半旧的组合）。 */
export const PROTOCOL_VERSION = 1;

/** 与 Go 侧 crypto.CipherMode_v4 同义：0=AES-128-CTR，1=AES-256-CTR。 */
export type CipherMode = 0 | 1;

/** 加密参数。不传则用 WASM 侧默认值（AES-256-CTR / 10000 迭代 / 16 字节盐）。 */
export interface EncryptOptions {
  cipherMode?: CipherMode;
  iterations?: number;
  saltLen?: number;
}

/** 信封头（只读信息，由 WASM 解析后返回）。 */
export interface EnvelopeHeader {
  magic: string;
  formatVersion: number;
  cipherMode: number;
  kdf: number;
  iterations: number;
  saltLen: number;
  ivLen: number;
  hintLen: number;
  hintHex: string;
  saltHex: string;
  ivHex: string;
  payloadOffset: number;
  payloadLength: number;
}

/** 信封常量（直接从 WASM 读取，前端不硬编码，避免漂移）。 */
export interface EnvelopeConstants {
  magic: string;
  formatVersion: number;
  headerSize: number;
  hintSize: number;
  defaultSaltLen: number;
  defaultIVLen: number;
  defaultIterations: number;
  cipherAES128CTR: number;
  cipherAES256CTR: number;
  defaultCipherMode: number;
  kdfPBKDF2SHA256: number;
  algorithm: string;
}

export type WorkerRequest =
  | { id: number; type: "init"; protocolVersion: number; execSource: string; wasm: ArrayBuffer }
  | { id: number; type: "encrypt"; data: ArrayBuffer; password: string; options?: EncryptOptions }
  | {
      id: number;
      type: "encryptWithEntropy";
      data: ArrayBuffer;
      password: string;
      options?: EncryptOptions;
      salt: ArrayBuffer;
      iv: ArrayBuffer;
    }
  | { id: number; type: "decrypt"; data: ArrayBuffer; password: string }
  | { id: number; type: "encryptText"; text: string; password: string; options?: EncryptOptions }
  | { id: number; type: "decryptText"; data: ArrayBuffer; password: string }
  | { id: number; type: "verifyPassword"; data: ArrayBuffer; password: string }
  | { id: number; type: "deriveKey"; password: string; salt: ArrayBuffer; keyLen: number }
  | { id: number; type: "parseHeader"; data: ArrayBuffer }
  | { id: number; type: "constants" };

/** 对联合类型逐支 Omit（直接 Omit<Union, K> 会把联合压成一个对象类型，字段全丢）。 */
type DistributiveOmit<T, K extends keyof never> = T extends unknown ? Omit<T, K> : never;

/** 去掉自增 id 之后的请求体，供 client 侧构造。 */
export type WorkerRequestBody = DistributiveOmit<WorkerRequest, "id">;

export type WorkerResponse =
  | { id: number; ok: true; bytes?: ArrayBuffer; text?: string; value?: unknown }
  | { id: number; ok: false; error: string };
