/**
 * useAttachments 单元测试
 *
 * 覆盖：
 *  1. addFiles —— 正常添加（image/file 分类）
 *  2. addFiles —— 超限拒绝（image 5MB / file 20MB）
 *  3. addFiles —— null / undefined / 空 FileList
 *  4. addFiles —— 单次超过 maxFilesPerBatch 截断 + onError
 *  5. removeAttachment —— 删除指定 id
 *  6. clearAttachments —— 清空
 *  7. serialize —— 字符串 + image + file 拼接（OpenAI multimodal 顺序）
 *  8. serializeAttachments（纯函数版）—— 同上
 *  9. mime 缺失时按扩展名兜底
 */
import { describe, expect, it, vi } from "vitest";
import { type Attachment, MAX_FILE_SIZE, MAX_IMAGE_SIZE, serializeAttachments, useAttachments } from "./useAttachments";

/**
 * 构造 mock File
 */
function makeFile(name: string, sizeBytes: number, mimeType = ""): File {
  // 用 Uint8Array 模拟二进制内容。File 构造器在 jsdom 中可用。
  const content = new Uint8Array(sizeBytes);
  // 给字节赋值（避免空数据被压缩成 0 字节）
  for (let i = 0; i < sizeBytes; i++) content[i] = i % 256;
  return new File([content], name, { type: mimeType });
}

describe("useAttachments.addFiles", () => {
  it("adds image files (mime image/*)", async () => {
    const { attachments, addFiles } = useAttachments();
    const file = makeFile("photo.png", 1024, "image/png");
    const result = await addFiles([file]);
    expect(result.added.length).toBe(1);
    expect(result.rejected.length).toBe(0);
    expect(attachments.value.length).toBe(1);
    expect(attachments.value[0].kind).toBe("image");
    expect(attachments.value[0].name).toBe("photo.png");
    expect(attachments.value[0].dataUrl).toMatch(/^data:image\/png;base64,/);
  });

  it("adds non-image files as kind=file", async () => {
    const { attachments, addFiles } = useAttachments();
    const file = makeFile("doc.pdf", 2048, "application/pdf");
    const result = await addFiles([file]);
    expect(result.added.length).toBe(1);
    expect(attachments.value[0].kind).toBe("file");
    expect(attachments.value[0].mimeType).toBe("application/pdf");
  });

  it("infers kind=image from extension when mime missing (Android/iOS quirk)", async () => {
    const { attachments, addFiles } = useAttachments();
    const file = makeFile("snapshot.jpg", 1024, ""); // mime 为空
    await addFiles([file]);
    expect(attachments.value[0].kind).toBe("image");
  });

  it("rejects image larger than MAX_IMAGE_SIZE", async () => {
    const onError = vi.fn();
    const { attachments, addFiles } = useAttachments({ onError });
    const big = makeFile("huge.png", MAX_IMAGE_SIZE + 1, "image/png");
    const result = await addFiles([big]);
    expect(result.added.length).toBe(0);
    expect(result.rejected.length).toBe(1);
    expect(result.rejected[0].reason).toContain("5");
    expect(attachments.value.length).toBe(0);
  });

  it("rejects file larger than MAX_FILE_SIZE", async () => {
    const { addFiles } = useAttachments();
    const big = makeFile("huge.zip", MAX_FILE_SIZE + 1, "application/zip");
    const result = await addFiles([big]);
    expect(result.added.length).toBe(0);
    expect(result.rejected[0].reason).toContain("20");
  });

  it("accepts image at exactly MAX_IMAGE_SIZE", async () => {
    const { attachments, addFiles } = useAttachments();
    const atLimit = makeFile("at.png", MAX_IMAGE_SIZE, "image/png");
    const result = await addFiles([atLimit]);
    expect(result.added.length).toBe(1);
    expect(attachments.value.length).toBe(1);
  });

  it("returns empty result for null FileList", async () => {
    const { addFiles } = useAttachments();
    const result = await addFiles(null);
    expect(result).toEqual({ added: [], rejected: [] });
  });

  it("returns empty result for empty FileList", async () => {
    const { addFiles } = useAttachments();
    const result = await addFiles([]);
    expect(result).toEqual({ added: [], rejected: [] });
  });

  it("truncates batch beyond maxFilesPerBatch and calls onError", async () => {
    const onError = vi.fn();
    const { addFiles } = useAttachments({ maxFilesPerBatch: 3, onError });
    const files = Array.from({ length: 5 }, (_, i) => makeFile(`f${i}.png`, 100, "image/png"));
    const result = await addFiles(files);
    expect(result.added.length).toBe(3);
    expect(onError).toHaveBeenCalled();
  });

  it("appends to existing attachments (does not replace)", async () => {
    const { attachments, addFiles } = useAttachments();
    await addFiles([makeFile("a.png", 100, "image/png")]);
    await addFiles([makeFile("b.pdf", 100, "application/pdf")]);
    expect(attachments.value.length).toBe(2);
    expect(attachments.value[0].name).toBe("a.png");
    expect(attachments.value[1].name).toBe("b.pdf");
  });
});

