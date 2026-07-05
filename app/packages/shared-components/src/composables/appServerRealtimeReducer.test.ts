/**
 * appServerRealtimeReducer 单元测试
 *
 * 覆盖：
 *  1) asRecord —— 普通对象 / 数组 / null / 原始类型
 *  2) readString —— 字符串与非字符串
 *  3) readRealtimeThreadId —— 多种字段位置（顶层/conversationId/payload/params/approval）
 *  4) readRealtimeCacheVersion —— 数字 / 数字字符串 / 缺失
 *  5) readRealtimeServerInstance —— 仅 connected 事件返回 serverInstanceId
 *  6) updateRealtimeServerInstance —— instance 切换时清空 Map
 *  7) processRealtimeEvent —— sequence 去重 / accept 决策
 *  8) MAX_TRACKED_REALTIME_SEQUENCES —— 超过上限时淘汰最早 sequence
 */

import {
  asRecord,
  createRealtimeSequenceTrackerState,
  MAX_TRACKED_REALTIME_SEQUENCES,
  type MinimalRealtimeEvent,
  processRealtimeEvent,
  type RealtimeSequenceTrackerState,
  readRealtimeCacheVersion,
  readRealtimeServerInstance,
  readRealtimeThreadId,
  readString,
  updateRealtimeServerInstance,
} from "@encv/shared-components/composables/appServerRealtimeReducer";
import { describe, expect, it } from "vitest";

// =============================================================================
// asRecord
// =============================================================================

describe("asRecord", () => {
  it("returns null for null / undefined", () => {
    expect(asRecord(null)).toBeNull();
    expect(asRecord(undefined)).toBeNull();
  });

  it("returns null for primitives", () => {
    expect(asRecord(42)).toBeNull();
    expect(asRecord("hello")).toBeNull();
    expect(asRecord(true)).toBeNull();
  });

  it("returns null for arrays (not plain object)", () => {
    expect(asRecord([])).toBeNull();
    expect(asRecord([1, 2, 3])).toBeNull();
  });

  it("returns the object for plain objects", () => {
    const obj = { a: 1 };
    expect(asRecord(obj)).toBe(obj);
    expect(asRecord({})).toEqual({});
  });
});

// =============================================================================
// readString
// =============================================================================

describe("readString", () => {
  it("returns the string for string input", () => {
    expect(readString("hello")).toBe("hello");
    expect(readString("")).toBe("");
  });

  it("returns empty string for non-string input", () => {
    expect(readString(42)).toBe("");
    expect(readString(null)).toBe("");
    expect(readString(undefined)).toBe("");
    expect(readString({})).toBe("");
    expect(readString([])).toBe("");
    expect(readString(true)).toBe("");
  });
});

// =============================================================================
// readRealtimeThreadId
// =============================================================================

describe("readRealtimeThreadId", () => {
  it("reads from event.threadId first", () => {
    const event: MinimalRealtimeEvent = {
      type: "message",
      threadId: "thread-A",
      conversationId: "thread-B",
      payload: { threadId: "thread-C" },
    };
    expect(readRealtimeThreadId(event)).toBe("thread-A");
  });

  it("falls back to event.conversationId", () => {
    const event: MinimalRealtimeEvent = {
      type: "message",
      conversationId: "thread-conv",
    };
    expect(readRealtimeThreadId(event)).toBe("thread-conv");
  });

  it("falls back to payload.threadId", () => {
    const event: MinimalRealtimeEvent = {
      type: "message",
      payload: { threadId: "thread-payload" },
    };
    expect(readRealtimeThreadId(event)).toBe("thread-payload");
  });

  it("falls back to params.threadId", () => {
    const event: MinimalRealtimeEvent = {
      type: "message",
      params: { threadId: "thread-params" },
    };
    expect(readRealtimeThreadId(event)).toBe("thread-params");
  });

  it("falls back to approval.threadId", () => {
    const event: MinimalRealtimeEvent = {
      type: "message",
      approval: { threadId: "thread-approval" },
    };
    expect(readRealtimeThreadId(event)).toBe("thread-approval");
  });

  it("returns empty string when no threadId is present", () => {
    expect(readRealtimeThreadId({ type: "message" })).toBe("");
    expect(readRealtimeThreadId({ type: "message", threadId: "" })).toBe("");
    expect(readRealtimeThreadId({ type: "message", threadId: 42 } as unknown as MinimalRealtimeEvent)).toBe("");
  });

  it("ignores non-string threadId values in nested containers", () => {
    const event: MinimalRealtimeEvent = {
      payload: { threadId: 42 as unknown as string },
      params: { threadId: null as unknown as string },
    };
    expect(readRealtimeThreadId(event)).toBe("");
  });
});

