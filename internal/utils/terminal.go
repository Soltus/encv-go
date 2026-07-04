//go:build !android

// Package utils 提供终端美化输出功能（基于 pterm）
package utils

import (
	"fmt"

	"github.com/pterm/pterm"
)

// init 初始化 pterm 的全局样式
func init() {
	// 基础样式设置
	pterm.Success.Prefix = pterm.Prefix{Text: " ✓ ", Style: pterm.NewStyle(pterm.FgGreen)}
	pterm.Error.Prefix = pterm.Prefix{Text: " ✗ ", Style: pterm.NewStyle(pterm.FgRed)}
	pterm.Info.Prefix = pterm.Prefix{Text: " ℹ ", Style: pterm.NewStyle(pterm.FgCyan)}
	pterm.Warning.Prefix = pterm.Prefix{Text: " ⚠ ", Style: pterm.NewStyle(pterm.FgYellow)}
}

// PrintSuccess 输出绿色成功消息
func PrintSuccess(format string, args ...any) {
	pterm.Success.Printfln(format, args...)
}

// PrintError 输出红色错误消息
func PrintError(format string, args ...any) {
	pterm.Error.Printfln(format, args...)
}

// PrintInfo 输出蓝色信息消息
func PrintInfo(format string, args ...any) {
	pterm.Info.Printfln(format, args...)
}

// PrintWarning 输出黄色警告消息
func PrintWarning(format string, args ...any) {
	pterm.Warning.Printfln(format, args...)
}

// PrintHeader 输出大标题
func PrintHeader(text string) {
	pterm.DefaultHeader.WithFullWidth().Println(text)
}

// PrintSection 输出带分隔线的章节标题
func PrintSection(text string) {
	pterm.Println()
	pterm.DefaultSection.Println(text)
}

// NewSpinner 创建一个新的加载动画，需要手动调用 Stop/Fail/Success
func NewSpinner(text string) (*pterm.SpinnerPrinter, error) {
	return pterm.DefaultSpinner.Start(text)
}

// PrintTable 输出表格
func PrintTable(header []string, data [][]string) {
	tableData := pterm.TableData{header}
	tableData = append(tableData, data...)
	_ = pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
}

// PrintBox 输出带边框的内容块
func PrintBox(title, content string) {
	_ = pterm.DefaultBox.WithTitle(title).Println(content)
}

// PrintKV 输出键值对信息（对齐）
func PrintKV(key, value string) {
	fmt.Printf("  %s: %s\n", pterm.Cyan(key), pterm.Gray(value))
}

// Green 返回绿色文本（用于内联）
func Green(text string) string {
	return pterm.Green(text)
}

// Yellow 返回黄色文本（用于内联）
func Yellow(text string) string {
	return pterm.Yellow(text)
}

// Cyan 返回青色文本（用于内联）
func Cyan(text string) string {
	return pterm.Cyan(text)
}
