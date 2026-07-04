// Package tools — 跨平台 high-level bash 工具抽象。
//
// 设计动机（参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §跨平台 bash 工具抽象）：
//   - 现有 command_run（[command_run.go](command_run.go)）是底层 os/exec wrapper，
//     LLM 需要自己拼 ls / cat / grep 命令 → Windows 后端直接挂
//   - high-level 工具把"业务意图"（list_dir / show_file / tail_lines 等）
//     翻译成平台相关的真实命令 + 参数 + 输出解析
//
// 关键设计：
//   - 10 个 high-level 工具共享一个统一的高层框架（parseArgs → resolveCommand →
//     runCommand → parseOutput）
//   - 每个工具有自己的 BashArgs 类型 + BuildShellCmd 函数 + ParseOutput 函数
//   - Windows 走 `powershell -Command "<PS 脚本>"` 包装
//   - 所有调用走 tool_whitelist（powershell 必须加进白名单）
//
// 文件关系：
//   - platform_dispatch.go: 平台检测 + 命令名映射
//   - high_level.go (本文件): 10 个工具的实现 + 统一框架
//   - command_run.go: 底层 os/exec 包装（runCommand 抽出来供 high_level 复用）
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ─── 公共 helper：runCommand（从 command_run.go 抽出供 high_level 复用）───

// runCommand 在 context 限定的 timeout 内执行命令，捕获 stdout/stderr。
//
// 行为：
//   - 找不到二进制（exec.LookPath 失败）→ 返回 (nil, *ToolError{Code: "ENOENT"})
//   - ctx 超时 / 取消 → 返回 *ToolError{Code: "TIMEOUT"}
//   - 非零 exit code → 返回 (combined, *ToolError{Code: "EXEC_FAILED"})
//   - 输出超过 maxBytes → 截断（不报错）
//
// 参数 timeoutSec 单位为秒（int），0 或负数 → 用 DefaultCommandTimeout。
//
// 复用关系：command_run.go 的 commandRunHandler 也应该改用此 helper（保持向后兼容）。
// 本次任务只新增 high_level 调用，老的 command_run 内部暂不切换。
func runCommand(ctx context.Context, bin string, args []string, timeoutSec int, maxBytes int) (stdout, stderr string, exitCode int, te *ToolError) {
	if timeoutSec <= 0 {
		timeoutSec = DefaultCommandTimeout
	}
	timeout := time.Duration(timeoutSec) * time.Second
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t0 := time.Now()
	cmd := exec.CommandContext(cctx, bin, args...)
	var so, se bytes.Buffer
	if maxBytes > 0 {
		so.Grow(maxBytes + 1024)
		se.Grow(maxBytes + 1024)
	}
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	_ = time.Since(t0).Milliseconds() // 保留给未来 metrics

	stdoutStr := so.String()
	stderrStr := se.String()
	if maxBytes > 0 && len(stdoutStr) > maxBytes {
		stdoutStr = stdoutStr[:maxBytes]
	}
	if maxBytes > 0 && len(stderrStr) > maxBytes {
		stderrStr = stderrStr[:maxBytes]
	}
	if err != nil {
		// 区分超时 vs 执行失败
		code := CodeExecFailed
		if cctx.Err() == context.DeadlineExceeded {
			code = CodeTimeout
		} else if _, ok := err.(*exec.ExitError); !ok {
			// 不是 ExitError（找不到二进制 / signal）→ ENOENT / 其他
			if strings.Contains(err.Error(), "executable file not found") ||
				strings.Contains(err.Error(), "no such file") {
				code = CodeENOENT
			}
		}
		return stdoutStr, stderrStr, cmd.ProcessState.ExitCode(), &ToolError{
			Code:        code,
			Message:     fmt.Sprintf("%s failed: %v", bin, err),
			Underlying:  err,
			Recoverable: code == CodeTimeout,
		}
	}
	return stdoutStr, stderrStr, 0, nil
}

// shellQuoteWindows 包装 Windows 平台参数（powershell -Command 模式）。
// powershell 单引号字符串：单引号内部 ' 必须双写（”）。
func shellQuoteWindows(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", "''") + "'"
}

// shellQuotePosix 包装 Linux/Darwin/Android 平台参数。
// 使用单引号 + 转义单引号（POSIX 标准）。
func shellQuotePosix(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\'\'`) + "'"
}

