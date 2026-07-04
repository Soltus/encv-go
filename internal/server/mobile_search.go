package server

// mobile_search.go — 搜索 + 向量搜索 + CJK 扩展 + bigram 过滤 + hybrid 评分。包含 LRU 缓存 hook（在 search_cache.go）。

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	vectorsearch "github.com/Soltus/encv-go/internal/search"
	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/gin-gonic/gin"
)

// BigramStrictness bigram 过滤严格度档位（详见各常量注释）
type BigramStrictness int

const (
	// BigramRelaxed 放宽：共享 ≥ 1 个 bigram 即可（结果过少时，宁多勿少）
	BigramRelaxed BigramStrictness = iota
	// BigramMedium 中等：共享 ≥ 一半 bigram（默认强度）
	BigramMedium
	// BigramStrict 严格：共享全部 bigram（结果过多时，收紧过滤）
	BigramStrict
)

func (s *Server) handleSearchFilesGin(c *gin.Context) {
	queryPath := utils.DecodeGinQueryParam(c.Query("path"))
	keyword := c.Query("keyword")
	recursive := c.Query("recursive") == "true"

	slog.Info("API: search files", "path", queryPath, "keyword", keyword, "recursive", recursive)

	files, err := s.searchFilesWithMounts(queryPath, keyword, recursive)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	slog.Info("API: search files result", "path", queryPath, "keyword", keyword, "count", len(files))
	c.JSON(http.StatusOK, gin.H{"files": files})
}

func (s *Server) searchFilesWithMounts(queryPath, keyword string, recursive bool) ([]mobileservice.FileInfo, error) {
	isMountRoot := queryPath == "/d" || queryPath == "/d/"
	hasWebdav := s.canUseWebdavIndex() || len(s.webdavFSByMount) > 0

	if keyword != "" {
		var files []mobileservice.FileInfo

		if isMountRoot {
			allMountFiles, allWebdavFiles := s.searchAcrossAllMounts(keyword, 200, recursive)

			if hasWebdav {
				containerExts := make(map[string]bool)
				for _, ext := range plugins.GetAllRegisteredContainerExtensions() {
					containerExts[strings.ToLower(ext)] = true
				}
				for _, f := range allMountFiles {
					ext := strings.ToLower(filepath.Ext(f.Name))
					if !containerExts[ext] {
						files = append(files, f)
					}
				}
				files = append(files, allWebdavFiles...)
			} else {
				files = allMountFiles
			}
		} else {
			mobileFiles, err := s.mobileSvc.SearchFiles(queryPath, keyword, recursive)
			if err != nil {
				return nil, err
			}

			if hasWebdav && recursive {
				containerExts := make(map[string]bool)
				for _, ext := range plugins.GetAllRegisteredContainerExtensions() {
					containerExts[strings.ToLower(ext)] = true
				}
				for _, f := range mobileFiles {
					ext := strings.ToLower(filepath.Ext(f.Name))
					if !containerExts[ext] {
						files = append(files, f)
					}
				}
				files = append(files, s.searchWebdavMounts(keyword, queryPath, 200)...)
			} else {
				files = mobileFiles
			}
		}

		return files, nil
	}

	// keyword 为空时，退化为 ListFiles
	return s.mobileSvc.SearchFiles(queryPath, keyword, recursive)
}

func (s *Server) searchAcrossAllMounts(keyword string, maxPerMount int, recursive bool) ([]mobileservice.FileInfo, []mobileservice.FileInfo) {
	var physicalFiles []mobileservice.FileInfo
	var webdavFiles []mobileservice.FileInfo

	if s.mountRegistry == nil {
		return physicalFiles, webdavFiles
	}

	for _, m := range s.mountRegistry.List() {
		if !m.Enabled {
			continue
		}
		mountQueryPath := "/d/" + m.Name

		results, err := s.mobileSvc.SearchFiles(mountQueryPath, keyword, recursive)
		if err != nil {
			slog.Warn("Search in mount failed", "mount", m.Name, "error", err)
			continue
		}

		physicalFiles = append(physicalFiles, results...)

		if entry, ok := s.webdavFSByMount[m.Name]; ok {
			prefix := ""
			if !recursive {
				prefix = ""
			}
			entries := entry.fs.SearchInIndex(keyword, prefix, maxPerMount)
			for _, e := range entries {
				if !recursive && strings.Contains(e.Path, "/") {
					continue
				}
				virtualPath := e.Path
				if strings.HasPrefix(virtualPath, "./") {
					virtualPath = strings.TrimPrefix(virtualPath, ".")
				}
				if !strings.HasPrefix(virtualPath, "/") {
					virtualPath = "/" + virtualPath
				}
				displayPath := "/d/" + m.Name + virtualPath
				webdavFiles = append(webdavFiles, mobileservice.FileInfo{
					Name:        e.Name,
					Path:        displayPath,
					IsDirectory: e.IsDir,
					Size:        e.Size,
					Modified:    e.ModTime,
				})
			}
		}
	}

	return physicalFiles, webdavFiles
}

