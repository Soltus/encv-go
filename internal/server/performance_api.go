package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
)

// handleGetTaskPerformance GET /api/tasks/:id/performance
//
// 返回指定任务的性能指标。
// 响应: 200 { metrics: PerformanceMetrics }
//
// 错误:
//   - 404 Not Found: 任务无性能指标
//   - 503 Service Unavailable: store 未配置
func (s *Server) handleGetTaskPerformance(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	if tm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task manager not configured"})
		return
	}
	store := tm.GetStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "performance store not configured"})
		return
	}

	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task id is required"})
		return
	}

	metrics, err := store.GetMetrics(taskID)
	if err != nil {
		if errors.Is(err, tasksystem.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no performance metrics for this task"})
			return
		}
		slog.Error("API: get task performance failed", "taskId", taskID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"metrics": metrics})
}

// handleGetCalibration GET /api/performance/calibration
//
// 返回当前硬件校准结果。
// 响应: 200 { calibration: CalibrationResult }
//   - 若未校准，calibration 为 null
func (s *Server) handleGetCalibration(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	if tm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task manager not configured"})
		return
	}
	store := tm.GetStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "performance store not configured"})
		return
	}

	cal, err := store.GetCalibration()
	if err != nil {
		slog.Error("API: get calibration failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"calibration": cal})
}

// handleRecalibrate POST /api/performance/calibration
//
// 手动重跑硬件校准（dev-only）。
// 响应: 200 { calibration: CalibrationResult }
func (s *Server) handleRecalibrate(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	if tm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task manager not configured"})
		return
	}
	store := tm.GetStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "performance store not configured"})
		return
	}

	slog.Info("API: recalibrating...")
	cal := performance.RunCalibration()
	if err := store.SaveCalibration(cal); err != nil {
		slog.Error("API: save calibration failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"calibration": cal})
}

// handleGetPerformanceHistory GET /api/performance/history?plugin=xxx&type=encrypt&limit=10
//
// 返回指定 plugin + taskType 的历史性能指标（按 created_at 倒序）。
// 响应: 200 { history: [PerformanceMetrics, ...] }
func (s *Server) handleGetPerformanceHistory(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	if tm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task manager not configured"})
		return
	}
	store := tm.GetStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "performance store not configured"})
		return
	}

	plugin := c.Query("plugin")
	taskType := c.Query("type")
	if plugin == "" || taskType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plugin and type query params are required"})
		return
	}

	limit := 10
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	history, err := store.ListMetricsByPlugin(plugin, taskType, limit)
	if err != nil {
		slog.Error("API: get performance history failed", "plugin", plugin, "type", taskType, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"history": history})
}
