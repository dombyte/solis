// Package config provides configuration loading and validation for the Solis monitor application.
// It uses Viper for YAML configuration with environment variable overrides.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/dombyte/solis/internal/logging"
	"github.com/spf13/viper"
)

// logger is the package-level logger for config operations.
var logger = logging.NewComponentLogger("config")

// AppConfig is the root configuration structure.
type AppConfig struct {
	// App contains application-level settings.
	App AppSettings `mapstructure:"app"`
	// Poller contains polling service settings.
	Poller PollerSettings `mapstructure:"poller"`
	// Modbus contains Modbus connection settings.
	Modbus ModbusSettings `mapstructure:"modbus"`
	// Storage contains database settings.
	Storage StorageSettings `mapstructure:"storage"`
	// Aggregator contains background computation settings.
	Aggregator AggregatorSettings `mapstructure:"aggregator"`
}

// AppSettings contains application-level configuration.
type AppSettings struct {
	// Debug sets the logging level: DEBUG, INFO, WARN, ERROR, FATAL
	Debug string `mapstructure:"debug"`
	// Port is the HTTP server port.
	Port int `mapstructure:"port"`
	// Timeout is the request timeout for the HTTP server.
	Timeout time.Duration `mapstructure:"timeout"`
	// ServeOnly enables serve-only mode (disables Modbus polling).
	// When true, the HTTP API serves cached and stored data without connecting to the inverter.
	ServeOnly bool `mapstructure:"serve_only"`
}

