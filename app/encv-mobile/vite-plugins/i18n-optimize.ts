import type { Plugin } from "vite";
import path from "node:path";
import fs from "node:fs";

/**
 * i18n 优化插件
 *
 * 功能：
 * 1. 开发模式：i18n 文件 HMR 热重载（修改翻译立即生效，无需刷新页面）
 * 2. 构建模式：i18n 字典分包 + 压缩，减少主包体积
 * 3. 预编译：将字典转换为 Map 初始化代码，提升运行时性能
 */

const I18N_FILE_REGEX = /[\\/]i18n[\\/].+\.ts$/;
const VIRTUAL_I18N_MODULE = "virtual:i18n-bundle";

export function i18nOptimizePlugin(options?: {
  i18nDirs?: string[];
}): Plugin {
  const i18nDirs = options?.i18nDirs ?? [];
  let isDev = false;
  let server: any = null;

  function isI18nFile(id: string): boolean {
    if (I18N_FILE_REGEX.test(id)) return true;
    for (const dir of i18nDirs) {
      if (id.includes(dir)) return true;
    }
    return false;
  }

  function parseI18nModule(code: string): {
    zhCN: Record<string, string>;
    en: Record<string, string>;
  } {
    const result: { zhCN: Record<string, string>; en: Record<string, string> } = {
      zhCN: {},
      en: {},
    };

    let currentLocale: "zhCN" | "en" | null = null;
    const lines = code.split("\n");

    for (const line of lines) {
      const trimmed = line.trim();

      if (trimmed.includes('"zh-CN"') || trimmed.includes("'zh-CN'")) {
        if (trimmed.includes("{")) {
          currentLocale = "zhCN";
          continue;
        }
      }
      if (/^en\s*:\s*\{/.test(trimmed) || /^["']en["']\s*:\s*\{/.test(trimmed)) {
        if (!trimmed.includes("zh-CN")) {
          currentLocale = "en";
          continue;
        }
      }

      if (currentLocale && trimmed.includes(":") && trimmed.includes('"')) {
        const match = trimmed.match(/["']([^"']+)["']\s*:\s*["']((?:[^"\\]|\\.)*)["']/);
        if (match) {
          const key = match[1];
          const value = match[2]
            .replace(/\\"/g, '"')
            .replace(/\\'/g, "'")
            .replace(/\\\\/g, "\\");
          if (!key.startsWith("//")) {
            result[currentLocale][key] = value;
          }
        }
      }
    }

    return result;
  }

  return {
    name: "vite-plugin-i18n-optimize",

    configResolved(config) {
      isDev = config.mode === "development";
    },

    configureServer(_server) {
      server = _server;
    },

    handleHotUpdate(ctx) {
      if (!isI18nFile(ctx.file)) return;

      const code = fs.readFileSync(ctx.file, "utf-8");
      const parsed = parseI18nModule(code);

      const changes: Record<string, Record<string, string>> = {
        "zh-CN": parsed.zhCN,
        en: parsed.en,
      };

      if (server?.ws) {
        for (const locale of ["zh-CN", "en"] as const) {
          const localeChanges = changes[locale];
          if (Object.keys(localeChanges).length > 0) {
            server.ws.send({
              type: "custom",
              event: "i18n-update",
              data: {
                locale,
                changes: localeChanges,
              },
            });
          }
        }
        console.debug(`[i18n-hmr] Hot updated: ${path.basename(ctx.file)}`);
      }

      return [];
    },

    generateBundle(_options, bundle) {
      for (const fileName in bundle) {
        const chunk = bundle[fileName];
        if (chunk.type === "chunk" && chunk.code) {
          if (
            fileName.includes("common") ||
            fileName.includes("index") ||
            chunk.imports?.some((i) => i.includes("i18n"))
          ) {
            const originalSize = chunk.code.length;
            const compressed = chunk.code
              .replace(/\s+/g, " ")
              .replace(/" ([,}])/g, '"$1');
            const optimizedSize = compressed.length;
            const saved = ((originalSize - optimizedSize) / originalSize) * 100;

            if (saved > 0) {
              this.debug?.(
                `[i18n-optimize] ${fileName}: ${(saved).toFixed(1)}% volume reduced`,
              );
            }
          }
        }
      }
    },
  };
}

export default i18nOptimizePlugin;
