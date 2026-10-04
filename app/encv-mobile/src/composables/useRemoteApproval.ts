/**
 * useRemoteApproval —— P4：**执行端**远程 Agent 审批（spec desktop-web-android-pairing）
 *
 * ⚠️ 语义（与既有会话级授权严格区分）：
 *   - 审批发生在**执行端**（谁的文件谁点头）；Hub / 发起端只搬运，不得代批。
 *   - 默认**每次都弹窗**；`trust_device` = 信任该设备（**进程级，本端服务重启后失效**）。
 *   - 破坏性工具即使已信任也强制确认（后端同样强制，前端只做展示与提示）。
 *   - 挂起超时由后端自动 decline（90s），**绝不默认同意**；这里只负责把倒计时显示出来。
 *
 * 数据来源：`GET /api/peerlink/agent/pending`（轮询，运维头）
 * 决策提交：`POST /api/peerlink/agent/approve`
 */

import { computed, ref } from "vue";
import { usePoll } from "@encv/shared-components/composables/usePoll";

export type RemoteDecision = "accept" | "decline" | "cancel" | "trust_device";

export interface RemoteApprovalRequest {
  callId: string;
  peerId: string;
  peerName: string;
  tool: string;
  destructive: boolean;
  createdAt: string;
  expiresAt: string;
}

const OPERATOR_HEADER = { "X-Peerlink-Operator": "1", "Content-Type": "application/json" };

// ── 注入点（测试 / 插件环境）：与 usePeerLink / useFederatedSearch 同款 ────
let fetchProvider: typeof fetch = (...args: Parameters<typeof fetch>) => fetch(...args);
export function setRemoteApprovalFetchProvider(fn: typeof fetch) {
  fetchProvider = fn;
}
export function __resetRemoteApprovalFetchProviderForTests() {
  fetchProvider = (...args: Parameters<typeof fetch>) => fetch(...args);
}

const pending = ref<RemoteApprovalRequest[]>([]);
const busy = ref(false);
const lastError = ref("");
/** 已提交决策但轮询还没刷新掉的 callId（避免重复点击/闪烁） */
const submitted = ref<Set<string>>(new Set());

let pollStarted = false;
let pollCtl: { start: () => void; stop: () => void } | null = null;

async function refreshPending(): Promise<void> {
  try {
    const res = await fetchProvider("/api/peerlink/agent/pending", { method: "GET", headers: OPERATOR_HEADER });
    // 未启用 / 未配对 → 静默降级为空（不打扰用户）
    if (!res.ok) {
      if (res.status !== 401) lastError.value = `pending_failed:${res.status}`;
      pending.value = [];
      return;
    }
    const data = (await res.json()) as { items?: RemoteApprovalRequest[] };
    const items = (data.items ?? []).filter(it => it && it.callId && !submitted.value.has(it.callId));
    pending.value = items;
    lastError.value = "";
  } catch (e) {
    // 网络/解析异常：静默降级，绝不让弹窗链路把 UI 拖崩
    lastError.value = e instanceof Error ? e.message : String(e);
    pending.value = [];
  }
}

/** 当前要展示的那一条（一次只弹一个，避免弹窗堆叠） */
const current = computed<RemoteApprovalRequest | null>(() => pending.value[0] ?? null);
/** 排队中的其余条数 */
const queued = computed(() => Math.max(0, pending.value.length - 1));

async function decide(callId: string, decision: RemoteDecision): Promise<boolean> {
  busy.value = true;
  submitted.value.add(callId);
  // 乐观移除：避免轮询抖动导致重复弹同一个
  pending.value = pending.value.filter(p => p.callId !== callId);
  try {
    const res = await fetchProvider("/api/peerlink/agent/approve", {
      method: "POST",
      headers: OPERATOR_HEADER,
      body: JSON.stringify({ callId, decision }),
    });
    if (!res.ok) {
      // 失败要从 submitted 里摘掉，允许重试
      submitted.value.delete(callId);
      lastError.value = `approve_failed:${res.status}`;
      return false;
    }
    lastError.value = "";
    return true;
  } catch (e) {
    submitted.value.delete(callId);
    lastError.value = e instanceof Error ? e.message : String(e);
    return false;
  } finally {
    busy.value = false;
    void refreshPending();
  }
}

// ── 轮询生命周期（全局单例：在 Tabs 挂载一次） ────────────────────────

const POLL_INTERVAL_MS = 2000;

export function startRemoteApprovalPolling(): void {
  if (pollStarted) return;
  pollStarted = true;
  pollCtl = usePoll(refreshPending, { intervalMs: POLL_INTERVAL_MS, immediate: true });
  pollCtl.start();
}

export function stopRemoteApprovalPolling(): void {
  pollCtl?.stop();
  pollCtl = null;
  pollStarted = false;
}

export function useRemoteApproval() {
  return { pending, current, queued, busy, lastError, decide, refreshPending };
}

/** 测试用：重置模块级状态 */
export function __resetRemoteApprovalForTests() {
  pending.value = [];
  busy.value = false;
  lastError.value = "";
  submitted.value = new Set();
}
