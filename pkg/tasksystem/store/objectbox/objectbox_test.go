//go:build objectbox

package objectbox

import (
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/pkg/tasksystem"
	"github.com/Soltus/encv-go/pkg/tasksystem/store/storetest"
)

func TestStoreSuite(t *testing.T) {
	storetest.RunStoreTests(t, func(t *testing.T) tasksystem.Store {
		tmpDir := t.TempDir()
		dbDir := filepath.Join(tmpDir, "objectbox")
		store, err := New(dbDir)
		if err != nil {
			t.Fatalf("New store: %v", err)
		}
		t.Cleanup(func() { store.Close() })
		return store
	})
}
