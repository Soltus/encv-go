import { onMounted, onUnmounted, type Ref, ref } from "vue";
import { checkServerStatus, DEFAULT_API_BASE_URL, getPersistedBackendIdentity, setApiBaseUrl } from "@encv/shared-components/api/encv";
import { getBackendStatus, isNative, restartBackend, stopBackend } from "@encv/shared-components/plugins/GoProcess";
import { useApiBaseProbe } from "./useApiBaseProbe";
import { eventBus } from "./useEventBus";
import { type TransportMode, useRealtimeTransport } from "./useRealtimeTransport";

const isOnline = ref(false);
const lastError = ref("");
const backendPort = ref(0);
const isRestarting = ref(false);
const isStopping = ref(false);
// 🆕 2026-06-10 详情页状态展示增强
const latencyMs = ref(0); // 上次 checkStatus HTTP 响应延迟
const lastCheckedAt = ref<Date | null>(null); // 上次探测时间
// 🆕 2026-06-15：复用桌面后端 performPingCheck 的 InstanceID 防劫持机制
//   持久化到 localStorage 的 instance_id（key: encv-server-instance-id）。
//   跨会话探测时如果变化 → 报告"backend instance changed"，防止
//   "返 200 application/json 的进程被错认为 encv-go" 的漏洞。
const backendInstanceId = ref("");
const backendVersion = ref("");
// 首次启动时从 localStorage 恢复（让 UI 立刻能展示"上次连的进程"）
const persisted = getPersistedBackendIdentity();
if (persisted) {
  backendInstanceId.value = persisted.instanceId;
  backendVersion.value = persisted.version;
}
let initialized = false;
let nativeBridgeListenerAdded = false;

// ============================================================
// 🆕 2026-06-16：详情页专属可观察数据（卡片放不下）
//  1) latencyHistory    — 最近 60 次探测延迟（SVG 折线图）
//  2) stateHistory      — 最近 10 次状态切换（时间线）
//  3) actionHistory     — 最近 30 次用户操作（操作日志）
//  4) metrics           — 应用元信息（App 版本 / 平台 / User-Agent / 启动时间）
//  5) recordAction()    — 父级调用，登记用户操作
//  6) networkPing()     — 主动 ping baseUrl 测 RTT（裸 ping，无 checkStatus 副作用）
// ============================================================

/** 延迟历史（环形 buffer，最多 60 条） */
const latencyHistory = ref<Array<{ at: number; ms: number }>>([]);
/** 状态切换历史（最多 10 条） */
const stateHistory = ref<Array<{ at: number; state: "online" | "offline" | "checking"; reason: string }>>([]);
/** 用户操作历史（最多 30 条） */
const actionHistory = ref<
  Array<{ at: number; action: "check" | "start" | "stop" | "restart" | "reconnect" | "ping"; success: boolean; detail?: string }>
>([]);
/** 应用元信息 */
const metrics = ref<{
  appStartTime: number;
  platform: string;
  userAgent: string;
  networkType: string;
  isNative: boolean;
  isSandboxBrowser: boolean;
  apiBaseUrl: string;
}>({
  appStartTime: Date.now(),
  platform: "unknown",
  userAgent: "",
  networkType: "unknown",
  isNative: false,
  isSandboxBrowser: false,
  apiBaseUrl: "",
});
// 初始化静态 metrics
metrics.value.platform = (typeof navigator !== "undefined" && (navigator as any).platform) || "unknown";
metrics.value.userAgent = (typeof navigator !== "undefined" && navigator.userAgent) || "";
metrics.value.isNative = isNative();
metrics.value.isSandboxBrowser = useRealtimeTransport().isSandboxBrowser.value;

function recordAction(action: "check" | "start" | "stop" | "restart" | "reconnect" | "ping", success: boolean, detail?: string) {
  actionHistory.value.unshift({ at: Date.now(), action, success, detail });
  if (actionHistory.value.length > 30) actionHistory.value.length = 30;
}

function recordStateChange(state: "online" | "offline" | "checking", reason = "") {
  stateHistory.value.unshift({ at: Date.now(), state, reason });
  if (stateHistory.value.length > 10) stateHistory.value.length = 10;
}

function recordLatency(ms: number) {
  latencyHistory.value.push({ at: Date.now(), ms });
  if (latencyHistory.value.length > 60) latencyHistory.value.shift();
}

/** 主动 ping baseUrl — 不修改 isOnline / lastError，纯粹测 RTT
 *  已无外部消费者（ServerStatusDetail.vue 于 2026-06-16 删除） */
