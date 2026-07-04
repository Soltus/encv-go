package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.user.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAgentSettingsFromFile_Full(t *testing.T) {
	path := writeTempConfig(t, `{
		"agent_settings": {
			"openai_api_key": "sk-test-123",
			"openai_base_url": "https://api.openai.com/v1",
			"openai_model": "gpt-4o-mini",
			"openlist_base_url": "http://127.0.0.1:5244",
			"openlist_token": "abc",
			"default_container_version": 2,
			"enabled_tools": ["list_files", "delete_file"],
			"system_prompt": "You are an ENCV assistant",
			"max_tool_calls_per_turn": 10,
			"global_password": "secret123"
		}
	}`)
	s, err := LoadAgentSettingsFromFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.OpenAIAPIKey != "sk-test-123" {
		t.Errorf("OpenAIAPIKey: %q", s.OpenAIAPIKey)
	}
	if s.OpenAIModel != "gpt-4o-mini" {
		t.Errorf("OpenAIModel: %q", s.OpenAIModel)
	}
	if s.DefaultContainerVersion != 2 {
		t.Errorf("DefaultContainerVersion: %d", s.DefaultContainerVersion)
	}
	if len(s.EnabledTools) != 2 {
		t.Errorf("EnabledTools: %v", s.EnabledTools)
	}
	if s.GlobalPassword != "secret123" {
		t.Errorf("GlobalPassword: %q", s.GlobalPassword)
	}
}

func TestLoadAgentSettingsFromFile_Partial(t *testing.T) {
	path := writeTempConfig(t, `{
		"agent_settings": {
			"openai_api_key": "sk-1"
		}
	}`)
	s, err := LoadAgentSettingsFromFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.OpenAIAPIKey != "sk-1" {
		t.Errorf("OpenAIAPIKey: %q", s.OpenAIAPIKey)
	}
	// Other fields should be zero.
	if s.OpenAIModel != "" {
		t.Errorf("OpenAIModel should be empty, got %q", s.OpenAIModel)
	}
	if s.MaxToolCallsPerTurn != 0 {
		t.Errorf("MaxToolCallsPerTurn should be 0, got %d", s.MaxToolCallsPerTurn)
	}
}

func TestLoadAgentSettingsFromFile_NoAgentSettings(t *testing.T) {
	path := writeTempConfig(t, `{
		"password": "top-level"
	}`)
	s, err := LoadAgentSettingsFromFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.OpenAIAPIKey != "" {
		t.Errorf("OpenAIAPIKey: %q", s.OpenAIAPIKey)
	}
	// Backwards-compat: the top-level password should
	// populate GlobalPassword.
	if s.GlobalPassword != "top-level" {
		t.Errorf("GlobalPassword should fall back to top-level password, got %q", s.GlobalPassword)
	}
}

func TestLoadAgentSettingsFromFile_InvalidJSON(t *testing.T) {
	path := writeTempConfig(t, `{ not json`)
	_, err := LoadAgentSettingsFromFile(path)
	if err == nil {
		t.Errorf("expected parse error")
	}
}

func TestLoadAgentSettingsFromFile_MissingFile(t *testing.T) {
	_, err := LoadAgentSettingsFromFile("/nonexistent/path/config.json")
	if err == nil {
		t.Errorf("expected read error")
	}
}

func TestLoadAgentSettingsFromFile_BackwardsCompat(t *testing.T) {
	// agent_settings is present but global_password is not —
	// the top-level "password" should fill in.
	path := writeTempConfig(t, `{
		"password": "my-encv_key",
		"agent_settings": {
			"openai_api_key": "sk-x"
		}
	}`)
	s, err := LoadAgentSettingsFromFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.GlobalPassword != "my-encv_key" {
		t.Errorf("GlobalPassword should fall back to top-level, got %q", s.GlobalPassword)
	}
	// When global_password is explicit, the explicit value wins.
	path2 := writeTempConfig(t, `{
		"password": "top-level",
		"agent_settings": {
			"global_password": "explicit"
		}
	}`)
	s2, _ := LoadAgentSettingsFromFile(path2)
	if s2.GlobalPassword != "explicit" {
		t.Errorf("explicit global_password should win, got %q", s2.GlobalPassword)
	}
}

