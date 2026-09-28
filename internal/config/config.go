// Package config provides configuration loading and validation for the Solis monitor application.
// It uses Viper for YAML configuration with environment variable overrides (prefix SOLIS_).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/utils"
)

// ErrInvalidConfig is wrapped by every validation failure.
var ErrInvalidConfig = errors.New("invalid config")

// removedKeys are settings that v3 ignores; their presence is reported as a warning.
func removedKeys() map[string]string {
	return map[string]string{
		"app.serve_only": "serve-only mode was removed in v3",
		"aggregator": "the aggregator cadence derives from poller.interval; " +
			"use `solis backfill` instead of backfill_current_year_monthly",
		"storage.monthly_retention": "monthly rows follow storage.daily_retention",
		"storage.yearly_retention":  "yearly rows follow storage.daily_retention",
		"storage.enable_migrations": "schema migrations always run on startup",
	}
}

// AppConfig is the root configuration structure.
type AppConfig struct {
	// App contains application-level settings.
	App AppSettings `mapstructure:"app"`
	// Poller contains polling service settings.
	Poller PollerSettings `mapstructure:"poller"`
	// Modbus contains Modbus connection settings.
	Modbus ModbusSettings `mapstructure:"modbus"`
	// Rollover contains the daily rollover settings.
	Rollover RolloverSettings `mapstructure:"rollover"`
	// Storage contains database settings.
	Storage StorageSettings `mapstructure:"storage"`
	// Warnings lists ignored/removed settings found while loading.
	Warnings []string `mapstructure:"-"`
}

// AppSettings contains application-level configuration.
type AppSettings struct {
	// Debug sets the logging level: DEBUG, INFO, WARN, ERROR, FATAL
	Debug string `mapstructure:"debug"`
	// Port is the HTTP server port.
	Port int `mapstructure:"port"`
	// Timeout is the request timeout for the HTTP server and the default context
	// timeout for storage operations.
	Timeout time.Duration `mapstructure:"timeout"`
}

// PollerSettings contains configuration for the background polling service.
// Interval also drives aggregator debounce (4x), heartbeat (5x) and health graces.
type PollerSettings struct {
	// Interval is the base interval between poll cycles (e.g. "5s").
	Interval time.Duration `mapstructure:"interval"`
	// BlockAttempts is the number of retry attempts per block.
	BlockAttempts int `mapstructure:"block_attempts"`
	// BlockRetryDelay is the delay between retry attempts for the same block.
	BlockRetryDelay time.Duration `mapstructure:"block_retry_delay"`
	// BlockInterval is the delay between successive block reads.
	BlockInterval time.Duration `mapstructure:"block_interval"`
	// PollTimeout is the maximum duration for a full poll cycle before aborting.
	PollTimeout time.Duration `mapstructure:"poll_timeout"`
}

// ModbusSettings contains Modbus connection configuration. Address selects the
// transport via URL scheme: "tcp://host:port" or "rtu://<serial device path>" (e.g.
// "rtu:///dev/ttyUSB0"). Speed, DataBits, Parity and StopBits apply to rtu only.
type ModbusSettings struct {
	// Address is the connection URL, e.g. "tcp://192.168.1.100:502" or "rtu:///dev/ttyUSB0".
	Address string `mapstructure:"address"`
	// Timeout is the connection/read timeout.
	Timeout time.Duration `mapstructure:"timeout"`
	// SlaveID is the Modbus unit/slave ID.
	SlaveID byte `mapstructure:"slave_id"`
	// Speed is the serial link speed in bps (rtu only, library default 19200).
	Speed uint `mapstructure:"speed"`
	// DataBits is the number of bits per serial character (rtu only, library default 8).
	DataBits uint `mapstructure:"data_bits"`
	// Parity is the serial link parity: N, E, or O (rtu only, default N).
	Parity string `mapstructure:"parity"`
	// StopBits is the number of serial stop bits (rtu only, library default 2, or 1 with parity).
	StopBits uint `mapstructure:"stop_bits"`
}

