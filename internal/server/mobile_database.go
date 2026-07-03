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
//
// 架构（2026-07-03 改造）：
//   - sqlite 是 base engine（IsBase=true），始终启用，不可关闭
//   - 其他引擎是可选服务，可独立启用/禁用
//   - 每个引擎有 Capabilities 描述其能力（向量搜索、高性能写入等）
//   - Available = 构建时是否包含该引擎的编译 tag
//   - Enabled = 配置中是否启用该引擎
type EngineInfo struct {
	Name         string   `json:"name"`
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	IsBase       bool     `json:"is_base"`
	Available    bool     `json:"available"`
	Enabled      bool     `json:"enabled"`
	Reason       string   `json:"reason,omitempty"`
	Capabilities []string `json:"capabilities"`
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

	enabledMap := s.cfg.Database.EnableEngines
	if enabledMap == nil {
		enabledMap = map[string]bool{}
	}

	isObjectBoxAvailable := isObjectBoxAvailable()

	return []EngineInfo{
		{
			Name:         "sqlite",
			Label:        "SQLite",
			Description:  "默认底座数据库，纯 Go 实现（glebarez/sqlite），零依赖全平台可用",
			IsBase:       true,
			Available:    true,
			Enabled:      true,
			Capabilities: []string{"事务 ACID", "SQL 查询", "全平台兼容", "零依赖", "任务存储"},
		},
		{
			Name:         "libsql",
			Label:        "LibSQL",
			Description:  "SQLite 增强版，CGO 原生库，支持向量搜索和高性能写入",
			IsBase:       false,
			Available:    isLibsqlAvailable(),
			Enabled:      enabledMap["libsql"],
			Reason: func() string {
				if !isLibsqlAvailable() {
					return "当前构建未包含 LibSQL 引擎（编译时未加 -tags libsql）"
				}
				return ""
			}(),
			Capabilities: []string{"向量搜索", "高性能写入", "SQLite 兼容", "MVCC 并发"},
		},
		{
			Name:         "turso",
			Label:        "Turso",
			Description:  "LibSQL 的分布式版本，支持边缘同步，purego 实现（桌面端专用）",
			IsBase:       false,
			Available:    !isMobile,
			Enabled:      enabledMap["turso"],
			Reason: func() string {
				if isMobile {
					return "移动端暂不支持 Turso 引擎（purego 方案不兼容 Android）"
				}
				return ""
			}(),
			Capabilities: []string{"分布式同步", "边缘计算", "向量搜索", "嵌入式数据库"},
		},
		{
			Name:         "objectbox",
			Label:        "ObjectBox",
			Description:  "面向对象 NoSQL 数据库，原生支持对象关系，写入性能远超 SQLite",
			IsBase:       false,
			Available:    isObjectBoxAvailable,
			Enabled:      enabledMap["objectbox"],
			Reason: func() string {
				if !isObjectBoxAvailable {
					return "当前构建未包含 ObjectBox 引擎（编译时未加 -tags objectbox）"
				}
				return ""
			}(),
			Capabilities: []string{"对象存储", "高性能写入", "ACID 事务", "跨平台", "零拷贝"},
		},
	}
}

// isObjectBoxAvailable 检查当前构建是否包含 ObjectBox 引擎。
// 通过尝试创建 stub/real store 来判断。
func isObjectBoxAvailable() bool {
	// objectbox 包有 build tag 分离的实现：
	//   - 带 objectbox tag: 真实实现，New() 返回可用 store
	//   - 不带 tag: stub 实现，New() 返回 ErrObjectBoxUnavailable
	// 我们通过调用 New 并检查错误类型来判断。
	// 但为了不真的创建数据库文件，用空路径 + 立即关闭的方式。
	// 实际上 stub 直接返回错误，real 会尝试打开文件。
	// 更简单的方式：直接 import 后的包行为。
	// 但我们不能直接 import（会循环依赖 or build tag 问题），
	// 所以通过 db_init.go 里的全局变量来判断。
	return objectBoxAvailable
}

// objectBoxAvailable 由 db_init.go 在初始化时设置（通过检测 objectbox store 的可用性）。
var objectBoxAvailable = false

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
