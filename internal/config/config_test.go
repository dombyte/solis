package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConfig() AppConfig {
	return AppConfig{
		App:      AppSettings{Debug: "INFO", Port: 8080, Timeout: 30 * time.Second},
		Poller:   PollerSettings{Interval: 5 * time.Second, BlockAttempts: 2, PollTimeout: 5 * time.Second},
		Modbus:   ModbusSettings{Address: "tcp://h:502", Timeout: time.Second},
		Rollover: RolloverSettings{Time: "23:59"},
		Storage: StorageSettings{
			Path: "x.db", DailyRetention: time.Hour, ErrorRetention: time.Hour,
			CleanupInterval: time.Hour,
			Synchronous:     "NORMAL", TempStore: "MEMORY",
		},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AppConfig)
		want   string
	}{
		{"valid", func(*AppConfig) {}, ""},
		{"app debug", func(c *AppConfig) { c.App.Debug = "DEBG" }, "invalid debug level"},
		{"app debug lower", func(c *AppConfig) { c.App.Debug = "warn" }, ""},
		{"app timeout", func(c *AppConfig) { c.App.Timeout = 0 }, "app timeout"},
		{"app port", func(c *AppConfig) { c.App.Port = 0 }, "invalid server port"},
		{"poll interval", func(c *AppConfig) { c.Poller.Interval = 0 }, "interval must be positive"},
		{"attempts", func(c *AppConfig) { c.Poller.BlockAttempts = 0 }, "block_attempts"},
		{"poll timeout", func(c *AppConfig) { c.Poller.PollTimeout = 0 }, "poll_timeout"},
		{"storage path", func(c *AppConfig) { c.Storage.Path = "" }, "storage path"},
		{"retention", func(c *AppConfig) { c.Storage.ErrorRetention = 0 }, "error_retention"},
		{
			"daily retention", func(c *AppConfig) { c.Storage.DailyRetention = 0 },
			"daily_retention",
		},
		{"sync", func(c *AppConfig) { c.Storage.Synchronous = "X" }, "synchronous"},
		{"temp", func(c *AppConfig) { c.Storage.TempStore = "X" }, "temp_store"},
		{"backups", func(c *AppConfig) { c.Storage.MaxBackups = -1 }, "max_backups"},
		{"backup interval", func(c *AppConfig) { c.Storage.BackupInterval = -1 }, "backup_interval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidConfig)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidate_Rules(t *testing.T) {
	errRule := errors.New("rule failed")
	var seen *AppConfig
	cfg := validConfig()
	pass := func(c *AppConfig) error { seen = c; return nil }
	require.NoError(t, cfg.Validate(pass))
	assert.Same(t, &cfg, seen, "rules get the config being validated")

	err := cfg.Validate(pass, func(*AppConfig) error { return errRule })
	require.ErrorIs(t, err, ErrInvalidConfig)
	assert.ErrorIs(t, err, errRule)
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadConfig_WithFile(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
app:
  debug: DEBUG
  port: 8081
modbus:
  address: "tcp://10.1.1.1:502"
poller:
  interval: 15m
rollover:
  time: "00:30"
`))
	require.NoError(t, err)
	assert.Equal(t, 8081, cfg.App.Port)
	assert.Equal(t, "DEBUG", cfg.App.Debug)
	assert.Equal(t, "tcp://10.1.1.1:502", cfg.Modbus.Address)
	assert.Equal(t, 15*time.Minute, cfg.Poller.Interval)
	assert.Equal(t, "00:30", cfg.Rollover.Time)
	assert.Empty(t, cfg.Warnings)
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, ""))
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.App.Port)
	assert.Equal(t, "INFO", cfg.App.Debug)
	assert.Equal(t, "23:59", cfg.Rollover.Time)
	assert.Equal(t, "./data/solis.db", cfg.Storage.Path)
	assert.Equal(t, 30*time.Second, cfg.Poller.Interval)
	assert.Equal(t, 365*24*time.Hour, cfg.Storage.DailyRetention)
	assert.Equal(t, 365*24*time.Hour, cfg.Storage.ErrorRetention)
}

func TestLoadConfig_LongDurationUnits(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
storage:
  daily_retention: 2w
  error_retention: 1y12h
  cleanup_interval: 1d
`))
	require.NoError(t, err)
	assert.Equal(t, 14*24*time.Hour, cfg.Storage.DailyRetention)
	assert.Equal(t, 365*24*time.Hour+12*time.Hour, cfg.Storage.ErrorRetention)
	assert.Equal(t, 24*time.Hour, cfg.Storage.CleanupInterval)

	_, err = LoadConfig(writeConfig(t, "storage:\n  daily_retention: 1x\n"))
	require.Error(t, err)

	t.Setenv("SOLIS_STORAGE_DAILY_RETENTION", "3d")
	cfg, err = LoadConfig(writeConfig(t, ""))
	require.NoError(t, err)
	assert.Equal(t, 3*24*time.Hour, cfg.Storage.DailyRetention)
}

func TestLoadConfig_MissingFileUsesDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.App.Port)
	assert.Len(t, cfg.Warnings, 1)
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	t.Setenv("SOLIS_APP_PORT", "9090")
	t.Setenv("SOLIS_MODBUS_ADDRESS", "tcp://10.0.0.1:502")
	t.Setenv("SOLIS_ROLLOVER_TIME", "22:00")
	cfg, err := LoadConfig(writeConfig(t, "modbus:\n  address: \"tcp://192.168.1.1:502\"\n"))
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.App.Port)
	assert.Equal(t, "tcp://10.0.0.1:502", cfg.Modbus.Address)
	assert.Equal(t, "22:00", cfg.Rollover.Time)
}

func TestLoadConfig_RemovedSettingsWarn(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
app:
  serve_only: true
aggregator:
  interval: 30s
  backfill_current_year_monthly: true
storage:
  monthly_retention: 87600h
  yearly_retention: 87600h
  enable_migrations: true
`))
	require.NoError(t, err)
	assert.Len(t, cfg.Warnings, 5)
}

func TestLoadConfig_Errors(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, "app:\n  port: [8080\n"))
	require.Error(t, err)

	_, err = LoadConfig(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")

	_, err = LoadConfig(writeConfig(t, "app:\n  port: 0\n"))
	assert.ErrorIs(t, err, ErrInvalidConfig)

	errRule := errors.New("rule failed")
	_, err = LoadConfig(writeConfig(t, "app:\n  port: 8080\n"),
		func(*AppConfig) error { return errRule })
	assert.ErrorIs(t, err, errRule)
}
