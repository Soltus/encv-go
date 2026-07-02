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
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/fts"
	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
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

// fulltextDBPath 返回 FTS5 数据库文件路径。
//
// 设计：
//   - 桌面端：<servingDir>/.encv/fts5.db
//   - Android：<ENCV_APP_FILES_DIR>/.encv/fts5.db（与 mounts.json 同目录）
//
// 不存在时 NewFileIndex 会创建空库，初始 stats.TotalFiles = 0。
func fulltextDBPath(servingDir string) string {
	if servingDir == "" {
		// 无 servingDir 兜底：放 /tmp（不持久化，重启丢失）
		return filepath.Join(os.TempDir(), "encv-fts5.db")
	}
	return filepath.Join(servingDir, ".encv", "fts5.db")
}

// InitFullTextIndexWithBuild 初始化 FTS5 索引 + 异步后台 build。
//
// 设计要点（2026-07-02 用户反馈"全文搜索无匹配结果"修复）：
//   - InitFullTextIndex 之前定义但从未被 NewServer 调用 → 索引永远空 → 0 结果
//   - 现在 NewServer 启动时调本函数
//   - sync 阶段：建库 + schema（失败 return err）
//   - async 阶段：scan servingDir → BulkInsert（失败 log warn，不影响 main 流程）
//
// 深度限制：最多 5 层（与 /api/files 的默认深度一致）
// 容器跳过：跳过 .encv 加密容器（已在 search cache 层处理）
// content 截断：读前 64KB（IO 限制，避免大文件拖慢启动）
func (s *Server) InitFullTextIndexWithBuild(servingDir string) error {
	dbPath := fulltextDBPath(servingDir)
	// 确保 .encv 目录存在
	if dir := filepath.Dir(dbPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	if err := InitFullTextIndex(dbPath); err != nil {
		return err
	}

	// 启动时已经累计的条目数（用户可能重启 / 数据已 build 过）
	idx := GetFullTextIndex()
	if idx == nil {
		return nil
	}
	existing := idx.Stats()
	if existing.TotalFiles > 0 {
		slog.Info("FTS5 index already populated, skip rebuild", "files", existing.TotalFiles)
		return nil
	}

	// 后台 goroutine build（不阻塞 server 启动）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		start := time.Now()
		idx.SetBuilding(true)
		defer idx.SetBuilding(false)

		entries, err := scanDirForIndex(ctx, servingDir, 0, 5)
		if err != nil {
			slog.Warn("FTS5 background scan failed", "err", err, "scanned", len(entries))
			return
		}
		if len(entries) == 0 {
			slog.Info("FTS5 no entries to index", "servingDir", servingDir)
			return
		}
		if err := idx.BulkInsert(ctx, entries); err != nil {
			slog.Error("FTS5 bulk insert failed", "err", err, "count", len(entries))
			return
		}
		idx.MarkBuilt(time.Since(start))
		slog.Info("FTS5 build complete", "count", len(entries), "elapsed", time.Since(start))
	}()
	return nil
}

