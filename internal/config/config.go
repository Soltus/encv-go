package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// Config 是应用程序的顶层配置结构，包含所有子模块的配置。
type Config struct {
	// --- 全局设置 ---
	// Password 用于加密和解密视频文件，请务必设置一个强密码。
	Password string `json:"password"`
	// Recover 在解密时是否尝试覆盖已有文件。
	Recover bool `json:"recover"`
	// DefaultContainerVersion 默认容器版本（2=已弃用, 3=稳定, 4=推荐）
	DefaultContainerVersion int `json:"default_container_version"`
	// StrictDeprecatedVersion 是否严格禁止使用已弃用版本创建容器
	StrictDeprecatedVersion bool `json:"strict_deprecated_version"`

	// --- v4 容器能力 ---
	// V4CipherMode v4 容器加密算法：0=AES-128-CTR（默认，推荐），1=AES-256-CTR（可选，更高强度）
	V4CipherMode int `json:"v4_cipher_mode"`
	// V4CompressionMode v4 容器压缩算法：none=不压缩（默认），zstd=zstd seekable 压缩
	V4CompressionMode string `json:"v4_compression_mode"`
	// V4EnableHMAC v4 容器是否启用 HMAC-SHA1-80 完整性校验（防 CTR 比特翻转攻击）
	V4EnableHMAC bool `json:"v4_enable_hmac"`
	// V4ZstdBlockSize zstd seekable 压缩块大小（字节），范围 1024-1048576，默认 65536
	V4ZstdBlockSize int `json:"v4_zstd_block_size"`

	// --- 加密/解密设置 ---
	// OutputPath 加密后的文件输出目录。
	OutputPath string `json:"output_path"`
	// TrackExtensions 视频容器的字幕/轨道文件扩展名列表，它们并不会打包到容器里。
	// TrackExtensions []string `json:"track_extensions"`
	// BinExtGroup 可以自定义加密容器文件的扩展名。已弃用，使用 PluginSettings. 替代。
	// BinExtGroup types.BinExtGroup `json:"bin_ext_group"`
	//  map 键是插件名，值是该插件的原始JSON配置
	PluginSettings map[string]json.RawMessage `json:"plugin_settings"`
	// 【关键新增】Provider 是一个可选的动态配置提供者
	// 如果 Provider 不为 nil，它将优先于 PluginSettings 被使用
	Provider ConfigProvider            `json:"-"` // 使用 `json:"-"` 确保它不会被序列化到文件
	Server   types.HttpServer          `json:"server"`
	Admin    types.AdminServer         `json:"admin"`
	Webdav   types.WebdavServer        `json:"webdav"`
	Proxy    types.OpenlistProxyServer `json:"proxy"`
	// --- 日志设置 ---
	// Log 配置结构化日志的输出级别和文件路径。
	Log types.LogConfig `json:"log"`
	// --- 数据库设置 ---
	// Database 数据库存储引擎配置。
	Database DatabaseConfig `json:"database"`
	// --- 预览设置 ---
	Preview *PreviewConfig      `json:"preview,omitempty"`
	Mobile  *types.MobileConfig `json:"mobile,omitempty"`
	// AgentSettings AI agent 配置（openai_api_key, model 等）
	// 使用 json.RawMessage 透传，避免定义详细子结构
	AgentSettings json.RawMessage `json:"agent_settings,omitempty"`
}

