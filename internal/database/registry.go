// Package database provides database lifecycle management including migrations, backups, and cleanup.
package database

import (
	"sort"
	"sync"

	"github.com/dombyte/solis/internal/database/migrations"
	"github.com/dombyte/solis/internal/logging"
)

// registryLogger is the package-level logger for registry operations.
var registryLogger = logging.NewComponentLogger("database.registry")

// logger is the package-level logger for database operations.

// Migration is an alias for migrations.Migration for convenience.
type Migration = migrations.Migration

// ErrNotImplemented is an alias for migrations.ErrNotImplemented.
var ErrNotImplemented = migrations.ErrNotImplemented

// MigrationFunc is an alias for migrations.MigrationFunc.
type MigrationFunc = migrations.MigrationFunc

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

// SchemaVersionConstants defines the version numbers used by the application.
const (
	// CurrentSchemaVersion is the latest schema version that this application version supports.
	// Increment this constant when adding new migrations.
	CurrentSchemaVersion = 2

	// MinCompatibleVersion is the minimum schema version that this application version can work with.
	// If a database has a version lower than this, migration will be required.
	MinCompatibleVersion = 1
)

// SchemaVersionTableSQL is an alias for migrations.SchemaVersionTableSQL.
const SchemaVersionTableSQL = migrations.SchemaVersionTableSQL
