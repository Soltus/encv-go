/**
 * useSectionDerivation 单元测试
 *
 * 覆盖：
 * 1. deriveSubSection 4 种 dimension 派生（plugin / type / category / none）
 * 2. categoryForExt ext → category 映射（video / audio / image / pdf / wps / text / alist-encrypted / misc）
 * 3. categoryLabel category → 中文 label 映射
 * 4. useSectionDerivation composable（静态维度 + 响应式维度）
 */
import { describe, expect, it } from "vitest";
import { computed, nextTick, ref } from "vue";
import type { EncvTask } from "@encv/shared-components/api/encv";
import {
  categoryForExt,
  categoryLabel,
  deriveSubSection,
  type SectionDimension,
  useSectionDerivation,
} from "@encv/shared-components/composables/useSectionDerivation";

// 构造测试用 EncvTask（只填派生关心的字段，其他用合理默认值）
function makeTask(overrides: Partial<EncvTask> = {}): EncvTask {
  return {
    id: "test-id",
    type: "encrypt",
    sourcePath: "/storage/test.mp4",
    status: "queued",
    progress: 0,
    createdAt: "2026-06-18T00:00:00Z",
    ...overrides,
  };
}

describe("deriveSubSection — dimension=plugin", () => {
  it("task 有 pluginName 时返回 plugin 维度", () => {
    const task = makeTask({ pluginName: "mpv-player" });
    const meta = deriveSubSection(task, "plugin");
    expect(meta.dimension).toBe("plugin");
    expect(meta.key).toBe("mpv-player");
    expect(meta.label).toBe("mpv-player");
  });

  it("task 无 pluginName 时 fallback 到 unknown", () => {
    const task = makeTask({ pluginName: undefined });
    const meta = deriveSubSection(task, "plugin");
    expect(meta.dimension).toBe("plugin");
    expect(meta.key).toBe("unknown");
    expect(meta.label).toBe("未知插件");
  });
});

describe("deriveSubSection — dimension=type", () => {
  it("type=encrypt 返回 encrypt 维度", () => {
    const task = makeTask({ type: "encrypt" });
    const meta = deriveSubSection(task, "type");
    expect(meta.dimension).toBe("type");
    expect(meta.key).toBe("encrypt");
    expect(meta.label).toBe("encrypt");
  });

  it("type=decrypt 返回 decrypt 维度", () => {
    const task = makeTask({ type: "decrypt" });
    const meta = deriveSubSection(task, "type");
    expect(meta.dimension).toBe("type");
    expect(meta.key).toBe("decrypt");
    expect(meta.label).toBe("decrypt");
  });
});

describe("deriveSubSection — dimension=category", () => {
  it("mp4 后缀派生为 video category", () => {
    const task = makeTask({ sourcePath: "/media/sample.mp4" });
    const meta = deriveSubSection(task, "category");
    expect(meta.dimension).toBe("category");
    expect(meta.key).toBe("video");
    expect(meta.label).toBe("视频");
  });

  it("mp3 后缀派生为 audio category", () => {
    const task = makeTask({ sourcePath: "/media/sample.mp3" });
    const meta = deriveSubSection(task, "category");
    expect(meta.key).toBe("audio");
    expect(meta.label).toBe("音频");
  });

  it("png 后缀派生为 image category", () => {
    const task = makeTask({ sourcePath: "/media/sample.png" });
    const meta = deriveSubSection(task, "category");
    expect(meta.key).toBe("image");
    expect(meta.label).toBe("图片");
  });

  it("pdf 后缀派生为 pdf category", () => {
    const task = makeTask({ sourcePath: "/docs/sample.pdf" });
    const meta = deriveSubSection(task, "category");
    expect(meta.key).toBe("pdf");
    expect(meta.label).toBe("PDF");
  });

  it("docx 后缀派生为 wps category", () => {
    const task = makeTask({ sourcePath: "/docs/sample.docx" });
    const meta = deriveSubSection(task, "category");
    expect(meta.key).toBe("wps");
    expect(meta.label).toBe("文档");
  });

  it("encv 后缀派生为 alist-encrypted category", () => {
    const task = makeTask({ sourcePath: "/secret/sample.encv" });
    const meta = deriveSubSection(task, "category");
    expect(meta.key).toBe("alist-encrypted");
    expect(meta.label).toBe("加密文件");
  });

  it("未知后缀派生为 misc category", () => {
    const task = makeTask({ sourcePath: "/misc/sample.xyz" });
    const meta = deriveSubSection(task, "category");
    expect(meta.key).toBe("misc");
    expect(meta.label).toBe("其他");
  });

  it("大写后缀也能正确派生（大小写不敏感）", () => {
    const task = makeTask({ sourcePath: "/media/sample.MP4" });
    const meta = deriveSubSection(task, "category");
    expect(meta.key).toBe("video");
  });
});

