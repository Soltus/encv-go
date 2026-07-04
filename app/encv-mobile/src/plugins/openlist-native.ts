/**
 * OpenList Native 桥接（主 app 端）
 *
 * 在主 app Capacitor runtime 中调用 plugin-openlist 暴露的 OpenListNative。
 * 当主 app 页面中需要直接调 OpenList 后端时使用。
 */
import type { OpenListRuntime } from "@/components-shared";

declare global {
  interface Window {
    OpenListNative?: {
      startOpenList(): string;
      stopOpenList(): boolean;
      getRuntimeStatus(): string;
      setAdminPassword(password: string): boolean;
      readConfig(): string;
      writeConfig(content: string): boolean;
      getVersion(): string;
      getDataDir(): string;
      getPort(): number;
      getIsRunning(): boolean;
    };
  }
}

function safe<T>(fallback: T, fn: () => T): T {
  try {
    return fn();
  } catch {
    return fallback;
  }
}

export const OpenListNative = {
  startOpenList(): number {
    return safe(0, () => parseInt(window.OpenListNative?.startOpenList() ?? "0", 10));
  },
  stopOpenList(): boolean {
    return safe(false, () => window.OpenListNative?.stopOpenList() ?? false);
  },
  getStatus(): OpenListRuntime {
    const empty: OpenListRuntime = {
      running: false,
      port: 0,
      pid: 0,
      dataSizeBytes: 0,
      lastError: "",
      lastUpdateTs: 0,
      dataDir: "",
      isInstalled: true,
    };
    return safe(empty, () => JSON.parse(window.OpenListNative?.getRuntimeStatus() ?? "{}"));
  },
  setPassword(password: string): boolean {
    return safe(false, () => window.OpenListNative?.setAdminPassword(password) ?? false);
  },
  readConfig(): string {
    return safe("{}", () => window.OpenListNative?.readConfig() ?? "{}");
  },
  writeConfig(content: string): boolean {
    return safe(false, () => window.OpenListNative?.writeConfig(content) ?? false);
  },
  getVersion(): string {
    return safe("unknown", () => window.OpenListNative?.getVersion() ?? "unknown");
  },
  getDataDir(): string {
    return safe("", () => window.OpenListNative?.getDataDir() ?? "");
  },
  getPort(): number {
    return safe(0, () => window.OpenListNative?.getPort() ?? 0);
  },
  getIsRunning(): boolean {
    return safe(false, () => window.OpenListNative?.getIsRunning() ?? false);
  },
};
