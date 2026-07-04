// internal/tools/search_files.go
//
// 递归搜索工具 — 支持 glob / regex / 复合布尔查询。
//
// AST 节点设计：
//   - 11 种叶子节点：name_glob / name_regex / content_regex / size_gt / size_lt / size_eq
//     / mtime_after / mtime_before / ext_eq / path_contains / path_not_contains
//   - 3 种复合节点：and / or / not
//
// 性能约束：
//   - 50000 文件扫描上限（防止 mount 过大阻塞）
//   - content_regex 文件大小上限 10MB
//   - 短路求值（AND 第一个 false 后续不再算；OR 第一个 true 后续不再算）
//
// 参考：
//   - Spec: /workspace/.trae/specs/agent-tools-scenarios-v2/spec.md
//   - Requirement: search_files 工具（核心）
package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── AST 节点定义 ──────────────────────────────────────────────

// SearchExpr 是搜索表达式的根类型（JSON 反序列化目标）。
//
// 叶子节点示例：
//   {"type":"name_glob","value":"*.mp4"}
//   {"type":"name_regex","value":"studio_.*\\.mp4"}
//   {"type":"content_regex","value":"error.*timeout"}
//   {"type":"size_gt","value":104857600}
//   {"type":"mtime_after","value":"2026-01-01T00:00:00Z"}
//   {"type":"ext_eq","value":"mp4"}
//   {"type":"path_contains","value":"subtitles"}
//   {"type":"path_not_contains","value":"trash"}
//
// 复合节点示例：
//   {"type":"and","children":[<expr>, <expr>, ...]}
//   {"type":"or","children":[<expr>, <expr>, ...]}
//   {"type":"not","child":<expr>}
type SearchExpr struct {
	Type     string       `json:"type"`
	Value    any          `json:"value,omitempty"`    // 叶子节点
	Children []SearchExpr `json:"children,omitempty"` // and / or
	Child    *SearchExpr  `json:"child,omitempty"`    // not
}

// ─── 参数 / 结果结构 ───────────────────────────────────────────

// SearchFilesArgs 工具参数。
type SearchFilesArgs struct {
	MountID    string     `json:"mount_id"`
	RelPath    string     `json:"rel_path"`    // 默认 "/"
	Recursive  bool       `json:"recursive"`   // 默认 true
	MaxResults int        `json:"max_results"` // 默认 200
	Expression SearchExpr `json:"expression"`
}

// SearchMatch 单个命中条目。
type SearchMatch struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	MTime string `json:"mtime"`
	Ext  string `json:"ext"`
}

// SearchFilesResult 工具结果。
type SearchFilesResult struct {
	Total         int           `json:"total"`
	Truncated     bool          `json:"truncated"`
	ScannedLimited bool         `json:"scanned_limited,omitempty"`
	Matches       []SearchMatch `json:"matches"`
}

// ─── 性能上限 ───────────────────────────────────────────────────

const (
	// MaxFilesScanned 单次搜索最多扫描的文件数。
	// 超过立即返回当前 partial + scanned_limited=true。
	MaxFilesScanned = 50000
	// MaxContentRegexSize content_regex 跳过的文件大小阈值。
	// 超过 10MB 的文件不再做内容正则扫描（防 OOM / 慢）。
	MaxContentRegexSize = 10 * 1024 * 1024
	// DefaultMaxResults 默认 max_results。
	DefaultMaxResults = 200
)

// ─── 注册定义 ───────────────────────────────────────────────────

