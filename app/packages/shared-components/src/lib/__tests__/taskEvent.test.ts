import { describe, expect, it } from "vitest";
import { normalizeTaskEvent } from "@encv/shared-components/lib/taskEvent";

describe("normalizeTaskEvent", () => {
  it("completed without error → status=completed, progress=100, completedAt set", () => {
    const out = normalizeTaskEvent("completed", { id: "t1", status: "running" });
    expect(out.status).toBe("completed");
    expect(out.progress).toBe(100);
    expect(typeof out.completedAt).toBe("string");
    expect(out.id).toBe("t1");
  });

  it("completed with error → status=failed, no progress override", () => {
    const out = normalizeTaskEvent("completed", { id: "t2", error: "boom" });
    expect(out.status).toBe("failed");
    expect(out.progress).toBeUndefined();
    expect(typeof out.completedAt).toBe("string");
  });

  it("non-completed events pass through unchanged", () => {
    const data = { id: "t3", status: "running", foo: 1 };
    expect(normalizeTaskEvent("update", data)).toEqual(data);
    expect(normalizeTaskEvent("progress", data)).toEqual(data);
    expect(normalizeTaskEvent("created", data)).toEqual(data);
  });
});