func (s *Server) searchWebdavMounts(keyword, queryPath string, maxPerMount int) []mobileservice.FileInfo {
	var results []mobileservice.FileInfo

	// 解析查询路径属于哪个挂载点
	// "/d" 或 "/d/" → 所有挂载点
	// "/d/<name>/..." → 只搜指定挂载点，子路径去掉 /d/<name>/ 前缀
	targetMount := ""
	subPath := ""
	if strings.HasPrefix(queryPath, "/d/") {
		rest := strings.TrimPrefix(queryPath, "/d/")
		slashIdx := strings.Index(rest, "/")
		if slashIdx > 0 {
			targetMount = rest[:slashIdx]
			subPath = rest[slashIdx:]
		} else if rest != "" {
			targetMount = rest
			subPath = ""
		}
	}

	for name, entry := range s.webdavFSByMount {
		if targetMount != "" && name != targetMount {
			continue
		}

		searchPrefix := strings.TrimPrefix(subPath, "/")

		entries := entry.fs.SearchInIndex(keyword, searchPrefix, maxPerMount)

		for _, e := range entries {
			virtualPath := e.Path
			if strings.HasPrefix(virtualPath, "./") {
				virtualPath = strings.TrimPrefix(virtualPath, ".")
			}
			if !strings.HasPrefix(virtualPath, "/") {
				virtualPath = "/" + virtualPath
			}

			displayPath := "/d/" + name + virtualPath

			results = append(results, mobileservice.FileInfo{
				Name:        e.Name,
				Path:        displayPath,
				IsDirectory: e.IsDir,
				Size:        e.Size,
				Modified:    e.ModTime,
			})
		}
	}

	return results
}

func (s *Server) handleIndexStatsGin(c *gin.Context) {
	stats := s.mobileSvc.GetIndexStats()
	if stats.TotalFiles == 0 && !stats.IsIndexing {
		s.mobileSvc.RebuildIndex()
		stats = s.mobileSvc.GetIndexStats()
	}
	stats.Source = "mobile"

	if s.canUseWebdavIndex() {
		wdStats := s.webdavFS.GetIndexStats()
		ordinaryFiles := stats.TotalFiles
		containerPhysicalCount := 0
		if ordinaryFiles > 0 && wdStats.Containers > 0 {
			containerPhysicalCount = wdStats.Containers
		}
		stats.TotalFiles = ordinaryFiles - containerPhysicalCount + wdStats.TotalFiles
		if stats.TotalFiles < 0 {
			stats.TotalFiles = 0
		}
		stats.Containers = wdStats.Containers
		stats.Source = "webdav"
	}

	c.JSON(http.StatusOK, stats)
}

func (s *Server) handleIndexRebuildGin(c *gin.Context) {
	s.mobileSvc.RebuildIndex()
	source := "mobile"
	if s.canUseWebdavIndex() {
		source = "webdav"
	}
	c.JSON(http.StatusOK, gin.H{"status": "indexing", "source": source})
}

func (s *Server) handleIndexClearGin(c *gin.Context) {
	s.mobileSvc.ClearIndex()
	c.JSON(http.StatusOK, gin.H{"status": "cleared"})
}

