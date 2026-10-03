package fts

// query_escape_test.go —— 查询转义的契约锁
//
// 2026-10-04 真机 bug：escapeFTS5 用**黑名单**只转义 ` \-"()*:^` 九个字符，
// 用户输入含 `/`（如路径 `/../../etc/passwd`）时会漏网 ⇒ 拼进 MATCH 表达式触发
// `fts5: syntax error near "/"` ⇒ 上层包装成 HTTP **502 对端故障**，还会被计入熔断。
//
// 黑名单永远列举不全，所以改成**白名单**：不安全的字符一律整体加双引号。
// 这组用例同时锁住"不再报错"与"常见文件名字符（.-_ 和 CJK）行为不变"。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseQuery_SpecialCharsAreQuoted(t *testing.T) {
	cases := []string{
		"/../../etc/passwd", // 触发过真机报错的那一个
		"a/b",
		"foo+bar",
		"a{b}c",
		"x[y]z",
		"a:b",
		"a^b",
	}
	for _, q := range cases {
		expr, _, _, err := ParseQuery(q)
		if err != nil {
			t.Fatalf("ParseQuery(%q) 不应报错: %v", q, err)
		}
		if !strings.HasPrefix(expr, `"`) || !strings.HasSuffix(expr, `"`) {
			t.Fatalf("含特殊字符的查询 %q 必须整体加双引号, got %q", q, expr)
		}
	}
}

// TestParseQuery_CommonFilenameCharsUnchanged 反向锁：
// 不能因为修 bug 就把 `-._` 和 CJK 也全部加引号 —— 那会改变现有搜索语义（phrase 更严格）。
func TestParseQuery_CommonFilenameCharsUnchanged(t *testing.T) {
	// ⚠️ 不能用 "在线播放" 这类多字 CJK：它会先被 cjkBigram 切成 "在线 线播 播放"
	// （**带空格**），含空格必然整体加引号 —— 旧实现同样如此，不是本次引入的变化。
	// 这里用两字词（bigram 后仍是 "在线"，无空格）来验证 CJK 不被误伤。
	cases := []string{
		"report-2026.pdf",
		"my_file_v2",
		"在线",
	}
	for _, q := range cases {
		expr, _, _, err := ParseQuery(q)
		if err != nil {
			t.Fatalf("ParseQuery(%q) 报错: %v", q, err)
		}
		if strings.HasPrefix(expr, `"`) {
			t.Fatalf("常见文件名字符 %q 应保持裸词（行为不变）, got %q", q, expr)
		}
	}
}

// TestSearch_SlashQueryNoSyntaxError 端到端：真正打到 FTS5，
// 含 `/` 的查询必须**不报语法错误**（旧实现在这里会 fts5: syntax error near "/"）。
func TestSearch_SlashQueryNoSyntaxError(t *testing.T) {
	idx := newTestIndex(t)

	entries := []FileEntry{
		{Path: "/docs/a.txt", Name: "a.txt", Content: "hello world", Modified: time.Now().Format(time.RFC3339)},
		{Path: "/docs/b.txt", Name: "b.txt", Content: "another file", Modified: time.Now().Format(time.RFC3339)},
	}
	if err := idx.BulkInsert(context.Background(), entries); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	// 旧实现在这里返回 `fts5: syntax error near "/"`
	if _, err := idx.Search(context.Background(), "/../../etc/passwd", SearchOptions{Limit: 10}); err != nil {
		t.Fatalf("含 / 的查询不得报语法错误（应被转义为字面量）: %v", err)
	}
	if _, err := idx.Search(context.Background(), "a/b", SearchOptions{Limit: 10}); err != nil {
		t.Fatalf("含 / 的查询不得报语法错误: %v", err)
	}
}

// TestParseQuery_InvalidInputIsSentinel 无效输入必须能被上层识别成"输入问题"
// （否则上层只能一律当故障 ⇒ 计入熔断）。
func TestParseQuery_InvalidInputIsSentinel(t *testing.T) {
	if _, _, _, err := ParseQuery("   "); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("空查询应带 ErrInvalidQuery, got %v", err)
	}
	// 未闭合引号 ⇒ tokenize 应报错
	if _, _, _, err := ParseQuery(`"unclosed`); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("未闭合引号应带 ErrInvalidQuery, got %v", err)
	}
}
