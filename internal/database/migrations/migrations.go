// Package migrations contains all database migrations for the Solis monitor application.
package migrations

import (
	"database/sql"
	"fmt"
)

// KeyMapping defines the mapping from old register keys to new standardized keys.
// This is used by the V2 migration to rename register keys in all tables.
var KeyMapping = map[string]string{
	// Energy & Consumption
	"pv_today_energy":                 "pv_energy_daily",
	"pv_month_energy":                 "pv_energy_monthly",
	"pv_year_energy":                  "pv_energy_yearly",
	"pv_total_energy":                 "pv_energy_total",
	"today_energy_consumption":        "energy_consumption_daily",
	"total_energy_consumption":        "energy_consumption_total",
	"today_energy_fed_into_grid":      "grid_export_daily",
	"total_energy_fed_into_grid":      "grid_export_total",
	"today_energy_imported_from_grid": "grid_import_daily",
	"total_energy_imported_from_grid": "grid_import_total",

	// Battery
	"today_battery_charge_energy":    "battery_charge_daily",
	"today_battery_discharge_energy": "battery_discharge_daily",
	"total_battery_charge_energy":    "battery_charge_total",
	"total_battery_discharge_energy": "battery_discharge_total",

	// Household & Backup (load prefix removed)
	"household_load_today_energy": "household_energy_daily",
	"household_load_month_energy": "household_energy_monthly",
	"household_load_year_energy":  "household_energy_yearly",
	"household_load_total_energy": "household_energy_total",
	"backup_load_today_energy":    "backup_energy_daily",
	"backup_load_month_energy":    "backup_energy_monthly",
	"backup_load_year_energy":     "backup_energy_yearly",
	"backup_load_total_energy":    "backup_energy_total",

	// Status Registers (keep as-is)
	"solis_status":     "solis_status",
	"operating_status": "operating_status",

	// Fault Status (simplified)
	"grid_fault_status_01":        "grid_fault_1",
	"backup_load_fault_status_02": "backup_fault_2",
	"battery_fault_status_03":     "battery_fault_3",
	"device_fault_status_04":      "device_fault_4",
	"device_fault_status_05":      "device_fault_5",
	"battery_fault_status_1_bms":  "battery_fault_1_bms",
	"battery_fault_status_2_bms":  "battery_fault_2_bms",

	// Power (removed - no longer polled)
	// "total_pv_power":     "pv_power_total",
	// "ac_grid_port_power": "ac_grid_power",

	// Net Grid (Computed)
	"today_grid_energy": "grid_energy_daily",
	"total_grid_energy": "grid_energy_total",

	// Information Registers (keep as-is)
	"solis_model_no":         "solis_model_no",
	"solis_dsp_version":      "solis_dsp_version",
	"solis_hmi_version":      "solis_hmi_version",
	"solis_protocol_version": "solis_protocol_version",
	"solis_serial_number":    "solis_serial_number",

	// Computed monthly registers (old names)
	"energy_consumption_month_energy":        "energy_consumption_monthly",
	"energy_fed_into_grid_month_energy":      "grid_export_monthly",
	"energy_imported_from_grid_month_energy": "grid_import_monthly",
	"battery_discharge_month_energy":         "battery_discharge_monthly",
	"battery_charge_month_energy":            "battery_charge_monthly",
	"month_grid_energy":                      "grid_energy_monthly",

	// Computed yearly registers (old names)
	"energy_consumption_year_energy":        "energy_consumption_yearly",
	"energy_fed_into_grid_year_energy":      "grid_export_yearly",
	"energy_imported_from_grid_year_energy": "grid_import_yearly",
	"battery_discharge_year_energy":         "battery_discharge_yearly",
	"battery_charge_year_energy":            "battery_charge_yearly",
	"year_grid_energy":                      "grid_energy_yearly",
}

