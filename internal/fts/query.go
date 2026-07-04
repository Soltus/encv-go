// Package fts - query.go: bool/phrase/regex query parser → FTS5 MATCH expression.
//
// 语法 (2026-07-02 大改升级)：
//   - 默认空格分隔，每个 token 隐式 AND
//   - 大写 AND / OR / NOT 作为布尔操作符
//   - "exact phrase" 双引号包裹表示精确短语
//   - regex:/.../  正则表达式（FTS5 命中后再用 Go 二次过滤）
//   - \  转义下一个字符（包括 " \ 本身）
//
// 示例：
//   - "在线 高清"        → 在线 AND 高清
//   - 在线 AND 高清      → 在线 AND 高清
//   - "在线" OR "播放"    → phrase("在线") OR phrase("播放")
//   - 在线 NOT 视频      → 在线 AND NOT 视频
//   - regex:^photo.*     → FTS5 任意词 + name 匹配 ^photo.*
//
// 用户输入的空格视为 AND（即空格 = 真空格文本匹配）。

package fts

import (
	"fmt"
	"regexp"
	"strings"
)

// allRegexMarker 当查询全是 regex 时插入的占位 token。
// Search 通过检测此标记决定走"全表扫 + regex 过滤"路径。
const allRegexMarker = "__FTZ5_REGEX_ONLY__"

// isOnlyRegexQuery 判断是否是纯 regex 查询（matchExpr 只含占位 token）。
func isOnlyRegexQuery(matchExpr string, regexes []*regexp.Regexp) bool {
	return matchExpr == allRegexMarker && len(regexes) > 0
}

// Query 中间表示：parse 后转为 FTS5 MATCH 字符串 + 可选 regex 二次过滤。
type Query struct {
	MatchExpr    string         // FTS5 MATCH 表达式（已转义）
	RegexFilters []*regexp.Regexp // 二次过滤的 regex（AND 语义）
	// NotTerms NOT 子句的词项。
	// FTS5 的 NOT 必须是二元操作符（"expr NOT expr"，且 NOT 紧贴前一个 expr，不能有 AND/OR），
	// 所以独立 "NOT 视频" / "A AND NOT 视频" 都报 syntax error。
	// 解决方案：在 Go 端做 NOT 过滤（snippet substring 检查 + 大小写不敏感）。
	// NotTerms 用大写比对待 snippet，命中任一就排除。
	NotTerms []string
}

// ParseQuery 把用户查询字符串解析为 FTS5 MATCH 表达式。
//
// 返回：
//   - matchExpr: FTS5 MATCH 字符串
//   - regexFilters: 正则二次过滤（在 FTS5 命中后做）
//   - notTerms: NOT 子句的词项（在 FTS5 命中后做 substring 排除）
//   - err: 解析错误（未闭合引号 / 无效 regex / 空 query）
func ParseQuery(input string) (string, []*regexp.Regexp, []string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil, nil, fmt.Errorf("empty query")
	}

	tokens, err := tokenize(input)
	if err != nil {
		return "", nil, nil, err
	}

	// CJK bigram 预处理：每个 word 切成 bigram
	// 例：在线 → 在线  (2-char 无变化)
	//     在线播放 → 在线 线播 播放  (3 bigrams)
	for i := range tokens {
		if tokens[i].kind == tokWord || tokens[i].kind == tokPhrase {
			tokens[i].value = cjkBigram(tokens[i].value)
		}
	}

	// 把 tokens 转为 FTS5 MATCH 表达式 + 收集 regex + 收集 NOT
	//
	// 关键设计：
	//   - word 和 word 之间默认加 AND
	//   - explicit op (AND/OR/NOT) 直接用
	//   - phrase 直接当 token
	//   - regex 加入 regex 列表 + 占位 token
	//   - NOT 提取到 notTerms（不在 FTS5 表达式里），由 Search 在 Go 端做 substring 排除
	//
	// 状态机：prevIsOp 表示上一个有效 token 是 op
	var matchParts []string
	var regexes []*regexp.Regexp
	var notTerms []string
	prevIsOp := true // 开头视为 op 后状态（避免前置 AND）

	for i, tok := range tokens {
		switch tok.kind {
		case tokAnd:
			if i == 0 || prevIsOp {
				continue // 跳过孤立的 AND
			}
			// 在前一个 word 后插入 AND
			if n := len(matchParts); n > 0 {
				matchParts[n-1] = matchParts[n-1] + " AND"
			}
			prevIsOp = true
		case tokOr:
			if i == 0 || prevIsOp {
				// 开头 OR → 插入孤立 OR
				matchParts = append(matchParts, "OR")
			} else {
				// 替换前一个隐式 AND 为 OR
				if n := len(matchParts); n > 0 {
					matchParts[n-1] = matchParts[n-1] + " OR"
				}
			}
			prevIsOp = true
		case tokNot:
			// FTS5 NOT 语法限制：必须是二元且紧贴，不能用 AND/OR 隔开。
			// 所以把 NOT 提到 notTerms，由 Go 端 substring 排除。
			_ = i
			_ = prevIsOp
			// NOT 标志 - 下一个 word/phrase 是 notTerm
			// 用 prevIsOp=true 让下一轮 word 不被 AND 连接
			prevIsOp = true
		case tokPhrase:
			// 检查上一个 token 是不是 NOT（NOT 后跟 phrase 视为 notTerm）
			if len(tokens) > 0 && i > 0 && tokens[i-1].kind == tokNot {
				notTerms = append(notTerms, strings.ToLower(tok.value))
				prevIsOp = false
				continue
			}
			// phrase 已经是 phrase（来自用户 "..."），直接包裹双引号 + 内部 " 转义
			matchParts = append(matchParts, fmt.Sprintf(`"%s"`, strings.ReplaceAll(tok.value, `"`, `""`)))
			prevIsOp = false
		case tokWord:
			// 检查上一个 token 是不是 NOT（NOT 后跟 word 视为 notTerm）
			if len(tokens) > 0 && i > 0 && tokens[i-1].kind == tokNot {
				notTerms = append(notTerms, strings.ToLower(tok.value))
				prevIsOp = false
				continue
			}
			matchParts = append(matchParts, escapeFTS5(tok.value))
			prevIsOp = false
		case tokRegex:
			re, err := regexp.Compile(tok.value)
			if err != nil {
				return "", nil, nil, fmt.Errorf("invalid regex %q: %w", tok.value, err)
			}
			if tok.value == "" {
				return "", nil, nil, fmt.Errorf("empty regex pattern")
			}
			regexes = append(regexes, re)
			// regex 标记占位（用特殊字符串让 Search 知道这是 regex-only 模式）
			matchParts = append(matchParts, allRegexMarker)
			prevIsOp = false
		case tokParenOpen, tokParenClose:
			matchParts = append(matchParts, tok.value)
			prevIsOp = tok.kind == tokParenOpen
		}
	}

	matchExpr := strings.Join(matchParts, " ")
	matchExpr = cleanLeadingOps(matchExpr)

	return matchExpr, regexes, notTerms, nil
}

