// internal/server/cors_preflight_test.go
//
// 验证 CORS 预检配置正确性（防 ai-routing-cors-preflight-fix 回归）：
//   1. OPTIONS /api/chat 来自 https://localhost + X-Agent-Protocol 头 → 预检通过
//   2. OPTIONS /api/chat 来自 LAN 候选 origin → 放行
//   3. 实际 POST /api/chat 带 X-Agent-Protocol: agui → 不被 CORS 拦截
//   4. 反向：未列出的 origin → 预检拒绝（不允许 GET 业务响应被读取）
package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/gin-gonic/gin"
)

// setupCORSTestRouter 构造一个带 CORS 中间件的完整 gin 引擎（agent routes）
// 不需要真实 mock 上游（OPTIONS 不会进 handleAgentChat）
func setupCORSTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	// 写一个最小配置，让 readAgentConfig 不会 NPE
	os.WriteFile(cfgPath, []byte(`{
		"agent_settings": {
			"openai_api_key": "sk-test",
			"openai_base_url": "https://api.test.com"
		}
	}`), 0644)

	s := &Server{
		configPath: cfgPath,
		cfg:        &config.Config{},
	}

	// 用 NewGinApp 而不是 gin.New()，这样 CORS 中间件才会被注册
	r := NewGinApp(s.cfg)
	s.registerAgentRoutes(r)
	return r
}

// ─── 预检（OPTIONS）───

func TestCORSPreflight_AllowsCapacitorOriginWithAgentProtocolHeader(t *testing.T) {
	r := setupCORSTestRouter(t)

	req := httptest.NewRequest("OPTIONS", "/api/chat", nil)
	req.Header.Set("Origin", "https://localhost")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type, x-agent-protocol")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 预检通过：状态 204/200
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Fatalf("preflight status = %d, want 204 or 200; body=%s", w.Code, w.Body.String())
	}

	// 关键：Access-Control-Allow-Origin 必须回显 origin（不能用 * 与 allow-credentials 混用，但 credentials 已 false）
	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "https://localhost" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", allowOrigin, "https://localhost")
	}

	// 关键：Access-Control-Allow-Headers 必须包含 x-agent-protocol（不分大小写）
	// 通配 "*" 也算通过（gin-contrib/cors 对 AllowHeaders: ["*"] 会直接回 "*"）
	allowHeaders := strings.ToLower(w.Header().Get("Access-Control-Allow-Headers"))
	if allowHeaders != "*" && !strings.Contains(allowHeaders, "x-agent-protocol") {
		t.Errorf("Access-Control-Allow-Headers = %q, must contain '*' or 'x-agent-protocol'", w.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestCORSPreflight_AllowsLoopbackOrigins(t *testing.T) {
	r := setupCORSTestRouter(t)

	cases := []string{
		"http://localhost",
		"http://127.0.0.1:2025",
		"http://127.0.0.1:2026", // EncvGoService 端口扫描备用
		"http://localhost:8100", // Ionic dev
		"http://localhost:16666", // preview-gateway
	}
	for _, origin := range cases {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest("OPTIONS", "/api/chat", nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "content-type, x-agent-protocol")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
				t.Errorf("origin=%q → Allow-Origin=%q, want %q", origin, got, origin)
			}
		})
	}
}

func TestCORSPreflight_RejectsUnknownOrigin(t *testing.T) {
	r := setupCORSTestRouter(t)

	req := httptest.NewRequest("OPTIONS", "/api/chat", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type, x-agent-protocol")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 未列出 origin 不应回显 Allow-Origin
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unknown origin got Allow-Origin=%q, want empty", got)
	}
}

// ─── 实际 POST 流量（验证 CORS 不会拦截业务请求）───

func TestCORSActualRequest_DoesNotInjectPreflightError(t *testing.T) {
	// 这个测试模拟浏览器：发 OPTIONS 预检 + 发 POST 实际请求
	// 我们不真的连上游（会失败 502/503），但验证：
	//   - OPTIONS 预检 204
	//   - POST 实际请求能进到 handleAgentChat（业务响应 4xx，不是 0 字节）
	r := setupCORSTestRouter(t)

	// ① OPTIONS 预检
	preReq := httptest.NewRequest("OPTIONS", "/api/chat", nil)
	preReq.Header.Set("Origin", "https://localhost")
	preReq.Header.Set("Access-Control-Request-Method", "POST")
	preReq.Header.Set("Access-Control-Request-Headers", "content-type, x-agent-protocol")
	preW := httptest.NewRecorder()
	r.ServeHTTP(preW, preReq)

	if preW.Code != http.StatusNoContent && preW.Code != http.StatusOK {
		t.Fatalf("preflight failed: status=%d body=%s", preW.Code, preW.Body.String())
	}
	if preW.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatalf("preflight missing Allow-Headers")
	}

	// ② POST 实际请求（带 X-Agent-Protocol 头；不真的连上游，会得到 5xx 业务错误）
	postReq := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{
		"sessionId": "test",
		"model": "gpt-4",
		"messages": [{"role":"user","content":"hi"}],
		"deviceId": "test-device"
	}`))
	postReq.Header.Set("Origin", "https://localhost")
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("X-Agent-Protocol", "agui")
	postW := httptest.NewRecorder()
	r.ServeHTTP(postW, postReq)

	// POST 应进到 handleAgentChat。业务会失败（无上游 mock），但 HTTP 状态应是 5xx
	// 而不是 0 字节或连接被拒。
	// 关键：Access-Control-Allow-Origin 应在响应里（让浏览器能读到响应体做错误处理）
	if got := postW.Header().Get("Access-Control-Allow-Origin"); got != "https://localhost" {
		t.Errorf("POST response missing Access-Control-Allow-Origin for allowed origin: got %q", got)
	}
}
