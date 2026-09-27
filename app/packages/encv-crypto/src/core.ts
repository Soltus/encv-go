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
 * 不使用 eval（不需要 unsafe-eval，CSP 更友好），按宿主能力分三条路径：
 *  1. 主线程：<script src=blobURL> 注入
 *  2. 经典 worker：importScripts(blobURL)
 *  3. module worker：动态 import(blobURL)
 *
 * 第 3 条是 2026-09-27 补的：module worker 的 WorkerGlobalScope 上**依然挂着**
 * importScripts 属性，但一旦调用就抛
 * `Failed to execute 'importScripts' on 'WorkerGlobalScope': Module scripts don't support importScripts()`。
 * 也就是说「有没有这个属性」回答不了「能不能用」，只能 try 一次才知道。
 */
export async function loadGoRuntime(execSource: string): Promise<GoRuntime> {
  const url = URL.createObjectURL(new Blob([execSource], { type: "text/javascript" }));
  try {
    await evalInHost(url, execSource);
  } finally {
    URL.revokeObjectURL(url);
  }
  const GoCtor = (globalThis as { Go?: new () => GoRuntime }).Go;
  if (typeof GoCtor !== "function") {
    throw new Error("wasm_exec.js 未定义 globalThis.Go，检查胶水脚本版本是否与构建 wasm 的 Go 版本一致");
  }
  return new GoCtor();
}

async function evalInHost(url: string, execSource: string): Promise<void> {
  if (typeof document !== "undefined") {
    await loadScriptTag(url);
    return;
  }

  // 两条路径的失败原因都要保留：只报最后一条会把真正的根因（为什么 importScripts 不行）吞掉。
  const failures: string[] = [];
  const describe = (error: unknown): string => (error instanceof Error ? error.message : String(error));

  if (typeof importScripts === "function") {
    try {
      importScripts(url);
      return;
    } catch (error) {
      failures.push(`importScripts 失败：${describe(error)}`);
    }
  } else {
    failures.push("宿主没有 importScripts（module worker）");
  }

  // module worker 的最终手段：动态 import。
  //
  // ⚠️ 用 data: URL 而不是 blob: URL，是为了扛住打包器的 query 注入：
  // Vite dev 会把 `import(url)` 改写成 `import(__vite__injectQuery(url, 'import'))`，
  // 而 blob: URL 一旦被追加 `?import`，浏览器在 Blob URL Store 里就查不到它了
  // （会真的去发请求，拿回 HTML → `Unexpected token '<'`）。
  // data: URL 不受影响；末尾那个未闭合的行注释 `//` 则是把自己废掉：
  // 追加进来的 `?import` 落在注释里，不会变成非法的 JS token。
  const dataUrl = `data:text/javascript;charset=utf-8,${encodeURIComponent(`${execSource}\n//`)}`;
  try {
    await import(/* @vite-ignore */ dataUrl);
    return;
  } catch (error) {
    failures.push(`动态 import 失败：${describe(error)}`);
  }

  // data: 被 CSP 拦下的环境，最后再试一次 blob:
  try {
    await import(/* @vite-ignore */ url);
    return;
  } catch (error) {
    failures.push(`动态 import(blob) 失败：${describe(error)}`);
  }

  throw new Error(`无法在当前宿主加载 wasm_exec.js：${failures.join("；")}`);
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
    // ⚠️ 每个导出都要 unwrap：wasm 侧一律返回「结果对象」 { ok, ... }，
    //    少了这一层解包，调用方拿到的是 {ok:true,value:{...}} 而不是里面的值，
    //    读字段全是 undefined —— 而且不报错，排查时很容易误判成 wasm 返回了空数据。
    version: () => unwrap(call("version")) as string,
    constants: () => unwrap(call("constants")) as unknown as EnvelopeConstants,
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
