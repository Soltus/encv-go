//go:build !libsql
// +build !libsql

package server

import (
	"log/slog"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// initLibsqlStore stub 版本：当前二进制未编译 -tags libsql。
// 返回 (nil, nil) 让 InitDatabase 进入"store==nil && err==nil → stub 路径"分支，
// 由 db_init.go 设置 s.dbFallbackReason = "当前构建未包含 LibSQL 引擎（编译时未加 -tags libsql）"。
//
// 这是"平台不支持"的一种特殊情况 — 但不是 OS 平台不支持，而是构建标签没启用。
// 前端会看到明确的提示，不再被误导为"当前平台不支持 Turso/LibSQL 引擎"。
func initLibsqlStore(dbPath string) (tasksystem.Store, error) {
	slog.Warn("libsql engine not compiled in", "hint", "build with -tags libsql to enable")
	return nil, nil
}

func isLibsqlAvailable() bool {
	return false
}

// initLibsqlStoreWithFallback stub 版本：调用 initLibsqlStore 后无条件降级到 sqlite。
// 返回 (nil, nil) 而非 (nil, err) — 这是与 libsql 编译版本的协议差异：
//   - libsql 版本失败时返回 (nil, err)，db_init.go 用 err 构造 fallbackReason
//   - stub 版本返回 (nil, nil)，db_init.go 走 "store==nil && err==nil" 分支构造 fallbackReason
func initLibsqlStoreWithFallback(dbPath string, actualEngine *string) (tasksystem.Store, error) {
	slog.Warn("libsql engine not compiled in, falling back to sqlite",
		"hint", "build with -tags libsql to enable libsql support")
	*actualEngine = "sqlite"
	return nil, nil
}
