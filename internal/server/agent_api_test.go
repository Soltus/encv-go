package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ─── readAgentConfig 测试 ──────────────────────────────────────

func TestReadAgentConfig_SystemPrompt(t *testing.T) {
	// 准备临时配置文件（含 system_prompt）
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "test-key",
			"openai_base_url": "https://api.example.com",
			"system_prompt": "你是一个有帮助的 AI 助手。"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	s := &Server{configPath: cfgPath}
	cfg := s.readAgentConfig()

	if cfg.SystemPrompt != "你是一个有帮助的 AI 助手。" {
		t.Errorf("SystemPrompt = %q, want %q", cfg.SystemPrompt, "你是一个有帮助的 AI 助手。")
	}
	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.example.com")
	}
}

func TestReadAgentConfig_EmptySystemPrompt(t *testing.T) {
	// system_prompt 字段缺失或为空
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "test-key",
			"openai_base_url": "https://api.example.com"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	s := &Server{configPath: cfgPath}
	cfg := s.readAgentConfig()

	if cfg.SystemPrompt != "" {
		t.Errorf("SystemPrompt 应为空字符串，得到 %q", cfg.SystemPrompt)
	}
}

func TestReadAgentConfig_NoAgentSettings(t *testing.T) {
	// 完全没有 agent_settings 节
	cfgJSON := `{"some_other": "data"}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	s := &Server{configPath: cfgPath}
	cfg := s.readAgentConfig()

	if cfg.SystemPrompt != "" || cfg.APIKey != "" || cfg.BaseURL != "" {
		t.Errorf("无 agent_settings 时应返回零值，got %+v", cfg)
	}
}

func TestReadAgentConfig_DefaultBaseURL(t *testing.T) {
	// base_url 缺失时应 fallback 到 OpenAI 默认值
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "test-key",
			"system_prompt": "hello"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	s := &Server{configPath: cfgPath}
	cfg := s.readAgentConfig()

	if cfg.BaseURL != "https://api.openai.com" {
		t.Errorf("默认 BaseURL = %q, want https://api.openai.com", cfg.BaseURL)
	}
}

// ─── System Prompt 注入逻辑测试（通过拦截上游请求验证） ──

// recordedRequest 记录 handleAgentChat 发出的上游 HTTP 请求
type recordedRequest struct {
	URL    string                 `json:"-"`
	Header map[string][]string    `json:"-"`
	Body   map[string]interface{} `json:"body"`
}

// setupChatRouter 创建 gin router 并拦截上游 HTTP 请求用于验证
// 返回 router 和 cleanup 函数（必须在测试结束时调用）
func setupChatRouter(s *Server, recorder *recordedRequest) (*gin.Engine, func()) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		recorder.URL = req.URL.String()
		recorder.Header = req.Header.Clone()
		json.NewDecoder(req.Body).Decode(&recorder.Body)
		// Agent Loop 使用 callOpenAIChatOnce（非流式），返回标准 OpenAI JSON 格式
		body := `{"choices":[{"message":{"role":"assistant","content":"ok","tool_calls":[]},"finish_reason":"stop"}]}`

		return &http.Response{
			StatusCode: 200,
			Header:     map[string][]string{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	s.registerAgentRoutes(r)

	cleanup := func() { http.DefaultTransport = origTransport }
	return r, cleanup
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHandleAgentChat_SystemPromptInjected(t *testing.T) {
	// 配置含 system_prompt → 应注入到转发请求的 messages[0]
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "sk-test",
			"openai_base_url": "https://api.test.com",
			"system_prompt": "你是文件管理助手。"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	s := &Server{configPath: cfgPath}
	var req recordedRequest
	r, cleanup := setupChatRouter(s, &req)
	defer cleanup()

	body := map[string]interface{}{
		"sessionId": "sess-1",
		"model":     "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "你好"},
		},
	}
	w := httptest.NewRecorder()
	reqJSON, _ := json.Marshal(body)
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/chat", strings.NewReader(string(reqJSON))))

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body = %s", w.Code, w.Body.String())
	}

	// 验证转发给 OpenAI 的 messages
	messages, ok := req.Body["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages 字段缺失或类型错误，body = %+v", req.Body)
	}
	if len(messages) < 2 {
		t.Fatalf("messages 长度 = %d，期望 >= 2（system + user），messages = %+v", len(messages), messages)
	}

	// messages[0] 必须是 system prompt
	sysMsg, ok := messages[0].(map[string]interface{})
	if !ok {
		t.Fatalf("messages[0] 不是 map，%T", messages[0])
	}
	if sysMsg["role"] != "system" {
		t.Errorf("messages[0].role = %q, want \"system\"", sysMsg["role"])
	}
	if sysMsg["content"] != "你是文件管理助手。" {
		t.Errorf("messages[0].content = %q, want \"你是文件管理助手。\"", sysMsg["content"])
	}

	// messages[1] 必须保持原始 user 消息
	userMsg := messages[1].(map[string]interface{})
	if userMsg["role"] != "user" || userMsg["content"] != "你好" {
		t.Errorf("messages[1] = %+v, want role=user content=你好", userMsg)
	}
}

func TestHandleAgentChat_NoSystemPrompt_WhenEmpty(t *testing.T) {
	// 配置无 system_prompt → 注入内置默认 prompt（强制 list_mounts + 禁止编造路径）
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "sk-test",
			"openai_base_url": "https://api.test.com"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	s := &Server{configPath: cfgPath}
	var req recordedRequest
	r, cleanup := setupChatRouter(s, &req)
	defer cleanup()

	body := map[string]interface{}{
		"sessionId": "sess-2",
		"model":     "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hi there"},
		},
	}
	w := httptest.NewRecorder()
	reqJSON, _ := json.Marshal(body)
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/chat", strings.NewReader(string(reqJSON))))

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body = %s", w.Code, w.Body.String())
	}

	messages, _ := req.Body["messages"].([]interface{})
	// 无配置时注入默认 system prompt → messages 长度 = 1(system) + 2(原始) = 3
	if len(messages) != 3 {
		t.Fatalf("无 system_prompt 时应注入默认 prompt，messages 长度应为 3，得到 %d", len(messages))
	}
	// 第一条必须是内置默认 system 消息
	first := messages[0].(map[string]interface{})
	if first["role"] != "system" {
		t.Errorf("messages[0].role = %q, want \"system\"（默认 prompt）", first["role"])
	}
	systemContent, _ := first["content"].(string)
	if !strings.Contains(systemContent, "list_mounts") {
		t.Error("默认 system prompt 应包含 list_mounts 指令")
	}
	// 后续消息保持原顺序
	second := messages[1].(map[string]interface{})
	if second["role"] != "user" {
		t.Errorf("messages[1].role = %q, want \"user\"", second["role"])
	}
}

func TestHandleAgentChat_MultiMessagesWithSystemPrompt(t *testing.T) {
	// 多轮对话 + system_prompt → system 在最前，其余顺序不变
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "sk-test",
			"openai_base_url": "https://api.test.com",
			"system_prompt": "只回复 JSON。"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	s := &Server{configPath: cfgPath}
	var req recordedRequest
	r, cleanup := setupChatRouter(s, &req)
	defer cleanup()

	body := map[string]interface{}{
		"sessionId": "sess-3",
		"messages": []map[string]string{
			{"role": "user", "content": "第一轮"},
			{"role": "assistant", "content": "回复1"},
			{"role": "user", "content": "第二轮"},
		},
	}
	w := httptest.NewRecorder()
	reqJSON, _ := json.Marshal(body)
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/chat", strings.NewReader(string(reqJSON))))

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body = %s", w.Code, w.Body.String())
	}

	messages, _ := req.Body["messages"].([]interface{})
	if len(messages) != 4 {
		t.Fatalf("期望 4 条消息（1 system + 3 原始），得到 %d", len(messages))
	}

	roles := make([]string, len(messages))
	for i, m := range messages {
		roles[i] = m.(map[string]interface{})["role"].(string)
	}
	expected := []string{"system", "user", "assistant", "user"}
	for i, got := range roles {
		if got != expected[i] {
			t.Errorf("messages[%d].role = %q, want %q", i, got, expected[i])
		}
	}

	// 验证 system 内容
	sysContent := messages[0].(map[string]interface{})["content"].(string)
	if sysContent != "只回复 JSON。" {
		t.Errorf("system content = %q", sysContent)
	}
}

func TestHandleAgentChat_NoAPIKey_Returns503(t *testing.T) {
	// 无 API Key → 503 错误
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "",
			"system_prompt": "test"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{configPath: cfgPath}
	s.registerAgentRoutes(r)

	body := `{"messages":[{"role":"user","content":"hi"}]}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/chat", strings.NewReader(body)))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("无 API Key 期望 503，得到 %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAgentChat_ToolsIncludedInRequest(t *testing.T) {
	// 验证 handleAgentChat 在转发请求时包含 tools 字段
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "sk-test-tools",
			"openai_base_url": "https://api.test.com"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	s := &Server{configPath: cfgPath}
	var req recordedRequest
	r, cleanup := setupChatRouter(s, &req)
	defer cleanup()

	body := map[string]interface{}{
		"sessionId": "sess-tools",
		"model":     "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "列出文件"},
		},
	}
	w := httptest.NewRecorder()
	reqJSON, _ := json.Marshal(body)
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/chat", strings.NewReader(string(reqJSON))))

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body = %s", w.Code, w.Body.String())
	}

	// 验证 tools 字段存在且非空
	tools, ok := req.Body["tools"].([]interface{})
	if !ok {
		t.Fatalf("请求体中缺少 tools 字段。完整 body keys: %+v", getMapKeys(req.Body))
	}
	if len(tools) == 0 {
		t.Fatal("tools 数组为空 — LLM 将无法调用任何函数")
	}

	// 验证包含核心工具
	toolNames := make([]string, len(tools))
	for i, toolIf := range tools {
		tool, ok := toolIf.(map[string]interface{})
		if !ok {
			t.Fatalf("tools[%d] 不是 map", i)
		}
		fn, ok := tool["function"].(map[string]interface{})
		if !ok {
			t.Fatalf("tools[%d].function 缺失", i)
		}
		name, _ := fn["name"].(string)
		toolNames[i] = name
	}

	hasListMounts := false
	for _, n := range toolNames {
		if n == "list_mounts" {
			hasListMounts = true
			break
		}
	}
	if !hasListMounts {
		t.Errorf("tools 中缺少 list_mounts。实际工具列表: %v", toolNames)
	}

	t.Logf("✅ 请求包含 %d 个工具: %v", len(tools), toolNames)
}

