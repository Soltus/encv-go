import { getApiBaseUrl, SERVER_INSTANCE_ID_KEY, SERVER_VERSION_KEY } from "./encv_core";
import { apiRequest } from "./core/request";
import { ApiError, isApiStatusAtLeast } from "./core/errors";
import { getFileExtension } from "./encv_files";

// encv_admin.ts - 拆分自 encv.ts

export interface BackendPermissions {
  storage: boolean;
}

export async function checkBackendPermissions(): Promise<BackendPermissions> {
  try {
    const result = await apiRequest<BackendPermissions>("/api/permissions");
    console.info("[API] permissions:", JSON.stringify(result));
    return result;
  } catch {
    console.debug("[API] checkPermissions failed");
    return { storage: false };
  }
}

export async function checkServerStatus(): Promise<{
  online: boolean;
  error?: string;
  instanceId?: string;
  version?: string;
  /** 🆕 2026-06-15：backend instance_id 跨会话变化（后端真崩重启场景，不是劫持）。
   *  仍然 online=true（ping 200 + JSON + 合法 instance_id）；
   *  上层 useServerStatus emit 'backend:instance-changed' 给 UI banner 提示。 */
  instanceChanged?: { previous: string; current: string };
}> {
  try {
    // 🆕 2026-06-15：复用桌面后端 performPingCheck 的 InstanceID 防劫持机制。
    //
    // 历史：老代码只校验 Content-Type: application/json，但任何返 JSON 200 的进程
    //   都能骗过——mock / 旧 encv-go / 上游代理 / 编译时拼出来的"伪 backend" 都会
    //   被误判为 online。然后 token / 文件路径 / 任务数据就泄露给错误进程。
    //
    // 新逻辑（对齐 internal/register/server_start.go:89 performPingCheck）：
    //   1. 调 /ping（无副作用的探测端点，后端返 PingResponse{status, version, instance_id, server_dir, webdav_dir}）
    //   2. status code == 200 + content-type: application/json
    //   3. **decode** 响应体成 PingResponse（不能只看 status == "ok"）
    //   4. **instance_id 必填**：为空或缺字段 → 不是 encv-go → 报 hijacked
    //   5. **跨会话比对**：localStorage 缓存的 instance_id 不一致 → "进程被替换" → 报 hijacked
    //   6. 通过 → 持久化新的 instance_id + version 覆盖旧值（upgrade 场景：version 变了清缓存）
    //
    // 为什么不用 /api/config：
    //   - /api/config 返空 JSON 也能 200，老逻辑下无法判断 backend 是不是 encv-go
    //   - /ping 必有 instance_id，是 encv-go 唯一契约
    const baseUrl = getApiBaseUrl();
    const response = await fetch(`${baseUrl}/ping`, {
      // 强制不要缓存——instance_id 是 freshness 敏感的，HTTP 缓存可能让上一进程的 instance_id
      // 错认给当前进程
      cache: "no-store",
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      return { online: false, error: `HTTP ${response.status}` };
    }
    const contentType = response.headers.get("content-type") || "";
    if (!contentType.includes("application/json")) {
      // Vite SPA fallback / 任何返回 text/html 的"假 backend"——老逻辑的 case 仍要拦截
      console.warn("[API] server probe returned non-JSON, treating as offline");
      return { online: false, error: `ping returned ${contentType || "unknown"} (likely vite SPA fallback or wrong port)` };
    }
    const ping = await parsePingResponse(response);
    if (!ping) {
      return { online: false, error: "ping response missing required fields (instance_id / status / version)" };
    }
    if (ping.status !== "ok") {
      return { online: false, error: `ping status=${ping.status}` };
    }
    // 🆕 2026-06-15 修复 #1（死锁根因）：
    //
    // 历史 bug：旧逻辑 "比对失败就 return online:false 且不 persist"——后端真的崩重启后，
    //   新 instance_id 跟 localStorage 老 ID 不一致 → 报 online:false + error: 'instance changed'
    //   → 永远不 persist 新 ID → 下次探测还是不一致 → 死循环 offline
    //
    // 修复（顺序关键）：
    //   1. **先 persist 新 instance_id**（不管一不一样都 persist——重启用）
    //   2. 再比对——不一致时**仍然 return online:true**（ping 实质是 200 + JSON + 合法 instance_id）
    //   3. hijack 警告通过专用 `instanceChanged: {previous, current}` 字段返回，不靠 error
    //   4. 上层 useServerStatus 收到 instanceChanged 字段 → emit instanceChanged 事件
    //      → UI 顶部 banner 提示（不阻塞状态机 / 不进 lastError）
    const previousId = readPersistedInstanceId();
    persistBackendIdentity(ping.instance_id, ping.version); // ① 永远先 persist
    let instanceChanged: { previous: string; current: string } | undefined;
    if (previousId && previousId !== ping.instance_id) {
      // ② 不一致 → 是 backend 真的崩重启（不是劫持；劫持场景下 ping 不会 200+JSON+合法 instance_id）
      console.warn("[API] backend instance_id changed (likely backend restart, not hijack)", {
        previous: previousId,
        current: ping.instance_id,
      });
      instanceChanged = { previous: previousId, current: ping.instance_id };
      // ③ 仍然 return online=true + 仅在 instanceChanged 字段带警告
      return {
        online: true,
        instanceId: ping.instance_id,
        version: ping.version,
        instanceChanged, // 上层用此发事件，UI 顶部 banner 提示
      };
    }
    console.info("[API] server online (ping OK)", { instanceId: shortHash(ping.instance_id), version: ping.version });
    return { online: true, instanceId: ping.instance_id, version: ping.version };
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    console.debug("[API] server offline:", msg);
    return { online: false, error: msg };
  }
}

/** PingResponse 子集（snake_case 与后端 internal/v2/types/types.go PingResponse 一致） */
interface PingResponse {
  status: string;
  version: string;
  instance_id: string;
  server_dir?: string;
  webdav_dir?: string;
}

/**
 * 安全 decode PingResponse。返回 null 表示"非 encv-go 响应"（字段缺失/类型错）。
 *
 * 不 throw：让调用方走 {online:false, error:...} 路径而不是 exception 路径。
 */
async function parsePingResponse(response: Response): Promise<PingResponse | null> {
  try {
    const obj = await response.json();
    if (!obj || typeof obj !== "object") return null;
    const status = typeof obj.status === "string" ? obj.status : "";
    const version = typeof obj.version === "string" ? obj.version : "";
    const instanceId = typeof obj.instance_id === "string" ? obj.instance_id : "";
    if (!status || !instanceId) {
      // 关键字段缺失 → 不是 encv-go 响应
      return null;
    }
    return {
      status,
      version,
      instance_id: instanceId,
      server_dir: typeof obj.server_dir === "string" ? obj.server_dir : undefined,
      webdav_dir: typeof obj.webdav_dir === "string" ? obj.webdav_dir : undefined,
    };
  } catch {
    return null;
  }
}

function readPersistedInstanceId(): string | null {
  if (typeof localStorage === "undefined") return null;
  return localStorage.getItem(SERVER_INSTANCE_ID_KEY);
}

function persistBackendIdentity(instanceId: string, version: string) {
  if (typeof localStorage === "undefined") return;
  localStorage.setItem(SERVER_INSTANCE_ID_KEY, instanceId);
  if (version) localStorage.setItem(SERVER_VERSION_KEY, version);
}

/** 短 hash 显示（前 8 字符）——给 UI 展示用，避免完整 UUID 占太多视觉空间 */
function shortHash(id: string): string {
  if (!id) return "(empty)";
  return id.length > 8 ? id.slice(0, 8) : id;
}

/** UI 读取持久化的 backend instance_id（用于"上次连接的是哪个进程"展示） */

export interface ServiceGuardResult {
  ready: boolean;
  servingDir: string;
  expected: string;
  envDevPreview?: boolean;
  envMobile?: boolean;
  detail?: string;
  remediation?: Array<{ scenario: string; command?: string; steps?: string[]; explain?: string }>;
  error?: string;
}

export async function checkServiceGuard(): Promise<ServiceGuardResult> {
  console.info("[API] checkServiceGuard");
  const baseUrl = getApiBaseUrl();
  const response = await fetch(`${baseUrl}/api/service-guard`);
  const data: ServiceGuardResult = await response.json();

  if (!data.ready) {
    const err = new Error(`ServiceGuard: ${data.detail}`) as Error & { code: string; payload: ServiceGuardResult };
    err.code = "SERVICE_GUARD_BLOCKED";
    err.payload = data;
    console.error("[API] checkServiceGuard BLOCKED —", data.detail);
    throw err;
  }

  console.info("[API] checkServiceGuard OK — servingDir:", data.servingDir);
  return data;
}

export interface BackendLogEntry {
  level: "debug" | "info" | "warn" | "error";
  message: string;
  timestamp: string; // HH:MM:SS
}

export interface RecentLogsResponse {
  logs: BackendLogEntry[];
  count: number;
  capacity: number;
}

export async function getRecentBackendLogs(since?: string): Promise<RecentLogsResponse> {
  const url = since ? `/api/logs/recent?since=${encodeURIComponent(since)}` : "/api/logs/recent";
  return apiRequest<RecentLogsResponse>(url);
}

export interface TextPreviewExts {
  extensions: string[];
  custom_extensions: string[];
}

let cachedTextExts: Set<string> | null = null;

export async function fetchTextPreviewExts(): Promise<Set<string>> {
  if (cachedTextExts) return cachedTextExts;
  try {
    const data = await apiRequest<TextPreviewExts>("/api/file/text-preview-exts", { timeoutMs: 5000 });
    const all = new Set([...(data.extensions || []), ...(data.custom_extensions || [])]);
    cachedTextExts = all;
    return all;
  } catch (err) {
    console.error("[API] fetchTextPreviewExts error:", err);
    return new Set();
  }
}

export function isTextPreviewable(name: string): boolean {
  if (!cachedTextExts) return false;
  const ext = getFileExtension(name);
  return cachedTextExts.has(ext);
}

export function invalidateTextExtsCache(): void {
  cachedTextExts = null;
}

export async function fetchConfig(): Promise<Record<string, unknown>> {
  // apiRequest 在 2xx 返回 HTML（vite SPA fallback）时抛 ApiError，避免上游 JSON.parse 报 "Unexpected token '<'"
  return apiRequest<Record<string, unknown>>("/api/config");
}

export async function updateConfig(config: Record<string, unknown>): Promise<{ message: string; needsRestart?: boolean }> {
  try {
    return await apiRequest<{ message: string; needsRestart?: boolean }>("/api/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(config),
    });
  } catch (e) {
    // 4xx/5xx 真实错误照抛；仅成功但响应体非 JSON 时回退默认消息（兼容旧后端）
    if (isApiStatusAtLeast(e, 400)) throw e;
    return { message: "config updated" };
  }
}

export async function fetchConfigSchema(): Promise<Record<string, unknown>> {
  return apiRequest<Record<string, unknown>>("/api/config/schema");
}
