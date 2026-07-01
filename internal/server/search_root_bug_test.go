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
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/encv/plugins"
)

func TestSearchRootRealWorldScenario(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "encv-search-real-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)
	defer os.Unsetenv("ENCV_MOUNTS_FILE")

	subDir := filepath.Join(tmpDir, "下载")
	os.MkdirAll(subDir, 0755)
	testFile := filepath.Join(subDir, "思源笔记shortcuts-Amwwzto.zip")
	os.WriteFile(testFile, []byte("test content 12345"), 0644)

	deepDir := filepath.Join(tmpDir, "Movies", "Action")
	os.MkdirAll(deepDir, 0755)
	deepFile := filepath.Join(deepDir, "test-movie.mp4")
	os.WriteFile(deepFile, []byte("fake movie"), 0644)

	t.Logf("tmpDir: %s", tmpDir)
	t.Logf("testFile: %s", testFile)

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

	t.Logf("=== 基础信息 ===")
	t.Logf("servingDir: %s", s.servingDir)
	t.Logf("canUseWebdavIndex: %v", s.canUseWebdavIndex())
	t.Logf("webdavFSByMount count: %d", len(s.webdavFSByMount))
	t.Logf("hasWebdav (computed): %v", s.canUseWebdavIndex() || len(s.webdavFSByMount) > 0)
	t.Logf("mount count: %d", len(s.mountRegistry.List()))
	for _, m := range s.mountRegistry.List() {
		t.Logf("  - mount: %s enabled=%v driver=%s root=%s", m.Name, m.Enabled, m.Driver, m.RootPath)
	}

	t.Logf("=== 构建索引 ===")
	s.mobileSvc.RebuildIndex()
	for i := 0; i < 100; i++ {
		stats := s.mobileSvc.GetIndexStats()
		if !stats.IsIndexing && stats.TotalFiles > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	stats := s.mobileSvc.GetIndexStats()
	t.Logf("index stats: files=%d dirs=%d totalSize=%d isIndexing=%v",
		stats.TotalFiles, stats.TotalDirs, stats.TotalSize, stats.IsIndexing)

	t.Run("direct_SearchFiles_primary_mount", func(t *testing.T) {
		results, err := s.mobileSvc.SearchFiles("/d/primary", "思源笔记", true)
		if err != nil {
			t.Fatalf("SearchFiles error: %v", err)
		}
		t.Logf("direct SearchFiles /d/primary result count: %d", len(results))
		for _, r := range results {
			t.Logf("  - %s (isDir=%v)", r.Path, r.IsDirectory)
		}
		if len(results) == 0 {
			t.Fatal("direct SearchFiles returns 0 results!")
		}
	})

	t.Run("http_root_d_recursive", func(t *testing.T) {
		searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=true",
			baseURL,
			url.QueryEscape("/d"),
			url.QueryEscape("思源笔记"))
		resp, err := http.Get(searchURL)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}

		var result struct {
			Files []struct {
				Name        string `json:"name"`
				Path        string `json:"path"`
				IsDirectory bool   `json:"isDirectory"`
			} `json:"files"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}

		t.Logf("HTTP /d recursive result count: %d", len(result.Files))
		for _, f := range result.Files {
			t.Logf("  - %s", f.Path)
		}

		found := false
		for _, f := range result.Files {
			if strings.Contains(f.Name, "思源笔记") && strings.HasSuffix(f.Name, ".zip") {
				found = true
				t.Logf("✅ FOUND target file: %s", f.Path)
				break
			}
		}
		if !found {
			t.Errorf("❌ target file NOT found in /d search results!")
		}
	})

	t.Run("http_primary_mount_recursive", func(t *testing.T) {
		searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=true",
			baseURL,
			url.QueryEscape("/d/primary"),
			url.QueryEscape("思源笔记"))
		resp, err := http.Get(searchURL)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Files []struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"files"`
		}
		json.NewDecoder(resp.Body).Decode(&result)

		t.Logf("HTTP /d/primary recursive result count: %d", len(result.Files))
		for _, f := range result.Files {
			t.Logf("  - %s", f.Path)
		}

		if len(result.Files) == 0 {
			t.Error("0 results from /d/primary search")
		}
	})

	t.Run("http_root_d_non_recursive", func(t *testing.T) {
		searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=false",
			baseURL,
			url.QueryEscape("/d"),
			url.QueryEscape("下载"))
		resp, _ := http.Get(searchURL)
		defer resp.Body.Close()

		var result struct {
			Files []struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"files"`
		}
		json.NewDecoder(resp.Body).Decode(&result)

		t.Logf("HTTP /d non-recursive result count: %d", len(result.Files))
		for _, f := range result.Files {
			t.Logf("  - %s", f.Path)
		}
	})
}