// Agent 是 agent_settings 段的类型化表示。
// 与 Config.AgentSettings json.RawMessage 并存使用：raw 通道保留向后兼容
// 的 wire 格式，类型化 Agent 供 server / agent_mock 等运行时使用。
type Agent struct {
	// --- OpenAI 侧 ---
	OpenAIAPIKey  string `json:"openai_api_key,omitempty"`
	OpenAIBaseURL string `json:"openai_base_url,omitempty"`
	OpenAIModel   string `json:"openai_model,omitempty"`

	// --- OpenList 侧 ---
	OpenListBaseURL string `json:"openlist_base_url,omitempty"`
	OpenListToken   string `json:"openlist_token,omitempty"`

	// --- 行为配置 ---
	Temperature            float64  `json:"temperature,omitempty"`
	EnabledTools           []string `json:"enabled_tools,omitempty"`
	SystemPrompt           string   `json:"system_prompt,omitempty"`
	MaxToolCallsPerTurn    int      `json:"max_tool_calls_per_turn,omitempty"`
	DefaultContainerVersion int     `json:"default_container_version,omitempty"`
	GlobalPassword         string   `json:"global_password,omitempty"`

	// ─── Mock 模式（参考 .trae/specs/agent-mock-mode/spec.md）───
	// MockMode 控制 agent 行为：
	//   "off"     — 默认，调用真实 OpenAI/gptgod
	//   "builtin" — 启用内置 12 个 mock 剧本（0 token 消耗）
	//   "custom"  — 仅使用 Agent.MockScenarios 中定义的自定义剧本
	MockMode string `json:"mock_mode,omitempty"`

	// MockSpeed 时间缩放因子：1.0=正常, 0.1=10x 慢放, 10=10x 快进, 0=零延迟同步
	MockSpeed float64 `json:"mock_speed,omitempty"`

	// MockScenarios 自定义剧本（仅 MockMode=="custom" 时使用）
	MockScenarios []MockScenario `json:"mock_scenarios,omitempty"`

	// ─── v2 多轮/分支剧本（参考 .trae/specs/agent-tools-scenarios-v2/spec.md）───
	// ToolWhitelist command_run 工具的允许命令列表。
	// 默认值（DefaultAgentConfig 注入）：ffprobe / ffmpeg / du / wc / find /
	//                                    stat / mediainfo / file。
	// 黑名单（与白名单叠加生效）：rm / mv / cp / chmod / chown / dd /
	//                              mkfs / shutdown / reboot。
	ToolWhitelist []string `json:"tool_whitelist,omitempty"`

	// SandboxPaths mount_id → 真实目录映射。command_run / search_files /
	// get_metadata 等需要访问物理文件系统的工具通过此映射把抽象 mount_id
	// 解析到主机绝对路径；空 map 时工具只能看到 mount 元信息。
	SandboxPaths map[string]string `json:"sandbox_paths,omitempty"`

	// MockRoundTimeoutSec 多轮剧本中「等待用户回复」的最长秒数。
	// 超时后由后端自动推 stream_end {finishReason: "timeout"}。
	// 范围 10-600；默认 60。
	MockRoundTimeoutSec int `json:"mock_round_timeout_sec,omitempty"`

	// MockRoundPauseEnabled 是否允许剧本在 mid-scenario 暂停等待用户输入。
	// false → 剧本忽略 pause_for_user 标记，一路跑完（自动机模式）。
	MockRoundPauseEnabled bool `json:"mock_round_pause_enabled,omitempty"`

	// MockScenariosDir 剧本外置 spec：YAML/JSON 剧本所在目录。
	// 设置后，Server 启动时扫描该目录下的 *.yaml / *.json 文件，
	// 校验 + 加载，注入到 MockEngine。空字符串 = 走 Go 字面量 fallback。
	// 详见 internal/server/mock_scenarios/SCHEMA.md。
	MockScenariosDir string `json:"mock_scenarios_dir,omitempty"`

	// MockScenariosHotReload 是否启用 fsnotify 热重载。
	// true → 检测到目录内 *.yaml / *.json 变更时自动 reload（500ms 防抖）。
	// 仅在 MockScenariosDir 非空时生效。
	MockScenariosHotReload bool `json:"mock_scenarios_hot_reload,omitempty"`
}