describe("useAttachments.removeAttachment", () => {
  it("removes attachment by id", async () => {
    const { attachments, addFiles, removeAttachment } = useAttachments();
    await addFiles([makeFile("a.png", 100, "image/png")]);
    await addFiles([makeFile("b.png", 100, "image/png")]);
    const idToRemove = attachments.value[0].id;
    removeAttachment(idToRemove);
    expect(attachments.value.length).toBe(1);
    expect(attachments.value[0].id).not.toBe(idToRemove);
  });

  it("does nothing for unknown id", async () => {
    const { attachments, addFiles, removeAttachment } = useAttachments();
    await addFiles([makeFile("a.png", 100, "image/png")]);
    removeAttachment("non-existent-id");
    expect(attachments.value.length).toBe(1);
  });
});

describe("useAttachments.clearAttachments", () => {
  it("clears all attachments", async () => {
    const { attachments, addFiles, clearAttachments } = useAttachments();
    await addFiles([makeFile("a.png", 100, "image/png")]);
    await addFiles([makeFile("b.pdf", 100, "application/pdf")]);
    clearAttachments();
    expect(attachments.value.length).toBe(0);
  });
});

describe("useAttachments.serialize", () => {
  it("serializes text only when no attachments", async () => {
    const { serialize } = useAttachments();
    const parts = serialize("hello");
    expect(parts).toEqual([{ type: "text", text: "hello" }]);
  });

  it("serializes text + image + file in OpenAI multimodal order", async () => {
    const { addFiles, serialize } = useAttachments();
    await addFiles([makeFile("photo.png", 100, "image/png"), makeFile("doc.pdf", 100, "application/pdf")]);
    const parts = serialize("describe this");
    expect(parts.length).toBe(3);
    expect(parts[0]).toEqual({ type: "text", text: "describe this" });
    expect(parts[1].type).toBe("image_url");
    expect((parts[1] as any).image_url.url).toMatch(/^data:image\/png;base64,/);
    expect(parts[2].type).toBe("file");
    expect((parts[2] as any).file.filename).toBe("doc.pdf");
    expect((parts[2] as any).file.file_data).toMatch(/^data:application\/pdf;base64,/);
  });

  it("omits text element when text is empty (attachments only)", async () => {
    const { addFiles, serialize } = useAttachments();
    await addFiles([makeFile("a.png", 100, "image/png")]);
    const parts = serialize("");
    expect(parts.length).toBe(1);
    expect(parts[0].type).toBe("image_url");
  });
});

describe("serializeAttachments (pure function)", () => {
  it("returns empty array for empty text and no attachments", () => {
    expect(serializeAttachments("", [])).toEqual([]);
  });

  it("returns just text when no attachments", () => {
    expect(serializeAttachments("hi", [])).toEqual([{ type: "text", text: "hi" }]);
  });

  it("encodes attachments in input order", () => {
    const atts: Attachment[] = [
      {
        id: "1",
        name: "a.png",
        mimeType: "image/png",
        sizeBytes: 1,
        dataUrl: "data:image/png;base64,AAA",
        kind: "image",
      },
      {
        id: "2",
        name: "b.txt",
        mimeType: "text/plain",
        sizeBytes: 1,
        dataUrl: "data:text/plain;base64,QQ==",
        kind: "file",
      },
    ];
    const parts = serializeAttachments("hi", atts);
    expect(parts).toEqual([
      { type: "text", text: "hi" },
      { type: "image_url", image_url: { url: "data:image/png;base64,AAA" } },
      { type: "file", file: { filename: "b.txt", file_data: "data:text/plain;base64,QQ==" } },
    ]);
  });
});