func (s *Server) handleVectorSearchTasksGin(c *gin.Context) {
	query := c.Query("q")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if query == "" {
		c.JSON(http.StatusOK, gin.H{"tasks": []gin.H{}, "vector_search": false})
		return
	}

	if s.searchSvc == nil {
		// 向量搜索不可用时，降级为普通字符串匹配
		tasks, _ := s.mobileSvc.GetTaskManager().ListPaginated("", 0, 200)
		var matched []mobileservice.MobileTask
		q := strings.ToLower(query)
		for _, t := range tasks {
			if strings.Contains(strings.ToLower(t.SourcePath), q) ||
				strings.Contains(strings.ToLower(t.Type), q) {
				matched = append(matched, *t)
			}
		}
		c.JSON(http.StatusOK, gin.H{"tasks": matched, "vector_search": false})
		return
	}

	ctx := context.Background()
	results, err := s.searchSvc.SearchTasks(ctx, query, limit)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	// 把搜索结果转换成前端能识别的任务格式
	// 从 TaskManager 获取完整任务信息
	tm := s.mobileSvc.GetTaskManager()
	var tasks []*mobileservice.MobileTask
	for _, r := range results {
		if r.Score < 0.05 {
			continue // 过滤掉相似度太低的结果
		}
		if task, err := tm.Get(r.RefID); err == nil && task != nil {
			// 复制一份再设置 score，避免修改 TaskManager 中的原始 task（指针共享）
			taskCopy := *task
			taskCopy.SearchScore = r.Score
			tasks = append(tasks, &taskCopy)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":         tasks,
		"vector_search": true,
		"total":         len(results),
	})
}

