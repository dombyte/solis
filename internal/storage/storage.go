// Package storage provides SQLite-based data persistence for the Solis monitor application.
// It handles raw data storage, aggregation, and retention cleanup.
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/utils"
	_ "modernc.org/sqlite"
)

// logger is the package-level logger for storage operations.
var logger = logging.NewComponentLogger("storage")

// RegisterValue represents a decoded register value ready for storage.
type RegisterValue struct {
	// Key is the register key (e.g., "pv_voltage_1", "status").
	Key string
	// RawValue is the raw numeric value before scaling.
	RawValue float64
	// StringValue holds the decoded string for String-type registers.
	StringValue string
	// Timestamp is when the value was read.
	Timestamp time.Time
}

// Storage is the SQLite storage backend for Solis monitor data.
type Storage struct {
	// db is the SQLite database connection.
	db *sql.DB
	// config holds the storage configuration.
	config *config.StorageSettings
	// path is the path to the SQLite database file.
	path string
	// lastVacuumTime tracks when the last VACUUM was performed.
	lastVacuumTime time.Time
	// lastAggregatedCleanupTime tracks when the last aggregated data cleanup was performed.
	lastAggregatedCleanupTime time.Time
}

// DB returns the underlying SQLite database connection.
func (s *Storage) DB() *sql.DB {
	return s.db
}

// New creates a new Storage instance and initializes the database.
func New(cfg *config.StorageSettings) (*Storage, error) {
	logger.Info().Msgf("Initializing storage at %s", cfg.Path)

	// Create parent directories if they don't exist
	dir := filepath.Dir(cfg.Path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Error().Msgf("Failed to create storage directory: %v", err)
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Build DSN with connection parameters
	dsn := cfg.Path

	// Open database connection
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		logger.Error().Msgf("Failed to open database: %v", err)
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool settings
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Enable WAL mode and other SQLite settings via PRAGMA
	if cfg.WalMode {
		if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
			logger.Warn().Msgf("Failed to set WAL mode: %v", err)
		}
	}

	// Set synchronous mode
	if cfg.Synchronous != "" {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA synchronous=%s;", cfg.Synchronous)); err != nil {
			logger.Warn().Msgf("Failed to set synchronous mode: %v", err)
		}
	}

	// Set temp store
	if cfg.TempStore != "" {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA temp_store=%s;", cfg.TempStore)); err != nil {
			logger.Warn().Msgf("Failed to set temp store: %v", err)
		}
	}

	// Verify the connection
	if err := db.Ping(); err != nil {
		logger.Error().Msgf("Failed to ping database: %v", err)
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	st := &Storage{
		db:                        db,
		config:                    cfg,
		path:                      cfg.Path,
		lastAggregatedCleanupTime: time.Now(),
	}

	// Initialize schema
	if err := st.initSchema(); err != nil {
		logger.Error().Msgf("Failed to initialize schema: %v", err)
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	logger.Info().Msg("Storage initialized successfully")
	return st, nil
}

// Close closes the database connection.
func (s *Storage) Close() error {
	logger.Info().Msg("Closing storage connection")
	if err := s.db.Close(); err != nil {
		logger.Error().Msgf("Error closing database: %v", err)
		return err
	}
	return nil
}

// initSchema creates the database tables if they don't exist.
func (s *Storage) initSchema() error {
	logger.Info().Msg("Initializing database schema")

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create daily_values table for daily energy totals
	// Stores one value per day per key, updated with the maximum value seen during the day
	dailySQL := `
		CREATE TABLE IF NOT EXISTS daily_values (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date DATE NOT NULL,
			register_key TEXT NOT NULL,
			value REAL NOT NULL,
			raw_value REAL NOT NULL,
			UNIQUE(register_key, date)
		);
		CREATE INDEX IF NOT EXISTS idx_daily_key_date ON daily_values(register_key, date);
		CREATE INDEX IF NOT EXISTS idx_daily_date ON daily_values(date);
	`

	if _, err := tx.Exec(dailySQL); err != nil {
		return fmt.Errorf("failed to create daily_values table: %w", err)
	}

	// Create monthly_values table for monthly energy totals
	// Stores one value per month per key, updated with the maximum value seen during the month
	monthlySQL := `
		CREATE TABLE IF NOT EXISTS monthly_values (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			month TEXT NOT NULL,
			register_key TEXT NOT NULL,
			value REAL NOT NULL,
			raw_value REAL NOT NULL,
			UNIQUE(register_key, month)
		);
		CREATE INDEX IF NOT EXISTS idx_monthly_key_month ON monthly_values(register_key, month);
		CREATE INDEX IF NOT EXISTS idx_monthly_month ON monthly_values(month);
	`

	if _, err := tx.Exec(monthlySQL); err != nil {
		return fmt.Errorf("failed to create monthly_values table: %w", err)
	}

	// Create yearly_values table for yearly energy totals
	// Stores one value per year per key, updated with the maximum value seen during the year
	yearlySQL := `
		CREATE TABLE IF NOT EXISTS yearly_values (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			year TEXT NOT NULL,
			register_key TEXT NOT NULL,
			value REAL NOT NULL,
			raw_value REAL NOT NULL,
			UNIQUE(register_key, year)
		);
		CREATE INDEX IF NOT EXISTS idx_yearly_key_year ON yearly_values(register_key, year);
		CREATE INDEX IF NOT EXISTS idx_yearly_year ON yearly_values(year);
	`

	if _, err := tx.Exec(yearlySQL); err != nil {
		return fmt.Errorf("failed to create yearly_values table: %w", err)
	}

	// Create total_values table for total (lifetime) energy values
	// Stores the latest value for each total register
	totalSQL := `
		CREATE TABLE IF NOT EXISTS total_values (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			register_key TEXT NOT NULL UNIQUE,
			value REAL NOT NULL,
			raw_value REAL NOT NULL,
			timestamp DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_total_key ON total_values(register_key);
	`

	if _, err := tx.Exec(totalSQL); err != nil {
		return fmt.Errorf("failed to create total_values table: %w", err)
	}

	// Create error_data table for error/fault register values
	// Only stores values when they change (same optimization as raw_data)
	errorSQL := `
		CREATE TABLE IF NOT EXISTS error_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			register_key TEXT NOT NULL,
			raw_value REAL NOT NULL,
			string_value TEXT,
			UNIQUE(register_key, timestamp)
		);
		CREATE INDEX IF NOT EXISTS idx_error_key_timestamp ON error_data(register_key, timestamp);
		CREATE INDEX IF NOT EXISTS idx_error_timestamp ON error_data(timestamp);
	`

	if _, err := tx.Exec(errorSQL); err != nil {
		return fmt.Errorf("failed to create error_data table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schema transaction: %w", err)
	}

	logger.Info().Msg("Database schema initialized")
	return nil
}

// getLastErrorValue retrieves the last stored error value for a register.
func (s *Storage) getLastErrorValue(tx *sql.Tx, key string) (*float64, error) {
	var lastValue float64
	query := `SELECT raw_value FROM error_data WHERE register_key = ? ORDER BY timestamp DESC LIMIT 1`
	err := tx.QueryRow(query, key).Scan(&lastValue)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query last error value: %w", err)
	}
	return &lastValue, nil
}