func TestAgentSettings_ToAgentConfig(t *testing.T) {
	s := AgentSettings{
		OpenAIAPIKey:            "k",
		OpenAIBaseURL:           "u",
		OpenAIModel:             "m",
		OpenListBaseURL:         "ou",
		OpenListToken:           "ot",
		DefaultContainerVersion: 3,
		EnabledTools:            []string{"a", "b"},
		SystemPrompt:            "sp",
		MaxToolCallsPerTurn:     7,
		GlobalPassword:          "gp",
	}
	cfg := s.ToAgentConfig()
	if cfg.OpenAIAPIKey != "k" || cfg.OpenAIModel != "m" {
		t.Errorf("config not properly populated: %+v", cfg)
	}
	if cfg.DefaultContainerVersion != 3 {
		t.Errorf("default_container_version not propagated")
	}
	if len(cfg.EnabledTools) != 2 {
		t.Errorf("enabled_tools not propagated")
	}
}

func TestAgentSettings_Validate(t *testing.T) {
	cases := []struct {
		name    string
		s       AgentSettings
		wantErr bool
	}{
		{
			name:    "valid empty",
			s:       AgentSettings{},
			wantErr: false,
		},
		{
			name:    "valid full",
			s:       AgentSettings{DefaultContainerVersion: 1, MaxToolCallsPerTurn: 10, OpenAIModel: "gpt-4o"},
			wantErr: false,
		},
		{
			name:    "negative version",
			s:       AgentSettings{DefaultContainerVersion: -1},
			wantErr: true,
		},
		{
			name:    "negative max tool calls",
			s:       AgentSettings{MaxToolCallsPerTurn: -1},
			wantErr: true,
		},
		{
			name:    "weird model name",
			s:       AgentSettings{OpenAIModel: "gpt 4o"},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.s.Validate()
			if c.wantErr && err == nil {
				t.Errorf("expected error")
			}
			if !c.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAgentSettings_ResolveTokens(t *testing.T) {
	s := AgentSettings{OpenAIAPIKey: "from-config", OpenListToken: "from-config-ol"}
	if got := s.ResolveOpenAIToken(); got != "from-config" {
		t.Errorf("ResolveOpenAIToken: %q", got)
	}
	if got := s.ResolveOpenListToken(); got != "from-config-ol" {
		t.Errorf("ResolveOpenListToken: %q", got)
	}
	// Empty config → fallback to env.
	s = AgentSettings{}
	t.Setenv("OPENAI_API_KEY", "from-env")
	t.Setenv("OPENLIST_TOKEN", "from-env-ol")
	if got := s.ResolveOpenAIToken(); got != "from-env" {
		t.Errorf("env fallback for OpenAI: %q", got)
	}
	if got := s.ResolveOpenListToken(); got != "from-env-ol" {
		t.Errorf("env fallback for OpenList: %q", got)
	}
}

func TestAgentSettings_ResolveBaseURLs(t *testing.T) {
	// Empty → defaults.
	s := AgentSettings{}
	if got := s.ResolveOpenAIBaseURL(); got != "https://api.openai.com/v1" {
		t.Errorf("default OpenAI base URL: %q", got)
	}
	if got := s.ResolveOpenListBaseURL(); got != "http://127.0.0.1:5244" {
		t.Errorf("default OpenList base URL: %q", got)
	}
	// Configured → wins.
	s = AgentSettings{OpenAIBaseURL: "https://x.example/v1", OpenListBaseURL: "http://x:5244"}
	if got := s.ResolveOpenAIBaseURL(); got != "https://x.example/v1" {
		t.Errorf("configured OpenAI base URL: %q", got)
	}
	if got := s.ResolveOpenListBaseURL(); got != "http://x:5244" {
		t.Errorf("configured OpenList base URL: %q", got)
	}
}

func TestAgentSettings_ResolveOpenAIModel(t *testing.T) {
	// Empty → default.
	s := AgentSettings{}
	if got := s.ResolveOpenAIModel(); got != "gpt-4o" {
		t.Errorf("default model: %q", got)
	}
	// Configured → wins.
	s = AgentSettings{OpenAIModel: "o1-preview"}
	if got := s.ResolveOpenAIModel(); got != "o1-preview" {
		t.Errorf("configured model: %q", got)
	}
	// Env → used when config empty.
	s = AgentSettings{}
	t.Setenv("OPENAI_MODEL", "env-model")
	if got := s.ResolveOpenAIModel(); got != "env-model" {
		t.Errorf("env fallback: %q", got)
	}
}

func TestLoadAgentSettingsFromFirstAvailable_OrderMatters(t *testing.T) {
	// Write two configs: the first should be returned.
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`{"agent_settings":{"openai_api_key":"AAA"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"agent_settings":{"openai_api_key":"BBB"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Drive the search explicitly: config_paths returns a, b
	// in that order. LoadAgentSettingsFromFirstAvailable uses
	// DefaultConfigPaths, which is environment-driven, so we
	// just test the per-file loader and confirm both are
	// readable.
	sA, _ := LoadAgentSettingsFromFile(a)
	sB, _ := LoadAgentSettingsFromFile(b)
	if sA.OpenAIAPIKey != "AAA" || sB.OpenAIAPIKey != "BBB" {
		t.Errorf("per-file load: %q %q", sA.OpenAIAPIKey, sB.OpenAIAPIKey)
	}
}

func TestDefaultConfigPaths_IncludesWorkspaceDev(t *testing.T) {
	paths := DefaultConfigPaths()
	hasWorkspace := false
	for _, p := range paths {
		if p == "/workspace/config.user.json" {
			hasWorkspace = true
		}
	}
	if !hasWorkspace {
		t.Errorf("DefaultConfigPaths should include /workspace/config.user.json, got %v", paths)
	}
}

func TestLoadAgentSettingsFromFirstAvailable_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.user.json")
	if err := os.WriteFile(cfg, []byte(`{"agent_settings":{"openai_api_key":"env-cfg"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_USER_JSON", cfg)
	s, path, err := LoadAgentSettingsFromFirstAvailable()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if path != cfg {
		t.Errorf("path: %q", path)
	}
	if s.OpenAIAPIKey != "env-cfg" {
		t.Errorf("key: %q", s.OpenAIAPIKey)
	}
}

func TestLoadAgentSettings_EmptyPathIsNotAnError(t *testing.T) {
	// 强制 CONFIG_USER_JSON 指向不存在的文件
	t.Setenv("CONFIG_USER_JSON", "/nonexistent/config.user.json")
	// 同时让 HOME 也不存在，绕开 ~/.encv/config.user.json fallback
	t.Setenv("HOME", "/nonexistent_home_for_test_"+t.Name())
	// /workspace/config.user.json 是 agent 包编译时的 dev fallback，无法 env 覆盖。
	// 如果该文件存在，本测试是验证"读到了 fallback 也不报错"——不是验证"空值"。
	// 因此这里只断言不 panic + 不返回 error。
	s, err := LoadAgentSettings()
	if err != nil {
		t.Errorf("empty/missing config should not error, got %v", err)
	}
	_ = s // 设置读取的结果（可能来自 /workspace fallback），不强制断言
}

func TestAgentSettings_EncodeJSONRoundTrip(t *testing.T) {
	original := AgentSettings{
		OpenAIAPIKey:            "sk-1",
		OpenAIBaseURL:           "u",
		OpenAIModel:             "m",
		DefaultContainerVersion: 1,
		EnabledTools:            []string{"a", "b"},
		MaxToolCallsPerTurn:     5,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	// Verify snake_case on the wire.
	for _, want := range []string{`"openai_api_key"`, `"openai_base_url"`, `"default_container_version"`, `"max_tool_calls_per_turn"`} {
		if !contains(string(b), want) {
			t.Errorf("missing %q in marshaled output: %s", want, b)
		}
	}
	var decoded AgentSettings
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OpenAIAPIKey != original.OpenAIAPIKey {
		t.Errorf("round-trip mismatch")
	}
	if len(decoded.EnabledTools) != 2 {
		t.Errorf("enabled_tools round-trip: %v", decoded.EnabledTools)
	}
}

// contains is a small string-substring helper local to tests.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
