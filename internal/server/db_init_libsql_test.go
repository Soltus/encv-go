//go:build libsql
// +build libsql

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// TestDatabaseFallbackReason_LibsqlReal 验证当二进制编译了 -tags libsql 时，
// libsql 引擎能真实初始化成功，不会降级到 sqlite。
//
// 这是 2026-07-02 修复 libsql 运行时回退问题的核心验证测试。
//
// 修复前的 bug 链：
//  1. libsql driver OpenConnector 不接受纯文件路径（url.Parse 后 Scheme 为空）
//     → 报 "unsupported URL scheme"
//  2. libsql driver executeNoArgs (C.libsql_execute) 不期望返回行
//     → PRAGMA journal_mode=WAL 返回一行导致 "Execute returned rows" 错误
//     → 所有 PRAGMA 设置失败 → InitDatabase 降级到 sqlite
//     → 用户看到"当前平台不支持"（硬编码消息）→ 严重误导调试
//
// 修复后：
//  1. OpenConnector 添加 case "" 分支处理纯文件路径
//  2. NewLocal 用 db.Query 而非 db.Exec 执行 PRAGMA
//     → libsql 成功初始化 → engine="libsql"，无 fallback
//
// 此测试仅在 -tags libsql 编译时运行。
// 不带 -tags libsql 时走 stub 路径，见 db_init_stub_test.go 的 TestDatabaseFallbackReason_LibsqlStub。
//
// 注意：此测试需要 CGO 和 libsql 原生库（pkg/libsql/libs/<os>_<arch>/libsql_experimental.a）。
// 运行方式：CGO_ENABLED=1 go test -tags libsql ./internal/server/ -run TestDatabaseFallbackReason_LibsqlReal -v
func TestDatabaseFallbackReason_LibsqlReal(t *testing.T) {
	s, baseURL, teardown := setupDatabaseTestServer(t, "libsql")
	defer teardown()

	// 关键断言 1：libsql 应该成功初始化，s.dbFallbackReason 必须为空
	if s.dbFallbackReason != "" {
		t.Fatalf("libsql should initialize successfully with -tags libsql, but fallback occurred. "+
			"s.dbFallbackReason = %q", s.dbFallbackReason)
	}
	t.Logf("✅ s.dbFallbackReason is empty — libsql initialized successfully (no fallback)")

	// 关键断言 2：/api/database/info 应返回 engine=libsql，无 fallback
	infoURL := fmt.Sprintf("%s/api/database/info", baseURL)
	resp, err := http.Get(infoURL)
	if err != nil {
		t.Fatalf("GET database info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("database info status = %d, want 200", resp.StatusCode)
	}

	var info struct {
		Engine          string `json:"engine"`
		RequestedEngine string `json:"requestedEngine"`
		FallbackReason  string `json:"fallbackReason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}

	t.Logf("✅ /api/database/info: engine=%q requestedEngine=%q fallbackReason=%q",
		info.Engine, info.RequestedEngine, info.FallbackReason)

	if info.Engine != "libsql" {
		t.Errorf("engine should be 'libsql' (real libsql init), got %q. "+
			"This means libsql failed to initialize and fell back to sqlite — check PRAGMA / driver bugs.",
			info.Engine)
	}
	if info.RequestedEngine != "libsql" {
		t.Errorf("requestedEngine should be 'libsql', got %q", info.RequestedEngine)
	}
	if info.FallbackReason != "" {
		t.Errorf("fallbackReason should be empty (libsql should succeed), got %q", info.FallbackReason)
	}
}
