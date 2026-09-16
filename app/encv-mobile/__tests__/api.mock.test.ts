import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  fetchPlugins,
  fetchTags,
  getExternalStreamUrl,
  listFiles,
  listFilesStream,
  NotFoundError,
  PermissionDeniedError,
  searchFiles,
} from "@encv/shared-components/api/encv";

const _originalFetch = globalThis.fetch;

function mockFetch(responseData: any, status = 200, ok = true) {
  return vi.fn().mockResolvedValue({
    ok,
    status,
    json: () => Promise.resolve(responseData),
  } as Response);
}

describe("API Mock Tests", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  describe("listFiles", () => {
    it("returns FileItem[] on success", async () => {
      const mockFiles = [{ name: "a.txt", path: "/a.txt", isDirectory: false }];
      fetchSpy.mockImplementation(mockFetch({ files: mockFiles }));

      const result = await listFiles("/");
      expect(result).toEqual(mockFiles);
      expect(fetchSpy).toHaveBeenCalledTimes(1);
    });

    it("throws PermissionDeniedError on 403 with PERMISSION_DENIED code", async () => {
      fetchSpy.mockImplementation(mockFetch({ code: "PERMISSION_DENIED", error: "Forbidden" }, 403, false));

      await expect(listFiles("/")).rejects.toThrow(PermissionDeniedError);
    });

    it("throws NotFoundError on 404", async () => {
      fetchSpy.mockImplementation(mockFetch({ code: "NOT_FOUND", error: "Not found" }, 404, false));

      await expect(listFiles("/missing")).rejects.toThrow(NotFoundError);
    });

    it("throws generic Error on other HTTP errors", async () => {
      fetchSpy.mockImplementation(mockFetch({}, 500, false));

      await expect(listFiles("/")).rejects.toThrow("HTTP error! status: 500");
    });

    it("returns empty array when files field is missing", async () => {
      fetchSpy.mockImplementation(mockFetch({}));

      const result = await listFiles("/");
      expect(result).toEqual([]);
    });
  });

  describe("searchFiles", () => {
    it("returns search results", async () => {
      const mockResults = [{ name: "findme.mp4", path: "/findme.mp4", isDirectory: false }];
      fetchSpy.mockImplementation(mockFetch({ files: mockResults }));

      const results = await searchFiles("/", "findme", false);
      expect(results).toEqual(mockResults);
    });
  });

  describe("fetchPlugins", () => {
    it("returns PluginMeta array", async () => {
      const plugins = [{ name: "video", supportedExtensions: ["mp4"], containerExtension: "" }];
      fetchSpy.mockImplementation(mockFetch({ plugins }));

      const result = await fetchPlugins();
      expect(result).toEqual(plugins);
    });
  });

  describe("fetchTags", () => {
    it("returns TagInfo array", async () => {
      const tags = [{ name: "fav", count: 5 }];
      fetchSpy.mockImplementation(mockFetch({ tags }));

      const result = await fetchTags();
      expect(result).toEqual(tags);
    });
  });

  describe("getExternalStreamUrl", () => {
    it("uses DEV format when import.meta.env.DEV is true", () => {
      const url = getExternalStreamUrl("/media/video.mp4");
      expect(url).toContain("/api/stream/external");
      expect(url).toContain("video.mp4");
    });
  });

  describe("listFilesStream", () => {
    it("streams files one by one via SSE", async () => {
      const mockFiles = [
        { name: "a.txt", path: "/a.txt", isDirectory: false, size: 100, modified: "2026-01-01" },
        { name: "b.mp4", path: "/b.mp4", isDirectory: false, size: 5000, modified: "2026-01-02" },
        { name: "c_dir", path: "/c_dir", isDirectory: true },
      ];

      const receivedFiles: any[] = [];

      // 构造 SSE 响应流
      const sseData = mockFiles.map(f => `data: ${JSON.stringify(f)}\n\n`).join("") + "data: [DONE]\n\n";

      fetchSpy.mockImplementation(() =>
        Promise.resolve({
          ok: true,
          status: 200,
          body: new ReadableStream({
            start(controller) {
              controller.enqueue(new TextEncoder().encode(sseData));
              controller.close();
            },
          }),
        } as Response)
      );

      const result = await listFilesStream("/", file => {
        receivedFiles.push(file);
      });

      expect(result.files).toEqual(mockFiles);
      expect(receivedFiles).toEqual(mockFiles);
      expect(fetchSpy).toHaveBeenCalledTimes(1);
    });

    it("handles empty directory stream", async () => {
      const received: any[] = [];

      fetchSpy.mockImplementation(() =>
        Promise.resolve({
          ok: true,
          body: new ReadableStream({
            start(controller) {
              controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
              controller.close();
            },
          }),
        } as Response)
      );

      const result = await listFilesStream("/", f => received.push(f));
      expect(result.files).toEqual([]);
      expect(received).toEqual([]);
    });

    it("throws PermissionDeniedError on 403", async () => {
      fetchSpy.mockImplementation(mockFetch({ code: "PERMISSION_DENIED", error: "Forbidden" }, 403, false));

      await expect(listFilesStream("/", () => {})).rejects.toThrow(PermissionDeniedError);
    });

    it("throws NotFoundError on 404", async () => {
      fetchSpy.mockImplementation(mockFetch({ code: "NOT_FOUND", error: "Not found" }, 404, false));

      await expect(listFilesStream("/missing", () => {})).rejects.toThrow(NotFoundError);
    });

    it("handles malformed JSON gracefully (skips bad entries)", async () => {
      const received: any[] = [];
      const goodFile = { name: "good.txt", path: "/good.txt", isDirectory: false, size: 50, modified: "2026-01-01" };

      const sseData = [
        "data: {broken json\n\n",
        `data: ${JSON.stringify(goodFile)}\n\n`,
        "data: not-json-at-all\n\n",
        "data: [DONE]\n\n",
      ].join("");

      fetchSpy.mockImplementation(() =>
        Promise.resolve({
          ok: true,
          body: new ReadableStream({
            start(controller) {
              controller.enqueue(new TextEncoder().encode(sseData));
              controller.close();
            },
          }),
        } as Response)
      );

      const result = await listFilesStream("/", f => received.push(f));
      expect(received).toEqual([goodFile]);
      expect(result.files).toEqual([goodFile]);
    });

    it("handles chunked SSE data (split across reads)", async () => {
      const goodFile = { name: "chunked.bin", path: "/chunked.bin", isDirectory: false, size: 999, modified: "2026-01-01" };
      const fullData = `data: ${JSON.stringify(goodFile)}\n\ndata: [DONE]\n\n`;
      const encoder = new TextEncoder();
      const encoded = encoder.encode(fullData);

      const mid = Math.floor(encoded.length / 2);
      const chunk1 = encoded.slice(0, mid);
      const chunk2 = encoded.slice(mid);

      let readCount = 0;
      fetchSpy.mockImplementation(() =>
        Promise.resolve({
          ok: true,
          body: new ReadableStream({
            pull(controller) {
              readCount++;
              if (readCount === 1) {
                controller.enqueue(chunk1);
              } else if (readCount === 2) {
                controller.enqueue(chunk2);
                controller.close();
              }
            },
          }),
        } as Response)
      );

      const result = await listFilesStream("/", () => {});
      expect(result.files).toEqual([goodFile]);
    });
  });
});
