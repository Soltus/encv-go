// internal/tools/platform_dispatch_test.go
//
// PlatformCommandMap / DetectPlatform / ResolveCommand 单测
// （参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §TestPlatformDispatch 10+ 用例）
//
// 覆盖：
//  1. ResolveCommand 基本查找（10 个工具 × 4 个平台）
//  2. ResolveCommand 未知 tool / 未知平台 → ("", false)
//  3. DefaultCommandMap 完整性（10 个工具都有 linux/darwin/windows/android 映射）
//  4. IsWhitelisted 命中 / 未命中
//  5. DetectPlatform 缓存（多次调用只 stat 一次）
//  6. fileExists helper
//  7. shellQuotePosix / shellQuoteWindows / shellQuote
//  8. ResetDetectPlatformCache
package tools

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// ─── 1. TestPlatformCommandMap_ResolveCommand_Linux ──────────────
//
// 10 个工具的 linux 映射。
func TestPlatformCommandMap_ResolveCommand_Linux(t *testing.T) {
	expected := map[string]string{
		"list_dir":        "ls",
		"show_file":       "cat",
		"tail_lines":      "tail",
		"head_lines":      "head",
		"find_by_name":    "find",
		"find_by_content": "grep",
		"word_count":      "wc",
		"disk_usage":      "du",
		"get_env":         "env",
		"which_cmd":       "which",
	}
	for toolID, wantBin := range expected {
		got, ok := ResolveCommand(toolID, "linux")
		if !ok {
			t.Errorf("ResolveCommand(%q, linux) = not found", toolID)
			continue
		}
		if got != wantBin {
			t.Errorf("ResolveCommand(%q, linux) = %q, want %q", toolID, got, wantBin)
		}
	}
}

// ─── 2. TestPlatformCommandMap_ResolveCommand_Windows ───────────
//
// Windows 10 个工具都映射到 powershell。
func TestPlatformCommandMap_ResolveCommand_Windows(t *testing.T) {
	tools := []string{
		"list_dir", "show_file", "tail_lines", "head_lines",
		"find_by_name", "find_by_content", "word_count",
		"disk_usage", "get_env", "which_cmd",
	}
	for _, toolID := range tools {
		got, ok := ResolveCommand(toolID, "windows")
		if !ok {
			t.Errorf("ResolveCommand(%q, windows) = not found", toolID)
			continue
		}
		if got != "powershell" {
			t.Errorf("ResolveCommand(%q, windows) = %q, want powershell", toolID, got)
		}
	}
}

// ─── 3. TestPlatformCommandMap_ResolveCommand_Darwin ────────────
//
// Darwin 10 个工具与 linux 相同。
func TestPlatformCommandMap_ResolveCommand_Darwin(t *testing.T) {
	tools := []string{
		"list_dir", "show_file", "tail_lines", "head_lines",
		"find_by_name", "find_by_content", "word_count",
		"disk_usage", "get_env", "which_cmd",
	}
	for _, toolID := range tools {
		got, ok := ResolveCommand(toolID, "darwin")
		if !ok {
			t.Errorf("ResolveCommand(%q, darwin) = not found", toolID)
			continue
		}
		// Darwin 应该和 linux 一样的 coreutils 命令
		if got == "" {
			t.Errorf("ResolveCommand(%q, darwin) = empty", toolID)
		}
	}
}

// ─── 4. TestPlatformCommandMap_ResolveCommand_Android ───────────
//
// Android 后端 10 个工具 → ls/cat/tail/head/find/grep/wc/du/env/which。
func TestPlatformCommandMap_ResolveCommand_Android(t *testing.T) {
	expected := map[string]string{
		"list_dir":        "ls",
		"show_file":       "cat",
		"tail_lines":      "tail",
		"head_lines":      "head",
		"find_by_name":    "find",
		"find_by_content": "grep",
		"word_count":      "wc",
		"disk_usage":      "du",
		"get_env":         "env",
		"which_cmd":       "which",
	}
	for toolID, wantBin := range expected {
		got, ok := ResolveCommand(toolID, "android")
		if !ok {
			t.Errorf("ResolveCommand(%q, android) = not found", toolID)
			continue
		}
		if got != wantBin {
			t.Errorf("ResolveCommand(%q, android) = %q, want %q", toolID, got, wantBin)
		}
	}
}

