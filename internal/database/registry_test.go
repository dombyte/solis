package database

import (
	"database/sql"
	"testing"
)

// Mock migration for testing
type mockMigration struct {
	version    int
	description string
	upCalled    bool
	downCalled  bool
}

func (m *mockMigration) Version() int {
	return m.version
}

func (m *mockMigration) Description() string {
	return m.description
}

func (m *mockMigration) Up(tx *sql.Tx) error {
	m.upCalled = true
	return nil
}

func (m *mockMigration) Down(tx *sql.Tx) error {
	m.downCalled = true
	return nil
}

func TestMigrationRegistry(t *testing.T) {
	registry := NewMigrationRegistry()

	// Test empty registry
	if registry.Count() != 0 {
		t.Errorf("Expected empty registry, got count: %d", registry.Count())
	}
	if registry.LatestVersion() != 0 {
		t.Errorf("Expected latest version 0 for empty registry, got: %d", registry.LatestVersion())
	}

	// Test registering migrations
	migration1 := &mockMigration{version: 1, description: "Migration 1"}
	migration2 := &mockMigration{version: 2, description: "Migration 2"}
	migration3 := &mockMigration{version: 3, description: "Migration 3"}

	registry.Register(migration1)
	registry.Register(migration2)
	registry.Register(migration3)

	if registry.Count() != 3 {
		t.Errorf("Expected 3 migrations, got: %d", registry.Count())
	}

	if registry.LatestVersion() != 3 {
		t.Errorf("Expected latest version 3, got: %d", registry.LatestVersion())
	}

	// Test getting specific migration
	got := registry.GetMigration(2)
	if got == nil {
		t.Fatal("Expected to get migration 2")
	}
	if got.Version() != 2 {
		t.Errorf("Expected version 2, got: %d", got.Version())
	}

	// Test getting non-existent migration
	got = registry.GetMigration(999)
	if got != nil {
		t.Error("Expected nil for non-existent migration")
	}

	// Test duplicate registration
	registry.Register(migration1) // Should be ignored
	if registry.Count() != 3 {
		t.Errorf("Expected still 3 migrations after duplicate, got: %d", registry.Count())
	}
}

func TestGetMigrationsFrom(t *testing.T) {
	registry := NewMigrationRegistry()

	// Register migrations 1, 2, 3, 5
	registry.Register(&mockMigration{version: 1, description: "V1"})
	registry.Register(&mockMigration{version: 2, description: "V2"})
	registry.Register(&mockMigration{version: 3, description: "V3"})
	registry.Register(&mockMigration{version: 5, description: "V5"})

	// Test getting migrations from version 0 (should get all)
	migrations := registry.GetMigrationsFrom(0)
	if len(migrations) != 4 {
		t.Errorf("Expected 4 migrations from 0, got: %d", len(migrations))
	}

	// Test getting migrations from version 2 (should get 3, 5)
	migrations = registry.GetMigrationsFrom(2)
	if len(migrations) != 2 {
		t.Errorf("Expected 2 migrations from 2, got: %d", len(migrations))
	}
	if migrations[0].Version() != 3 || migrations[1].Version() != 5 {
		t.Errorf("Expected migrations 3 and 5, got: %d and %d", migrations[0].Version(), migrations[1].Version())
	}

	// Test getting migrations from version 5 (should get none)
	migrations = registry.GetMigrationsFrom(5)
	if len(migrations) != 0 {
		t.Errorf("Expected 0 migrations from 5, got: %d", len(migrations))
	}

	// Test getting migrations from version 10 (should get none)
	migrations = registry.GetMigrationsFrom(10)
	if len(migrations) != 0 {
		t.Errorf("Expected 0 migrations from 10, got: %d", len(migrations))
	}
}

