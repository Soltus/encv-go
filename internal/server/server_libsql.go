//go:build libsql
// +build libsql

package server

import (
	"log/slog"

	libsqlstore "github.com/Soltus/encv-go/pkg/tasksystem/store/libsql"
	"github.com/Soltus/encv-go/pkg/tasksystem"
)

func initLibsqlStore(dbPath string) (tasksystem.Store, error) {
	return libsqlstore.NewLocal(dbPath)
}

func isLibsqlAvailable() bool {
	return true
}

func initLibsqlStoreWithFallback(dbPath string, actualEngine *string) (tasksystem.Store, error) {
	store, err := initLibsqlStore(dbPath)
	if err != nil {
		slog.Error("failed to init libsql store, falling back to sqlite", "err", err)
		*actualEngine = "sqlite"
		return nil, err
	}
	return store, nil
}