// =============================================================================
// readRealtimeCacheVersion
// =============================================================================

describe("readRealtimeCacheVersion", () => {
  it("reads numeric cacheVersion from payload", () => {
    const event: MinimalRealtimeEvent = {
      type: "message",
      payload: { cacheVersion: 17 },
    };
    expect(readRealtimeCacheVersion(event)).toBe(17);
  });

  it("parses numeric string cacheVersion from payload", () => {
    const event: MinimalRealtimeEvent = {
      type: "message",
      payload: { cacheVersion: "42" },
    };
    expect(readRealtimeCacheVersion(event)).toBe(42);
  });

  it("falls back to params / approval / top-level", () => {
    expect(readRealtimeCacheVersion({ params: { cacheVersion: 5 } })).toBe(5);
    expect(readRealtimeCacheVersion({ approval: { cacheVersion: 7 } })).toBe(7);
    expect(readRealtimeCacheVersion({ cacheVersion: 9 })).toBe(9);
  });

  it("returns null when no cacheVersion is present", () => {
    expect(readRealtimeCacheVersion({ type: "message" })).toBeNull();
  });

  it("returns null for NaN / Infinity / unparseable strings", () => {
    expect(readRealtimeCacheVersion({ payload: { cacheVersion: NaN } })).toBeNull();
    expect(readRealtimeCacheVersion({ payload: { cacheVersion: Infinity } })).toBeNull();
    expect(readRealtimeCacheVersion({ payload: { cacheVersion: "not-a-number" } })).toBeNull();
  });

  it("payload takes priority over top-level", () => {
    const event: MinimalRealtimeEvent = {
      type: "message",
      payload: { cacheVersion: 1 },
      cacheVersion: 2,
    };
    expect(readRealtimeCacheVersion(event)).toBe(1);
  });
});

// =============================================================================
// readRealtimeServerInstance
// =============================================================================

describe("readRealtimeServerInstance", () => {
  it("returns serverInstanceId for type=connected", () => {
    expect(readRealtimeServerInstance({ type: "connected", serverInstanceId: "srv-1" })).toBe("srv-1");
  });

  it("returns empty string for non-connected events", () => {
    expect(readRealtimeServerInstance({ type: "message", serverInstanceId: "srv-1" })).toBe("");
    expect(readRealtimeServerInstance({ type: "stream_end", serverInstanceId: "srv-1" })).toBe("");
    expect(readRealtimeServerInstance({ type: "text_delta", serverInstanceId: "srv-1" })).toBe("");
  });

  it("returns empty string for connected event without serverInstanceId", () => {
    expect(readRealtimeServerInstance({ type: "connected" })).toBe("");
  });

  it("returns empty string for non-string serverInstanceId", () => {
    expect(
      readRealtimeServerInstance({
        type: "connected",
        serverInstanceId: 42 as unknown as string,
      })
    ).toBe("");
  });

  it("returns empty string for events without type", () => {
    expect(readRealtimeServerInstance({ serverInstanceId: "srv-1" })).toBe("");
  });
});

// =============================================================================
// updateRealtimeServerInstance
// =============================================================================

