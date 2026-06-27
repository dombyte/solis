package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dombyte/solis/internal/config"
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
	
	// Register V1 migration directly to avoid circular imports
	// In the future, we can use a better registration mechanism
	registry.Register(V1MigrationForManager())
	
	return &DatabaseManager{
		config:       storageConfig,
		backupConfig: backupConfig,
		registry:     registry,
		dbPath:       storageConfig.Path,
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

	// Step 1: Check if database file exists
	dbFileExists := false
	if _, err := os.Stat(m.dbPath); err == nil {
		dbFileExists = true
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("error checking database file: %w", err)
	}

	// Step 2: Open database connection
	db, err := sql.Open("sqlite", m.dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Configure connection pool
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Step 3: Check current schema version
	currentVersion, err := m.getCurrentSchemaVersion(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get current schema version: %w", err)
	}

	managerLogger.Info().Msgf("Current schema version: %d", currentVersion)

	// Step 4: Always create a backup if database file exists
	if dbFileExists {
		backupPath, err := CreateBackup(m.dbPath, m.backupConfig)
		if err != nil {
			managerLogger.Error().Msgf("Failed to create backup: %v", err)
			// Continue without backup - this is a warning, not a failure
			managerLogger.Warn().Msg("Proceeding without backup - data may be at risk")
		} else {
			managerLogger.Info().Msgf("Backup created: %s", backupPath)
		}
	}

	// Step 5: If this is a legacy database (version 0) or version mismatch, migrate
	if currentVersion < CurrentSchemaVersion {
		managerLogger.Info().Msgf("Database needs migration (current: %d, target: %d)", currentVersion, CurrentSchemaVersion)

		// Apply pending migrations
		appliedCount, err := m.applyMigrations(db, currentVersion)
		if err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}
		managerLogger.Info().Msgf("Migrations applied: %d", appliedCount)
	} else {
		managerLogger.Info().Msgf("Database is up to date (version: %d)", currentVersion)
	}

	// Step 5: Cleanup old backups
	if m.backupConfig.Enabled && m.backupConfig.MaxBackups > 0 {
		if err := CleanupBackups(m.dbPath, m.backupConfig.MaxBackups); err != nil {
			managerLogger.Warn().Msgf("Failed to cleanup old backups: %v", err)
			// Continue - this is not a failure
		}
	}

	// Step 6: Initialize Storage instance with the database connection
	// We need to create a new Storage instance with the database
	managerLogger.Info().Msg("Creating Storage instance")

	// Since we can't reuse the db connection (Storage creates its own),
	// we'll let Storage handle the connection
	st, err := storage.New(m.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Storage: %w", err)
	}

	m.storage = st
	m.db = st.DB() // Store reference to the connection
	m.isInitialized = true

	managerLogger.Info().Msg("Database initialization completed successfully")

	return st, nil
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

// CreateBackup creates a backup of the database.
func (m *DatabaseManager) CreateBackup() (string, error) {
	if !m.isInitialized {
		return "", errors.New("DatabaseManager not initialized")
	}
	return CreateBackup(m.dbPath, m.backupConfig)
}

// CleanupBackups performs cleanup of old backup files.
func (m *DatabaseManager) CleanupBackups() error {
	if !m.isInitialized {
		return errors.New("DatabaseManager not initialized")
	}
	return CleanupBackups(m.dbPath, m.backupConfig.MaxBackups)
}

// GetBackupList returns a list of all backup files.
func (m *DatabaseManager) GetBackupList() ([]BackupInfo, error) {
	if !m.isInitialized {
		return nil, errors.New("DatabaseManager not initialized")
	}
	return ListBackups(m.dbPath)
}

// GetCurrentVersion returns the current schema version.
func (m *DatabaseManager) GetCurrentVersion() (int, error) {
	if !m.isInitialized || m.db == nil {
		return 0, errors.New("DatabaseManager not initialized")
	}
	return m.executor.GetCurrentVersion(m.db)
}

// RegisterMigrations registers additional migrations with the manager.
// This can be used by other packages to add their own migrations.
func (m *DatabaseManager) RegisterMigrations(migrations ...Migration) {
	for _, migration := range migrations {
		m.registry.Register(migration)
	}
}

// RegisterAllMigrations registers all known migrations from the migrations package.
func (m *DatabaseManager) RegisterAllMigrations() {
	// Import the migrations package and register all migrations
	// Since we can't import it directly (circular dependency), we'll use a function
	// that can be called to register migrations
	// For now, we'll register the V1 migration directly
	m.registry.Register(V1MigrationForManager())
}

// V1MigrationForManager returns the V1 migration that can be registered with the manager.
// This is a temporary solution until we resolve the circular import issue.
func V1MigrationForManager() Migration {
	// We'll create a simple migration here to avoid import issues
	return &v1MigrationImpl{}
}

// v1MigrationImpl implements the Migration interface for V1 schema.
type v1MigrationImpl struct{}

func (m *v1MigrationImpl) Version() int {
	return 1
}

func (m *v1MigrationImpl) Description() string {
	return "Initial schema"
}

func (m *v1MigrationImpl) Up(tx *sql.Tx) error {
	// This is the same SQL as in the migrations package
	tables := []string{
		`CREATE TABLE IF NOT EXISTS daily_values (id INTEGER PRIMARY KEY AUTOINCREMENT, date DATE NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL, raw_value REAL NOT NULL, UNIQUE(register_key, date));`,
		`CREATE INDEX IF NOT EXISTS idx_daily_key_date ON daily_values(register_key, date);`,
		`CREATE INDEX IF NOT EXISTS idx_daily_date ON daily_values(date);`,
		`CREATE TABLE IF NOT EXISTS monthly_values (id INTEGER PRIMARY KEY AUTOINCREMENT, month TEXT NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL, raw_value REAL NOT NULL, UNIQUE(register_key, month));`,
		`CREATE INDEX IF NOT EXISTS idx_monthly_key_month ON monthly_values(register_key, month);`,
		`CREATE INDEX IF NOT EXISTS idx_monthly_month ON monthly_values(month);`,
		`CREATE TABLE IF NOT EXISTS yearly_values (id INTEGER PRIMARY KEY AUTOINCREMENT, year TEXT NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL, raw_value REAL NOT NULL, UNIQUE(register_key, year));`,
		`CREATE INDEX IF NOT EXISTS idx_yearly_key_year ON yearly_values(register_key, year);`,
		`CREATE INDEX IF NOT EXISTS idx_yearly_year ON yearly_values(year);`,
		`CREATE TABLE IF NOT EXISTS total_values (id INTEGER PRIMARY KEY AUTOINCREMENT, register_key TEXT NOT NULL UNIQUE, value REAL NOT NULL, raw_value REAL NOT NULL, timestamp DATETIME NOT NULL);`,
		`CREATE INDEX IF NOT EXISTS idx_total_key ON total_values(register_key);`,
		`CREATE TABLE IF NOT EXISTS error_data (id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp DATETIME NOT NULL, register_key TEXT NOT NULL, raw_value REAL NOT NULL, string_value TEXT, UNIQUE(register_key, timestamp));`,
		`CREATE INDEX IF NOT EXISTS idx_error_key_timestamp ON error_data(register_key, timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_error_timestamp ON error_data(timestamp);`,
		SchemaVersionTableSQL,
	}

	for _, sql := range tables {
		if _, err := tx.Exec(sql); err != nil {
			return err
		}
	}

	// Insert version record
	insertSQL := `INSERT OR IGNORE INTO schema_version (version, description, applied_at, success) VALUES (?, ?, CURRENT_TIMESTAMP, 1)`
	_, err := tx.Exec(insertSQL, 1, "Initial schema")
	return err
}

func (m *v1MigrationImpl) Down(tx *sql.Tx) error {
	return ErrNotImplemented
}

// GetStorage returns the initialized Storage instance.
func (m *DatabaseManager) GetStorage() *storage.Storage {
	return m.storage
}

// Close closes the database connection and cleans up resources.
func (m *DatabaseManager) Close() error {
	if m.storage != nil {
		return m.storage.Close()
	}
	return nil
}

// VerifyDatabasePath ensures the database directory exists.
func VerifyDatabasePath(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "" || dir == "." {
		return nil
	}
	
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}
	
	return nil
}