// cjkBigram 把 CJK 字符串切成 bigram，用空格分隔。
//
// 例：在线播放 → 在线 线播 播放
//     photo.jpg → photo.jpg  (无 CJK，原样返回)
//
// 目的：FTS5 默认 tokenize 把 "在线播放" 当作一个 token，搜 "在线" 不命中。
// 切成 bigram 后，搜 "在线" 也能命中 "在线播放"。
//
// 实现：rune-level 扫描，识别连续 CJK 段切 bigrams，保留非 CJK 原样。
func cjkBigram(s string) string {
	if !containsCJK(s) {
		return s
	}
	runes := []rune(s)
	var result strings.Builder
	i := 0
	for i < len(runes) {
		if !isCJK(runes[i]) {
			// 非 CJK 段：原样输出到下一个 CJK 或末尾
			j := i
			for j < len(runes) && !isCJK(runes[j]) {
				j++
			}
			result.WriteString(string(runes[i:j]))
			i = j
		} else {
			// CJK 段：切成 bigrams
			j := i
			for j < len(runes) && isCJK(runes[j]) {
				j++
			}
			cjkPart := runes[i:j]
			if len(cjkPart) >= 2 {
				for k := 0; k < len(cjkPart)-1; k++ {
					if k > 0 {
						result.WriteByte(' ')
					}
					result.WriteString(string(cjkPart[k : k+2]))
				}
			} else {
				result.WriteString(string(cjkPart))
			}
			i = j
		}
	}
	return result.String()
}

func containsCJK(s string) bool {
	for _, r := range s {
		if isCJK(r) {
			return true
		}
	}
	return false
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||   // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) ||   // CJK Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0xF900 && r <= 0xFAFF) ||   // CJK Compatibility Ideographs
		(r >= 0x2F800 && r <= 0x2FA1F)    // CJK Compatibility Supplement
}

// cleanLeadingOps 移除 MATCH 表达式开头的孤立 AND/OR/NOT。
//
// 关键：NOT 必须保留（"NOT 视频" → "NOT 视频"，不剥）。
// 只有孤立的 AND/OR 在开头才剥（"AND foo" → "foo"，因为 AND 是默认连接符）。
func cleanLeadingOps(expr string) string {
	expr = strings.TrimSpace(expr)
	for {
		trimmed := false
		// 只剥 AND / OR，保留 NOT
		for _, op := range []string{"AND ", "OR "} {
			if strings.HasPrefix(expr, op) {
				expr = strings.TrimSpace(expr[len(op):])
				trimmed = true
			}
		}
		if !trimmed {
			break
		}
	}
	return expr
}

