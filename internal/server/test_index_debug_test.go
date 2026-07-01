package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/encv/plugins"
)

func TestSearch_IndexDebug(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "encv-search-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)
	defer os.Unsetenv("ENCV_MOUNTS_FILE")

	subDir := filepath.Join(tmpDir, "下载")
	os.MkdirAll(subDir, 0755)
	testFile := filepath.Join(subDir, "思源笔记shortcuts-Test.zip")
	os.WriteFile(testFile, []byte("test content"), 0644)

	t.Logf("tmpDir: %s", tmpDir)
	t.Logf("testFile: %s", testFile)

	// Verify file exists
	if _, err := os.Stat(testFile); err != nil {
		t.Fatalf("test file doesn't exist: %v", err)
	}

	listener, _ := net.Listen("tcp", ":0")
	availablePort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := &config.Config{
		Password: "test-password",
		Server: types.HttpServer{
			Port: availablePort,
			Dir:  tmpDir,
		},
		Webdav: types.WebdavServer{
			Root:     "/webdav/",
			Dir:      tmpDir,
			Username: "",
			Password: "",
		},
		Log: types.LogConfig{
			Level: "debug",
		},
		PluginSettings: map[string]json.RawMessage{},
	}

	plugins.InitializeWithSettings(cfg.PluginSettings)

	ctx := config.NewContext(context.Background(), cfg)
	s := NewServer(ctx, "")

	addr, err := s.Start("test")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	host, port, _ := net.SplitHostPort(addr)
	if host == "" || host == "::" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	t.Logf("servingDir: %s", s.servingDir)
	t.Logf("mobileSvc.servingDir: %s", s.mobileSvc.GetServingDir())

	// Rebuild index
	s.mobileSvc.RebuildIndex()
	
	// Wait longer
	for i := 0; i < 100; i++ {
		stats := s.mobileSvc.GetIndexStats()
		if !stats.IsIndexing {
			t.Logf("index done after %d waits: files=%d dirs=%d", i, stats.TotalFiles, stats.TotalDirs)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	stats := s.mobileSvc.GetIndexStats()
	t.Logf("final index stats: files=%d dirs=%d totalSize=%d", stats.TotalFiles, stats.TotalDirs, stats.TotalSize)

	// Try direct SearchFiles call
	results, err := s.mobileSvc.SearchFiles("/d/primary", "思源笔记", true)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	t.Logf("direct SearchFiles result count: %d", len(results))
	for _, r := range results {
		t.Logf("  - %s", r.Path)
	}

	// Try HTTP API
	searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=true",
		baseURL,
		url.QueryEscape("/d"),
		url.QueryEscape("思源笔记"))
	resp, _ := http.Get(searchURL)
	defer resp.Body.Close()
	t.Logf("HTTP status: %d", resp.StatusCode)
}
