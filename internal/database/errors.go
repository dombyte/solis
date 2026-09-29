package database

import (
	"errors"
	"fmt"
)

// ErrSchemaTooOld is returned by Prepare for databases older than MinCompatibleVersion.
var ErrSchemaTooOld = errors.New("database schema too old")

// ErrSchemaTooNew is returned by Prepare for databases written by a newer version.
var ErrSchemaTooNew = errors.New("database schema is newer than this version")

// SchemaTooNewError carries the schema version of a database a newer release migrated.
type SchemaTooNewError struct {
	// Version is the database's schema version.
	Version int
}

func (e *SchemaTooNewError) Error() string {
	return fmt.Sprintf("%v: schema version %d, this version supports up to %d; "+
		"run the newer release or restore a backup", ErrSchemaTooNew, e.Version,
		CurrentSchemaVersion)
}

// Unwrap returns ErrSchemaTooNew.
func (e *SchemaTooNewError) Unwrap() error { return ErrSchemaTooNew }

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
