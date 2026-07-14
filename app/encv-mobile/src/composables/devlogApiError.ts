/**
 * devlogApiError - 把 agent 加解密错误结构化推送到 DevLogs 面板
 *
 * 为什么不只 console.error？
 *   - APK 上 console.error 输出到 logcat，普通用户看不到
 *   - useFrontendLogs.hijackConsole() 会把 console.* 镜像到 DevLogs
 *     面板（专门给"用户能看到的开发者日志"），但 console.error 默认红字，
 *     大量非致命 fallback（如 key 解析失败）会刷屏，淹没真正的网络错误
 *
 * 这个 helper 的设计目标：
 *   1. 写完整的 context（base URL / deviceId / status / body）方便排错
 *   2. 用 console.error 等级（红色）让用户一眼看到"真实失败"
 *   3. 拼成单行 JSON-like 字符串，DevLogs 表格不会被多行 object 撑爆
 *
 * 替代方案：直接 emit 到 useFrontendLogs 内部 logs[]
 *   - 内部 logs 是 ref，不直接暴露 push 方法
 *   - hijackConsole() 已经把 console.error 镜像过去了，重复 push 会双倍
 *   - 所以走 console 路线 + 在 helper 里加结构化前缀
 */

import { getAgentApiBaseContext } from "@encv/shared-components/composables/useAgentApiBase";

export type ApiErrorKind =
  | "encrypt" // POST /api/encrypt-key
  | "decrypt" // POST /api/decrypt-key
  | "fetch-models" // GET  /api/models
  | "test" // GET  /test
  | "roundtrip" // 本地合成：encrypt → decrypt 验证一致性
  | "sync-doctor" // GET  /api/sync/doctor
  | "sync-doctor-copy" // 用户点击 "复制诊断 JSON" 失败
  | "lan-access" // GET  /api/network/lan-access
  | "fork" // POST /api/agent/fork
  | "replay" // POST /api/agent/replay
  | "skills" // GET  /api/skills
  | "unknown";

export interface ApiErrorContext {
  kind: ApiErrorKind;
  endpoint: string; // 如 '/api/encrypt-key'
  status?: number; // HTTP status
  body?: string; // response body 文本
  deviceId?: string; // 设备指纹
  extra?: Record<string, unknown>;
}

/**
 * 推一条 agent API 错误到 console（自动镜像到 DevLogs）
 *
 * 字符串格式：固定前缀 [AGENT-API] 便于 DevLogs 搜索过滤
 */
export function devlogApiError(err: unknown, ctx: ApiErrorContext): void {
  const base = getAgentApiBaseContext();
  const errMsg = err instanceof Error ? err.message : String(err);
  const payload = {
    kind: ctx.kind,
    endpoint: ctx.endpoint,
    base: base.base,
    baseSource: base.source,
    isNative: base.isNative,
    env: base.env,
    deviceId: ctx.deviceId ? `${ctx.deviceId.slice(0, 8)}…` : "n/a",
    status: ctx.status ?? "n/a",
    body: ctx.body ? ctx.body.slice(0, 200) : "n/a",
    err: errMsg.slice(0, 200),
    ...ctx.extra,
  };
  // 拼成单行 JSON：DevLogs 表格 / logcat 都更易解析
  console.error(
    `[AGENT-API] ${ctx.kind} failed → ${ctx.endpoint}\n` +
      `  base=${base.base} (${base.source})\n` +
      `  status=${payload.status} device=${payload.deviceId} env=${base.env} native=${base.isNative}\n` +
      `  err=${payload.err}\n` +
      `  body=${payload.body}` +
      (ctx.extra ? `\n  extra=${JSON.stringify(ctx.extra)}` : "")
  );
}

/**
 * 推一条 info 级日志（如 round-trip OK）
 */
export function devlogApiInfo(msg: string, ctx?: Partial<ApiErrorContext>): void {
  const base = getAgentApiBaseContext();
  const tag = ctx?.kind ? `[AGENT-API] ${ctx.kind}` : "[AGENT-API]";
  console.info(`${tag} ${msg}\n  base=${base.base} (${base.source})${ctx?.status ? ` status=${ctx.status}` : ""}`);
}
