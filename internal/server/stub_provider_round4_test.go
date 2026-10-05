package server

// stub_provider_round4_test.go —— vNext Round 4 回归锁
//
// 这一条测试是整个"告别假剧本"方向的核心证据：
//
//	mock 模式 = off（明确不走假剧本）
//	上游 = 模拟服务商（只有"模型大脑"是替身）
//	 ⇒ 工具调用是真的
//	 ⇒ tool_result 必须包含**真实文件系统里的真文件名**（替身不可能知道它）
//	 ⇒ 工具结果回灌后链路必须继续第二轮

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/stubprovider"
	"github.com/Soltus/encv-go/internal/tools"
	"github.com/gin-gonic/gin"
)

func TestStubProvider_RealToolChainNotFakeScript(t *testing.T) {
	// 1. 真实文件系统：造一个只存在于磁盘上的真文件
	dir := t.TempDir()
	realName := "round4_real_file.txt"
	if err := os.WriteFile(filepath.Join(dir, realName), []byte("hello from real fs"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. 模拟服务商（替身大脑），作为"用户配置的服务商"被真实调用
	prov := stubprovider.New()
	stubSrv := httptest.NewServer(prov.Handler())
	defer stubSrv.Close()

	// 3. agent 配置：base_url 指向替身，mock_mode 明确 off
	cfgPath := filepath.Join(t.TempDir(), "config.user.json")
	cfg := fmt.Sprintf(`{"agent_settings":{"openai_api_key":"stub-key","openai_base_url":%q,"openai_model":"stub:fs_overview","mock_mode":"off"}}`, stubSrv.URL)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	tools.RegisterAll()
	s := &Server{
		configPath:     cfgPath,
		servingDir:     dir,
		mockEngine:     NewMockEngine(),
		runtimeProfile: ProfileProduction,
	}
	s.toolDeps = &tools.ToolDeps{
		ResolveMount: func(mountID string) (string, bool) { return dir, true },
		SandboxCheck: func(abs string) bool { return strings.HasPrefix(abs, dir) },
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	s.registerAgentRoutes(r)

	raw, _ := json.Marshal(map[string]any{
		"sessionId": "round4-real",
		"model":     "stub:fs_overview",
		"messages":  []map[string]string{{"role": "user", "content": "有哪些文件"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	out := rec.Body.String()

	// 断言 1：没有走 mock 假剧本（X-Mock-* 头是剧本路径的标记）
	if rec.Header().Get("X-Mock-Scenario") != "" || rec.Header().Get("X-Mock-Mode") != "" {
		t.Fatalf("must NOT go through mock script; headers=%v", rec.Header())
	}
	// 断言 2：真实文件名出现在输出里 —— 替身脚本里没有写它，只能来自真实执行
	if !strings.Contains(out, realName) {
		t.Fatalf("tool_result must come from REAL fs (want %q); body=%s", realName, out)
	}
	// 断言 3：链路真的继续了（工具结果回灌 → 再次请求替身）
	if prov.RequestCount() < 2 {
		t.Fatalf("provider requests=%d, want >=2 (loop must continue after real tool execution)", prov.RequestCount())
	}
}

func TestStubProvider_DisabledByDefault(t *testing.T) {
	// 未设 ENCV_STUB_PROVIDER ⇒ 替身大脑必须关闭（不得悄悄冒充真实模型）
	t.Setenv("ENCV_STUB_PROVIDER", "")
	s := &Server{}
	if s.StubProviderEnabled() {
		t.Fatal("stub provider must be disabled unless ENCV_STUB_PROVIDER=1")
	}
	t.Setenv("ENCV_STUB_PROVIDER", "1")
	if !s.StubProviderEnabled() {
		t.Fatal("ENCV_STUB_PROVIDER=1 must enable stub provider")
	}
	t.Setenv("ENCV_STUB_PROVIDER", "yes")
	if s.StubProviderEnabled() {
		t.Fatal("only literal '1' enables it (fail closed)")
	}
}

func TestStubProvider_RuntimeDeclaresEnabled(t *testing.T) {
	t.Setenv("ENCV_STUB_PROVIDER", "1")
	s := &Server{stubProvider: stubprovider.New()}
	info := s.snapshotRuntimeInfo()
	if !info.StubProviderEnabled {
		t.Fatal("/api/runtime must declare stub_provider_enabled=true when enabled")
	}
	t.Setenv("ENCV_STUB_PROVIDER", "")
	if s.snapshotRuntimeInfo().StubProviderEnabled {
		t.Fatal("/api/runtime must declare stub_provider_enabled=false when disabled")
	}
}
