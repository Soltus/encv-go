package server

// mobile_search_fulltext_rebuilder.go — FTS 索引重建器实现（service.FTSRebuilder 接口）
//
// 2026-07-03 新增（spec fts-rebuild-task）
//
// 职责：
//   - 实现 service.FTSRebuilder interface
//   - 包装 GetFullTextIndex() + scanDirForIndex + BulkInsert
//   - 分阶段推送进度（10% init / 10-50% scanning / 50-95% indexing / 100% done）
//   - 支持取消（ctx.Done 时立即返回）
//   - 分批 BulkInsert（每批 1000 条，避免大目录一次性事务占用过多内存）
//
// 进度推送策略：
//   - scanning 阶段：定时器每 500ms 推送一次（基于 elapsed 时间估算）
//   - indexing 阶段：每批 1000 条推送一次（基于实际插入数量）
//   - 速率计算：基于已扫描/已索引数量 / elapsed 时间
//   - ETA 估算：剩余数量 / 当前速率

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/fts"
)

// ftsRebuilderImpl FTS 索引重建器实现。
//
// 持有 servingDir 引用（扫描起点），不持有 *fts.FileIndex（运行时调 GetFullTextIndex 取全局实例）。
// 这样保证 FTSRebuilder 永远操作最新的 fulltextIndex（即使 server 重启重新 InitFullTextIndex）。
type ftsRebuilderImpl struct {
	servingDir string
	maxDepth   int // 扫描深度限制，默认 5
	batchSize  int // BulkInsert 每批大小，默认 1000
}

// NewFTSRebuilder 创建 FTS 索引重建器实例。
//
// 调用方：server.NewServer 后调 taskManager.SetFTSRebuilder(NewFTSRebuilder(servingDir))
func NewFTSRebuilder(servingDir string) *ftsRebuilderImpl {
	return &ftsRebuilderImpl{
		servingDir: servingDir,
		maxDepth:   5,
		batchSize:  1000,
	}
}

