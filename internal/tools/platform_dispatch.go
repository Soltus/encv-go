// Package tools — 跨平台命令分发（platform_dispatch）。
//
// 设计动机（参考 .trae/specs/mobile-agent-polish-2026q2/spec.md §跨平台 bash 工具抽象）：
//   - 现有 command_run 是底层 os/exec wrapper，ls / cat 在 Windows 不存在
//   - high-level 工具（list_dir / show_file / tail_lines 等）需要按平台分发到
//     真实命令名：Linux/Darwin/Android → bash 工具链；Windows → powershell
//   - 平台检测：runtime.GOOS + 启发式（/system/build.prop → android）
//
// 数据结构：
//   - PlatformCommandMap: command_id → platform → real command name
//   - DefaultCommandMap: 预置 10 个 high-level 工具的映射
//
// 用法（high_level.go 内部）：
//
//	cmdName, ok := ResolveCommand("list_dir", "linux")  // → "ls", true
//	cmdName, ok := ResolveCommand("list_dir", "windows") // → "powershell", true
//	cmdName, ok := ResolveCommand("unknown_tool", "linux") // → "", false
package tools

import (
	"os"
	"runtime"
	"sync"
)

// PlatformCommandMap 是 command_id → platform → real command name 的二维映射。
//
// 示例（节选）：
//
//	PlatformCommandMap{
//	    "list_dir": {
//	        "linux":   "ls",
//	        "darwin":  "ls",
//	        "android": "ls",
//	        "windows": "powershell",
//	    },
//	    "show_file": {
//	        "linux":   "cat",
//	        "darwin":  "cat",
//	        "android": "cat",
//	        "windows": "powershell",
//	    },
//	    ...
//	}
type PlatformCommandMap map[string]map[string]string

// DefaultCommandMap 预置的 10 个 high-level 工具的平台映射。
//
// 设计原则（参考 spec §Scenario: 高层工具 → 真实命令转换）：
//   - Linux / Darwin / Android 共用 coreutils + findutils（bash 生态）
//   - Windows 用 powershell（兼容性最好，windows server / 桌面通用）
//   - 不引入 pwsh（PowerShell Core）— Windows 7/Server 默认不装，兼容性差
var DefaultCommandMap = PlatformCommandMap{
	"list_dir": {
		"linux":   "ls",
		"darwin":  "ls",
		"android": "ls",
		"windows": "powershell",
	},
	"show_file": {
		"linux":   "cat",
		"darwin":  "cat",
		"android": "cat",
		"windows": "powershell",
	},
	"tail_lines": {
		"linux":   "tail",
		"darwin":  "tail",
		"android": "tail",
		"windows": "powershell",
	},
	"head_lines": {
		"linux":   "head",
		"darwin":  "head",
		"android": "head",
		"windows": "powershell",
	},
	"find_by_name": {
		"linux":   "find",
		"darwin":  "find",
		"android": "find",
		"windows": "powershell",
	},
	"find_by_content": {
		"linux":   "grep",
		"darwin":  "grep",
		"android": "grep",
		"windows": "powershell",
	},
	"word_count": {
		"linux":   "wc",
		"darwin":  "wc",
		"android": "wc",
		"windows": "powershell",
	},
	"disk_usage": {
		"linux":   "du",
		"darwin":  "du",
		"android": "du",
		"windows": "powershell",
	},
	"get_env": {
		"linux":   "env",
		"darwin":  "env",
		"android": "env",
		"windows": "powershell",
	},
	"which_cmd": {
		"linux":   "which",
		"darwin":  "which",
		"android": "which",
		"windows": "powershell",
	},
}

// 平台名常量（避免散落的字符串字面量）。
const (
	PlatformLinux   = "linux"
	PlatformDarwin  = "darwin"
	PlatformWindows = "windows"
	PlatformAndroid = "android"
)

// detectPlatformOnce + cachedPlatform 用 sync.Once 缓存 DetectPlatform 结果。
// 平台在进程生命周期内不会变，避免每次 handler 调都 stat()。
var (
	detectPlatformOnce sync.Once
	cachedPlatform     string
)