// PollerSettings contains configuration for the background polling service.
type PollerSettings struct {
	// Interval is the base interval between poll cycles.
	// Example: "15m", "10s", "1h"
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

// ModbusSettings contains Modbus connection configuration.
// Only TCP is supported.
type ModbusSettings struct {
	// Type is the connection type: only "tcp" is supported.
	Type string `mapstructure:"type"`
	// Host is the Modbus server IP address or hostname.
	Host string `mapstructure:"host"`
	// Port is the Modbus server port (default: 502).
	Port int `mapstructure:"port"`
	// Timeout is the connection/read timeout.
	Timeout time.Duration `mapstructure:"timeout"`
	// UnitID is the Modbus unit/slave ID.
	UnitID byte `mapstructure:"unit_id"`
}

// StorageSettings contains SQLite database configuration.
type StorageSettings struct {
	// Path is the path to the SQLite database file.
	Path string `mapstructure:"path"`
	// DailyRetention is the retention period for daily aggregated data.
	DailyRetention time.Duration `mapstructure:"daily_retention"`
	// MonthlyRetention is the retention period for monthly aggregated data.
	MonthlyRetention time.Duration `mapstructure:"monthly_retention"`
	// YearlyRetention is the retention period for yearly aggregated data.
	YearlyRetention time.Duration `mapstructure:"yearly_retention"`
	// ErrorRetention is the retention period for error/fault data.
	ErrorRetention time.Duration `mapstructure:"error_retention"`
	// WalMode enables Write-Ahead Logging for better concurrency.
	WalMode bool `mapstructure:"wal_mode"`
	// Synchronous controls the synchronous mode for SQLite.
	// "OFF", "NORMAL", "FULL", "EXTRA"
	Synchronous string `mapstructure:"synchronous"`
	// TempStore controls where temporary files are stored.
	// "DEFAULT", "FILE", "MEMORY"
	TempStore string `mapstructure:"temp_store"`

	// Database maintenance settings
	// EnableMigrations enables automatic schema migrations on startup.
	EnableMigrations bool `mapstructure:"enable_migrations"`
	// EnableBackup enables database backup functionality.
	EnableBackup bool `mapstructure:"enable_backup"`
	// MaxBackups is the maximum number of backup files to keep (0 = unlimited).
	MaxBackups int `mapstructure:"max_backups"`
	// BackupInterval is the interval for periodic online backups (e.g., "24h").
	BackupInterval time.Duration `mapstructure:"backup_interval"`
	// CleanupInterval is the interval for periodic retention cleanup (e.g., "24h").
	// Default: 24h
	CleanupInterval time.Duration `mapstructure:"cleanup_interval"`
}

// AggregatorSettings contains configuration for the background aggregator service.
type AggregatorSettings struct {
	// Interval is the interval between computation cycles.
	// This controls how often monthly/yearly/net values are recomputed.
	// Example: "60s", "5m", "1h"
	// Default: "60s"
	Interval time.Duration `mapstructure:"interval"`
}

// setDefaults configures default values for Viper.
func setDefaults(v *viper.Viper) {
	// App defaults
	v.SetDefault("app.debug", "INFO")
	v.SetDefault("app.port", 8080)
	v.SetDefault("app.timeout", "30s")
	v.SetDefault("app.serve_only", false)

	// Poller defaults
	v.SetDefault("poller.interval", "30s")
	v.SetDefault("poller.block_attempts", 3)
	v.SetDefault("poller.block_retry_delay", "1s")
	v.SetDefault("poller.block_interval", "0s")
	v.SetDefault("poller.poll_timeout", "30s")

	// Aggregator defaults
	v.SetDefault("aggregator.interval", "60s")

	// Modbus defaults
	v.SetDefault("modbus.type", "tcp")
	v.SetDefault("modbus.host", "192.168.1.100")
	v.SetDefault("modbus.port", 502)
	v.SetDefault("modbus.timeout", "5s")
	v.SetDefault("modbus.unit_id", 1)

	// Storage defaults
	v.SetDefault("storage.path", "./data/solis.db")
	v.SetDefault("storage.daily_retention", "8760h")
	v.SetDefault("storage.monthly_retention", "8760h")
	v.SetDefault("storage.yearly_retention", "8760h")
	v.SetDefault("storage.error_retention", "720h")
	v.SetDefault("storage.wal_mode", true)
	v.SetDefault("storage.synchronous", "NORMAL")
	v.SetDefault("storage.temp_store", "MEMORY")

	// Database maintenance defaults
	v.SetDefault("storage.enable_migrations", true)
	v.SetDefault("storage.enable_backup", true)
	v.SetDefault("storage.max_backups", 3)
	v.SetDefault("storage.backup_interval", "24h")
	v.SetDefault("storage.cleanup_interval", "24h")
}

// LoadConfig loads configuration from a YAML file and environment variables.
// It supports automatic environment variable overrides with the prefix "SOLIS_".
// Environment variables use underscore notation (e.g., SOLIS_MODBUS_HOST).
func LoadConfig(configPath string) (*AppConfig, error) {
	logger.Info().Msgf("Loading configuration from %s", configPath)

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(configPath)
	v.AutomaticEnv()
	v.SetEnvPrefix("SOLIS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			logger.Error().Msgf("Failed to read config file: %v", err)
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		logger.Info().Msg("Config file not found, using defaults")
	}

	var config AppConfig
	if err := v.Unmarshal(&config); err != nil {
		logger.Error().Msgf("Failed to unmarshal config: %v", err)
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		logger.Error().Msgf("Invalid configuration: %v", err)
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	logger.Info().Msg("Configuration loaded and validated successfully")
	return &config, nil
}

// validateConfig validates the loaded configuration values.
func validateConfig(cfg *AppConfig) error {
	if err := validateModbusConfig(&cfg.Modbus); err != nil {
		return err
	}
	if err := validateAppConfig(&cfg.App); err != nil {
		return err
	}
	if err := validatePollerConfig(&cfg.Poller); err != nil {
		return err
	}
	if err := validateStorageConfig(&cfg.Storage); err != nil {
		return err
	}
	return nil
}

// validateModbusConfig validates Modbus configuration
func validateModbusConfig(cfg *ModbusSettings) error {
	if cfg.Type != "tcp" {
		return fmt.Errorf("invalid modbus type: %s (only tcp is supported)", cfg.Type)
	}
	if cfg.Host == "" {
		return fmt.Errorf("modbus host is required for tcp connections")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid modbus port: %d (must be 1-65535)", cfg.Port)
	}
	return nil
}

// validateAppConfig validates App configuration
func validateAppConfig(cfg *AppSettings) error {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", cfg.Port)
	}
	return nil
}

// validatePollerConfig validates Poller configuration
func validatePollerConfig(cfg *PollerSettings) error {
	if cfg.Interval <= 0 {
		return fmt.Errorf("poller interval must be positive")
	}
	if cfg.BlockAttempts <= 0 {
		return fmt.Errorf("block_attempts must be at least 1")
	}
	if cfg.PollTimeout <= 0 {
		return fmt.Errorf("poll_timeout must be positive")
	}
	return nil
}

// validateStorageConfig validates Storage configuration
func validateStorageConfig(cfg *StorageSettings) error {
	if err := validateStoragePath(cfg); err != nil {
		return err
	}
	if err := validateStorageRetention(cfg); err != nil {
		return err
	}
	logRetentionConfig(cfg)
	if err := validateStorageSyncMode(cfg); err != nil {
		return err
	}
	if err := validateStorageTempStore(cfg); err != nil {
		return err
	}
	if err := validateStorageBackup(cfg); err != nil {
		return err
	}
	return nil
}

// validateStoragePath validates the storage path
func validateStoragePath(cfg *StorageSettings) error {
	if cfg.Path == "" {
		return fmt.Errorf("storage path is required")
	}
	return nil
}

// validateStorageRetention validates retention settings
func validateStorageRetention(cfg *StorageSettings) error {
	if cfg.DailyRetention <= 0 {
		return fmt.Errorf("daily_retention must be positive")
	}
	if cfg.MonthlyRetention <= 0 {
		return fmt.Errorf("monthly_retention must be positive")
	}
	if cfg.YearlyRetention <= 0 {
		return fmt.Errorf("yearly_retention must be positive")
	}
	if cfg.ErrorRetention <= 0 {
		return fmt.Errorf("error_retention must be positive")
	}
	if cfg.CleanupInterval <= 0 {
		return fmt.Errorf("cleanup_interval must be positive")
	}
	return nil
}

// logRetentionConfig logs the retention configuration
func logRetentionConfig(cfg *StorageSettings) {
	logger.Info().Msgf("Retention configuration: daily=%s, monthly=%s, yearly=%s, error=%s, cleanup_interval=%s",
		cfg.DailyRetention, cfg.MonthlyRetention,
		cfg.YearlyRetention, cfg.ErrorRetention, cfg.CleanupInterval)
}

// validateStorageSyncMode validates the synchronous mode
func validateStorageSyncMode(cfg *StorageSettings) error {
	validSyncModes := map[string]bool{"OFF": true, "NORMAL": true, "FULL": true, "EXTRA": true}
	if !validSyncModes[cfg.Synchronous] {
		return fmt.Errorf("invalid synchronous mode: %s (must be OFF, NORMAL, FULL, or EXTRA)", cfg.Synchronous)
	}
	return nil
}

// validateStorageTempStore validates the temp store setting
func validateStorageTempStore(cfg *StorageSettings) error {
	validTempStores := map[string]bool{"DEFAULT": true, "FILE": true, "MEMORY": true}
	if !validTempStores[cfg.TempStore] {
		return fmt.Errorf("invalid temp_store: %s (must be DEFAULT, FILE, or MEMORY)", cfg.TempStore)
	}
	return nil
}

// validateStorageBackup validates backup settings
func validateStorageBackup(cfg *StorageSettings) error {
	if cfg.MaxBackups < 0 {
		return fmt.Errorf("max_backups must be >= 0")
	}
	if cfg.BackupInterval < 0 {
		return fmt.Errorf("backup_interval must be >= 0")
	}
	return nil
}