// storeDailyValue updates the daily value for a register.
// Creates a new entry for the current date if none exists, or updates the existing one
// with the maximum value seen so far that day.
func (s *Storage) storeDailyValue(tx *sql.Tx, key string, value *solis.Value, timestamp time.Time) error {
	date := timestamp.Format("2006-01-02")
	reg, ok := solis.RegisterMapByKey[key]
	if !ok {
		return nil
	}

	// Get existing value for today
	var existingValue float64
	err := tx.QueryRow(`
		SELECT value FROM daily_values 
		WHERE register_key = ? AND date = ?
	`, key, date).Scan(&existingValue)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to query daily value: %w", err)
	}

	decodedValue := value.RawValue * reg.Scale

	// For energy registers, we want the MAXIMUM value seen during the day
	// (they reset at midnight, so the highest value is the end-of-day total)
	if err == sql.ErrNoRows {
		// New day, insert new record
		_, err = tx.Exec(`
			INSERT INTO daily_values (date, register_key, value, raw_value)
			VALUES (?, ?, ?, ?)
		`, date, key, decodedValue, value.RawValue)
	} else {
		// Update existing record if new value is higher
		if decodedValue > existingValue {
			_, err = tx.Exec(`
				UPDATE daily_values 
				SET value = ?, raw_value = ?
				WHERE register_key = ? AND date = ?
			`, decodedValue, value.RawValue, key, date)
		}
	}

	return err
}

