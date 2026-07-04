/**
 * useTestCaseGeneration 单元测试
 *
 * 重点覆盖：
 * 1. categoryForExt：ext → category 映射（含带点前缀 / 未知 ext）
 * 2. sampleFileForCategory：category → sample 文件名
 * 3. selectSourcePath：路径拼接正确性
 * 4. deriveExtraFieldCombinations：笛卡尔积展开（select 字段 / bool 忽略 / 单选项忽略）
 * 5. generateCases：用例数量 / id 唯一性 / extraFields 填充
 */
import { describe, expect, it } from "vitest";
import { ref } from "vue";
import type { PluginMeta, TaskField, TaskOptions } from "@/api/encv";
import { useTestCaseGeneration } from "@/composables/useTestCaseGeneration";

// ==================== 测试夹具 ====================

function makeTaskField(overrides: Partial<TaskField> = {}): TaskField {
  return {
    key: "field",
    label: "field",
    type: "string",
    required: false,
    defaultValue: "",
    help: "",
    ...overrides,
  };
}

function makeTaskOptions(overrides: Partial<TaskOptions> = {}): TaskOptions {
  return {
    passwordStrategy: "global",
    supportVersionSelect: true,
    supportedVersions: [4],
    defaultVersion: 4,
    extraFields: [],
    ...overrides,
  };
}

function makePlugin(name: string, overrides: Partial<PluginMeta> = {}): PluginMeta {
  return {
    name,
    supportedExtensions: [".mp4"],
    supportedMimePrefixes: [],
    containerExtension: "encv",
    taskOptions: makeTaskOptions(),
    ...overrides,
  };
}

// ==================== categoryForExt ====================

describe("useTestCaseGeneration.categoryForExt", () => {
  const { categoryForExt } = useTestCaseGeneration({
    mockRoot: ref("/d/automation/"),
    plugins: ref([]),
  });

  it("视频 ext（mp4/mkv/avi/mov/webm/flv/wmv）→ video", () => {
    expect(categoryForExt("mp4")).toBe("video");
    expect(categoryForExt("mkv")).toBe("video");
    expect(categoryForExt("avi")).toBe("video");
    expect(categoryForExt("mov")).toBe("video");
    expect(categoryForExt("webm")).toBe("video");
    expect(categoryForExt("flv")).toBe("video");
    expect(categoryForExt("wmv")).toBe("video");
  });

  it("音频 ext（mp3/flac/ogg/m4a/wav/aac/opus）→ audio", () => {
    expect(categoryForExt("mp3")).toBe("audio");
    expect(categoryForExt("flac")).toBe("audio");
    expect(categoryForExt("ogg")).toBe("audio");
    expect(categoryForExt("m4a")).toBe("audio");
    expect(categoryForExt("wav")).toBe("audio");
    expect(categoryForExt("aac")).toBe("audio");
    expect(categoryForExt("opus")).toBe("audio");
  });

  it("图片 ext（png/jpg/jpeg/gif/webp/bmp/tiff）→ image", () => {
    expect(categoryForExt("png")).toBe("image");
    expect(categoryForExt("jpg")).toBe("image");
    expect(categoryForExt("jpeg")).toBe("image");
    expect(categoryForExt("gif")).toBe("image");
    expect(categoryForExt("webp")).toBe("image");
    expect(categoryForExt("bmp")).toBe("image");
    expect(categoryForExt("tiff")).toBe("image");
  });

  it("pdf → pdf", () => {
    expect(categoryForExt("pdf")).toBe("pdf");
  });

  it("WPS ext（doc/docx/xls/xlsx/ppt/pptx）→ wps", () => {
    expect(categoryForExt("doc")).toBe("wps");
    expect(categoryForExt("docx")).toBe("wps");
    expect(categoryForExt("xls")).toBe("wps");
    expect(categoryForExt("xlsx")).toBe("wps");
    expect(categoryForExt("ppt")).toBe("wps");
    expect(categoryForExt("pptx")).toBe("wps");
  });

  it("文本 ext（txt/md/rtf/log）→ text", () => {
    expect(categoryForExt("txt")).toBe("text");
    expect(categoryForExt("md")).toBe("text");
    expect(categoryForExt("rtf")).toBe("text");
    expect(categoryForExt("log")).toBe("text");
  });

  it("加密 ext（encv/ae）→ alist-encrypted", () => {
    expect(categoryForExt("encv")).toBe("alist-encrypted");
    expect(categoryForExt("ae")).toBe("alist-encrypted");
  });

  it("未知 ext → misc", () => {
    expect(categoryForExt("unknown")).toBe("misc");
    expect(categoryForExt("xyz")).toBe("misc");
    expect(categoryForExt("")).toBe("misc");
  });

  it("容错：支持带点前缀（.mp4 → video）", () => {
    expect(categoryForExt(".mp4")).toBe("video");
    expect(categoryForExt(".MP4")).toBe("video");
    expect(categoryForExt(".mkv")).toBe("video");
  });

  it("容错：大小写不敏感（MP4/MP3/JPG → video/audio/image）", () => {
    expect(categoryForExt("MP4")).toBe("video");
    expect(categoryForExt("MP3")).toBe("audio");
    expect(categoryForExt("JPG")).toBe("image");
    expect(categoryForExt("PDF")).toBe("pdf");
  });
});

