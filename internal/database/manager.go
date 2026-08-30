package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/database/migrations"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/storage"
	_ "modernc.org/sqlite"
)

// logger is the package-level logger for database manager operations.
var managerLogger = logging.NewComponentLogger("database.manager")

// DatabaseManager manages the complete lifecycle of the application database,
// including migrations, backups, cleanup, and online backup scheduling.
type DatabaseManager struct {
	// config contains the storage configuration.
	config *config.StorageSettings
	// backupConfig contains the backup-specific configuration.
	backupConfig *BackupConfig
	// storage is the initialized storage instance (created during Initialize).
	storage *storage.Storage
	// registry contains all registered migrations.
	registry *MigrationRegistry
	// executor handles migration execution.
	executor *MigrationExecutor
	// db is the underlying SQLite database connection.
	db *sql.DB
	// dbPath is the path to the database file.
	dbPath string
	// isInitialized tracks if the manager has been initialized.
	isInitialized bool
}

// NewDatabaseManager creates a new DatabaseManager.
func NewDatabaseManager(storageConfig *config.StorageSettings, backupConfig *BackupConfig) *DatabaseManager {
	// Create migration registry
	registry := NewMigrationRegistry()

	// Register migrations from the migrations package
	registry.Register(migrations.GetV1Migration())
	registry.Register(migrations.GetV2Migration())

	// Register the V2 migration from the migrations package
	registry.Register(migrations.GetV2Migration())

	return &DatabaseManager{
		config:        storageConfig,
		backupConfig:  backupConfig,
		registry:      registry,
		dbPath:        storageConfig.Path,
		isInitialized: false,
	}
}

// Initialize performs the complete database initialization sequence.
// This includes:
// 1. Opening or creating the database file
// 2. Checking current schema version
// 3. Creating backup if needed
// 4. Applying pending migrations
// 5. Cleaning up old backups
// 6. Returning the initialized Storage
func (m *DatabaseManager) Initialize() (*storage.Storage, error) {
	if m.isInitialized {
		return m.storage, nil
	}

	managerLogger.Info().Msgf("Starting database initialization (path: %s)", m.dbPath)

	dbFileExists := m.checkDatabaseFileExists()

	db, err := m.openAndVerifyDatabase()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	currentVersion, err := m.getCurrentSchemaVersion(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get current schema version: %w", err)
	}
	managerLogger.Info().Msgf("Current schema version: %d", currentVersion)

	if dbFileExists {
		m.createBackupIfNeeded()
	}

	if err := m.applyPendingMigrations(db, currentVersion); err != nil {
		return nil, err
	}

	m.cleanupOldBackups()

	st, err := m.createStorageInstance()
	if err != nil {
		return nil, err
	}

	m.runStartupCleanup(st)

	managerLogger.Info().Msg("Database initialization completed successfully")
	return st, nil
}

// checkDatabaseFileExists checks if the database file exists
func (m *DatabaseManager) checkDatabaseFileExists() bool {
	_, statErr := os.Stat(m.dbPath)
	if statErr == nil {
		return true
	}
	if !os.IsNotExist(statErr) {
		managerLogger.Error().Msgf("Error checking database file: %v", statErr)
	}
	return false
}

