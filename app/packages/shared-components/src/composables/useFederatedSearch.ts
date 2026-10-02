/**
 * useFederatedSearch —— 互通搜索索引（spec desktop-web-android-pairing P3）
 *
 * ⚠️ 语义红线（与"挂载网络驱动器"划清界限）：
 *   - 联邦搜索 = **向对端发查询、拿回带来源标注的引用**；
 *     不搬运索引、不合并命名空间、不把远端路径伪装成本地路径。
 *   - 每条命中都带 `source`（local/peer）+ `peerId/peerName` + 原始远端路径，
 *     UI 必须显示来源徽章；跨端打开/取回是**显式动作**。
 *   - 对端离线/超时/报错 → 只降级为"该端无结果"（`peerStatuses` 记录状态），
 *     **绝不阻塞本端主搜索**，也不抛错打断用户。
 */

import { ref } from "vue";

export type HitSource = "local" | "peer";

export interface FederatedHit {
  /** 去重键：`local#<path>` 或 `<peerId>#<path>` */
  key: string;
  source: HitSource;
  peerId: string;
  peerName: string;
  path: string;
  name?: string;
  size?: number;
  mtime?: string;
  /** 原始条目（保留对端返回的其它字段） */
  raw: Record<string, unknown>;
}

export type PeerSearchState = "ok" | "offline" | "timeout" | "error";

export interface PeerSearchStatus {
  peerId: string;
  name: string;
  state: PeerSearchState;
  httpStatus?: number;
  detail?: string;
}

export interface FederatedOptions {
  kind?: "file" | "fulltext" | "vector";
  limit?: number;
  /** 已配对设备；不传则自行拉取（可注入以避免重复请求） */
  peers?: { id: string; name: string; online: boolean }[];
  /** 单端超时（默认 2s，与后端 DefaultCallTimeout 对齐） */
  timeoutMs?: number;
  /** 只搜对端（跳过本端） */
  skipLocal?: boolean;
}

export interface FederatedResult {
  items: FederatedHit[];
  peerStatuses: PeerSearchStatus[];
}

// ── 注入点（测试 / 插件环境） ────────────────────────────────────────
type FetchLike = typeof fetch;

let fetchProvider: FetchLike = (...args: Parameters<typeof fetch>) => fetch(...args);
let peerlinkBaseProvider: () => string = () => "/api/peerlink";
/** 本端搜索：默认打同源 `/api/files/search-fulltext` */
let localSearchProvider: (q: string, kind: string, limit: number) => Promise<unknown[]> = async (q, _kind, limit) => {
  const res = await fetchProvider(`/api/files/search-fulltext?q=${encodeURIComponent(q)}&limit=${limit}`);
  if (!res.ok) throw new Error(`local search failed: ${res.status}`);
  const data = (await res.json()) as { results?: unknown[]; items?: unknown[] };
  return (data.results ?? data.items ?? []) as unknown[];
};

export function setFederatedSearchProviders(p: {
  fetch?: FetchLike;
  peerlinkBase?: () => string;
  localSearch?: (q: string, kind: string, limit: number) => Promise<unknown[]>;
}) {
  if (p.fetch) fetchProvider = p.fetch;
  if (p.peerlinkBase) peerlinkBaseProvider = p.peerlinkBase;
  if (p.localSearch) localSearchProvider = p.localSearch;
}

export function __resetFederatedSearchProvidersForTests() {
  fetchProvider = (...args: Parameters<typeof fetch>) => fetch(...args);
  peerlinkBaseProvider = () => "/api/peerlink";
  localSearchProvider = async (q, _kind, limit) => {
    const res = await fetchProvider(`/api/files/search-fulltext?q=${encodeURIComponent(q)}&limit=${limit}`);
    if (!res.ok) throw new Error(`local search failed: ${res.status}`);
    const data = (await res.json()) as { results?: unknown[]; items?: unknown[] };
    return (data.results ?? data.items ?? []) as unknown[];
  };
}

const OPERATOR_HEADER = { "X-Peerlink-Operator": "1" };

function asHit(raw: unknown, source: HitSource, peerId: string, peerName: string): FederatedHit | null {
  if (!raw || typeof raw !== "object") return null;
  const o = raw as Record<string, unknown>;
  const path = typeof o.path === "string" ? o.path : typeof o.name === "string" ? o.name : "";
  if (!path) return null;
  return {
    key: `${peerId}#${path}`,
    source,
    peerId,
    peerName,
    path,
    name: typeof o.name === "string" ? o.name : undefined,
    size: typeof o.size === "number" ? o.size : undefined,
    mtime: typeof o.mtime === "string" ? o.mtime : undefined,
    raw: o,
  };
}

