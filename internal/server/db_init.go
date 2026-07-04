package server

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Soltus/encv-go/internal/kernel"
	"github.com/Soltus/encv-go/pkg/tasksystem"
	sqlitestore "github.com/Soltus/encv-go/pkg/tasksystem/store/sqlite"
	objectboxstore "github.com/Soltus/encv-go/pkg/tasksystem/store/objectbox"
	tursostore "github.com/Soltus/encv-go/pkg/tasksystem/store/tursogo"
)

// initObjectBoxAvailability 检测当前构建是否包含 ObjectBox 引擎。
//
// 通过尝试调用 objectboxstore.New 并检查错误类型来判断：
//   - stub 构建（无 objectbox tag）：返回 ErrObjectBoxUnavailable
//   - 真实构建（有 objectbox tag）：会尝试打开数据库文件
//
// 为了不真的创建数据库文件，我们用临时目录 + 立即清理的方式。
func initObjectBoxAvailability() {
	// 2026-07-04：添加 panic recovery 和 goroutine 超时保护。
	// 原因：Android 真机上如果 binary 包含 objectbox CGO 但 C 库缺失/不匹配，
	// objectboxstore.New() 在 C 层可能 panic 或 hang，导致整个 Go 进程 crash。
	// 该函数在 InitDatabase 的 switch 之前被调用，不在 switch 的 recover 保护内。
	defer func() {
		if r := recover(); r != nil {
			slog.Error("objectbox probe: panicked, treating as unavailable", "panic", r)
			objectBoxAvailable = false
		}
	}()

	// 使用带超时的 goroutine 执行探测，避免 C 层 hang 导致整个启动卡住。
	type probeResult struct {
		available bool
		err       error
	}
	probeCh := make(chan probeResult, 1)
	go func() {
		tmpDir, err := os.MkdirTemp("", "encv-objectbox-probe-*")
		if err != nil {
			slog.Warn("objectbox probe: failed to create temp dir", "err", err)
			probeCh <- probeResult{available: false, err: err}
			return
		}
		defer os.RemoveAll(tmpDir)

		store, err := objectboxstore.New(tmpDir)
		if err != nil {
			if err == objectboxstore.ErrObjectBoxUnavailable {
				probeCh <- probeResult{available: false, err: err}
			} else {
				slog.Warn("objectbox probe: init failed but engine is available", "err", err)
				probeCh <- probeResult{available: true, err: err}
			}
			return
		}
		probeCh <- probeResult{available: true, err: nil}
		if store != nil {
			_ = store.Close()
		}
	}()

	select {
	case res := <-probeCh:
		objectBoxAvailable = res.available
		if res.err != nil && res.err != objectboxstore.ErrObjectBoxUnavailable {
			slog.Warn("objectbox probe: init error", "err", res.err)
		}
	case <-time.After(3 * time.Second):
		// 3 秒超时：C 层 hang 或设备性能极差
		slog.Error("objectbox probe: timed out after 3s, treating as unavailable")
		objectBoxAvailable = false
	}
}

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
	// 先检测各引擎可用性（设置全局变量供 getAvailableEngines 使用）
	initObjectBoxAvailability()

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