async function networkPing(): Promise<{ ok: boolean; ms: number; error?: string }> {
  const t0 = performance.now();
  try {
    const { getApiBaseUrl } = await import("@encv/shared-components/api/encv");
    const baseUrl = getApiBaseUrl();
    metrics.value.apiBaseUrl = baseUrl;
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), 5000);
    const res = await fetch(`${baseUrl}/ping`, { method: "GET", signal: ctrl.signal });
    clearTimeout(timer);
    const ms = Math.round(performance.now() - t0);
    if (res.ok) {
      recordAction("ping", true, `${ms}ms`);
      return { ok: true, ms };
    }
    recordAction("ping", false, `HTTP ${res.status}`);
    return { ok: false, ms, error: `HTTP ${res.status}` };
  } catch (e) {
    const ms = Math.round(performance.now() - t0);
    const err = e instanceof Error ? e.message : String(e);
    recordAction("ping", false, err);
    return { ok: false, ms, error: err };
  }
}

// 探测一次 HTTP + 测延迟
async function probeHttp() {
  const t0 = performance.now();
  const result = await checkServerStatus();
  const dt = Math.round(performance.now() - t0);
  latencyMs.value = dt;
  lastCheckedAt.value = new Date();
  // 🆕 记录到延迟历史（详情页趋势图）
  if (result.online) recordLatency(dt);
  // 🆕 2026-06-15：把 checkServerStatus 探测到的 instance_id / version 同步到
  //   useServerStatus 共享状态，让 UI 任何地方都能显示当前 backend 身份
  if (result.instanceId) backendInstanceId.value = result.instanceId;
  if (result.version) backendVersion.value = result.version;
  // 🆕 2026-06-15 修复：instance 变化通过专门事件发出（不依赖 lastError）
  //   之前：hijack 警告进 lastError → 永远显示"backend instance changed" → 状态卡 offline
  //   现在：emit 'backend:instance-changed' → UI 顶部 banner 提示，状态机立即恢复 online
  if (result.instanceChanged) {
    eventBus.emit("backend:instance-changed", result.instanceChanged);
  }
  return { ...result, latency: dt };
}

function onServerStatus(data: { online: boolean }) {
  if (isRestarting.value && !data.online) return;
  const wasOnline = isOnline.value;
  isOnline.value = data.online;
  // 🆕 状态切换 → push 历史
  if (wasOnline !== data.online) {
    recordStateChange(data.online ? "online" : "offline", data.online ? "connected" : "disconnected");
  }
  if (data.online) {
    lastError.value = "";
  }
}

function onConnectionError(data: { error: string }) {
  if (isRestarting.value) return;
  // 🆕 2026-06-10 sandbox 浏览器下不显示 "Connection closed (code: 1006)"
  //  （trae 反代 :16000 不支持 WS 是已知架构限制，不是用户后端故障）
  // transport 内部已分流：http-poll / native-bridge 模式不 emit connection-error
  // 这里再加一层 isSandboxBrowser 防御：万一 transport 误触发了，仍不打扰用户
  const transport = useRealtimeTransport();
  if (transport.isSandboxBrowser.value) return;
  lastError.value = data.error;
}

async function checkStatus() {
  const result = await probeHttp();
  isOnline.value = result.online;
  lastError.value = result.error || "";
  if (result.online) {
    isRestarting.value = false;
    isStopping.value = false;
  }
  // 🆕 2026-06-16 详情页：记录操作历史
  recordAction("check", result.online, result.online ? `${result.latency}ms` : result.error || "offline");
  return result;
}

/**
 * 手动重连：先跑 probe 探测链，命中后用新 baseUrl 重建 transport。
 * 用于：
 *   - 冷启动后仍 offline（探测失败） → 再次尝试
 *   - 用户在 Settings 改了 baseUrl → 立即让状态同步
 *   - transport 死掉且 HTTP 链路也没救 → 重探
 */
async function manualReconnect(): Promise<{ ok: boolean; baseUrl?: string; error?: string }> {
  isRestarting.value = true;
  const transport = useRealtimeTransport();
  transport.disconnect();
  try {
    const result = await useApiBaseProbe().probe({ force: true });
    // probe 成功 → setApiBaseUrl 已写，再用 checkStatus 探一次确认"链路真通"
    const check = await checkStatus();
    if (check.online) {
      transport.connect();
      eventBus.emit("api-base:connected", { baseUrl: result.baseUrl, source: result.source });
      return { ok: true, baseUrl: result.baseUrl };
    }
    // 探测说通但 check 失败——罕见（race / 服务端崩），保留探测结果
    return { ok: false, baseUrl: result.baseUrl, error: check.error || "post-probe health check failed" };
  } catch (e) {
    const errMsg = e instanceof Error ? e.message : String(e);
    lastError.value = errMsg;
    isOnline.value = false;
    eventBus.emit("api-base:disconnected", { error: errMsg });
    return { ok: false, error: errMsg };
  } finally {
    isRestarting.value = false;
  }
}

