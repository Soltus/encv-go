package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/encv/plugins"
)

func TestSearchIndexPathPrefixDebug(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "encv-search-debug-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)
	defer os.Unsetenv("ENCV_MOUNTS_FILE")

	downloadDir := filepath.Join(tmpDir, "下载")
	os.MkdirAll(downloadDir, 0755)
	testFile := filepath.Join(downloadDir, "思源笔记shortcuts-Amwwzto.zip")
	os.WriteFile(testFile, []byte("test"), 0644)

	listener, _ := net.Listen("tcp", ":0")
	availablePort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := &config.Config{
		Password: "test",
		Server: types.HttpServer{
			Port: availablePort,
			Dir:  tmpDir,
		},
		Webdav: types.WebdavServer{
			Root: "/webdav/",
			Dir:  tmpDir,
		},
		Log: types.LogConfig{
			Level: "debug",
		},
		PluginSettings: map[string]json.RawMessage{},
	}

	plugins.InitializeWithSettings(cfg.PluginSettings)
	ctx := config.NewContext(context.Background(), cfg)
	s := NewServer(ctx, "")
	addr, _ := s.Start("test")
	defer s.Stop()

	host, port, _ := net.SplitHostPort(addr)
	if host == "" || host == "::" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%s", host, port)
	_ = baseURL

	s.mobileSvc.RebuildIndex()
	for i := 0; i < 100; i++ {
		stats := s.mobileSvc.GetIndexStats()
		if !stats.IsIndexing && stats.TotalFiles > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	stats := s.mobileSvc.GetIndexStats()
	t.Logf("index: files=%d dirs=%d", stats.TotalFiles, stats.TotalDirs)

	// Test 1: direct SearchFiles with /d/primary
	r1, err1 := s.mobileSvc.SearchFiles("/d/primary", "思源笔记", true)
	t.Logf("SearchFiles(/d/primary) count=%d err=%v", len(r1), err1)
	for _, f := range r1 {
		t.Logf("  -> %s", f.Path)
	}

	// Test 2: direct SearchFiles with /d/primary/
	r2, err2 := s.mobileSvc.SearchFiles("/d/primary/", "思源笔记", true)
	t.Logf("SearchFiles(/d/primary/) count=%d err=%v", len(r2), err2)
	for _, f := range r2 {
		t.Logf("  -> %s", f.Path)
	}

	// Test 3: searchAcrossAllMounts
	pFiles, wFiles := s.searchAcrossAllMounts("思源笔记", 200, true)
	t.Logf("searchAcrossAllMounts: physical=%d webdav=%d", len(pFiles), len(wFiles))
	for _, f := range pFiles {
		t.Logf("  physical: %s", f.Path)
	}
	for _, f := range wFiles {
		t.Logf("  webdav: %s", f.Path)
	}

	// Verify: target file should be in physicalFiles
	found := false
	for _, f := range pFiles {
		if f.Name == "思源笔记shortcuts-Amwwzto.zip" {
			found = true
			t.Logf("✅ target found in physicalFiles: %s", f.Path)
			break
		}
	}
	if !found {
		t.Errorf("❌ target NOT found in physicalFiles!")
	}
}