func (s *Server) handleVectorSearchFilesGin(c *gin.Context) {
	query := c.Query("q")
	path := utils.DecodeGinQueryParam(c.Query("path"))
	recursive := c.Query("recursive") == "true"
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if query == "" {
		c.JSON(http.StatusOK, gin.H{"files": []gin.H{}, "vector_search": false, "search_mode": "none"})
		return
	}

	// 🆕 2026-07-02 搜索结果 LRU 缓存（详见 search_cache.go）：
	//   解决用户反馈"先搜'在线'，再搜'在线 视频'重新 loading 几秒"问题。
	//   连续搜索同一 query 直接命中缓存（< 1ms 返回），不走 DB/向量/混合评分。
	//   缓存 key 包含所有影响结果的因素（path/recursive/limit）以避免错配。
	cacheKey := buildSearchCacheKey(path, query, recursive, limit)
	if s.searchCache != nil {
		if entry, ok := s.searchCache.Get(cacheKey); ok {
			// 缓存命中：直接返回上次结果（前端无需重新 loading）
			c.Header("X-Search-Cache", "HIT")
			c.JSON(http.StatusOK, gin.H{
				"files":         entry.files,
				"vector_search": entry.vectorSearch,
				"search_mode":   entry.searchMode,
				"total":         entry.total,
			})
			return
		}
		c.Header("X-Search-Cache", "MISS")
	}

	// 🆕 2026-07-02 修复长文件名被关键词搜索漏掉：
	//   连续 CJK 查询（如"在线视频"）做整体子串匹配时，
	//   "在线"和"视频"被分隔开的长文件名（如"在线播放-高清视频-2026-07-02-最终版.mp4"）
	//   不包含"在线视频"完整子串 → 漏掉。
	//   拆为单字 AND 序列后，matchKeyword 会对每个单字都做子串匹配，
	//   召回所有包含"在/线/视/频"任一字的文件，再交给向量搜索 + 混合评分精排。
	expandedQuery := expandCJKQueryForSearch(query)

	// 先调用现有文件搜索拿到候选结果（遵守递归设置 + /d 多挂载点遍历）
	files, err := s.searchFilesWithMounts(path, expandedQuery, recursive)
	if err != nil {
		writeServiceErrorGin(c, err)
		return
	}

	// 🆕 2026-07-02 智能搜索策略（见 debug-discipline.md §3.5）：
	//   - 关键词精确匹配 ≥ 20 → strict 模式，只返回关键词结果（避免向量噪声）
	//   - 关键词匹配 1~19 → combined 模式，关键词结果 + 向量重排序
	//   - 关键词匹配 0 → greedy 模式，纯向量 fallback（bigram 过滤可放宽）
	//
	// 前端据 search_mode 字段对 greedy 结果加视觉标记，让用户看出是宽松匹配。
	//
	// strict 阈值 50（不是默认 20）的原因：
	//   - CJK 连续查询扩展为单字 AND 后候选会变多（如"在线视频" → "在/线/视/频" 会命中 5~20 个文件）
	//   - 把阈值提高到 50 避免扩展后误触发 strict 模式（strict 只返回关键词结果不精排）
	const strictThreshold = 50

	if len(files) >= strictThreshold {
		// strict 模式：结果过多，只返回关键词匹配（可能按名称排序）
		writeSearchResponseGin(c, s, cacheKey, files, false, "strict", len(files))
		return
	}

	// combined 模式：1~19 个关键词结果，用混合评分重排序
	if s.searchSvc != nil && len(files) > 0 {
		ctx := context.Background()
		// 先把结果批量索引到向量搜索
		batch := make([]vectorsearch.FileIndexItem, 0, len(files))
		for _, f := range files {
			batch = append(batch, vectorsearch.FileIndexItem{
				Path:  f.Path,
				Name:  f.Name,
				Size:  f.Size,
				MTime: f.Modified,
			})
		}
		s.searchSvc.IndexFilesBatch(ctx, batch)

		// 然后用向量搜索 + 混合评分重排序
		results, vErr := s.searchSvc.SearchFiles(ctx, query, limit)
		queryBigrams := extractBigrams(query)
		if vErr == nil && len(results) > 0 {
			// 构建向量 score map
			vectorScoreMap := make(map[string]float64)
			for _, r := range results {
				if r.Score >= 0.05 {
					vectorScoreMap[r.RefID] = r.Score
				}
			}

			// 用混合评分重排序所有文件
			sortedFiles := make([]mobileservice.FileInfo, len(files))
			copy(sortedFiles, files)

			// 计算混合评分
			for i := range sortedFiles {
				vecScore, ok := vectorScoreMap[sortedFiles[i].Path]
				if !ok {
					vecScore = 0
				}
				sortedFiles[i].Score = computeHybridScore(sortedFiles[i].Name, queryBigrams, vecScore)
			}

			// 按混合评分排序
			for i := 0; i < len(sortedFiles); i++ {
				for j := i + 1; j < len(sortedFiles); j++ {
					if sortedFiles[j].Score > sortedFiles[i].Score {
						sortedFiles[i], sortedFiles[j] = sortedFiles[j], sortedFiles[i]
					}
				}
			}

			c.JSON(http.StatusOK, gin.H{
				"files":         sortedFiles,
				"vector_search": true,
				"search_mode":   "combined",
				"total":         len(sortedFiles),
			})
			// 🆕 2026-07-02 写缓存（combined 模式重排序结果）
			if s.searchCache != nil {
				cp := make([]mobileservice.FileInfo, len(sortedFiles))
				copy(cp, sortedFiles)
				s.searchCache.Set(cacheKey, &searchResultCacheEntry{
					files:        cp,
					vectorSearch: true,
					searchMode:   "combined",
					total:        len(sortedFiles),
					cachedAt:     time.Now(),
					fromQuery:    c.Query("q"),
				})
			}
			return
		}
	}

	// greedy 模式：关键词搜索返回 0 结果，若向量搜索可用，fallback 到纯向量搜索。
	//
	// 场景：搜索 "在线高清"（无空格）应匹配 "在线播放-高清视频.mp4"。
	// 关键词 "在线高清" 不是文件名字串 → SearchFiles 返回 0 候选；
	// 但向量搜索 bigram 分词让两者共享 token（在线/高清/在/线/高/清），
	// 余弦相似度高 → 能匹配。
	//
	// greedy 模式的 bigram 过滤强度根据结果数量动态调整：
	//   - 结果 < 5：放宽（共享 ≥ 1 个 bigram 即可，宁多勿少）
	//   - 结果 5~20：中等（共享 ≥ 一半 bigram）
	//   - 结果 > 20：收紧（共享全部 bigram 或 score ≥ 0.5）
	if s.searchSvc != nil && len(files) == 0 {
		vectorResults := s.vectorSearchFallback(path, query, recursive, limit)
		if len(vectorResults) > 0 {
			c.JSON(http.StatusOK, gin.H{
				"files":         vectorResults,
				"vector_search": true,
				"search_mode":   "greedy",
				"total":         len(vectorResults),
			})
			// 🆕 2026-07-02 写缓存（greedy 模式 fallback 结果）
			if s.searchCache != nil {
				cp := make([]mobileservice.FileInfo, len(vectorResults))
				copy(cp, vectorResults)
				s.searchCache.Set(cacheKey, &searchResultCacheEntry{
					files:        cp,
					vectorSearch: true,
					searchMode:   "greedy",
					total:        len(vectorResults),
					cachedAt:     time.Now(),
					fromQuery:    c.Query("q"),
				})
			}
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"files":         files,
		"vector_search": false,
		"search_mode":   "none",
		"total":         len(files),
	})
	// 🆕 2026-07-02 写缓存（none 模式兜底结果）
	if s.searchCache != nil {
		cp := make([]mobileservice.FileInfo, len(files))
		copy(cp, files)
		s.searchCache.Set(cacheKey, &searchResultCacheEntry{
			files:        cp,
			vectorSearch: false,
			searchMode:   "none",
			total:        len(files),
			cachedAt:     time.Now(),
			fromQuery:    c.Query("q"),
		})
	}
}

