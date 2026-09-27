package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
)

// MigrationExecutor handles the execution of database migrations.
type MigrationExecutor struct {
	registry *MigrationRegistry
	log      zerolog.Logger
}

// NewMigrationExecutor creates a new MigrationExecutor.
func NewMigrationExecutor(registry *MigrationRegistry, log zerolog.Logger) *MigrationExecutor {
	return &MigrationExecutor{registry: registry, log: log}
}

// GetCurrentVersion retrieves the current schema version from the database.
// Returns 0 if the schema_version table doesn't exist or is empty.
func (e *MigrationExecutor) GetCurrentVersion(db *sql.DB) (int, error) {
	// Check if schema_version table exists
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='schema_version'`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to check for schema_version table: %w", err)
	}

	if count == 0 {
		// schema_version table doesn't exist - this is a legacy database
		return 0, nil
	}

	// Get the highest version from schema_version table
	var version int
	err = db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version
		WHERE success = 1`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to query schema version: %w", err)
	}

	return version, nil
}

// GetPendingMigrations returns all migrations that need to be applied.
// These are migrations with version > currentVersion.
func (e *MigrationExecutor) GetPendingMigrations(currentVersion int) []Migration {
	return e.registry.GetMigrationsFrom(currentVersion)
}

// recordMigrationSQL marks a migration version as applied.
const recordMigrationSQL = `INSERT OR REPLACE INTO schema_version
	(version, description, applied_at, success) VALUES (?, ?, CURRENT_TIMESTAMP, 1)`

// markLegacySQL records a legacy database as V1 unless a V1 row already exists.
const markLegacySQL = `INSERT OR IGNORE INTO schema_version
	(version, description, applied_at, success) VALUES (?, ?, CURRENT_TIMESTAMP, 1)`

// ApplyMigration applies a single migration and records it in one transaction.
func (e *MigrationExecutor) ApplyMigration(db *sql.DB, migration Migration) error {
	version := migration.Version()
	e.log.Info().Int("version", version).Str("description", migration.Description()).
		Msg("applying migration")
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	if err := applyInTx(tx, migration); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}
	e.log.Info().Int("version", version).Msg("migration applied")
	return nil
}

// applyInTx runs the migration and records its version inside tx.
func applyInTx(tx *sql.Tx, migration Migration) error {
	version := migration.Version()
	if err := migration.Up(tx); err != nil {
		return fmt.Errorf("migration %d failed: %w", version, err)
	}
	if _, err := tx.Exec(SchemaVersionTableSQL); err != nil {
		return fmt.Errorf("failed to ensure schema_version table exists: %w", err)
	}
	if _, err := tx.Exec(recordMigrationSQL, version, migration.Description()); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}
	return nil
}

// ApplyPendingMigrations applies all pending migrations to the database.
// Returns the number of migrations applied and any error that occurred.
func (e *MigrationExecutor) ApplyPendingMigrations(db *sql.DB, currentVersion int) (int, error) {
	pending := e.GetPendingMigrations(currentVersion)
	if len(pending) == 0 {
		e.log.Info().Int("current_version", currentVersion).Msg("no pending migrations")
		return 0, nil
	}

	e.log.Info().Int("count", len(pending)).Int("current_version", currentVersion).
		Msg("applying pending migrations")

	appliedCount := 0
	for _, migration := range pending {
		e.log.Debug().Int("version", migration.Version()).Msg("migration starting")
		if err := e.ApplyMigration(db, migration); err != nil {
			return appliedCount, fmt.Errorf("failed to apply migration %d: %w", migration.Version(), err)
		}
		appliedCount++
	}

	return appliedCount, nil
}

// EnsureSchemaVersionTable ensures the schema_version table exists.
// This is a helper for legacy databases that don't have the table yet.
func (e *MigrationExecutor) EnsureSchemaVersionTable(db *sql.DB) error {
	_, err := db.Exec(SchemaVersionTableSQL)
	return err
}

// MarkLegacyAsV1 marks a legacy database (without schema_version table) as V1.
// This is used when migrating from a pre-migration-system database.
func (e *MigrationExecutor) MarkLegacyAsV1(db *sql.DB) error {
	// First ensure schema_version table exists
	if err := e.EnsureSchemaVersionTable(db); err != nil {
		return err
	}

	_, err := db.Exec(markLegacySQL, 1, "Legacy database - marked as V1")
	return err
}
