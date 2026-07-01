//go:build !libsql
// +build !libsql

package server

import (
	"log/slog"

	"github.com/Soltus/encv-go/pkg/tasksystem"
)

func initLibsqlStore(dbPath string) (tasksystem.Store, error) {
	slog.Warn("libsql engine not compiled in")
	return nil, nil
}

func isLibsqlAvailable() bool {
	return false
}

func initLibsqlStoreWithFallback(dbPath string, actualEngine *string) (tasksystem.Store, error) {
	slog.Warn("libsql engine not compiled in, falling back to sqlite")
	*actualEngine = "sqlite"
	return nil, nil
}
