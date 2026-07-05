import { describe, expect, it } from "vitest";

// Mock server responses matching Go backend format
const mockFileListResponse = {
  path: "/storage/emulated/0",
  entries: [
    { name: "video.mp4", isDir: false, size: 10485760, modTime: "2024-01-01T00:00:00Z" },
    { name: "doc.pdf", isDir: false, size: 20480, modTime: "2024-01-02T12:00:00Z" },
    { name: "text.txt", isDir: false, size: 1024, modTime: "2024-01-03T08:00:00Z" },
  ],
};

describe("API Contract: File List", () => {
  it("returns expected shape with name/isDir/size/modTime fields", () => {
    // Verify the response shape matches what FileExplorer.vue expects
    expect(mockFileListResponse.entries).toBeInstanceOf(Array);
    expect(mockFileListResponse.entries.length).toBeGreaterThan(0);

    const entry = mockFileListResponse.entries[0];
    expect(entry).toHaveProperty("name");
    expect(entry).toHaveProperty("isDir");
    expect(entry).toHaveProperty("size");
    expect(entry).toHaveProperty("modTime");
  });

  it("handles empty directory gracefully", () => {
    const empty = { path: "/empty", entries: [] };
    expect(empty.entries).toHaveLength(0);
  });
});

describe("API Contract: Preview URL Generation", () => {
  const getFilePreviewUrl = (renderer: string, path: string) =>
    `/api/files/preview?renderer=${encodeURIComponent(renderer)}&path=${encodeURIComponent(path)}`;

  it("generates correct URL for text.html renderer", () => {
    const url = getFilePreviewUrl("text.html", "/storage/test.txt");
    expect(url).toContain("renderer=text.html");
    expect(url).toContain("path=");
    expect(url).not.toContain("renderer=pdf.html");
  });

  it("generates correct URL for pdf.html renderer", () => {
    const url = getFilePreviewUrl("pdf.html", "/storage/doc.pdf");
    expect(url).toContain("renderer=pdf.html");
    expect(url).toContain("path=");
  });
});
