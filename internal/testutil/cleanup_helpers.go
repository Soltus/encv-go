// Package testutil/cleanup_helpers.go
// =====================================================
// 配套 cleanup.go 的 fmt 包装（避免在 cleanup.go 顶部 import fmt，
// 减少 import 顺序对用户的阅读干扰）。
// =====================================================

package testutil

import "fmt"

// sprintfImpl 是 fmtSprintf 的真实实现。
func sprintfImpl(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