func (s *Server) vectorSearchFallback(path, query string, recursive bool, limit int) []mobileservice.FileInfo {
	if s.searchSvc == nil {
		return nil
	}

	// 1. 列出搜索路径下的所有文件
	var allFiles []mobileservice.FileInfo
	isMountRoot := path == "/d" || path == "/d/"
	if isMountRoot {
		// 遍历所有挂载点
		for _, m := range s.mountRegistry.List() {
			if !m.Enabled {
				continue
			}
			files := s.mobileSvc.ListAllFilesForVectorIndex("/d/"+m.Name, recursive, 500-len(allFiles))
			allFiles = append(allFiles, files...)
			if len(allFiles) >= 500 {
				break
			}
		}
	} else {
		allFiles = s.mobileSvc.ListAllFilesForVectorIndex(path, recursive, 500)
	}

	if len(allFiles) == 0 {
		return nil
	}

	// 2. 批量索引到向量库
	ctx := context.Background()
	batch := make([]vectorsearch.FileIndexItem, 0, len(allFiles))
	for _, f := range allFiles {
		batch = append(batch, vectorsearch.FileIndexItem{
			Path:  f.Path,
			Name:  f.Name,
			Size:  f.Size,
			MTime: f.Modified,
		})
	}
	s.searchSvc.IndexFilesBatch(ctx, batch)

	// 3. 向量搜索
	results, err := s.searchSvc.SearchFiles(ctx, query, limit)
	if err != nil || len(results) == 0 {
		return nil
	}

	// 4. 计算查询的 bigram 集合（非单字 CJK bigram + 英文单词）
	//    用于过滤只共享单个 bigram 的弱相关结果
	queryBigrams := extractBigrams(query)

	// 5. 映射回 FileInfo，附带 score + bigram 重叠过滤（动态强度）
	fileMap := make(map[string]mobileservice.FileInfo, len(allFiles))
	for _, f := range allFiles {
		fileMap[f.Path] = f
	}

	// 先收集候选结果（score >= 0.05），用于判断结果数量级决定过滤强度
	type candidate struct {
		file  mobileservice.FileInfo
		score float64
	}
	var candidates []candidate
	for _, r := range results {
		if r.Score < 0.05 {
			continue
		}
		if f, ok := fileMap[r.RefID]; ok {
			candidates = append(candidates, candidate{file: f, score: r.Score})
		}
	}

	// 根据候选数量决定 bigram 过滤强度（见 debug-discipline.md §3.5）
	//   - 候选 < 5：relaxed（共享 ≥ 1 个 bigram 即可，宁多勿少）
	//   - 候选 5~20：medium（共享 ≥ 一半 bigram）
	//   - 候选 > 20：strict（共享全部 bigram）
	var strictness BigramStrictness
	switch {
	case len(candidates) < 5:
		strictness = BigramRelaxed
	case len(candidates) <= 20:
		strictness = BigramMedium
	default:
		strictness = BigramStrict
	}

	// 6. bigram 过滤 + 混合评分重排序
	//    用混合评分（bigram重叠率 + 向量相似度 + 命中密度）替代纯向量评分，
	//    解决长文件名相关性稀释问题。
	var matched []mobileservice.FileInfo
	for _, c := range candidates {
		if !hasSufficientBigramOverlapEx(c.file.Name, queryBigrams, strictness) {
			continue
		}
		f := c.file
		// 使用混合评分替代纯向量评分
		f.Score = computeHybridScore(f.Name, queryBigrams, c.score)
		matched = append(matched, f)
	}

	// 按混合评分重新排序
	for i := 0; i < len(matched); i++ {
		for j := i + 1; j < len(matched); j++ {
			if matched[j].Score > matched[i].Score {
				matched[i], matched[j] = matched[j], matched[i]
			}
		}
	}

	return matched
}

