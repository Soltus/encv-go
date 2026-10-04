/**
 * useDeviceId - 获取稳定的设备唯一标识
 *
 * 优先级：
 *   1. @capacitor/device 的 getId()（原生平台，跨安装稳定）
 *   2. @capacitor/device 的 getInfo()（fallback）
 *   3. Web 端 fallback：crypto.randomUUID() 持久化到 localStorage
 *
 * 用途：API Key 加密加盐、设备绑定配置等安全场景
 *
 * 🚨 2026-10-05（docs/persisted-state-selfhealing.md P1-1）：落盘值必须**校验后再用**。
 *   原实现 `if (stored)` 就直接用 ⇒ 一旦 localStorage 里躺着垃圾值（"null"、
 *   "[object Object]"、被别的版本写坏的残留），设备身份就被污染，而用户无从察觉
 *   （表现：配对/加密加盐用了错的 id，且重启无效，因为值不会自己变好）。
 *   现在：坏值 ⇒ 清除 + warn 留痕 + 重新解析。
 */

const DEVICE_ID_KEY = "encv-device-id";
let _cachedId: string | null = null;

/**
 * 落盘的 deviceId 是否**可用**（保守：只挡明显坏值，不误伤历史合法值）。
 *
 * 判据：长度 8..200、无空白、无 JSON/引号类污染字符。
 * 不校验前缀（native:/web: 之外历史上也可能有裸 UUID）。
 */
export function isValidDeviceId(v: unknown): v is string {
  if (typeof v !== "string") return false;
  const s = v.trim();
  if (s.length < 8 || s.length > 200) return false;
  if (/\s/.test(s)) return false;
  if (/["'{}[\]\\]/.test(s)) return false;
  return true;
}

/** 坏值清除 + 留痕（不清除的话下次启动还是它，永远不会自愈） */
function dropBadStoredDeviceId(bad: string): void {
  try {
    localStorage.removeItem(DEVICE_ID_KEY);
  } catch {
    /* ignore */
  }
  console.warn(`[deviceId] 丢弃无效的落盘设备标识（${bad.slice(0, 40)}），将重新解析`);
}

/**
 * 获取设备 ID（带内存 + localStorage 缓存）
 */
export async function getDeviceId(): Promise<string> {
  // 内存缓存命中
  if (_cachedId) return _cachedId;

  // localStorage 缓存命中（**先校验**：坏值不能被当成身份）
  try {
    const stored = localStorage.getItem(DEVICE_ID_KEY);
    if (stored) {
      if (isValidDeviceId(stored)) {
        _cachedId = stored.trim();
        return _cachedId;
      }
      dropBadStoredDeviceId(stored);
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
      if (isValidDeviceId(stored)) {
        _cachedId = stored.trim();
        return _cachedId;
      }
      dropBadStoredDeviceId(stored);
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
