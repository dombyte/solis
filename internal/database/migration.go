package database

import (
	"database/sql"
	"fmt"

	"github.com/dombyte/solis/internal/logging"
)

// migrationLogger is the package-level logger for migration operations.
var migrationLogger = logging.NewComponentLogger("database.migration")

// MigrationExecutor handles the execution of database migrations.
type MigrationExecutor struct {
	registry *MigrationRegistry
	config   *BackupConfig
	dbPath   string
}

// NewMigrationExecutor creates a new MigrationExecutor.
func NewMigrationExecutor(registry *MigrationRegistry, config *BackupConfig, dbPath string) *MigrationExecutor {
	return &MigrationExecutor{
		registry: registry,
		config:   config,
		dbPath:   dbPath,
	}
}

// GetCurrentVersion retrieves the current schema version from the database.
// Returns 0 if the schema_version table doesn't exist or is empty.
func (e *MigrationExecutor) GetCurrentVersion(db *sql.DB) (int, error) {
	// Check if schema_version table exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to check for schema_version table: %w", err)
	}

	if count == 0 {
		// schema_version table doesn't exist - this is a legacy database
		return 0, nil
	}

	// Get the highest version from schema_version table
	var version int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version WHERE success = 1").Scan(&version)
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

// ApplyMigration applies a single migration.
func (e *MigrationExecutor) ApplyMigration(db *sql.DB, migration Migration) error {
	version := migration.Version()
	description := migration.Description()

	migrationLogger.Info().Msgf("Applying migration (version: %d, description: %s)", version, description)

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}

	// Apply the migration
	if err := migration.Up(tx); err != nil {
		_ = tx.Rollback() // #nosec G104
		return fmt.Errorf("migration %d failed: %w", version, err)
	}

	// Record successful migration in schema_version table
	// First ensure schema_version table exists
	if _, err := tx.Exec(SchemaVersionTableSQL); err != nil {
		_ = tx.Rollback() // #nosec G104
		return fmt.Errorf("failed to ensure schema_version table exists: %w", err)
	}

	// Insert or update version record
	insertSQL := `INSERT OR REPLACE INTO schema_version (version, description, applied_at, success) VALUES (?, ?, CURRENT_TIMESTAMP, 1)`
	if _, err := tx.Exec(insertSQL, version, description); err != nil {
		_ = tx.Rollback() // #nosec G104
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	migrationLogger.Info().Msgf("Migration applied successfully (version: %d)", version)

	return nil
}

// ApplyPendingMigrations applies all pending migrations to the database.
// Returns the number of migrations applied and any error that occurred.
func (e *MigrationExecutor) ApplyPendingMigrations(db *sql.DB, currentVersion int) (int, error) {
	pending := e.GetPendingMigrations(currentVersion)
	if len(pending) == 0 {
		migrationLogger.Info().Msgf("No pending migrations (current_version: %d)", currentVersion)
		return 0, nil
	}

	migrationLogger.Info().Msgf("Applying pending migrations (count: %d, current_version: %d)", len(pending), currentVersion)

	appliedCount := 0
	for _, migration := range pending {
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

	// Mark as V1
	insertSQL := `INSERT OR IGNORE INTO schema_version (version, description, applied_at, success) VALUES (?, ?, CURRENT_TIMESTAMP, 1)`
	_, err := db.Exec(insertSQL, 1, "Legacy database - marked as V1")
	return err
}