// DetectPlatform 返回当前进程所在的平台（用于命令分派）。
//
// 算法（按优先级）：
//  1. runtime.GOOS == "windows" → "windows"
//  2. runtime.GOOS == "darwin" → "darwin"
//  3. runtime.GOOS == "linux"：
//     a. /system/build.prop 存在 → "android"
//     b. 否则 → "linux"
//  4. 其他 GOOS（freebsd / openbsd / ...）→ 直接返回 runtime.GOOS
//
// 简化决策（不做 root / ro.build.* 检测）：Android 真机后端运行环境下
// /system/build.prop 几乎必存在（init 进程创建），而 Linux 服务器/桌面
// 不会创建该文件。误判概率极低。
//
// 性能：首次调用 stat 一次，结果在进程内缓存。
func DetectPlatform() string {
	detectPlatformOnce.Do(func() {
		cachedPlatform = detectPlatformImpl()
	})
	return cachedPlatform
}

// detectPlatformImpl 是 DetectPlatform 的非缓存实现，便于测试时强制刷新。
//
// 不导出 + 用 buildPlatFromGOOS 分离，方便单测覆盖每个 GOOS 分支。
func detectPlatformImpl() string {
	switch runtime.GOOS {
	case "windows":
		return PlatformWindows
	case "darwin":
		return PlatformDarwin
	case "linux":
		// 进一步探测：/system/build.prop 存在 → android
		if fileExists("/system/build.prop") {
			return PlatformAndroid
		}
		return PlatformLinux
	default:
		// 其他 GOOS（freebsd / netbsd / plan9 / js / wasm）→ 原样返回
		return runtime.GOOS
	}
}

// fileExists 是 os.Stat 的便捷封装。
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ResetDetectPlatformCache 重置 DetectPlatform 缓存（仅单测用）。
//
// 单测要模拟不同平台时调此函数 + 临时设置 ENCV_FORCE_PLATFORM 环境变量。
// 不导出到主代码路径。
func ResetDetectPlatformCache() {
	detectPlatformOnce = sync.Once{}
	cachedPlatform = ""
}

// ForceDetectPlatform 用环境变量强制返回指定平台（用于单测）。
//
// 约定：设置 ENCV_FORCE_PLATFORM=android 时 → 无论真实环境如何都返回 "android"。
// 不在生产代码读取此变量（避免用户意外劫持命令分派）。
//
// 实际读取逻辑在 detectPlatformImpl 前面加一个环境变量短路。
// 把它做成函数式（无锁）以便 mock。
func detectPlatformWithOverride() string {
	if v := os.Getenv("ENCV_FORCE_PLATFORM"); v != "" {
		return v
	}
	return detectPlatformImpl()
}

// ResolveCommand 查表返回 command_id 在指定平台下的真实命令名。
//
// 返回 (realCmd, true) 表示命中；("", false) 表示未注册或平台不支持。
//
// 用法：
//
//	cmdName, ok := ResolveCommand("list_dir", "linux")  // ("ls", true)
//	cmdName, ok := ResolveCommand("list_dir", "windows") // ("powershell", true)
//	cmdName, ok := ResolveCommand("unknown", "linux")    // ("", false)
func ResolveCommand(toolID, platform string) (string, bool) {
	byPlatform, ok := DefaultCommandMap[toolID]
	if !ok {
		return "", false
	}
	cmd, ok := byPlatform[platform]
	if !ok {
		return "", false
	}
	return cmd, true
}

// IsWhitelisted 检查 cmdName 是否在 DefaultToolWhitelist 中。
//
// 这是 high_level.go 的 helper：所有 high-level 工具在 ResolveCommand 拿到
// 真实命令名后，必须通过此白名单检查才能调 os/exec。
//
// 注意：DefaultToolWhitelist 是 map（add 引用传递），单测可临时改它。
func IsWhitelisted(cmdName string) bool {
	_, ok := DefaultToolWhitelist[cmdName]
	return ok
}
