import { beforeEach, describe, expect, it, vi } from "vitest";
import { usePathResolver } from "@/composables/usePathResolver";

vi.mock("@/api/encv", () => ({}));

describe("usePathResolver - normalize", () => {
  const { normalize } = usePathResolver();

  it("empty string stays empty", () => {
    expect(normalize("")).toBe("");
  });

  it("whitespace-only becomes empty", () => {
    expect(normalize("  ")).toBe("");
    expect(normalize("\t\n")).toBe("");
  });

  it("already normalized absolute path is unchanged", () => {
    expect(normalize("/test.mp4")).toBe("/test.mp4");
  });

  it("collapses leading double slash", () => {
    expect(normalize("//test.mp4")).toBe("/test.mp4");
  });

  it("collapses double slash in the middle", () => {
    expect(normalize("/folder//file.txt")).toBe("/folder/file.txt");
  });

  it("converts Windows backslashes to forward slashes and prepends /", () => {
    expect(normalize("C:\\Users\\test.mp4")).toBe("/C:/Users/test.mp4");
  });

  it("prepends / to relative paths", () => {
    expect(normalize("relative/path")).toBe("/relative/path");
  });

  it("trims whitespace then normalizes", () => {
    expect(normalize("  /folder/test.mp4  ")).toBe("/folder/test.mp4");
  });
});

describe("usePathResolver - resolveFileItem", () => {
  const { resolveFileItem } = usePathResolver();

  it("extracts and normalizes file.path", () => {
    expect(resolveFileItem({ name: "a.txt", path: "//docs/a.txt", isDirectory: false })).toBe("/docs/a.txt");
  });

  it("returns empty string for null input", () => {
    expect(resolveFileItem(null as any)).toBe("");
  });

  it("returns empty string for undefined input", () => {
    expect(resolveFileItem(undefined as any)).toBe("");
  });

  it("returns empty string when file.path is empty", () => {
    expect(resolveFileItem({ name: "x", path: "", isDirectory: false })).toBe("");
  });
});

describe("usePathResolver - isAbsolutePath", () => {
  const { isAbsolutePath } = usePathResolver();

  it("returns true for paths starting with /", () => {
    expect(isAbsolutePath("/home/user/file.mp4")).toBe(true);
    expect(isAbsolutePath("/")).toBe(true);
  });

  it("returns false for relative paths", () => {
    expect(isAbsolutePath("relative/path")).toBe(false);
    expect(isAbsolutePath("./file.txt")).toBe(false);
  });

  it("returns false for Windows-style paths without leading /", () => {
    expect(isAbsolutePath("C:\\Users\\file.mp4")).toBe(false);
  });

  it("returns false for empty string", () => {
    expect(isAbsolutePath("")).toBe(false);
  });
});

describe("usePathResolver - getMockPaths", () => {
  beforeEach(() => {
    vi.stubEnv("DEV", "true");
  });

  afterAll(() => {
    vi.unstubAllEnvs();
  });

  it("returns mock array in DEV mode", () => {
    const { getMockPaths } = usePathResolver();
    const result = getMockPaths();
    expect(result).toEqual(["/mock/video.mp4", "/mock/doc.txt", "/mock/report.pdf", "/mock/data.csv"]);
  });

  it("returns new array instance each call (not same reference)", () => {
    const { getMockPaths } = usePathResolver();
    const a = getMockPaths();
    const b = getMockPaths();
    expect(a).toEqual(b);
    expect(a).not.toBe(b);
  });
});