func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestHandleAgentChat_EmptyMessages_Returns400(t *testing.T) {
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "sk-test",
			"system_prompt": "test"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{configPath: cfgPath}
	s.registerAgentRoutes(r)

	body := `{"messages":[]}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/chat", strings.NewReader(body)))

	if w.Code != http.StatusBadRequest {
		t.Errorf("空消息期望 400，得到 %d: %s", w.Code, w.Body.String())
	}
}

// ─── POST /api/agent/reset-key 测试 ──────────────────────────

func TestHandleAgentResetKey_ClearsExistingKey(t *testing.T) {
	// 准备：config 里已有 openai_api_key（模拟 deviceId-bound 密文无法解开的场景）
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "enc:WnahXwgOG286btu3814/aw/wAlCTVENJqwQfnBJ1T7iPqg+43g/5IrAyaH7u/nw/l4yrnR2guhqEYK40XQF/uNKEO0j4CoAETjv94crMOvI=",
			"openai_base_url": "https://api.openai.com/v1",
			"openai_model": "gpt-4o"
		},
		"server": {"port": 2025}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{configPath: cfgPath}
	s.registerAgentRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/agent/reset-key", strings.NewReader("{}")))

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d: %s", w.Code, w.Body.String())
	}

	// 验证响应
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if reset, _ := resp["reset"].(bool); !reset {
		t.Errorf("期望 reset=true，得到 %v", resp["reset"])
	}
	if prevLen, _ := resp["prevLen"].(float64); prevLen < 50 {
		t.Errorf("期望 prevLen > 50（密文长度），得到 %v", resp["prevLen"])
	}

	// 关键：写回磁盘后 openai_api_key 字段应为空字符串
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("读回 config 失败: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析 config 失败: %v", err)
	}
	var agent map[string]interface{}
	if err := json.Unmarshal(raw["agent_settings"], &agent); err != nil {
		t.Fatalf("解析 agent_settings 失败: %v", err)
	}
	if key, _ := agent["openai_api_key"].(string); key != "" {
		t.Errorf("openai_api_key 应已被清空，得到 %q", key)
	}

	// 关键：其它字段保留
	if model, _ := agent["openai_model"].(string); model != "gpt-4o" {
		t.Errorf("openai_model 应保留，得到 %q", model)
	}
	if baseURL, _ := agent["openai_base_url"].(string); baseURL != "https://api.openai.com/v1" {
		t.Errorf("openai_base_url 应保留，得到 %q", baseURL)
	}
}

func TestHandleAgentResetKey_NoAgentSettings(t *testing.T) {
	// 没有 agent_settings 块 → no-op 返回 reset=false
	cfgJSON := `{"server": {"port": 2025}}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{configPath: cfgPath}
	s.registerAgentRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/agent/reset-key", strings.NewReader("{}")))

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if reset, _ := resp["reset"].(bool); reset {
		t.Errorf("没有 agent_settings 时期望 reset=false")
	}
}