// shellQuote 按平台返回带引号的参数。
func shellQuote(platform, arg string) string {
	if platform == PlatformWindows {
		return shellQuoteWindows(arg)
	}
	return shellQuotePosix(arg)
}

// ─── 通用 high-level 工具框架 ──────────────────────────────────

// BashArgs 是各工具共用的"基础参数"（mount_id + rel_path 之类）。
// 各具体工具可以在自己的 Args struct 上扩展字段。
type BashArgs struct {
	// MountID 挂载点 ID（解析为物理路径）
	MountID string `json:"mount_id,omitempty"`
	// Path 文件 / 目录路径（相对 mount 根，或绝对路径）
	Path string `json:"path,omitempty"`
	// Root 搜索根（仅 find_by_* 使用）
	Root string `json:"root,omitempty"`
	// Pattern 模式字符串
	Pattern string `json:"pattern,omitempty"`
	// N 行数（head_lines / tail_lines）
	N int `json:"n,omitempty"`
	// Cmd 命令名（which_cmd）
	Cmd string `json:"cmd,omitempty"`
	// TimeoutSec 自定义超时（默认 5s）
	TimeoutSec int `json:"timeout_sec,omitempty"`
}

// resolveMountPath 把 BashArgs.MountID + Path 解析为物理绝对路径。
//
// 语义：
//   - MountID 空 → 直接用 Path（认为是绝对路径或相对 cwd）
//   - MountID 非空 → 用 deps.ResolveMount 解析 → 拼上 Path
//
// 返回的路径已 safeJoin 校验（防止 .. 越权）。
func resolveMountPath(args BashArgs, deps *ToolDeps) (string, *ToolError) {
	if args.MountID == "" {
		// 无 mount：直接信任 Path
		if args.Path == "" {
			return "", NewToolError(CodeInvalidArgs, "path is required when mount_id is empty")
		}
		return filepath.Clean(args.Path), nil
	}
	if deps == nil || deps.ResolveMount == nil {
		return "", NewToolError(CodeInvalidArgs, "deps.ResolveMount not initialized")
	}
	root, ok := deps.ResolveMount(args.MountID)
	if !ok {
		return "", NewToolError(CodeMountNotFound, fmt.Sprintf("mount not found: %s", args.MountID))
	}
	rel := args.Path
	if rel == "" {
		rel = "/"
	}
	abs, err := safeJoin(root, rel)
	if err != nil {
		return "", &ToolError{
			Code:        CodePathEscape,
			Message:     fmt.Sprintf("path escapes mount root: %s", rel),
			Underlying:  err,
			Recoverable: false,
		}
	}
	return abs, nil
}

// runHighLevel 把"平台命令 + 参数"统一走 runCommand 调用。
//
// 关键步骤：
//  1. ResolveCommand(toolID, platform) 查表拿真实命令名
//  2. DefaultToolWhitelist 检查（防止任意命令执行）
//  3. 区分 Windows（powershell -Command "<PS>"）与 POSIX（直接 exec）
//  4. runCommand 调底层
//  5. 调用 ParseOutput 解析输出
//
// 失败时返回 (zero, *ToolError)；成功返回 (parsedOutput, nil)。
func runHighLevel(
	ctx context.Context,
	toolID string,
	platform string,
	bin string,
	args []string,
	timeoutSec int,
	parser func(stdout string) (any, *ToolError),
) (ToolResult, *ToolError) {
	if bin == "" {
		return ToolResult{}, &ToolError{
			Code:    CodeUnsupportedPlatform,
			Message: fmt.Sprintf("tool %s not supported on platform %s", toolID, platform),
		}
	}
	if !IsWhitelisted(bin) {
		return ToolResult{}, &ToolError{
			Code:    CodeNotInWhitelist,
			Message: fmt.Sprintf("command %s not in tool_whitelist", bin),
		}
	}
	timeout := DefaultCommandTimeout
	if timeoutSec > 0 && timeoutSec <= 30 {
		timeout = timeoutSec
	}
	stdout, stderr, exitCode, te := runCommand(ctx, bin, args, timeout, MaxCommandOutputBytes)
	if te != nil {
		return ToolResult{}, te
	}
	_ = stderr
	_ = exitCode // 已并入 te.Message
	parsed, perr := parser(stdout)
	if perr != nil {
		return ToolResult{}, perr
	}
	b, _ := json.Marshal(parsed)
	return ToolResult{Result: string(b), Status: "success"}, nil
}