// DatabaseConfig 数据库存储引擎配置。
//
// 架构（2026-07-03 改造）：
//   - SQLite 是默认底座（base engine），始终启用，不可关闭
//   - 其他引擎（libsql / turso / objectbox）作为可选服务，独立启用/禁用
//   - Engine 字段保留向后兼容（旧配置的主存储引擎选择）
//   - 各可选引擎的启用开关在 EnableEngines map 中
type DatabaseConfig struct {
	// Engine 主存储引擎类型："sqlite" | "turso" | "libsql" | "objectbox"
	// 默认 "sqlite"（向后兼容）
	Engine string `json:"engine"`
	// Path 数据库文件路径（仅 sqlite/turso 本地模式使用）
	// 留空则使用默认路径：{data_dir}/encv.db
	Path string `json:"path,omitempty"`
	// TursoSyncURL Turso 同步 URL（仅 turso 模式使用）
	// 例如 "libsql://your-db.turso.io"
	TursoSyncURL string `json:"turso_sync_url,omitempty"`
	// TursoAuthToken Turso 认证令牌（仅 turso 模式使用）
	TursoAuthToken string `json:"turso_auth_token,omitempty"`
	// EnableEngines 可选引擎的启用开关（除 sqlite 外的引擎）
	// key = 引擎名（"libsql" | "turso" | "objectbox"）
	// value = 是否启用
	EnableEngines map[string]bool `json:"enable_engines,omitempty"`
}

// MockScenario — 自定义 mock 剧本配置项（与 internal/server/agent_mock.go 中的同名类型语义一致）
// 这里只保留 JSON 序列化所需的字段；运行时类型转换由 server 包处理
type MockScenario struct {
	ID          string     `json:"id"`
	Description string     `json:"description,omitempty"`
	ExactMatch  string     `json:"exact_match,omitempty"`
	Keywords    []string   `json:"keywords,omitempty"`
	Regex       string     `json:"regex,omitempty"`
	Steps       []MockStep `json:"steps"`
}

type MockStep struct {
	DelayMs int                     `json:"delay_ms"`
	Events  []map[string]interface{} `json:"events"`
}

// DefaultAgentConfig 返回一个包含所有 agent 默认值的配置实例。
// 同时为 mock 相关字段补充防御性默认值（即便结构体已设置零值）。
func DefaultAgentConfig() *Agent {
	a := &Agent{
		OpenAIModel:            "gpt-4o-mini",
		Temperature:            0.7,
		MaxToolCallsPerTurn:    5,
		DefaultContainerVersion: 4,
	}
	if a.MockMode == "" {
		a.MockMode = "off"
	}
	if a.MockSpeed == 0 {
		a.MockSpeed = 1.0
	}
	// v2 defaults（参考 spec §ToolWhitelist / §MockRoundTimeoutSec）
	if len(a.ToolWhitelist) == 0 {
		a.ToolWhitelist = []string{
			"ffprobe", "ffmpeg", "du", "wc", "find", "stat", "mediainfo", "file",
		}
	}
	if a.MockRoundTimeoutSec == 0 {
		a.MockRoundTimeoutSec = 60
	}
	// 校验范围：10-600
	if a.MockRoundTimeoutSec < 10 {
		a.MockRoundTimeoutSec = 10
	}
	if a.MockRoundTimeoutSec > 600 {
		a.MockRoundTimeoutSec = 600
	}
	// MockRoundPauseEnabled bool 零值=false；默认值是 true，所以不能用零值判断。
	// 用 omitempty 序列化时如果用户没设置就保持 true；如果显式设为 false 也保留。
	// 实际后端启动时通过 cfg.Agent.MockRoundPauseEnabled 读取，没有"未设置"概念。
	if !a.MockRoundPauseEnabled {
		// 首次初始化：把 true 注入（用户保存过的 false 不受影响）
		// —— 但因为 Agent 在 Load 时是空值，这里只能依赖调用方显式注入。
		// 防御性补刀：loadAndMerge 阶段如果没有任何 agent_settings 段，
		// Agent 整体走 DefaultAgentConfig，此时把 PauseEnabled 设回 true。
		a.MockRoundPauseEnabled = true
	}
	return a
}

type PreviewConfig struct {
	TextExtensions []string `json:"text_extensions,omitempty"`
}

// ConfigProvider 定义了获取插件配置的抽象接口
// 任何实现了此接口的结构体都可以为 ENCV 插件提供配置
type ConfigProvider interface {
	// GetPluginSettings 根据插件名称获取其原始的 JSON 配置
	GetPluginSettings(pluginName string) (json.RawMessage, error)
}

// contextKey 是一个不导出的类型，用于防止 context 中的 key 冲突。
// 这是一个在 Go 中使用 context.WithValue 的标准做法。
type contextKey string

// configKey 是我们用来存储配置的私有 key。
const configKey = contextKey("encv-go-config")

