package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
)

func validConfig() AppConfig {
	return AppConfig{
		App:      AppSettings{Debug: "INFO", Port: 8080, Timeout: 30 * time.Second},
		Poller:   PollerSettings{Interval: 5 * time.Second, BlockAttempts: 2, PollTimeout: 5 * time.Second},
		Modbus:   ModbusSettings{Type: "tcp", Host: "h", Port: 502},
		Rollover: RolloverSettings{Time: "23:59"},
		Storage: StorageSettings{
			Path: "x.db", DailyRetention: time.Hour, MonthlyRetention: time.Hour,
			YearlyRetention: time.Hour, ErrorRetention: time.Hour, CleanupInterval: time.Hour,
			Synchronous: "NORMAL", TempStore: "MEMORY",
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
		{"modbus type", func(c *AppConfig) { c.Modbus.Type = "rtu" }, "invalid modbus type"},
		{"modbus host", func(c *AppConfig) { c.Modbus.Host = "" }, "modbus host is required"},
		{"modbus port", func(c *AppConfig) { c.Modbus.Port = 70000 }, "invalid modbus port"},
		{"app port", func(c *AppConfig) { c.App.Port = 0 }, "invalid server port"},
		{"poll interval", func(c *AppConfig) { c.Poller.Interval = 0 }, "interval must be positive"},
		{"attempts", func(c *AppConfig) { c.Poller.BlockAttempts = 0 }, "block_attempts"},
		{"poll timeout", func(c *AppConfig) { c.Poller.PollTimeout = 0 }, "poll_timeout"},
		{"rollover 12h", func(c *AppConfig) { c.Rollover.Time = "11:59 PM" }, "invalid rollover"},
		{"rollover 24:00", func(c *AppConfig) { c.Rollover.Time = "24:00" }, "invalid rollover"},
		{"storage path", func(c *AppConfig) { c.Storage.Path = "" }, "storage path"},
		{"retention", func(c *AppConfig) { c.Storage.ErrorRetention = 0 }, "error_retention"},
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

func TestRolloverParsed(t *testing.T) {
	r := RolloverSettings{Time: "23:59"}
	p, err := r.Parsed()
	require.NoError(t, err)
	assert.Equal(t, "23:59", p.String())

	cfg := validConfig()
	cfg.Rollover.Time = "9:05"
	assert.ErrorIs(t, cfg.Validate(), period.ErrInvalidRollover)
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
  host: 10.1.1.1
poller:
  interval: 15m
rollover:
  time: "00:30"
`))
	require.NoError(t, err)
	assert.Equal(t, 8081, cfg.App.Port)
	assert.Equal(t, "DEBUG", cfg.App.Debug)
	assert.Equal(t, "10.1.1.1", cfg.Modbus.Host)
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
}

func TestLoadConfig_MissingFileUsesDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.App.Port)
	assert.Len(t, cfg.Warnings, 1)
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	t.Setenv("SOLIS_APP_PORT", "9090")
	t.Setenv("SOLIS_MODBUS_HOST", "10.0.0.1")
	t.Setenv("SOLIS_ROLLOVER_TIME", "22:00")
	cfg, err := LoadConfig(writeConfig(t, "modbus:\n  host: 192.168.1.1\n"))
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.App.Port)
	assert.Equal(t, "10.0.0.1", cfg.Modbus.Host)
	assert.Equal(t, "22:00", cfg.Rollover.Time)
}

func TestLoadConfig_RemovedSettingsWarn(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
app:
  serve_only: true
aggregator:
  interval: 30s
  backfill_current_year_monthly: true
`))
	require.NoError(t, err)
	assert.Len(t, cfg.Warnings, 2)
}

func TestLoadConfig_Errors(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, "app:\n  port: [8080\n"))
	require.Error(t, err)

	_, err = LoadConfig(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")

	_, err = LoadConfig(writeConfig(t, "rollover:\n  time: 11:59 PM\n"))
	assert.ErrorIs(t, err, ErrInvalidConfig)
}
