import type { FileItem } from "@/api/encv";
import { decodeAlistFilename, getAlistEncryptStreamUrl } from "@/api/encv";
import { getFieldValue } from "@/composables/useConfig";

const MAX_CACHE_SIZE = 500;
const sessionPasswords = new Map<string, string>();
const decodedNames = new Map<string, string>();
const sessionKeys: string[] = [];
const decodedKeys: string[] = [];

function lruPush(keys: string[], key: string, map: Map<string, unknown>, value: unknown) {
  if (map.has(key)) {
    const i = keys.indexOf(key);
    if (i > -1) keys.splice(i, 1);
    keys.push(key);
    return;
  }
  if (keys.length >= MAX_CACHE_SIZE) {
    const oldest = keys.shift();
    if (oldest !== undefined) map.delete(oldest);
  }
  keys.push(key);
  map.set(key, value);
}

export function isAlistEncrypted(file: FileItem): boolean {
  if (file.isDirectory) return false;
  const suffix = getFieldValue(["plugin_settings", "alist_encrypt", "suffix"]) as string;
  return !!suffix && file.name.endsWith(suffix);
}

export function getSessionPassword(path: string): string | undefined {
  return sessionPasswords.get(path);
}

export function setSessionPassword(path: string, password: string): void {
  lruPush(sessionKeys, path, sessionPasswords, password);
}

export function getDecodedName(path: string): string | undefined {
  return decodedNames.get(path);
}

function getEncType(): string {
  return (getFieldValue(["plugin_settings", "alist_encrypt", "enc_type"]) as string) || "aesctr";
}

export async function loadDecodedName(file: FileItem, password = ""): Promise<string | null> {
  if (!isAlistEncrypted(file)) return null;
  const cached = decodedNames.get(file.path);
  if (cached) return cached;

  const baseName = file.name.replace(/\.bin$/i, "");
  try {
    const result = await decodeAlistFilename({ encodedName: baseName, password, encType: getEncType() });
    if (result.success && result.plain_name) {
      lruPush(decodedKeys, file.path, decodedNames, result.plain_name);
      return result.plain_name;
    }
  } catch {
    // API call failed - return null to show original encrypted name
  }
  return null;
}

export function getStreamUrl(file: FileItem, password = ""): string {
  return getAlistEncryptStreamUrl({ path: file.path, password });
}

export function clearPasswordCache(): void {
  sessionPasswords.clear();
  sessionKeys.length = 0;
}

export function clearDecodeCache(): void {
  decodedNames.clear();
  decodedKeys.length = 0;
}
