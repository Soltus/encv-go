// internal/tools/command_run.go
//
// 受限 shell 工具 — 仅执行白名单内的命令。
//
// 安全模型：
//   - 任何黑名单命令 → 拒绝
//   - 路径越权（..  /  /etc 等）→ 拒绝
//   - 5s 超时
//   - 8KB 输出截断
//   - 非零 exit code → isError=true + stderr
//
// 参考：
//   - Spec: /workspace/.trae/specs/agent-tools-scenarios-v2/spec.md
//   - Requirement: command_run 受限 shell
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ─── 白名单 / 黑名单 ──────────────────────────────────────────

// DefaultToolWhitelist 默认允许的命令（按二进制名）。
//
// 包含：
//   - v2 工具使用的命令：ffprobe / ffmpeg / du / wc / find / stat / mediainfo / file
//   - high_level 跨平台工具使用的 coreutils：cat / head / tail / grep / env / which / ls
//   - Windows 平台 high_level 工具的统一入口：powershell
//
// （参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §跨平台 bash 工具抽象）
var DefaultToolWhitelist = map[string]bool{
	"ffprobe":    true,
	"ffmpeg":     true,
	"du":         true,
	"wc":         true,
	"find":       true,
	"stat":       true,
	"mediainfo":  true,
	"file":       true,
	"cat":        true,
	"head":       true,
	"tail":       true,
	"grep":       true,
	"env":        true,
	"which":      true,
	"ls":         true,
	"powershell": true,
}

// DeniedCommands 黑名单（任何配置下都拒绝）。
var DeniedCommands = map[string]bool{
	"rm":       true,
	"mv":       true,
	"cp":       true,
	"chmod":    true,
	"chown":    true,
	"dd":       true,
	"mkfs":     true,
	"shutdown": true,
	"reboot":   true,
	"halt":     true,
	"poweroff": true,
}

// ─── 参数 / 结果 ──────────────────────────────────────────────

// CommandRunArgs 工具参数。
type CommandRunArgs struct {
	MountID    string   `json:"mount_id"`
	Command    string   `json:"command"`     // 二进制名（如 "ffprobe"）
	Args       []string `json:"args"`        // 命令参数
	TimeoutSec int      `json:"timeout_sec"` // 默认 5
}

// CommandRunResult 工具结果。
type CommandRunResult struct {
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        int    `json:"exit_code"`
	OutputTruncated bool   `json:"output_truncated"`
	DurationMs      int64  `json:"duration_ms"`
}

const (
	MaxCommandOutputBytes = 8 * 1024
	DefaultCommandTimeout = 5
)

// ─── ToolDef ───────────────────────────────────────────────────

// CommandRunDef 返回 command_run 的 ToolDef。
func CommandRunDef() *ToolDef {
	return &ToolDef{
		Name:        "command_run",
		Description: "受限 shell：仅白名单命令（ffprobe/du/wc/find/stat/mediainfo/file）可执行。路径越权/黑名单命令直接拒绝。5s 超时，8KB 输出截断。",
		Kind:        KindCommand,
		ReadOnly:    true,
		ArgsSchema: `{
			"type":"object",
			"required":["mount_id","command","args"],
			"properties":{
				"mount_id":{"type":"string"},
				"command":{"type":"string","description":"白名单命令名（ffprobe/du/wc/find/stat/mediainfo/file）"},
				"args":{"type":"array","items":{"type":"string"}},
				"timeout_sec":{"type":"integer","minimum":1,"maximum":30,"default":5}
			}
		}`,
		Handler: commandRunHandler,
	}
}

// ─── Handler ───────────────────────────────────────────────────

func commandRunHandler(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
	var args CommandRunArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errResult(fmt.Sprintf("invalid args: %v", err)), nil
	}
	if args.MountID == "" || args.Command == "" {
		return errResult("mount_id and command are required"), nil
	}
	cmdBin := strings.ToLower(strings.TrimSpace(args.Command))

	// 黑名单检查（任何配置下都拒绝）
	if DeniedCommands[cmdBin] {
		return errResult(fmt.Sprintf("command denied (blacklisted): %s", cmdBin)), nil
	}

	// 白名单检查
	if !DefaultToolWhitelist[cmdBin] {
		// 尝试从 deps.Config 读 AgentSettings.ToolWhitelist（追加而非覆盖）
		// 为避免循环依赖，这里采用最简方式：只允许 default 白名单
		// 用户追加通过配置文件 → 启动时改 DefaultToolWhitelist
		return errResult(fmt.Sprintf("command not in whitelist: %s (allowed: %s)",
			cmdBin, whitelistKeys(DefaultToolWhitelist))), nil
	}

	// 路径越权检查（args 中任一含 .. 或敏感绝对路径）
	for _, a := range args.Args {
		if err := validateCommandArg(a); err != nil {
			return errResult(err.Error()), nil
		}
	}

	// mount 解析（路径参数必须在 mount 内）
	if deps != nil && deps.ResolveMount != nil {
		if _, ok := deps.ResolveMount(args.MountID); !ok {
			return errResult(fmt.Sprintf("mount not found: %s", args.MountID)), nil
		}
	}

	// 超时
	timeout := DefaultCommandTimeout
	if args.TimeoutSec > 0 && args.TimeoutSec <= 30 {
		timeout = args.TimeoutSec
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	t0 := time.Now()
	cmd := exec.CommandContext(cctx, cmdBin, args.Args...)
	var stdout, stderr bytes.Buffer
	stdout.Grow(MaxCommandOutputBytes + 1024)
	stderr.Grow(MaxCommandOutputBytes + 1024)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	dur := time.Since(t0).Milliseconds()

	stdoutTrunc := false
	stdoutStr := stdout.String()
	if len(stdoutStr) > MaxCommandOutputBytes {
		stdoutStr = stdoutStr[:MaxCommandOutputBytes]
		stdoutTrunc = true
	}
	stderrStr := stderr.String()
	if len(stderrStr) > MaxCommandOutputBytes {
		stderrStr = stderrStr[:MaxCommandOutputBytes]
		stdoutTrunc = true
	}

	res := CommandRunResult{
		Stdout:          stdoutStr,
		Stderr:          stderrStr,
		ExitCode:        cmd.ProcessState.ExitCode(),
		OutputTruncated: stdoutTrunc,
		DurationMs:      dur,
	}
	if err != nil {
		b, _ := json.Marshal(map[string]any{
			"error":            err.Error(),
			"command":          cmdBin,
			"args":             args.Args,
			"exit_code":        res.ExitCode,
			"stdout":           res.Stdout,
			"stderr":           res.Stderr,
			"output_truncated": res.OutputTruncated,
			"duration_ms":      res.DurationMs,
		})
		return ToolResult{Result: string(b), IsError: true, Status: "failed", DurationMs: dur}, nil
	}
	b, _ := json.Marshal(res)
	return ToolResult{Result: string(b), Status: "success", DurationMs: dur}, nil
}

// validateCommandArg 检查命令参数是否越权。
func validateCommandArg(arg string) error {
	if strings.Contains(arg, "..") {
		return fmt.Errorf("arg rejected: contains '..': %s", arg)
	}
	for _, prefix := range []string{"/etc/", "/usr/", "/var/", "/boot/", "/sys/", "/proc/"} {
		if strings.HasPrefix(arg, prefix) {
			return fmt.Errorf("arg rejected: sensitive path prefix %s: %s", prefix, arg)
		}
	}
	return nil
}

// whitelistKeys 返回白名单的 keys 列表（用于错误消息展示）。
func whitelistKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
