import { describe, it, expect, vi, beforeEach } from "vitest";
import { useMockGenLog } from "@encv/shared-components/composables/useMockGenLog";
import type { MockSpecDiag } from "@encv/shared-components/api/mockGenerator";

function plan(index: number, total: number, relativePath: string): MockSpecDiag {
  return {
    index,
    total,
    relativePath,
    status: "pending",
    encoder: "",
    runner: "static",
    ffmpegArgs: [],
    exitCode: 0,
    stderr: "",
  };
}

function diag(index: number, total: number, relativePath: string, status: "ok" | "failed"): MockSpecDiag {
  return { ...plan(index, total, relativePath), status, encoder: "x", runner: "ffmpeg", ffmpegArgs: ["-c", "copy"] };
}

describe("useMockGenLog", () => {
  it("onSpecPlan pushes a pending entry and sets the total", () => {
    const log = useMockGenLog();
    log.onSpecPlan(plan(1, 2, "a"));
    expect(log.mockGenLogTotal.value).toBe(2);
    expect(log.mockGenLog.value).toHaveLength(1);
    expect(log.mockGenLog.value[0].status).toBe("pending");
  });

  it("onSpecDiag replaces the pending entry with the diagnostic", () => {
    const log = useMockGenLog();
    log.onSpecPlan(plan(1, 1, "a"));
    log.onSpecDiag(diag(1, 1, "a", "ok"));
    expect(log.mockGenLog.value[0].status).toBe("ok");
    expect(log.mockGenLog.value[0].encoder).toBe("x");
  });

  it("onProgress marks the matching ok entry and increments counters", () => {
    const log = useMockGenLog();
    log.onSpecDiag(diag(1, 1, "p", "ok"));
    log.onProgress({ relativePath: "p", size: 100 });
    expect(log.lastCount.value).toBe(1);
    expect(log.lastSize.value).toBe(100);
    expect(log.mockGenLog.value[0]._marked).toBe(true);
  });

  it("onSpecFailed collects a skipped file and marks the entry failed", () => {
    const log = useMockGenLog();
    log.onSpecDiag(diag(1, 1, "b", "ok"));
    log.onSpecFailed({ relativePath: "b", reason: "r", exitCode: 1, stderr: "err" });
    expect(log.skippedFiles.value).toHaveLength(1);
    expect(log.mockGenLog.value[0].status).toBe("failed");
  });

  it("summary reflects counts and the disconnected flag", () => {
    const log = useMockGenLog();
    log.onSpecDiag(diag(1, 3, "a", "ok"));
    log.onSpecDiag(diag(2, 3, "b", "failed"));
    const s = log.mockGenLogSummary.value!;
    expect(s.total).toBe(3);
    expect(s.ok).toBe(1);
    expect(s.failed).toBe(1);
    expect(s.skipped).toBe(0);
    expect(s.disconnected).toBe(true);
  });

  it("resetMockGenLog clears all state", () => {
    const log = useMockGenLog();
    log.onSpecDiag(diag(1, 1, "a", "ok"));
    log.resetMockGenLog();
    expect(log.mockGenLog.value).toHaveLength(0);
    expect(log.mockGenLogTotal.value).toBe(0);
    expect(log.skippedFiles.value).toHaveLength(0);
  });

  it("copyMockGenLog writes the log to the clipboard and flips the copied flag", async () => {
    const log = useMockGenLog();
    log.onSpecDiag(diag(1, 1, "a", "ok"));
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    log.copyMockGenLog();
    await new Promise(r => setTimeout(r, 0));
    expect(writeText).toHaveBeenCalled();
    expect(log.mockGenLogCopied.value).toBe(true);
  });
});
