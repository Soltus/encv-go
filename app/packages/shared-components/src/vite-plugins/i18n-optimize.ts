import fs from "node:fs";
import path from "node:path";
import type { Plugin } from "vite";

const I18N_FILE_REGEX = /[\\/]i18n[\\/].+\.ts$/;

export function i18nOptimizePlugin(options?: { i18nDirs?: string[] }): Plugin {
  const i18nDirs = options?.i18nDirs ?? [];
  let isDev = false;
  let isBuild = false;
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
          const value = match[2].replace(/\\"/g, '"').replace(/\\'/g, "'").replace(/\\\\/g, "\\");
          if (!key.startsWith("//")) {
            result[currentLocale][key] = value;
          }
        }
      }
    }

    return result;
  }

  function compactI18nStrings(code: string): string {
    let result = code;

    result = result.replace(/"([a-zA-Z0-9_.-]{3,})"\s*:\s*"((?:[^"\\]|\\.){2,})"(?=\s*[,}\]])/g, '"$1":"$2"');

    return result;
  }

  return {
    name: "vite-plugin-i18n-optimize",

    configResolved(config) {
      isDev = config.mode === "development";
      isBuild = config.command === "build";
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
      if (!isBuild) return;

      let i18nOriginalSize = 0;
      let i18nOptimizedSize = 0;
      let i18nChunkCount = 0;

      for (const fileName in bundle) {
        const chunk = bundle[fileName];
        if (chunk.type !== "chunk" || !chunk.code) continue;

        const moduleIds = (chunk as any).moduleIds as string[] | undefined;
        const hasI18nModule =
          fileName.includes("i18n") || (moduleIds && moduleIds.some(mid => mid.includes("/i18n/") || mid.includes("useI18n")));

        if (!hasI18nModule) continue;

        i18nChunkCount++;
        const originalSize = chunk.code.length;
        i18nOriginalSize += originalSize;

        let optimized = chunk.code;
        optimized = compactI18nStrings(optimized);

        if (optimized.length !== originalSize) {
          (chunk as any).code = optimized;
        }

        i18nOptimizedSize += optimized.length;
      }

      if (i18nChunkCount > 0 && i18nOriginalSize > 0) {
        const saved = ((i18nOriginalSize - i18nOptimizedSize) / i18nOriginalSize) * 100;
        console.info(
          `\n  [i18n-optimize] ${i18nChunkCount} i18n chunk(s): ` +
            `${(i18nOriginalSize / 1024).toFixed(1)} KB → ${(i18nOptimizedSize / 1024).toFixed(1)} KB ` +
            `(${(saved).toFixed(1)}% reduced)\n`
        );
      }
    },
  };
}

export default i18nOptimizePlugin;
