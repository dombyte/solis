// Package migrations provides database migration types and implementations.
package migrations

import (
	"database/sql"
	"fmt"
)

// Migration represents a database schema migration that can be applied or reverted.
type Migration interface {
	// Version returns the target version for this migration.
	Version() int

	// Description returns a human-readable description of what this migration does.
	Description() string

	// Up applies the migration to the database.
	// This should be implemented as idempotent (safe to run multiple times).
	Up(tx *sql.Tx) error

	// Down reverts the migration (optional for development/debugging).
	// Can return ErrNotImplemented if down migration is not supported.
	Down(tx *sql.Tx) error
}

// ErrNotImplemented is returned when a down migration is not implemented.
var ErrNotImplemented = fmt.Errorf("down migration not implemented")

// MigrationFunc is a function type that implements the Up method of Migration.
type MigrationFunc func(tx *sql.Tx) error

// SchemaVersionTableSQL is the SQL to create the schema version tracking table.
const SchemaVersionTableSQL = `
	CREATE TABLE IF NOT EXISTS schema_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version INTEGER NOT NULL UNIQUE,
		description TEXT,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		success BOOLEAN NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_schema_version ON schema_version(version);
`