// ─── 10 个 high-level 工具实现 ──────────────────────────────────

// ─── 1. list_dir ───────────────────────────────────────────────

// listDirBuildArgs 构造真实命令的参数列表。
//
//   - Linux/Darwin/Android: ls -la {path}
//   - Windows: powershell -Command "Get-ChildItem -LiteralPath {path} | Format-List"
func listDirBuildArgs(platform, absPath string) (bin string, args []string) {
	bin = resolveRealCmd("list_dir", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		// Get-ChildItem + Format-List → 类似 ls -la
		ps := fmt.Sprintf("Get-ChildItem -LiteralPath %s -Force | Format-List",
			shellQuoteWindows(absPath))
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{"-la", absPath}
}

// listDirParseOutput 把 ls 输出解析为目录条目列表。
//
// 输出格式（每行）：
//
//	drwxr-xr-x  2 user group 4096 May 1 12:00 dirname
//
// 简化：仅按行 split，不强解析 mode bits（容错性优先）。
func listDirParseOutput(stdout string) (any, *ToolError) {
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	out := make([]map[string]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		// 提取最后一段作为 name（避免 mode 解析的脆弱性）
		idx := strings.LastIndexAny(line, " \t")
		name := line
		if idx >= 0 {
			name = strings.TrimSpace(line[idx+1:])
		}
		// 简单标记 dir 或 file
		entry := map[string]string{
			"name": name,
			"line": line,
		}
		if strings.HasPrefix(line, "d") || strings.HasPrefix(line, "l") {
			entry["type"] = "dir"
		} else if strings.HasPrefix(line, "-") {
			entry["type"] = "file"
		} else {
			entry["type"] = "other"
		}
		out = append(out, entry)
	}
	return map[string]any{
		"entries": out,
		"count":   len(out),
		"raw":     stdout,
	}, nil
}

// listDirHandler 是 list_dir 工具的 BashLikeHandler。
func listDirHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	abs, te := resolveMountPath(args, deps)
	if te != nil {
		return ToolResult{}, te
	}
	bin, realArgs := listDirBuildArgs(platform, abs)
	return runHighLevel(ctx, "list_dir", platform, bin, realArgs, args.TimeoutSec, listDirParseOutput)
}

// ─── 2. show_file ──────────────────────────────────────────────

// showFileBuildArgs 构造 cat / Get-Content 参数。
//
//   - Linux/Darwin/Android: cat {path}
//   - Windows: powershell -Command "Get-Content -LiteralPath {path} -TotalCount {n}"
func showFileBuildArgs(platform, absPath string, maxLines int) (bin string, args []string) {
	bin = resolveRealCmd("show_file", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := fmt.Sprintf("Get-Content -LiteralPath %s", shellQuoteWindows(absPath))
		if maxLines > 0 {
			ps += fmt.Sprintf(" -TotalCount %d", maxLines)
		}
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{absPath}
}

func showFileParseOutput(stdout string) (any, *ToolError) {
	return map[string]any{
		"content": stdout,
		"bytes":   len(stdout),
		"lines":   strings.Count(stdout, "\n") + 1,
	}, nil
}

func showFileHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	abs, te := resolveMountPath(args, deps)
	if te != nil {
		return ToolResult{}, te
	}
	bin, realArgs := showFileBuildArgs(platform, abs, args.N)
	return runHighLevel(ctx, "show_file", platform, bin, realArgs, args.TimeoutSec, showFileParseOutput)
}

// ─── 3. tail_lines ─────────────────────────────────────────────

func tailLinesBuildArgs(platform, absPath string, n int) (bin string, args []string) {
	bin = resolveRealCmd("tail_lines", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := fmt.Sprintf("Get-Content -LiteralPath %s -Tail %d", shellQuoteWindows(absPath), n)
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{"-n", strconv.Itoa(n), absPath}
}

func tailLinesHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	if args.N <= 0 {
		args.N = 10
	}
	abs, te := resolveMountPath(args, deps)
	if te != nil {
		return ToolResult{}, te
	}
	bin, realArgs := tailLinesBuildArgs(platform, abs, args.N)
	return runHighLevel(ctx, "tail_lines", platform, bin, realArgs, args.TimeoutSec, func(stdout string) (any, *ToolError) {
		return map[string]any{
			"content": stdout,
			"lines":   strings.Count(stdout, "\n") + 1,
			"n":       args.N,
		}, nil
	})
}

