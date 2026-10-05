package server

// agent_profile_round1_test.go —— vNext Round 1 回归锁
//
// 锁定两件事：
//  1. readAgentConfig 不再因 agent_settings 里的非字符串字段（mock_speed /
//     enabled_tools / max_tool_calls_per_turn…）而静默丢掉 API Key。
//  2. production profile 下 mock 恒为 off：不得短路剧本、不得暴露 presets、
//     不得绑定外置剧本目录。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/gin-gonic/gin"
)

func writeRound1Config(t *testing.T, agentSettings string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.user.json")
	if err := os.WriteFile(p, []byte(`{"agent_settings":`+agentSettings+`}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestRuntimeProfileFromEnv(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"test", ProfileTest},
		{"TEST", ProfileTest},
		{"dev", ProfileTest},
		{"", ProfileProduction},
		{"weird", ProfileProduction},
		{"production", ProfileProduction},
	}
	for _, c := range cases {
		t.Setenv("ENCV_RUNTIME_PROFILE", c.env)
		if got := runtimeProfileFromEnv(); got != c.want {
			t.Errorf("env=%q: got %q, want %q", c.env, got, c.want)
		}
	}
}

func TestRuntimeProfile_ZeroValueIsProduction(t *testing.T) {
	s := &Server{}
	if s.RuntimeProfile() != ProfileProduction {
		t.Fatalf("zero-value profile = %q, want production (fail closed)", s.RuntimeProfile())
	}
	if s.mockEnabled() {
		t.Fatal("zero-value server must not enable mock")
	}
}

func TestEffectiveMockMode(t *testing.T) {
	prod := &Server{runtimeProfile: ProfileProduction}
	if got := prod.effectiveMockMode(&config.Agent{MockMode: "builtin"}); got != "off" {
		t.Fatalf("production + builtin: got %q, want off", got)
	}
	if got := prod.effectiveMockMode(&config.Agent{MockMode: "custom"}); got != "off" {
		t.Fatalf("production + custom: got %q, want off", got)
	}
	testp := &Server{runtimeProfile: ProfileTest}
	if got := testp.effectiveMockMode(&config.Agent{MockMode: "builtin"}); got != "builtin" {
		t.Fatalf("test + builtin: got %q, want builtin", got)
	}
	if got := testp.effectiveMockMode(&config.Agent{}); got != "off" {
		t.Fatalf("test + empty: got %q, want off", got)
	}
}

// TestReadAgentConfig_ToleratesNonStringFields 是本次修复的核心回归锁：
// 旧实现用 map[string]string 解析 agent_settings，任何数字/数组字段都会让整段
// 解析失败 ⇒ API Key 静默为空 ⇒ handleAgentChat 直接 503 "no_api_key"。
func TestReadAgentConfig_ToleratesNonStringFields(t *testing.T) {
	p := writeRound1Config(t, `{
		"openai_api_key":"sk-plain-round1",
		"openai_base_url":"https://api.openai.com",
		"openai_model":"gpt-4o",
		"mock_speed":10.0,
		"enabled_tools":["list_files","read_file"],
		"max_tool_calls_per_turn":5,
		"default_container_version":4
	}`)
	s := &Server{configPath: p}
	cfg := s.readAgentConfig("device-round1")
	if cfg.APIKey == "" {
		t.Fatal("APIKey must not be empty: non-string agent_settings fields must not kill parsing")
	}
	if cfg.OpenAIModel != "gpt-4o" {
		t.Fatalf("OpenAIModel = %q, want gpt-4o", cfg.OpenAIModel)
	}
	if cfg.BaseURL != "https://api.openai.com" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
}

func TestReadAgentConfig_MissingSectionIsNotError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.user.json")
	if err := os.WriteFile(p, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{configPath: p}
	cfg := s.readAgentConfig("d")
	if cfg.APIKey != "" {
		t.Fatalf("missing agent_settings should yield empty key, got %q", cfg.APIKey)
	}
}

func TestHandleAgentMockPresets_DisabledInProduction(t *testing.T) {
	p := writeRound1Config(t, `{"mock_mode":"builtin"}`)
	s := &Server{configPath: p, runtimeProfile: ProfileProduction}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/agent/mock/presets", s.handleAgentMockPresets)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/agent/mock/presets", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got struct {
		Presets  []map[string]interface{} `json:"presets"`
		MockMode string                   `json:"mockMode"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	if len(got.Presets) != 0 {
		t.Fatalf("production must not expose mock presets, got %d", len(got.Presets))
	}
	if got.MockMode != "off" {
		t.Fatalf("mockMode = %q, want off", got.MockMode)
	}
}

func TestLoadScenariosFromAgentConfig_SkippedInProduction(t *testing.T) {
	p := writeRound1Config(t, `{"mock_scenarios_dir":"/tmp/round1-scenarios","mock_mode":"builtin"}`)
	s := &Server{configPath: p, runtimeProfile: ProfileProduction}
	s.loadScenariosFromAgentConfig()
	if s.scenariosDir != "" {
		t.Fatalf("production must not bind scenarios dir, got %q", s.scenariosDir)
	}
	if s.scenarioLoader != nil {
		t.Fatal("production must not create scenario loader")
	}
}
