// Package database provides database lifecycle management: migrations, backups and
// cleanup.
package database

import (
	"sort"
	"sync"

	"github.com/dombyte/solis/internal/database/migrations"
)

// Migration is an alias for migrations.Migration for convenience.
type Migration = migrations.Migration

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

// Register adds a migration to the registry; a duplicate version is ignored and
// reported as false. The registry sorts migrations internally.
func (r *MigrationRegistry) Register(migration Migration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	version := migration.Version()
	if _, exists := r.migrations[version]; exists {
		return false
	}
	r.migrations[version] = migration
	r.versions = append(r.versions, version)
	sort.Ints(r.versions)
	return true
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
	CurrentSchemaVersion = 3

	// MinCompatibleVersion is the oldest schema this version can migrate. The V1/V2
	// migrations were removed in v3, so older databases must be upgraded with a v2
	// release first.
	MinCompatibleVersion = 2
)

// SchemaVersionTableSQL is an alias for migrations.SchemaVersionTableSQL.
const SchemaVersionTableSQL = migrations.SchemaVersionTableSQL
