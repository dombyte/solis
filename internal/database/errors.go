package database

import (
	"errors"
	"fmt"
)

// ErrSchemaTooOld is returned by Prepare for databases older than MinCompatibleVersion.
var ErrSchemaTooOld = errors.New("database schema too old")

// SchemaTooOldError carries the schema version of a database that cannot be migrated.
type SchemaTooOldError struct {
	// Version is the schema version found in the database (0 = no schema_version table).
	Version int
}

// Error describes the version found and the required upgrade path.
func (e *SchemaTooOldError) Error() string {
	return fmt.Sprintf("database: schema version %d is older than %d; "+
		"upgrade with a v2 release first", e.Version, MinCompatibleVersion)
}

// Unwrap returns ErrSchemaTooOld.
func (e *SchemaTooOldError) Unwrap() error { return ErrSchemaTooOld }
