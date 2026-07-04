package config

import (
	"encoding/json"
	"os"
	"testing"
)

// TestDefaultConfig_V4Options 验证 DefaultConfig 对 4 个新增 v4 配置项的默认值。
func TestDefaultConfig_V4Options(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.V4CipherMode != 0 {
		t.Errorf("V4CipherMode default = %d, want 0 (AES-128-CTR)", cfg.V4CipherMode)
	}
	if cfg.V4CompressionMode != "none" {
		t.Errorf("V4CompressionMode default = %q, want \"none\"", cfg.V4CompressionMode)
	}
	if !cfg.V4EnableHMAC {
		t.Errorf("V4EnableHMAC default = false, want true")
	}
	if cfg.V4ZstdBlockSize != 65536 {
		t.Errorf("V4ZstdBlockSize default = %d, want 65536", cfg.V4ZstdBlockSize)
	}
}

// TestLoad_V4Options 验证 Load() 能正确解析 4 个新增 v4 配置项。
func TestLoad_V4Options(t *testing.T) {
	jsonStr := `{
  "password": "test123",
  "recover": false,
  "output_path": "./encrypted",
  "server": {"port": 1999, "dir": "./"},
  "admin": {"port": 0, "password": ""},
  "webdav": {"port": 0, "root": "", "dir": "", "username": "", "password": ""},
  "proxy": {"sites": {}, "disable_signature_verification": false},
  "log": {"level": "info", "file": "", "console": true},
  "plugin_settings": {},
  "v4_cipher_mode": 1,
  "v4_compression_mode": "zstd",
  "v4_enable_hmac": false,
  "v4_zstd_block_size": 131072
}`

	tmpfile, err := os.CreateTemp("", "config-v4-test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.WriteString(jsonStr)
	tmpfile.Close()

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.V4CipherMode != 1 {
		t.Errorf("V4CipherMode = %d, want 1 (AES-256-CTR)", cfg.V4CipherMode)
	}
	if cfg.V4CompressionMode != "zstd" {
		t.Errorf("V4CompressionMode = %q, want \"zstd\"", cfg.V4CompressionMode)
	}
	if cfg.V4EnableHMAC {
		t.Errorf("V4EnableHMAC = true, want false")
	}
	if cfg.V4ZstdBlockSize != 131072 {
		t.Errorf("V4ZstdBlockSize = %d, want 131072", cfg.V4ZstdBlockSize)
	}
}

// TestLoad_V4Options_PartialOverride 验证 4 个 v4 配置项都可以独立被用户配置覆盖。
func TestLoad_V4Options_PartialOverride(t *testing.T) {
	jsonStr := `{
  "password": "test123",
  "recover": false,
  "output_path": "./encrypted",
  "server": {"port": 1999, "dir": "./"},
  "admin": {"port": 0, "password": ""},
  "webdav": {"port": 0, "root": "", "dir": "", "username": "", "password": ""},
  "proxy": {"sites": {}, "disable_signature_verification": false},
  "log": {"level": "info", "file": "", "console": true},
  "plugin_settings": {},
  "v4_cipher_mode": 1
}`

	tmpfile, err := os.CreateTemp("", "config-v4-partial-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.WriteString(jsonStr)
	tmpfile.Close()

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.V4CipherMode != 1 {
		t.Errorf("V4CipherMode = %d, want 1 (user override)", cfg.V4CipherMode)
	}
	// 其他字段保持默认值
	if cfg.V4CompressionMode != "none" {
		t.Errorf("V4CompressionMode = %q, want \"none\" (default)", cfg.V4CompressionMode)
	}
	if !cfg.V4EnableHMAC {
		t.Errorf("V4EnableHMAC = false, want true (default)")
	}
	if cfg.V4ZstdBlockSize != 65536 {
		t.Errorf("V4ZstdBlockSize = %d, want 65536 (default)", cfg.V4ZstdBlockSize)
	}
}

// TestConfig_V4Options_JSONSerialization 验证 4 个新字段能被正确序列化/反序列化。
func TestConfig_V4Options_JSONSerialization(t *testing.T) {
	cfg := &Config{
		V4CipherMode:      1,
		V4CompressionMode: "zstd",
		V4EnableHMAC:      false,
		V4ZstdBlockSize:   1024,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var roundTrip Config
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if roundTrip.V4CipherMode != 1 {
		t.Errorf("V4CipherMode = %d, want 1", roundTrip.V4CipherMode)
	}
	if roundTrip.V4CompressionMode != "zstd" {
		t.Errorf("V4CompressionMode = %q, want \"zstd\"", roundTrip.V4CompressionMode)
	}
	if roundTrip.V4EnableHMAC {
		t.Errorf("V4EnableHMAC = true, want false")
	}
	if roundTrip.V4ZstdBlockSize != 1024 {
		t.Errorf("V4ZstdBlockSize = %d, want 1024", roundTrip.V4ZstdBlockSize)
	}
}
