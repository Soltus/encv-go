import { describe, expect, it } from "vitest";
import { proxySafeEncode } from "@encv/shared-components/api/encv";

function decodeDouble(raw: string): string {
  try {
    return decodeURIComponent(decodeURIComponent(raw));
  } catch {
    return raw;
  }
}

describe("proxySafeEncode: double encoding for WAF-safe path transmission", () => {
  it("double-encodes @ symbol to survive WAF truncation", () => {
    const encoded = proxySafeEncode("@");
    expect(encoded).toBe("%2540");
    expect(decodeDouble(encoded)).toBe("@");
  });

  it("double-encodes full special-char filename", () => {
    const path = "/04-boundary-test/special-chars-!@#$%^&*()_+.txt";
    const encoded = proxySafeEncode(path);
    expect(encoded).toContain("%2540");
    expect(encoded).toContain("%2523");
    expect(encoded).toContain("%2524");
    expect(decodeDouble(encoded)).toBe(path);
  });

  it("round-trips normal paths unchanged", () => {
    const paths = ["/01-plain-media/document/notes.txt", "/03-encv-containers/container.sccgv", "/", "/a/b/c.txt", ".hidden-file"];
    for (const p of paths) {
      expect(decodeDouble(proxySafeEncode(p))).toBe(p);
    }
  });

  it("round-trips unicode filenames", () => {
    const paths = ["中文文件名.txt", "long-unicode-filename-中文-日本語-한국어-العربية-עברית-ไทย-ελληνικά.txt", "emoji-test-😀🎉🚀🔥.txt"];
    for (const p of paths) {
      expect(decodeDouble(proxySafeEncode(p))).toBe(p);
    }
  });

  it("round-trips spaces and dots", () => {
    const paths = ["spaces   in   name.txt", "trailing-space.txt ", "..dotfile"];
    for (const p of paths) {
      expect(decodeDouble(proxySafeEncode(p))).toBe(p);
    }
  });

  it("produces different output than single encodeURIComponent", () => {
    const input = "!@#$%^&*()";
    const single = encodeURIComponent(input);
    const double = proxySafeEncode(input);
    expect(double).not.toBe(single);
    expect(double.length).toBeGreaterThan(single.length);
  });

  it("@ is the critical character that gets double-wrapped", () => {
    const input = "!@#$%";
    const single = encodeURIComponent(input);
    const double = proxySafeEncode(input);
    expect(single).toBe("!%40%23%24%25");
    expect(double).toBe("!%2540%2523%2524%2525");
  });

  it("empty string stays empty", () => {
    expect(proxySafeEncode("")).toBe("");
  });
});
