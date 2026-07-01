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

// setupMultiTokenSearchServer 创建含中文多 token 文件名的搜索测试 server。
//
// 测试文件：在线播放-高清视频.mp4
// 用户搜索场景：
//   - "在线 高清"（空格分隔多 token）应匹配
//   - "在线高清"（无分隔）应通过向量搜索匹配
func setupMultiTokenSearchServer(t *testing.T) (*Server, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "encv-multitoken-search-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	oldMountsFile := os.Getenv("ENCV_MOUNTS_FILE")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)

	videoDir := filepath.Join(tmpDir, "视频")
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("MkdirAll 视频: %v", err)
	}

	// 关键测试文件：名字含分隔符，多 token 搜索应能匹配
	testFile := filepath.Join(videoDir, "在线播放-高清视频.mp4")
	if err := os.WriteFile(testFile, []byte("fake video content"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("WriteFile: %v", err)
	}

	// 另一个干扰文件，只含部分 token
	distractorFile := filepath.Join(videoDir, "在线文档.pdf")
	if err := os.WriteFile(distractorFile, []byte("doc"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("WriteFile distractor: %v", err)
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
			Root: "/webdav/",
			Dir:  tmpDir,
		},
		Log:            types.LogConfig{Level: "error"},
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

// waitForIndexReady 轮询等待索引构建完成。
func waitForIndexReady(s *Server) {
	for i := 0; i < 50; i++ {
		stats := s.mobileSvc.GetIndexStats()
		if !stats.IsIndexing {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSearch_MultiToken_KeywordSearch 验证关键词搜索支持空格分隔的多 token AND 匹配。
//
// Bug：搜索 "在线 高清"（空格分隔）应当匹配 "在线播放-高清视频.mp4"，
// 但当前 SearchFiles 用 strings.Contains(name, "在线 高清") 做字面子串匹配，
// 文件名中没有空格，所以匹配失败。
//
// 期望行为：空格分隔的多个 token 都必须是文件名的子串（AND 逻辑）。
// "在线 高清" → ["在线", "高清"] → 两个都是 "在线播放-高清视频.mp4" 的子串 → 匹配
func TestSearch_MultiToken_KeywordSearch(t *testing.T) {
	_, baseURL, teardown := setupMultiTokenSearchServer(t)
	defer teardown()

	searchURL := fmt.Sprintf("%s/api/files/search?path=%s&keyword=%s&recursive=true",
		baseURL,
		url.QueryEscape("/d"),
		url.QueryEscape("在线 高清"))

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
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	t.Logf("multi-token keyword search result count: %d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  - %s (%s)", f.Name, f.Path)
	}

	found := false
	for _, f := range result.Files {
		if f.Name == "在线播放-高清视频.mp4" {
			found = true
			t.Logf("✅ found target file via multi-token keyword search: path=%s", f.Path)
			break
		}
	}
	if !found {
		t.Errorf("❌ BUG: multi-token keyword search '在线 高清' should match '在线播放-高清视频.mp4' but did not")
	}

	// 干扰文件 "在线文档.pdf" 只含 "在线" 不含 "高清"，不应匹配
	for _, f := range result.Files {
		if f.Name == "在线文档.pdf" {
			t.Errorf("❌ distractor '在线文档.pdf' should NOT match '在线 高清' (only has 在线, missing 高清)")
		}
	}
}

// TestSearch_MultiToken_VectorSearch_NoSpace 验证向量搜索端点 /api/search/files
// 用无空格关键词 "在线高清" 能通过向量语义匹配到 "在线播放-高清视频.mp4"。
//
// Bug：handleVectorSearchFilesGin 先做关键词搜索（strings.Contains 失败 → 0 候选），
// 然后 len(files) > 3 不成立 → 跳过向量重排序 → 返回空。
//
// 期望行为：当关键词搜索返回 0 结果时，若向量搜索可用，应 fallback 到纯向量搜索。
func TestSearch_MultiToken_VectorSearch_NoSpace(t *testing.T) {
	s, baseURL, teardown := setupMultiTokenSearchServer(t)
	defer teardown()

	// 先构建索引，让文件进入 fileIndex（SearchFiles 递归路径会用索引）
	s.mobileSvc.RebuildIndex()
	waitForIndexReady(s)

	searchURL := fmt.Sprintf("%s/api/search/files?q=%s&path=%s&recursive=true&limit=50",
		baseURL,
		url.QueryEscape("在线高清"),
		url.QueryEscape("/d"))

	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("GET vector search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("vector search status = %d, want 200", resp.StatusCode)
	}

	var result struct {
		Files []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"files"`
		VectorSearch bool `json:"vector_search"`
		Total        int  `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	t.Logf("vector search (no space) result count: %d (vector_search=%v)", len(result.Files), result.VectorSearch)
	for _, f := range result.Files {
		t.Logf("  - %s (%s)", f.Name, f.Path)
	}

	found := false
	for _, f := range result.Files {
		if f.Name == "在线播放-高清视频.mp4" {
			found = true
			t.Logf("✅ found target file via vector search (no space): path=%s", f.Path)
			break
		}
	}
	if !found {
		t.Errorf("❌ BUG: vector search '在线高清' should match '在线播放-高清视频.mp4' via bigram overlap but did not")
	}
}

// TestSearch_MultiToken_VectorSearch_WithSpace 验证向量搜索端点用空格分隔多 token 也能匹配。
// "在线 高清" → 关键词搜索应该已能匹配（AND 逻辑），向量重排序进一步提升排序。
func TestSearch_MultiToken_VectorSearch_WithSpace(t *testing.T) {
	s, baseURL, teardown := setupMultiTokenSearchServer(t)
	defer teardown()

	s.mobileSvc.RebuildIndex()
	waitForIndexReady(s)

	searchURL := fmt.Sprintf("%s/api/search/files?q=%s&path=%s&recursive=true&limit=50",
		baseURL,
		url.QueryEscape("在线 高清"),
		url.QueryEscape("/d"))

	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("GET vector search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("vector search status = %d, want 200", resp.StatusCode)
	}

	var result struct {
		Files []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"files"`
		VectorSearch bool `json:"vector_search"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	t.Logf("vector search (with space) result count: %d (vector_search=%v)", len(result.Files), result.VectorSearch)
	for _, f := range result.Files {
		t.Logf("  - %s (%s)", f.Name, f.Path)
	}

	found := false
	for _, f := range result.Files {
		if f.Name == "在线播放-高清视频.mp4" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("❌ BUG: vector search '在线 高清' should match '在线播放-高清视频.mp4'")
	}
}

// TestSearch_VectorSearch_ReturnsScore 验证向量搜索 fallback 路径返回 score 字段，
// 前端可据此显示相关度。
//
// 场景：搜索 "在线高清"（无空格）触发向量 fallback，结果应含 score > 0。
func TestSearch_VectorSearch_ReturnsScore(t *testing.T) {
	s, baseURL, teardown := setupMultiTokenSearchServer(t)
	defer teardown()

	s.mobileSvc.RebuildIndex()
	waitForIndexReady(s)

	searchURL := fmt.Sprintf("%s/api/search/files?q=%s&path=%s&recursive=true&limit=50",
		baseURL,
		url.QueryEscape("在线高清"),
		url.QueryEscape("/d"))

	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("GET vector search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("vector search status = %d, want 200", resp.StatusCode)
	}

	var result struct {
		Files []struct {
			Name  string  `json:"name"`
			Path  string  `json:"path"`
			Score float64 `json:"score"`
		} `json:"files"`
		VectorSearch bool `json:"vector_search"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	t.Logf("vector search result count: %d (vector_search=%v)", len(result.Files), result.VectorSearch)
	for _, f := range result.Files {
		t.Logf("  - %s score=%.4f", f.Name, f.Score)
	}

	if !result.VectorSearch {
		t.Fatal("vector_search should be true when fallback path is used")
	}

	// 找到目标文件并验证 score > 0
	found := false
	for _, f := range result.Files {
		if f.Name == "在线播放-高清视频.mp4" {
			found = true
			if f.Score <= 0 {
				t.Errorf("score should be > 0 for vector search result, got %f", f.Score)
			} else {
				t.Logf("✅ score=%.4f for target file", f.Score)
			}
			break
		}
	}
	if !found {
		t.Error("target file not found in vector search results")
	}
}