// ==================== sampleFileForCategory ====================

describe("useTestCaseGeneration.sampleFileForCategory", () => {
  const { sampleFileForCategory } = useTestCaseGeneration({
    mockRoot: ref("/d/automation/"),
    plugins: ref([]),
  });

  it("各 category 返回对应 sample 文件名", () => {
    expect(sampleFileForCategory("video")).toBe("sample.mp4");
    expect(sampleFileForCategory("audio")).toBe("sample.mp3");
    expect(sampleFileForCategory("image")).toBe("sample.png");
    expect(sampleFileForCategory("pdf")).toBe("sample.pdf");
    expect(sampleFileForCategory("wps")).toBe("sample.docx");
    expect(sampleFileForCategory("text")).toBe("sample.txt");
    expect(sampleFileForCategory("alist-encrypted")).toBe("sample.encv");
  });

  it("未知 category → sample.bin", () => {
    expect(sampleFileForCategory("misc")).toBe("sample.bin");
    expect(sampleFileForCategory("unknown")).toBe("sample.bin");
    expect(sampleFileForCategory("")).toBe("sample.bin");
  });
});

// ==================== selectSourcePath ====================

describe("useTestCaseGeneration.selectSourcePath", () => {
  it("按 supportedExtensions[0] 拼接正确路径", () => {
    const { selectSourcePath } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins: ref([]),
    });
    const plugin = makePlugin("video-plugin", { supportedExtensions: [".mp4"] });
    expect(selectSourcePath(plugin)).toBe("/d/automation/01-plain-media/video/sample.mp4");
  });

  it("音频 plugin → audio/sample.mp3", () => {
    const { selectSourcePath } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins: ref([]),
    });
    const plugin = makePlugin("audio-plugin", { supportedExtensions: [".mp3"] });
    expect(selectSourcePath(plugin)).toBe("/d/automation/01-plain-media/audio/sample.mp3");
  });

  it("空 supportedExtensions → misc/sample.bin", () => {
    const { selectSourcePath } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins: ref([]),
    });
    const plugin = makePlugin("empty", { supportedExtensions: [] });
    expect(selectSourcePath(plugin)).toBe("/d/automation/01-plain-media/misc/sample.bin");
  });

  it("mockRoot 变化时路径同步变化", () => {
    const mockRoot = ref("/d/automation/");
    const { selectSourcePath } = useTestCaseGeneration({
      mockRoot,
      plugins: ref([]),
    });
    const plugin = makePlugin("p", { supportedExtensions: [".mp4"] });
    expect(selectSourcePath(plugin)).toBe("/d/automation/01-plain-media/video/sample.mp4");
    mockRoot.value = "/tmp/mock/";
    expect(selectSourcePath(plugin)).toBe("/tmp/mock/01-plain-media/video/sample.mp4");
  });
});

// ==================== deriveExtraFieldCombinations ====================

