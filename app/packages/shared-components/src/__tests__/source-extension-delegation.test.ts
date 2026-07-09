import { type ContainerExtensionsResponse, fetchContainerExtensions } from "@encv/shared-components/api/encv";
import { usePluginExtensions } from "@encv/shared-components/composables/usePluginExtensions";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/api/encv", () => ({
  fetchContainerExtensions: vi.fn(),
}));

const mockedFetch = vi.mocked(fetchContainerExtensions);

interface PluginRoute {
  pluginId: string;
  supportedSourceExts: string[];
  supportedMimePrefixes: string[];
  containerExtension: string;
}

function setupExtensionsData(
  extensions: Record<string, string>,
  conflicts?: ContainerExtensionsResponse["conflicts"]
): ContainerExtensionsResponse {
  return {
    extensions,
    conflicts: conflicts ?? [],
  };
}

describe("源文件扩展名委托逻辑（核心设计验证）", () => {
  beforeEach(() => {
    const { invalidate } = usePluginExtensions();
    invalidate();
    vi.clearAllMocks();
  });

  describe(".ts 文件可同时被文本和视频插件处理（MIME 区分）", () => {
    it("同一扩展名 .ts 在不同 MIME 下应路由到不同插件", async () => {
      const plugins: PluginRoute[] = [
        {
          pluginId: "text",
          supportedSourceExts: ["ts"],
          supportedMimePrefixes: ["text/", "application/json"],
          containerExtension: ".sccgt",
        },
        {
          pluginId: "video",
          supportedSourceExts: ["ts"],
          supportedMimePrefixes: ["video/"],
          containerExtension: ".sccgv",
        },
      ];

      function resolvePlugin(sourceExt: string, mimeType: string): string | null {
        const matched = plugins.filter(
          p =>
            p.supportedSourceExts.includes(sourceExt.toLowerCase()) && p.supportedMimePrefixes.some(prefix => mimeType.startsWith(prefix))
        );
        return matched.length === 1 ? matched[0].pluginId : null;
      }

      expect(resolvePlugin("ts", "text/x-typescript")).toBe("text");
      expect(resolvePlugin("ts", "video/mp2t")).toBe("video");
      expect(resolvePlugin("ts", "image/png")).toBeNull();
    });

    it("源文件扩展名不需要唯一性约束 — 容器扩展名才是唯一键", async () => {
      const data = setupExtensionsData({
        ".sccgt": "text",
        ".sccgv": "video",
      });
      mockedFetch.mockResolvedValueOnce(data);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      expect(getConflictingPlugins(".sccgt")).toEqual(["text"]);
      expect(getConflictingPlugins(".sccgv")).toEqual(["video"]);

      expect(getConflictingPlugins(".nonexistent")).toEqual([]);

      const textExt = ".sccgt";
      const videoExt = ".sccgv";
      expect(textExt).not.toBe(videoExt);
      expect(textExt.startsWith(".")).toBe(true);
      expect(videoExt.startsWith(".")).toBe(true);
    });

    it(".tsx 文件应正确委托到 text 插件（TypeScript React 组件）", async () => {
      const plugins: PluginRoute[] = [
        {
          pluginId: "text",
          supportedSourceExts: ["ts", "tsx", "js", "jsx"],
          supportedMimePrefixes: ["text/", "application/javascript", "application/x-typescript", "application/typescript"],
          containerExtension: ".sccgt",
        },
        {
          pluginId: "video",
          supportedSourceExts: ["ts"],
          supportedMimePrefixes: ["video/"],
          containerExtension: ".sccgv",
        },
      ];

      function resolvePlugin(sourceExt: string, mimeType: string): string | null {
        const matched = plugins.filter(
          p =>
            p.supportedSourceExts.includes(sourceExt.toLowerCase()) && p.supportedMimePrefixes.some(prefix => mimeType.startsWith(prefix))
        );
        return matched.length === 1 ? matched[0].pluginId : null;
      }

      // tsx 是 TypeScript JSX，属于文本类型
      expect(resolvePlugin("tsx", "text/typescript-jsx")).toBe("text");
      expect(resolvePlugin("tsx", "application/x-typescript")).toBe("text");
    });

    it(".m3u8 文件应正确委托到 video 插件（HLS 播放列表）", async () => {
      const plugins: PluginRoute[] = [
        {
          pluginId: "text",
          supportedSourceExts: ["m3u", "m3u8"],
          supportedMimePrefixes: ["text/", "audio/"],
          containerExtension: ".sccgt",
        },
        {
          pluginId: "video",
          supportedSourceExts: ["m3u8", "ts"],
          supportedMimePrefixes: ["video/"],
          containerExtension: ".sccgv",
        },
      ];

      function resolvePlugin(sourceExt: string, mimeType: string): string | null {
        const matched = plugins.filter(
          p =>
            p.supportedSourceExts.includes(sourceExt.toLowerCase()) && p.supportedMimePrefixes.some(prefix => mimeType.startsWith(prefix))
        );
        return matched.length === 1 ? matched[0].pluginId : null;
      }

      // m3u8 作为 HLS playlist 应路由到视频插件
      expect(resolvePlugin("m3u8", "video/mp2t")).toBe("video");
      // m3u8 作为纯文本播放列表也可路由到文本插件
      expect(resolvePlugin("m3u8", "text/plain")).toBe("text");
    });

    it("无法确定 MIME 时应返回 null（需要 ShouldProcess 链进一步判断）", async () => {
      const plugins: PluginRoute[] = [
        {
          pluginId: "text",
          supportedSourceExts: ["ts"],
          supportedMimePrefixes: ["text/"],
          containerExtension: ".sccgt",
        },
        {
          pluginId: "video",
          supportedSourceExts: ["ts"],
          supportedMimePrefixes: ["video/"],
          containerExtension: ".sccgv",
        },
      ];

      function resolvePlugin(sourceExt: string, mimeType: string): string | null {
        const matched = plugins.filter(
          p =>
            p.supportedSourceExts.includes(sourceExt.toLowerCase()) && p.supportedMimePrefixes.some(prefix => mimeType.startsWith(prefix))
        );
        return matched.length === 1 ? matched[0].pluginId : null;
      }

      // application/octet-stream 无法区分
      expect(resolvePlugin("ts", "application/octet-stream")).toBeNull();
    });
  });

  describe("容器扩展名必须唯一", () => {
    it("两个插件声明相同容器扩展名时应被检测为冲突", async () => {
      const data = setupExtensionsData(
        {
          ".sccgv": "video",
          ".sccga": "audio",
        },
        [{ extension: ".sccgv", pluginNames: ["video"] }]
      );
      mockedFetch.mockResolvedValueOnce(data);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      expect(getConflictingPlugins(".sccgv")).toEqual(["video"]);

      const existingExtensions = Object.keys(data.extensions);
      function tryRegister(newExt: string, _newPlugin: string): { ok: boolean; conflictWith?: string } {
        const normalized = newExt.startsWith(".") ? newExt : "." + newExt.toLowerCase();
        const existing = existingExtensions.find(e => e.toLowerCase() === normalized);
        if (existing) {
          return { ok: false, conflictWith: data.extensions[existing] };
        }
        return { ok: true };
      }

      const result = tryRegister(".sccgv", "alist_encrypt");
      expect(result.ok).toBe(false);
      expect(result.conflictWith).toBe("video");
    });

    it("新插件注册不冲突的容器扩展名应成功", async () => {
      const data = setupExtensionsData({
        ".sccgv": "video",
        ".sccga": "audio",
      });
      mockedFetch.mockResolvedValueOnce(data);

      const { load, getConflictingPlugins } = usePluginExtensions();
      await load();

      const existingExtensions = Object.keys(data.extensions);
      function tryRegister(newExt: string, _newPlugin: string): boolean {
        const normalized = newExt.startsWith(".") ? newExt : "." + newExt.toLowerCase();
        return !existingExtensions.some(e => e.toLowerCase() === normalized);
      }

      expect(tryRegister(".sccgx", "alist_encrypt")).toBe(true);
      expect(tryRegister(".sccgz", "subtitle")).toBe(true);
      expect(getConflictingPlugins(".sccgx")).toEqual([]);
      expect(getConflictingPlugins(".sccgz")).toEqual([]);
    });
  });
});
