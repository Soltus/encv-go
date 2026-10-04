/**
 * usePeerLink — 双端互联的前端抽象（spec desktop-web-android-pairing P2a/P2b）
 *
 * 边界（写死防跑偏）：
 *   - `peer`（另一台已配对设备）**不是** `baseUrl` 的候选，绝不污染既有 baseUrl 语义
 *     （与 useApiBaseProbe 的探测链完全无关）。所有 peerlink 调用都走**同源** `/api/peerlink/*`。
 *   - 桌面端（web）跑在 cnb 公网、安卓端在 NAT 后 ⇒ 不存在 LAN 直连；本模块只负责
 *     「申请票据 → 轮询配对状态 → 列出/解配已配对设备」，不负责建立长连接（那是 Go 侧 Edge 的事）。
 *   - psk / token 只在内存流转（用于二维码与 SAS 核对），**禁止写入 localStorage**。
 *
 * shared 边界：base URL 与 fetch 由应用层注入（默认同源相对路径 + 原生 fetch）。
 */

import { computed, ref } from "vue";

export interface PeerTicket {
  pairingId: string;
  psk: string;
  hub: string;
  expiresIn: number;
}

export interface PairedPeerInfo {
  peerId: string;
  sas: string;
  name: string;
  platform: string;
  pairedAt: string;
}

export interface PeerListItem {
  id: string;
  deviceId: string;
  name: string;
  platform: string;
  pairedAt: string;
  lastSeen: string;
  online: boolean;
}

export type PeerLinkStatus = "idle" | "pairing" | "paired" | "timeout" | "error";

// ── 注入点：应用层可覆盖 base URL / fetch（测试与插件环境需要） ──────────
let baseUrlProvider: () => string = () => "/api/peerlink";
let fetchProvider: typeof fetch = (...args: Parameters<typeof fetch>) => fetch(...args);

export function setPeerLinkBaseUrlProvider(fn: () => string) {
  baseUrlProvider = fn;
}

export function setPeerLinkFetchProvider(fn: typeof fetch) {
  fetchProvider = fn;
}

/** 复位注入（测试用） */
export function __resetPeerLinkProvidersForTests() {
  baseUrlProvider = () => "/api/peerlink";
  fetchProvider = (...args: Parameters<typeof fetch>) => fetch(...args);
}

/**
 * 测试专用：复位模块级单例状态。
 * ⚠️ 本测试跑在 vitest isolate:false 的 fast project 里，必须用显式 reset，
 *    **禁止** `vi.resetModules()`（会污染共享模块注册表，见 useFormFactor 同款坑）。
 */
export function __resetPeerLinkStateForTests() {
  state.status.value = "idle";
  state.ticket.value = null;
  state.paired.value = null;
  state.peers.value = [];
  state.lastError.value = "";
  state.busy.value = false;
  __resetPeerLinkProvidersForTests();
}

const OPERATOR_HEADER = { "X-Peerlink-Operator": "1" };

function url(path: string, query?: Record<string, string>) {
  const base = baseUrlProvider().replace(/\/+$/, "");
  const qs = query ? `?${new URLSearchParams(query).toString()}` : "";
  return `${base}${path}${qs}`;
}

async function getJSON<T>(path: string, query?: Record<string, string>, headers?: Record<string, string>): Promise<T> {
  const res = await fetchProvider(url(path, query), { method: "GET", headers: { ...OPERATOR_HEADER, ...headers } });
  if (!res.ok) {
    throw new Error(`peerlink ${path} failed: ${res.status}`);
  }
  return (await res.json()) as T;
}

const state = {
  status: ref<PeerLinkStatus>("idle"),
  ticket: ref<PeerTicket | null>(null),
  paired: ref<PairedPeerInfo | null>(null),
  peers: ref<PeerListItem[]>([]),
  lastError: ref<string>(""),
  busy: ref(false),
};

