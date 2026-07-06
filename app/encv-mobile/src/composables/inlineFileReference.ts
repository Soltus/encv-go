// ──────────────────────────────────────────────────────────────
// inlineFileReference
// 解析 AssistantMessage 文本中的 path:line:col 模式。
// 参考 codex-web MessageBlocks.tsx:71-81 INLINE_FILE_REFERENCE_PATTERN。
// ──────────────────────────────────────────────────────────────

export const FILE_REFERENCE_EXTENSIONS =
  "tsx?|jsx?|mjs|cjs|css|scss|sass|less|mdx?|jsonc?|ya?ml|toml|lock|html?|xml|svg|png|jpe?g|gif|webp|bmp|ico|pdf|docx?|xlsx?|xlsm|pptx?|txt|csv|tsv|log|py|ps1|sh|bat|cmd|rs|go|java|cs|cpp|c|h|hpp|sql|env|ini";

export const FILE_REFERENCE_LOCATION_SUFFIX = "(?::\\d+(?::\\d+)?)?";

export interface FileReference {
  start: number;
  end: number;
  path: string;
  line?: number;
  col?: number;
}

const URL_PREFIX_PATTERN = /(?:https?|file):\/\/[^\s]*$/i;

/**
 * 单条 file ref 匹配模式（global + case-insensitive, per spec 约束）。
 * 拼接 3 个备选：
 *   1) Windows 绝对路径:   C:\foo\bar.ts:42
 *   2) 相对/嵌套路径:      ./src/main.go:10  /  src/utils/index.ts
 *   3) 纯文件名:           package.json
 *
 * 注意：path 中间段（[\\w.-]+）不允许空格——避免"fix src/main.go"被误识别为
 * "fix src/main.go"整体。文件名的最后一段允许空格（[\\w .-]+）。
 */
const INLINE_FILE_REFERENCE_PATTERN = new RegExp(
  [
    // 1) Windows 绝对路径
    `[a-z]:[\\\\/][^\\r\\n"'<>|]+?\\.(?:${FILE_REFERENCE_EXTENSIONS})\\b${FILE_REFERENCE_LOCATION_SUFFIX}`,
    // 2) 相对/嵌套路径（中间段无空格）
    `(?:\\.{1,2}[\\\\/])?(?:[\\w.-]+[\\\\/])+[\\w .-]+\\.(?:${FILE_REFERENCE_EXTENSIONS})\\b${FILE_REFERENCE_LOCATION_SUFFIX}`,
    // 3) 纯文件名
    `\\b[\\w.-]+\\.(?:${FILE_REFERENCE_EXTENSIONS})\\b${FILE_REFERENCE_LOCATION_SUFFIX}`,
  ].join("|"),
  "gi"
);

function stripLocationSuffix(value: string): { path: string; line?: number; col?: number } {
  const match = /^(.*\.[a-z0-9]{1,12})(?::(\d+)(?::(\d+))?)$/i.exec(value);
  if (!match) return { path: value };
  return {
    path: match[1] ?? value,
    line: match[2] ? Number(match[2]) : undefined,
    col: match[3] ? Number(match[3]) : undefined,
  };
}

/**
 * Parse a message body and return all inline file references (path[:line[:col]]).
 * - global + case-insensitive（per spec 约束）
 * - 跳过 URL 尾部的路径段：例如 `https://example.com/file.ts:1` 不应被识别为本地引用
 * - 保留原始文本中的 start/end 索引，供上层切分 segments 使用
 */
export function parseFileReferences(text: string): FileReference[] {
  if (!text) return [];
  const out: FileReference[] = [];
  // 必须重置 lastIndex 状态（全局正则属性）
  INLINE_FILE_REFERENCE_PATTERN.lastIndex = 0;
  for (const m of text.matchAll(INLINE_FILE_REFERENCE_PATTERN)) {
    const raw = m[0];
    const index = m.index ?? 0;
    // URL 防护：
    // 1) match 自身包含 `://` → 显然是 URL 内部段，丢弃
    // 2) match 前 24 字符以 URL scheme 结尾 → 是 URL 路径段，丢弃
    if (raw.includes("://")) continue;
    if (index > 0 && URL_PREFIX_PATTERN.test(text.slice(Math.max(0, index - 24), index))) {
      continue;
    }
    const { path, line, col } = stripLocationSuffix(raw);
    out.push({
      start: index,
      end: index + raw.length,
      path,
      ...(line !== undefined ? { line } : {}),
      ...(col !== undefined ? { col } : {}),
    });
  }
  return out;
}