// RolloverSettings contains the daily rollover configuration.
type RolloverSettings struct {
	// Time is the rollover time of day, strict 24-hour HH:MM (default 23:59).
	Time string `mapstructure:"time"`
}

// StorageSettings contains SQLite database configuration.
type StorageSettings struct {
	// Path is the path to the SQLite database file.
	Path string `mapstructure:"path"`
	// DailyRetention is the retention of daily rows; monthly and yearly rows follow it.
	// Only periods of already frozen years are ever deleted, so computed values never
	// shrink (e.g. "1y", "2w", "400d", "8760h").
	DailyRetention time.Duration `mapstructure:"daily_retention"`
	// ErrorRetention is the retention of error/status change rows (e.g. "1y", "30d").
	ErrorRetention time.Duration `mapstructure:"error_retention"`
	// WalMode enables Write-Ahead Logging for better concurrency.
	WalMode bool `mapstructure:"wal_mode"`
	// Synchronous controls the synchronous mode for SQLite: OFF, NORMAL, FULL, EXTRA.
	Synchronous string `mapstructure:"synchronous"`
	// TempStore controls where temporary files are stored: DEFAULT, FILE, MEMORY.
	TempStore string `mapstructure:"temp_store"`
	// EnableBackup enables database backup functionality.
	EnableBackup bool `mapstructure:"enable_backup"`
	// MaxBackups is the maximum number of backup files to keep (0 = unlimited).
	MaxBackups int `mapstructure:"max_backups"`
	// BackupInterval is the interval for periodic online backups (e.g., "24h").
	BackupInterval time.Duration `mapstructure:"backup_interval"`
	// CleanupInterval is the interval for periodic retention cleanup (default 24h).
	CleanupInterval time.Duration `mapstructure:"cleanup_interval"`
}

// setDefaults configures default values for Viper.
func setDefaults(v *viper.Viper) {
	defaults := map[string]any{
		"app.debug": "INFO", "app.port": 8080, "app.timeout": "30s",
		"poller.interval": "30s", "poller.block_attempts": 3,
		"poller.block_retry_delay": "1s", "poller.block_interval": "0s",
		"poller.poll_timeout": "30s",
		"modbus.address":      "tcp://192.168.1.100:502",
		"modbus.timeout":      "5s", "modbus.slave_id": 1,
		"rollover.time":           "23:59",
		"storage.path":            "./data/solis.db",
		"storage.daily_retention": "1y", "storage.error_retention": "1y",
		"storage.wal_mode": true, "storage.synchronous": "NORMAL",
		"storage.temp_store":    "MEMORY",
		"storage.enable_backup": true, "storage.max_backups": 3,
		"storage.backup_interval": "24h", "storage.cleanup_interval": "24h",
	}
	for k, val := range defaults {
		v.SetDefault(k, val)
	}
}

// LoadConfig loads configuration from a YAML file and environment variables.
// Environment variables use the SOLIS_ prefix with underscores (e.g. SOLIS_MODBUS_ADDRESS).
// A missing file falls back to defaults; an invalid configuration is an error.
func LoadConfig(configPath string) (*AppConfig, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(configPath)
	v.AutomaticEnv()
	v.SetEnvPrefix("SOLIS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	setDefaults(v)

	var warnings []string
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		warnings = append(warnings, "config file not found, using defaults")
	}

	var cfg AppConfig
	if err := v.Unmarshal(&cfg, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		durationHook, mapstructure.StringToSliceHookFunc(",")))); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	for key, why := range removedKeys() {
		if v.InConfig(key) {
			warnings = append(warnings, fmt.Sprintf("ignoring removed setting %q: %s", key, why))
		}
	}
	cfg.Warnings = warnings

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate validates every settings section.
func (c *AppConfig) Validate() error {
	validators := []interface{ Validate() error }{
		&c.Modbus, &c.App, &c.Poller, &c.Rollover, &c.Storage,
	}
	for _, v := range validators {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
		}
	}
	if err := c.validatePollTimeout(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return nil
}

