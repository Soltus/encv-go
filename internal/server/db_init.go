package server

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"

	"github.com/Soltus/encv-go/internal/kernel"
	"github.com/Soltus/encv-go/pkg/tasksystem"
	sqlitestore "github.com/Soltus/encv-go/pkg/tasksystem/store/sqlite"
	objectboxstore "github.com/Soltus/encv-go/pkg/tasksystem/store/objectbox"
	tursostore "github.com/Soltus/encv-go/pkg/tasksystem/store/tursogo"
)

// InitDatabase 初始化数据库 store，根据 cfg.Database.Engine 选择 sqlite / libsql / turso。
//
// 降级规则（参见 .trae/rules/graceful-degradation.md L2 级）：
//   - 失败时回退到 sqlite，但必须把真实失败原因写入 s.dbFallbackReason
//   - handleDatabaseInfo 优先使用 s.dbFallbackReason，不再硬编码"当前平台不支持"
//   - slog.Error 同时记录详细错误到日志，便于 CI / logcat 排查
//
// 2026-07-02 修复：此前 handleDatabaseInfo 把所有 libsql/turso 降级都说成"当前平台不支持"，
// 但真实原因可能是 C 库加载失败、PRAGMA 失败、schema 失败、panic 等，严重误导调试。
func (s *Server) InitDatabase(servingDir string) (string, string) {
	dbPath := s.cfg.Database.Path
	if dbPath == "" {
		dbPath = filepath.Join(servingDir, "encv-tasks.db")
	}
	var dbStore tasksystem.Store
	var err error
	actualEngine := s.cfg.Database.Engine
	if actualEngine == "" {
		actualEngine = "sqlite"
	}
	switch s.cfg.Database.Engine {
	case "", "sqlite":
		dbStore, err = sqlitestore.New(dbPath)
		if err != nil {
			slog.Error("failed to init sqlite store", "err", err)
			s.dbFallbackReason = fmt.Sprintf("SQLite 初始化失败: %v", err)
		}
	case "objectbox":
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("objectbox store init panicked, falling back to sqlite", "panic", r)
					s.dbFallbackReason = fmt.Sprintf("ObjectBox 初始化 panic: %v", r)
					actualEngine = "sqlite"
					dbStore, err = sqlitestore.New(dbPath)
					if err != nil {
						slog.Error("failed to init sqlite fallback", "err", err)
						s.dbFallbackReason = fmt.Sprintf("ObjectBox panic 后 SQLite fallback 也失败: %v", err)
					}
				}
			}()
			store, initErr := objectboxstore.New(filepath.Join(filepath.Dir(dbPath), "objectbox"))
			if initErr != nil {
				// stub 路径（未编译 objectbox 标签）或真实初始化失败
				if initErr == objectboxstore.ErrObjectBoxUnavailable {
					s.dbFallbackReason = "当前构建未包含 ObjectBox 引擎（编译时未加 -tags objectbox）"
				} else {
					s.dbFallbackReason = fmt.Sprintf("ObjectBox 初始化失败: %v", initErr)
				}
				slog.Warn("objectbox fallback to sqlite", "reason", s.dbFallbackReason)
				actualEngine = "sqlite"
				dbStore, err = sqlitestore.New(dbPath)
				if err != nil {
					slog.Error("failed to init sqlite fallback", "err", err)
					s.dbFallbackReason = fmt.Sprintf("ObjectBox 降级后 SQLite fallback 也失败: %v", err)
				}
			} else {
				dbStore = store
			}
		}()
	case "libsql":
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("libsql store init panicked, falling back to sqlite", "panic", r)
					s.dbFallbackReason = fmt.Sprintf("LibSQL 初始化 panic: %v", r)
					actualEngine = "sqlite"
					dbStore, err = sqlitestore.New(dbPath)
					if err != nil {
						slog.Error("failed to init sqlite fallback", "err", err)
						s.dbFallbackReason = fmt.Sprintf("LibSQL panic 后 SQLite fallback 也失败: %v", err)
					}
				}
			}()
			store, initErr := initLibsqlStoreWithFallback(dbPath, &actualEngine)
			if initErr != nil || store == nil {
				// initLibsqlStoreWithFallback 已经在内部把真实原因记到 slog，
				// 这里同时把它写到 s.dbFallbackReason 让前端可见。
				if initErr != nil {
					s.dbFallbackReason = fmt.Sprintf("LibSQL 初始化失败: %v", initErr)
				} else {
					// store == nil && initErr == nil → stub 路径（未编译 libsql 标签）
					s.dbFallbackReason = "当前构建未包含 LibSQL 引擎（编译时未加 -tags libsql）"
				}
				slog.Warn("libsql fallback to sqlite", "reason", s.dbFallbackReason)

				if actualEngine == "sqlite" {
					dbStore, err = sqlitestore.New(dbPath)
					if err != nil {
						slog.Error("failed to init sqlite fallback", "err", err)
						s.dbFallbackReason = fmt.Sprintf("LibSQL 降级后 SQLite fallback 也失败: %v", err)
					}
				}
			} else {
				dbStore = store
			}
		}()
	case "turso":
		if runtime.GOOS == "android" || runtime.GOOS == "ios" {
			slog.Warn("turso engine not supported on mobile, falling back to sqlite", "os", runtime.GOOS)
			s.dbFallbackReason = fmt.Sprintf("移动端(%s)不支持 Turso 引擎（purego 方案不兼容 Android）", runtime.GOOS)
			actualEngine = "sqlite"
			dbStore, err = sqlitestore.New(dbPath)
			if err != nil {
				slog.Error("failed to init sqlite fallback", "err", err)
				s.dbFallbackReason = fmt.Sprintf("Turso 降级后 SQLite fallback 也失败: %v", err)
			}
			break
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("turso store init panicked, falling back to sqlite", "panic", r)
					s.dbFallbackReason = fmt.Sprintf("Turso 初始化 panic: %v", r)
					actualEngine = "sqlite"
					dbStore, err = sqlitestore.New(dbPath)
					if err != nil {
						slog.Error("failed to init sqlite fallback", "err", err)
						s.dbFallbackReason = fmt.Sprintf("Turso panic 后 SQLite fallback 也失败: %v", err)
					}
				}
			}()
			dbStore, err = tursostore.NewLocal(dbPath)
			if err != nil {
				slog.Error("failed to init turso store, falling back to sqlite", "err", err)
				s.dbFallbackReason = fmt.Sprintf("Turso 初始化失败: %v", err)
				actualEngine = "sqlite"
				dbStore, err = sqlitestore.New(dbPath)
				if err != nil {
					slog.Error("failed to init sqlite fallback", "err", err)
					s.dbFallbackReason = fmt.Sprintf("Turso 降级后 SQLite fallback 也失败: %v", err)
				}
			}
		}()
	default:
		slog.Warn("unknown database engine, falling back to sqlite", "engine", s.cfg.Database.Engine)
		s.dbFallbackReason = fmt.Sprintf("未知数据库引擎: %q", s.cfg.Database.Engine)
		actualEngine = "sqlite"
		dbStore, err = sqlitestore.New(dbPath)
	}
	if dbStore != nil {
		tm := s.mobileSvc.GetTaskManager()
		if err := tm.ReplaceStore(dbStore); err != nil {
			slog.Error("failed to replace store", "err", err)
			s.dbFallbackReason = fmt.Sprintf("ReplaceStore 失败: %v", err)
		} else {
			slog.Info("database store initialized", "engine", actualEngine, "path", dbPath)
			// 初始化成功时清空降级原因（避免误报）
			if actualEngine == s.cfg.Database.Engine || s.cfg.Database.Engine == "" {
				s.dbFallbackReason = ""
			}
			// 将 DB 服务注册到微内核（如果启用了微内核）
			if s.microKernel != nil {
				s.registerDBService(actualEngine, dbStore)
			}
		}
	}
	return actualEngine, dbPath
}

// registerDBService 将数据库服务注册到微内核，并启用任务记录。
// 这样所有微服务调用都会自动写入任务表（同一个 SQLite/libsql/Turso 数据库）。
func (s *Server) registerDBService(engine string, store tasksystem.Store) {
	dbSvc := &kernel.DBService{}
	dbSvc.SetStore(store)
	s.microKernel.RegisterService("db", func() (kernel.Service, error) {
		return dbSvc, nil
	})
	slog.Info("kernel: db service registered", "engine", engine)

	// 启用任务记录（所有微内核调用自动写入 tasks 表）
	s.microKernel.EnableTaskRecording(&kernel.TaskRecordingConfig{
		Store:              store,
		DefaultTriggeredBy: "system",
		RecordErrors:       true,
		RecordSuccess:      true,
	})
	slog.Info("kernel: task recording enabled")
}
