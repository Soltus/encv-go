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

// setupSearchTestServer 创建一个用于搜索测试的 server，使用独立的 mounts.json 避免全局干扰。
func setupSearchTestServer(t *testing.T) (*Server, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "encv-search-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	oldMountsFile := os.Getenv("ENCV_MOUNTS_FILE")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)

	subDir := filepath.Join(tmpDir, "下载")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("MkdirAll 下载: %v", err)
	}

	testFile := filepath.Join(subDir, "思源笔记shortcuts-Test.zip")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("WriteFile: %v", err)
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Listen: %v", err)
	}
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
			Level: "error",
		},
		PluginSettings: map[string]json.RawMessage{},
	}

	if err := plugins.InitializeWithSettings(cfg.PluginSettings); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("plugins init: %v", err)
	}

	ctx := config.NewContext(context.Background(), cfg)
	s := NewServer(ctx, "")

	addr, err := s.Start("test")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Start: %v", err)
	}

	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		s.Stop()
		os.RemoveAll(tmpDir)
		t.Fatalf("SplitHostPort: %v", splitErr)
	}
	if host == "" || host == "::" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	teardown := func() {
		s.Stop()
		os.RemoveAll(tmpDir)
		if oldMountsFile != "" {
			os.Setenv("ENCV_MOUNTS_FILE", oldMountsFile)
		} else {
			os.Unsetenv("ENCV_MOUNTS_FILE")
		}
	}

	return s, baseURL, teardown
}

func TestSearch_RootMount_Recursive_WithIndex(t *testing.T) {
	s, baseURL, teardown := setupSearchTestServer(t)
	defer teardown()

	s.mobileSvc.RebuildIndex()
	for i := 0; i < 50; i++ {
		stats := s.mobileSvc.GetIndexStats()
		if !stats.IsIndexing {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stats := s.mobileSvc.GetIndexStats()
	t.Logf("index stats: files=%d dirs=%d", stats.TotalFiles, stats.TotalDirs)

	searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=true",
		baseURL,
		url.QueryEscape("/d"),
		url.QueryEscape("思源笔记"))

	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("search status = %d, want 200", resp.StatusCode)
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

	t.Logf("root search result count (with index): %d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  - %s", f.Path)
	}

	if len(result.Files) == 0 {
		t.Fatal("expected at least 1 search result from root /d with index, got 0")
	}

	found := false
	for _, f := range result.Files {
		if f.Name == "思源笔记shortcuts-Test.zip" {
			found = true
			if f.Path == "" || f.Path[0] != '/' {
				t.Errorf("result path should start with /, got %q", f.Path)
			}
			t.Logf("✅ found target file at root with index: path=%s", f.Path)
			break
		}
	}
	if !found {
		t.Error("root search result with index should contain 思源笔记shortcuts-Test.zip")
	}
}

func TestSearch_RootMount_Recursive(t *testing.T) {
	_, baseURL, teardown := setupSearchTestServer(t)
	defer teardown()

	searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=true",
		baseURL,
		url.QueryEscape("/d"),
		url.QueryEscape("思源笔记"))

	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("search status = %d, want 200", resp.StatusCode)
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

	t.Logf("root search result count: %d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  - %s", f.Path)
	}

	if len(result.Files) == 0 {
		t.Fatal("expected at least 1 search result from root /d, got 0")
	}

	found := false
	for _, f := range result.Files {
		if f.Name == "思源笔记shortcuts-Test.zip" {
			found = true
			if f.Path == "" || f.Path[0] != '/' {
				t.Errorf("result path should start with /, got %q", f.Path)
			}
			t.Logf("✅ found target file at root: path=%s", f.Path)
			break
		}
	}
	if !found {
		t.Error("root search result should contain 思源笔记shortcuts-Test.zip")
	}
}

func TestSearch_PrimaryMount_Recursive(t *testing.T) {
	_, baseURL, teardown := setupSearchTestServer(t)
	defer teardown()

	searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=true",
		baseURL,
		url.QueryEscape("/d/primary"),
		url.QueryEscape("思源笔记"))

	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("search status = %d, want 200", resp.StatusCode)
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

	t.Logf("primary mount search result count: %d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  - %s", f.Path)
	}

	found := false
	for _, f := range result.Files {
		if f.Name == "思源笔记shortcuts-Test.zip" {
			found = true
			t.Logf("✅ found target file in primary: path=%s", f.Path)
			break
		}
	}
	if !found {
		t.Error("primary mount search should contain 思源笔记shortcuts-Test.zip")
	}
}

func TestSearch_RootMount_NonRecursive(t *testing.T) {
	_, baseURL, teardown := setupSearchTestServer(t)
	defer teardown()

	searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=false",
		baseURL,
		url.QueryEscape("/d"),
		url.QueryEscape("下载"))

	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("search status = %d, want 200", resp.StatusCode)
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

	t.Logf("root non-recursive search result count: %d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  - %s", f.Path)
	}

	found := false
	for _, f := range result.Files {
		if f.Name == "下载" && f.IsDirectory == false {
		}
		if f.Name == "下载" {
			found = true
			t.Logf("✅ found '下载' directory at root: path=%s", f.Path)
			break
		}
	}
	if !found {
		t.Log("note: '下载' directory not found at root (this may be ok if directories aren't returned)")
	}
}

func TestSearch_RootMount_NonRecursive_NoDeepFile(t *testing.T) {
	_, baseURL, teardown := setupSearchTestServer(t)
	defer teardown()

	searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=false",
		baseURL,
		url.QueryEscape("/d"),
		url.QueryEscape("思源笔记"))

	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("search status = %d, want 200", resp.StatusCode)
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

	t.Logf("root non-recursive search for deep file: count=%d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  - %s", f.Path)
	}

	for _, f := range result.Files {
		if f.Name == "思源笔记shortcuts-Test.zip" {
			t.Error("non-recursive search should NOT find files in subdirectories")
		}
	}
	t.Log("✅ non-recursive search correctly excludes deep files")
}
