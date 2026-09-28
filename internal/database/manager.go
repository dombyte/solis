package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	_ "modernc.org/sqlite" // SQLite driver

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/database/migrations"
	"github.com/dombyte/solis/internal/utils"
)

// Cleaner runs retention cleanup (implemented by storage).
type Cleaner interface {
	CleanupAll(ctx context.Context) error
}

// Manager owns the database lifecycle outside the storage connection: pre-migration
// backups, schema migrations, periodic online backups and retention cleanup scheduling.
type Manager struct {
	cfg      *config.StorageSettings
	backup   *BackupConfig
	registry *MigrationRegistry
	executor *MigrationExecutor
	clock    utils.Clock
	log      zerolog.Logger
}

// NewManager creates a manager with all migrations registered.
func NewManager(cfg *config.StorageSettings, backup *BackupConfig, clock utils.Clock,
	log zerolog.Logger) *Manager {
	registry := NewMigrationRegistry()
	registry.Register(migrations.GetV1Migration())
	registry.Register(migrations.GetV2Migration())
	registry.Register(migrations.GetV3Migration())
	registry.Register(migrations.GetV4Migration())
	return &Manager{
		cfg: cfg, backup: backup, registry: registry,
		executor: NewMigrationExecutor(registry, log), clock: clock, log: log,
	}
}

// Prepare backs up an existing database (when migrations are pending) and applies the
// pending migrations on a temporary connection. Storage opens its own connection
// afterwards.
func (m *Manager) Prepare(ctx context.Context) (err error) {
	m.log.Debug().Str("path", m.cfg.Path).Msg("database prepare starting")
	_, statErr := os.Stat(m.cfg.Path)
	exists := statErr == nil
	db, err := openMigrationDB(ctx, m.cfg.Path)
	if err != nil {
		return err
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			err = errors.Join(err, fmt.Errorf("database: close migration connection: %w", cErr))
		}
	}()

	current, err := m.executor.GetCurrentVersion(db)
	if err != nil {
		return fmt.Errorf("database: current schema version: %w", err)
	}
	if current >= CurrentSchemaVersion {
		m.log.Info().Int("version", current).Msg("database schema is up to date")
		return nil
	}
	m.log.Debug().Int("current_version", current).Msg("database migrations pending")
	if exists {
		m.backupBeforeMigration()
	}
	if err := m.migrate(db, current); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(FULL);"); err != nil {
		m.log.Warn().Err(err).Msg("WAL checkpoint after migrations failed")
	}
	m.cleanupBackups()
	m.log.Debug().Msg("database prepare completed")
	return nil
}

func openMigrationDB(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("database: ping: %w", err), db.Close())
	}
	return db, nil
}

func (m *Manager) migrate(db *sql.DB, current int) error {
	if current == 0 {
		m.log.Info().Msg("legacy database detected, marking as V1")
		if err := m.executor.MarkLegacyAsV1(db); err != nil {
			return fmt.Errorf("database: mark legacy database as V1: %w", err)
		}
		current = 1
	}
	n, err := m.executor.ApplyPendingMigrations(db, current)
	if err != nil {
		return fmt.Errorf("database: migration failed: %w", err)
	}
	m.log.Info().Int("applied", n).Msg("migrations applied")
	return nil
}

func (m *Manager) backupBeforeMigration() {
	path, err := CreateBackup(m.cfg.Path, m.backup, m.clock.Now(), m.log)
	if err != nil {
		m.log.Error().Err(err).Msg("pre-migration backup failed, proceeding without backup")
		return
	}
	m.log.Info().Str("file", path).Msg("pre-migration backup created")
}

func (m *Manager) cleanupBackups() {
	if !m.backup.Enabled || m.backup.MaxBackups <= 0 {
		return
	}
	if err := CleanupBackups(m.cfg.Path, m.backup.MaxBackups, m.log); err != nil {
		m.log.Warn().Err(err).Msg("backup cleanup failed")
	}
}

// RunPeriodicBackups creates online backups every BackupInterval until ctx is done.
func (m *Manager) RunPeriodicBackups(ctx context.Context) {
	if !m.backup.Enabled || m.backup.BackupInterval <= 0 {
		m.log.Info().Msg("periodic backups disabled")
		return
	}
	m.every(ctx, m.backup.BackupInterval, false, func() {
		if path, err := CreateBackup(m.cfg.Path, m.backup, m.clock.Now(), m.log); err != nil {
			m.log.Error().Err(err).Msg("online backup failed")
		} else {
			m.log.Info().Str("file", path).Msg("online backup created")
		}
		m.cleanupBackups()
	})
}

// RunPeriodicCleanup runs retention cleanup now and every CleanupInterval until ctx is
// done.
func (m *Manager) RunPeriodicCleanup(ctx context.Context, c Cleaner) {
	if m.cfg.CleanupInterval <= 0 {
		m.log.Info().Msg("periodic retention cleanup disabled")
		return
	}
	m.every(ctx, m.cfg.CleanupInterval, true, func() {
		if err := c.CleanupAll(ctx); err != nil {
			m.log.Error().Err(err).Msg("retention cleanup failed")
		}
	})
}

// every runs fn on each tick (and once immediately when now is true) until ctx is done.
func (m *Manager) every(ctx context.Context, d time.Duration, now bool, fn func()) {
	if now {
		fn()
	}
	t := m.clock.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C():
			fn()
		}
	}
}
