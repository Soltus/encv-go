//go:build !libsql
// +build !libsql

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestDatabaseFallbackReason_LibsqlStub 验证当 libsql 引擎被请求但二进制未编译 -tags libsql 时，
// /api/database/info 返回的 fallbackReason 是明确的"当前构建未包含 LibSQL 引擎"，
// 而非误导性的硬编码"当前平台不支持 Turso/LibSQL 引擎"。
//
// 复现场景：
//   - 用户切换到 libsql 引擎并重启应用
//   - CI 构建二进制时未加 -tags libsql（或 libsql 原生库构建失败导致 LIBSQL_READY=0）
//   - 运行时 InitDatabase 走 stub 路径（initLibsqlStoreWithFallback 返回 (nil, nil)）
//   - 前端 DatabaseDetail.vue 显示 fallbackReason
//
// 2026-07-02 修复前的 bug：
//   - handleDatabaseInfo 硬编码 "当前平台不支持 Turso/LibSQL 引擎，已自动回退到 SQLite"
//   - 用户误以为是 OS 平台不支持，实际是构建标签未启用
//
// 修复后：
//   - InitDatabase 把真实原因写入 s.dbFallbackReason
//   - handleDatabaseInfo 优先使用 s.dbFallbackReason
//   - 前端能看到 "当前构建未包含 LibSQL 引擎（编译时未加 -tags libsql）"
//
// 此测试仅在不带 -tags libsql 编译时运行（走 stub 路径）。
// 带 -tags libsql 时走真实初始化路径，见 db_init_libsql_test.go 的 TestDatabaseFallbackReason_LibsqlReal。
func TestDatabaseFallbackReason_LibsqlStub(t *testing.T) {
	s, baseURL, teardown := setupDatabaseTestServer(t, "libsql")
	defer teardown()

	// 内部状态断言：s.dbFallbackReason 应该被 InitDatabase 设置为明确消息
	if s.dbFallbackReason == "" {
		t.Fatal("s.dbFallbackReason should be set when libsql stub fallback occurred")
	}
	if !strings.Contains(s.dbFallbackReason, "当前构建未包含 LibSQL 引擎") {
		t.Errorf("s.dbFallbackReason should mention '未包含 LibSQL 引擎', got: %q", s.dbFallbackReason)
	}
	t.Logf("✅ s.dbFallbackReason = %q", s.dbFallbackReason)

	// API 端点验证：/api/database/info 应返回真实的 fallbackReason
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

	if info.Engine != "sqlite" {
		t.Errorf("engine should be sqlite (fallback), got %q", info.Engine)
	}
	if info.RequestedEngine != "libsql" {
		t.Errorf("requestedEngine should be libsql, got %q", info.RequestedEngine)
	}
	if info.FallbackReason == "" {
		t.Error("fallbackReason should not be empty when engine fell back")
	}

	// 关键断言：不能是硬编码的"当前平台不支持"
	if strings.Contains(info.FallbackReason, "当前平台不支持") {
		t.Errorf("fallbackReason should NOT contain misleading '当前平台不支持' (hardcoded message), got: %q", info.FallbackReason)
	}

	// 应该包含真实原因：未编译 libsql 标签
	if !strings.Contains(info.FallbackReason, "未包含 LibSQL 引擎") {
		t.Errorf("fallbackReason should mention '未包含 LibSQL 引擎' (real reason), got: %q", info.FallbackReason)
	}
}
