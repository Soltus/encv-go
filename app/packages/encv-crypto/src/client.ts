/**
 * client.ts —— 主线程侧的门面：把 Worker（或主线程直连）包装成类型安全的异步 API。
 *
 * 三种宿主（VuePress / Obsidian / 思源）只在这里汇合：
 * 它们各自只负责「怎么把 encv.wasm 与 wasm_exec.js 读进来」，
 * 加解密路径完全一致，杜绝「每个插件各写一套加密」的漂移。
 */

import { fromBase64, toBase64, utf8Decode, utf8Encode } from "./base64";
import { loadEncvWasm } from "./core";
import {
  PROTOCOL_VERSION,
  type EncryptOptions,
  type EnvelopeConstants,
  type EnvelopeHeader,
  type WorkerRequest,
  type WorkerRequestBody,
  type WorkerResponse,
} from "./protocol";

export type CryptoRuntimeMode = "worker" | "main";

export interface CryptoClientOptions {
  /** 读取 encv.wasm 的字节（fetch / 插件 API / 本地文件皆可）。 */
  loadWasm: () => Promise<ArrayBuffer | Uint8Array>;
  /** 读取 wasm_exec.js 的源码文本（不是 URL，是为了支持不能用 fetch 的环境）。 */
  loadExecSource: () => Promise<string>;
  /** 提供则用 Worker 模式；不提供则退化为主线程模式（笔记量级完全够用）。 */
  createWorker?: () => Worker;
  /** 单次调用超时，默认 60s。 */
  timeoutMs?: number;
}

export interface CryptoClient {
  readonly mode: CryptoRuntimeMode;
  /** 字节加密 → ENCVS1 信封。 */
  encrypt(plain: Uint8Array, password: string, options?: EncryptOptions): Promise<Uint8Array>;
  /** 信封解密 → 明文字节；密码错误抛错（不会返回乱码）。 */
  decrypt(blob: Uint8Array, password: string): Promise<Uint8Array>;
  /** 文本加密 → ENCVS1 信封。 */
  encryptText(text: string, password: string, options?: EncryptOptions): Promise<Uint8Array>;
  /** 信封解密 → 明文字符串。 */
  decryptText(blob: Uint8Array, password: string): Promise<string>;
  /** 文本加密 → base64（适合直接嵌进 Markdown / 配置）。 */
  encryptTextToBase64(text: string, password: string, options?: EncryptOptions): Promise<string>;
  /** base64 信封解密 → 明文字符串。 */
  decryptTextFromBase64(text: string, password: string): Promise<string>;
  /** 只判断密码对不对，不解出明文。 */
  verifyPassword(blob: Uint8Array, password: string): Promise<boolean>;
  /** PBKDF2 密钥派生（与 Go 侧 crypto.GenerateKey 完全同一实现）。 */
  deriveKey(password: string, salt: Uint8Array, keyLen: number): Promise<Uint8Array>;
  /** 读取信封头元信息（不做密码校验）。 */
  parseHeader(blob: Uint8Array): Promise<EnvelopeHeader>;
  /** 信封常量（从 wasm 读，前端不硬编码）。 */
  constants(): Promise<EnvelopeConstants>;
  /** 释放 Worker / 资源。 */
  dispose(): void;
}

/** fetch 一个二进制资源（浏览器、思源、VuePress 通用）。 */
export async function fetchBinary(url: string): Promise<ArrayBuffer> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`加载 ${url} 失败：HTTP ${res.status}`);
  return res.arrayBuffer();
}

/** fetch 一个文本资源。 */
export async function fetchText(url: string): Promise<string> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`加载 ${url} 失败：HTTP ${res.status}`);
  return res.text();
}

/** 拼接 URL，容忍 base 末尾有无斜杠。 */
export function joinUrl(base: string, name: string): string {
  return `${base.replace(/\/$/, "")}/${name}`;
}

/**
 * 创建客户端。
 *
 * 两种运行模式共用同一套 API 与同一份 WASM：
 *  - worker：加解密在后台线程，主线程不卡（大文件推荐）；
 *  - main：没有 Worker 可用时（如 Obsidian 插件环境）退而求其次，
 *    PBKDF2 10000 迭代处理一条笔记约 10ms 级，UI 无感。
 */
export async function createCryptoClient(options: CryptoClientOptions): Promise<CryptoClient> {
  const transport = options.createWorker ? await createWorkerTransport(options) : await createMainTransport(options);

  let nextId = 1;
  const send = async (body: WorkerRequestBody): Promise<WorkerResponse> => transport.send({ id: nextId++, ...body } as WorkerRequest);

  const bytes = async (body: WorkerRequestBody): Promise<Uint8Array> => {
    const res = await send(body);
    if (!res.ok) throw new Error(res.error);
    return new Uint8Array(res.bytes ?? new ArrayBuffer(0));
  };
  const value = async <T>(body: WorkerRequestBody): Promise<T> => {
    const res = await send(body);
    if (!res.ok) throw new Error(res.error);
    return res.value as T;
  };

  return {
    mode: transport.mode,
    encrypt: (plain, password, opts) => bytes({ type: "encrypt", data: toBuffer(plain), password, options: opts }),
    decrypt: (blob, password) => bytes({ type: "decrypt", data: toBuffer(blob), password }),
    encryptText: (text, password, opts) => bytes({ type: "encryptText", text, password, options: opts }),
    decryptText: async (blob, password) => {
      const res = await send({ type: "decryptText", data: toBuffer(blob), password });
      if (!res.ok) throw new Error(res.error);
      return res.text ?? "";
    },
    encryptTextToBase64: async (text, password, opts) => toBase64(await bytes({ type: "encryptText", text, password, options: opts })),
    decryptTextFromBase64: async (text, password) => {
      const res = await send({ type: "decryptText", data: toBuffer(fromBase64(text)), password });
      if (!res.ok) throw new Error(res.error);
      return res.text ?? "";
    },
    verifyPassword: (blob, password) => value<boolean>({ type: "verifyPassword", data: toBuffer(blob), password }),
    deriveKey: (password, salt, keyLen) => bytes({ type: "deriveKey", password, salt: toBuffer(salt), keyLen }),
    parseHeader: blob => value<EnvelopeHeader>({ type: "parseHeader", data: toBuffer(blob) }),
    constants: () => value<EnvelopeConstants>({ type: "constants" }),
    dispose: () => transport.dispose(),
  };
}