describe("useTestCaseGeneration.deriveExtraFieldCombinations", () => {
  const { deriveExtraFieldCombinations } = useTestCaseGeneration({
    mockRoot: ref("/d/automation/"),
    plugins: ref([]),
  });

  it("无 extraFields → 返回 [{}]", () => {
    const plugin = makePlugin("p", { taskOptions: makeTaskOptions({ extraFields: [] }) });
    const combos = deriveExtraFieldCombinations(plugin);
    expect(combos).toEqual([{}]);
  });

  it("1 个 select 字段 2 选项 → 2 个组合", () => {
    const plugin = makePlugin("p", {
      taskOptions: makeTaskOptions({
        extraFields: [makeTaskField({ key: "cipherMode", type: "select", options: ["0", "1"] })],
      }),
    });
    const combos = deriveExtraFieldCombinations(plugin);
    expect(combos).toHaveLength(2);
    expect(combos).toContainEqual({ cipherMode: "0" });
    expect(combos).toContainEqual({ cipherMode: "1" });
  });

  it("2 个 select 字段各 2 选项 → 4 个组合（笛卡尔积）", () => {
    const plugin = makePlugin("p", {
      taskOptions: makeTaskOptions({
        extraFields: [
          makeTaskField({ key: "cipherMode", type: "select", options: ["0", "1"] }),
          makeTaskField({ key: "compressionMode", type: "select", options: ["none", "zstd"] }),
        ],
      }),
    });
    const combos = deriveExtraFieldCombinations(plugin);
    expect(combos).toHaveLength(4);
    expect(combos).toContainEqual({ cipherMode: "0", compressionMode: "none" });
    expect(combos).toContainEqual({ cipherMode: "0", compressionMode: "zstd" });
    expect(combos).toContainEqual({ cipherMode: "1", compressionMode: "none" });
    expect(combos).toContainEqual({ cipherMode: "1", compressionMode: "zstd" });
  });

  it("bool 字段被忽略（不参与笛卡尔积）", () => {
    const plugin = makePlugin("p", {
      taskOptions: makeTaskOptions({
        extraFields: [
          makeTaskField({ key: "cipherMode", type: "select", options: ["0", "1"] }),
          makeTaskField({ key: "fastMode", type: "bool" }),
          makeTaskField({ key: "verbose", type: "bool" }),
        ],
      }),
    });
    const combos = deriveExtraFieldCombinations(plugin);
    // bool 字段被忽略，只有 select 字段参与 → 2 个组合
    expect(combos).toHaveLength(2);
    expect(combos.every(c => !("fastMode" in c))).toBe(true);
    expect(combos.every(c => !("verbose" in c))).toBe(true);
  });

  it("select 字段只有 1 个选项 → 被忽略（不参与笛卡尔积）", () => {
    const plugin = makePlugin("p", {
      taskOptions: makeTaskOptions({
        extraFields: [
          makeTaskField({ key: "single", type: "select", options: ["only"] }),
          makeTaskField({ key: "cipherMode", type: "select", options: ["0", "1"] }),
        ],
      }),
    });
    const combos = deriveExtraFieldCombinations(plugin);
    // single 只有 1 选项被忽略，只有 cipherMode 参与 → 2 个组合
    expect(combos).toHaveLength(2);
    expect(combos.every(c => !("single" in c))).toBe(true);
  });

  it("string / password 字段被忽略", () => {
    const plugin = makePlugin("p", {
      taskOptions: makeTaskOptions({
        extraFields: [
          makeTaskField({ key: "name", type: "string" }),
          makeTaskField({ key: "pwd", type: "password" }),
          makeTaskField({ key: "cipherMode", type: "select", options: ["0", "1"] }),
        ],
      }),
    });
    const combos = deriveExtraFieldCombinations(plugin);
    expect(combos).toHaveLength(2);
    expect(combos.every(c => !("name" in c))).toBe(true);
    expect(combos.every(c => !("pwd" in c))).toBe(true);
  });

  it("3 个 select 字段各 2 选项 → 8 个组合", () => {
    const plugin = makePlugin("p", {
      taskOptions: makeTaskOptions({
        extraFields: [
          makeTaskField({ key: "a", type: "select", options: ["a1", "a2"] }),
          makeTaskField({ key: "b", type: "select", options: ["b1", "b2"] }),
          makeTaskField({ key: "c", type: "select", options: ["c1", "c2"] }),
        ],
      }),
    });
    const combos = deriveExtraFieldCombinations(plugin);
    expect(combos).toHaveLength(8);
  });

  it("taskOptions 为 null 时容错返回 [{}]", () => {
    // 运行时可能遇到 taskOptions 为 null 的脏数据
    const plugin = { name: "broken", supportedExtensions: [".mp4"], taskOptions: null } as unknown as PluginMeta;
    const combos = deriveExtraFieldCombinations(plugin);
    expect(combos).toEqual([{}]);
  });
});

// ==================== generateCases ====================