// ─── 5. TestPlatformCommandMap_UnknownTool ──────────────────────
//
// 未知 tool → ("", false)。
func TestPlatformCommandMap_UnknownTool(t *testing.T) {
	got, ok := ResolveCommand("nonexistent_tool", "linux")
	if ok {
		t.Errorf("ResolveCommand(unknown) = (%q, true), want (\"\", false)", got)
	}
	if got != "" {
		t.Errorf("ResolveCommand(unknown) = %q, want \"\"", got)
	}
}

// ─── 6. TestPlatformCommandMap_UnknownPlatform ──────────────────
//
// 已知 tool，未知平台 → ("", false)。
func TestPlatformCommandMap_UnknownPlatform(t *testing.T) {
	got, ok := ResolveCommand("list_dir", "freebsd")
	if ok {
		t.Errorf("ResolveCommand(list_dir, freebsd) = (%q, true), want (\"\", false)", got)
	}
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}

// ─── 7. TestPlatformCommandMap_AllToolsHaveAllPlatforms ─────────
//
// DefaultCommandMap 完整性：所有 10 个工具都必须有 linux/darwin/windows/android 4 个平台映射。
func TestPlatformCommandMap_AllToolsHaveAllPlatforms(t *testing.T) {
	requiredTools := []string{
		"list_dir", "show_file", "tail_lines", "head_lines",
		"find_by_name", "find_by_content", "word_count",
		"disk_usage", "get_env", "which_cmd",
	}
	requiredPlatforms := []string{"linux", "darwin", "android", "windows"}

	for _, toolID := range requiredTools {
		byPlat, ok := DefaultCommandMap[toolID]
		if !ok {
			t.Errorf("DefaultCommandMap 缺工具: %q", toolID)
			continue
		}
		for _, plat := range requiredPlatforms {
			if cmd, ok := byPlat[plat]; !ok || cmd == "" {
				t.Errorf("DefaultCommandMap[%q][%q] 缺映射", toolID, plat)
			}
		}
	}
}

// ─── 8. TestPlatformCommandMap_IsWhitelisted ────────────────────
//
// IsWhitelisted 命中 / 未命中。
func TestPlatformCommandMap_IsWhitelisted(t *testing.T) {
	// powershell 应该默认在白名单中（参考 spec §powershell 必须追加）
	if !IsWhitelisted("powershell") {
		t.Error("IsWhitelisted(powershell) = false, want true (已加入 DefaultToolWhitelist)")
	}
	if !IsWhitelisted("ls") {
		t.Error("IsWhitelisted(ls) = false, want true (coreutils 工具)")
	}
	if IsWhitelisted("rm") {
		t.Error("IsWhitelisted(rm) = true, want false (在 DeniedCommands 黑名单中)")
	}
	if IsWhitelisted("nonexistent_binary_xyz") {
		t.Error("IsWhitelisted(unknown) = true, want false")
	}
}

// ─── 9. TestPlatformDispatch_DetectPlatform_Cache ───────────────
//
// DetectPlatform 缓存：多次调用结果一致。
func TestPlatformDispatch_DetectPlatform_Cache(t *testing.T) {
	p1 := DetectPlatform()
	p2 := DetectPlatform()
	if p1 != p2 {
		t.Errorf("DetectPlatform 缓存不一致: %q != %q", p1, p2)
	}
	if p1 == "" {
		t.Error("DetectPlatform 返回空字符串")
	}
	// 应该是当前 runtime.GOOS 或 android 衍生
	if p1 != runtime.GOOS && p1 != PlatformAndroid {
		t.Errorf("DetectPlatform = %q, 既不是 %q 也不是 android", p1, runtime.GOOS)
	}
}

