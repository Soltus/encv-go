package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port int
	Host string

	UpstreamMobile   string
	UpstreamPlugin   string
	UpstreamEncvGo   string
	UpstreamOpenlist string
	UpstreamSimverse string

	SpawnGo           bool
	SpawnVite         bool
	SpawnPluginVite   bool
	SpawnOpenlist     bool
	SpawnSimverseVite bool

	EncvDevPreview string
	EncvMobile     string

	MobileDataDir string
	MobileDir     string
	RepoRoot      string
	PluginWebDir  string
	SimverseFrontendDir string

	AirBin  string
	NodeBin string
}

func Load() *Config {
	return &Config{
		Port: envInt("PORT", 16666),
		Host: envStr("HOST", "0.0.0.0"),

		UpstreamMobile:   envStr("UPSTREAM_MOBILE", "http://127.0.0.1:8100"),
		UpstreamPlugin:   envStr("UPSTREAM_PLUGIN", "http://127.0.0.1:5174"),
		UpstreamEncvGo:   envStr("UPSTREAM_ENCV_GO", "http://127.0.0.1:2025"),
		UpstreamOpenlist: envStr("UPSTREAM_OPENLIST", "http://127.0.0.1:5244"),
		UpstreamSimverse: envStr("UPSTREAM_SIMVERSE", "http://127.0.0.1:5176"),

		SpawnGo:           envBool("SPAWN_GO", true),
		SpawnVite:         envBool("SPAWN_VITE", true),
		SpawnPluginVite:   envBool("SPAWN_PLUGIN_VITE", false),
		SpawnOpenlist:     envBool("SPAWN_OPENLIST", false),
		SpawnSimverseVite: envBool("SPAWN_SIMVERSE_VITE", true),

		EncvDevPreview: envStr("ENCV_DEV_PREVIEW", "1"),
		EncvMobile:     envStr("ENCV_MOBILE", "1"),

		RepoRoot:            envStr("REPO_ROOT", "/workspace"),
		MobileDir:           envStr("MOBILE_DIR", ""),
		MobileDataDir:       envStr("MOBILE_DATA_DIR", "/storage/emulated/0"),
		PluginWebDir:        envStr("PLUGIN_WEB_DIR", ""),
		SimverseFrontendDir: envStr("SIMVERSE_FRONTEND_DIR", ""),

		AirBin:  envStr("AIR_BIN", ""),
		NodeBin: envStr("NODE_BIN", ""),
	}
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || v == "true" || v == "yes" || v == "on"
	}
	return def
}
