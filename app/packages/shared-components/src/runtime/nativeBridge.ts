/**
 * nativeBridge — 应用层 → 共享层的「原生桥接能力」注入 DI 注册点
 *
 * 背景：shared 作为共享层，不得反向依赖应用层（@/...）内部实现，尤其是
 * Capacitor 原生插件 @/plugins/GoProcess（backend 启停 / WorkManager / OpenList
 * 广播 / Android deps 等）。但部分通用模块（任务取消双写、server 状态探测、
 * OpenList 桥接、依赖清单）运行时需要这些原生能力。
 *
 * 约定（镜像 appCapabilities / appNavigation 范式）：
 *   - shared 内部一律通过 getNativeBridge() 取用这些能力，绝不 import @/plugins/GoProcess。
 *   - app 在启动期（stores/registerSharedNativeBridge）调用 setNativeBridge(...) 注入具体实现。
 *   - 未注入时：isNative 默认 false（web SPA，安全）；其余原生专属函数抛清晰错误，
 *     便于在 standalone / 测试环境尽早暴露「忘记注入」的问题。
 *   - 调用方均先用 isNative() 守卫（web 永不调用原生专属函数），故默认抛错不会被误触发。
 */

// ─── 类型（与 @/plugins/GoProcess 保持结构兼容，shared 内自包含定义避免反向依赖）───

export interface GoProcessResult {
  success: boolean;
  lastError?: string;
}

export interface GoProcessStatus {
  running: boolean;
  port: number;
  lastError?: string;
}

export interface AndroidDepsManifest {
  schema_version?: number;
  generated_at?: string;
  items: Array<{
    name: string;
    version: string;
    version_range?: string;
    source?: string;
    kind?: string;
    importance?: string;
    description?: string;
  }>;
}

export interface OpenListRuntime {
  isInstalled: boolean;
  running: boolean;
  port: number;
  pid: number;
  dataSizeBytes: number;
  lastError: string;
  lastUpdateTs: number;
}

export type OpenListStatusListener = (status: OpenListRuntime) => void;

export interface OpenListListenerHandle {
  remove: () => Promise<void>;
}

export interface NativeBridge {
  /** 是否运行在原生（Capacitor/APK）环境。默认 false（web SPA）。 */
  isNative: () => boolean;
  /** 入队取消任务的 WorkManager 持久化请求（native only）。 */
  enqueueCancelWorker: (taskId: string) => Promise<{ success: boolean }>;
  /** 重启后端（native only）。 */
  restartBackend: () => Promise<GoProcessResult>;
  /** 停止后端（native only）。 */
  stopBackend: () => Promise<GoProcessResult>;
  /** 查询后端运行状态（native only）。 */
  getBackendStatus: () => Promise<GoProcessStatus>;
  /** 读取 android-deps.json manifest（native only，web 返回 null）。 */
  getAndroidDeps: () => Promise<AndroidDepsManifest | null>;
  /** 订阅 OpenList 状态广播（native only）。 */
  addOpenListStatusListener: (callback: OpenListStatusListener) => Promise<OpenListListenerHandle>;
  /** 一次性快照 OpenList 运行时状态（native only）。 */
  getOpenListRuntime: () => Promise<OpenListRuntime>;
}

const notInjected = (name: string): never => {
  throw new Error(`[nativeBridge] ${name} 未注入（需在 app 启动期调用 setNativeBridge）`);
};

const defaults: NativeBridge = {
  isNative: () => false,
  enqueueCancelWorker: () => notInjected("enqueueCancelWorker"),
  restartBackend: () => notInjected("restartBackend"),
  stopBackend: () => notInjected("stopBackend"),
  getBackendStatus: () => notInjected("getBackendStatus"),
  getAndroidDeps: () => notInjected("getAndroidDeps"),
  addOpenListStatusListener: () => notInjected("addOpenListStatusListener"),
  getOpenListRuntime: () => notInjected("getOpenListRuntime"),
};

let bridge: NativeBridge = { ...defaults };

/** app 启动期调用：注入 / 覆盖原生桥接能力。可多次部分覆盖。 */
export function setNativeBridge(partial: Partial<NativeBridge>): void {
  bridge = { ...bridge, ...partial };
}

/** shared 内部取用原生桥接能力。 */
export function getNativeBridge(): NativeBridge {
  return bridge;
}

// ─── 顶层委托函数（与 @/plugins/GoProcess 形态一致，便于消费方直接 import）───
// 全部经 getNativeBridge() 转发到 app 注入的实现；未注入时 isNative 默认 false，
// 其余原生专属函数抛清晰错误（调用方均先 isNative() 守卫，web 不会触发）。

export function isNative(): boolean {
  return bridge.isNative();
}

export function enqueueCancelWorker(taskId: string): Promise<{ success: boolean }> {
  return bridge.enqueueCancelWorker(taskId);
}

export function restartBackend(): Promise<GoProcessResult> {
  return bridge.restartBackend();
}

export function stopBackend(): Promise<GoProcessResult> {
  return bridge.stopBackend();
}

export function getBackendStatus(): Promise<GoProcessStatus> {
  return bridge.getBackendStatus();
}

export function getAndroidDeps(): Promise<AndroidDepsManifest | null> {
  return bridge.getAndroidDeps();
}

export function addOpenListStatusListener(callback: OpenListStatusListener): Promise<OpenListListenerHandle> {
  return bridge.addOpenListStatusListener(callback);
}

export function getOpenListRuntime(): Promise<OpenListRuntime> {
  return bridge.getOpenListRuntime();
}
