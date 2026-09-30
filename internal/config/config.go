// Package config provides configuration loading and validation for the Solis monitor application.
// It uses Viper for YAML configuration with environment variable overrides (prefix SOLIS_).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/dombyte/solis/internal/util"
)

// ErrInvalidConfig is wrapped by every validation failure.
var ErrInvalidConfig = errors.New("invalid config")

// ValidationError is one invalid setting, named by its field path ("poller.interval").
type ValidationError struct {
	// Field is the setting's path in config.yaml; "rule" for a rule error without one.
	Field string
	// Err is the underlying validation failure.
	Err error
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Err.Error() }

// Unwrap exposes the underlying failure to errors.Is/As.
func (e *ValidationError) Unwrap() error { return e.Err }

// ValidationErrors lists every problem Validate found.
type ValidationErrors []*ValidationError

func (e ValidationErrors) Error() string {
	msgs := make([]string, len(e))
	for i, ve := range e {
		msgs[i] = ve.Error()
	}
	return fmt.Sprintf("%v: %s", ErrInvalidConfig, strings.Join(msgs, "; "))
}

// Unwrap exposes ErrInvalidConfig and every problem to errors.Is/As.
func (e ValidationErrors) Unwrap() []error {
	errs := make([]error, 0, len(e)+1)
	errs = append(errs, ErrInvalidConfig)
	for _, ve := range e {
		errs = append(errs, ve)
	}
	return errs
}

// add records a problem at field.
func (e *ValidationErrors) add(field string, err error) {
	*e = append(*e, &ValidationError{Field: field, Err: err})
}

// addf records a formatted problem at field.
func (e *ValidationErrors) addf(field, format string, args ...any) {
	e.add(field, fmt.Errorf(format, args...))
}

// addRule records a rule error: its own ValidationErrors keep their fields, anything else
// is filed under "rule".
func (e *ValidationErrors) addRule(err error) {
	var many ValidationErrors
	var one *ValidationError
	switch {
	case errors.As(err, &many):
		*e = append(*e, many...)
	case errors.As(err, &one):
		*e = append(*e, one)
	default:
		e.add("rule", err)
	}
}

// Rule is an extra validation injected by the composition root for settings whose rules
// are owned by other packages (Modbus address, rollover time, health grace). A rule names
// the setting it rejects by returning a *ValidationError.
type Rule func(*AppConfig) error

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
	// SlaveID is the Modbus unit/slave ID (1-247). Decoded as int so an out-of-range
	// value is rejected instead of silently wrapping into a byte.
	SlaveID int `mapstructure:"slave_id"`
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

// envPrefix is the prefix of every environment override (SOLIS_MODBUS_ADDRESS, ...).
const envPrefix = "SOLIS"

// Numeric defaults (string and duration defaults are self-describing in setDefaults).
const (
	defaultPort          = 8080
	defaultBlockAttempts = 3
	defaultMaxBackups    = 3
)

// Bounds checked by Validate.
const (
	minSlaveID = 1
	maxSlaveID = 247 // 0 is broadcast, 248-255 are reserved
	// minInterval bounds poller.interval and app.timeout: a bare number or a typo like
	// "5ms" must not make the app poll or time out requests in microseconds.
	minInterval = time.Second
)

// setDefaults configures default values for Viper. Every key needs a default (zero
// values included): viper's AutomaticEnv only applies SOLIS_* overrides to keys it
// already knows, so a key without a default silently ignores its env variable.
func setDefaults(v *viper.Viper) {
	defaults := map[string]any{
		"app.debug": "INFO", "app.port": defaultPort, "app.timeout": "30s",
		"poller.interval": "30s", "poller.block_attempts": defaultBlockAttempts,
		"poller.block_retry_delay": "1s", "poller.block_interval": "0s",
		"poller.poll_timeout": "30s",
		"modbus.address":      "tcp://192.168.1.100:502",
		"modbus.timeout":      "5s", "modbus.slave_id": 1,
		// rtu serial settings: 0/"" = library default (19200 8N2)
		"modbus.speed": 0, "modbus.data_bits": 0, "modbus.parity": "", "modbus.stop_bits": 0,
		"rollover.time":           "23:59",
		"storage.path":            "./data/solis.db",
		"storage.daily_retention": "10y", "storage.error_retention": "10y",
		"storage.wal_mode": true, "storage.synchronous": "NORMAL",
		"storage.temp_store":    "MEMORY",
		"storage.enable_backup": true, "storage.max_backups": defaultMaxBackups,
		"storage.backup_interval": "24h", "storage.cleanup_interval": "24h",
	}
	for k, val := range defaults {
		v.SetDefault(k, val)
	}
}

// LoadConfig loads configuration from a YAML file and environment variables.
// Environment variables use the SOLIS_ prefix with underscores (e.g. SOLIS_MODBUS_ADDRESS).
// A missing file falls back to defaults; an invalid configuration (including a failed
// rule) is an error.
func LoadConfig(configPath string, rules ...Rule) (*AppConfig, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(configPath)
	v.AutomaticEnv()
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	setDefaults(v)

	warnings, err := readConfigFile(v)
	if err != nil {
		return nil, err
	}

	var cfg AppConfig
	if err := v.Unmarshal(&cfg, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		durationHook, mapstructure.StringToSliceHookFunc(",")))); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	cfg.Warnings = append(warnings, removedKeyWarnings(v)...)

	if err := cfg.Validate(rules...); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// readConfigFile reads the config file; a missing file is a warning, not an error.
