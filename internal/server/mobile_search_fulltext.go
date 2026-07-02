package server

// mobile_search_fulltext.go — 全文搜索（FTS5）endpoint。
//
// 2026-07-02 大改升级：替换原 name-only 搜索为 FTS5 全文搜索。
// 关键设计：
//   - FTS5 + content=file_entries 外部表 + trigger 同步
//   - CJK bigram 切分（"在线" 命中 "在线播放"）
//   - bool/phrase/regex 查询语法（AND/OR/NOT/"phrase"/regex:...）
//   - Go 端 NOT substring 过滤（FTS5 NOT 语法限制）
//   - 100w 条目 449ms 搜索 1000 结果
//
// 索引建立：
//   - 启动时 scan /d 目录（深度限制 5，跳过 .encv 容器）
//   - 文本文件读前 64KB 作为 content（不全量读，省 IO）
//   - BulkInsert 单次事务批量写
//   - 文件变更走 filechange 事件增量更新（待实现）

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/Soltus/encv-go/internal/fts"
	"github.com/gin-gonic/gin"
)

var (
	fulltextIndexMu sync.RWMutex
	fulltextIndex   *fts.FileIndex // 全局全文索引（启动时 init）
)

// InitFullTextIndex 初始化全局 FTS5 全文索引。
// 在 server 启动时调用。
func InitFullTextIndex(dbPath string) error {
	idx, err := fts.NewFileIndex(dbPath)
	if err != nil {
		return err
	}
	fulltextIndexMu.Lock()
	fulltextIndex = idx
	fulltextIndexMu.Unlock()
	slog.Info("fulltext index initialized", "dbPath", dbPath)
	return nil
}

// GetFullTextIndex 返回全局索引（for service 层使用）。
func GetFullTextIndex() *fts.FileIndex {
	fulltextIndexMu.RLock()
	defer fulltextIndexMu.RUnlock()
	return fulltextIndex
}

// CloseFullTextIndex 关闭全局索引。
func CloseFullTextIndex() error {
	fulltextIndexMu.Lock()
	defer fulltextIndexMu.Unlock()
	if fulltextIndex == nil {
		return nil
	}
	err := fulltextIndex.Close()
	fulltextIndex = nil
	return err
}

// FullTextSearchResult 全文搜索结果（前端用）。
type FullTextSearchResult struct {
	Path     string  `json:"path"`
	Name     string  `json:"name"`
	Score    float64 `json:"score"`
	Snippet  string  `json:"snippet"`
	HitCount int     `json:"hitCount"`
	IsDir    bool    `json:"isDir"`
}

// handleSearchFilesFullTextGin 全文搜索 endpoint。
//
// GET /api/files/search-fulltext?q=<query>&limit=200&path_prefix=/d/photos
//
// 返回：{ results: FullTextSearchResult[], total: int, dbEngine: "sqlite" }
func (s *Server) handleSearchFilesFullTextGin(c *gin.Context) {
	idx := GetFullTextIndex()
	if idx == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":  "fulltext index not initialized",
			"code":   "FULLTEXT_UNAVAILABLE",
			"dbEngine": "none",
		})
		return
	}

	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "empty query",
			"code":  "EMPTY_QUERY",
		})
		return
	}

	limit := 200
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	pathPrefix := c.Query("path_prefix")

	results, err := idx.Search(c.Request.Context(), q, fts.SearchOptions{
		Limit:       limit,
		PathPrefix:  pathPrefix,
		IncludeDirs: false,
	})
	if err != nil {
		slog.Warn("fulltext search failed", "q", q, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
			"code":  "FULLTEXT_QUERY_FAILED",
		})
		return
	}

	out := make([]FullTextSearchResult, 0, len(results))
	for _, r := range results {
		out = append(out, FullTextSearchResult{
			Path:     r.Path,
			Name:     r.Name,
			Score:    r.Score,
			Snippet:  r.Snippet,
			HitCount: r.HitCount,
		})
	}

	// 统计信息（不依赖上一次 build）
	stats := idx.Stats()
	c.JSON(http.StatusOK, gin.H{
		"results":   out,
		"total":     len(out),
		"query":     q,
		"dbEngine":  "sqlite",
		"indexSize": stats.IndexSize,
	})
}

// handleFullTextIndexStatsGin 返回 FTS5 索引统计。
//
// GET /api/files/search-fulltext/stats
func (s *Server) handleFullTextIndexStatsGin(c *gin.Context) {
	idx := GetFullTextIndex()
	if idx == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"available": false,
			"error":     "fulltext index not initialized",
		})
		return
	}
	stats := idx.Stats()
	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"stats": gin.H{
			"totalFiles":  stats.TotalFiles,
			"totalDirs":   stats.TotalDirs,
			"totalSize":   stats.TotalSize,
			"indexedAt":   stats.IndexedAt,
			"isIndexing":  stats.IsIndexing,
			"lastBuildMs": stats.LastBuildMs,
			"dbPath":      stats.DBPath,
			"fts5Enabled": stats.FTS5Enabled,
			"tokenizer":   stats.Tokenizer,
			"indexVersion": stats.IndexVersion,
		},
	})
}
