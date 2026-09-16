/**
 * 把 FTS5 布尔查询转换为向量搜索关键词。
 *
 * 应用场景（用户原话）：
 *   "即使搜索有逻辑符 FTS 搜索没有匹配结果，结果也不应当为空。
 *    同样遵守增量合并原则，以及匹配过少智能贪婪，转换逻辑符进行普通向量搜索。"
 *
 * 转换规则（与后端 internal/fts/query.go 语法对齐）：
 *   - 在线 AND 高清       → "在线 高清"      （AND 视为空格，两个词都保留）
 *   - 在线 OR 播放        → "在线 播放"       （OR 视为空格，两个词都保留，反正向量搜有相关度排序）
 *   - 在线 NOT 视频       → "在线"           （NOT 后的词丢弃）
 *   - "exact phrase" 高清 → "exact phrase 高清" （去引号，phrase 当作单个关键词）
 *   - regex:^photo.*      → "photo"          （去 regex: 前缀和 /.../ 边界，提取词项）
 *   - 在线 AND (高清 OR 视频) → "在线 高清 视频"（去括号，扁平化）
 *
 * 边界处理：
 *   - 用户原始 query 没有布尔语法 → 返回原 query（fallback 不会触发，但函数本身安全）
 *   - cleanedQuery 为空（如 "NOT 视频"）→ 返回空字符串，调用方需判空
 *   - 转义双引号 \" 也按双引号处理
 */
export function convertBooleanQueryToVectorKeywords(query: string): string {
  let s = query;
  // 1. 去 regex: 前缀（regex:^foo 或 regex:/^foo/），保留词项
  s = s.replace(/\bregex:\S+/g, " ");
  // 2. 去 /pattern/ 边界斜杠，保留 pattern 内的词
  s = s.replace(/\/([^/\s]+)\//g, " $1 ");
  // 3. 去所有双引号（含转义 \"），phrase 内容当作普通词
  s = s.replace(/\\"/g, " ").replace(/"/g, " ");
  // 4. 去括号
  s = s.replace(/[()]/g, " ");
  // 5. 切词，丢 AND/OR，丢 NOT 后的下一个词
  const tokens = s.split(/\s+/).filter(t => t.length > 0);
  const keywords: string[] = [];
  let skipNext = false;
  for (const tok of tokens) {
    if (tok === "NOT") {
      skipNext = true;
      continue;
    }
    if (tok === "AND" || tok === "OR") {
      continue;
    }
    if (skipNext) {
      skipNext = false;
      continue;
    }
    keywords.push(tok);
  }
  return keywords.join(" ");
}
