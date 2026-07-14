import { apiRequest } from "./core/request";

// encv_webdav.ts - 拆分自 encv.ts

export interface WebDAVConfig {
  id: string;
  name: string;
  url: string;
  username: string;
  password: string;
  mountPath: string;
  isBuiltIn?: boolean;
}

export interface RemoteWebDAVInfo {
  enabled: boolean;
  url: string;
  username: string;
  root: string;
}

export interface OpenlistSiteInfo {
  host: string;
  description: string;
  proxyUrl: string;
  isBuiltIn?: boolean;
}

export interface RemoteInfo {
  webdav: RemoteWebDAVInfo;
  openlistSites: Record<string, OpenlistSiteInfo>;
}

export type LocalOpenListState = "not_installed" | "port_conflict" | "running" | "stopped";

export interface LocalOpenListStatus {
  state: LocalOpenListState;
  running: boolean;
  pid: number;
  port: number;
  dataDirSize: number;
  lastHeartbeat: number;
  error?: string;
}

export async function fetchLocalOpenListStatus(): Promise<LocalOpenListStatus> {
  console.debug("[API] fetchLocalOpenListStatus");
  return apiRequest<LocalOpenListStatus>("/openlist/local/status");
}

const WEBDAV_CONFIGS_KEY = "encv-webdav-configs";

export function getWebDAVConfigs(): WebDAVConfig[] {
  const stored = localStorage.getItem(WEBDAV_CONFIGS_KEY);
  return stored ? JSON.parse(stored) : [];
}

export function saveWebDAVConfigs(configs: WebDAVConfig[]) {
  localStorage.setItem(WEBDAV_CONFIGS_KEY, JSON.stringify(configs));
}

export interface LocalWebDAVTestResult {
  available: boolean;
  url?: string;
  authRequired?: boolean;
  details?: {
    propfindRoot: string;
    authWorks: string;
    dirReadable: string;
  };
  error?: string;
}

export async function testLocalWebDAV(): Promise<LocalWebDAVTestResult> {
  return apiRequest<LocalWebDAVTestResult>("/api/webdav/test-local");
}

export interface WebDAVTestResult {
  success: boolean;
  reachable: boolean;
  is_webdav: boolean;
  auth_ok: boolean;
  dir_readable: boolean;
  status_code: number;
  dav_header?: string;
  error?: string;
}

export async function testWebDAVConnection(config: Omit<WebDAVConfig, "id">): Promise<WebDAVTestResult> {
  console.info("[API] testWebDAV");
  return apiRequest<WebDAVTestResult>("/api/webdav/test", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
}