/** 从 URL 创建客户端（浏览器 / 思源 / VuePress 的快捷方式）。 */
export function createCryptoClientFromUrls(options: {
  wasmUrl: string;
  execUrl: string;
  createWorker?: () => Worker;
  timeoutMs?: number;
}): Promise<CryptoClient> {
  return createCryptoClient({
    loadWasm: () => fetchBinary(options.wasmUrl),
    loadExecSource: () => fetchText(options.execUrl),
    createWorker: options.createWorker,
    timeoutMs: options.timeoutMs,
  });
}

// ────────────────────────────── 内部实现 ──────────────────────────────

interface Transport {
  readonly mode: CryptoRuntimeMode;
  send(req: WorkerRequest): Promise<WorkerResponse>;
  dispose(): void;
}

/** 拷出独立 ArrayBuffer：Uint8Array 可能是 Go 内存的视图，跨 postMessage 必须先复制。 */
function toBuffer(view: Uint8Array): ArrayBuffer {
  const buffer = new ArrayBuffer(view.byteLength);
  new Uint8Array(buffer).set(view);
  return buffer;
}

async function createMainTransport(options: CryptoClientOptions): Promise<Transport> {
  const [wasm, execSource] = await Promise.all([options.loadWasm(), options.loadExecSource()]);
  const api = await loadEncvWasm({ wasm, execSource });

  return {
    mode: "main",
    dispose: () => {},
    async send(req) {
      switch (req.type) {
        case "init":
          return { id: req.id, ok: true };
        case "encrypt":
          return okBytes(req.id, api.encrypt(new Uint8Array(req.data), req.password, req.options));
        case "encryptWithEntropy":
          return okBytes(
            req.id,
            api.encryptWithEntropy(new Uint8Array(req.data), req.password, req.options, new Uint8Array(req.salt), new Uint8Array(req.iv))
          );
        case "decrypt":
          return okBytes(req.id, api.decrypt(new Uint8Array(req.data), req.password));
        case "encryptText":
          return okBytes(req.id, api.encryptText(req.text, req.password, req.options));
        case "decryptText":
          return { id: req.id, ok: true, text: api.decryptText(new Uint8Array(req.data), req.password) };
        case "verifyPassword":
          return { id: req.id, ok: true, value: api.verifyPassword(new Uint8Array(req.data), req.password) };
        case "deriveKey":
          return okBytes(req.id, api.deriveKey(req.password, new Uint8Array(req.salt), req.keyLen));
        case "parseHeader":
          return { id: req.id, ok: true, value: api.parseHeader(new Uint8Array(req.data)) };
        case "constants":
          return { id: req.id, ok: true, value: api.constants() };
      }
    },
  };
}

async function createWorkerTransport(options: CryptoClientOptions): Promise<Transport> {
  const timeoutMs = options.timeoutMs ?? 60_000;
  const createWorker = options.createWorker;
  if (!createWorker) throw new Error("createWorker 缺失");

  const [wasm, execSource] = await Promise.all([options.loadWasm(), options.loadExecSource()]);
  const worker = createWorker();

  const pending = new Map<
    number,
    { resolve: (res: WorkerResponse) => void; reject: (err: Error) => void; timer: ReturnType<typeof setTimeout> }
  >();

  worker.onmessage = (event: MessageEvent<WorkerResponse>) => {
    const res = event.data;
    const entry = pending.get(res.id);
    if (!entry) return;
    clearTimeout(entry.timer);
    pending.delete(res.id);
    entry.resolve(res);
  };

  worker.onerror = (event: ErrorEvent) => {
    const err = new Error(`worker 错误：${event.message}`);
    for (const entry of pending.values()) {
      clearTimeout(entry.timer);
      entry.reject(err);
    }
    pending.clear();
  };

  const send = (req: WorkerRequest): Promise<WorkerResponse> =>
    new Promise<WorkerResponse>((resolve, reject) => {
      const timer = setTimeout(() => {
        pending.delete(req.id);
        reject(new Error(`调用 ${req.type} 超时（${timeoutMs}ms）`));
      }, timeoutMs);
      pending.set(req.id, { resolve, reject, timer });
      const transfer = req.type === "init" ? [req.wasm] : [];
      worker.postMessage(req, transfer);
    });

  const initRes = await send({
    id: 0,
    type: "init",
    protocolVersion: PROTOCOL_VERSION,
    execSource,
    wasm: wasm instanceof Uint8Array ? toBuffer(wasm) : wasm,
  });
  if (!initRes.ok) throw new Error(`worker 初始化失败：${initRes.error}`);

  return {
    mode: "worker",
    send,
    dispose: () => {
      for (const entry of pending.values()) clearTimeout(entry.timer);
      pending.clear();
      worker.terminate();
    },
  };
}

function okBytes(id: number, bytes: Uint8Array): WorkerResponse {
  return { id, ok: true, bytes: toBuffer(bytes) };
}

export { utf8Decode, utf8Encode };