// validatePollTimeout keeps one poll cycle inside the supervisor's healthy grace: the
// poller beats at cycle start and end, so a longer cycle would restart a working poller.
func (c *AppConfig) validatePollTimeout() error {
	grace := health.HealthyGraceFactor * c.Poller.Interval
	if c.Poller.PollTimeout >= grace {
		return fmt.Errorf("poll_timeout %s must be below %d x poller.interval (%s)",
			c.Poller.PollTimeout, health.HealthyGraceFactor, grace)
	}
	return nil
}

// durationHook decodes duration strings with the extra units d, w and y ("1y", "2w").
func durationHook(_ reflect.Type, to reflect.Type, data any) (any, error) {
	s, ok := data.(string)
	if !ok || to != reflect.TypeFor[time.Duration]() {
		return data, nil
	}
	return utils.ParseDuration(s)
}

// ModbusClientSettings converts the section into the Modbus client's settings.
func (m *ModbusSettings) ModbusClientSettings() modbus.Settings {
	return modbus.Settings{Address: m.Address, UnitID: m.SlaveID, Timeout: m.Timeout,
		Speed: m.Speed, DataBits: m.DataBits, Parity: m.Parity, StopBits: m.StopBits}
}

// Validate validates Modbus configuration (the client's own rules, checked at startup).
func (m *ModbusSettings) Validate() error {
	return m.ModbusClientSettings().Validate()
}

// Validate validates App configuration.
func (a *AppSettings) Validate() error {
	if !oneOf(strings.ToUpper(a.Debug), "DEBUG", "INFO", "WARN", "ERROR", "FATAL") {
		return fmt.Errorf("invalid debug level: %q (must be DEBUG, INFO, WARN, ERROR, or "+
			"FATAL)", a.Debug)
	}
	if a.Timeout <= 0 {
		return errors.New("app timeout must be positive")
	}
	return validatePort("server port", a.Port)
}

func validatePort(name string, port int) error {
	const maxPort = 65535
	if port <= 0 || port > maxPort {
		return fmt.Errorf("invalid %s: %d (must be 1-65535)", name, port)
	}
	return nil
}

// Validate validates Poller configuration.
func (p *PollerSettings) Validate() error {
	if p.Interval <= 0 {
		return errors.New("poller interval must be positive")
	}
	if p.BlockAttempts <= 0 {
		return errors.New("block_attempts must be at least 1")
	}
	if p.PollTimeout <= 0 {
		return errors.New("poll_timeout must be positive")
	}
	return nil
}

// Validate validates the rollover time (strict 24-hour HH:MM, hard failure otherwise).
func (r *RolloverSettings) Validate() error {
	_, err := period.ParseRollover(r.Time)
	return err
}

// Parsed returns the parsed rollover time; call only after Validate succeeded.
func (r *RolloverSettings) Parsed() (period.Rollover, error) {
	return period.ParseRollover(r.Time)
}

// Validate validates Storage configuration.
func (s *StorageSettings) Validate() error {
	if s.Path == "" {
		return errors.New("storage path is required")
	}
	if err := s.validateRetention(); err != nil {
		return err
	}
	if !oneOf(s.Synchronous, "OFF", "NORMAL", "FULL", "EXTRA") {
		return fmt.Errorf("invalid synchronous mode: %s (must be OFF, NORMAL, FULL, or "+
			"EXTRA)", s.Synchronous)
	}
	if !oneOf(s.TempStore, "DEFAULT", "FILE", "MEMORY") {
		return fmt.Errorf("invalid temp_store: %s (must be DEFAULT, FILE, or MEMORY)",
			s.TempStore)
	}
	if s.MaxBackups < 0 {
		return errors.New("max_backups must be >= 0")
	}
	if s.BackupInterval < 0 {
		return errors.New("backup_interval must be >= 0")
	}
	return nil
}

func (s *StorageSettings) validateRetention() error {
	durations := []struct {
		name string
		d    time.Duration
	}{
		{"daily_retention", s.DailyRetention}, {"error_retention", s.ErrorRetention},
		{"cleanup_interval", s.CleanupInterval},
	}
	for _, d := range durations {
		if d.d <= 0 {
			return fmt.Errorf("%s must be positive", d.name)
		}
	}
	return nil
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