func TestHandleAgentResetKey_AlreadyEmpty(t *testing.T) {
	// openai_api_key 已经是空 → reset=true, prevLen=0
	cfgJSON := `{
		"agent_settings": {
			"openai_api_key": "",
			"openai_model": "gpt-4o"
		}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{configPath: cfgPath}
	s.registerAgentRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/agent/reset-key", strings.NewReader("{}")))

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if prevLen, _ := resp["prevLen"].(float64); prevLen != 0 {
		t.Errorf("空值时期望 prevLen=0，得到 %v", resp["prevLen"])
	}
}

func TestHandleAgentResetKey_NoConfigPath(t *testing.T) {
	// configPath 为空时 → 404
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{configPath: ""}
	s.registerAgentRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/agent/reset-key", strings.NewReader("{}")))

	if w.Code != http.StatusNotFound {
		t.Errorf("configPath 为空时期望 404，得到 %d", w.Code)
	}
}

func TestHandleAgentResetKey_PreservesOtherFields(t *testing.T) {
	// 验证：top-level 其它字段不被破坏
	cfgJSON := `{
		"admin": {"password": "123456"},
		"agent_settings": {
			"openai_api_key": "enc:some-encrypted-blob-here",
			"openai_base_url": "https://api.openai.com/v1",
			"openai_model": "gpt-4o",
			"enabled_tools": ["list_files", "read_file"]
		},
		"server": {"port": 2025},
		"log": {"level": "info"}
	}`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.user.json")
	os.WriteFile(cfgPath, []byte(cfgJSON), 0644)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{configPath: cfgPath}
	s.registerAgentRoutes(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/agent/reset-key", strings.NewReader("{}")))

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d: %s", w.Code, w.Body.String())
	}

	// 验证：top-level 字段全部保留
	data, _ := os.ReadFile(cfgPath)
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	if _, ok := raw["admin"]; !ok {
		t.Errorf("admin 字段被破坏")
	}
	if _, ok := raw["server"]; !ok {
		t.Errorf("server 字段被破坏")
	}
	if _, ok := raw["log"]; !ok {
		t.Errorf("log 字段被破坏")
	}

	// 验证：agent_settings 内 enabled_tools 等其它字段保留
	var agent map[string]interface{}
	json.Unmarshal(raw["agent_settings"], &agent)
	tools, ok := agent["enabled_tools"].([]interface{})
	if !ok || len(tools) != 2 {
		t.Errorf("enabled_tools 应保留，得到 %v", agent["enabled_tools"])
	}
}

// ─── deriveKey 忽略 deviceId 测试 ────────────────────────────

func TestDeriveKey_IgnoresDeviceID(t *testing.T) {
	// 关键修复：deviceId 参数不再影响派生结果
	// 之前 deviceId-bound 设计导致设备变化 → 密文永久解不出
	key1 := deriveKey()
	key2 := deriveKey("")
	key3 := deriveKey("native:abc123")
	key4 := deriveKey("web:xyz789")

	if len(key1) != 32 {
		t.Errorf("派生 key 长度应 32，得到 %d", len(key1))
	}
	// 4 次派生应得到相同的 key（deviceId 全部被忽略）
	for i, k := range [][]byte{key1, key2, key3, key4} {
		for j, other := range [][]byte{key1, key2, key3, key4} {
			if i == j {
				continue
			}
			match := true
			for n := range k {
				if k[n] != other[n] {
					match = false
					break
				}
			}
			if !match {
				t.Errorf("deriveKey 应忽略 deviceId，但 key %d 与 key %d 不一致", i+1, j+1)
			}
		}
	}
}

func TestEncryptDecrypt_RoundTrip_IgnoresDeviceID(t *testing.T) {
	// 跨 deviceId 加密 → 跨 deviceId 解密，必须能 round-trip
	plaintext := "sk-test-round-trip-1234567890abcdef"
	enc1 := EncryptApiKey(plaintext, "deviceA")
	enc2 := EncryptApiKey(plaintext, "deviceB")

	// 解密时用任意 deviceId 都应能解出
	dec1 := DecryptApiKey(enc1, "deviceA")
	dec2 := DecryptApiKey(enc1, "deviceB")
	dec3 := DecryptApiKey(enc1, "")
	dec4 := DecryptApiKey(enc1, "native:abc")

	if dec1 != plaintext {
		t.Errorf("deviceA 解密失败: got %q", dec1)
	}
	if dec2 != plaintext {
		t.Errorf("deviceB 解密 enc1 失败: got %q", dec2)
	}
	if dec3 != plaintext {
		t.Errorf("空 deviceId 解密失败: got %q", dec3)
	}
	if dec4 != plaintext {
		t.Errorf("native deviceId 解密失败: got %q", dec4)
	}

	// 不同 deviceId 加密的密文也应能互相解开
	dec5 := DecryptApiKey(enc2, "deviceA")
	if dec5 != plaintext {
		t.Errorf("deviceA 解密 enc2 失败: got %q", dec5)
	}
}

// 防止未使用 import 报警
var _ = io.EOF

// ════════════════════════════════════════════════════════════
// 平台级 Tool Use 文本解析器测试
// ════════════════════════════════════════════════════════════

func TestExtractToolCallsFromText_FullJSONArray(t *testing.T) {
	// 策略 1: 整个文本就是 JSON 数组
	input := `[{"name":"list_mounts","arguments":{}}]`
	calls, remaining := extractToolCallsFromText(input)
	if len(calls) != 1 || calls[0].Name != "list_mounts" {
		t.Fatalf("expected 1 call(list_mounts), got %d calls: %+v", len(calls), calls)
	}
	if remaining != "" {
		t.Errorf("expected empty remaining, got %q", remaining)
	}
}

func TestExtractToolCallsFromText_MultipleCalls(t *testing.T) {
	input := `[{"name":"list_mounts","arguments":{}},{"name":"list_files","arguments":{"mount_id":"default","rel_path":"/"}}]`
	calls, remaining := extractToolCallsFromText(input)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "list_mounts" || calls[1].Name != "list_files" {
		t.Errorf("wrong names: %+v", calls)
	}
	if remaining != "" {
		t.Errorf("expected empty remaining, got %q", remaining)
	}
}

func TestExtractToolCallsFromText_WithArguments(t *testing.T) {
	input := `[{"name":"read_file","arguments":{"mount_id":"default","rel_path":"/test.txt"}}]`
	calls, _ := extractToolCallsFromText(input)
	if len(calls) != 1 || calls[0].Name != "read_file" {
		t.Fatalf("expected 1 call(read_file), got %+v", calls)
	}
	var args map[string]string
	json.Unmarshal(calls[0].Arguments, &args)
	if args["mount_id"] != "default" || args["rel_path"] != "/test.txt" {
		t.Errorf("wrong args: %+v", args)
	}
}

func TestExtractToolCallsFromText_PlainText_NoMatch(t *testing.T) {
	input := `你好，今天天气不错。有什么可以帮你的？`
	calls, remaining := extractToolCallsFromText(input)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
	if remaining != input {
		t.Errorf("remaining should be original text, got %q", remaining)
	}
}

func TestExtractToolCallsFromText_CodeBlock(t *testing.T) {
	// 策略 3: ```json 代码块
	input := "我来帮你查看文件列表：\n\n```json\n[{\"name\":\"list_mounts\",\"arguments\":{}}]\n```\n\n请稍等。"
	calls, remaining := extractToolCallsFromText(input)
	if len(calls) != 1 || calls[0].Name != "list_mounts" {
		t.Fatalf("expected 1 call(list_mounts), got %d: %+v", len(calls), calls)
	}
	if remaining == "" {
		t.Error("expected non-empty remaining text")
	}
}

