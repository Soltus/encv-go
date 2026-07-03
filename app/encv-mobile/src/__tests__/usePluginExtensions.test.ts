import { beforeEach, describe, expect, it, vi } from "vitest";
import { type ContainerExtensionsResponse, fetchContainerExtensions } from "@/api/encv";
import { usePluginExtensions } from "@/composables/usePluginExtensions";

vi.mock("@/api/encv", () => ({
  fetchContainerExtensions: vi.fn(),
}));

const mockedFetch = vi.mocked(fetchContainerExtensions);

function setupMockData(overrides?: Partial<ContainerExtensionsResponse>): ContainerExtensionsResponse {
  return {
    extensions: {
      ".sccgv": "video",
      ".sccga": "audio",
      ".sccgi": "image",
      ".sccgt": "text",
      ".myenc": "alist_encrypt",
    },
    conflicts: [],
    ...overrides,
  };
}

describe("usePluginExtensions", () => {
  beforeEach(() => {
    const { invalidate } = usePluginExtensions();
    invalidate();
    vi.clearAllMocks();
  });

  describe("getConflictingPlugins", () => {
    it("无冲突后缀应返回空数组", async () => {
      const mockData = setupMockData();
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins(".unknown");
      expect(result).toEqual([]);
    });

    it('与 video 冲突时应返回 ["video"]', async () => {
      const mockData = setupMockData();
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins(".sccgv");
      expect(result).toEqual(["video"]);
    });

    it("传入无前缀点号的 suffix 应自动规范化后再检测", async () => {
      const mockData = setupMockData();
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins("sccgv");
      expect(result).toEqual(["video"]);
    });

    it("传入空值或仅点号应返回空数组（空值安全）", async () => {
      const mockData = setupMockData();
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      expect(getConflictingPlugins("")).toEqual([]);
      expect(getConflictingPlugins(".")).toEqual([]);
    });
  });

  describe("加密容器后缀名冲突检测（Alist-Encrypt 场景）", () => {
    it("Alist-Encrypt 使用 .sccgv 应检测到与 video 插件冲突", async () => {
      const mockData = setupMockData({
        conflicts: [{ extension: ".sccgv", pluginNames: ["video"] }],
      });
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins(".sccgv");
      expect(result).toContain("video");
      expect(result.length).toBeGreaterThanOrEqual(1);
    });

    it("Alist-Encrypt 使用 .sccga 应检测到与 audio 插件冲突", async () => {
      const mockData = setupMockData({
        conflicts: [{ extension: ".sccga", pluginNames: ["audio"] }],
      });
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins(".sccga");
      expect(result).toContain("audio");
    });

    it("Alist-Encrypt 使用 .sccgi 应检测到与 image 插件冲突", async () => {
      const mockData = setupMockData({
        conflicts: [{ extension: ".sccgi", pluginNames: ["image"] }],
      });
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins(".sccgi");
      expect(result).toContain("image");
    });

    it("Alist-Encrypt 使用 .sccgt 应检测到与 text 插件冲突", async () => {
      const mockData = setupMockData({
        conflicts: [{ extension: ".sccgt", pluginNames: ["text"] }],
      });
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins(".sccgt");
      expect(result).toContain("text");
    });

    it("Alist-Encrypt 使用唯一后缀 .myenc 不应触发冲突", async () => {
      const mockData = setupMockData({
        extensions: {
          ".sccgv": "video",
          ".sccga": "audio",
          ".sccgi": "image",
          ".sccgt": "text",
        },
        conflicts: [],
      });
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins(".myenc");
      expect(result).toEqual([]);
    });

    it("大小写不敏感：SCCGV 应与 .sccgv 视为相同后缀", async () => {
      const mockData = setupMockData({
        conflicts: [{ extension: ".sccgv", pluginNames: ["video"] }],
      });
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      expect(getConflictingPlugins("SCCGV")).toEqual(["video"]);
      expect(getConflictingPlugins("SccGv")).toEqual(["video"]);
    });

    it("多插件同时声明同一扩展名时冲突列表应包含所有插件", async () => {
      const mockData = setupMockData({
        extensions: {
          ".sccgv": "video",
          ".sccga": "audio",
        },
        conflicts: [{ extension: ".custom", pluginNames: ["video", "audio"] }],
      });
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const result = getConflictingPlugins(".custom");
      expect(result).toContain("video");
      expect(result).toContain("audio");
      expect(result.length).toBe(2);
    });
  });

  describe("API 不可用时的阻断行为（防御深度）", () => {
    it("未调用 load() 时 getConflictingPlugins 应返回 [UNAVAILABLE] 阻断保存", () => {
      const { getConflictingPlugins, UNAVAILABLE } = usePluginExtensions();

      // 不调用 load()，模拟 API 不可用
      // 必须返回 UNAVAILABLE 标记，触发 disabled 保存按钮
      const result = getConflictingPlugins(".sccgv");
      expect(result).toEqual([UNAVAILABLE]);
      expect(result.length).toBe(1);
    });

    it("UNAVAILABLE 标记使 suffixConflict.length > 0 成立，禁用保存", () => {
      const { getConflictingPlugins, UNAVAILABLE } = usePluginExtensions();

      const suffixConflict = getConflictingPlugins(".any-value");
      expect(suffixConflict.length).toBeGreaterThan(0);
      expect(suffixConflict).toContain(UNAVAILABLE);

      // 这对应 PluginSettings.vue 的 :disabled 条件
      const shouldDisableSave = suffixConflict.length > 0;
      expect(shouldDisableSave).toBe(true);
    });

    it("isExtensionCheckAvailable 在未加载时应返回 false", () => {
      const { isExtensionCheckAvailable } = usePluginExtensions();

      expect(isExtensionCheckAvailable()).toBe(false);
    });

    it("isExtensionCheckAvailable 在加载成功后应返回 true", async () => {
      const mockData = setupMockData();
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, isExtensionCheckAvailable } = usePluginExtensions();
      await load();

      expect(isExtensionCheckAvailable()).toBe(true);
    });

    it("load() 失败后 isExtensionCheckAvailable 仍为 false（data 未设置）", async () => {
      mockedFetch.mockRejectedValueOnce(new Error("404 Not Found"));

      const { load, isExtensionCheckAvailable, getConflictingPlugins, UNAVAILABLE } = usePluginExtensions();
      await expect(load()).rejects.toThrow();

      expect(isExtensionCheckAvailable()).toBe(false);
      expect(getConflictingPlugins(".sccgv")).toEqual([UNAVAILABLE]);
    });

    it("invalidate() 后恢复为不可用状态", async () => {
      const mockData = setupMockData();
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, invalidate, isExtensionCheckAvailable, getConflictingPlugins, UNAVAILABLE } = usePluginExtensions();
      await load();
      expect(isExtensionCheckAvailable()).toBe(true);
      expect(getConflictingPlugins(".sccgv")).toEqual(["video"]);

      invalidate();

      expect(isExtensionCheckAvailable()).toBe(false);
      expect(getConflictingPlugins(".sccgv")).toEqual([UNAVAILABLE]);
    });
  });

  describe("数据加载与缓存", () => {
    it("load() 后 extensions 数据应可用", async () => {
      const mockData = setupMockData();
      mockedFetch.mockResolvedValueOnce(mockData);

      const { load, getExtensions } = usePluginExtensions();
      await load();

      const extMap = getExtensions();
      expect(extMap).toBeDefined();
      expect(Object.keys(extMap!).length).toBeGreaterThanOrEqual(5);
    });

    it("invalidate() 后应重新加载数据", async () => {
      const firstData = setupMockData({ extensions: { ".old": "plugin1" } });
      const secondData = setupMockData({ extensions: { ".new": "plugin2" } });

      mockedFetch.mockResolvedValueOnce(firstData);
      const { load, getExtensions, invalidate } = usePluginExtensions();
      await load();
      expect(getExtensions()).toEqual(expect.objectContaining({ ".old": "plugin1" }));

      invalidate();
      vi.clearAllMocks();

      mockedFetch.mockResolvedValueOnce(secondData);
      await load();
      expect(getExtensions()).toHaveProperty(".new");
    });
  });
});
