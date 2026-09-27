package migrations

import "database/sql"

// V3MetaTableSQL creates the key/value meta table used by v3 for the cutover date,
// closed-period watermarks and the total baseline. Existing tables stay byte-identical.
const V3MetaTableSQL = `CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY, value TEXT NOT NULL)`

// V3Migration adds the meta table (Plan.md D1).
type V3Migration struct{}

// Version returns the migration version (3).
func (m *V3Migration) Version() int { return 3 }

// Description returns a description of what this migration does.
func (m *V3Migration) Description() string {
	return "v3 meta table (cutover date, closed-period watermarks, total baseline)"
}

// Up creates the meta table.
func (m *V3Migration) Up(tx *sql.Tx) error {
	_, err := tx.Exec(V3MetaTableSQL)
	return err
}

// Down drops the meta table.
func (m *V3Migration) Down(tx *sql.Tx) error {
	_, err := tx.Exec(`DROP TABLE IF EXISTS meta`)
	return err
}

// GetV3Migration returns a new V3Migration instance.
func GetV3Migration() Migration {
	return &V3Migration{}
}