// NewContext 将配置对象存入一个新的 context 中，并返回这个新 context。
// parent: 父 context，通常是 context.Background() 或请求的 context
// cfg: 要存储的配置对象
func NewContext(parent context.Context, cfg *Config) context.Context {
	return context.WithValue(parent, configKey, cfg)
}

// FromContext 从 context 中提取配置对象。
// 如果 context 中没有存储配置，它会返回 nil。
func FromContext(ctx context.Context) *Config {
	if cfg, ok := ctx.Value(configKey).(*Config); ok {
		return cfg
	}
	return nil
}

// DefaultConfig 返回一个包含所有默认值的配置实例。
func DefaultConfig() *Config {
	return &Config{
		OutputPath:              "./encrypted",
		DefaultContainerVersion: 4,
		V4CipherMode:            0,           // 默认 AES-128-CTR
		V4CompressionMode:       "none",      // 默认不压缩
		V4EnableHMAC:            true,        // 默认启用 HMAC-SHA1-80
		V4ZstdBlockSize:         65536,       // 默认 64KB zstd 块
		Server:                  types.HttpServer{Port: 1999, Dir: "./"},
		Webdav: types.WebdavServer{
			Root: "",
			Dir:  "",
		},
		Proxy: types.OpenlistProxyServer{
			DisableSignatureVerification: false,
		},
		Log: types.LogConfig{
			Level: "info",
			File:  "",
		},
		Database: DatabaseConfig{
			Engine: "sqlite", // 默认 SQLite，持久化可靠
		},
	}
}

func (c *Config) GetEffectiveDefaultVersion() int {
	if c.DefaultContainerVersion > 0 && types.IsValidVersion(c.DefaultContainerVersion) {
		return c.DefaultContainerVersion
	}
	return types.DefaultContainerVersion
}

func (c *Config) IsStrictMode() bool {
	return c.StrictDeprecatedVersion
}

// Load 加载配置。优先级（低→高）：
//
//	DefaultConfig() → config.user.json → config.dev.json（dev 最高优先级）
//
// 显式指定路径时走单文件模式（向后兼容）
func Load(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath != "" {
		return loadSingleFile(cfg, configPath)
	}

	candidates := findMergeCandidates()
	if candidates == nil {
		slog.Info("No config files found, using default settings")
		return finalize(cfg), nil
	}

	if candidates.User != "" {
		cfg = loadAndMerge(cfg, candidates.User)
	}
	if candidates.Dev != "" {
		cfg = loadAndMerge(cfg, candidates.Dev)
	}

	return finalize(cfg), nil
}

// ApplyMobileOverlay 将 mobile 配置段作为运行时 overlay 应用到顶层字段。
// 这是唯一的 mobile→顶层 映射入口，不修改持久化的配置文件。
//
// 触发条件（满足任一即生效）：
//   - 环境变量 ENCV_MOBILE=1（Android 真机，由 EncvGoService.kt 设置）
//   - 环境变量 ENCV_DEV_PREVIEW=1（桌面端移动预览，由 Makefile dev-mobile 设置）
//
// 不触发的场景：
//   - 桌面端正常启动（无任何 mobile 相关环境变量）— mobile 段被忽略
func ApplyMobileOverlay(cfg *Config) {
	if cfg.Mobile == nil {
		return
	}
	if cfg.Mobile.Server != nil && cfg.Mobile.Server.Dir != "" {
		cfg.Server.Dir = cfg.Mobile.Server.Dir
	}
	if cfg.Mobile.Output != nil && cfg.Mobile.Output.Path != "" {
		cfg.OutputPath = cfg.Mobile.Output.Path
	}
	if cfg.Mobile.Webdav != nil && cfg.Mobile.Webdav.Dir != "" {
		cfg.Webdav.Dir = cfg.Mobile.Webdav.Dir
	}
}

