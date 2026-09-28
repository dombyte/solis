package database

import (
	"context"
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
func (e *MigrationExecutor) GetCurrentVersion(ctx context.Context, db *sql.DB) (int, error) {
	// Check if schema_version table exists
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='schema_version'`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to check for schema_version table: %w", err)
	}

	if count == 0 {
		// no schema_version table: a fresh or pre-migration-system database
		return 0, nil
	}

	// Get the highest version from schema_version table
	var version int
	err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version
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

// ApplyMigration applies a single migration and records it in one transaction.
func (e *MigrationExecutor) ApplyMigration(ctx context.Context, db *sql.DB,
	migration Migration,
) error {
	version := migration.Version()
	e.log.Info().Int("version", version).Str("description", migration.Description()).
		Msg("applying migration")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	if err := applyInTx(ctx, tx, migration); err != nil {
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
func applyInTx(ctx context.Context, tx *sql.Tx, migration Migration) error {
	version := migration.Version()
	if err := migration.Up(ctx, tx); err != nil {
		return fmt.Errorf("migration %d failed: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, SchemaVersionTableSQL); err != nil {
		return fmt.Errorf("failed to ensure schema_version table exists: %w", err)
	}
	_, err := tx.ExecContext(ctx, recordMigrationSQL, version, migration.Description())
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}
	return nil
}

// ApplyPendingMigrations applies all pending migrations to the database.
// Returns the number of migrations applied and any error that occurred.
func (e *MigrationExecutor) ApplyPendingMigrations(ctx context.Context, db *sql.DB,
	currentVersion int,
) (int, error) {
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
		if err := e.ApplyMigration(ctx, db, migration); err != nil {
			return appliedCount, fmt.Errorf("failed to apply migration %d: %w", migration.Version(), err)
		}
		appliedCount++
	}

	return appliedCount, nil
}