// SearchFilesDef 返回 search_files 的 ToolDef。
func SearchFilesDef() *ToolDef {
	return &ToolDef{
		Name:        "search_files",
		Description: "递归搜索文件，支持 glob/正则/逻辑运算符（AND/OR/NOT）。返回匹配的文件列表（含 size / mtime / ext）。",
		Kind:        KindFileRead,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["mount_id"],
			"properties":{
				"mount_id":{"type":"string","description":"挂载点 ID（list_mounts 返回）"},
				"rel_path":{"type":"string","default":"/","description":"搜索根路径（相对 mount 根）"},
				"recursive":{"type":"boolean","default":true},
				"max_results":{"type":"integer","default":200,"maximum":1000},
				"expression":{"type":"object","description":"复合 AST（叶子+and/or/not）"}
			}
		}`,
		Handler: searchFilesHandler,
	}
}

// ─── Handler 实现 ───────────────────────────────────────────────

func searchFilesHandler(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
	var args SearchFilesArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errResult(fmt.Sprintf("invalid args: %v", err)), nil
	}
	if args.MountID == "" {
		return errResult("mount_id is required"), nil
	}
	if deps == nil || deps.ResolveMount == nil {
		return errResult("deps.ResolveMount not initialized"), nil
	}
	rootAbs, ok := deps.ResolveMount(args.MountID)
	if !ok {
		return errResult(fmt.Sprintf("mount not found: %s", args.MountID)), nil
	}

	// 解析 rel_path
	rel := strings.TrimSpace(args.RelPath)
	if rel == "" || rel == "/" {
		rel = "."
	}
	searchRoot := filepath.Join(rootAbs, rel)
	if !strings.HasPrefix(searchRoot, rootAbs) {
		// 路径越权
		return errResult("rel_path escapes mount root"), nil
	}
	if _, err := os.Stat(searchRoot); err != nil {
		return errResult(fmt.Sprintf("search root not accessible: %v", err)), nil
	}

	// 默认值
	recursive := true
	if argsJSON != "" && !strings.Contains(argsJSON, "recursive") {
		// 用户没传 → 用默认 true
	} else {
		recursive = args.Recursive
	}
	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = DefaultMaxResults
	}

	// 编译 AST 节点的预编译数据（避免每文件重编译）
	compiled, err := compileExpr(&args.Expression)
	if err != nil {
		return errResult(err.Error()), nil
	}

	// 遍历
	var matches []SearchMatch
	total := 0
	scanned := 0
	scannedLimited := false
	truncated := false

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过不可访问的目录
		}
		// ctx 取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			// 跳过隐藏目录
			if strings.HasPrefix(d.Name(), ".") && path != searchRoot {
				return filepath.SkipDir
			}
			return nil
		}
		scanned++
		if scanned > MaxFilesScanned {
			scannedLimited = true
			return filepath.SkipAll
		}

		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		relPath, _ := filepath.Rel(rootAbs, path)
		relPath = "/" + strings.ReplaceAll(relPath, string(os.PathSeparator), "/")

		ok, err2 := compiled.match(ctx, path, relPath, info)
		if err2 != nil {
			return nil // 表达式错误不应中断遍历
		}
		if !ok {
			return nil
		}

		total++
		if int64(len(matches)) < int64(maxResults) {
			matches = append(matches, SearchMatch{
				Path:  relPath,
				Size:  info.Size(),
				MTime: info.ModTime().UTC().Format(time.RFC3339),
				Ext:   strings.TrimPrefix(strings.ToLower(filepath.Ext(info.Name())), "."),
			})
		} else {
			truncated = true
		}
		return nil
	}

	if recursive {
		err = filepath.WalkDir(searchRoot, walk)
	} else {
		entries, _ := os.ReadDir(searchRoot)
		for _, e := range entries {
			full := filepath.Join(searchRoot, e.Name())
			_ = walk(full, e, nil)
		}
	}
	if err != nil && err != filepath.SkipAll && err != context.Canceled {
		slog.Warn("search_files: walk error", "error", err, "mount", args.MountID)
	}

	result := SearchFilesResult{
		Total:          total,
		Truncated:      truncated,
		ScannedLimited: scannedLimited,
		Matches:        matches,
	}
	if result.Matches == nil {
		result.Matches = []SearchMatch{}
	}
	b, _ := json.Marshal(result)
	return ToolResult{Result: string(b), Status: "success"}, nil
}

// errResult 构造错误 ToolResult 的辅助函数。
func errResult(msg string) ToolResult {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return ToolResult{Result: string(b), IsError: true, Status: "failed"}
}

// ─── AST 编译与求值 ────────────────────────────────────────────

// compiledExpr 是编译后的 AST（带预编译的正则等）。
type compiledExpr struct {
	match func(ctx context.Context, fullPath, relPath string, info os.FileInfo) (bool, error)
}

func compileExpr(e *SearchExpr) (*compiledExpr, error) {
	if e == nil {
		// nil 表达式 → 全部命中
		return &compiledExpr{match: func(_ context.Context, _, _ string, _ os.FileInfo) (bool, error) { return true, nil }}, nil
	}
	switch e.Type {
	case "and":
		children := make([]*compiledExpr, 0, len(e.Children))
		for i := range e.Children {
			c, err := compileExpr(&e.Children[i])
			if err != nil {
				return nil, err
			}
			children = append(children, c)
		}
		return &compiledExpr{
			match: func(ctx context.Context, p, r string, i os.FileInfo) (bool, error) {
				for _, c := range children {
					ok, err := c.match(ctx, p, r, i)
					if err != nil {
						return false, err
					}
					if !ok {
						return false, nil // AND 短路
					}
				}
				return true, nil
			},
		}, nil

	case "or":
		children := make([]*compiledExpr, 0, len(e.Children))
		for i := range e.Children {
			c, err := compileExpr(&e.Children[i])
			if err != nil {
				return nil, err
			}
			children = append(children, c)
		}
		return &compiledExpr{
			match: func(ctx context.Context, p, r string, i os.FileInfo) (bool, error) {
				for _, c := range children {
					ok, err := c.match(ctx, p, r, i)
					if err != nil {
						return false, err
					}
					if ok {
						return true, nil // OR 短路
					}
				}
				return false, nil
			},
		}, nil

	case "not":
		if e.Child == nil {
			return nil, fmt.Errorf("not: child required")
		}
		c, err := compileExpr(e.Child)
		if err != nil {
			return nil, err
		}
		return &compiledExpr{
			match: func(ctx context.Context, p, r string, i os.FileInfo) (bool, error) {
				ok, err := c.match(ctx, p, r, i)
				if err != nil {
					return false, err
				}
				return !ok, nil
			},
		}, nil

	// ── 叶子节点 ──
	case "name_glob":
		pattern, _ := e.Value.(string)
		re, err := globToRegex(pattern)
		if err != nil {
			return nil, fmt.Errorf("name_glob: %v", err)
		}
		return &compiledExpr{
			match: func(_ context.Context, _, _ string, i os.FileInfo) (bool, error) {
				return re.MatchString(i.Name()), nil
			},
		}, nil

	case "name_regex":
		pattern, _ := e.Value.(string)
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("name_regex: %v", err)
		}
		return &compiledExpr{
			match: func(_ context.Context, _, _ string, i os.FileInfo) (bool, error) {
				return re.MatchString(i.Name()), nil
			},
		}, nil

	case "content_regex":
		pattern, _ := e.Value.(string)
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("content_regex: %v", err)
		}
		return &compiledExpr{
			match: func(_ context.Context, fullPath, _ string, info os.FileInfo) (bool, error) {
				if info.Size() > MaxContentRegexSize {
					return false, nil
				}
				return matchContentRegex(fullPath, re)
			},
		}, nil

	case "size_gt":
		v, _ := toInt64(e.Value)
		return &compiledExpr{
			match: func(_ context.Context, _ string, _ string, i os.FileInfo) (bool, error) {
				return i.Size() > v, nil
			},
		}, nil

	case "size_lt":
		v, _ := toInt64(e.Value)
		return &compiledExpr{
			match: func(_ context.Context, _, _ string, i os.FileInfo) (bool, error) {
				return i.Size() < v, nil
			},
		}, nil

	case "size_eq":
		v, _ := toInt64(e.Value)
		return &compiledExpr{
			match: func(_ context.Context, _, _ string, i os.FileInfo) (bool, error) {
				return i.Size() == v, nil
			},
		}, nil

	case "mtime_after":
		ts, err := parseTime(e.Value)
		if err != nil {
			return nil, fmt.Errorf("mtime_after: %v", err)
		}
		return &compiledExpr{
			match: func(_ context.Context, _, _ string, i os.FileInfo) (bool, error) {
				return i.ModTime().After(ts), nil
			},
		}, nil

	case "mtime_before":
		ts, err := parseTime(e.Value)
		if err != nil {
			return nil, fmt.Errorf("mtime_before: %v", err)
		}
		return &compiledExpr{
			match: func(_ context.Context, _, _ string, i os.FileInfo) (bool, error) {
				return i.ModTime().Before(ts), nil
			},
		}, nil

	case "ext_eq":
		target, _ := e.Value.(string)
		target = strings.ToLower(strings.TrimPrefix(target, "."))
		return &compiledExpr{
			match: func(_ context.Context, _, _ string, i os.FileInfo) (bool, error) {
				ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(i.Name()), "."))
				return ext == target, nil
			},
		}, nil

	case "path_contains":
		substr, _ := e.Value.(string)
		return &compiledExpr{
			match: func(_ context.Context, _, relPath string, _ os.FileInfo) (bool, error) {
				return strings.Contains(relPath, substr), nil
			},
		}, nil

	case "path_not_contains":
		substr, _ := e.Value.(string)
		return &compiledExpr{
			match: func(_ context.Context, _, relPath string, _ os.FileInfo) (bool, error) {
				return !strings.Contains(relPath, substr), nil
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown_expr_type: %s", e.Type)
	}
}

// ─── 工具函数 ───────────────────────────────────────────────────

// globToRegex 把简单 glob 编译成正则。
// 支持：*（非 / 任意字符）、**（跨 / 任意字符）、?（单字符）、.（字面点）。
func globToRegex(pattern string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	i := 0
	for i < len(pattern) {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				sb.WriteString(".*")
				i += 2
			} else {
				sb.WriteString("[^/]*")
				i++
			}
		case '?':
			sb.WriteString("[^/]")
			i++
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
			i++
		default:
			sb.WriteByte(c)
			i++
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}

// toInt64 把 any 转换为 int64（支持 float64 / int / int64 / string）。
func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case float64:
		return int64(x), nil
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	case string:
		return strconv.ParseInt(x, 10, 64)
	default:
		return 0, fmt.Errorf("not a number: %v", v)
	}
}

// parseTime 把 any 解析为 time.Time。
func parseTime(v any) (time.Time, error) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("not a string: %v", v)
	}
	// 尝试 RFC3339，再尝试 yyyy-mm-dd
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unsupported time format: %s", s)
}

// matchContentRegex 在文件中查找正则（限制文件大小，bufio 扫描）。
func matchContentRegex(path string, re *regexp.Regexp) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// 大行（长字符串单行）适当放大 buffer
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		if re.Match(scanner.Bytes()) {
			return true, nil
		}
	}
	return false, nil
}
