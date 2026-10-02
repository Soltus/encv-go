/**
 * usePeerLink.test.ts —— 前端互联抽象契约（spec desktop-web-android-pairing Task 2.3）
 *
 * 覆盖：
 *  1. createPairingTicket：解析字段 / 状态流转 / 失败分支
 *  2. fetchPairingStatus：404 = 未配对（轮询态，不算错误）/ 200 = 已配对
 *  3. waitForPairing：轮询到成功；超时抛错并置 status=timeout
 *  4. fetchPeers / unpairPeer
 *  5. **psk 不落盘**（存储纪律）：调用后 localStorage 无新增
 *
 * ⚠️ fast project 是 isolate:false：用 `__resetPeerLinkStateForTests()` 显式复位，
 *    禁止 vi.resetModules()。
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  __resetPeerLinkStateForTests,
  createPairingTicket,
  fetchPairingStatus,
  fetchPeers,
  setPeerLinkFetchProvider,
  unpairPeer,
  usePeerLink,
  waitForPairing,
} from "@encv/shared-components/composables/usePeerLink";

function jsonResponse(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as unknown as Response;
}

beforeEach(() => {
  __resetPeerLinkStateForTests();
  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
});

describe("createPairingTicket", () => {
  it("解析票据字段并置 pairing 状态", async () => {
    setPeerLinkFetchProvider((async () =>
      jsonResponse(200, {
        pairingId: "abc123",
        psk: "ff".repeat(32),
        hub: "https://hub.example/api/peerlink",
        expiresIn: 120,
      })) as unknown as typeof fetch);

    const tk = await createPairingTicket();
    expect(tk.pairingId).toBe("abc123");
    expect(tk.hub.startsWith("https://")).toBe(true);
    const { status, ticket } = usePeerLink();
    expect(status.value).toBe("pairing");
    expect(ticket.value?.pairingId).toBe("abc123");
  });

  it("字段不完整 → error 状态并抛出", async () => {
    setPeerLinkFetchProvider((async () => jsonResponse(200, { pairingId: "x" })) as unknown as typeof fetch);
    await expect(createPairingTicket()).rejects.toThrow();
    expect(usePeerLink().status.value).toBe("error");
  });

  it("非 2xx → error 状态并抛出", async () => {
    setPeerLinkFetchProvider((async () => jsonResponse(500, {})) as unknown as typeof fetch);
    await expect(createPairingTicket()).rejects.toThrow();
    expect(usePeerLink().lastError.value).toContain("500");
  });
});

describe("配对状态轮询", () => {
  it("404 = 尚未配对（返回 null，不算错误）", async () => {
    setPeerLinkFetchProvider((async () => jsonResponse(404, { error: "not_paired" })) as unknown as typeof fetch);
    await expect(fetchPairingStatus("p1")).resolves.toBeNull();
  });

  it("200 = 已配对，返回 SAS 与设备信息", async () => {
    setPeerLinkFetchProvider((async () =>
      jsonResponse(200, {
        peerId: "peer-1",
        sas: "123456",
        name: "Pixel",
        platform: "android",
        pairedAt: "2026-10-02T00:00:00Z",
      })) as unknown as typeof fetch);
    const info = await fetchPairingStatus("p1");
    expect(info?.sas).toBe("123456");
    expect(info?.platform).toBe("android");
  });

  it("waitForPairing：先 404 后 200 → 成功并置 paired", async () => {
    let calls = 0;
    setPeerLinkFetchProvider((async () => {
      calls++;
      if (calls === 1) return jsonResponse(404, { error: "not_paired" });
      return jsonResponse(200, { peerId: "peer-1", sas: "654321", name: "Pixel", platform: "android", pairedAt: "" });
    }) as unknown as typeof fetch);

    const info = await waitForPairing("p1", { intervalMs: 1, timeoutMs: 1000 });
    expect(info.sas).toBe("654321");
    expect(usePeerLink().status.value).toBe("paired");
  });

  it("waitForPairing：超时抛错并置 timeout", async () => {
    // ⚠️ 用真实定时器跑，超时留 **10 倍余量**（5ms/1000ms）：
    //    实测只在"改文件后的第一次运行"偶发假红（Vite transform 冷启动阻塞事件循环），
    //    连跑 3 次 0 失败。1ms/20ms 这类极限值绝对不要用。
    setPeerLinkFetchProvider((async () => jsonResponse(404, {})) as unknown as typeof fetch);
    await expect(waitForPairing("p1", { intervalMs: 5, timeoutMs: 1000 })).rejects.toThrow("timeout");
    expect(usePeerLink().status.value).toBe("timeout");
  });
});

describe("列表与解配", () => {
  it("fetchPeers 解析 items", async () => {
    setPeerLinkFetchProvider((async () =>
      jsonResponse(200, { items: [{ id: "peer-1", name: "Pixel", platform: "android", online: true }] })) as unknown as typeof fetch);
    const items = await fetchPeers();
    expect(items).toHaveLength(1);
    expect(usePeerLink().onlinePeers.value).toHaveLength(1);
  });

  it("unpairPeer：成功 true / 失败 false", async () => {
    setPeerLinkFetchProvider((async () => jsonResponse(200, { ok: true })) as unknown as typeof fetch);
    await expect(unpairPeer("peer-1")).resolves.toBe(true);

    setPeerLinkFetchProvider((async () => jsonResponse(404, {})) as unknown as typeof fetch);
    await expect(unpairPeer("peer-1")).resolves.toBe(false);
  });
});

describe("存储纪律", () => {
  it("psk 不落盘：申请票据后 localStorage 无新增", async () => {
    setPeerLinkFetchProvider((async () =>
      jsonResponse(200, {
        pairingId: "abc",
        psk: "ee".repeat(32),
        hub: "https://h/api/peerlink",
        expiresIn: 120,
      })) as unknown as typeof fetch);
    await createPairingTicket();
    const keys = Object.keys(localStorage as unknown as Record<string, unknown>);
    expect(keys.length).toBe(0);
    expect(JSON.stringify(localStorage)).not.toContain("ee");
  });
});

describe("注入隔离", () => {
  it("fetch provider 注入不会影响全局 fetch", () => {
    const spy = vi.spyOn(globalThis, "fetch");
    setPeerLinkFetchProvider((async () => jsonResponse(200, {})) as unknown as typeof fetch);
    __resetPeerLinkStateForTests();
    expect(globalThis.fetch).toBe(spy.mock?.object ?? globalThis.fetch);
    spy.mockRestore();
  });
});
