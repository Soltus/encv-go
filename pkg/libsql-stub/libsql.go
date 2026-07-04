// Package libsql provides stub types to satisfy transitive imports from
// github.com/libsql/sqlite-antlr4-parser.
//
// The Rust libsql project (github.com/libsql/libsql) does not ship a Go module;
// this stub satisfies the import so that the ANTLR4 SQLite parser compiles.
// At runtime, use github.com/tursodatabase/go-libsql for actual libsql support.
package libsql

// LibsqlDb is a placeholder for the native libsql database handle.
type LibsqlDb struct{ ptr uintptr }

// LibsqlResult is a placeholder for query results.
type LibsqlResult struct{ ptr uintptr }

// LibsqlStmt is a placeholder for prepared statements.
type LibsqlStmt struct{ ptr uintptr }

// NewDb creates a new stub database handle.
func NewDb() (*LibsqlDb, error) { return &LibsqlDb{}, nil }