/**
 * ① 桌面端申请一次性配对票据（二维码内容来源）。
 * 票据 120s 过期、一次性；**不含任何内网地址**。
 */
export async function createPairingTicket(hubOverride?: string): Promise<PeerTicket> {
  state.busy.value = true;
  state.lastError.value = "";
  try {
    const res = await fetchProvider(url("/ticket"), {
      method: "POST",
      headers: { "Content-Type": "application/json", ...OPERATOR_HEADER },
      body: JSON.stringify(hubOverride ? { hub: hubOverride } : {}),
    });
    if (!res.ok) throw new Error(`ticket failed: ${res.status}`);
    const tk = (await res.json()) as PeerTicket;
    if (!tk.pairingId || !tk.psk || !tk.hub) throw new Error("ticket 响应字段不完整");
    state.ticket.value = tk;
    state.status.value = "pairing";
    return tk;
  } catch (e) {
    state.status.value = "error";
    state.lastError.value = e instanceof Error ? e.message : String(e);
    throw e;
  } finally {
    state.busy.value = false;
  }
}

/** ② 单次查询配对状态（未配对 = null） */
export async function fetchPairingStatus(pairingId: string): Promise<PairedPeerInfo | null> {
  try {
    return await getJSON<PairedPeerInfo>("/pairing/status", { pairingId });
  } catch (e) {
    // 404 = 尚未配对（正常轮询态），不算错误
    if (e instanceof Error && e.message.includes("404")) return null;
    throw e;
  }
}

/**
 * ③ 轮询等待扫码方完成配对。
 * @param pairingId 票据 ID
 * @param opts.timeoutMs 总超时（默认 115s，与后端 120s 票据有效期对齐）
 * @param opts.intervalMs 轮询间隔（默认 1500ms）
 */
export async function waitForPairing(pairingId: string, opts: { timeoutMs?: number; intervalMs?: number } = {}): Promise<PairedPeerInfo> {
  const timeoutMs = opts.timeoutMs ?? 115_000;
  const intervalMs = opts.intervalMs ?? 1_500;
  const deadline = Date.now() + timeoutMs;

  for (;;) {
    const info = await fetchPairingStatus(pairingId);
    if (info) {
      state.paired.value = info;
      state.status.value = "paired";
      return info;
    }
    if (Date.now() >= deadline) {
      state.status.value = "timeout";
      state.lastError.value = "配对超时，请刷新二维码重试";
      throw new Error("peerlink: pairing timeout");
    }
    await new Promise(r => setTimeout(r, intervalMs));
  }
}

/** ④ 已配对设备列表（脱敏，无密钥） */
export async function fetchPeers(): Promise<PeerListItem[]> {
  const res = await getJSON<{ items: PeerListItem[] }>("/peers");
  state.peers.value = res.items ?? [];
  return state.peers.value;
}

// ── 扫码端（安卓）：解析二维码 / 作为 Edge 连到 Hub ──────────────────────
//
// 桌面端生成的二维码内容 = `{"v":2,"hub":..,"pairingId":..,"psk":..,"exp":..}`
// （见 encv-mobile `PeerPairingPanel.vue` 的 pairingCode）。扫码端只需把它交给
// 本端 Go 后端的 `POST /api/peerlink/edge/pair`（Edge 由 Go 进程常驻，不在 WebView 里）。
//
// ⚠️ 契约红线：
//   - hub 必须是 https，**或**本机回环 http（R3：明文 http 跨端会被浏览器按混合内容拦截，
//     后端 `validateHubURL` 同样拒绝）。
//   - psk 只在内存流转，绝不落 localStorage。

export interface PairingQRPayload {
  v?: number;
  hub: string;
  pairingId: string;
  psk: string;
  exp?: number;
}

export type PairingQRParse =
  | { ok: true; payload: PairingQRPayload }
  | { ok: false; reason: "not_json" | "missing_field" | "bad_hub" | "expired" };

