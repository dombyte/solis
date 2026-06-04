package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *AppConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid TCP config",
			config: &AppConfig{
				App: AppSettings{Port: 8080, Timeout: 30 * time.Second, Debug: "INFO"},
				Modbus: ModbusSettings{
					Type:    "tcp",
					Host:    "192.168.1.100",
					Port:    502,
					Timeout: 5 * time.Second,
					UnitID:  1,
				},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					WalMode:     true,
					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "valid RTU config",
			config: &AppConfig{
				App: AppSettings{Port: 8080, Timeout: 30 * time.Second, Debug: "INFO"},
				Modbus: ModbusSettings{
					Type:       "rtu",
					SerialPort: "/dev/ttyUSB0",
					BaudRate:   9600,
					DataBits:   8,
					StopBits:   1,
					Parity:     "none",
					Timeout:    5 * time.Second,
					UnitID:     1,
				},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					WalMode:     true,
					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "valid rtu_over_tcp config",
			config: &AppConfig{
				App: AppSettings{Port: 8080, Timeout: 30 * time.Second, Debug: "INFO"},
				Modbus: ModbusSettings{
					Type:    "rtu_over_tcp",
					Host:    "192.168.1.100",
					Port:    502,
					Timeout: 5 * time.Second,
					UnitID:  1,
				},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					WalMode:     true,
					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid modbus type",
			config: &AppConfig{
				Modbus: ModbusSettings{Type: "invalid"},
				App:    AppSettings{Port: 8080},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "invalid modbus type",
		},
		{
			name: "tcp without host",
			config: &AppConfig{
				Modbus: ModbusSettings{Type: "tcp", Port: 502},
				App:    AppSettings{Port: 8080},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "host is required",
		},
		{
			name: "invalid tcp port",
			config: &AppConfig{
				Modbus: ModbusSettings{Type: "tcp", Host: "192.168.1.100", Port: 0},
				App:    AppSettings{Port: 8080},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "invalid modbus port",
		},
		{
			name: "invalid server port",
			config: &AppConfig{
				App:    AppSettings{Port: 0},
				Modbus: ModbusSettings{Type: "tcp", Host: "192.168.1.100", Port: 502},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "invalid server port",
		},
		{
			name: "rtu without serial port",
			config: &AppConfig{
				Modbus: ModbusSettings{Type: "rtu", BaudRate: 9600},
				App:    AppSettings{Port: 8080},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "serial_port is required",
		},
		{
			name: "invalid synchronous mode",
			config: &AppConfig{
				App:    AppSettings{Port: 8080},
				Modbus: ModbusSettings{Type: "tcp", Host: "192.168.1.100", Port: 502},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "INVALID",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "invalid synchronous mode",
		},
		{
			name: "invalid temp_store",
			config: &AppConfig{
				App:    AppSettings{Port: 8080},
				Modbus: ModbusSettings{Type: "tcp", Host: "192.168.1.100", Port: 502},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "INVALID",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "invalid temp_store",
		},
		{
			name: "zero poller interval",
			config: &AppConfig{
				App:    AppSettings{Port: 8080},
				Modbus: ModbusSettings{Type: "tcp", Host: "192.168.1.100", Port: 502},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      0,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "interval must be positive",
		},
		{
			name: "zero block attempts",
			config: &AppConfig{
				App:    AppSettings{Port: 8080},
				Modbus: ModbusSettings{Type: "tcp", Host: "192.168.1.100", Port: 502},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 0,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "block_attempts must be at least 1",
		},
		{
			name: "zero poll timeout",
			config: &AppConfig{
				App:    AppSettings{Port: 8080},
				Modbus: ModbusSettings{Type: "tcp", Host: "192.168.1.100", Port: 502},
				Storage: StorageSettings{
					Path:           "./data/solis.db",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,

					Synchronous: "NORMAL",
					TempStore:   "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   0,
				},
			},
			wantErr:     true,
			errContains: "poll_timeout must be positive",
		},
		{
			name: "empty storage path",
			config: &AppConfig{
				App:    AppSettings{Port: 8080},
				Modbus: ModbusSettings{Type: "tcp", Host: "192.168.1.100", Port: 502},
				Storage: StorageSettings{
					Path:           "",
					DailyRetention: 365 * 24 * time.Hour,
					ErrorRetention: 30 * 24 * time.Hour,
					Synchronous:    "NORMAL",
					TempStore:      "MEMORY",
				},
				Poller: PollerSettings{
					Interval:      15 * time.Minute,
					BlockAttempts: 3,
					PollTimeout:   30 * time.Second,
				},
			},
			wantErr:     true,
			errContains: "storage path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateConfig() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestLoadConfig_WithFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	configContent := `
app:
  debug: DEBUG
  port: 8080
  timeout: 30s

modbus:
  type: tcp
  host: 192.168.1.100
  port: 502
  timeout: 5s
  unit_id: 1

storage:
  path: ./data/solis.db
  raw_retention: 168h
  min_raw_retention: 15m
  daily_retention: 8760h
  error_retention: 720h
  wal_mode: true
  synchronous: NORMAL
  temp_store: MEMORY

poller:
  interval: 15m
  block_attempts: 3
  block_retry_delay: 1s
  block_interval: 0s
  poll_timeout: 30s

metrics:
  enabled: false
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load the config
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify loaded values
	if config.App.Port != 8080 {
		t.Errorf("App.Port = %v, want %v", config.App.Port, 8080)
	}
	if config.App.Debug != "DEBUG" {
		t.Errorf("App.Debug = %v, want %v", config.App.Debug, "DEBUG")
	}
	if config.Modbus.Host != "192.168.1.100" {
		t.Errorf("Modbus.Host = %v, want %v", config.Modbus.Host, "192.168.1.100")
	}
	if config.Modbus.Type != "tcp" {
		t.Errorf("Modbus.Type = %v, want %v", config.Modbus.Type, "tcp")
	}
	if config.Storage.Path != "./data/solis.db" {
		t.Errorf("Storage.Path = %v, want %v", config.Storage.Path, "./data/solis.db")
	}
	if config.Poller.Interval != 15*time.Minute {
		t.Errorf("Poller.Interval = %v, want %v", config.Poller.Interval, 15*time.Minute)
	}
}

func TestLoadConfig_WithEnvVars(t *testing.T) {
	// Set environment variables
	os.Setenv("SOLIS_APP_DEBUG", "DEBUG")
	os.Setenv("SOLIS_APP_PORT", "9090")
	os.Setenv("SOLIS_MODBUS_HOST", "10.0.0.1")
	os.Setenv("SOLIS_MODBUS_PORT", "503")
	defer func() {
		os.Unsetenv("SOLIS_APP_DEBUG")
		os.Unsetenv("SOLIS_APP_PORT")
		os.Unsetenv("SOLIS_MODBUS_HOST")
		os.Unsetenv("SOLIS_MODBUS_PORT")
	}()

	// Create a minimal config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	configContent := `
app:
  timeout: 30s
modbus:
  type: tcp
  host: 192.168.1.100
  port: 502
  timeout: 5s
  unit_id: 1
storage:
  path: ./data/solis.db
  raw_retention: 168h
  min_raw_retention: 15m
  daily_retention: 8760h
  error_retention: 720h
  wal_mode: true
  synchronous: NORMAL
  temp_store: MEMORY
poller:
  interval: 15m
  block_attempts: 3
  block_retry_delay: 1s
  block_interval: 0s
  poll_timeout: 30s
metrics:
  enabled: false
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load the config
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify environment variables were used
	if config.App.Debug != "DEBUG" {
		t.Errorf("App.Debug = %v, want %v", config.App.Debug, "DEBUG")
	}
	if config.App.Port != 9090 {
		t.Errorf("App.Port = %v, want %v", config.App.Port, 9090)
	}
	if config.Modbus.Host != "10.0.0.1" {
		t.Errorf("Modbus.Host = %v, want %v", config.Modbus.Host, "10.0.0.1")
	}
	if config.Modbus.Port != 503 {
		t.Errorf("Modbus.Port = %v, want %v", config.Modbus.Port, 503)
	}
}

func TestLoadConfig_WithDefaults(t *testing.T) {
	// Create an empty config file to test defaults
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "empty.yaml")

	// Write empty file
	err := os.WriteFile(configPath, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to write empty config file: %v", err)
	}

	// Load config with empty file (should use defaults)
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify defaults
	if config.App.Port != 8080 {
		t.Errorf("App.Port = %v, want %v (default)", config.App.Port, 8080)
	}
	if config.App.Debug != "INFO" {
		t.Errorf("App.Debug = %v, want %v (default)", config.App.Debug, "INFO")
	}
	if config.Modbus.Host != "192.168.1.100" {
		t.Errorf("Modbus.Host = %v, want %v (default)", config.Modbus.Host, "192.168.1.100")
	}
	if config.Modbus.Port != 502 {
		t.Errorf("Modbus.Port = %v, want %v (default)", config.Modbus.Port, 502)
	}
	if config.Storage.Path != "./data/solis.db" {
		t.Errorf("Storage.Path = %v, want %v (default)", config.Storage.Path, "./data/solis.db")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	// Create a temporary config file with invalid YAML
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.yaml")

	// Write invalid YAML
	configContent := `
app:
  port: [8080
  invalid yaml syntax
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config should fail
	_, err = LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig() expected error for invalid YAML, got nil")
	}
	// The error might be from read or unmarshal, just check it's not nil
	if err == nil {
		t.Fatal("LoadConfig() expected error for invalid YAML, got nil")
	}
}

func TestLoadConfig_InvalidFile(t *testing.T) {
	// Try to load from a directory instead of a file
	tempDir := t.TempDir()

	// Load config from directory should fail
	_, err := LoadConfig(tempDir)
	if err == nil {
		t.Fatal("LoadConfig() expected error for directory, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read config file") {
		t.Errorf("LoadConfig() error = %v, expected to contain 'failed to read config file'", err)
	}
}