// RebuildWithProgress 实现 service.FTSRebuilder interface。
//
// 流程：
//   1. 取全局 fulltextIndex，nil 返回 error
//   2. 清空旧索引（idx.Clear）
//   3. 设 building=true
//   4. 扫描 servingDir（后台 goroutine + 定时器推送进度）
//   5. 分批 BulkInsert（每批推送进度）
//   6. MarkBuilt
//   7. 全程检查 ctx.Err()
func (r *ftsRebuilderImpl) RebuildWithProgress(
	ctx context.Context,
	progressCb func(progress int, phase, speed, eta string),
) error {
	idx := GetFullTextIndex()
	if idx == nil {
		return fmt.Errorf("fulltext index not initialized")
	}

	start := time.Now()

	// 阶段 1：初始化（0-10%）
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if progressCb != nil {
		progressCb(5, "initializing", "", "")
	}

	if err := idx.Clear(ctx); err != nil {
		return fmt.Errorf("clear old index: %w", err)
	}
	idx.SetBuilding(true)
	defer idx.SetBuilding(false)

	if progressCb != nil {
		progressCb(10, "scanning", "", "")
	}

	// 阶段 2：扫描文件系统（10-50%）
	// scanDirForIndex 是一次性返回所有 entries，无中间进度。
	// 这里在 goroutine 里跑扫描，主循环定期推送基于时间估算的进度。
	entries, err := r.scanWithProgress(ctx, progressCb, start)
	if err != nil && ctx.Err() == nil {
		// 扫描失败但 ctx 未取消 → 继续用已扫描的部分（部分索引也优于 0）
		slog.Warn("FTS rebuild: scan returned error, using partial entries",
			"err", err, "entries", len(entries))
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if len(entries) == 0 {
		// 空目录也视为成功（用户可能确实没有文件）
		if progressCb != nil {
			progressCb(100, "completed", "", "")
		}
		idx.MarkBuilt(time.Since(start))
		return nil
	}

	// 阶段 3：分批 BulkInsert（50-95%）
	if progressCb != nil {
		progressCb(50, "indexing", "", "")
	}

	totalEntries := len(entries)
	inserted := 0
	for i := 0; i < totalEntries; i += r.batchSize {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		end := i + r.batchSize
		if end > totalEntries {
			end = totalEntries
		}
		batch := entries[i:end]

		if err := idx.BulkInsert(ctx, batch); err != nil {
			return fmt.Errorf("bulk insert batch [%d:%d]: %w", i, end, err)
		}
		inserted += len(batch)

		// 推送进度（50-95% 区间按比例）
		if progressCb != nil {
			ratio := float64(inserted) / float64(totalEntries)
			progress := 50 + int(ratio*45) // 50 → 95
			if progress > 95 {
				progress = 95
			}
			elapsed := time.Since(start).Seconds()
			speed := ""
			eta := ""
			if elapsed > 0 && inserted > 0 {
				rate := float64(inserted) / elapsed
				speed = fmt.Sprintf("%.0f files/s", rate)
				remaining := float64(totalEntries - inserted)
				if rate > 0 {
					etaSec := remaining / rate
					eta = formatETA(etaSec)
				}
			}
			progressCb(progress, "indexing", speed, eta)
		}
	}

	// 阶段 4：完成（100%）
	idx.MarkBuilt(time.Since(start))
	if progressCb != nil {
		progressCb(100, "completed", "", "")
	}

	slog.Info("FTS rebuild complete",
		"totalEntries", totalEntries,
		"inserted", inserted,
		"elapsed", time.Since(start))
	return nil
}

// scanWithProgress 在 goroutine 里跑 scanDirForIndex，主循环定期推送基于时间估算的进度。
//
// 估算策略：
//   - 先用 estimateEntryCount 做一次轻量遍历计数（不读文件内容）
//   - 假设扫描速率 ~5000 entries/s（保守估计）
//   - progress = 15 + (elapsed * rate / estimatedTotal) * 30，clamp 到 [15, 45]
func (r *ftsRebuilderImpl) scanWithProgress(
	ctx context.Context,
	progressCb func(progress int, phase, speed, eta string),
	_ time.Time,
) ([]fts.FileEntry, error) {
	estimatedTotal := estimateEntryCount(r.servingDir, r.maxDepth)
	scanStart := time.Now()

	if progressCb != nil {
		progressCb(15, "scanning", "", "")
	}

	scanDone := make(chan struct{})
	var entries []fts.FileEntry
	var scanErr error
	go func() {
		entries, scanErr = scanDirForIndex(ctx, r.servingDir, 0, r.maxDepth)
		close(scanDone)
	}()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if progressCb != nil && estimatedTotal > 0 {
				elapsed := time.Since(scanStart).Seconds()
				estimatedRate := 5000.0
				scannedEstimate := int(elapsed * estimatedRate)
				if scannedEstimate > estimatedTotal {
					scannedEstimate = estimatedTotal
				}
				ratio := float64(scannedEstimate) / float64(estimatedTotal)
				progress := 15 + int(ratio*30) // 15 → 45
				if progress > 45 {
					progress = 45
				}
				speed := fmt.Sprintf("%.0f files/s", estimatedRate)
				remaining := float64(estimatedTotal-scannedEstimate) / estimatedRate
				eta := formatETA(remaining)
				progressCb(progress, "scanning", speed, eta)
			}
		case <-scanDone:
			if progressCb != nil && ctx.Err() == nil {
				progressCb(50, "scanning-complete", "", "")
			}
			return entries, scanErr
		case <-ctx.Done():
			// ctx 取消：等 scanDone 后返回（scanDirForIndex 内部会检查 ctx）
			<-scanDone
			return entries, ctx.Err()
		}
	}
}

// estimateEntryCount 估算目录下的总 entry 数（用于进度计算）。
//
// 用 filepath.WalkDir 做一次快速遍历计数，不读文件内容。
// 失败时返回 0（进度推送降级为时间估算）。
func estimateEntryCount(rootDir string, maxDepth int) int {
	if rootDir == "" {
		return 0
	}
	count := 0
	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			if path != rootDir {
				rel, relErr := filepath.Rel(rootDir, path)
				if relErr == nil {
					depth := len(strings.Split(filepath.ToSlash(rel), "/"))
					if depth > maxDepth {
						return filepath.SkipDir
					}
				}
			}
			// 跳过不需要索引的目录
			name := strings.ToLower(d.Name())
			if name == ".encv" || name == ".git" || name == "node_modules" ||
				name == ".ds_store" || name == ".svn" || name == ".idea" {
				return filepath.SkipDir
			}
		}
		count++
		return nil
	})
	return count
}

// formatETA 格式化 ETA 字符串。
func formatETA(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
