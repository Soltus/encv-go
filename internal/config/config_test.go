package config

import (
	"os"
	"testing"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

func TestMobileOverlay_RealDeviceScenario(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	t.Setenv("HOME", "/data/user/0/com.encvgo.app/files")
	defer func() {
		os.Unsetenv("ENCV_MOBILE")
		os.Unsetenv("HOME")
	}()

	jsonStr := `{
  "server": {"dir": "/", "port": 2025},
  "output_path": "./output",
  "mobile": {
    "server": {"dir": "/storage/emulated/0"},
    "output": {"path": "/storage/emulated/0/encv-output"},
    "webdav": {"dir": ""}
  }
}`

	tmpfile, err := os.CreateTemp("", "config-test-*.json")
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

	t.Logf("server.dir = %q (expect /storage/emulated/0)", cfg.Server.Dir)
	t.Logf("output_path = %q (expect /storage/emulated/0/encv-output)", cfg.OutputPath)
	t.Logf("webdav.dir = %q (expect empty or original)", cfg.Webdav.Dir)
	t.Logf("mobile = %+v", cfg.Mobile)

	if cfg.Server.Dir != "/storage/emulated/0" {
		t.Errorf("server.dir = %q, want /storage/emulated/0", cfg.Server.Dir)
	}
	if cfg.OutputPath != "/storage/emulated/0/encv-output" {
		t.Errorf("output_path = %q, want /storage/emulated/0/encv-output", cfg.OutputPath)
	}
}

func TestMobileOverlay_DisabledOnDesktop(t *testing.T) {
	os.Unsetenv("ENCV_MOBILE")
	os.Unsetenv("ENCV_DEV_PREVIEW")

	jsonStr := `{
  "server": {"dir": "/", "port": 2025},
  "output_path": "./output",
  "mobile": {
    "server": {"dir": "/storage/emulated/0"}
  }
}`

	tmpfile, err := os.CreateTemp("", "config-test-*.json")
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

	t.Logf("server.dir = %q (expect cwd, NOT /storage/emulated/0)", cfg.Server.Dir)

	if cfg.Server.Dir == "/storage/emulated/0" {
		t.Error("overlay should NOT apply without ENCV_MOBILE or ENCV_DEV_PREVIEW")
	}
}

func TestMobileOverlay_NilMobile(t *testing.T) {
	t.Setenv("ENCV_MOBILE", "1")
	defer os.Unsetenv("ENCV_MOBILE")

	jsonStr := `{"server": {"dir": "/", "port": 2025}}`

	tmpfile, err := os.CreateTemp("", "config-test-*.json")
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

	t.Logf("server.dir = %q (nil mobile should not crash)", cfg.Server.Dir)
	if cfg.Mobile != nil {
		t.Error("mobile should be nil when not in JSON")
	}
}
