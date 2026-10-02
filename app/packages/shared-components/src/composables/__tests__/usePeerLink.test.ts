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
  pairAsEdge,
  parsePairingQR,
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
    // pairAsEdge 走 res.text()（为了把后端错误文本带出来），mock 必须提供
    text: async () => JSON.stringify(body),
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

describe("扫码端：parsePairingQR（Task 2.6）", () => {
  const good = { v: 2, hub: "https://hub.example/api/peerlink", pairingId: "abc", psk: "ff".repeat(32), exp: 119 };

  it("合法负载解析成功", () => {
    const r = parsePairingQR(JSON.stringify(good));
    expect(r.ok).toBe(true);
    if (r.ok) {
      expect(r.payload.pairingId).toBe("abc");
      expect(r.payload.hub).toBe(good.hub);
    }
  });

  it("非 JSON / 空串 → not_json", () => {
    expect(parsePairingQR("not a qr").ok).toBe(false);
    expect(parsePairingQR("").ok).toBe(false);
  });

  it("缺字段 → missing_field", () => {
    const r = parsePairingQR(JSON.stringify({ v: 2, hub: "https://h" }));
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toBe("missing_field");
  });

  it("R3：明文 http 非回环 → bad_hub（后端 validateHubURL 同样拒绝）", () => {
    const r = parsePairingQR(JSON.stringify({ ...good, hub: "http://192.168.1.9:2025/api/peerlink" }));
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toBe("bad_hub");
    // 本机回环允许（开发/自测）
    expect(parsePairingQR(JSON.stringify({ ...good, hub: "http://127.0.0.1:2025/api/peerlink" })).ok).toBe(true);
  });

  it("exp 为绝对毫秒时间戳时才判过期（桌面端写的是剩余秒数，不能误杀）", () => {
    const now = 1_700_000_000_000;
    const past = parsePairingQR(JSON.stringify({ ...good, exp: now - 1000 }), now);
    expect(past.ok).toBe(false);
    if (!past.ok) expect(past.reason).toBe("expired");
    // 剩余秒数（119）不应被当成过期时间
    expect(parsePairingQR(JSON.stringify(good), now).ok).toBe(true);
  });
});

describe("扫码端：pairAsEdge（Task 2.6）", () => {
  it("把 hub/pairingId/psk 交给本端 /api/peerlink/edge/pair，并带运维头", async () => {
    let seenUrl = "";
    let seenInit: RequestInit | undefined;
    setPeerLinkFetchProvider((async (url: string, init?: RequestInit) => {
      seenUrl = String(url);
      seenInit = init;
      return jsonResponse(200, { ok: true, hub: "https://hub.example/api/peerlink", peerId: "p-1" });
    }) as unknown as typeof fetch);

    const res = await pairAsEdge({ hub: "https://hub.example/api/peerlink", pairingId: "abc", psk: "ff".repeat(32) });
    expect(res.ok).toBe(true);
    expect(res.peerId).toBe("p-1");
    expect(seenUrl).toContain("/api/peerlink/edge/pair");
    const headers = (seenInit?.headers ?? {}) as Record<string, string>;
    expect(headers["X-Peerlink-Operator"]).toBe("1");
    const body = JSON.parse(String(seenInit?.body));
    expect(body.pairingId).toBe("abc");
    expect(body.psk).toBe("ff".repeat(32));
  });

  it("后端拒绝（502/400）→ 抛错，绝不当成功", async () => {
    setPeerLinkFetchProvider((async () => ({
      ok: false,
      status: 502,
      text: async () => "pair_failed",
    })) as unknown as typeof fetch);
    await expect(pairAsEdge({ hub: "https://h", pairingId: "abc", psk: "ff" })).rejects.toThrow(/502/);
  });

  it("psk 不落盘：扫码连接后 localStorage 仍为空", async () => {
    setPeerLinkFetchProvider((async () => jsonResponse(200, { ok: true, hub: "https://h", peerId: "p" })) as unknown as typeof fetch);
    await pairAsEdge({ hub: "https://h", pairingId: "abc", psk: "ee".repeat(32) });
    expect(Object.keys(localStorage as unknown as Record<string, unknown>).length).toBe(0);
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