describe("updateRealtimeServerInstance", () => {
  it("returns same instance for non-connected event", () => {
    const versions = new Map<string, number>([["t1", 5]]);
    const result = updateRealtimeServerInstance(versions, "srv-A", {
      type: "message",
    });
    expect(result).toBe("srv-A");
    expect(versions.size).toBe(1);
    expect(versions.get("t1")).toBe(5);
  });

  it("returns same instance for same connected instance", () => {
    const versions = new Map<string, number>([["t1", 5]]);
    const result = updateRealtimeServerInstance(versions, "srv-A", {
      type: "connected",
      serverInstanceId: "srv-A",
    });
    expect(result).toBe("srv-A");
    expect(versions.size).toBe(1);
    expect(versions.get("t1")).toBe(5);
  });

  it("clears versions map when instance changes", () => {
    const versions = new Map<string, number>([
      ["t1", 5],
      ["t2", 9],
    ]);
    const result = updateRealtimeServerInstance(versions, "srv-A", {
      type: "connected",
      serverInstanceId: "srv-B",
    });
    expect(result).toBe("srv-B");
    expect(versions.size).toBe(0);
  });

  it("sets initial instance from connected event", () => {
    const versions = new Map<string, number>();
    const result = updateRealtimeServerInstance(versions, "", {
      type: "connected",
      serverInstanceId: "srv-X",
    });
    expect(result).toBe("srv-X");
    expect(versions.size).toBe(0);
  });
});

// =============================================================================
// processRealtimeEvent — sequence 去重
// =============================================================================

describe("processRealtimeEvent", () => {
  it("accepts event without sequence (no dedup)", () => {
    const state = createRealtimeSequenceTrackerState();
    const result = processRealtimeEvent(state, { type: "message" });
    expect(result.accept).toBe(true);
    expect(result.decision.accepted).toBe(true);
    expect(result.decision.threadId).toBe("");
    expect(result.decision.cacheVersion).toBeNull();
  });

  it("accepts first occurrence of a sequence", () => {
    const state = createRealtimeSequenceTrackerState();
    const result = processRealtimeEvent(state, {
      type: "message",
      sequence: 1,
      payload: { threadId: "t1", cacheVersion: 3 },
    });
    expect(result.accept).toBe(true);
    expect(result.decision.accepted).toBe(true);
    expect(result.decision.threadId).toBe("t1");
    expect(result.decision.cacheVersion).toBe(3);
    expect(state.seenSequences.has(1)).toBe(true);
  });

  it("rejects duplicate sequence", () => {
    const state = createRealtimeSequenceTrackerState();
    processRealtimeEvent(state, { type: "message", sequence: 1 });
    const result = processRealtimeEvent(state, { type: "message", sequence: 1 });
    expect(result.accept).toBe(false);
    expect(result.decision.accepted).toBe(false);
  });

  it("treats NaN / Infinity sequence as non-dedup (always accepted)", () => {
    const state = createRealtimeSequenceTrackerState();
    const r1 = processRealtimeEvent(state, { type: "message", sequence: NaN });
    const r2 = processRealtimeEvent(state, { type: "message", sequence: Infinity });
    expect(r1.accept).toBe(true);
    expect(r2.accept).toBe(true);
    expect(state.seenSequences.size).toBe(0);
  });

  it("clears seenSequences on server instance change", () => {
    const state = createRealtimeSequenceTrackerState("srv-A");
    processRealtimeEvent(state, { type: "message", sequence: 1 });
    processRealtimeEvent(state, { type: "message", sequence: 2 });
    expect(state.seenSequences.size).toBe(2);

    // 切换 server instance
    processRealtimeEvent(state, {
      type: "connected",
      serverInstanceId: "srv-B",
    });
    expect(state.serverInstance).toBe("srv-B");
    expect(state.seenSequences.size).toBe(0);

    // 原 sequence 现在应该可被重新接受
    const result = processRealtimeEvent(state, { type: "message", sequence: 1 });
    expect(result.accept).toBe(true);
  });

  it("returns empty threadId when event has none", () => {
    const state = createRealtimeSequenceTrackerState();
    const result = processRealtimeEvent(state, { type: "message", sequence: 9 });
    expect(result.decision.threadId).toBe("");
    expect(result.decision.cacheVersion).toBeNull();
  });
});