func TestGetMigrationsTo(t *testing.T) {
	registry := NewMigrationRegistry()

	// Register migrations 1, 2, 3, 5
	registry.Register(&mockMigration{version: 1, description: "V1"})
	registry.Register(&mockMigration{version: 2, description: "V2"})
	registry.Register(&mockMigration{version: 3, description: "V3"})
	registry.Register(&mockMigration{version: 5, description: "V5"})

	// Test getting migrations to version 3 (should get 1, 2, 3)
	migrations := registry.GetMigrationsTo(3)
	if len(migrations) != 3 {
		t.Errorf("Expected 3 migrations to 3, got: %d", len(migrations))
	}

	// Test getting migrations to version 0 (should get none)
	migrations = registry.GetMigrationsTo(0)
	if len(migrations) != 0 {
		t.Errorf("Expected 0 migrations to 0, got: %d", len(migrations))
	}
}

func TestGetVersions(t *testing.T) {
	registry := NewMigrationRegistry()

	// Register migrations out of order
	registry.Register(&mockMigration{version: 3, description: "V3"})
	registry.Register(&mockMigration{version: 1, description: "V1"})
	registry.Register(&mockMigration{version: 2, description: "V2"})

	versions := registry.GetVersions()
	if len(versions) != 3 {
		t.Errorf("Expected 3 versions, got: %d", len(versions))
	}

	// Check that versions are sorted
	for i := 1; i < len(versions); i++ {
		if versions[i-1] >= versions[i] {
			t.Errorf("Versions not sorted. Index %d: %d, Index %d: %d", i-1, versions[i-1], i, versions[i])
		}
	}

	// Check exact order
	if versions[0] != 1 || versions[1] != 2 || versions[2] != 3 {
		t.Errorf("Expected versions [1, 2, 3], got: %v", versions)
	}
}

func TestSimpleMigration(t *testing.T) {
	upCalled := false
	migration := NewSimpleMigration(1, "Test migration", func(tx *sql.Tx) error {
		upCalled = true
		return nil
	})

	if migration.Version() != 1 {
		t.Errorf("Expected version 1, got: %d", migration.Version())
	}

	if migration.Description() != "Test migration" {
		t.Errorf("Expected description 'Test migration', got: %s", migration.Description())
	}

	// Test Up method
	tx, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer tx.Close()

	// Need a real transaction
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer testDB.Close()

	actualTx, err := testDB.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	if err := migration.Up(actualTx); err != nil {
		t.Errorf("Up method failed: %v", err)
	}
	actualTx.Commit()

	if !upCalled {
		t.Error("Up method was not called")
	}

	// Test Down method
	if err := migration.Down(nil); err != ErrNotImplemented {
		t.Errorf("Expected ErrNotImplemented for Down, got: %v", err)
	}
}

func TestMigrationInterface(t *testing.T) {
	// Test that SimpleMigration implements Migration interface
	var _ Migration = (*SimpleMigration)(nil)

	// Test that mockMigration implements Migration interface
	var _ Migration = (*mockMigration)(nil)
}

func TestSchemaVersionConstants(t *testing.T) {
	// These tests ensure the constants are set to expected values
	// They will need to be updated when new migrations are added
	if CurrentSchemaVersion < 1 {
		t.Errorf("CurrentSchemaVersion should be at least 1, got: %d", CurrentSchemaVersion)
	}

	if MinCompatibleVersion > CurrentSchemaVersion {
		t.Errorf("MinCompatibleVersion should not be greater than CurrentSchemaVersion")
	}

	if MinCompatibleVersion < 1 {
		t.Errorf("MinCompatibleVersion should be at least 1, got: %d", MinCompatibleVersion)
	}
}

func TestSchemaVersionTableSQL(t *testing.T) {
	// Test that the schema version table SQL is valid
	// We'll try to execute it on an in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(SchemaVersionTableSQL); err != nil {
		t.Fatalf("Failed to execute schema version table SQL: %v", err)
	}

	// Verify table was created
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&count); err != nil {
		t.Fatalf("Failed to check for schema_version table: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 schema_version table, got: %d", count)
	}
}