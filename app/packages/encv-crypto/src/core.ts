/**
 * core.ts —— 加载 Go 编译的 WASM，并把 globalThis.encvCrypto 包成类型安全的同步 API。
 *
 * 关键点：这个文件里没有一行业务密码学代码。
 * 它只是「加载」和「转发」，加解密全部由 cmd/encv-wasm（复用 internal/v2/crypto）执行。
 * 任何需要改算法的需求，都应该改 Go 侧，而不是在这里补一段 JS。
 */

import type { CipherMode, EncryptOptions, EnvelopeConstants, EnvelopeHeader } from "./protocol";

/** WASM 导出函数的原始返回形状：{ ok, data | text | value | error }。 */
interface WasmResult {
  ok?: boolean;
  data?: Uint8Array;
  text?: string;
  value?: unknown;
  error?: string;
}

/** globalThis.Go（由 wasm_exec.js 定义）的最小类型声明。 */
interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<number>;
}

export interface EncvWasmApi {
  version(): string;
  constants(): EnvelopeConstants;
  encrypt(plain: Uint8Array, password: string, options?: EncryptOptions): Uint8Array;
  encryptWithEntropy(
    plain: Uint8Array,
    password: string,
    options: EncryptOptions | undefined,
    salt: Uint8Array,
    iv: Uint8Array
  ): Uint8Array;
  decrypt(blob: Uint8Array, password: string): Uint8Array;
  encryptText(text: string, password: string, options?: EncryptOptions): Uint8Array;
  decryptText(blob: Uint8Array, password: string): string;
  verifyPassword(blob: Uint8Array, password: string): boolean;
  deriveKey(password: string, salt: Uint8Array, keyLen: number): Uint8Array;
  parseHeader(blob: Uint8Array): EnvelopeHeader;
}

/**
 * 求值 Go 的 wasm_exec.js。
 *
 * 不使用 eval：
 *  - Worker 环境用 importScripts(blobURL)（经典 worker 才有 importScripts）；
 *  - 主线程用 <script src=blobURL> 注入。
 * 两条路径都不需要 unsafe-eval，CSP 更友好。
 */
export async function loadGoRuntime(execSource: string): Promise<GoRuntime> {
  const url = URL.createObjectURL(new Blob([execSource], { type: "text/javascript" }));
  try {
    if (typeof importScripts === "function") {
      importScripts(url);
    } else {
      await loadScriptTag(url);
    }
  } finally {
    URL.revokeObjectURL(url);
  }
  const GoCtor = (globalThis as { Go?: new () => GoRuntime }).Go;
  if (typeof GoCtor !== "function") {
    throw new Error("wasm_exec.js 未定义 globalThis.Go，检查胶水脚本版本是否与构建 wasm 的 Go 版本一致");
  }
  return new GoCtor();
}

function loadScriptTag(url: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = url;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("加载 wasm_exec.js 失败"));
    document.head.appendChild(script);
  });
}

/** 把 WASM 的「结果对象」转成异常或值，统一错误语义。 */
function unwrap(result: WasmResult, bytes: true): Uint8Array;
function unwrap(result: WasmResult): unknown;
function unwrap(result: WasmResult, bytes?: boolean): unknown {
  if (result?.ok !== true) {
    throw new Error(result?.error ?? "encv wasm: unknown error");
  }
  if (bytes) {
    return result.data ?? new Uint8Array(0);
  }
  if (result.data !== undefined) return result.data;
  if (result.text !== undefined) return result.text;
  return result.value;
}

/**
 * 加载并启动 WASM，返回可直接调用的同步 API。
 *
 * 注意：go.run() 故意不 await —— cmd/encv-wasm 的 main 注册完回调后会永久阻塞，
 * 等待 run() 的 promise 会把调用方一起挂住。
 */
export async function loadEncvWasm(options: { wasm: ArrayBuffer | Uint8Array; execSource: string }): Promise<EncvWasmApi> {
  const bytes = options.wasm instanceof Uint8Array ? options.wasm : new Uint8Array(options.wasm);
  const go = await loadGoRuntime(options.execSource);

  const result = await WebAssembly.instantiate(bytes as BufferSource, go.importObject);
  void go.run(result.instance);

  const api = (globalThis as { encvCrypto?: Record<string, (...args: never[]) => WasmResult> }).encvCrypto;
  if (!api) {
    throw new Error("wasm 未挂载 globalThis.encvCrypto，检查产物是否由 cmd/encv-wasm 构建");
  }

  const call = (name: string, ...args: unknown[]): WasmResult => {
    const fn = api[name];
    if (typeof fn !== "function") {
      throw new Error(`wasm 缺少导出函数：${name}（wasm 与 SDK 版本不匹配？）`);
    }
    return fn(...(args as never[]));
  };

  return {
    version: () => call("version") as unknown as string,
    constants: () => call("constants") as unknown as EnvelopeConstants,
    encrypt: (plain, password, opts) => unwrap(call("encrypt", plain, password, opts ?? null), true) as Uint8Array,
    encryptWithEntropy: (plain, password, opts, salt, iv) =>
      unwrap(call("encryptWithEntropy", plain, password, opts ?? null, salt, iv), true) as Uint8Array,
    decrypt: (blob, password) => unwrap(call("decrypt", blob, password), true) as Uint8Array,
    encryptText: (text, password, opts) => unwrap(call("encryptText", text, password, opts ?? null), true) as Uint8Array,
    decryptText: (blob, password) => unwrap(call("decryptText", blob, password)) as string,
    verifyPassword: (blob, password) => unwrap(call("verifyPassword", blob, password)) as boolean,
    deriveKey: (password, salt, keyLen) => unwrap(call("deriveKey", password, salt, keyLen), true) as Uint8Array,
    parseHeader: blob => unwrap(call("parseHeader", blob)) as EnvelopeHeader,
  };
}

export type { CipherMode, EncryptOptions };
