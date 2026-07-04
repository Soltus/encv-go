// internal/tools/read_file_v2.go
//
// read_file 增强版 — 支持分页 / 范围 / 二进制检测。
//
// 与 v1 read_file 共存，名字不同（read_file_v2）避免冲突。
// v1 read_file 仍由 agent_fs_bridge 走自己的路径。
package tools

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// ─── 参数 / 结果 ───────────────────────────────────────────────

// ReadFileV2Args 工具参数。
type ReadFileV2Args struct {
	MountID   string `json:"mount_id"`
	RelPath   string `json:"rel_path"`
	StartLine int    `json:"start_line,omitempty"` // 1-based, default 1
	EndLine   int    `json:"end_line,omitempty"`   // 1-based, inclusive, 0 = no limit
	MaxBytes  int    `json:"max_bytes,omitempty"`  // default 1MB
}

// ReadFileV2Result 工具结果。
type ReadFileV2Result struct {
	Path         string   `json:"path"`
	TotalLines   int      `json:"total_lines"`
	TotalBytes   int64    `json:"total_bytes"`
	Lines        []string `json:"lines,omitempty"`
	Binary       bool     `json:"binary"`
	Truncated    bool     `json:"truncated"`
	Encoding     string   `json:"encoding,omitempty"`
	ContentB64   string   `json:"content_base64,omitempty"`
	Warning      string   `json:"warning,omitempty"`
}

// ─── ToolDef ────────────────────────────────────────────────────

// ReadFileV2Def 返回 read_file_v2 的 ToolDef。
func ReadFileV2Def() *ToolDef {
	return &ToolDef{
		Name:        "read_file_v2",
		Description: "读取文件（v2）：支持分页 start_line/end_line、二进制检测、max_bytes 截断。",
		Kind:        KindFileRead,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["mount_id","rel_path"],
			"properties":{
				"mount_id":{"type":"string"},
				"rel_path":{"type":"string"},
				"start_line":{"type":"integer","minimum":1,"default":1},
				"end_line":{"type":"integer","minimum":1},
				"max_bytes":{"type":"integer","minimum":1024,"default":1048576}
			}
		}`,
		Handler: readFileV2Handler,
	}
}

// ─── Handler ────────────────────────────────────────────────────

func readFileV2Handler(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
	var args ReadFileV2Args
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errResult(fmt.Sprintf("invalid args: %v", err)), nil
	}
	if args.MountID == "" || args.RelPath == "" {
		return errResult("mount_id and rel_path are required"), nil
	}
	if deps == nil || deps.ResolveMount == nil {
		return errResult("deps not initialized"), nil
	}
	rootAbs, ok := deps.ResolveMount(args.MountID)
	if !ok {
		return errResult(fmt.Sprintf("mount not found: %s", args.MountID)), nil
	}
	absPath, err := safeJoin(rootAbs, args.RelPath)
	if err != nil {
		return errResult(err.Error()), nil
	}

	// 默认值
	if args.StartLine <= 0 {
		args.StartLine = 1
	}
	if args.MaxBytes <= 0 {
		args.MaxBytes = 1 * 1024 * 1024
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return errResult(fmt.Sprintf("stat: %v", err)), nil
	}
	if info.IsDir() {
		return errResult("path is a directory, use list_files"), nil
	}

	// ctx 取消
	select {
	case <-ctx.Done():
		return errResult("cancelled"), nil
	default:
	}

	res := ReadFileV2Result{
		Path:       "/" + strings.TrimPrefix(strings.ReplaceAll(strings.TrimPrefix(absPath, rootAbs), string(os.PathSeparator), "/"), "/"),
		TotalBytes: info.Size(),
	}

	// 二进制检测：读前 1KB 扫描非 UTF-8 字符
	detectBuf := make([]byte, 1024)
	f, err := os.Open(absPath)
	if err != nil {
		return errResult(fmt.Sprintf("open: %v", err)), nil
	}
	defer f.Close()
	n, _ := f.Read(detectBuf)
	detectBuf = detectBuf[:n]
	if n > 0 && !utf8.Valid(detectBuf) {
		// 二进制：返回 base64 + 截断
		res.Binary = true
		res.Encoding = "binary"
		res.Warning = "Binary file detected. Use get_metadata or specialized tool."
		// 限制 base64 长度
		maxBase64 := 1024
		truncated := n > maxBase64
		if truncated {
			res.ContentB64 = base64.StdEncoding.EncodeToString(detectBuf[:maxBase64])
			res.Truncated = true
		} else {
			res.ContentB64 = base64.StdEncoding.EncodeToString(detectBuf)
		}
		b, _ := json.Marshal(res)
		return ToolResult{Result: string(b), Status: "success"}, nil
	}

	// 文本文件：分页读
	maxBytes := args.MaxBytes
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxBytes+1024)

	allLines := make([]string, 0, 256)
	totalBytes := int64(0)
	truncated := false
	idx := 0
	for scanner.Scan() {
		idx++
		if idx < args.StartLine {
			continue
		}
		if args.EndLine > 0 && idx > args.EndLine {
			break
		}
		line := scanner.Text()
		lineBytes := int64(len(line)) + 1 // 加上换行符
		if totalBytes+lineBytes > int64(maxBytes) {
			truncated = true
			break
		}
		allLines = append(allLines, line)
		totalBytes += lineBytes
	}
	res.TotalLines = idx
	res.Lines = allLines
	res.Truncated = truncated
	res.Encoding = "utf-8"

	b, _ := json.Marshal(res)
	return ToolResult{Result: string(b), Status: "success"}, nil
}
