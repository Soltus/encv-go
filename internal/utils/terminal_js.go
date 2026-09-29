//go:build js

// js/wasm 版的终端输出。
//
// 存在的理由：同包的 terminal.go 基于 pterm，而 pterm 依赖 atomicgo.dev/keyboard
// —— 那个库在 js/wasm 下没有实现（undefined: initInput / openInputTTY），
// 一旦 wasm 链路引用本包（例如 internal/v2/plugins/text 用 utils.DetectFileMIMEType），
// 整条依赖链就编译不过。
//
// 这里给出**同名、无 pterm** 的等价实现，让库代码在 wasm 下照常可用，
// 而不是让调用方各写一份绕过逻辑。
package utils

import "fmt"

// PrintSuccess 输出成功消息（无颜色：wasm 环境通常是浏览器 console）
func PrintSuccess(format string, args ...any) {
	fmt.Printf("✓ "+format+"\n", args...)
}

// PrintError 输出错误消息
func PrintError(format string, args ...any) {
	fmt.Printf("✗ "+format+"\n", args...)
}

// PrintInfo 输出信息消息
func PrintInfo(format string, args ...any) {
	fmt.Printf("ℹ "+format+"\n", args...)
}

// PrintWarning 输出警告消息
func PrintWarning(format string, args ...any) {
	fmt.Printf("⚠ "+format+"\n", args...)
}

// PrintHeader 输出大标题
func PrintHeader(text string) {
	fmt.Printf("\n=== %s ===\n", text)
}

// PrintSection 输出章节标题
func PrintSection(text string) {
	fmt.Printf("\n--- %s ---\n", text)
}

// PrintTable 输出表格（纯文本对齐）
func PrintTable(header []string, data [][]string) {
	fmt.Println(header)
	for _, row := range data {
		fmt.Println(row)
	}
}

// PrintBox 输出带边框的内容块
func PrintBox(title, content string) {
	fmt.Printf("[%s] %s\n", title, content)
}

// PrintKV 输出键值对
func PrintKV(key, value string) {
	fmt.Printf("  %s: %s\n", key, value)
}

// Green 返回文本（wasm 下无 ANSI 颜色，原样返回）
func Green(text string) string { return text }

// Yellow 返回文本（wasm 下无 ANSI 颜色，原样返回）
func Yellow(text string) string { return text }

// Cyan 返回文本（wasm 下无 ANSI 颜色，原样返回）
func Cyan(text string) string { return text }
