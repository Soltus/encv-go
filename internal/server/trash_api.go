package server

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleListTrashGin GET /api/trash
//
// 列出回收站所有条目。
// 响应: 200 { items: [...] }
//
// 错误:
//   - 503 Service Unavailable: trashManager 未注入
//   - 500 Internal Server Error: store 查询失败
func (s *Server) handleListTrashGin(c *gin.Context) {
	tm := s.mobileSvc.GetTrashManager()
	if tm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "trash manager not configured"})
		return
	}

	items, err := tm.List()
	if err != nil {
		slog.Error("API: list trash failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// handleRestoreTrashGin POST /api/trash/restore
//
// 从回收站还原文件到指定路径。
// Body: { "trashId": "...", "destPath": "..." }  (destPath 可选，空则还原到原路径)
// 响应: 200 { taskId: "..." }
//
// 错误:
//   - 400 Bad Request: 缺少 trashId
//   - 404 Not Found: trashId 不存在
//   - 500 Internal Server Error: 还原失败
//   - 503 Service Unavailable: trashManager 未注入
func (s *Server) handleRestoreTrashGin(c *gin.Context) {
	tm := s.mobileSvc.GetTrashManager()
	if tm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "trash manager not configured"})
		return
	}

	var req struct {
		TrashID  string `json:"trashId"`
		DestPath string `json:"destPath,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	if req.TrashID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trashId is required"})
		return
	}

	slog.Info("API: restore trash", "trashId", req.TrashID, "destPath", req.DestPath)

	taskID, err := tm.Restore(req.TrashID, req.DestPath, "user")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "trash item not found"})
			return
		}
		slog.Error("API: restore trash failed", "trashId", req.TrashID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"taskId": taskID})
}

// handlePurgeTrashGin DELETE /api/trash/:id
//
// 永久删除回收站指定条目。
// 响应: 200 { success: true }
//
// 错误:
//   - 404 Not Found: id 不存在
//   - 500 Internal Server Error: 删除失败
//   - 503 Service Unavailable: trashManager 未注入
func (s *Server) handlePurgeTrashGin(c *gin.Context) {
	tm := s.mobileSvc.GetTrashManager()
	if tm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "trash manager not configured"})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trash id is required"})
		return
	}

	slog.Info("API: purge trash", "id", id)

	if err := tm.Purge(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "trash item not found"})
			return
		}
		slog.Error("API: purge trash failed", "id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleEmptyTrashGin DELETE /api/trash
//
// 清空回收站（删除所有条目及其文件）。
// 响应: 200 { success: true }
//
// 错误:
//   - 500 Internal Server Error: 清空失败（部分失败也返回 500，错误信息含失败数）
//   - 503 Service Unavailable: trashManager 未注入
func (s *Server) handleEmptyTrashGin(c *gin.Context) {
	tm := s.mobileSvc.GetTrashManager()
	if tm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "trash manager not configured"})
		return
	}

	slog.Info("API: empty trash")

	if err := tm.Empty(); err != nil {
		slog.Error("API: empty trash failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
