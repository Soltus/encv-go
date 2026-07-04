/**
 * useAgentApiBase - Agent API 基础 URL 解析
 *
 * Agent 端点（/api/encrypt-key、/api/decrypt-key、/api/chat 等）都挂载在
 * encv-go (Go 主后端) 的根路径下。三种环境下需要不同拼装：
 *
 *   ① Vite dev (web):
 *      - Vite :8100 不做反向代理（D9 决策）
 *      - 由 preview-gateway :16666 统一接管 /agent-api/* 转发到 encv-go :2025
 *      - 路径前缀必须是 '/agent-api'（带前缀的相对路径）
 *
 *   ② Capacitor APK (production):
 *      - 没有 preview-gateway，没有 vite proxy
 *      - encv-go 在设备本地 127.0.0.1:2025 上跑（useServerStatus 启动）
 *      - 直接绝对 URL 打到 encv-go 根路径
 *
 *   ③ Web SPA 静态托管:
 *      - 同 APK：走绝对 URL（getApiBaseUrl() 默认 127.0.0.1:2025）
 *
 * 调用方约定：
 *   fetch(`${getAgentApiBase()}/api/encrypt-key`, ...)
 *   fetch(`${getAgentApiBase()}/api/decrypt-key`, ...)
 *   fetch(`${getAgentApiBase()}/api/chat`,        ...)
 *   fetch(`${getAgentApiBase()}/test`,            ...)
 *
 * 行为表（与 dev / preview-gateway / APK 三态穷举验证）：
 *   import.meta.env.DEV = true            → '/agent-api'           (vite dev, 走网关)
 *   import.meta.env.DEV = false, native   → 'http://127.0.0.1:2025' (APK, 直连)
 *   import.meta.env.DEV = false, web SPA  → 用户配置 / DEFAULT_API_BASE_URL
 */

import { DEFAULT_API_BASE_URL, getApiBaseUrl } from "@/api/encv";
import { isNative } from "@/plugins/GoProcess";

// =============================================================================
// Agent Protocol Negotiation（useAgent.send() 据此决定是否带 X-Agent-Protocol 头）
// =============================================================================
//
// AG-UI 协议协商：
//   - 'agui'  → useAgent.send() 总是发 X-Agent-Protocol: agui header
//   - 'legacy'→ 不发 header（用于回滚调试：后端按默认走 legacy 自定义 SSE）
//   - 'auto'  → 同 'agui'（默认行为：始终带 header，未来加新协议时再扩）
//
// 持久化到 localStorage('encv-agent-protocol') 便于用户从 DevTools 切回 legacy 排查。

export type AgentProtocol = "agui" | "legacy" | "auto";

const AGENT_PROTOCOL_STORAGE_KEY = "encv-agent-protocol";

function isAgentProtocol(v: string | null): v is AgentProtocol {
  return v === "agui" || v === "legacy" || v === "auto";
}

/** 获取当前协议选择（同步；带 localStorage 容错） */
export function getAgentProtocol(): AgentProtocol {
  try {
    const v = localStorage.getItem(AGENT_PROTOCOL_STORAGE_KEY);
    if (isAgentProtocol(v)) return v;
  } catch {
    // SSR / 隐私模式 fallback
  }
  return "auto";
}

/** 设置协议选择并持久化 */
export function setAgentProtocol(protocol: AgentProtocol): void {
  try {
    if (protocol === "auto") {
      // 'auto' 视作默认值，不持久化（清掉旧值，恢复「总是 agui」默认行为）
      localStorage.removeItem(AGENT_PROTOCOL_STORAGE_KEY);
    } else {
      localStorage.setItem(AGENT_PROTOCOL_STORAGE_KEY, protocol);
    }
  } catch {
    // 静默失败
  }
}

/** useAgent.send() 用：决定是否带 X-Agent-Protocol: agui header */
export function shouldSendAGUIHeader(): boolean {
  const p = getAgentProtocol();
  // 'agui' 和 'auto' 都发 header；'legacy' 模式不发（用于回滚调试）
  return p !== "legacy";
}

/**
 * 解析 Agent API 基础 URL（同步）
 *
 * 注意：本函数只读 env + localStorage + isNative()，无副作用。
 * 任何需要根据后端状态动态调整的逻辑都不应放这里。
 *
 * Phase X1 改造：native APK 模式下返回空字符串（相对路径），
 * 由 main.ts 安装的 window.fetch override 路由到 ApiProxy 插件，
 * 绕开 WebView CORS preflight。dev 走 vite + preview-gateway 保持不变。
 */
export function getAgentApiBase(): string {
  if (import.meta.env.DEV) {
    // dev: 走 preview-gateway 统一前缀
    return "/agent-api";
  }
  // prod
  if (isNative()) {
    // native: 相对路径，window.fetch 已被 override 走 ApiProxy 插件
    return "";
  }
  // prod web SPA: 走用户配置 / 默认绝对 URL
  return getApiBaseUrl() || DEFAULT_API_BASE_URL;
}

/**
 * 解析 Agent API base 的详细上下文（用于错误日志、DevLogs、状态徽标）
 * 让排错时一眼看清"当前 agent API 实际打到哪里"。
 */
export interface AgentApiBaseContext {
  base: string;
  source: "dev-gateway" | "native-default" | "user-configured" | "web-fallback";
  isNative: boolean;
  env: "dev" | "prod";
  /** 完整 URL 拼接样例（如 `${base}/api/encrypt-key`），便于日志对比 */
  sampleUrl: string;
}

export function getAgentApiBaseContext(): AgentApiBaseContext {
  const env = import.meta.env.DEV ? "dev" : "prod";
  const native = isNative();

  if (env === "dev") {
    return {
      base: "/agent-api",
      source: "dev-gateway",
      isNative: native,
      env,
      sampleUrl: `${location.origin}/agent-api/api/encrypt-key`,
    };
  }

  // prod
  if (native) {
    // native: 相对路径，sampleUrl 拼成 /api/encrypt-key 体现"由 ApiProxy 接管"
    return {
      base: "",
      source: "native-default",
      isNative: true,
      env,
      sampleUrl: "/api/encrypt-key",
    };
  }

  // prod web SPA
  const apiBaseUrl = getApiBaseUrl();
  const hasUserOverride = (() => {
    try {
      return !!localStorage.getItem("encv-server-url");
    } catch {
      return false;
    }
  })();
  const source: AgentApiBaseContext["source"] = hasUserOverride ? "user-configured" : "web-fallback";

  return {
    base: apiBaseUrl || DEFAULT_API_BASE_URL,
    source,
    isNative: false,
    env,
    sampleUrl: `${apiBaseUrl || DEFAULT_API_BASE_URL}/api/encrypt-key`,
  };
}