func mergeConfig(base, overlay *Config) *Config {
	if overlay == nil {
		return base
	}
	baseData, err := json.Marshal(base)
	if err != nil {
		return base
	}
	overlayData, err := json.Marshal(overlay)
	if err != nil {
		return base
	}
	var baseMap, overlayMap map[string]interface{}
	if json.Unmarshal(baseData, &baseMap) != nil || json.Unmarshal(overlayData, &overlayMap) != nil {
		return base
	}
	deepMerge(baseMap, overlayMap)
	resultData, _ := json.Marshal(baseMap)
	var result Config
	if json.Unmarshal(resultData, &result) != nil {
		return base
	}
	result.Provider = base.Provider
	return &result
}

func deepMerge(base, overlay map[string]interface{}) {
	for k, ov := range overlay {
		if ov == nil {
			continue
		}
		bv, ok := base[k]
		if !ok {
			base[k] = ov
			continue
		}
		bm, bo := bv.(map[string]interface{})
		om, oo := ov.(map[string]interface{})
		if bo && oo {
			deepMerge(bm, om)
		} else if !isZeroValue(ov) {
			base[k] = ov
		}
	}
}

func isZeroValue(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.String:
		return rv.String() == ""
	case reflect.Bool:
		return !rv.Bool()
	default:
		return false
	}
}

type mergeCandidates struct {
	Dev  string
	User string
}

func findMergeCandidates() *mergeCandidates {
	dirs := searchDirs()
	var c mergeCandidates
	for _, dir := range dirs {
		if c.Dev == "" && exists(filepath.Join(dir, "config.dev.json")) {
			c.Dev = filepath.Join(dir, "config.dev.json")
		}
		if c.User == "" && exists(filepath.Join(dir, "config.user.json")) {
			c.User = filepath.Join(dir, "config.user.json")
		}
		if c.Dev != "" && c.User != "" {
			break
		}
	}
	if c.Dev == "" && c.User == "" {
		return nil
	}
	return &c
}

func loadAndMerge(base *Config, path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return base
	}
	var overlay Config
	if json.Unmarshal(data, &overlay) != nil {
		return base
	}
	slog.Info("Merged config file", "path", path)
	return mergeConfig(base, &overlay)
}

func loadSingleFile(cfg *Config, path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		slog.Info("Config file not found, using default settings", "path", path)
		return finalize(cfg), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file '%s': %w", path, err)
	}
	return finalize(cfg), nil
}

func finalize(cfg *Config) *Config {
	if cfg.Server.Dir == "/" {
		cfg.Server.Dir, _ = os.Getwd()
	}
	if os.Getenv("ENCV_MOBILE") == "1" || os.Getenv("ENCV_DEV_PREVIEW") == "1" {
		ApplyMobileOverlay(cfg)
	}
	slog.Info("Configuration loaded", "log_level", cfg.Log.Level)
	return cfg
}

func searchDirs() []string {
	var dirs []string
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if exePath, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exePath))
	}
	return dirs
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// GetPluginSettingsFor 是一个泛型辅助函数，用于安全地获取并解析特定插件的配置。
// T 是插件配置结构体的类型，例如 VideoPluginConfig。
// 它会从 map 中查找插件配置，并将其反序列化为 T 类型的指针。
func GetPluginSettingsFor[T any](cfg *Config, pluginName string) (*T, error) {
	var rawSettings json.RawMessage
	var err error

	// 1. 优先检查是否有动态 Provider
	if cfg.Provider != nil {
		rawSettings, err = cfg.Provider.GetPluginSettings(pluginName)
		if err != nil {
			return nil, fmt.Errorf("failed to get settings for plugin '%s' from provider: %w", pluginName, err)
		}
	} else {
		// 2. 如果没有 Provider，则使用传统的 PluginSettings map
		result, ok := cfg.PluginSettings[pluginName]
		if !ok {
			// 在没有 Provider 的情况下，如果 map 中没有，说明用户确实没配置
			return nil, fmt.Errorf("no settings found for plugin '%s'", pluginName)
		}
		rawSettings = result
	}

	// 3. 使用统一的辅助函数解析配置
	return UnmarshalPluginSettings[T](rawSettings, pluginName)
}

