package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgentSettings is the JSON shape of the `agent_settings` block
// inside the user config. The shape is stable: it is what the
// encv-mobile front-end writes when the user saves the
// Settings → AI Assistant form, and what the agent-demo reads on
// startup.
//
// Field names use snake_case to match the Vue schema fields
// 1:1, so the agent loader and the schema-driven UI can agree
// without any explicit mapping.
type AgentSettings struct {
	OpenAIAPIKey  string   `json:"openai_api_key"`
	OpenAIBaseURL string   `json:"openai_base_url"`
	OpenAIModel   string   `json:"openai_model"`
	OpenListBaseURL string `json:"openlist_base_url"`
	OpenListToken string   `json:"openlist_token"`

	DefaultContainerVersion int      `json:"default_container_version"`
	EnabledTools            []string `json:"enabled_tools"`
	SystemPrompt            string   `json:"system_prompt"`
	MaxToolCallsPerTurn     int      `json:"max_tool_calls_per_turn"`

	// GlobalPassword is the fallback for plugins whose
	// PasswordStrategy == global. It is intentionally not
	// surfaced in the UI schema (the schema uses the same
	// top-level "password" field, which the loader maps onto
	// GlobalPassword for backwards compatibility).
	GlobalPassword string `json:"global_password"`

	// ─── v2 多轮/分支剧本（参考 .trae/specs/agent-tools-scenarios-v2/spec.md）───
	// ToolWhitelist command_run 工具的允许命令列表。
	// 默认值：ffprobe / ffmpeg / du / wc / find / stat / mediainfo / file。
	// 黑名单（与白名单叠加生效）：rm / mv / cp / chmod / chown / dd /
	//                              mkfs / shutdown / reboot。
	ToolWhitelist []string `json:"tool_whitelist,omitempty"`

	// SandboxPaths mount_id → 真实目录映射。command_run / search_files /
	// get_metadata 等需要访问物理文件系统的工具通过此映射把抽象 mount_id
	// 解析到主机绝对路径。
	SandboxPaths map[string]string `json:"sandbox_paths,omitempty"`

	// MockRoundTimeoutSec 多轮剧本中「等待用户回复」的最长秒数。
	// 范围 10-600；默认 60。
	MockRoundTimeoutSec int `json:"mock_round_timeout_sec,omitempty"`

	// MockRoundPauseEnabled 是否允许剧本在 mid-scenario 暂停等待用户输入。
	// false → 剧本忽略 pause_for_user 标记，一路跑完（自动机模式）。
	MockRoundPauseEnabled bool `json:"mock_round_pause_enabled,omitempty"`
}

// DefaultConfigPaths returns the ordered list of candidate
// config.user.json paths the loader will try, in priority order.
//
//  1. $ENV{CONFIG_USER_JSON} if set (escape hatch for CI /
//     sandboxed runs)
//  2. $HOME/.encv/config.user.json (the canonical user location)
//  3. /workspace/config.user.json (the dev fallback, used by
//     the agent-demo when running inside the encv-go repo)
//
// The list is small and stable; callers should iterate it in
// order and use the first existing file.
func DefaultConfigPaths() []string {
	out := []string{}
	if env := os.Getenv("CONFIG_USER_JSON"); env != "" {
		out = append(out, env)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, filepath.Join(home, ".encv", "config.user.json"))
	}
	// Always include the workspace dev fallback.
	out = append(out, "/workspace/config.user.json")
	return out
}

// LoadAgentSettings finds the first existing config file from
// DefaultConfigPaths and unmarshals its `agent_settings` block
// into an AgentSettings struct. Missing files are not an error:
// the loader walks the list and returns the first hit. If none
// of the candidates exist, a zero-value AgentSettings is
// returned together with a nil error — the caller can decide
// whether to fall back to environment variables or to refuse
// to start.
func LoadAgentSettings() (AgentSettings, error) {
	settings, path, err := LoadAgentSettingsFromFirstAvailable()
	if err != nil {
		return settings, err
	}
	if path == "" {
		// No config found; this is not an error but we report
		// it through the empty path so callers can log.
		return settings, nil
	}
	return settings, nil
}

// LoadAgentSettingsFromFirstAvailable is the explicit
// version of LoadAgentSettings that also returns the resolved
// path. Tests prefer this form to assert "loaded from this
// file".
func LoadAgentSettingsFromFirstAvailable() (AgentSettings, string, error) {
	for _, p := range DefaultConfigPaths() {
		if p == "" {
			continue
		}
		if _, statErr := os.Stat(p); statErr != nil {
			continue
		}
		settings, err := LoadAgentSettingsFromFile(p)
		if err != nil {
			return AgentSettings{}, p, err
		}
		return settings, p, nil
	}
	return AgentSettings{}, "", nil
}

