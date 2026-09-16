import { getApiBaseUrl } from "./encv_core";
import { apiRequest } from "./core/request";
import type { RemoteInfo } from "./encv_webdav";

// encv_openlist.ts - 拆分自 encv.ts

export async function fetchRemoteInfo(): Promise<RemoteInfo> {
  return apiRequest<RemoteInfo>("/api/remote/info");
}

export async function addOpenlistSite(siteId: string, host: string, description: string): Promise<void> {
  await apiRequest<void>("/api/remote/openlist", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ siteId, host, description }),
  });
}

export async function updateOpenlistSite(siteId: string, host: string, description: string): Promise<void> {
  await apiRequest<void>(`/api/remote/openlist/${siteId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ host, description }),
  });
}

export async function deleteOpenlistSite(siteId: string): Promise<void> {
  await apiRequest<void>(`/api/remote/openlist/${siteId}`, { method: "DELETE" });
}

export function getAlistEncryptStreamUrl(params: { path: string; password: string }): string {
  // 注意：path 用单次 encodeURIComponent（不是 proxySafeEncode）。
  // 双重编码（proxySafeEncode）是为经过 WAF / 代理的场景，而 alist-encrypt
  // stream 端点会自行解码一次，单编码才是正确的客户端编码层次。
  if (import.meta.env.DEV) {
    return `/api/alist-encrypt/stream?path=${encodeURIComponent(params.path)}&password=${encodeURIComponent(params.password)}`;
  }
  const baseUrl = getApiBaseUrl();
  return `${baseUrl}/api/alist-encrypt/stream?path=${encodeURIComponent(params.path)}&password=${encodeURIComponent(params.password)}`;
}

export interface AlistDecodeResult {
  plain_name: string;
  success: boolean;
}

export async function decodeAlistFilename(params: { encodedName: string; password: string; encType?: string }): Promise<AlistDecodeResult> {
  return apiRequest<AlistDecodeResult>("/api/alist-encrypt/decode-filename", {
    query: {
      encoded: params.encodedName,
      password: params.password,
      ...(params.encType ? { enc_type: params.encType } : {}),
    },
  });
}
