package server

// mobile_tasks.go — 任务系统 CRUD：list / create / batch_create / cancel / resume / list_runs / retry / remove / clear。

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	vectorsearch "github.com/Soltus/encv-go/internal/search"
	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleGetTasksGin(c *gin.Context) {
	// 🆕 2026-06-23 Task 5：分页 API（10 万任务虚拟滚动支撑）
	//   - ?runId=xxx  → 只返回 task.RunId == runId 的 task（空则不过滤）
	//   - &offset=0   → 默认 0
	//   - &limit=100  → 默认 100，最大 500（防滥用）
	//   - 响应头 X-Total-Count = 过滤后、分页前的总数
	runId := c.Query("runId")

	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}

	tasks, totalCount := s.mobileSvc.GetTaskManager().ListPaginated(runId, offset, limit)
	c.Header("X-Total-Count", strconv.Itoa(totalCount))
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (s *Server) handleCreateTaskGin(c *gin.Context) {
	var req struct {
		Type              string            `json:"type"`
		SourcePath        string            `json:"sourcePath"`
		TargetPath        string            `json:"targetPath,omitempty"`
		Password          string            `json:"password,omitempty"`
		SecondaryPassword string            `json:"secondaryPassword,omitempty"`
		Version           int               `json:"version,omitempty"`
		PluginName        string            `json:"pluginName,omitempty"`
		ExtraFields       map[string]string `json:"extraFields,omitempty"`
		// 🆕 2026-06-18 Task 16：加解密参数持久化
		CipherMode      int    `json:"cipherMode,omitempty"`
		CompressionMode string `json:"compressionMode,omitempty"`
		// 🆕 v6 2026-06-18：runId + triggeredBy（单一数据源）
		RunId       string `json:"runId,omitempty"`
		TriggeredBy string `json:"triggeredBy,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	slog.Info("API: create task", "type", req.Type, "source", req.SourcePath,
		"target", req.TargetPath, "version", req.Version,
		"pluginName", req.PluginName,
		"hasPassword", req.Password != "",
		"hasSecondaryPassword", req.SecondaryPassword != "",
		"hasExtraFields", len(req.ExtraFields) > 0,
		"cipherMode", req.CipherMode,
		"compressionMode", req.CompressionMode,
		"runId", req.RunId,
		"triggeredBy", req.TriggeredBy)

	// 🆕 v6 2026-06-18：统一走 CreateWithRunMeta（含 crypto params + run meta）
	//   - runId 非空 → 自动化测试/AI agent 任务，前端按 runId 聚合
	//   - runId 空 → 后端兜底派生 "manual-${id}"（2026-06-22），不再有孤儿 task
	//   - triggeredBy 空 → 后端兜底 'user'（2026-06-22）
	compressionMode := req.CompressionMode
	if compressionMode == "" {
		compressionMode = "none"
	}
	task := s.mobileSvc.GetTaskManager().CreateWithRunMeta(
		req.Type, req.SourcePath, req.TargetPath,
		req.Password, req.SecondaryPassword, req.Version, req.PluginName, req.ExtraFields,
		req.CipherMode, compressionMode,
		req.RunId, req.TriggeredBy,
	)

	s.UpdateTaskSearchIndex(task.ID, task.SourcePath, task.Type, task.SourcePath, task.Status)

	c.JSON(http.StatusCreated, task)
}

func (s *Server) handleCreateTaskBatchGin(c *gin.Context) {
	var req struct {
		RunId       string `json:"runId,omitempty"`
		TriggeredBy string `json:"triggeredBy,omitempty"`
		Tasks       []struct {
			Type              string            `json:"type"`
			SourcePath        string            `json:"sourcePath"`
			TargetPath        string            `json:"targetPath,omitempty"`
			Password          string            `json:"password,omitempty"`
			SecondaryPassword string            `json:"secondaryPassword,omitempty"`
			Version           int               `json:"version,omitempty"`
			PluginName        string            `json:"pluginName,omitempty"`
			ExtraFields       map[string]string `json:"extraFields,omitempty"`
			CipherMode        int               `json:"cipherMode,omitempty"`
			CompressionMode   string            `json:"compressionMode,omitempty"`
		} `json:"tasks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	if len(req.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tasks array is empty"})
		return
	}

	slog.Info("API: batch create tasks",
		"count", len(req.Tasks),
		"runId", req.RunId,
		"triggeredBy", req.TriggeredBy)

	specs := make([]mobileservice.BatchTaskSpec, 0, len(req.Tasks))
	for _, t := range req.Tasks {
		specs = append(specs, mobileservice.BatchTaskSpec{
			Type:              t.Type,
			SourcePath:        t.SourcePath,
			TargetPath:        t.TargetPath,
			Password:          t.Password,
			SecondaryPassword: t.SecondaryPassword,
			Version:           t.Version,
			PluginName:        t.PluginName,
			ExtraFields:       t.ExtraFields,
			CipherMode:        t.CipherMode,
			CompressionMode:   t.CompressionMode,
		})
	}

	tasks := s.mobileSvc.GetTaskManager().CreateBatch(specs, req.RunId, req.TriggeredBy)

	// 增量更新搜索索引
	if s.searchSvc != nil {
		ctx := context.Background()
		batch := make([]vectorsearch.TaskIndexItem, 0, len(tasks))
		for _, t := range tasks {
			batch = append(batch, vectorsearch.TaskIndexItem{
				ID:         t.ID,
				Name:       t.SourcePath,
				TaskType:   t.Type,
				SourcePath: t.SourcePath,
				Status:     t.Status,
			})
		}
		if err := s.searchSvc.IndexTasksBatch(ctx, batch); err != nil {
			slog.Warn("batch update task search index failed", "error", err)
		}
	}

	c.JSON(http.StatusCreated, tasks)
}

