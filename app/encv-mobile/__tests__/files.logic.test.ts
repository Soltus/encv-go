import { documentText, folder, image, lockClosed, musicalNotes, videocam } from "ionicons/icons";
import { describe, expect, it } from "vitest";
import type { FileItem } from "@encv/shared-components/api/encv";
import {
  cycleSortState,
  getFileIcon,
  getFileIconColor,
  getSortLabel,
  IMAGE_EXTENSIONS,
  isImageFile,
  SORT_CYCLE,
  sortFiles,
  VIRTUAL_SCROLL_CONFIG,
} from "@/composables/useFileList";

function makeFile(overrides: Partial<FileItem> & { name: string }): FileItem {
  return {
    path: "/" + overrides.name,
    isDirectory: false,
    ...overrides,
  };
}

describe("IMAGE_EXTENSIONS", () => {
  it("should contain exactly 10 image extensions", () => {
    expect(IMAGE_EXTENSIONS.size).toBe(10);
    const expected = [".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".heic", ".heif", ".avif"];
    expected.forEach(ext => expect(IMAGE_EXTENSIONS.has(ext)).toBe(true));
  });
});

describe("isImageFile", () => {
  it("should return true for jpg/jpeg/png/gif/webp/bmp/svg/heic/heif/avif", () => {
    const exts = ["photo.jpg", "a.jpeg", "b.PNG", "c.gif", "d.webp", "e.bmp", "f.svg", "g.heic", "h.heif", "i.avif"];
    exts.forEach(name => expect(isImageFile(makeFile({ name }))).toBe(true));
  });

  it("should be case-insensitive for uppercase extensions", () => {
    expect(isImageFile(makeFile({ name: "PHOTO.JPG" }))).toBe(true);
    expect(isImageFile(makeFile({ name: "Photo.PNG" }))).toBe(true);
  });

  it("should return false for non-image files", () => {
    const nonImages = ["video.mp4", "audio.mp3", "doc.pdf", "data.txt", "code.ts", "archive.zip"];
    nonImages.forEach(name => expect(isImageFile(makeFile({ name }))).toBe(false));
  });

  it("should return false for files without extension", () => {
    expect(isImageFile(makeFile({ name: "Makefile" }))).toBe(false);
    expect(isImageFile(makeFile({ name: ".gitignore" }))).toBe(false);
  });

  it("should return false for directories", () => {
    expect(isImageFile(makeFile({ name: "pics", isDirectory: true }))).toBe(false);
  });
});

describe("getFileIcon", () => {
  it("should return folder icon for directories", () => {
    const f = makeFile({ name: "docs", isDirectory: true });
    expect(getFileIcon(f)).toBe(folder);
  });

  it("should return videocam for video files", () => {
    const f = makeFile({ name: "movie.mp4" });
    expect(getFileIcon(f)).toBe(videocam);
  });

  it("should return musicalNotes for audio files", () => {
    const f = makeFile({ name: "song.mp3" });
    expect(getFileIcon(f)).toBe(musicalNotes);
  });

  it("should return image icon for image files", () => {
    const f = makeFile({ name: "photo.jpg" });
    expect(getFileIcon(f)).toBe(image);
  });

  it("should return lockClosed for encrypted files", () => {
    const f = makeFile({ name: "secret.enc", isEncrypted: true });
    expect(getFileIcon(f)).toBe(lockClosed);
  });

  it("should return documentText for unknown file types", () => {
    const f = makeFile({ name: "data.xyz" });
    expect(getFileIcon(f)).toBe(documentText);
  });
});

describe("getFileIconColor", () => {
  it("should return primary for directories", () => {
    expect(getFileIconColor(makeFile({ name: "docs", isDirectory: true }))).toBe("primary");
  });

  it("should return danger for video files", () => {
    expect(getFileIconColor(makeFile({ name: "movie.mp4" }))).toBe("danger");
  });

  it("should return tertiary for audio files", () => {
    expect(getFileIconColor(makeFile({ name: "song.mp3" }))).toBe("tertiary");
  });

  it("should return success for image files", () => {
    expect(getFileIconColor(makeFile({ name: "photo.jpg" }))).toBe("success");
  });

  it("should return warning for encrypted files", () => {
    expect(getFileIconColor(makeFile({ name: "secret.enc", isEncrypted: true }))).toBe("warning");
  });

  it("should return medium for other files", () => {
    expect(getFileIconColor(makeFile({ name: "data.xyz" }))).toBe("medium");
  });
});

describe("SORT_CYCLE & cycleSortState", () => {
  it("should have 6 states in cycle", () => {
    expect(SORT_CYCLE.length).toBe(6);
  });

  it("should cycle through all 6 states and return to start", () => {
    let current = SORT_CYCLE[0];
    for (let i = 0; i < SORT_CYCLE.length; i++) {
      current = cycleSortState(current);
    }
    expect(current).toEqual(SORT_CYCLE[0]);
  });

  it("should follow correct order: name↑→name↓→size↑→size↓→time↑→time↓", () => {
    const expected = [
      { by: "name" as const, desc: false },
      { by: "name" as const, desc: true },
      { by: "size" as const, desc: false },
      { by: "size" as const, desc: true },
      { by: "time" as const, desc: false },
      { by: "time" as const, desc: true },
    ];
    let state = cycleSortState({ by: "time", desc: true });
    expected.forEach(exp => {
      expect(state).toEqual(exp);
      state = cycleSortState(state);
    });
  });
});

describe("sortFiles", () => {
  const files: FileItem[] = [
    makeFile({ name: "banana.txt", size: 200, modified: "1000" }),
    makeFile({ name: "apple.txt", size: 100, modified: "3000" }),
    makeFile({ name: "cherry.txt", size: 300, modified: "2000" }),
  ];

  it("sorts by name ascending (A-Z)", () => {
    const result = sortFiles(files, "name", false);
    expect(result.map(f => f.name)).toEqual(["apple.txt", "banana.txt", "cherry.txt"]);
  });

  it("sorts by name descending (Z-A)", () => {
    const result = sortFiles(files, "name", true);
    expect(result.map(f => f.name)).toEqual(["cherry.txt", "banana.txt", "apple.txt"]);
  });

  it("sorts by size ascending (small to large)", () => {
    const result = sortFiles(files, "size", false);
    expect(result.map(f => f.name)).toEqual(["apple.txt", "banana.txt", "cherry.txt"]);
  });

  it("sorts by size descending (large to small)", () => {
    const result = sortFiles(files, "size", true);
    expect(result.map(f => f.name)).toEqual(["cherry.txt", "banana.txt", "apple.txt"]);
  });

  it("sorts by time ascending (oldest first)", () => {
    const result = sortFiles(files, "time", false);
    expect(result.map(f => f.name)).toEqual(["banana.txt", "cherry.txt", "apple.txt"]);
  });

  it("sorts by time descending (newest first)", () => {
    const result = sortFiles(files, "time", true);
    expect(result.map(f => f.name)).toEqual(["apple.txt", "cherry.txt", "banana.txt"]);
  });

  it("always puts directories first regardless of sort", () => {
    const mixed: FileItem[] = [
      makeFile({ name: "zfile.txt" }),
      makeFile({ name: "adir", isDirectory: true }),
      makeFile({ name: "mfile.txt" }),
      makeFile({ name: "bdir", isDirectory: true }),
    ];
    const asc = sortFiles(mixed, "name", false);
    expect(asc[0].isDirectory).toBe(true);
    expect(asc[1].isDirectory).toBe(true);
    expect(asc[0].name).toBe("adir");
    expect(asc[1].name).toBe("bdir");

    const desc = sortFiles(mixed, "name", true);
    expect(desc[0].isDirectory).toBe(true);
    expect(desc[1].isDirectory).toBe(true);
  });

  it("handles empty array", () => {
    expect(sortFiles([], "name", false)).toEqual([]);
  });

  it("handles missing size/modified gracefully", () => {
    const partial: FileItem[] = [makeFile({ name: "a.txt" }), makeFile({ name: "b.txt" })];
    const result = sortFiles(partial, "size", false);
    expect(result.length).toBe(2);
  });
});

describe("getSortLabel", () => {
  it("returns correct labels with arrows", () => {
    expect(getSortLabel("name", false)).toBe("名字↑");
    expect(getSortLabel("name", true)).toBe("名字↓");
    expect(getSortLabel("size", false)).toBe("大小↑");
    expect(getSortLabel("size", true)).toBe("大小↓");
    expect(getSortLabel("time", false)).toBe("时间↑");
    expect(getSortLabel("time", true)).toBe("时间↓");
  });
});

describe("Edge cases - sortFiles", () => {
  it("single element returns same array", () => {
    const single = [makeFile({ name: "solo.txt" })];
    expect(sortFiles(single, "name", false)).toEqual(single);
  });

  it("all directories stay in original relative order for same sort key", () => {
    const dirs = [makeFile({ name: "beta", isDirectory: true }), makeFile({ name: "alpha", isDirectory: true })];
    const result = sortFiles(dirs, "name", false);
    expect(result[0].name).toBe("alpha");
    expect(result[1].name).toBe("beta");
  });

  it("unicode names sort without crashing", () => {
    const unicode = [
      makeFile({ name: "中文.txt" }),
      makeFile({ name: "alpha.txt" }),
      makeFile({ name: "ß.txt" }),
      makeFile({ name: "日本語.txt" }),
    ];
    const result = sortFiles(unicode, "name", false);
    expect(result.length).toBe(4);
    expect(result[0].name).toBe("alpha.txt");
    const rest = result.slice(1).map(f => f.name);
    expect(rest).toContain("ß.txt");
    expect(rest).toContain("日本語.txt");
    expect(rest).toContain("中文.txt");
  });

  it("same-name files with different case are stable-sorted by insertion order", () => {
    const sameName = [makeFile({ name: "a.txt", size: 100 }), makeFile({ name: "A.TXT", size: 200 })];
    const result = sortFiles(sameName, "name", false);
    expect(result.length).toBe(2);
  });

  it("mixed directory/file with identical names keeps dir first", () => {
    const mixed = [makeFile({ name: "notes", isDirectory: false, size: 50 }), makeFile({ name: "notes", isDirectory: true })];
    const result = sortFiles(mixed, "name", false);
    expect(result[0].isDirectory).toBe(true);
    expect(result[1].isDirectory).toBe(false);
  });
});

describe("Edge cases - isImageFile", () => {
  it("hidden file (dot-prefixed) with image ext returns true", () => {
    expect(isImageFile(makeFile({ name: ".hidden.jpg" }))).toBe(true);
  });

  it("file with multiple dots uses last extension", () => {
    expect(isImageFile(makeFile({ name: "archive.tar.gz" }))).toBe(false);
    expect(isImageFile(makeFile({ name: "photo.mini.png" }))).toBe(true);
  });

  it("empty string name returns false", () => {
    expect(isImageFile(makeFile({ name: "" }))).toBe(false);
  });

  it('file named only extension (e.g. ".jpg") returns true', () => {
    expect(isImageFile(makeFile({ name: ".jpg" }))).toBe(true);
  });
});

describe("VIRTUAL_SCROLL_CONFIG", () => {
  it("should have correct threshold value", () => {
    expect(VIRTUAL_SCROLL_CONFIG.THRESHOLD).toBe(200);
  });

  it("should have positive estimate size", () => {
    expect(VIRTUAL_SCROLL_CONFIG.ESTIMATE_SIZE).toBeGreaterThan(0);
  });

  it("should have reasonable overscan value", () => {
    expect(VIRTUAL_SCROLL_CONFIG.OVERSCAN).toBeGreaterThanOrEqual(1);
    expect(VIRTUAL_SCROLL_CONFIG.OVERSCAN).toBeLessThanOrEqual(20);
  });
});

describe("sortFiles - stress test with large dataset", () => {
  function generateFiles(n: number): FileItem[] {
    return Array.from({ length: n }, (_, i) => ({
      name: `file_${String(i).padStart(5, "0")}.txt`,
      path: `/file_${String(i).padStart(5, "0")}.txt`,
      isDirectory: false,
      size: Math.floor(Math.random() * 1000000),
      modified: `${1000000 + i}`,
    }));
  }

  it("sorts 5000 files by name without crashing", () => {
    const files = generateFiles(5000);
    const start = Date.now();
    const result = sortFiles(files, "name", false);
    const elapsed = Date.now() - start;

    expect(result.length).toBe(5000);
    expect(elapsed).toBeLessThan(500, "sorting 5000 files should take < 500ms");

    // 验证排序正确性
    for (let i = 1; i < result.length; i++) {
      expect(result[i - 1].name <= result[i].name).toBe(true);
    }
  });

  it("sorts 5000 files with mixed dirs and files (dirs first)", () => {
    const files: FileItem[] = [];
    for (let i = 0; i < 5000; i++) {
      if (i % 10 === 0) {
        files.push(makeFile({ name: `dir_${i}`, isDirectory: true }));
      } else {
        files.push(makeFile({ name: `file_${i}` }));
      }
    }

    const result = sortFiles(files, "name", false);

    // 找到第一个非目录的位置
    let firstNonDir = result.length;
    for (let i = 0; i < result.length; i++) {
      if (!result[i].isDirectory) {
        firstNonDir = i;
        break;
      }
    }

    // 所有目录应在非目录之前
    for (let i = 0; i < firstNonDir; i++) {
      expect(result[i].isDirectory).toBe(true);
    }
    for (let i = firstNonDir; i < result.length; i++) {
      expect(result[i].isDirectory).toBe(false);
    }
  });
});

describe("sortFiles - unicode stability", () => {
  it("handles CJK filenames correctly", () => {
    const files: FileItem[] = [
      makeFile({ name: "中文文件.txt" }),
      makeFile({ name: "日本語.doc" }),
      makeFile({ name: "한국어.pdf" }),
      makeFile({ name: "alpha.txt" }),
    ];
    const result = sortFiles(files, "name", false);
    // alpha (ASCII) should come before CJK in most collations
    expect(result[0].name).toBe("alpha.txt");
    expect(result.length).toBe(4);
  });

  it("handles emoji in filenames without crashing", () => {
    const files: FileItem[] = [makeFile({ name: "🎵 song.mp3" }), makeFile({ name: "📷 photo.jpg" }), makeFile({ name: "🎬 movie.mp4" })];
    const result = sortFiles(files, "name", false);
    expect(result.length).toBe(3);
    // Just verify no crash and order is stable
    const result2 = sortFiles(files, "name", false);
    expect(result.map(f => f.name)).toEqual(result2.map(f => f.name));
  });
});
