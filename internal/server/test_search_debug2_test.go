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

func TestSearch_WebdavFilterBug(t *testing.T) {
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
			Level: "info",
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

	// Build index
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

	t.Logf("canUseWebdavIndex: %v", s.canUseWebdavIndex())
	t.Logf("webdavFSByMount count: %d", len(s.webdavFSByMount))
	t.Logf("hasWebdav (computed): %v", s.canUseWebdavIndex() || len(s.webdavFSByMount) > 0)

	// Test root search
	searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=true",
		baseURL,
		url.QueryEscape("/d"),
		url.QueryEscape("思源笔记"))
	resp, _ := http.Get(searchURL)
	defer resp.Body.Close()

	var result struct {
		Files []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"files"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	t.Logf("root /d search result count: %d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  - %s", f.Path)
	}

	if len(result.Files) == 0 {
		t.Fatal("BUG: root /d search returns 0 results!")
	}
}
