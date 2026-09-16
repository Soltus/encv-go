import { type Dirent, existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import type { Plugin } from "vite";

export interface FileSizeLimitOptions {
  /** 单文件行数上限，超过则报错（默认 2000） */
  maxLines?: number;
  /** 受检文件扩展名（默认 [".vue", ".ts"]） */
  extensions?: string[];
  /** 排除路径的正则（默认排除 node_modules/dist/.git/coverage 及 .d.ts） */
  exclude?: RegExp[];
  /** 构建时是否直接报错阻断（默认 true） */
  failOnError?: boolean;
  /**
   * 扫描整个 pnpm 工作区（自动向上查找 pnpm-workspace.yaml 作为根），
   * 而非仅当前 config.root。用于「覆盖 pnpm 工作区」的统一门禁。
   * 默认 false（只扫当前包，避免跨包构建互相阻断）。
   */
  scanWorkspace?: boolean;
  /** scanWorkspace 时的显式工作区根目录 */
  workspaceRoot?: string;
}

const DEFAULT_EXCLUDE: RegExp[] = [/[\\/]node_modules[\\/]/, /[\\/]dist[\\/]/, /[\\/]\.git[\\/]/, /[\\/]coverage[\\/]/, /\.d\.ts$/];

function findWorkspaceRoot(start: string): string {
  let dir = start;
  for (;;) {
    if (existsSync(join(dir, "pnpm-workspace.yaml"))) return dir;
    const parent = resolve(dir, "..");
    if (parent === dir) return start;
    dir = parent;
  }
}

function collectFiles(dir: string, exts: Set<string>, exclude: RegExp[], out: string[]): void {
  let entries: Dirent[];
  try {
    entries = readdirSync(dir, { withFileTypes: true });
  } catch {
    return;
  }
  for (const e of entries) {
    const full = join(dir, e.name);
    if (exclude.some(re => re.test(full))) continue;
    if (e.isDirectory()) {
      collectFiles(full, exts, exclude, out);
    } else if (e.isFile()) {
      const dot = e.name.lastIndexOf(".");
      const ext = dot >= 0 ? e.name.slice(dot) : "";
      if (exts.has(ext)) out.push(full);
    }
  }
}

/**
 * 自定义 Vite 插件：文件行数门禁
 *
 * 覆盖 pnpm 工作区 —— 插件放在 @encv/shared-components 中，各包 vite 配置统一引入，
 * 保证规则一致。默认只扫描「当前包」（config.root），设置 scanWorkspace:true 可升级为
 * 扫描整个工作区，强制所有包都不得出现超过 maxLines 行的 .vue/.ts 文件。
 *
 * 超过上限直接报错（build 阶段 this.error 阻断构建），dev 阶段仅警告。
 */
export function fileSizeLimitPlugin(options: FileSizeLimitOptions = {}): Plugin {
  const {
    maxLines = 2000,
    extensions = [".vue", ".ts"],
    exclude = DEFAULT_EXCLUDE,
    failOnError = true,
    scanWorkspace = false,
    workspaceRoot,
  } = options;

  let root = "";
  let command: "build" | "serve" = "build";
  const violations: { rel: string; lines: number }[] = [];

  return {
    name: "file-size-limit",
    enforce: "pre",

    configResolved(config) {
      root = config.root;
      command = config.command;
    },

    buildStart() {
      violations.length = 0;
      const base = scanWorkspace ? (workspaceRoot ? resolve(root, workspaceRoot) : findWorkspaceRoot(root)) : root;

      const exts = new Set(extensions.map(e => (e.startsWith(".") ? e : "." + e)));
      const files: string[] = [];
      collectFiles(base, exts, exclude, files);

      for (const f of files) {
        let lines = 0;
        try {
          lines = readFileSync(f, "utf-8").split("\n").length;
        } catch {
          continue;
        }
        if (lines > maxLines) {
          violations.push({ rel: relative(base, f), lines });
        }
      }

      if (violations.length === 0) {
        console.log(
          `\n[file-size-limit] ✅ 未发现超过 ${maxLines} 行的文件（扫描 ${files.length} 个 .vue/.ts，范围：${scanWorkspace ? "工作区" : "当前包"}）\n`
        );
        return;
      }

      violations.sort((a, b) => b.lines - a.lines);
      const list = violations.map(v => `  🚫 ${v.rel}  (${v.lines} 行，超过 ${maxLines})`).join("\n");
      console.error(`\n[file-size-limit] 发现 ${violations.length} 个超过 ${maxLines} 行的文件，必须拆分：\n${list}\n`);

      if (command === "build" && failOnError) {
        this.error(
          `[file-size-limit] ${violations.length} 个 .vue/.ts 文件超过 ${maxLines} 行上限，请拆分后再构建（如需全工作区门禁，置 scanWorkspace:true）`
        );
      } else {
        console.warn(`[file-size-limit] ⚠️ ${command === "serve" ? "dev 模式" : "failOnError=false"} 仅警告，不阻断构建\n`);
      }
    },
  };
}
