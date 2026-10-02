/**
 * useRemoteApproval 单测 —— spec desktop-web-android-pairing P4（执行端审批）
 *
 * 锁定：
 *  1. 挂起请求能被拉取并作为"当前要弹的那一条"（一次只弹一个，其余排队）
 *  2. 决策提交带正确的 callId + decision（trust_device / accept / decline）
 *  3. **未授权/未启用（401）静默降级为空**，绝不打扰用户、绝不抛错
 *  4. 已决策的 callId 不再重复弹出（防抖/防重复点击）
 */

import { beforeEach, describe, expect, it } from "vitest";
import {
  __resetRemoteApprovalForTests,
  __resetRemoteApprovalFetchProviderForTests,
  type RemoteApprovalRequest,
  setRemoteApprovalFetchProvider,
  useRemoteApproval,
} from "../useRemoteApproval";

function req(over: Partial<RemoteApprovalRequest> = {}): RemoteApprovalRequest {
  return {
    callId: "c1",
    peerId: "peer-x",
    peerName: "Desktop",
    tool: "encrypt_video",
    destructive: true,
    createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 80_000).toISOString(),
    ...over,
  };
}

function mockFetch(handler: (url: string, init?: RequestInit) => { status: number; body: unknown }) {
  const calls: { url: string; init?: RequestInit }[] = [];
  setRemoteApprovalFetchProvider(((url: string, init?: RequestInit) => {
    calls.push({ url, init });
    const r = handler(url, init);
    return Promise.resolve({
      ok: r.status >= 200 && r.status < 300,
      status: r.status,
      json: () => Promise.resolve(r.body),
    } as Response);
  }) as typeof fetch);
  return calls;
}

describe("useRemoteApproval（P4 执行端审批）", () => {
  beforeEach(() => {
    __resetRemoteApprovalForTests();
    __resetRemoteApprovalFetchProviderForTests();
  });

  it("拉取挂起请求 → 当前一条 + 排队数", async () => {
    mockFetch(() => ({ status: 200, body: { items: [req({ callId: "c1" }), req({ callId: "c2" })] } }));
    const { refreshPending, current, queued } = useRemoteApproval();
    await refreshPending();
    expect(current.value?.callId).toBe("c1");
    expect(queued.value).toBe(1);
  });

  it("决策提交带 callId + decision", async () => {
    const calls = mockFetch(() => ({ status: 200, body: { ok: true } }));
    const { decide } = useRemoteApproval();
    const ok = await decide("c9", "trust_device");
    expect(ok).toBe(true);
    // 决策后会自动补一次 pending 刷新，故按 URL 过滤出 approve 调用
    const approve = calls.filter(c => c.url.includes("/approve"));
    expect(approve).toHaveLength(1);
    const body = JSON.parse(String(approve[0].init?.body));
    expect(body).toEqual({ callId: "c9", decision: "trust_device" });
  });

  it("401 未授权 → 静默降级为空（不抛错）", async () => {
    mockFetch(() => ({ status: 401, body: {} }));
    const { refreshPending, pending } = useRemoteApproval();
    await expect(refreshPending()).resolves.toBeUndefined();
    expect(pending.value).toEqual([]);
  });

  it("已决策的 callId 不再重复弹出", async () => {
    mockFetch(url =>
      url.includes("/approve")
        ? { status: 200, body: { ok: true } }
        : { status: 200, body: { items: [req({ callId: "c1" })] } },
    );
    const { decide, refreshPending, current } = useRemoteApproval();
    await decide("c1", "accept");
    await refreshPending();
    expect(current.value).toBeNull();
  });
});
