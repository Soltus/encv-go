/**
 * useFilesView.searchTokens.ts
 *
 * 从 useFilesView.ts 拆出的纯函数工具（无副作用）。
 * 提供：
 *   - tokenizeQuery:  query 字符串 → 词法 token（UI 语法高亮用）
 *   - renderSnippet:  命中片段（`<<...>>` 包裹）→ 高亮 parts
 *   - operatorSymbols:  op → 视觉符号映射（AND→＆ / OR→｜ / NOT→￢）
 *
 * 单元测试：见同目录 __tests__/useFilesView.searchTokens.test.ts
 *
 * 与 Go 端 internal/fts/query.go ParseQuery 的对应关系（仅 UI 层面，不影响后端）：
 *   - word:  普通单词（FTS5 unicode61 + CJK bigram）
 *   - phrase: "exact phrase"（FTS5 双引号短语）
 *   - regex:  regex:^pat  或  regex:/^pat/（Go 端二次过滤）
 *   - op:     AND/OR/NOT（必须大写，Go 端大小写敏感）
 *
 * 重要 (2026-07-02 用户反馈)：
 *   - 用户输入的 AND/OR/NOT 实际上**全部当普通文本**搜索
 *   - 视觉层用符号 ＆/｜/￢ 替代英文（用户明确要求）
 *   - 后端 FTS5 是否真按 operator 解析不在前端关心范围（前端只管显示）
 *   - 符号映射表见 operatorSymbols（测试覆盖）
 *
 * 2026-07-02 拆分自 useFilesView.ts
 */

export interface QueryToken {
  kind: "op" | "phrase" | "regex" | "word";
  text: string;
  /**
   * 视觉展示符号（仅 op 类型有值）。
   *   AND → '＆'（全角 AND 符号 U+FF06）
   *   OR  → '｜'（全角竖线 U+FF5C）
   *   NOT → '￢'（全角否定符号 U+FFE2）
   * 其他 kind 留空（用 text 自身展示）。
   */
  display?: string;
}

/**
 * op 文本 → 视觉符号映射。
 *
 * 设计原则：所有符号都是全角（与 CJK 等宽），UI 渲染时不会挤压。
 * 与原始英文保持视觉对比（warning/primary/danger 底色）。
 */
export const operatorSymbols: Record<string, string> = {
  AND: "＆", // ＆
  OR: "｜", // ｜
  NOT: "￢", // ￢
};

/**
 * 把 query 切成 token，给 UI 做语法高亮。
 *
 * 规则：
 *   - 空白分隔（连续空格算一个分隔符）
 *   - "..." → phrase（包含双引号内的全部字符）
 *   - regex:PATTERN 或 regex:/PATTERN/ → regex
 *   - AND|OR|NOT（必须大写，且独立成词）→ op
 *   - 其他 → word
 *
 * 注意：与 Go 端 ParseQuery 不完全一致：
 *   - Go 端对 `""` (空 phrase) 返回 error
 *   - Go 端对孤立 NOT 走 substring 过滤（不入 FTS5）
 *   - 前端为了高亮容错度高，孤立 NOT 也接受为 op token
 */
export function tokenizeQuery(query: string): QueryToken[] {
  const tokens: QueryToken[] = [];
  if (!query) return tokens;
  let i = 0;
  while (i < query.length) {
    // skip whitespace
    while (i < query.length && query[i] === " ") i++;
    if (i >= query.length) break;
    // phrase: "..."
    if (query[i] === '"') {
      const end = query.indexOf('"', i + 1);
      if (end > i) {
        const text = query.slice(i + 1, end);
        // 跳过空 phrase（用户误输入 ""），不产生无用 token
        if (text) {
          tokens.push({ kind: "phrase", text });
        }
        i = end + 1;
        continue;
      }
    }
    // regex: PATTERN  或  regex:/PATTERN/
    if (query.startsWith("regex:", i)) {
      const rest = query.slice(i + 6);
      if (rest.startsWith("/")) {
        const end = rest.indexOf("/", 1);
        if (end > 0) {
          tokens.push({ kind: "regex", text: rest.slice(1, end) });
          i = i + 6 + end + 1;
          continue;
        }
      } else {
        // 读到下一个空格
        const end = rest.indexOf(" ");
        const pattern = end > 0 ? rest.slice(0, end) : rest;
        if (pattern) {
          tokens.push({ kind: "regex", text: pattern });
          i = i + 6 + pattern.length;
          continue;
        }
      }
    }
    // op: 大写 AND/OR/NOT
    const restOfWord = query.slice(i);
    const opMatch = /^(AND|OR|NOT)(\s|$)/.exec(restOfWord);
    if (opMatch) {
      tokens.push({ kind: "op", text: opMatch[1], display: operatorSymbols[opMatch[1]] });
      i += opMatch[1].length;
      continue;
    }
    // word: 读到下一个空格
    const spaceIdx = query.indexOf(" ", i);
    const word = spaceIdx > 0 ? query.slice(i, spaceIdx) : query.slice(i);
    if (word) tokens.push({ kind: "word", text: word });
    i = spaceIdx > 0 ? spaceIdx : query.length;
  }
  return tokens;
}

export interface SnippetPart {
  text: string;
  highlight: boolean;
}

/**
 * 把 FTS5 snippet 的 `<<...>>` 高亮标记切分为高亮 parts。
 *
 * 输入示例：`...hello <<world>> foo...`
 * 输出示例：`[{ text: '...hello ', highlight: false }, { text: 'world', highlight: true }, { text: ' foo...', highlight: false }]`
 *
 * 边界：
 *   - undefined / '' → []
 *   - 只有 `<<` 没有 `>>` → 整段视为高亮（容错）
 *   - 只有 `>>` 没有 `<<` → `>>` 之后正常显示（容错）
 *   - 多段高亮交替正确切换
 */
export function renderSnippet(snippet: string | undefined): SnippetPart[] {
  if (!snippet) return [];
  const parts: SnippetPart[] = [];
  // split by '<<' / '>>' but keep delimiter
  const segments = snippet.split(/(<<|>>)/);
  let inHighlight = false;
  let cur = "";
  for (const seg of segments) {
    if (seg === "<<") {
      if (cur) {
        parts.push({ text: cur, highlight: inHighlight });
        cur = "";
      }
      inHighlight = true;
    } else if (seg === ">>") {
      if (cur) {
        // 只有在 inHighlight 状态时，>> 才关闭高亮
        // 否则视为普通文本（容错：孤立 >> 不应改变高亮状态）
        parts.push({ text: cur, highlight: inHighlight });
        cur = "";
      }
      inHighlight = false;
    } else if (seg) {
      cur += seg;
    }
  }
  if (cur) {
    parts.push({ text: cur, highlight: inHighlight });
  }
  return parts;
}
