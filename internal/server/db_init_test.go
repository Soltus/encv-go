package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/encv/plugins"
)

// setupDatabaseTestServer 创建一个用于数据库引擎测试的 server。
// engine 指定 cfg.Database.Engine（"libsql" / "turso" / "sqlite" / 其他）。
//
// 与 setupSearchTestServer 的区别：本 helper 允许自定义 Database.Engine，
// 用于测试 InitDatabase 的降级路径与 fallbackReason 传递。
func setupDatabaseTestServer(t *testing.T, engine string) (*Server, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "encv-db-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	oldMountsFile := os.Getenv("ENCV_MOUNTS_FILE")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Listen: %v", err)
	}
	availablePort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := &config.Config{
		Password: "test-password",
		Server: types.HttpServer{
			Port: availablePort,
			Dir:  tmpDir,
		},
		Webdav: types.WebdavServer{
			Root:     "/webdav/",
			Dir:      tmpDir,
			Username: "",
			Password: "",
		},
		Log: types.LogConfig{
			Level: "error",
		},
		PluginSettings: map[string]json.RawMessage{},
		Database: config.DatabaseConfig{
			Engine: engine,
		},
	}

	if err := plugins.InitializeWithSettings(cfg.PluginSettings); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("plugins init: %v", err)
	}

	ctx := config.NewContext(context.Background(), cfg)
	s := NewServer(ctx, "")

	addr, err := s.Start("test")
	if err != nil {
		s.Stop()
		os.RemoveAll(tmpDir)
		t.Fatalf("Start: %v", err)
	}

	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		s.Stop()
		os.RemoveAll(tmpDir)
		t.Fatalf("SplitHostPort: %v", splitErr)
	}
	if host == "" || host == "::" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	teardown := func() {
		s.Stop()
		os.RemoveAll(tmpDir)
		if oldMountsFile != "" {
			os.Setenv("ENCV_MOUNTS_FILE", oldMountsFile)
		} else {
			os.Unsetenv("ENCV_MOUNTS_FILE")
		}
	}

	return s, baseURL, teardown
}

// TestDatabaseFallbackReason_LibsqlStub 见 db_init_stub_test.go（//go:build !libsql）。
// TestDatabaseFallbackReason_LibsqlReal 见 db_init_libsql_test.go（//go:build libsql）。
//
// 这两个测试用 build tag 隔离：
//   - 默认 `go test`（无 -tags libsql）跑 stub 测试 — 验证 stub 路径的 fallbackReason
//   - `go test -tags libsql` 跑 real 测试 — 验证 libsql 真实初始化成功（不应有 fallback）
// 同一个测试函数名不能在两种 build 下都跑，因为断言逻辑相反（stub 期望 fallback，real 期望成功）。
//
// ⚠️ 命名铁律：Go test 文件必须以 `_test.go` 结尾才能被识别为测试文件。
//   - ✅ db_init_stub_test.go（以 _test.go 结尾）
//   - ❌ db_init_test_stub.go（以 _stub.go 结尾，会被当作普通源文件，测试不运行！）
// 这是 2026-07-02 实测踩坑：文件名写错导致测试静默不运行，差点以为 build tag 无效。

// TestDatabaseFallbackReason_UnknownEngine 验证未知引擎时 fallbackReason 包含具体引擎名。
func TestDatabaseFallbackReason_UnknownEngine(t *testing.T) {
	s, baseURL, teardown := setupDatabaseTestServer(t, "postgres") // 未知引擎
	defer teardown()

	if s.dbFallbackReason == "" {
		t.Fatal("s.dbFallbackReason should be set for unknown engine")
	}
	if !strings.Contains(s.dbFallbackReason, "postgres") {
		t.Errorf("s.dbFallbackReason should contain unknown engine name 'postgres', got: %q", s.dbFallbackReason)
	}
	t.Logf("✅ s.dbFallbackReason = %q", s.dbFallbackReason)

	infoURL := fmt.Sprintf("%s/api/database/info", baseURL)
	resp, err := http.Get(infoURL)
	if err != nil {
		t.Fatalf("GET database info: %v", err)
	}
	defer resp.Body.Close()

	var info struct {
		Engine         string `json:"engine"`
		FallbackReason string `json:"fallbackReason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if info.Engine != "sqlite" {
		t.Errorf("engine should be sqlite (fallback), got %q", info.Engine)
	}
	if !strings.Contains(info.FallbackReason, "postgres") {
		t.Errorf("fallbackReason should contain 'postgres', got: %q", info.FallbackReason)
	}
	t.Logf("✅ /api/database/info: engine=%q fallbackReason=%q", info.Engine, info.FallbackReason)
}

// TestDatabaseFallbackReason_SqliteNoFallback 验证 sqlite 引擎正常初始化时 fallbackReason 为空。
func TestDatabaseFallbackReason_SqliteNoFallback(t *testing.T) {
	s, baseURL, teardown := setupDatabaseTestServer(t, "sqlite")
	defer teardown()

	if s.dbFallbackReason != "" {
		t.Errorf("s.dbFallbackReason should be empty for successful sqlite init, got: %q", s.dbFallbackReason)
	}

	infoURL := fmt.Sprintf("%s/api/database/info", baseURL)
	resp, err := http.Get(infoURL)
	if err != nil {
		t.Fatalf("GET database info: %v", err)
	}
	defer resp.Body.Close()

	var info struct {
		Engine         string `json:"engine"`
		FallbackReason string `json:"fallbackReason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if info.Engine != "sqlite" {
		t.Errorf("engine should be sqlite, got %q", info.Engine)
	}
	if info.FallbackReason != "" {
		t.Errorf("fallbackReason should be empty for sqlite (no fallback), got: %q", info.FallbackReason)
	}
	t.Logf("✅ /api/database/info: engine=%q fallbackReason=%q (empty as expected)", info.Engine, info.FallbackReason)
}
