/**
 * useFederatedSearch.test.ts —— 联邦搜索契约（spec desktop-web-android-pairing P3）
 *
 * 覆盖：
 *  1. 本端 + 对端**并发**合并，来源标注正确（local / peer + peerName）
 *  2. 去重键 = `peerId#path`：同 peer 内去重，**不同 peer 的同路径保留两条**（跨端引用）
 *  3. 对端离线(503) → state=offline，不抛错、不影响本端结果（降级）
 *  4. 对端超时(abort) → state=timeout
 *  5. 本端搜索失败 → 仍返回对端结果
 *  6. 空查询 → 直接空结果
 */

import { beforeEach, describe, expect, it } from "vitest";
import {
  __resetFederatedSearchProvidersForTests,
  searchFederated,
  setFederatedSearchProviders,
} from "@encv/shared-components/composables/useFederatedSearch";

function jsonResponse(status: number, body: unknown): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body } as unknown as Response;
}

const PEERS = [{ id: "peer-a", name: "Pixel", online: true }];

beforeEach(() => {
  __resetFederatedSearchProvidersForTests();
});

describe("合并与来源标注", () => {
  it("本端 + 对端结果合并，分别标注 source/peerName", async () => {
    setFederatedSearchProviders({
      localSearch: async () => [{ path: "/Movies/a.mp4", name: "a.mp4", size: 10 }],
      fetch: ((url: string) => {
        if (url.includes("/search")) {
          return Promise.resolve(jsonResponse(200, { items: [{ path: "/sdcard/b.pdf", name: "b.pdf" }] }));
        }
        return Promise.resolve(jsonResponse(200, { items: [] }));
      }) as unknown as typeof fetch,
    });

    const res = await searchFederated("报告", { peers: PEERS, skipLocal: false });
    expect(res.items).toHaveLength(2);
    const local = res.items.find(i => i.source === "local");
    const peer = res.items.find(i => i.source === "peer");
    expect(local?.peerId).toBe("local");
    expect(peer?.peerId).toBe("peer-a");
    expect(peer?.peerName).toBe("Pixel");
    expect(peer?.path).toBe("/sdcard/b.pdf"); // 远端路径原样保留（不伪装成本地路径）
    expect(res.peerStatuses[0]).toMatchObject({ peerId: "peer-a", state: "ok" });
  });

  it("去重键含 peerId：不同 peer 的同名路径保留两条", async () => {
    setFederatedSearchProviders({
      localSearch: async () => [],
      fetch: ((url: string) => {
        if (url.includes("peerId=peer-a"))
          return Promise.resolve(jsonResponse(200, { items: [{ path: "/x/y.txt" }, { path: "/x/y.txt" }] }));
        if (url.includes("peerId=peer-b")) return Promise.resolve(jsonResponse(200, { items: [{ path: "/x/y.txt" }] }));
        return Promise.resolve(jsonResponse(200, { items: [] }));
      }) as unknown as typeof fetch,
    });

    const res = await searchFederated("y", {
      peers: [
        { id: "peer-a", name: "A", online: true },
        { id: "peer-b", name: "B", online: true },
      ],
      skipLocal: true,
    });
    // peer-a 内部重复被去掉（1 条），peer-b 的同路径是另一份引用（再 1 条）
    expect(res.items).toHaveLength(2);
    expect(res.items.map(i => i.key).sort()).toEqual(["peer-a#/x/y.txt", "peer-b#/x/y.txt"]);
  });
});

describe("降级语义", () => {
  it("对端离线 503 → state=offline，本端结果不受影响", async () => {
    setFederatedSearchProviders({
      localSearch: async () => [{ path: "/local/1.mp4" }],
      fetch: (() => Promise.resolve(jsonResponse(503, { error: "peer_offline" }))) as unknown as typeof fetch,
    });
    const res = await searchFederated("x", { peers: PEERS });
    expect(res.items).toHaveLength(1);
    expect(res.items[0].source).toBe("local");
    expect(res.peerStatuses[0]).toMatchObject({ state: "offline", httpStatus: 503 });
  });

  it("对端超时（504）→ state=timeout", async () => {
    setFederatedSearchProviders({
      localSearch: async () => [],
      fetch: (() => Promise.resolve(jsonResponse(504, { error: "peer_timeout" }))) as unknown as typeof fetch,
    });
    const res = await searchFederated("x", { peers: PEERS, skipLocal: true });
    expect(res.items).toHaveLength(0);
    expect(res.peerStatuses[0]).toMatchObject({ state: "timeout", httpStatus: 504 });
  });

  it("本端搜索失败 → 仍返回对端结果（不整体失败）", async () => {
    setFederatedSearchProviders({
      localSearch: async () => {
        throw new Error("local down");
      },
      fetch: (() => Promise.resolve(jsonResponse(200, { items: [{ path: "/remote/z.pdf" }] }))) as unknown as typeof fetch,
    });
    const res = await searchFederated("z", { peers: PEERS });
    expect(res.items).toHaveLength(1);
    expect(res.items[0].source).toBe("peer");
  });

  it("空查询 → 空结果且不发请求", async () => {
    let calls = 0;
    setFederatedSearchProviders({
      localSearch: async () => {
        calls++;
        return [];
      },
      fetch: (() => {
        calls++;
        return Promise.resolve(jsonResponse(200, { items: [] }));
      }) as unknown as typeof fetch,
    });
    const res = await searchFederated("   ", { peers: PEERS });
    expect(res.items).toHaveLength(0);
    expect(calls).toBe(0);
  });
});