/**
 * Hub 地址是否可接受（R3）：https 一律放行；http 只允许本机回环（开发/自测）。
 */
export function isAllowedHubAddress(hub: string): boolean {
  const raw = String(hub ?? "").trim();
  if (!raw) return false;
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return false;
  }
  if (u.protocol === "https:") return true;
  if (u.protocol === "http:") return ["127.0.0.1", "localhost", "[::1]", "::1"].includes(u.hostname);
  return false;
}

/**
 * 解析二维码文本。
 * `exp` 语义：桌面端写的是**剩余秒数**（不是绝对时间戳）⇒ 这里只在它看起来像
 * 绝对毫秒时间戳时才判过期；真正的 120s 一次性有效期由后端票据保证。
 */
export function parsePairingQR(text: string, nowMs: number = Date.now()): PairingQRParse {
  const raw = String(text ?? "").trim();
  if (!raw) return { ok: false, reason: "not_json" };
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return { ok: false, reason: "not_json" };
  }
  if (typeof data !== "object" || data === null) return { ok: false, reason: "not_json" };
  const p = data as Partial<PairingQRPayload>;
  if (!p.hub || !p.pairingId || !p.psk) return { ok: false, reason: "missing_field" };
  if (!isAllowedHubAddress(p.hub)) return { ok: false, reason: "bad_hub" };
  // 绝对时间戳（ms）才判过期，避免把"剩余秒数"当成过期时间误杀
  if (typeof p.exp === "number" && p.exp > 1e12 && p.exp <= nowMs) {
    return { ok: false, reason: "expired" };
  }
  return { ok: true, payload: { v: p.v, hub: p.hub, pairingId: p.pairingId, psk: p.psk, exp: p.exp } };
}

export interface EdgePairResult {
  ok: boolean;
  hub: string;
  peerId: string;
  /**
   * SAS 6 位安全码（2026-10-04 新增）。
   * 桌面端点「一致，信任该设备」时手里有 SAS；扫码端（手机）此前**什么都不显示**，
   * 用户根本不知拿什么去比对。本端 Go 的 `/edge/pair` 现在把远端 Hub 回的 SAS 透出，
   * 由 PeerScanPanel 显示。为空 = 旧后端，UI 需优雅降级（不显示核对区）。
   */
  sas?: string;
}

/**
 * 配对失败的原因码（2026-10-05）。
 *
 * 由来：手机拿一张失效的码去配对时，后端此前只回一句 `pair_rejected:401`，
 *   界面只能显示"连接失败" ⇒ 用户拿着过期码反复扫，却**没有任何"刷新二维码"的引导**
 *   （真机反馈：不知道该怎么办）。现在后端把远端错误码透出来，前端据此给出具体动作。
 */
export type PairFailureReason =
  | "ticket_expired"
  | "ticket_used"
  | "ticket_not_found"
  | "bad_hub"
  | "invalid_request"
  | "unknown";

/** 携带原因码的配对失败（UI 据此给不同的引导文案，而不是一律"连接失败"）。 */
export class PairEdgeError extends Error {
  readonly reason: PairFailureReason;
  /** 是否属于"刷新二维码重扫即可解决"（后端 refreshQr 或票据类错误码） */
  readonly refreshQr: boolean;
  readonly status: number;

  constructor(reason: PairFailureReason, status: number, detail: string, refreshQr: boolean) {
    super(`edge/pair failed: ${status} ${reason}${detail ? ` ${detail}` : ""}`);
    this.name = "PairEdgeError";
    this.reason = reason;
    this.status = status;
    this.refreshQr = refreshQr;
  }
}

function toPairFailureReason(code: string): PairFailureReason {
  switch (code) {
    case "ticket_expired":
    case "ticket_used":
    case "ticket_not_found":
      return code;
    case "invalid_hub":
      return "bad_hub";
    case "invalid_json":
    case "invalid_request":
      return "invalid_request";
    default:
      return "unknown";
  }
}

