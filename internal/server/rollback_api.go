package server

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/Soltus/encv-go/pkg/tasksystem"
)

// handleRollbackTaskGin POST /api/tasks/:id/rollback
//
// 回滚一个已完成的任务。先调 CanRollback 校验，通过后调 Rollback 创建回滚任务。
// 响应: 202 Accepted { taskId: "newTaskId" }
//
// 错误:
//   - 400 Bad Request: CanRollback 校验失败（任务未完成 / 已回滚 / 无策略等）
//   - 404 Not Found: 任务不存在
//   - 500 Internal Server Error: Rollback 执行失败
//   - 503 Service Unavailable: rollbackManager 未注入
func (s *Server) handleRollbackTaskGin(c *gin.Context) {
	rm := s.mobileSvc.GetRollbackManager()
	if rm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rollback manager not configured"})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task id is required"})
		return
	}

	// 先校验是否可回滚
	if err := rm.CanRollback(id); err != nil {
		// 区分"任务不存在"（404）和"校验失败"（400）
		if errors.Is(err, tasksystem.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		slog.Info("API: rollback rejected by CanRollback", "taskId", id, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建并异步执行回滚任务
	newTaskID, err := rm.Rollback(id, "user")
	if err != nil {
		slog.Error("API: rollback failed", "taskId", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("API: rollback task created", "originalTaskId", id, "rollbackTaskId", newTaskID)
	c.JSON(http.StatusAccepted, gin.H{"taskId": newTaskID})
}
