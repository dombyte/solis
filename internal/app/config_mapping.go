package app

import (
	"fmt"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/modbus"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/storage"
)

// ConfigRules returns the config validations owned by other packages; pass them to
// config.LoadConfig so config itself stays free of domain imports.
func ConfigRules() []config.Rule {
	return []config.Rule{validateModbus, validateRollover, validatePollTimeout}
}

// modbusSettings converts the config section into the Modbus client's settings.
func modbusSettings(m config.ModbusSettings) modbus.Settings {
	return modbus.Settings{Address: m.Address, UnitID: m.SlaveID, Timeout: m.Timeout,
		Speed: m.Speed, DataBits: m.DataBits, Parity: m.Parity, StopBits: m.StopBits}
}

// validateModbus applies the Modbus client's own settings rules.
func validateModbus(c *config.AppConfig) error {
	return modbusSettings(c.Modbus).Validate()
}

// validateRollover requires a strict 24-hour HH:MM rollover time.
func validateRollover(c *config.AppConfig) error {
	_, err := period.ParseRollover(c.Rollover.Time)
	return err
}

// validatePollTimeout keeps one poll cycle inside the supervisor's healthy grace: the
// poller beats at cycle start and end, so a longer cycle would restart a working poller.
func validatePollTimeout(c *config.AppConfig) error {
	grace := health.HealthyGraceFactor * c.Poller.Interval
	if c.Poller.PollTimeout >= grace {
		return fmt.Errorf("poll_timeout %s must be below %d x poller.interval (%s)",
			c.Poller.PollTimeout, health.HealthyGraceFactor, grace)
	}
	return nil
}

// StorageSettings maps the storage section onto the storage package's settings.
func StorageSettings(s config.StorageSettings) storage.Settings {
	return storage.Settings{Path: s.Path, DailyRetention: s.DailyRetention,
		ErrorRetention: s.ErrorRetention, WalMode: s.WalMode, Synchronous: s.Synchronous,
		TempStore: s.TempStore}
}

// DatabaseSettings maps the storage section onto the database manager's settings.
func DatabaseSettings(s config.StorageSettings) database.Settings {
	return database.Settings{Path: s.Path, CleanupInterval: s.CleanupInterval}
}
