//go:build !race
// +build !race

// cjkBigram 单元测试（防回归）
//
// 2026-07-02 发现 bug：旧实现按 byte 扫描，对 CJK 多字节字符处理错误。
// "在线 高清" 被错误切为 "在线 在线 高清"（重复写）。
// 新实现按 rune 扫描，正确处理 CJK。

package fts

import "testing"

func TestCjkBigram(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// 边界
		{"", ""},
		{"abc", "abc"},               // 无 CJK 原样返回
		{"在线", "在线"},                // 2 CJK → 1 bigram
		{"在线播放", "在线 线播 播放"},      // 4 CJK → 3 bigrams
		{"在", "在"},                  // 1 CJK → 原样
		{"x在线y", "x在线y"},            // CJK 在中间
		// 关键回归 case
		{"在线 高清", "在线 高清"},         // 两个 2-char 词之间有空格（应该是 "在线" + " " + "高清"）
		{"高清 视频", "高清 视频"},         // 同上
		{"Lorem 在线 高清", "Lorem 在线 高清"}, // CJK 段嵌入
		// 真实场景
		{"hello 在线", "hello 在线"},
		{"在线 hello 高清", "在线 hello 高清"},
		{"在线播放 高清视频", "在线 线播 播放 高清 清视 视频"}, // 两个 CJK 段，中间空格
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cjkBigram(tt.input)
			if got != tt.expected {
				t.Errorf("cjkBigram(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