// ─── 4. head_lines ─────────────────────────────────────────────

func headLinesBuildArgs(platform, absPath string, n int) (bin string, args []string) {
	bin = resolveRealCmd("head_lines", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := fmt.Sprintf("Get-Content -LiteralPath %s -Head %d", shellQuoteWindows(absPath), n)
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{"-n", strconv.Itoa(n), absPath}
}

func headLinesHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	if args.N <= 0 {
		args.N = 10
	}
	abs, te := resolveMountPath(args, deps)
	if te != nil {
		return ToolResult{}, te
	}
	bin, realArgs := headLinesBuildArgs(platform, abs, args.N)
	return runHighLevel(ctx, "head_lines", platform, bin, realArgs, args.TimeoutSec, func(stdout string) (any, *ToolError) {
		return map[string]any{
			"content": stdout,
			"lines":   strings.Count(stdout, "\n") + 1,
			"n":       args.N,
		}, nil
	})
}

// ─── 5. find_by_name ───────────────────────────────────────────

func findByNameBuildArgs(platform, absRoot, pattern string) (bin string, args []string) {
	bin = resolveRealCmd("find_by_name", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := fmt.Sprintf("Get-ChildItem -LiteralPath %s -Recurse -Filter %s -File -ErrorAction SilentlyContinue | Select-Object -ExpandProperty FullName",
			shellQuoteWindows(absRoot), shellQuoteWindows(pattern))
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{absRoot, "-name", pattern}
}

func findByNameParseOutput(stdout string) (any, *ToolError) {
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	matches := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimRight(l, "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		matches = append(matches, l)
	}
	return map[string]any{
		"matches": matches,
		"count":   len(matches),
	}, nil
}

func findByNameHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	if args.Pattern == "" {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "pattern is required for find_by_name"}
	}
	abs, te := resolveMountPath(args, deps)
	if te != nil {
		return ToolResult{}, te
	}
	bin, realArgs := findByNameBuildArgs(platform, abs, args.Pattern)
	return runHighLevel(ctx, "find_by_name", platform, bin, realArgs, args.TimeoutSec, findByNameParseOutput)
}

// ─── 6. find_by_content ────────────────────────────────────────

func findByContentBuildArgs(platform, absRoot, regex string) (bin string, args []string) {
	bin = resolveRealCmd("find_by_content", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := fmt.Sprintf("Select-String -Path %s -Pattern %s -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Path",
			shellQuoteWindows(absRoot+"\\*"), shellQuoteWindows(regex))
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{"-rn", regex, absRoot}
}

func findByContentHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	if args.Pattern == "" {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "pattern is required for find_by_content"}
	}
	abs, te := resolveMountPath(args, deps)
	if te != nil {
		return ToolResult{}, te
	}
	bin, realArgs := findByContentBuildArgs(platform, abs, args.Pattern)
	return runHighLevel(ctx, "find_by_content", platform, bin, realArgs, args.TimeoutSec, func(stdout string) (any, *ToolError) {
		lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
		matches := make([]string, 0, len(lines))
		for _, l := range lines {
			l = strings.TrimRight(l, "\r")
			if strings.TrimSpace(l) == "" {
				continue
			}
			matches = append(matches, l)
		}
		return map[string]any{
			"matches": matches,
			"count":   len(matches),
		}, nil
	})
}

// ─── 7. word_count ─────────────────────────────────────────────

func wordCountBuildArgs(platform, absPath string) (bin string, args []string) {
	bin = resolveRealCmd("word_count", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := fmt.Sprintf("Get-Content -LiteralPath %s | Measure-Object -Line -Word -Character | Select-Object Lines, Words, Characters | ConvertTo-Json -Compress",
			shellQuoteWindows(absPath))
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{"-l", absPath}
}

func wordCountHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	abs, te := resolveMountPath(args, deps)
	if te != nil {
		return ToolResult{}, te
	}
	bin, realArgs := wordCountBuildArgs(platform, abs)
	return runHighLevel(ctx, "word_count", platform, bin, realArgs, args.TimeoutSec, func(stdout string) (any, *ToolError) {
		// 简化：直接返回原始文本（" 42 path/to/file"），LLM 自己解析
		return map[string]any{
			"raw":    strings.TrimSpace(stdout),
			"source": "wc -l",
		}, nil
	})
}