// storeMonthlyValue updates the monthly value for a register.
// Creates a new entry for the current month if none exists, or updates the existing one
// with the maximum value seen so far that month.
func (s *Storage) storeMonthlyValue(tx *sql.Tx, key string, value *solis.Value, timestamp time.Time) error {
	month := timestamp.Format("2006-01")
	reg, ok := solis.RegisterMapByKey[key]
	if !ok {
		return nil
	}

	// Get existing value for this month
	var existingValue float64
	err := tx.QueryRow(`
		SELECT value FROM monthly_values 
		WHERE register_key = ? AND month = ?
	`, key, month).Scan(&existingValue)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to query monthly value: %w", err)
	}

	decodedValue := value.RawValue * reg.Scale

	// For energy registers, we want the MAXIMUM value seen during the month
	// (they reset at the start of a new month, so the highest value is the end-of-month total)
	if err == sql.ErrNoRows {
		// New month, insert new record
		_, err = tx.Exec(`
			INSERT INTO monthly_values (month, register_key, value, raw_value)
			VALUES (?, ?, ?, ?)
		`, month, key, decodedValue, value.RawValue)
	} else {
		// Update existing record if new value is higher
		if decodedValue > existingValue {
			_, err = tx.Exec(`
				UPDATE monthly_values 
				SET value = ?, raw_value = ?
				WHERE register_key = ? AND month = ?
			`, decodedValue, value.RawValue, key, month)
		}
	}

	return err
}

// storeYearlyValue updates the yearly value for a register.
// Creates a new entry for the current year if none exists, or updates the existing one
// with the maximum value seen so far that year.
func (s *Storage) storeYearlyValue(tx *sql.Tx, key string, value *solis.Value, timestamp time.Time) error {
	year := timestamp.Format("2006")
	reg, ok := solis.RegisterMapByKey[key]
	if !ok {
		return nil
	}

	// Get existing value for this year
	var existingValue float64
	err := tx.QueryRow(`
		SELECT value FROM yearly_values 
		WHERE register_key = ? AND year = ?
	`, key, year).Scan(&existingValue)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to query yearly value: %w", err)
	}

	decodedValue := value.RawValue * reg.Scale

	// For energy registers, we want the MAXIMUM value seen during the year
	// (they reset at the start of a new year, so the highest value is the end-of-year total)
	if err == sql.ErrNoRows {
		// New year, insert new record
		_, err = tx.Exec(`
			INSERT INTO yearly_values (year, register_key, value, raw_value)
			VALUES (?, ?, ?, ?)
		`, year, key, decodedValue, value.RawValue)
	} else {
		// Update existing record if new value is higher
		if decodedValue > existingValue {
			_, err = tx.Exec(`
				UPDATE yearly_values 
				SET value = ?, raw_value = ?
				WHERE register_key = ? AND year = ?
			`, decodedValue, value.RawValue, key, year)
		}
	}

	return err
}

// storeTotalValue updates the total (lifetime) value for a register.
// Always updates with the latest value since total registers only increase.
func (s *Storage) storeTotalValue(tx *sql.Tx, key string, value *solis.Value, timestamp time.Time) error {
	reg, ok := solis.RegisterMapByKey[key]
	if !ok {
		return nil
	}

	decodedValue := value.RawValue * reg.Scale
	timestampStr := timestamp.Format(time.RFC3339)

	// Get existing value
	var existingValue float64
	err := tx.QueryRow(`
		SELECT value FROM total_values 
		WHERE register_key = ?
	`, key).Scan(&existingValue)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to query total value: %w", err)
	}

	// For total registers, we want the MAXIMUM value seen (they only increase)
	if err == sql.ErrNoRows {
		// No existing value, insert new record
		_, err = tx.Exec(`
			INSERT INTO total_values (register_key, value, raw_value, timestamp)
			VALUES (?, ?, ?, ?)
		`, key, decodedValue, value.RawValue, timestampStr)
	} else {
		// Update existing record if new value is higher
		if decodedValue > existingValue {
			_, err = tx.Exec(`
				UPDATE total_values 
				SET value = ?, raw_value = ?, timestamp = ?
				WHERE register_key = ?
			`, decodedValue, value.RawValue, timestampStr, key)
		}
	}

	return err
}

