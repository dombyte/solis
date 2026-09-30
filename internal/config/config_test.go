package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConfig() AppConfig {
	return AppConfig{
		App:      AppSettings{Debug: "INFO", Port: 8080, Timeout: 30 * time.Second},
		Poller:   PollerSettings{Interval: 5 * time.Second, BlockAttempts: 2, PollTimeout: 5 * time.Second},
		Modbus:   ModbusSettings{Address: "tcp://h:502", Timeout: time.Second, SlaveID: 1},
		Rollover: RolloverSettings{Time: "23:59"},
		Storage: StorageSettings{
			Path: "x.db", DailyRetention: time.Hour, ErrorRetention: time.Hour,
			CleanupInterval: time.Hour,
			Synchronous:     "NORMAL", TempStore: "MEMORY",
		},
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*AppConfig)
		want   string
	}{
		{"valid", func(*AppConfig) {}, ""},
		{"app debug", func(c *AppConfig) { c.App.Debug = "DEBG" }, "app.debug: invalid debug level"},
		{"app debug lower", func(c *AppConfig) { c.App.Debug = "warn" }, ""},
		{"app timeout", func(c *AppConfig) { c.App.Timeout = 0 }, "app.timeout: must be at least"},
		{
			"app timeout below 1s", func(c *AppConfig) { c.App.Timeout = 30 * time.Nanosecond },
			"app.timeout: must be at least 1s, got 30ns",
		},
		{"app port", func(c *AppConfig) { c.App.Port = 0 }, "app.port: invalid server port: 0"},
		{"poll interval", func(c *AppConfig) { c.Poller.Interval = 0 }, "poller.interval: must be at least"},
		{
			"poll interval ms", func(c *AppConfig) { c.Poller.Interval = 5 * time.Millisecond },
			"poller.interval: must be at least 1s, got 5ms",
		},
		{
			"negative retry delay", func(c *AppConfig) { c.Poller.BlockRetryDelay = -1 },
			"poller.block_retry_delay: must be >= 0",
		},
		{"negative block interval", func(c *AppConfig) { c.Poller.BlockInterval = -1 }, "poller.block_interval: must be >= 0"},
		{"slave id 0", func(c *AppConfig) { c.Modbus.SlaveID = 0 }, "modbus.slave_id: must be 1-247"},
		{"slave id 300", func(c *AppConfig) { c.Modbus.SlaveID = 300 }, "got 300"},
		{"slave id 247", func(c *AppConfig) { c.Modbus.SlaveID = 247 }, ""},
		{"attempts", func(c *AppConfig) { c.Poller.BlockAttempts = 0 }, "poller.block_attempts: must be at least 1"},
		{"poll timeout", func(c *AppConfig) { c.Poller.PollTimeout = 0 }, "poller.poll_timeout: must be positive"},
		{"storage path", func(c *AppConfig) { c.Storage.Path = "" }, "storage.path: is required"},
		{"retention", func(c *AppConfig) { c.Storage.ErrorRetention = 0 }, "storage.error_retention: must be positive"},
		{
			"daily retention", func(c *AppConfig) { c.Storage.DailyRetention = 0 },
			"storage.daily_retention: must be positive",
		},
		{"sync", func(c *AppConfig) { c.Storage.Synchronous = "X" }, "storage.synchronous: invalid mode"},
		{"temp", func(c *AppConfig) { c.Storage.TempStore = "X" }, "storage.temp_store: invalid value"},
		{"backups", func(c *AppConfig) { c.Storage.MaxBackups = -1 }, "storage.max_backups: must be >= 0"},
		{"backup interval", func(c *AppConfig) { c.Storage.BackupInterval = -1 }, "storage.backup_interval: must be >= 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "rule", ve.Field, "a rule error without a field")

	named := func(*AppConfig) error { return &ValidationError{Field: "rollover.time", Err: errRule} }
	many := func(*AppConfig) error {
		return ValidationErrors{{Field: "a", Err: errRule}, {Field: "b", Err: errRule}}
	}
	var errs ValidationErrors
	require.ErrorAs(t, cfg.Validate(named, many), &errs)
	fields := make([]string, len(errs))
	for i, e := range errs {
		fields[i] = e.Field
	}
	assert.Equal(t, []string{"rollover.time", "a", "b"}, fields, "rules keep their fields")
}

// Validate reports every problem with its field path, not only the first one.
func TestValidate_ReportsAllProblems(t *testing.T) {
	cfg := validConfig()
	cfg.App.Port = 0
	cfg.Poller.Interval = 0
	cfg.Modbus.SlaveID = 0
	cfg.Storage.Path = ""
	err := cfg.Validate(func(*AppConfig) error { return errors.New("rule failed") })
	require.ErrorIs(t, err, ErrInvalidConfig)
	assert.EqualError(t, err, "invalid config: "+
		"app.port: invalid server port: 0 (must be 1-65535); "+
		"poller.interval: must be at least 1s, got 0s; "+
		"modbus.slave_id: must be 1-247, got 0; "+
		"storage.path: is required; "+
		"rule: rule failed")
	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	assert.Len(t, errs, 5)
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
	assert.Equal(t, 10*365*24*time.Hour, cfg.Storage.DailyRetention)
	assert.Equal(t, 10*365*24*time.Hour, cfg.Storage.ErrorRetention)
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

// Every key must accept its SOLIS_* override, including keys whose default is a zero
// value (review RT-M1: the rtu serial settings silently ignored their env variables).
func TestLoadConfig_EnvOverridesKeysWithoutFileEntry(t *testing.T) {
	t.Setenv("SOLIS_MODBUS_SPEED", "9600")
	t.Setenv("SOLIS_MODBUS_DATA_BITS", "7")
	t.Setenv("SOLIS_MODBUS_PARITY", "E")
	t.Setenv("SOLIS_MODBUS_STOP_BITS", "1")
	t.Setenv("SOLIS_MODBUS_SLAVE_ID", "3")
	cfg, err := LoadConfig(writeConfig(t, "modbus:\n  address: \"rtu:///dev/ttyUSB0\"\n"))
	require.NoError(t, err)
	assert.Equal(t, uint(9600), cfg.Modbus.Speed)
	assert.Equal(t, uint(7), cfg.Modbus.DataBits)
	assert.Equal(t, "E", cfg.Modbus.Parity)
	assert.Equal(t, uint(1), cfg.Modbus.StopBits)
	assert.Equal(t, 3, cfg.Modbus.SlaveID)
}

// Guards RT-M1 for future keys: every mapstructure key of AppConfig has a default.
func TestSetDefaults_CoversEveryKey(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	for _, key := range configKeys(reflect.TypeFor[AppConfig](), "") {
		assert.True(t, v.IsSet(key), "no default for %s: its SOLIS_* override would be ignored", key)
	}
}

// configKeys lists the dotted mapstructure keys of the leaf fields of t.
func configKeys(t reflect.Type, prefix string) []string {
	var keys []string
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}
		if f.Type.Kind() == reflect.Struct && f.Type != reflect.TypeFor[time.Duration]() {
			keys = append(keys, configKeys(f.Type, prefix+tag+".")...)
			continue
		}
		keys = append(keys, prefix+tag)
	}
	return keys
}

// A bare number is not a duration (review RT-M3: `timeout: 30` used to become 30ns).
func TestLoadConfig_BareNumberDurationRejected(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, "app:\n  timeout: 30\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a duration string")
	_, err = LoadConfig(writeConfig(t, "poller:\n  interval: 5\n"))
	require.Error(t, err)
}

// An out-of-range slave_id is rejected instead of wrapping (review RT-M2: 300 -> 44).
func TestLoadConfig_SlaveIDOutOfRange(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, "modbus:\n  slave_id: 300\n"))
	require.ErrorIs(t, err, ErrInvalidConfig)
	assert.Contains(t, err.Error(), "modbus.slave_id: must be 1-247, got 300")
}

func TestLoadConfig_RemovedSettingViaEnvWarns(t *testing.T) {
	t.Setenv("SOLIS_APP_SERVE_ONLY", "true")
	cfg, err := LoadConfig(writeConfig(t, "app:\n  port: 8080\n"))
	require.NoError(t, err)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0], "app.serve_only")
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