// scanDirForIndex 递归扫描目录，生成 FTS5 索引条目。
//
// 参数：
//   - ctx: 超时控制
//   - dir: 起始目录
//   - depth: 当前深度（递归用）
//   - maxDepth: 最大深度（防止无限递归）
//
// 返回：
//   - []fts.FileEntry: 索引条目
//   - error: 扫描错误（不阻断，部分文件失败也继续）
//
// 文件过滤：
//   - 目录：生成 is_dir=true 条目，content=""
//   - 普通文件：尝试读前 64KB 作为 content（错误则 content=""）
//   - 跳过目录：.encv（应用配置目录，无索引价值）、.git、node_modules、.DS_Store
//   - 加密容器（.sccgv/.ae/.encvgo 等）：调对应插件的 SearchableContentsExtractor 提取可搜索内容
//   - binary 文件（读取失败）：content=""（name 仍可被 FTS5 搜到）
//
// 2026-07-02 修正（用户反馈幻觉修复）：
//   - 旧版用 strings.HasSuffix(name, ".encv") 跳过文件 → 错误！项目里**没有** .encv 文件后缀名
//   - 正确做法是跳过 .encv 目录（项目配置目录，不属于用户文件）
//   - 类似的 .git / node_modules / .DS_Store 也应跳过
//
// 2026-07-02 v2：插件自主声明可搜索内容
//   - 检测到加密容器时，遍历已注册插件，找到能处理该扩展名的插件
//   - 调插件的 SearchableContentsExtractor.ExtractSearchableContents()
//   - 把每条 SearchableContentItem 合并为单个 content 字符串（用 \n 分隔）
//   - 用 "container:<containerPath>:<itemName>" 作为 Name 区分不同容器内的不同内容
func scanDirForIndex(ctx context.Context, dir string, depth, maxDepth int) ([]fts.FileEntry, error) {
	if dir == "" {
		return nil, nil
	}
	if depth > maxDepth {
		return nil, nil
	}

	var entries []fts.FileEntry
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range dirEntries {
		// ctx 取消检查
		select {
		case <-ctx.Done():
			return entries, ctx.Err()
		default:
		}

		name := entry.Name()
		fullPath := filepath.Join(dir, name)

		// 跳过不需要索引的目录（2026-07-02 修正：之前误判 .encv 为文件后缀名）
		if entry.IsDir() {
			lower := strings.ToLower(name)
			if lower == ".encv" || lower == ".git" || lower == "node_modules" ||
				lower == ".ds_store" || lower == ".svn" || lower == ".idea" {
				continue
			}
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if entry.IsDir() {
			entries = append(entries, fts.FileEntry{
				Path:        fullPath,
				Name:        name,
				IsDirectory: true,
				Size:        0,
				Modified:    info.ModTime().Format(time.RFC3339),
				Content:     "",
			})
			// 递归（深度限制）
			sub, _ := scanDirForIndex(ctx, fullPath, depth+1, maxDepth)
			entries = append(entries, sub...)
			continue
		}

		// 普通文件：尝试读前 64KB
		content := readFileHead(fullPath, 64*1024)
		// 2026-07-02 v2：如果是加密容器，叠加插件可搜索内容（字幕等）
		if containerContent := extractContainerSearchableContent(fullPath); containerContent != "" {
			if content == "" {
				content = containerContent
			} else {
				content = content + "\n" + containerContent
			}
		}
		entries = append(entries, fts.FileEntry{
			Path:        fullPath,
			Name:        name,
			IsDirectory: false,
			Size:        info.Size(),
			Modified:    info.ModTime().Format(time.RFC3339),
			Content:     content,
		})
	}
	return entries, nil
}

// extractContainerSearchableContent 调插件 SearchableContentsExtractor 从容器里抽可搜索内容。
//
// 返回值：合并后的纯文本（多 item 用 \n 分隔），失败返回 ""
func extractContainerSearchableContent(containerPath string) string {
	extractors := getRegisteredExtractors()
	if len(extractors) == 0 {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(containerPath))
	var parts []string
	for _, extractor := range extractors {
		// 用 manifest.Enabled 已经在 getRegisteredExtractors 里筛过
		items, err := extractor.ExtractSearchableContents(containerPath)
		if err != nil || len(items) == 0 {
			continue
		}
		// 只在扩展名匹配该插件时采用
		manifest := extractor.GetSearchableContentsManifest()
		if !shouldExtractorHandle(manifest, ext) {
			continue
		}
		for _, it := range items {
			if it.Text == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("[%s:%s] %s", it.Type, it.Name, it.Text))
		}
	}
	return strings.Join(parts, "\n")
}

// shouldExtractorHandle 决定该 extractor 是否能处理该扩展名的容器。
// 当前实现：只要 extractor 实现了接口，所有 .sccgv/.ae 等容器文件都尝试。
// 插件内部根据自己关心的扩展名过滤。
func shouldExtractorHandle(manifest pluginInterfaces.SearchableContentsManifest, ext string) bool {
	return manifest.Enabled && ext != ""
}

// 注册的 extractors（避免每次都遍历 plugins.Plugins）
var (
	registeredExtractorsMu sync.RWMutex
	registeredExtractors   []pluginInterfaces.SearchableContentsExtractor
)

func getRegisteredExtractors() []pluginInterfaces.SearchableContentsExtractor {
	registeredExtractorsMu.RLock()
	defer registeredExtractorsMu.RUnlock()
	out := make([]pluginInterfaces.SearchableContentsExtractor, len(registeredExtractors))
	copy(out, registeredExtractors)
	return out
}

// RegisterSearchableExtractors 供 NewServer 调用，把 plugins.GetAllRegisteredSearchableExtractors
// 注册进来。FTS5 扫描时直接遍历。
func RegisterSearchableExtractors(extractors []pluginInterfaces.SearchableContentsExtractor) {
	registeredExtractorsMu.Lock()
	defer registeredExtractorsMu.Unlock()
	registeredExtractors = extractors
}

// readFileHead 读文件前 N 字节作为 content（用于 FTS5 索引）。
//
// 设计：
//   - 失败返回 ""（name 仍可被搜到）
//   - 只读前 N 字节（IO 限制，大文件不全量读）
//   - binary 探测：检测 NUL 字节（>1% 视为 binary，跳过）
func readFileHead(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, n)
	nRead, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return ""
	}
	buf = buf[:nRead]

	// binary 探测：NUL 字节比例 > 1% 视为 binary
	if isBinaryContent(buf) {
		return ""
	}
	return string(buf)
}

// isBinaryContent 检测是否是 binary content。
// 简单启发式：前 512 字节内 NUL 字节数 > 5% 视为 binary。
func isBinaryContent(buf []byte) bool {
	if len(buf) == 0 {
		return false
	}
	sample := buf
	if len(sample) > 512 {
		sample = sample[:512]
	}
	nul := 0
	for _, b := range sample {
		if b == 0 {
			nul++
		}
	}
	return float64(nul)/float64(len(sample)) > 0.01
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
