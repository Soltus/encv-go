/**
 * usePeerDegradation —— P5 / Task 5.5：降级矩阵**持久性** UI 的数据层
 * (spec desktop-web-android-pairing)
 *
 * ⚠️ 为什么不是 Toast：对端离线 / Hub 漂移 / token 失效都是**持续状态**，
 *    Toast 一闪而过会让用户以为"刚才那次失败了"，而不是"这个功能现在不可用"。
 *    必须以**常驻条幅**呈现，状态恢复后自动消失。
 *
 * 降级矩阵：
 *   - peer_offline    ：对端离线 → 跨端搜索/远程调试暂不可用
 *   - hub_disconnected：本端 Edge 未连上 Hub（重连中）→ 对端看不到我
 *   - hub_not_paired  ：本端从未作为 Edge 配对（或重启后 token 失效）→ 需重新扫码
 *   - rate_limited    ：被限流（R12）→ 稍后自动恢复
 *   - circuit_open    ：对端连续失败已熔断（R12）→ 冷却后自动重试
 */

import { computed, ref } from "vue";
import { usePoll } from "@encv/shared-components/composables/usePoll";

export type DegradeKind = "peer_offline" | "hub_disconnected" | "hub_not_paired" | "rate_limited" | "circuit_open";

export interface DegradeNotice {
  kind: DegradeKind;
  /** 涉及的对端名（可为空：本端自身状态） */
  peerName: string;
  detail?: string;
}

const OPERATOR_HEADER = { "X-Peerlink-Operator": "1" };

const peers = ref<{ id: string; name: string; online: boolean }[]>([]);
const edge = ref<{ running: boolean; connected: boolean } | null>(null);
const lastError = ref<{ kind: DegradeKind; detail?: string } | null>(null);

let started = false;
let ctl: { start: () => void; stop: () => void } | null = null;

async function refresh(): Promise<void> {
  // 对端列表
  try {
    const r = await fetch("/api/peerlink/peers", { method: "GET", headers: OPERATOR_HEADER });
    if (r.ok) {
      const d = (await r.json()) as { items?: { id: string; name: string; online?: boolean }[] };
      peers.value = (d.items ?? []).map(p => ({ id: p.id, name: p.name, online: !!p.online }));
    } else {
      peers.value = [];
    }
  } catch {
    peers.value = [];
  }
  // 本端作为 Edge 的连接状态
  try {
    const r2 = await fetch("/api/peerlink/edge/status", { method: "GET", headers: OPERATOR_HEADER });
    if (r2.ok) {
      const d2 = (await r2.json()) as { running?: boolean; connected?: boolean };
      edge.value = { running: !!d2.running, connected: !!d2.connected };
    } else {
      edge.value = null;
    }
  } catch {
    edge.value = null;
  }
}

const notices = computed<DegradeNotice[]>(() => {
  const out: DegradeNotice[] = [];
  for (const p of peers.value) {
    if (!p.online) {
      out.push({ kind: "peer_offline", peerName: p.name || p.id });
    }
  }
  const e = edge.value;
  if (e && e.running && !e.connected) {
    out.push({ kind: "hub_disconnected", peerName: "" });
  }
  // 有对端配对过、但本端从未作为 Edge 连接 ⇒ 说明"我"没连上 Hub（重启后 token 失效的典型表现）
  if (peers.value.length > 0 && (!e || !e.running)) {
    out.push({ kind: "hub_not_paired", peerName: "" });
  }
  if (lastError.value) {
    out.push({ kind: lastError.value.kind, peerName: "", detail: lastError.value.detail });
  }
  return out;
});

export function startPeerDegradationPolling(): void {
  if (started) return;
  started = true;
  ctl = usePoll(refresh, { intervalMs: 10_000, immediate: true });
  ctl.start();
}

export function stopPeerDegradationPolling(): void {
  ctl?.stop();
  ctl = null;
  started = false;
}

/** 由调用方在遇到 429 / 熔断时上报（持久显示，恢复后调用 clearTransient） */
export function reportPeerDegradation(kind: DegradeKind, detail?: string): void {
  lastError.value = { kind, detail };
}

export function clearPeerDegradation(): void {
  lastError.value = null;
}

export function usePeerDegradation() {
  return { notices, peers, edge, refresh, reportPeerDegradation, clearPeerDegradation };
}
