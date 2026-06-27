// Package database provides database lifecycle management including migrations, backups, and cleanup.
package database

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"

	"github.com/dombyte/solis/internal/logging"
)

// registryLogger is the package-level logger for registry operations.
var registryLogger = logging.NewComponentLogger("database.registry")

// logger is the package-level logger for database operations.

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

// SimpleMigration is a basic implementation of Migration with just Up functionality.
type SimpleMigration struct {
	version    int
	description string
	upFunc     MigrationFunc
}

// NewSimpleMigration creates a new SimpleMigration with the given version, description, and up function.
func NewSimpleMigration(version int, description string, upFunc MigrationFunc) *SimpleMigration {
	return &SimpleMigration{
		version:    version,
		description: description,
		upFunc:     upFunc,
	}
}

// Version returns the target version for this migration.
func (m *SimpleMigration) Version() int {
	return m.version
}

// Description returns a human-readable description of what this migration does.
func (m *SimpleMigration) Description() string {
	return m.description
}

// Up applies the migration to the database.
func (m *SimpleMigration) Up(tx *sql.Tx) error {
	if m.upFunc != nil {
		return m.upFunc(tx)
	}
	return nil
}

// Down always returns ErrNotImplemented for SimpleMigration.
func (m *SimpleMigration) Down(tx *sql.Tx) error {
	return ErrNotImplemented
}

// MigrationRegistry manages a collection of migrations and tracks the current schema version.
type MigrationRegistry struct {
	mu         sync.RWMutex
	migrations map[int]Migration
	versions   []int // Sorted list of version numbers
}

// NewMigrationRegistry creates a new empty migration registry.
func NewMigrationRegistry() *MigrationRegistry {
	return &MigrationRegistry{
		migrations: make(map[int]Migration),
		versions:   make([]int, 0),
	}
}

// Register adds a migration to the registry.
// Migrations should be registered in order, but the registry will sort them internally.
func (r *MigrationRegistry) Register(migration Migration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	version := migration.Version()
	if _, exists := r.migrations[version]; exists {
		registryLogger.Warn().Msgf("Migration version already registered (version: %d)", version)
		return
	}

	r.migrations[version] = migration
	r.versions = append(r.versions, version)
	sort.Ints(r.versions)

	registryLogger.Debug().Msgf("Registered migration (version: %d, description: %s)", version, migration.Description())
}

// GetMigration returns the migration for the given version, or nil if not found.
func (r *MigrationRegistry) GetMigration(version int) Migration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.migrations[version]
}

// GetVersions returns a sorted slice of all registered version numbers.
func (r *MigrationRegistry) GetVersions() []int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions := make([]int, len(r.versions))
	copy(versions, r.versions)
	return versions
}

// GetMigrationsFrom returns all migrations from the given version (exclusive) to the latest.
// If fromVersion is greater than or equal to the latest version, returns an empty slice.
func (r *MigrationRegistry) GetMigrationsFrom(fromVersion int) []Migration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	migrations := make([]Migration, 0)
	for _, version := range r.versions {
		if version > fromVersion {
			migrations = append(migrations, r.migrations[version])
		}
	}
	return migrations
}

// GetMigrationsTo returns all migrations from the earliest to the given version (inclusive).
func (r *MigrationRegistry) GetMigrationsTo(toVersion int) []Migration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	migrations := make([]Migration, 0)
	for _, version := range r.versions {
		if version <= toVersion {
			migrations = append(migrations, r.migrations[version])
		}
	}
	return migrations
}

// LatestVersion returns the highest version number among registered migrations.
// Returns 0 if no migrations are registered.
func (r *MigrationRegistry) LatestVersion() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.versions) == 0 {
		return 0
	}
	return r.versions[len(r.versions)-1]
}

// Count returns the number of registered migrations.
func (r *MigrationRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.migrations)
}

// SchemaVersionConstants defines the version numbers used by the application.
const (
	// CurrentSchemaVersion is the latest schema version that this application version supports.
	// Increment this constant when adding new migrations.
	CurrentSchemaVersion = 1

	// MinCompatibleVersion is the minimum schema version that this application version can work with.
	// If a database has a version lower than this, migration will be required.
	MinCompatibleVersion = 1
)

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