/** 扫码后：让**本端** Go 进程作为 Edge 去连 Hub（长连接由 Go 侧承载）。 */
export async function pairAsEdge(input: {
  hub: string;
  pairingId: string;
  psk: string;
  deviceId?: string;
  name?: string;
}): Promise<EdgePairResult> {
  const res = await fetchProvider(url("/edge/pair"), {
    method: "POST",
    headers: { "Content-Type": "application/json", ...OPERATOR_HEADER },
    body: JSON.stringify({
      hub: input.hub,
      pairingId: input.pairingId,
      psk: input.psk,
      ...(input.deviceId ? { deviceId: input.deviceId } : {}),
      ...(input.name ? { name: input.name } : {}),
    }),
  });
  const text = await res.text();
  if (!res.ok) {
    // ⚠️ 原因码必须带出去：一律抛 "连接失败" 会让"码过期了"和"连不上会合点"
    //    在 UI 上长得一模一样，用户只能反复重试同一个注定失败的动作。
    let code = "";
    let refreshQr = false;
    let detail = text.slice(0, 200);
    try {
      const body = JSON.parse(text) as { error?: string; reason?: string; refreshQr?: boolean; detail?: string };
      code = String(body.reason || body.error || "");
      refreshQr = body.refreshQr === true;
      if (body.detail) detail = body.detail;
    } catch {
      /* 非 JSON 响应：退化为 unknown */
    }
    const reason = toPairFailureReason(code);
    // 票据类错误即便旧后端没带 refreshQr，也应当引导刷新二维码（客户端兜底）
    const ticketLike = reason === "ticket_expired" || reason === "ticket_used" || reason === "ticket_not_found";
    throw new PairEdgeError(reason, res.status, detail, refreshQr || ticketLike);
  }
  const out = JSON.parse(text) as EdgePairResult;
  if (!out?.ok) throw new Error("edge/pair 返回 ok=false");
  return out;
}

export interface EdgeStatus {
  running: boolean;
  connected?: boolean;
  hub?: string;
  peerId?: string;
  /**
   * 最近一次连接失败原因（2026-10-04 新增，后端 /edge/status 提供）。
   * 有它才能区分「正在连」和「根本连不上」——此前 UI 一律显示"连接中…"（静默失败）。
   */
  lastErr?: string;
  /** 已退避重试次数 */
  attempts?: number;
}

/** 本端 Edge 的运行状态（是否正连着 Hub）。 */
export async function fetchEdgeStatus(): Promise<EdgeStatus> {
  return await getJSON<EdgeStatus>("/edge/status");
}

/**
 * 断开本端 Edge（停止长连接，token 作废）。
 *
 * 2026-10-04：补这个封装是为了让**扫码端（手机）**也能解除与对端的连接 ——
 *   此前只有 Hub 侧有 `unpairPeer(peerId)`，但手机是 Edge，它根本没有 peers 列表
 *   （`/peers` 只有 Hub 有）⇒ 手机端 UI 只能干瞪眼，连"断开"入口都没有。
 */
export async function stopEdge(): Promise<boolean> {
  try {
    const res = await fetchProvider(url("/edge/stop"), {
      method: "POST",
      headers: { "Content-Type": "application/json", ...OPERATOR_HEADER },
      body: "{}",
    });
    return res.ok;
  } catch {
    return false;
  }
}

// ── 云控热更新（2026-10-05）───────────────────────────────────────
//
// 待办 ②（docs/HANDOVER-cloud-hot-update.md §5）要求：
//   桌面端能看热更历史 / 触发回滚；移动端能看自己装了哪版并能退回去。
// 边界：这些端点都是**运维**接口（X-Peerlink-Operator），绝不暴露给未配对对端。

export interface BundleManifestItem {
  name: string;
  version: string;
  size: number;
  sha256: string;
  updatedAt?: string;
  abi?: string;
}