// LoadAgentSettingsFromFile is the lowest-level entry point. It
// reads a single file, extracts the `agent_settings` block, and
// applies backwards-compatible defaults so a partial config still
// yields a usable AgentSettings.
//
// Behaviour:
//   - A file that does not parse as JSON → error.
//   - A file with no `agent_settings` block → zero-value settings,
//     nil error (caller may fall back to env).
//   - A file with `agent_settings` but missing fields → those
//     fields are zero values; this loader does not invent
//     defaults that the schema does not declare.
func LoadAgentSettingsFromFile(path string) (AgentSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentSettings{}, fmt.Errorf("read %s: %w", path, err)
	}
	// Parse the entire file as a generic map first so we can
	// look up `agent_settings` without committing to the full
	// config schema (which has many other top-level keys like
	// plugin_settings, server, admin, etc.).
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return AgentSettings{}, fmt.Errorf("parse %s as JSON: %w", path, err)
	}
	raw, ok := root["agent_settings"]
	if !ok {
		// No `agent_settings` block — but the pre-Phase-6
		// "password" field at the top level may still be
		// present, and we want to expose it as GlobalPassword.
		settings := AgentSettings{}
		if top, ok := root["password"]; ok {
			var pwd string
			if err := json.Unmarshal(top, &pwd); err == nil {
				settings.GlobalPassword = pwd
			}
		}
		return settings, nil
	}
	var settings AgentSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return AgentSettings{}, fmt.Errorf("parse agent_settings in %s: %w", path, err)
	}
	// 1. 工具白名单（默认包含本次新增的 high_level 跨平台工具命令）
	if len(settings.ToolWhitelist) == 0 {
		settings.ToolWhitelist = []string{
			"ffprobe", "ffmpeg", "du", "wc", "find", "stat", "mediainfo", "file",
			// high_level 跨平台工具使用的 coreutils + powershell
			"cat", "head", "tail", "grep", "env", "which", "ls", "powershell",
		}
	}
	// Always do backwards-compat: the top-level "password" field
	// is the pre-Phase-6 convention used by encv-go configs.
	// Apply it whenever GlobalPassword is empty, regardless of
	// whether `agent_settings` was present.
	if settings.GlobalPassword == "" {
		if top, ok := root["password"]; ok {
			var pwd string
			if err := json.Unmarshal(top, &pwd); err == nil {
				settings.GlobalPassword = pwd
			}
		}
	}
	return settings, nil
}

// ToAgentConfig converts AgentSettings into the AgentConfig that
// NewAgent consumes. The conversion is straightforward but
// centralised so the rest of the codebase does not need to know
// about AgentSettings.
//
// If EnabledTools is empty, the empty slice is preserved (the
// agent-demo interprets that as "no whitelist — register
// everything").
func (s AgentSettings) ToAgentConfig() AgentConfig {
	return AgentConfig{
		OpenAIAPIKey:           s.OpenAIAPIKey,
		OpenAIBaseURL:          s.OpenAIBaseURL,
		OpenAIModel:            s.OpenAIModel,
		OpenListBaseURL:        s.OpenListBaseURL,
		OpenListToken:          s.OpenListToken,
		DefaultContainerVersion: s.DefaultContainerVersion,
		EnabledTools:           s.EnabledTools,
		SystemPrompt:           s.SystemPrompt,
		MaxToolCallsPerTurn:    s.MaxToolCallsPerTurn,
		GlobalPassword:         s.GlobalPassword,
	}
}

// Validate performs sanity checks on the settings, returning an
// error suitable for surfacing to the user via the Settings UI.
//
// It is intentionally lenient: missing API keys are not
// errors, because the Settings UI may legitimately be opened
// before any key is configured. The error is reserved for
// structurally impossible values (e.g. negative version, empty
// model when one is set).
func (s AgentSettings) Validate() error {
	if s.DefaultContainerVersion < 0 {
		return fmt.Errorf("agent_settings: default_container_version must be >= 0, got %d", s.DefaultContainerVersion)
	}
	if s.MaxToolCallsPerTurn < 0 {
		return fmt.Errorf("agent_settings: max_tool_calls_per_turn must be >= 0, got %d", s.MaxToolCallsPerTurn)
	}
	if strings.TrimSpace(s.OpenAIModel) != "" && !isLikelyModelName(s.OpenAIModel) {
		// Not a hard error, but a useful signal for the UI.
		// We log via fmt.Errorf because the agent-demo prints
		// these to the console.
		return fmt.Errorf("agent_settings: openai_model %q has an unexpected shape", s.OpenAIModel)
	}
	return nil
}

// isLikelyModelName is a very loose heuristic: model names
// from OpenAI are short ASCII identifiers. We only flag
// obviously-wrong shapes (whitespace, leading slash, etc.).
func isLikelyModelName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if len(s) > 128 {
		return false
	}
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' {
			return false
		}
	}
	return true
}

// ResolveOpenAIToken returns a non-empty OpenAI API key, falling
// back to the $OPENAI_API_KEY environment variable. This is the
// recommended resolution order: the user-configured key takes
// precedence; an env-var override is a common dev / CI workflow.
func (s AgentSettings) ResolveOpenAIToken() string {
	if s.OpenAIAPIKey != "" {
		return s.OpenAIAPIKey
	}
	return os.Getenv("OPENAI_API_KEY")
}

// ResolveOpenListToken mirrors ResolveOpenAIToken for the
// OpenList side.
func (s AgentSettings) ResolveOpenListToken() string {
	if s.OpenListToken != "" {
		return s.OpenListToken
	}
	return os.Getenv("OPENLIST_TOKEN")
}

// ResolveOpenAIBaseURL returns the configured base URL or
// OpenAI's default.
func (s AgentSettings) ResolveOpenAIBaseURL() string {
	if s.OpenAIBaseURL != "" {
		return s.OpenAIBaseURL
	}
	if env := os.Getenv("OPENAI_BASE_URL"); env != "" {
		return env
	}
	return "https://api.openai.com/v1"
}

// ResolveOpenListBaseURL returns the configured base URL or
// localhost:5244 (the OpenList dev default).
func (s AgentSettings) ResolveOpenListBaseURL() string {
	if s.OpenListBaseURL != "" {
		return s.OpenListBaseURL
	}
	if env := os.Getenv("OPENLIST_BASE_URL"); env != "" {
		return env
	}
	return "http://127.0.0.1:5244"
}

// ResolveOpenAIModel returns the configured model or
// "gpt-4o".
func (s AgentSettings) ResolveOpenAIModel() string {
	if s.OpenAIModel != "" {
		return s.OpenAIModel
	}
	if env := os.Getenv("OPENAI_MODEL"); env != "" {
		return env
	}
	return "gpt-4o"
}