// openAndVerifyDatabase opens the database and verifies the connection
func (m *DatabaseManager) openAndVerifyDatabase() (*sql.DB, error) {
	db, err := sql.Open("sqlite", m.dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// createBackupIfNeeded creates a backup if the database file exists
func (m *DatabaseManager) createBackupIfNeeded() {
	backupPath, err := CreateBackup(m.dbPath, m.backupConfig)
	if err != nil {
		managerLogger.Error().Msgf("Failed to create backup: %v", err)
		managerLogger.Warn().Msg("Proceeding without backup - data may be at risk")
	} else {
		managerLogger.Info().Msgf("Backup created: %s", backupPath)
	}
}

// applyPendingMigrations applies any pending migrations
func (m *DatabaseManager) applyPendingMigrations(db *sql.DB, currentVersion int) error {
	if currentVersion < CurrentSchemaVersion {
		managerLogger.Info().Msgf("Database needs migration (current: %d, target: %d)", currentVersion, CurrentSchemaVersion)

		appliedCount, err := m.applyMigrations(db, currentVersion)
		if err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		managerLogger.Info().Msgf("Migrations applied: %d", appliedCount)
	} else {
		managerLogger.Info().Msgf("Database is up to date (version: %d)", currentVersion)
	}
	return nil
}

// cleanupOldBackups cleans up old backup files
func (m *DatabaseManager) cleanupOldBackups() {
	if m.backupConfig.Enabled && m.backupConfig.MaxBackups > 0 {
		if err := CleanupBackups(m.dbPath, m.backupConfig.MaxBackups); err != nil {
			managerLogger.Warn().Msgf("Failed to cleanup old backups: %v", err)
		}
	}
}

// createStorageInstance creates a new Storage instance
func (m *DatabaseManager) createStorageInstance() (*storage.Storage, error) {
	managerLogger.Info().Msg("Creating Storage instance")

	st, err := storage.New(m.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Storage: %w", err)
	}

	m.storage = st
	m.db = st.DB()
	m.isInitialized = true

	return st, nil
}

// runStartupCleanup runs retention cleanup on startup
func (m *DatabaseManager) runStartupCleanup(st *storage.Storage) {
	managerLogger.Info().Msg("Running retention cleanup on startup")
	if err := st.CleanupAll(); err != nil {
		managerLogger.Warn().Msgf("Startup retention cleanup failed (will retry later via poller): %v", err)
	} else {
		managerLogger.Info().Msg("Startup retention cleanup completed")
	}
}

// getCurrentSchemaVersion retrieves the current schema version from the database.
func (m *DatabaseManager) getCurrentSchemaVersion(db *sql.DB) (int, error) {
	// First create the executor if not already done
	if m.executor == nil {
		m.executor = NewMigrationExecutor(m.registry, m.backupConfig, m.dbPath)
	}

	return m.executor.GetCurrentVersion(db)
}

// applyMigrations applies all pending migrations.
func (m *DatabaseManager) applyMigrations(db *sql.DB, currentVersion int) (int, error) {
	// Handle legacy database case
	if currentVersion == 0 {
		managerLogger.Info().Msg("Legacy database detected, marking as V1")
		if err := m.executor.MarkLegacyAsV1(db); err != nil {
			return 0, fmt.Errorf("failed to mark legacy database as V1: %w", err)
		}
		currentVersion = 1
	}

	// Apply pending migrations
	return m.executor.ApplyPendingMigrations(db, currentVersion)
}

// StartPeriodicBackups starts a background goroutine that creates online backups
// at the configured interval. It stops when the context is cancelled.
func (m *DatabaseManager) StartPeriodicBackups(ctx context.Context) error {
	if !m.backupConfig.Enabled || m.backupConfig.BackupInterval <= 0 {
		managerLogger.Debug().Msg("Periodic backups disabled or interval not configured")
		return nil
	}

	managerLogger.Info().Msgf("Starting periodic online backups (interval: %s)", m.backupConfig.BackupInterval)

	go func() {
		ticker := time.NewTicker(m.backupConfig.BackupInterval)
		defer ticker.Stop()

		// Create initial backup after startup
		select {
		case <-ctx.Done():
			managerLogger.Debug().Msg("Periodic backups stopped before first backup")
			return
		case <-ticker.C:
			m.createOnlineBackup()
		}

		for {
			select {
			case <-ctx.Done():
				managerLogger.Info().Msg("Periodic backups stopped")
				return
			case <-ticker.C:
				m.createOnlineBackup()
			}
		}
	}()

	return nil
}

// createOnlineBackup creates a backup of the current database.
func (m *DatabaseManager) createOnlineBackup() {
	if m.storage == nil || m.db == nil {
		managerLogger.Warn().Msg("Cannot create backup: DatabaseManager not initialized")
		return
	}

	managerLogger.Info().Msg("Creating backup")

	// Create backup
	backupPath, err := CreateBackup(m.dbPath, m.backupConfig)
	if err != nil {
		managerLogger.Error().Msgf("Failed to create online backup: %v", err)
		return
	}

	// Cleanup old backups
	if m.backupConfig.MaxBackups > 0 {
		if err := CleanupBackups(m.dbPath, m.backupConfig.MaxBackups); err != nil {
			managerLogger.Warn().Msgf("Failed to cleanup old backups after online backup: %v", err)
		}
	}

	managerLogger.Info().Msgf("Online backup created successfully: %s", backupPath)
}

// StartPeriodicCleanup starts a background goroutine that runs retention cleanup
// at the configured interval. It should be called if the poller is not running (serve-only mode).
func (m *DatabaseManager) StartPeriodicCleanup(ctx context.Context) error {
	if m.storage == nil || m.storage.Config() == nil || m.storage.Config().CleanupInterval <= 0 {
		managerLogger.Debug().Msg("Periodic cleanup disabled or not configured")
		return nil
	}

	cleanupInterval := m.storage.Config().CleanupInterval
	managerLogger.Info().Msgf("Starting periodic retention cleanup (interval: %s)", cleanupInterval)

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	// Run cleanup immediately on startup
	m.runCleanup()

	go func() {
		for {
			select {
			case <-ctx.Done():
				managerLogger.Info().Msg("Periodic retention cleanup stopped")
				return
			case <-ticker.C:
				m.runCleanup()
			}
		}
	}()

	return nil
}

// runCleanup executes the retention cleanup.
func (m *DatabaseManager) runCleanup() {
	if m.storage == nil {
		managerLogger.Warn().Msg("Cannot run cleanup: storage not configured")
		return
	}

	managerLogger.Debug().Msg("Running retention cleanup...")
	if err := m.storage.CleanupAll(); err != nil {
		managerLogger.Error().Msgf("Retention cleanup failed: %v", err)
	} else {
		managerLogger.Info().Msg("Retention cleanup completed successfully")
	}
}

// Close closes the database connection and cleans up resources.
func (m *DatabaseManager) Close() error {
	if m.storage != nil {
		return m.storage.Close()
	}
	return nil
}
