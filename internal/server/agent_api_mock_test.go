// internal/server/agent_api_mock_test.go
//
// Mock 模式 HTTP 集成测试 — 验证 handleAgentChat 在 cfg.Agent.MockMode != "off" 时
// 短路到 mockEngine.Run，并正确注入响应头与 SSE 事件。
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/gin-gonic/gin"
)

// ─── 工具函数 ────────────────────────────────────────────────

// buildMockModeConfig 写入一个含 agent_settings.mock_mode 的临时配置文件。
// API key 故意填一个非空占位值（mock 路径不调 OpenAI，但 cfg.APIKey == "" 会触发 503 提前返回）。
//
// 注意：readAgentConfig 用 map[string]string 反序列化 agent_settings ——
// 任何数字字段（如 mock_speed）都会导致反序列化失败 → cfg.APIKey 为空 → 503。
// 所以这里只放字符串字段，mock_speed 通过 DefaultAgentConfig() 的 1.0 默认值生效。
func buildMockModeConfig(t *testing.T, agentCfg config.Agent) string {
	t.Helper()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")

	// 只放 string 字段以兼容 readAgentConfig 的 map[string]string
	agentSettings := map[string]interface{}{
		"openai_api_key":  "sk-mock-test-placeholder",
		"openai_base_url": "https://api.openai.com",
		"openai_model":    "gpt-4o",
		"mock_mode":       agentCfg.MockMode,
	}
	if agentCfg.MockSpeed != 0 {
		// 改用 DefaultAgentConfig 默认值（1.0）以避免 readAgentConfig 失败
		// （这里只是注释提醒，实际写入不包含该字段）
		_ = agentCfg.MockSpeed
	}
	fullCfg := map[string]interface{}{
		"agent_settings": agentSettings,
		"server": map[string]interface{}{
			"port": 0,
			"dir":  tmpDir,
		},
	}
	raw, err := json.MarshalIndent(fullCfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath
}

// buildChatRequest 构造 POST /api/chat 请求体。
func buildChatRequest(sessionID, userText string) *http.Request {
	body := map[string]interface{}{
		"sessionId": sessionID,
		"model":     "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": userText},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// newMockTestHTTPServer 构造一个带 mock 配置的测试 HTTP Server。
func newMockTestHTTPServer(t *testing.T, agentCfg config.Agent) *Server {
	t.Helper()
	cfgPath := buildMockModeConfig(t, agentCfg)
	s := &Server{
		configPath: cfgPath,
		mockEngine: NewMockEngine(),
	}
	return s
}

// ─── 集成测试 — builtin 模式 ─────────────────────────────────

// TestHandleAgentChat_MockBuiltin 验证 builtin 模式短路 + 响应头 + SSE 事件内容。
func TestHandleAgentChat_MockBuiltin(t *testing.T) {
	resetSessionsForTest()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newMockTestHTTPServer(t, config.Agent{
		MockMode:  "builtin",
		MockSpeed: 10.0,
	})
	s.registerAgentRoutes(r)

	req := buildChatRequest("test-device-mock", "有哪些视频文件")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// 状态码：mock 路径返回 200（SSE）
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// 响应头：X-Mock-Scenario / X-Mock-Mode
	if got := rec.Header().Get("X-Mock-Scenario"); got != "list_files_query" {
		t.Errorf("X-Mock-Scenario = %q, want list_files_query", got)
	}
	if got := rec.Header().Get("X-Mock-Mode"); got != "builtin" {
		t.Errorf("X-Mock-Mode = %q, want builtin", got)
	}

	body := rec.Body.String()

	// SSE 内容校验
	if !strings.Contains(body, "tool_call") {
		t.Error("SSE 输出应含 tool_call 事件")
	}
	if !strings.Contains(body, "stream_end") {
		t.Error("SSE 输出应含 stream_end 事件")
	}
	// list_mounts 工具调用名应出现
	if !strings.Contains(body, "list_mounts") {
		t.Error("SSE 输出应含 list_mounts 工具调用")
	}
	// 真实 5 步流程断言：list_mounts + 2 次 list_files + read_file 都应出现
	for _, expected := range []string{"call_mount", "call_files1", "call_files2", "call_read"} {
		if !strings.Contains(body, expected) {
			t.Errorf("SSE 输出应含真实流程 tool_call id: %s", expected)
		}
	}
	// 不应再出现老剧本硬编码的假文件名（user 真机测试时复用的"假数据"）
	if strings.Contains(body, "studio_video") {
		t.Error("SSE 输出不应再含硬编码假文件名 studio_video（已改为真实流程）")
	}
	// 第一个 stream_start 应含 mock:true
	if !strings.Contains(body, `"mock":true`) {
		t.Error("首个 stream_start 应含 mock:true 字段")
	}
}

// ─── 集成测试 — custom 模式无匹配 fallback ────────────────────

// TestHandleAgentChat_MockCustom_NoMatch 验证 custom 模式无匹配时
// Match 返回 nil → 继续走真实 OpenAI 路径（不调 mock）。
//
// 真实路径需要 APIKey+网络；这里用 cfg.APIKey 非空但 base_url 故意指向无效地址，
// 让 callOpenAIStream 快速失败后返回 stream_error。校验关键点：没有 X-Mock-* 头。
func TestHandleAgentChat_MockCustom_NoMatch(t *testing.T) {
	resetSessionsForTest()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// custom 模式 + 故意不命中的输入
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "sk-mock-placeholder",
			"openai_base_url": "http://127.0.0.1:1",
			"openai_model": "gpt-4o",
			"mock_mode": "custom",
			"mock_speed": 10.0
		},
		"server": {"port": 0, "dir": "` + tmpDir + `"}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	s := &Server{configPath: cfgPath, mockEngine: NewMockEngine()}
	s.registerAgentRoutes(r)

	// 用完全不会命中任何 custom 剧本的输入
	req := buildChatRequest("test-custom", "完全不相关的内容")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// 关键断言：custom 模式无匹配 → 不应注入 X-Mock-* 头
	if got := rec.Header().Get("X-Mock-Scenario"); got != "" {
		t.Errorf("custom 无匹配时不应有 X-Mock-Scenario, got %q", got)
	}
	if got := rec.Header().Get("X-Mock-Mode"); got != "" {
		t.Errorf("custom 无匹配时不应有 X-Mock-Mode, got %q", got)
	}
}

// ─── 集成测试 — builtin fallback ─────────────────────────────

// TestHandleAgentChat_MockBuiltin_Fallback 验证 builtin 模式无匹配
// 时 Match 内部 fallback 到 default_friendly（不是走真实 API）。
func TestHandleAgentChat_MockBuiltin_Fallback(t *testing.T) {
	resetSessionsForTest()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newMockTestHTTPServer(t, config.Agent{
		MockMode:  "builtin",
		MockSpeed: 10.0,
	})
	s.registerAgentRoutes(r)

	// 完全不会命中任何内置剧本的输入
	req := buildChatRequest("test-fallback", "随便聊聊天气")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Mock-Scenario"); got != "default_friendly" {
		t.Errorf("builtin 无匹配应 fallback 到 default_friendly, got X-Mock-Scenario=%q", got)
	}
	if got := rec.Header().Get("X-Mock-Mode"); got != "builtin" {
		t.Errorf("X-Mock-Mode = %q, want builtin", got)
	}
}