// UnmarshalPluginSettings 是一个通用的辅助函数，用于将原始 JSON 解析为具体的插件配置结构体
// 它不依赖于任何全局的 config 对象
func UnmarshalPluginSettings[T any](rawSettings json.RawMessage, pluginName string) (*T, error) {
	var settings T
	if len(rawSettings) == 0 {
		// 如果没有提供配置，返回零值
		return &settings, nil
	}
	if err := json.Unmarshal(rawSettings, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

// 智能地查找 config.user.json 文件
func FindConfigPath(flagPath string) (string, error) {
	// 1. 最高优先级：命令行标志指定的路径
	if flagPath != "" {
		if _, err := os.Stat(flagPath); err == nil {
			slog.Info("Using config from command-line flag", "path", flagPath)
			return flagPath, nil
		}
		return "", fmt.Errorf("config file specified by flag not found: %s", flagPath)
	}

	// 2. 次高优先级：环境变量
	if envPath := os.Getenv("ENCV_CONFIG_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			slog.Info("Using config from environment variable ENCV_CONFIG_PATH", "path", envPath)
			return envPath, nil
		}
		return "", fmt.Errorf("config file from environment variable not found: %s", envPath)
	}

	// 3. 再次优先级：当前工作目录
	// 这完美适配了 `go run ./cmd/encv start` 的场景
	wd, err := os.Getwd()
	if err == nil {
		wdConfigPath := filepath.Join(wd, "config.user.json")
		if _, err := os.Stat(wdConfigPath); err == nil {
			slog.Info("Using config from current working directory", "path", wdConfigPath)
			return wdConfigPath, nil
		}
	}

	// 4. 最低优先级：可执行文件所在目录
	// 这适配了生产环境，将配置文件和二进制文件放在一起
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		exeConfigPath := filepath.Join(exeDir, "config.user.json")
		if _, err := os.Stat(exeConfigPath); err == nil {
			slog.Info("Using config from executable directory", "path", exeConfigPath)
			return exeConfigPath, nil
		}
	}

	return "", fmt.Errorf("config.user.json not found in any of the standard locations (cwd, exe dir, env var, or flag)")
}

// --- 全局 MIME 类型映射表，以 OpenList 为准 ---
var ContentTypes = map[string]string{
	// Text
	"txt":        "text/plain; charset=utf-8",
	"htm":        "text/html; charset=utf-8",
	"html":       "text/html; charset=utf-8",
	"xml":        "text/xml; charset=utf-8",
	"java":       "text/x-java-source; charset=utf-8",
	"properties": "text/plain; charset=utf-8",
	"sql":        "text/plain; charset=utf-8",
	"js":         "application/javascript; charset=utf-8",
	"md":         "text/plain; charset=utf-8",
	"json":       "application/json; charset=utf-8",
	"conf":       "text/plain; charset=utf-8",
	"ini":        "text/plain; charset=utf-8",
	"vue":        "text/plain; charset=utf-8",
	"php":        "text/plain; charset=utf-8",
	"py":         "text/x-python; charset=utf-8",
	"bat":        "text/plain; charset=utf-8",
	"gitignore":  "text/plain; charset=utf-8",
	"yml":        "application/x-yaml; charset=utf-8",
	"yaml":       "application/x-yaml; charset=utf-8",
	"go":         "text/plain; charset=utf-8",
	"sh":         "application/x-sh; charset=utf-8",
	"c":          "text/plain; charset=utf-8",
	"cpp":        "text/plain; charset=utf-8",
	"h":          "text/plain; charset=utf-8",
	"hpp":        "text/plain; charset=utf-8",
	"tsx":        "text/plain; charset=utf-8",
	"vtt":        "text/plain; charset=utf-8",
	"srt":        "text/plain; charset=utf-8",
	"ass":        "text/plain; charset=utf-8",
	"rs":         "text/plain; charset=utf-8",
	"lrc":        "text/plain; charset=utf-8",
	"strm":       "text/plain; charset=utf-8",

	// Audio
	"mp3":  "audio/mpeg",
	"flac": "audio/flac",
	"ogg":  "audio/ogg",
	"m4a":  "audio/mp4",
	"wav":  "audio/wav",
	"opus": "audio/opus",
	"wma":  "audio/x-ms-wma",

	// Video
	"mp4":  "video/mp4",
	"mkv":  "video/x-matroska",
	"avi":  "video/x-msvideo",
	"mov":  "video/quicktime",
	"rmvb": "application/vnd.rn-realmedia-vbr",
	"webm": "video/webm",
	"flv":  "video/x-flv",
	"m3u8": "application/vnd.apple.mpegurl",

	// Image
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"tiff": "image/tiff",
	"png":  "image/png",
	"gif":  "image/gif",
	"bmp":  "image/bmp",
	"svg":  "image/svg+xml",
	"ico":  "image/x-icon",
	"swf":  "application/x-shockwave-flash",
	"webp": "image/webp",
	"avif": "image/avif",

	// Iframe
	"doc":  "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xls":  "application/vnd.ms-excel",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"ppt":  "application/vnd.ms-powerpoint",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"pdf":  "application/pdf",
	"epub": "application/epub+zip",
}

