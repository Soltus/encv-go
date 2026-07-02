package server

// mobile_api.go — 通用 helper + 健康检查入口。
// 业务子域 handler 已拆分到 mobile_*.go：
//   - mobile_files.go    文件 CRUD
//   - mobile_search.go   搜索 + 向量 + CJK 扩展 + bigram
//   - mobile_tasks.go    任务 CRUD
//   - mobile_webdav.go   WebDAV + index
//   - mobile_openlist.go Openlist 站点
//   - mobile_metadata.go 标签 + 插件 + 容器扩展
//   - mobile_stream.go   SSE 流式
//   - mobile_database.go 数据库管理
//   - mobile_logs.go     API 日志 + 构建信息
//   - mobile_sparse.go   Sparse container

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/gin-gonic/gin"
)

func isValidSiteID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func writeServiceErrorGin(c *gin.Context, err error) {
	switch err.(type) {
	case *mobileservice.PermissionError:
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case *mobileservice.ForbiddenError:
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case *mobileservice.NotFoundError:
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case *mobileservice.BadRequestError:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case *mobileservice.UnsupportedMediaTypeError:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func (s *Server) handlePingGin(c *gin.Context) {
	c.JSON(http.StatusOK, types.PingResponse{
		Status:        types.ServiceStatuses.OK,
		Version:       s.version,
		InstanceID:    s.instanceID,
		ServerDirPath: s.servingDir,
		WebdavDirPath: s.webdavDir,
	})
}

func (s *Server) handleHealthGin(c *gin.Context) {
	// 🆕 2026-06-14：跨进程 IPC 重构 — 改为 JSON 响应（含心跳状态）
	//
	// 历史：返回 {"status":"ok"}。parent（Kotlin EncvGoService）只能判存活不能判 hang。
	// 新：返回 {"status","heartbeat_age_ms","heartbeat_ok"}。
	//     parent 通过 heartbeat_ok 字段判 hang，连续 N 次 false 才销毁进程。
	//
	// 向后兼容：保留 "status" 字段，前端 / 沙箱 preview-gateway 用 code==200 判断不受影响。
	hbMs := atomic.LoadInt64(&s.lastHeartbeatMs)
	var hbAgeMs int64 = -1
	var hbOK bool = false
	if hbMs > 0 {
		hbAgeMs = time.Now().UnixMilli() - hbMs
		hbOK = hbAgeMs < HeartbeatStaleThreshold.Milliseconds()
	}
	c.JSON(http.StatusOK, gin.H{
		"status":           "ok",
		"heartbeat_age_ms": hbAgeMs,
		"heartbeat_ok":     hbOK,
	})
}

func (s *Server) handleServerShutdownGin(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "shutting_down"})
	go func() {
		slog.Info("Shutdown requested via API")
		if s.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.server.Shutdown(ctx); err != nil {
				slog.Error("Server shutdown error", "error", err)
			}
		}
		s.readerService.Cleanup()
		os.Exit(0)
	}()
}
