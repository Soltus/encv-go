package config

import (
	"os"
	"testing"
)

// 模拟旧版 Go 二进制（扁平 MobileConfig）遇到新嵌套格式 JSON 的行为
func TestOldBinaryWithNewNestedJSON(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	defer os.Unsetenv("ENCV_MOBILE")

	jsonStr := `{
  "server": {"dir": "/", "port": 2025},
  "mobile": {
    "server": {"dir": "/storage/emulated/0"},
    "output": {"path": "/storage/emulated/0/encv-output"}
  }
}`

	tmpfile, err := os.CreateTemp("", "config-old-test-*.json")
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

	t.Logf("=== 新代码解析嵌套 JSON ===")
	t.Logf("server.dir     = %q", cfg.Server.Dir)
	t.Logf("cfg.Mobile     = %+v", cfg.Mobile)
	if cfg.Mobile != nil {
		t.Logf("Mobile.Server  = %+v", cfg.Mobile.Server)
		t.Logf("Mobile.Output = %+v", cfg.Mobile.Output)
		if cfg.Mobile.Server != nil {
			t.Logf("Server.Dir      = %q", cfg.Mobile.Server.Dir)
		}
		if cfg.Mobile.Output != nil {
			t.Logf("Output.Path     = %q", cfg.Mobile.Output.Path)
		}
	}

	if cfg.Server.Dir != "/storage/emulated/0" {
		t.Errorf("overlay FAILED: server.dir=%q want /storage/emulated/0", cfg.Server.Dir)
	}
}

// 模拟 mobile 段存在但子对象为空的情况
func TestMobileOverlay_EmptySubObjects(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	defer os.Unsetenv("ENCV_MOBILE")

	testCases := []struct {
		name    string
		json    string
		wantDir string
	}{
		{
			name:    "mobile server dir empty",
			json:    `{"server":{"dir":"/","port":2025},"mobile":{"server":{"dir":""}}}`,
			wantDir: "/workspace", // fallback to cwd since empty string
		},
		{
			name:    "mobile server null",
			json:    `{"server":{"dir":"/","port":2025},"mobile":{"server":null}}`,
			wantDir: "/workspace",
		},
		{
			name:    "mobile missing server key entirely",
			json:    `{"server":{"dir":"/","port":2025},"mobile":{"output":{"path":"/tmp"}}}`,
			wantDir: "/workspace",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp("", "config-edge-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())
			tmpfile.WriteString(tc.json)
			tmpfile.Close()

			cfg, err := Load(tmpfile.Name())
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			t.Logf("server.dir=%q  mobile=%+v", cfg.Server.Dir, cfg.Mobile)
		})
	}
}

// 验证 finalize() 中 os.Getwd() 在 overlay 之前的执行不影响最终结果
func TestFinalizeOrder_GetwdBeforeOverlay(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	defer os.Unsetenv("ENCV_MOBILE")

	jsonStr := `{"server":{"dir":"/","port":2025},"mobile":{"server":{"dir":"/storage/emulated/0"}}}`

	tmpfile, err := os.CreateTemp("", "config-order-*.json")
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

	t.Logf("finalize order test:")
	t.Logf("  After Getwd fallback: server.dir would be = %q (cwd)", func() string { d, _ := os.Getwd(); return d }())
	t.Logf("  After overlay:        server.dir = %q", cfg.Server.Dir)

	if cfg.Server.Dir != "/storage/emulated/0" {
		t.Errorf("overlay must override Getwd result: got %q", cfg.Server.Dir)
	}
}