// ─── 8. disk_usage ─────────────────────────────────────────────

func diskUsageBuildArgs(platform, absPath string) (bin string, args []string) {
	bin = resolveRealCmd("disk_usage", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := fmt.Sprintf("$s = (Get-ChildItem -LiteralPath %s -Recurse -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum; Write-Output (\"{0:N2} MB\" -f ($s / 1MB))",
			shellQuoteWindows(absPath))
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{"-sh", absPath}
}

func diskUsageHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	abs, te := resolveMountPath(args, deps)
	if te != nil {
		return ToolResult{}, te
	}
	bin, realArgs := diskUsageBuildArgs(platform, abs)
	return runHighLevel(ctx, "disk_usage", platform, bin, realArgs, args.TimeoutSec, func(stdout string) (any, *ToolError) {
		return map[string]any{
			"raw":    strings.TrimSpace(stdout),
			"source": "du -sh",
		}, nil
	})
}

// ─── 9. get_env ────────────────────────────────────────────────

func getEnvBuildArgs(platform string) (bin string, args []string) {
	bin = resolveRealCmd("get_env", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := "Get-ChildItem Env: | Select-Object Name,Value | ConvertTo-Json -Compress"
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{}
}

func getEnvHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	// argsJSON 在本工具中不需要解析（无参数）
	bin, realArgs := getEnvBuildArgs(platform)
	return runHighLevel(ctx, "get_env", platform, bin, realArgs, 0, func(stdout string) (any, *ToolError) {
		// 简化：按行 split（key=value）
		lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
		env := make(map[string]string, len(lines))
		for _, l := range lines {
			l = strings.TrimRight(l, "\r")
			idx := strings.Index(l, "=")
			if idx <= 0 {
				continue
			}
			env[l[:idx]] = l[idx+1:]
		}
		return map[string]any{
			"env":   env,
			"raw":   stdout,
			"count": len(env),
		}, nil
	})
}

// ─── 10. which_cmd ─────────────────────────────────────────────

func whichCmdBuildArgs(platform, cmd string) (bin string, args []string) {
	bin = resolveRealCmd("which_cmd", platform)
	if bin == "" {
		return "", nil
	}
	if platform == PlatformWindows {
		ps := fmt.Sprintf("Get-Command %s -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source", shellQuoteWindows(cmd))
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", ps}
	}
	return bin, []string{cmd}
}

func whichCmdHandler(ctx context.Context, platform string, argsJSON string, deps *ToolDeps) (ToolResult, *ToolError) {
	var args BashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "invalid args: " + err.Error(), Underlying: err}
	}
	if args.Cmd == "" {
		return ToolResult{}, &ToolError{Code: CodeInvalidArgs, Message: "cmd is required for which_cmd"}
	}
	bin, realArgs := whichCmdBuildArgs(platform, args.Cmd)
	return runHighLevel(ctx, "which_cmd", platform, bin, realArgs, args.TimeoutSec, func(stdout string) (any, *ToolError) {
		path := strings.TrimSpace(stdout)
		found := path != ""
		return map[string]any{
			"cmd":   args.Cmd,
			"path":  path,
			"found": found,
		}, nil
	})
}

// ─── 共享：resolveRealCmd ─────────────────────────────────────

// resolveRealCmd 是 ResolveCommand 的便捷封装：返回命令名（无 bool）。
// 找不到时返回空串（调用方决定如何处理）。
func resolveRealCmd(toolID, platform string) string {
	cmd, _ := ResolveCommand(toolID, platform)
	return cmd
}

// ─── ToolDef 工厂（10 个）─────────────────────────────────────

// ListDirDef 返回 list_dir 工具的 ToolDef。
func ListDirDef() *ToolDef {
	return &ToolDef{
		Name:        "list_dir",
		Description: "列出目录内容。Linux/Darwin/Android 走 ls -la；Windows 走 powershell Get-ChildItem。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"properties":{
				"mount_id":{"type":"string","description":"挂载点 ID（可省略，path 用绝对路径）"},
				"path":{"type":"string","description":"目录路径"},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(listDirHandler),
	}
}