describe("deriveSubSection — dimension=none", () => {
  it("返回 all / 全部 兜底", () => {
    const task = makeTask();
    const meta = deriveSubSection(task, "none");
    expect(meta.dimension).toBe("none");
    expect(meta.key).toBe("all");
    expect(meta.label).toBe("全部");
  });
});

describe("categoryForExt", () => {
  it("video 后缀集合都映射到 video", () => {
    for (const ext of ["mp4", "mkv", "avi", "mov", "webm", "flv", "wmv"]) {
      expect(categoryForExt(ext)).toBe("video");
    }
  });

  it("带前导点的 ext 也能识别", () => {
    expect(categoryForExt(".mp4")).toBe("video");
    expect(categoryForExt(".pdf")).toBe("pdf");
  });

  it("空字符串 fallback 到 misc", () => {
    expect(categoryForExt("")).toBe("misc");
  });
});

describe("categoryLabel", () => {
  it("已知 category 返回中文 label", () => {
    expect(categoryLabel("video")).toBe("视频");
    expect(categoryLabel("audio")).toBe("音频");
    expect(categoryLabel("image")).toBe("图片");
    expect(categoryLabel("pdf")).toBe("PDF");
    expect(categoryLabel("wps")).toBe("文档");
    expect(categoryLabel("text")).toBe("文本");
    expect(categoryLabel("alist-encrypted")).toBe("加密文件");
    expect(categoryLabel("misc")).toBe("其他");
  });

  it("未知 category 返回原值", () => {
    expect(categoryLabel("unknown-cat")).toBe("unknown-cat");
  });
});

describe("useSectionDerivation composable", () => {
  it("静态维度：derive(task) 按固定维度派生", () => {
    const { derive } = useSectionDerivation("plugin");
    const task = makeTask({ pluginName: "ffmpeg" });
    const meta = derive(task);
    expect(meta.dimension).toBe("plugin");
    expect(meta.key).toBe("ffmpeg");
  });

  it("静态维度：返回的 deriveSubSection 与裸函数一致", () => {
    const { deriveSubSection: dimDerive } = useSectionDerivation("category");
    const task = makeTask({ sourcePath: "/a/b/test.mp3" });
    expect(dimDerive(task, "category")).toEqual(deriveSubSection(task, "category"));
  });

  it("响应式维度：dimension 变化时 derive 自动跟随", async () => {
    const dim = ref<SectionDimension>("plugin");
    const { derive } = useSectionDerivation(computed(() => dim.value));
    const task = makeTask({ pluginName: "mpv", sourcePath: "/x/test.mp4" });

    // 初始 plugin 维度
    expect(derive(task).key).toBe("mpv");

    // 切到 category 维度
    dim.value = "category";
    await nextTick();
    expect(derive(task).key).toBe("video");

    // 切到 none 维度
    dim.value = "none";
    await nextTick();
    expect(derive(task).key).toBe("all");
  });
});
