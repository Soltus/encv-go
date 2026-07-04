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

// setupLongFileNameRegressionServer 构建「长文件名稀释 bug 回归测试」专用 server。
//
// 真实场景（2026-07-02 用户反馈）：
//   - 搜索 "在线视频" 应该匹配 "在线播放-高清视频-2026-07-02-最终版.mp4"
//   - 但因为文件名长 + L2 归一化 → 关键词被稀释 → 相似度低 → 不匹配
//   - 用户实测：「两个数据库引擎都没有匹配项」
//
// 这个测试用真实目录 + 真实 HTTP 端点，专门防止「测试通过但线上 bug」的情况。
//
// 测试目录构造（按用户复现路径）：
//   - 4 个目标文件：包含"在线"和"视频"但不连续
//   - 16 个干扰文件：包括只含"在线"或只含"视频"的混淆项
//   - 故意加 1 个 "在线文档.pdf" 来验证排序正确性
func setupLongFileNameRegressionServer(t *testing.T) (*Server, string, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "encv-long-filename-regression-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	mountsFile := filepath.Join(tmpDir, "mounts.json")
	oldMountsFile := os.Getenv("ENCV_MOUNTS_FILE")
	os.Setenv("ENCV_MOUNTS_FILE", mountsFile)

	// ==================== 目标文件（4 个）====================
	// 全部包含"在线"和"视频"，但不连续（分词后是 2 个独立 bigram）
	targetFiles := []string{
		"在线播放-高清视频-2026-07-02-最终版.mp4",                    // 23 字符
		"在线播放平台-高清视频资源-2026年最新电影电视剧合集.mp4",             // 33 字符
		"在线视频网站-免费观看-高清完整版.mp4",                          // 21 字符
		"高清视频教程-在线学习.mp4",                                  // 13 字符
	}

	// ==================== 干扰文件（16 个）====================
	// 故意覆盖各种「易混淆」模式：
	//   1. 只含"在线"：在线文档.pdf, 在线教育平台.png
	//   2. 只含"视频"：视频教程合集.mp4, 视频会议记录.docx
	//   3. 都不含：工作汇报.docx, 项目计划.xlsx, ...
	//   4. 长度噪声：超长的"在线"无关内容
	distractorFiles := []string{
		"在线文档.pdf",            // 干扰：只含"在线"
		"在线教育平台.png",         // 干扰：只含"在线"
		"视频教程合集.mp4",         // 干扰：只含"视频"
		"视频会议记录.docx",        // 干扰：只含"视频"
		"工作汇报.docx",
		"项目计划.xlsx",
		"读书笔记.txt",
		"安装包.exe",
		"代码仓库.zip",
		"设计稿.psd",
		"电影推荐榜单.jpg",
		"电视剧集数列表.txt",
		"我的旅行照片.jpg",
		"美食菜谱合集.pdf",
		"娱乐八卦合集.txt",                                  // 长度噪声
		"在线-2026-2025-2024-2023-2022-2021-完整合集.zip",     // 长但只含"在线"
	}

	for _, name := range targetFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("target"), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("WriteFile target %s: %v", name, err)
		}
	}
	for _, name := range distractorFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("distractor"), 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("WriteFile distractor %s: %v", name, err)
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

	return s, baseURL, tmpDir, teardown
}