// ShowFileDef 返回 show_file 工具的 ToolDef。
func ShowFileDef() *ToolDef {
	return &ToolDef{
		Name:        "show_file",
		Description: "显示文件内容。Linux/Darwin/Android 走 cat；Windows 走 powershell Get-Content。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["path"],
			"properties":{
				"mount_id":{"type":"string"},
				"path":{"type":"string"},
				"n":{"type":"integer","description":"最大行数（仅 Windows 生效，用 -TotalCount）"},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(showFileHandler),
	}
}

// TailLinesDef 返回 tail_lines 工具的 ToolDef。
func TailLinesDef() *ToolDef {
	return &ToolDef{
		Name:        "tail_lines",
		Description: "显示文件末尾 n 行。Linux/Darwin/Android 走 tail -n；Windows 走 powershell Get-Content -Tail。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["path","n"],
			"properties":{
				"mount_id":{"type":"string"},
				"path":{"type":"string"},
				"n":{"type":"integer","minimum":1,"default":10},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(tailLinesHandler),
	}
}

// HeadLinesDef 返回 head_lines 工具的 ToolDef。
func HeadLinesDef() *ToolDef {
	return &ToolDef{
		Name:        "head_lines",
		Description: "显示文件开头 n 行。Linux/Darwin/Android 走 head -n；Windows 走 powershell Get-Content -Head。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["path","n"],
			"properties":{
				"mount_id":{"type":"string"},
				"path":{"type":"string"},
				"n":{"type":"integer","minimum":1,"default":10},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(headLinesHandler),
	}
}

// FindByNameDef 返回 find_by_name 工具的 ToolDef。
func FindByNameDef() *ToolDef {
	return &ToolDef{
		Name:        "find_by_name",
		Description: "按文件名 pattern 递归查找。Linux/Darwin/Android 走 find -name；Windows 走 powershell Get-ChildItem -Recurse -Filter。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["path","pattern"],
			"properties":{
				"mount_id":{"type":"string"},
				"path":{"type":"string","description":"搜索根目录"},
				"pattern":{"type":"string","description":"文件名 pattern（glob 风格）"},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(findByNameHandler),
	}
}

// FindByContentDef 返回 find_by_content 工具的 ToolDef。
func FindByContentDef() *ToolDef {
	return &ToolDef{
		Name:        "find_by_content",
		Description: "按内容 regex 递归搜索。Linux/Darwin/Android 走 grep -rn；Windows 走 powershell Select-String。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["path","pattern"],
			"properties":{
				"mount_id":{"type":"string"},
				"path":{"type":"string","description":"搜索根目录"},
				"pattern":{"type":"string","description":"regex pattern"},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(findByContentHandler),
	}
}

// WordCountDef 返回 word_count 工具的 ToolDef。
func WordCountDef() *ToolDef {
	return &ToolDef{
		Name:        "word_count",
		Description: "统计文件行数 / 词数 / 字节数。Linux/Darwin/Android 走 wc -l；Windows 走 powershell Measure-Object。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["path"],
			"properties":{
				"mount_id":{"type":"string"},
				"path":{"type":"string"},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(wordCountHandler),
	}
}

// DiskUsageDef 返回 disk_usage 工具的 ToolDef。
func DiskUsageDef() *ToolDef {
	return &ToolDef{
		Name:        "disk_usage",
		Description: "显示目录磁盘占用。Linux/Darwin/Android 走 du -sh；Windows 走 powershell Measure-Object -Sum。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["path"],
			"properties":{
				"mount_id":{"type":"string"},
				"path":{"type":"string"},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(diskUsageHandler),
	}
}

// GetEnvDef 返回 get_env 工具的 ToolDef。
func GetEnvDef() *ToolDef {
	return &ToolDef{
		Name:        "get_env",
		Description: "列出所有环境变量。Linux/Darwin/Android 走 env；Windows 走 powershell Get-ChildItem Env:。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"properties":{
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(getEnvHandler),
	}
}

// WhichCmdDef 返回 which_cmd 工具的 ToolDef。
func WhichCmdDef() *ToolDef {
	return &ToolDef{
		Name:        "which_cmd",
		Description: "查找命令的绝对路径。Linux/Darwin/Android 走 which；Windows 走 powershell Get-Command。",
		Kind:        KindBashLike,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["cmd"],
			"properties":{
				"cmd":{"type":"string","description":"要查找的命令名"},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: BashLikeHandlerToToolHandler(whichCmdHandler),
	}
}