describe("useTestCaseGeneration.generateCases", () => {
  it("单 plugin 单版本无 extraFields → 1 个用例", () => {
    const plugins = ref<PluginMeta[]>([makePlugin("video-plugin", { supportedExtensions: [".mp4"] })]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", [4]);
    expect(cases).toHaveLength(1);
    expect(cases[0].pluginName).toBe("video-plugin");
    expect(cases[0].taskType).toBe("encrypt");
    expect(cases[0].version).toBe(4);
    expect(cases[0].sourcePath).toBe("/d/automation/01-plain-media/video/sample.mp4");
    expect(cases[0].extraFields).toEqual({});
    expect(cases[0].expectedBehavior).toBe("might-fail");
  });

  it("单 plugin 多版本 → 每版本 1 个用例", () => {
    const plugins = ref<PluginMeta[]>([makePlugin("p")]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", [2, 3, 4]);
    expect(cases).toHaveLength(3);
    expect(cases.map(c => c.version).sort()).toEqual([2, 3, 4]);
  });

  it("带 select extraFields → 笛卡尔积展开", () => {
    const plugins = ref<PluginMeta[]>([
      makePlugin("p", {
        taskOptions: makeTaskOptions({
          extraFields: [
            makeTaskField({ key: "cipherMode", type: "select", options: ["0", "1"] }),
            makeTaskField({ key: "compressionMode", type: "select", options: ["none", "zstd"] }),
          ],
        }),
      }),
    ]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", [4]);
    // 2 cipher × 2 compression = 4
    expect(cases).toHaveLength(4);
    const combos = cases.map(c => `${c.extraFields.cipherMode}-${c.extraFields.compressionMode}`).sort();
    expect(combos).toEqual(["0-none", "0-zstd", "1-none", "1-zstd"]);
  });

  it("多 plugin × 多版本 → 数量正确", () => {
    const plugins = ref<PluginMeta[]>([
      makePlugin("p1", { supportedExtensions: [".mp4"] }),
      makePlugin("p2", { supportedExtensions: [".mp3"] }),
    ]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", [3, 4]);
    // 2 plugin × 2 version = 4
    expect(cases).toHaveLength(4);
  });

  it("所有用例 id 唯一", () => {
    const plugins = ref<PluginMeta[]>([
      makePlugin("p1", {
        taskOptions: makeTaskOptions({
          extraFields: [makeTaskField({ key: "cipherMode", type: "select", options: ["0", "1"] })],
        }),
      }),
      makePlugin("p2"),
    ]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", [3, 4]);
    const ids = cases.map(c => c.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("id 格式包含 plugin / taskType / version / combo", () => {
    const plugins = ref<PluginMeta[]>([
      makePlugin("mp4-plugin", {
        taskOptions: makeTaskOptions({
          extraFields: [makeTaskField({ key: "cipherMode", type: "select", options: ["0", "1"] })],
        }),
      }),
    ]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", [4]);
    const withCombo = cases.find(c => c.extraFields.cipherMode === "1");
    expect(withCombo?.id).toBe("mp4-plugin-encrypt-v4-cipherMode=1");
    // 无 combo 时用 'default'
    const plugins2 = ref<PluginMeta[]>([makePlugin("plain")]);
    const { generateCases: gen2 } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins: plugins2,
    });
    const cases2 = gen2("decrypt", [3]);
    expect(cases2[0].id).toBe("plain-decrypt-v3-default");
  });

  it("sourcePath 按 plugin.supportedExtensions[0] 派生", () => {
    const plugins = ref<PluginMeta[]>([
      makePlugin("video", { supportedExtensions: [".mkv"] }),
      makePlugin("audio", { supportedExtensions: [".flac"] }),
      makePlugin("image", { supportedExtensions: [".png"] }),
    ]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", [4]);
    expect(cases[0].sourcePath).toBe("/d/automation/01-plain-media/video/sample.mp4");
    expect(cases[1].sourcePath).toBe("/d/automation/01-plain-media/audio/sample.mp3");
    expect(cases[2].sourcePath).toBe("/d/automation/01-plain-media/image/sample.png");
  });

  it("空 plugin 列表 → 空数组", () => {
    const plugins = ref<PluginMeta[]>([]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", [4]);
    expect(cases).toEqual([]);
  });

  it("空 versions 列表 → 空数组", () => {
    const plugins = ref<PluginMeta[]>([makePlugin("p")]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const cases = generateCases("encrypt", []);
    expect(cases).toEqual([]);
  });

  it("encrypt + decrypt 各生成一组用例", () => {
    const plugins = ref<PluginMeta[]>([makePlugin("p")]);
    const { generateCases } = useTestCaseGeneration({
      mockRoot: ref("/d/automation/"),
      plugins,
    });
    const encrypts = generateCases("encrypt", [4]);
    const decrypts = generateCases("decrypt", [4]);
    expect(encrypts.every(c => c.taskType === "encrypt")).toBe(true);
    expect(decrypts.every(c => c.taskType === "decrypt")).toBe(true);
    expect(encrypts).toHaveLength(1);
    expect(decrypts).toHaveLength(1);
  });
});