// ReverseKeyMapping creates a mapping from new keys back to old keys for lookup.
// This is useful for checking if a key was renamed.
var ReverseKeyMapping = func() map[string]string {
	reverse := make(map[string]string)
	for oldKey, newKey := range KeyMapping {
		if oldKey != newKey { // Only include actual renames
			reverse[newKey] = oldKey
		}
	}
	return reverse
}()

// TablesToMigrate contains the list of tables that have register_key columns to update.
var TablesToMigrate = []string{
	"daily_values",
	"monthly_values",
	"yearly_values",
	"total_values",
	"error_data",
}

// V2Migration implements the Migration interface for V2 schema (register key renaming).
type V2Migration struct{}

// Version returns the migration version (2).
func (m *V2Migration) Version() int {
	return 2
}

// Description returns a description of what this migration does.
func (m *V2Migration) Description() string {
	return "Register key standardization - rename old keys to new standardized naming pattern"
}

// Up applies the V2 migration to rename register keys in all relevant tables.
// This uses a transaction-based approach with temporary tables for safety.
func (m *V2Migration) Up(tx *sql.Tx) error {
	for _, table := range TablesToMigrate {
		if err := migrateTable(tx, table); err != nil {
			return fmt.Errorf("failed to migrate table %s: %w", table, err)
		}
	}
	return nil
}

// migrateTable performs the key renaming migration for a single table.
// It uses SQLite's UPDATE statements directly since SQLite supports ALTER TABLE
// but for data migrations, UPDATE is more straightforward.
func migrateTable(tx *sql.Tx, tableName string) error {
	// For each old key that needs to be renamed, update all rows with that key
	for oldKey, newKey := range KeyMapping {
		// Skip if the key hasn't changed
		if oldKey == newKey {
			continue
		}

		// Check if the table has the register_key column
		var columnCount int
		err := tx.QueryRow(
			fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = 'register_key'", tableName),
		).Scan(&columnCount)
		if err != nil {
			return fmt.Errorf("failed to check for register_key column in %s: %w", tableName, err)
		}

		if columnCount == 0 {
			// Table doesn't have register_key column, skip it
			continue
		}

		// Perform the update for this table
		updateSQL := fmt.Sprintf("UPDATE %s SET register_key = ? WHERE register_key = ?", tableName)
		_, err = tx.Exec(updateSQL, newKey, oldKey)
		if err != nil {
			return fmt.Errorf("failed to update key from %s to %s in %s: %w", oldKey, newKey, tableName, err)
		}
	}

	return nil
}

// Down reverts the V2 migration by renaming keys back to their original names.
// This is provided for development/debugging purposes.
func (m *V2Migration) Down(tx *sql.Tx) error {
	// Create reverse mapping for rollback
	reverseMapping := make(map[string]string)
	for oldKey, newKey := range KeyMapping {
		if oldKey != newKey {
			reverseMapping[newKey] = oldKey
		}
	}

	for _, table := range TablesToMigrate {
		// Check if the table has the register_key column
		var columnCount int
		err := tx.QueryRow(
			fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = 'register_key'", table),
		).Scan(&columnCount)
		if err != nil {
			return fmt.Errorf("failed to check for register_key column in %s: %w", table, err)
		}

		if columnCount == 0 {
			// Table doesn't have register_key column, skip it
			continue
		}

		for newKey, oldKey := range reverseMapping {
			// Perform the rollback update for this table
			updateSQL := fmt.Sprintf("UPDATE %s SET register_key = ? WHERE register_key = ?", table)
			_, err = tx.Exec(updateSQL, oldKey, newKey)
			if err != nil {
				return fmt.Errorf("failed to rollback key from %s to %s in %s: %w", newKey, oldKey, table, err)
			}
		}
	}

	return nil
}

// GetV2Migration returns a new V2Migration instance.
// This can be used for registration with the migration registry.
func GetV2Migration() Migration {
	return &V2Migration{}
}
