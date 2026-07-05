import type { FileItem } from "@encv/shared-components/api/encv";
import { cycleSortState, getSortLabel, sortFiles } from "@encv/shared-components/composables/useFileList";
import { describe, expect, it } from "vitest";

function makeFile(name: string, opts: Partial<FileItem> = {}): FileItem {
  return {
    name,
    path: "/" + name,
    isDirectory: false,
    size: 0,
    modified: "2026-01-01T00:00:00Z",
    ...opts,
  };
}

describe("sortFiles", () => {
  it("name 升序：按字典序", () => {
    const files = [makeFile("b.txt"), makeFile("a.txt"), makeFile("c.txt")];
    const sorted = sortFiles(files, "name", false);
    expect(sorted.map(f => f.name)).toEqual(["a.txt", "b.txt", "c.txt"]);
  });

  it("size 降序：大的在前", () => {
    const files = [makeFile("a.txt", { size: 100 }), makeFile("b.txt", { size: 500 }), makeFile("c.txt", { size: 200 })];
    const sorted = sortFiles(files, "size", true);
    expect(sorted.map(f => f.name)).toEqual(["b.txt", "c.txt", "a.txt"]);
  });

  it("relevance 降序：score 高的在前", () => {
    // 混合相关度（不是纯向量）：score ∈ [0, 1]
    const files = [makeFile("a.mp4", { score: 0.3 }), makeFile("b.mp4", { score: 0.9 }), makeFile("c.mp4", { score: 0.6 })];
    const sorted = sortFiles(files, "relevance", true);
    expect(sorted.map(f => f.name)).toEqual(["b.mp4", "c.mp4", "a.mp4"]);
  });

  it("relevance 升序：score 低的在前", () => {
    const files = [makeFile("a.mp4", { score: 0.3 }), makeFile("b.mp4", { score: 0.9 }), makeFile("c.mp4", { score: 0.6 })];
    const sorted = sortFiles(files, "relevance", false);
    expect(sorted.map(f => f.name)).toEqual(["a.mp4", "c.mp4", "b.mp4"]);
  });

  it("relevance：无 score 的项排到末尾（降序时）", () => {
    const files = [
      makeFile("a.mp4", { score: 0.5 }),
      makeFile("b.mp4"), // 无 score
      makeFile("c.mp4", { score: 0.8 }),
    ];
    const sorted = sortFiles(files, "relevance", true);
    // 降序：c(0.8) > a(0.5) > b(无 score=-1)
    expect(sorted.map(f => f.name)).toEqual(["c.mp4", "a.mp4", "b.mp4"]);
  });

  it("relevance：所有项都无 score 时稳定排序", () => {
    const files = [makeFile("a.mp4"), makeFile("b.mp4"), makeFile("c.mp4")];
    const sorted = sortFiles(files, "relevance", true);
    // 无 score → 全部 -1，相对顺序保持
    expect(sorted.map(f => f.name)).toEqual(["a.mp4", "b.mp4", "c.mp4"]);
  });

  it("目录始终在文件前面（不论排序方式）", () => {
    const dir = { name: "docs", path: "/docs", isDirectory: true } as FileItem;
    const fileA = makeFile("a.txt", { score: 0.1 });
    const fileB = makeFile("b.txt", { score: 0.9 });
    const sorted = sortFiles([fileA, dir, fileB], "relevance", true);
    expect(sorted[0].isDirectory).toBe(true);
  });
});

describe("cycleSortState", () => {
  it("name asc → name desc → size asc → size desc → time asc → time desc → name asc", () => {
    let s: { by: "name" | "size" | "time" | "relevance"; desc: boolean } = { by: "name", desc: false };
    const expectSeq: Array<{ by: "name" | "size" | "time" | "relevance"; desc: boolean }> = [
      { by: "name", desc: true },
      { by: "size", desc: false },
      { by: "size", desc: true },
      { by: "time", desc: false },
      { by: "time", desc: true },
      { by: "name", desc: false }, // 回环
    ];
    for (const exp of expectSeq) {
      s = cycleSortState(s);
      expect(s).toEqual(exp);
    }
  });
});

describe("getSortLabel", () => {
  it('name desc → "名字↓"', () => {
    expect(getSortLabel("name", true)).toBe("名字↓");
  });
  it('relevance desc → "相关度↓"', () => {
    expect(getSortLabel("relevance", true)).toBe("相关度↓");
  });
  it('size asc → "大小↑"', () => {
    expect(getSortLabel("size", false)).toBe("大小↑");
  });
});