// ─── 集成测试 — off 模式不短路 ───────────────────────────────

// TestHandleAgentChat_MockOff 验证 mock_mode="off" 时不走 mock 路径。
func TestHandleAgentChat_MockOff(t *testing.T) {
	resetSessionsForTest()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// base_url 指向无效地址 → callOpenAIStream 快速失败 → 返回 stream_error
	// 但绝不出现 X-Mock-* 响应头
	tmpDir := t.TempDir()
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "sk-test",
			"openai_base_url": "http://127.0.0.1:1",
			"openai_model": "gpt-4o",
			"mock_mode": "off"
		},
		"server": {"port": 0, "dir": "` + tmpDir + `"}
	}`
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	s := &Server{configPath: cfgPath, mockEngine: NewMockEngine()}
	s.registerAgentRoutes(r)

	req := buildChatRequest("test-off", "hello")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// mock_mode=off → 不应注入 X-Mock-* 响应头
	if got := rec.Header().Get("X-Mock-Scenario"); got != "" {
		t.Errorf("mock_mode=off 不应有 X-Mock-Scenario, got %q", got)
	}
	if got := rec.Header().Get("X-Mock-Mode"); got != "" {
		t.Errorf("mock_mode=off 不应有 X-Mock-Mode, got %q", got)
	}
}
