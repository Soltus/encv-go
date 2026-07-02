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

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/pkg/encv/plugins"
)

// setupSearchModeTestServer 创建含 fileCount 个 "testNN.txt" 文件的搜索测试 server。
// 用于测试 handleVectorSearchFilesGin 的 search_mode 四模式（strict/combined/greedy/none）。
//
// 文件直接放在挂载点根目录（tmpDir），搜索 "test" 关键词能匹配全部 fileCount 个文件。
// 通过 fileCount 控制关键词匹配数量，触发不同 search_mode：
//   - fileCount >= 20 → strict 模式（关键词匹配 >= 20）
//   - 1 <= fileCount < 20 → combined 模式（关键词匹配 1~19，向量重排序）
//   - fileCount == 0 + 无关键词匹配 → greedy 模式（需配合无子串匹配的查询）
func setupSearchModeTestServer(t *testing.T, fileCount int) (*Server, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "encv-search-mode-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	oldMountsFile := os.Getenv("ENCV_MOUNTS_FILE")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)

	// 在挂载点根目录创建 fileCount 个 testNN.txt 文件
	for i := 1; i <= fileCount; i++ {
		name := fmt.Sprintf("test%02d.txt", i)
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("test content"), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("WriteFile %s: %v", name, err)
		}
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

// searchModeResult 是 /api/search/files 响应的解析结构，包含 search_mode 字段。
type searchModeResult struct {
	Files []struct {
		Name  string  `json:"name"`
		Path  string  `json:"path"`
		Score float64 `json:"score"`
	} `json:"files"`
	VectorSearch bool   `json:"vector_search"`
	SearchMode   string `json:"search_mode"`
	Total        int    `json:"total"`
}

// doSearchModeRequest 发起一次 /api/search/files 请求并解析响应。
func doSearchModeRequest(t *testing.T, baseURL, q, path string, recursive bool, limit string) searchModeResult {
	t.Helper()
	u := fmt.Sprintf("%s/api/search/files?q=%s&path=%s&recursive=%t",
		baseURL, url.QueryEscape(q), url.QueryEscape(path), recursive)
	if limit != "" {
		u += "&limit=" + limit
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("search status = %d, want 200", resp.StatusCode)
	}
	var result searchModeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return result
}

// TestSearchMode_None_EmptyQuery 验证空查询返回 search_mode == "none"。
//
// 触发条件：query == "" → handleVectorSearchFilesGin 直接返回 none 模式，不进入搜索逻辑。
// 这是 handleVectorSearchFilesGin 的第一个分支（行 2511-2514）。
func TestSearchMode_None_EmptyQuery(t *testing.T) {
	// 用例 1：q=（空字符串）→ search_mode == "none", vector_search == false, files == []
	t.Run("empty_q_param", func(t *testing.T) {
		_, baseURL, teardown := setupSearchTestServer(t)
		defer teardown()

		result := doSearchModeRequest(t, baseURL, "", "/d", true, "")
		if result.SearchMode != "none" {
			t.Errorf("search_mode = %q, want %q", result.SearchMode, "none")
		}
		if result.VectorSearch {
			t.Errorf("vector_search = true, want false")
		}
		if len(result.Files) != 0 {
			t.Errorf("files count = %d, want 0 (空查询不应返回文件)", len(result.Files))
		}
		t.Logf("空查询正确返回 none 模式, files=%d", len(result.Files))
	})

	// 用例 2：无 q 参数（c.Query("q") 返回 ""）→ search_mode == "none"
	t.Run("missing_q_param", func(t *testing.T) {
		_, baseURL, teardown := setupSearchTestServer(t)
		defer teardown()

		// 不带 q 参数直接请求
		resp, err := http.Get(fmt.Sprintf("%s/api/search/files?path=%s&recursive=true",
			baseURL, url.QueryEscape("/d")))
		if err != nil {
			t.Fatalf("GET search: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("search status = %d, want 200", resp.StatusCode)
		}
		var result searchModeResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if result.SearchMode != "none" {
			t.Errorf("search_mode = %q, want %q", result.SearchMode, "none")
		}
		if result.VectorSearch {
			t.Errorf("vector_search = true, want false")
		}
		if len(result.Files) != 0 {
			t.Errorf("files count = %d, want 0", len(result.Files))
		}
		t.Logf("缺失 q 参数正确返回 none 模式, files=%d", len(result.Files))
	})
}

// TestSearchMode_Strict_ManyKeywordMatches 验证关键词匹配 >= 20 时返回 strict 模式。
//
// 触发条件：len(files) >= strictThreshold(20) → strict 模式，只返回关键词结果，不做向量重排序。
// 这是 handleVectorSearchFilesGin 的第二个分支（行 2531-2540）。
func TestSearchMode_Strict_ManyKeywordMatches(t *testing.T) {
	// 用例 1：25 个文件含 "test" 关键词 → 25 匹配 >= 20 → strict 模式
	t.Run("twenty_five_matches", func(t *testing.T) {
		s, baseURL, teardown := setupSearchModeTestServer(t, 25)
		defer teardown()

		// 构建索引确保搜索能找到全部 25 个文件
		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		result := doSearchModeRequest(t, baseURL, "test", "/d", true, "")
		if result.SearchMode != "strict" {
			t.Errorf("search_mode = %q, want %q (25 个关键词匹配应走 strict)", result.SearchMode, "strict")
		}
		if result.VectorSearch {
			t.Errorf("vector_search = true, want false (strict 模式不做向量重排序)")
		}
		if len(result.Files) < 20 {
			t.Errorf("files count = %d, want >= 20 (strict 模式应返回全部关键词匹配)", len(result.Files))
		}
		t.Logf("25 个匹配正确返回 strict 模式, files=%d, total=%d", len(result.Files), result.Total)
	})

	// 用例 2：20 个文件（边界值 >= 20）→ strict 模式
	t.Run("twenty_matches_boundary", func(t *testing.T) {
		s, baseURL, teardown := setupSearchModeTestServer(t, 20)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		result := doSearchModeRequest(t, baseURL, "test", "/d", true, "")
		if result.SearchMode != "strict" {
			t.Errorf("search_mode = %q, want %q (20 个关键词匹配 = 阈值边界应走 strict)", result.SearchMode, "strict")
		}
		if result.VectorSearch {
			t.Errorf("vector_search = true, want false (strict 模式不做向量重排序)")
		}
		if len(result.Files) < 20 {
			t.Errorf("files count = %d, want >= 20", len(result.Files))
		}
		t.Logf("20 个匹配（边界）正确返回 strict 模式, files=%d", len(result.Files))
	})
}

// TestSearchMode_Combined_FewMatches 验证关键词匹配 1~19 时返回 combined 模式。
//
// 触发条件：1 <= len(files) < 20 且 searchSvc != nil 且向量重排序成功 → combined 模式。
// 这是 handleVectorSearchFilesGin 的第三个分支（行 2543-2598）。
func TestSearchMode_Combined_FewMatches(t *testing.T) {
	// 用例 1：5 个文件含 "test" → 5 匹配（1~19 区间）→ combined 模式
	t.Run("five_matches_test_files", func(t *testing.T) {
		s, baseURL, teardown := setupSearchModeTestServer(t, 5)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		// 若向量搜索服务不可用（searchSvc == nil），combined 分支会被跳过
		if s.searchSvc == nil {
			t.Skip("searchSvc == nil：向量搜索服务未初始化，combined 模式无法触发，跳过此用例")
		}

		result := doSearchModeRequest(t, baseURL, "test", "/d", true, "")
		if result.SearchMode != "combined" {
			t.Errorf("search_mode = %q, want %q (5 个关键词匹配应走 combined)", result.SearchMode, "combined")
		}
		if !result.VectorSearch {
			t.Errorf("vector_search = false, want true (combined 模式启用了向量重排序)")
		}
		if len(result.Files) < 1 {
			t.Errorf("files count = %d, want >= 1", len(result.Files))
		}
		t.Logf("5 个匹配正确返回 combined 模式, files=%d, vector_search=%v", len(result.Files), result.VectorSearch)
	})

	// 用例 2：用多 token 中文文件名验证 combined 模式
	// setupMultiTokenSearchServer 含 "在线播放-高清视频.mp4"，搜索 "在线 高清"（空格分隔）
	// 关键词 AND 匹配 → 1 个匹配（1~19 区间）→ combined 模式
	t.Run("one_match_chinese_multitoken", func(t *testing.T) {
		s, baseURL, teardown := setupMultiTokenSearchServer(t)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		if s.searchSvc == nil {
			t.Skip("searchSvc == nil：向量搜索服务未初始化，combined 模式无法触发，跳过此用例")
		}

		result := doSearchModeRequest(t, baseURL, "在线 高清", "/d", true, "")
		if result.SearchMode != "combined" {
			t.Errorf("search_mode = %q, want %q (1 个关键词匹配应走 combined)", result.SearchMode, "combined")
		}
		if !result.VectorSearch {
			t.Errorf("vector_search = false, want true (combined 模式启用了向量重排序)")
		}
		if len(result.Files) < 1 {
			t.Errorf("files count = %d, want >= 1", len(result.Files))
		}
		t.Logf("1 个中文匹配正确返回 combined 模式, files=%d, vector_search=%v", len(result.Files), result.VectorSearch)
	})
}

// TestSearchMode_Greedy_NoKeywordMatch 验证关键词无匹配时返回 greedy 模式。
//
// 触发条件：len(files) == 0 且 searchSvc != nil 且 vectorSearchFallback 返回非空 → greedy 模式。
// 这是 handleVectorSearchFilesGin 的第四个分支（行 2611-2622）。
func TestSearchMode_Greedy_NoKeywordMatch(t *testing.T) {
	// 用例 1：搜索 "在线视频"（无子串匹配）→ 0 关键词匹配 → greedy fallback
	// "在线视频" 不是 "在线播放-高清视频.mp4" 的子串，但向量 bigram 共享 "在线"+"视频"
	t.Run("zaixian_shipin_greedy", func(t *testing.T) {
		s, baseURL, teardown := setupMultiTokenSearchServer(t)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		if s.searchSvc == nil {
			t.Skip("searchSvc == nil：向量搜索服务未初始化，greedy 模式无法触发，跳过此用例")
		}

		result := doSearchModeRequest(t, baseURL, "在线视频", "/d", true, "")
		if result.SearchMode != "greedy" {
			t.Errorf("search_mode = %q, want %q (0 关键词匹配应走 greedy)", result.SearchMode, "greedy")
		}
		if !result.VectorSearch {
			t.Errorf("vector_search = false, want true (greedy 模式启用了向量 fallback)")
		}
		if len(result.Files) < 1 {
			t.Errorf("files count = %d, want >= 1 (greedy fallback 应返回向量匹配结果)", len(result.Files))
		}
		t.Logf("0 关键词匹配正确返回 greedy 模式, files=%d, vector_search=%v", len(result.Files), result.VectorSearch)
	})

	// 用例 2：搜索 "在线高清"（无空格，无子串匹配）→ 0 关键词匹配 → greedy fallback
	// "在线高清" 不是 "在线播放-高清视频.mp4" 的子串，但向量 bigram 共享 "在线"+"高清"
	t.Run("zaixian_gaoqing_greedy", func(t *testing.T) {
		s, baseURL, teardown := setupMultiTokenSearchServer(t)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		if s.searchSvc == nil {
			t.Skip("searchSvc == nil：向量搜索服务未初始化，greedy 模式无法触发，跳过此用例")
		}

		result := doSearchModeRequest(t, baseURL, "在线高清", "/d", true, "")
		if result.SearchMode != "greedy" {
			t.Errorf("search_mode = %q, want %q (0 关键词匹配应走 greedy)", result.SearchMode, "greedy")
		}
		if !result.VectorSearch {
			t.Errorf("vector_search = false, want true (greedy 模式启用了向量 fallback)")
		}
		if len(result.Files) < 1 {
			t.Errorf("files count = %d, want >= 1", len(result.Files))
		}
		t.Logf("0 关键词匹配正确返回 greedy 模式, files=%d, vector_search=%v", len(result.Files), result.VectorSearch)
	})
}

// TestSearchMode_LimitParameter 验证 limit 参数不会导致 panic，且边界值正确处理。
//
// limit 参数处理逻辑（行 2504-2509）：
//   - limit 为空 → 默认 50
//   - limit 可解析为正整数 → 使用该值
//   - limit=0 → Atoi 成功但 n>0 检查失败 → 保持默认 50
//   - limit 为非数字 → Atoi 失败 → 保持默认 50
func TestSearchMode_LimitParameter(t *testing.T) {
	// 用例 1：limit=5 → 不 panic，返回结果
	t.Run("limit_five", func(t *testing.T) {
		s, baseURL, teardown := setupSearchModeTestServer(t, 25)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		// 25 个匹配 >= 20 → strict 模式，limit 不影响 strict 路径
		result := doSearchModeRequest(t, baseURL, "test", "/d", true, "5")
		if result.SearchMode != "strict" {
			t.Logf("note: search_mode = %q (25 匹配应走 strict, limit 不影响 strict 路径)", result.SearchMode)
		}
		if len(result.Files) == 0 {
			t.Errorf("files count = 0, want > 0 (limit=5 不应导致无结果)")
		}
		t.Logf("limit=5 不 panic, files=%d, search_mode=%s", len(result.Files), result.SearchMode)
	})

	// 用例 2：limit=0 → Atoi 成功但 n>0 检查失败 → 用默认 50，不 panic
	t.Run("limit_zero_fallback_default", func(t *testing.T) {
		s, baseURL, teardown := setupSearchModeTestServer(t, 25)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		result := doSearchModeRequest(t, baseURL, "test", "/d", true, "0")
		// limit=0 不会 panic，且 25 匹配 >= 20 → strict 模式
		if len(result.Files) == 0 {
			t.Errorf("files count = 0, want > 0 (limit=0 应 fallback 到默认 50, 不应导致无结果)")
		}
		t.Logf("limit=0 不 panic, files=%d, search_mode=%s", len(result.Files), result.SearchMode)
	})
}

// TestSearchMode_SearchSvcNil_AllModesFallback 验证 searchSvc == nil 时
// combined 和 greedy 分支被跳过，统一落到兜底 none 模式。
//
// 这是降级场景测试（见 .trae/rules/graceful-degradation.md）：
// 向量搜索不可用时，combined（向量重排序）和 greedy（向量 fallback）都无法触发，
// 1~19 个关键词匹配和 0 个关键词匹配都落到最后的兜底分支 → search_mode == "none"。
func TestSearchMode_SearchSvcNil_AllModesFallback(t *testing.T) {
	// 用例 1：searchSvc == nil + 5 个关键词匹配 → 跳过 combined → 兜底 none 模式
	t.Run("few_matches_searchsvc_nil_fallback_none", func(t *testing.T) {
		s, baseURL, teardown := setupSearchModeTestServer(t, 5)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		// 手动置 nil 模拟向量搜索不可用的降级场景
		originalSvc := s.searchSvc
		s.searchSvc = nil
		defer func() { s.searchSvc = originalSvc }()

		result := doSearchModeRequest(t, baseURL, "test", "/d", true, "")
		// 5 个匹配（1~19）但 searchSvc == nil → 跳过 combined → 兜底 none
		if result.SearchMode != "none" {
			t.Errorf("search_mode = %q, want %q (searchSvc==nil 时 1~19 匹配应跳过 combined 落到 none)",
				result.SearchMode, "none")
		}
		if result.VectorSearch {
			t.Errorf("vector_search = true, want false (searchSvc==nil 时不应启用向量搜索)")
		}
		// 兜底分支返回关键词搜索结果（至少 5 个 test 文件，可能含 automation 挂载点的额外匹配）
		if len(result.Files) < 5 {
			t.Errorf("files count = %d, want >= 5 (兜底 none 模式应返回关键词搜索结果)", len(result.Files))
		}
		t.Logf("searchSvc==nil + 5 匹配正确降级到 none 模式, files=%d", len(result.Files))
	})

	// 用例 2：searchSvc == nil + 0 个关键词匹配 → 跳过 greedy → 兜底 none 模式
	t.Run("zero_matches_searchsvc_nil_fallback_none", func(t *testing.T) {
		s, baseURL, teardown := setupMultiTokenSearchServer(t)
		defer teardown()

		s.mobileSvc.RebuildIndex()
		waitForIndexReady(s)

		// 手动置 nil 模拟向量搜索不可用的降级场景
		originalSvc := s.searchSvc
		s.searchSvc = nil
		defer func() { s.searchSvc = originalSvc }()

		// "在线视频" 无子串匹配 → 0 关键词匹配，searchSvc==nil → 跳过 greedy → 兜底 none
		result := doSearchModeRequest(t, baseURL, "在线视频", "/d", true, "")
		if result.SearchMode != "none" {
			t.Errorf("search_mode = %q, want %q (searchSvc==nil 时 0 匹配应跳过 greedy 落到 none)",
				result.SearchMode, "none")
		}
		if result.VectorSearch {
			t.Errorf("vector_search = true, want false (searchSvc==nil 时不应启用向量搜索)")
		}
		// 0 关键词匹配 + 无向量 fallback → 兜底返回空列表
		if len(result.Files) != 0 {
			t.Errorf("files count = %d, want 0 (0 匹配 + searchSvc==nil 应返回空)", len(result.Files))
		}
		t.Logf("searchSvc==nil + 0 匹配正确降级到 none 模式, files=%d", len(result.Files))
	})
}
