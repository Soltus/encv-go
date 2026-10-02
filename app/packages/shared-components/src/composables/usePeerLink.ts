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
  };
}