/**
 * App 切回前台时触发一次探测（节流内置）
 *
 * 场景：用户在聊天过程中把 app 切到后台几分钟，网络环境可能已变
 *  （切 WiFi / 出隧道 / VPN 切换）。切回前台时再跑一次 probe，
 *  若 baseUrl 变了 → setApiBaseUrl → api-base:connected → useRealtimeTransport 重连。
 */
let _visibilityListenerAdded = false;
function setupVisibilityProbe() {
  if (_visibilityListenerAdded) return;
  if (typeof document === "undefined") return;
  _visibilityListenerAdded = true;

  const transport = useRealtimeTransport();
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState !== "visible") return;
    if (isNative()) {
      // APK 模式：backend 跑在设备本地 127.0.0.1，IP 不会变；只需重连 transport
      if (!isOnline.value) {
        console.info("[useServerStatus] visibility visible + offline → reconnect");
        transport.forceReconnect();
      }
      return;
    }
    // web/dev 模式：跑探测（cached → loopback → LAN 候选）
    console.info("[useServerStatus] visibility visible → probe()");
    useApiBaseProbe()
      .probe()
      .then(result => {
        if (result.baseUrl) {
          // probe 成功 → api-base:connected 事件会自动 fire（useApiBaseProbe.commit）
          // 重新 check status 以更新 isOnline
          checkStatus().then(check => {
            if (check.online) {
              transport.connect();
            }
          });
        }
      })
      .catch(e => {
        // 全失败（all-candidates-failed）不弹错 — 保留旧值
        console.debug("[useServerStatus] visibility probe failed:", e instanceof Error ? e.message : String(e));
      });
  });
}

function addNativeBridgeListener() {
  if (nativeBridgeListenerAdded) return;
  nativeBridgeListenerAdded = true;

  if (typeof window !== "undefined") {
    const transport = useRealtimeTransport();
    const syncStatus = (event: Event) => {
      const customEvent = event as CustomEvent;
      const detail = customEvent.detail || {};
      console.log("[ENCV] Backend status from native bridge:", detail);
      if (typeof detail.running === "boolean") {
        isOnline.value = detail.running;
      }
      if (detail.port && detail.port > 0) {
        backendPort.value = detail.port;
        const newUrl = `http://127.0.0.1:${detail.port}`;
        if (DEFAULT_API_BASE_URL !== newUrl) {
          setApiBaseUrl(newUrl);
        }
        isOnline.value = true;
        lastError.value = "";
        isRestarting.value = false;
        isStopping.value = false;
        transport.connect();
      }
      if (detail.error) {
        lastError.value = detail.error;
        isOnline.value = false;
        isRestarting.value = false;
        isStopping.value = false;
      }
      if (detail.running === false && !detail.port) {
        backendPort.value = 0;
        isStopping.value = false;
      }
    };

    window.addEventListener("encv:backend-ready", syncStatus as EventListener);
    window.addEventListener("encv:backend-status", syncStatus as EventListener);
  }
}

async function handleRestart(): Promise<boolean> {
  if (!isNative()) {
    isOnline.value = false;
    useRealtimeTransport().disconnect();
    recordAction("restart", false, "not native");
    return false;
  }
  isRestarting.value = true;
  isStopping.value = false;
  isOnline.value = false;
  lastError.value = "";
  useRealtimeTransport().disconnect();
  recordStateChange("checking", "restarting");
  try {
    const result = await restartBackend();
    isRestarting.value = false;
    if (result.success) {
      isOnline.value = true;
      const status = await getBackendStatus();
      if (status.running && status.port > 0) {
        backendPort.value = status.port;
        const newUrl = `http://127.0.0.1:${status.port}`;
        if (DEFAULT_API_BASE_URL !== newUrl) {
          setApiBaseUrl(newUrl);
        }
        lastError.value = "";
        useRealtimeTransport().connect();
      }
      recordAction("restart", true, `port=${status.port}`);
    } else {
      lastError.value = result.lastError || lastError.value;
      recordAction("restart", false, result.lastError || "unknown");
    }
    return result.success;
  } catch (e) {
    isRestarting.value = false;
    lastError.value = e instanceof Error ? e.message : String(e);
    return false;
  }
}

