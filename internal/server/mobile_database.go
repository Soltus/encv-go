package server

// mobile_database.go — 数据库管理：info / export / import / backup / restore / available_engines。

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/gin-gonic/gin"
)

// EngineInfo 数据库引擎信息（/api/database/info 响应里列出当前可用引擎）
type EngineInfo struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func (s *Server) handleDatabaseInfo(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()
	actualEngine := tm.GetStoreEngine()
	requestedEngine := s.cfg.Database.Engine
	if requestedEngine == "" {
		requestedEngine = "sqlite"
	}
	concurrency := tm.GetStoreConcurrency()

	fallbackReason := ""
	if requestedEngine != actualEngine {
		// 🆕 2026-07-02 修复：优先使用 InitDatabase 记录的真实失败原因
		//
		// 历史：硬编码"当前平台不支持 Turso/LibSQL 引擎"，但真实原因可能是
		//       C 库加载失败 / PRAGMA 失败 / schema 失败 / panic / 未编译 libsql 标签等。
		//       这违反 graceful-degradation.md L2 规范："降级原因明确"。
		//
		// 现在：InitDatabase 把真实失败原因写入 s.dbFallbackReason，这里直接透传给前端。
		//       如果 s.dbFallbackReason 为空（理论上不应该，但作为兜底），才用旧的硬编码消息。
		if s.dbFallbackReason != "" {
			fallbackReason = s.dbFallbackReason
		} else if requestedEngine == "turso" || requestedEngine == "libsql" {
			fallbackReason = "当前平台不支持 Turso/LibSQL 引擎，已自动回退到 SQLite"
		} else {
			fallbackReason = "引擎初始化失败，已自动回退到 SQLite"
		}
	}

	availableEngines := s.getAvailableEngines()

	totalTasks := 0
	hasCalibration := false
	if store := tm.GetStore(); store != nil {
		if tasks, err := store.ListTasks(tasksystem.TaskFilter{Limit: 0}); err == nil {
			totalTasks = len(tasks)
		}
		if cal, err := store.GetCalibration(); err == nil && cal != nil {
			hasCalibration = true
		}
	} else {
		tasks, _ := tm.ListPaginated("", 0, 0)
		totalTasks = len(tasks)
	}

	c.JSON(http.StatusOK, gin.H{
		"engine":           actualEngine,
		"requestedEngine":  requestedEngine,
		"fallbackReason":   fallbackReason,
		"availableEngines": availableEngines,
		"concurrency":      concurrency,
		"taskCount":        totalTasks,
		"hasCalibration":   hasCalibration,
	})
}

func (s *Server) getAvailableEngines() []EngineInfo {
	isMobile := runtime.GOOS == "android" || runtime.GOOS == "ios"
	return []EngineInfo{
		{
			Name:      "sqlite",
			Label:     "SQLite",
			Available: true,
		},
		{
			Name:      "libsql",
			Label:     "LibSQL",
			Available: isLibsqlAvailable(),
			Reason: func() string {
				if !isLibsqlAvailable() {
					return "当前构建未包含 LibSQL 引擎"
				}
				return ""
			}(),
		},
		{
			Name:      "turso",
			Label:     "Turso",
			Available: !isMobile,
			Reason: func() string {
				if isMobile {
					return "移动端暂不支持 Turso 引擎"
				}
				return ""
			}(),
		},
	}
}

func (s *Server) handleDatabaseExport(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()

	dump, err := tm.ExportDatabase()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 设置下载文件名
	filename := fmt.Sprintf("encv-db-%s-%s.json",
		dump.Engine,
		dump.ExportedAt.Format("20060102-150405"),
	)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/json")

	c.JSON(http.StatusOK, dump)
}

func (s *Server) handleDatabaseImport(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()

	var dump tasksystem.DatabaseDump
	if err := c.ShouldBindJSON(&dump); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}

	if dump.Version == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dump: missing version"})
		return
	}

	slog.Info("API: database import",
		"sourceEngine", dump.Engine,
		"taskCount", len(dump.Tasks),
		"trashCount", len(dump.Trash),
		"snapshotCount", len(dump.Snapshots),
		"metricCount", len(dump.Metrics),
	)

	if err := tm.ImportDatabase(&dump); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("API: database import completed")
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"imported": gin.H{
			"tasks":     len(dump.Tasks),
			"trash":     len(dump.Trash),
			"snapshots": len(dump.Snapshots),
			"metrics":   len(dump.Metrics),
		},
	})
}

func (s *Server) handleDatabaseBackup(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()

	dump, err := tm.ExportDatabase()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 生成备份文件路径（在 servingDir 下的 .backups 目录）
	backupDir := filepath.Join(s.servingDir, ".encv-backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create backup dir: " + err.Error()})
		return
	}

	filename := fmt.Sprintf("encv-db-%s-%s.json",
		dump.Engine,
		dump.ExportedAt.Format("20060102-150405"),
	)
	filePath := filepath.Join(backupDir, filename)

	data, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal: " + err.Error()})
		return
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write file: " + err.Error()})
		return
	}

	slog.Info("API: database backup created", "path", filePath, "size", len(data))

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"path":   filePath,
		"size":   len(data),
		"name":   filename,
	})
}

func (s *Server) handleDatabaseRestore(c *gin.Context) {
	tm := s.mobileSvc.GetTaskManager()

	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	if req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	// 安全检查：路径必须在 servingDir 内（防止路径穿越）
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	absServingDir, _ := filepath.Abs(s.servingDir)
	if !strings.HasPrefix(absPath, absServingDir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path outside serving directory"})
		return
	}

	// 读取文件
	data, err := os.ReadFile(absPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read file: " + err.Error()})
		return
	}

	var dump tasksystem.DatabaseDump
	if err := json.Unmarshal(data, &dump); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}

	slog.Info("API: database restore",
		"path", absPath,
		"sourceEngine", dump.Engine,
		"taskCount", len(dump.Tasks),
	)

	if err := tm.ImportDatabase(&dump); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("API: database restore completed")
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"restored": gin.H{
			"tasks":     len(dump.Tasks),
			"trash":     len(dump.Trash),
			"snapshots": len(dump.Snapshots),
			"metrics":   len(dump.Metrics),
		},
	})
}