func expandCJKQueryForSearch(query string) string {
	// 已有空格分隔的查询不动（用户已显式分词）
	if strings.ContainsAny(query, " \t\n\r") {
		return query
	}
	runes := []rune(query)
	if len(runes) < 2 {
		return query
	}
	// 检测是否包含 CJK 字符（汉/日/韩）；纯英文/数字查询不动
	hasCJK := false
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) ||
			(r >= 0x3040 && r <= 0x30FF) || // 日文假名
			(r >= 0xAC00 && r <= 0xD7AF) { // 韩文
			hasCJK = true
			break
		}
	}
	if !hasCJK {
		return query
	}
	// 拆为单字，空格连接
	parts := make([]string, len(runes))
	for i, r := range runes {
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

func buildSearchCacheKey(path, query string, recursive bool, limit int) string {
	h := sha1.New()
	h.Write([]byte(path))
	h.Write([]byte{0x00}) // null 分隔避免 path+query 拼接歧义
	h.Write([]byte(query))
	h.Write([]byte{0x00})
	var recByte byte
	if recursive {
		recByte = 1
	}
	h.Write([]byte{recByte})
	h.Write([]byte{0x00})
	h.Write([]byte(strconv.Itoa(limit)))
	return "search:" + hex.EncodeToString(h.Sum(nil))
}

func writeSearchResponseGin(c *gin.Context, s *Server, cacheKey string, files []mobileservice.FileInfo, vectorSearch bool, searchMode string, total int) {
	if s.searchCache != nil {
		cp := make([]mobileservice.FileInfo, len(files))
		copy(cp, files)
		s.searchCache.Set(cacheKey, &searchResultCacheEntry{
			files:        cp,
			vectorSearch: vectorSearch,
			searchMode:   searchMode,
			total:        total,
			cachedAt:     time.Now(),
			fromQuery:    c.Query("q"),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"files":         files,
		"vector_search": vectorSearch,
		"search_mode":   searchMode,
		"total":         total,
	})
}

func extractBigrams(query string) []string {
	tokens := vectorsearch.Tokenize(query)
	var bigrams []string
	for _, tok := range tokens {
		// 只保留长度 >= 2 的 token（CJK bigram 或英文单词），过滤单字
		if len([]rune(tok)) >= 2 {
			bigrams = append(bigrams, strings.ToLower(tok))
		}
	}
	return bigrams
}

func hasSufficientBigramOverlap(fileName string, queryBigrams []string) bool {
	return hasSufficientBigramOverlapEx(fileName, queryBigrams, BigramMedium)
}

func hasSufficientBigramOverlapEx(fileName string, queryBigrams []string, strictness BigramStrictness) bool {
	if len(queryBigrams) == 0 {
		return true
	}
	lowerName := strings.ToLower(fileName)
	shared := 0
	for _, bg := range queryBigrams {
		if strings.Contains(lowerName, bg) {
			shared++
		}
	}
	switch strictness {
	case BigramRelaxed:
		return shared >= 1
	case BigramStrict:
		return shared >= len(queryBigrams)
	default: // BigramMedium
		threshold := (len(queryBigrams) + 1) / 2 // 向上取整
		return shared >= threshold
	}
}

func countSharedBigrams(fileName string, queryBigrams []string) int {
	if len(queryBigrams) == 0 {
		return 0
	}
	lowerName := strings.ToLower(fileName)
	shared := 0
	for _, bg := range queryBigrams {
		if strings.Contains(lowerName, bg) {
			shared++
		}
	}
	return shared
}

func computeHybridScore(fileName string, queryBigrams []string, vectorScore float64) float64 {
	nQuery := len(queryBigrams)
	if nQuery == 0 {
		return vectorScore
	}

	shared := countSharedBigrams(fileName, queryBigrams)

	// 1. bigram 重叠率（召回导向：查询中有多少出现在文件里）
	bigramRecall := float64(shared) / float64(nQuery)

	// 2. 关键词命中密度（精准导向：共享 bigram 占文件 bigram 的比例）
	fileBigrams := extractBigrams(fileName)
	nFile := len(fileBigrams)
	bigramPrecision := 0.0
	if nFile > 0 {
		bigramPrecision = float64(shared) / float64(nFile)
	}

	// 3. 混合评分（加权平均）
	//    - bigramRecall 权重最高（用户最关心"查询词是否都命中"）
	//    - bigramPrecision 辅助（短而精准的匹配更好，惩罚长文件名噪音）
	//    - vectorScore 补充（捕捉语义近似）
	hybrid := 0.5*bigramRecall + 0.3*bigramPrecision + 0.2*vectorScore

	// 确保在 [0, 1] 范围内
	if hybrid < 0 {
		hybrid = 0
	}
	if hybrid > 1 {
		hybrid = 1
	}
	return hybrid
}

func (s *Server) handleSearchStatsGin(c *gin.Context) {
	if s.searchSvc == nil {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"engine":    "none",
		})
		return
	}

	stats := s.searchSvc.Stats()
	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"stats":     stats,
	})
}