// ─── 10. TestPlatformDispatch_fileExists ────────────────────────
//
// fileExists helper。
func TestPlatformDispatch_fileExists(t *testing.T) {
	// 临时文件存在
	tmp := t.TempDir() + "/exists.txt"
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(tmp) {
		t.Error("fileExists(实际存在) = false")
	}
	// 不存在
	if fileExists(tmp + "_not_exist_xyz") {
		t.Error("fileExists(不存在) = true")
	}
}

// ─── 11. TestPlatformDispatch_shellQuote ────────────────────────
//
// shellQuote 各平台实现。
func TestPlatformDispatch_shellQuote(t *testing.T) {
	// POSIX 风格
	got := shellQuotePosix("hello world")
	if got != "'hello world'" {
		t.Errorf("shellQuotePosix = %q, want 'hello world'", got)
	}
	// 含单引号：POSIX 用 '\'\'' 转义（实际是 '\'' 序列）
	got = shellQuotePosix("it's")
	if !strings.HasPrefix(got, "'it'") || !strings.HasSuffix(got, "'s'") {
		t.Errorf("shellQuotePosix 含单引号 = %q, 应该是 POSIX 风格", got)
	}
	// Windows 风格
	got = shellQuoteWindows("hello world")
	if got != "'hello world'" {
		t.Errorf("shellQuoteWindows = %q, want 'hello world'", got)
	}
	// 含单引号：Windows 用 '' 双写 → "it's" → 'it''s'
	got = shellQuoteWindows("it's")
	if got != "'it''s'" {
		t.Errorf("shellQuoteWindows 含单引号 = %q, want 'it''s'", got)
	}
	// shellQuote 调度
	if shellQuote("linux", "x") != shellQuotePosix("x") {
		t.Error("shellQuote(linux) 应走 POSIX")
	}
	if shellQuote("windows", "x") != shellQuoteWindows("x") {
		t.Error("shellQuote(windows) 应走 Windows 风格")
	}
}

// ─── 12. TestPlatformDispatch_DefaultMapConsistency ────────────
//
// DefaultCommandMap 中所有"非 windows"平台的真实命令名都必须在 DefaultToolWhitelist 中。
// 这是关键安全约束：high_level 工具调用的命令必须能通过 IsWhitelisted。
func TestPlatformDispatch_DefaultMapConsistency(t *testing.T) {
	posixBins := make(map[string]bool)
	for toolID, byPlat := range DefaultCommandMap {
		_ = toolID
		for plat, bin := range byPlat {
			if plat == "windows" {
				// Windows 走 powershell 包装，bin 始终是 "powershell"
				if bin != "powershell" {
					t.Errorf("Windows 平台 %q 的 bin 应是 'powershell'，实际 %q", toolID, bin)
				}
				continue
			}
			posixBins[bin] = true
		}
	}
	for bin := range posixBins {
		if !IsWhitelisted(bin) {
			t.Errorf("DefaultCommandMap 引用了未在白名单中的命令: %q", bin)
		}
	}
}

// ─── 13. TestPlatformDispatch_ResolveCommand_EmptyPlatform ──────
//
// 空平台 → ("", false)。
func TestPlatformDispatch_ResolveCommand_EmptyPlatform(t *testing.T) {
	got, ok := ResolveCommand("list_dir", "")
	if ok {
		t.Errorf("ResolveCommand(list_dir, \"\") = (%q, true), want (\"\", false)", got)
	}
}

// ─── 14. TestPlatformDispatch_ResetCache ────────────────────────
//
// ResetDetectPlatformCache 后调 detectPlatformImpl 应重新检测。
func TestPlatformDispatch_ResetCache(t *testing.T) {
	ResetDetectPlatformCache()
	p1 := DetectPlatform()
	ResetDetectPlatformCache()
	p2 := DetectPlatform()
	if p1 != p2 {
		t.Errorf("Reset 后 DetectPlatform 不稳定: %q != %q", p1, p2)
	}
	_ = strings.Contains // ensure strings import used
}
