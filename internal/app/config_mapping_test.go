package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/storage"
)

// ruleConfig is testConfig with production poller timings (the grace rule rejects the
// fast integration-test interval).
func ruleConfig(t *testing.T) *config.AppConfig {
	t.Helper()
	cfg := testConfig(t, 502, 8080)
	cfg.Poller.Interval, cfg.Poller.PollTimeout = 5*time.Second, 5*time.Second
	return cfg
}

func TestConfigRules(t *testing.T) {
	rtu := func(m config.ModbusSettings) config.ModbusSettings {
		m.Address, m.Timeout = "rtu:///dev/ttyUSB0", time.Second
		return m
	}
	tests := []struct {
		name   string
		mutate func(*config.AppConfig)
		want   string
	}{
		{"valid", func(*config.AppConfig) {}, ""},
		{"modbus rtu valid", func(c *config.AppConfig) { c.Modbus = rtu(config.ModbusSettings{}) },
			""},
		{"modbus scheme", func(c *config.AppConfig) { c.Modbus.Address = "udp://h:502" },
			"must be tcp://host:port or rtu://<device>"},
		{"modbus no scheme", func(c *config.AppConfig) { c.Modbus.Address = "h:502" },
			"must be tcp://host:port or rtu://<device>"},
		{"modbus host", func(c *config.AppConfig) { c.Modbus.Address = "tcp://:502" },
			"host:port required"},
		{"modbus port", func(c *config.AppConfig) { c.Modbus.Address = "tcp://h:70000" },
			"tcp port \"70000\" must be 1-65535"},
		{"modbus timeout", func(c *config.AppConfig) { c.Modbus.Timeout = 0 },
			"timeout 0s must be positive"},
		{"modbus parity", func(c *config.AppConfig) {
			c.Modbus = rtu(config.ModbusSettings{Parity: "X"})
		}, "invalid parity"},
		{"modbus data bits", func(c *config.AppConfig) {
			c.Modbus = rtu(config.ModbusSettings{DataBits: 9})
		}, "data_bits 9"},
		{"modbus stop bits", func(c *config.AppConfig) {
			c.Modbus = rtu(config.ModbusSettings{StopBits: 3})
		}, "stop_bits 3"},
		{"poll timeout vs grace", func(c *config.AppConfig) { c.Poller.PollTimeout = 15 * time.Second },
			"poll_timeout 15s must be below 3 x poller.interval"},
		{"rollover 12h", func(c *config.AppConfig) { c.Rollover.Time = "11:59 PM" },
			"invalid rollover"},
		{"rollover 24:00", func(c *config.AppConfig) { c.Rollover.Time = "24:00" },
			"invalid rollover"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ruleConfig(t)
			tt.mutate(cfg)
			err := cfg.Validate(ConfigRules()...)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, config.ErrInvalidConfig)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestConfigRules_RolloverSentinel(t *testing.T) {
	cfg := ruleConfig(t)
	cfg.Rollover.Time = "9:05"
	assert.ErrorIs(t, cfg.Validate(ConfigRules()...), period.ErrInvalidRollover)
}

func TestSettingsMapping(t *testing.T) {
	s := config.StorageSettings{Path: "x.db", DailyRetention: time.Hour,
		ErrorRetention: time.Minute, WalMode: true, Synchronous: "FULL", TempStore: "FILE",
		CleanupInterval: 2 * time.Hour}
	assert.Equal(t, storage.Settings{Path: "x.db", DailyRetention: time.Hour,
		ErrorRetention: time.Minute, WalMode: true, Synchronous: "FULL", TempStore: "FILE"},
		StorageSettings(s))
	assert.Equal(t, database.Settings{Path: "x.db", CleanupInterval: 2 * time.Hour},
		DatabaseSettings(s))
}