async function handleStop(): Promise<boolean> {
  if (!isNative()) {
    recordAction("stop", false, "not native");
    return false;
  }
  isStopping.value = true;
  isRestarting.value = false;
  isOnline.value = false;
  lastError.value = "";
  useRealtimeTransport().disconnect();
  recordStateChange("checking", "stopping");
  try {
    const result = await stopBackend();
    isStopping.value = false;
    backendPort.value = 0;
    recordAction("stop", result.success, result.success ? "stopped" : "failed");
    return result.success;
  } catch (e) {
    isStopping.value = false;
    lastError.value = e instanceof Error ? e.message : String(e);
    recordAction("stop", false, e instanceof Error ? e.message : String(e));
    return false;
  }
}

export function useServerStatus() {
  const transport = useRealtimeTransport();

  addNativeBridgeListener();
  setupVisibilityProbe();

  onMounted(async () => {
    if (!initialized) {
      initialized = true;
      if (isNative()) {
        const status = await getBackendStatus();
        if (status.running && status.port > 0) {
          isOnline.value = true;
          backendPort.value = status.port;
          lastError.value = status.lastError || "";
          transport.connect();
        } else if (status.lastError) {
          lastError.value = status.lastError;
        }
      } else {
        // Web/dev 模式：跑探测链（cached → loopback → LAN 候选）
        // 探测成功 → 写 localStorage + setApiBaseUrl + connect
        // 探测失败 → 保留旧值兜底，不弹死错误
        try {
          const result = await useApiBaseProbe().probe();
          if (result.baseUrl) {
            const check = await checkStatus();
            if (check.online) {
              transport.connect();
              eventBus.emit("api-base:connected", { baseUrl: result.baseUrl, source: result.source });
            } else {
              // 罕见：探测到 URL 但 health check 失败
              lastError.value = check.error || "post-probe health check failed";
              isOnline.value = false;
            }
          } else {
            // 全失败，尝试 legacy checkStatus 兜底
            const fallback = await checkStatus();
            if (fallback.online) transport.connect();
          }
        } catch (probeErr) {
          // 🆕 任何意外 throw → 兜底 legacy checkStatus，不让 [App] 错误边界捕获
          console.warn(
            "[useServerStatus] probe threw unexpectedly, falling back:",
            probeErr instanceof Error ? probeErr.message : String(probeErr)
          );
          try {
            const fallback = await checkStatus();
            if (fallback.online) transport.connect();
          } catch (fallbackErr) {
            console.debug(
              "[useServerStatus] legacy fallback also failed (expected in trae sandbox):",
              fallbackErr instanceof Error ? fallbackErr.message : String(fallbackErr)
            );
          }
        }
      }
      eventBus.on("server:status", onServerStatus);
      eventBus.on("server:connection-error", onConnectionError);
    }
  });

  onUnmounted(() => {
    // server:status / server:connection-error 是模块级单例订阅（不在 composable 内
    // 注销），所以多 component 共享 isOnline 状态
  });

  return {
    isOnline,
    lastError,
    backendPort,
    isRestarting,
    isStopping,
    checkStatus,
    connectionState: transport.connectionState,
    // 🆕 2026-06-10 详情页状态展示（统一从 transport 读，不再各自判定）
    latencyMs,
    transportMode: transport.transportMode as Readonly<Ref<TransportMode>>,
    lastCheckedAt,
    isSandboxBrowser: transport.isSandboxBrowser,
    // 🆕 2026-06-15：当前 backend 的 instance_id + version
    //   来自 desktop 后端 /ping（PingResponse.instance_id），用于：
    //   1. 防"端口被劫持"——performPingCheck 模式复用
    //   2. UI 详情页展示当前 backend 身份（用户/AI 都可看）
    backendInstanceId,
    backendVersion,
    restartBackend: handleRestart,
    stopBackend: handleStop,
    // 手动重连：跑探测链 + 重建 transport（用于 Settings "立即探测" / 错误 banner "重试"）
    manualReconnect,
    // 直接暴露 probe composable，UI 可调 probe() / setManual() / resetToDefault()
    probe: useApiBaseProbe,
    // 🆕 2026-06-16：详情页专属可观察数据
    //   趋势图 / 时间线 / 操作历史 / 应用元信息 / 主动 ping
    latencyHistory,
    stateHistory,
    actionHistory,
    metrics,
    recordAction,
    networkPing,
  };
}
