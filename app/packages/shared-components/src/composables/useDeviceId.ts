/**
 * useDeviceId - 获取稳定的设备唯一标识
 *
 * 优先级：
 *   1. @capacitor/device 的 getId()（原生平台，跨安装稳定）
 *   2. @capacitor/device 的 getInfo()（fallback）
 *   3. Web 端 fallback：crypto.randomUUID() 持久化到 localStorage
 *
 * 用途：API Key 加密加盐、设备绑定配置等安全场景
 */

const DEVICE_ID_KEY = "encv-device-id";
let _cachedId: string | null = null;

/**
 * 获取设备 ID（带内存 + localStorage 缓存）
 */
export async function getDeviceId(): Promise<string> {
  // 内存缓存命中
  if (_cachedId) return _cachedId;

  // localStorage 缓存命中
  try {
    const stored = localStorage.getItem(DEVICE_ID_KEY);
    if (stored && typeof stored === "string" && stored.length > 8) {
      _cachedId = stored;
      return stored;
    }
  } catch {
    // ignore
  }

  // 尝试 Capacitor 原生 API
  const id = await resolveNativeId();
  _cachedId = id;

  // 持久化到 localStorage
  try {
    localStorage.setItem(DEVICE_ID_KEY, id);
  } catch {
    // ignore
  }

  return id;
}

/** 同步版本（仅当已缓存过时有效，否则返回空串） */
export function getDeviceIdSync(): string {
  if (_cachedId) return _cachedId;
  try {
    const stored = localStorage.getItem(DEVICE_ID_KEY);
    if (stored) {
      _cachedId = stored;
      return stored;
    }
  } catch {
    /* ignore */
  }
  return "";
}

async function resolveNativeId(): Promise<string> {
  // 尝试 @capacitor/device getId()
  try {
    const { Device } = await import("@capacitor/device");
    const info = await Device.getId();
    if (info?.identifier && typeof info.identifier === "string" && info.identifier.length > 4) {
      return `native:${info.identifier}`;
    }
  } catch {
    // @capacitor/device 不可用（Web 端或未安装）
  }

  // Fallback: getInfo() 拼接
  try {
    const { Device } = await import("@capacitor/device");
    const info = await Device.getInfo();
    const parts = [info.model || "", info.platform || "", String(info.osVersion || ""), info.manufacturer || ""];
    const raw = parts.join("|");
    if (raw.length > 4) {
      let hash = 0;
      for (let i = 0; i < raw.length; i++) {
        hash = (hash << 5) - hash + raw.charCodeAt(i);
        hash |= 0;
      }
      return `web:${Math.abs(hash).toString(16)}`;
    }
  } catch {
    // 完全不可用
  }

  // 最终 fallback: crypto.randomUUID
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `web:${crypto.randomUUID()}`;
  }
  return `web:${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}

/**
 * 清除缓存的设备 ID（测试用 / 设备迁移场景）
 */
export function clearDeviceIdCache(): void {
  _cachedId = null;
  try {
    localStorage.removeItem(DEVICE_ID_KEY);
  } catch {
    /* ignore */
  }
}
