import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { decodeAlistFilename, getAlistEncryptStreamUrl } from "@encv/shared-components/api/encv";

function mockFetch(responseData: any, status = 200, ok = true) {
  return vi.fn().mockResolvedValue({
    ok,
    status,
    json: () => Promise.resolve(responseData),
  } as Response);
}

describe("decodeAlistFilename API", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("decodes filename successfully", async () => {
    fetchSpy.mockImplementation(mockFetch({ plain_name: "movie.mp4", success: true }));

    const result = await decodeAlistFilename({ encodedName: "abcXYZ", password: "mypass" });
    expect(result.plain_name).toBe("movie.mp4");
    expect(result.success).toBe(true);
    expect(fetchSpy).toHaveBeenCalledTimes(1);

    const calledUrl = fetchSpy.mock.calls[0][0] as string;
    expect(calledUrl).toContain("encoded=abcXYZ");
    expect(calledUrl).toContain("password=mypass");
  });

  it("returns failure for wrong password", async () => {
    fetchSpy.mockImplementation(mockFetch({ plain_name: "", success: false }));

    const result = await decodeAlistFilename({ encodedName: "abc", password: "wrong" });
    expect(result.success).toBe(false);
    expect(result.plain_name).toBe("");
  });

  it("throws on HTTP error", async () => {
    fetchSpy.mockImplementation(mockFetch({}, 500, false));

    await expect(decodeAlistFilename({ encodedName: "x", password: "p" })).rejects.toThrow();
  });

  it("passes encType parameter when provided", async () => {
    fetchSpy.mockImplementation(mockFetch({ plain_name: "f.txt", success: true }));

    await decodeAlistFilename({ encodedName: "x", password: "p", encType: "aesctr" });

    const calledUrl = fetchSpy.mock.calls[0][0] as string;
    expect(calledUrl).toContain("enc_type=aesctr");
  });
});

describe("getAlistEncryptStreamUrl", () => {
  it("returns DEV format URL", () => {
    const url = getAlistEncryptStreamUrl({ path: "/media/video.bin", password: "secret123" });
    expect(url).toBe("/api/alist-encrypt/stream?path=%2Fmedia%2Fvideo.bin&password=secret123");
  });

  it("URL-encodes special characters in path", () => {
    const url = getAlistEncryptStreamUrl({ path: "/folder/name with spaces.bin", password: "p@ss&w0rd" });
    expect(url).toContain("path=");
    expect(url).not.toContain(" ");
    expect(url).toContain("password=p%40ss%26w0rd");
  });
});
