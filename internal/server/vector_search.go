package server

import (
	"context"
	"log/slog"

	"github.com/Soltus/encv-go/internal/search"
)

func (s *Server) InitVectorSearch(dbPath, actualEngine string) {
	searchDriver := actualEngine
	if searchDriver == "libsql" {
		searchDriver = "turso"
	}
	var searchSvc *vectorsearch.SearchService
	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("vector search init panicked, continuing without it", "panic", r)
				searchSvc = nil
			}
		}()
		svc, initErr := vectorsearch.NewSearchServiceFromPath(dbPath, searchDriver)
		if initErr != nil {
			slog.Warn("failed to init vector search service, continuing without it", "err", initErr)
		} else {
			searchSvc = svc
		}
	}()
	if searchSvc != nil {
		s.searchSvc = searchSvc
		slog.Info("vector search service initialized", "driver", searchDriver)
		go s.rebuildTaskSearchIndex()
	}
}

func (s *Server) rebuildTaskSearchIndex() {
	if s.searchSvc == nil {
		return
	}

	tm := s.mobileSvc.GetTaskManager()
	if tm == nil {
		return
	}

	tasks := tm.List()
	if len(tasks) == 0 {
		return
	}

	ctx := context.Background()

	batchSize := 100
	for i := 0; i < len(tasks); i += batchSize {
		end := i + batchSize
		if end > len(tasks) {
			end = len(tasks)
		}

		batch := make([]vectorsearch.TaskIndexItem, 0, end-i)
		for _, t := range tasks[i:end] {
			name := t.SourcePath
			if name == "" {
				name = t.ID
			}
			batch = append(batch, vectorsearch.TaskIndexItem{
				ID:         t.ID,
				Name:       name,
				TaskType:   t.Type,
				SourcePath: t.SourcePath,
				Status:     string(t.Status),
			})
		}

		if err := s.searchSvc.IndexTasksBatch(ctx, batch); err != nil {
			slog.Warn("rebuild task search index batch failed", "start", i, "error", err)
		}
	}

	slog.Info("task search index rebuilt", "count", len(tasks))
}

func (s *Server) UpdateTaskSearchIndex(taskID, name, taskType, sourcePath, status string) {
	if s.searchSvc == nil {
		return
	}
	ctx := context.Background()
	s.searchSvc.IndexTask(ctx, taskID, name, taskType, sourcePath, status)
}

func (s *Server) RemoveTaskFromSearchIndex(taskID string) {
	if s.searchSvc == nil {
		return
	}
	ctx := context.Background()
	s.searchSvc.DeleteTask(ctx, taskID)
}
