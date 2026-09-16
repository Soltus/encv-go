import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { parseSseEvent, generateMockFilesViaBackend, resetMockFilesViaBackend } from "@encv/shared-components/api/mockGenerator";

function sseResponse(body: string): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(body));
      controller.close();
    },
  });
  return new Response(stream, { status: 200, headers: { "Content-Type": "text/event-stream" } });
}

describe("parseSseEvent", () => {
  it("parses a named event with a single data line", () => {
    expect(parseSseEvent('event: progress\ndata: {"a":1}')).toEqual({
      event: "progress",
      data: '{"a":1}',
    });
  });

  it("joins multiple data lines with a newline", () => {
    expect(parseSseEvent("event: x\ndata: line1\ndata: line2")).toEqual({
      event: "x",
      data: "line1\nline2",
    });
  });

  it("defaults the event to 'message' when no event: line is present", () => {
    expect(parseSseEvent("data: hello")).toEqual({ event: "message", data: "hello" });
  });

  it("returns null for an empty block with only the default message event", () => {
    expect(parseSseEvent("")).toBeNull();
  });
});

describe("generateMockFilesViaBackend", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("parses an SSE stream, fires callbacks and returns the final result", async () => {
    const body = [
      "event: starting",
      'data: {"total": 1, "type": "all", "root": "/x"}',
      "",
      "event: spec_plan",
      'data: {"index":1,"total":1,"relativePath":"a","status":"pending","encoder":"","runner":"static","ffmpegArgs":[],"exitCode":0,"stderr":""}',
      "",
      "event: spec_diag",
      'data: {"index":1,"total":1,"relativePath":"a","status":"ok","encoder":"x","runner":"ffmpeg","ffmpegArgs":["-c","copy"],"exitCode":0,"stderr":""}',
      "",
      "event: progress",
      'data: {"relativePath":"a","size":123}',
      "",
      "event: done",
      'data: {"count":1,"skipped":0,"totalSize":123}',
      "",
    ].join("\n");

    (global.fetch as unknown as vi.Mock).mockResolvedValue(sseResponse(body));

    const onSpecPlan = vi.fn();
    const onSpecDiag = vi.fn();
    const onProgress = vi.fn();

    const result = await generateMockFilesViaBackend({ root: "/x", onSpecPlan, onSpecDiag, onProgress });

    expect(result).toEqual({ count: 1, skipped: 0, totalSize: 123 });
    expect(onSpecPlan).toHaveBeenCalled();
    expect(onSpecDiag).toHaveBeenCalled();
    expect(onProgress).toHaveBeenCalledWith({ relativePath: "a", size: 123 });
  });

  it("rethrows a fatal error event as a thrown Error", async () => {
    const body = ["event: error", "data: boom", ""].join("\n");
    (global.fetch as unknown as vi.Mock).mockResolvedValue(sseResponse(body));

    await expect(generateMockFilesViaBackend({ root: "/x" })).rejects.toThrow("boom");
  });
});

describe("resetMockFilesViaBackend", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("POSTs to the reset endpoint and returns the parsed json", async () => {
    (global.fetch as unknown as vi.Mock).mockResolvedValue(new Response(JSON.stringify({ removed: 3 }), { status: 200 }));

    const r = await resetMockFilesViaBackend("/x");

    expect(r).toEqual({ removed: 3 });
    const [url] = (global.fetch as unknown as vi.Mock).mock.calls[0];
    expect(String(url)).toContain("/api/mock/reset");
  });
});