func (s *Server) handleCancelTaskGin(c *gin.Context) {
	id := c.Param("id")

	task, err := s.mobileSvc.GetTaskManager().Cancel(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

func (s *Server) handleCancelRunGin(c *gin.Context) {
	runId := c.Param("runId")
	if runId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runId is required"})
		return
	}
	err := s.mobileSvc.GetTaskManager().CancelByRunId(runId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "runId": runId})
}

func (s *Server) handleResumeRunGin(c *gin.Context) {
	runId := c.Param("runId")
	if runId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runId is required"})
		return
	}
	count, err := s.mobileSvc.GetTaskManager().ResumePausedByRunId(runId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resumed", "runId": runId, "resumed": count})
}

func (s *Server) handleResumeAllPausedGin(c *gin.Context) {
	count, err := s.mobileSvc.GetTaskManager().ResumeAllPaused()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resumed", "resumed": count})
}

func (s *Server) handleGetRunSummaryGin(c *gin.Context) {
	runId := c.Param("runId")
	if runId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runId is required"})
		return
	}
	summary := s.mobileSvc.GetTaskManager().GetRunSummary(runId)
	c.JSON(http.StatusOK, summary)
}

func (s *Server) handleListRunsGin(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	runs := tm.ListRuns()
	// 批量补 summary（避免前端 N+1 调用 /summary）
	type runWithSummary struct {
		RunID       string                   `json:"runId"`
		StartedAt   time.Time                `json:"startedAt"`
		TriggeredBy string                   `json:"triggeredBy"`
		Summary     mobileservice.RunSummary `json:"summary"`
	}
	result := make([]runWithSummary, 0, len(runs))
	for _, r := range runs {
		summary := tm.GetRunSummary(r.RunID)
		result = append(result, runWithSummary{
			RunID:       r.RunID,
			StartedAt:   r.StartedAt,
			TriggeredBy: r.TriggeredBy,
			Summary:     summary,
		})
	}
	c.JSON(http.StatusOK, gin.H{"runs": result})
}

func (s *Server) handleRetryTaskGin(c *gin.Context) {
	id := c.Param("id")

	task, err := s.mobileSvc.GetTaskManager().Retry(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

func (s *Server) handleRemoveTaskGin(c *gin.Context) {
	id := c.Param("id")

	if err := s.mobileSvc.GetTaskManager().RemoveTask(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleClearCompletedTasksGin(c *gin.Context) {
	count := s.mobileSvc.GetTaskManager().ClearCompleted()
	c.JSON(http.StatusOK, gin.H{"ok": true, "removed": count})
}