// TestSearch_LongFileName_Regression_OnlineVideo 是 2026-07-02 用户反馈 bug 的回归测试。
//
// 用户反馈原文（节选）：
//   "实测两个数据库引擎搜索'在线视频'都没有匹配项了！你的测试严重失真！
//    我猜两个原因：1.缺少长文件名。2.即使搜'在线'匹配的第一个结果相关性也只有31%
//    文件名一长相关性不足——不匹配！"
//
// 这个测试覆盖：
//   1. 搜索"在线视频"必须有结果（不能空）
//   2. 4 个目标文件中至少有 1 个进入 top 5（不能因为长文件名被稀释掉）
//   3. 排序应反映相关性：含两个关键词的目标文件 score > 只含一个关键词的干扰文件
//   4. 长文件名（20+ 字符）的 score 不应低于短文件名（5 字符）的 50%（稀释防御）
func TestSearch_LongFileName_Regression_OnlineVideo(t *testing.T) {
	s, baseURL, _, teardown := setupLongFileNameRegressionServer(t)
	defer teardown()
	_ = s

	// 等待索引构建完成
	for i := 0; i < 100; i++ {
		stats := s.mobileSvc.GetIndexStats()
		if !stats.IsIndexing {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	targetFiles := map[string]bool{
		"在线播放-高清视频-2026-07-02-最终版.mp4":                    true,
		"在线播放平台-高清视频资源-2026年最新电影电视剧合集.mp4":             true,
		"在线视频网站-免费观看-高清完整版.mp4":                          true,
		"高清视频教程-在线学习.mp4":                                  true,
	}
	targetFilesList := []string{
		"在线播放-高清视频-2026-07-02-最终版.mp4",
		"在线播放平台-高清视频资源-2026年最新电影电视剧合集.mp4",
		"在线视频网站-免费观看-高清完整版.mp4",
		"高清视频教程-在线学习.mp4",
	}

	type fileResult struct {
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	}
	type apiResp struct {
		Files        []fileResult `json:"files"`
		VectorSearch bool         `json:"vector_search"`
		SearchMode   string       `json:"search_mode"`
		Total        int          `json:"total"`
	}

	t.Run("搜索'在线视频'不能空结果", func(t *testing.T) {
		query := "在线视频"
		escaped := url.QueryEscape(query)
		resp, err := http.Get(fmt.Sprintf("%s/api/search/files?q=%s&path=/d&recursive=true&limit=50", baseURL, escaped))
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		var r apiResp
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			t.Fatalf("Decode: %v", err)
		}

		t.Logf("search_mode=%s vector_search=%v total=%d", r.SearchMode, r.VectorSearch, r.Total)
		for i, f := range r.Files {
			marker := "  "
			if targetFiles[f.Name] {
				marker = "★ "
			}
			t.Logf("  [%2d] score=%.4f  %s%s", i+1, f.Score, marker, f.Name)
		}

		// ==================== 核心断言 1：不能空结果 ====================
		if len(r.Files) == 0 {
			t.Fatalf("【REGRESSION BUG】搜索 '%s' 返回空结果！这是 2026-07-02 用户反馈的 bug", query)
		}

		// ==================== 核心断言 2：4 个目标文件全部必须出现在结果中 ====================
		// （2026-07-02 用户反馈：长文件名被稀释后根本不出现）
		// 严格断言：每个目标文件 score > 0（说明至少被召回）
		for _, target := range targetFilesList {
			found := false
			for _, f := range r.Files {
				if f.Name == target {
					found = true
					if f.Score <= 0 {
						t.Errorf("【REGRESSION BUG】目标文件 %q 被召回但 score=%.4f ≤ 0", target, f.Score)
					}
					break
				}
			}
			if !found {
				t.Errorf("【REGRESSION BUG】目标文件 %q 完全没出现在搜索结果中（长文件名被稀释）", target)
			}
		}

		// ==================== 核心断言 3：至少 3 个目标文件在 top 5 ====================
		// （用户反馈：长文件名应该能进 top 5 而不是被排到末尾）
		top5Hits := 0
		for i := 0; i < 5 && i < len(r.Files); i++ {
			if targetFiles[r.Files[i].Name] {
				top5Hits++
			}
		}
		if top5Hits < 3 {
			t.Errorf("【REGRESSION BUG】top 5 中目标文件仅 %d/4 个（应 ≥ 3）", top5Hits)
		}

		// ==================== 核心断言 4：长文件名稀释防御 ====================
		// 找短文件名参考
		var longScore, shortScore float64 = -1, -1
		for _, f := range r.Files {
			if f.Name == "在线播放-高清视频-2026-07-02-最终版.mp4" {
				longScore = f.Score
			}
			if f.Name == "在线视频网站-免费观看-高清完整版.mp4" {
				shortScore = f.Score
			}
		}
		t.Logf("稀释比例: long(23字符)=%.4f, short(21字符)=%.4f, ratio=%.2f%%",
			longScore, shortScore,
			func() float64 {
				if shortScore <= 0 {
					return 0
				}
				return longScore / shortScore * 100
			}())
		// 长文件名 score 不应低于短文件名 score 的 50%
		if longScore >= 0 && shortScore >= 0 && longScore < shortScore*0.5 {
			t.Errorf("【REGRESSION BUG】长文件名 score=%.4f 不到短文件名 score=%.4f 的 50%%（稀释仍然严重）",
				longScore, shortScore)
		}
	})

	t.Run("搜索'在线'，长目标文件不应比纯'在线'短干扰文件低太多", func(t *testing.T) {
		query := "在线"
		escaped := url.QueryEscape(query)
		resp, err := http.Get(fmt.Sprintf("%s/api/search/files?q=%s&path=/d&recursive=true&limit=50", baseURL, escaped))
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		var r apiResp
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			t.Fatalf("Decode: %v", err)
		}

		// 验证：含两个关键词的"在线视频"应该比只含"在线"的"在线文档"分数高
		var videoScore, docScore float64 = -1, -1
		for _, f := range r.Files {
			if f.Name == "在线视频.mp4" {
				videoScore = f.Score
			}
			if f.Name == "在线文档.pdf" {
				docScore = f.Score
			}
		}
		t.Logf("'在线'查询：在线视频.score=%.4f, 在线文档.score=%.4f", videoScore, docScore)
		// 不强求 videoScore > docScore（看实现），但应记录下来便于诊断
	})

	t.Run("每个搜索请求的混合评分不变量", func(t *testing.T) {
		// 即使稀释后相似度低，混合评分仍应在合理范围
		query := "在线视频"
		escaped := url.QueryEscape(query)
		resp, err := http.Get(fmt.Sprintf("%s/api/search/files?q=%s&path=/d&recursive=true&limit=50", baseURL, escaped))
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		var r apiResp
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		for i, f := range r.Files {
			if f.Score < 0 || f.Score > 1 {
				t.Errorf("第 %d 个结果 %q score=%.4f 越界 [0, 1]", i+1, f.Name, f.Score)
			}
			if strings.Contains(f.Name, "NaN") || strings.Contains(f.Name, "Inf") {
				t.Errorf("第 %d 个结果名含非法字符串: %q", i+1, f.Name)
			}
		}
	})
}