// =============================================================================
// MAX_TRACKED_REALTIME_SEQUENCES 容量淘汰
// =============================================================================

describe("MAX_TRACKED_REALTIME_SEQUENCES", () => {
  it("exports 2_000 as the constant", () => {
    expect(MAX_TRACKED_REALTIME_SEQUENCES).toBe(2_000);
  });

  it("keeps last 2_000 sequences when over capacity (drops oldest 1/4)", () => {
    const state = createRealtimeSequenceTrackerState();
    // 写入 2_000 个 + 1 个新值 → 触发淘汰
    for (let i = 1; i <= MAX_TRACKED_REALTIME_SEQUENCES; i++) {
      processRealtimeEvent(state, { type: "message", sequence: i });
    }
    expect(state.seenSequences.size).toBe(MAX_TRACKED_REALTIME_SEQUENCES);

    // 再加一个更新的 sequence
    processRealtimeEvent(state, { type: "message", sequence: MAX_TRACKED_REALTIME_SEQUENCES + 1 });
    // 容量上限触发：删掉最早的 1/4 = 500，保留 1_500 个老 sequence + 1 个新 = 1_501
    const expected = MAX_TRACKED_REALTIME_SEQUENCES - Math.floor(MAX_TRACKED_REALTIME_SEQUENCES / 4) + 1;
    expect(state.seenSequences.size).toBe(expected);
    // 最早的 500 个应被淘汰
    expect(state.seenSequences.has(1)).toBe(false);
    expect(state.seenSequences.has(500)).toBe(false);
    // 501 之后仍在
    expect(state.seenSequences.has(501)).toBe(true);
    // 最新一个
    expect(state.seenSequences.has(MAX_TRACKED_REALTIME_SEQUENCES + 1)).toBe(true);
  });

  it("does not drop the latest sequence when over capacity", () => {
    const state = createRealtimeSequenceTrackerState();
    // 构造：先放 2_000 个不连续的大 sequence，再放一个比所有都小的「late arrival」
    for (let i = 100_000; i < 100_000 + MAX_TRACKED_REALTIME_SEQUENCES; i++) {
      processRealtimeEvent(state, { type: "message", sequence: i });
    }
    // late arrival: sequence 1（远小于所有现存 sequence）→ 不应触发淘汰
    // （因为没有比 1 小的 element 可删）
    const before = state.seenSequences.size;
    processRealtimeEvent(state, { type: "message", sequence: 1 });
    // 容量已满但没有任何 sequence < 1，所以只加 1 个，总数 +1
    expect(state.seenSequences.size).toBe(before + 1);
    expect(state.seenSequences.has(1)).toBe(true);
  });
});

// =============================================================================
// 集成：instance 切换 + dedup 联合行为
// =============================================================================

describe("processRealtimeEvent integration", () => {
  it("preserves seenSequences for non-connected events but resets on connected", () => {
    const state: RealtimeSequenceTrackerState = {
      serverInstance: "srv-A",
      seenSequences: new Set([1, 2, 3]),
    };

    // 非 connected：维持
    let r = processRealtimeEvent(state, { type: "message", sequence: 4 });
    expect(r.accept).toBe(true);
    expect(state.seenSequences.has(4)).toBe(true);
    expect(state.serverInstance).toBe("srv-A");

    // 重复 sequence
    r = processRealtimeEvent(state, { type: "message", sequence: 1 });
    expect(r.accept).toBe(false);

    // connected 到相同 instance：维持 seenSequences
    r = processRealtimeEvent(state, {
      type: "connected",
      serverInstanceId: "srv-A",
    });
    expect(state.seenSequences.has(1)).toBe(true);

    // connected 到新 instance：清空
    processRealtimeEvent(state, {
      type: "connected",
      serverInstanceId: "srv-B",
    });
    expect(state.seenSequences.size).toBe(0);
  });
});