// StoreAllRegisters stores all register values.
// Stable registers are skipped (only cached, not stored in DB).
// Status registers go to error_data table.
// Daily registers go to daily_values table.
// Monthly registers go to monthly_values table.
// Yearly registers go to yearly_values table.
// Total registers go to total_values table.
func (s *Storage) StoreAllRegisters(values map[string]*solis.Value, timestamp time.Time) error {
	if len(values) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for key, value := range values {
		// Look up the register definition
		reg, ok := solis.RegisterMapByKey[key]
		if !ok {
			logger.Warn().Msgf("Unknown register key: %s", key)
			continue
		}

		// Stable registers: only cache, no DB storage
		if reg.Stability == solis.Stable {
			logger.Debug().Msgf("Skipping stable register %s (cache only)", key)
			continue
		}

		// Status registers: store in error_data table
		if reg.Status {
			// Only store if value has changed
			lastValue, err := s.getLastErrorValue(tx, key)
			if err != nil {
				logger.Warn().Msgf("Error getting last error value for %s: %v", key, err)
				// Continue anyway, we'll store the new value
			} else if lastValue != nil && *lastValue == value.RawValue {
				logger.Debug().Msgf("Error value for %s unchanged (%.2f), skipping", key, value.RawValue)
				continue
			}

			if _, err := tx.Exec(`
				INSERT INTO error_data (timestamp, register_key, raw_value, string_value)
				VALUES (?, ?, ?, ?)
			`, timestamp, key, value.RawValue, value.StringValue); err != nil {
				logger.Error().Msgf("Failed to insert error data for %s: %v", key, err)
				return fmt.Errorf("failed to insert error data: %w", err)
			}
			logger.Debug().Msgf("Stored error data for %s", key)
			continue
		}

		// Daily registers: update daily_values table
		if solis.IsDailyRegister(key) {
			if err := s.storeDailyValue(tx, key, value, timestamp); err != nil {
				logger.Error().Msgf("Failed to store daily value for %s: %v", key, err)
				return err
			}
			logger.Debug().Msgf("Stored/updated daily value for %s", key)
			// Daily registers are NOT stored in raw_data (per user requirement: no backward compatibility)
			continue
		}

		// Monthly registers: update monthly_values table
		if solis.IsMonthlyRegister(key) {
			if err := s.storeMonthlyValue(tx, key, value, timestamp); err != nil {
				logger.Error().Msgf("Failed to store monthly value for %s: %v", key, err)
				return err
			}
			logger.Debug().Msgf("Stored/updated monthly value for %s", key)
			// Monthly registers are NOT stored in raw_data
			continue
		}

		// Yearly registers: update yearly_values table
		if solis.IsYearlyRegister(key) {
			if err := s.storeYearlyValue(tx, key, value, timestamp); err != nil {
				logger.Error().Msgf("Failed to store yearly value for %s: %v", key, err)
				return err
			}
			logger.Debug().Msgf("Stored/updated yearly value for %s", key)
			// Yearly registers are NOT stored in raw_data
			continue
		}

		// Total registers: update total_values table
		if solis.IsTotalRegister(key) {
			if err := s.storeTotalValue(tx, key, value, timestamp); err != nil {
				logger.Error().Msgf("Failed to store total value for %s: %v", key, err)
				return err
			}
			logger.Debug().Msgf("Stored/updated total value for %s", key)
			continue
		}

		// Dynamic registers (non-status, non-daily, non-monthly, non-yearly, non-total):
		// No longer stored in database - only in cache
		continue
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Debug().Msgf("Stored register values successfully")
	return nil
}

// CleanupDailyData removes daily data older than the configured retention period.
func (s *Storage) CleanupDailyData() error {
	logger.Info().Msgf("Cleaning up daily data older than %s", s.config.DailyRetention)

	cutoff := time.Now().Add(-s.config.DailyRetention)
	cutoffDate := cutoff.Format("2006-01-02")

	result, err := s.db.Exec(`
		DELETE FROM daily_values 
		WHERE date < ?
	`, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to cleanup daily data: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Warn().Msgf("Error getting rows affected: %v", err)
	} else {
		logger.Info().Msgf("Deleted %d old daily data rows", rowsAffected)
	}

	// Run VACUUM every 72 hours to reclaim space from deleted rows
	if s.lastVacuumTime.IsZero() || time.Since(s.lastVacuumTime) > 72*time.Hour {
		if _, err := s.db.Exec("VACUUM;"); err != nil {
			logger.Warn().Msgf("VACUUM failed: %v", err)
		} else {
			s.lastVacuumTime = time.Now()
			logger.Info().Msg("Database VACUUM completed")
		}
	}

	return nil
}

// CleanupMonthlyData removes monthly data older than the configured retention period.
func (s *Storage) CleanupMonthlyData() error {
	logger.Info().Msgf("Cleaning up monthly data older than %s", s.config.MonthlyRetention)

	cutoff := time.Now().Add(-s.config.MonthlyRetention)
	cutoffMonth := cutoff.Format("2006-01")

	result, err := s.db.Exec(`
		DELETE FROM monthly_values 
		WHERE month < ?
	`, cutoffMonth)
	if err != nil {
		return fmt.Errorf("failed to cleanup monthly data: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Warn().Msgf("Error getting rows affected: %v", err)
	} else {
		logger.Info().Msgf("Deleted %d old monthly data rows", rowsAffected)
	}

	return nil
}

// CleanupYearlyData removes yearly data older than the configured retention period.
func (s *Storage) CleanupYearlyData() error {
	logger.Info().Msgf("Cleaning up yearly data older than %s", s.config.YearlyRetention)

	cutoff := time.Now().Add(-s.config.YearlyRetention)
	cutoffYear := cutoff.Format("2006")

	result, err := s.db.Exec(`
		DELETE FROM yearly_values 
		WHERE year < ?
	`, cutoffYear)
	if err != nil {
		return fmt.Errorf("failed to cleanup yearly data: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Warn().Msgf("Error getting rows affected: %v", err)
	} else {
		logger.Info().Msgf("Deleted %d old yearly data rows", rowsAffected)
	}

	return nil
}

// CleanupErrorData removes error data older than the configured retention period.
func (s *Storage) CleanupErrorData() error {
	logger.Info().Msgf("Cleaning up error data older than %s", s.config.ErrorRetention)

	cutoff := time.Now().Add(-s.config.ErrorRetention)

	result, err := s.db.Exec(`
		DELETE FROM error_data 
		WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup error data: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Warn().Msgf("Error getting rows affected: %v", err)
	} else {
		logger.Info().Msgf("Deleted %d old error data rows", rowsAffected)
	}

	// Run VACUUM every 72 hours to reclaim space from deleted rows
	if s.lastVacuumTime.IsZero() || time.Since(s.lastVacuumTime) > 72*time.Hour {
		if _, err := s.db.Exec("VACUUM;"); err != nil {
			logger.Warn().Msgf("VACUUM failed: %v", err)
		} else {
			s.lastVacuumTime = time.Now()
			logger.Info().Msg("Database VACUUM completed")
		}
	}

	return nil
}

// Interval represents the aggregation interval for historical data.
// Only IntervalRaw is supported now. Aggregated intervals have been removed to simplify the storage layer.
type Interval string

const (
	// IntervalRaw represents raw data points (no aggregation).
	IntervalRaw Interval = "raw"
)

// HistoryDataPoint represents a single historical data point.
// For raw interval:
//   - Timestamp is the exact time when the value was recorded
//   - Min, Max, Count fields are omitted (nil)
type HistoryDataPoint struct {
	Timestamp string   `json:"timestamp"`
	Value     float64  `json:"value"`
	Min       *float64 `json:"min,omitempty"`   // Minimum value in aggregation window (for aggregated intervals)
	Max       *float64 `json:"max,omitempty"`   // Maximum value in aggregation window (for aggregated intervals)
	Count     *int     `json:"count,omitempty"` // Number of data points in aggregation window (for aggregated intervals)
}

// HistoryResult represents historical data for a register.
type HistoryResult struct {
	Key      string             `json:"key"`
	Unit     string             `json:"unit"`
	Interval Interval           `json:"interval"`
	Data     []HistoryDataPoint `json:"data"`
}

// ErrorDataPoint represents a single error/fault data point.
type ErrorDataPoint struct {
	Timestamp   string  `json:"timestamp"`
	RawValue    float64 `json:"raw_value"`
	StringValue string  `json:"string_value,omitempty"`
}

// DailyDataPoint represents a daily aggregated value for energy registers.
type DailyDataPoint struct {
	Date     string  `json:"date"`      // YYYY-MM-DD format
	Value    float64 `json:"value"`     // Decoded value (scaled)
	RawValue float64 `json:"raw_value"` // Raw value before scaling
}

// MonthlyDataPoint represents a monthly aggregated value for energy registers.
type MonthlyDataPoint struct {
	Month    string  `json:"month"`     // YYYY-MM format
	Value    float64 `json:"value"`     // Decoded value (scaled)
	RawValue float64 `json:"raw_value"` // Raw value before scaling
}

// YearlyDataPoint represents a yearly aggregated value for energy registers.
type YearlyDataPoint struct {
	Year     string  `json:"year"`      // YYYY format
	Value    float64 `json:"value"`     // Decoded value (scaled)
	RawValue float64 `json:"raw_value"` // Raw value before scaling
}

// TotalDataPoint represents a total (lifetime) value for energy registers.
type TotalDataPoint struct {
	Value     float64 `json:"value"`     // Decoded value (scaled)
	RawValue  float64 `json:"raw_value"` // Raw value before scaling
	Timestamp string  `json:"timestamp"` // When it was last updated (RFC3339 format)
}

// MarshalJSON implements json.Marshaler for HistoryDataPoint to ensure float64 values are rounded.
func (h HistoryDataPoint) MarshalJSON() ([]byte, error) {
	type Alias HistoryDataPoint
	aux := struct {
		Value float64 `json:"value"`
		Min   *float64 `json:"min,omitempty"`
		Max   *float64 `json:"max,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(&h),
	}
	if h.Value != 0 {
		rounded := utils.RoundTo2DecimalPlaces(h.Value)
		aux.Value = rounded
	}
	if h.Min != nil {
		rounded := utils.RoundTo2DecimalPlaces(*h.Min)
		aux.Min = &rounded
	}
	if h.Max != nil {
		rounded := utils.RoundTo2DecimalPlaces(*h.Max)
		aux.Max = &rounded
	}
	return json.Marshal(aux)
}

// MarshalJSON implements json.Marshaler for DailyDataPoint to ensure float64 values are rounded.
func (d DailyDataPoint) MarshalJSON() ([]byte, error) {
	type Alias DailyDataPoint
	return json.Marshal(struct {
		Date     string  `json:"date"`
		Value    float64 `json:"value"`
		RawValue float64 `json:"raw_value"`
		*Alias
	}{
		Alias:    (*Alias)(&d),
		Date:     d.Date,
		Value:    utils.RoundTo2DecimalPlaces(d.Value),
		RawValue: utils.RoundTo2DecimalPlaces(d.RawValue),
	})
}

// MarshalJSON implements json.Marshaler for MonthlyDataPoint to ensure float64 values are rounded.
func (m MonthlyDataPoint) MarshalJSON() ([]byte, error) {
	type Alias MonthlyDataPoint
	return json.Marshal(struct {
		Month    string  `json:"month"`
		Value    float64 `json:"value"`
		RawValue float64 `json:"raw_value"`
		*Alias
	}{
		Alias:    (*Alias)(&m),
		Month:    m.Month,
		Value:    utils.RoundTo2DecimalPlaces(m.Value),
		RawValue: utils.RoundTo2DecimalPlaces(m.RawValue),
	})
}

// MarshalJSON implements json.Marshaler for YearlyDataPoint to ensure float64 values are rounded.
func (y YearlyDataPoint) MarshalJSON() ([]byte, error) {
	type Alias YearlyDataPoint
	return json.Marshal(struct {
		Year     string  `json:"year"`
		Value    float64 `json:"value"`
		RawValue float64 `json:"raw_value"`
		*Alias
	}{
		Alias:    (*Alias)(&y),
		Year:     y.Year,
		Value:    utils.RoundTo2DecimalPlaces(y.Value),
		RawValue: utils.RoundTo2DecimalPlaces(y.RawValue),
	})
}

// MarshalJSON implements json.Marshaler for TotalDataPoint to ensure float64 values are rounded.
func (t TotalDataPoint) MarshalJSON() ([]byte, error) {
	type Alias TotalDataPoint
	return json.Marshal(struct {
		Value     float64 `json:"value"`
		RawValue  float64 `json:"raw_value"`
		Timestamp string  `json:"timestamp"`
		*Alias
	}{
		Alias:     (*Alias)(&t),
		Value:     utils.RoundTo2DecimalPlaces(t.Value),
		RawValue:  utils.RoundTo2DecimalPlaces(t.RawValue),
		Timestamp: t.Timestamp,
	})
}

// MarshalJSON implements json.Marshaler for ErrorDataPoint to ensure float64 values are rounded.
func (e ErrorDataPoint) MarshalJSON() ([]byte, error) {
	type Alias ErrorDataPoint
	return json.Marshal(struct {
		Timestamp   string  `json:"timestamp"`
		RawValue    float64 `json:"raw_value"`
		StringValue string  `json:"string_value,omitempty"`
		*Alias
	}{
		Alias:      (*Alias)(&e),
		Timestamp:  e.Timestamp,
		RawValue:   utils.RoundTo2DecimalPlaces(e.RawValue),
		StringValue: e.StringValue,
	})
}

// internalDataPoint is used internally for aggregation with time.Time timestamp.
// Kept for potential future use, though aggregation is currently removed.
type internalDataPoint struct {
	timestamp time.Time
	value     float64
	min       float64
	max       float64
	count     int
}

// GetErrorHistory retrieves historical error data for a specific register key.
func (s *Storage) GetErrorHistory(key string, start, end time.Time) ([]*ErrorDataPoint, error) {
	rows, err := s.db.Query(`
		SELECT timestamp, raw_value, string_value
		FROM error_data
		WHERE register_key = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp
		LIMIT 50000
	`, key, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query error history: %w", err)
	}
	defer rows.Close()

	result := make([]*ErrorDataPoint, 0)
	for rows.Next() {
		var dp ErrorDataPoint
		if err := rows.Scan(&dp.Timestamp, &dp.RawValue, &dp.StringValue); err != nil {
			return nil, fmt.Errorf("failed to scan error history: %w", err)
		}
		result = append(result, &dp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating error history: %w", err)
	}

	return result, nil
}

// GetDailyHistory retrieves daily values for a specific register key.
func (s *Storage) GetDailyHistory(key string, startDate, endDate time.Time) ([]*DailyDataPoint, error) {
	start := startDate.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	rows, err := s.db.Query(`
		SELECT date, value, raw_value
		FROM daily_values
		WHERE register_key = ? AND date >= ? AND date <= ?
		ORDER BY date
		LIMIT 10000
	`, key, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily history: %w", err)
	}
	defer rows.Close()

	result := make([]*DailyDataPoint, 0)
	for rows.Next() {
		var dp DailyDataPoint
		if err := rows.Scan(&dp.Date, &dp.Value, &dp.RawValue); err != nil {
			return nil, fmt.Errorf("failed to scan daily history: %w", err)
		}
		result = append(result, &dp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating daily history: %w", err)
	}

	return result, nil
}

// GetMonthlyHistory retrieves monthly values for a specific register key.
func (s *Storage) GetMonthlyHistory(key string, startMonth, endMonth time.Time) ([]*MonthlyDataPoint, error) {
	start := startMonth.Format("2006-01")
	end := endMonth.Format("2006-01")

	rows, err := s.db.Query(`
		SELECT month, value, raw_value
		FROM monthly_values
		WHERE register_key = ? AND month >= ? AND month <= ?
		ORDER BY month
		LIMIT 10000
	`, key, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query monthly history: %w", err)
	}
	defer rows.Close()

	result := make([]*MonthlyDataPoint, 0)
	for rows.Next() {
		var dp MonthlyDataPoint
		if err := rows.Scan(&dp.Month, &dp.Value, &dp.RawValue); err != nil {
			return nil, fmt.Errorf("failed to scan monthly history: %w", err)
		}
		result = append(result, &dp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating monthly history: %w", err)
	}

	return result, nil
}

// GetYearlyHistory retrieves yearly values for a specific register key.
func (s *Storage) GetYearlyHistory(key string, startYear, endYear time.Time) ([]*YearlyDataPoint, error) {
	start := startYear.Format("2006")
	end := endYear.Format("2006")

	rows, err := s.db.Query(`
		SELECT year, value, raw_value
		FROM yearly_values
		WHERE register_key = ? AND year >= ? AND year <= ?
		ORDER BY year
		LIMIT 1000
	`, key, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query yearly history: %w", err)
	}
	defer rows.Close()

	result := make([]*YearlyDataPoint, 0)
	for rows.Next() {
		var dp YearlyDataPoint
		if err := rows.Scan(&dp.Year, &dp.Value, &dp.RawValue); err != nil {
			return nil, fmt.Errorf("failed to scan yearly history: %w", err)
		}
		result = append(result, &dp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating yearly history: %w", err)
	}

	return result, nil
}

// StoreMonthlyDataPoint stores a computed monthly data point in the monthly_values table.
// This is used for backfilling historical computed values.
func (s *Storage) StoreMonthlyDataPoint(key string, dp *MonthlyDataPoint) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get register definition to apply scaling
	reg, ok := solis.RegisterMapByKey[key]
	if !ok {
		return fmt.Errorf("register %s not found", key)
	}

	// Calculate decoded value using the register's scale
	decodedValue := dp.RawValue * reg.Scale

	// Get existing value for this month
	var existingValue float64
	err = tx.QueryRow(`
		SELECT value FROM monthly_values 
		WHERE register_key = ? AND month = ?
	`, key, dp.Month).Scan(&existingValue)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to query existing monthly value: %w", err)
	}

	if err == sql.ErrNoRows {
		// New month, insert new record
		_, err = tx.Exec(`
			INSERT INTO monthly_values (month, register_key, value, raw_value)
			VALUES (?, ?, ?, ?)
		`, dp.Month, key, decodedValue, dp.RawValue)
	} else {
		// Update existing record if new value is higher
		if decodedValue > existingValue {
			_, err = tx.Exec(`
				UPDATE monthly_values 
				SET value = ?, raw_value = ?
				WHERE register_key = ? AND month = ?
			`, decodedValue, dp.RawValue, key, dp.Month)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to store monthly data point: %w", err)
	}

	return tx.Commit()
}

// StoreYearlyDataPoint stores a computed yearly data point in the yearly_values table.
// This is used for backfilling historical computed values.
func (s *Storage) StoreYearlyDataPoint(key string, dp *YearlyDataPoint) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get register definition to apply scaling
	reg, ok := solis.RegisterMapByKey[key]
	if !ok {
		return fmt.Errorf("register %s not found", key)
	}

	// Calculate decoded value using the register's scale
	decodedValue := dp.RawValue * reg.Scale

	// Get existing value for this year
	var existingValue float64
	err = tx.QueryRow(`
		SELECT value FROM yearly_values 
		WHERE register_key = ? AND year = ?
	`, key, dp.Year).Scan(&existingValue)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to query existing yearly value: %w", err)
	}

	if err == sql.ErrNoRows {
		// New year, insert new record
		_, err = tx.Exec(`
			INSERT INTO yearly_values (year, register_key, value, raw_value)
			VALUES (?, ?, ?, ?)
		`, dp.Year, key, decodedValue, dp.RawValue)
	} else {
		// Update existing record if new value is higher
		if decodedValue > existingValue {
			_, err = tx.Exec(`
				UPDATE yearly_values 
				SET value = ?, raw_value = ?
				WHERE register_key = ? AND year = ?
			`, decodedValue, dp.RawValue, key, dp.Year)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to store yearly data point: %w", err)
	}

	return tx.Commit()
}

// GetTotalHistory retrieves the total (lifetime) value for a specific register key.
// Returns the latest stored value.
func (s *Storage) GetTotalHistory(key string) (*TotalDataPoint, error) {
	var dp TotalDataPoint
	err := s.db.QueryRow(`
		SELECT value, raw_value, timestamp
		FROM total_values
		WHERE register_key = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`, key).Scan(&dp.Value, &dp.RawValue, &dp.Timestamp)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query total history: %w", err)
	}

	return &dp, nil
}

// GetMonthlySum returns the sum of daily values for a given register key and month.
// This is used for computing monthly energy values from daily accumulations.
func (s *Storage) GetMonthlySum(key string, month string) (float64, float64, error) {
	// Query to sum all daily values for this register and month
	// month format is "2006-01", so LIKE "2006-01%" matches all dates in that month
	monthPattern := month + "%"
	query := `
		SELECT SUM(value), SUM(raw_value)
		FROM daily_values
		WHERE register_key = ? AND date LIKE ?
	`

	var sumValue, sumRawValue sql.NullFloat64
	err := s.db.QueryRow(query, key, monthPattern).Scan(&sumValue, &sumRawValue)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get monthly sum: %w", err)
	}

	// If no rows, return 0
	if !sumValue.Valid {
		return 0, 0, nil
	}
	if !sumRawValue.Valid {
		return 0, 0, nil
	}

	return sumValue.Float64, sumRawValue.Float64, nil
}

// GetYearlySum returns the sum of daily values for a given register key and year.
// This is used for computing yearly energy values from daily accumulations.
func (s *Storage) GetYearlySum(key string, year string) (float64, float64, error) {
	// Query to sum all daily values for this register and year
	// year format is "2006", so LIKE "2006%" matches all dates in that year
	yearPattern := year + "%"
	query := `
		SELECT SUM(value), SUM(raw_value)
		FROM daily_values
		WHERE register_key = ? AND date LIKE ?
	`

	var sumValue, sumRawValue sql.NullFloat64
	err := s.db.QueryRow(query, key, yearPattern).Scan(&sumValue, &sumRawValue)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get yearly sum: %w", err)
	}

	// If no rows, return 0
	if !sumValue.Valid {
		return 0, 0, nil
	}
	if !sumRawValue.Valid {
		return 0, 0, nil
	}

	return sumValue.Float64, sumRawValue.Float64, nil
}
