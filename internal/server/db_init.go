package server

import (
	"log/slog"
	"path/filepath"
	"runtime"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	sqlitestore "github.com/Soltus/encv-go/pkg/tasksystem/store/sqlite"
	tursostore "github.com/Soltus/encv-go/pkg/tasksystem/store/tursogo"
)

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
		}
	case "libsql":
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("libsql store init panicked, falling back to sqlite", "panic", r)
					actualEngine = "sqlite"
					dbStore, err = sqlitestore.New(dbPath)
					if err != nil {
						slog.Error("failed to init sqlite fallback", "err", err)
					}
				}
			}()
			store, initErr := initLibsqlStoreWithFallback(dbPath, &actualEngine)
			if initErr != nil || store == nil {
				if actualEngine == "sqlite" {
					dbStore, err = sqlitestore.New(dbPath)
					if err != nil {
						slog.Error("failed to init sqlite fallback", "err", err)
					}
				}
			} else {
				dbStore = store
			}
		}()
	case "turso":
		if runtime.GOOS == "android" || runtime.GOOS == "ios" {
			slog.Warn("turso engine not supported on mobile, falling back to sqlite", "os", runtime.GOOS)
			actualEngine = "sqlite"
			dbStore, err = sqlitestore.New(dbPath)
			if err != nil {
				slog.Error("failed to init sqlite fallback", "err", err)
			}
			break
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("turso store init panicked, falling back to sqlite", "panic", r)
					actualEngine = "sqlite"
					dbStore, err = sqlitestore.New(dbPath)
					if err != nil {
						slog.Error("failed to init sqlite fallback", "err", err)
					}
				}
			}()
			dbStore, err = tursostore.NewLocal(dbPath)
			if err != nil {
				slog.Error("failed to init turso store, falling back to sqlite", "err", err)
				actualEngine = "sqlite"
				dbStore, err = sqlitestore.New(dbPath)
				if err != nil {
					slog.Error("failed to init sqlite fallback", "err", err)
				}
			}
		}()
	default:
		slog.Warn("unknown database engine, falling back to sqlite", "engine", s.cfg.Database.Engine)
		actualEngine = "sqlite"
		dbStore, err = sqlitestore.New(dbPath)
	}
	if dbStore != nil {
		tm := s.mobileSvc.GetTaskManager()
		if err := tm.ReplaceStore(dbStore); err != nil {
			slog.Error("failed to replace store", "err", err)
		} else {
			slog.Info("database store initialized", "engine", actualEngine, "path", dbPath)
		}
	}
	return actualEngine, dbPath
}