func readConfigFile(v *viper.Viper) ([]string, error) {
	err := v.ReadInConfig()
	if err == nil {
		return nil, nil
	}
	var notFound viper.ConfigFileNotFoundError
	if !errors.As(err, &notFound) && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return []string{"config file not found, using defaults"}, nil
}

// removedKeyWarnings lists removed settings that are still present in the file or set
// through their SOLIS_* environment variable.
func removedKeyWarnings(v *viper.Viper) []string {
	var warnings []string
	for key, why := range removedKeys() {
		if v.InConfig(key) || envSet(key) {
			warnings = append(warnings, fmt.Sprintf("ignoring removed setting %q: %s", key, why))
		}
	}
	return warnings
}

// envSet reports whether the SOLIS_* variable of key is set.
func envSet(key string) bool {
	_, ok := os.LookupEnv(envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_")))
	return ok
}

// Validate validates every settings section, then applies rules, and returns every
// problem found as ValidationErrors (nil when the config is valid).
func (c *AppConfig) Validate(rules ...Rule) error {
	var errs ValidationErrors
	c.App.validate(&errs)
	c.Poller.validate(&errs)
	c.Modbus.validate(&errs)
	c.Storage.validate(&errs)
	for _, r := range rules {
		if err := r(c); err != nil {
			errs.addRule(err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// durationHook decodes duration strings with the extra units d, w and y ("1y", "2w").
// A bare number is rejected: mapstructure would read `timeout: 30` as 30 nanoseconds.
func durationHook(_ reflect.Type, to reflect.Type, data any) (any, error) {
	if to != reflect.TypeFor[time.Duration]() {
		return data, nil
	}
	s, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("%w: %v is not a duration string (use a unit, e.g. \"30s\")",
			util.ErrInvalidDuration, data)
	}
	return util.ParseDuration(s)
}

func (a *AppSettings) validate(errs *ValidationErrors) {
	if !oneOf(strings.ToUpper(a.Debug), "DEBUG", "INFO", "WARN", "ERROR", "FATAL") {
		errs.addf("app.debug", "invalid debug level: %q (must be DEBUG, INFO, WARN, ERROR, "+
			"or FATAL)", a.Debug)
	}
	if a.Timeout < minInterval {
		errs.addf("app.timeout", "must be at least %s, got %s", minInterval, a.Timeout)
	}
	const maxPort = 65535
	if a.Port <= 0 || a.Port > maxPort {
		errs.addf("app.port", "invalid server port: %d (must be 1-65535)", a.Port)
	}
}

func (p *PollerSettings) validate(errs *ValidationErrors) {
	if p.Interval < minInterval {
		errs.addf("poller.interval", "must be at least %s, got %s", minInterval, p.Interval)
	}
	if p.BlockAttempts <= 0 {
		errs.addf("poller.block_attempts", "must be at least 1, got %d", p.BlockAttempts)
	}
	if p.PollTimeout <= 0 {
		errs.addf("poller.poll_timeout", "must be positive, got %s", p.PollTimeout)
	}
	if p.BlockRetryDelay < 0 {
		errs.addf("poller.block_retry_delay", "must be >= 0, got %s", p.BlockRetryDelay)
	}
	if p.BlockInterval < 0 {
		errs.addf("poller.block_interval", "must be >= 0, got %s", p.BlockInterval)
	}
}

// validate checks the structural Modbus settings (the address format and serial
// parameters are checked by the modbus package through an injected rule).
func (m *ModbusSettings) validate(errs *ValidationErrors) {
	if m.SlaveID < minSlaveID || m.SlaveID > maxSlaveID {
		errs.addf("modbus.slave_id", "must be %d-%d, got %d", minSlaveID, maxSlaveID, m.SlaveID)
	}
}

func (s *StorageSettings) validate(errs *ValidationErrors) {
	if s.Path == "" {
		errs.addf("storage.path", "is required")
	}
	positive := []struct {
		field string
		d     time.Duration
	}{
		{"storage.daily_retention", s.DailyRetention},
		{"storage.error_retention", s.ErrorRetention},
		{"storage.cleanup_interval", s.CleanupInterval},
	}
	for _, p := range positive {
		if p.d <= 0 {
			errs.addf(p.field, "must be positive, got %s", p.d)
		}
	}
	if !oneOf(s.Synchronous, "OFF", "NORMAL", "FULL", "EXTRA") {
		errs.addf("storage.synchronous", "invalid mode %q (must be OFF, NORMAL, FULL, or "+
			"EXTRA)", s.Synchronous)
	}
	if !oneOf(s.TempStore, "DEFAULT", "FILE", "MEMORY") {
		errs.addf("storage.temp_store", "invalid value %q (must be DEFAULT, FILE, or MEMORY)",
			s.TempStore)
	}
	s.validateBackups(errs)
}

func (s *StorageSettings) validateBackups(errs *ValidationErrors) {
	if s.MaxBackups < 0 {
		errs.addf("storage.max_backups", "must be >= 0, got %d", s.MaxBackups)
	}
	if s.BackupInterval < 0 {
		errs.addf("storage.backup_interval", "must be >= 0, got %s", s.BackupInterval)
	}
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