export interface BundleReportItem {
  at: string;
  peerId: string;
  name: string;
  version: string;
  ok: boolean;
  rolledBack?: boolean;
  previousVersion?: string;
  error?: string;
}

/** 云控视角：仓库里有哪些包 / 各设备装到哪版 / 最近的下发与回滚记录 */
export interface BundleStatus {
  available: BundleManifestItem[];
  deviceVer: Record<string, string>;
  reports: BundleReportItem[];
  reportCount: number;
}

/** GET /api/peerlink/bundle/status（运维） */
export async function fetchBundleStatus(): Promise<BundleStatus> {
  const res = await getJSON<BundleStatus>("/bundle/status");
  return {
    available: res.available ?? [],
    deviceVer: res.deviceVer ?? {},
    reports: res.reports ?? [],
    reportCount: res.reportCount ?? (res.reports?.length ?? 0),
  };
}

/**
 * POST /api/peerlink/bundle/rollback（运维）：**云控一键回滚**。
 *
 * 为什么不是"再推一个旧版本"：推旧版要重新打包 + 重走下载/校验/切换（分钟级），
 * 而设备本地本来就有上一次的备份，回滚只是搬回来（秒级）。
 */
export async function rollbackBundle(peerId: string, name: string): Promise<{ ok: boolean; version?: string }> {
  const res = await fetchProvider(url("/bundle/rollback"), {
    method: "POST",
    headers: { "Content-Type": "application/json", ...OPERATOR_HEADER },
    body: JSON.stringify({ peerId, name }),
  });
  const text = await res.text();
  if (!res.ok) {
    let reason = "";
    try {
      reason = String((JSON.parse(text) as { reason?: string; error?: string }).reason ?? "");
    } catch {
      /* ignore */
    }
    throw new Error(`bundle/rollback failed: ${res.status}${reason ? ` ${reason}` : ""}`);
  }
  return JSON.parse(text) as { ok: boolean; version?: string };
}

/** 本端（设备侧）一个包的生效状态 */
export interface LocalBundle {
  name: string;
  version: string;
  kind: "dir" | "file";
  /** 有没有上一版可退（false ⇒ 回滚必然被拒，UI 应禁用按钮） */
  rollable: boolean;
}

/** GET /api/peerlink/bundle/local：本端装了哪些包、各是哪版、能不能退 */
export async function fetchLocalBundles(): Promise<LocalBundle[]> {
  const res = await getJSON<{ items: LocalBundle[] }>("/bundle/local");
  return res.items ?? [];
}

/** POST /api/peerlink/bundle/local/rollback：设备**自己**退回上一版（不经过云端） */
export async function rollbackLocalBundle(name: string): Promise<{ ok: boolean; version?: string }> {
  const res = await fetchProvider(url("/bundle/local/rollback"), {
    method: "POST",
    headers: { "Content-Type": "application/json", ...OPERATOR_HEADER },
    body: JSON.stringify({ name }),
  });
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`local rollback failed: ${res.status} ${text.slice(0, 120)}`);
  }
  return JSON.parse(text) as { ok: boolean; version?: string };
}

/** ⑤ 解配（token 立即作废） */
export async function unpairPeer(peerId: string): Promise<boolean> {
  const res = await fetchProvider(url("/unpair"), {
    method: "POST",
    headers: { "Content-Type": "application/json", ...OPERATOR_HEADER },
    body: JSON.stringify({ peerId }),
  });
  if (!res.ok) return false;
  await fetchPeers().catch(() => undefined);
  return true;
}

/** 组件内消费 */
export function usePeerLink() {
  return {
    status: state.status,
    ticket: state.ticket,
    paired: state.paired,
    peers: state.peers,
    lastError: state.lastError,
    busy: state.busy,
    onlinePeers: computed(() => state.peers.value.filter(p => p.online)),
    createPairingTicket,
    waitForPairing,
    fetchPeers,
    unpairPeer,
    pairAsEdge,
    fetchEdgeStatus,
  };
}
