// Package storage is the only package that touches SQLite (single connection, WAL).
// It exposes narrow per-consumer interfaces (PollerStore, AggregatorStore, ReadStore),
// enforces the immutability of closed periods and the per-writer key domains, and runs
// period sums as SQL with explicit bounds.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	_ "modernc.org/sqlite" // SQLite driver

	"github.com/dombyte/solis/internal/database/migrations"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/utils"
)

const dirPerm = 0o750

// Settings are the connection, pragma and retention settings of the storage.
type Settings struct {
	// Path is the SQLite database file.
	Path string
	// DailyRetention is the retention of daily rows (monthly/yearly rows follow it).
	DailyRetention time.Duration
	// ErrorRetention is the retention of status change rows.
	ErrorRetention time.Duration
	// WalMode enables write-ahead logging.
	WalMode bool
	// Synchronous is the SQLite synchronous mode (OFF, NORMAL, FULL, EXTRA; "" = default).
	Synchronous string
	// TempStore is the SQLite temp_store mode (DEFAULT, FILE, MEMORY; "" = default).
	TempStore string
}

// Storage is the SQLite storage backend.
type Storage struct {
	db    *sql.DB
	cfg   Settings
	keys  KeyLookup
	clock utils.Clock
	log   zerolog.Logger

	// mu serializes writes and guards meta (the in-memory copy of the meta table).
	mu   sync.Mutex
	meta metaState

	cleanupMu  sync.Mutex
	lastVacuum time.Time
}

// New opens (and if needed creates) the database and loads the close state.
func New(cfg Settings, keys KeyLookup, clock utils.Clock,
	log zerolog.Logger,
) (*Storage, error) {
	db, err := open(cfg, log)
	if err != nil {
		return nil, err
	}
	s := &Storage{db: db, cfg: cfg, keys: keys, clock: clock, log: log}
	if err := s.initSchema(); err != nil {
		return nil, errors.Join(fmt.Errorf("storage: init schema: %w", err), db.Close())
	}
	if err := s.loadMeta(context.Background()); err != nil {
		return nil, errors.Join(fmt.Errorf("storage: load meta: %w", err), db.Close())
	}
	return s, nil
}

func open(cfg Settings, log zerolog.Logger) (*sql.DB, error) {
	if dir := filepath.Dir(cfg.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return nil, fmt.Errorf("storage: create directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("storage: open database: %w", err)
	}
	// SQLite with WAL works best with exactly one connection (no "database is locked").
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	configurePragmas(db, cfg, log)
	if err := db.Ping(); err != nil {
		return nil, errors.Join(fmt.Errorf("storage: ping database: %w", err), db.Close())
	}
	return db, nil
}

func configurePragmas(db *sql.DB, cfg Settings, log zerolog.Logger) {
	var pragmas []string
	if cfg.WalMode {
		pragmas = append(pragmas, "PRAGMA journal_mode=WAL;")
	}
	// Synchronous and TempStore are validated enums (validated by config).
	if cfg.Synchronous != "" {
		pragmas = append(pragmas, "PRAGMA synchronous="+cfg.Synchronous+";")
	}
	if cfg.TempStore != "" {
		pragmas = append(pragmas, "PRAGMA temp_store="+cfg.TempStore+";")
	}
	for _, p := range pragmas {
		log.Debug().Str("pragma", p).Msg("applying pragma")
		if _, err := db.Exec(p); err != nil {
			log.Warn().Err(err).Str("pragma", p).Msg("failed to apply pragma")
		}
	}
}

// schemaSQL mirrors the V1 migration plus the V3 meta table; IF NOT EXISTS keeps it a
// no-op on migrated databases.
func schemaSQL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS daily_values (id INTEGER PRIMARY KEY AUTOINCREMENT,
			date DATE NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL,
			raw_value REAL NOT NULL, UNIQUE(register_key, date))`,
		`CREATE INDEX IF NOT EXISTS idx_daily_key_date ON daily_values(register_key, date)`,
		`CREATE INDEX IF NOT EXISTS idx_daily_date ON daily_values(date)`,
		`CREATE TABLE IF NOT EXISTS monthly_values (id INTEGER PRIMARY KEY AUTOINCREMENT,
			month TEXT NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL,
			raw_value REAL NOT NULL, UNIQUE(register_key, month))`,
		`CREATE INDEX IF NOT EXISTS idx_monthly_key_month ON monthly_values(register_key, month)`,
		`CREATE INDEX IF NOT EXISTS idx_monthly_month ON monthly_values(month)`,
		`CREATE TABLE IF NOT EXISTS yearly_values (id INTEGER PRIMARY KEY AUTOINCREMENT,
			year TEXT NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL,
			raw_value REAL NOT NULL, UNIQUE(register_key, year))`,
		`CREATE INDEX IF NOT EXISTS idx_yearly_key_year ON yearly_values(register_key, year)`,
		`CREATE INDEX IF NOT EXISTS idx_yearly_year ON yearly_values(year)`,
		`CREATE TABLE IF NOT EXISTS total_values (id INTEGER PRIMARY KEY AUTOINCREMENT,
			register_key TEXT NOT NULL UNIQUE, value REAL NOT NULL, raw_value REAL NOT NULL,
			timestamp DATETIME NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_total_key ON total_values(register_key)`,
		`CREATE TABLE IF NOT EXISTS error_data (id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL, register_key TEXT NOT NULL, raw_value REAL NOT NULL,
			string_value TEXT, UNIQUE(register_key, timestamp))`,
		`CREATE INDEX IF NOT EXISTS idx_error_key_timestamp ON error_data(register_key, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_error_timestamp ON error_data(timestamp)`,
		migrations.V3MetaTableSQL,
	}
}

func (s *Storage) initSchema() error {
	return s.withTx(context.Background(), func(tx *sql.Tx) error {
		for _, stmt := range schemaSQL() {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

// withTx runs fn in a transaction, committing on success.
func (s *Storage) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, fmt.Errorf("storage: rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit: %w", err)
	}
	return nil
}

// Ping checks the connection (health probe).
func (s *Storage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection.
func (s *Storage) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("storage: close: %w", err)
	}
	return nil
}

// lookup resolves a register or returns ErrUnknownKey.
func (s *Storage) lookup(key string) (solis.Register, error) {
	reg, ok := s.keys.ByKey(key)
	if !ok {
		return solis.Register{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	return reg, nil
}

// rawOf converts a decoded value back to the register's raw unit.
func rawOf(reg solis.Register, value float64) float64 {
	return value / reg.Scale
}