// escapeFTS5 转义 FTS5 特殊字符。
//
// FTS5 关键字：AND OR NOT NEAR
// 包含空格/引号的词需要双引号包裹。
func escapeFTS5(s string) string {
	upper := strings.ToUpper(s)
	if upper == "AND" || upper == "OR" || upper == "NOT" || upper == "NEAR" {
		// 把关键字当普通词搜
		return fmt.Sprintf(`"%s"`, s)
	}
	if strings.ContainsAny(s, ` \-"()*:^`) {
		return fmt.Sprintf(`"%s"`, strings.ReplaceAll(s, `"`, `""`))
	}
	return s
}

// matchAnyRegex 检查 name + snippet 是否匹配任一 regex（AND 语义）。
func matchAnyRegex(name, snippet string, regexes []*regexp.Regexp) bool {
	if len(regexes) == 0 {
		return true
	}
	for _, re := range regexes {
		if !re.MatchString(name) && !re.MatchString(snippet) {
			return false
		}
	}
	return true
}

// ─── token 分类 + tokenizer ─────────────────────────────

type tokKind int

const (
	tokWord tokKind = iota
	tokAnd
	tokOr
	tokNot
	tokPhrase
	tokRegex
	tokParenOpen
	tokParenClose
)

type token struct {
	kind  tokKind
	value string
}

// tokenize 把用户输入切成 tokens。
//
// 处理规则：
//   - 双引号包裹 → phrase
//   - 裸的大写 AND / OR / NOT → 操作符
//   - regex:/.../  → 正则（捕获组里支持嵌套 \/ 通过转义）
//   - 反斜杠 \  → 转义下一个字符
//   - 空格 → 分词边界（不是 AND 操作符本身，但 parser 会处理为隐式 AND）
func tokenize(input string) ([]token, error) {
	var tokens []token
	var buf strings.Builder
	i := 0
	flushWord := func() {
		if buf.Len() > 0 {
			word := buf.String()
			buf.Reset()
			upper := strings.ToUpper(word)
			switch upper {
			case "AND":
				tokens = append(tokens, token{kind: tokAnd, value: word})
			case "OR":
				tokens = append(tokens, token{kind: tokOr, value: word})
			case "NOT":
				tokens = append(tokens, token{kind: tokNot, value: word})
			default:
				tokens = append(tokens, token{kind: tokWord, value: word})
			}
		}
	}

	for i < len(input) {
		c := input[i]
		switch c {
		case ' ', '\t':
			flushWord()
			i++
		case '"':
			// phrase
			flushWord()
			i++ // skip opening "
			var phrase strings.Builder
			for i < len(input) && input[i] != '"' {
				if input[i] == '\\' && i+1 < len(input) {
					phrase.WriteByte(input[i+1])
					i += 2
				} else {
					phrase.WriteByte(input[i])
					i++
				}
			}
			if i >= len(input) {
				return nil, fmt.Errorf("unclosed quote in phrase")
			}
			i++ // skip closing "
			tokens = append(tokens, token{kind: tokPhrase, value: phrase.String()})
		case '\\':
			// 转义下一个字符
			if i+1 < len(input) {
				buf.WriteByte(input[i+1])
				i += 2
			} else {
				i++
			}
		case '(':
			flushWord()
			tokens = append(tokens, token{kind: tokParenOpen, value: "("})
			i++
		case ')':
			flushWord()
			tokens = append(tokens, token{kind: tokParenClose, value: ")"})
			i++
		default:
			// 检测 regex: 两种语法都支持：
			//   - regex:^sun       （裸模式，无定界符）
			//   - regex:/^sun/     （带 / 定界符，body 内支持 / via \/）
			if c == 'r' && i+6 <= len(input) && input[i:i+6] == "regex:" {
				flushWord()
				i += 6 // skip "regex:"
				var pattern strings.Builder
				if i < len(input) && input[i] == '/' {
					// 带 / 定界符模式
					i++ // skip opening /
					for i < len(input) && input[i] != '/' {
						if input[i] == '\\' && i+1 < len(input) {
							pattern.WriteByte(input[i+1])
							i += 2
						} else {
							pattern.WriteByte(input[i])
							i++
						}
					}
					if i >= len(input) {
						return nil, fmt.Errorf("unclosed regex /")
					}
					i++ // skip closing /
				} else {
					// 裸模式：读直到下一个空格或定界符
					for i < len(input) && input[i] != ' ' && input[i] != '\t' {
						if input[i] == '\\' && i+1 < len(input) {
							pattern.WriteByte(input[i+1])
							i += 2
						} else {
							pattern.WriteByte(input[i])
							i++
						}
					}
				}
				tokens = append(tokens, token{kind: tokRegex, value: pattern.String()})
				continue
			}
			buf.WriteByte(c)
			i++
		}
	}
	flushWord()

	return tokens, nil
}