/** 带超时的单次 fetch（超时即 abort，不留悬挂请求） */
async function fetchWithTimeout(url: string, timeoutMs: number): Promise<Response> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    return await fetchProvider(url, { method: "GET", headers: OPERATOR_HEADER, signal: ctrl.signal });
  } finally {
    clearTimeout(timer);
  }
}

/**
 * 联邦搜索：本端 + 各在线对端**并发**查询，合并去重并标注来源。
 * 任一端失败只降级，不影响其它端结果。
 */
export async function searchFederated(query: string, opts: FederatedOptions = {}): Promise<FederatedResult> {
  const kind = opts.kind ?? "fulltext";
  const limit = opts.limit ?? 50;
  const timeoutMs = opts.timeoutMs ?? 2_000;
  const q = query.trim();
  if (!q) return { items: [], peerStatuses: [] };

  const peers =
    opts.peers ??
    (await (async () => {
      try {
        const res = await fetchProvider(`${peerlinkBaseProvider().replace(/\/+$/, "")}/peers`, {
          method: "GET",
          headers: OPERATOR_HEADER,
        });
        if (!res.ok) return [] as { id: string; name: string; online: boolean }[];
        const d = (await res.json()) as { items?: { id: string; name: string; online?: boolean }[] };
        return (d.items ?? []).map(p => ({ id: p.id, name: p.name, online: !!p.online }));
      } catch {
        return [] as { id: string; name: string; online: boolean }[];
      }
    })());

  const tasks: Promise<{ source: HitSource; peerId: string; peerName: string; hits: FederatedHit[] }>[] = [];

  if (!opts.skipLocal) {
    tasks.push(
      localSearchProvider(q, kind, limit)
        .then(list => ({
          source: "local" as HitSource,
          peerId: "local",
          peerName: "",
          hits: (list ?? []).map(r => asHit(r, "local", "local", "")).filter(Boolean) as FederatedHit[],
        }))
        .catch(() => ({ source: "local" as HitSource, peerId: "local", peerName: "", hits: [] as FederatedHit[] }))
    );
  }

  const base = peerlinkBaseProvider().replace(/\/+$/, "");
  for (const p of peers.filter(x => x.online)) {
    const url = `${base}/search?peerId=${encodeURIComponent(p.id)}&q=${encodeURIComponent(q)}&kind=${encodeURIComponent(kind)}&limit=${limit}`;
    tasks.push(
      fetchWithTimeout(url, timeoutMs)
        .then(async res => {
          if (!res.ok) {
            const err = new Error(`peer search failed: ${res.status}`) as Error & { httpStatus?: number };
            err.httpStatus = res.status;
            throw err;
          }
          const d = (await res.json()) as { items?: unknown[] };
          return {
            source: "peer" as HitSource,
            peerId: p.id,
            peerName: p.name,
            hits: (d.items ?? []).map(r => asHit(r, "peer", p.id, p.name)).filter(Boolean) as FederatedHit[],
          };
        })
        .catch(e => {
          const err = e as Error & { httpStatus?: number; name?: string };
          throw Object.assign(err, { peerId: p.id, peerName: p.name, httpStatus: err.httpStatus });
        })
    );
  }

  const settled = await Promise.allSettled(tasks);
  const items: FederatedHit[] = [];
  const seen = new Set<string>();
  const peerStatuses: PeerSearchStatus[] = [];

  for (const s of settled) {
    if (s.status === "fulfilled") {
      for (const h of s.value.hits) {
        if (seen.has(h.key)) continue; // 同 peer 内去重（不同 peer 的同路径是不同引用，保留）
        seen.add(h.key);
        items.push(h);
      }
      if (s.value.source === "peer") {
        peerStatuses.push({ peerId: s.value.peerId, name: s.value.peerName, state: "ok" });
      }
      continue;
    }
    const e = s.reason as Error & { peerId?: string; peerName?: string; httpStatus?: number };
    if (!e?.peerId) continue; // 本端失败不计入 peerStatuses
    const state: PeerSearchState =
      e.httpStatus === 503 ? "offline" : e.httpStatus === 504 ? "timeout" : e.name === "AbortError" ? "timeout" : "error";
    peerStatuses.push({ peerId: e.peerId, name: e.peerName ?? "", state, httpStatus: e.httpStatus, detail: e.message });
  }

  return { items, peerStatuses };
}

/** 组件内消费 */
export function useFederatedSearch() {
  const searching = ref(false);
  const lastError = ref("");

  async function run(query: string, opts: FederatedOptions = {}): Promise<FederatedResult> {
    searching.value = true;
    lastError.value = "";
    try {
      return await searchFederated(query, opts);
    } catch (e) {
      lastError.value = e instanceof Error ? e.message : String(e);
      return { items: [], peerStatuses: [] };
    } finally {
      searching.value = false;
    }
  }

  return { searching, lastError, run, searchFederated };
}