func TestExtractToolCallsFromText_Empty(t *testing.T) {
	calls, remaining := extractToolCallsFromText("")
	if len(calls) != 0 || remaining != "" {
		t.Errorf("empty input should return nil, got calls=%d remaining=%q", len(calls), remaining)
	}
}

func TestFindJSONArrayEnd_Simple(t *testing.T) {
	s := `{"a":1}`
	idx := findJSONArrayEnd(s)
	if idx != -1 {
		t.Errorf("object should return -1, got %d", idx)
	}
}

func TestFindJSONArrayEnd_Nested(t *testing.T) {
	s := `[{"name":"test","arguments":{"key":"value[with_brackets]"}}]`
	idx := findJSONArrayEnd(s)
	if idx <= 0 {
		t.Errorf("expected positive index, got %d", idx)
	}
	// 验证截取后是合法 JSON
	var v []interface{}
	json.Unmarshal([]byte(s[:idx]), &v)
	if len(v) != 1 {
		t.Error("should parse as array with 1 element")
	}
}

func TestParsedToolCallsToAccumulator(t *testing.T) {
	calls := []parsedToolCall{
		{Name: "list_mounts", Arguments: json.RawMessage(`{}`)},
		{Name: "read_file", Arguments: json.RawMessage(`{"path":"/f"}`)},
	}
	accums := parsedToolCallsToAccumulator(calls)
	if len(accums) != 2 {
		t.Fatalf("expected 2 accumulators, got %d", len(accums))
	}
	if accums[0].Function.Name != "list_mounts" {
		t.Errorf("[0] wrong name: %s", accums[0].Function.Name)
	}
	if accums[1].Function.Name != "read_file" {
		t.Errorf("[1] wrong name: %s", accums[1].Function.Name)
	}
	if accums[1].Function.Arguments != `{"path":"/f"}` {
		t.Errorf("[1] wrong args: %s", accums[1].Function.Arguments)
	}
	// ID 应该以 ptc_ 开头
	if !strings.HasPrefix(accums[0].ID, "ptc_") {
		t.Errorf("[0] ID should start with ptc_: %s", accums[0].ID)
	}
}
