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
	// Metrics contains optional Prometheus settings.
	Metrics MetricsSettings `mapstructure:"metrics"`
	// Registers contains register-specific settings.
	Registers RegistersSettings `mapstructure:"registers"`
}

// AppSettings contains application-level configuration.
type AppSettings struct {
	// Debug sets the logging level: DEBUG, INFO, WARN, ERROR, FATAL
	Debug string `mapstructure:"debug"`
	// Port is the HTTP server port.
	Port int `mapstructure:"port"`
	// Timeout is the request timeout for the HTTP server.
	Timeout time.Duration `mapstructure:"timeout"`
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
	// JitterMax is the maximum random delay added before each poll for RTU connections.
	// This helps avoid collisions with other devices on the same RTU bus.
	// Example: "500ms" for 0-500ms random delay.
	JitterMax time.Duration `mapstructure:"jitter_max"`
}

// ModbusSettings contains Modbus connection configuration.
type ModbusSettings struct {
	// Type is the connection type: "tcp", "rtu", or "rtu_over_tcp".
	Type string `mapstructure:"type"`
	// Host is the Modbus server IP address or hostname (for TCP).
	Host string `mapstructure:"host"`
	// Port is the Modbus server port (default: 502).
	Port int `mapstructure:"port"`
	// Timeout is the connection/read timeout.
	Timeout time.Duration `mapstructure:"timeout"`
	// UnitID is the Modbus unit/slave ID.
	UnitID byte `mapstructure:"unit_id"`
	// SerialPort is the serial port for RTU connections.
	SerialPort string `mapstructure:"serial_port"`
	// BaudRate is the baud rate for RTU connections.
	BaudRate int `mapstructure:"baud_rate"`
	// DataBits is the number of data bits for RTU connections.
	DataBits int `mapstructure:"data_bits"`
	// StopBits is the number of stop bits for RTU connections.
	StopBits int `mapstructure:"stop_bits"`
	// Parity is the parity setting for RTU connections: "none", "even", "odd".
	Parity string `mapstructure:"parity"`
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
}

// MetricsSettings contains Prometheus metrics configuration.
type MetricsSettings struct {
	// Enabled enables the Prometheus metrics endpoint.
	Enabled bool `mapstructure:"enabled"`
}

// RegistersSettings contains register-specific configuration.
type RegistersSettings struct {
	// DisabledKeys is a list of register keys to disable.
	// Disabled registers will not be polled, stored, cached, or returned by the API.
	DisabledKeys []string `mapstructure:"disabled_keys"`
}

// setDefaults configures default values for Viper.
func setDefaults(v *viper.Viper) {
	// App defaults
	v.SetDefault("app.debug", "INFO")
	v.SetDefault("app.port", 8080)
	v.SetDefault("app.timeout", "30s")

	// Poller defaults
	v.SetDefault("poller.interval", "30s")
	v.SetDefault("poller.block_attempts", 3)
	v.SetDefault("poller.block_retry_delay", "1s")
	v.SetDefault("poller.block_interval", "0s")
	v.SetDefault("poller.poll_timeout", "30s")
	v.SetDefault("poller.jitter_max", "500ms")

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

	// Metrics and registers defaults
	v.SetDefault("metrics.enabled", false)
	v.SetDefault("registers.disabled_keys", []string{})
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
	// Validate Modbus settings
	if cfg.Modbus.Type != "tcp" && cfg.Modbus.Type != "rtu" && cfg.Modbus.Type != "rtu_over_tcp" {
		return fmt.Errorf("invalid modbus type: %s (must be tcp, rtu, or rtu_over_tcp)", cfg.Modbus.Type)
	}

	if cfg.Modbus.Type == "tcp" || cfg.Modbus.Type == "rtu_over_tcp" {
		if cfg.Modbus.Host == "" {
			return fmt.Errorf("modbus host is required for tcp and rtu_over_tcp connections")
		}
		if cfg.Modbus.Port <= 0 || cfg.Modbus.Port > 65535 {
			return fmt.Errorf("invalid modbus port: %d (must be 1-65535)", cfg.Modbus.Port)
		}
	}

	if cfg.Modbus.Type == "rtu" {
		if cfg.Modbus.SerialPort == "" {
			return fmt.Errorf("serial_port is required for rtu connections")
		}
		if cfg.Modbus.BaudRate <= 0 {
			return fmt.Errorf("invalid baud rate: %d", cfg.Modbus.BaudRate)
		}
	}

	// Validate App server settings
	if cfg.App.Port <= 0 || cfg.App.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", cfg.App.Port)
	}

	// Validate Poller settings
	if cfg.Poller.Interval <= 0 {
		return fmt.Errorf("poller interval must be positive")
	}
	if cfg.Poller.BlockAttempts <= 0 {
		return fmt.Errorf("block_attempts must be at least 1")
	}
	if cfg.Poller.PollTimeout <= 0 {
		return fmt.Errorf("poll_timeout must be positive")
	}

	// Validate Storage settings - storage is always enabled
	if cfg.Storage.Path == "" {
		return fmt.Errorf("storage path is required")
	}
	if cfg.Storage.DailyRetention <= 0 {
		return fmt.Errorf("daily_retention must be positive")
	}
	// Set defaults for monthly and yearly retention if not configured
	if cfg.Storage.MonthlyRetention <= 0 {
		cfg.Storage.MonthlyRetention = 365 * 24 * time.Hour
		logger.Info().Msgf("Using default monthly_retention: %s", cfg.Storage.MonthlyRetention)
	}
	if cfg.Storage.YearlyRetention <= 0 {
		cfg.Storage.YearlyRetention = 365 * 24 * time.Hour
		logger.Info().Msgf("Using default yearly_retention: %s", cfg.Storage.YearlyRetention)
	}
	if cfg.Storage.ErrorRetention <= 0 {
		return fmt.Errorf("error_retention must be positive")
	}

	validSyncModes := map[string]bool{"OFF": true, "NORMAL": true, "FULL": true, "EXTRA": true}
	if !validSyncModes[cfg.Storage.Synchronous] {
		return fmt.Errorf("invalid synchronous mode: %s (must be OFF, NORMAL, FULL, or EXTRA)", cfg.Storage.Synchronous)
	}

	validTempStores := map[string]bool{"DEFAULT": true, "FILE": true, "MEMORY": true}
	if !validTempStores[cfg.Storage.TempStore] {
		return fmt.Errorf("invalid temp_store: %s (must be DEFAULT, FILE, or MEMORY)", cfg.Storage.TempStore)
	}
	return nil
}
