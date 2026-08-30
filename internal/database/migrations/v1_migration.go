package migrations

import "database/sql"

// V1Migration implements the Migration interface for V1 schema (initial schema).
type V1Migration struct{}

// Version returns the target version for this migration.
func (m *V1Migration) Version() int {
	return 1
}

// Description returns a human-readable description of what this migration does.
func (m *V1Migration) Description() string {
	return "Initial schema"
}

// Up applies the V1 migration to create the initial database schema.
func (m *V1Migration) Up(tx *sql.Tx) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS daily_values (id INTEGER PRIMARY KEY AUTOINCREMENT, date DATE NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL, raw_value REAL NOT NULL, UNIQUE(register_key, date));`,
		`CREATE INDEX IF NOT EXISTS idx_daily_key_date ON daily_values(register_key, date);`,
		`CREATE INDEX IF NOT EXISTS idx_daily_date ON daily_values(date);`,
		`CREATE TABLE IF NOT EXISTS monthly_values (id INTEGER PRIMARY KEY AUTOINCREMENT, month TEXT NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL, raw_value REAL NOT NULL, UNIQUE(register_key, month));`,
		`CREATE INDEX IF NOT EXISTS idx_monthly_key_month ON monthly_values(register_key, month);`,
		`CREATE INDEX IF NOT EXISTS idx_monthly_month ON monthly_values(month);`,
		`CREATE TABLE IF NOT EXISTS yearly_values (id INTEGER PRIMARY KEY AUTOINCREMENT, year TEXT NOT NULL, register_key TEXT NOT NULL, value REAL NOT NULL, raw_value REAL NOT NULL, UNIQUE(register_key, year));`,
		`CREATE INDEX IF NOT EXISTS idx_yearly_key_year ON yearly_values(register_key, year);`,
		`CREATE INDEX IF NOT EXISTS idx_yearly_year ON yearly_values(year);`,
		`CREATE TABLE IF NOT EXISTS total_values (id INTEGER PRIMARY KEY AUTOINCREMENT, register_key TEXT NOT NULL UNIQUE, value REAL NOT NULL, raw_value REAL NOT NULL, timestamp DATETIME NOT NULL);`,
		`CREATE INDEX IF NOT EXISTS idx_total_key ON total_values(register_key);`,
		`CREATE TABLE IF NOT EXISTS error_data (id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp DATETIME NOT NULL, register_key TEXT NOT NULL, raw_value REAL NOT NULL, string_value TEXT, UNIQUE(register_key, timestamp));`,
		`CREATE INDEX IF NOT EXISTS idx_error_key_timestamp ON error_data(register_key, timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_error_timestamp ON error_data(timestamp);`,
		SchemaVersionTableSQL,
	}

	for _, sql := range tables {
		if _, err := tx.Exec(sql); err != nil {
			return err
		}
	}

	// Insert version record
	insertSQL := `INSERT OR IGNORE INTO schema_version (version, description, applied_at, success) VALUES (?, ?, CURRENT_TIMESTAMP, 1)`
	_, err := tx.Exec(insertSQL, 1, "Initial schema")
	return err
}

// Down is not implemented for V1 migration.
func (m *V1Migration) Down(tx *sql.Tx) error {
	return ErrNotImplemented
}

// GetV1Migration returns a new V1Migration instance.
func GetV1Migration() Migration {
	return &V1Migration{}
}