// GetTextPreviewExtensions 返回所有 MIME 类型含 "text/" 的扩展名列表
func GetTextPreviewExtensions() []string {
	var exts []string
	for ext, mime := range ContentTypes {
		if len(mime) >= 5 && mime[:5] == "text/" {
			exts = append(exts, ext)
		}
	}
	return exts
}

// AppDataDir 返回某子系统【应用数据】的持久化目录（与 server.mountRegistryDataPath 同脉络）。
//
// 平台/env 矩阵（复用 mountRegistryDataPath 的目录派生，取同级 .encv/<subdir>）：
//
//	Android 任意   <ENCV_APP_FILES_DIR>/.encv/<subdir>
//	Linux   dev    $XDG_DATA_HOME/encv-dev/.encv/<subdir>
//	Linux   prod   $XDG_DATA_HOME/encv/.encv/<subdir>
//	macOS   dev    $HOME/Library/Application Support/encv-dev/.encv/<subdir>
//	macOS   prod   $HOME/Library/Application Support/encv/.encv/<subdir>
//	Windows dev    %LOCALAPPDATA%\encv-dev\.encv\<subdir>
//	Windows prod   %LOCALAPPDATA%\encv\.encv\<subdir>
//
// 设计原则（续43 用户硬要求）：应用数据（DB / 任务持久化 / 回收站 / FTS 索引）必须落在
// app 私有/标准数据目录，绝不进 servingDir（静态 web 根；Android 上是打包私有只读资产，
// 写不进去也不该混用户媒体）。servingDir 只装用户内容。
//
// 优先级：ENCV_<SUBDIR_UPPER>_DIR（明确指定）> 派生默认值。
func AppDataDir(subdir string) string {
	envKey := "ENCV_" + strings.ToUpper(subdir) + "_DIR"
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	// 与 themeDataPath/kernelDataPath/simverseDataPath 完全一致：
	// 复用 mountRegistryDataPath 的目录派生（filepath.Dir(mountRegistryDataPath)），再拼子目录。
	return filepath.Join(appDataParent(), subdir)
}

// appDataParent 返回 mountRegistryDataPath 的父目录（与 filepath.Dir(mountRegistryDataPath(nil)) 一致）。
//
//	Android：<ENCV_APP_FILES_DIR>/.encv   （mounts.json 在 <base>/.encv/mounts.json）
//	桌面：   <XDG/LOCALAPPDATA/...>/encv(-dev)
func appDataParent() string {
	isAndroid := os.Getenv("ENCV_MOBILE") == "1"
	if isAndroid {
		base := os.Getenv("ENCV_APP_FILES_DIR")
		if base == "" {
			base = "/data/user/0/com.encvgo.app/files"
		}
		return filepath.Join(base, ".encv")
	}
	isDev := os.Getenv("ENCV_DEV") == "1" || os.Getenv("ENCV_DEV_PREVIEW") == "1"
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		if isDev {
			return filepath.Join(base, "encv-dev")
		}
		return filepath.Join(base, "encv")
	case "darwin":
		base := filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
		if isDev {
			return filepath.Join(base, "encv-dev")
		}
		return filepath.Join(base, "encv")
	default: // linux 等
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(os.Getenv("HOME"), ".local", "share")
		}
		if isDev {
			return filepath.Join(base, "encv-dev")
		}
		return filepath.Join(base, "encv")
	}
}
